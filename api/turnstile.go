// Package api: Turnstile verification (optional; fail closed).
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TurnstileVerifyURL is the siteverify endpoint (overridable in tests).
var TurnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// VerifyTurnstile checks a token with a 10 s timeout. It fails closed:
// any error, non-200, success!=true, wrong action, or unknown hostname is a
// failure. Test-key results are accepted only when allowTestKeys is set.
func VerifyTurnstile(secret, token, remoteIP string, hostnames []string, allowTestKeys bool) bool {
	return VerifyTurnstileURL(TurnstileVerifyURL, secret, token, remoteIP, hostnames, allowTestKeys)
}

// VerifyTurnstileURL is the injectable core (tests point it at httptest).
func VerifyTurnstileURL(verifyURL, secret, token, remoteIP string, hostnames []string, allowTestKeys bool) bool {
	if token == "" || len(token) > 2048 || secret == "" {
		return false
	}
	form := url.Values{"secret": {secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false
	}
	var data struct {
		Success  bool   `json:"success"`
		Action   string `json:"action"`
		Hostname string `json:"hostname"`
		Metadata *struct {
			ResultWithTestingKey bool `json:"result_with_testing_key"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	if !data.Success {
		return false
	}
	if data.Metadata != nil && data.Metadata.ResultWithTestingKey {
		return allowTestKeys
	}
	if data.Action != "check" {
		return false
	}
	for _, h := range hostnames {
		if data.Hostname == h {
			return true
		}
	}
	return false
}
