// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file obeys robots.txt in the fetches of link cards (ADR-0062 §10, §12): it fetches the file
// of an origin, keeps the rules that apply to the crawler, and answers whether a URL may be
// fetched.

const (
	// LinkCardRobotsNamespace is the KeyValueStore namespace of the rules of robots.txt, keyed by
	// the origin. Only the rules that apply to the crawler are kept, never the file.
	LinkCardRobotsNamespace = "content.link_card_robots"
	// LinkCardRobotsTTL is how long the rules of a file that was read, or the absence of a file,
	// are kept.
	LinkCardRobotsTTL = 24 * time.Hour
	// LinkCardRobotsUnreachableTTL is how long a file that could not be read counts as a complete
	// disallow.
	LinkCardRobotsUnreachableTTL = time.Hour
)

// ErrLinkCardRetryLater reports a fetch that waits for a robots.txt that cannot be read now: the
// job tries again after LinkCardRobotsUnreachableTTL, and nothing is remembered as failed.
var ErrLinkCardRetryLater = errors.New("content: robots.txt cannot be read now")

// errRobotsDisallowed reports a URL that robots.txt disallows. errRobotsAssumed reports a URL
// refused only because robots.txt could not be read, which RFC 9309 §2.3.1.4 takes for a
// complete disallow; for a page it ends with ErrLinkCardRetryLater. errRobotsForbidden is the same
// for a destination that the HTTPClient refuses, which no retry changes.
var (
	errRobotsDisallowed = fmt.Errorf("%w: robots.txt disallows the URL", errLinkCardUnusable)
	errRobotsAssumed    = fmt.Errorf("%w: robots.txt cannot be read, which disallows the URL", errLinkCardUnusable)
	errRobotsForbidden  = fmt.Errorf("%w: robots.txt is at a refused destination", errLinkCardUnusable)
)

// ignoresRobots returns the link_cards_ignore_robots_txt instance setting.
func (l *LinkCards) ignoresRobots(ctx context.Context) (bool, error) {
	if l.d.IgnoreRobotsTxt == nil {
		return false, nil
	}
	return l.d.IgnoreRobotsTxt(ctx)
}

// checkRobots returns nil when the robots.txt of u's origin lets the crawler fetch u, or the
// setting ignores it, and otherwise errRobotsDisallowed or errRobotsAssumed. Any other error is a
// *LinkCardPostponed, a failure of the stores, or the end of ctx. A URL without an origin is left
// to the HTTPClient, which refuses it. A file that must be requested becomes part of the fetch that
// hold is for.
func (l *LinkCards) checkRobots(ctx context.Context, hold *linkCardHostHold, u string) error {
	if ignore, err := l.ignoresRobots(ctx); err != nil || ignore {
		return err
	}
	origin, target, ok := content.RobotsOrigin(u)
	if !ok {
		return nil
	}
	rules, err := l.robotsRules(ctx, hold, origin)
	switch {
	case err != nil:
		return err
	case rules.Forbidden:
		return fmt.Errorf("%w: %s", errRobotsForbidden, u)
	case rules.Assumed:
		return fmt.Errorf("%w: %s", errRobotsAssumed, u)
	case !rules.toRules().Allowed(target):
		return fmt.Errorf("%w: %s", errRobotsDisallowed, u)
	}
	return nil
}

// cachedRobots is what the store keeps for an origin. Rules hold each rule as its pattern after
// "+" for allow or "-" for disallow, which is about as large as the lines of the file.
type cachedRobots struct {
	// Assumed is a file that could not be read, taken for a complete disallow.
	Assumed bool `json:"assumed,omitempty"`
	// Forbidden is a file at or behind a destination that the HTTPClient refuses. It is a complete
	// disallow too, but waiting does not change it.
	Forbidden bool     `json:"forbidden,omitempty"`
	Rules     []string `json:"rules,omitempty"`
}

func newCachedRobots(rules content.RobotsRules) cachedRobots {
	out := cachedRobots{Rules: make([]string, 0, len(rules))}
	for _, r := range rules {
		sign := "-"
		if r.Allow {
			sign = "+"
		}
		out.Rules = append(out.Rules, sign+r.Pattern)
	}
	return out
}

func (c cachedRobots) toRules() content.RobotsRules {
	out := make(content.RobotsRules, 0, len(c.Rules))
	for _, r := range c.Rules {
		if r != "" {
			out = append(out, content.RobotsRule{Allow: r[0] == '+', Pattern: r[1:]})
		}
	}
	return out
}

// robotsRules returns the kept rules of origin, or fetches its file and keeps what it finds. An
// entry that cannot be read counts as missing.
func (l *LinkCards) robotsRules(ctx context.Context, hold *linkCardHostHold, origin string) (cachedRobots, error) {
	raw, err := l.d.KV.Get(ctx, LinkCardRobotsNamespace, origin)
	switch {
	case err == nil:
		var c cachedRobots
		if json.Unmarshal(raw, &c) == nil {
			return c, nil
		}
	case !errors.Is(err, kernel.ErrKeyNotFound):
		return cachedRobots{}, fmt.Errorf("read link card robots rules: %w", err)
	}
	rules, ttl, err := l.fetchRobots(ctx, hold, origin)
	if err != nil {
		return cachedRobots{}, err
	}
	raw, err = json.Marshal(rules)
	if err != nil {
		return cachedRobots{}, err
	}
	if err := l.d.KV.Set(ctx, LinkCardRobotsNamespace, origin, raw, ttl); err != nil {
		return cachedRobots{}, fmt.Errorf("write link card robots rules: %w", err)
	}
	return rules, nil
}

// fetchRobots fetches the robots.txt of origin and returns what it means for the crawler and how
// long that holds (ADR-0062 §10): the rules of a file that was read, no rules for a file that does
// not exist or redirects too many times, and an assumed complete disallow for a 429, a failure of
// the server or the network, or a refused redirect. A 429 or a 503 also pauses the host, which
// changes nothing of that: the pause never outlasts the assumed disallow. The errors, which say
// nothing about the file, are the end of ctx, a *LinkCardPostponed, and a failure of the stores.
func (l *LinkCards) fetchRobots(ctx context.Context, hold *linkCardHostHold, origin string) (cachedRobots, time.Duration, error) {
	unreachable := func(why string, err error) (cachedRobots, time.Duration, error) {
		if ctx.Err() != nil {
			return cachedRobots{}, 0, ctx.Err()
		}
		l.d.Logger.DebugContext(ctx, "link card robots.txt cannot be read", slog.String("origin", origin), slog.String("why", why), slog.Any("error", err))
		return cachedRobots{Assumed: true}, LinkCardRobotsUnreachableTTL, nil
	}
	file := content.RobotsFileURL(origin)
	if err := hold.enter(ctx, content.LinkHost(file)); err != nil {
		return cachedRobots{}, 0, err
	}
	resp, err := l.d.HTTP.Do(ctx, &kernel.HTTPRequest{
		URL: file, Purpose: kernel.HTTPPurposeRobots, Agent: kernel.HTTPAgentLinkCard,
		Header:        http.Header{"Accept": {"text/plain"}},
		CheckRedirect: l.checkRedirectHost(hold, l.fetchRefused),
	})
	var postponed *LinkCardPostponed
	if errors.As(err, &postponed) {
		return cachedRobots{}, 0, err
	}
	if errors.Is(err, kernel.ErrTooManyRedirects) {
		return cachedRobots{}, LinkCardRobotsTTL, nil
	}
	if errors.Is(err, kernel.ErrForbiddenDestination) {
		return cachedRobots{Forbidden: true}, LinkCardRobotsUnreachableTTL, nil
	}
	if err != nil {
		return unreachable("fetch", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if hostPauseStatus(resp.StatusCode) {
		hold.pause(ctx, file, resp)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= http.StatusInternalServerError:
		return unreachable(fmt.Sprintf("status %d", resp.StatusCode), nil)
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		return cachedRobots{}, LinkCardRobotsTTL, nil
	}
	body, err := io.ReadAll(resp.Body)
	truncated := errors.Is(err, kernel.ErrResponseTooLarge)
	if err != nil && !truncated {
		return unreachable("read", err)
	}
	return newCachedRobots(content.ParseRobotsTxt(body, kernel.LinkCardAgentToken, truncated)), LinkCardRobotsTTL, nil
}
