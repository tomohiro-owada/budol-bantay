// Package api implements the Budol Bantay HTTP API (net/http, stdlib only).
//
// Endpoints (versioned under /v1; see docs/openapi.yaml):
//
//	POST /v1/check   analyze an already-masked message (rules + optional LLM)
//	GET  /v1/health  liveness probe (never reveals keys)
//	GET  /v1/stats   today's aggregate counters (Bearer BUDOL_STATS_TOKEN)
package api

import (
	"os"
	"strconv"
	"strings"
)

// Version is the API version string reported in responses and User-Agent.
const Version = "1"

// APIVersion is the URL version prefix.
const APIVersion = "v1"

// MaxBodyBytes caps POST /v1/check request bodies (8 KB).
const MaxBodyBytes = 8 * 1024

// Config holds every runtime setting. All secrets arrive via the environment
// (or the sourced env files on the server); config/project.json only carries
// non-secret defaults that cmd/build renders into deploy/budol-api.env.example.
type Config struct {
	Env                   string // BUDOL_ENV: dev|prod
	Listen                string // BUDOL_LISTEN, default 127.0.0.1:8790
	AllowedOrigins        []string
	TrustedProxy          string  // TRUSTED_PROXY, e.g. 127.0.0.1
	TrustCFHeaders        bool    // TRUST_CF_HEADERS=1
	IPSalt                string  // BUDOL_IP_SALT
	MaxTextChars          int     // MAX_TEXT_CHARS, default 1500
	StatsToken            string  // BUDOL_STATS_TOKEN
	StateDir              string  // BUDOL_STATE_DIR, default ~/budol/state
	RatePerMin            int     // requests/min per IP, default 6
	RateBurst             int     // burst, default 3
	AIRatePerHour         int     // AI calls/hour per IP, default 20
	AIMaxConcurrent       int     // AI semaphore size, default 3
	AIDailyCap            int     // global daily AI cap, default 2000
	CapTZ                 string  // CAP_TZ day boundary, default Asia/Manila
	AIDailyNeurons        float64 // AI_DAILY_NEURON_BUDGET: Workers AI neurons/day (UTC day), 0 = off, default 8000
	AITimeoutSecs         int     // default 25
	TurnstileSecret       string  // TURNSTILE_SECRET (optional)
	TurnstileHosts        []string
	TurnstileAllowTK      bool   // TURNSTILE_ALLOW_TEST_KEYS=1
	AIMode                string // AI_MODE: mock|<anything else means provider>
	AIStyle               string // AI_API_STYLE: openai_responses|openai_chat
	AIBaseURL             string // AI_BASE_URL
	AIModel               string // AI_MODEL
	AIFallbackModel       string // AI_FALLBACK_MODEL (optional)
	AIFallbackStyle       string // AI_FALLBACK_STYLE (default: AIStyle)
	AIKeyEnv              string // AI_KEY_ENV: NAME of the env var holding the key
	AIMaxTokens           int    // AI_MAX_OUTPUT_TOKENS, default 3000
	AITemperature         float64
	AIReasoningEffort     string // AI_REASONING_EFFORT (responses style), default "minimal"
	AIChatReasoningEffort string // AI_CHAT_REASONING_EFFORT (chat style; e.g. "low" for gpt-oss), default ""
	AIJSONMode            bool   // AI_JSON_MODE=1: send response_format json_object (chat style)
	AIKey                 string // resolved from AIKeyEnv (never logged)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// envNonNegFloat reads a float >= 0; unlike envInt, an explicit 0 is kept
// (it means "disabled"). Unset or invalid values fall back to def.
func envNonNegFloat(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return def
	}
	return f
}

func parseList(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// LoadConfig reads the runtime configuration from the environment.
func LoadConfig() Config {
	aiMode := envOr("AI_MODE", "")
	c := Config{
		Env:                   envOr("BUDOL_ENV", "prod"),
		Listen:                envOr("BUDOL_LISTEN", "127.0.0.1:8790"),
		AllowedOrigins:        parseList(os.Getenv("ALLOWED_ORIGINS")),
		TrustedProxy:          strings.TrimSpace(os.Getenv("TRUSTED_PROXY")),
		TrustCFHeaders:        os.Getenv("TRUST_CF_HEADERS") == "1",
		IPSalt:                os.Getenv("BUDOL_IP_SALT"),
		MaxTextChars:          envInt("MAX_TEXT_CHARS", 1500),
		StatsToken:            os.Getenv("BUDOL_STATS_TOKEN"),
		StateDir:              envOr("BUDOL_STATE_DIR", os.Getenv("HOME")+"/budol/state"),
		RatePerMin:            envInt("BUDOL_RATE_PER_MIN", 6),
		RateBurst:             envInt("BUDOL_RATE_BURST", 3),
		AIRatePerHour:         envInt("AI_RATE_PER_HOUR", 20),
		AIMaxConcurrent:       envInt("AI_MAX_CONCURRENT", 3),
		AIDailyCap:            envInt("AI_DAILY_CAP", 2000),
		CapTZ:                 envOr("CAP_TZ", "Asia/Manila"),
		AIDailyNeurons:        envNonNegFloat("AI_DAILY_NEURON_BUDGET", 8000),
		AITimeoutSecs:         envInt("AI_TIMEOUT_S", 25),
		TurnstileSecret:       os.Getenv("TURNSTILE_SECRET"),
		TurnstileHosts:        parseList(os.Getenv("TURNSTILE_HOSTNAMES")),
		TurnstileAllowTK:      os.Getenv("TURNSTILE_ALLOW_TEST_KEYS") == "1",
		AIMode:                aiMode,
		AIStyle:               envOr("AI_API_STYLE", "openai_chat"),
		AIBaseURL:             envOr("AI_BASE_URL", "https://api.cloudflare.com/client/v4/accounts/${CF_ACCOUNT_ID}/ai/v1"),
		AIModel:               envOr("AI_MODEL", "@cf/openai/gpt-oss-120b"),
		AIFallbackModel:       strings.TrimSpace(os.Getenv("AI_FALLBACK_MODEL")),
		AIFallbackStyle:       strings.TrimSpace(os.Getenv("AI_FALLBACK_STYLE")),
		AIKeyEnv:              envOr("AI_KEY_ENV", "AI_API_KEY"),
		AIMaxTokens:           envInt("AI_MAX_OUTPUT_TOKENS", 2500),
		AITemperature:         0.2,
		AIReasoningEffort:     envOr("AI_REASONING_EFFORT", "minimal"),
		AIChatReasoningEffort: envOr("AI_CHAT_REASONING_EFFORT", "low"),
		AIJSONMode:            os.Getenv("AI_JSON_MODE") == "1",
	}
	if strings.EqualFold(c.AIChatReasoningEffort, "none") {
		c.AIChatReasoningEffort = "" // for non-reasoning models
	}
	c.AIBaseURL = ExpandBaseURL(c.AIBaseURL)
	if t := strings.TrimSpace(os.Getenv("AI_TEMPERATURE")); t != "" {
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			c.AITemperature = f
		}
	}
	if c.AIKeyEnv != "" {
		c.AIKey = os.Getenv(c.AIKeyEnv)
	}
	if c.MaxTextChars > 10000 {
		c.MaxTextChars = 10000
	}
	return c
}

// MockAllowed reports whether the mock provider may be used.
func (c Config) MockAllowed() bool { return c.AIMode == "mock" && c.Env == "dev" }

// AIConfigured reports whether an LLM call can be attempted (key present or
// dev mock). It never reveals the key itself.
func (c Config) AIConfigured() bool {
	if c.AIMode == "mock" {
		return c.Env == "dev"
	}
	if c.AIMode == "off" {
		return false
	}
	// A Workers AI URL without its account id can never work: treat as
	// not configured (rules-only) instead of failing every request.
	if strings.Contains(c.AIBaseURL, "${CF_ACCOUNT_ID}") || strings.Contains(c.AIBaseURL, "/accounts//") {
		return false
	}
	return c.AIKey != ""
}

// ExpandBaseURL substitutes ${CF_ACCOUNT_ID} (a non-secret account id needed
// by Cloudflare Workers AI URLs) from the environment. Nothing else is
// expanded, so a base URL can never pull a secret out of the environment.
func ExpandBaseURL(u string) string {
	if !strings.Contains(u, "${CF_ACCOUNT_ID}") {
		return u
	}
	return strings.ReplaceAll(u, "${CF_ACCOUNT_ID}", strings.TrimSpace(os.Getenv("CF_ACCOUNT_ID")))
}
