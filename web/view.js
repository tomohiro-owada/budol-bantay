// Pure view helpers (no DOM) so they can be unit-tested with node --test.

// A result is "safe" (no scam signals found) only in the lowest band. Scam
// advice ("Huwag i-click", "I-report: CICC") must never be shown for it.
export function isSafeBand(band) {
  return band === "none";
}

// Neutral guidance for the safe band. Deliberately never says "100% safe".
export const SAFE_HEAD = {
  tl: "Mukhang ligtas, pero mag-ingat pa rin",
  en: "Looks OK, but stay careful",
  ja: "問題は見つかりませんでしたが、念のため注意してください",
};
export const SAFE_ADVICE = {
  tl: [
    "Walang nakitang karaniwang senyales ng scam, pero hindi ito garantiya — may mga bagong paraan ang scammers.",
    "Kung may humihingi ng pera, OTP/MPIN o password, huminto muna at i-verify sa official app o hotline na ikaw mismo ang nagbukas.",
    "Kung may duda, magtanong muna sa pamilya o sa taong pinagkakatiwalaan mo.",
  ],
  en: [
    "No common scam signals were found, but this is not a guarantee — scammers keep inventing new tricks.",
    "If anyone asks for money, an OTP/MPIN or a password, stop and verify through the official app or hotline that you open yourself.",
    "If you are unsure, ask family or someone you trust first.",
  ],
  ja: [
    "よくある詐欺の兆候は見つかりませんでしたが、安全の保証ではありません。手口は常に新しくなります。",
    "お金・OTP/MPIN・パスワードを求められたら一旦止まり、自分で開いた公式アプリや窓口で確認してください。",
    "迷ったら、まず家族や信頼できる人に相談してください。",
  ],
};

// What to show under "what to do": neutral text for the safe band, otherwise
// the AI steps (when present) or the rules-based scam advice.
export function guidance({ band, lang, aiActions, rulesAdvice }) {
  if (isSafeBand(band)) {
    return { safe: true, head: SAFE_HEAD[lang] || SAFE_HEAD.en, items: SAFE_ADVICE[lang] || SAFE_ADVICE.en };
  }
  const items = aiActions && aiActions.length ? aiActions : rulesAdvice;
  return { safe: false, head: null, items };
}

// Compact "report a scam" line (safe results) and the full one live in app.js.
export const REPORT_COMPACT = {
  tl: "Kung may natanggap kang scam: <b>CICC 1326</b> · {acg} · official support ng bangko/e-wallet mo.",
  en: "If you received a scam: <b>CICC 1326</b> · {acg} · your bank/e-wallet's official support.",
  ja: "詐欺を受け取った場合: <b>CICC 1326</b> · {acg} · 銀行・電子ウォレットの公式サポート。",
};
