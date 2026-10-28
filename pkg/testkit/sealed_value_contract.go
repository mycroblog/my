// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

// SealedValueHarness is a rewrap.Store on a fresh database with the product schema, with the
// UnitOfWork of the same database.
type SealedValueHarness struct {
	UnitOfWork uow.UnitOfWork
	Store      rewrap.Store
	// Schema is the product schema of the dialect; every sealed column it declares (a column
	// named *_enc or sealed_*) must be one of Store.Columns.
	Schema string
	// Insert stores sealed as the value of column in a new row with the ID rowID, filling the
	// other columns of the row with any valid values.
	Insert func(t *testing.T, column, rowID string, sealed []byte)
}

// SealedValueStoreContract is the contract of rewrap.Store, and of rewrap.Service on it, that
// every database adapter must pass.
func SealedValueStoreContract(t *testing.T, newHarness func(t *testing.T) SealedValueHarness) {
	t.Helper()
	t.Run("knows every sealed column of the schema", func(t *testing.T) {
		t.Parallel()
		sealedColumnsOfSchema(t, newHarness(t))
	})
	t.Run("lists the values of a column by row ID in pages", func(t *testing.T) {
		t.Parallel()
		sealedValuesList(t, newHarness(t))
	})
	t.Run("replaces a value only while the row holds the value read", func(t *testing.T) {
		t.Parallel()
		sealedValuesReplace(t, newHarness(t))
	})
	t.Run("rewrap resumes after an interruption and never wraps twice", func(t *testing.T) {
		t.Parallel()
		rewrapResumes(t, newHarness(t))
	})
	t.Run("rewrap keeps the values it cannot unwrap", func(t *testing.T) {
		t.Parallel()
		rewrapKeepsFailures(t, newHarness(t))
	})
	t.Run("rewrap keeps a value rewritten after it was read", func(t *testing.T) {
		t.Parallel()
		rewrapKeepsChanged(t, newHarness(t))
	})
}

// sealedRowID returns the n-th row ID of the tests, a UUID that sorts by n.
func sealedRowID(n int) string { return fmt.Sprintf("01890a5d-ac96-774b-bcce-%012x", n) }

var (
	schemaTable  = regexp.MustCompile(`^CREATE TABLE (?:IF NOT EXISTS )?"?([A-Za-z_][A-Za-z0-9_]*)"?`)
	sealedColumn = regexp.MustCompile(`^\s+([a-z_][a-z0-9_]*_enc|sealed_[a-z0-9_]+)\s`)
)

func sealedColumnsOfSchema(t *testing.T, h SealedValueHarness) {
	var found []string
	table := ""
	for line := range strings.SplitSeq(h.Schema, "\n") {
		if m := schemaTable.FindStringSubmatch(line); m != nil {
			table = m[1]
			continue
		}
		if m := sealedColumn.FindStringSubmatch(line); m != nil && table != "" {
			found = append(found, table+"."+m[1])
		}
	}
	slices.Sort(found)
	if len(found) == 0 {
		t.Fatal("the schema declares no sealed column; the harness passed no schema?")
	}
	cols := h.Store.Columns()
	if !slices.IsSorted(cols) {
		t.Errorf("Columns() = %v, not sorted", cols)
	}
	if !slices.Equal(found, cols) {
		t.Errorf("Columns() = %v, want the sealed columns of the schema %v", cols, found)
	}
}

func listAll(t *testing.T, s rewrap.Store, column, after string, limit int) []rewrap.Value {
	t.Helper()
	got, err := s.List(t.Context(), column, after, limit)
	if err != nil {
		t.Fatalf("List(%s, %q, %d): %v", column, after, limit, err)
	}
	return got
}

func rowIDs(vs []rewrap.Value) []string {
	ids := make([]string, len(vs))
	for i, v := range vs {
		ids[i] = v.RowID
	}
	return ids
}

func sealedValuesList(t *testing.T, h SealedValueHarness) {
	ctx := t.Context()
	for i, col := range h.Store.Columns() {
		base := i * 10
		for _, n := range []int{3, 1, 2} {
			h.Insert(t, col, sealedRowID(base+n), []byte(fmt.Sprintf("%s-%d", col, n)))
		}
		first := listAll(t, h.Store, col, "", 2)
		if want := []string{sealedRowID(base + 1), sealedRowID(base + 2)}; !slices.Equal(rowIDs(first), want) {
			t.Fatalf("List(%s) first page = %v, want %v", col, rowIDs(first), want)
		}
		if !bytes.Equal(first[0].Sealed, []byte(col+"-1")) {
			t.Errorf("List(%s) value = %q", col, first[0].Sealed)
		}
		next := listAll(t, h.Store, col, first[1].RowID, 2)
		if want := []string{sealedRowID(base + 3)}; !slices.Equal(rowIDs(next), want) {
			t.Fatalf("List(%s) second page = %v, want %v", col, rowIDs(next), want)
		}
		if last := listAll(t, h.Store, col, next[0].RowID, 2); len(last) != 0 {
			t.Errorf("List(%s) after the last row = %v, want none", col, rowIDs(last))
		}
	}
	if _, err := h.Store.List(ctx, "accounts.username", "", 10); !errors.Is(err, rewrap.ErrUnknownColumn) {
		t.Errorf("List of an unknown column: %v, want ErrUnknownColumn", err)
	}
	err := h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := h.Store.Replace(ctx, tx, "accounts.username", sealedRowID(1), nil, nil)
		return err
	})
	if !errors.Is(err, rewrap.ErrUnknownColumn) {
		t.Errorf("Replace of an unknown column: %v, want ErrUnknownColumn", err)
	}
}

func replaceSealed(t *testing.T, h SealedValueHarness, column, rowID string, old, sealed []byte) bool {
	t.Helper()
	var ok bool
	if err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		var err error
		ok, err = h.Store.Replace(t.Context(), tx, column, rowID, old, sealed)
		return err
	}); err != nil {
		t.Fatalf("Replace(%s, %s): %v", column, rowID, err)
	}
	return ok
}

func sealedValueOf(t *testing.T, h SealedValueHarness, column, rowID string) []byte {
	t.Helper()
	after := ""
	for {
		vs := listAll(t, h.Store, column, after, 100)
		for _, v := range vs {
			if v.RowID == rowID {
				return v.Sealed
			}
		}
		if len(vs) < 100 {
			t.Fatalf("no row %s in %s", rowID, column)
		}
		after = vs[len(vs)-1].RowID
	}
}

func sealedValuesReplace(t *testing.T, h SealedValueHarness) {
	for i, col := range h.Store.Columns() {
		id := sealedRowID(i)
		h.Insert(t, col, id, []byte("v1"))
		if replaceSealed(t, h, col, id, []byte("other"), []byte("v2")) {
			t.Errorf("Replace(%s) of a value the row does not hold = true", col)
		}
		if got := sealedValueOf(t, h, col, id); !bytes.Equal(got, []byte("v1")) {
			t.Fatalf("value of %s after a mismatched Replace = %q, want v1", col, got)
		}
		if !replaceSealed(t, h, col, id, []byte("v1"), []byte("v2")) {
			t.Errorf("Replace(%s) of the value the row holds = false", col)
		}
		if got := sealedValueOf(t, h, col, id); !bytes.Equal(got, []byte("v2")) {
			t.Fatalf("value of %s after Replace = %q, want v2", col, got)
		}
		if replaceSealed(t, h, col, sealedRowID(900+i), []byte("v1"), []byte("v2")) {
			t.Errorf("Replace(%s) of a missing row = true", col)
		}
		// A transaction that rolls back keeps the value.
		rollback := errors.New("rollback")
		err := h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			if _, err := h.Store.Replace(t.Context(), tx, col, id, []byte("v2"), []byte("v3")); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatalf("Tx = %v", err)
		}
		if got := sealedValueOf(t, h, col, id); !bytes.Equal(got, []byte("v2")) {
			t.Errorf("value of %s after a rollback = %q, want v2", col, got)
		}
	}
}

// sealedColumns returns the columns of s with an AAD of the row ID and the column name.
func sealedColumns(s rewrap.Store) []rewrap.Column {
	var cols []rewrap.Column
	for _, name := range s.Columns() {
		cols = append(cols, rewrap.Column{Name: name, AAD: func(id string) []byte { return kernel.AAD(id, name) }})
	}
	return cols
}

func newRewrapService(h SealedValueHarness, store rewrap.Store, km kernel.KeyManager) *rewrap.Service {
	return rewrap.NewService(rewrap.Options{
		UnitOfWork: h.UnitOfWork, Store: store, Sealer: kernel.NewSealer(km), Columns: sealedColumns(h.Store), BatchSize: 2,
	})
}

// sealRows stores n secrets sealed under the KEK keyID in each column, from the row number
// first, and returns the secrets by column and row ID.
func sealRows(t *testing.T, h SealedValueHarness, keyID string, first, n int, secrets map[string]map[string]string) {
	t.Helper()
	s := kernel.NewSealer(NewKeyManager(keyID))
	for i, col := range h.Store.Columns() {
		if secrets[col] == nil {
			secrets[col] = map[string]string{}
		}
		for j := range n {
			id := sealedRowID(i*1000 + first + j)
			secret := fmt.Sprintf("secret %s %s", col, id)
			sealed, err := s.Seal(t.Context(), []byte(secret), kernel.AAD(id, col))
			if err != nil {
				t.Fatal(err)
			}
			h.Insert(t, col, id, sealed)
			secrets[col][id] = secret
		}
	}
}

// checkSecrets opens every stored value with km and compares it with secrets.
func checkSecrets(t *testing.T, h SealedValueHarness, km kernel.KeyManager, secrets map[string]map[string]string) {
	t.Helper()
	s := kernel.NewSealer(km)
	for col, rows := range secrets {
		for id, want := range rows {
			got, err := s.Open(t.Context(), sealedValueOf(t, h, col, id), kernel.AAD(id, col))
			if err != nil || string(got) != want {
				t.Errorf("open %s %s = %q, %v; want %q", col, id, got, err, want)
			}
		}
	}
}

// failingStore fails Replace after a number of calls, like a process that stops halfway.
type failingStore struct {
	rewrap.Store
	left int
}

var errInterrupted = errors.New("interrupted")

func (s *failingStore) Replace(ctx context.Context, tx uow.Tx, column, rowID string, old, sealed []byte) (bool, error) {
	if s.left == 0 {
		return false, errInterrupted
	}
	s.left--
	return s.Store.Replace(ctx, tx, column, rowID, old, sealed)
}

func outdated(t *testing.T, svc *rewrap.Service) int {
	t.Helper()
	st, err := svc.Status(t.Context())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	n := 0
	for _, c := range st {
		n += c.Outdated()
	}
	return n
}

func rewrapResumes(t *testing.T, h SealedValueHarness) {
	ctx := t.Context()
	secrets := map[string]map[string]string{}
	sealRows(t, h, "kek-old", 0, 5, secrets)
	sealRows(t, h, "kek-new", 5, 1, secrets)
	cols := len(h.Store.Columns())
	ring := NewKeyRing("kek-new", "kek-old")

	// The first run stops in the middle of the second batch of the first column.
	stopped := newRewrapService(h, &failingStore{Store: h.Store, left: 3}, ring)
	reports, err := stopped.Rewrap(ctx, nil)
	if !errors.Is(err, errInterrupted) || len(reports) != 0 {
		t.Fatalf("interrupted Rewrap = %v, %v; want errInterrupted and no completed column", reports, err)
	}
	svc := newRewrapService(h, h.Store, ring)
	// Each batch writes in one transaction: the first batch is re-wrapped and the second kept.
	if got, want := outdated(t, svc), 5*cols-2; got != want {
		t.Errorf("outdated values after the interruption = %d, want %d", got, want)
	}
	// Every value still opens under the KEKs of the ring.
	checkSecrets(t, h, ring, secrets)
	reports, err = svc.Rewrap(ctx, nil)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	if len(reports) != cols {
		t.Fatalf("Rewrap reports %d columns, want %d", len(reports), cols)
	}
	for i, r := range reports {
		wantRewrapped, wantCurrent := 5, 1
		if i == 0 {
			wantRewrapped, wantCurrent = 3, 3
		}
		if r.Rewrapped != wantRewrapped || r.Current != wantCurrent || r.Failed != 0 || r.Changed != 0 {
			t.Errorf("report of %s = %+v; want %d rewrapped and %d current", r.Column, r, wantRewrapped, wantCurrent)
		}
	}
	if n := outdated(t, svc); n != 0 {
		t.Errorf("outdated values after Rewrap = %d, want 0", n)
	}
	checkSecrets(t, h, NewKeyManager("kek-new"), secrets)

	// Running it again changes nothing.
	before := map[string][]rewrap.Value{}
	for _, col := range h.Store.Columns() {
		before[col] = listAll(t, h.Store, col, "", 100)
	}
	reports, err = svc.Rewrap(ctx, nil)
	if err != nil {
		t.Fatalf("Rewrap again: %v", err)
	}
	for _, r := range reports {
		if r.Rewrapped != 0 || r.Current != 6 {
			t.Errorf("report of %s on the second run = %+v; want 6 current", r.Column, r)
		}
		after := listAll(t, h.Store, r.Column, "", 100)
		if !slices.EqualFunc(before[r.Column], after, func(a, b rewrap.Value) bool { return a.RowID == b.RowID && bytes.Equal(a.Sealed, b.Sealed) }) {
			t.Errorf("the second run rewrote values of %s", r.Column)
		}
	}
	st, err := svc.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range st {
		if len(c.Keys) != 1 || c.Keys[0] != (rewrap.KeyCount{KeyID: "kek-new", Count: 6, Current: true}) || c.Total() != 6 {
			t.Errorf("Status of %s = %+v; want 6 under kek-new", c.Column, c)
		}
	}
}

func rewrapKeepsFailures(t *testing.T, h SealedValueHarness) {
	ctx := t.Context()
	secrets := map[string]map[string]string{}
	sealRows(t, h, "kek-old", 0, 3, secrets)
	col := h.Store.Columns()[0]
	// A value sealed for another row does not authenticate in this one.
	moved := sealedRowID(500)
	h.Insert(t, col, moved, sealedValueOf(t, h, col, sealedRowID(0)))
	// A value with a broken header.
	broken := sealedRowID(501)
	h.Insert(t, col, broken, []byte("not sealed"))

	snapshot := func() map[string][]rewrap.Value {
		out := map[string][]rewrap.Value{}
		for _, c := range h.Store.Columns() {
			out[c] = listAll(t, h.Store, c, "", 100)
		}
		return out
	}
	before := snapshot()
	// The operator configured a wrong previous KEK: nothing can be unwrapped, and nothing changes.
	wrong := newRewrapService(h, h.Store, NewKeyRing("kek-new", "kek-wrong"))
	reports, err := wrong.Rewrap(ctx, nil)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	for _, r := range reports {
		want := 3
		if r.Column == col {
			want = 5
		}
		if r.Rewrapped != 0 || r.Failed != want || len(r.Failures) != want {
			t.Errorf("report of %s with a wrong KEK = %+v; want %d failed", r.Column, r, want)
		}
		for _, f := range r.Failures {
			switch {
			case f.RowID == broken:
				if f.KeyID != "" || !errors.Is(f.Err, kernel.ErrDecrypt) {
					t.Errorf("failure of the broken value = %+v", f)
				}
			case f.KeyID != "kek-old" || !errors.Is(f.Err, kernel.ErrUnknownKey):
				t.Errorf("failure of %s = %+v; want kek-old unknown", f.RowID, f)
			}
		}
	}
	if after := snapshot(); !mapsEqualValues(before, after) {
		t.Error("Rewrap with a wrong KEK changed stored values")
	}

	// With the right previous KEK, all but the bad values are re-wrapped.
	right := newRewrapService(h, h.Store, NewKeyRing("kek-new", "kek-old"))
	reports, err = right.Rewrap(ctx, nil)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	for _, r := range reports {
		wantFailed := 0
		if r.Column == col {
			wantFailed = 2
		}
		if r.Rewrapped != 3 || r.Failed != wantFailed {
			t.Errorf("report of %s = %+v; want 3 rewrapped and %d failed", r.Column, r, wantFailed)
		}
	}
	checkSecrets(t, h, NewKeyManager("kek-new"), secrets)
	if got := sealedValueOf(t, h, col, broken); !bytes.Equal(got, []byte("not sealed")) {
		t.Errorf("the broken value became %q", got)
	}
	if got := sealedValueOf(t, h, col, moved); !bytes.Equal(got, before[col][0].Sealed) {
		t.Error("the moved value was rewritten")
	}
	if n := outdated(t, right); n != 2 {
		t.Errorf("outdated values = %d, want the 2 bad ones", n)
	}
}

func mapsEqualValues(a, b map[string][]rewrap.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if !slices.EqualFunc(va, b[k], func(x, y rewrap.Value) bool { return x.RowID == y.RowID && bytes.Equal(x.Sealed, y.Sealed) }) {
			return false
		}
	}
	return true
}

// rewritingStore rewrites a row the first time a page lists it, like the server writing a new
// value between the read and the write of Rewrap.
type rewritingStore struct {
	rewrap.Store
	h      SealedValueHarness
	column string
	rowID  string
	value  []byte
	done   bool
}

func (s *rewritingStore) List(ctx context.Context, column, after string, limit int) ([]rewrap.Value, error) {
	vs, err := s.Store.List(ctx, column, after, limit)
	if err != nil || s.done || column != s.column {
		return vs, err
	}
	for _, v := range vs {
		if v.RowID == s.rowID {
			s.done = true
			err = s.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
				_, err := s.Replace(ctx, tx, column, v.RowID, v.Sealed, s.value)
				return err
			})
		}
	}
	return vs, err
}

func rewrapKeepsChanged(t *testing.T, h SealedValueHarness) {
	ctx := t.Context()
	secrets := map[string]map[string]string{}
	sealRows(t, h, "kek-old", 0, 3, secrets)
	col := h.Store.Columns()[0]
	id := sealedRowID(1)
	newer, err := kernel.NewSealer(NewKeyManager("kek-new")).Seal(ctx, []byte("rotated secret"), kernel.AAD(id, col))
	if err != nil {
		t.Fatal(err)
	}
	store := &rewritingStore{Store: h.Store, h: h, column: col, rowID: id, value: newer}
	reports, err := newRewrapService(h, store, NewKeyRing("kek-new", "kek-old")).Rewrap(ctx, nil)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	if r := reports[0]; r.Rewrapped != 2 || r.Changed != 1 || r.Failed != 0 {
		t.Errorf("report of %s = %+v; want 2 rewrapped and 1 changed", r.Column, r)
	}
	if got := sealedValueOf(t, h, col, id); !bytes.Equal(got, newer) {
		t.Error("Rewrap overwrote the value written after it read the row")
	}
	secrets[col][id] = "rotated secret"
	checkSecrets(t, h, NewKeyManager("kek-new"), secrets)
}
