// Budol Bantay frontend (ES module, no inline scripts).
// Rules run locally via engine.js + mask.js and render INSTANTLY.
// If AI is ON, a MASKED copy is POSTed to {{API_URL}}/v1/check and the
// merged result is rendered. See privacy.html for the privacy model.
import { analyze, messages } from "./engine.js";
import { mask } from "./mask.js";

// ---- build-time config (placeholders replaced by `go run ./cmd/build`) ----
const API_URL = "{{API_URL}}";
const TURNSTILE_SITEKEY = "{{TURNSTILE_SITEKEY}}";
const APP_NAME = "{{NAME}}";
const SITE_URL = "{{SITE_URL}}";
const AI_DISPLAY_NAME = {{AI_DISPLAY_NAME_JSON}};
const AI_PRIVACY_NOTE = {
  tl: {{AI_PRIVACY_TL_JSON}},
  en: {{AI_PRIVACY_EN_JSON}},
  ja: {{AI_PRIVACY_JA_JSON}},
};
const TURNSTILE_ON = Boolean(TURNSTILE_SITEKEY) && TURNSTILE_SITEKEY !== "CHANGE-ME";
const AI_TIMEOUT_MS = 30000;

/* ===================== i18n ===================== */
const T = {
tl: {
  tagline: "I-paste ang kahina-hinalang text, Messenger, Viber o email. Titingnan namin ang mga karaniwang senyales ng scam.",
  pasteLabel: "I-paste ang mensahe dito",
  senderLabel: "Sino ang nag-send? (optional — pangalan o number na nakita mo)",
  check: "I-check", clear: "Burahin",
  aiToggle: "AI analysis",
  examples: "Mga halimbawang mensahe (EXAMPLES — gawa-gawa lang)",
  exNote: "Gawa-gawa ang lahat ng halimbawa, pati mga link at number. Huwag bisitahin ang mga link.",
  hi: "MATAAS ang panganib — malamang SCAM",
  mid: "KAHINA-HINALA — mag-ingat",
  lo: "May ilang senyales — i-verify muna",
  none: "Walang nakitang karaniwang red flag",
  hiSub: "Huwag i-click, huwag magbayad, huwag ibigay ang OTP/MPIN.",
  midSub: "Maraming scam ang ganito ang itsura. I-verify sa official na app o hotline.",
  loSub: "Hindi ibig sabihin na scam, pero mag-double check.",
  noneSub: "Hindi ito garantiya na ligtas. Bago at iba-iba ang modus ng scammers.",
  why: "Bakit?", todo: "Ano ang dapat gawin", score: "Risk score", empty: "Walang mensahe. Mag-paste muna.",
  share: "I-download bilang larawan (walang laman ng mensahe)",
  disc: "Hindi kami konektado sa CICC, NTC, BSP, GCash, Maya o anumang bangko/ahensya. Gabay lang ito, hindi legal o financial advice.",
  footer: "Non-profit project. Hindi kami konektado sa gobyerno o mga kumpanya.",
  privacyLink: "Privacy",
  termsLink: "Terms",
  resultDisc: "Hindi perpekto ang pagsusuri na ito. Kung may duda, huwag magdesisyon nang mag-isa — kumonsulta sa pamilya, kaibigan, o mga taong pinagkakatiwalaan mo.",
  landDisc: "Hindi perpekto ang pagsusuri. Kung may duda, kumonsulta sa pamilya, kaibigan o taong pinagkakatiwalaan mo bago magdesisyon.",
  noticeTitle: "Gagamitin ba ang AI?",
  noticeYes: "Sige, ituloy", noticeNo: "Huwag gamitin ang AI",
  sentTitle: "Ito ang ipinadala / Sent for AI analysis",
  sentHint: "Masked na kopya lang ang ipinadala — walang personal na detalye.",
  aiWorking: "Sinusuri ng AI…",
  aiTitle: "AI analysis",
  aiBy: "mula sa AI provider",
  finalResult: "Pinagsamang resulta (rules + AI)",
  rulesResult: "Resulta ng rules (sa phone mo)",
  extraRules: "Mga senyales mula sa rules",
  extraAI: "Mga dagdag na senyales mula sa AI",
  aiActions: "Hakbang mula sa AI",
  injection: "May utos sa loob ng mensahe na nagtatangkang lokohin ang AI (injection detected) — itinaas ang risk score.",
  maskedLabel: "Na-mask",
  fbRulesOnly: "Rules-only ang resulta sa itaas.",
  fbOfflineT: "Hindi available ang AI ngayon (offline).",
  fbTimeoutT: "Hindi available ang AI ngayon (timeout).",
  fbRateT: "Hindi available ang AI ngayon (masyadong maraming request — subukan ulit mamaya).",
  fbQuotaT: "Hindi available ang AI ngayon (ubos ang daily quota).",
  fbErrorT: "Hindi available ang AI ngayon (error).",
  fbBadT: "Hindi available ang AI ngayon (hindi maintindihan ang sagot ng AI).",
  fbUncfgT: "Hindi available ang AI ngayon (hindi naka-configure).",
  fbDeniedT: "Hindi available ang AI ngayon (tinanggihan ang request).",
  fbOffT: "Naka-OFF ang AI.",
  fbOfflineE: "AI is unavailable right now (offline).",
  fbTimeoutE: "AI is unavailable right now (timeout).",
  fbRateE: "AI is unavailable right now (too many requests — try again later).",
  fbQuotaE: "AI is unavailable right now (daily quota used up).",
  fbErrorE: "AI is unavailable right now (error).",
  fbBadE: "AI is unavailable right now (AI answer could not be understood).",
  fbUncfgE: "AI is unavailable right now (not configured).",
  fbDeniedE: "AI is unavailable right now (request denied).",
  fbOffE: "AI is OFF.",
  fbOfflineJ: "AIは現在利用できません(オフライン)。",
  fbTimeoutJ: "AIは現在利用できません(タイムアウト)。",
  fbRateJ: "AIは現在利用できません(リクエスト過多 — 後でもう一度)。",
  fbQuotaJ: "AIは現在利用できません(本日の上限に達しました)。",
  fbErrorJ: "AIは現在利用できません(エラー)。",
  fbBadJ: "AIは現在利用できません(AIの回答を解釈できませんでした)。",
  fbUncfgJ: "AIは現在利用できません(未設定)。",
  fbDeniedJ: "AIは現在利用できません(リクエストが拒否されました)。",
  fbOffJ: "AIはOFFです。",
  cnt: { phone: "telepono", email: "email", card: "card", account: "account", otp: "code", name: "pangalan", address: "address" },
},
en: {
  tagline: "Paste a suspicious SMS, Messenger, Viber or email message. We check it for common scam warning signs.",
  pasteLabel: "Paste the message here",
  senderLabel: "Who sent it? (optional — the name or number you saw)",
  check: "Check", clear: "Clear",
  aiToggle: "AI analysis",
  examples: "Example messages (EXAMPLES — made up)",
  exNote: "All examples, links and numbers are fictional. Do not visit the links.",
  hi: "HIGH risk — very likely a SCAM",
  mid: "SUSPICIOUS — be careful",
  lo: "Some warning signs — verify first",
  none: "No common red flags found",
  hiSub: "Don't click, don't pay, never share your OTP/MPIN.",
  midSub: "Many scams look like this. Verify through the official app or hotline.",
  loSub: "Not necessarily a scam, but double-check.",
  noneSub: "This is NOT a guarantee it is safe. Scam tactics change constantly.",
  why: "Why?", todo: "What to do", score: "Risk score", empty: "No message yet. Paste one first.",
  share: "Download as image (message text not included)",
  disc: "Not affiliated with CICC, NTC, BSP, GCash, Maya or any bank/agency. Guidance only, not legal or financial advice.",
  footer: "Non-profit project. Not affiliated with government or companies.",
  privacyLink: "Privacy",
  termsLink: "Terms",
  resultDisc: "This check is not perfect. If you are unsure, do not decide alone — ask family, friends, or people you trust.",
  landDisc: "This check is not perfect. If you are unsure, ask family, friends or someone you trust before you decide.",
  noticeTitle: "Use AI analysis?",
  noticeYes: "Yes, continue", noticeNo: "Don't use AI",
  sentTitle: "Ito ang ipinadala / Sent for AI analysis",
  sentHint: "Only a masked copy was sent — no personal details.",
  aiWorking: "Checking with AI…",
  aiTitle: "AI analysis",
  aiBy: "from the AI provider",
  finalResult: "Combined result (rules + AI)",
  rulesResult: "Rules result (on your phone)",
  extraRules: "Signals from the rules",
  extraAI: "Extra signals from AI",
  aiActions: "Steps from AI",
  injection: "The message contains instructions trying to trick the AI (injection detected) — the risk score was raised.",
  maskedLabel: "Masked",
  fbRulesOnly: "The result above is rules-only.",
  fbOfflineT: "Hindi available ang AI ngayon (offline).",
  fbTimeoutT: "Hindi available ang AI ngayon (timeout).",
  fbRateT: "Hindi available ang AI ngayon (masyadong maraming request — subukan ulit mamaya).",
  fbQuotaT: "Hindi available ang AI ngayon (ubos ang daily quota).",
  fbErrorT: "Hindi available ang AI ngayon (error).",
  fbBadT: "Hindi available ang AI ngayon (hindi maintindihan ang sagot ng AI).",
  fbUncfgT: "Hindi available ang AI ngayon (hindi naka-configure).",
  fbDeniedT: "Hindi available ang AI ngayon (tinanggihan ang request).",
  fbOffT: "Naka-OFF ang AI.",
  fbOfflineE: "AI is unavailable right now (offline).",
  fbTimeoutE: "AI is unavailable right now (timeout).",
  fbRateE: "AI is unavailable right now (too many requests — try again later).",
  fbQuotaE: "AI is unavailable right now (daily quota used up).",
  fbErrorE: "AI is unavailable right now (error).",
  fbBadE: "AI is unavailable right now (AI answer could not be understood).",
  fbUncfgE: "AI is unavailable right now (not configured).",
  fbDeniedE: "AI is unavailable right now (request denied).",
  fbOffE: "AI is OFF.",
  fbOfflineJ: "AIは現在利用できません(オフライン)。",
  fbTimeoutJ: "AIは現在利用できません(タイムアウト)。",
  fbRateJ: "AIは現在利用できません(リクエスト過多 — 後でもう一度)。",
  fbQuotaJ: "AIは現在利用できません(本日の上限に達しました)。",
  fbErrorJ: "AIは現在利用できません(エラー)。",
  fbBadJ: "AIは現在利用できません(AIの回答を解釈できませんでした)。",
  fbUncfgJ: "AIは現在利用できません(未設定)。",
  fbDeniedJ: "AIは現在利用できません(リクエストが拒否されました)。",
  fbOffJ: "AIはOFFです。",
  cnt: { phone: "phone", email: "email", card: "card", account: "account", otp: "code", name: "name", address: "address" },
},
ja: {
  tagline: "怪しいSMS・Messenger・Viber・メールを貼り付けると、よくある詐欺の兆候をチェックします。",
  pasteLabel: "メッセージをここに貼り付け",
  senderLabel: "送信者(任意 — 表示された名前や番号)",
  check: "チェック", clear: "消去",
  aiToggle: "AI分析",
  examples: "サンプルメッセージ(例文 — 架空)",
  exNote: "例文・リンク・番号はすべて架空です。リンクにはアクセスしないでください。",
  hi: "危険度 高 — 詐欺の可能性が非常に高い",
  mid: "要注意 — 怪しい",
  lo: "いくつか注意点あり — まず確認を",
  none: "よくある危険サインは見つかりませんでした",
  hiSub: "クリックしない・支払わない・OTP/MPINは絶対に教えない。",
  midSub: "多くの詐欺がこの形です。公式アプリや窓口で確認を。",
  loSub: "詐欺とは限りませんが、念のため確認を。",
  noneSub: "安全の保証ではありません。手口は常に変わります。",
  why: "理由", todo: "やるべきこと", score: "リスクスコア", empty: "メッセージがありません。先に貼り付けてください。",
  share: "画像として保存(本文は含まれません)",
  disc: "CICC・NTC・BSP・GCash・Maya・銀行・政府機関とは無関係です。参考情報であり法的・金融アドバイスではありません。",
  footer: "非営利プロジェクト。政府・企業とは無関係です。",
  privacyLink: "プライバシー",
  termsLink: "利用規約",
  resultDisc: "このチェックは完全ではありません。迷ったときは一人で判断せず、家族・友人・信頼できる人に相談してください。",
  landDisc: "このチェックは完全ではありません。迷ったら、決める前に家族・友人・信頼できる人に相談してください。",
  noticeTitle: "AI分析を使いますか?",
  noticeYes: "はい、続ける", noticeNo: "AIを使わない",
  sentTitle: "Ito ang ipinadala / Sent for AI analysis",
  sentHint: "マスク済みコピーのみ送信 — 個人情報は含まれません。",
  aiWorking: "AIが分析中…",
  aiTitle: "AI analysis",
  aiBy: "AIプロバイダーより",
  finalResult: "統合結果(ルール+AI)",
  rulesResult: "ルールの結果(端末内)",
  extraRules: "ルールが検出した兆候",
  extraAI: "AIが追加で検出した兆候",
  aiActions: "AIからの対策",
  injection: "メッセージ内にAIを騙そうとする指示が含まれていました(injection検出) — リスクスコアを引き上げました。",
  maskedLabel: "マスク件数",
  fbRulesOnly: "上記はルールのみの結果です。",
  fbOfflineT: "Hindi available ang AI ngayon (offline).",
  fbTimeoutT: "Hindi available ang AI ngayon (timeout).",
  fbRateT: "Hindi available ang AI ngayon (masyadong maraming request — subukan ulit mamaya).",
  fbQuotaT: "Hindi available ang AI ngayon (ubos ang daily quota).",
  fbErrorT: "Hindi available ang AI ngayon (error).",
  fbBadT: "Hindi available ang AI ngayon (hindi maintindihan ang sagot ng AI).",
  fbUncfgT: "Hindi available ang AI ngayon (hindi naka-configure).",
  fbDeniedT: "Hindi available ang AI ngayon (tinanggihan ang request).",
  fbOffT: "Naka-OFF ang AI.",
  fbOfflineE: "AI is unavailable right now (offline).",
  fbTimeoutE: "AI is unavailable right now (timeout).",
  fbRateE: "AI is unavailable right now (too many requests — try again later).",
  fbQuotaE: "AI is unavailable right now (daily quota used up).",
  fbErrorE: "AI is unavailable right now (error).",
  fbBadE: "AI is unavailable right now (AI answer could not be understood).",
  fbUncfgE: "AI is unavailable right now (not configured).",
  fbDeniedE: "AI is unavailable right now (request denied).",
  fbOffE: "AI is OFF.",
  fbOfflineJ: "AIは現在利用できません(オフライン)。",
  fbTimeoutJ: "AIは現在利用できません(タイムアウト)。",
  fbRateJ: "AIは現在利用できません(リクエスト過多 — 後でもう一度)。",
  fbQuotaJ: "AIは現在利用できません(本日の上限に達しました)。",
  fbErrorJ: "AIは現在利用できません(エラー)。",
  fbBadJ: "AIは現在利用できません(AIの回答を解釈できませんでした)。",
  fbUncfgJ: "AIは現在利用できません(未設定)。",
  fbDeniedJ: "AIは現在利用できません(リクエストが拒否されました)。",
  fbOffJ: "AIはOFFです。",
  cnt: { phone: "電話番号", email: "メール", card: "カード", account: "口座", otp: "コード", name: "名前", address: "住所" },
},
};

const SCAM_TYPE = {
  phishing_link: { tl: "phishing link", en: "phishing link", ja: "フィッシングリンク" },
  otp_theft: { tl: "pagnanakaw ng OTP", en: "OTP theft", ja: "OTP窃取" },
  fake_prize: { tl: "pekeng premyo", en: "fake prize", ja: "偽の賞品" },
  advance_fee: { tl: "advance fee", en: "advance fee", ja: "前払い金詐欺" },
  job_task: { tl: "peke/abusadong trabaho o task", en: "job/task scam", ja: "求人・タスク詐欺" },
  parcel: { tl: "parcel modus", en: "parcel scam", ja: "荷物詐欺" },
  loan: { tl: "pekeng loan", en: "fake loan", ja: "偽ローン" },
  gov_impersonation: { tl: "pagpapanggap na ahensya ng gobyerno", en: "government impersonation", ja: "政府機関なりすまし" },
  bank_ewallet_impersonation: { tl: "pagpapanggap na bangko/e-wallet", en: "bank/e-wallet impersonation", ja: "銀行・電子ウォレットなりすまし" },
  investment_crypto: { tl: "peke/abusadong investment o crypto", en: "investment/crypto scam", ja: "投資・暗号資産詐欺" },
  family_emergency: { tl: "pagpapanggap na kapamilya/emergency", en: "family/emergency impersonation", ja: "家族・緊急事態なりすまし" },
  romance: { tl: "romance scam", en: "romance scam", ja: "ロマンス詐欺" },
  other: { tl: "iba pa", en: "other", ja: "その他" },
  none: { tl: "wala", en: "none", ja: "なし" },
};

/* ===================== state ===================== */
let LANG = localStorage.getItem("bb.lang") || "tl";
if (!T[LANG]) LANG = "tl";
let AI_ON = localStorage.getItem("bb.ai") !== "0"; // default ON
let ACK = localStorage.getItem("bb.aiNoticeAck") === "1";
let RULES = null, MSG = null, LAST = null, LAST_AI = null;
const $ = (s) => document.querySelector(s);
const LI = () => LANG;
function tr(k) { return T[LANG][k]; }
function esc(s) {
  return String(s).replace(/[&<>"']/g, (m) => ({"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"}[m]));
}
function bandFor(score) {
  const lv = (RULES && RULES.levels) || { hi: 60, mid: 30, lo: 10 };
  return score >= lv.hi ? "hi" : score >= lv.mid ? "mid" : score >= lv.lo ? "lo" : "none";
}
const COL = { hi: "#c62828", mid: "#ef6c00", lo: "#b26a00", none: "#2e7d32" };
// The CSP has no style-src 'unsafe-inline', so inline style="" attributes are
// blocked. Dynamic colours/widths are carried in data-* attributes and applied
// through the CSSOM, which the CSP allows.
function paintDyn() {
  document.querySelectorAll("[data-bg]").forEach((e) => { e.style.background = e.dataset.bg; });
  document.querySelectorAll("[data-w]").forEach((e) => { e.style.width = e.dataset.w + "%"; });
}
function isLocalhost() {
  return ["localhost", "127.0.0.1", "[::1]"].includes(location.hostname);
}

/* ===================== static UI ===================== */
function privacyShortHTML() {
  // Accurate short notice; full text lives in privacy.html.
  const host = esc(API_URL.replace(/^https?:\/\//, "").split("/")[0]);
  const prov = esc(AI_DISPLAY_NAME);
  const note = AI_PRIVACY_NOTE[LANG] || AI_PRIVACY_NOTE.en;
  const body = {
    tl: `Rules check sa phone mo lang ginagawa. Kapag ON ang AI, masked na kopya ang ipinapadala sa aming server (${host} API) at ipinapasa nito sa AI provider (${prov}). Hindi kami nag-iimbak o nagla-log ng text; anonymous daily counters lang. ${esc(note)} Maaari mong i-OFF ang AI.`,
    en: `Rules check happens on your phone. When AI is ON, a masked copy is sent to our server (${host} API), which forwards it to the AI provider (${prov}). We don't store or log the text; only anonymous daily counters. ${esc(note)} You can turn AI off.`,
    ja: `ルールチェックは端末内のみ。AIがONの場合、マスク済みコピーがサーバー(${host} API)に送られ、AIプロバイダー(${prov})に転送されます。テキストは保存・記録せず、匿名の日次カウンタのみ。${esc(note)} AIはOFFにできます。`,
  };
  // Current language only (the language switch re-renders it); full text in privacy.html.
  return `<p>${body[LANG]}</p>`;
}

function applyLang() {
  document.querySelectorAll("[data-i]").forEach((e) => {
    const v = T[LANG][e.dataset.i];
    if (typeof v === "string") e.textContent = v;
  });
  document.querySelectorAll(".lang button").forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.l === LANG)));
  document.documentElement.lang = LANG === "tl" ? "fil" : LANG;
  $("#aitoggle").checked = AI_ON;
  $("#privacyshort").innerHTML = privacyShortHTML();
  $("#reportline").innerHTML = reportHTML();
  renderExamples();
  if (LAST) renderRules(LAST, true);
  if (LAST_AI) renderAIDone(LAST_AI, true);
}

/* ===================== report-a-scam line ===================== */
// Official PH channels only (verified 2026-10-29): CICC Inter-Agency Response
// Center hotline 1326 (cicc.gov.ph/report), PNP Anti-Cybercrime Group
// (acg.pnp.gov.ph), and the user's own bank / e-wallet official support.
function reportHTML() {
  const a = (u, t) => `<a href="${u}" target="_blank" rel="noopener noreferrer">${t}</a>`;
  const cicc = a("https://cicc.gov.ph/report/", "cicc.gov.ph");
  const acg = a("https://acg.pnp.gov.ph/", "acg.pnp.gov.ph");
  const body = {
    tl: `<strong>Na-scam o may natanggap na scam?</strong> Tumawag sa <strong>CICC hotline 1326</strong> (24/7, libre; ${cicc}), mag-report sa <strong>PNP Anti-Cybercrime Group</strong> (${acg}), at sa official support ng bangko/e-wallet mo (hal. GCash, Maya).`,
    en: `<strong>Scammed or received a scam?</strong> Call the <strong>CICC hotline 1326</strong> (24/7, toll-free; ${cicc}), report to the <strong>PNP Anti-Cybercrime Group</strong> (${acg}), and to your bank or e-wallet's official support (e.g. GCash, Maya).`,
    ja: `<strong>被害に遭った・詐欺を受け取った場合:</strong> <strong>CICCホットライン 1326</strong>(24時間・無料、${cicc})、<strong>PNPサイバー犯罪対策グループ</strong>(${acg})、ご自身の銀行・電子ウォレット(GCash、Mayaなど)の公式サポートへ。`,
  };
  return body[LANG] || body.en;
}

/* ===================== examples (fictional) ===================== */
const EX = [
  { s: "GCash", m: "GCash: Ang iyong account ay pansamantalang na-lock dahil sa kahina-hinalang aktibidad. I-verify ang account within 24 hours para hindi ma-deactivate: https://gcash-ph-verify.top/login" },
  { s: "LBC", m: "LBC Express: Ang parcel mo ay naka-hold dahil incomplete address. I-update at magbayad ng ₱45 redelivery fee bago mag 6PM: bit.ly/lbc-ph-upd8" },
  { s: "09XX XXX 0001", m: "Hi! Hiring kami ng part-time online staff. ₱800-₱3,000/araw, mag-like at review lang ng products. No experience needed. Message sa Telegram @hr_anna_ph" },
  { s: "", m: "CONGRATULATIONS! Nanalo ka ng ₱50,000 at Toyota Vios sa Shopee 10th Anniversary Raffle. Para ma-claim, magpadala ng ₱1,500 processing fee sa GCash 09XX-XXX-0002. DTI Permit No. 12345" },
  { s: "09XX XXX 0003", m: "Good day po, from Maya Security Team. May nag-attempt mag-login sa account mo. Para ma-cancel, paki-send ang 6-digit OTP na natanggap mo ngayon din." },
  { s: "", m: "Congrats! Approved na ang loan mo na ₱30,000, no collateral. Magbayad lang ng ₱999 insurance fee para ma-release ngayon din. Click: tinyurl.com/loanph-fast" },
  { s: "DSWD", m: "DSWD: Kasama ka sa listahan ng ₱8,000 ayuda (AICS). Mag-register bago mag Oct 31 o ma-forfeit ang benepisyo: https://dswd-ayuda.gov-ph.claim-ph.icu" },
  { s: "09XX XXX 0004", m: "Ma, si Carlo 'to, bagong number ko. Nasira phone ko. Pa-send muna ₱3,000 sa GCash 09XX-XXX-0004 emergency lang, huwag mo muna sabihin kay Papa." },
  { s: "", m: "Join our VIP investment group! Guaranteed 30% daily profit sa USDT. Minimum ₱5,000 lang. Withdraw anytime. Chat sa WhatsApp para sa slot, limited slots!" },
  { s: "", m: "Dear BDO Customer, your online banking access will be suspended. Update your account information immediately to avoid restriction: http://bdo-online-secure.com.ph-update.info/verify" },
  { s: "BANK", m: "(CONTROL / legit-style) 482913 is your one-time PIN for your transaction. Valid for 5 minutes. Never share your OTP with anyone, including bank staff." },
  { s: "", m: "(CONTROL / normal) Hi Migs, kita tayo sa SM Southmall 3pm bukas ha. Dala ka ng payong, uulan daw." },
];
function renderExamples() {
  $("#exlist").innerHTML = EX.map((e, k) =>
    `<button class="ex" data-k="${k}"><b>EXAMPLE ${k + 1}</b>${e.s ? ` · ${esc(e.s)}` : ""}<br>${esc(e.m)}</button>`
  ).join("");
  document.querySelectorAll(".ex").forEach((b) => (b.onclick = () => {
    const e = EX[+b.dataset.k];
    $("#msg").value = e.m; $("#sender").value = e.s; run();
    $("#result").scrollIntoView({ behavior: "smooth" });
  }));
}

/* ===================== rules render (instant) ===================== */
function adviceFor(cats) {
  const adv = [MSG.advice.always[LANG]];
  for (const k of ["cred", "pay", "job", "parcel", "gov", "invest", "family", "loan"]) {
    if (cats.includes(k) && MSG.advice[k]) adv.push(MSG.advice[k][LANG]);
  }
  adv.push(MSG.advice.report[LANG]);
  return adv;
}
function flagLine(h, src) {
  const f = MSG.flags[h.id];
  const txt = f ? f[LANG] : h.id;
  const tag = src ? `<span class="src ${src}">${src}</span>` : "";
  return `<li>${esc(txt)}${tag} <span class="ev">“${esc(h.ev)}”</span></li>`;
}
function renderRules(r, keepAI) {
  LAST = r;
  const adv = adviceFor(r.cats);
  $("#result").innerHTML = `<div class="card" id="rulescard"><div class="verdict" data-bg="${COL[r.level]}"><h2>${esc(tr(r.level))}</h2><p>${esc(tr(r.level + "Sub"))}</p>
  <div class="meter" title="${esc(tr("score"))} ${r.score}/100"><i data-w="${Math.max(3, r.score)}"></i></div><p>${esc(tr("score"))}: ${r.score}/100 · ${esc(tr("rulesResult"))}</p></div>
  ${r.hits.length ? `<div class="sec2"><h3>${esc(tr("why"))}</h3><ul>${r.hits.map((h) => flagLine(h, null)).join("")}</ul></div>` : ""}
  <div class="sec2"><h3>${esc(tr("todo"))}</h3><ul>${adv.map((a) => `<li>${esc(a)}</li>`).join("")}</ul></div>
  <div class="sec2"><button class="btn" id="dl">${esc(tr("share"))}</button></div>
  <div class="resultdisc" role="note">${esc(tr("resultDisc"))}</div>
  <div class="reportline">${reportHTML()}</div>
  <div class="disc">${esc(tr("disc"))}</div></div>`;
  $("#dl").onclick = () => shareImg();
  paintDyn();
  if (!keepAI) { $("#airesult").innerHTML = ""; LAST_AI = null; }
}

/* ===================== AI flow ===================== */
function countsLine(counts) {
  const names = T[LANG].cnt;
  const parts = [];
  for (const k of ["phone", "email", "card", "account", "otp", "name", "address"]) {
    if (counts[k] > 0) parts.push(`${counts[k]} ${names[k]}`);
  }
  return parts.length ? parts.join(", ") : "—";
}
function ensureAck() {
  if (ACK) return Promise.resolve(true);
  const h = location.hash.replace(/^#/, "");
  const q = new URLSearchParams(h);
  // #auto auto-acknowledges, but only when served from localhost (dev/e2e).
  if (q.has("auto") && isLocalhost()) {
    ACK = true; localStorage.setItem("bb.aiNoticeAck", "1");
    return Promise.resolve(true);
  }
  const body = $("#noticeBody");
  const order = [LANG, ...["tl", "en", "ja"].filter((l) => l !== LANG)];
  const ackText = {
    tl: `Kapag ON ang AI, masked na kopya ng mensahe ang ipinapadala sa aming server (${esc(API_URL.replace(/^https?:\/\//, "").split("/")[0])} API), at ipinapasa nito sa AI provider (${esc(AI_DISPLAY_NAME)}). ${esc(AI_PRIVACY_NOTE.tl)} Hindi kami nag-iimbak o nagla-log ng text; anonymous daily counters lang. Maaari mong i-OFF ang AI anumang oras.`,
    en: `When AI is ON, a masked copy of the message is sent to our server (${esc(API_URL.replace(/^https?:\/\//, "").split("/")[0])} API), which forwards it to the AI provider (${esc(AI_DISPLAY_NAME)}). ${esc(AI_PRIVACY_NOTE.en)} We don't store or log the text; only anonymous daily counters. You can turn AI off at any time.`,
    ja: `AIがONの場合、マスク済みコピーがサーバー(${esc(API_URL.replace(/^https?:\/\//, "").split("/")[0])} API)に送られ、AIプロバイダー(${esc(AI_DISPLAY_NAME)})に転送されます。${esc(AI_PRIVACY_NOTE.ja)} テキストは保存・記録せず、匿名の日次カウンタのみ。AIはいつでもOFFにできます。`,
  };
  body.innerHTML = order.map((l) => `<p><strong>${l.toUpperCase()}:</strong> ${ackText[l]}</p>`).join("");
  $("#notice").hidden = false;
  return new Promise((resolve) => {
    $("#noticeYes").onclick = () => {
      ACK = true; localStorage.setItem("bb.aiNoticeAck", "1");
      $("#notice").hidden = true; resolve(true);
    };
    $("#noticeNo").onclick = () => {
      AI_ON = false; localStorage.setItem("bb.ai", "0");
      $("#aitoggle").checked = false; $("#notice").hidden = true; resolve(false);
    };
  });
}

let tsPromise = null;
function loadTurnstile() {
  if (!TURNSTILE_ON) return Promise.resolve(null);
  if (tsPromise) return tsPromise;
  tsPromise = new Promise((resolve) => {
    const to = setTimeout(() => resolve(null), 8000);
    const s = document.createElement("script");
    s.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    s.async = true; s.defer = true;
    s.onload = () => { clearTimeout(to); resolve(window.turnstile || null); };
    s.onerror = () => { clearTimeout(to); resolve(null); };
    document.head.appendChild(s);
  });
  return tsPromise;
}
async function getTurnstileToken() {
  if (!TURNSTILE_ON) return "";
  try {
    const ts = await loadTurnstile();
    if (!ts) return "";
    const slot = $("#tslot");
    slot.innerHTML = "";
    const token = await Promise.race([
      new Promise((resolve, reject) => {
        let wid = null;
        try {
          wid = ts.render(slot, {
            sitekey: TURNSTILE_SITEKEY,
            callback: (tok) => { try { ts.reset(wid); } catch (e) { /* ignore */ } resolve(tok || ""); },
            "error-callback": () => reject(new Error("turnstile")),
            "expired-callback": () => resolve(""),
            size: "invisible", execution: "execute", appearance: "interaction-only", action: "check",
          });
        } catch (e) { reject(e); return; }
        try { ts.execute(wid, { action: "check" }); }
        catch (e) { try { ts.execute(wid); } catch (e2) { reject(e2); } }
      }),
      new Promise((_, reject) => setTimeout(() => reject(new Error("timeout")), 12000)),
    ]);
    try { slot.innerHTML = ""; } catch (e) { /* ignore */ }
    return token || "";
  } catch (e) { return ""; }
}

function fbKey(reason) {
  return { offline: "Offline", timeout: "Timeout", rate_limited: "Rate", quota: "Quota", error: "Error", bad_output: "Bad", unconfigured: "Uncfg", disabled: "Off", skipped: "Off", turnstile_failed: "Denied", origin_not_allowed: "Denied" }[reason] || "Error";
}
function renderFallback(reason, detail) {
  const k = fbKey(reason);
  const box = `<div class="card" id="ai-section" data-ai-status="${esc(reason)}"><div class="sec2"><h3>${esc(tr("aiTitle"))}</h3>
  <div class="note warn"><p>${esc(tr("fb" + k + "T"))}</p><p>${esc(tr("fb" + k + "E"))}</p><p>${esc(tr("fb" + k + "J"))}</p>
  <p>${esc(tr("fbRulesOnly"))}${detail ? ` <span class="ev">(${esc(detail)})</span>` : ""}</p></div></div></div>`;
  $("#airesult").innerHTML = box;
  LAST_AI = { fallback: true, reason, detail };
}
function renderAIDone(payload, keep) {
  if (keep && LAST_AI && LAST_AI.fallback) { renderFallback(LAST_AI.reason, LAST_AI.detail); return; }
  LAST_AI = payload;
  const { resp, final, band, masked, counts } = payload;
  const ai = resp.ai;
  const L = LANG;
  const stName = (SCAM_TYPE[ai.scam_type] || SCAM_TYPE.other)[L];
  const serverFlags = (resp.flags || []).map((f) => {
    const m = (f.message || {});
    return `<li>${esc(m[L] || m.en || f.id)}<span class="src rules">rules</span> <span class="ev">“${esc(f.evidence || "")}”</span></li>`;
  }).join("");
  const aiFlags = (ai.red_flags || []).map((f) => `<li>${esc(f[L] || f.en)}<span class="src ai">AI</span></li>`).join("");
  const aiActs = (ai.actions || []).map((a) => `<li>${esc(a[L] || a.en)}</li>`).join("");
  $("#airesult").innerHTML = `<div class="card" id="ai-section" data-ai-status="ok">
  <div class="verdict" data-bg="${COL[band]}"><h2>${esc(tr("aiTitle"))} · ${esc(tr(band))}</h2>
  <p>${esc(tr("finalResult"))}: ${final}/100 · ${esc(ai.verdict)} (${esc(stName)}, risk ${ai.risk}) · ${esc(tr("aiBy"))} ${esc(AI_DISPLAY_NAME)}</p>
  <div class="meter" title="${esc(tr("score"))} ${final}/100"><i data-w="${Math.max(3, final)}"></i></div></div>
  <div class="sec2"><h3>${esc(tr("sentTitle"))}</h3>
  <pre class="sent">${esc(masked)}</pre>
  <p class="counts">${esc(tr("maskedLabel"))}: ${esc(countsLine(counts))} · ${esc(tr("sentHint"))}</p></div>
  <div class="sec2"><h3>${esc(tr("extraRules"))}</h3>
  ${serverFlags ? `<ul>${serverFlags}</ul>` : `<p class="ev">—</p>`}</div>
  <div class="sec2"><h3>${esc(tr("extraAI"))}</h3>
  <p>${esc(ai.explanation[L] || ai.explanation.en)}</p>
  ${aiFlags ? `<ul>${aiFlags}</ul>` : ""}
  ${ai.injection_detected ? `<div class="note warn"><p>${esc(tr("injection"))}</p></div>` : ""}</div>
  <div class="sec2"><h3>${esc(tr("aiActions"))}</h3><ul>${aiActs}</ul></div>
  <div class="resultdisc" role="note">${esc((resp.disclaimer && resp.disclaimer[L]) || tr("resultDisc"))}</div>
  </div>`;
  paintDyn();
}

async function runAI(local, text, sender) {
  const ok = await ensureAck();
  if (!ok || !AI_ON) { renderFallback("disabled", "ai=off"); return; }
  // Mask locally, then show exactly what is sent.
  const m = mask(text);
  const ms = mask(sender || "");
  const maskedSender = ms.masked;
  $("#airesult").innerHTML = `<div class="card"><div class="sec2"><h3>${esc(tr("sentTitle"))}</h3>
  <pre class="sent">${esc(m.masked)}</pre>
  <p class="counts">${esc(tr("maskedLabel"))}: ${esc(countsLine(m.counts))} · ${esc(tr("sentHint"))}</p>
  <p><span class="spin"></span> ${esc(tr("aiWorking"))}</p></div></div>`;
  const token = await getTurnstileToken();
  const payload = {
    text: m.masked, lang: LANG, sender: maskedSender,
    client_rules: { score: local.score, band: local.level, flags: local.hits.map((h) => h.id) },
    turnstile_token: token,
  };
  let resp = null, httpStatus = 0;
  try {
    if (navigator.onLine === false) { renderFallback("offline", "navigator.onLine"); return; }
    const ctrl = new AbortController();
    const to = setTimeout(() => ctrl.abort(), AI_TIMEOUT_MS);
    let res;
    try {
      res = await fetch(API_URL + "/v1/check", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload), signal: ctrl.signal, referrerPolicy: "no-referrer",
      });
    } finally { clearTimeout(to); }
    httpStatus = res.status;
    if (!res.ok) {
      if (res.status === 429) renderFallback("rate_limited", "http 429");
      else if (res.status === 403) renderFallback("turnstile_failed", "http 403");
      else if (res.status >= 500) renderFallback("error", "http " + res.status);
      else renderFallback("error", "http " + res.status);
      return;
    }
    resp = await res.json();
  } catch (e) {
    if (e && e.name === "AbortError") renderFallback("timeout", "30s");
    else if (navigator.onLine === false) renderFallback("offline", "network");
    else renderFallback("error", "network");
    return;
  }
  if (!resp || resp.ai_status !== "ok" || !resp.ai) {
    const st = (resp && resp.ai_status) || "error";
    const map = { quota: "quota", rate_limited: "rate_limited", timeout: "timeout", bad_output: "bad_output", unconfigured: "unconfigured", disabled: "disabled", skipped: "disabled", error: "error" };
    renderFallback(map[st] || "error", "ai_status=" + st);
    return;
  }
  // Client merge: server already merged with its own masked-text rules;
  // the local raw-text rules may be higher (e.g. name/phone signals).
  const base = local.score >= 30 ? local.score : 0;
  const final = Math.max(base, resp.score);
  renderAIDone({ resp, final, band: bandFor(final), masked: resp.masked_text || m.masked, counts: resp.masked_counts || m.counts });
}

/* ===================== main flow ===================== */
async function run() {
  if (!RULES || !MSG) return;
  const text = $("#msg").value.trim();
  if (!text) {
    $("#result").innerHTML = `<p class="ev">${esc(tr("empty"))}</p>`;
    $("#airesult").innerHTML = ""; LAST = null; LAST_AI = null; return;
  }
  const sender = $("#sender").value;
  const local = analyze(text, sender, RULES);
  renderRules(local, false);
  if (AI_ON) {
    try { await runAI(local, text, sender); }
    catch (e) { renderFallback("error", "exception"); }
  } else {
    renderFallback("disabled", "ai=off");
  }
}

function shareImg() {
  if (!LAST) return;
  const r = LAST;
  const fin = (LAST_AI && !LAST_AI.fallback) ? LAST_AI.final : r.score;
  const band = bandFor(fin);
  const W = 1080, H = 1080, cv = document.createElement("canvas");
  cv.width = W; cv.height = H;
  const x = cv.getContext("2d");
  x.fillStyle = "#fff"; x.fillRect(0, 0, W, H);
  x.fillStyle = COL[band]; x.fillRect(0, 0, W, 300);
  x.fillStyle = "#fff"; x.font = "bold 40px system-ui,sans-serif";
  x.fillText("🛡️ " + APP_NAME, 60, 80);
  x.font = "bold 54px system-ui,sans-serif";
  wrap(x, tr(band), 60, 170, W - 120, 64);
  x.font = "34px system-ui,sans-serif";
  x.fillText(`${tr("score")}: ${fin}/100`, 60, 278);
  x.fillStyle = "#1b1f24"; x.font = "bold 38px system-ui,sans-serif";
  let y = 380; x.fillText(tr("why"), 60, y); y += 56;
  x.font = "32px system-ui,sans-serif";
  for (const h of r.hits.slice(0, 5)) { y = wrap(x, "• " + ((MSG.flags[h.id] || {})[LANG] || h.id), 60, y, W - 120, 42) + 8; if (y > 860) break; }
  x.fillStyle = "#0b57d0"; x.font = "bold 32px system-ui,sans-serif";
  x.fillText("CICC Hotline 1326", 60, 960);
  x.fillStyle = "#5b6570"; x.font = "26px system-ui,sans-serif";
  let host = SITE_URL.replace(/^https?:\/\//, "").replace(/\/$/, "");
  x.fillText(`${host} · not affiliated with any agency`, 60, 1010);
  cv.toBlob((b) => {
    const a = document.createElement("a");
    a.href = URL.createObjectURL(b); a.download = "budol-bantay-result.png"; a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 5000);
  }, "image/png");
}
function wrap(ctx, text, x, y, maxW, lh) {
  let line = "";
  const words = LANG === "ja" ? [...text] : text.split(" ");
  const sep = LANG === "ja" ? "" : " ";
  for (const w of words) {
    const test = line ? line + sep + w : w;
    if (ctx.measureText(test).width > maxW && line) { ctx.fillText(line, x, y); y += lh; line = w; }
    else line = test;
  }
  ctx.fillText(line, x, y);
  return y + lh;
}

/* ===================== boot ===================== */
async function boot() {
  $("#go").onclick = run;
  $("#clr").onclick = () => { $("#msg").value = ""; $("#sender").value = ""; $("#result").innerHTML = ""; $("#airesult").innerHTML = ""; LAST = null; LAST_AI = null; };
  document.querySelectorAll(".lang button").forEach((b) => (b.onclick = () => {
    LANG = b.dataset.l; localStorage.setItem("bb.lang", LANG); applyLang();
  }));
  $("#aitoggle").onchange = (e) => {
    AI_ON = e.target.checked;
    localStorage.setItem("bb.ai", AI_ON ? "1" : "0");
  };
  const [rulesRes] = await Promise.all([fetch("rules.json", { referrerPolicy: "no-referrer" })]);
  if (!rulesRes.ok) {
    $("#result").innerHTML = `<div class="note err"><p>rules.json HTTP ${rulesRes.status} — ${esc(tr("fbErrorE"))}</p></div>`;
    return;
  }
  RULES = await rulesRes.json();
  MSG = messages(RULES);
  applyLang();
  // Dev hooks: #demo=N preloads and runs example N; #auto auto-acks the AI
  // notice, only when served from localhost (headless e2e).
  try {
    const q = new URLSearchParams(location.hash.replace(/^#/, ""));
    const n = parseInt(q.get("demo") || "", 10);
    if (n >= 1 && n <= EX.length) {
      $("#msg").value = EX[n - 1].m; $("#sender").value = EX[n - 1].s;
      await run();
    }
  } catch (e) { /* ignore bad hashes */ }
}
boot();
