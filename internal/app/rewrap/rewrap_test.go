// SPDX-License-Identifier: AGPL-3.0-only

package rewrap_test

import (
	"context"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

type columnsStore struct{ cols []string }

func (s columnsStore) Columns() []string { return s.cols }
func (columnsStore) List(context.Context, string, string, int) ([]rewrap.Value, error) {
	return nil, nil
}
func (columnsStore) Replace(context.Context, uow.Tx, string, string, []byte, []byte) (bool, error) {
	return false, nil
}

func TestNewServiceRequiresEverySealedColumn(t *testing.T) {
	aad := func(id string) []byte { return kernel.AAD(id) }
	for _, tc := range []struct {
		name    string
		columns []rewrap.Column
		panics  bool
	}{
		{"all", []rewrap.Column{{Name: "b.y", AAD: aad}, {Name: "a.x", AAD: aad}}, false},
		{"missing", []rewrap.Column{{Name: "a.x", AAD: aad}}, true},
		{"extra", []rewrap.Column{{Name: "a.x", AAD: aad}, {Name: "b.y", AAD: aad}, {Name: "c.z", AAD: aad}}, true},
		{"no AAD", []rewrap.Column{{Name: "a.x", AAD: aad}, {Name: "b.y"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); (r != nil) != tc.panics {
					t.Errorf("NewService panic = %v, want panic %v", r, tc.panics)
				}
			}()
			rewrap.NewService(rewrap.Options{Store: columnsStore{cols: []string{"a.x", "b.y"}}, Columns: tc.columns})
		})
	}
}
