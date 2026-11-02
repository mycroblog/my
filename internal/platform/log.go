// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// NewLogger returns the process logger: JSON lines on w at level and above. Every record
// logged with a request's context carries that request's ID as request_id, and every record
// carries the severity of Cloud Logging next to its level (logSeverity).
func NewLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	json := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: logSeverity})
	return slog.New(requestIDHandler{clientGoneHandler{json}})
}

// logSeverity is the ReplaceAttr of the process logger: it writes the record's level as level,
// as before (the JSON log format of Lambda and the examples of alerts read it), followed by
// severity, which Cloud Logging reads instead (ADR-0103 §4 (1)): DEBUG, INFO, WARNING, ERROR,
// and CRITICAL above ERROR. Only the record's own level is a slog.Level at the top level.
func logSeverity(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 || a.Key != slog.LevelKey {
		return a
	}
	level, ok := a.Value.Any().(slog.Level)
	if !ok {
		return a
	}
	// A group without a key is written inline, as two attributes; their values are strings,
	// so that ReplaceAttr leaves them alone.
	return slog.Attr{Value: slog.GroupValue(slog.String(slog.LevelKey, level.String()), slog.String(logSeverityKey, severityOf(level)))}
}

// logSeverityKey is the key of the severity of Cloud Logging.
const logSeverityKey = "severity"

// severityOf returns the Cloud Logging severity of level.
func severityOf(level slog.Level) string {
	switch {
	case level > slog.LevelError:
		return "CRITICAL"
	case level > slog.LevelWarn:
		return "ERROR"
	case level > slog.LevelInfo:
		return "WARNING"
	case level > slog.LevelDebug:
		return "INFO"
	default:
		return "DEBUG"
	}
}

// requestIDHandler adds the request ID found in the context to every record.
type requestIDHandler struct{ slog.Handler }

func (h requestIDHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h requestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDHandler{h.Handler.WithAttrs(attrs)}
}

func (h requestIDHandler) WithGroup(name string) slog.Handler {
	return requestIDHandler{h.Handler.WithGroup(name)}
}

// clientGoneHandler writes the ERROR lines of an HTTP request whose client has left at INFO: the
// request's context was canceled, so its error is most likely only that, and nothing here needs
// the operator (see clientGone for the error reports). The other records pass unchanged.
type clientGoneHandler struct{ slog.Handler }

func (h clientGoneHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelError && clientLeft(ctx) {
		r.Level = slog.LevelInfo
		if !h.Enabled(ctx, r.Level) {
			return nil
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h clientGoneHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return clientGoneHandler{h.Handler.WithAttrs(attrs)}
}

func (h clientGoneHandler) WithGroup(name string) slog.Handler {
	return clientGoneHandler{h.Handler.WithGroup(name)}
}

// clientLeft reports whether ctx is the context of an HTTP request whose client has left. The
// server cancels that context when the connection closes.
func clientLeft(ctx context.Context) bool {
	if !errors.Is(ctx.Err(), context.Canceled) {
		return false
	}
	_, ok := ctx.Value(requestRouteKey{}).(*requestRoute)
	return ok
}

type requestIDKey struct{}

// RequestID returns the ID of the HTTP request that ctx belongs to, or "" outside a request.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
