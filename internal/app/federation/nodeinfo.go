// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// nodeInfoSchemas are the NodeInfo schemas read, the preferred first.
var nodeInfoSchemas = []string{
	"http://nodeinfo.diaspora.software/ns/schema/2.2",
	"http://nodeinfo.diaspora.software/ns/schema/2.1",
	"http://nodeinfo.diaspora.software/ns/schema/2.0",
}

// errNoNodeInfo reports a server whose NodeInfo cannot be read.
var errNoNodeInfo = errors.New("federation: no usable NodeInfo")

type observePayload struct {
	Domain string `json:"domain"`
}

func (r *RemoteActors) handleObserve(ctx context.Context, j queue.Job) queue.Result {
	var p observePayload
	if err := json.Unmarshal(j.Payload, &p); err != nil || p.Domain == "" {
		return queue.Fail(fmt.Errorf("federation: observe payload %s: %w", j.Payload, err))
	}
	err := r.ObserveInstance(ctx, p.Domain)
	switch {
	case err == nil:
		return queue.Done()
	case errors.Is(err, errNoNodeInfo):
		r.d.Logger.InfoContext(ctx, "NodeInfo not observed", slog.String("domain", p.Domain), slog.Any("reason", err))
		return queue.Done()
	}
	return queue.Retry(0, err)
}

// ObserveInstance reads the software a server reports in its NodeInfo 2.x
// (/.well-known/nodeinfo, then the linked document on the same host) and records it in
// instance_nodeinfo_observations when it differs from the last observation. A server without a
// usable NodeInfo fails with an error wrapping errNoNodeInfo.
func (r *RemoteActors) ObserveInstance(ctx context.Context, domain string) error {
	host, err := CanonicalHost(domain)
	if err != nil {
		return fmt.Errorf("%w: %w", errNoNodeInfo, err)
	}
	var links struct {
		Links []struct {
			Rel  string `json:"rel"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if err := r.getJSON(ctx, "https://"+host+"/.well-known/nodeinfo", &links); err != nil {
		return err
	}
	href := ""
	for _, schema := range nodeInfoSchemas {
		for _, l := range links.Links {
			if l.Rel == schema && href == "" {
				href = l.Href
			}
		}
	}
	doc, err := CanonicalIRI(href)
	if err != nil || doc.Host() != host || !strings.HasPrefix(string(doc), "https://") {
		return fmt.Errorf("%w: %s links NodeInfo %q", errNoNodeInfo, host, href)
	}
	var info struct {
		Software struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"software"`
	}
	if err := r.getJSON(ctx, string(doc), &info); err != nil {
		return err
	}
	o := federation.NodeInfoObservation{
		Domain:          host,
		SoftwareName:    federation.NormalizeSoftware(info.Software.Name, true),
		SoftwareVersion: federation.NormalizeSoftware(info.Software.Version, false),
		ObservedAt:      r.d.Clock.Now(),
	}
	if o.SoftwareName == "" {
		return fmt.Errorf("%w: %s reports no software name", errNoNodeInfo, host)
	}
	return r.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		last, err := r.d.Instances.LatestNodeInfoObservation(ctx, tx, host)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return err
		case last.SameSoftware(o):
			return nil
		}
		o.ID = r.d.IDs.NewID()
		return r.d.Instances.InsertNodeInfoObservation(ctx, tx, o)
	})
}

// getJSON reads a public JSON document. Refusals and answers that are not JSON fail with an
// error wrapping errNoNodeInfo; network errors and server errors are returned as they are, so
// that the job tries again.
func (r *RemoteActors) getJSON(ctx context.Context, u string, v any) error {
	if err := r.identifies(ctx); err != nil {
		return fmt.Errorf("%w: %w", errNoNodeInfo, err)
	}
	resp, err := r.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodGet, URL: u, Header: http.Header{"Accept": {"application/json"}}, Purpose: kernel.HTTPPurposeJSON,
		CheckRedirect: r.d.Fetcher.redirectPolicy(),
	})
	switch {
	case errors.Is(err, ErrFetchRefused), errors.Is(err, kernel.ErrForbiddenDestination), errors.Is(err, kernel.ErrInvalidHTTPRequest),
		errors.Is(err, kernel.ErrTooManyRedirects), errors.Is(err, kernel.ErrResponseTooLarge):
		return fmt.Errorf("%w: %s: %w", errNoNodeInfo, u, err)
	case err != nil:
		return err
	}
	body, err := resp.ReadAll()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("federation: %s answered %d", u, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return fmt.Errorf("%w: %s answered %d", errNoNodeInfo, u, resp.StatusCode)
	case errors.Is(err, kernel.ErrResponseTooLarge):
		return fmt.Errorf("%w: %s: %w", errNoNodeInfo, u, err)
	case err != nil:
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: %s: %w", errNoNodeInfo, u, err)
	}
	return nil
}
