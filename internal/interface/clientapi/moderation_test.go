// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeOwnModeration keeps the actions on accounts, newest last, as the accounts see them.
type fakeOwnModeration struct {
	on      map[string][]appmoderation.TargetActionView // account ID -> actions, oldest first
	filters []appmoderation.OwnActionFilter
}

func (f *fakeOwnModeration) OwnActions(_ context.Context, accountID string, flt appmoderation.OwnActionFilter, limit int) ([]appmoderation.TargetActionView, error) {
	f.filters = append(f.filters, flt)
	var out []appmoderation.TargetActionView
	for _, a := range slices.Backward(f.on[accountID]) {
		if (flt.Before == "" || a.ID < flt.Before) && (!flt.Active || a.State == moderation.ActionActive) && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeOwnModeration) OwnAction(_ context.Context, accountID, id string) (appmoderation.TargetActionView, error) {
	for _, a := range f.on[accountID] {
		if a.ID == id {
			return a, nil
		}
	}
	return appmoderation.TargetActionView{}, appmoderation.ErrNotFound
}

func actionID(i int) string { return fmt.Sprintf("01890a5d-ac96-774b-bcce-b302099b%04d", i) }

// aliceActions are three actions on alice: a lifted warning, an expiring limit, and a
// suspension in effect.
func aliceActions() []appmoderation.TargetActionView {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ends, lifted := at.Add(30*24*time.Hour), at.Add(time.Hour)
	return []appmoderation.TargetActionView{
		{ID: actionID(1), Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryViolation, PublicReason: "be nice",
			StatusIDs: []string{}, IssuedAt: at, LiftedAt: &lifted, State: moderation.ActionLifted},
		{ID: actionID(2), Kind: moderation.ActionLimit, ReasonCategory: moderation.CategorySpam, StatusIDs: []string{},
			ExpiresAt: &ends, IssuedAt: at.Add(time.Minute), State: moderation.ActionActive},
		{ID: actionID(3), Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategoryLegal, StatusIDs: []string{},
			IssuedAt: at.Add(2 * time.Minute), State: moderation.ActionActive},
	}
}

func newOwnModerationEnv(t *testing.T) (*authEnv, *fakeOwnModeration) {
	t.Helper()
	f := &fakeOwnModeration{on: map[string][]appmoderation.TargetActionView{aliceAccountID: aliceActions()}}
	return newAuthEnv(t, func(o *clientapi.Options) { o.OwnModeration = f }), f
}

func TestListOwnModerationActions(t *testing.T) {
	e, f := newOwnModerationEnv(t)

	res := e.send(t, http.MethodGet, "/moderation/actions", "read-accounts", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing that the moderators keep to themselves has a field to leak through.
	for _, field := range []string{"private_note", "report_id", "issuer", "issued_by", "lift_reason", "notify_target"} {
		if strings.Contains(string(body), field) {
			t.Errorf("the body has %q: %s", field, body)
		}
	}
	var page clientapi.ModerationActionPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 || page.NextCursor != nil {
		t.Fatalf("page = %+v, want the 3 actions on one page", page)
	}
	suspend, limit, warn := page.Items[0], page.Items[1], page.Items[2]
	// alice's labels are in her language, Japanese.
	if suspend.Id != actionID(3) || suspend.Kind != clientapi.AdminAccountActionKindSuspend || suspend.State != clientapi.ModerationActionStateActive ||
		!strings.HasPrefix(suspend.KindLabel, "凍結") || suspend.ReasonCategoryLabel != "法的な理由" || suspend.ExpiresAt != nil || suspend.PublicReason != nil {
		t.Errorf("suspend = %+v", suspend)
	}
	if limit.ExpiresAt == nil || !limit.ExpiresAt.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) || limit.ReasonCategoryLabel != "スパム" {
		t.Errorf("limit = %+v", limit)
	}
	if warn.State != clientapi.ModerationActionStateLifted || warn.LiftedAt == nil || warn.PublicReason == nil || *warn.PublicReason != "be nice" ||
		warn.KindLabel != "警告" || len(warn.StatusIds) != 0 {
		t.Errorf("warn = %+v", warn)
	}

	// Pages follow next_cursor; active keeps the actions in effect.
	res = e.send(t, http.MethodGet, "/moderation/actions?limit=2", "read-accounts", "")
	first := decodeJSON[clientapi.ModerationActionPage](t, res)
	if len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("first page = %+v", first)
	}
	res = e.send(t, http.MethodGet, "/moderation/actions?limit=2&cursor="+*first.NextCursor, "read-accounts", "")
	if second := decodeJSON[clientapi.ModerationActionPage](t, res); len(second.Items) != 1 || second.Items[0].Id != actionID(1) || second.NextCursor != nil {
		t.Errorf("second page = %+v", second)
	}
	res = e.send(t, http.MethodGet, "/moderation/actions?active=true", "read-accounts", "")
	if active := decodeJSON[clientapi.ModerationActionPage](t, res); len(active.Items) != 2 || !f.filters[len(f.filters)-1].Active {
		t.Errorf("active = %+v, filter %+v", active, f.filters[len(f.filters)-1])
	}

	// Another account sees only its own actions, in its language.
	f.on[bobAccountID] = []appmoderation.TargetActionView{{ID: actionID(9), Kind: moderation.ActionWarn, ReasonCategory: moderation.CategoryOther,
		StatusIDs: []string{}, IssuedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), State: moderation.ActionActive}}
	res = e.send(t, http.MethodGet, "/moderation/actions", "bob", "")
	if bobs := decodeJSON[clientapi.ModerationActionPage](t, res); len(bobs.Items) != 1 || bobs.Items[0].Id != actionID(9) || bobs.Items[0].KindLabel != "Warning" ||
		bobs.Items[0].ReasonCategoryLabel != "Other" {
		t.Errorf("bob's actions = %+v", bobs)
	}

	for _, path := range []string{"/moderation/actions?limit=0", "/moderation/actions?cursor=%21"} {
		if res := e.send(t, http.MethodGet, path, "read-accounts", ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, res.StatusCode)
		}
	}
	// A page cursor is the base64 of an ID; any other content is a broken cursor.
	for name, id := range malformedIDs {
		path := "/moderation/actions?cursor=" + base64.RawURLEncoding.EncodeToString([]byte(id))
		if res := e.send(t, http.MethodGet, path, "read-accounts", ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s cursor = %d, want 400", name, res.StatusCode)
		}
	}
	if res := e.send(t, http.MethodGet, "/moderation/actions", "follow-push", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("without read:accounts = %d, want 403", res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, "/moderation/actions", "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous = %d, want 401", res.StatusCode)
	}
}
