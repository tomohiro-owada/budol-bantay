# Scam-message checker: design proposal (draft, 2026-10-26)

Working name **Budol Bantay** (placeholder; the final name is undecided).
Status: original design notes (historical). The implemented design is described in README.md, docs/BUILD-SPEC.md and docs/PROVIDER.md.
PoC: `poc/index.html`.

---

## 0. TL;DR
- **Recommendation: (d) hybrid, phased.** Ship **v1 as a fully client-side, rules-only static page** (option a) at `abalol.xyz/budol-bantay/`. Add an **opt-in "Explain with AI" button later** (option c) through a free Cloudflare Worker and Workers AI Llama. The button redacts the message on the phone first and only sends it when the user taps it.
- **Do not use an in-browser LLM (b) as the main engine.** It means a download of about 0.7 GB, it fails on many mid-range Android phones and on in-app browsers, and FB shares open in the FB in-app browser.
- **The tool never says "safe".** The best possible verdict is "Walang nakitang karaniwang red flag / No common red flags found", followed by "this is not a guarantee".

## 1. Architecture options

| | (a) Static + rules (JS or Go→WASM) | (b) In-browser LLM (WebLLM / transformers.js) | (c) Static + free serverless LLM | (d) Hybrid: a first, c opt-in |
|---|---|---|---|---|
| Privacy | Best. Text never leaves the device | Best. On-device | Text goes to a third party (Cloudflare/Groq) | a by default; c only after explicit tap + redaction |
| Cost | ₱0 (GitHub Pages) | ₱0 hosting, but users pay with mobile data | Free tier, with daily caps | ₱0 |
| Works on cheap phones / FB in-app browser | Yes, instantly, even offline once cached | Often **no** (see below) | Yes | Yes |
| Taglish quality | Good for known patterns. Blind to new ones | 1B–3B models are weak at Tagalog and hallucinate | Good with 8B+/70B | Rules give the verdict; AI only explains |
| Abuse risk | None | None | Real: bots can drain the quota | Limited to the button |
| Maintenance | Rule updates + test corpus | Model pinning, cache issues | Worker + quota monitoring | Both, but AI is optional |

**Facts behind (b), checked 2026-10-26:**
- WebLLM `Llama-3.2-1B-Instruct-q4f16_1` is a **695 MB download** and needs about **879 MB VRAM**. Llama 3.2 3B needs about 2.26 GB. SmolLM2-360M is 204 MB. Source: pinggy.io blog and web-llm `config.ts` / issue #683.
- Measured on a Pixel 8a (8 GB RAM, Chrome 149): roughly 9–10 tok/s decode, with **about 76 s to first token for a 1.2k-token prompt**. The same model **never completed in the LINE in-app browser**, and on iPhone 11 Safari the tab reloaded. Source: ludion.ai/blog/webgpu-reports-vs-reality.
- WebLLM init fails on some Qualcomm Adreno GPUs with `VK_ERROR_DEVICE_LOST`. Source: mlc-llm issue #3502.
- Conclusion: (b) is unsuitable for PH mid-range phones on prepaid data, and for traffic arriving from FB.

**Facts behind (c), checked 2026-10-26:**
- **Cloudflare Workers AI:** 10,000 Neurons/day free, reset 00:00 UTC (08:00 PH). Free-plan usage simply stops at the cap; nothing is billed. Source: developers.cloudflare.com/workers-ai/platform/pricing/. Estimated free calls per day, assuming a prompt of about 600 input tokens and about 250 output tokens:
  - `@cf/meta/llama-3.2-3b-instruct`: about 10 neurons/call, so **~950 calls/day**
  - `@cf/meta/llama-3.1-8b-instruct-fp8-fast`: about 11 neurons/call, so **~900 calls/day**
  - `@cf/meta/llama-3.3-70b-instruct-fp8-fast`: about 67 neurons/call, so **~150 calls/day**
- **Groq:** **Llama was removed from the free/Developer tiers on 2026-08-16.** The free tier now offers gpt-oss-20b/120b and Qwen at 1,000 req/day and 200k tokens/day. Sources: console.groq.com/docs/deprecations and /docs/rate-limits. If you want Llama, use Workers AI. Groq is the fallback only if non-Llama models are acceptable.
- The Worker itself would be JS/TS. Go on Workers via WASM is possible but awkward.
- Llama licence: the Llama 3.x Community License allows this use. It requires "Built with Llama" attribution and compliance with the Acceptable Use Policy.

**Why (d):**
- Rules catch the large majority of PH smishing. The playbook is narrow and repetitive: links, OTP, fees, raffles, parcels, tasks, ayuda.
- Rules are explainable and private, and they work everywhere.
- AI is only for "explain this in plain Taglish" and for messages the rules can't classify. It never overrides a high-risk rules verdict downwards.

### Engine language (Go preference)
I measured a minimal Go→WASM build with `regexp` on the box: **2.6 MB raw / ~730 KB gzip**. That is acceptable once cached, but heavy on first load over 3G/4G. Options:
1. **Go engine compiled with TinyGo.** Probably a few hundred KB; not measured. The same Go package can also serve a CLI test runner and a future Messenger bot or Worker.
2. **Rules as a data file (`rules.json`) and a tiny JS interpreter (~30 KB)**, with Go used for the rule compiler/validator and a CI corpus test.

Option 2 has the fastest first paint. Option 1 fits your Go preference better. I suggest starting with option 1 and falling back to 2 if TinyGo output is over ~400 KB gzip. The PoC is plain JS only so it could be built quickly.

## 2. Data sources

Verification legend: **✅** = loaded from the box (HTTP 200 or headless Chrome showed the right title). **🛡️** = exists (cited by official FB posts, PNA, PIA or news) but the site is behind a Cloudflare bot challenge, so I could not open it from the box. A human with a browser should open these before launch.

| Source | What we use | Status |
|---|---|---|
| CICC / I-ARC: **Hotline 1326** (24/7); Smart 0947-714-7105, Globe 0966-976-5971, DITO 0991-481-4225; report@cicc.gov.ph; cicc.gov.ph/report/; FB facebook.com/CICCgovph | "What to do / report" text | 🛡️ cicc.gov.ph. Numbers confirmed via scamwatchpilipinas.com/faqs/ ✅ and a CICC FB post from May 2026 |
| eGovPH app → Report → Scam (PhilSTAR Life explainer) | Reporting option | via news |
| NTC memo, 2023-09-27: telcos must block SMS with clickable domains, URLs, shorteners, Smart Links and QR codes (Globe newsroom, Malaya, GMA) | Basis for the "links in SMS are a red flag" rule | 🛡️ ntc.gov.ph; news pages found via search |
| **RA 12010 (AFASA)**, full text in the SC E-Library: elibrary.judiciary.gov.ph/thebookshelf/showdocs/2/97690 | Legal framing (social-engineering schemes, money mules) | ✅ |
| **BSP AFASA IRR booklet** (bsp.gov.ph/Regulations/Banking Laws/AFASA-Booklet-with-IRRs.pdf): banks/EMIs "must not send clickable links or QR codes via email, IM or SMS" unless the customer prompted it and it is info-only | Basis for the "bank/e-wallet + link" rule | 🛡️ (403 from box) |
| BSP Consumer Assistance: BSP Online Buddy (BOB) chatbot, consumeraffairs@bsp.gov.ph, after complaining to the bank first | Escalation advice | 🛡️ |
| GCash Help Center, "How can I protect my GCash account?"; official FB video "Official GCash will never send links" (SMS spoofing) | E-wallet facts. Spoofing means sender ID ≠ proof | ✅ title loaded once in headless Chrome, then challenged |
| Maya: "What is text hijacking or spoofing…" (maya.ph/stories/…) and support.maya.ph "Someone scammed me… what should I do?" | E-wallet facts and what to do after being scammed | ✅ both |
| ScamWatch Pilipinas FAQ (scamwatchpilipinas.com/faqs/) | Reporting flow | ✅ |
| PNP-ACG (acg.pnp.gov.ph), DICT (dict.gov.ph), SEC (sec.gov.ph, investment advisories) | Agency links; SEC for investment and lending checks | 🛡️ |
| Official brand domains (gcash.com, maya.ph, lbcexpress.com, jtexpress.ph, dswd.gov.ph, sss.gov.ph…) | Look-alike detection allowlist | gcash.com, maya.ph, lbcexpress.com, jtexpress.ph, dswd.gov.ph, sss.gov.ph ✅. The remaining domains in the PoC list still need checking |
| PhishTank (phishtank.com/developer_info.php), URLhaus (urlhaus.abuse.ch/api/), mitchellkrogza/Phishing.Database | Optional build-time blocklist of PH-brand phishing domains | ✅ pages load. **Read each ToS before use**: PhishTank needs registration and attribution; abuse.ch has "fair use principles". Lists go stale within days, so heuristics matter more |

**Licence notes**
- Philippine government works carry no copyright (IP Code, RA 8293, s.176). Prior approval is only needed for profit, and this project is non-profit. We will still link rather than rehost.
- GCash and Maya pages are copyrighted. We use **facts and paraphrase** with a link, and never copy text or images.
- No scraping. The advisory list is curated manually, around monthly, maybe from the FB Page workflow.
- No "known-legit sender ID" list will be used as a *safe* signal, because GCash and Maya both warn about spoofed sender IDs. A sender is only used as a *negative* signal: a regular 09xx number claiming to be a bank, agency or courier.
- Test corpus: write **synthetic** messages modelled on advisories. Do not reuse real screenshots, which contain other people's numbers.

## 3. Detection rules (v0, implemented in the PoC)

Score = sum of weights, plus combo boosts, clamped to 0–100.

| Score | Taglish | English | Japanese |
|---|---|---|---|
| ≥60 | "MATAAS ang panganib" | High risk | 危険度 高 |
| 30–59 | "Kahina-hinala" | Suspicious | 要注意 |
| 10–29 | "May ilang senyales" | Some warning signs | 注意点あり |
| <10 | "Walang nakitang karaniwang red flag" | No common red flags found | よくある危険サインなし (+ not a guarantee) |

**Links**
- `shortener` (+25): bit.ly, tinyurl, cutt.ly, is.gd, rb.gy, s.id, shorturl.at…
- `lookalike` (+35): a brand or agency token appears in the host but the registrable domain is not official (handles `.com.ph` / `.gov.ph`). Also matches `gov-ph` that doesn't end in `.gov.ph`.
- `riskytld` (+15): .top .xyz .icu .vip .cc .shop .online .site…
- `rawip` (+25): raw IP address or punycode `xn--`
- `brandlink` (+20): a bank or e-wallet is mentioned AND there is a link (AFASA)
- `link` (+10): any link. Dropped when a stronger link rule fires
- `qr` (+10): asks the user to scan a QR code

**Credentials / account**
- `credential` (+40): asks to send, ibigay, reply or enter an OTP, MPIN, PIN, password, CVV or code. Ignored when negated ("never share", "huwag ibigay").
- `verifyacct` (+15): "i-verify / update / login … account / KYC"
- `threat` (+15): na-lock, suspended, deactivated, ma-forfeit…
- `remote` (+30): AnyDesk, TeamViewer, .apk, "download this app"

**Pressure / social engineering**
- `urgency` (+15): within 24 hours, ngayon din, agad, final notice, limited slots…
- `secrecy` (+15): "huwag mo munang sabihin", "keep confidential"
- `offplatform` (+15): Telegram, WhatsApp, @handles
- `replyYes` (+10): "reply YES"
- `senderMobile` (+20): the optional sender field holds a 09xx/+639 number and the opening of the message claims a bank, e-wallet, agency or courier

**Money**
- `sendMoney` (+20): send, padala, load, deposit, top-up, near an amount or a GCash/Maya number
- `personalAcct` (+15): "GCash/Maya 09xx…"
- `feeToClaim` (+25): a fee, "bayad" or "magbayad" in a prize, loan, job or parcel context

**Scam families**
- `prize` (+25) and `fakePermit` (+10, "DTI permit" used as a credibility prop)
- `job` (+20), `highpay` (+15, e.g. "₱800–3,000/araw"), `task` (+30, like/review then recharge)
- `parcel` (+25): courier mention + on hold / incomplete address / redelivery
- `loan` (+25): approved na, no collateral, pre-approved
- `ayuda` (+20): DSWD, SSS, 4Ps, AICS + claim / register / listahan
- `invest` (+30): guaranteed, % daily, USDT, crypto, VIP group
- `family` (+25): "bagong number ko", "nasira phone", "si Carlo 'to"

**Combo boosts and reductions**
- +10 for credential + (threat or urgency)
- +10 for lookalike + verifyacct
- −25 for `otpNotice`: a standard "never share your OTP" notice with no link and no request

PoC self-test (`poc/index.html#selftest`): the 10 scam examples score 60–100 (all high). The 2 controls, a legit-style OTP notice and a casual chat, score 0 and get "no common red flags".

**Avoiding false reassurance**
- Never output "safe" or "legit", in any language.
- The lowest band always shows "not a guarantee" plus the general advice and the report line.
- AI (phase 2) may raise the risk level or add explanation, but can never lower a rules verdict of ≥30.
- Keep a regression corpus of synthetic examples, including tricky legit messages (bank OTP notices, real courier SMS without links). CI fails if a scam drops a band or a control rises.

**Next rules to add**
- Homoglyphs (`gcazh`, `rnaya`, Cyrillic letters)
- Mismatches between the brand named and the link
- Unusually precise small fees (₱45, ₱99)
- Romance and "investment coach" scripts
- SIM-registration and "e-SIM upgrade" scams
- Fake BIR/LTO fines and fake Meralco disconnection notices
- Number normalisation ("O9I7", spaced digits)

## 4. UX

- Single page at `abalol.xyz/budol-bantay/`, mobile-first, no external fonts or scripts, about 40 KB.
- A CSP with `connect-src 'none'` in v1 makes "nothing is sent" technically true and auditable.
- **Flow:** paste box, optional sender field, then the **I-check** button. The result card has:
  1. Coloured verdict and score bar
  2. **Bakit?**: each red flag in plain Taglish, with the matched snippet
  3. **Ano ang dapat gawin**: steps per category (OTP shared, money sent, parcel, ayuda, job, investment via SEC, family call-back)
  4. Report line: CICC 1326, report@cicc.gov.ph, eGovPH
  5. Disclaimer
- Language toggle: Taglish (default), English, 日本語. Remembered in localStorage only if you want that.
- **Share image:** a 1080×1080 PNG drawn on canvas showing the verdict, red-flag list, "CICC Hotline 1326" and the URL. **It never includes the pasted text or the sender**, for privacy. It is downloaded locally; later this can use the Web Share API (`navigator.share({files})`) on Android.
- Examples drawer with clearly labelled **EXAMPLE** messages, so people can learn the modus without pasting anything.
- **FB Page (later, optional):**
  - "Modus of the Week" posts made from the share-image template
  - A pinned post linking the checker
  - Comments pointing to it
  - A Messenger bot is possible later, but it needs a server (a Worker would do) and a Meta privacy policy. Defer it; it breaks the "nothing leaves your phone" story.
- **Name ideas:**
  1. **Budol Bantay**: "budol" is the everyday PH word for being conned. Catchy, Taglish, and memorable on FB.
  2. **Scam Bantay**: clearest for English and Japanese readers, and best for search.
  3. **Text Bantay** (or *Modus Bantay*): emphasises SMS. Narrower scope, since the tool also covers Messenger, Viber and email.

## 5. Privacy and legal

- **v1:** no network calls after load (enforced by CSP), no analytics, no storage of messages, no cookies.
- GitHub Pages still sees normal request logs (IP, user agent) for the page load itself. Say so in the privacy note.
- The UI suggests blanking out names, account numbers and addresses before pasting.
- **Phase 2 AI button:**
  - Explicit per-use consent: "Ipapadala ang (na-redact na) mensahe sa Cloudflare para sa AI explanation."
  - Client-side redaction before sending: phone numbers, emails, account/card numbers, OTP-like 4–8 digit codes, and personal names after "Hi/Dear/si".
  - The Worker keeps no logs of the body (no `console.log` of content; Workers Logs/Logpush off).
  - Cloudflare Turnstile (free), plus per-IP rate limiting (Workers rate-limit binding), a max of about 1,500 characters, and a daily budget guard.
  - When the quota is gone, show "AI busy, rules result above still applies".
  - Cloudflare's no-training statement for Workers AI should be checked on its own terms page before launch.
- **Disclaimers:**
  - Not affiliated with CICC, NTC, BSP, DICT, PNP, SEC, GCash, Maya, banks or couriers. Logos are not used.
  - Informational only, not legal or financial advice.
  - The score is heuristic. When in doubt, contact the institution through its official app or hotline.
- **Data Privacy Act (RA 10173):** v1 processes nothing server-side, which minimises obligations. Phase 2 needs a short privacy notice (purpose, processor = Cloudflare, no retention).
- **Llama** (phase 2): show "Built with Llama" and follow the Llama AUP.
- **Infrastructure:** GitHub Pages (existing) plus, optionally, the Cloudflare Workers free plan. **No VPS needed.**

## 6. Decisions for the maintainer
1. **Name and slug:** Budol Bantay / Scam Bantay / Text Bantay?
2. **Engine:** Go (TinyGo→WASM, one codebase for a future bot) or JS interpreter + Go-validated `rules.json` (smallest page)?
3. **Phase 2 AI:** include at all? If yes, Workers AI Llama 3.1 8B fp8-fast (about 900 calls/day free) or 3.3 70B (about 150/day, better Tagalog)? A Cloudflare account is needed.
4. **Optional sender field:** keep it, or keep the UI to one box only?
5. **Share image:** OK as verdict-only (no message text)?
6. **FB Page:** a new "Budol Bantay" Page, or post under an existing Bantay Page?
7. **Blocklists** (PhishTank/URLhaus at build time): worth the ToS and maintenance, or heuristics only for v1?
8. **Who curates monthly rule updates** from new advisories (you, or an agent routine that drafts PRs for review)?

---

## 7. v1 as built (2026-10-26)

### Engine decision: JS engine + Go-validated `rules.json` (TinyGo dropped)

Measured on this box (see DECISION UPDATE in `docs/BUILD-SPEC.md`):

- TinyGo default WASM build: **1.36 MB raw / ~488 KB gzip** — over budget.
- That build **panics at runtime** with `regexp: unhandled case in compile`.
- `-scheduler=none` gets to ~308 KB gzip but **crashes** with
  "memory access out of bounds" inside `regexp/syntax`.

TinyGo's regexp is unreliable for our patterns, so the engine is now:

- `engine/rules.json`: single source of truth (patterns, weights, i18n,
  advice, combos, levels). Patterns stay in the RE2∩JS common subset
  (no lookbehind/ahead, backreferences, named groups, possessive
  quantifiers); `go test` enforces this plus schema/i18n/corpus checks.
- `engine/` (standard Go): reference implementation and validator, plus
  `cmd/bbcheck` and `cmd/validate`.
- `web/engine.js`: small ES-module interpreter of the same file for the
  browser; `web/test/parity.test.mjs` (`node --test`) asserts the same
  level/score/hit-ids as Go via `engine/testdata/golden.json`.
- Masking: `web/mask.js` (device) + `engine/mask.go` (server second
  layer), both tested against `engine/testdata/pii.json`.
- Size: engine.js + rules.json + mask.js together are ~60 KB unminified.

### Server API design (Go, `api/` + `cmd/budol-api/`)

Replaces the Cloudflare Worker (kept in git history). Runs as the
unprivileged service user under `~/budol/`, listening on
`127.0.0.1:8790`; TLS/hostname come from nginx or a cloudflared tunnel.

- `POST /v1/check` (≤8 KB JSON): `{text (already masked), lang, sender,
  ai, client_rules, turnstile_token}`. The server re-masks with
  `engine.Mask` (mask-aware rules treat `[PHONE]`/`[CODE]` like the real
  thing), runs `engine.Analyze`, optionally calls the LLM, and merges:
  `final = rules≥30 ? max(rules, ai.risk) : ai.risk`, injection ⇒ ≥40.
  A rules result is ALWAYS returned; AI trouble is `ai_status`
  (`ok|disabled|skipped|quota|rate_limited|timeout|error|bad_output|
  `unconfigured`), never an HTTP error. Full contract:
  `docs/openapi.yaml` (covered by `api/openapi_test.go`).
- Abuse protection (in-process): per-IP token buckets (salted SHA-256
  keys, memory-only), 20 AI calls/hour/IP, global daily AI cap (default
  2000, persisted counters only), AI semaphore (3, 2 s wait), `http`
  timeouts, exact-match CORS, optional Turnstile (action `check`,
  fail-closed), structured metadata-only logging (a test asserts no
  text/IP/key ever hits the logs).
- LLM provider (`config/project.json` → `ai` section, no secrets): opencode
  Go, `openai_responses` style (`{base_url}/responses`), default model
  Muse Spark with a `deepseek-v4.1-flash`/`openai_chat` fallback.
  Live-test lessons baked in: `reasoning.effort: minimal` (a `high` effort
  with 1200 tokens spent everything on reasoning → `incomplete` /
  `max_output_tokens` with no message), 3000 output tokens, 25 s timeout,
  never parse the raw body (non-`completed` or missing `output_text` is a
  provider error; a message item without usable text is `bad_output`), and
  `usage.opencode.allowance` (USD float) feeds a running daily counter.
  `cmd/aismoke` spends one real call for smoke-testing (prints metadata
  only, never the key). `AI_MODE=mock` (dev only) gives deterministic
  output for `make dev` / `make e2e`.

### Frontend (`web/` → `dist/<slug>/`)

`index.html` + `app.js` + `style.css` + `privacy.html`, ES modules over
`engine.js`/`mask.js`, no inline scripts. Rules render instantly; AI is ON
by default (`bb.ai`) with a first-use 3-language notice (`bb.aiNoticeAck`),
local masking, a visible "what was sent" block, invisible Turnstile
(explicit render/execute/reset, action `check`), and a 30 s POST.
Client merge: `max(local≥30 ? local : 0, response.score)`; flags are marked
`rules` vs `AI`; every non-`ok` outcome (plus network/timeout/429/403/5xx/
offline) shows a 3-language rules-only note. Verdicts never say "safe";
the 1080×1080 share image is verdict-only. CSP is generated by
`cmd/build` (Turnstile's `unsafe-inline` style exception only when a real
sitekey is configured, with an explaining comment). Provider name and
retention note render from `ai.display_name` / `ai.privacy_note`.
