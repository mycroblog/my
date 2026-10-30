// SPDX-License-Identifier: AGPL-3.0-only

package schemaapply

import (
	"slices"
	"testing"

	"ariga.io/atlas/sql/schema"
)

func TestDropsNamesDroppedSchemasTablesAndColumns(t *testing.T) {
	users := schema.NewTable("users")
	changes := []schema.Change{
		&schema.AddTable{T: schema.NewTable("posts")},
		&schema.DropTable{T: schema.NewTable("old")},
		&schema.DropSchema{S: schema.New("legacy")},
		&schema.ModifyTable{T: users, Changes: []schema.Change{
			&schema.AddColumn{C: schema.NewColumn("bio")},
			&schema.DropColumn{C: schema.NewColumn("nickname")},
			&schema.DropIndex{I: schema.NewIndex("users_nickname")},
		}},
		&schema.ModifyTable{T: schema.NewTable("follows"), Changes: []schema.Change{
			&schema.ModifyColumn{From: schema.NewColumn("note"), To: schema.NewColumn("note")},
		}},
	}
	want := []string{"table old", "schema legacy", "column users.nickname"}
	if got := Drops(changes); !slices.Equal(got, want) {
		t.Errorf("Drops = %q, want %q", got, want)
	}
	if got := Drops([]schema.Change{&schema.AddTable{T: users}}); len(got) > 0 {
		t.Errorf("Drops of an added table = %q, want none", got)
	}
}
