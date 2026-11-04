// SPDX-License-Identifier: AGPL-3.0-only

package httpclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Attribute keys of the client span, from the OpenTelemetry semantic conventions.
const (
	attrMethod        = "http.request.method"
	attrServerAddress = "server.address"
	attrStatus        = "http.response.status_code"
)

// RequestSpan is the client span of one outbound request (ADR-0055 §13), named after its
// method. Of the destination it carries the host name alone (server.address, §4), for an
// operator to find the deliveries that fail: never the URL, its path, or its query.
var RequestSpan = kernel.SpanSpec{
	Name: "HTTP", Kind: kernel.SpanClient, AttrKeys: []string{attrMethod, attrServerAddress, attrStatus},
}

// startSpan starts the client span of r.
func (c *Client) startSpan(ctx context.Context, r *kernel.HTTPRequest) (context.Context, kernel.Span) {
	method := methodLabel(r.Method)
	attrs := kernel.NewAttrs(kernel.StringAttr(attrMethod, method))
	if u, err := url.Parse(r.URL); err == nil && u.Hostname() != "" {
		attrs = attrs.With(kernel.StringAttr(attrServerAddress, strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))))
	}
	return c.opts.Tracer.Start(ctx, RequestSpan.Named(method), attrs)
}

// endSpan ends the client span of a request with its status, failed for a response of 4xx or
// 5xx (the semantic conventions of a client span) or a request that got no response.
func endSpan(span kernel.Span, resp *kernel.HTTPResponse, err error) {
	switch {
	case err != nil:
		span.End(errorType(err))
	case resp.StatusCode >= 400:
		span.Set(kernel.IntAttr(attrStatus, int64(resp.StatusCode)))
		span.End(strconv.Itoa(resp.StatusCode))
	default:
		span.Set(kernel.IntAttr(attrStatus, int64(resp.StatusCode)))
		span.End("")
	}
}
