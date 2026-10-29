// Package api: Workers AI neuron accounting and the daily neuron budget.
//
// Workers AI gives 10,000 neurons/day free and resets at 00:00 UTC (08:00
// Asia/Manila). Past that, Free-plan requests fail and Paid-plan requests are
// billed. The budget below keeps the API strictly inside the free tier: it
// estimates the neuron cost of every provider call from the token usage the
// provider reports, reserves the worst case before a call, and answers
// rules-only (ai_status "quota") once the day's budget is gone.
//
// The day boundary is ALWAYS UTC here, never CAP_TZ: Cloudflare resets its
// allowance at 00:00 UTC, so a Manila-midnight window would allow roughly
// twice the free allowance in the 8 hours between the two boundaries.
package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// NeuronRate is a model's price in neurons per 1M tokens
// (https://developers.cloudflare.com/workers-ai/platform/pricing/, 2026-10-29).
type NeuronRate struct {
	InPerM  float64
	OutPerM float64
}

// neuronRates lists the text-generation models we may use (id without the
// "@cf/" prefix is NOT accepted: ids are matched exactly, case-insensitively).
var neuronRates = map[string]NeuronRate{
	"@cf/meta/llama-3.2-1b-instruct":               {2457, 18252},
	"@cf/meta/llama-3.2-3b-instruct":               {4625, 30475},
	"@cf/meta/llama-3.1-8b-instruct-fp8-fast":      {4119, 34868},
	"@cf/meta/llama-3.1-8b-instruct-fp8":           {13778, 26128},
	"@cf/meta/llama-3.1-8b-instruct-awq":           {11161, 24215},
	"@cf/meta/llama-3.3-70b-instruct-fp8-fast":     {26668, 204805},
	"@cf/meta/llama-4-scout-17b-16e-instruct":      {24545, 77273},
	"@cf/google/gemma-3-12b-it":                    {31371, 50560},
	"@cf/google/gemma-4-26b-a4b-it":                {9091, 27273},
	"@cf/mistralai/mistral-small-3.1-24b-instruct": {31876, 50488},
	"@cf/qwen/qwen3-30b-a3b-fp8":                   {4625, 30475},
	"@cf/qwen/qwen3.8-27b":                         {40909, 290909},
	"@cf/openai/gpt-oss-20b":                       {18182, 27273},
	"@cf/openai/gpt-oss-120b":                      {31818, 68182},
	"@cf/aisingapore/gemma-sea-lion-v4-27b-it":     {31876, 50488},
	"@cf/ibm-granite/granite-4.0-h-micro":          {1542, 10158},
	"@cf/zai-org/glm-4.7-flash":                    {5500, 36400},
	"@cf/zai-org/glm-5.3-flash":                    {13636, 45455},
	"@cf/nvidia/nemotron-3-120b-a12b":              {45455, 136364},
}

// unknownRate prices a model that is not in the table. It is deliberately
// pessimistic (more than gpt-oss-120b, less than a frontier model) so an
// unlisted model burns the budget faster, never slower.
var unknownRate = NeuronRate{InPerM: 60000, OutPerM: 205000}

// RateFor returns the neuron rate for model and whether it is a known model.
func RateFor(model string) (NeuronRate, bool) {
	r, ok := neuronRates[strings.ToLower(strings.TrimSpace(model))]
	if !ok {
		return unknownRate, false
	}
	return r, true
}

// KnownNeuronModels returns the ids with a built-in rate (for tools/tests).
func KnownNeuronModels() []string {
	out := make([]string, 0, len(neuronRates))
	for k := range neuronRates {
		out = append(out, k)
	}
	return out
}

// EstimateNeurons converts token usage to neurons for model.
func EstimateNeurons(model string, inTokens, outTokens int) float64 {
	r, _ := RateFor(model)
	if inTokens < 0 {
		inTokens = 0
	}
	if outTokens < 0 {
		outTokens = 0
	}
	return float64(inTokens)/1e6*r.InPerM + float64(outTokens)/1e6*r.OutPerM
}

// NeuronBudget is the persisted, concurrency-safe daily neuron ledger.
// Budget <= 0 disables it (Reserve always succeeds, nothing is persisted).
type NeuronBudget struct {
	mu       sync.Mutex
	dir      string
	budget   float64
	now      func() time.Time
	day      string  // UTC date of used
	used     float64 // settled neurons today
	reserved float64 // in-flight worst-case reservations
}

// NewNeuronBudget loads today's (UTC) ledger from dir/neurons.json.
func NewNeuronBudget(dir string, budget float64, now func() time.Time) *NeuronBudget {
	if now == nil {
		now = time.Now
	}
	b := &NeuronBudget{dir: dir, budget: budget, now: now}
	b.day = b.today()
	b.load()
	return b
}

func (b *NeuronBudget) today() string { return b.now().UTC().Format("2006-01-02") }

func (b *NeuronBudget) path() string { return filepath.Join(b.dir, "neurons.json") }

func (b *NeuronBudget) load() {
	if b.dir == "" {
		return
	}
	raw, err := os.ReadFile(b.path())
	if err != nil {
		return
	}
	var saved struct {
		Day  string  `json:"day"`
		Used float64 `json:"used"`
	}
	if json.Unmarshal(raw, &saved) != nil || saved.Day != b.day || saved.Used < 0 {
		return
	}
	b.used = saved.Used
}

func (b *NeuronBudget) rollLocked() {
	if t := b.today(); t != b.day {
		b.day, b.used, b.reserved = t, 0, 0
	}
}

func (b *NeuronBudget) saveLocked() {
	if b.dir == "" {
		return
	}
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(map[string]interface{}{"day": b.day, "used": b.used})
	if err != nil {
		return
	}
	tmp := b.path() + ".tmp"
	if os.WriteFile(tmp, append(raw, '\n'), 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, b.path())
}

// Enabled reports whether a budget is enforced.
func (b *NeuronBudget) Enabled() bool { return b != nil && b.budget > 0 }

// Reserve books est neurons for an in-flight call. It fails (false) when
// settled use plus all reservations plus est would exceed the budget.
func (b *NeuronBudget) Reserve(est float64) bool {
	if !b.Enabled() {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	if b.used+b.reserved+est > b.budget {
		return false
	}
	b.reserved += est
	return true
}

// Settle releases a reservation of est and records the actual neurons spent.
func (b *NeuronBudget) Settle(est, actual float64) {
	if !b.Enabled() {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	b.reserved -= est
	if b.reserved < 0 {
		b.reserved = 0
	}
	if actual > 0 {
		b.used += actual
	}
	b.saveLocked()
}

// Exhaust marks the whole day's budget as spent (the provider said the free
// allocation is used up), so we answer rules-only until 00:00 UTC.
func (b *NeuronBudget) Exhaust() {
	if !b.Enabled() {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	if b.used < b.budget {
		b.used = b.budget
	}
	b.saveLocked()
}

// Used returns today's settled neurons (UTC day).
func (b *NeuronBudget) Used() float64 {
	if !b.Enabled() {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	return b.used
}

// Remaining returns the unspent budget (excluding reservations).
func (b *NeuronBudget) Remaining() float64 {
	if !b.Enabled() {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	if r := b.budget - b.used; r > 0 {
		return r
	}
	return 0
}

// Budget returns the configured daily budget (0 = disabled).
func (b *NeuronBudget) Budget() float64 {
	if b == nil {
		return 0
	}
	return b.budget
}
