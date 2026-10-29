# Budol Bantay — Taglish scam-message checker

> "Budol Bantay" is a **placeholder product name**; the final name is undecided and
> the repository can be renamed later (see "How to rename").

Paste a suspicious SMS / Messenger / Viber / email message. The page scores it
against known Philippine scam patterns instantly **on your device**, and (if AI is
ON) shows an AI explanation for a **masked** copy of the message. Results are in
Taglish, English and Japanese. It is a free, non-profit guidance tool: it is not
perfect, not legal or financial advice, and not affiliated with any government
agency, bank, e-wallet or courier.

If you were scammed, report it: CICC hotline **1326** (cicc.gov.ph/report), the PNP
Anti-Cybercrime Group (acg.pnp.gov.ph), and your own bank / e-wallet's official support.

## Architecture

```
phone browser                              API server (Go, standard library only)
┌──────────────────────────┐               ┌────────────────────────────────┐
│ dist/<slug>/             │               │ cmd/budol-api + api/           │
│  app.js ─▶ engine.js     │  masked text  │  re-mask (engine.Mask)         │
│  (rules, instant)        │ ────────────▶ │  engine.Analyze (rules)        │
│  mask.js (PII masking)   │  POST /v1/check│  LLM: Cloudflare Workers AI    │
│  rules.json              │ ◀──────────── │  merge → final score           │
└──────────────────────────┘               └────────────────────────────────┘
        reverse proxy / tunnel (nginx or cloudflared) in front of the API
```

- `engine/rules.json` is the single source of truth for detection patterns, weights,
  explanations and advice. The Go engine (`engine/`) is the reference implementation;
  the browser engine (`web/engine.js`) interprets the same file.
  `node --test web/test/*.test.mjs` checks parity via `engine/testdata/golden.json`.
- Masking is shared semantics: `web/mask.js` on the device, `engine.Mask` again on the
  server (second layer). Both pass `engine/testdata/pii.json`.
- The API always returns a rules result for valid input; AI trouble is reported as
  `ai_status`, never as an HTTP error. Every response carries a `disclaimer`.
- AI runs on the Cloudflare Workers AI **free tier only**. `api/neurons.go` keeps a
  persistent per-UTC-day neuron ledger (default budget 8,000 of 10,000) and falls back
  to rules-only when it is used up. See `eval/RESULTS.md` for the model evaluation and
  `docs/PROVIDER.md` for the provider comparison.

## How to run

Requirements: Go, Node 20+, Python 3 (dataset builder), headless Chrome (optional, for e2e).

```sh
make test        # go vet + go test + node --test
make build-dev   # go run ./cmd/build -env dev   -> dist/<slug>/
make build-prod  # go run ./cmd/build -env prod  -> dist/<slug>/
make dev         # mock-AI API on 127.0.0.1:18790 + static site on 127.0.0.1:8788
                 # open http://localhost:8788/budol-bantay/
make e2e         # bash scripts/e2e.sh: serves both, screenshots, asserts the AI section
```

- The dev API runs with `BUDOL_ENV=dev AI_MODE=mock` (the mock refuses non-dev envs).
- Real-provider smoke test (spends one LLM call, never prints the key):
  `CF_ACCOUNT_ID=… AI_API_KEY=… BUDOL_ENV=dev go run ./cmd/aismoke`
- Model evaluation: `go run ./cmd/aibench -mode rules|est|run|score` on
  `eval/dataset.json` (59 fictional Taglish messages). `-mode run` needs
  `CF_ACCOUNT_ID` and `AI_API_KEY` in the environment and enforces a per-day neuron limit.
- `cmd/bbcheck` (stdin → Analyze JSON) and `cmd/validate` (rules.json check).

### Configuration and secrets

Secrets (`AI_API_KEY`, `TURNSTILE_SECRET`, `BUDOL_STATS_TOKEN`, `BUDOL_IP_SALT`) are read
from the environment only and are **never** committed. Keep them in a private env file
(`.env`, `env.local` and `*.env` are git-ignored). `CF_ACCOUNT_ID` is not secret but is
also taken from the environment. Non-secret defaults live in `config/project.json`;
`go run ./cmd/build` renders `deploy/budol-api.env.example` from it.

## How to rename

Edit only `config/project.json` (`name`, `slug`, origins, API URLs, `githubUrl`, AI
provider section incl. `display_name` / `privacy_note`), then rebuild:

```sh
go run ./cmd/build -env dev|prod
```

The build copies `web/*` to `dist/<slug>/`, substitutes all placeholders (API URL, CSP,
provider wording, GitHub Issues link) and renders `deploy/budol-api.env.example` +
`deploy/budol-api.json` (non-secrets only).

## Privacy model

- The rules check happens on your device; nothing leaves it.
- If AI is ON (default), a **masked** copy is sent to the API, which forwards it to
  Cloudflare Workers AI. Cloudflare states that it does not train on customer content
  and does not retain inputs/outputs after the response (limited legal/security
  exceptions): https://developers.cloudflare.com/workers-ai/platform/data-usage/
- The API does not store or log message text (not even masked); it writes a
  metadata-only log line and anonymous daily counters. See `web/privacy.html` and
  `web/terms.html` for the user-facing text.

## Deploying

`deploy/DEPLOY.md` is a generic guide for running the single Go binary on your own
Linux server behind nginx or a Cloudflare tunnel. Nothing in the repo deploys anything.

## Bugs and feedback

Please use this repository's **Issues**. Do not put personal details or real message
contents in a public issue.

## License

MIT, see `LICENSE`. The scam-message samples in `eval/` and `engine/testdata/` are
fictional (invented numbers, names and look-alike domains). Third-party model licences
(Apache-2.0, Gemma terms, Llama licence) apply to the models you call, not to this code.
