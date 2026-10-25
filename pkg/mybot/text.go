// SPDX-License-Identifier: MIT

package mybot

import (
	"html"
	"strings"
	"unicode"
)

// PlainText turns the HTML content of a post into plain text: the text of every element, mentions and links as
// they read (`@alice`, `https://example.com/`), line breaks for <br> and between paragraphs, and character
// references decoded. It is for reading a post, not for showing it: nothing of the markup survives.
func PlainText(content string) string {
	var b strings.Builder
	paragraphs := 0
	for content != "" {
		i := strings.IndexByte(content, '<')
		if i < 0 {
			b.WriteString(html.UnescapeString(content))
			break
		}
		b.WriteString(html.UnescapeString(content[:i]))
		content = content[i:]
		if len(content) < 2 || !opensTag(content[1]) {
			// A '<' that opens no tag, such as in "1 < 2", is text.
			b.WriteByte('<')
			content = content[1:]
			continue
		}
		end := strings.IndexByte(content, '>')
		if end < 0 {
			// A '<' that opens no tag is text.
			b.WriteString(html.UnescapeString(content))
			break
		}
		switch tagName(content[1:end]) {
		case "br":
			b.WriteByte('\n')
		case "p":
			if paragraphs > 0 {
				b.WriteString("\n\n")
			}
			paragraphs++
		}
		content = content[end+1:]
	}
	return strings.TrimSpace(b.String())
}

// opensTag reports whether c, after a '<', starts a tag, a closing tag, a comment, or a declaration.
func opensTag(c byte) bool {
	return c == '/' || c == '!' || c == '?' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// tagName is the lowercase name of an opening tag from what is between its < and >, "" for a closing tag, a
// comment, or a declaration.
func tagName(tag string) string {
	if tag == "" || !unicode.IsLetter(rune(tag[0])) {
		return ""
	}
	end := strings.IndexFunc(tag, func(r rune) bool { return unicode.IsSpace(r) || r == '/' })
	if end >= 0 {
		tag = tag[:end]
	}
	return strings.ToLower(tag)
}
