package engine

import (
	"strings"
	"testing"
)

// Mask-aware rules: [PHONE] must behave like a PH mobile number in
// personalAcct, sendMoney and senderMobile; [CODE] must behave like an OTP
// in the credential patterns. See docs/BUILD-SPEC.md DECISION UPDATE 2.
func TestMaskAware(t *testing.T) {
	has := func(r Result, id string) bool {
		for _, h := range r.Hits {
			if h.ID == id {
				return true
			}
		}
		return false
	}

	// personalAcct fires on masked phone.
	r := Analyze("GCash: pakisend sa GCash [PHONE] ngayon", "")
	if !has(r, "personalAcct") {
		t.Errorf("personalAcct did not fire on [PHONE]: %+v", r.Hits)
	}
	// raw still fires.
	r = Analyze("GCash: pakisend sa GCash 09171234567 ngayon", "")
	if !has(r, "personalAcct") {
		t.Errorf("personalAcct regressed on raw number: %+v", r.Hits)
	}

	// sendMoney fires on masked phone.
	r = Analyze("Pa-send muna sa GCash [PHONE] emergency", "")
	if !has(r, "sendMoney") {
		t.Errorf("sendMoney did not fire on [PHONE]: %+v", r.Hits)
	}
	r = Analyze("Pa-send muna sa GCash 09171234567 emergency", "")
	if !has(r, "sendMoney") {
		t.Errorf("sendMoney regressed on raw number: %+v", r.Hits)
	}

	// senderMobile fires when sender is masked.
	r = Analyze("GCash: i-verify ang account mo dito", "[PHONE]")
	if !has(r, "senderMobile") {
		t.Errorf("senderMobile did not fire on [PHONE] sender: %+v", r.Hits)
	}

	// credential fires on masked OTP.
	r = Analyze("paki-send ang [CODE] ngayon din", "")
	if !has(r, "credential") {
		t.Errorf("credential did not fire on [CODE]: %+v", r.Hits)
	}
	// reversed order (credB) with masked OTP.
	r = Analyze("[CODE] ang OTP mo, paki-send agad", "")
	if !has(r, "credential") {
		t.Errorf("credential (credB) did not fire on [CODE]: %+v", r.Hits)
	}

	// End-to-end: mask a raw scam text, then analyze the masked text.
	// The money-trail rules must survive masking.
	raw := "CONGRATULATIONS! Nanalo ka ng premyo. Para ma-claim, magpadala ng processing fee sa GCash 09171234567."
	masked := Mask(raw).Masked
	if strings.Contains(masked, "09171234567") {
		t.Fatalf("Mask did not redact phone: %q", masked)
	}
	if !strings.Contains(masked, "[PHONE]") {
		t.Fatalf("Mask missing [PHONE]: %q", masked)
	}
	r = Analyze(masked, "")
	if !has(r, "personalAcct") || !has(r, "sendMoney") {
		t.Errorf("masked scam lost money-trail rules: %q -> %+v", masked, r.Hits)
	}
}
