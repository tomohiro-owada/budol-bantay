package api

import (
	"strings"
	"testing"
)

// Shape slips seen live with open-weight models (gpt-oss, Llama 3.3) are
// repaired without inventing content.
func TestNormalizeSplitLanguageItemsAndObjectActions(t *testing.T) {
	raw := `{"verdict":"scam","risk":80,"scam_type":"otp_theft",
"red_flags":[{"tl":"Humihingi ng code."},{"en":"Asks for a code."},{"ja":"コードを要求。"}],
"explanation":{"tl":"a","en":"b","ja":"c"},
"actions":{"tl":"Huwag ibigay.","en":"Don't share.","ja":"共有しない。"},
"injection_detected":false}`
	out, reason := ParseAIOutput(raw)
	if reason != "" {
		t.Fatalf("reason=%s", reason)
	}
	if len(out.RedFlags) != 1 || out.RedFlags[0].Ja != "コードを要求。" || out.RedFlags[0].Tl != "Humihingi ng code." {
		t.Fatalf("red flags not merged: %+v", out.RedFlags)
	}
	if len(out.Actions) != 1 || out.Actions[0].En != "Don't share." {
		t.Fatalf("actions not wrapped: %+v", out.Actions)
	}
}

// Limits are characters, not bytes; over-long text is truncated and lists
// are capped, so output stays bounded.
func TestLongJapaneseTruncatedAndListsCapped(t *testing.T) {
	ja := strings.Repeat("詐", 250) // 250 chars = 750 bytes
	item := `{"tl":"x","en":"y","ja":"` + ja + `"}`
	raw := `{"verdict":"scam","risk":72.6,"scam_type":"other","red_flags":[` +
		strings.Repeat(item+",", 6) + item + `],"explanation":{"tl":"a","en":"b","ja":"c"},"actions":[],"injection_detected":false}`
	out, reason := ParseAIOutput(raw)
	if reason != "" {
		t.Fatalf("reason=%s", reason)
	}
	if len(out.RedFlags) != maxRedFlags {
		t.Fatalf("want %d flags, got %d", maxRedFlags, len(out.RedFlags))
	}
	if n := len([]rune(out.RedFlags[0].Ja)); n != maxFlagChars {
		t.Fatalf("ja runes = %d", n)
	}
	if out.Risk != 73 {
		t.Fatalf("risk = %d", out.Risk)
	}
}

func TestLegitimateIsNotASafeClaim(t *testing.T) {
	o := AIOutput{Explanation: Lang3{Tl: "x", En: "Verify via the official app to ensure it is legitimate.", Ja: "y"}}
	if HasSafeClaim(o) {
		t.Fatal("'is legitimate' must not count as a safe claim")
	}
	o.Explanation.En = "This message is legit."
	if !HasSafeClaim(o) {
		t.Fatal("'is legit' must still be refused")
	}
}

func TestMissingLanguageStillRejected(t *testing.T) {
	raw := `{"verdict":"scam","risk":80,"scam_type":"other","red_flags":[],"explanation":{"tl":"a","en":"b"},"actions":[],"injection_detected":false}`
	if _, reason := ParseAIOutput(raw); reason == "" {
		t.Fatal("explanation without ja must be rejected")
	}
}
