// Command aibench evaluates Workers AI text models on eval/dataset.json with
// the PRODUCTION prompt, parser (api.ParseAIOutput) and merge rule.
//
// Modes:
//
//	rules  rules-engine-only baseline (offline, free)
//	est    token/neuron estimate per model and checks/day inside a budget (offline)
//	run    call the models (needs CF_ACCOUNT_ID + AI_API_KEY; spends neurons,
//	       guarded by a persisted per-UTC-day neuron limit; never prints keys)
//	score  score eval/results/*.jsonl (offline)
//
// Examples:
//
//	go run ./cmd/aibench -mode rules
//	go run ./cmd/aibench -mode est
//	set -a; . ./env.local; set +a     # your private env file (never committed)
//	go run ./cmd/aibench -mode run -models @cf/google/gemma-4-26b-a4b-it -sample 20 -neuron-limit 2000
//	go run ./cmd/aibench -mode score
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"scam-checker/api"
	"scam-checker/engine"
)

// Item is one labeled message in eval/dataset.json.
type Item struct {
	ID       string `json:"id"`
	Label    string `json:"label"` // scam | legit
	Category string `json:"category"`
	Channel  string `json:"channel"`
	Sender   string `json:"sender"`
	Text     string `json:"text"`
	Source   string `json:"source"`
	Hard     bool   `json:"hard"`
}

// Record is one model call outcome (one line of results/<model>.jsonl).
type Record struct {
	Model     string  `json:"model"`
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Rules     int     `json:"rules"`
	LatencyMS int64   `json:"latency_ms"`
	InTok     int     `json:"in_tokens"`
	OutTok    int     `json:"out_tokens"`
	Neurons   float64 `json:"neurons"`
	Measured  bool    `json:"measured,omitempty"`
	Err       string  `json:"err,omitempty"`
	Raw       string  `json:"raw,omitempty"`
}

// DefaultModels are the shortlisted candidates (see eval/RESULTS.md).
var DefaultModels = []string{
	"@cf/openai/gpt-oss-120b",
	"@cf/openai/gpt-oss-20b",
	"@cf/google/gemma-4-26b-a4b-it",
	"@cf/qwen/qwen3-30b-a3b-fp8",
	"@cf/aisingapore/gemma-sea-lion-v4-27b-it",
	"@cf/meta/llama-3.1-8b-instruct-fp8",
}

// ScamThreshold: final risk >= this counts as "predicted scam".
const ScamThreshold = 50

func main() {
	mode := flag.String("mode", "rules", "rules|est|run|score")
	dataset := flag.String("dataset", "eval/dataset.json", "labeled dataset")
	resDir := flag.String("results", "eval/results", "results directory")
	models := flag.String("models", strings.Join(DefaultModels, ","), "comma-separated model ids")
	sample := flag.Int("sample", 0, "run only a stratified sample of N items (0 = all)")
	ids := flag.String("ids", "", "comma-separated item ids (overrides -sample)")
	limit := flag.Float64("neuron-limit", 2000, "max estimated neurons this process may spend (also capped by the day's persisted total)")
	dayLimit := flag.Float64("day-limit", 9000, "max estimated neurons per UTC day across all runs of this tool")
	reserveOut := flag.Int("reserve-out", 1300, "output tokens assumed per call for the pre-call limit check (the real cost is the provider-reported usage.neurons)")
	gap := flag.Duration("gap", 300*time.Millisecond, "pause between calls")
	outTok := flag.Int("est-out", 800, "assumed output tokens per check for -mode est")
	budget := flag.Float64("budget", 10000, "neuron budget for the checks/day calculation")
	flag.Parse()

	items, err := loadDataset(*dataset)
	if err != nil {
		fatal(err)
	}
	ml := splitList(*models)
	if *ids != "" && (*mode == "rules" || *mode == "score") {
		want := map[string]bool{}
		for _, id := range splitList(*ids) {
			want[id] = true
		}
		var sel []Item
		for _, it := range items {
			if want[it.ID] {
				sel = append(sel, it)
			}
		}
		items = sel
	}
	switch *mode {
	case "rules":
		rulesBaseline(items)
	case "est":
		estimate(items, ml, *outTok, *budget)
	case "run":
		if err := run(items, ml, *resDir, *sample, *ids, *limit, *dayLimit, *gap, *reserveOut); err != nil {
			fatal(err)
		}
	case "score":
		if err := score(items, ml, *resDir); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unknown -mode %q", *mode))
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "aibench:", err); os.Exit(1) }

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func loadDataset(path string) ([]Item, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var items []Item
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Label != "scam" && it.Label != "legit" {
			return nil, fmt.Errorf("item %s: label must be scam|legit", it.ID)
		}
	}
	return items, nil
}

// prompt builds the production (system, user) pair and the rules result for
// an item, exactly like cmd/aieval and the API server do.
func prompt(it Item) (system, user string, rules engine.Result) {
	masked := engine.Mask(it.Text)
	rules = engine.Analyze(masked.Masked, it.Sender)
	var flags []string
	for _, h := range rules.Hits {
		flags = append(flags, h.ID)
	}
	system, user = api.BuildPrompt(masked.Masked, "tl", api.RulesHints{Score: rules.Score, Level: rules.Level, Flags: flags})
	return
}

// Metrics is a confusion matrix with scam as the positive class.
type Metrics struct{ TP, FP, TN, FN int }

func (m *Metrics) Add(label string, predictedScam bool) {
	switch {
	case label == "scam" && predictedScam:
		m.TP++
	case label == "scam":
		m.FN++
	case predictedScam:
		m.FP++
	default:
		m.TN++
	}
}
func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
func (m Metrics) N() int             { return m.TP + m.FP + m.TN + m.FN }
func (m Metrics) Accuracy() float64  { return ratio(m.TP+m.TN, m.N()) }
func (m Metrics) Precision() float64 { return ratio(m.TP, m.TP+m.FP) }
func (m Metrics) Recall() float64    { return ratio(m.TP, m.TP+m.FN) }

func rulesBaseline(items []Item) {
	var m, hard Metrics
	var fps, fns []string
	for _, it := range items {
		_, _, r := prompt(it)
		pred := r.Score >= ScamThreshold
		m.Add(it.Label, pred)
		if it.Hard {
			hard.Add(it.Label, pred)
		}
		if pred && it.Label == "legit" {
			fps = append(fps, fmt.Sprintf("%s(%d)", it.ID, r.Score))
		}
		if !pred && it.Label == "scam" {
			fns = append(fns, fmt.Sprintf("%s(%d)", it.ID, r.Score))
		}
	}
	fmt.Printf("rules-only baseline, threshold score>=%d, n=%d (scam=%d)\n", ScamThreshold, m.N(), m.TP+m.FN)
	fmt.Printf("  accuracy %.1f%%  precision %.1f%%  recall %.1f%%   TP=%d FP=%d TN=%d FN=%d\n",
		100*m.Accuracy(), 100*m.Precision(), 100*m.Recall(), m.TP, m.FP, m.TN, m.FN)
	fmt.Printf("  hard subset (n=%d): accuracy %.1f%%  precision %.1f%%  recall %.1f%%\n",
		hard.N(), 100*hard.Accuracy(), 100*hard.Precision(), 100*hard.Recall())
	fmt.Printf("  false positives: %s\n  false negatives: %s\n", strings.Join(fps, " "), strings.Join(fns, " "))
}

func estTokens(s string) int { return (len([]rune(s)) + 2) / 3 } // pessimistic for Taglish

func estimate(items []Item, models []string, outTokens int, budget float64) {
	sumIn := 0
	for _, it := range items {
		s, u, _ := prompt(it)
		sumIn += estTokens(s + u)
	}
	avgIn := sumIn / len(items)
	fmt.Printf("avg prompt ≈ %d input tokens (chars/3, pessimistic; measured on gpt-oss-120b: ~965); assumed output %d tokens\n", avgIn, outTokens)
	fmt.Printf("%-46s %9s %9s %10s %12s\n", "model", "in n/M", "out n/M", "neur/check", "checks/10k")
	for _, m := range models {
		r, known := api.RateFor(m)
		n := api.EstimateNeurons(m, avgIn, outTokens)
		flag := ""
		if !known {
			flag = " (unknown rate: pessimistic default)"
		}
		fmt.Printf("%-46s %9.0f %9.0f %10.1f %12d%s\n", m, r.InPerM, r.OutPerM, n, int(budget/n), flag)
	}
}

// ---- run ----

type daySpent struct {
	Day     string  `json:"day"`
	Neurons float64 `json:"neurons"`
}

func spentPath(dir string) string { return filepath.Join(dir, ".neurons-spent.json") }

func loadSpent(dir string) daySpent {
	today := time.Now().UTC().Format("2006-01-02")
	var d daySpent
	if raw, err := os.ReadFile(spentPath(dir)); err == nil {
		_ = json.Unmarshal(raw, &d)
	}
	if d.Day != today {
		return daySpent{Day: today}
	}
	return d
}

func saveSpent(dir string, d daySpent) {
	raw, _ := json.Marshal(d)
	_ = os.WriteFile(spentPath(dir), raw, 0o600)
}

func safeName(model string) string {
	return strings.NewReplacer("@cf/", "", "/", "__", "@", "").Replace(model)
}

func stratified(items []Item, n int) []Item {
	if n <= 0 || n >= len(items) {
		return items
	}
	var scam, legit []Item
	for _, it := range items {
		if it.Label == "scam" {
			scam = append(scam, it)
		} else {
			legit = append(legit, it)
		}
	}
	want := func(group []Item) int {
		k := int(float64(n)*float64(len(group))/float64(len(items)) + 0.5)
		if k < 1 {
			k = 1
		}
		if k > len(group) {
			k = len(group)
		}
		return k
	}
	pick := func(group []Item, k int) []Item {
		var out []Item
		for i := 0; i < k; i++ {
			out = append(out, group[i*len(group)/k]) // evenly spaced
		}
		return out
	}
	return append(pick(scam, want(scam)), pick(legit, want(legit))...)
}

func run(items []Item, models []string, resDir string, sample int, ids string, limit, dayLimit float64, gap time.Duration, reserveOut int) error {
	cfg := api.LoadConfig()
	if !cfg.AIConfigured() {
		return fmt.Errorf("AI not configured: set CF_ACCOUNT_ID and AI_API_KEY in the environment")
	}
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		return err
	}
	if ids != "" {
		want := map[string]bool{}
		for _, id := range splitList(ids) {
			want[id] = true
		}
		var sel []Item
		for _, it := range items {
			if want[it.ID] {
				sel = append(sel, it)
			}
		}
		items = sel
	} else {
		items = stratified(items, sample)
	}
	spent := loadSpent(resDir)
	fmt.Printf("neurons already spent today (UTC %s, this tool): %.0f; process limit %.0f; day limit %.0f\n", spent.Day, spent.Neurons, limit, dayLimit)
	client := &http.Client{Timeout: time.Duration(cfg.AITimeoutSecs) * time.Second}
	var processSpent float64
	for _, model := range models {
		mcfg := cfg
		mcfg.AIModel = model
		mcfg.AIStyle = "openai_chat"
		// reasoning_effort only for gpt-oss; other models reject unknown params.
		mcfg.AIChatReasoningEffort = ""
		if strings.Contains(model, "gpt-oss") {
			mcfg.AIChatReasoningEffort = "low"
		}
		out, err := os.OpenFile(filepath.Join(resDir, safeName(model)+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		done := doneIDs(filepath.Join(resDir, safeName(model)+".jsonl"))
		w := bufio.NewWriter(out)
		for _, it := range items {
			if done[it.ID] {
				continue
			}
			system, user, rules := prompt(it)
			est := api.EstimateNeurons(model, estTokens(system+user), reserveOut)
			if processSpent+est > limit || spent.Neurons+est > dayLimit {
				fmt.Printf("STOP: next call (~%.0f neurons worst case) would exceed the limit (process %.0f/%.0f, day %.0f/%.0f)\n", est, processSpent, limit, spent.Neurons, dayLimit)
				w.Flush()
				out.Close()
				saveSpent(resDir, spent)
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(mcfg.AITimeoutSecs)*time.Second)
			start := time.Now()
			res, perr := api.CallLLM(ctx, client, mcfg, system, user, model, api.SessionID())
			lat := time.Since(start).Milliseconds()
			cancel()
			rec := Record{Model: model, ID: it.ID, Label: it.Label, Rules: rules.Score, LatencyMS: lat}
			if perr != nil {
				rec.Err = fmt.Sprintf("status=%d timeout=%v badoutput=%v quota=%v", perr.Status, perr.Timeout, perr.BadOutput, perr.QuotaExhausted)
				c := api.EstimateNeurons(model, estTokens(system+user), 0)
				if perr.Status == 0 {
					c = est
				}
				rec.Neurons = c
				if perr.QuotaExhausted {
					fmt.Println("provider says the free allocation is used up; stopping")
					spent.Neurons = dayLimit
				}
			} else {
				rec.InTok, rec.OutTok, rec.Raw = res.InTokens, res.OutTokens, res.RawText
				rec.Neurons = api.EstimateNeurons(model, res.InTokens, res.OutTokens)
				if res.Neurons > 0 {
					rec.Neurons = res.Neurons // measured, as reported by Workers AI
					rec.Measured = true
				}
			}
			processSpent += rec.Neurons
			spent.Neurons += rec.Neurons
			b, _ := json.Marshal(rec)
			w.Write(append(b, '\n'))
			w.Flush()
			saveSpent(resDir, spent)
			if perr != nil && perr.QuotaExhausted {
				out.Close()
				return nil
			}
			time.Sleep(gap)
		}
		out.Close()
		fmt.Printf("done %s (process neurons so far ≈ %.0f)\n", model, processSpent)
	}
	return nil
}

func doneIDs(path string) map[string]bool {
	done := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return done
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Err == "" {
			done[r.ID] = true
		}
	}
	return done
}

// ---- score ----

// ModelScore aggregates one model's results.
type ModelScore struct {
	Model                  string
	Calls, Errors, Valid   int
	AI, Merged, MergedHard Metrics
	LatencyMS              []int64
	Neurons                float64
	InTok, OutTok          int
	TagalogOK              int
}

// merged applies the production merge rule used by cmd/aieval.
func merged(rules int, out api.AIOutput) int {
	final := out.Risk
	if rules >= 30 && rules > final {
		final = rules
	}
	if out.InjectionDetected && final < 40 {
		final = 40
	}
	return final
}

var tagalogMarkers = []string{" ang ", " ng ", " sa ", " mo ", " po ", " huwag ", " ay ", " na ", " para ", " ito ", " ka ", " niyo ", " nyo "}

// looksTagalog is a crude check that the tl explanation is Filipino/Taglish
// rather than English or empty. It does NOT replace the human spot-check.
func looksTagalog(s string) bool {
	s = " " + strings.ToLower(s) + " "
	n := 0
	for _, m := range tagalogMarkers {
		if strings.Contains(s, m) {
			n++
		}
	}
	return n >= 2
}

func percentile(v []int64, p float64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := int(p * float64(len(s)-1))
	return s[i]
}

func score(items []Item, models []string, resDir string) error {
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	var all []ModelScore
	for _, model := range models {
		path := filepath.Join(resDir, safeName(model)+".jsonl")
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		ms := ModelScore{Model: model}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var r Record
			if json.Unmarshal(sc.Bytes(), &r) != nil {
				continue
			}
			it, ok := byID[r.ID]
			if !ok {
				continue
			}
			ms.Calls++
			ms.Neurons += r.Neurons
			if r.Err != "" {
				ms.Errors++
				continue
			}
			ms.LatencyMS = append(ms.LatencyMS, r.LatencyMS)
			ms.InTok += r.InTok
			ms.OutTok += r.OutTok
			out, reason := api.ParseAIOutput(r.Raw)
			if reason != "" {
				// An invalid/bad output falls back to rules-only in production.
				ms.Merged.Add(it.Label, r.Rules >= ScamThreshold)
				continue
			}
			ms.Valid++
			ms.AI.Add(it.Label, out.Risk >= ScamThreshold || out.Verdict == "scam" || out.Verdict == "likely_scam")
			pred := merged(r.Rules, out) >= ScamThreshold
			ms.Merged.Add(it.Label, pred)
			if it.Hard {
				ms.MergedHard.Add(it.Label, pred)
			}
			if looksTagalog(out.Explanation.Tl) {
				ms.TagalogOK++
			}
		}
		f.Close()
		if ms.Calls > 0 {
			all = append(all, ms)
		}
	}
	if len(all) == 0 {
		return fmt.Errorf("no results in %s", resDir)
	}
	fmt.Println("| model | n | schema-valid | AI-only acc/prec/rec | merged acc/prec/rec | hard-subset acc | Tagalog-ish tl | p50/p95 latency | in/out tok (avg) | neurons/check | checks per 10k |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|")
	for _, m := range all {
		ok := m.Calls - m.Errors
		avg := func(a int) int { return a / max1(ok) }
		per := m.Neurons / float64(max1(m.Calls))
		fmt.Printf("| %s | %d | %.0f%% (%d/%d) | %.0f/%.0f/%.0f%% | %.0f/%.0f/%.0f%% | %.0f%% | %.0f%% | %d / %d ms | %d / %d | %.1f | %d |\n",
			m.Model, m.Calls, 100*ratio(m.Valid, m.Calls), m.Valid, m.Calls,
			100*m.AI.Accuracy(), 100*m.AI.Precision(), 100*m.AI.Recall(),
			100*m.Merged.Accuracy(), 100*m.Merged.Precision(), 100*m.Merged.Recall(),
			100*m.MergedHard.Accuracy(), 100*ratio(m.TagalogOK, m.Valid),
			percentile(m.LatencyMS, 0.5), percentile(m.LatencyMS, 0.95), avg(m.InTok), avg(m.OutTok), per, int(10000/maxF(per, 0.01)))
	}
	return nil
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
