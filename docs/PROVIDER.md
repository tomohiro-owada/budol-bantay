# AI provider choice (2026-10-26, Asia/Manila)

The user decided against opencode Go / Muse Spark Contributor (the terms forbid third-party use, and the model trains on requests). This file records the replacement research. Every claim links its source, read on 2026-10-26.

## Decision

**Default: Cloudflare Workers AI, model `@cf/openai/gpt-oss-120b`** (OpenAI-compatible chat endpoint; fallback model `@cf/openai/gpt-oss-20b`).
**Alternate 1: Groq, `openai/gpt-oss-120b`.** This is the same model weights, the fastest option, and it has a free tier.
**Alternate 2: OVHcloud AI Endpoints, `gpt-oss-120b`.** It is the cheapest at scale and has contractual no-storage, hosted in the EU.

Switching providers means changing env only: `AI_BASE_URL`, `AI_MODEL`, `AI_API_STYLE`, `AI_KEY_ENV` (+ `AI_CHAT_REASONING_EFFORT`, `AI_JSON_MODE`). See "Config per provider".

## Requirement check

| | Cloudflare Workers AI | Groq | OVHcloud AI Endpoints | Gemini API | Mistral La Plateforme | DeepInfra |
|---|---|---|---|---|---|---|
| Public free app for 3rd parties allowed | Yes. Self-Serve Agreement only bars reselling access / signing up for others (§2.2.1) | Yes. "make the Cloud Services and AI Model Services available to End Users through your Customer Applications" (Services Agreement §3.1) | Yes. Standard Public Cloud service; Client responsible for its use | Partly. Must not be used in apps "likely to be accessed by individuals under the age of 18"; "not for consumer use" | Yes (commercial API) | Yes (commercial API) |
| No training on request data | Yes: "Cloudflare does not use Customer Content processed by Workers AI to train AI models" | Yes: "Groq is not permitted to use Inputs or Outputs for training" (§4.2) | Yes: "Your data is never used to train our models" | Paid tier only. Free tier: "uses the content you submit … to improve … products" | Free mode: may train unless opted out; PAYG: opt-out control | Yes: "We do not train on your data" |
| Retention | "Zero data retention … does not retain inputs or outputs after they are no longer needed" (limited legal/security exceptions) | Not retained by default; abuse/reliability logs "up to 30 days"; ZDR toggle available to all customers | "we only keep what is strictly necessary for billing"; Annex 10: no backup/storage, even ephemeral | Paid: 55 days for abuse monitoring | 30 rolling days (abuse); ZDR on PAYG by request | Not stored to disk; no content logs |
| Free tier fit | 10,000 neurons/day free (≈110 checks/day with gpt-oss-120b) | Free plan: gpt-oss-120b 1K RPD / 200K TPD (≈100 checks/day) | No production free tier (anonymous = 2 req/min/IP, testing only) | Free tier fails privacy | Free mode trains by default | Pay-as-you-go only |
| Paid price (gpt-oss-120b unless noted) | $0.35 in / $0.75 out per 1M (+$5/mo Workers Paid above free) | $0.15 / $0.60 per 1M | €0.08 / €0.40 per 1M | 2.5 Flash-Lite $0.10 / $0.40 | not evaluated | not evaluated |
| OpenAI-compatible | Yes `/ai/v1/chat/completions` | Yes `/openai/v1/chat/completions` | Yes `/v1/chat/completions` | Yes (compat layer) | Yes | Yes |
| PH / JP access | Global network (no geo restriction found) | No PH/JP restriction found | Hosted in EU (Gravelines); global access | Available regions incl. PH/JP | EU | US |
| Tested live from the box | No (needs an account). The same gpt-oss-120b weights were tested via OVH | No (needs a key). Same weights tested via OVH | **Yes** (official keyless anonymous tier) | No | No | No |

Not evaluated in depth: Cerebras (free tier is "for development, evaluation, and experimentation; not intended for production use" — https://www.cerebras.ai/pricing), Together, Fireworks, OpenRouter (data policy depends on the upstream; adds a hop), Meta Llama API (standard tier: no time to verify terms this round).

### Sources
- Workers AI data usage: https://developers.cloudflare.com/workers-ai/platform/data-usage/
- Workers AI pricing: https://developers.cloudflare.com/workers-ai/platform/pricing/
- Workers AI OpenAI-compatible endpoints: https://developers.cloudflare.com/workers-ai/configuration/open-ai-compatibility/
- Workers AI limits (300 req/min text generation): https://developers.cloudflare.com/workers-ai/platform/limits/
- gpt-oss-120b on Workers AI: https://developers.cloudflare.com/workers-ai/models/gpt-oss-120b/
- Cloudflare Self-Serve Subscription Agreement: https://www.cloudflare.com/terms/
- Groq data: https://console.groq.com/docs/your-data · rate limits: https://console.groq.com/docs/rate-limits · models/pricing: https://console.groq.com/docs/models · Services Agreement: https://console.groq.com/docs/legal/services-agreement
- OVHcloud AI Endpoints: https://www.ovhcloud.com/en/public-cloud/ai-endpoints/ · catalog/prices: https://www.ovhcloud.com/en/public-cloud/ai-endpoints/catalog/ · capabilities and limits (anonymous 2 req/min): https://docs.ovhcloud.com/en/guides/public-cloud/ai-machine-learning/ai-endpoints-capabilities
- Gemini API terms: https://ai.google.dev/gemini-api/terms · pricing: https://ai.google.dev/gemini-api/docs/pricing · abuse monitoring (55 days): https://ai.google.dev/gemini-api/docs/usage-policies
- Mistral training: https://help.mistral.ai/en/articles/347617-do-you-use-my-user-data-to-train-your-artificial-intelligence-models · retention: https://legal.mistral.ai/terms/privacy-policy/?lang=en · ZDR: https://help.mistral.ai/en/articles/347612-can-i-activate-zero-data-retention-zdr
- DeepInfra data: https://deepinfra.com/docs/data

## Live Taglish eval (OVHcloud anonymous endpoint, from the dev box)

Tool: `go run ./cmd/aieval`. It uses the production prompt, parser and merge rule on 9 fictional messages (`cmd/aieval/messages.json`). The OVH anonymous tier is rate-limited (2 req/min) and slow, so the **latencies are an upper bound, not representative** of keyed Workers AI / Groq.

Final run, gpt-oss-120b, with the final prompt (2026-10-26 ~14:00–14:15 Asia/Manila): **9/9 correct**.

| Message | Expected | Rules | AI verdict / risk | Final | Injection flag | Latency (OVH anon) | Tokens in/out |
|---|---|---|---|---|---|---|---|
| sim-otp (new SIM-upgrade OTP scam) | scam | 40 | scam / 70 | 70 | no | 28.1 s | 960 / 993 |
| injection attempt | scam | 70 | likely_scam / 90 | 90 | **yes** | 21.6 s | 967 / 861 |
| normal meetup | legit | 0 | no_red_flags / 5 | 5 | no | 9.7 s | 943 / 399 |
| GCash lock link | scam | 100 | scam / 92 | 100 | no | 31.8 s | 982 / 1229 |
| TikTok "like" task job | scam | 80 | likely_scam / 85 | 85 | no | 30.0 s | 984 / 1198 |
| real BPI OTP notice | legit | 0 | no_red_flags / 5 | 5 | no | 22.7 s | 954 / 577 |
| "Ma, bagong number" family emergency | scam | 75 | likely_scam / 80 | 80 | no | 21.3 s | 974 / 902 |
| DSWD ayuda prize + fee | scam | 100 | scam / 92 | 100 | no | 30.5 s | 982 / 1223 |
| real GCash OTP notice | legit | 0 | no_red_flags / 5 | 5 | no | 15.4 s | 947 / 498 |

Earlier comparison runs (first full run, before the prompt/schema fixes noted below; 8 messages):

| Model (on OVH) | Correct (final score band vs expected) | Notes |
|---|---|---|
| gpt-oss-120b | 7/8, then 9/9 after fixes | Natural Taglish; injection detected; one false "safe claim" rejection was a bug in our filter ("is legitimate" matched "is legit"), now fixed |
| Qwen3.8-27B | 8/8 | Best, most natural Taglish; ~1.2k output tokens; 4–5x the output price |
| Llama 3.3 70B | 7/8 | Flags `injection_detected` on ordinary scams (5/8); flagged a real bank OTP notice as likely scam |
| gpt-oss-20b | 7/8 | Flagged a real bank OTP notice as likely scam |

Fixes made because of the eval (all models benefit):
- Schema limits now count characters, not bytes (Japanese is 3 bytes/char). Over-long strings are truncated and lists capped instead of rejected.
- Common shape slips (one language per item; a single object instead of an array) are merged or wrapped. Content is never invented, and a missing language is still rejected.
- The prompt now has a concrete JSON example, plus a line saying a genuine OTP notice that only delivers a code is not itself a red flag.

## Cost estimate (measured tokens, gpt-oss-120b, reasoning effort low)

Measured per check (final run): ~965 input tokens, ~875 output tokens on average (range 400–1,230, incl. reasoning).

| Provider | Per check | 100/day | 300/day | 2,000/day (hard cap) |
|---|---|---|---|---|
| Workers AI (31,818 / 68,182 neurons per M in/out) | ≈90 neurons ≈ $0.0010 | $0 (inside 10k free neurons ≈ 110 checks/day) | ≈$5.6/mo + $5 Workers Paid | ≈$56/mo + $5 |
| Groq ($0.15/$0.60) | ≈$0.00067 | $0 (free plan, ≈110/day by 200K TPD) | ≈$6/mo (Developer plan) | ≈$40/mo |
| OVHcloud (€0.08/€0.40) | ≈€0.00043 | ≈€1.3/mo | ≈€3.9/mo | ≈€26/mo |

The Workers AI free allocation resets daily at 00:00 UTC (08:00 Manila). On the Free plan, requests beyond 10k neurons/day fail; budol-api then returns a rules-only result (`ai_status` error), which is the designed fallback. To avoid hitting that, the API now keeps a neuron ledger (`AI_DAILY_NEURON_BUDGET`, default 8000, UTC day; see `eval/RESULTS.md` §7). The older count cap `AI_DAILY_CAP` uses `CAP_TZ` (Manila) and is only a backstop.

Model shortlist, licenses, data policy and the evaluation harness: `eval/RESULTS.md`. Note: `@cf/google/gemma-3-12b-it` and `@cf/meta/llama-3.1-8b-instruct` are deprecated on Workers AI since 2026-05-30.

## Config per provider (env only)

```sh
# Cloudflare Workers AI (default)
CF_ACCOUNT_ID=<account id>            # not secret
AI_API_STYLE=openai_chat
AI_BASE_URL=https://api.cloudflare.com/client/v4/accounts/${CF_ACCOUNT_ID}/ai/v1
AI_MODEL=@cf/openai/gpt-oss-120b
AI_CHAT_REASONING_EFFORT=low
AI_KEY_ENV=AI_API_KEY                 # ~/.config/budol/ai.env: AI_API_KEY=<API token>

# Groq
AI_BASE_URL=https://api.groq.com/openai/v1
AI_MODEL=openai/gpt-oss-120b
AI_CHAT_REASONING_EFFORT=low
AI_JSON_MODE=1

# OVHcloud AI Endpoints
AI_BASE_URL=https://oai.endpoints.kepler.ai.cloud.ovh.net/v1
AI_MODEL=gpt-oss-120b
AI_CHAT_REASONING_EFFORT=low
AI_JSON_MODE=1
```

Untested on Workers AI: whether `/ai/v1/chat/completions` accepts `reasoning_effort` for gpt-oss. If it returns 4xx, set `AI_CHAT_REASONING_EFFORT=none`. Verify with `go run ./cmd/aieval` once the account exists.
