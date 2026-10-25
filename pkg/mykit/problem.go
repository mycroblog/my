// SPDX-License-Identifier: MIT

package mykit

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// ProblemTypeAboutBlank is the type of a problem that means no more than its HTTP status (RFC 9457 §4.2.1).
const ProblemTypeAboutBlank = "about:blank"

// maxProblemBytes bounds the body of an error answer that the client reads.
const maxProblemBytes = 1 << 20

// Error makes a *Problem, the RFC 9457 problem document of an answer with a status of 400 or more, an error:
//
//	var p *mykit.Problem
//	if errors.As(err, &p) && p.Type == "/api/v1/problems/blocking" { ... }
//
// Branch on Type and Status, never on Title or Detail, which the server localizes from Accept-Language.
// The members beyond the standard ones (such as `reason`) are in AdditionalProperties.
func (p *Problem) Error() string {
	var b strings.Builder
	b.WriteString("mykit: ")
	b.WriteString(strconv.Itoa(p.Status))
	b.WriteString(" ")
	b.WriteString(p.Type)
	if p.Title != "" {
		b.WriteString(": ")
		b.WriteString(p.Title)
	}
	if p.Detail != nil && *p.Detail != "" {
		b.WriteString(": ")
		b.WriteString(*p.Detail)
	}
	return b.String()
}

// problemFrom reads the problem of an error answer and closes its body. An answer that is no problem document
// (from a proxy, say) becomes a problem of type about:blank with the answer's status.
func problemFrom(resp *http.Response) *Problem {
	defer func() { _ = resp.Body.Close() }()
	p := &Problem{}
	if isJSON(resp.Header.Get("Content-Type")) {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxProblemBytes))
		if err == nil && json.Unmarshal(body, p) != nil {
			p = &Problem{}
		}
	}
	// The status of the answer is the one that happened, whatever the document says.
	p.Status = resp.StatusCode
	if p.Type == "" {
		p.Type = ProblemTypeAboutBlank
	}
	if p.Title == "" {
		p.Title = http.StatusText(resp.StatusCode)
	}
	return p
}

// isJSON reports whether a Content-Type is JSON, application/problem+json among them.
func isJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}
