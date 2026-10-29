// Package api: deterministic mock LLM for local dev ONLY, ported from
// worker/src/mock-ai.ts.
//
// Used only when AI_MODE=mock, which is refused unless BUDOL_ENV=dev, so a
// production misconfiguration can't silently serve mock answers.
package api

import (
	"encoding/json"
	"errors"
	"strings"
)

// MockModelID identifies mock output in the model field.
const MockModelID = "mock"

// IsDevOrigin reports whether an Origin header is a local dev origin.
func IsDevOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	lower := strings.ToLower(origin)
	if strings.HasPrefix(lower, "http://") {
		lower = lower[len("http://"):]
	} else if strings.HasPrefix(lower, "https://") {
		lower = lower[len("https://"):]
	} else {
		return false
	}
	host := lower
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	if host == "localhost" || host == "127.0.0.1" || host == "[::1]" {
		return true
	}
	return strings.HasSuffix(host, ".localhost")
}

var mockInjectionREs = []string{
	"ignore", "disregard", "system prompt", "jailbreak",
}

func tri(tl, en, ja string) Lang3 { return Lang3{Tl: tl, En: en, Ja: ja} }

func mockInjection(masked string) bool {
	lower := strings.ToLower(masked)
	for _, k := range mockInjectionREs {
		if strings.Contains(lower, k) {
			return true
		}
	}
	if strings.Contains(lower, "rate") && strings.Contains(lower, "safe") {
		return true
	}
	if strings.Contains(lower, "output") && strings.Contains(lower, "only") && strings.Contains(lower, "true") {
		return true
	}
	return false
}

// RunMockAI returns deterministic schema-valid JSON based on keywords and
// rules flags. It returns invalid JSON when the text contains __MOCK_BAD__
// and an error when it contains __MOCK_FAIL__.
func RunMockAI(maskedText string, rules RulesHints) (string, error) {
	if strings.Contains(maskedText, "__MOCK_FAIL__") {
		return "", errors.New("mock ai failure")
	}
	if strings.Contains(maskedText, "__MOCK_BAD__") {
		return "this is not valid json {{{", nil
	}
	lower := strings.ToLower(maskedText)
	injection := mockInjection(maskedText)

	var out AIOutput
	switch {
	case strings.Contains(lower, "prize") || strings.Contains(lower, "winner") ||
		strings.Contains(lower, "congrat") || strings.Contains(lower, "nanalo") ||
		strings.Contains(lower, "premyo"):
		out = AIOutput{
			Verdict: "likely_scam", Risk: 75, ScamType: "fake_prize",
			RedFlags: []Lang3{tri(
				"May pekeng premyo daw na kailangang i-claim agad.",
				"Claims a prize you must claim immediately.",
				"すぐに受け取る必要がある賞金を主張しています。",
			)},
			Explanation: tri(
				"Hindi ka mananalo sa raffle na hindi mo sinalihan. Hingian ka nito ng fee o info, kaya likely scam ito.",
				"You cannot win a raffle you never joined. It will ask for a fee or personal info, so this is likely a scam.",
				"参加していない懸賞には当選しません。手数料や個人情報を求められるため、詐欺の可能性が高いです。",
			),
			Actions: []Lang3{tri(
				"Huwag mag-click o magbayad; i-report sa CICC 1326.",
				"Do not click or pay; report to CICC 1326.",
				"クリックや支払いをせず、CICC 1326 に報告してください。",
			)},
			InjectionDetected: injection,
		}
	case (strings.Contains(lower, "otp") || strings.Contains(lower, "one-time") ||
		strings.Contains(lower, "pin") || strings.Contains(lower, "verification code")) &&
		(strings.Contains(lower, "link") || strings.Contains(lower, "click") ||
			strings.Contains(lower, "login") || strings.Contains(lower, "verify") ||
			strings.Contains(lower, "pindot")):
		out = AIOutput{
			Verdict: "likely_scam", Risk: 85, ScamType: "otp_theft",
			RedFlags: []Lang3{tri(
				"Hinihingi ang OTP at pinapa-click pa ng link.",
				"Asks for your OTP and pushes a link to click.",
				"OTP を聞き出し、リンクのクリックを促しています。",
			)},
			Explanation: tri(
				"Hindi hihingi ng OTP ang GCash, Maya o bangko sa link. Gusto nilang makuha ang account mo, kaya likely scam ito.",
				"GCash, Maya or banks never ask for OTPs via a link. They want to take over your account, so this is likely a scam.",
				"GCash、Maya、銀行がリンク経由で OTP を求めることはありません。アカウント乗っ取りが目的のため、詐欺の可能性が高いです。",
			),
			Actions: []Lang3{tri(
				"Huwag ibigay ang OTP; gamitin lang ang official app.",
				"Never share the OTP; use only the official app.",
				"OTP を共有せず、公式アプリのみを使用してください。",
			)},
			InjectionDetected: injection,
		}
	case strings.Contains(lower, "task") || strings.Contains(lower, "recharge") ||
		strings.Contains(lower, "part-time") || strings.Contains(lower, "job") ||
		strings.Contains(lower, "trabaho") || strings.Contains(lower, "kita"):
		out = AIOutput{
			Verdict: "likely_scam", Risk: 70, ScamType: "job_task",
			RedFlags: []Lang3{tri(
				"Task scam: kikita daw sa simpleng tasks o recharge.",
				"Task scam: earnings promised for simple tasks or recharges.",
				"タスク詐欺: 簡単なタスクやチャージで稼げると約束しています。",
			)},
			Explanation: tri(
				"Ang task/job scam ay nagpapadala ng maliit na kita sa umpisa tapos hihingi ng recharge. Huwag magpadala ng pera.",
				"Task scams pay a little at first, then ask for recharges. Never send money.",
				"タスク詐欺は最初に少額を支払い、後でチャージを求めます。送金しないでください。",
			),
			Actions: []Lang3{tri(
				"I-block at i-report sa CICC 1326.",
				"Block and report to CICC 1326.",
				"ブロックして CICC 1326 に報告してください。",
			)},
			InjectionDetected: injection,
		}
	case strings.Contains(lower, "http") || strings.Contains(lower, "www.") ||
		strings.Contains(lower, ".com") || strings.Contains(lower, "link") ||
		strings.Contains(lower, "click") || strings.Contains(lower, "login") ||
		strings.Contains(lower, "verify") || strings.Contains(lower, "account"):
		if strings.Contains(lower, "suspend") || strings.Contains(lower, "account") {
			// fall through to link case below
		}
		out = AIOutput{
			Verdict: "suspicious", Risk: 55, ScamType: "phishing_link",
			RedFlags: []Lang3{tri(
				"May kahina-hinalang link na kailangang i-click.",
				"Contains a suspicious link you are pushed to click.",
				"クリックを促す不審なリンクが含まれています。",
			)},
			Explanation: tri(
				"Hindi nagpapadala ng login links ang GCash, Maya o bangko (AFASA RA 12010). I-verify sa official app, hindi sa link.",
				"GCash, Maya and banks never send login links (AFASA RA 12010). Verify in the official app, not via the link.",
				"GCash、Maya、銀行がログインリンクを送ることはありません（AFASA RA 12010）。リンクではなく公式アプリで確認してください。",
			),
			Actions: []Lang3{tri(
				"Huwag i-click; buksan ang official app mismo.",
				"Do not click; open the official app yourself.",
				"クリックせず、公式アプリを自分で開いてください。",
			)},
			InjectionDetected: injection,
		}
	case len(rules.Flags) > 0:
		risk := 30 + len(rules.Flags)*10
		if risk > 60 {
			risk = 60
		}
		out = AIOutput{
			Verdict: "suspicious", Risk: risk, ScamType: "other",
			RedFlags: []Lang3{tri(
				"May ilang kahina-hinalang senyales mula sa rules check.",
				"The rules check found some suspicious signals.",
				"ルールチェックで不審な兆候が見つかりました。",
			)},
			Explanation: tri(
				"May nakitang senyales ang rules check. Mag-ingat at i-verify sa official channel bago kumilos.",
				"The rules check found some signals. Be careful and verify via official channels before acting.",
				"ルールチェックで兆候が見つかりました。行動する前に公式チャネルで確認してください。",
			),
			Actions: []Lang3{tri(
				"I-verify sa official app o hotline.",
				"Verify via the official app or hotline.",
				"公式アプリやホットラインで確認してください。",
			)},
			InjectionDetected: injection,
		}
	default:
		out = AIOutput{
			Verdict: "no_red_flags", Risk: 5, ScamType: "none",
			RedFlags: []Lang3{},
			Explanation: tri(
				"Walang nakitang red flags, pero mag-ingat pa rin at i-verify sa official channel kung may hinging pera o info.",
				"No red flags found, but stay cautious and verify via official channels if money or info is requested.",
				"危険信号は見つかりませんでしたが、金銭や情報が求められる場合は公式チャネルで確認してください。",
			),
			Actions: []Lang3{tri(
				"Kapag may duda, tawagan ang official hotline.",
				"If unsure, call the official hotline.",
				"不安な場合は公式ホットラインに電話してください。",
			)},
			InjectionDetected: injection,
		}
	}
	if injection {
		out.InjectionDetected = true
		if out.Risk < 65 {
			out.Risk = 65
		}
		if out.Verdict == "no_red_flags" || out.Verdict == "unclear" {
			out.Verdict = "suspicious"
		}
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
