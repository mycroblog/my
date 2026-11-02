// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file keeps the fetches of link cards polite to each host: one fetch at a time, a pause
// between fetches, and a longer one when the host answers 429 or 503. A fetch is one request with
// its redirects, and with the robots.txt it needs. No worker waits: a fetch that cannot start now
// fails with a *LinkCardPostponed and its job comes back later, without using an attempt. Every
// entry in the store has a TTL, so a crash stops nothing for longer than that.

const (
	// LinkCardHostInterval is the least time from the end of a fetch from a host to the start of
	// the next. RFC 9309 gives no number; one a second is a rate that no site should notice and
	// that still takes a burst of a few hundred links within minutes.
	LinkCardHostInterval = time.Second
	// DefaultLinkCardHostPause is how long fetches from a host stop after a 429 or a 503 without a
	// usable Retry-After: long enough to be felt by a site in trouble, and short enough that a
	// site with a passing failure loses little.
	DefaultLinkCardHostPause = time.Minute
	// MaxLinkCardHostPause bounds the pause that a Retry-After asks for, so that one answer cannot
	// stop a host's cards for long. It matches LinkCardRobotsUnreachableTTL.
	MaxLinkCardHostPause = time.Hour
	// LinkCardHostLockTTL is the longest a host counts as being fetched from after a worker
	// crashed. It is above what a fetch can take: the 30 s of a page or an image, and the 10 s of a
	// robots.txt at each of its redirects.
	LinkCardHostLockTTL = 2 * time.Minute
	// LinkCardHostLockNamespace is the KeyValueStore namespace of the lock of a fetch from a host,
	// keyed by the host.
	LinkCardHostLockNamespace = "content.link_card_host_lock"
	// LinkCardHostNotBeforeNamespace is the namespace of the time before which no fetch from a host
	// starts, in Unix milliseconds, keyed by the host. Only the holder of the lock writes it.
	LinkCardHostNotBeforeNamespace = "content.link_card_host_not_before"
	// LinkCardHostQueueNamespace is the namespace of the next time that a postponed job of a host
	// is told to come back, in Unix milliseconds, keyed by the host. It spreads the jobs of a burst
	// so that about one wakes up per LinkCardHostInterval; the lock and the time above enforce the
	// limits.
	LinkCardHostQueueNamespace = "content.link_card_host_queue"
	// LinkCardThrottleTries is how many times a URL may be answered 429 or 503 before it counts
	// like any other answer that makes no card: the page is remembered as failed, and a card goes
	// without the image. It ends the wait of a site that always answers so.
	LinkCardThrottleTries = 5
	// LinkCardThrottleNamespace is the namespace of those answers, keyed as the failure memory.
	// They are counted over LinkCardFailureTTL.
	LinkCardThrottleNamespace = "content.link_card_throttle"
	// LinkCardPendingNamespace is the namespace of a page whose card waits for its image, keyed as
	// the failure memory.
	LinkCardPendingNamespace = "content.link_card_pending"
	// LinkCardPendingTTL is how long that page is kept: above MaxLinkCardHostPause, so that a
	// paused host of the image costs no second request of the page, and short enough that the page
	// is still current.
	LinkCardPendingTTL = 2 * time.Hour
)

const labelHTTPStatus = "http.response.status_code"

// LinkCardHostPauses counts the pauses of fetches from hosts. No label names the host, which is
// not a finite value; the log line has it.
var LinkCardHostPauses = kernel.Instrument{
	Name: "my.link_card.host_pauses", Unit: "{pause}", Description: "Pauses of the link card fetches from a host, by the status that the host answered.",
	LabelKeys: []string{labelHTTPStatus},
}

// LinkCardPostponed reports a fetch that a host's limits hold back: its job comes back After later.
// Nothing about the URL is remembered as failed.
type LinkCardPostponed struct {
	Host  string
	After time.Duration
}

func (p *LinkCardPostponed) Error() string {
	return fmt.Sprintf("content: link card fetch from %s postponed for %s", p.Host, p.After)
}

// hostPauseStatus reports whether a status asks the crawler to stop for a while.
func hostPauseStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

// hostPause returns how long to stop for an answer with these headers at now.
func hostPause(h http.Header, now time.Time) time.Duration {
	d, ok := parseRetryAfter(h.Get("Retry-After"), now)
	if !ok {
		return DefaultLinkCardHostPause
	}
	return min(max(d, LinkCardHostInterval), MaxLinkCardHostPause)
}

// parseRetryAfter reads a Retry-After value at now (RFC 9110 §10.2.3): whole seconds, or an HTTP
// date. It reports false for anything else. A number too large to represent counts as the largest
// wait, and a date in the past as a negative one.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if strings.Trim(v, "0123456789") == "" {
		secs, err := strconv.ParseInt(v, 10, 64)
		if err != nil || secs > int64(MaxLinkCardHostPause/time.Second) {
			return MaxLinkCardHostPause, true
		}
		return time.Duration(secs) * time.Second, true
	}
	at, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	return at.Sub(now), true
}

// linkCardHostHold is the hosts that one fetch holds: it enters each before requesting from it,
// and leaves them all when the fetch ends. It belongs to one goroutine.
type linkCardHostHold struct {
	l      *LinkCards
	held   map[string]string // host -> token of its lock
	paused map[string]bool
}

func (l *LinkCards) newHostHold() *linkCardHostHold {
	return &linkCardHostHold{l: l, held: map[string]string{}, paused: map[string]bool{}}
}

// enter takes host for this fetch, or fails with a *LinkCardPostponed when another fetch from it is
// running or its time has not come. Any other error is a failure of the store. An empty host is
// left to the HTTPClient, which refuses it.
func (h *linkCardHostHold) enter(ctx context.Context, host string) error {
	if _, held := h.held[host]; held || host == "" {
		return nil
	}
	kv, now := h.l.d.KV, h.l.d.Clock.Now()
	token := strconv.FormatUint(rand.Uint64(), 36) //nolint:gosec // telling two holders apart needs no cryptographic randomness
	ok, err := kv.CompareAndSwap(ctx, LinkCardHostLockNamespace, host, nil, []byte(token), LinkCardHostLockTTL)
	if err != nil {
		return fmt.Errorf("lock link card host: %w", err)
	}
	if !ok {
		return h.l.postpone(ctx, host, now.Add(LinkCardHostInterval))
	}
	raw, err := kv.Get(ctx, LinkCardHostNotBeforeNamespace, host)
	if err != nil && !errors.Is(err, kernel.ErrKeyNotFound) {
		h.unlock(ctx, host, token)
		return fmt.Errorf("read link card host time: %w", err)
	}
	if notBefore, ok := parseHostTime(raw); ok && notBefore.After(now) {
		h.unlock(ctx, host, token)
		return h.l.postpone(ctx, host, notBefore)
	}
	h.held[host] = token
	return nil
}

// leave ends the fetch: the next one from each host starts LinkCardHostInterval from now, or when
// its pause ends. A failure of the store is logged, and leaves the TTLs to free the host. It does
// nothing after the first call.
func (h *linkCardHostHold) leave(ctx context.Context) {
	ctx = context.WithoutCancel(ctx)
	next := h.l.d.Clock.Now().Add(LinkCardHostInterval)
	for host, token := range h.held {
		if !h.paused[host] {
			if err := h.l.d.KV.Set(ctx, LinkCardHostNotBeforeNamespace, host, formatHostTime(next), LinkCardHostInterval); err != nil {
				h.l.d.Logger.WarnContext(ctx, "write link card host time", slog.String("host", host), slog.Any("error", err))
			}
		}
		h.unlock(ctx, host, token)
		delete(h.held, host)
	}
}

func (h *linkCardHostHold) unlock(ctx context.Context, host, token string) {
	ctx = context.WithoutCancel(ctx)
	if _, err := h.l.d.KV.CompareAndSwap(ctx, LinkCardHostLockNamespace, host, []byte(token), nil, 0); err != nil {
		h.l.d.Logger.WarnContext(ctx, "unlock link card host", slog.String("host", host), slog.Any("error", err))
	}
}

// pause stops the fetches from the host that answered resp, a 429 or a 503 to a request of u, and
// returns the host and the end of the pause. A failure of the store is logged: the answer still
// says what it says for this fetch.
func (h *linkCardHostHold) pause(ctx context.Context, u string, resp *kernel.HTTPResponse) (string, time.Time) {
	host := content.LinkHost(resp.URL)
	if host == "" {
		host = content.LinkHost(u)
	}
	now := h.l.d.Clock.Now()
	d := hostPause(resp.Header, now)
	until := now.Add(d)
	if err := h.l.d.KV.Set(ctx, LinkCardHostNotBeforeNamespace, host, formatHostTime(until), d); err != nil {
		h.l.d.Logger.WarnContext(ctx, "write link card host pause", slog.String("host", host), slog.Any("error", err))
	}
	h.paused[host] = true
	h.l.hostPauses.Add(ctx, 1, kernel.NewLabels(labelHTTPStatus, strconv.Itoa(resp.StatusCode)))
	h.l.d.Logger.InfoContext(ctx, "link card host paused", slog.String("host", host), slog.Int("status", resp.StatusCode),
		slog.Float64("pause_seconds", d.Seconds()))
	return host, until
}

// throttled handles a 429 or a 503 to a request of u: it pauses the host and fails with a
// *LinkCardPostponed until the URL has had LinkCardThrottleTries of them, and then returns nil, so
// that the answer is handled like any other that is not a 200. Any other status returns nil too.
func (h *linkCardHostHold) throttled(ctx context.Context, u string, resp *kernel.HTTPResponse) error {
	if !hostPauseStatus(resp.StatusCode) {
		return nil
	}
	host, until := h.pause(ctx, u, resp)
	key := linkCardKey(u)
	tries, err := h.l.d.KV.IncrBy(ctx, LinkCardThrottleNamespace, key, 1, LinkCardFailureTTL)
	if err != nil {
		return fmt.Errorf("count link card throttling: %w", err)
	}
	if tries < LinkCardThrottleTries {
		return h.l.postpone(ctx, host, until)
	}
	if err := h.l.d.KV.Delete(ctx, LinkCardThrottleNamespace, key); err != nil {
		h.l.d.Logger.WarnContext(ctx, "reset link card throttling", slog.String("url", u), slog.Any("error", err))
	}
	return nil
}

// postpone returns the error that sends a job back to a host that may be fetched from at ready: at
// ready, or after the jobs already told to come back then. Losing a race with another job or
// failing to reach the store costs only an extra run of a job, so neither is an error.
func (l *LinkCards) postpone(ctx context.Context, host string, ready time.Time) error {
	now, slot := l.d.Clock.Now(), ready
	for range 3 {
		raw, err := l.d.KV.Get(ctx, LinkCardHostQueueNamespace, host)
		if err != nil && !errors.Is(err, kernel.ErrKeyNotFound) {
			l.d.Logger.WarnContext(ctx, "read link card host queue", slog.String("host", host), slog.Any("error", err))
			break
		}
		slot = ready
		if tail, ok := parseHostTime(raw); ok && tail.After(slot) {
			slot = tail
		}
		next := slot.Add(LinkCardHostInterval)
		swapped, err := l.d.KV.CompareAndSwap(ctx, LinkCardHostQueueNamespace, host, raw, formatHostTime(next), next.Sub(now))
		if err != nil {
			l.d.Logger.WarnContext(ctx, "write link card host queue", slog.String("host", host), slog.Any("error", err))
			break
		}
		if swapped {
			break
		}
	}
	return &LinkCardPostponed{Host: host, After: max(slot.Sub(now), time.Millisecond)}
}

func formatHostTime(t time.Time) []byte {
	return []byte(strconv.FormatInt(t.UnixMilli(), 10))
}

// parseHostTime reads a time that formatHostTime wrote; an entry that cannot be read counts as none.
func parseHostTime(raw []byte) (time.Time, bool) {
	ms, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(ms), true
}

// linkCardPending is what a card takes from its page. A fetch postponed for the image keeps it in
// the store, so that the job does not request the page again.
type linkCardPending struct {
	Card     content.LinkCard `json:"card"`
	FinalURL string           `json:"final_url"`
	ImageURL string           `json:"image_url"`
	ImageAlt string           `json:"image_alt"`
	Creator  string           `json:"creator"`
}

// keptPage returns the page that a postponed fetch of u kept; an entry that cannot be read counts
// as none.
func (l *LinkCards) keptPage(ctx context.Context, u string) (linkCardPending, bool, error) {
	raw, err := l.d.KV.Get(ctx, LinkCardPendingNamespace, linkCardKey(u))
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return linkCardPending{}, false, nil
	}
	if err != nil {
		return linkCardPending{}, false, fmt.Errorf("read link card pending page: %w", err)
	}
	var page linkCardPending
	if json.Unmarshal(raw, &page) != nil {
		return linkCardPending{}, false, nil
	}
	return page, true, nil
}

// keepPage keeps the page of u for the next run of the job. A failure only costs requesting the
// page again.
func (l *LinkCards) keepPage(ctx context.Context, u string, page linkCardPending) {
	raw, err := json.Marshal(page)
	if err == nil {
		err = l.d.KV.Set(ctx, LinkCardPendingNamespace, linkCardKey(u), raw, LinkCardPendingTTL)
	}
	if err != nil {
		l.d.Logger.WarnContext(ctx, "keep link card pending page", slog.String("url", u), slog.Any("error", err))
	}
}

func (l *LinkCards) dropPage(ctx context.Context, u string) {
	if err := l.d.KV.Delete(ctx, LinkCardPendingNamespace, linkCardKey(u)); err != nil {
		l.d.Logger.WarnContext(ctx, "drop link card pending page", slog.String("url", u), slog.Any("error", err))
	}
}
