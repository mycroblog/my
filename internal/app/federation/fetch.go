// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The fetch rules of federation.md §6.1.
const (
	// FetchFailureTTL is how long a failed fetch of a URI is remembered, so that the URI is not
	// fetched again in the meantime.
	FetchFailureTTL = time.Hour
	// FetchFailureNamespace is the KeyValueStore namespace of that memory, keyed by the SHA-256
	// of the URI.
	FetchFailureNamespace = "federation.fetch_failure"
	// ActivityStreamsAccept is the Accept header of every fetch.
	ActivityStreamsAccept = `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`
	activityStreamsNS     = "https://www.w3.org/ns/activitystreams"
)

var (
	// ErrFetchGone reports a document whose server answered 410 Gone.
	ErrFetchGone = errors.New("federation: the remote document is gone")
	// ErrFetchNotFound reports a document that does not exist or cannot be used: 404 or another
	// refusal, a response that is not ActivityStreams, a document that cannot be normalized,
	// or one whose id does not have the origin it was served from.
	ErrFetchNotFound = errors.New("federation: the remote document is not usable")
	// ErrFetchFailed reports a fetch that got no usable answer: a network error, a timeout,
	// 429, or a server error.
	ErrFetchFailed = errors.New("federation: the remote document could not be fetched")
	// ErrFetchUnavailable reports a server that is marked unavailable (federation.md §7.3), so
	// that nothing is fetched from it now.
	ErrFetchUnavailable = errors.New("federation: the remote server is unavailable")
	// ErrFetchRefused reports a URI of this instance or of a server whose federation is
	// suspended (federation.md §11).
	ErrFetchRefused = errors.New("federation: fetching from this server is refused")
)

// FetcherDeps are the ports and settings a Fetcher uses.
type FetcherDeps struct {
	// Client signs every fetch (federation.md §2).
	Client    *SignedHTTPClient
	KV        kernel.KeyValueStore
	Clock     kernel.Clock
	Domains   DomainPolicy
	Instances InstanceStates
	// Key returns the key that signs fetches: the RSA key of the instance actor.
	Key func(ctx context.Context) (SigningKey, error)
	// Host is this instance's host, whose URIs are never fetched (federation.md §1.2).
	Host string
	// Modes reads the federation mode (ADR-0076): Fetch takes content in (ingest) and FetchActor
	// fetches actors and their keys (identify). nil federates both ways.
	Modes ModeSettings
}

// Fetcher fetches remote ActivityPub documents (federation.md §6.1): a GET signed with the
// instance actor's key that accepts only ActivityStreams, keeps a document only when its id has
// the origin it was served from (fetching the id once more when it differs from the requested
// URI), and remembers failures for FetchFailureTTL.
type Fetcher struct {
	d FetcherDeps
}

// NewFetcher returns a Fetcher on d.
func NewFetcher(d FetcherDeps) *Fetcher { return &Fetcher{d: d} }

// Fetched is a fetched and normalized document.
type Fetched struct {
	Document *federation.Document
	// Raw is the document as served.
	Raw []byte
}

// Fetch fetches the document with the IRI uri (its fragment is ignored) to take it in: a post, a
// thread, a collection. It fails with an error wrapping ErrFetchGone, ErrFetchNotFound,
// ErrFetchFailed, ErrFetchUnavailable, or ErrFetchRefused; other errors come from the stores. A
// fetch that the federation mode does not make (ingest, ADR-0076) fails with ErrFetchRefused and
// ErrFederationRestricted.
func (f *Fetcher) Fetch(ctx context.Context, uri string) (*Fetched, error) {
	return f.fetch(ctx, uri, federation.FlowIngest)
}

// FetchActor is Fetch of an actor or of its key, which verifying signatures and addressing
// deliveries need: the federation mode makes it unless it is off (identify, ADR-0076).
func (f *Fetcher) FetchActor(ctx context.Context, uri string) (*Fetched, error) {
	return f.fetch(ctx, uri, federation.FlowIdentify)
}

// fetch is Fetch of a document of flow.
func (f *Fetcher) fetch(ctx context.Context, uri string, flow federation.Flow) (*Fetched, error) {
	target, err := fetchTarget(uri)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchNotFound, err)
	}
	switch ok, err := modeAllows(ctx, f.d.Modes, flow); {
	case err != nil:
		return nil, err
	case !ok:
		return nil, fmt.Errorf("%w: %w: %s (%s)", ErrFetchRefused, ErrFederationRestricted, target, flow)
	}
	if err := f.check(ctx, target); err != nil {
		return nil, err
	}
	memory := failureKey(target)
	if v, err := f.d.KV.Get(ctx, FetchFailureNamespace, memory); err == nil {
		return nil, fmt.Errorf("%s fetched recently: %w", target, rememberedFailure(string(v)))
	} else if !errors.Is(err, kernel.ErrKeyNotFound) {
		return nil, fmt.Errorf("read fetch failure memory: %w", err)
	}

	got, err := f.fetchOnce(ctx, target)
	if err == nil && got.Document.ID() != target {
		// The id differs from the URI: fetch the id once and require that it names itself.
		id := got.Document.ID()
		if got, err = f.fetchOnce(ctx, id); err == nil && got.Document.ID() != id {
			err = fmt.Errorf("%w: %s names %s", ErrFetchNotFound, id, got.Document.ID())
		}
	}
	if err != nil {
		if kind := failureKind(err); kind != "" && ctx.Err() == nil {
			if serr := f.d.KV.Set(ctx, FetchFailureNamespace, memory, []byte(kind), FetchFailureTTL); serr != nil {
				return nil, errors.Join(err, fmt.Errorf("write fetch failure memory: %w", serr))
			}
		}
		return nil, err
	}
	return got, nil
}

// check refuses URIs of this instance, of a suspended server, and of an unavailable one.
func (f *Fetcher) check(ctx context.Context, target federation.IRI) error {
	host := target.Host()
	if err := f.refuses(ctx, host); err != nil {
		return err
	}
	state, err := f.d.Instances.InstanceState(ctx, host)
	if err != nil {
		return err
	}
	if state == federation.InstanceUnavailable {
		return fmt.Errorf("%w: %s", ErrFetchUnavailable, host)
	}
	return nil
}

// refuses fails with ErrFetchRefused for host when it is this instance or a suspended server
// (a domain block of federation, federation.md §6.1).
func (f *Fetcher) refuses(ctx context.Context, host string) error {
	if strings.EqualFold(host, f.d.Host) {
		return fmt.Errorf("%w: %s is local", ErrFetchRefused, host)
	}
	suspended, err := f.d.Domains.FederationSuspended(ctx, host)
	if err != nil {
		return err
	}
	if suspended {
		return fmt.Errorf("%w: %s", ErrFetchRefused, host)
	}
	return nil
}

// checkRedirect is the kernel.HTTPRequest.CheckRedirect of the requests to remote servers: a
// redirect hop to this instance or to a suspended server ends the request with ErrFetchRefused.
func (f *Fetcher) checkRedirect(ctx context.Context, u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFetchRefused, err)
	}
	host, err := CanonicalHost(parsed.Hostname())
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFetchRefused, err)
	}
	return f.refuses(ctx, host)
}

// redirectPolicy returns f.checkRedirect, or nil for a nil f.
func (f *Fetcher) redirectPolicy() func(context.Context, string) error {
	if f == nil {
		return nil
	}
	return f.checkRedirect
}

func (f *Fetcher) fetchOnce(ctx context.Context, target federation.IRI) (*Fetched, error) {
	if err := f.check(ctx, target); err != nil {
		return nil, err
	}
	key, err := f.d.Key(ctx)
	if err != nil {
		return nil, fmt.Errorf("federation: signing key for fetches: %w", err)
	}
	resp, err := f.d.Client.Do(ctx, &kernel.HTTPRequest{
		Method: http.MethodGet, URL: string(target), Header: http.Header{"Accept": {ActivityStreamsAccept}},
		Purpose: kernel.HTTPPurposeJSON, CheckRedirect: f.checkRedirect,
	}, key)
	switch {
	case errors.Is(err, ErrFetchRefused):
		return nil, fmt.Errorf("%s: %w", target, err)
	case errors.Is(err, kernel.ErrForbiddenDestination), errors.Is(err, kernel.ErrInvalidHTTPRequest), errors.Is(err, kernel.ErrResponseTooLarge):
		return nil, fmt.Errorf("%w: %s: %w", ErrFetchNotFound, target, err)
	case err != nil:
		return nil, fmt.Errorf("%w: %s: %w", ErrFetchFailed, target, err)
	}
	if err := statusError(target, resp.StatusCode); err != nil {
		discard(resp.HTTPResponse)
		return nil, err
	}
	if !isActivityStreams(resp.Header.Get("Content-Type")) {
		discard(resp.HTTPResponse)
		return nil, fmt.Errorf("%w: %s is %q, not ActivityStreams", ErrFetchNotFound, target, resp.Header.Get("Content-Type"))
	}
	body, err := resp.ReadAll()
	switch {
	case errors.Is(err, kernel.ErrResponseTooLarge):
		return nil, fmt.Errorf("%w: %s: %w", ErrFetchNotFound, target, err)
	case err != nil:
		return nil, fmt.Errorf("%w: %s: %w", ErrFetchFailed, target, err)
	}
	doc, err := Normalize(body, f.d.Clock.Now())
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrFetchNotFound, target, err)
	}
	id := doc.ID()
	final, err := CanonicalIRI(resp.URL)
	if id == "" || err != nil || !federation.SameOrigin(final, id) {
		return nil, fmt.Errorf("%w: %s served a document with id %q", ErrFetchNotFound, resp.URL, id)
	}
	return &Fetched{Document: doc, Raw: body}, nil
}

// fetchTarget returns the canonical IRI to fetch for uri: without its fragment, https only.
func fetchTarget(uri string) (federation.IRI, error) {
	iri, err := CanonicalIRI(uri)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(string(iri))
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("%q is not an https URI", uri)
	}
	u.Fragment, u.RawFragment = "", ""
	return federation.IRI(u.String()), nil
}

func statusError(target federation.IRI, status int) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusGone:
		return fmt.Errorf("%w: %s", ErrFetchGone, target)
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500:
		return fmt.Errorf("%w: %s answered %d", ErrFetchFailed, target, status)
	}
	return fmt.Errorf("%w: %s answered %d", ErrFetchNotFound, target, status)
}

// isActivityStreams reports whether a Content-Type is an ActivityStreams document:
// application/activity+json, or application/ld+json with the ActivityStreams profile.
func isActivityStreams(contentType string) bool {
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mt {
	case "application/activity+json":
		return true
	case "application/ld+json":
		for p := range strings.FieldsSeq(params["profile"]) {
			if p == activityStreamsNS {
				return true
			}
		}
	}
	return false
}

func failureKey(target federation.IRI) string {
	sum := sha256.Sum256([]byte(target))
	return hex.EncodeToString(sum[:])
}

// failureKind is the remembered value of a failed fetch, "" for failures that are not
// remembered (refusals that the next fetch checks anyway, and errors of the stores).
func failureKind(err error) string {
	switch {
	case errors.Is(err, ErrFetchGone):
		return "gone"
	case errors.Is(err, ErrFetchNotFound):
		return "not_found"
	case errors.Is(err, ErrFetchFailed):
		return "failed"
	}
	return ""
}

func rememberedFailure(kind string) error {
	switch kind {
	case "gone":
		return ErrFetchGone
	case "not_found":
		return ErrFetchNotFound
	}
	return ErrFetchFailed
}
