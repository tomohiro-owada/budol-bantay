// web/engine.js — browser/Node ES-module interpreter of engine/rules.json.
// Same semantics as the Go reference engine (engine/engine.go).
// Usage: import { analyze } from '../engine.js'; analyze(text, sender, rules)

function rx(src, flags) {
  return new RegExp(src, flags);
}

function escapeRegExp(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function regDomain(h, twoLevelRe) {
  h = h.toLowerCase().replace(/^www\./, "");
  const p = h.split(".");
  if (twoLevelRe.test(h) && p.length >= 3) return p.slice(-3).join(".");
  if (p.length >= 2) return p.slice(-2).join(".");
  return h;
}

function extractUrls(t, rules, P) {
  const out = [];
  const urlRe = rx(P.url.src, "gi");
  const notUrlRe = rx(P.notUrl.src, P.notUrl.flags.includes("i") ? "i" : "");
  const twoLevelRe = rx(P.twoLevel.src, P.twoLevel.flags.includes("i") ? "i" : "");
  const rawIpRe = rx(P.rawIp.src, P.rawIp.flags.includes("i") ? "i" : "");
  const allowed = new Set((rules.allowedTlds || []).map((s) => s.toLowerCase()));
  for (const m of t.matchAll(urlRe)) {
    let s = m[0].replace(/[.,;:!?)]+$/, "");
    if (notUrlRe.test(s)) continue;
    let h = s.replace(/^https?:\/\//i, "").split(/[/:?#]/)[0].toLowerCase();
    if (!h) continue;
    if (!rawIpRe.test(h)) {
      const tld = h.split(".").pop().toLowerCase();
      if (!allowed.has(tld) && !tld.startsWith("xn--")) continue;
    }
    out.push({ raw: s, host: h, reg: regDomain(h, twoLevelRe) });
  }
  return out;
}

function first60HasBrand(t, P) {
  const head = [...t].slice(0, 60).join("");
  const ew = rx(P.brandEwallet.src, "i");
  const bk = rx(P.brandBank.src, "i");
  const gv = rx(P.brandGov.src, "i");
  const co = rx(P.brandCourier.src, "i");
  return ew.test(head) || bk.test(head) || gv.test(head) || co.test(head);
}

function matchEv(re, t) {
  const m = t.match(re);
  if (!m) return "";
  return String(m[0]).slice(0, 60);
}

function credTest(t, P) {
  const reA = rx(P.credA.src, "gi");
  const reB = rx(P.credB.src, "gi");
  const reNeg = rx(P.neg.src, "i");
  const reNeg2 = rx(P.neg2.src, "i");
  const reStart = rx(P.start.src, "i");
  const all = [];
  for (const m of t.matchAll(reA)) all.push({ s: m.index, e: m.index + m[0].length, m0: m[0] });
  for (const m of t.matchAll(reB)) all.push({ s: m.index, e: m.index + m[0].length, m0: m[0] });
  all.sort((a, b) => a.s - b.s);
  for (const l of all) {
    const pre = t.slice(Math.max(0, l.s - 25), l.s);
    const fw = (l.m0.trim().split(/\s+/)[0] || "");
    if (reNeg.test(pre + fw + " ")) continue;
    // fresh regexes (no /g/) to avoid lastIndex issues — already non-global
    if (reStart.test(l.m0) && reNeg2.test(pre)) continue;
    return String(l.m0).slice(0, 60);
  }
  return "";
}

function lookalikeTest(urls, rules, P) {
  const nonAln = rx(P.nonAlnum.src, "gi");
  const govPh = rx(P.govPh.src, "i");
  for (const u of urls) {
    const stripped = u.host.toLowerCase().replace(nonAln, "");
    for (const k of Object.keys(rules.official || {})) {
      const pat = k === "jnt" ? "j-?n?t" : escapeRegExp(k);
      if (rx(pat, "i").test(stripped)) {
        const allow = rules.official[k] || [];
        const ok = allow.some((a) => String(a).toLowerCase() === String(u.reg).toLowerCase());
        if (!ok) return u.host;
      }
    }
    if (govPh.test(u.host) && !u.host.toLowerCase().endsWith(".gov.ph")) return u.host;
  }
  return "";
}

export function analyze(text, sender, rules) {
  const P = rules.patterns;
  const t = String(text ?? "").replace(/[\u200b\u200c\u200d\ufeff]/g, "");
  const snd = String(sender ?? "").trim();
  const urls = extractUrls(t, rules, P);

  const hasPrize = rx(P.hasPrize.src, "i").test(t);
  const hasLoan = rx(P.hasLoan.src, "i").test(t);
  const hasJob = rx(P.hasJob.src, "i").test(t);
  const hasParcel = rx(P.brandCourier.src, "i").test(t);

  const weights = {};
  const cats = {};
  for (const r of rules.rules) {
    weights[r.id] = r.w;
    cats[r.id] = r.cat;
  }
  const ruleById = {};
  for (const r of rules.rules) ruleById[r.id] = r;

  const hits = [];
  const add = (id, ev) => {
    if (!ev) return;
    hits.push({ id, w: weights[id], cat: cats[id], ev: String(ev).slice(0, 60) });
  };

  const shortSet = new Set((rules.shorteners || []).map((s) => s.toLowerCase()));
  const riskySet = new Set((rules.riskyTlds || []).map((s) => s.toLowerCase()));

  const uShort = urls.find((u) => shortSet.has(u.reg.toLowerCase()) || shortSet.has(u.host.toLowerCase()));
  if (uShort) add("shortener", uShort.host);

  add("lookalike", lookalikeTest(urls, rules, P));

  const uRisky = urls.find((u) => riskySet.has(u.host.split(".").pop().toLowerCase()));
  if (uRisky) add("riskytld", uRisky.host);

  const rawIpRe = rx(P.rawIp.src, P.rawIp.flags.includes("i") ? "i" : "");
  const uRaw = urls.find((u) => rawIpRe.test(u.host) || u.host.includes("xn--"));
  if (uRaw) add("rawip", uRaw.host);

  if (urls.length > 0 && (rx(P.brandEwallet.src, "i").test(t) || rx(P.brandBank.src, "i").test(t))) {
    add("brandlink", urls[0].host);
  }
  if (urls.length > 0) add("link", urls[0].host);

  add("credential", credTest(t, P));

  const reFor = (id) => {
    const r = ruleById[id];
    if (!r || !r.pattern) return null;
    return rx(r.pattern, (r.flags || "i").includes("i") ? "i" : "");
  };

  add("verifyacct", matchEv(reFor("verifyacct"), t));
  add("threat", matchEv(reFor("threat"), t));
  add("urgency", matchEv(reFor("urgency"), t));
  add("prize", matchEv(reFor("prize"), t));

  if (hasPrize || hasLoan || hasJob || hasParcel) {
    const feeRe = reFor("feeToClaim");
    if (feeRe && feeRe.test(t)) {
      const r = ruleById["feeToClaim"];
      let ev = "";
      if (r.evPattern) {
        const evRe = rx(r.evPattern, (r.evFlags || "i").includes("i") ? "i" : "");
        ev = (t.match(evRe) || [""])[0];
      }
      add("feeToClaim", ev || "fee");
    }
  }

  add("sendMoney", matchEv(reFor("sendMoney"), t));
  add("personalAcct", matchEv(reFor("personalAcct"), t));
  add("job", matchEv(reFor("job"), t));
  add("highpay", matchEv(reFor("highpay"), t));
  add("task", matchEv(reFor("task"), t));
  add("offplatform", matchEv(reFor("offplatform"), t));

  if (hasParcel && reFor("parcel") && reFor("parcel").test(t)) add("parcel", "parcel/hold");

  add("loan", matchEv(reFor("loan"), t));

  if (rx(P.brandGov.src, "i").test(t) && reFor("ayuda") && reFor("ayuda").test(t)) {
    add("ayuda", (t.match(rx(P.brandGov.src, "i")) || [""])[0] || "ayuda");
  }

  add("invest", matchEv(reFor("invest"), t));
  add("family", matchEv(reFor("family"), t));
  add("secrecy", matchEv(reFor("secrecy"), t));
  add("remote", matchEv(reFor("remote"), t));
  add("qr", matchEv(reFor("qr"), t));
  add("replyYes", matchEv(reFor("replyYes"), t));

  if (snd) {
    const norm = snd.replace(/[\s-]/g, "");
    if (rx(P.senderMobile.src, "i").test(norm) && first60HasBrand(t, P)) add("senderMobile", snd);
  }

  add("fakePermit", matchEv(reFor("fakePermit"), t));

  // drop plain link when a stronger link rule fired
  const strong = new Set(rules.linkDropStrong || []);
  let score = hits.reduce((s, h) => s + h.w, 0);
  if (hits.some((h) => strong.has(h.id))) {
    const i = hits.findIndex((h) => h.id === "link");
    if (i >= 0) {
      score -= hits[i].w;
      hits.splice(i, 1);
    }
  }

  // otpNotice mitigator
  const mit = rules.otpMitigator || { suppressIf: [], requiresNoUrls: true };
  const suppress = new Set(mit.suppressIf || []);
  const hasCredLike = hits.some((h) => suppress.has(h.id));
  const otpRe = reFor("otpNotice");
  if (!(mit.requiresNoUrls && urls.length > 0) && !hasCredLike && otpRe && otpRe.test(t) && urls.length === 0) {
    add("otpNotice", "never share");
    score += weights["otpNotice"] || 0;
  }

  const has = (id) => hits.some((h) => h.id === id);
  for (const cb of rules.combos || []) {
    if (!(cb.ifAll || []).every((id) => has(id))) continue;
    if ((cb.ifAny || []).length > 0 && !(cb.ifAny || []).some((id) => has(id))) continue;
    score += cb.add || 0;
  }

  score = Math.max(0, Math.min(100, score));
  const lv = rules.levels || { hi: 60, mid: 30, lo: 10 };
  const level = score >= lv.hi ? "hi" : score >= lv.mid ? "mid" : score >= lv.lo ? "lo" : "none";

  hits.sort((a, b) => b.w - a.w);
  const catsOut = [...new Set(hits.map((h) => h.cat))];

  return { score, level, hits, cats: catsOut, urls };
}

export function messages(rules) {
  const flags = {};
  for (const r of rules.rules) flags[r.id] = { tl: r.tl, en: r.en, ja: r.ja };
  return { flags, advice: rules.advice };
}
