// SPDX-License-Identifier: AGPL-3.0-only

// Command protodoc is a protoc plugin that writes the reference of the plugin protocol: one Markdown page for every
// package of api/plugin, from the comments of the .proto files. buf runs it from api/plugin/buf.gen.yaml with the
// strategy all, so that it gets every file at once, and scripts/plugin-generate.sh moves the pages into docs/plugins.
//
// A page holds the services of its package with their RPCs, its messages, and its enums. The comments become the prose
// as they are: a blank line of a comment starts a new paragraph. The types of the package link to their headings, whose
// anchors are the ones GitHub and the documentation site generate (a repeated heading gets -1, -2, ...).
package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Marker is the first line of every page, which tells the generated pages from the hand-written ones of docs/plugins.
const Marker = `<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->`

// pages names the page of each package: its file and its title. A package missing here fails the generation, so that
// a new package gets a name before it gets a page.
var pages = map[protoreflect.FullName]struct{ file, title string }{
	"my.plugin.probe.v1":       {"probe.md", "Probe protocol"},
	"my.plugin.translator.v1":  {"translator.md", "Translator protocol"},
	"my.plugin.billing.v1":     {"billing.md", "Billing protocol"},
	"my.plugin.mailer.v1":      {"mailer.md", "Mailer protocol"},
	"my.plugin.media.v1":       {"media.md", "Media protocol"},
	"my.plugin.key_manager.v1": {"key-manager.md", "Key manager protocol"},
	"my.plugin.hook.v1":        {"hooks.md", "Decision hook protocol"},
}

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			p, ok := pages[f.Desc.Package()]
			if !ok {
				return fmt.Errorf("protodoc: the package %s has no page: add it to pages in tools/protodoc", f.Desc.Package())
			}
			g := gen.NewGeneratedFile(p.file, "")
			if _, err := g.Write([]byte(render(f, p.title))); err != nil {
				return err
			}
		}
		return nil
	})
}

// page builds the Markdown of one package and the anchors of its headings.
type page struct {
	out  *strings.Builder // nil while the anchors are assigned
	seen map[string]int
	pkg  protoreflect.FullName
	// anchors maps the full name of a message or an enum to the anchor of its heading.
	anchors map[protoreflect.FullName]string
}

// render returns the page of f.
func render(f *protogen.File, title string) string {
	p := &page{pkg: f.Desc.Package(), anchors: map[protoreflect.FullName]string{}}
	// The anchors are assigned in the order of the headings first, so that a link may come before its heading.
	p.seen = map[string]int{}
	p.file(f)
	var b strings.Builder
	p.out, p.seen = &b, map[string]int{}
	fmt.Fprintf(&b, "%s\n\n# %s\n\n", Marker, title)
	fmt.Fprintf(&b, "The services, messages, and enums of the package %s, which [%s](../../%s) defines.\n",
		code(string(f.Desc.Package())), code(path.Base(f.Desc.Path())), "api/plugin/"+f.Desc.Path())
	p.para(comment(f.Desc.SourceLocations().ByPath(protoreflect.SourcePath{2}).LeadingComments))
	p.file(f)
	return b.String()
}

// file walks the headings of f in order, writing them when p.out is set.
func (p *page) file(f *protogen.File) {
	if len(f.Services) > 0 {
		p.head(2, "Services")
	}
	for _, s := range f.Services {
		p.head(3, code(s.GoName))
		p.para(comment(string(s.Comments.Leading)))
		for _, m := range s.Methods {
			p.head(4, code(string(m.Desc.Name())))
			if p.out != nil {
				fmt.Fprintf(p.out, "\n```proto\nrpc %s(%s%s) returns (%s%s);\n```\n", m.Desc.Name(),
					streamWord(m.Desc.IsStreamingClient()), m.Input.Desc.Name(), streamWord(m.Desc.IsStreamingServer()), m.Output.Desc.Name())
				fmt.Fprintf(p.out, "\nRequest: %s. Response: %s.\n", p.typeName(m.Input.Desc.FullName()), p.typeName(m.Output.Desc.FullName()))
			}
			p.para(comment(string(m.Comments.Leading)))
		}
	}
	if len(f.Messages) > 0 {
		p.head(2, "Messages")
		p.messages(f.Messages)
	}
	if len(f.Enums) > 0 {
		p.head(2, "Enums")
		for _, e := range f.Enums {
			p.enum(e)
		}
	}
}

// messages writes each message, then the messages and enums nested in it.
func (p *page) messages(ms []*protogen.Message) {
	for _, m := range ms {
		if m.Desc.IsMapEntry() {
			continue
		}
		p.anchors[m.Desc.FullName()] = p.head(3, code(p.localName(m.Desc.FullName())))
		p.para(comment(string(m.Comments.Leading)))
		p.fields(m)
		p.messages(m.Messages)
		for _, e := range m.Enums {
			p.enum(e)
		}
	}
}

// enum writes the heading of e and its values.
func (p *page) enum(e *protogen.Enum) {
	p.anchors[e.Desc.FullName()] = p.head(3, code(p.localName(e.Desc.FullName())))
	p.para(comment(string(e.Comments.Leading)))
	if p.out == nil {
		return
	}
	p.out.WriteString("\n")
	for _, v := range e.Values {
		fmt.Fprintf(p.out, "- %s = %d%s\n", code(string(v.Desc.Name())), v.Desc.Number(), item(string(v.Comments.Leading), string(v.Comments.Trailing)))
	}
}

// fields writes the list of the fields of m.
func (p *page) fields(m *protogen.Message) {
	if p.out == nil {
		return
	}
	if len(m.Fields) == 0 {
		p.out.WriteString("\nNo fields.\n")
		return
	}
	p.out.WriteString("\n")
	for _, f := range m.Fields {
		notes := []string{p.fieldType(f.Desc), fmt.Sprintf("field %d", f.Desc.Number())}
		if o := f.Desc.ContainingOneof(); o != nil {
			if o.IsSynthetic() {
				notes = append(notes, "optional")
			} else {
				notes = append(notes, "one of "+code(string(o.Name())))
			}
		}
		fmt.Fprintf(p.out, "- %s (%s)%s\n", code(string(f.Desc.Name())), strings.Join(notes, ", "), item(string(f.Comments.Leading), string(f.Comments.Trailing)))
	}
}

// head writes a heading and returns its anchor.
func (p *page) head(level int, text string) string {
	a := slug(text)
	if c := p.seen[a]; c > 0 {
		p.seen[a] = c + 1
		a += fmt.Sprintf("-%d", c)
	} else {
		p.seen[a] = 1
	}
	if p.out != nil {
		fmt.Fprintf(p.out, "\n%s %s\n", strings.Repeat("#", level), text)
	}
	return a
}

// para writes a paragraph, or nothing for an empty one.
func (p *page) para(text string) {
	if p.out != nil && text != "" {
		p.out.WriteString("\n" + text + "\n")
	}
}

// fieldType returns the type of f, linked when it is a message or an enum of the page.
func (p *page) fieldType(f protoreflect.FieldDescriptor) string {
	if f.IsMap() {
		return "map of " + p.kindName(f.MapKey()) + " to " + p.kindName(f.MapValue())
	}
	if f.IsList() {
		return "repeated " + p.kindName(f)
	}
	return p.kindName(f)
}

// kindName returns the type of a field without its cardinality.
func (p *page) kindName(f protoreflect.FieldDescriptor) string {
	switch f.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return p.typeName(f.Message().FullName())
	case protoreflect.EnumKind:
		return p.typeName(f.Enum().FullName())
	default:
		return code(f.Kind().String())
	}
}

// typeName returns the name of a message or an enum, linked to its heading when the page has one.
func (p *page) typeName(name protoreflect.FullName) string {
	if a, ok := p.anchors[name]; ok {
		return "[" + code(p.localName(name)) + "](#" + a + ")"
	}
	return code(string(name))
}

// localName returns name within the package, such as Post.Body of my.plugin.hook.v1.Post.Body.
func (p *page) localName(name protoreflect.FullName) string {
	return strings.TrimPrefix(string(name), string(p.pkg)+".")
}

// item returns what follows a list item: a colon and the comment, its later lines indented to stay in the item, or
// nothing without a comment.
func item(leading, trailing string) string {
	text := comment(leading)
	if t := comment(trailing); t != "" {
		text = strings.TrimSpace(text + "\n\n" + t)
	}
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = "  " + lines[i]
		}
	}
	return ": " + strings.Join(lines, "\n")
}

// comment returns the text of a comment: the space after each // removed, the lines kept, and one blank line between
// its paragraphs.
func comment(c string) string {
	lines := strings.Split(c, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(strings.TrimPrefix(l, " "), unicode.IsSpace)
	}
	text := strings.Join(lines, "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	// A bare URL becomes an autolink, which Markdown renders as a link on every site.
	text = bareURL.ReplaceAllString(text, "<$1>")
	return strings.TrimSpace(text)
}

// bareURL matches a URL of a comment, without the punctuation that ends its sentence.
var bareURL = regexp.MustCompile(`(https?://[^\s<>()]*[^\s<>().,;:!?])`)

func streamWord(stream bool) string {
	if stream {
		return "stream "
	}
	return ""
}

func code(s string) string { return "`" + s + "`" }

// slug returns the anchor GitHub and the documentation site generate for a heading: lower case, letters, digits,
// marks, hyphens, and underscores kept, spaces turned into hyphens, everything else dropped.
func slug(heading string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(heading) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
