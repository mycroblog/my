// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestClientAPIAccountNotes checks the private notes on accounts (FR-310): the writer reads the
// note on the relationship, the account it is on does not, an empty note or a DELETE removes it,
// and a note that is too long or on the writer's own account is a 422.
func TestClientAPIAccountNotes(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	alice, bob, carol := e.user("alice"), e.user("bob"), e.user("carol")
	note := func(from, to testUser, text string) clientapi.Relationship {
		t.Helper()
		var r clientapi.Relationship
		e.want(e.do(http.MethodPut, "/accounts/"+to.id+"/note", from.token, map[string]string{"note": text}, &r), http.StatusOK, "note")
		return r
	}
	text := func(r clientapi.Relationship) string {
		if r.Note == nil {
			return "<absent>"
		}
		return *r.Note
	}

	if r := note(alice, bob, "Met at the meetup"); text(r) != "Met at the meetup" {
		t.Errorf("note = %+v", r)
	}
	if rs := e.relationships(alice, bob.id, carol.id); len(rs) != 2 || text(rs[0]) != "Met at the meetup" || text(rs[1]) != "" {
		t.Errorf("alice's relationships = %+v, want the note on bob alone", rs)
	}
	if rs := e.relationships(bob, alice.id); len(rs) != 1 || text(rs[0]) != "" {
		t.Errorf("bob's relationship with alice = %+v, want no note: the note is alice's", rs)
	}
	if rs := e.relationships(carol, bob.id); len(rs) != 1 || text(rs[0]) != "" {
		t.Errorf("carol's relationship with bob = %+v, want no note", rs)
	}
	if r := note(alice, bob, strings.Repeat("あ", 2000)); len([]rune(text(r))) != 2000 {
		t.Errorf("the longest note = %d characters", len([]rune(text(r))))
	}
	e.want(e.do(http.MethodPut, "/accounts/"+bob.id+"/note", alice.token, map[string]string{"note": strings.Repeat("a", 2001)}, nil), http.StatusUnprocessableEntity, "too long")
	e.want(e.do(http.MethodPut, "/accounts/"+alice.id+"/note", alice.token, map[string]string{"note": "me"}, nil), http.StatusUnprocessableEntity, "self")
	e.want(e.do(http.MethodPut, "/accounts/"+bob.id+"/note", "", map[string]string{"note": "x"}, nil), http.StatusUnauthorized, "anonymous")

	if r := note(alice, bob, ""); text(r) != "" {
		t.Errorf("empty note = %+v, want the note removed", r)
	}
	note(alice, carol, "Carol")
	if r := e.relate("unnote", alice, carol, nil); text(r) != "" {
		t.Errorf("DELETE note = %+v, want the note removed", r)
	}
	if r := e.relate("unnote", alice, carol, nil); text(r) != "" {
		t.Errorf("DELETE note again = %+v", r)
	}
}
