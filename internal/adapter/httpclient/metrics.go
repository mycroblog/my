// SPDX-License-Identifier: AGPL-3.0-only

package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/kernel"
)

// record records the duration of one request until its response headers, or until it failed
// (ADR-0056 §1). The destination is never a label.
func (c *Client) record(ctx context.Context, r *kernel.HTTPRequest, resp *kernel.HTTPResponse, err error, d time.Duration) {
	l := kernel.NewLabels(sysmetrics.LabelHTTPPurpose, purposeLabel(r.Purpose), sysmetrics.LabelHTTPMethod, methodLabel(r.Method))
	if resp != nil {
		l = l.With(sysmetrics.LabelHTTPStatusClass, statusClass(resp.StatusCode))
	}
	if err != nil {
		l = l.With(sysmetrics.LabelErrorType, errorType(err))
	}
	c.duration.Record(context.WithoutCancel(ctx), d.Seconds(), l)
}

// purposeLabel returns the my.http.purpose of p.
func purposeLabel(p kernel.HTTPPurpose) string {
	switch p {
	case kernel.HTTPPurposeJSON:
		return "json"
	case kernel.HTTPPurposeHTML:
		return "html"
	case kernel.HTTPPurposeImage:
		return "image"
	case kernel.HTTPPurposeVideo:
		return "video"
	case kernel.HTTPPurposeHumanCheck:
		return "human_check"
	case kernel.HTTPPurposeWebPush:
		return "web_push"
	case kernel.HTTPPurposeWebhook:
		return "webhook"
	case kernel.HTTPPurposeClientMetadata:
		return "client_metadata"
	case kernel.HTTPPurposeLoginProvider:
		return "login_provider"
	}
	return "_OTHER"
}

// methodLabel returns the http.request.method of m: a method of RFC 9110 or PATCH, and _OTHER
// for any other, as the semantic conventions ask.
func methodLabel(m string) string {
	switch m {
	case "":
		return http.MethodGet
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		return m
	}
	return "_OTHER"
}

// statusClass returns the class of a status code: 1xx to 5xx.
func statusClass(code int) string {
	if code < 100 || code > 599 {
		return "_OTHER"
	}
	return string(rune('0'+code/100)) + "xx"
}

// errorType returns the error.type of a failed request: whether the SSRF guard refused the
// destination (forbidden_destination), another refusal of the client, or how the request
// failed on the network.
func errorType(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, kernel.ErrForbiddenDestination):
		return "forbidden_destination"
	case errors.Is(err, kernel.ErrTooManyRedirects):
		return "too_many_redirects"
	case errors.Is(err, kernel.ErrResponseTooLarge):
		return "response_too_large"
	case errors.Is(err, kernel.ErrInvalidHTTPRequest):
		return "invalid_request"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	return "_OTHER"
}
