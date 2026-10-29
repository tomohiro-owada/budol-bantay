// Package api: HTTP server, routing, CORS, logging and the /v1/check flow.
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"scam-checker/engine"
)

var flagRE = regexp.MustCompile(`^[a-zA-Z]{1,24}$`)

var validBands = map[string]bool{"hi": true, "mid": true, "lo": true, "none": true}

// Server is the budol-api HTTP handler.
type Server struct {
	cfg        Config
	log        *slog.Logger
	limiter    *Limiter // general per-IP bucket
	aiLimiter  *Limiter // stricter per-IP AI bucket
	counters   *Counters
	budget     *NeuronBudget // daily Workers AI neuron ledger (UTC day)
	aiSem      chan struct{}
	httpClient *http.Client
	nowFunc    func() time.Time // tests may override
}

// NewServer builds a Server. httpClient may be nil (a default is built).
func NewServer(cfg Config, logger *slog.Logger, httpClient *http.Client) *Server {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Duration(cfg.AITimeoutSecs) * time.Second}
	}
	perSec := float64(cfg.RatePerMin) / 60.0
	if perSec <= 0 {
		perSec = 6.0 / 60.0
	}
	burst := float64(cfg.RateBurst)
	if burst <= 0 {
		burst = 3
	}
	aiPerSec := float64(cfg.AIRatePerHour) / 3600.0
	if aiPerSec <= 0 {
		aiPerSec = 20.0 / 3600.0
	}
	aiBurst := float64(cfg.AIRatePerHour)
	if aiBurst <= 0 {
		aiBurst = 20
	}
	maxConc := cfg.AIMaxConcurrent
	if maxConc <= 0 {
		maxConc = 3
	}
	return &Server{
		cfg:        cfg,
		log:        logger,
		limiter:    NewLimiter(burst, perSec),
		aiLimiter:  NewLimiter(aiBurst, aiPerSec),
		counters:   NewCounters(cfg.StateDir, cfg.CapTZ),
		budget:     NewNeuronBudget(cfg.StateDir, cfg.AIDailyNeurons, nil),
		aiSem:      make(chan struct{}, maxConc),
		httpClient: httpClient,
		nowFunc:    time.Now,
	}
}

// Counters exposes the counters (tests).
func (s *Server) Counters() *Counters { return s.counters }

func requestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(strings.Repeat("0", 8)))
	}
	return hex.EncodeToString(b[:])
}

// clientIP resolves the client IP per config: RemoteAddr by default;
// X-Forwarded-For/X-Real-IP only when the direct peer equals TRUSTED_PROXY;
// CF-Connecting-IP only when TRUST_CF_HEADERS=1.
func (s *Server) clientIP(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if s.cfg.TrustCFHeaders {
		if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
			return cf
		}
	}
	if s.cfg.TrustedProxy != "" && peer == s.cfg.TrustedProxy {
		// Prefer X-Real-IP (nginx sets it to $remote_addr, not client-controlled).
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
			return xrip
		}
		// Otherwise use the LAST X-Forwarded-For hop (appended by our proxy).
		// Earlier entries are client-supplied and spoofable.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	return peer
}

func (s *Server) originAllowed(origin string) bool {
	for _, a := range s.cfg.AllowedOrigins {
		if origin == a {
			return true
		}
	}
	return false
}

func setCORS(w http.ResponseWriter, origin string, allowed bool) {
	w.Header().Set("Vary", "Origin")
	if origin != "" && allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}

type errBody struct {
	Error errDetail `json:"error"`
}
type errDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// Disclaimer is returned with every successful /v1/check response and shown
// next to every result in the web UI (keep web/app.js T.*.resultDisc in sync).
var Disclaimer = map[string]string{
	"tl": "Hindi perpekto ang pagsusuri na ito. Kung may duda, huwag magdesisyon nang mag-isa — kumonsulta sa pamilya, kaibigan, o mga taong pinagkakatiwalaan mo.",
	"en": "This check is not perfect. If you are unsure, do not decide alone — ask family, friends, or people you trust.",
	"ja": "このチェックは完全ではありません。迷ったときは一人で判断せず、家族・友人・信頼できる人に相談してください。",
}

var errMessages = map[string]string{
	"bad_request":            "invalid request",
	"unsupported_media_type": "content-type must be application/json",
	"payload_too_large":      "request body too large",
	"origin_not_allowed":     "origin not allowed",
	"turnstile_failed":       "turnstile verification failed",
	"rate_limited":           "rate limit exceeded",
	"method_not_allowed":     "method not allowed",
	"not_found":              "not found",
	"unauthorized":           "unauthorized",
	"internal":               "internal error",
}

func errStatus(code string) int {
	switch code {
	case "bad_request":
		return 400
	case "unsupported_media_type":
		return 415
	case "payload_too_large":
		return 413
	case "origin_not_allowed", "turnstile_failed":
		return 403
	case "rate_limited":
		return 429
	case "method_not_allowed":
		return 405
	case "not_found":
		return 404
	case "unauthorized":
		return 401
	default:
		return 500
	}
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, code, rid string) {
	status := errStatus(code)
	msg := errMessages[code]
	if msg == "" {
		msg = code
	}
	origin := r.Header.Get("Origin")
	setCORS(w, origin, origin != "" && s.originAllowed(origin))
	w.Header().Set("Content-Type", "application/json")
	if code == "rate_limited" {
		w.Header().Set("Retry-After", "60")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errBody{Error: errDetail{Code: code, Message: msg, RequestID: rid}})
}

// logReq writes one metadata-only JSON log line. NEVER pass text, hashes,
// tokens or keys.
func (s *Server) logReq(route string, status int, band, aiStatus, model, code string, latency time.Duration, rid string) {
	attrs := []slog.Attr{
		slog.String("request_id", rid),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Int64("latency_ms", latency.Milliseconds()),
	}
	if band != "" {
		attrs = append(attrs, slog.String("band", band))
	}
	if aiStatus != "" {
		attrs = append(attrs, slog.String("ai_status", aiStatus))
	}
	if model != "" {
		attrs = append(attrs, slog.String("model", model))
	}
	if code != "" {
		attrs = append(attrs, slog.String("code", code))
	}
	s.log.LogAttrs(context.Background(), slog.LevelInfo, "request", attrs...)
}

// ServeHTTP routes /v1/*.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/check":
		s.handleCheck(w, r)
	case "/v1/health":
		s.handleHealth(w, r)
	case "/v1/stats":
		s.handleStats(w, r)
	default:
		rid := requestID()
		start := s.nowFunc()
		s.writeError(w, r, "not_found", rid)
		s.logReq("unknown", 404, "", "", "", "not_found", s.nowFunc().Sub(start), rid)
	}
}

// ---------- /v1/health ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	rid := requestID()
	start := s.nowFunc()
	origin := r.Header.Get("Origin")
	if r.Method == "OPTIONS" {
		setCORS(w, origin, origin == "" || s.originAllowed(origin))
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(204)
		s.logReq("health", 204, "", "", "", "", s.nowFunc().Sub(start), rid)
		return
	}
	if r.Method != "GET" {
		s.writeError(w, r, "method_not_allowed", rid)
		s.logReq("health", 405, "", "", "", "method_not_allowed", s.nowFunc().Sub(start), rid)
		return
	}
	setCORS(w, origin, origin != "" && s.originAllowed(origin))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true, "version": Version,
		"engine_version": engine.RulesVersion(),
		"ai_configured":  s.cfg.AIConfigured(),
	})
	s.logReq("health", 200, "", "", "", "", s.nowFunc().Sub(start), rid)
}

// ---------- /v1/stats ----------

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	rid := requestID()
	start := s.nowFunc()
	origin := r.Header.Get("Origin")
	if r.Method == "OPTIONS" {
		setCORS(w, origin, origin == "" || s.originAllowed(origin))
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(204)
		s.logReq("stats", 204, "", "", "", "", s.nowFunc().Sub(start), rid)
		return
	}
	if r.Method != "GET" {
		s.writeError(w, r, "method_not_allowed", rid)
		s.logReq("stats", 405, "", "", "", "method_not_allowed", s.nowFunc().Sub(start), rid)
		return
	}
	auth := r.Header.Get("Authorization")
	token := ""
	if strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimPrefix(auth, "Bearer ")
	}
	ok := s.cfg.StatsToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.StatsToken)) == 1
	if !ok {
		s.writeError(w, r, "unauthorized", rid)
		s.logReq("stats", 401, "", "", "", "unauthorized", s.nowFunc().Sub(start), rid)
		return
	}
	setCORS(w, origin, origin != "" && s.originAllowed(origin))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"counters": s.counters.Snapshot(),
		"neurons": map[string]interface{}{
			"day_utc": time.Now().UTC().Format("2006-01-02"),
			"budget":  s.budget.Budget(),
			"used":    s.budget.Used(),
		},
	})
	s.logReq("stats", 200, "", "", "", "", s.nowFunc().Sub(start), rid)
}

// ---------- /v1/check ----------

type checkRequest struct {
	Text           string       `json:"text"`
	Lang           string       `json:"lang"`
	Sender         string       `json:"sender"`
	AI             *bool        `json:"ai"`
	ClientRules    *clientRules `json:"client_rules"`
	TurnstileToken string       `json:"turnstile_token"`
}
type clientRules struct {
	Score int      `json:"score"`
	Band  string   `json:"band"`
	Flags []string `json:"flags"`
}

type flagOut struct {
	ID       string `json:"id"`
	Weight   int    `json:"weight"`
	Category string `json:"category"`
	Evidence string `json:"evidence"`
	Message  Lang3  `json:"message"`
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	rid := requestID()
	start := s.nowFunc()
	origin := r.Header.Get("Origin")

	// 1. CORS origin allowlist (exact match; no Origin = non-browser, allowed).
	if origin != "" && !s.originAllowed(origin) {
		s.writeError(w, r, "origin_not_allowed", rid)
		s.logReq("check", 403, "", "", "", "origin_not_allowed", s.nowFunc().Sub(start), rid)
		return
	}
	// 2. Method and path.
	if r.Method == "OPTIONS" {
		setCORS(w, origin, true)
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(204)
		s.logReq("check", 204, "", "", "", "", s.nowFunc().Sub(start), rid)
		return
	}
	if r.Method != "POST" {
		s.writeError(w, r, "method_not_allowed", rid)
		s.logReq("check", 405, "", "", "", "method_not_allowed", s.nowFunc().Sub(start), rid)
		return
	}
	// 3. Rate limit (keyed by salted hash).
	ip := s.clientIP(r)
	if ok, wait := s.limiter.Allow(HashKey(ip, s.cfg.IPSalt)); !ok {
		s.counters.Inc("rate_limited")
		origin := r.Header.Get("Origin")
		setCORS(w, origin, origin != "" && s.originAllowed(origin))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", itoa(wait))
		w.WriteHeader(429)
		_ = json.NewEncoder(w).Encode(errBody{Error: errDetail{Code: "rate_limited", Message: errMessages["rate_limited"], RequestID: rid}})
		s.logReq("check", 429, "", "", "", "rate_limited", s.nowFunc().Sub(start), rid)
		return
	}
	// 4. Body validation: JSON content-type, 8 KB cap.
	ct := r.Header.Get("Content-Type")
	if ct == "" || !strings.Contains(strings.ToLower(ct), "application/json") {
		s.writeError(w, r, "unsupported_media_type", rid)
		s.logReq("check", 415, "", "", "", "unsupported_media_type", s.nowFunc().Sub(start), rid)
		return
	}
	if r.ContentLength > MaxBodyBytes {
		s.writeError(w, r, "payload_too_large", rid)
		s.logReq("check", 413, "", "", "", "payload_too_large", s.nowFunc().Sub(start), rid)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if err != nil || int64(len(raw)) > MaxBodyBytes {
		s.writeError(w, r, "payload_too_large", rid)
		s.logReq("check", 413, "", "", "", "payload_too_large", s.nowFunc().Sub(start), rid)
		return
	}
	var body checkRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		s.writeError(w, r, "bad_request", rid)
		s.logReq("check", 400, "", "", "", "bad_request", s.nowFunc().Sub(start), rid)
		return
	}
	lang := body.Lang
	if lang == "" {
		lang = "tl"
	}
	wantAI := true
	if body.AI != nil {
		wantAI = *body.AI
	}
	if !validCheckBody(body, lang, s.cfg.MaxTextChars) {
		s.writeError(w, r, "bad_request", rid)
		s.logReq("check", 400, "", "", "", "bad_request", s.nowFunc().Sub(start), rid)
		return
	}
	// 5. Turnstile (optional; required only when TURNSTILE_SECRET is set).
	if s.cfg.TurnstileSecret != "" {
		remote := ""
		if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			remote = h
		} else {
			remote = r.RemoteAddr
		}
		if !VerifyTurnstile(s.cfg.TurnstileSecret, body.TurnstileToken, remote, s.cfg.TurnstileHosts, s.cfg.TurnstileAllowTK) {
			s.counters.Inc("turnstile_fail")
			s.writeError(w, r, "turnstile_failed", rid)
			s.logReq("check", 403, "", "", "", "turnstile_failed", s.nowFunc().Sub(start), rid)
			return
		}
	}
	// Re-mask server-side (second layer) and run the rules engine on it.
	masked := engine.Mask(body.Text)
	res := engine.Analyze(masked.Masked, body.Sender)
	ruleBand := res.Level
	s.counters.Inc("req")
	s.counters.Inc("level_" + ruleBand)

	flags := make([]flagOut, 0, len(res.Hits))
	flagIDs := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		msg, _ := engine.RuleText(h.ID)
		flags = append(flags, flagOut{
			ID: h.ID, Weight: h.W, Category: h.Cat,
			Evidence: h.Ev, Message: Lang3{Tl: msg.Tl, En: msg.En, Ja: msg.Ja},
		})
		flagIDs = append(flagIDs, h.ID)
	}

	// AI decision.
	aiStatus, aiOut, model := s.runAI(r.Context(), wantAI, ip, masked.Masked, lang, RulesHints{
		Score: res.Score, Level: ruleBand, Flags: flagIDs,
	})

	// Merge: final = rules>=30 ? max(rules, ai.risk) : ai.risk; injection => >=40.
	final := res.Score
	if aiOut != nil {
		if res.Score >= 30 {
			if aiOut.Risk > final {
				final = aiOut.Risk
			}
		} else {
			final = aiOut.Risk
		}
		if aiOut.InjectionDetected && final < 40 {
			final = 40
		}
	}
	band := engine.BandFor(final)

	setCORS(w, origin, origin == "" || s.originAllowed(origin))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"version": Version, "api_version": APIVersion,
		"engine_version": engine.RulesVersion(),
		"score":          final,
		"band":           band,
		"flags":          flags,
		"rules":          map[string]interface{}{"score": res.Score, "band": ruleBand},
		"ai":             aiOut,
		"ai_status":      aiStatus,
		"masked_text":    masked.Masked,
		"masked_counts":  masked.Counts,
		"model":          model,
		"request_id":     rid,
		"disclaimer":     Disclaimer,
	})
	s.logReq("check", 200, band, aiStatus, derefModel(model), "", s.nowFunc().Sub(start), rid)
}

func derefModel(m *string) string {
	if m == nil {
		return ""
	}
	return *m
}

func validCheckBody(b checkRequest, lang string, maxChars int) bool {
	n := len([]rune(b.Text))
	if n < 1 || n > maxChars {
		return false
	}
	if lang != "tl" && lang != "en" && lang != "ja" {
		return false
	}
	if len([]rune(b.Sender)) > 64 {
		return false
	}
	if len(b.TurnstileToken) > 2048 {
		return false
	}
	if b.ClientRules != nil {
		cr := b.ClientRules
		if cr.Score < 0 || cr.Score > 100 {
			return false
		}
		if !validBands[cr.Band] {
			return false
		}
		if len(cr.Flags) > 40 {
			return false
		}
		for _, f := range cr.Flags {
			if !flagRE.MatchString(f) {
				return false
			}
		}
	}
	return true
}

// runAI decides and executes the LLM call. It returns (ai_status, ai, model).
// A rules result is always returned; AI problems surface as ai_status.
func (s *Server) runAI(ctx context.Context, wantAI bool, ip, maskedText, lang string, hints RulesHints) (string, *AIOutput, *string) {
	if !wantAI {
		return "disabled", nil, nil
	}
	if s.cfg.AIMode == "off" {
		return "skipped", nil, nil
	}
	if s.cfg.AIMode == "mock" {
		if !s.cfg.MockAllowed() {
			return "unconfigured", nil, nil
		}
	} else if s.cfg.AIKey == "" {
		return "unconfigured", nil, nil
	}
	// Per-IP AI bucket (applies to mock too: it exercises the same path).
	if ok, _ := s.aiLimiter.Allow(HashKey(ip, s.cfg.IPSalt)); !ok {
		s.counters.Inc("rate_limited")
		return "rate_limited", nil, nil
	}
	// Global daily cap.
	snap := s.counters.Snapshot()
	used := snap["ai_ok"] + snap["ai_err"] + snap["bad_output"]
	if s.cfg.AIDailyCap > 0 && int(used) >= s.cfg.AIDailyCap {
		s.counters.Inc("quota")
		return "quota", nil, nil
	}
	if s.cfg.AIMode == "mock" {
		return s.runMock(maskedText, hints)
	}
	// Concurrency semaphore: wait at most 2 s.
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	case <-time.After(2 * time.Second):
		s.counters.Inc("rate_limited")
		return "rate_limited", nil, nil
	}
	system, user := BuildPrompt(maskedText, lang, hints)
	session := SessionID()
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.AITimeoutSecs)*time.Second)
	defer cancel()
	res, perr := s.callBudgeted(callCtx, s.cfg.AIStyle, system, user, s.cfg.AIModel, session)
	if perr != nil {
		if perr.Budget {
			// Our own neuron budget refused the call: rules-only for now.
			s.counters.Inc("quota")
			return "quota", nil, nil
		}
		if perr.BadOutput {
			// The provider answered with a message-shaped but unusable
			// payload (e.g. incomplete with partial text, or a message
			// without output_text). Try the fallback once, then bad_output.
			return s.finishBadOutput(callCtx, system, user, session)
		}
		if s.cfg.AIFallbackModel != "" && s.cfg.AIFallbackModel != s.cfg.AIModel {
			res2, perr2 := s.callBudgeted(callCtx, s.fallbackStyle(), system, user, s.cfg.AIFallbackModel, session)
			if perr2 == nil {
				return s.finishAI(res2)
			}
			perr = perr2
			if perr.Budget {
				s.counters.Inc("quota")
				return "quota", nil, nil
			}
		}
		s.counters.Inc("ai_err")
		if perr.Timeout || callCtx.Err() == context.DeadlineExceeded {
			return "timeout", nil, nil
		}
		return "error", nil, nil
	}
	out, vErr := parseAIOutput(res.RawText)
	if vErr != "" {
		return s.finishBadOutput(callCtx, system, user, session)
	}
	return s.finishAIWith(res, out)
}

// callBudgeted runs one provider call under the daily neuron budget: it
// reserves a pessimistic cost first (refusing with ProviderError.Budget when
// the day's budget is gone), then settles the real cost from the provider's
// token usage. A rejected request (HTTP status) is charged its input only; a
// transport error, timeout or unusable output is charged the full reservation.
// A "free allocation used up" reply exhausts the day's budget outright.
func (s *Server) callBudgeted(ctx context.Context, style, system, user, model, session string) (LLMResult, *ProviderError) {
	est := EstimateNeurons(model, (len(system)+len(user)+2)/3, s.cfg.AIMaxTokens)
	if !s.budget.Reserve(est) {
		return LLMResult{}, &ProviderError{Budget: true, Err: fmt.Errorf("daily neuron budget reached")}
	}
	res, perr := CallLLMWithStyle(ctx, s.httpClient, s.cfg, style, system, user, model, session)
	switch {
	case perr == nil:
		s.budget.Settle(est, EstimateNeurons(model, res.InTokens, res.OutTokens))
	case perr.QuotaExhausted:
		s.budget.Settle(est, 0)
		s.budget.Exhaust()
	case perr.Status != 0:
		s.budget.Settle(est, EstimateNeurons(model, (len(system)+len(user)+2)/3, 0))
	default:
		s.budget.Settle(est, est)
	}
	return res, perr
}

// finishBadOutput tries the fallback model once after a bad provider
// payload or bad model JSON, then records bad_output (still HTTP 200).
func (s *Server) finishBadOutput(callCtx context.Context, system, user, session string) (string, *AIOutput, *string) {
	if s.cfg.AIFallbackModel != "" && s.cfg.AIFallbackModel != s.cfg.AIModel {
		if res2, perr2 := s.callBudgeted(callCtx, s.fallbackStyle(), system, user, s.cfg.AIFallbackModel, session); perr2 == nil {
			if out2, vErr2 := parseAIOutput(res2.RawText); vErr2 == "" {
				return s.finishAIWith(res2, out2)
			}
		}
	}
	s.counters.Inc("bad_output")
	return "bad_output", nil, nil
}

// fallbackStyle returns the api_style for the fallback model call.
func (s *Server) fallbackStyle() string {
	if s.cfg.AIFallbackStyle != "" {
		return s.cfg.AIFallbackStyle
	}
	return s.cfg.AIStyle
}

func (s *Server) runMock(maskedText string, hints RulesHints) (string, *AIOutput, *string) {
	raw, err := RunMockAI(maskedText, hints)
	if err != nil {
		s.counters.Inc("ai_err")
		return "error", nil, nil
	}
	out, vErr := parseAIOutput(raw)
	if vErr != "" {
		s.counters.Inc("bad_output")
		return "bad_output", nil, nil
	}
	m := MockModelID
	s.counters.Inc("ai_ok")
	if out.InjectionDetected {
		s.counters.Inc("injection")
	}
	return "ok", &out, &m
}

func (s *Server) finishAI(res LLMResult) (string, *AIOutput, *string) {
	out, vErr := parseAIOutput(res.RawText)
	if vErr != "" {
		s.counters.Inc("bad_output")
		return "bad_output", nil, nil
	}
	return s.finishAIWith(res, out)
}

func (s *Server) finishAIWith(res LLMResult, out AIOutput) (string, *AIOutput, *string) {
	s.counters.Inc("ai_ok")
	s.counters.AddTokens(res.InTokens, res.OutTokens)
	s.counters.AddAllowance(res.Allowance)
	if out.InjectionDetected {
		s.counters.Inc("injection")
	}
	m := res.Model
	return "ok", &out, &m
}

func parseAIOutput(raw string) (AIOutput, string) {
	return ParseAIOutput(raw)
}
