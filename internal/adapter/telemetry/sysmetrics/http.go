// SPDX-License-Identifier: AGPL-3.0-only

package sysmetrics

import "github.com/mycroblog/my/internal/app/kernel"

// Label keys of the outbound HTTP metrics. The destination is never a label (ADR-0055 §10).
const (
	LabelHTTPPurpose     = "my.http.purpose"
	LabelHTTPMethod      = "http.request.method"
	LabelHTTPStatusClass = "my.http.status_class"
	LabelErrorType       = "error.type"
)

// The outbound HTTP metrics of ADR-0055 §10, measured by internal/adapter/httpclient.
var (
	HTTPClientRequestDuration = kernel.Instrument{
		Name: "http.client.request.duration", Unit: "s",
		Description: "Duration of outbound HTTP requests until the response headers, by purpose.",
		LabelKeys:   []string{LabelHTTPPurpose, LabelHTTPMethod, LabelHTTPStatusClass, LabelErrorType},
		Buckets:     kernel.DurationBuckets,
	}
	HTTPClientOpenConnections = kernel.Instrument{
		Name: "http.client.open_connections", Unit: "{connection}",
		Description: "Open connections of the outbound HTTP client.",
	}
)
