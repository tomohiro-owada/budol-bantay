package engine

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/golden.json from current engine")

type goldenEntry struct {
	ID    string   `json:"id"`
	Score int      `json:"score"`
	Level string   `json:"level"`
	Hits  []string `json:"hits"`
}

// TestGolden writes/validates engine/testdata/golden.json.
// Run with: go test ./engine/ -run TestGolden -update
// The node parity test (web/test/parity.test.mjs) compares web/engine.js
// against this file: same score and same hit ids per corpus item.
func TestGolden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var items []corpusItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	var golden []goldenEntry
	for _, it := range items {
		r := Analyze(it.Text, it.Sender)
		ids := []string{}
		for _, h := range r.Hits {
			ids = append(ids, h.ID)
		}
		sort.Strings(ids)
		golden = append(golden, goldenEntry{ID: it.ID, Score: r.Score, Level: r.Level, Hits: ids})
	}
	path := filepath.Join("testdata", "golden.json")
	if *updateGolden {
		b, err := json.MarshalIndent(golden, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		b = append(b, '\n')
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d entries)", path, len(golden))
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden.json missing: run go test ./engine/ -run TestGolden -update: %v", err)
	}
	var want []goldenEntry
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(golden) {
		t.Fatalf("golden.json has %d entries, corpus has %d; re-run with -update", len(want), len(golden))
	}
	byID := map[string]goldenEntry{}
	for _, g := range want {
		byID[g.ID] = g
	}
	for _, g := range golden {
		w, ok := byID[g.ID]
		if !ok {
			t.Errorf("golden.json missing id %q; re-run with -update", g.ID)
			continue
		}
		if w.Score != g.Score || w.Level != g.Level || !equalStrSlice(w.Hits, g.Hits) {
			t.Errorf("%s: engine drift: got score=%d level=%s hits=%v; golden score=%d level=%s hits=%v; re-run with -update if intentional",
				g.ID, g.Score, g.Level, g.Hits, w.Score, w.Level, w.Hits)
		}
	}
}

// TestPatternsCompile ensures every pattern in rules.json compiles and stays
// in the RE2∩JS common subset (no lookaround, backrefs, named groups, possessives).
func TestPatternsCompile(t *testing.T) {
	for name, p := range Loaded.Patterns {
		if _, err := compilePat(p.Src, p.Flags); err != nil {
			t.Errorf("patterns.%s: %v", name, err)
		}
	}
	for _, r := range Loaded.Rules {
		if r.Pattern != "" {
			if _, err := compilePat(r.Pattern, r.Flags); err != nil {
				t.Errorf("rule %s pattern: %v", r.ID, err)
			}
		}
		if r.EvPattern != "" {
			if _, err := compilePat(r.EvPattern, r.EvFlags); err != nil {
				t.Errorf("rule %s evPattern: %v", r.ID, err)
			}
		}
	}
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
