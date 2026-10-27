// SPDX-License-Identifier: AGPL-3.0-only

// Package otlptest is an OTLP/HTTP trace and log receiver for tests: it decodes the spans that the
// otel Tracer exports and the log records of the otlp error exporter, so that tests outside the
// OpenTelemetry adapters check what leaves the process without importing OpenTelemetry
// (ADR-0063 §1, ADR-0103 §4 (3)).
package otlptest

import (
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Span is one exported span. IDs are lower-case hex; a root has no ParentSpanID.
type Span struct {
	Name                          string
	TraceID, SpanID, ParentSpanID string
	// Kind is server, client, internal, producer, or consumer.
	Kind string
	// Links are the span IDs that the span links to.
	Links []string
	// Attrs are the attributes, integers in decimal.
	Attrs map[string]string
	// Resource are the attributes of the resource of the span.
	Resource map[string]string
	// Gzip reports whether the request that carried the span was gzip-compressed.
	Gzip bool
}

// Collector is an http.Handler that takes OTLP/HTTP trace exports (protobuf, gzip or not) and
// keeps their spans. It is safe for concurrent use.
type Collector struct {
	mu    sync.Mutex
	spans []Span
}

// ServeHTTP implements http.Handler.
func (c *Collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req coltracepb.ExportTraceServiceRequest
	_, gz, ok := readRequest(w, r, &req)
	if !ok {
		return
	}
	var spans []Span
	for _, rs := range req.GetResourceSpans() {
		res := attrs(rs.GetResource().GetAttributes())
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				out := Span{
					Name: s.GetName(), TraceID: hex.EncodeToString(s.GetTraceId()), SpanID: hex.EncodeToString(s.GetSpanId()),
					ParentSpanID: hex.EncodeToString(s.GetParentSpanId()), Kind: kind(s.GetKind()),
					Attrs: attrs(s.GetAttributes()), Resource: res, Gzip: gz,
				}
				for _, l := range s.GetLinks() {
					out.Links = append(out.Links, hex.EncodeToString(l.GetSpanId()))
				}
				spans = append(spans, out)
			}
		}
	}
	c.mu.Lock()
	c.spans = append(c.spans, spans...)
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/x-protobuf")
}

// Spans returns the spans received so far, in the order they arrived.
func (c *Collector) Spans() []Span {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.spans)
}

// readRequest decodes the protobuf body of r, gzip-compressed or not, into m and returns the
// decompressed body and whether it was compressed; on failure it answers 400 and returns false.
func readRequest(w http.ResponseWriter, r *http.Request, m proto.Message) (body []byte, gz, ok bool) {
	var rd io.Reader = r.Body
	gz = r.Header.Get("Content-Encoding") == "gzip"
	if gz {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return nil, gz, false
		}
		rd = zr
	}
	b, err := io.ReadAll(rd)
	if err == nil {
		err = proto.Unmarshal(b, m)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, gz, false
	}
	return b, gz, true
}

// Log is one exported log record. IDs are lower-case hex, empty when the record has none.
type Log struct {
	EventName       string
	TimeUnixNano    uint64
	SeverityNumber  int
	SeverityText    string
	Body            string
	TraceID, SpanID string
	// Attrs are the attributes, integers in decimal.
	Attrs map[string]string
	// Resource are the attributes of the resource of the record.
	Resource map[string]string
	// Gzip reports whether the request that carried the record was gzip-compressed.
	Gzip bool
}

// LogCollector is an http.Handler that takes OTLP/HTTP log exports (protobuf, gzip or not) and
// keeps their records and the decompressed request bodies. It is safe for concurrent use.
type LogCollector struct {
	mu     sync.Mutex
	logs   []Log
	bodies [][]byte
}

// ServeHTTP implements http.Handler.
func (c *LogCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req collogspb.ExportLogsServiceRequest
	body, gz, ok := readRequest(w, r, &req)
	if !ok {
		return
	}
	var logs []Log
	for _, rl := range req.GetResourceLogs() {
		res := attrs(rl.GetResource().GetAttributes())
		for _, sl := range rl.GetScopeLogs() {
			for _, l := range sl.GetLogRecords() {
				logs = append(logs, Log{
					EventName: l.GetEventName(), TimeUnixNano: l.GetTimeUnixNano(),
					SeverityNumber: int(l.GetSeverityNumber()), SeverityText: l.GetSeverityText(),
					Body: l.GetBody().GetStringValue(), TraceID: hexID(l.GetTraceId()), SpanID: hexID(l.GetSpanId()),
					Attrs: attrs(l.GetAttributes()), Resource: res, Gzip: gz,
				})
			}
		}
	}
	c.mu.Lock()
	c.logs = append(c.logs, logs...)
	c.bodies = append(c.bodies, body)
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/x-protobuf")
}

// Logs returns the log records received so far, in the order they arrived.
func (c *LogCollector) Logs() []Log {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.logs)
}

// Bodies returns the decompressed bodies of the requests received so far.
func (c *LogCollector) Bodies() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.bodies)
}

// hexID returns the lower-case hex of id, empty for a missing or all-zero ID.
func hexID(id []byte) string {
	for _, b := range id {
		if b != 0 {
			return hex.EncodeToString(id)
		}
	}
	return ""
}

func attrs(kvs []*commonpb.KeyValue) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		v := kv.GetValue()
		switch v.GetValue().(type) {
		case *commonpb.AnyValue_IntValue:
			m[kv.GetKey()] = fmt.Sprint(v.GetIntValue())
		default:
			m[kv.GetKey()] = v.GetStringValue()
		}
	}
	return m
}

func kind(k tracepb.Span_SpanKind) string {
	switch k {
	case tracepb.Span_SPAN_KIND_SERVER:
		return "server"
	case tracepb.Span_SPAN_KIND_CLIENT:
		return "client"
	case tracepb.Span_SPAN_KIND_PRODUCER:
		return "producer"
	case tracepb.Span_SPAN_KIND_CONSUMER:
		return "consumer"
	}
	return "internal"
}
