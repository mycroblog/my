// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
)

const svgOpen = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 100 100">`

func wrapSVG(body string) string { return svgOpen + body + `</svg>` }

// TestSanitizeBrandSVGRejects covers every rule that rejects the upload (ADR-0116 §2).
func TestSanitizeBrandSVGRejects(t *testing.T) {
	deep := strings.Repeat("<g>", instance.MaxBrandSVGDepth) + strings.Repeat("</g>", instance.MaxBrandSVGDepth)
	tests := []struct {
		name string
		kind instance.BrandKind
		src  string
		want instance.BrandProblemCode
	}{
		{"not XML", instance.BrandIcon, `<svg viewBox="0 0 1 1"><g></svg>`, instance.BrandMalformed},
		{"empty", instance.BrandIcon, ``, instance.BrandMalformed},
		{"two roots", instance.BrandIcon, wrapSVG("") + wrapSVG(""), instance.BrandMalformed},
		{"text after the root", instance.BrandIcon, wrapSVG("") + "x", instance.BrandMalformed},
		{"non-UTF-8 encoding", instance.BrandIcon, `<?xml version="1.0" encoding="ISO-8859-1"?>` + wrapSVG(""), instance.BrandMalformed},
		{"unknown entity", instance.BrandIcon, wrapSVG(`<title>&nbsp;</title>`), instance.BrandMalformed},
		{"DOCTYPE", instance.BrandIcon, `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">` + wrapSVG(""), instance.BrandDoctype},
		{"XXE", instance.BrandIcon, `<!DOCTYPE svg [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>` + wrapSVG(`<title>&xxe;</title>`), instance.BrandDoctype},
		{"billion laughs", instance.BrandIcon, `<!DOCTYPE svg [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">]>` + wrapSVG(`<title>&b;</title>`), instance.BrandDoctype},
		{"root is not svg", instance.BrandIcon, `<html xmlns="http://www.w3.org/1999/xhtml"><svg viewBox="0 0 1 1"/></html>`, instance.BrandNotSVG},
		{"root in another namespace", instance.BrandIcon, `<svg xmlns="http://example.com/x" viewBox="0 0 1 1"/>`, instance.BrandNotSVG},
		{"no viewBox", instance.BrandIcon, `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`, instance.BrandNoViewBox},
		{"viewBox not numbers", instance.BrandIcon, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 a b"/>`, instance.BrandNoViewBox},
		{"viewBox three numbers", instance.BrandIcon, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10"/>`, instance.BrandNoViewBox},
		{"viewBox zero", instance.BrandIcon, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 0"/>`, instance.BrandNoViewBox},
		{"viewBox NaN", instance.BrandIcon, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 NaN NaN"/>`, instance.BrandNoViewBox},
		{"viewBox huge", instance.BrandIcon, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1e300 1e300"/>`, instance.BrandNoViewBox},
		{"input too large", instance.BrandIcon, wrapSVG(`<desc>` + strings.Repeat("a", instance.MaxBrandSVGInputBytes) + `</desc>`), instance.BrandTooLarge},
		{"output too large", instance.BrandIcon, wrapSVG(`<desc>` + strings.Repeat("&amp;", instance.MaxBrandSVGBytes/4) + `</desc>`), instance.BrandTooLarge},
		{"too many elements", instance.BrandIcon, wrapSVG(strings.Repeat("<g/>", instance.MaxBrandSVGElements)), instance.BrandTooManyElements},
		{"dropped elements count too", instance.BrandIcon, wrapSVG("<metadata>" + strings.Repeat("<x/>", instance.MaxBrandSVGElements) + "</metadata>"), instance.BrandTooManyElements},
		{"too deep", instance.BrandIcon, wrapSVG(deep), instance.BrandTooDeep},
		{"too many uses", instance.BrandIcon, wrapSVG(`<path id="p" d="M0 0"/>` + strings.Repeat(`<use href="#p"/>`, instance.MaxBrandSVGUses+1)), instance.BrandTooManyUses},
		{"use expansion bomb", instance.BrandIcon, wrapSVG(useBomb(8, 10)), instance.BrandTooManyUses},
		{"use cycle", instance.BrandIcon, wrapSVG(`<g id="a"><use href="#b"/></g><g id="b"><use href="#a"/></g>`), instance.BrandTooManyUses},
		{"use of itself", instance.BrandIcon, wrapSVG(`<g id="a"><use href="#a"/></g>`), instance.BrandTooManyUses},
		{"icon not square", instance.BrandIcon, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 102"/>`, instance.BrandAspectRatio},
		{"logo taller than wide", instance.BrandLogo, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 99 100"/>`, instance.BrandAspectRatio},
		{"logo too wide", instance.BrandLogoDark, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 801 100"/>`, instance.BrandAspectRatio},
		{"text", instance.BrandIcon, wrapSVG(`<text x="0" y="10">my</text>`), instance.BrandText},
		{"tspan", instance.BrandIcon, wrapSVG(`<g><tspan>my</tspan></g>`), instance.BrandText},
		{"text inside a dropped element", instance.BrandIcon, wrapSVG(`<switch><text>my</text></switch>`), instance.BrandText},
		{"CSS @import", instance.BrandIcon, wrapSVG(`<style>@import url(https://evil.example/x.css);</style>`), instance.BrandUnsafeCSS},
		{"CSS @IMPORT uppercase", instance.BrandIcon, wrapSVG(`<style>@IMPORT "https://evil.example/x.css";</style>`), instance.BrandUnsafeCSS},
		{"CSS @import by entity", instance.BrandIcon, wrapSVG(`<style>&#64;import "https://evil.example/x.css";</style>`), instance.BrandUnsafeCSS},
		{"CSS @import in CDATA", instance.BrandIcon, wrapSVG(`<style><![CDATA[@import "x.css";]]></style>`), instance.BrandUnsafeCSS},
		{"CSS @font-face", instance.BrandIcon, wrapSVG(`<style>@font-face{font-family:x;src:url(https://evil.example/f.woff)}</style>`), instance.BrandUnsafeCSS},
		{"CSS @namespace", instance.BrandIcon, wrapSVG(`<style>@namespace svg url(http://www.w3.org/2000/svg);</style>`), instance.BrandUnsafeCSS},
		{"CSS remote url", instance.BrandIcon, wrapSVG(`<style>path{fill:url(https://evil.example/track.svg#a)}</style>`), instance.BrandUnsafeCSS},
		{"CSS URL uppercase", instance.BrandIcon, wrapSVG(`<style>path{fill:URL("//evil.example/a")}</style>`), instance.BrandUnsafeCSS},
		{"CSS data url", instance.BrandIcon, wrapSVG(`<style>path{fill:url(data:image/svg+xml;base64,PHN2Zz4=)}</style>`), instance.BrandUnsafeCSS},
		{"CSS unterminated url", instance.BrandIcon, wrapSVG(`<style>path{fill:url(#a}</style>`), instance.BrandUnsafeCSS},
		{"CSS expression", instance.BrandIcon, wrapSVG(`<style>path{width:expression(alert(1))}</style>`), instance.BrandUnsafeCSS},
		{"CSS escape", instance.BrandIcon, wrapSVG(`<style>path{fill:u\72l(https://evil.example/)}</style>`), instance.BrandUnsafeCSS},
		{"CSS escaped at-rule", instance.BrandIcon, wrapSVG(`<style>@\69mport "x.css";</style>`), instance.BrandUnsafeCSS},
		{"CSS less-than", instance.BrandIcon, wrapSVG(`<style>&lt;/style&gt;&lt;script&gt;alert(1)&lt;/script&gt;</style>`), instance.BrandUnsafeCSS},
		{"CSS image-set", instance.BrandIcon, wrapSVG(`<style>path{fill:image-set("https://evil.example/a.png" 1x)}</style>`), instance.BrandUnsafeCSS},
		{"CSS -moz-binding", instance.BrandIcon, wrapSVG(`<style>path{-moz-binding:url(#a)}</style>`), instance.BrandUnsafeCSS},
		{"CSS behavior", instance.BrandIcon, wrapSVG(`<style>path{behavior:url(#a)}</style>`), instance.BrandUnsafeCSS},
		{"style attribute remote url", instance.BrandIcon, wrapSVG(`<path style="fill:url('https://evil.example/a')" d="M0 0"/>`), instance.BrandUnsafeCSS},
		{"style attribute expression", instance.BrandIcon, wrapSVG(`<path style="width: EXPRESSION(alert(1))" d="M0 0"/>`), instance.BrandUnsafeCSS},
		{"style attribute javascript", instance.BrandIcon, wrapSVG(`<path style="background:javascript:alert(1)" d="M0 0"/>`), instance.BrandUnsafeCSS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := instance.SanitizeBrandSVG(tt.kind, []byte(tt.src))
			var be *instance.BrandError
			if !errors.As(err, &be) {
				t.Fatalf("SanitizeBrandSVG = %q, %v; want a %s error", got.SVG, err, tt.want)
			}
			if be.Code != tt.want {
				t.Errorf("code = %s (%v), want %s", be.Code, err, tt.want)
			}
			if !errors.Is(err, instance.ErrInvalidBrand) {
				t.Errorf("%v does not wrap ErrInvalidBrand", err)
			}
		})
	}
}

// useBomb nests levels groups, each using the previous one fan times.
func useBomb(levels, fan int) string {
	var b strings.Builder
	b.WriteString(`<defs><path id="l0" d="M0 0h1"/>`)
	for i := 1; i <= levels; i++ {
		b.WriteString(`<g id="l` + strconv.Itoa(i) + `">`)
		for range fan {
			b.WriteString(`<use href="#l` + strconv.Itoa(i-1) + `"/>`)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</defs>`)
	return b.String()
}

// forbiddenInOutput are the strings that no sanitized SVG may contain, whatever its input.
var forbiddenInOutput = []string{
	"<script", "script>", "foreignobject", "onload", "onclick", "onerror", "onbegin", "onmouseover", "onfocus",
	"javascript:", "data:", "<image", "<a ", "<a>", "<iframe", "<animate", "<set", "<filter", "<fe", "http://evil", "https://evil",
	"//evil", "<!", "<?", "<html", "<body", "<embed", "<object", "<handler", "<listener", "xml:base", "<use href=\"http", "xml-stylesheet",
}

// TestSanitizeBrandSVGNeutralizes covers known SVG XSS vectors: each is dropped from an output that
// stays a valid, usable SVG.
func TestSanitizeBrandSVGNeutralizes(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		dropped []string
	}{
		{"script", `<script>alert(1)</script><path d="M0 0"/>`, []string{"<script>"}},
		{"script in CDATA", `<script><![CDATA[alert(document.domain)]]></script>`, []string{"<script>"}},
		{"script with xlink:href", `<script xlink:href="https://evil.example/x.js"/>`, []string{"<script>"}},
		{"script in a prefixed SVG namespace", `<svg:script xmlns:svg="http://www.w3.org/2000/svg">alert(1)</svg:script>`, []string{"<script>"}},
		{"XHTML script", `<html:script xmlns:html="http://www.w3.org/1999/xhtml">alert(1)</html:script>`, []string{"<script>"}},
		{"foreignObject", `<foreignObject width="100" height="100"><body xmlns="http://www.w3.org/1999/xhtml"><iframe src="javascript:alert(1)"></iframe></body></foreignObject>`, []string{"<foreignObject>"}},
		{"onload on the root's child", `<g onload="alert(1)"><path d="M0 0"/></g>`, []string{"onload"}},
		{"onclick", `<path d="M0 0" onclick="alert(1)"/>`, []string{"onclick"}},
		{"onmouseover uppercase", `<rect width="1" height="1" ONMOUSEOVER="alert(1)"/>`, []string{"ONMOUSEOVER"}},
		{"onerror on image", `<image href="x" onerror="alert(1)"/>`, []string{"<image>"}},
		{"onbegin on animate", `<animate onbegin="alert(1)" attributeName="x" dur="1s"/>`, []string{"<animate>"}},
		{"set to javascript href", `<a><set attributeName="href" to="javascript:alert(1)"/><circle r="5"/></a>`, []string{"<a>"}},
		{"animate href", `<a href="#x"><animate attributeName="href" values="javascript:alert(1)"/></a>`, []string{"<a>"}},
		{"anchor with javascript", `<a href="javascript:alert(1)"><rect width="1" height="1"/></a>`, []string{"<a>"}},
		{"anchor with xlink javascript", `<a xlink:href="javascript:alert(1)"><rect width="1" height="1"/></a>`, []string{"<a>"}},
		{"use of an external file", `<use href="https://evil.example/sprite.svg#a"/>`, []string{"href"}},
		{"use of an external file by xlink", `<use xlink:href="//evil.example/sprite.svg#a"/>`, []string{"xlink:href"}},
		{"use of a data URL", `<use href="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9ImFsZXJ0KDEpIi8+#x"/>`, []string{"href"}},
		{"use of javascript", `<use xlink:href="javascript:alert(1)"/>`, []string{"xlink:href"}},
		{"gradient href to a remote file", `<linearGradient id="g" href="https://evil.example/g.svg#g"/>`, []string{"href"}},
		{"image with a data URL", `<image href="data:image/png;base64,iVBORw0KGgo="/>`, []string{"<image>"}},
		{"iframe", `<iframe xmlns="http://www.w3.org/1999/xhtml" src="javascript:alert(1)"/>`, []string{"<iframe>"}},
		{"embed and object", `<embed src="x.swf"/><object data="x"/>`, []string{"<embed>", "<object>"}},
		{"handler (SVG Tiny)", `<handler type="application/ecmascript">alert(1)</handler>`, []string{"<handler>"}},
		{"listener", `<listener event="load" handler="#h"/>`, []string{"<listener>"}},
		{"filter", `<filter id="f"><feImage href="https://evil.example/a.png"/></filter><rect filter="url(#f)" width="1" height="1"/>`, []string{"<filter>", "filter"}},
		{"fill to a remote url", `<rect fill="url(https://evil.example/a.svg#p)" width="1" height="1"/>`, []string{"fill"}},
		{"mask to a remote url", `<rect mask="url('//evil.example/a#m')" width="1" height="1"/>`, []string{"mask"}},
		{"attribute with javascript", `<rect fill="javascript:alert(1)" width="1" height="1"/>`, []string{"fill"}},
		{"attribute with an escaped javascript", `<rect fill="&#106;avascript:alert(1)" width="1" height="1"/>`, []string{"fill"}},
		{"attribute with tabs in the scheme", "<rect fill=\"java&#9;script:alert(1)\" width=\"1\" height=\"1\"/>", []string{"fill"}},
		{"xml:base", `<g xml:base="https://evil.example/"><use href="#a"/></g><path id="a" d="M0 0"/>`, []string{"xml:base"}},
		{"xml-stylesheet", ``, nil},
		{"comment", `<!-- <script>alert(1)</script> -->`, nil},
		{"inkscape metadata", `<sodipodi:namedview xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"/><metadata><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/></metadata>`, []string{"<namedview>", "<metadata>"}},
		{"undeclared prefix", `<x:script>alert(1)</x:script>`, []string{"<script>"}},
		{"element in title", `<title><script>alert(1)</script>t</title>`, []string{"<script>"}},
		{"html in style", `<style><b>x</b></style>`, []string{"<b>"}},
		{"type on a path", `<path type="text/ecmascript" d="M0 0"/>`, []string{"type"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := wrapSVG(tt.body)
			if tt.name == "xml-stylesheet" {
				src = `<?xml version="1.0" encoding="UTF-8"?><?xml-stylesheet href="https://evil.example/a.css" type="text/css"?>` + src
			}
			got, err := instance.SanitizeBrandSVG(instance.BrandIcon, []byte(src))
			if err != nil {
				t.Fatalf("SanitizeBrandSVG: %v", err)
			}
			out := strings.ToLower(string(got.SVG))
			for _, f := range forbiddenInOutput {
				if strings.Contains(out, strings.ToLower(f)) {
					t.Errorf("output has %q:\n%s", f, got.SVG)
				}
			}
			if _, err := instance.SanitizeBrandSVG(instance.BrandIcon, got.SVG); err != nil {
				t.Errorf("the output is not a valid brand SVG: %v\n%s", err, got.SVG)
			}
			names := got.DroppedNames()
			for _, d := range tt.dropped {
				if !slices.Contains(names, d) {
					t.Errorf("dropped = %v, want %s among them", names, d)
				}
			}
		})
	}
}

// TestSanitizeBrandSVGKeeps checks that what an exported logo uses comes through, rewritten.
func TestSanitizeBrandSVGKeeps(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<!-- Generator: an editor -->
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" viewBox="0, 0, 400, 100" width="400" height="100">
  <title>Example &amp; Co</title>
  <defs>
    <style>.a{fill:url(#g)} @media (prefers-color-scheme: dark) { .a { fill: #fff } } .b > .c { mask: url( "#m" ) }</style>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="1" gradientUnits="objectBoundingBox"><stop offset="0" stop-color="#123456"/><stop offset="1" stop-color="#abcdef" stop-opacity=".5"/></linearGradient>
    <clipPath id="c"><rect width="10" height="10"/></clipPath>
    <mask id="m"><circle cx="5" cy="5" r="5" fill="#fff"/></mask>
    <pattern id="p" width="4" height="4" patternUnits="userSpaceOnUse"><path d="M0 0h2v2H0z"/></pattern>
    <symbol id="s" viewBox="0 0 10 10"><polygon points="0,0 10,0 5,10"/></symbol>
  </defs>
  <g class="a" clip-path="url(#c)" transform="translate(1 2)" style="opacity:.9;fill:url('#p')">
    <path d="M0 0L10 10" stroke="#000" stroke-width="2" fill-rule="evenodd"/>
    <use xlink:href="#s" x="20" width="10" height="10"/>
    <use href="#s" x="40"/>
    <ellipse cx="1" cy="1" rx="1" ry="1"/><line x1="0" y1="0" x2="1" y2="1"/><polyline points="0 0 1 1"/>
  </g>
</svg>`
	got, err := instance.SanitizeBrandSVG(instance.BrandLogo, []byte(src))
	if err != nil {
		t.Fatalf("SanitizeBrandSVG: %v", err)
	}
	if got.ViewWidth != 400 || got.ViewHeight != 100 {
		t.Errorf("viewBox = %gx%g", got.ViewWidth, got.ViewHeight)
	}
	if len(got.Dropped) != 0 {
		t.Errorf("dropped = %v, want nothing", got.Dropped)
	}
	out := string(got.SVG)
	for _, want := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" viewBox="0, 0, 400, 100"`,
		`<title>Example &amp; Co</title>`, `@media (prefers-color-scheme: dark)`, `.b &gt; .c`,
		`<use xlink:href="#s" x="20"`, `<use href="#s" x="40"/>`, `clip-path="url(#c)"`, `style="opacity:.9;fill:url(&#39;#p&#39;)"`,
		`<stop offset="1" stop-color="#abcdef" stop-opacity=".5"/>`, `<pattern id="p"`, `<mask id="m">`, `<symbol id="s" viewBox="0 0 10 10">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<!--") || strings.Contains(out, "<?xml") {
		t.Errorf("output keeps a comment or the declaration:\n%s", out)
	}
	again, err := instance.SanitizeBrandSVG(instance.BrandLogo, got.SVG)
	if err != nil || string(again.SVG) != out {
		t.Errorf("sanitizing the output again = %q, %v; want the same", again.SVG, err)
	}
}

func TestSanitizeBrandSVGAspectRatio(t *testing.T) {
	for _, tt := range []struct {
		kind instance.BrandKind
		vb   string
		ok   bool
	}{
		{instance.BrandIcon, "0 0 100 100", true},
		{instance.BrandIcon, "0 0 100.5 100", true},
		{instance.BrandIcon, "0 0 101 100", false},
		{instance.BrandIcon, "0 0 99 100", false},
		{instance.BrandLogo, "0 0 100 100", true},
		{instance.BrandLogo, "0 0 800 100", true},
		{instance.BrandLogo, "-10 -10 800 100", true},
		{instance.BrandLogoDark, "0 0 800.1 100", false},
	} {
		_, err := instance.SanitizeBrandSVG(tt.kind, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="`+tt.vb+`"/>`))
		if (err == nil) != tt.ok {
			t.Errorf("%s %q: err = %v, want ok %v", tt.kind, tt.vb, err, tt.ok)
		}
	}
}

func TestSanitizeBrandSVGWithoutNamespace(t *testing.T) {
	got, err := instance.SanitizeBrandSVG(instance.BrandIcon, []byte(`<svg viewBox="0 0 1 1"><path d="M0 0"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if want := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="M0 0"/></svg>` + "\n"; string(got.SVG) != want {
		t.Errorf("SVG = %q, want %q", got.SVG, want)
	}
}

func TestSanitizeBrandSVGLimitsAtTheEdge(t *testing.T) {
	deep := strings.Repeat("<g>", instance.MaxBrandSVGDepth-1) + strings.Repeat("</g>", instance.MaxBrandSVGDepth-1)
	uses := `<path id="p" d="M0 0"/>` + strings.Repeat(`<use href="#p"/>`, instance.MaxBrandSVGUses)
	many := strings.Repeat("<g/>", instance.MaxBrandSVGElements-1)
	for name, body := range map[string]string{"depth": deep, "uses": uses, "elements": many} {
		if _, err := instance.SanitizeBrandSVG(instance.BrandIcon, []byte(wrapSVG(body))); err != nil {
			t.Errorf("%s at the limit: %v", name, err)
		}
	}
}
