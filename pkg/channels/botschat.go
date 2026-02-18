package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const (
	botschatStatusInterval = 25 * time.Second
	botschatMinBackoff     = 1 * time.Second
	botschatMaxBackoff     = 30 * time.Second
)

type BotsChatChannel struct {
	*BaseChannel
	config     config.BotsChatConfig
	mu         sync.Mutex
	conn       *websocket.Conn
	connected  bool
	e2eKey     []byte
	stopCancel context.CancelFunc
	cloudURL   string
}

func NewBotsChatChannel(cfg config.BotsChatConfig, messageBus *bus.MessageBus) (*BotsChatChannel, error) {
	accountID := cfg.AccountID
	if accountID == "" {
		accountID = "default"
	}
	allowList := make([]string, 0, len(cfg.AllowFrom))
	for _, s := range cfg.AllowFrom {
		allowList = append(allowList, s)
	}
	base := NewBaseChannel("botschat", cfg, messageBus, allowList)
	return &BotsChatChannel{
		BaseChannel: base,
		config:      cfg,
		cloudURL:    strings.TrimPrefix(strings.TrimPrefix(cfg.CloudURL, "https://"), "http://"),
	}, nil
}

func (c *BotsChatChannel) buildWSURL() string {
	scheme := "wss"
	if strings.HasPrefix(c.config.CloudURL, "http://") {
		scheme = "ws"
	}
	accountID := c.config.AccountID
	if accountID == "" {
		accountID = "default"
	}
	return fmt.Sprintf("%s://%s/api/gateway/%s?token=%s",
		scheme, c.cloudURL, accountID, url.QueryEscape(c.config.PairingToken))
}

// redactURLForLog returns a URL safe for logging (token query param masked).
func redactURLForLog(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid-url>"
	}
	if q := u.Query(); q.Get("token") != "" {
		q.Set("token", "***")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (c *BotsChatChannel) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	c.stopCancel = cancel
	go c.run(runCtx)
	return nil
}

func (c *BotsChatChannel) Stop(ctx context.Context) error {
	if c.stopCancel != nil {
		c.stopCancel()
		c.stopCancel = nil
	}
	c.mu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	c.e2eKey = nil
	c.mu.Unlock()
	c.setRunning(false)
	logger.InfoC("botschat", "BotsChat channel stopped")
	return nil
}

func (c *BotsChatChannel) run(ctx context.Context) {
	backoff := botschatMinBackoff
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		wsURL := c.buildWSURL()
		logger.InfoCF("botschat", "Connecting to BotsChat cloud", map[string]interface{}{"url": redactURLForLog(wsURL)})

		dialer := websocket.DefaultDialer
		dialer.HandshakeTimeout = 10 * time.Second
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			logger.ErrorCF("botschat", "Failed to connect", map[string]interface{}{"error": err.Error()})
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < botschatMaxBackoff {
					backoff *= 2
					if backoff > botschatMaxBackoff {
						backoff = botschatMaxBackoff
					}
				}
			}
			continue
		}

		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()
		backoff = botschatMinBackoff

		if err := c.sendAuth(); err != nil {
			_ = conn.Close()
			continue
		}

		tickerDone := make(chan struct{})
		go c.statusTicker(conn, tickerDone)

		c.readLoop(ctx, conn)
		close(tickerDone)
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
			c.connected = false
			c.e2eKey = nil
		}
		c.mu.Unlock()
		c.setRunning(false)
		_ = conn.Close()

		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			if backoff < botschatMaxBackoff {
				backoff *= 2
				if backoff > botschatMaxBackoff {
					backoff = botschatMaxBackoff
				}
			}
		}
	}
}

func (c *BotsChatChannel) sendAuth() error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("no connection")
	}
	auth := map[string]interface{}{
		"type":   "auth",
		"token":  c.config.PairingToken,
		"agents": nil,
		"model":  "",
	}
	data, err := json.Marshal(auth)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (c *BotsChatChannel) statusTicker(conn *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(botschatStatusInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			c.mu.Lock()
			cur := c.conn
			c.mu.Unlock()
			if cur != conn || cur == nil {
				return
			}
			status := map[string]interface{}{
				"type":      "status",
				"connected": true,
				"agents":    []string{},
				"model":     "",
			}
			data, _ := json.Marshal(status)
			_ = cur.WriteMessage(websocket.TextMessage, data)
		}
	}
}

func (c *BotsChatChannel) readLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, message, err := conn.ReadMessage()
		if err != nil {
			logger.WarnCF("botschat", "Read error", map[string]interface{}{"error": err.Error()})
			return
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			logger.WarnCF("botschat", "Invalid JSON", map[string]interface{}{"error": err.Error()})
			continue
		}
		msgType, _ := msg["type"].(string)
		switch msgType {
		case "ping":
			c.send(conn, map[string]interface{}{"type": "pong"})
		case "auth.ok":
			c.mu.Lock()
			c.connected = true
			c.mu.Unlock()
			c.setRunning(true)
			logger.InfoC("botschat", "Authenticated with BotsChat cloud")
			if userId, ok := msg["userId"].(string); ok && userId != "" && c.config.E2EPassword != "" {
				go c.deriveE2EKey(userId)
			}
		case "auth.fail":
			reason, _ := msg["reason"].(string)
			logger.ErrorCF("botschat", "Auth failed", map[string]interface{}{"reason": reason})
			return
		case "user.message":
			c.handleUserMessage(msg)
		case "user.media":
			c.handleUserMedia(msg)
		case "user.command":
			c.handleUserCommand(msg)
		case "user.action":
			c.handleUserAction(msg)
		case "models.request":
			c.send(conn, map[string]interface{}{"type": "models.list", "models": []interface{}{}})
		case "settings.defaultModel":
			logger.DebugCF("botschat", "Unhandled message type", map[string]interface{}{"type": msgType})
		default:
			// ignore
		}
	}
}

func (c *BotsChatChannel) deriveE2EKey(userId string) {
	key := deriveE2EKey(c.config.E2EPassword, userId)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.e2eKey = key
	logger.InfoC("botschat", "E2E key derived")
}

func (c *BotsChatChannel) send(conn *websocket.Conn, payload map[string]interface{}) {
	if conn == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = conn.WriteMessage(websocket.TextMessage, data)
}

func (c *BotsChatChannel) handleUserMessage(msg map[string]interface{}) {
	sessionKey, _ := msg["sessionKey"].(string)
	text, _ := msg["text"].(string)
	userId, _ := msg["userId"].(string)
	messageId, _ := msg["messageId"].(string)
	encrypted, _ := msg["encrypted"].(bool)

	if encrypted {
		c.mu.Lock()
		key := c.e2eKey
		c.mu.Unlock()
		if key != nil {
			cipherBytes, err := e2eFromBase64(text)
			if err == nil {
				decrypted, err := decryptE2EText(key, cipherBytes, messageId)
				if err == nil {
					text = decrypted
				} else {
					logger.WarnCF("botschat", "Decryption failed", map[string]interface{}{"messageId": messageId, "error": err.Error()})
					text = "[Decryption Failed]"
				}
			}
		}
	}

	metadata := make(map[string]string)
	if messageId != "" {
		metadata["message_id"] = messageId
	}
	if sessionKey == "" {
		sessionKey = userId
	}
	c.HandleMessage(userId, sessionKey, text, nil, metadata)
}

func (c *BotsChatChannel) handleUserMedia(msg map[string]interface{}) {
	sessionKey, _ := msg["sessionKey"].(string)
	userId, _ := msg["userId"].(string)
	// Treat as user.message with empty text; no media passed to agent
	fake := map[string]interface{}{
		"sessionKey": sessionKey,
		"text":       "",
		"userId":     userId,
		"messageId":  "media-" + fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	c.handleUserMessage(fake)
}

func (c *BotsChatChannel) handleUserCommand(msg map[string]interface{}) {
	sessionKey, _ := msg["sessionKey"].(string)
	userId := "command"
	command, _ := msg["command"].(string)
	args, _ := msg["args"].(string)
	text := "/" + command
	if args != "" {
		text += " " + args
	}
	fake := map[string]interface{}{
		"sessionKey": sessionKey,
		"text":       text,
		"userId":     userId,
		"messageId":  "cmd-" + fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	c.handleUserMessage(fake)
}

func (c *BotsChatChannel) handleUserAction(msg map[string]interface{}) {
	sessionKey, _ := msg["sessionKey"].(string)
	action, _ := msg["action"].(string)
	params, _ := msg["params"].(map[string]interface{})
	kind := action
	if k, ok := params["kind"].(string); ok && k != "" {
		kind = k
	}
	value, _ := params["value"].(string)
	if v, ok := params["selected"].(string); ok && v != "" {
		value = v
	}
	label := value
	if l, ok := params["label"].(string); ok && l != "" {
		label = l
	}
	text := fmt.Sprintf("[Action: kind=%s] User selected: %q", kind, label)
	fake := map[string]interface{}{
		"sessionKey": sessionKey,
		"text":       text,
		"userId":     "action",
		"messageId":  "action-" + fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	c.handleUserMessage(fake)
}

func (c *BotsChatChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	c.mu.Lock()
	conn := c.conn
	connected := c.connected
	key := c.e2eKey
	c.mu.Unlock()

	if !connected || conn == nil {
		return fmt.Errorf("botschat: not connected")
	}

	messageId := uuid.New().String()
	payload := map[string]interface{}{
		"type":       "agent.text",
		"sessionKey": msg.ChatID,
		"messageId":  messageId,
	}

	if key != nil {
		ciphertext, err := encryptE2EText(key, msg.Content, messageId)
		if err != nil {
			return fmt.Errorf("botschat: encrypt: %w", err)
		}
		payload["text"] = e2eToBase64(ciphertext)
		payload["encrypted"] = true
	} else {
		payload["text"] = msg.Content
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
