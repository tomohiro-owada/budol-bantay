// Package api: LLM provider client (openai_responses and openai_chat styles).
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LLMResult is one provider call outcome.
type LLMResult struct {
	RawText   string
	Model     string
	InTokens  int
	OutTokens int
	// Allowance is usage.opencode.allowance (a float, in USD) when the
	// provider reports it; 0 otherwise. Numbers only, never logged.
	Allowance float64
	// Neurons is usage.neurons as reported by Workers AI (0 when absent).
	Neurons float64
}

// ProviderError marks a transport/model error. The server maps it to
// ai_status "error" (or "timeout"), except when BadOutput is set, which
// maps to ai_status "bad_output" (a message exists but is unusable).
type ProviderError struct {
	Timeout   bool
	BadOutput bool
	// Status is the HTTP status of a non-2xx provider reply (0 otherwise).
	Status int
	// QuotaExhausted is set when the provider says the free allocation is
	// used up (Workers AI error 4006 / "daily free allocation").
	QuotaExhausted bool
	// Budget is set when OUR daily neuron budget refused the call, so the
	// provider was never contacted.
	Budget bool
	Err    error
}

func (e *ProviderError) Error() string { return e.Err.Error() }

// SessionID returns a random per-request opencode session id.
// It is sent as x-opencode-session; it is never logged.
func SessionID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func estimateTokens(s string) int { return (len(s) + 3) / 4 }

func doPost(ctx context.Context, client *http.Client, url, key, session, userAgent string, payload interface{}) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("User-Agent", userAgent)
	// The session header is opencode-specific; never send it elsewhere.
	if strings.Contains(url, "opencode.ai/") {
		req.Header.Set("x-opencode-session", session)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

// statusError builds the ProviderError for a non-2xx reply. The body is only
// inspected for the free-allocation-exhausted marker; it is never kept.
func statusError(status int, raw []byte) *ProviderError {
	low := strings.ToLower(string(raw))
	exhausted := strings.Contains(low, "daily free allocation") ||
		strings.Contains(low, `"code":4006`) || strings.Contains(low, `"code": 4006`)
	return &ProviderError{Status: status, QuotaExhausted: exhausted, Err: fmt.Errorf("provider status %d", status)}
}

// callResponses implements the openai_responses style:
// POST {base}/responses {model, instructions, input, max_output_tokens,
// reasoning?, temperature?}.
//
// A real-provider live test (opencode Go /responses) showed:
//   - output[0] is {"type":"reasoning"} with no content, output[1] is
//     {"type":"message"} with content [{"type":"output_text","text":...}];
//     both shapes are handled below.
//   - With a high reasoning effort and a small max_output_tokens budget, ALL
//     tokens went to reasoning: status "incomplete",
//     incomplete_details.reason "max_output_tokens", and no message at all.
//
// We therefore NEVER parse the raw body here: a non-completed status or a
// missing output_text is a ProviderError (ai_status "error", or "bad_output"
// when a message item exists but carries no usable text).
func callResponses(ctx context.Context, client *http.Client, cfg Config, system, user, model, session string, omitTemp bool) (LLMResult, *ProviderError) {
	url := strings.TrimRight(cfg.AIBaseURL, "/") + "/responses"
	payload := map[string]interface{}{
		"model":             model,
		"instructions":      system,
		"input":             user,
		"max_output_tokens": cfg.AIMaxTokens,
	}
	// Reasoning effort is sent only for openai_responses, and omitted when
	// empty. Default "minimal" keeps reasoning short so the budget goes to
	// the answer instead of "incomplete" on max_output_tokens.
	if cfg.AIReasoningEffort != "" {
		payload["reasoning"] = map[string]interface{}{"effort": cfg.AIReasoningEffort}
	}
	if !omitTemp {
		payload["temperature"] = cfg.AITemperature
	}
	userAgent := "budol-bantay-api/" + Version
	status, raw, err := doPost(ctx, client, url, cfg.AIKey, session, userAgent, payload)
	if err != nil {
		return LLMResult{}, &ProviderError{Timeout: ctx.Err() == context.DeadlineExceeded, Err: err}
	}
	if status == 400 && !omitTemp && strings.Contains(strings.ToLower(string(raw)), "temperature") {
		// Some reasoning models reject temperature; omit and retry once.
		return callResponses(ctx, client, cfg, system, user, model, session, true)
	}
	if status < 200 || status >= 300 {
		return LLMResult{}, statusError(status, raw)
	}
	var v responsesEnvelope
	if err := json.Unmarshal(raw, &v); err != nil {
		// The envelope itself is unreadable; nothing usable to validate.
		return LLMResult{}, &ProviderError{Err: fmt.Errorf("provider bad envelope: %v", err)}
	}
	text := v.OutputText
	if text == "" {
		var sb strings.Builder
		for _, item := range v.Output {
			for _, c := range item.Content {
				if c.Type == "output_text" && c.Text != "" {
					sb.WriteString(c.Text)
				}
			}
		}
		text = sb.String()
	}
	hasMessage := false
	for _, item := range v.Output {
		if item.Type == "message" {
			hasMessage = true
			break
		}
	}
	if v.Status != "" && v.Status != "completed" {
		return LLMResult{}, &ProviderError{
			BadOutput: hasMessage,
			Err:       fmt.Errorf("provider status %q", v.Status),
		}
	}
	if text == "" {
		// No output_text anywhere: never fall back to the raw body (it is
		// the whole API envelope, not model output). Reasoning-only output
		// is an error; an empty/invalid message is bad_output.
		return LLMResult{}, &ProviderError{
			BadOutput: hasMessage,
			Err:       fmt.Errorf("provider returned no output_text"),
		}
	}
	inTok, outTok := 0, 0
	if v.Usage != nil {
		inTok, outTok = v.Usage.InputTokens, v.Usage.OutputTokens
	}
	if inTok == 0 {
		inTok = estimateTokens(system + user)
	}
	if outTok == 0 {
		outTok = estimateTokens(text)
	}
	return LLMResult{
		RawText: text, Model: model,
		InTokens: inTok, OutTokens: outTok,
		Allowance: allowanceOf(v),
	}, nil
}

// responsesEnvelope is the subset of the openai_responses payload we use:
// status, output_text / output[].content[] text, token usage, and the
// opencode allowance (a float, in USD) reported beside usage.
type responsesEnvelope struct {
	Status     string `json:"status"`
	OutputText string `json:"output_text"`
	Output     []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		Opencode     *struct {
			Allowance json.RawMessage `json:"allowance"`
		} `json:"opencode"`
	} `json:"usage"`
	Opencode *struct {
		Allowance json.RawMessage `json:"allowance"`
	} `json:"opencode"`
}

// parseAllowance parses one allowance value: a JSON number or a quoted
// numeric string. Anything else is 0. Numbers only.
func parseAllowance(raw json.RawMessage) float64 {
	s := strings.TrimSpace(string(raw))
	s = strings.Trim(s, `"`)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0
	}
	return f
}

// allowanceOf reads usage.opencode.allowance (a float, in USD), accepting
// either the nested (usage.opencode) or top-level (opencode) position.
// Non-numeric values are ignored (0). Numbers only.
func allowanceOf(v responsesEnvelope) float64 {
	var nums []json.RawMessage
	if v.Usage != nil && v.Usage.Opencode != nil {
		nums = append(nums, v.Usage.Opencode.Allowance)
	}
	if v.Opencode != nil {
		nums = append(nums, v.Opencode.Allowance)
	}
	for _, n := range nums {
		if f := parseAllowance(n); f > 0 {
			return f
		}
	}
	return 0
}

// callChat implements the openai_chat style:
// POST {base}/chat/completions with system+user messages.
func callChat(ctx context.Context, client *http.Client, cfg Config, system, user, model, session string) (LLMResult, *ProviderError) {
	url := strings.TrimRight(cfg.AIBaseURL, "/") + "/chat/completions"
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"max_tokens":  cfg.AIMaxTokens,
		"temperature": cfg.AITemperature,
	}
	if cfg.AIJSONMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	if cfg.AIChatReasoningEffort != "" {
		payload["reasoning_effort"] = cfg.AIChatReasoningEffort
	}
	userAgent := "budol-bantay-api/" + Version
	status, raw, err := doPost(ctx, client, url, cfg.AIKey, session, userAgent, payload)
	if err != nil {
		return LLMResult{}, &ProviderError{Timeout: ctx.Err() == context.DeadlineExceeded, Err: err}
	}
	if status < 200 || status >= 300 {
		return LLMResult{}, statusError(status, raw)
	}
	var v struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int     `json:"prompt_tokens"`
			CompletionTokens int     `json:"completion_tokens"`
			Neurons          float64 `json:"neurons"`
		} `json:"usage"`
	}
	text := ""
	inTok, outTok := 0, 0
	reported := 0.0
	if err := json.Unmarshal(raw, &v); err == nil {
		if len(v.Choices) > 0 {
			text = v.Choices[0].Message.Content
		}
		if v.Usage != nil {
			inTok, outTok = v.Usage.PromptTokens, v.Usage.CompletionTokens
			reported = v.Usage.Neurons
		}
	}
	if strings.TrimSpace(text) == "" {
		// Never parse the raw envelope as model output.
		return LLMResult{}, &ProviderError{BadOutput: true, Err: fmt.Errorf("empty chat completion content")}
	}
	if inTok == 0 {
		inTok = estimateTokens(system + user)
	}
	if outTok == 0 {
		outTok = estimateTokens(text)
	}
	return LLMResult{RawText: text, Model: model, InTokens: inTok, OutTokens: outTok, Neurons: reported}, nil
}

// CallLLM runs one provider call in the configured api_style.
func CallLLM(ctx context.Context, client *http.Client, cfg Config, system, user, model, session string) (LLMResult, *ProviderError) {
	return CallLLMWithStyle(ctx, client, cfg, cfg.AIStyle, system, user, model, session)
}

// CallLLMWithStyle runs one provider call with an explicit api_style
// (used for the fallback model, which may live behind the other style).
func CallLLMWithStyle(ctx context.Context, client *http.Client, cfg Config, style, system, user, model, session string) (LLMResult, *ProviderError) {
	if style == "openai_chat" {
		return callChat(ctx, client, cfg, system, user, model, session)
	}
	return callResponses(ctx, client, cfg, system, user, model, session, false)
}
