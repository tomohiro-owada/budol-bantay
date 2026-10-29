package api

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEstimateNeurons(t *testing.T) {
	// Measured gpt-oss-120b check (docs/PROVIDER.md): ~965 in / ~875 out ≈ 90 neurons.
	got := EstimateNeurons("@cf/openai/gpt-oss-120b", 965, 875)
	if math.Abs(got-90.4) > 0.5 {
		t.Fatalf("gpt-oss-120b = %.2f, want ~90.4", got)
	}
	if got := EstimateNeurons("@CF/Qwen/Qwen3-30B-A3B-FP8 ", 1000000, 1000000); math.Abs(got-(4625+30475)) > 0.01 {
		t.Fatalf("case/space-insensitive lookup failed: %v", got)
	}
	// Unknown models are priced pessimistically (above gpt-oss-120b).
	r, known := RateFor("@cf/vendor/new-model")
	if known || r.InPerM <= neuronRates["@cf/openai/gpt-oss-120b"].InPerM || r.OutPerM <= neuronRates["@cf/openai/gpt-oss-120b"].OutPerM {
		t.Fatalf("unknown rate must be pessimistic: %+v known=%v", r, known)
	}
	if EstimateNeurons("@cf/openai/gpt-oss-120b", -5, -5) != 0 {
		t.Fatal("negative tokens must count as 0")
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

func TestNeuronBudgetReserveSettle(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	b := NewNeuronBudget(t.TempDir(), 100, clk.Now)
	if !b.Reserve(60) {
		t.Fatal("first reserve")
	}
	// In-flight reservations count against the budget.
	if b.Reserve(60) {
		t.Fatal("second reserve must fail while the first is in flight")
	}
	b.Settle(60, 20) // actual was much cheaper than the reservation
	if got := b.Used(); got != 20 {
		t.Fatalf("used = %v", got)
	}
	if !b.Reserve(60) {
		t.Fatal("reserve after settle (20+60<=100)")
	}
	b.Settle(60, 60)
	if b.Reserve(21) { // 80 used + 21 > 100
		t.Fatal("must refuse beyond budget")
	}
	if !b.Reserve(20) {
		t.Fatal("exactly at budget is allowed")
	}
	b.Settle(20, 20)
	if b.Remaining() != 0 {
		t.Fatalf("remaining = %v", b.Remaining())
	}
}

// The day is the UTC day (Workers AI resets 00:00 UTC), NOT Manila midnight.
func TestNeuronBudgetResetsAtUTCMidnight(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 10, 29, 23, 59, 0, 0, time.UTC)}
	b := NewNeuronBudget(t.TempDir(), 100, clk.Now)
	if !b.Reserve(100) {
		t.Fatal("reserve")
	}
	b.Settle(100, 100)
	if b.Reserve(1) {
		t.Fatal("budget spent")
	}
	// 2026-10-30 07:59 Manila = 23:59 UTC the same UTC day: still spent.
	clk.Set(time.Date(2026, 10, 29, 23, 59, 59, 0, time.UTC))
	if b.Reserve(1) {
		t.Fatal("Manila midnight (16:00 UTC) must NOT reset the budget")
	}
	// 08:00 Manila on the 30th = 00:00 UTC: new day.
	clk.Set(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	if !b.Reserve(100) {
		t.Fatal("00:00 UTC must reset the budget")
	}
	if b.Used() != 0 {
		t.Fatalf("used after reset = %v", b.Used())
	}
}

func TestNeuronBudgetPersistsAndIgnoresStaleFile(t *testing.T) {
	dir := t.TempDir()
	clk := &fakeClock{t: time.Date(2026, 10, 29, 5, 0, 0, 0, time.UTC)}
	b := NewNeuronBudget(dir, 100, clk.Now)
	b.Reserve(30)
	b.Settle(30, 30)
	// "Restart": a new ledger on the same dir keeps today's use.
	b2 := NewNeuronBudget(dir, 100, clk.Now)
	if b2.Used() != 30 {
		t.Fatalf("persisted used = %v", b2.Used())
	}
	// Next UTC day: yesterday's file is ignored.
	clk.Set(time.Date(2026, 10, 30, 0, 0, 1, 0, time.UTC))
	b3 := NewNeuronBudget(dir, 100, clk.Now)
	if b3.Used() != 0 {
		t.Fatalf("stale file must be ignored, used = %v", b3.Used())
	}
	// A corrupt file never blocks startup.
	if err := os.WriteFile(filepath.Join(dir, "neurons.json"), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if NewNeuronBudget(dir, 100, clk.Now).Used() != 0 {
		t.Fatal("corrupt file must read as empty")
	}
}

func TestNeuronBudgetDisabledAndExhaust(t *testing.T) {
	var off *NeuronBudget
	if !off.Reserve(1e9) || off.Enabled() {
		t.Fatal("nil budget must allow everything")
	}
	b0 := NewNeuronBudget(t.TempDir(), 0, nil)
	if !b0.Reserve(1e9) {
		t.Fatal("budget 0 means disabled")
	}
	b := NewNeuronBudget(t.TempDir(), 100, nil)
	b.Exhaust()
	if b.Reserve(1) {
		t.Fatal("Exhaust must block the rest of the day")
	}
}

func TestNeuronBudgetConcurrent(t *testing.T) {
	b := NewNeuronBudget(t.TempDir(), 1000, nil)
	var ok int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b.Reserve(10) {
				atomic.AddInt64(&ok, 1)
				b.Settle(10, 10)
			}
		}()
	}
	wg.Wait()
	if ok != 100 || b.Used() != 1000 {
		t.Fatalf("granted %d calls, used %v (want exactly 100 / 1000)", ok, b.Used())
	}
}

func TestEnvNonNegFloat(t *testing.T) {
	t.Setenv("AI_DAILY_NEURON_BUDGET", "0")
	if envNonNegFloat("AI_DAILY_NEURON_BUDGET", 8000) != 0 {
		t.Fatal("explicit 0 must be kept (disabled)")
	}
	t.Setenv("AI_DAILY_NEURON_BUDGET", "-3")
	if envNonNegFloat("AI_DAILY_NEURON_BUDGET", 8000) != 8000 {
		t.Fatal("negative falls back")
	}
	t.Setenv("AI_DAILY_NEURON_BUDGET", "abc")
	if envNonNegFloat("AI_DAILY_NEURON_BUDGET", 8000) != 8000 {
		t.Fatal("invalid falls back")
	}
	t.Setenv("AI_DAILY_NEURON_BUDGET", "")
	if LoadConfig().AIDailyNeurons != 8000 {
		t.Fatal("default must be 8000 (80% of the 10k free tier)")
	}
}

// End to end: once the day's neuron budget is spent, /v1/check still returns
// 200 with rules and ai_status "quota", and the provider is NOT called again.
func TestServerNeuronBudgetFallsBackToRulesOnly(t *testing.T) {
	var calls int64
	reply := chatReply(t) // usage 800 in / 600 out
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	defer llm.Close()
	srv, _ := newTestServer(t, func(c *Config) {
		c.AIMode, c.AIKey, c.AIStyle = "", "test-key", "openai_chat"
		c.AIBaseURL = llm.URL
		c.AIModel = "@cf/openai/gpt-oss-120b"
		c.AIMaxTokens = 1200
		c.AIDailyNeurons = 200 // 66.4/call actual, ~117 reserved
	})
	body := `{"text":"GCash: i-verify ang account dito http://example.com/login"}`
	var statuses []string
	for i := 0; i < 4; i++ {
		rec := postCheck(t, srv, "", body, jsonCT)
		v := checkJSON(t, rec.Body.String())
		if rec.Code != 200 {
			t.Fatalf("call %d code %d", i, rec.Code)
		}
		if _, ok := v["rules"].(map[string]interface{}); !ok {
			t.Fatalf("call %d lost the rules result", i)
		}
		statuses = append(statuses, v["ai_status"].(string))
	}
	if statuses[0] != "ok" || statuses[1] != "ok" || statuses[2] != "quota" || statuses[3] != "quota" {
		t.Fatalf("statuses = %v", statuses)
	}
	if calls != 2 {
		t.Fatalf("provider called %d times, want 2 (no calls once over budget)", calls)
	}
	if u := srv.budget.Used(); math.Abs(u-2*66.4) > 1 {
		t.Fatalf("used = %v, want ~132.8", u)
	}
	if srv.Counters().Snapshot()["quota"] != 2 {
		t.Fatalf("quota counter = %v", srv.Counters().Snapshot())
	}
}

// Fallback calls are charged to the same budget, and a fallback refused by
// the budget ends as "quota", not as a provider error.
func TestServerNeuronBudgetCoversFallback(t *testing.T) {
	var primary, fallback int64
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(buf), "gpt-oss-20b") {
			atomic.AddInt64(&fallback, 1)
			_, _ = io.WriteString(w, chatReply(t))
			return
		}
		atomic.AddInt64(&primary, 1)
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"errors":[{"code":5000}]}`)
	}))
	defer llm.Close()
	srv, _ := newTestServer(t, func(c *Config) {
		c.AIMode, c.AIKey, c.AIStyle = "", "test-key", "openai_chat"
		c.AIBaseURL = llm.URL
		c.AIModel = "@cf/openai/gpt-oss-120b"
		c.AIFallbackModel = "@cf/openai/gpt-oss-20b"
		c.AIMaxTokens = 1200
		c.AIDailyNeurons = 1000
	})
	rec := postCheck(t, srv, "", `{"text":"Magandang araw! Paalala lang na magdala ng payong bukas."}`, jsonCT)
	if v := checkJSON(t, rec.Body.String()); v["ai_status"] != "ok" {
		t.Fatalf("fallback should answer: %s", rec.Body.String())
	}
	if primary != 1 || fallback != 1 {
		t.Fatalf("primary=%d fallback=%d", primary, fallback)
	}
	// 500 on the primary charges input only; fallback charges its real use.
	want := EstimateNeurons("@cf/openai/gpt-oss-20b", 800, 600)
	if u := srv.budget.Used(); u < want || u > want+60 {
		t.Fatalf("used = %.1f, want ~%.1f + small primary input charge", u, want)
	}
}

// Workers AI says the free allocation is gone (error 4006): stop calling it
// for the rest of the UTC day, even if our own estimate said there was room.
func TestServerProviderQuotaErrorExhaustsDay(t *testing.T) {
	var calls int64
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":4006,"message":"you have used up your daily free allocation of 10,000 neurons"}]}`)
	}))
	defer llm.Close()
	srv, _ := newTestServer(t, func(c *Config) {
		c.AIMode, c.AIKey, c.AIStyle = "", "test-key", "openai_chat"
		c.AIBaseURL = llm.URL
		c.AIModel = "@cf/openai/gpt-oss-120b"
		c.AIDailyNeurons = 8000
	})
	body := `{"text":"Magandang araw! Paalala lang na magdala ng payong bukas."}`
	first := checkJSON(t, postCheck(t, srv, "", body, jsonCT).Body.String())
	second := checkJSON(t, postCheck(t, srv, "", body, jsonCT).Body.String())
	if first["ai_status"] != "error" || second["ai_status"] != "quota" {
		t.Fatalf("statuses = %v / %v", first["ai_status"], second["ai_status"])
	}
	if calls != 1 {
		t.Fatalf("provider called %d times after exhaustion, want 1", calls)
	}
}

// The budget survives a restart (new Server on the same state dir).
func TestServerNeuronBudgetSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatReply(t))
	}))
	defer llm.Close()
	mk := func() *Server {
		cfg := testConfig(dir)
		cfg.AIMode, cfg.AIKey, cfg.AIStyle = "", "test-key", "openai_chat"
		cfg.AIBaseURL = llm.URL
		cfg.AIModel = "@cf/openai/gpt-oss-120b"
		cfg.AIMaxTokens = 1200
		cfg.AIDailyNeurons = 150
		return NewServer(cfg, nil, nil)
	}
	body := `{"text":"Magandang araw! Paalala lang na magdala ng payong bukas."}`
	s1 := mk()
	if v := checkJSON(t, postCheck(t, s1, "", body, jsonCT).Body.String()); v["ai_status"] != "ok" {
		t.Fatalf("first: %v", v["ai_status"])
	}
	s2 := mk() // restart
	if v := checkJSON(t, postCheck(t, s2, "", body, jsonCT).Body.String()); v["ai_status"] != "quota" {
		t.Fatalf("after restart want quota (66 used + 117 reserved > 150), got %v", v["ai_status"])
	}
}
