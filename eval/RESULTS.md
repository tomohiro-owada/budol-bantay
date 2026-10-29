# Workers AI model evaluation — Budol Bantay (2026-10-29, Asia/Manila)

**Status: research + harness done; pilot run DONE (20 messages × 6 models, real Workers AI calls, neurons measured — see §9).**
The full 59-message run of the shortlist is NOT done and should wait for the 08:00 Manila reset (00:00 UTC), see §9.5.
Sections 1–8 were written before the pilot; the neuron figures in §5 are *estimates* and §9 supersedes them.

## 1. Candidates (text generation, Workers AI catalog read 2026-10-29)

Neuron rates are from <https://developers.cloudflare.com/workers-ai/platform/pricing/> (updated 2026-10-20).
Licenses are from each model's Hugging Face card / Cloudflare model page.

| Model | Neurons per 1M in / out | Context | License (upstream) | Notes |
|---|---|---|---|---|
| `@cf/openai/gpt-oss-120b` (current default) | 31,818 / 68,182 | 128k | Apache-2.0 (+ OpenAI gpt-oss usage policy) | Reasoning (`low` effort). Only model with a real measurement: 9/9 on the 9-message set via OVH (docs/PROVIDER.md), ~965 in / ~875 out tokens ≈ 90 neurons |
| `@cf/openai/gpt-oss-20b` (current fallback) | 18,182 / 27,273 | 128k | Apache-2.0 (+ usage policy) | Reasoning. On OVH it flagged a real bank OTP notice as scam (docs/PROVIDER.md) |
| `@cf/google/gemma-4-26b-a4b-it` | 9,091 / 27,273 | 256k | Apache-2.0 (Cloudflare page links ai.google.dev/gemma/docs/gemma_4_license, which redirects to /gemma/apache_2) | MoE 26B/4B active, reasoning-capable. Cheapest strong general model; Tagalog quality unmeasured |
| `@cf/qwen/qwen3-30b-a3b-fp8` | 4,625 / 30,475 | 32k | Apache-2.0 | MoE 30B/3B active, thinking model: output tokens can balloon unless thinking is off. Qwen3.8-27B (best Taglish on OVH) costs 40,909 / 290,909 – too expensive for the free tier |
| `@cf/aisingapore/gemma-sea-lion-v4-27b-it` | 31,876 / 50,488 | 128k | Gemma Terms of Use (`license:gemma`) | Southeast-Asia-tuned (SEA-LION covers Filipino), so the most likely to be natural in Taglish; no license link on the CF page, verify the Gemma terms/prohibited-use policy before relying on it |
| `@cf/meta/llama-3.1-8b-instruct-fp8` | 13,778 / 26,128 | — | Llama 3.1 Community License (+ Acceptable Use Policy; "Built with Llama"; >700M MAU clause irrelevant) | The only small Llama still listed. OVH eval showed Llama 3.3 70B flags injection on ordinary scams; 8B expected weaker |

Considered and left out:
- `@cf/meta/llama-4-scout-17b-16e-instruct` 24,545 / 77,273 (Llama 4 Community License): ~86 neurons/check, no cheaper than gpt-oss-120b.
- `@cf/mistralai/mistral-small-3.1-24b-instruct` 31,876 / 50,488 (Apache-2.0): ~71 neurons/check; weaker Taglish is likely.
- `@cf/zai-org/glm-4.7-flash` 5,500 / 36,400 (MIT), `@cf/ibm-granite/granite-4.0-h-micro` 1,542 / 10,158 (Apache-2.0): cheap; good reserve candidates if the main six disappoint.
- `@cf/meta/llama-3.3-70b-instruct-fp8-fast` 26,668 / 204,805: ~190 neurons/check, only ~40 checks/day.
- Availability traps: `@cf/google/gemma-3-12b-it`, `@cf/meta/llama-3.1-8b-instruct` (and `-awq`, `llama-3-8b*`) were **deprecated 2026-05-30** (Workers AI changelog 2026-05-08). `@cf/meta/llama-3.1-8b-instruct-fp8-fast` is still in the pricing table but its model page now redirects to "unavailable", so use `-fp8` (not `-fast`). Models needing a paid plan (kimi, glm-5.x, deepseek-v4*) cannot be used on the Free plan.
- DESIGN.md's old estimates for Llama 3.2 3B / 3.1 8B fp8-fast / 3.3 70B predate this catalog; do not use them.

## 2. Data policy (provider must not train on submitted data)

<https://developers.cloudflare.com/workers-ai/platform/data-usage/> (last updated 2026-10-16), read this turn:
- "**No training**: Cloudflare does not use Customer Content processed by Workers AI to train AI models."
- "**Zero data retention**: … does not retain inputs or outputs after they are no longer needed to process the request and deliver the response", with limited legal/security exceptions; even then the content "is not used to train AI models or improve Cloudflare or third-party products".
- Storage happens only if we add it (R2/KV/AI Gateway logging/caching/batch). The API calls `/ai/v1/chat/completions` directly: no AI Gateway, no logging of text. Keep it that way.
- Cloudflare does not train the models; each model's own license applies (table above).
- Caveat: the models index page has a `data-model-zdr` attribute that reads "No" for every model, including ones on this list. The data-usage page does not make a per-model exception, so I read it as an unused/generic facet, but it is worth one question to Cloudflare support if the privacy text must be airtight.
- Free-plan terms: Self-Serve Agreement §2.2.1 only bars reselling access (docs/PROVIDER.md).

## 3. Test set — `eval/dataset.json` (59 messages)

Built by `eval/build_dataset.py` (rebuild: `python3 eval/build_dataset.py` from the repo root).
- 37 scam / 22 legit; 12 flagged `hard` (rules-evasive scams written without trigger words, and legit messages that look scammy: Smart promo, BPI "we never send links" notice, Meralco bill, LBC pickup, a sibling asking for GCash fare, Lalamove at the gate, PhilHealth notice, ...).
- Reused: 17 from `engine/testdata/corpus.json` and 3 from `cmd/aieval/messages.json` (the ambiguous `extra-13-link-lo` is excluded; 14 near-duplicates dropped to keep a full run inside the free tier). 39 are new.
- Scam types: GCash/Maya/bank phishing, fake parcel (J&T, Messenger "Ma'am" variant), "Ma'am/Sir" job and TikTok/Shopee task scams, investment/crypto, loan (harassment + advance fee), fake prize, family impersonation (SMS and Messenger), OTP requests (incl. "wrong number, forward the code"), gov (LTO/BIR/DSWD/NTC), romance, sextortion, Marketplace/Lalamove, prompt injection.
- Legit: bank/e-wallet OTP notices, InstaPay credit, Shopee/LBC/Grab/Lalamove updates, family chat, work chat, promos, utility/PhilHealth/BPI notices.
- All numbers, names and domains are fictional; phone numbers use 09XX placeholders.

## 4. Measured now: rules-only baseline (`go run ./cmd/aibench -mode rules`)

Scam = positive, threshold rules score ≥ 50:

| | accuracy | precision | recall | TP / FP / TN / FN |
|---|---|---|---|---|
| rules only, all 59 | 83.1% | 100% | 73.0% | 27 / 0 / 22 / 10 |
| rules only, 12 hard | 75.0% | 100% | 40.0% | |

The rules never false-alarm on this set, but miss 10 scams (score of each): sim-otp 40, n-s03 35, n-s05 20, n-s09 20, n-s15 40, n-s18 0,
n-s19 20, n-s21 30, n-s22 45, n-s24 0 (prompt-injection phrased as a bank). That is the room the AI has to fill, and the reason to keep AI on for as many checks as the free tier allows.

## 5. Estimated (NOT measured) neurons per check and checks/day

Input ≈ 965 tokens (measured on gpt-oss-120b; the prompt is ~4.3k chars of English + the message). Output is the unknown: the schema asks for tl/en/ja in every field
(~500 tokens for a non-reasoning model, ~875 measured for gpt-oss-120b at `low`, up to ~1,300 when reasoning runs long).
Cells are `neurons per check / checks per day inside an 8,000-neuron budget`:

| Model | 500 out | 800 out | 1,300 out |
|---|---|---|---|
| gpt-oss-120b | 65 / 123 | 85 / 93 | 119 / 67 |
| gpt-oss-20b | 31 / 256 | 39 / 203 | 53 / 150 |
| gemma-4-26b-a4b-it | 22 / 356 | 31 / 261 | 44 / 180 |
| qwen3-30b-a3b-fp8 | 20 / 406 | 29 / 277 | 44 / 181 |
| gemma-sea-lion-v4-27b-it | 56 / 142 | 71 / 112 | 96 / 82 |
| llama-3.1-8b-instruct-fp8 | 26 / 303 | 34 / 233 | 47 / 169 |

(`go run ./cmd/aibench -mode est -est-out 800` recomputes this from the code's rate table, with a pessimistic chars/3 token estimate.)
Cutting the output to Taglish only (drop `en`/`ja`; the web UI shows one language) would roughly halve output tokens: that is the biggest free-tier lever, but it
changes the production schema, so it was not done here.

## 6. Recommendation (written before the pilot — superseded by §9.4)

1. **Keep `@cf/openai/gpt-oss-120b` as the only model that has real evidence (9/9)**, but it costs ~85–90 neurons/check ≈ **90 checks/day**.
   Candidate to replace it as primary, pending the live run: **`@cf/google/gemma-4-26b-a4b-it`** (~2.7× cheaper, ~260 checks/day, Apache-2.0).
   Try **`gemma-sea-lion-v4-27b-it`** as the Taglish-quality challenger; use **`gpt-oss-20b`** as fallback only if it does not mislabel real OTP notices on this set.
   Switch the primary only if the candidate matches gpt-oss-120b on this set: recall on scams ≥ 95%, zero false positives on the 22 legit, ≥ 98% schema-valid, and the Tagalog spot-check reads naturally.
2. **Settings:** `AI_CHAT_REASONING_EFFORT=low` for gpt-oss only (`none` for other models; they reject unknown params), `AI_MAX_OUTPUT_TOKENS=2500` (reservation is computed from it),
   temperature 0.2, no JSON mode until verified on Workers AI.
3. **Daily cap, enforced in code (api/neurons.go):** `AI_DAILY_NEURON_BUDGET=8000` neurons per **UTC** day (80% of the 10,000 free allowance; the margin covers estimate error and other use of the same account such as playground and eval runs),
   plus the old count cap lowered to `AI_DAILY_CAP=150` as a backstop. That is ≈ 90 checks/day on gpt-oss-120b and 200–260 on gemma-4. Beyond the cap `/v1/check` still returns 200 with the rules result and `ai_status: "quota"`.
4. **Bug fixed on the way:** the old count cap used `CAP_TZ=Asia/Manila` (reset 00:00 Manila = 16:00 UTC) while Workers AI resets 00:00 UTC (08:00 Manila). A Manila-day cap can let through up to ~2× the free allowance. The neuron ledger always uses the UTC day.

## 7. How the cap works

- `EstimateNeurons(model, in, out)` uses the pricing table above (unknown models are priced pessimistically at 60,000 / 205,000).
- Before each provider call (primary and fallback) the server reserves a worst case (prompt/3 tokens in, `AI_MAX_OUTPUT_TOKENS` out). If `used + in-flight + reservation > budget`, no call is made → `ai_status: quota`.
- After the call it settles the real cost from the provider's token usage. HTTP rejections are charged input only; timeouts/transport errors/unusable output are charged the full reservation.
- An error 4006 / "daily free allocation" reply exhausts the day immediately.
- State persists in `$BUDOL_STATE_DIR/neurons.json` (UTC day) so a restart or cron `ensure.sh` cannot reset the budget. `GET /v1/stats` reports `neurons: {day_utc, budget, used}`.
- Caveat: the estimate has not been compared with the Cloudflare dashboard yet. After the first live run, compare `used` with the dashboard's Workers AI neurons and adjust the 8000 if they diverge.

## 8. To complete the live evaluation (original instructions; the pilot in §9 used these tools)

Use the variable names the code already reads: `CF_ACCOUNT_ID` and `AI_API_KEY` (not `CF_WORKERS_AI_TOKEN`).

```sh
# never print the token; the run command only prints counts/neurons
set -a; . ./env.local; set +a   # your private env file, never committed
# pilot: 20 stratified messages, 6 models ≈ 4k neurons, hard-stopped at 4000
go run ./cmd/aibench -mode run -sample 20 -neuron-limit 4000
# remaining budget the same UTC day (resets 00:00 UTC = 08:00 Manila): full 59 on the 2-3 best models
go run ./cmd/aibench -mode run -models @cf/google/gemma-4-26b-a4b-it,@cf/aisingapore/gemma-sea-lion-v4-27b-it -neuron-limit 4000
go run ./cmd/aibench -mode score          # markdown table: validity, acc/prec/rec, latency, neurons
```

`aibench` refuses to spend more than `-day-limit` (default 9,000 estimated neurons) per UTC day across all runs, using worst-case reservations, and stops on error 4006.
A full 6-model × 59-message run (~11k neurons) does not fit in one free day: run the pilot first, then finish the shortlist after 08:00 Manila.
Then spot-check ~10 `explanation.tl` strings by hand (results are in `eval/results/*.jsonl`, git-ignored) and fill in this file's results table.

## 9. Pilot results (measured, 2026-10-29 ≈ 23:05–23:25 Asia/Manila = 15:05–15:25 UTC)

### 9.1 Method

- **Token/endpoint:** the user's Workers AI *Read* token works for `POST /accounts/{id}/ai/v1/chat/completions` (HTTP 200) and the model list (`/ai/models/search`, HTTP 200). All six model ids below exist in the live list. The GraphQL analytics API answered `not authorized for that account`, so the dashboard could not be used to cross-check.
- **Measured neurons:** Workers AI returns `usage.neurons` (a float) in every chat-completions response. `api/provider.go` now keeps it (`LLMResult.Neurons`) and `cmd/aibench` records it per call (`"measured": true` in the jsonl) instead of estimating from tokens. The reported value matches the pricing table: e.g. llama-3.1-8b with 12 in / 5 out tokens → 0.296 neurons = 12×13,778/1M + 5×26,128/1M. All 120 pilot calls were measured, none estimated. neurons/check below = mean of the 20 measured calls (includes runaway/invalid outputs, because they are billed too).
- **Sample (20 of 59, fixed ids in this order):** scams (11): n-s03, n-s06, n-s13, n-s15, n-s22 (5 *hard*), poc-02-parcel-hold, poc-10-bdo-spoof, poc-01-gcash-lock, n-s18, n-s24 (prompt injection), ev-sim-otp. Legit (9): n-l04, n-l08, n-l10, n-l15 (4 *hard*), poc-11-otp-notice, n-l01, n-l12, n-l17, n-l05. 9 hard in total. Rules miss 6 of the 11 scams, so it is deliberately harder than the full set.
- **Prompt/parse/merge:** production prompt, `api.ParseAIOutput` and the production merge rule (final = max(AI risk, rules if ≥30), injection ≥40); predicted scam = final ≥ 50. An invalid output (parse failure) falls back to the rules score, as in production. `max_tokens` 2500, temperature 0.2, `reasoning_effort=low` only for gpt-oss, no JSON mode. HTTP timeout 120 s here (production default is 25 s, see 9.3). The six models were run in parallel, one process each, so latencies include some self-inflicted queueing.
- **Spend control:** per-process and per-day neuron limits in aibench; I polled the cumulative total and stopped well before 6,000.
- **Neurons used today (UTC day 2026-10-29):** pilot 5,822 (sum of the 120 measured calls) + about 100 for smoke tests and three aborted/duplicate calls ≈ **5.9k of 10k**. Anything else running on the same account today (playground, other agents) is not included because the analytics API is not readable with this token.

### 9.2 Results (n = 20; 11 scam, 9 legit; 5 hard scams, 4 hard legit)

Metrics are the production merged result (rules + AI, parse failure → rules only). Scam = positive.

| Model | Accuracy | Precision | Recall | Hard-scam recall | FP / FN | Parse failures | **Measured neurons/check** (mean, min–max) | Checks/day @10k free / @8k budget | Latency p50 / p95 / max | Out tokens (mean) |
|---|---|---|---|---|---|---|---|---|---|---|
| rules only (baseline, same 20) | 70% | 100% | 45.5% | 2/5 (40%) | 0 / 6 | – | 0 | – | local | – |
| `gpt-oss-120b` | **100%** | 100% | 100% | 5/5 | 0 / 0 | 0 | **85.2** (49–113) | 117 / **93** | 14.1 / 20.2 / 21 s | 800 |
| `gpt-oss-20b` | 95% | 100% | 91% | 4/5 | 0 / 1 | 2 (bad JSON) | **29.0** (22–35) | 345 / **276** | 5.1 / 7.7 / 8 s | 417 |
| `gemma-4-26b-a4b-it` | 100% | 100% | 100% | 5/5 | 0 / 0 | 2 (hit 2,500 max tokens, no JSON; rules rescued both) | **54.5** (33–77) | 183 / **146** | 33.4 / 56.3 / 61 s | 1,686 |
| `qwen3-30b-a3b-fp8` | 75% | 88% | 64% | 3/5 | 1 / 4 | 1 (bad JSON) | **33.0** (23–41) | 302 / **242** | 9.0 / 16.4 / 17 s | 944 |
| `gemma-sea-lion-v4-27b-it` | 90% | 85% | 100% | 5/5 | 2 / 0 | 0 | **60.3** (45–72) | 165 / **132** | 13.7 / 18.7 / 26 s | 583 |
| `llama-3.1-8b-instruct-fp8` | 85% | 79% | 100% | 5/5 | 3 / 0 | 2 (1 invalid, 1 ran to 2,500 tokens) | **29.2** (17–78) | 342 / **273** | 33.6 / 38.7 / 115 s | 639 |

Checks/day = floor(budget ÷ mean measured neurons/check). The AI_DAILY_CAP=150 count backstop is therefore only binding for gpt-oss-20b and qwen/llama/gemma-4 at 8k; for the others the neuron budget binds first.

Wrong decisions: gpt-oss-20b — n-s03 and n-s13 were unparseable, rules alone then missed n-s03 (hard); qwen — ev-sim-otp, n-s03, n-s15 (scores 40–45, just under 50) and n-s24 (unparseable) missed, n-l04 false alarm; sea-lion — n-l01 (a real bank OTP notice) and n-l04 flagged "suspicious" (60); llama — n-l04, n-l08, n-l12 flagged (60) and n-l01/n-l15 unparseable.

Estimates vs measurement (neurons/check): gpt-oss-120b 85→**85.2** (exact), gpt-oss-20b 39→**29.0**, gemma-4 31→**54.5** (it reasons; the output averaged 1,686 tokens instead of ~800), qwen3 29→**33.0**, sea-lion 71→**60.3**, llama 34→**29.2**.

### 9.3 Explanation quality (`explanation.tl`, quick judgment on n-s13, n-s22, n-l10)

- **gpt-oss-120b:** best. Correct, complete, practical advice, mildly formal Tagalog with English terms ("advance fee scam", "red flag"); sometimes dry. Natural Taglish.
- **gemma-sea-lion-v4:** very natural and conversational ("Ito ay isang paraan para nakawin ang iyong pera"), direct advice. Over-cautious on legit messages.
- **gemma-4-26b:** natural Taglish, concise and accurate when it finishes (it did on 18/20), but slow and prone to runaway output.
- **gpt-oss-20b:** fine and accurate, somewhat generic; the advice is thinner than 120b.
- **qwen3-30b:** awkward/wrong wording ("pukpukin ang katiwalian sa pagmamahal ng pamilya", "nag-uutos na wala silang nagpapadala"); do not show to users.
- **llama-3.1-8b:** poor: advice is incoherent or backwards ("Huwag mag-load ng pera sa kanyang bagong numero", "Kung may gusto kang mag-order sa Lalamove…"). Do not use.
- Every valid `tl` string passed the crude Tagalog-marker check; the judgment above is mine from three samples per model, not a rigorous rating.

**Latency note:** production has `AI_TIMEOUT_S=25`. Only gpt-oss-20b (max 8 s), gpt-oss-120b (max 21 s) and qwen (max 17 s) stayed under that in this run. sea-lion's slowest call was 26 s (would time out) and gemma-4/llama regularly exceeded it, with a timeout charging the neurons anyway. Parallel runs probably inflated these numbers somewhat; re-measure serially before relying on sea-lion.

### 9.4 Recommendation (n=20 is a pilot; 5 hard scams only)

1. **Primary: `@cf/openai/gpt-oss-120b`.** Only model with 20/20 correct, 0 false alarms, 5/5 hard scams, 100% schema-valid, best Taglish, latency inside the 25 s timeout. Measured **85.2 neurons/check = 93 checks/day at the 8,000 budget** (117 at the full 10k). That is the free-tier fit; `AI_DAILY_NEURON_BUDGET=8000` binds before `AI_DAILY_CAP=150`.
2. **Runner-up / fallback: `@cf/openai/gpt-oss-20b`.** 29.0 neurons/check = **276 checks/day at 8k**, 5 s latency, 0 false alarms, 91% recall (both misses came from invalid JSON that fell back to rules). It is already the configured `fallback_model`. If more than ~90 AI checks/day are needed, use it as primary for the overflow, accepting a lower recall.
3. Not recommended: llama-3.1-8b (3 false positives, nonsensical Tagalog), qwen3-30b (misses 4 scams, awkward Tagalog), sea-lion (2 false positives incl. a real OTP notice, 60 neurons, slow tail). gemma-4-26b was accurate where it finished but 33 s p50 and 2/20 runaway outputs make it unusable until the reasoning is turned off; that was not tested.
4. Keep `AI_MAX_OUTPUT_TOKENS` ≤ 2500: a runaway call is billed (gemma-4/llama up to 77 neurons for a useless answer).

### 9.5 Next steps

- **A full 59-message run of the shortlist (gpt-oss-120b + gpt-oss-20b) should wait for the 08:00 Manila reset (00:00 UTC).** The 39 remaining messages cost about 39×85 + 39×29 ≈ **4.5k neurons**; with ≈5.9k already used today that would total ≈10.4k, over the 10,000 free allowance. After the reset, run (the 20 pilot messages are skipped automatically, and results are appended):

```sh
export CF_ACCOUNT_ID=…   # AI_API_KEY from the env; never print it
go run ./cmd/aibench -mode run -models @cf/openai/gpt-oss-120b,@cf/openai/gpt-oss-20b -neuron-limit 5000 -day-limit 5000
go run ./cmd/aibench -mode score -models @cf/openai/gpt-oss-120b,@cf/openai/gpt-oss-20b
```
- Optional second run the same day after the reset: gemma-4-26b with reasoning off, and sea-lion run serially, if a cheaper/faster primary is still wanted.
- `AI_TIMEOUT_S`: 25 s is tight for 120b (max 21 s here); consider 30 s.
- Compare 8,000 vs the dashboard figure once the account has a token with analytics access.
