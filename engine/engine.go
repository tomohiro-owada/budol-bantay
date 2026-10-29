// Package engine is the reference rules engine. All regex patterns and
// rule data live in engine/rules.json (single source of truth); this file
// only interprets them. The browser engine in web/engine.js implements the
// same semantics over the same JSON.
package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed rules.json
var rulesJSON []byte

// LangText holds tl/en/ja strings.
type LangText struct {
	Tl string `json:"tl"`
	En string `json:"en"`
	Ja string `json:"ja"`
}

// PatternEntry is one named helper pattern in rules.json.
type PatternEntry struct {
	Src   string `json:"src"`
	Flags string `json:"flags"`
}

// RuleEntry mirrors one entry in rules.json.
type RuleEntry struct {
	ID        string `json:"id"`
	W         int    `json:"w"`
	Cat       string `json:"cat"`
	Tl        string `json:"tl"`
	En        string `json:"en"`
	Ja        string `json:"ja"`
	Kind      string `json:"kind"`
	Pattern   string `json:"pattern,omitempty"`
	Flags     string `json:"flags,omitempty"`
	EvPattern string `json:"evPattern,omitempty"`
	EvFlags   string `json:"evFlags,omitempty"`
}

// Combo is a declarative score boost.
type Combo struct {
	IfAll []string `json:"ifAll"`
	IfAny []string `json:"ifAny"`
	Add   int      `json:"add"`
}

// RulesFile is the on-disk shape of rules.json.
type RulesFile struct {
	Shorteners     []string                `json:"shorteners"`
	RiskyTLDs      []string                `json:"riskyTlds"`
	AllowedTLDs    []string                `json:"allowedTlds"`
	Official       map[string][]string     `json:"official"`
	Rules          []RuleEntry             `json:"rules"`
	Advice         map[string]LangText     `json:"advice"`
	Patterns       map[string]PatternEntry `json:"patterns"`
	Combos         []Combo                 `json:"combos"`
	Levels         map[string]int          `json:"levels"`
	LinkDropStrong []string                `json:"linkDropStrong"`
	OtpMitigator   struct {
		SuppressIf      []string `json:"suppressIf"`
		RequiresNoURLs  bool     `json:"requiresNoUrls"`
	} `json:"otpMitigator"`
}

// Hit is one fired rule.
type Hit struct {
	ID  string `json:"id"`
	W   int    `json:"w"`
	Cat string `json:"cat"`
	Ev  string `json:"ev"`
}

// URLInfo is an extracted URL.
type URLInfo struct {
	Raw  string `json:"raw"`
	Host string `json:"host"`
	Reg  string `json:"reg"`
}

// Result is the Analyze output.
type Result struct {
	Score int       `json:"score"`
	Level string    `json:"level"`
	Hits  []Hit     `json:"hits"`
	Cats  []string  `json:"cats"`
	URLs  []URLInfo `json:"urls"`
}

// MaskCounts counts each masked class.
type MaskCounts struct {
	Phone   int `json:"phone"`
	Email   int `json:"email"`
	Card    int `json:"card"`
	Account int `json:"account"`
	OTP     int `json:"otp"`
	Name    int `json:"name"`
	Address int `json:"address"`
}

// MaskResult is the Mask output.
type MaskResult struct {
	Masked string     `json:"masked"`
	Counts MaskCounts `json:"counts"`
}

// Loaded rules data.
var (
	Loaded      RulesFile
	shortSet    map[string]bool
	riskySet    map[string]bool
	allowedTLDs map[string]bool
	officialMap map[string][]string
	ruleByID    map[string]RuleEntry

	rxURL            *regexp.Regexp
	rxNotURL         *regexp.Regexp
	rxTwoLevel       *regexp.Regexp
	rxBrandEwallet   *regexp.Regexp
	rxBrandBank      *regexp.Regexp
	rxBrandGov       *regexp.Regexp
	rxBrandCourier   *regexp.Regexp
	rxHasPrize       *regexp.Regexp
	rxHasLoan        *regexp.Regexp
	rxHasJob         *regexp.Regexp
	rxCredA          *regexp.Regexp
	rxCredB          *regexp.Regexp
	rxNeg            *regexp.Regexp
	rxNeg2           *regexp.Regexp
	rxStart          *regexp.Regexp
	rxGovPh          *regexp.Regexp
	rxNonAln         *regexp.Regexp
	rxRawIP          *regexp.Regexp
	rxSenderMobile   *regexp.Regexp
	rxStripZW        *regexp.Regexp
	rxRule           map[string]*regexp.Regexp
	rxRuleEv         map[string]*regexp.Regexp
	levelHi, levelMid, levelLo int
)

func compilePat(src, flags string) (*regexp.Regexp, error) {
	if err := checkCommonSubset(src); err != nil {
		return nil, err
	}
	for _, c := range flags {
		if c != 'i' {
			return nil, fmt.Errorf("unsupported flag %q (only 'i' allowed)", string(c))
		}
	}
	pat := jsUnicodeToRE2(src)
	if strings.Contains(flags, "i") {
		pat = "(?i)" + pat
	}
	return regexp.Compile(pat)
}

func mustCompilePat(src, flags string) *regexp.Regexp {
	rx, err := compilePat(src, flags)
	if err != nil {
		panic("engine: bad pattern " + src + ": " + err.Error())
	}
	return rx
}

// checkCommonSubset rejects constructs that work in only one of RE2/JS:
// lookahead/lookbehind, backreferences, named groups, possessive quantifiers.
func checkCommonSubset(src string) error {
	for _, f := range []string{"(?=", "(?!", "(?<", "(?P"} {
		if strings.Contains(src, f) {
			return fmt.Errorf("pattern uses forbidden construct %q (must stay in RE2∩JS subset)", f)
		}
	}
	if backrefRE.MatchString(src) {
		return fmt.Errorf("pattern uses backreference (must stay in RE2∩JS subset)")
	}
	if possessiveRE.MatchString(src) {
		return fmt.Errorf("pattern uses possessive quantifier (must stay in RE2∩JS subset)")
	}
	return nil
}

var (
	backrefRE    = regexp.MustCompile(`\\[1-9]`)
	possessiveRE = regexp.MustCompile(`(\+\+|\*\+|\?\+|\{\d+(,\d*)?\}\+)`)
	// JS-style \uXXXX escapes are translated to RE2 \x{XXXX} in compilePat.
	jsUnicodeRE = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
)

func jsUnicodeToRE2(src string) string {
	return jsUnicodeRE.ReplaceAllString(src, `\\x{$1}`)
}

func init() {
	if err := json.Unmarshal(rulesJSON, &Loaded); err != nil {
		panic("engine: invalid rules.json: " + err.Error())
	}
	if err := validateLoaded(); err != nil {
		panic("engine: invalid rules.json: " + err.Error())
	}
	shortSet = map[string]bool{}
	for _, s := range Loaded.Shorteners {
		shortSet[strings.ToLower(s)] = true
	}
	riskySet = map[string]bool{}
	for _, s := range Loaded.RiskyTLDs {
		riskySet[strings.ToLower(s)] = true
	}
	allowedTLDs = map[string]bool{}
	for _, s := range Loaded.AllowedTLDs {
		allowedTLDs[strings.ToLower(s)] = true
	}
	officialMap = Loaded.Official
	ruleByID = map[string]RuleEntry{}
	for _, r := range Loaded.Rules {
		ruleByID[r.ID] = r
	}
	get := func(name string) PatternEntry {
		p, ok := Loaded.Patterns[name]
		if !ok {
			panic("engine: rules.json missing patterns." + name)
		}
		return p
	}
	rxURL = mustCompilePat(get("url").Src, get("url").Flags)
	rxNotURL = mustCompilePat(get("notUrl").Src, get("notUrl").Flags)
	rxTwoLevel = mustCompilePat(get("twoLevel").Src, get("twoLevel").Flags)
	rxBrandEwallet = mustCompilePat(get("brandEwallet").Src, get("brandEwallet").Flags)
	rxBrandBank = mustCompilePat(get("brandBank").Src, get("brandBank").Flags)
	rxBrandGov = mustCompilePat(get("brandGov").Src, get("brandGov").Flags)
	rxBrandCourier = mustCompilePat(get("brandCourier").Src, get("brandCourier").Flags)
	rxHasPrize = mustCompilePat(get("hasPrize").Src, get("hasPrize").Flags)
	rxHasLoan = mustCompilePat(get("hasLoan").Src, get("hasLoan").Flags)
	rxHasJob = mustCompilePat(get("hasJob").Src, get("hasJob").Flags)
	rxCredA = mustCompilePat(get("credA").Src, get("credA").Flags)
	rxCredB = mustCompilePat(get("credB").Src, get("credB").Flags)
	rxNeg = mustCompilePat(get("neg").Src, get("neg").Flags)
	rxNeg2 = mustCompilePat(get("neg2").Src, get("neg2").Flags)
	rxStart = mustCompilePat(get("start").Src, get("start").Flags)
	rxGovPh = mustCompilePat(get("govPh").Src, get("govPh").Flags)
	rxNonAln = mustCompilePat(get("nonAlnum").Src, get("nonAlnum").Flags)
	rxRawIP = mustCompilePat(get("rawIp").Src, get("rawIp").Flags)
	rxSenderMobile = mustCompilePat(get("senderMobile").Src, get("senderMobile").Flags)
	rxStripZW = mustCompilePat(get("stripZw").Src, get("stripZw").Flags)

	rxRule = map[string]*regexp.Regexp{}
	rxRuleEv = map[string]*regexp.Regexp{}
	for _, r := range Loaded.Rules {
		if r.Pattern != "" {
			rxRule[r.ID] = mustCompilePat(r.Pattern, r.Flags)
		}
		if r.EvPattern != "" {
			rxRuleEv[r.ID] = mustCompilePat(r.EvPattern, r.EvFlags)
		}
	}
	levelHi = Loaded.Levels["hi"]
	levelMid = Loaded.Levels["mid"]
	levelLo = Loaded.Levels["lo"]
}

func validateLoaded() error {
	seen := map[string]bool{}
	for _, r := range Loaded.Rules {
		if r.ID == "" {
			return errStr("empty rule id")
		}
		if seen[r.ID] {
			return errStr("duplicate rule id " + r.ID)
		}
		seen[r.ID] = true
		if r.Tl == "" || r.En == "" || r.Ja == "" {
			return errStr("rule " + r.ID + " missing tl/en/ja")
		}
		if r.ID == "otpNotice" {
			if r.W > 0 {
				return errStr("otpNotice weight must be <= 0")
			}
		} else {
			if r.W < 1 || r.W > 60 {
				return errStr(fmt.Sprintf("rule %q weight %d out of range 1..60", r.ID, r.W))
			}
		}
		if r.Cat == "" {
			return errStr("rule " + r.ID + " missing cat")
		}
		if r.Kind == "" {
			return errStr("rule " + r.ID + " missing kind")
		}
		switch r.Kind {
		case "shortener", "lookalike", "riskytld", "rawip", "brandlink", "link",
			"credential", "feeToClaim", "parcel", "ayuda", "senderMobile", "otpNotice", "regex":
		default:
			return errStr("rule " + r.ID + " unknown kind " + r.Kind)
		}
		if r.Pattern != "" {
			if _, err := compilePat(r.Pattern, r.Flags); err != nil {
				return errStr("rule " + r.ID + " bad pattern: " + err.Error())
			}
		}
		if r.EvPattern != "" {
			if _, err := compilePat(r.EvPattern, r.EvFlags); err != nil {
				return errStr("rule " + r.ID + " bad evPattern: " + err.Error())
			}
		}
		// kinds that require a test pattern
		switch r.Kind {
		case "regex", "feeToClaim", "parcel", "ayuda", "otpNotice":
			if r.Pattern == "" {
				return errStr("rule " + r.ID + " kind " + r.Kind + " needs pattern")
			}
		}
	}
	if len(Loaded.Shorteners) == 0 {
		return errStr("shorteners empty")
	}
	if len(Loaded.RiskyTLDs) == 0 {
		return errStr("riskyTlds empty")
	}
	if len(Loaded.AllowedTLDs) == 0 {
		return errStr("allowedTlds empty")
	}
	if len(Loaded.Official) == 0 {
		return errStr("official empty")
	}
	for name, p := range Loaded.Patterns {
		if p.Src == "" {
			return errStr("patterns." + name + " empty")
		}
		if _, err := compilePat(p.Src, p.Flags); err != nil {
			return errStr("patterns." + name + " bad: " + err.Error())
		}
	}
	for _, need := range []string{"url", "notUrl", "twoLevel", "brandEwallet", "brandBank", "brandGov", "brandCourier", "hasPrize", "hasLoan", "hasJob", "credA", "credB", "neg", "neg2", "start", "govPh", "nonAlnum", "rawIp", "senderMobile", "stripZw"} {
		if _, ok := Loaded.Patterns[need]; !ok {
			return errStr("patterns." + need + " missing")
		}
	}
	if Loaded.Levels["hi"] == 0 || Loaded.Levels["mid"] == 0 || Loaded.Levels["lo"] == 0 {
		return errStr("levels hi/mid/lo must be set")
	}
	if !(Loaded.Levels["hi"] > Loaded.Levels["mid"] && Loaded.Levels["mid"] > Loaded.Levels["lo"]) {
		return errStr("levels must satisfy hi > mid > lo")
	}
	for _, c := range Loaded.Combos {
		if len(c.IfAll) == 0 {
			return errStr("combo with empty ifAll")
		}
		for _, id := range append(append([]string{}, c.IfAll...), c.IfAny...) {
			if _, ok := seen[id]; !ok {
				return errStr("combo references unknown rule " + id)
			}
		}
	}
	return nil
}

// Validate returns an error if rules.json is invalid (used by tests and cmd/validate).
func Validate() error {
	return validateLoaded()
}

type errStr string

func (e errStr) Error() string { return string(e) }

// ---------- URL handling ----------

func regDomain(h string) string {
	h = strings.ToLower(h)
	h = strings.TrimPrefix(h, "www.")
	parts := strings.Split(h, ".")
	if rxTwoLevel.MatchString(h) && len(parts) >= 3 {
		return strings.Join(parts[len(parts)-3:], ".")
	}
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return h
}

func extractURLs(t string) []URLInfo {
	var out []URLInfo
	for _, m := range rxURL.FindAllString(t, -1) {
		s := strings.TrimRight(m, ".,;:!?)]")
		if rxNotURL.MatchString(s) {
			continue
		}
		h := s
		lh := strings.ToLower(h)
		if strings.HasPrefix(lh, "http://") {
			h = h[len("http://"):]
		} else if strings.HasPrefix(lh, "https://") {
			h = h[len("https://"):]
		}
		h = strings.Split(h, "/")[0]
		h = strings.Split(h, ":")[0]
		h = strings.Split(h, "?")[0]
		h = strings.Split(h, "#")[0]
		h = strings.ToLower(h)
		if h == "" {
			continue
		}
		// raw-IP hosts pass through; otherwise check TLD allowlist.
		if !rxRawIP.MatchString(h) {
			labels := strings.Split(h, ".")
			tld := strings.ToLower(labels[len(labels)-1])
			if _, ok := allowedTLDs[tld]; !ok {
				if !strings.HasPrefix(tld, "xn--") {
					continue
				}
			}
		}
		out = append(out, URLInfo{Raw: s, Host: h, Reg: regDomain(h)})
	}
	return out
}

type ctx struct {
	t         string
	sender    string
	urls      []URLInfo
	hasPrize  bool
	hasLoan   bool
	hasJob    bool
	hasParcel bool
}

func first60HasBrand(t string) bool {
	head := t
	if len([]rune(head)) > 60 {
		head = string([]rune(t)[:60])
	}
	return rxBrandEwallet.MatchString(head) || rxBrandBank.MatchString(head) || rxBrandGov.MatchString(head) || rxBrandCourier.MatchString(head)
}

func credTest(c *ctx) string {
	// Combine both alternatives in order of appearance: scan with both and pick earliest.
	locA := rxCredA.FindAllStringIndex(c.t, -1)
	locB := rxCredB.FindAllStringIndex(c.t, -1)
	type loc struct {
		s, e int
	}
	var all []loc
	for _, l := range locA {
		all = append(all, loc{l[0], l[1]})
	}
	for _, l := range locB {
		all = append(all, loc{l[0], l[1]})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].s < all[j].s })
	for _, l := range all {
		m0 := c.t[l.s:l.e]
		preStart := l.s - 25
		if preStart < 0 {
			preStart = 0
		}
		pre := c.t[preStart:l.s]
		firstWord := strings.Fields(m0)
		fw := ""
		if len(firstWord) > 0 {
			fw = firstWord[0]
		}
		if rxNeg.MatchString(pre + fw + " ") {
			continue
		}
		if rxStart.MatchString(m0) && rxNeg2.MatchString(pre) {
			continue
		}
		ev := m0
		if len(ev) > 60 {
			ev = ev[:60]
		}
		return ev
	}
	return ""
}

func matchEv(re *regexp.Regexp, t string) string {
	m := re.FindString(t)
	if m == "" {
		return ""
	}
	if len(m) > 60 {
		m = m[:60]
	}
	return m
}

// Analyze scores text + optional sender.
func Analyze(text, sender string) Result {
	t := rxStripZW.ReplaceAllString(text, "")
	c := &ctx{t: t, sender: strings.TrimSpace(sender), urls: extractURLs(t)}
	c.hasPrize = rxHasPrize.MatchString(t)
	c.hasLoan = rxHasLoan.MatchString(t)
	c.hasJob = rxHasJob.MatchString(t)
	c.hasParcel = rxBrandCourier.MatchString(t)

	weights := map[string]int{}
	cats := map[string]string{}
	for _, r := range Loaded.Rules {
		weights[r.ID] = r.W
		cats[r.ID] = r.Cat
	}
	w := func(id string) int { return weights[id] }
	cat := func(id string) string { return cats[id] }

	var hits []Hit
	add := func(id, ev string) {
		if ev == "" {
			return
		}
		hits = append(hits, Hit{ID: id, W: w(id), Cat: cat(id), Ev: ev})
	}

	// link rules
	for _, u := range c.urls {
		_ = u
		break
	}
	// shortener
	for _, u := range c.urls {
		if shortSet[strings.ToLower(u.Reg)] || shortSet[strings.ToLower(u.Host)] {
			add("shortener", u.Host)
			break
		}
	}
	// lookalike
	add("lookalike", lookalikeTest(c))
	// riskytld
	for _, u := range c.urls {
		parts := strings.Split(u.Host, ".")
		if len(parts) > 0 && riskySet[strings.ToLower(parts[len(parts)-1])] {
			add("riskytld", u.Host)
			break
		}
	}
	// rawip
	for _, u := range c.urls {
		if rxRawIP.MatchString(u.Host) || strings.Contains(u.Host, "xn--") {
			add("rawip", u.Host)
			break
		}
	}
	// brandlink
	if len(c.urls) > 0 && (rxBrandEwallet.MatchString(c.t) || rxBrandBank.MatchString(c.t)) {
		add("brandlink", c.urls[0].Host)
	}
	// link
	if len(c.urls) > 0 {
		add("link", c.urls[0].Host)
	}

	add("credential", credTest(c))
	add("verifyacct", matchEv(rxRule["verifyacct"], c.t))
	add("threat", matchEv(rxRule["threat"], c.t))
	add("urgency", matchEv(rxRule["urgency"], c.t))
	add("prize", matchEv(rxRule["prize"], c.t))
	// feeToClaim conditional
	if c.hasPrize || c.hasLoan || c.hasJob || c.hasParcel {
		if rxRule["feeToClaim"].MatchString(c.t) {
			ev := ""
			if rxEv, ok := rxRuleEv["feeToClaim"]; ok {
				ev = rxEv.FindString(c.t)
			}
			if ev == "" {
				ev = "fee"
			}
			add("feeToClaim", ev)
		}
	}
	add("sendMoney", matchEv(rxRule["sendMoney"], c.t))
	add("personalAcct", matchEv(rxRule["personalAcct"], c.t))
	add("job", matchEv(rxRule["job"], c.t))
	add("highpay", matchEv(rxRule["highpay"], c.t))
	add("task", matchEv(rxRule["task"], c.t))
	add("offplatform", matchEv(rxRule["offplatform"], c.t))
	if c.hasParcel && rxRule["parcel"].MatchString(c.t) {
		add("parcel", "parcel/hold")
	}
	add("loan", matchEv(rxRule["loan"], c.t))
	if rxBrandGov.MatchString(c.t) && rxRule["ayuda"].MatchString(c.t) {
		ev := rxBrandGov.FindString(c.t)
		if ev == "" {
			ev = "ayuda"
		}
		add("ayuda", ev)
	}
	add("invest", matchEv(rxRule["invest"], c.t))
	add("family", matchEv(rxRule["family"], c.t))
	add("secrecy", matchEv(rxRule["secrecy"], c.t))
	add("remote", matchEv(rxRule["remote"], c.t))
	add("qr", matchEv(rxRule["qr"], c.t))
	add("replyYes", matchEv(rxRule["replyYes"], c.t))
	// senderMobile
	if c.sender != "" {
		norm := strings.ReplaceAll(strings.ReplaceAll(c.sender, " ", ""), "-", "")
		if rxSenderMobile.MatchString(norm) && first60HasBrand(c.t) {
			add("senderMobile", c.sender)
		}
	}
	add("fakePermit", matchEv(rxRule["fakePermit"], c.t))

	// drop plain link when a stronger link rule fired
	hasStrong := false
	for _, h := range hits {
		for _, s := range Loaded.LinkDropStrong {
			if h.ID == s {
				hasStrong = true
				break
			}
		}
		if hasStrong {
			break
		}
	}
	score := 0
	for _, h := range hits {
		score += h.W
	}
	if hasStrong {
		for i, h := range hits {
			if h.ID == "link" {
				score -= h.W
				hits = append(hits[:i], hits[i+1:]...)
				break
			}
		}
	}

	// otpNotice mitigator
	hasCredLike := false
	for _, h := range hits {
		for _, s := range Loaded.OtpMitigator.SuppressIf {
			if h.ID == s {
				hasCredLike = true
				break
			}
		}
		if hasCredLike {
			break
		}
	}
	noURLs := len(c.urls) == 0
	if Loaded.OtpMitigator.RequiresNoURLs && !noURLs {
		// requires no URLs: skip
	} else if !hasCredLike && rxRule["otpNotice"].MatchString(c.t) && noURLs {
		add("otpNotice", "never share")
		score += w("otpNotice")
	}

	has := func(id string) bool {
		for _, h := range hits {
			if h.ID == id {
				return true
			}
		}
		return false
	}
	for _, cb := range Loaded.Combos {
		okAll := true
		for _, id := range cb.IfAll {
			if !has(id) {
				okAll = false
				break
			}
		}
		if !okAll {
			continue
		}
		if len(cb.IfAny) > 0 {
			okAny := false
			for _, id := range cb.IfAny {
				if has(id) {
					okAny = true
					break
				}
			}
			if !okAny {
				continue
			}
		}
		score += cb.Add
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	level := "none"
	switch {
	case score >= levelHi:
		level = "hi"
	case score >= levelMid:
		level = "mid"
	case score >= levelLo:
		level = "lo"
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].W > hits[j].W })
	catSet := map[string]bool{}
	var catList []string
	for _, h := range hits {
		if !catSet[h.Cat] {
			catSet[h.Cat] = true
			catList = append(catList, h.Cat)
		}
	}
	if len(c.urls) == 0 {
		c.urls = nil
	}
	if len(hits) == 0 {
		// keep hits non-nil? return empty slice for stable JSON
		hits = []Hit{}
	}
	if len(catList) == 0 {
		catList = []string{}
	}
	urls := c.urls
	if len(urls) == 0 {
		urls = []URLInfo{}
	}
	return Result{Score: score, Level: level, Hits: hits, Cats: catList, URLs: urls}
}

func lookalikeTest(c *ctx) string {
	for _, u := range c.urls {
		stripped := rxNonAln.ReplaceAllString(strings.ToLower(u.Host), "")
		for k, allow := range officialMap {
			kk := k
			var re *regexp.Regexp
			if k == "jnt" {
				re = regexp.MustCompile(`(?i)j-?n?t`)
			} else {
				re = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(kk))
			}
			if re.MatchString(stripped) {
				allowed := false
				for _, a := range allow {
					if strings.ToLower(u.Reg) == strings.ToLower(a) {
						allowed = true
						break
					}
				}
				if !allowed {
					return u.Host
				}
			}
		}
		if rxGovPh.MatchString(u.Host) && !strings.HasSuffix(strings.ToLower(u.Host), ".gov.ph") {
			return u.Host
		}
	}
	return ""
}

// Messages returns the i18n JSON (flags + advice) for the frontend.
func Messages() string {
	type flag struct {
		Tl string `json:"tl"`
		En string `json:"en"`
		Ja string `json:"ja"`
	}
	flags := map[string]flag{}
	for _, r := range Loaded.Rules {
		flags[r.ID] = flag{Tl: r.Tl, En: r.En, Ja: r.Ja}
	}
	out := map[string]interface{}{
		"flags":  flags,
		"advice": Loaded.Advice,
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// RulesJSON returns the raw embedded rules.json.
func RulesJSON() string { return string(rulesJSON) }
