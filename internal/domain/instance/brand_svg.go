// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
)

// The SVG sanitizer of the brand images (ADR-0116 §2). It reads the tokens of the upload with
// encoding/xml and writes a new document from the allowed elements and attributes only, so nothing
// of the input reaches the output unless a rule lets it through. The output is served on the
// server's own origin, so a missed construct is script on that origin: the rules are an allowlist,
// and everything else is dropped (reported) or rejected.

// Limits of the brand SVG.
const (
	// MaxBrandSVGInputBytes is the largest upload, before sanitizing (256 KiB).
	MaxBrandSVGInputBytes = 256 << 10
	// MaxBrandSVGBytes is the largest sanitized SVG (128 KiB).
	MaxBrandSVGBytes = 128 << 10
	// MaxBrandSVGElements is the most elements of an upload, dropped ones included.
	MaxBrandSVGElements = 5000
	// MaxBrandSVGDepth is the deepest nesting of elements, the root being 1.
	MaxBrandSVGDepth = 32
	// MaxBrandSVGUses is the most use elements of the sanitized SVG.
	MaxBrandSVGUses = 200
	// maxBrandSVGExpansion bounds the elements that the use references draw once expanded, so
	// that uses of groups of uses cannot multiply into a rendering bomb.
	maxBrandSVGExpansion = 20000
	// Aspect ratios: an icon is square within brandIconRatioTolerance; a logo is 1:1 to 8:1.
	brandIconRatioTolerance = 0.01
	brandLogoMaxRatio       = 8
	// maxBrandViewBox bounds the numbers of the viewBox, so that its size fits the stored pixel sizes.
	maxBrandViewBox = 1e6
)

// Namespaces of SVG documents.
const (
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
)

// brandElements are the SVG elements kept; every other element is dropped with its content,
// except text and tspan, which are rejected (ADR-0116 §2).
var brandElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "symbol": true, "use": true, "title": true, "desc": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true,
	"linearGradient": true, "radialGradient": true, "stop": true, "clipPath": true, "mask": true, "pattern": true,
	"style": true,
}

// brandTextElements keep their character data; the others drop it (it is only whitespace in SVG).
var brandTextElements = map[string]bool{"title": true, "desc": true, "style": true}

// brandAttributes are the attributes without a namespace that are kept on every allowed element:
// geometry and transforms, the presentation attributes, the attributes of gradients, patterns,
// clip paths, and masks, and id, class, and style. Event handlers (on*) and everything else are
// dropped.
var brandAttributes = map[string]bool{
	// Geometry and transforms.
	"d": true, "points": true, "x": true, "y": true, "width": true, "height": true, "r": true, "rx": true, "ry": true,
	"cx": true, "cy": true, "x1": true, "y1": true, "x2": true, "y2": true, "fx": true, "fy": true, "fr": true,
	"transform": true, "viewBox": true, "preserveAspectRatio": true, "pathLength": true, "refX": true, "refY": true,
	"version": true,
	// Presentation attributes.
	"fill": true, "fill-opacity": true, "fill-rule": true,
	"stroke": true, "stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true, "stroke-miterlimit": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "stroke-opacity": true,
	"opacity": true, "stop-color": true, "stop-opacity": true, "clip-path": true, "clip-rule": true, "mask": true,
	"color": true, "display": true, "visibility": true, "overflow": true, "vector-effect": true, "paint-order": true,
	"shape-rendering": true, "color-interpolation": true, "mix-blend-mode": true, "isolation": true,
	"transform-origin": true,
	// Gradients, patterns, clip paths, and masks.
	"gradientUnits": true, "gradientTransform": true, "spreadMethod": true, "offset": true,
	"patternUnits": true, "patternContentUnits": true, "patternTransform": true,
	"clipPathUnits": true, "maskUnits": true, "maskContentUnits": true,
	// Identity and styling.
	"id": true, "class": true, "style": true, "type": true,
}

// BrandDropped is something that the sanitizer removed: an element with its content, or an
// attribute.
type BrandDropped struct {
	// Kind is "element" or "attribute".
	Kind string
	// Name is the local name of the element or attribute.
	Name string
}

// BrandSVG is a sanitized brand SVG.
type BrandSVG struct {
	// SVG is the new document, which only the allowed elements and attributes make up.
	SVG []byte
	// ViewWidth and ViewHeight are the size of the viewBox.
	ViewWidth, ViewHeight float64
	// Dropped lists what was removed, each once, in the order first met.
	Dropped []BrandDropped
}

// svgNode is an element of the document being rebuilt.
type svgNode struct {
	name     string
	attrs    []svgAttr
	children []*svgNode
	text     string
}

type svgAttr struct {
	name, value string
}

// SanitizeBrandSVG checks and rebuilds an uploaded SVG of kind (ADR-0116 §2). It fails with a
// *BrandError when the upload breaks a rule that dropping cannot mend: malformed XML, a DOCTYPE or
// an entity declaration, a root other than svg, a missing or unreadable viewBox, the size, element,
// depth, and use limits, the aspect ratio of the kind, text that is not outlined, and CSS that may
// load anything or escape the checks. Elements and attributes outside the allowlist, references
// other than to #id in the document, and data: URLs are dropped and listed in BrandSVG.Dropped.
func SanitizeBrandSVG(kind BrandKind, src []byte) (BrandSVG, error) {
	if len(src) > MaxBrandSVGInputBytes {
		return BrandSVG{}, brandError(BrandTooLarge, RenditionSVG, "the SVG is at most %d bytes", MaxBrandSVGInputBytes)
	}
	s := &svgSanitizer{seen: map[BrandDropped]bool{}}
	root, err := s.parse(src)
	if err != nil {
		return BrandSVG{}, err
	}
	w, h, err := parseViewBox(root)
	if err != nil {
		return BrandSVG{}, err
	}
	if err := checkAspectRatio(kind, w, h); err != nil {
		return BrandSVG{}, err
	}
	if s.uses > MaxBrandSVGUses {
		return BrandSVG{}, brandError(BrandTooManyUses, RenditionSVG, "the SVG has %d use elements; at most %d", s.uses, MaxBrandSVGUses)
	}
	if err := checkExpansion(root); err != nil {
		return BrandSVG{}, err
	}
	out := writeSVG(root)
	if len(out) > MaxBrandSVGBytes {
		return BrandSVG{}, brandError(BrandTooLarge, RenditionSVG, "the sanitized SVG is %d bytes; at most %d", len(out), MaxBrandSVGBytes)
	}
	return BrandSVG{SVG: out, ViewWidth: w, ViewHeight: h, Dropped: s.dropped}, nil
}

type svgSanitizer struct {
	elements int
	uses     int
	dropped  []BrandDropped
	seen     map[BrandDropped]bool
}

func (s *svgSanitizer) drop(kind, name string) {
	d := BrandDropped{Kind: kind, Name: name}
	if !s.seen[d] {
		s.seen[d] = true
		s.dropped = append(s.dropped, d)
	}
}

// parse reads the tokens of src into the tree of the kept elements.
func (s *svgSanitizer) parse(src []byte) (*svgNode, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	// Strict, with no entities besides XML's five and character references, and no charset
	// conversion: anything but UTF-8 is malformed.
	d.Strict = true
	d.Entity = nil
	d.CharsetReader = nil
	var (
		root  *svgNode
		stack []*svgNode // the kept elements open; nil for a dropped one
		depth int
		ended bool
	)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, brandError(BrandMalformed, RenditionSVG, "%v", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			// <!DOCTYPE ...> and <!ENTITY ...>: no external subset or entity is ever read.
			return nil, brandError(BrandDoctype, RenditionSVG, "the SVG must not have a DOCTYPE or entity declarations")
		case xml.ProcInst, xml.Comment:
			// Dropped: the output is a new document (no xml-stylesheet processing instruction).
		case xml.StartElement:
			if ended {
				return nil, brandError(BrandMalformed, RenditionSVG, "content after the root element")
			}
			depth++
			s.elements++
			if s.elements > MaxBrandSVGElements {
				return nil, brandError(BrandTooManyElements, RenditionSVG, "the SVG has more than %d elements", MaxBrandSVGElements)
			}
			if depth > MaxBrandSVGDepth {
				return nil, brandError(BrandTooDeep, RenditionSVG, "the elements nest more than %d deep", MaxBrandSVGDepth)
			}
			isSVG := t.Name.Space == svgNamespace || t.Name.Space == ""
			if isSVG && (t.Name.Local == "text" || t.Name.Local == "tspan" || t.Name.Local == "textPath") {
				return nil, brandError(BrandText, RenditionSVG, "convert the text to outlines (paths)")
			}
			if root == nil {
				if !isSVG || t.Name.Local != "svg" {
					return nil, brandError(BrandNotSVG, RenditionSVG, "the root element must be svg")
				}
			}
			parent := (*svgNode)(nil)
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			keep := isSVG && brandElements[t.Name.Local] && (root == nil || parent != nil)
			if !keep {
				if root != nil && parent != nil {
					// The top of a dropped subtree; what it holds goes with it.
					s.drop("element", t.Name.Local)
				}
				stack = append(stack, nil)
				continue
			}
			if parent != nil && brandTextElements[parent.name] {
				// title, desc, and style hold text only.
				s.drop("element", t.Name.Local)
				stack = append(stack, nil)
				continue
			}
			n := &svgNode{name: t.Name.Local}
			attrs, err := s.attributes(n.name, t.Attr)
			if err != nil {
				return nil, err
			}
			n.attrs = attrs
			if n.name == "use" {
				s.uses++
			}
			if root == nil {
				root = n
			} else {
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			depth--
			stack = stack[:len(stack)-1]
			if depth == 0 {
				ended = true
			}
		case xml.CharData:
			if len(stack) == 0 {
				if len(bytes.TrimSpace(t)) > 0 {
					return nil, brandError(BrandMalformed, RenditionSVG, "text outside the root element")
				}
				continue
			}
			n := stack[len(stack)-1]
			if n == nil || !brandTextElements[n.name] {
				continue
			}
			n.text += string(t)
			if n.name == "style" {
				if err := checkCSS(n.text); err != nil {
					return nil, err
				}
			}
		}
	}
	if root == nil || !ended {
		return nil, brandError(BrandMalformed, RenditionSVG, "the SVG has no complete root element")
	}
	return root, nil
}

// attributes returns the attributes of an element that are kept, in their order.
func (s *svgSanitizer) attributes(element string, in []xml.Attr) ([]svgAttr, error) {
	var out []svgAttr
	has := map[string]bool{}
	for _, a := range in {
		// Namespace declarations: the output declares its own.
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		name := a.Name.Local
		switch {
		case a.Name.Space == "" && brandAttributes[name]:
		case (a.Name.Space == xlinkNamespace || a.Name.Space == "") && name == "href":
			if a.Name.Space == xlinkNamespace {
				name = "xlink:href"
			}
		default:
			s.drop("attribute", attrLabel(a.Name))
			continue
		}
		if has[name] {
			continue
		}
		v := a.Value
		switch {
		case name == "style":
			if err := checkCSS(v); err != nil {
				return nil, err
			}
		case name == "href" || name == "xlink:href":
			if !isFragmentRef(strings.TrimSpace(v)) {
				s.drop("attribute", name)
				continue
			}
			v = strings.TrimSpace(v)
		case name == "type" && element != "style":
			s.drop("attribute", name)
			continue
		default:
			if !safeAttrValue(v) {
				s.drop("attribute", name)
				continue
			}
		}
		has[name] = true
		out = append(out, svgAttr{name: name, value: v})
	}
	return out, nil
}

func attrLabel(n xml.Name) string {
	switch n.Space {
	case "":
		return n.Local
	case xlinkNamespace:
		return "xlink:" + n.Local
	case "http://www.w3.org/XML/1998/namespace":
		return "xml:" + n.Local
	}
	return n.Local
}

// isFragmentRef reports whether v refers to an element of the same document: # and an ID.
func isFragmentRef(v string) bool {
	id, ok := strings.CutPrefix(v, "#")
	return ok && isSVGID(id)
}

// isSVGID reports whether id is a plain ID that a reference may name.
func isSVGID(id string) bool {
	if id == "" || len(id) > 256 {
		return false
	}
	for i := range len(id) {
		c := id[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '.' && c != ':' {
			return false
		}
	}
	return true
}

// safeAttrValue reports whether the value of a kept attribute refers to nothing outside the
// document: every url(...) is url(#id), and no script or data URL appears in it.
func safeAttrValue(v string) bool {
	lower := strings.ToLower(v)
	compact := strings.Map(func(r rune) rune {
		if r <= ' ' {
			return -1
		}
		return r
	}, lower)
	for _, scheme := range []string{"javascript:", "vbscript:", "data:"} {
		if strings.Contains(compact, scheme) {
			return false
		}
	}
	if strings.Contains(lower, `\`) {
		return false
	}
	return urlsAreFragments(lower)
}

// urlsAreFragments reports whether every url( in the lowercase s is url(#id), quoted or not.
func urlsAreFragments(s string) bool {
	for {
		i := strings.Index(s, "url(")
		if i < 0 {
			return true
		}
		s = s[i+len("url("):]
		end := strings.IndexByte(s, ')')
		if end < 0 {
			return false
		}
		arg := strings.TrimSpace(s[:end])
		if len(arg) >= 2 && (arg[0] == '"' || arg[0] == '\'') && arg[len(arg)-1] == arg[0] {
			arg = arg[1 : len(arg)-1]
		}
		if !isFragmentRef(arg) {
			return false
		}
		s = s[end+1:]
	}
}

// cssForbidden are the constructs that make CSS load something, run script in old engines, or
// escape the checks (ADR-0116 §2). The CSS is lowercased first. An escape (\) is forbidden, so no
// keyword below can be spelled another way.
var cssForbidden = []string{
	"@import", "@font-face", "@namespace", "expression(", `\`, "<",
	"image-set(", "image(", "cross-fade(", "element(", "src(", "-moz-binding", "behavior", "javascript:", "vbscript:", "data:",
}

// checkCSS checks the text of a style element or a style attribute. CSS that breaks a rule is
// rejected rather than dropped, since dropping it would change the image silently.
func checkCSS(css string) error {
	lower := strings.ToLower(css)
	for _, f := range cssForbidden {
		if strings.Contains(lower, f) {
			return brandError(BrandUnsafeCSS, RenditionSVG, "the CSS must not contain %q", f)
		}
	}
	if !urlsAreFragments(lower) {
		return brandError(BrandUnsafeCSS, RenditionSVG, "the CSS may only use url(#id)")
	}
	return nil
}

// parseViewBox returns the width and height of the root's viewBox.
func parseViewBox(root *svgNode) (float64, float64, error) {
	var vb string
	found := false
	for _, a := range root.attrs {
		if a.name == "viewBox" {
			vb, found = a.value, true
		}
	}
	if !found {
		return 0, 0, brandError(BrandNoViewBox, RenditionSVG, "the svg element must have a viewBox")
	}
	fields := strings.FieldsFunc(vb, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	if len(fields) != 4 {
		return 0, 0, brandError(BrandNoViewBox, RenditionSVG, "the viewBox must be four numbers")
	}
	nums := make([]float64, 4)
	for i, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > maxBrandViewBox {
			return 0, 0, brandError(BrandNoViewBox, RenditionSVG, "the viewBox must be four numbers")
		}
		nums[i] = n
	}
	if nums[2] <= 0 || nums[3] <= 0 {
		return 0, 0, brandError(BrandNoViewBox, RenditionSVG, "the viewBox must have a positive width and height")
	}
	return nums[2], nums[3], nil
}

// checkAspectRatio checks the viewBox of kind: square for an icon, 1:1 to 8:1 for a logo.
func checkAspectRatio(kind BrandKind, w, h float64) error {
	ratio := w / h
	switch kind {
	case BrandIcon:
		if math.Abs(ratio-1) >= brandIconRatioTolerance {
			return brandError(BrandAspectRatio, RenditionSVG, "an icon must be square; its viewBox is %gx%g", w, h)
		}
	case BrandLogo, BrandLogoDark:
		if ratio < 1 || ratio > brandLogoMaxRatio {
			return brandError(BrandAspectRatio, RenditionSVG, "a logo must be 1 to %d times as wide as it is high; its viewBox is %gx%g", brandLogoMaxRatio, w, h)
		}
	}
	return nil
}

// checkExpansion bounds the elements that the document draws once every reference (use, and
// the href of gradients and patterns) is expanded, and rejects reference cycles.
func checkExpansion(root *svgNode) error {
	ids := map[string]*svgNode{}
	var index func(n *svgNode)
	index = func(n *svgNode) {
		for _, a := range n.attrs {
			if a.name == "id" {
				if _, dup := ids[a.value]; !dup {
					ids[a.value] = n
				}
			}
		}
		for _, c := range n.children {
			index(c)
		}
	}
	index(root)
	sizes := map[*svgNode]int{}
	visiting := map[*svgNode]bool{}
	tooMany := brandError(BrandTooManyUses, RenditionSVG, "the use references expand to more than %d elements", maxBrandSVGExpansion)
	var size func(n *svgNode) (int, error)
	size = func(n *svgNode) (int, error) {
		if v, ok := sizes[n]; ok {
			return v, nil
		}
		if visiting[n] {
			return 0, brandError(BrandTooManyUses, RenditionSVG, "the references form a cycle")
		}
		visiting[n] = true
		total := 1
		for _, a := range n.attrs {
			if a.name != "href" && a.name != "xlink:href" {
				continue
			}
			if t, ok := ids[strings.TrimPrefix(a.value, "#")]; ok {
				v, err := size(t)
				if err != nil {
					return 0, err
				}
				total += v
			}
		}
		for _, c := range n.children {
			v, err := size(c)
			if err != nil {
				return 0, err
			}
			total += v
			if total > maxBrandSVGExpansion {
				return 0, tooMany
			}
		}
		if total > maxBrandSVGExpansion {
			return 0, tooMany
		}
		visiting[n] = false
		sizes[n] = total
		return total, nil
	}
	_, err := size(root)
	return err
}

// writeSVG writes the tree as a new UTF-8 document, escaping every value.
func writeSVG(root *svgNode) []byte {
	var b bytes.Buffer
	usesXlink := false
	var scan func(n *svgNode)
	scan = func(n *svgNode) {
		for _, a := range n.attrs {
			if a.name == "xlink:href" {
				usesXlink = true
			}
		}
		for _, c := range n.children {
			scan(c)
		}
	}
	scan(root)
	var write func(n *svgNode, isRoot bool)
	write = func(n *svgNode, isRoot bool) {
		b.WriteString("<" + n.name)
		if isRoot {
			b.WriteString(` xmlns="` + svgNamespace + `"`)
			if usesXlink {
				b.WriteString(` xmlns:xlink="` + xlinkNamespace + `"`)
			}
		}
		for _, a := range n.attrs {
			b.WriteString(" " + a.name + `="`)
			_ = xml.EscapeText(&b, []byte(a.value))
			b.WriteString(`"`)
		}
		if len(n.children) == 0 && n.text == "" {
			b.WriteString("/>")
			return
		}
		b.WriteString(">")
		if n.text != "" {
			_ = xml.EscapeText(&b, []byte(n.text))
		}
		for _, c := range n.children {
			write(c, false)
		}
		b.WriteString("</" + n.name + ">")
	}
	write(root, true)
	b.WriteString("\n")
	return b.Bytes()
}

// DroppedNames returns the dropped elements and attributes as "<name>" and "name", for logs.
func (s BrandSVG) DroppedNames() []string {
	out := make([]string, 0, len(s.Dropped))
	for _, d := range s.Dropped {
		if d.Kind == "element" {
			out = append(out, "<"+d.Name+">")
		} else {
			out = append(out, d.Name)
		}
	}
	slices.Sort(out)
	return out
}
