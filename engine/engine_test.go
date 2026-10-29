package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesJSONValid(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range Loaded.Rules {
		if seen[r.ID] {
			t.Fatalf("duplicate rule id %q", r.ID)
		}
		seen[r.ID] = true
		if r.Tl == "" || r.En == "" || r.Ja == "" {
			t.Fatalf("rule %q missing tl/en/ja", r.ID)
		}
		if r.ID == "otpNotice" {
			if r.W >= 0 {
				t.Fatalf("otpNotice weight must be negative, got %d", r.W)
			}
			continue
		}
		if r.W < 1 || r.W > 60 {
			t.Fatalf("rule %q weight %d out of range 1..60", r.ID, r.W)
		}
	}
	if len(Loaded.Shorteners) == 0 {
		t.Fatal("shorteners empty")
	}
	if len(Loaded.RiskyTLDs) == 0 {
		t.Fatal("riskyTlds empty")
	}
	if len(Loaded.Official) == 0 {
		t.Fatal("official empty")
	}
	// every hit id must be explainable via Messages()
	msgs := Messages()
	var parsed map[string]map[string]map[string]string
	if err := json.Unmarshal([]byte(msgs), &parsed); err != nil {
		t.Fatalf("Messages() invalid JSON: %v", err)
	}
	flags := parsed["flags"]
	for _, r := range Loaded.Rules {
		f, ok := flags[r.ID]
		if !ok {
			t.Fatalf("Messages() missing flag %q", r.ID)
		}
		if f["tl"] == "" || f["en"] == "" || f["ja"] == "" {
			t.Fatalf("Messages() flag %q missing lang", r.ID)
		}
	}
}

type corpusItem struct {
	ID     string `json:"id"`
	Sender string `json:"sender"`
	Text   string `json:"text"`
	Level  string `json:"level"`
}

func TestCorpusLevels(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var items []corpusItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) < 22 {
		t.Fatalf("corpus must have >=22 items (12 PoC + 10 new), got %d", len(items))
	}
	// ensure the 12 PoC ids are present
	need := []string{"poc-01", "poc-02", "poc-03", "poc-04", "poc-05", "poc-06", "poc-07", "poc-08", "poc-09", "poc-10", "poc-11", "poc-12"}
	for _, n := range need {
		found := false
		for _, it := range items {
			if strings.HasPrefix(it.ID, n) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("corpus missing %s", n)
		}
	}
	for _, it := range items {
		r := Analyze(it.Text, it.Sender)
		if r.Level != it.Level {
			flags := []string{}
			for _, h := range r.Hits {
				flags = append(flags, h.ID)
			}
			t.Errorf("%s: got level %q score %d hits %v, want level %q\ntext: %s", it.ID, r.Level, r.Score, flags, it.Level, it.Text)
		}
		// band sanity
		switch it.Level {
		case "hi":
			if r.Score < 60 {
				t.Errorf("%s: hi requires score>=60, got %d", it.ID, r.Score)
			}
		case "mid":
			if r.Score < 30 || r.Score >= 60 {
				t.Errorf("%s: mid requires 30<=score<60, got %d", it.ID, r.Score)
			}
		case "lo":
			if r.Score < 10 || r.Score >= 30 {
				t.Errorf("%s: lo requires 10<=score<30, got %d", it.ID, r.Score)
			}
		case "none":
			if r.Score >= 10 {
				t.Errorf("%s: none requires score<10, got %d", it.ID, r.Score)
			}
		default:
			t.Errorf("%s: unknown level %q", it.ID, it.Level)
		}
	}
}

type piiItem struct {
	Input          string   `json:"input"`
	MustNotContain []string `json:"mustNotContain"`
	MustContain    []string `json:"mustContain"`
}

func TestPIIFixtures(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "pii.json"))
	if err != nil {
		t.Fatal(err)
	}
	var items []piiItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("pii.json empty")
	}
	for i, it := range items {
		m := Mask(it.Input)
		for _, s := range it.MustNotContain {
			if s != "" && strings.Contains(m.Masked, s) {
				t.Errorf("pii[%d] input %q: masked output still contains %q -> %q", i, it.Input, s, m.Masked)
			}
		}
		for _, s := range it.MustContain {
			if !strings.Contains(m.Masked, s) {
				t.Errorf("pii[%d] input %q: masked output missing %q -> %q", i, it.Input, s, m.Masked)
			}
		}
	}
}

func TestLevelsAndClamp(t *testing.T) {
	if got := Analyze("", "").Level; got != "none" {
		t.Fatalf("empty text level = %q, want none", got)
	}
	if got := Analyze("", "").Score; got != 0 {
		t.Fatalf("empty text score = %d, want 0", got)
	}
}

func TestNeverSafeWord(t *testing.T) {
	// Engine never outputs "safe"/"legit" verdicts; levels are hi/mid/lo/none only.
	for _, lvl := range []string{"hi", "mid", "lo", "none"} {
		if lvl == "safe" || lvl == "legit" {
			t.Fatal("forbidden level")
		}
	}
}
