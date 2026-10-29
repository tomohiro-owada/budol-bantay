package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDatasetValidAndBalanced(t *testing.T) {
	items, err := loadDataset("../../eval/dataset.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 40 || len(items) > 70 {
		t.Fatalf("dataset size %d, want ~40-60", len(items))
	}
	seen := map[string]bool{}
	scam := 0
	for _, it := range items {
		if seen[it.ID] {
			t.Fatalf("duplicate id %s", it.ID)
		}
		seen[it.ID] = true
		if strings.TrimSpace(it.Text) == "" {
			t.Fatalf("%s empty", it.ID)
		}
		if it.Label == "scam" {
			scam++
		}
	}
	if scam < 15 || len(items)-scam < 15 {
		t.Fatalf("unbalanced: %d scam of %d", scam, len(items))
	}
}

func TestMetrics(t *testing.T) {
	var m Metrics
	m.Add("scam", true)
	m.Add("scam", true)
	m.Add("scam", false)
	m.Add("legit", true)
	m.Add("legit", false)
	if m.TP != 2 || m.FN != 1 || m.FP != 1 || m.TN != 1 {
		t.Fatalf("%+v", m)
	}
	if got := m.Precision(); got < 0.666 || got > 0.667 {
		t.Fatalf("precision %v", got)
	}
	if got := m.Recall(); got < 0.666 || got > 0.667 {
		t.Fatalf("recall %v", got)
	}
	if got := m.Accuracy(); got != 0.6 {
		t.Fatalf("accuracy %v", got)
	}
	var empty Metrics
	if empty.Precision() != 0 || empty.Accuracy() != 0 {
		t.Fatal("empty metrics must not divide by zero")
	}
}

func TestStratifiedKeepsBothClasses(t *testing.T) {
	items, _ := loadDataset("../../eval/dataset.json")
	s := stratified(items, 20)
	sc, lg := 0, 0
	for _, it := range s {
		if it.Label == "scam" {
			sc++
		} else {
			lg++
		}
	}
	if sc < 8 || lg < 5 || len(s) < 18 || len(s) > 22 {
		t.Fatalf("sample = %d scam + %d legit", sc, lg)
	}
	if len(stratified(items, 0)) != len(items) || len(stratified(items, 9999)) != len(items) {
		t.Fatal("0 / oversize sample must return everything")
	}
}

func TestLooksTagalog(t *testing.T) {
	if !looksTagalog("Huwag mong i-click ang link, ito ay peke para sa account mo.") {
		t.Fatal("taglish should pass")
	}
	if looksTagalog("This message asks you to click a suspicious link.") {
		t.Fatal("english must not pass")
	}
}

const stubReply = `{"choices":[{"message":{"content":"{\"verdict\":\"scam\",\"risk\":90,\"scam_type\":\"phishing_link\",\"red_flags\":[{\"tl\":\"May link.\",\"en\":\"Has a link.\",\"ja\":\"リンクあり。\"}],\"explanation\":{\"tl\":\"Ito ay peke, huwag mong i-click ang link na ito.\",\"en\":\"Fake.\",\"ja\":\"偽物です。\"},\"actions\":[{\"tl\":\"Huwag i-click.\",\"en\":\"Do not click.\",\"ja\":\"開かない。\"}],\"injection_detected\":false}"}}],"usage":{"prompt_tokens":1000,"completion_tokens":500}}`

// run() against a stub: stops at the neuron limit, writes resumable results,
// and score() reads them back. No network, no keys.
func TestRunRespectsNeuronLimitAndScores(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		_, _ = io.WriteString(w, stubReply)
	}))
	defer srv.Close()
	t.Setenv("AI_BASE_URL", srv.URL)
	t.Setenv("AI_API_KEY", "stub-key")
	t.Setenv("AI_KEY_ENV", "AI_API_KEY")
	t.Setenv("AI_MODE", "")
	items, _ := loadDataset("../../eval/dataset.json")
	dir := t.TempDir()
	model := "@cf/google/gemma-4-26b-a4b-it"
	// Stub usage costs ≈22.7 neurons/call; the worst-case guard (≈78) stops the
	// run once spent+78 > 200, i.e. after 6 calls (136 spent) — never beyond 200.
	if err := run(items, []string{model}, dir, 0, "", 200, 9000, time.Millisecond, 2500); err != nil {
		t.Fatal(err)
	}
	if calls != 6 {
		t.Fatalf("calls = %d, want 6 with a 200-neuron limit", calls)
	}
	first := calls
	// Resume: the same limit again does not redo finished items, and the day total is persisted.
	spent := loadSpent(dir)
	if spent.Neurons <= 0 {
		t.Fatalf("day total not persisted: %+v", spent)
	}
	// A day limit already reached blocks all calls.
	if err := run(items, []string{model}, dir, 0, "", 1e9, spent.Neurons, time.Millisecond, 2500); err != nil {
		t.Fatal(err)
	}
	if calls != first {
		t.Fatalf("day limit ignored: %d -> %d calls", first, calls)
	}
	if _, err := os.Stat(filepath.Join(dir, safeName(model)+".jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := score(items, []string{model}, dir); err != nil {
		t.Fatal(err)
	}
}

func TestRunRefusesWithoutConfig(t *testing.T) {
	t.Setenv("AI_API_KEY", "")
	t.Setenv("AI_BASE_URL", "https://api.cloudflare.com/client/v4/accounts/${CF_ACCOUNT_ID}/ai/v1")
	t.Setenv("CF_ACCOUNT_ID", "")
	items, _ := loadDataset("../../eval/dataset.json")
	if err := run(items, DefaultModels[:1], t.TempDir(), 5, "", 100, 100, 0, 2500); err == nil {
		t.Fatal("must refuse to run unconfigured")
	}
}
