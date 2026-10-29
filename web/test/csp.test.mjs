// The page CSP has no style-src 'unsafe-inline' (prod, Turnstile off), so
// inline style="" attributes would be silently blocked (this once removed the
// verdict colours). Dynamic styles must go through data-* + CSSOM (paintDyn).
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

for (const f of ["app.js", "index.html", "terms.html", "privacy.html"]) {
  test(`no inline style attribute in ${f}`, () => {
    const src = readFileSync(new URL(`../${f}`, import.meta.url), "utf8");
    assert.ok(!/\sstyle\s*=\s*["'`]/.test(src), `${f} contains an inline style attribute`);
  });
}
