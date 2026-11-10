// SPDX-License-Identifier: MIT

package integration_test

import (
	"testing"

	"github.com/mycroblog/my/internal/platform/platformtest"

	"github.com/mycroblog/my/pkg/mykit"
)

// TestAccountNotes writes a private note on an account with mykit and reads it back on the
// relationship, as the writer only.
func TestAccountNotes(t *testing.T) {
	t.Parallel()
	srv := platformtest.Start(t, platformtest.Options{})
	alice, bob := srv.User(t, "alice"), srv.User(t, "bob")
	aliceAPI, bobAPI := signIn(t, srv, alice, mykit.Config{}), signIn(t, srv, bob, mykit.Config{})

	set, err := aliceAPI.SetAccountNoteWithResponse(t.Context(), bob.AccountID, mykit.SetAccountNoteJSONRequestBody{Note: "Met at the meetup"})
	if err != nil {
		t.Fatal(err)
	}
	if set.JSON200 == nil || set.JSON200.Note == nil || *set.JSON200.Note != "Met at the meetup" {
		t.Fatalf("setAccountNote = %d %s, want the note", set.StatusCode(), set.Body)
	}
	note := func(api *mykit.API, id string) string {
		t.Helper()
		res, err := api.GetRelationshipsWithResponse(t.Context(), &mykit.GetRelationshipsParams{Id: []mykit.Id{id}})
		if err != nil {
			t.Fatal(err)
		}
		if res.JSON200 == nil || len(res.JSON200.Items) != 1 || res.JSON200.Items[0].Note == nil {
			t.Fatalf("getRelationships = %d %s", res.StatusCode(), res.Body)
		}
		return *res.JSON200.Items[0].Note
	}
	if got := note(aliceAPI, bob.AccountID); got != "Met at the meetup" {
		t.Errorf("alice's note on bob = %q", got)
	}
	if got := note(bobAPI, alice.AccountID); got != "" {
		t.Errorf("bob's note on alice = %q, want none: the note is alice's", got)
	}
	if _, err := aliceAPI.DeleteAccountNoteWithResponse(t.Context(), bob.AccountID); err != nil {
		t.Fatal(err)
	}
	if got := note(aliceAPI, bob.AccountID); got != "" {
		t.Errorf("alice's note on bob after the delete = %q", got)
	}
}
