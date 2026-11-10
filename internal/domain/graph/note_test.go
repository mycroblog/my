// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"errors"
	"strings"
	"testing"
)

func TestNoteValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		note Note
		want error
	}{
		{"valid", Note{AccountID: idA, TargetAccountID: idB, Text: "Met at the meetup"}, nil},
		{"longest", Note{AccountID: idA, TargetAccountID: idB, Text: strings.Repeat("あ", MaxNoteLength)}, nil},
		{"self", Note{AccountID: idA, TargetAccountID: idA, Text: "me"}, ErrSelfNote},
		{"empty", Note{AccountID: idA, TargetAccountID: idB}, ErrInvalidNote},
		{"too long", Note{AccountID: idA, TargetAccountID: idB, Text: strings.Repeat("a", MaxNoteLength+1)}, ErrInvalidNote},
		{"invalid UTF-8", Note{AccountID: idA, TargetAccountID: idB, Text: "\xff"}, ErrInvalidNote},
		{"NUL", Note{AccountID: idA, TargetAccountID: idB, Text: "a\x00b"}, ErrInvalidNote},
	} {
		err := tc.note.Validate()
		if (tc.want == nil) != (err == nil) || (tc.want != nil && !errors.Is(err, tc.want)) {
			t.Errorf("%s: Validate = %v, want %v", tc.name, err, tc.want)
		}
	}
	if err := (Note{AccountID: idA, Text: "x"}).Validate(); err == nil {
		t.Error("missing target: want an error")
	}
}
