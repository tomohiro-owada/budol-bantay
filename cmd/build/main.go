// Command build generates dist/<slug>/ from config/project.json.
//
// Decision-update build (no TinyGo/WASM): copies web/* (engine.js, mask.js,
// future index.html/style.css) into dist/<slug>/ with placeholder
// substitution, and embeds engine/rules.json as dist/<slug>/rules.json.
// Worker wrangler.jsonc is rendered when its template exists; otherwise skipped.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Project struct {
	Name       string  `json:"name"`
	Slug       string  `json:"slug"`
	SiteOrigin string  `json:"siteOrigin"`
	Prod       Env     `json:"prod"`
	Dev        Env     `json:"dev"`
	API        APIConf `json:"api"`
	AI         AIConf  `json:"ai"`
	// GithubURL is the public repo (e.g. https://github.com/<owner>/<repo>); terms.html
	// sends bug reports and feedback to its Issues page. Empty = no link, text only.
	GithubURL string `json:"githubUrl"`
}

type Env struct {
	TurnstileSiteKey string `json:"turnstileSiteKey"`
	SiteOrigin       string `json:"siteOrigin,omitempty"`
	APIURL           string `json:"apiUrl,omitempty"`
}

// APIConf holds non-secret API defaults from config/project.json.
type APIConf struct {
	Listen             string `json:"listen"`
	AllowedOriginsProd string `json:"allowedOriginsProd"`
	AllowedOriginsDev  string `json:"allowedOriginsDev"`
	TrustedProxy       string `json:"trustedProxy"`
	MaxTextChars       int    `json:"maxTextChars"`
	RatePerMin         int    `json:"ratePerMin"`
	RateBurst          int    `json:"rateBurst"`
	AIRatePerHour      int    `json:"aiRatePerHour"`
	AIMaxConcurrent    int    `json:"aiMaxConcurrent"`
	AIDailyCap         int    `json:"aiDailyCap"`
	AIDailyNeurons     int    `json:"aiDailyNeurons"`
	CapTz              string `json:"capTz"`
	AITimeoutS         int    `json:"aiTimeoutS"`
}

// AIConf holds the LLM provider config (no secrets; the key itself lives
// only in ~/.config/budol/ai.env on the server or the dev shell env).
// DisplayName and PrivacyNote feed the frontend privacy wording so that
// changing provider updates the wording without touching web/.
type AIConf struct {
	Provider         string            `json:"provider"`
	APIStyle         string            `json:"api_style"`
	BaseURL          string            `json:"base_url"`
	Model            string            `json:"model"`
	FallbackModel    string            `json:"fallback_model"`
	FallbackAPIStyle string            `json:"fallback_api_style"`
	KeyEnv           string            `json:"key_env"`
	TimeoutS         int               `json:"timeout_s"`
	MaxOutputTokens  int               `json:"max_output_tokens"`
	Temperature      float64           `json:"temperature"`
	ReasoningEffort  string            `json:"reasoning_effort"`
	ChatReasoning    string            `json:"chat_reasoning_effort"`
	JSONMode         bool              `json:"json_mode"`
	DisplayName      string            `json:"display_name"`
	PrivacyNote      map[string]string `json:"privacy_note"`
}

func main() {
	envName := flag.String("env", "dev", "env: dev|prod")
	flag.Parse()
	if *envName != "dev" && *envName != "prod" {
		fmt.Fprintf(os.Stderr, "unknown -env %q (want dev|prod)\n", *envName)
		os.Exit(1)
	}
	if err := run(*envName); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n", err)
		os.Exit(1)
	}
}

func run(envName string) error {
	cfgData, err := os.ReadFile(filepath.Join("config", "project.json"))
	if err != nil {
		return fmt.Errorf("read config/project.json: %w", err)
	}
	var p Project
	if err := json.Unmarshal(cfgData, &p); err != nil {
		return fmt.Errorf("parse config/project.json: %w", err)
	}
	if p.Slug == "" || p.Name == "" || p.SiteOrigin == "" {
		return fmt.Errorf("config/project.json missing name/slug/siteOrigin")
	}
	var e Env
	siteOrigin := p.SiteOrigin
	apiURL := p.Dev.APIURL
	allowedOrigins := p.API.AllowedOriginsDev
	if envName == "prod" {
		e = p.Prod
		apiURL = p.Prod.APIURL
		allowedOrigins = p.API.AllowedOriginsProd
	} else {
		e = p.Dev
		if e.SiteOrigin != "" {
			siteOrigin = e.SiteOrigin
		}
		if e.APIURL != "" {
			apiURL = e.APIURL
		}
	}
	if apiURL == "" {
		apiURL = "http://localhost:18790"
	}
	if allowedOrigins == "" {
		allowedOrigins = siteOrigin
	}
	apiOrigin := originOf(apiURL)
	siteURL := strings.TrimRight(p.SiteOrigin, "/") + "/" + p.Slug + "/"

	// Turnstile is "configured" only with a real sitekey. The
	// CHANGE-ME placeholder (prod default) means: skip the widget and
	// leave 'unsafe-inline' out of style-src.
	turnstileOn := e.TurnstileSiteKey != "" && e.TurnstileSiteKey != "CHANGE-ME"
	csp := buildCSP(apiOrigin, turnstileOn)
	cspComment := "<!-- No inline <script> is used (ES modules only). -->"
	if turnstileOn {
		cspComment = "<!-- style-src 'unsafe-inline' is present only because the " +
			"Cloudflare Turnstile widget injects inline styles at runtime. " +
			"No inline <script> is used (ES modules only). -->"
	}

	fbTL, fbEN, fbJA := "GitHub page ng proyekto", "the project's GitHub page", "プロジェクトのGitHubページ"
	if gh := strings.TrimRight(strings.TrimSpace(p.GithubURL), "/"); gh != "" {
		link := `<a href="` + gh + `/issues" rel="noopener noreferrer">GitHub Issues</a>`
		fbTL, fbEN, fbJA = link, link, link
	} else {
		fmt.Fprintln(os.Stderr, "build: note: config/project.json \"githubUrl\" is empty; terms.html names the project's GitHub page without a link (TODO)")
	}

	outDir := filepath.Join("dist", p.Slug)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// 1. Web files: copy top-level web/*.js/*.html/*.css (skip web/test/).
	webDir := "web"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		return fmt.Errorf("web/ not found")
	} else if err != nil {
		return err
	} else {
		reps := map[string]string{
			"{{NAME}}":                 p.Name,
			"{{SLUG}}":                 p.Slug,
			"{{TURNSTILE_SITEKEY}}":    e.TurnstileSiteKey,
			"{{API_URL}}":              apiURL,
			"{{API_ORIGIN}}":           apiOrigin,
			"{{SITE_URL}}":             siteURL,
			"{{SITE_ORIGIN}}":          p.SiteOrigin,
			"{{DEV_SITE_ORIGIN}}":      siteOrigin,
			"{{CSP}}":                  csp,
			"{{CSP_COMMENT}}":          cspComment,
			"{{FEEDBACK_TL}}":          fbTL,
			"{{FEEDBACK_EN}}":          fbEN,
			"{{FEEDBACK_JA}}":          fbJA,
			"{{AI_DISPLAY_NAME}}":      p.AI.DisplayName,
			"{{AI_DISPLAY_NAME_JSON}}": jsonStr(p.AI.DisplayName),
			"{{AI_PRIVACY_TL}}":        p.AI.PrivacyNote["tl"],
			"{{AI_PRIVACY_EN}}":        p.AI.PrivacyNote["en"],
			"{{AI_PRIVACY_JA}}":        p.AI.PrivacyNote["ja"],
			"{{AI_PRIVACY_TL_JSON}}":   jsonStr(p.AI.PrivacyNote["tl"]),
			"{{AI_PRIVACY_EN_JSON}}":   jsonStr(p.AI.PrivacyNote["en"]),
			"{{AI_PRIVACY_JA_JSON}}":   jsonStr(p.AI.PrivacyNote["ja"]),
		}
		entries, err := os.ReadDir(webDir)
		if err != nil {
			return err
		}
		for _, de := range entries {
			if de.IsDir() {
				continue
			}
			name := de.Name()
			if !(strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".css")) {
				continue
			}
			src := filepath.Join(webDir, name)
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			out := string(data)
			for k, v := range reps {
				out = strings.ReplaceAll(out, k, v)
			}
			if err := os.WriteFile(filepath.Join(outDir, name), []byte(out), 0o644); err != nil {
				return err
			}
			fmt.Printf("copied web/%s\n", name)
		}
	}

	// 2. Embed rules.json (single source of truth for the JS engine).
	rulesData, err := os.ReadFile(filepath.Join("engine", "rules.json"))
	if err != nil {
		return fmt.Errorf("read engine/rules.json: %w", err)
	}
	// Validate it parses before embedding.
	var jsCheck map[string]interface{}
	if err := json.Unmarshal(rulesData, &jsCheck); err != nil {
		return fmt.Errorf("parse engine/rules.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rules.json"), rulesData, 0o644); err != nil {
		return err
	}
	fmt.Printf("embedded rules.json (%d bytes)\n", len(rulesData))

	// 3. (worker retired: no wrangler render; the API lives on the server.)

	fmt.Printf("done: dist/%s/ (env=%s site=%s api=%s turnstile=%v)\n", p.Slug, envName, siteURL, apiURL, turnstileOn)

	// 4. Render deploy/budol-api.env.example + deploy/budol-api.json
	// (non-secret settings only; secrets are set on the server, never here).
	if err := renderAPIEnv(p, envName, apiURL, allowedOrigins); err != nil {
		return err
	}
	return nil
}

func renderAPIEnv(p Project, envName, apiURL, allowedOrigins string) error {
	if err := os.MkdirAll("deploy", 0o755); err != nil {
		return err
	}
	hostnames := ""
	if u, err := url.Parse(p.SiteOrigin); err == nil && u.Hostname() != "" {
		hostnames = u.Hostname()
	}
	header := "# GENERATED by `go run ./cmd/build -env " + envName + "`. Do not edit; edit config/project.json.\n" +
		"# Non-secret settings only. Secrets live on the server and are NEVER in this file:\n" +
		"#   ~/.config/budol/ai.env (mode 600): " + p.AI.KeyEnv + "=...  (the actual key)\n" +
		"#   ~/budol/budol-api.env (live copy of this file): TURNSTILE_SECRET, BUDOL_STATS_TOKEN, BUDOL_IP_SALT\n"
	env := header +
		"BUDOL_ENV=" + envName + "\n" +
		"BUDOL_LISTEN=" + p.API.Listen + "\n" +
		"ALLOWED_ORIGINS=" + allowedOrigins + "\n" +
		"# Honour X-Forwarded-For/X-Real-IP only from this peer (nginx on the same host).\n" +
		"TRUSTED_PROXY=" + p.API.TrustedProxy + "\n" +
		"# Set to 1 only when behind a Cloudflare tunnel (honours CF-Connecting-IP).\n" +
		"TRUST_CF_HEADERS=\n" +
		"MAX_TEXT_CHARS=" + itoa(p.API.MaxTextChars) + "\n" +
		"BUDOL_RATE_PER_MIN=" + itoa(p.API.RatePerMin) + "\n" +
		"BUDOL_RATE_BURST=" + itoa(p.API.RateBurst) + "\n" +
		"AI_RATE_PER_HOUR=" + itoa(p.API.AIRatePerHour) + "\n" +
		"AI_MAX_CONCURRENT=" + itoa(p.API.AIMaxConcurrent) + "\n" +
		"AI_DAILY_CAP=" + itoa(p.API.AIDailyCap) + "\n" +
		"# Workers AI free tier = 10,000 neurons per UTC day (reset 00:00 UTC = 08:00 Manila). The API\n" +
		"# estimates neurons per call and answers rules-only (ai_status quota) once this many are spent.\n" +
		"# Keep it below 10000 (margin for estimate error and other use of the account). 0 = off.\n" +
		"AI_DAILY_NEURON_BUDGET=" + itoa(p.API.AIDailyNeurons) + "\n" +
		"CAP_TZ=" + p.API.CapTz + "\n" +
		"AI_TIMEOUT_S=" + itoa(p.API.AITimeoutS) + "\n" +
		"# Non-secret Cloudflare account id. Must stay ABOVE AI_BASE_URL: ensure.sh\n" +
		"# sources this file with bash, which expands ${CF_ACCOUNT_ID} on that line.\n" +
		"CF_ACCOUNT_ID=\n" +
		"AI_API_STYLE=" + p.AI.APIStyle + "\n" +
		"AI_BASE_URL=" + p.AI.BaseURL + "\n" +
		"AI_MODEL=" + p.AI.Model + "\n" +
		"AI_FALLBACK_MODEL=" + p.AI.FallbackModel + "\n" +
		"AI_FALLBACK_STYLE=" + p.AI.FallbackAPIStyle + "\n" +
		"# NAME of the env var holding the key (the key itself is NOT stored here).\n" +
		"AI_KEY_ENV=" + p.AI.KeyEnv + "\n" +
		"AI_MAX_OUTPUT_TOKENS=" + itoa(p.AI.MaxOutputTokens) + "\n" +
		"AI_TEMPERATURE=" + ftoa(p.AI.Temperature) + "\n" +
		"AI_REASONING_EFFORT=" + p.AI.ReasoningEffort + "\n" +
		"AI_CHAT_REASONING_EFFORT=" + p.AI.ChatReasoning + "\n" +
		"AI_JSON_MODE=" + boolEnv(p.AI.JSONMode) + "\n" +

		"TURNSTILE_HOSTNAMES=" + hostnames + "\n" +
		"# Secrets below are EMPTY here by design; set them in the live ~/budol/budol-api.env:\n" +
		"# TURNSTILE_SECRET=\n" +
		"# BUDOL_STATS_TOKEN=\n" +
		"# BUDOL_IP_SALT=\n"
	if err := os.WriteFile(filepath.Join("deploy", "budol-api.env.example"), []byte(env), 0o644); err != nil {
		return err
	}
	fmt.Printf("rendered deploy/budol-api.env.example (api=%s)\n", apiURL)
	cfgJSON, err := json.MarshalIndent(map[string]interface{}{
		"api_url":         apiURL,
		"allowed_origins": allowedOrigins,
		"listen":          p.API.Listen,
		"ai":              p.AI,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("deploy", "budol-api.json"), append(cfgJSON, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("rendered deploy/budol-api.json\n")
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// jsonStr renders s as a JSON string literal (with quotes) for safe
// placeholder substitution into web/*.js.
func jsonStr(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// buildCSP renders the page Content-Security-Policy. 'unsafe-inline' is
// added to style-src only when the Turnstile widget is configured (it
// injects inline styles at runtime); scripts stay 'self'-only with no
// inline scripts. connect-src needs 'self' (rules.json is same-origin)
// plus the API origin.
func buildCSP(apiOrigin string, turnstileOn bool) string {
	style := `'self'`
	if turnstileOn {
		style = `'self' 'unsafe-inline'`
	}
	return "default-src 'self'; " +
		"script-src 'self' https://challenges.cloudflare.com; " +
		"frame-src https://challenges.cloudflare.com; " +
		"connect-src 'self' " + apiOrigin + "; " +
		"img-src 'self' data: blob:; " +
		"style-src " + style + "; " +
		"base-uri 'none'; form-action 'none'"
}

func ftoa(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", f), "0"), ".")
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

func boolEnv(b bool) string {
	if b {
		return "1"
	}
	return ""
}
