// web/mask.js — on-device PII masking. ES module, no dependencies.
// Same semantics as the Go reference (engine/mask.go).
// Returns { masked, counts: { phone, email, card, account, otp, name, address } }.
// Never masks URLs/domains, brand names, peso amounts, or times/dates.

const URL_RE = /\b((?:https?:\/\/)?(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:[a-z]{2,24}|xn--[a-z0-9-]+)(?::\d+)?(?:\/[^\s<>"']*)?)|\bhttps?:\/\/\d{1,3}(?:\.\d{1,3}){3}[^\s]*/gi;

const BRAND_STOP = new Set([
  "gcash", "maya", "paymaya", "bdo", "bpi",
  "landbank", "metrobank", "unionbank", "security",
  "bank", "banko", "dswd", "sss", "philhealth",
  "lbc", "shopee", "lazada", "cicc", "ntc",
  "bsp", "meralco", "globe", "smart", "hiring",
  "team", "there", "here", "all", "everyone",
  "friends", "guys", "po", "sir", "maam",
]);

function stripDigits(s) {
  return s.replace(/\D/g, "");
}

export function mask(text) {
  const counts = { phone: 0, email: 0, card: 0, account: 0, otp: 0, name: 0, address: 0 };
  let s = String(text ?? "");

  // 1. emails
  s = s.replace(/\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b/g, () => {
    counts.email++;
    return "[EMAIL]";
  });

  // 2. protect URLs with placeholders
  const urls = [];
  s = s.replace(URL_RE, (m) => {
    const ph = "\x00URL" + urls.length + "\x00";
    urls.push(m);
    return ph;
  });
  // reset lastIndex (replace with /g/ advances it)
  URL_RE.lastIndex = 0;

  const restoreURLs = (str) => {
    for (let i = 0; i < urls.length; i++) {
      str = str.split("\x00URL" + i + "\x00").join(urls[i]);
    }
    return str;
  };

  // 3. cards: 13-19 digits optionally spaced/dashed
  s = s.replace(/\b(?:\d[ \-]?){13,19}\b/g, (m) => {
    const d = stripDigits(m);
    if (d.length < 13 || d.length > 19) return m;
    counts.card++;
    return "[CARD]";
  });

  // 4. phones
  s = s.replace(/\+63[\s\-]?9[0-9Xx]{2}[\s\-]?[0-9Xx]{3}[\s\-]?[0-9Xx]{4}\b/g, () => {
    counts.phone++;
    return "[PHONE]";
  });
  s = s.replace(/\b09[0-9Xx]{2}[\s\-]?[0-9Xx]{3}[\s\-]?[0-9Xx]{4}\b/g, () => {
    counts.phone++;
    return "[PHONE]";
  });
  s = s.replace(/\(0\d{1,3}\)[\s\-]?\d{3,4}[\s\-]?\d{4}\b|\b0\d{2,3}[\s\-]?\d{3,4}[\s\-]?\d{4}\b/g, (m) => {
    const d = stripDigits(m);
    if (d.length < 7) return m;
    counts.phone++;
    return "[PHONE]";
  });

  // 5a. explicit account labels: keep label, mask number
  s = s.replace(/\b((?:acct\.?|account)(?:\s*(?:no\.?|number|#))?[\s:]*)([0-9][0-9\s\-]{6,}[0-9])\b/gi, (m, label) => {
    counts.account++;
    return label + "[ACCOUNT]";
  });

  // 5b. generic 8+ digit runs
  s = s.replace(/\b\d[\d\s\-]{7,}\d\b/g, (m) => {
    const trimmed = m.trim();
    if (trimmed.includes(",") || trimmed.includes(":")) return m;
    if (/^\d{4}\-\d{1,2}\-\d{1,2}$/.test(trimmed)) return m;
    const d = stripDigits(trimmed);
    if (d.length < 8 || d.length > 19) return m;
    if (d.length === 8 && (d.startsWith("19") || d.startsWith("20"))) return m;
    counts.account++;
    return "[ACCOUNT]";
  });

  // 6. OTP-like 4-8 digit codes near keywords
  {
    const kwRe = /\b(otp|one-?time|code|pin|verification code)\b/gi;
    const numRe = /\b\d{4,8}\b/g;
    const kwLocs = [...s.matchAll(kwRe)].map((m) => ({ s: m.index, e: m.index + m[0].length }));
    if (kwLocs.length > 0) {
      const numLocs = [...s.matchAll(numRe)].map((m) => ({ s: m.index, e: m.index + m[0].length }));
      const reps = [];
      for (const nl of numLocs) {
        const lo = Math.max(0, nl.s - 6);
        const pre = s.slice(lo, nl.s);
        if (pre.includes("₱") || pre.toLowerCase().includes("php")) continue;
        if (pre.includes(",") && pre.length > 0 && /[0-9]/.test(pre[pre.length - 1])) continue;
        let near = false;
        for (const kl of kwLocs) {
          if (Math.abs(nl.s - kl.s) <= 40 || (nl.s > kl.s && nl.s - kl.e <= 40) || (kl.s > nl.s && kl.s - nl.e <= 40)) {
            near = true;
            break;
          }
        }
        if (near) reps.push(nl);
      }
      // apply from end to keep indices stable
      reps.sort((a, b) => b.s - a.s);
      for (const r of reps) {
        const inner = s.slice(r.s, r.e);
        if (inner.includes("[")) continue;
        s = s.slice(0, r.s) + "[CODE]" + s.slice(r.e);
        counts.otp++;
      }
    }
  }

  // 7. names after greetings (greeting case-insensitive, name case-sensitive)
  s = s.replace(/\b(hi|hello|dear|good day|ma'am|mam|maam|sir|si|ako si|this is)\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,2})/gi, (m, _greet, name, _off, _str) => {
    if (!name || !/^[A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,2}$/.test(name)) return m;
    const first = name.split(/\s+/)[0].toLowerCase().replace(/^[.,]+|[.,]+$/g, "");
    if (BRAND_STOP.has(first)) return m;
    if (m.includes("\x00")) return m;
    counts.name++;
    const idx = m.lastIndexOf(name);
    if (idx < 0) return m;
    return m.slice(0, idx) + "[NAME]";
  });

  // 8. addresses
  s = s.replace(/#\d+\s*[A-Za-z0-9\s]+\b(St|Street|Ave|Avenue|Rd|Road|Blvd|Drive|Lane)\b\.?/g, (m) => {
    if (m.includes("\x00")) return m;
    counts.address++;
    return "[ADDRESS]";
  });
  s = s.replace(/\bBlk\.?\s*\d+\s*(Lot\s*\d+)?/gi, (m) => {
    if (m.includes("\x00")) return m;
    counts.address++;
    return "[ADDRESS]";
  });
  s = s.replace(/\bBlock\s*\d+\s*(Lot\s*\d+)?/gi, (m) => {
    if (m.includes("[ADDRESS]") || m.includes("\x00")) return m;
    if (!/[0-9]/.test(m)) return m;
    counts.address++;
    return "[ADDRESS]";
  });
  s = s.replace(/\b(?:Brgy|Barangay)\.?\s+[A-Za-z0-9][A-Za-z0-9.\s]{1,30}/g, (m) => {
    if (m.includes("\x00")) return m;
    counts.address++;
    return "[ADDRESS]";
  });
  s = s.replace(/\b(?:Purok|Phase)\s*[A-Za-z0-9]*\s*\d+[A-Za-z0-9\s\-]{0,20}/gi, (m) => {
    if (m.includes("\x00")) return m;
    counts.address++;
    return "[ADDRESS]";
  });

  s = restoreURLs(s);
  return { masked: s, counts };
}
