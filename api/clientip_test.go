package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPNotSpoofableViaXFF(t *testing.T) {
	s := &Server{cfg: Config{TrustedProxy: "127.0.0.1"}}
	r := httptest.NewRequest("POST", "/v1/check", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9")
	if got := s.clientIP(r); got != "203.0.113.9" {
		t.Fatalf("want last XFF hop, got %q", got)
	}
	r.Header.Set("X-Real-IP", "198.51.100.7")
	if got := s.clientIP(r); got != "198.51.100.7" {
		t.Fatalf("want X-Real-IP, got %q", got)
	}
	// Untrusted peer: headers ignored.
	r.RemoteAddr = "192.0.2.1:1234"
	if got := s.clientIP(r); got != "192.0.2.1" {
		t.Fatalf("want peer, got %q", got)
	}
}
