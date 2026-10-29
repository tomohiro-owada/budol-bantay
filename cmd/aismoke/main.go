// Command aismoke sends ONE fictional example (corpus example 1) through the
// configured LLM provider and prints only ai_status, model, verdict, risk
// and latency. It NEVER prints the key or request headers.
//
// The key comes from the environment variable named by key_env
// (default AI_API_KEY). Do NOT run this unless you intend to spend a
// real provider call.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"scam-checker/api"
	"scam-checker/engine"
)

// fictionalExample mirrors corpus poc-01 (crafted, fictional).
const fictionalExample = "GCash: Ang iyong account ay pansamantalang na-lock dahil sa kahina-hinalang aktibidad. I-verify ang account within 24 hours para hindi ma-deactivate: https://gcash-ph-verify.top/login"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "aismoke: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := api.LoadConfig()
	if cfg.AIMode == "mock" {
		if !cfg.MockAllowed() {
			return fmt.Errorf("refusing AI_MODE=mock unless BUDOL_ENV=dev")
		}
		return runMock()
	}
	if cfg.AIKey == "" {
		return fmt.Errorf("no key: env %s is empty", cfg.AIKeyEnv)
	}
	masked := engine.Mask(fictionalExample)
	res := engine.Analyze(masked.Masked, "GCash")
	var ids []string
	for _, h := range res.Hits {
		ids = append(ids, h.ID)
	}
	system, user := api.BuildPrompt(masked.Masked, "tl", api.RulesHints{
		Score: res.Score, Level: res.Level, Flags: ids,
	})
	client := &http.Client{Timeout: time.Duration(cfg.AITimeoutSecs) * time.Second}
	session := api.SessionID()
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.AITimeoutSecs)*time.Second)
	defer cancel()
	res1, perr := api.CallLLM(ctx, client, cfg, system, user, cfg.AIModel, session)
	model := cfg.AIModel
	if perr != nil && cfg.AIFallbackModel != "" && cfg.AIFallbackModel != cfg.AIModel {
		style := cfg.AIFallbackStyle
		if style == "" {
			style = cfg.AIStyle
		}
		res2, perr2 := api.CallLLMWithStyle(ctx, client, cfg, style, system, user, cfg.AIFallbackModel, session)
		if perr2 == nil {
			res1, perr = res2, nil
			model = cfg.AIFallbackModel
		}
	}
	latency := time.Since(start)
	if perr != nil {
		status := "error"
		if perr.Timeout || ctx.Err() == context.DeadlineExceeded {
			status = "timeout"
		}
		fmt.Printf("ai_status=%s model=%s latency_ms=%d\n", status, model, latency.Milliseconds())
		return fmt.Errorf("provider call failed")
	}
	out, reason := api.ParseAIOutput(res1.RawText)
	if reason != "" {
		fmt.Printf("ai_status=bad_output model=%s latency_ms=%d\n", res1.Model, latency.Milliseconds())
		return fmt.Errorf("bad output: %s", reason)
	}
	fmt.Printf("ai_status=ok model=%s verdict=%s risk=%d latency_ms=%d\n",
		res1.Model, out.Verdict, out.Risk, latency.Milliseconds())
	return nil
}

func runMock() error {
	masked := engine.Mask(fictionalExample)
	res := engine.Analyze(masked.Masked, "GCash")
	var ids []string
	for _, h := range res.Hits {
		ids = append(ids, h.ID)
	}
	start := time.Now()
	raw, err := api.RunMockAI(masked.Masked, api.RulesHints{Score: res.Score, Level: res.Level, Flags: ids})
	latency := time.Since(start)
	if err != nil {
		fmt.Printf("ai_status=error model=%s latency_ms=%d\n", api.MockModelID, latency.Milliseconds())
		return err
	}
	out, reason := api.ParseAIOutput(raw)
	if reason != "" {
		fmt.Printf("ai_status=bad_output model=%s latency_ms=%d\n", api.MockModelID, latency.Milliseconds())
		return fmt.Errorf("bad output: %s", reason)
	}
	fmt.Printf("ai_status=ok model=%s verdict=%s risk=%d latency_ms=%d\n",
		api.MockModelID, out.Verdict, out.Risk, latency.Milliseconds())
	return nil
}
