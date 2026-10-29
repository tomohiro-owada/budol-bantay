// Package api: daily aggregate counters with atomic persistence.
// Counters only — never message text.
package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CounterNames is the fixed set of daily counters. ai_allowance_usd is the
// running daily sum of usage.opencode.allowance (a float, in USD); every
// other counter is an integer count stored as a number.
var CounterNames = []string{
	"req", "ai_ok", "ai_err", "quota", "rate_limited",
	"turnstile_fail", "bad_output", "injection",
	"level_hi", "level_mid", "level_lo", "level_none",
	"ai_in_tokens", "ai_out_tokens", "ai_allowance_usd",
}

func validCounter(name string) bool {
	for _, n := range CounterNames {
		if n == name {
			return true
		}
	}
	return false
}

// Counters tracks today's aggregates and persists them atomically.
type Counters struct {
	mu     sync.Mutex
	dir    string
	loc    *time.Location
	day    string
	counts map[string]float64
}

// NewCounters loads (or starts) today's counters in dir.
func NewCounters(dir, tz string) *Counters {
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		loc = time.UTC
	}
	c := &Counters{dir: dir, loc: loc, counts: map[string]float64{}}
	c.day = c.today()
	c.load()
	return c
}

func (c *Counters) today() string { return time.Now().In(c.loc).Format("2006-01-02") }

func (c *Counters) path() string { return filepath.Join(c.dir, "counters.json") }

func (c *Counters) load() {
	raw, err := os.ReadFile(c.path())
	if err != nil {
		return
	}
	var saved struct {
		Day    string             `json:"day"`
		Counts map[string]float64 `json:"counts"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		return
	}
	if saved.Day != c.day {
		return // yesterday's file: start fresh
	}
	for k, v := range saved.Counts {
		if validCounter(k) && v >= 0 {
			c.counts[k] = v
		}
	}
}

// Inc adds one to name and persists atomically (counters only).
func (c *Counters) Inc(name string) {
	c.Add(name, 1)
}

// Add adds n to name and persists atomically (counters only).
func (c *Counters) Add(name string, n float64) {
	if !validCounter(name) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.today(); t != c.day {
		c.day = t
		c.counts = map[string]float64{}
	}
	c.counts[name] += n
	c.saveLocked()
}

// AddTokens records LLM token usage (numbers only).
func (c *Counters) AddTokens(inTok, outTok int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.today(); t != c.day {
		c.day = t
		c.counts = map[string]float64{}
	}
	if inTok > 0 {
		c.counts["ai_in_tokens"] += float64(inTok)
	}
	if outTok > 0 {
		c.counts["ai_out_tokens"] += float64(outTok)
	}
	c.saveLocked()
}

// AddAllowance adds one provider-reported usage.opencode.allowance value
// (a float, in USD) to the running daily sum. Numbers only.
func (c *Counters) AddAllowance(usd float64) {
	if usd <= 0 {
		return
	}
	c.Add("ai_allowance_usd", usd)
}

// Snapshot returns a copy of today's counters.
func (c *Counters) Snapshot() map[string]float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.today(); t != c.day {
		c.day = t
		c.counts = map[string]float64{}
	}
	out := make(map[string]float64, len(CounterNames))
	for _, n := range CounterNames {
		out[n] = c.counts[n]
	}
	return out
}

func (c *Counters) saveLocked() {
	if c.dir == "" {
		return
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return
	}
	saved := map[string]interface{}{
		"day":    c.day,
		"counts": c.counts,
	}
	b, err := json.Marshal(saved)
	if err != nil {
		return
	}
	tmp := c.path() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path())
}
