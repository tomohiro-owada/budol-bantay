package engine

import (
	"crypto/sha256"
	"fmt"
)

// RulesVersion returns a short hash of the embedded rules.json so clients
// can tell which engine data produced a score.
func RulesVersion() string {
	sum := sha256.Sum256(rulesJSON)
	return fmt.Sprintf("%x", sum[:])[:12]
}

// RuleText returns the tl/en/ja explanation for a rule id.
func RuleText(id string) (LangText, bool) {
	r, ok := ruleByID[id]
	if !ok {
		return LangText{}, false
	}
	return LangText{Tl: r.Tl, En: r.En, Ja: r.Ja}, true
}

// BandFor maps a 0..100 score to a band using the rules.json thresholds.
func BandFor(score int) string {
	switch {
	case score >= levelHi:
		return "hi"
	case score >= levelMid:
		return "mid"
	case score >= levelLo:
		return "lo"
	default:
		return "none"
	}
}
