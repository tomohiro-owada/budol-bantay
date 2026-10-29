// Test that docs/openapi.yaml matches the implementation exactly: every
// documented path/method/status is served, and every documented error code
// is producible (internal is covered via the errStatus unit mapping).
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	yamlPathRE   = regexp.MustCompile(`^  (/v1/\S+):\s*$`)
	yamlMethodRE = regexp.MustCompile(`^    (get|post|options|put|delete|patch):\s*$`)
	yamlStatusRE = regexp.MustCompile(`^        '([0-9]{3})':\s*$`)
)

type routeTuple struct {
	path, method, status string
}

func parseOpenAPI(t *testing.T) ([]routeTuple, []string) {
	t.Helper()
	raw, err := os.ReadFile("../docs/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	var tuples []routeTuple
	var path, method string
	for _, line := range strings.Split(string(raw), "\n") {
		if m := yamlPathRE.FindStringSubmatch(line); m != nil {
			path, method = m[1], ""
			continue
		}
		if m := yamlMethodRE.FindStringSubmatch(line); m != nil {
			method = m[1]
			continue
		}
		if m := yamlStatusRE.FindStringSubmatch(line); m != nil && path != "" && method != "" {
			tuples = append(tuples, routeTuple{path, method, m[1]})
		}
	}
	// ErrorCode enum values.
	var codes []string
	inEnum := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "ErrorCode:" {
			inEnum = true
			continue
		}
		if inEnum {
			if m := regexp.MustCompile(`^        - ([a-z_]+)\s*$`).FindStringSubmatch(line); m != nil {
				codes = append(codes, m[1])
				continue
			}
			if strings.TrimSpace(line) == "enum:" {
				continue
			}
			if strings.HasPrefix(line, "    ") && strings.HasSuffix(strings.TrimSpace(line), ":") && !strings.HasPrefix(line, "      ") {
				break
			}
		}
	}
	sort.Strings(codes)
	return tuples, codes
}

func tupleKey(path, method, status string) string { return method + " " + path + " " + status }

func TestOpenAPICoverage(t *testing.T) {
	tuples, codes := parseOpenAPI(t)

	want := []routeTuple{
		{"/v1/check", "post", "200"},
		{"/v1/check", "post", "400"},
		{"/v1/check", "post", "403"},
		{"/v1/check", "post", "405"},
		{"/v1/check", "post", "413"},
		{"/v1/check", "post", "415"},
		{"/v1/check", "post", "429"},
		{"/v1/check", "options", "204"},
		{"/v1/check", "options", "403"},
		{"/v1/health", "get", "200"},
		{"/v1/health", "get", "405"},
		{"/v1/health", "options", "204"},
		{"/v1/stats", "get", "200"},
		{"/v1/stats", "get", "401"},
		{"/v1/stats", "get", "405"},
		{"/v1/stats", "options", "204"},
	}
	gotSet := map[string]bool{}
	for _, tu := range tuples {
		gotSet[tupleKey(tu.path, tu.method, tu.status)] = true
	}
	for _, w := range want {
		if !gotSet[tupleKey(w.path, w.method, w.status)] {
			t.Errorf("openapi.yaml missing %s %s %s", w.method, w.path, w.status)
		}
	}
	if len(tuples) != len(want) {
		for _, tu := range tuples {
			found := false
			for _, w := range want {
				if tu == w {
					found = true
				}
			}
			if !found {
				t.Errorf("openapi.yaml documents unexpected %s %s %s (add a serving case or remove it)", tu.method, tu.path, tu.status)
			}
		}
	}

	// Every documented error code must map to its HTTP status in code.
	wantCodes := []string{
		"bad_request", "unsupported_media_type", "payload_too_large",
		"origin_not_allowed", "turnstile_failed", "rate_limited",
		"method_not_allowed", "not_found", "unauthorized", "internal",
	}
	sortedWant := append([]string{}, wantCodes...)
	sort.Strings(sortedWant)
	if strings.Join(codes, ",") != strings.Join(sortedWant, ",") {
		t.Fatalf("ErrorCode enum = %v", codes)
	}
	statusFor := map[string]int{
		"bad_request": 400, "unsupported_media_type": 415,
		"payload_too_large": 413, "origin_not_allowed": 403,
		"turnstile_failed": 403, "rate_limited": 429,
		"method_not_allowed": 405, "not_found": 404,
		"unauthorized": 401, "internal": 500,
	}
	for _, c := range codes {
		if errStatus(c) != statusFor[c] {
			t.Errorf("errStatus(%q) = %d, want %d", c, errStatus(c), statusFor[c])
		}
	}

	// Serve every tuple.
	base, _ := newTestServer(t, nil)
	serve := func(srv *Server, method, path, origin, body, ctype, auth string) *httptest.ResponseRecorder {
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, path, strings.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.RemoteAddr = "192.0.2.9:1234"
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}
	checkBody := `{"text":"Magandang araw! Paalala lang na magdala ng payong bukas."}`
	assertErr := func(rec *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		if rec.Code != status {
			t.Fatalf("status = %d, want %d (%s)", rec.Code, status, rec.Body.String())
		}
		var v map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatalf("error body not JSON: %v", err)
		}
		e := v["error"].(map[string]interface{})
		if e["code"] != code || e["message"] == "" || e["request_id"] == "" {
			t.Fatalf("bad envelope: %s", rec.Body.String())
		}
	}

	// check POST 200.
	rec := serve(base, "POST", "/v1/check", "", checkBody, jsonCT, "")
	if rec.Code != 200 {
		t.Fatalf("check 200 = %d %s", rec.Code, rec.Body.String())
	}
	var ok200 map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &ok200); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"version", "api_version", "engine_version", "score", "band", "flags", "rules", "ai", "ai_status", "masked_text", "masked_counts", "model", "request_id"} {
		if _, present := ok200[k]; !present {
			t.Errorf("200 missing key %q", k)
		}
	}
	// check POST 400 / 415 / 413 / 405.
	assertErr(serve(base, "POST", "/v1/check", "", `{}`, jsonCT, ""), 400, "bad_request")
	assertErr(serve(base, "POST", "/v1/check", "", checkBody, "text/plain", ""), 415, "unsupported_media_type")
	assertErr(serve(base, "POST", "/v1/check", "", `{"text":"`+strings.Repeat("y", 9000)+`"}`, jsonCT, ""), 413, "payload_too_large")
	assertErr(serve(base, "GET", "/v1/check", "", "", "", ""), 405, "method_not_allowed")
	// check POST 403 origin.
	assertErr(serve(base, "POST", "/v1/check", "https://evil.example", checkBody, jsonCT, ""), 403, "origin_not_allowed")
	// check POST 403 turnstile_failed.
	ts := fakeSiteverify(t, false, "", "", false)
	defer ts.Close()
	old := TurnstileVerifyURL
	TurnstileVerifyURL = ts.URL
	defer func() { TurnstileVerifyURL = old }()
	tsSrv, _ := newTestServer(t, func(c *Config) {
		c.TurnstileSecret = "s"
		c.TurnstileHosts = []string{"h"}
	})
	assertErr(serve(tsSrv, "POST", "/v1/check", "", `{"text":"hi there friend","turnstile_token":"x"}`, jsonCT, ""), 403, "turnstile_failed")
	// check POST 429.
	rlSrv, _ := newTestServer(t, func(c *Config) {
		c.RatePerMin = 60
		c.RateBurst = 1
	})
	serve(rlSrv, "POST", "/v1/check", "", checkBody, jsonCT, "")
	rec = serve(rlSrv, "POST", "/v1/check", "", checkBody, jsonCT, "")
	assertErr(rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 missing Retry-After")
	}
	// check OPTIONS 204 / 403.
	if rec := serve(base, "OPTIONS", "/v1/check", "http://localhost:8788", "", "", ""); rec.Code != 204 {
		t.Fatalf("check OPTIONS 204 = %d", rec.Code)
	}
	assertErr(serve(base, "OPTIONS", "/v1/check", "https://evil.example", "", "", ""), 403, "origin_not_allowed")
	// health GET 200 / POST 405 / OPTIONS 204.
	rec = serve(base, "GET", "/v1/health", "", "", "", "")
	if rec.Code != 200 {
		t.Fatalf("health 200 = %d", rec.Code)
	}
	assertErr(serve(base, "POST", "/v1/health", "", "", "", ""), 405, "method_not_allowed")
	if rec := serve(base, "OPTIONS", "/v1/health", "", "", "", ""); rec.Code != 204 {
		t.Fatalf("health OPTIONS = %d", rec.Code)
	}
	// stats GET 200 / 401 / 405 + OPTIONS 204.
	assertErr(serve(base, "GET", "/v1/stats", "", "", "", ""), 401, "unauthorized")
	rec = serve(base, "GET", "/v1/stats", "", "", "", "Bearer test-stats-token")
	if rec.Code != 200 {
		t.Fatalf("stats 200 = %d %s", rec.Code, rec.Body.String())
	}
	assertErr(serve(base, "POST", "/v1/stats", "", "", "", ""), 405, "method_not_allowed")
	if rec := serve(base, "OPTIONS", "/v1/stats", "", "", "", ""); rec.Code != 204 {
		t.Fatalf("stats OPTIONS = %d", rec.Code)
	}
	// not_found on unknown paths.
	assertErr(serve(base, "GET", "/v1/does-not-exist", "", "", "", ""), 404, "not_found")
}
