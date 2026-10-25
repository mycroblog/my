// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/web"
	"github.com/mycroblog/my/pkg/testkit"
)

// crawlerSettings returns instance settings with search_indexing and ai_usage stored.
func crawlerSettings(search, ai string, more ...instance.CurrentSetting) func(context.Context) (instance.Settings, error) {
	current := append([]instance.CurrentSetting{
		{Key: instance.SettingSearchIndexing, RevisionID: testkit.NamedID("revision-1"), Value: search},
		{Key: instance.SettingAIUsage, RevisionID: testkit.NamedID("revision-2"), Value: ai},
	}, more...)
	return func(context.Context) (instance.Settings, error) { return instance.NewSettings(current), nil }
}

// crawlerHeaders are the crawler statements of a response.
type crawlerHeaders struct{ robotsMeta, tdmMeta, xRobotsTag, contentUsage, tdm string }

func crawlerStatements(t *testing.T, mux http.Handler, path string) (int, crawlerHeaders) {
	t.Helper()
	rec := get(mux, path, nil)
	m := metas(t, rec.Body.String())
	h := rec.Header()
	return rec.Code, crawlerHeaders{
		robotsMeta: m["robots"], tdmMeta: m["tdm-reservation"],
		xRobotsTag: h.Get("X-Robots-Tag"), contentUsage: h.Get("Content-Usage"), tdm: h.Get("tdm-reservation"),
	}
}

// TestPagesStateTheCrawlerPolicy checks the meta tags and headers of ADR-0057: a page states the
// most restrictive of the instance's settings and its owner's, unlisted posts and the generic
// shell are never indexed, and an instance without settings denies everything.
func TestPagesStateTheCrawlerPolicy(t *testing.T) {
	indexable := alice("Alice", "")
	optedOut := alice("Alice", "")
	optedOut.Actor.Profile.AIOptOut = true
	hidden := alice("Alice", "")
	hidden.Actor.Profile.Indexable = false

	allowAll := crawlerHeaders{}
	noIndex := crawlerHeaders{robotsMeta: "noindex, noarchive", xRobotsTag: "noindex, noarchive"}
	denyAll := crawlerHeaders{
		robotsMeta: "noindex, noarchive, noai, noimageai", tdmMeta: "1",
		xRobotsTag: "noindex, noarchive, noai, noimageai", contentUsage: "train-ai=n, ai-use=n", tdm: "1",
	}
	noAI := crawlerHeaders{
		robotsMeta: "noai, noimageai", tdmMeta: "1",
		xRobotsTag: "noai, noimageai", contentUsage: "train-ai=n, ai-use=n", tdm: "1",
	}
	noTraining := crawlerHeaders{
		robotsMeta: "noai, noimageai", tdmMeta: "1",
		xRobotsTag: "noai, noimageai", contentUsage: "train-ai=n", tdm: "1",
	}

	for _, tc := range []struct {
		name     string
		settings func(context.Context) (instance.Settings, error)
		owner    appcontent.ProfilePreview
		vis      content.Visibility
		path     string
		want     crawlerHeaders
	}{
		{"no settings, landing", nil, indexable, content.VisibilityPublic, "/", denyAll},
		{"no settings, profile", nil, indexable, content.VisibilityPublic, "/@alice", denyAll},
		{"allowed, landing", crawlerSettings("allow", "allow"), indexable, content.VisibilityPublic, "/", allowAll},
		{"allowed, about", crawlerSettings("allow", "allow"), indexable, content.VisibilityPublic, "/about", allowAll},
		{"allowed, profile", crawlerSettings("allow", "allow"), indexable, content.VisibilityPublic, "/@alice", allowAll},
		{"allowed, post", crawlerSettings("allow", "allow"), indexable, content.VisibilityPublic, "/@alice/status/" + statusID, allowAll},
		{"allowed, unlisted post", crawlerSettings("allow", "allow"), indexable, content.VisibilityUnlisted, "/@alice/status/" + statusID, noIndex},
		{"allowed, owner not indexable", crawlerSettings("allow", "allow"), hidden, content.VisibilityPublic, "/@alice/status/" + statusID, noIndex},
		{"allowed, owner opted out of AI", crawlerSettings("allow", "allow"), optedOut, content.VisibilityPublic, "/@alice", noAI},
		{"allowed, app screen", crawlerSettings("allow", "allow"), indexable, content.VisibilityPublic, "/home", noIndex},
		{"allowed, remote account", crawlerSettings("allow", "allow"), indexable, content.VisibilityPublic, "/@bob@remote.example", noIndex},
		{"training denied, profile", crawlerSettings("allow", "deny_training"), indexable, content.VisibilityPublic, "/@alice", noTraining},
		{"training denied, owner opted out", crawlerSettings("allow", "deny_training"), optedOut, content.VisibilityPublic, "/@alice", noAI},
		// An owner cannot allow what the instance denies.
		{"search denied, indexable owner", crawlerSettings("deny", "deny"), indexable, content.VisibilityPublic, "/@alice", denyAll},
		{"search denied, not found", crawlerSettings("deny", "allow"), indexable, content.VisibilityPublic, "/@nobody", noIndex},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePreviews{
				profiles: map[string]appcontent.ProfilePreview{"alice": tc.owner},
				statuses: map[string]appcontent.StatusPreview{statusID: status(tc.owner, tc.vis, "<p>hi</p>")},
			}
			o := web.Options{Settings: tc.settings}
			if tc.settings == nil {
				// previewHandler defaults to allowing everything; an instance without settings is
				// a handler without Options.Settings.
				o.Settings = func(context.Context) (instance.Settings, error) { return instance.Settings{}, nil }
			}
			code, got := crawlerStatements(t, previewHandler(t, p, o), tc.path)
			if code != http.StatusOK && code != http.StatusNotFound {
				t.Fatalf("GET %s = %d", tc.path, code)
			}
			if got != tc.want {
				t.Errorf("GET %s states %+v, want %+v", tc.path, got, tc.want)
			}
		})
	}
}

// TestPagePolicyChangesTheETag checks that a page's ETag changes with the crawler policy, so that
// caches do not keep a statement that no longer holds.
func TestPagePolicyChangesTheETag(t *testing.T) {
	settings := crawlerSettings("allow", "allow")
	p := &fakePreviews{profiles: map[string]appcontent.ProfilePreview{"alice": alice("Alice", "")}}
	mux := previewHandler(t, p, web.Options{Settings: func(ctx context.Context) (instance.Settings, error) { return settings(ctx) }})
	before := get(mux, "/@alice", nil).Header().Get("ETag")
	generic := get(mux, "/home", nil).Header().Get("ETag")
	settings = crawlerSettings("allow", "deny")
	if get(mux, "/@alice", nil).Header().Get("ETag") == before {
		t.Errorf("the profile page kept its ETag after the policy changed")
	}
	if get(mux, "/home", nil).Header().Get("ETag") == generic {
		t.Errorf("the generic shell kept its ETag after the policy changed")
	}
}

func TestRobotsTxt(t *testing.T) {
	for _, tc := range []struct {
		name            string
		settings        func(context.Context) (instance.Settings, error)
		training, aiUse bool
		contentUsage    string
		contentSignal   string
	}{
		{
			name: "no settings", training: true, aiUse: true,
			contentUsage: "search=n, train-ai=n, ai-use=n", contentSignal: "search=no, ai-train=no, ai-input=no",
		},
		{
			name: "training denied", settings: crawlerSettings("allow", "deny_training"), training: true,
			contentUsage: "search=y, train-ai=n, ai-use=y", contentSignal: "search=yes, ai-train=no, ai-input=yes",
		},
		{
			name: "allowed", settings: crawlerSettings("allow", "allow"),
			contentUsage: "search=y, train-ai=y, ai-use=y", contentSignal: "search=yes, ai-train=yes, ai-input=yes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, mux := newHandler(t, web.Options{Settings: tc.settings})
			rec := get(mux, web.PathRobots, nil)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" ||
				rec.Header().Get("Cache-Control") != "public, max-age=3600" || rec.Header().Get("ETag") == "" {
				t.Fatalf("GET /robots.txt = %d %v", rec.Code, rec.Header())
			}
			body := rec.Body.String()
			for agent, want := range map[string]bool{"GPTBot": tc.training, "CCBot": tc.training, "Google-Extended": tc.training, "ClaudeBot": tc.training, "OAI-SearchBot": tc.aiUse, "ChatGPT-User": tc.aiUse, "Claude-User": tc.aiUse} {
				if got := strings.Contains(body, "User-agent: "+agent+"\n"); got != want {
					t.Errorf("names %s: %v, want %v\n%s", agent, got, want, body)
				}
			}
			// Link preview fetchers and search engines are never named: noindex states the search
			// policy, and previews keep working (ADR-0028).
			for _, agent := range []string{"facebookexternalhit", "Twitterbot", "Googlebot", "Bingbot", "Applebot"} {
				if strings.Contains(body, "User-agent: "+agent+"\n") {
					t.Errorf("names %s:\n%s", agent, body)
				}
			}
			_, every, ok := strings.Cut(body, "User-agent: *\n")
			if !ok {
				t.Fatalf("no group for every crawler:\n%s", body)
			}
			want := "Content-Usage: " + tc.contentUsage + "\nContent-Signal: " + tc.contentSignal + "\n" +
				"Disallow: /api/\nDisallow: /oauth/\nDisallow: /auth/\nDisallow: /mail/\nDisallow: /ap/\nDisallow: /media/proxy/\nAllow: /\n"
			if every != want {
				t.Errorf("group for every crawler =\n%s\nwant\n%s", every, want)
			}
		})
	}
}

func TestTDMRep(t *testing.T) {
	_, mux := newHandler(t, web.Options{})
	rec := get(mux, web.PathTDMRep, nil)
	var got []map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 1 ||
		got[0]["location"] != "/*" || got[0]["tdm-reservation"] != float64(1) {
		t.Errorf("GET %s = %d %s", web.PathTDMRep, rec.Code, rec.Body)
	}
	// Allowing AI states nothing rather than tdm-reservation: 0.
	_, mux = newHandler(t, web.Options{Settings: crawlerSettings("deny", "allow")})
	if rec := get(mux, web.PathTDMRep, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET %s with AI allowed = %d, want 404", web.PathTDMRep, rec.Code)
	}
}

func TestSecurityTxt(t *testing.T) {
	_, mux := newHandler(t, web.Options{})
	if rec := get(mux, web.PathSecurityTxt, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET %s without a contact = %d, want 404", web.PathSecurityTxt, rec.Code)
	}
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	_, mux = newHandler(t, web.Options{
		Settings: crawlerSettings("deny", "deny",
			instance.CurrentSetting{Key: instance.SettingSecurityContact, RevisionID: testkit.NamedID("revision-3"), Value: "mailto:security@social.example"}),
		DefaultLocale: locale("ja"),
		Now:           func() time.Time { return now },
	})
	rec := get(mux, web.PathSecurityTxt, nil)
	want := "Contact: mailto:security@social.example\n" +
		"Expires: 2026-11-20T09:00:00Z\n" +
		"Preferred-Languages: ja, en\n" +
		"Canonical: https://social.example/.well-known/security.txt\n"
	if rec.Code != http.StatusOK || rec.Body.String() != want || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("GET %s = %d %v\n%s", web.PathSecurityTxt, rec.Code, rec.Header(), rec.Body)
	}
}

func TestSetMediaHeaders(t *testing.T) {
	for _, tc := range []struct {
		search, ai         string
		robots, contentUse string
	}{
		{"deny", "deny", "noindex, noarchive, noai, noimageai", "train-ai=n, ai-use=n"},
		{"allow", "deny_training", "noai, noimageai", "train-ai=n"},
		{"allow", "allow", "", ""},
	} {
		s, _ := crawlerSettings(tc.search, tc.ai)(context.Background())
		h := http.Header{}
		web.SetMediaHeaders(h, s)
		if h.Get("X-Robots-Tag") != tc.robots || h.Get("Content-Usage") != tc.contentUse {
			t.Errorf("%s/%s: headers %v", tc.search, tc.ai, h)
		}
	}
}
