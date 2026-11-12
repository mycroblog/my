// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/diagnostics"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The UX measurements of the web client (ADR-0167 §3): POST /client_measurements takes the
// samples by the path of the error reports (client_errors.go), and GET /admin/ux_measurements
// sums them for the operators, with manage_errors.

// UXMeasurements counts the UX measurements and sums them. *appdiagnostics.UX implements it.
type UXMeasurements interface {
	Record(ctx context.Context, ms []appdiagnostics.UXMeasurement)
	LastHours(ctx context.Context, hours int) (time.Time, []appdiagnostics.UXSummary, error)
}

// clientMeasurementsPattern is the http.ServeMux pattern of reportClientMeasurements.
const clientMeasurementsPattern = "POST " + BasePath + "/client_measurements"

// ClientMeasurementLimit bounds the reports of UX measurements from one client IP, signed in or
// not.
var ClientMeasurementLimit = kernel.Limit{Name: "api.client_measurements", Max: 10, Window: time.Minute}

// Bounds of a report of UX measurements.
const (
	// maxClientMeasurementBody is the largest body that the endpoint reads.
	maxClientMeasurementBody = 16 << 10
	// maxClientMeasurements is the most samples of one report; a report with more is ignored.
	maxClientMeasurements = 50
)

// Bounds of the window of GET /admin/ux_measurements, in hours.
const (
	defaultUXMeasurementHours = 24
	maxUXMeasurementHours     = 720
)

// registerClientMeasurements registers POST /client_measurements (operation
// reportClientMeasurements), which the strict server does not generate.
func (s *Server) registerClientMeasurements(r Router) {
	r.Handle(clientMeasurementsPattern, s.guard(clientMeasurementsPattern, http.HandlerFunc(s.serveClientMeasurements)))
}

// serveClientMeasurements serves POST /client_measurements: always 204. It ignores a body that is
// not application/json, is over maxClientMeasurementBody, or is not a report of at most
// maxClientMeasurements samples, the reports of a client IP past ClientMeasurementLimit, and
// every report while the setting ux_measurement is off. UXMeasurements drops the samples whose
// name or value is not one of their metric.
func (s *Server) serveClientMeasurements(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if s.o.UXMeasurements == nil {
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return
	}
	if r.ContentLength > maxClientMeasurementBody {
		// The rest of the body is not read; the connection is closed instead of drained.
		w.Header().Set("Connection", "close")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxClientMeasurementBody+1))
	if err != nil || len(body) > maxClientMeasurementBody {
		w.Header().Set("Connection", "close")
		return
	}
	if !s.takeClientMeasurements(r) || !s.uxMeasurementOn(r.Context()) {
		return
	}
	var rep ClientMeasurementReport
	if err := json.Unmarshal(body, &rep); err != nil || len(rep.Measurements) > maxClientMeasurements {
		return
	}
	ms := make([]appdiagnostics.UXMeasurement, len(rep.Measurements))
	for i, m := range rep.Measurements {
		ms[i] = appdiagnostics.UXMeasurement{Metric: diagnostics.UXMetric(m.Metric), Name: m.Name, Value: m.Value}
	}
	s.o.UXMeasurements.Record(r.Context(), ms)
}

// takeClientMeasurements takes one report of the request's client IP from ClientMeasurementLimit.
// Like the other limits it lets the report through without a RateLimiter or when the store fails.
func (s *Server) takeClientMeasurements(r *http.Request) bool {
	if s.o.RateLimiter == nil {
		return true
	}
	d, err := s.o.RateLimiter.Take(r.Context(), s.o.ClientMeasurementLimit, s.clientKey(r))
	if err != nil {
		s.o.Logger.WarnContext(r.Context(), "client api rate limit unavailable", slog.String("limit", s.o.ClientMeasurementLimit.Name), slog.Any("error", err))
		return true
	}
	return d.Allowed
}

// uxMeasurementOn reports whether the setting ux_measurement is on: its default without
// Instance, and off when the settings cannot be read, so that nothing is taken that the setting
// may refuse.
func (s *Server) uxMeasurementOn(ctx context.Context) bool {
	if s.o.Instance == nil {
		return true
	}
	settings, err := s.o.Instance.Settings(ctx)
	if err != nil {
		s.o.Logger.WarnContext(ctx, "client api: instance settings unavailable", slog.Any("error", err))
		return false
	}
	return settings.UXMeasurement()
}

// ListUXMeasurements serves GET /api/v1/admin/ux_measurements.
func (s *Server) ListUXMeasurements(ctx context.Context, req ListUXMeasurementsRequestObject) (ListUXMeasurementsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageErrors); err != nil {
		return nil, err
	} else if p != nil {
		return ListUXMeasurementsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if s.o.UXMeasurements == nil {
		return ListUXMeasurementsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	hours := defaultUXMeasurementHours
	if h := req.Params.Hours; h != nil {
		if *h < 1 || *h > maxUXMeasurementHours {
			return ListUXMeasurementsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "hours")), nil
		}
		hours = *h
	}
	since, sums, err := s.o.UXMeasurements.LastHours(ctx, hours)
	if err != nil {
		return nil, err
	}
	out := UXMeasurementList{Since: since, Items: make([]UXMeasurementSummary, len(sums))}
	for i, sm := range sums {
		out.Items[i] = newUXMeasurementSummary(sm)
	}
	cache := cacheNoStore
	return ListUXMeasurements200JSONResponse{Body: out, Headers: ListUXMeasurements200ResponseHeaders{CacheControl: &cache}}, nil
}

// newUXMeasurementSummary presents a summary as UXMeasurementSummary.
func newUXMeasurementSummary(sm appdiagnostics.UXSummary) UXMeasurementSummary {
	bounds := diagnostics.UXBounds(sm.Metric)
	out := UXMeasurementSummary{
		Metric: UXMetric(sm.Metric), Name: sm.Name, Samples: sm.Samples, Buckets: make([]UXMeasurementBucket, len(sm.Buckets)),
	}
	for i, n := range sm.Buckets {
		out.Buckets[i].Samples = n
		if i < len(bounds) {
			le := bounds[i]
			out.Buckets[i].Le = &le
		}
	}
	if p75, ok := diagnostics.UXQuantileBound(sm.Metric, sm.Buckets, 0.75); ok {
		out.P75 = &p75
	}
	return out
}
