// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// MemoryPolicy defines configurable memory (MemSkill) parameters.
// Used for retrieve, session summary, long-term compress, and evolution (Phase 4).
type MemoryPolicy interface {
	RetrieveLimit() int
	RecentDays() int
	SessionSummaryMessageThreshold() int
	SessionSummaryTokenPercent() int
	SessionSummaryKeepCount() int
	SessionRelevantHistoryLimit() int   // max turns for query-based session history; 0 = disabled
	SessionRelevantFallbackKeep() int   // fallback last N messages when no match; 0 = no history when no match, nil/omit = 8
	LongTermCompressCharThreshold() int
	EvolutionEnabled() bool
}

// Default memory policy constants (match current PicoClaw behavior).
const (
	DefaultRetrieveLimit                  = 10
	DefaultRecentDays                     = 3
	DefaultSessionSummaryMessageThreshold = 20
	DefaultSessionSummaryTokenPercent     = 75
	DefaultSessionSummaryKeepCount        = 4
	DefaultSessionRelevantHistoryLimit    = 0  // 0 = feature off
	DefaultSessionRelevantFallbackKeep    = 8
)

// policyOverridesPath returns workspace/memory/policy_overrides.json.
func policyOverridesPath(workspace string) string {
	return filepath.Join(workspace, "memory", "policy_overrides.json")
}

// PolicyOverrides is a subset of memory config for workspace overrides (from reflection).
type PolicyOverrides struct {
	RetrieveLimit                  *int  `json:"retrieve_limit,omitempty"`
	RecentDays                     *int  `json:"recent_days,omitempty"`
	SessionSummaryMessageThreshold *int  `json:"session_summary_message_threshold,omitempty"`
	SessionSummaryTokenPercent     *int  `json:"session_summary_token_percent,omitempty"`
	SessionSummaryKeepCount        *int  `json:"session_summary_keep_count,omitempty"`
	SessionRelevantHistoryLimit   *int `json:"session_relevant_history_limit,omitempty"`
	SessionRelevantFallbackKeep   *int `json:"session_relevant_fallback_keep,omitempty"`
	LongTermCompressCharThreshold *int `json:"long_term_compress_char_threshold,omitempty"`
	EvolutionEnabled               *bool `json:"evolution_enabled,omitempty"`
}

func loadOverrides(workspace string) *PolicyOverrides {
	path := policyOverridesPath(workspace)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var o PolicyOverrides
	if json.Unmarshal(data, &o) != nil {
		return nil
	}
	return &o
}

// PolicyFromConfig returns a MemoryPolicy from config and optional workspace overrides. Nil cfg uses defaults.
func PolicyFromConfig(cfg *config.MemoryConfig, workspace string) MemoryPolicy {
	p := &policyImpl{
		retrieveLimit:                  DefaultRetrieveLimit,
		recentDays:                     DefaultRecentDays,
		sessionSummaryMessageThreshold: DefaultSessionSummaryMessageThreshold,
		sessionSummaryTokenPercent:     DefaultSessionSummaryTokenPercent,
		sessionSummaryKeepCount:        DefaultSessionSummaryKeepCount,
		sessionRelevantHistoryLimit:    DefaultSessionRelevantHistoryLimit,
		sessionRelevantFallbackKeep:    DefaultSessionRelevantFallbackKeep,
		longTermCompressCharThreshold: 0,
		evolutionEnabled:              false,
	}
	if cfg != nil {
		p.retrieveLimit = cfg.RetrieveLimit
		p.recentDays = cfg.RecentDays
		p.sessionSummaryMessageThreshold = cfg.SessionSummaryMessageThreshold
		p.sessionSummaryTokenPercent = cfg.SessionSummaryTokenPercent
		p.sessionSummaryKeepCount = cfg.SessionSummaryKeepCount
		p.sessionRelevantHistoryLimit = cfg.SessionRelevantHistoryLimit
		if cfg.SessionRelevantFallbackKeep != nil {
			p.sessionRelevantFallbackKeep = *cfg.SessionRelevantFallbackKeep
		}
		p.longTermCompressCharThreshold = cfg.LongTermCompressCharThreshold
		p.evolutionEnabled = cfg.EvolutionEnabled
	}
	// Merge workspace overrides
	if workspace != "" {
		if o := loadOverrides(workspace); o != nil {
			if o.RetrieveLimit != nil {
				p.retrieveLimit = *o.RetrieveLimit
			}
			if o.RecentDays != nil {
				p.recentDays = *o.RecentDays
			}
			if o.SessionSummaryMessageThreshold != nil {
				p.sessionSummaryMessageThreshold = *o.SessionSummaryMessageThreshold
			}
			if o.SessionSummaryTokenPercent != nil {
				p.sessionSummaryTokenPercent = *o.SessionSummaryTokenPercent
			}
			if o.SessionSummaryKeepCount != nil {
				p.sessionSummaryKeepCount = *o.SessionSummaryKeepCount
			}
			if o.SessionRelevantHistoryLimit != nil {
				p.sessionRelevantHistoryLimit = *o.SessionRelevantHistoryLimit
			}
			if o.SessionRelevantFallbackKeep != nil {
				p.sessionRelevantFallbackKeep = *o.SessionRelevantFallbackKeep
			}
			if o.LongTermCompressCharThreshold != nil {
				p.longTermCompressCharThreshold = *o.LongTermCompressCharThreshold
			}
			if o.EvolutionEnabled != nil {
				p.evolutionEnabled = *o.EvolutionEnabled
			}
		}
	}
	return p
}

// DefaultPolicy returns a MemoryPolicy with all defaults (full fallback, no evolution).
func DefaultPolicy() MemoryPolicy {
	return &policyImpl{
		retrieveLimit:                  DefaultRetrieveLimit,
		recentDays:                     DefaultRecentDays,
		sessionSummaryMessageThreshold: DefaultSessionSummaryMessageThreshold,
		sessionSummaryTokenPercent:     DefaultSessionSummaryTokenPercent,
		sessionSummaryKeepCount:        DefaultSessionSummaryKeepCount,
		sessionRelevantHistoryLimit:    DefaultSessionRelevantHistoryLimit,
		sessionRelevantFallbackKeep:    DefaultSessionRelevantFallbackKeep,
		longTermCompressCharThreshold: 0,
		evolutionEnabled:              false,
	}
}

type policyImpl struct {
	retrieveLimit                  int
	recentDays                     int
	sessionSummaryMessageThreshold int
	sessionSummaryTokenPercent     int
	sessionSummaryKeepCount        int
	sessionRelevantHistoryLimit    int
	sessionRelevantFallbackKeep    int
	longTermCompressCharThreshold  int
	evolutionEnabled               bool
}

func (p *policyImpl) RetrieveLimit() int {
	if p.retrieveLimit <= 0 {
		return DefaultRetrieveLimit
	}
	return p.retrieveLimit
}

func (p *policyImpl) RecentDays() int {
	if p.recentDays <= 0 {
		return DefaultRecentDays
	}
	return p.recentDays
}

func (p *policyImpl) SessionSummaryMessageThreshold() int {
	if p.sessionSummaryMessageThreshold <= 0 {
		return DefaultSessionSummaryMessageThreshold
	}
	return p.sessionSummaryMessageThreshold
}

func (p *policyImpl) SessionSummaryTokenPercent() int {
	if p.sessionSummaryTokenPercent <= 0 {
		return DefaultSessionSummaryTokenPercent
	}
	return p.sessionSummaryTokenPercent
}

func (p *policyImpl) SessionSummaryKeepCount() int {
	if p.sessionSummaryKeepCount <= 0 {
		return DefaultSessionSummaryKeepCount
	}
	return p.sessionSummaryKeepCount
}

func (p *policyImpl) SessionRelevantHistoryLimit() int {
	return p.sessionRelevantHistoryLimit
}

func (p *policyImpl) SessionRelevantFallbackKeep() int {
	return p.sessionRelevantFallbackKeep
}

func (p *policyImpl) LongTermCompressCharThreshold() int {
	return p.longTermCompressCharThreshold
}

func (p *policyImpl) EvolutionEnabled() bool {
	return p.evolutionEnabled
}

// UpdateFromReflection applies reflection output to workspace policy overrides.
// Saves a snapshot before updating. Parses JSON or key: value lines from reflectionResult.
func UpdateFromReflection(workspace string, store *MemoryStore, reflectionResult string) error {
	overrides := parseReflectionToOverrides(reflectionResult)
	if overrides == nil {
		return nil
	}
	path := policyOverridesPath(workspace)
	// Load existing overrides to merge
	current, _ := os.ReadFile(path)
	if len(current) > 0 && store != nil {
		_ = store.SavePolicySnapshot(current)
	}
	merged := loadOverrides(workspace)
	if merged == nil {
		merged = &PolicyOverrides{}
	}
	mergeOverrides(merged, overrides)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	logger.InfoCF("agent", "Memory policy updated from reflection", map[string]interface{}{
		"workspace": workspace,
	})
	return os.WriteFile(path, data, 0644)
}

func mergeOverrides(dst, src *PolicyOverrides) {
	if src.RetrieveLimit != nil {
		dst.RetrieveLimit = src.RetrieveLimit
	}
	if src.RecentDays != nil {
		dst.RecentDays = src.RecentDays
	}
	if src.SessionSummaryMessageThreshold != nil {
		dst.SessionSummaryMessageThreshold = src.SessionSummaryMessageThreshold
	}
	if src.SessionSummaryTokenPercent != nil {
		dst.SessionSummaryTokenPercent = src.SessionSummaryTokenPercent
	}
	if src.SessionSummaryKeepCount != nil {
		dst.SessionSummaryKeepCount = src.SessionSummaryKeepCount
	}
	if src.SessionRelevantHistoryLimit != nil {
		dst.SessionRelevantHistoryLimit = src.SessionRelevantHistoryLimit
	}
	if src.SessionRelevantFallbackKeep != nil {
		dst.SessionRelevantFallbackKeep = src.SessionRelevantFallbackKeep
	}
	if src.LongTermCompressCharThreshold != nil {
		dst.LongTermCompressCharThreshold = src.LongTermCompressCharThreshold
	}
	if src.EvolutionEnabled != nil {
		dst.EvolutionEnabled = src.EvolutionEnabled
	}
}

func parseReflectionToOverrides(s string) *PolicyOverrides {
	s = strings.TrimSpace(s)
	// Try JSON first (e.g. {"retrieve_limit": 15})
	var o PolicyOverrides
	if json.Unmarshal([]byte(s), &o) == nil {
		return &o
	}
	// Try key: value or key = value lines
	o = PolicyOverrides{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, sep := range []string{":", "="} {
			i := strings.Index(line, sep)
			if i <= 0 {
				continue
			}
			key := strings.TrimSpace(strings.ToLower(line[:i]))
			val := strings.TrimSpace(line[i+len(sep):])
			if n, err := strconv.Atoi(val); err == nil {
				switch key {
				case "retrieve_limit":
					o.RetrieveLimit = &n
				case "recent_days":
					o.RecentDays = &n
				case "session_summary_message_threshold":
					o.SessionSummaryMessageThreshold = &n
				case "session_summary_token_percent":
					o.SessionSummaryTokenPercent = &n
				case "session_summary_keep_count":
					o.SessionSummaryKeepCount = &n
				case "session_relevant_history_limit":
					o.SessionRelevantHistoryLimit = &n
				case "session_relevant_fallback_keep":
					o.SessionRelevantFallbackKeep = &n
				case "long_term_compress_char_threshold":
					o.LongTermCompressCharThreshold = &n
				}
			}
			if strings.HasPrefix(key, "evolution") {
				b := strings.Contains(strings.ToLower(val), "true") || val == "1"
				o.EvolutionEnabled = &b
			}
			break
		}
	}
	// Return nil if nothing parsed
	if o.RetrieveLimit == nil && o.RecentDays == nil && o.SessionSummaryMessageThreshold == nil &&
		o.SessionSummaryTokenPercent == nil && o.SessionSummaryKeepCount == nil &&
		o.SessionRelevantHistoryLimit == nil && o.SessionRelevantFallbackKeep == nil &&
		o.LongTermCompressCharThreshold == nil && o.EvolutionEnabled == nil {
		return nil
	}
	return &o
}
