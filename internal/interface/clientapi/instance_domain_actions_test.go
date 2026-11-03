// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// newPublicDomainActionsEnv is newAuthEnv with domain actions of every kind and state and the
// domain action settings visibility and rationale.
func newPublicDomainActionsEnv(t *testing.T, visibility, rationale instance.Audience) *authEnv {
	t.Helper()
	fake := &fakeInstance{settings: []instance.CurrentSetting{
		{Key: instance.SettingDomainActionsVisibility, RevisionID: testkit.NamedID("revision-1"), Value: string(visibility)},
		{Key: instance.SettingDomainActionsRationaleVisibility, RevisionID: testkit.NamedID("revision-2"), Value: string(rationale)},
	}}
	f := &fakeModeration{now: adminNow}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Moderation = f
		o.Instance = fake
	})
	issue := func(in appmoderation.DomainActionInput) moderation.DomainAction {
		t.Helper()
		a, err := f.IssueDomainAction(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	issue(appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainSuspend, PublicComment: "spam", PrivateNote: "see #12"})
	issue(appmoderation.DomainActionInput{Domain: "media.example", Kind: moderation.DomainRejectMedia, PublicComment: "gore"})
	lifted := issue(appmoderation.DomainActionInput{Domain: "lifted.example", Kind: moderation.DomainSuspend})
	if err := f.LiftDomainAction(t.Context(), lifted.ID, ""); err != nil {
		t.Fatal(err)
	}
	issue(appmoderation.DomainActionInput{Domain: "spam.example.com", Kind: moderation.DomainLimit, Obfuscate: true, PrivateNote: "secret"})
	f.now = f.now.Add(-time.Hour) // the next action expires before now
	issue(appmoderation.DomainActionInput{Domain: "expired.example", Kind: moderation.DomainLimit, Duration: time.Minute})
	f.now = f.now.Add(time.Hour)
	return e
}

const (
	digestBadExample  = "86bbe8ffb912a153c9a8b396246aeeae079cd324af4dd947a2bf59a698eabc62" // printf bad.example | sha256sum
	digestSpamExample = "6d2c4ec71792d50d6e19f3a20e35865232e75bc87478fceb8384d901dd7283d5" // printf spam.example.com | sha256sum
)

func TestListInstanceDomainActions(t *testing.T) {
	spam := clientapi.InstanceDomainAction{Domain: "spam.*******.com", Digest: digestSpamExample, Kind: clientapi.InstanceDomainActionKindLimit}
	bad := clientapi.InstanceDomainAction{Domain: "bad.example", Digest: digestBadExample, Kind: clientapi.InstanceDomainActionKindSuspend}
	comment := "spam"
	badWithComment := bad
	badWithComment.Comment = &comment
	for _, tc := range []struct {
		name                  string
		visibility, rationale instance.Audience
		token                 string
		want                  []clientapi.InstanceDomainAction
		wantCache             string
	}{
		{"everyone, without comments", instance.AudienceEveryone, instance.AudienceOff, "", []clientapi.InstanceDomainAction{spam, bad}, "public, max-age=60"},
		{"everyone, comments for everyone", instance.AudienceEveryone, instance.AudienceEveryone, "", []clientapi.InstanceDomainAction{spam, badWithComment}, "public, max-age=60"},
		{"everyone, comments for users, anonymous", instance.AudienceEveryone, instance.AudienceSignedIn, "", []clientapi.InstanceDomainAction{spam, bad}, "public, max-age=60"},
		{"everyone, comments for users, logged in", instance.AudienceEveryone, instance.AudienceSignedIn, "read", []clientapi.InstanceDomainAction{spam, badWithComment}, "private, max-age=60"},
		{"users, logged in", instance.AudienceSignedIn, instance.AudienceEveryone, "none", []clientapi.InstanceDomainAction{spam, badWithComment}, "private, max-age=60"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPublicDomainActionsEnv(t, tc.visibility, tc.rationale)
			e.auth.tokens["none"] = bearer("", aliceAccountID)
			res := e.send(t, http.MethodGet, "/instance/domain_actions", tc.token, "")
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.StatusCode)
			}
			if cc := res.Header.Get("Cache-Control"); cc != tc.wantCache {
				t.Errorf("Cache-Control = %q, want %q", cc, tc.wantCache)
			}
			page := decode[clientapi.InstanceDomainActionPage](t, res)
			if !reflect.DeepEqual(page.Items, tc.want) || page.NextCursor != nil {
				t.Errorf("page = %+v, want %+v without a next cursor", page, tc.want)
			}
		})
	}
}

func TestListInstanceDomainActionsNeverShowsPrivateNotes(t *testing.T) {
	e := newPublicDomainActionsEnv(t, instance.AudienceEveryone, instance.AudienceEveryone)
	res := e.send(t, http.MethodGet, "/instance/domain_actions", "", "")
	page := decode[clientapi.InstanceDomainActionPage](t, res)
	for _, it := range page.Items {
		if it.Comment != nil && (*it.Comment == "see #12" || *it.Comment == "secret") {
			t.Errorf("item %+v shows a private note", it)
		}
		if it.Domain == "media.example" || it.Domain == "lifted.example" || it.Domain == "expired.example" {
			t.Errorf("item %+v is not a limit or suspension in effect", it)
		}
	}
}

func TestListInstanceDomainActionsPages(t *testing.T) {
	e := newPublicDomainActionsEnv(t, instance.AudienceEveryone, instance.AudienceOff)
	res := e.send(t, http.MethodGet, "/instance/domain_actions?limit=1", "", "")
	page := decode[clientapi.InstanceDomainActionPage](t, res)
	if len(page.Items) != 1 || page.Items[0].Digest != digestSpamExample || page.NextCursor == nil {
		t.Fatalf("first page = %+v, want spam.example.com and a next cursor", page)
	}
	res = e.send(t, http.MethodGet, "/instance/domain_actions?limit=1&cursor="+*page.NextCursor, "", "")
	page = decode[clientapi.InstanceDomainActionPage](t, res)
	if len(page.Items) != 1 || page.Items[0].Digest != digestBadExample || page.NextCursor != nil {
		t.Errorf("second page = %+v, want bad.example and no next cursor", page)
	}
	for _, query := range []string{"limit=0", "limit=41", "cursor=%21"} {
		if res := e.send(t, http.MethodGet, "/instance/domain_actions?"+query, "", ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET ?%s status = %d, want 400", query, res.StatusCode)
		}
	}
}

func TestListInstanceDomainActionsRefusesReaders(t *testing.T) {
	for _, tc := range []struct {
		name       string
		visibility instance.Audience
		token      string
		wantStatus int
		wantType   string
	}{
		{"none, anonymous", instance.AudienceOff, "", http.StatusNotFound, "about:blank"},
		{"none, logged in", instance.AudienceOff, "read", http.StatusNotFound, "about:blank"},
		{"users, anonymous", instance.AudienceSignedIn, "", http.StatusUnauthorized, clientapi.ProblemTypeUnauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPublicDomainActionsEnv(t, tc.visibility, instance.AudienceEveryone)
			res := e.send(t, http.MethodGet, "/instance/domain_actions", tc.token, "")
			if res.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.wantStatus)
			}
			if p := decode[clientapi.Problem](t, res); p.Type != tc.wantType {
				t.Errorf("type = %q, want %q", p.Type, tc.wantType)
			}
			if tc.wantStatus == http.StatusUnauthorized && res.Header.Get("WWW-Authenticate") != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", res.Header.Get("WWW-Authenticate"))
			}
		})
	}
	// Without instance settings, the default (none) applies.
	e := newAuthEnv(t, nil)
	if res := e.send(t, http.MethodGet, "/instance/domain_actions", "", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("status without settings = %d, want 404", res.StatusCode)
	}
}
