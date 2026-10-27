// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/federation"
)

// JobProbeInstances fetches the NodeInfo of unavailable servers and marks those that answer
// available again (federation.md §7.3). The scheduler enqueues it once a day; one job handles
// ProbeBatch servers and enqueues the job of the next batch.
const JobProbeInstances = "federation.probe_instances"

// ProbeInterval is how often the probe runs.
const ProbeInterval = 24 * time.Hour

// ProbeBatch is the number of servers one JobProbeInstances probes.
const ProbeBatch = 50

// nodeInfoSchemaPrefix is the start of the rel of the NodeInfo 2.x documents
// (https://nodeinfo.diaspora.software/protocol).
const nodeInfoSchemaPrefix = "http://nodeinfo.diaspora.software/ns/schema/2."

// ProbeDeps are the ports of the probe.
type ProbeDeps struct {
	Repository Repository
	Health     *Health
	// HTTP fetches the NodeInfo documents, unsigned.
	HTTP kernel.HTTPClient
	// Domains and Host are the domain blocks of fetches (Fetcher): the probe never requests a
	// suspended server or this instance's Host, neither first nor on a redirect hop. A nil
	// Domains refuses only Host.
	Domains DomainPolicy
	Host    string
	// Jobs enqueues the next batch, outside any transaction.
	Jobs queue.JobQueue
	// Batch is the number of servers per job; zero means ProbeBatch.
	Batch int
	// Logger logs servers that stay unavailable; nil means slog.Default().
	Logger *slog.Logger
	// Modes reads the federation mode (ADR-0076): the probe reads NodeInfo (identify) unless it
	// is off. nil federates both ways.
	Modes ModeSettings
}

// Prober runs JobProbeInstances.
type Prober struct {
	d ProbeDeps
	// policy applies the domain blocks of fetches to the probe's requests.
	policy *Fetcher
}

// NewProber returns the probe on d.
func NewProber(d ProbeDeps) *Prober {
	if d.Batch <= 0 {
		d.Batch = ProbeBatch
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	domains := d.Domains
	if domains == nil {
		domains = NoDomainRestrictions{}
	}
	return &Prober{d: d, policy: NewFetcher(FetcherDeps{Domains: domains, Host: d.Host})}
}

// probePayload is the payload of JobProbeInstances: the cursor, the last domain of the previous
// batch ("" or no payload for the first).
type probePayload struct {
	After string `json:"after,omitempty"`
}

// RegisterJobs registers the JobProbeInstances handler and its daily schedule.
func (p *Prober) RegisterJobs(h *queue.JobHandlerRegistry, s *queue.Scheduler) {
	h.Register(JobProbeInstances, p.handle)
	s.Add(queue.Schedule{Name: JobProbeInstances, Every: ProbeInterval, Job: queue.NewJob{Type: JobProbeInstances}})
}

func (p *Prober) handle(ctx context.Context, j queue.Job) queue.Result {
	var pl probePayload
	if len(j.Payload) > 0 && string(j.Payload) != "null" {
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return queue.Fail(fmt.Errorf("federation: probe payload %s: %w", j.Payload, err))
		}
	}
	if err := p.Run(ctx, pl.After); err != nil {
		return queue.Retry(0, err)
	}
	return queue.Done()
}

// Run probes the batch of unavailable servers after the domain after, marks those that answer
// available again, and enqueues the next batch when this one was full. It is safe to repeat.
func (p *Prober) Run(ctx context.Context, after string) error {
	if ok, err := modeAllows(ctx, p.d.Modes, federation.FlowIdentify); err != nil || !ok {
		return err
	}
	servers, err := p.d.Repository.ListUnavailableInstances(ctx, after, p.d.Batch)
	if err != nil {
		return err
	}
	for _, s := range servers {
		if err := p.probe(ctx, s.Domain); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.d.Logger.InfoContext(ctx, "server still unavailable", slog.String("domain", s.Domain), slog.Any("reason", err))
			continue
		}
		if err := p.d.Health.Reached(ctx, s.Domain, federation.ReasonProbeSucceeded); err != nil {
			return err
		}
	}
	if len(servers) < p.d.Batch {
		return nil
	}
	last := servers[len(servers)-1].Domain
	b, err := json.Marshal(probePayload{After: last})
	if err != nil {
		return err
	}
	return p.d.Jobs.Enqueue(ctx, queue.NewJob{Type: JobProbeInstances, Payload: b, IdempotencyKey: "probe_instances:" + last})
}

// errProbe reports a server whose NodeInfo could not be read.
var errProbe = errors.New("federation: NodeInfo not available")

// probe fetches the NodeInfo discovery document of domain and the NodeInfo 2.x document it links
// to on the same host. It returns nil when both answer 2xx with JSON.
func (p *Prober) probe(ctx context.Context, domain string) error {
	var disco struct {
		Links []struct {
			Rel  string `json:"rel"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if err := p.getJSON(ctx, "https://"+domain+"/.well-known/nodeinfo", &disco); err != nil {
		return err
	}
	for _, l := range disco.Links {
		if !strings.HasPrefix(l.Rel, nodeInfoSchemaPrefix) {
			continue
		}
		u, err := url.Parse(l.Href)
		if err != nil || !strings.EqualFold(u.Host, domain) {
			continue
		}
		var doc map[string]any
		return p.getJSON(ctx, u.String(), &doc)
	}
	return fmt.Errorf("%w: %s links no NodeInfo 2.x document on its host", errProbe, domain)
}

func (p *Prober) getJSON(ctx context.Context, u string, v any) error {
	if err := p.policy.checkRedirect(ctx, u); err != nil {
		return err
	}
	h := http.Header{}
	h.Set("Accept", "application/json")
	resp, err := p.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodGet, URL: u, Header: h, Purpose: kernel.HTTPPurposeJSON, CheckRedirect: p.policy.checkRedirect,
	})
	if err != nil {
		return err
	}
	body, err := resp.ReadAll()
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %s answered %d", errProbe, u, resp.StatusCode)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: %s: %w", errProbe, u, err)
	}
	return nil
}
