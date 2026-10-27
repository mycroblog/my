// SPDX-License-Identifier: MIT

package mybot

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rate is a number of events in a window of time, such as 60 posts an hour.
type Rate struct {
	N   int
	Per time.Duration
}

// The defaults of the send budget (ADR-0094 §6) and of the reply limits (§8).
var (
	// DefaultPostRate is the budget of posts, replies included: 60 an hour, six tenths of the server's 300 in
	// three hours.
	DefaultPostRate = Rate{N: 60, Per: time.Hour}
	// DefaultWriteRate is the budget of the other writes (reactions, favourites, follows): 60 a minute.
	DefaultWriteRate = Rate{N: 60, Per: time.Minute}
	// DefaultReplyLimits are the most replies to one sender: 5 in 10 minutes and 15 in an hour.
	DefaultReplyLimits = []Rate{{N: 5, Per: 10 * time.Minute}, {N: 15, Per: time.Hour}}
)

// DefaultMaxSendWait is the longest a write waits for the send budget before it is dropped.
const DefaultMaxSendWait = 10 * time.Minute

// ErrBudgetExceeded is the error of a write that would have waited for the send budget longer than
// Config.MaxSendWait. The write was not sent.
var ErrBudgetExceeded = errors.New("mybot: the send budget is used up for longer than the longest wait")

// sendKind is the budget a write draws on.
type sendKind int

const (
	sendPost sendKind = iota
	sendWrite
)

// bucket is a token bucket that holds up to rate.N tokens and gains rate.N every rate.Per.
type bucket struct {
	rate   Rate
	tokens float64
	last   time.Time
}

func newBucket(r Rate) *bucket { return &bucket{rate: r, tokens: float64(r.N)} }

// wait refills the bucket at now and returns how long until it holds one token.
func (b *bucket) wait(now time.Time) time.Duration {
	if b.rate.N <= 0 || b.rate.Per <= 0 {
		return 0
	}
	if !b.last.IsZero() {
		b.tokens = min(float64(b.rate.N), b.tokens+float64(now.Sub(b.last))*float64(b.rate.N)/float64(b.rate.Per))
	}
	b.last = now
	if b.tokens >= 1 {
		return 0
	}
	return time.Duration((1 - b.tokens) * float64(b.rate.Per) / float64(b.rate.N))
}

// take uses one token, which wait made sure of or reserved.
func (b *bucket) take() {
	if b.rate.N > 0 && b.rate.Per > 0 {
		b.tokens--
	}
}

// budget is the client-side send budget of a bot: a token bucket for posts and one for the other writes, and
// the pause that the server's RateLimit fields ask for when less than a tenth of the limit remains.
type budget struct {
	mu      sync.Mutex
	buckets map[sendKind]*bucket
	pause   time.Time
}

func newBudget(post, write Rate) *budget {
	return &budget{buckets: map[sendKind]*bucket{sendPost: newBucket(post), sendWrite: newBucket(write)}}
}

// reserve reserves a send of kind at now and returns how long to wait before sending it. It reports false, and
// reserves nothing, when the wait would be longer than maxWait. During a pause of the server the bucket counts
// from the end of the pause, so that the writes waiting for it go out spaced by the budget, not all at its end.
func (b *budget) reserve(kind sendKind, now time.Time, maxWait time.Duration) (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bk := b.buckets[kind]
	from := now
	if b.pause.After(from) {
		from = b.pause
	}
	// Never count back from the bucket's last refill, which would drain it.
	if bk.last.After(from) {
		from = bk.last
	}
	wait := from.Sub(now) + bk.wait(from)
	if wait > maxWait {
		return 0, false
	}
	bk.take()
	return wait, true
}

// observe reads the RateLimit and RateLimit-Policy fields of an answer (draft-ietf-httpapi-ratelimit-headers,
// `"name";r=<remaining>;t=<seconds>` and `"name";q=<quota>;w=<window>`): when less than a tenth of a policy's
// quota remains, the writes wait until its window resets.
func (b *budget) observe(h http.Header, now time.Time) {
	limits := h.Values("RateLimit")
	if len(limits) == 0 {
		return
	}
	quotas := map[string]int{}
	for _, item := range structuredItems(h.Values("RateLimit-Policy")) {
		if q, ok := item.params["q"]; ok {
			quotas[item.name] = q
		}
	}
	for _, item := range structuredItems(limits) {
		r, okR := item.params["r"]
		t, okT := item.params["t"]
		q, okQ := quotas[item.name]
		if !okR || !okT || !okQ || q <= 0 || r*10 >= q {
			continue
		}
		until := now.Add(time.Duration(t) * time.Second)
		b.mu.Lock()
		if until.After(b.pause) {
			b.pause = until
		}
		b.mu.Unlock()
	}
}

// structuredItem is a member of a structured field list: a string or token and its integer parameters.
type structuredItem struct {
	name   string
	params map[string]int
}

// structuredItems reads the members of list fields such as `"api";r=12;t=30, "posts";r=1;t=600`, skipping the
// parameters that are not integers.
func structuredItems(values []string) []structuredItem {
	var items []structuredItem
	for _, v := range values {
		for member := range strings.SplitSeq(v, ",") {
			parts := strings.Split(member, ";")
			item := structuredItem{name: strings.Trim(strings.TrimSpace(parts[0]), `"`), params: map[string]int{}}
			for _, p := range parts[1:] {
				k, val, ok := strings.Cut(strings.TrimSpace(p), "=")
				if !ok {
					continue
				}
				if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
					item.params[strings.TrimSpace(k)] = n
				}
			}
			items = append(items, item)
		}
	}
	return items
}

// observingTransport hands the RateLimit fields of every answer to a budget.
type observingTransport struct {
	next   http.RoundTripper
	budget *budget
	now    func() time.Time
}

func (t *observingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err == nil {
		t.budget.observe(resp.Header, t.now())
	}
	return resp, err
}

// replyLimiter counts the replies to each sender, for the reply limits of §8.
type replyLimiter struct {
	mu     sync.Mutex
	limits []Rate
	sent   map[string][]time.Time
}

func newReplyLimiter(limits []Rate) *replyLimiter {
	return &replyLimiter{limits: limits, sent: map[string][]time.Time{}}
}

// full reports whether one more reply to sender at now would go over a limit.
func (l *replyLimiter) full(sender string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fullLocked(sender, now)
}

func (l *replyLimiter) fullLocked(sender string, now time.Time) bool {
	longest := time.Duration(0)
	for _, r := range l.limits {
		longest = max(longest, r.Per)
	}
	times := l.sent[sender]
	// Drop what no window reaches any more.
	keep := times[:0]
	for _, at := range times {
		if now.Sub(at) < longest {
			keep = append(keep, at)
		}
	}
	if len(keep) == 0 {
		delete(l.sent, sender)
	} else {
		l.sent[sender] = keep
	}
	for _, r := range l.limits {
		if r.N <= 0 || r.Per <= 0 {
			continue
		}
		n := 0
		for _, at := range keep {
			if now.Sub(at) < r.Per {
				n++
			}
		}
		if n >= r.N {
			return true
		}
	}
	return false
}

// take records a reply to sender at now, reporting false, and recording nothing, when it would go over a limit.
func (l *replyLimiter) take(sender string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fullLocked(sender, now) {
		return false
	}
	l.sent[sender] = append(l.sent[sender], now)
	return true
}

// untake forgets the reply to sender recorded at at, which was not sent.
func (l *replyLimiter) untake(sender string, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	times := l.sent[sender]
	for i := len(times) - 1; i >= 0; i-- {
		if times[i].Equal(at) {
			l.sent[sender] = append(times[:i], times[i+1:]...)
			return
		}
	}
}
