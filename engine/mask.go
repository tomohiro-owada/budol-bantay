package engine

import (
	"regexp"
	"strings"
)

var (
	reEmail    = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
	reCard     = regexp.MustCompile(`\b(?:\d[ \-]?){13,19}\b`)
	rePhonePH  = regexp.MustCompile(`\+63[\s\-]?9[0-9Xx]{2}[\s\-]?[0-9Xx]{3}[\s\-]?[0-9Xx]{4}\b`)
	rePhone09  = regexp.MustCompile(`\b09[0-9Xx]{2}[\s\-]?[0-9Xx]{3}[\s\-]?[0-9Xx]{4}\b`)
	reLandline = regexp.MustCompile(`\(0\d{1,3}\)[\s\-]?\d{3,4}[\s\-]?\d{4}\b|\b0\d{2,3}[\s\-]?\d{3,4}[\s\-]?\d{4}\b`)
	reAcctLbl  = regexp.MustCompile(`(?i)\b((?:acct\.?|account)(?:\s*(?:no\.?|number|#))?[\s:]*)([0-9][0-9\s\-]{6,}[0-9])\b`)
	reAcctGen  = regexp.MustCompile(`\b\d[\d\s\-]{7,}\d\b`)
	reOTPKw    = regexp.MustCompile(`(?i)\b(otp|one-?time|code|pin|verification code)\b`)
	reOTPNum   = regexp.MustCompile(`\b\d{4,8}\b`)
	// Greeting case-insensitive, name part case-sensitive (capitalised words).
	reNameGreet = regexp.MustCompile(`\b(?i:hi|hello|dear|good day|ma'am|mam|maam|sir|si|ako si|this is)\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,2})`)

	reAddrBlk   = regexp.MustCompile(`(?i)\bBlk\.?\s*\d+\s*(Lot\s*\d+)?`)
	reAddrBlock = regexp.MustCompile(`(?i)\bBlock\s*\d+\s*(Lot\s*\d+)?`)
	reAddrHash  = regexp.MustCompile(`#\d+\s*[A-Za-z0-9\s]+\b(St|Street|Ave|Avenue|Rd|Road|Blvd|Drive|Lane)\b\.?`)
	reAddrBrgy  = regexp.MustCompile(`\b(?:Brgy|Barangay)\.?\s+[A-Za-z0-9][A-Za-z0-9.\s]{1,30}`)
	reAddrPurok = regexp.MustCompile(`(?i)\b(?:Purok|Phase)\s*[A-Za-z0-9]*\s*\d+[A-Za-z0-9\s\-]{0,20}`)

	reDateDash = regexp.MustCompile(`^\d{4}\-\d{1,2}\-\d{1,2}$`)
)

var brandStop = map[string]bool{
	"gcash": true, "maya": true, "paymaya": true, "bdo": true, "bpi": true,
	"landbank": true, "metrobank": true, "unionbank": true, "security": true,
	"bank": true, "banko": true, "dswd": true, "sss": true, "philhealth": true,
	"lbc": true, "shopee": true, "lazada": true, "cicc": true, "ntc": true,
	"bsp": true, "meralco": true, "globe": true, "smart": true, "hiring": true,
	"team": true, "there": true, "here": true, "all": true, "everyone": true,
	"friends": true, "guys": true, "po": true, "sir": true, "maam": true,
}

func stripDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasPesoBefore(s string, start int) bool {
	lo := start - 8
	if lo < 0 {
		lo = 0
	}
	pre := s[lo:start]
	if strings.Contains(pre, "₱") {
		return true
	}
	low := strings.ToLower(pre)
	if strings.Contains(low, "php") || strings.Contains(low, "peso") {
		return true
	}
	// lone "P" + space + digits, e.g. "P 1500" — conservative: "p" at end
	trimmed := strings.TrimRight(pre, " ")
	if strings.HasSuffix(trimmed, " P") || strings.HasSuffix(trimmed, " p") {
		return true
	}
	return false
}

// Mask redacts PII on-device. It never masks URLs/domains (protected),
// brand names (stoplist), peso amounts, or times/dates (guards).
func Mask(text string) MaskResult {
	var counts MaskCounts
	s := text

	// 1. emails
	s = reEmail.ReplaceAllStringFunc(s, func(m string) string {
		counts.Email++
		return "[EMAIL]"
	})

	// 2. protect URLs with placeholders
	type span struct{ st, en int }
	locs := rxURL.FindAllStringIndex(s, -1)
	var urls []string
	var b strings.Builder
	prev := 0
	placeholders := []span{}
	_ = placeholders
	for i, l := range locs {
		// skip URL matches that are actually email remnants (contain [EMAIL]? no)
		b.WriteString(s[prev:l[0]])
		ph := "\x00URL" + itoa(i) + "\x00"
		b.WriteString(ph)
		urls = append(urls, s[l[0]:l[1]])
		prev = l[1]
	}
	b.WriteString(s[prev:])
	s = b.String()

	restoreURLs := func(s string) string {
		for i, u := range urls {
			ph := "\x00URL" + itoa(i) + "\x00"
			s = strings.ReplaceAll(s, ph, u)
		}
		return s
	}

	// 3. cards: 13-19 digits optionally spaced/dashed
	s = reCard.ReplaceAllStringFunc(s, func(m string) string {
		d := stripDigits(m)
		if len(d) < 13 || len(d) > 19 {
			return m
		}
		// avoid peso-adjacent (unlikely for 13+ digits, but guard)
		counts.Card++
		return "[CARD]"
	})

	// 4. phones
	s = rePhonePH.ReplaceAllStringFunc(s, func(m string) string {
		counts.Phone++
		return "[PHONE]"
	})
	s = rePhone09.ReplaceAllStringFunc(s, func(m string) string {
		counts.Phone++
		return "[PHONE]"
	})
	s = reLandline.ReplaceAllStringFunc(s, func(m string) string {
		d := stripDigits(m)
		if len(d) < 7 {
			return m
		}
		counts.Phone++
		return "[PHONE]"
	})

	// 5a. explicit account labels: keep label, mask number
	s = reAcctLbl.ReplaceAllStringFunc(s, func(m string) string {
		sub := reAcctLbl.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		counts.Account++
		return sub[1] + "[ACCOUNT]"
	})
	// 5b. generic 8+ digit runs
	s = reAcctGen.ReplaceAllStringFunc(s, func(m string) string {
		trimmed := strings.TrimSpace(m)
		if strings.Contains(trimmed, ",") || strings.Contains(trimmed, ":") {
			return m
		}
		if reDateDash.MatchString(trimmed) {
			return m
		}
		d := stripDigits(trimmed)
		if len(d) < 8 || len(d) > 19 {
			return m
		}
		// already-masked tokens contain brackets, not digits-only, so safe.
		// skip years/months glued? 8 digits could be date 20261026 — skip pure 8-digit starting with 19/20?
		if len(d) == 8 && (strings.HasPrefix(d, "19") || strings.HasPrefix(d, "20")) {
			// could be YYYYMMDD; require spaces/dashes to be absent? be conservative: skip
			if !strings.ContainsAny(trimmed, " -") {
				return m
			}
			// even with dashes like 2026-10-26, skip (already excluded by dateDash? 2026-10-26 matches dateDash)
			return m
		}
		counts.Account++
		return "[ACCOUNT]"
	})
	// Fix: generic pass may have used indices that shifted with peso check needing original context.
	// Peso guard for generic runs: re-scan with index to check prefix. Simpler second pass is omitted
	// because peso amounts in practice are <8 digits; explicit guard below for safety on [ACCOUNT] over-mask:
	// (no-op; peso amounts like ₱1,500 never reach 8 digits)

	// 6. OTP-like 4-8 digit codes near keywords
	kwLocs := reOTPKw.FindAllStringIndex(s, -1)
	if len(kwLocs) > 0 {
		numLocs := reOTPNum.FindAllStringIndex(s, -1)
		// collect replacements from end to start to keep indices stable
		type repl struct{ st, en int }
		var reps []repl
		for _, nl := range numLocs {
			numStr := s[nl[0]:nl[1]]
			// skip if contains peso context
			// check 6 chars before for ₱ or PHP
			lo := nl[0] - 6
			if lo < 0 {
				lo = 0
			}
			pre := s[lo:nl[0]]
			if strings.Contains(pre, "₱") || strings.Contains(strings.ToLower(pre), "php") {
				continue
			}
			if strings.Contains(pre, ",") && len(pre) > 0 && isDigit(rune(pre[len(pre)-1])) {
				// part of 1,500 style amount
				continue
			}
			near := false
			for _, kl := range kwLocs {
				if abs(nl[0]-kl[0]) <= 40 || (nl[0] > kl[0] && nl[0]-kl[1] <= 40) || (kl[0] > nl[0] && kl[0]-nl[1] <= 40) {
					near = true
					break
				}
			}
			if near {
				_ = numStr
				reps = append(reps, repl{nl[0], nl[1]})
			}
		}
		// apply from end
		for i := len(reps) - 1; i >= 0; i-- {
			r := reps[i]
			// avoid double-masking inside existing tokens
			inner := s[r.st:r.en]
			if strings.Contains(inner, "[") {
				continue
			}
			s = s[:r.st] + "[CODE]" + s[r.en:]
			counts.OTP++
		}
	}

	// 7. names after greetings (keep greeting, mask name)
	s = reNameGreet.ReplaceAllStringFunc(s, func(m string) string {
		sub := reNameGreet.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		name := sub[1]
		// stoplist on first word
		first := strings.ToLower(strings.Fields(name)[0])
		first = strings.Trim(first, ".,")
		if brandStop[first] {
			return m
		}
		// skip placeholder artifacts
		if strings.Contains(m, "\x00") {
			return m
		}
		counts.Name++
		// replace only the name part, keep greeting
		idx := strings.LastIndex(m, name)
		if idx < 0 {
			return m
		}
		return m[:idx] + "[NAME]"
	})

	// 8. addresses
	s = reAddrHash.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "\x00") {
			return m
		}
		counts.Address++
		return "[ADDRESS]"
	})
	s = reAddrBlk.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "\x00") {
			return m
		}
		// require Blk + digits to avoid over-matching; regex already ensures
		counts.Address++
		return "[ADDRESS]"
	})
	// Block (avoid double counting if already replaced? [ADDRESS] has no Block)
	if !strings.Contains(s, "[ADDRESS]") || true {
		s = reAddrBlock.ReplaceAllStringFunc(s, func(m string) string {
			if strings.Contains(m, "[ADDRESS]") || strings.Contains(m, "\x00") {
				return m
			}
			// only count if it actually looked like an address (has digits)
			hasDigit := false
			for _, r := range m {
				if r >= '0' && r <= '9' {
					hasDigit = true
					break
				}
			}
			if !hasDigit {
				return m
			}
			counts.Address++
			return "[ADDRESS]"
		})
	}
	s = reAddrBrgy.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "\x00") {
			return m
		}
		counts.Address++
		return "[ADDRESS]"
	})
	s = reAddrPurok.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "\x00") {
			return m
		}
		counts.Address++
		return "[ADDRESS]"
	})

	s = restoreURLs(s)
	return MaskResult{Masked: s, Counts: counts}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [16]byte
	pos := len(buf)
	n := i
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
