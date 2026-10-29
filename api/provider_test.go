// Tests for the openai_responses provider client: incomplete statuses,
// reasoning-only output, the reasoning effort param, and the opencode
// allowance counter. See the live-test notes in provider.go.
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func providerServerMut(t *testing.T, style string, transcript *capturedLLM, reply string, status int, mut func(*Config)) (*Server, *httptest.Server) {
	t.Helper()
	llm := fakeLLM(t, transcript, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	})
	srv, _ := newTestServer(t, func(c *Config) {
		c.AIMode = ""
		c.AIKey = "test-key"
		c.AIStyle = style
		c.AIBaseURL = llm.URL
		if mut != nil {
			mut(c)
		}
	})
	return srv, llm
}

func responsesCfg(baseURL string) Config {
	return Config{
		AIStyle:           "openai_responses",
		AIBaseURL:         baseURL,
		AIModel:           "test-model",
		AIMaxTokens:       3000,
		AITemperature:     0.2,
		AIReasoningEffort: "minimal",
		AIKey:             "test-key",
	}
}

func callOnce(t *testing.T, cfg Config, reply string) (capturedLLM, LLMResult, *ProviderError) {
	t.Helper()
	var got capturedLLM
	llm := fakeLLM(t, &got, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	})
	defer llm.Close()
	cfg.AIBaseURL = llm.URL
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, perr := CallLLMWithStyle(ctx, &http.Client{}, cfg, cfg.AIStyle, "sys", "user <message>x</message>", cfg.AIModel, "sess")
	return got, res, perr
}

// The live failure: high effort + max_output_tokens=1200 spent everything on
// reasoning -> status incomplete, reasoning-only, no message. That is a
// ProviderError WITHOUT BadOutput (ai_status "error", not "bad_output").
func TestResponsesIncompleteReasoningOnly(t *testing.T) {
	reply := `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},` +
		`"output":[{"type":"reasoning"}],"usage":{"input_tokens":900,"output_tokens":1200}}`
	_, _, perr := callOnce(t, responsesCfg(""), reply)
	if perr == nil {
		t.Fatal("incomplete reasoning-only must be a ProviderError")
	}
	if perr.BadOutput {
		t.Fatalf("reasoning-only must not be BadOutput: %v", perr.Err)
	}

	// End to end: still HTTP 200 with rules, ai_status error.
	var got capturedLLM
	srv, llm := providerServerMut(t, "openai_responses", &got, reply, 200, nil)
	defer llm.Close()
	rec := postCheck(t, srv, "", `{"text":"GCash: i-verify ang account dito http://example.com/login"}`, jsonCT)
	v := checkJSON(t, rec.Body.String())
	if rec.Code != 200 || v["ai_status"] != "error" || v["ai"] != nil {
		t.Fatalf("incomplete = %d %s", rec.Code, rec.Body.String())
	}

	// Incomplete WITH a (partial) message item -> bad_output, not error.
	partial, _ := json.Marshal("partial {\"verdict\":")
	reply2 := `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},` +
		`"output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":` +
		string(partial) + `}]}]}`
	srv2, llm2 := providerServerMut(t, "openai_responses", &got, reply2, 200, nil)
	defer llm2.Close()
	rec = postCheck(t, srv2, "", `{"text":"GCash: i-verify ang account dito http://example.com/login"}`, jsonCT)
	v = checkJSON(t, rec.Body.String())
	if rec.Code != 200 || v["ai_status"] != "bad_output" || v["ai"] != nil {
		t.Fatalf("incomplete+message = %d %s", rec.Code, rec.Body.String())
	}
}

// Status completed but reasoning-only output (no output_text anywhere) is
// an error, never a raw-body parse.
func TestResponsesReasoningOnlyCompleted(t *testing.T) {
	_, _, perr := callOnce(t, responsesCfg(""), `{"status":"completed","output":[{"type":"reasoning"}]}`)
	if perr == nil || perr.BadOutput {
		t.Fatalf("reasoning-only must be a plain ProviderError, got %+v", perr)
	}
}

// The live success shape: reasoning item first, then a message item with
// output_text content. Usage + allowance are recorded.
func TestResponsesReasoningThenMessage(t *testing.T) {
	reply := `{"status":"completed","output":[{"type":"reasoning"},` +
		`{"type":"message","content":[{"type":"output_text","text":` +
		quoted(t, validAI(t, "suspicious", "phishing_link", 55, false)) + `}]}],` +
		`"usage":{"input_tokens":980,"output_tokens":42},"opencode":{"allowance":1.25}}`
	_, res, perr := callOnce(t, responsesCfg(""), reply)
	if perr != nil {
		t.Fatalf("valid shape must succeed: %v", perr.Err)
	}
	if res.InTokens != 980 || res.OutTokens != 42 {
		t.Fatalf("usage = %d/%d", res.InTokens, res.OutTokens)
	}
	if res.Allowance != 1.25 {
		t.Fatalf("allowance = %v", res.Allowance)
	}
	if _, reason := ParseAIOutput(res.RawText); reason != "" {
		t.Fatalf("output_text not valid AI JSON: %s", reason)
	}
}

// The reasoning effort param is sent for openai_responses and omitted when
// the configured effort is empty.
func TestResponsesReasoningParam(t *testing.T) {
	reply := `{"status":"completed","output_text":` + quoted(t, validAI(t, "no_red_flags", "none", 5, false)) + `}`
	got, _, perr := callOnce(t, responsesCfg(""), reply)
	if perr != nil {
		t.Fatalf("call: %v", perr.Err)
	}
	reasoning, ok := got.body["reasoning"].(map[string]interface{})
	if !ok || reasoning["effort"] != "minimal" {
		t.Fatalf("reasoning param = %v (body=%v)", got.body["reasoning"], got.body)
	}

	cfg := responsesCfg("")
	cfg.AIReasoningEffort = ""
	got2, _, perr := callOnce(t, cfg, reply)
	if perr != nil {
		t.Fatalf("call: %v", perr.Err)
	}
	if _, present := got2.body["reasoning"]; present {
		t.Fatalf("reasoning must be omitted when effort is empty: %v", got2.body)
	}

	// openai_chat never sends reasoning.
	chatCfg := responsesCfg("")
	chatCfg.AIStyle = "openai_chat"
	chatReply := `{"choices":[{"message":{"content":` + quoted(t, validAI(t, "no_red_flags", "none", 5, false)) + `}}]}`
	got3, _, perr := callOnce(t, chatCfg, chatReply)
	if perr != nil {
		t.Fatalf("chat call: %v", perr.Err)
	}
	if _, present := got3.body["reasoning"]; present {
		t.Fatalf("chat must not send reasoning: %v", got3.body)
	}
}

// usage.opencode.allowance (a float, in USD) is recorded into the counters
// as a running daily sum (number only), in both envelope positions.
func TestResponsesAllowanceCounter(t *testing.T) {
	mkReply := func(allowance string) string {
		return `{"status":"completed","output_text":` + quoted(t, validAI(t, "no_red_flags", "none", 5, false)) +
			`,"usage":{"input_tokens":10,"output_tokens":20},` + allowance + `}`
	}
	var got capturedLLM
	srv, llm := providerServerMut(t, "openai_responses", &got,
		mkReply(`"opencode":{"allowance":1.5}`), 200, nil)
	defer llm.Close()
	body := `{"text":"Magandang araw! Paalala lang na magdala ng payong bukas."}`
	for i := 0; i < 2; i++ {
		rec := postCheck(t, srv, "", body, jsonCT)
		if v := checkJSON(t, rec.Body.String()); v["ai_status"] != "ok" {
			t.Fatalf("call %d = %d %s", i, rec.Code, rec.Body.String())
		}
	}
	srv2, llm2 := providerServerMut(t, "openai_responses", &got,
		mkReply(`"opencode":{"allowance":"0.75"}`), 200, nil)
	defer llm2.Close()
	_ = srv2 // nested-string form parses at the unit level below

	req := httptest.NewRequest("GET", "/v1/stats", nil)
	req.Header.Set("Authorization", "Bearer test-stats-token")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	var v struct {
		Counters map[string]float64 `json:"counters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Counters["ai_allowance_usd"] != 3.0 {
		t.Fatalf("allowance sum = %v", v.Counters["ai_allowance_usd"])
	}
	if v.Counters["ai_in_tokens"] != 20 || v.Counters["ai_out_tokens"] != 40 {
		t.Fatalf("tokens = %v", v.Counters)
	}

	// Nested usage.opencode.allowance position, incl. quoted-string form.
	for _, pos := range []string{
		`"usage":{"input_tokens":1,"output_tokens":1,"opencode":{"allowance":0.75}}`,
		`"usage":{"input_tokens":1,"output_tokens":1},"opencode":{"allowance":"0.75"}`,
	} {
		reply := `{"status":"completed","output_text":` + quoted(t, validAI(t, "no_red_flags", "none", 5, false)) + `,` + pos + `}`
		_, res, perr := callOnce(t, responsesCfg(""), reply)
		if perr != nil {
			t.Fatalf("call: %v", perr.Err)
		}
		if res.Allowance != 0.75 {
			t.Fatalf("allowance(%s) = %v", pos, res.Allowance)
		}
	}
	// Non-numeric allowance is ignored, not an error.
	bad := `{"status":"completed","output_text":` + quoted(t, validAI(t, "no_red_flags", "none", 5, false)) +
		`,"opencode":{"allowance":"n/a"}}`
	_, res, perr := callOnce(t, responsesCfg(""), bad)
	if perr != nil || res.Allowance != 0 {
		t.Fatalf("non-numeric allowance = %v %+v", res.Allowance, perr)
	}
}

// The raw API envelope must never leak into model output: with no
// output_text, RawText stays empty even though the body is valid JSON.
func TestResponsesNeverParsesRawBody(t *testing.T) {
	_, res, perr := callOnce(t, responsesCfg(""), `{"status":"completed","id":"resp_123","usage":{"input_tokens":5,"output_tokens":5}}`)
	if perr == nil {
		t.Fatal("missing output_text must be a ProviderError")
	}
	if res.RawText != "" {
		t.Fatalf("RawText must be empty, got %q", res.RawText)
	}
	if strings.Contains(perr.Error(), "resp_123") {
		t.Fatalf("error must not carry envelope content: %v", perr.Err)
	}
}
