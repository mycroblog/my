// SPDX-License-Identifier: AGPL-3.0-only

package stream

import "testing"

// TestStatusEditedSignalRoundTrips checks that a KindStatusEdited signal crosses the broker with
// its revision, author, visibility, and, for the account a mention is for, AccountID.
func TestStatusEditedSignalRoundTrips(t *testing.T) {
	for _, e := range []Event{
		{Kind: KindStatusEdited, EntryID: "s1", RevisionID: "r1", AuthorAccountID: "a1", Visibility: "public"},
		{Kind: KindStatusEdited, EntryID: "s1", RevisionID: "r1", AuthorAccountID: "a1", AuthorDomain: "remote.example", Visibility: "private"},
		{Kind: KindStatusEdited, EntryID: "s1", RevisionID: "r1", AuthorAccountID: "a1", Visibility: "direct", AccountID: "a2"},
	} {
		b, err := encodeSignal("p1", e)
		if err != nil {
			t.Fatalf("encode %+v: %v", e, err)
		}
		origin, got, err := decodeSignal(b)
		if err != nil || origin != "p1" || got != e {
			t.Errorf("decode %s = %q, %+v, %v; want p1, %+v", b, origin, got, err, e)
		}
	}
}
