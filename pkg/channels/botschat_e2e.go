// Package channels — BotsChat E2E crypto compatible with botsChat/packages/plugin/src/e2e-crypto.ts.
// AES-256-CTR with PBKDF2 key derivation and HKDF-style nonce derivation.
package channels

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/pbkdf2"
)

const (
	e2eSaltPrefix = "botschat-e2e:"
	e2ePBKDF2Iter = 310_000
	e2eKeyLen     = 32
	e2eNonceLen   = 16
)

// deriveE2EKey derives a 32-byte key from password and userId using PBKDF2-SHA256.
// Salt = "botschat-e2e:" + userId; 310,000 iterations.
func deriveE2EKey(password, userId string) []byte {
	salt := e2eSaltPrefix + userId
	return pbkdf2.Key([]byte(password), []byte(salt), e2ePBKDF2Iter, e2eKeyLen, sha256.New)
}

// e2eNonce derives a 16-byte nonce for AES-CTR: HMAC-SHA256(key, "nonce-"+contextId || 0x01)[:16].
func e2eNonce(key []byte, contextId string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte("nonce-" + contextId))
	h.Write([]byte{0x01})
	return h.Sum(nil)[:e2eNonceLen]
}

// encryptE2EText encrypts plaintext with AES-256-CTR; contextId must be unique per message.
func encryptE2EText(key []byte, plaintext string, contextId string) ([]byte, error) {
	return encryptE2EBytes(key, []byte(plaintext), contextId)
}

// decryptE2EText decrypts ciphertext and returns the UTF-8 string.
func decryptE2EText(key, ciphertext []byte, contextId string) (string, error) {
	b, err := decryptE2EBytes(key, ciphertext, contextId)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// encryptE2EBytes encrypts raw bytes (e.g. for media); contextId e.g. messageId+":media".
func encryptE2EBytes(key, plaintext []byte, contextId string) ([]byte, error) {
	if len(key) != e2eKeyLen {
		return nil, errors.New("e2e: key must be 32 bytes")
	}
	nonce := e2eNonce(key, contextId)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, nonce)
	ciphertext := make([]byte, len(plaintext))
	stream.XORKeyStream(ciphertext, plaintext)
	return ciphertext, nil
}

// decryptE2EBytes decrypts raw bytes.
func decryptE2EBytes(key, ciphertext []byte, contextId string) ([]byte, error) {
	if len(key) != e2eKeyLen {
		return nil, errors.New("e2e: key must be 32 bytes")
	}
	nonce := e2eNonce(key, contextId)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, nonce)
	plaintext := make([]byte, len(ciphertext))
	stream.XORKeyStream(plaintext, ciphertext)
	return plaintext, nil
}

// e2eToBase64 encodes ciphertext for JSON transport (StdEncoding, matches Node Buffer.toString("base64")).
func e2eToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// e2eFromBase64 decodes base64 from JSON.
func e2eFromBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
