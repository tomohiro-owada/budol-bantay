// Package api: strict validation of LLM JSON output, ported from
// worker/src/schema.ts. We ALWAYS parse and validate ourselves.
package api

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Verdicts and scam types: the only allowed values. Never "safe"/"legit".
var verdictSet = map[string]bool{
	"scam": true, "likely_scam": true, "suspicious": true,
	"unclear": true, "no_red_flags": true,
}

var scamTypeSet = map[string]bool{
	"phishing_link": true, "otp_theft": true, "fake_prize": true,
	"advance_fee": true, "job_task": true, "parcel": true, "loan": true,
	"gov_impersonation": true, "bank_ewallet_impersonation": true,
	"investment_crypto": true, "family_emergency": true, "romance": true,
	"other": true, "none": true,
}

const (
	maxRedFlags     = 5
	maxActions      = 4
	maxFlagChars    = 200
	maxExplainChars = 600
	maxActionChars  = 200
)

var aiKeys = []string{
	"verdict", "risk", "scam_type", "red_flags",
	"explanation", "actions", "injection_detected",
}

// Verdict-claim heuristic: refuse outputs that declare the message safe or
// legit. Never claim certainty that a message is safe.
var safeClaimREs = []*regexp.Regexp{
	regexp.MustCompile(`(?i)this is safe`),
	regexp.MustCompile(`(?i)\bit is safe\b`),
	regexp.MustCompile(`(?i)\bis safe\b`),
	regexp.MustCompile(`(?i)ligtas ito`),
	regexp.MustCompile(`(?i)\bis legit\b`), // not "is legitimate" (e.g. "verify it is legitimate")
	regexp.MustCompile(`安全です`),
}

// Lang3 holds tl/en/ja strings.
type Lang3 struct {
	Tl string `json:"tl"`
	En string `json:"en"`
	Ja string `json:"ja"`
}

// AIOutput is the validated model verdict.
type AIOutput struct {
	Verdict           string  `json:"verdict"`
	Risk              int     `json:"risk"`
	ScamType          string  `json:"scam_type"`
	RedFlags          []Lang3 `json:"red_flags"`
	Explanation       Lang3   `json:"explanation"`
	Actions           []Lang3 `json:"actions"`
	InjectionDetected bool    `json:"injection_detected"`
}

func textsOf(o AIOutput) []string {
	var out []string
	for _, f := range o.RedFlags {
		out = append(out, f.Tl, f.En, f.Ja)
	}
	out = append(out, o.Explanation.Tl, o.Explanation.En, o.Explanation.Ja)
	for _, a := range o.Actions {
		out = append(out, a.Tl, a.En, a.Ja)
	}
	return out
}

// HasSafeClaim reports whether any output text makes a "safe" verdict claim.
func HasSafeClaim(o AIOutput) bool {
	for _, t := range textsOf(o) {
		for _, re := range safeClaimREs {
			if re.MatchString(t) {
				return true
			}
		}
	}
	return false
}

func validLang3(m map[string]interface{}, maxLen int) (Lang3, bool) {
	if len(m) != 3 {
		return Lang3{}, false
	}
	get := func(k string) (string, bool) {
		v, ok := m[k]
		if !ok {
			return "", false
		}
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		if !ok || s == "" {
			return "", false
		}
		// Limits are in characters (runes), not bytes: Japanese is 3 bytes
		// per char. Over-long strings are truncated, not rejected, so a
		// verbose model still yields a usable (bounded) result.
		return truncateRunes(s, maxLen), true
	}
	tl, ok := get("tl")
	if !ok {
		return Lang3{}, false
	}
	en, ok := get("en")
	if !ok {
		return Lang3{}, false
	}
	ja, ok := get("ja")
	if !ok {
		return Lang3{}, false
	}
	return Lang3{Tl: tl, En: en, Ja: ja}, true
}

// ValidateAIOutput strictly validates a decoded JSON value. It rejects
// anything with extra/missing keys, bad enums, bad lengths, or safe claims.
func ValidateAIOutput(v interface{}) (AIOutput, string) {
	var zero AIOutput
	m, ok := v.(map[string]interface{})
	if !ok {
		return zero, "not an object"
	}
	for k := range m {
		found := false
		for _, want := range aiKeys {
			if k == want {
				found = true
				break
			}
		}
		if !found {
			return zero, "extra key: " + k
		}
	}
	for _, want := range aiKeys {
		if _, ok := m[want]; !ok {
			return zero, "missing key: " + want
		}
	}
	verdict, _ := m["verdict"].(string)
	if !verdictSet[verdict] {
		return zero, "bad verdict"
	}
	riskF, ok := m["risk"].(float64)
	if !ok || riskF < 0 || riskF > 100 {
		return zero, "bad risk"
	}
	riskF = float64(int(riskF + 0.5)) // tolerate 72.5-style floats
	scamType, _ := m["scam_type"].(string)
	if !scamTypeSet[scamType] {
		return zero, "bad scam_type"
	}
	rf, ok := normalizeLangList(m["red_flags"])
	if !ok {
		return zero, "bad red_flags"
	}
	if len(rf) > maxRedFlags {
		rf = rf[:maxRedFlags] // keep the first N; never unbounded
	}
	flags := make([]Lang3, 0, len(rf))
	for _, item := range rf {
		im, ok := item.(map[string]interface{})
		if !ok {
			return zero, "bad red_flag item"
		}
		l, ok := validLang3(im, maxFlagChars)
		if !ok {
			return zero, "bad red_flag item"
		}
		flags = append(flags, l)
	}
	em, ok := m["explanation"].(map[string]interface{})
	if !ok {
		return zero, "bad explanation"
	}
	expl, ok := validLang3(em, maxExplainChars)
	if !ok {
		return zero, "bad explanation"
	}
	ac, ok := normalizeLangList(m["actions"])
	if !ok {
		return zero, "bad actions"
	}
	if len(ac) > maxActions {
		ac = ac[:maxActions]
	}
	actions := make([]Lang3, 0, len(ac))
	for _, item := range ac {
		im, ok := item.(map[string]interface{})
		if !ok {
			return zero, "bad action item"
		}
		l, ok := validLang3(im, maxActionChars)
		if !ok {
			return zero, "bad action item"
		}
		actions = append(actions, l)
	}
	inj, ok := m["injection_detected"].(bool)
	if !ok {
		return zero, "bad injection_detected"
	}
	out := AIOutput{
		Verdict: verdict, Risk: int(riskF), ScamType: scamType,
		RedFlags: flags, Explanation: expl, Actions: actions,
		InjectionDetected: inj,
	}
	if HasSafeClaim(out) {
		return zero, "safe claim"
	}
	return out, ""
}

// ParseAIOutput extracts the first balanced JSON object from raw model text
// and strictly validates it. It returns the output or a short error reason.
func ParseAIOutput(raw string) (AIOutput, string) {
	var zero AIOutput
	js := ExtractJSON(raw)
	if js == "" {
		return zero, "no json"
	}
	var v interface{}
	if err := json.Unmarshal([]byte(js), &v); err != nil {
		return zero, "bad json"
	}
	return ValidateAIOutput(v)
}

// ExtractJSON robustly pulls the first balanced {...} object out of raw
// model text, stripping Markdown code fences first. It respects JSON strings
// and escapes while scanning.
func ExtractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	// Strip one layer of ``` fences (optionally ```json ... ```).
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s[3:], "```"); idx >= 0 {
			inner := s[3 : 3+idx]
			if nl := strings.Index(inner, "\n"); nl >= 0 {
				first := strings.TrimSpace(inner[:nl])
				if first == "" || strings.EqualFold(first, "json") {
					s = strings.TrimSpace(inner[nl+1:])
				} else {
					s = strings.TrimSpace(inner)
				}
			} else {
				s = strings.TrimSpace(inner)
			}
		} else {
			// Unclosed fence: drop the fence line.
			if nl := strings.Index(s, "\n"); nl >= 0 {
				s = strings.TrimSpace(s[nl+1:])
			}
		}
	}
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// truncateRunes cuts s to at most n runes, ending with "…" when cut.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// normalizeLangList repairs two common shape slips seen with open-weight
// models, without inventing content:
//   - a single {"tl","en","ja"} object instead of an array -> [obj]
//   - each language split into its own item ([{"tl"},{"en"},{"ja"}]) ->
//     consecutive single-language items are merged into one object.
//
// Anything else is returned unchanged for strict validation.
func normalizeLangList(v interface{}) ([]interface{}, bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		return []interface{}{t}, true
	case []interface{}:
		out := make([]interface{}, 0, len(t))
		var cur map[string]interface{}
		for _, item := range t {
			im, ok := item.(map[string]interface{})
			if !ok || len(im) != 1 {
				if cur != nil {
					out = append(out, cur)
					cur = nil
				}
				out = append(out, item)
				continue
			}
			var k string
			for kk := range im {
				k = kk
			}
			if k != "tl" && k != "en" && k != "ja" {
				out = append(out, item)
				continue
			}
			if cur == nil {
				cur = map[string]interface{}{}
			}
			if _, dup := cur[k]; dup {
				out = append(out, cur)
				cur = map[string]interface{}{}
			}
			cur[k] = im[k]
			if len(cur) == 3 {
				out = append(out, cur)
				cur = nil
			}
		}
		if cur != nil {
			out = append(out, cur)
		}
		return out, true
	}
	return nil, false
}
