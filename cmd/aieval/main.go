// Command aieval runs the fictional eval messages through ONE provider/model
// using the production prompt, parser and merge rule, and prints per-message
// verdict quality, latency and token usage. It never prints keys or headers.
//
// Usage (anonymous OVHcloud example, no key):
//
//	AI_BASE_URL=https://oai.endpoints.kepler.ai.cloud.ovh.net/v1 AI_API_STYLE=openai_chat \
//	AI_MODEL=gpt-oss-120b AI_KEY_ENV= go run ./cmd/aieval -gap 31
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"scam-checker/api"
	"scam-checker/engine"
)

//go:embed messages.json
var messagesJSON []byte

type msg struct {
	ID     string `json:"id"`
	Expect string `json:"expect"`
	Sender string `json:"sender"`
	Text   string `json:"text"`
}

func main() {
	gap := flag.Int("gap", 0, "seconds to wait between calls (rate limits)")
	only := flag.String("only", "", "comma-separated message ids")
	dump := flag.String("dump", "", "directory to write raw model outputs (fictional eval data only)")
	flag.Parse()
	var msgs []msg
	if err := json.Unmarshal(messagesJSON, &msgs); err != nil {
		panic(err)
	}
	cfg := api.LoadConfig()
	client := &http.Client{Timeout: time.Duration(cfg.AITimeoutSecs) * time.Second}
	correct, n := 0, 0
	for i, m := range msgs {
		if *only != "" && !strings.Contains(","+*only+",", ","+m.ID+",") {
			continue
		}
		if i > 0 && *gap > 0 && n > 0 {
			time.Sleep(time.Duration(*gap) * time.Second)
		}
		n++
		masked := engine.Mask(m.Text)
		res := engine.Analyze(masked.Masked, m.Sender)
		var ids []string
		for _, h := range res.Hits {
			ids = append(ids, h.ID)
		}
		system, user := api.BuildPrompt(masked.Masked, "tl", api.RulesHints{Score: res.Score, Level: res.Level, Flags: ids})
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.AITimeoutSecs)*time.Second)
		start := time.Now()
		r, perr := api.CallLLM(ctx, client, cfg, system, user, cfg.AIModel, api.SessionID())
		lat := time.Since(start).Milliseconds()
		cancel()
		if perr != nil {
			fmt.Printf("%-17s expect=%-5s rules=%3d ERROR %v latency_ms=%d\n", m.ID, m.Expect, res.Score, perr.Err, lat)
			continue
		}
		if *dump != "" {
			_ = os.WriteFile(*dump+"/"+m.ID+".txt", []byte(r.RawText), 0o600)
		}
		out, reason := api.ParseAIOutput(r.RawText)
		if reason != "" {
			fmt.Printf("%-17s expect=%-5s rules=%3d BAD_OUTPUT(%s) latency_ms=%d in=%d out=%d\n", m.ID, m.Expect, res.Score, reason, lat, r.InTokens, r.OutTokens)
			continue
		}
		final := out.Risk
		if res.Score >= 30 && res.Score > final {
			final = res.Score
		}
		if out.InjectionDetected && final < 40 {
			final = 40
		}
		ok := (m.Expect == "scam" && final >= 60) || (m.Expect == "legit" && final < 30)
		if ok {
			correct++
		}
		tl := out.Explanation.Tl
		if len([]rune(tl)) > 150 {
			tl = string([]rune(tl)[:150]) + "…"
		}
		fmt.Printf("%-17s expect=%-5s rules=%3d ai=%-12s risk=%3d final=%3d inj=%-5v ok=%-5v latency_ms=%d in=%d out=%d | %s\n",
			m.ID, m.Expect, res.Score, out.Verdict, out.Risk, final, out.InjectionDetected, ok, lat, r.InTokens, r.OutTokens, tl)
	}
	fmt.Printf("SUMMARY model=%s correct=%d/%d\n", cfg.AIModel, correct, n)
	if n == 0 {
		os.Exit(1)
	}
}
