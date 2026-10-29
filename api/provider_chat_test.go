package api

import (
	"os"
	"testing"
)

func chatCfg() Config {
	c := responsesCfg("")
	c.AIStyle = "openai_chat"
	return c
}

func chatReply(t *testing.T) string {
	return `{"choices":[{"message":{"content":` + quoted(t, validAI(t, "scam", "otp_theft", 90, false)) + `}}],"usage":{"prompt_tokens":800,"completion_tokens":600}}`
}

// Non-opencode providers must not receive the opencode session header, and
// JSON mode / chat reasoning effort are sent only when configured.
func TestChatHeadersAndOptionalParams(t *testing.T) {
	got, res, perr := callOnce(t, chatCfg(), chatReply(t))
	if perr != nil {
		t.Fatalf("call: %v", perr.Err)
	}
	if got.sess != "" {
		t.Fatalf("x-opencode-session leaked to non-opencode provider: %q", got.sess)
	}
	if got.auth != "Bearer test-key" {
		t.Fatalf("auth header missing")
	}
	if _, ok := got.body["response_format"]; ok {
		t.Fatalf("response_format must be omitted by default")
	}
	if _, ok := got.body["reasoning_effort"]; ok {
		t.Fatalf("reasoning_effort must be omitted by default")
	}
	if res.InTokens != 800 || res.OutTokens != 600 {
		t.Fatalf("usage not parsed: %+v", res)
	}

	c := chatCfg()
	c.AIJSONMode = true
	c.AIChatReasoningEffort = "low"
	got2, _, perr := callOnce(t, c, chatReply(t))
	if perr != nil {
		t.Fatalf("call: %v", perr.Err)
	}
	rf, _ := got2.body["response_format"].(map[string]interface{})
	if rf["type"] != "json_object" {
		t.Fatalf("response_format = %v", got2.body["response_format"])
	}
	if got2.body["reasoning_effort"] != "low" {
		t.Fatalf("reasoning_effort = %v", got2.body["reasoning_effort"])
	}
}

// No key configured (e.g. local testing) -> no Authorization header at all.
func TestChatNoKeyNoAuthHeader(t *testing.T) {
	c := chatCfg()
	c.AIKey = ""
	got, _, perr := callOnce(t, c, chatReply(t))
	if perr != nil {
		t.Fatalf("call: %v", perr.Err)
	}
	if got.auth != "" {
		t.Fatalf("Authorization sent without a key")
	}
}

// An empty chat completion (e.g. a reasoning model that spent its budget)
// is bad_output; the raw envelope is never parsed as model output.
func TestChatEmptyContentIsBadOutput(t *testing.T) {
	_, _, perr := callOnce(t, chatCfg(), `{"choices":[{"message":{"content":""}}]}`)
	if perr == nil || !perr.BadOutput {
		t.Fatalf("want BadOutput error, got %v", perr)
	}
}

func TestExpandBaseURLOnlyAccountID(t *testing.T) {
	os.Setenv("CF_ACCOUNT_ID", "abc123")
	os.Setenv("AI_API_KEY", "should-never-appear")
	defer os.Unsetenv("CF_ACCOUNT_ID")
	defer os.Unsetenv("AI_API_KEY")
	got := ExpandBaseURL("https://api.cloudflare.com/client/v4/accounts/${CF_ACCOUNT_ID}/ai/v1")
	if got != "https://api.cloudflare.com/client/v4/accounts/abc123/ai/v1" {
		t.Fatalf("got %q", got)
	}
	if ExpandBaseURL("https://x/${AI_API_KEY}") != "https://x/${AI_API_KEY}" {
		t.Fatalf("must not expand other variables")
	}
}

func TestAIConfiguredNeedsAccountID(t *testing.T) {
	c := Config{AIKey: "k", AIBaseURL: "https://api.cloudflare.com/client/v4/accounts//ai/v1"}
	if c.AIConfigured() {
		t.Fatal("missing CF account id must disable AI")
	}
	c.AIBaseURL = "https://api.cloudflare.com/client/v4/accounts/abc/ai/v1"
	if !c.AIConfigured() {
		t.Fatal("should be configured")
	}
}
