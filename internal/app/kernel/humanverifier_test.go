// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"testing"
	"time"
)

func TestHumanCheckWidgetScriptSrc(t *testing.T) {
	param := HumanCheckWidget{ScriptURL: "https://js.example/api.js?render=explicit", LanguageParam: "hl"}
	if got := param.ScriptSrc("pt-BR"); got != "https://js.example/api.js?hl=pt-BR&render=explicit" {
		t.Errorf("ScriptSrc(pt-BR) = %q", got)
	}
	if got := param.ScriptSrc(""); got != param.ScriptURL {
		t.Errorf("ScriptSrc(\"\") = %q, want ScriptURL", got)
	}
	attr := HumanCheckWidget{ScriptURL: "https://js.example/api.js", LanguageAttribute: "data-language"}
	if got := attr.ScriptSrc("ja"); got != attr.ScriptURL {
		t.Errorf("ScriptSrc(ja) = %q, want ScriptURL when the attribute selects the language", got)
	}
}

func TestHumanCheckHTTPLimits(t *testing.T) {
	l, ok := DefaultHTTPLimits(HTTPPurposeHumanCheck)
	if !ok || l.MaxBodyBytes != 64<<10 || l.Timeout != 10*time.Second {
		t.Errorf("DefaultHTTPLimits(HTTPPurposeHumanCheck) = %+v, %v; want 64 KiB and 10s (ADR-0106 §9)", l, ok)
	}
}
