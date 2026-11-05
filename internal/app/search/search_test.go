// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"reflect"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/queue"
)

// TestPageHoldsOnlyIDsAndACursor keeps text, HTML, highlights, and scores out of the port's
// results (ADR-0149 §4): nothing a backend returns can be rendered.
func TestPageHoldsOnlyIDsAndACursor(t *testing.T) {
	typ := reflect.TypeFor[Page]()
	var fields []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		fields = append(fields, f.Name+" "+f.Type.String())
	}
	if want := []string{"StatusIDs []string", "Next string"}; !slices.Equal(fields, want) {
		t.Errorf("the fields of Page = %q, want %q", fields, want)
	}
}

// TestNoneHasNoCapability checks what search = none tells the clients (ADR-0149 §2).
func TestNoneHasNoCapability(t *testing.T) {
	s := NewService(nil)
	if s.Enabled() {
		t.Error("Enabled() = true with no backend")
	}
	if got := s.Visibilities(); len(got) != 0 {
		t.Errorf("Visibilities() = %v with no backend", got)
	}
}

// TestJobsSucceedWithoutABackend checks that every job type exists with search = none, and that
// its handler succeeds doing nothing (ADR-0149 §7).
func TestJobsSucceedWithoutABackend(t *testing.T) {
	reg := &queue.JobHandlerRegistry{}
	NewService(nil).RegisterJobs(reg)
	for _, jobType := range []string{JobSyncStatus, JobSyncAuthor, JobSyncDomain, JobNarrow, JobRebuild} {
		h, _, ok := reg.Lookup(jobType)
		if !ok {
			t.Errorf("no handler of %s", jobType)
			continue
		}
		if res := h(t.Context(), queue.Job{Type: jobType, Payload: []byte(`not what it expects`)}); !res.IsDone() {
			t.Errorf("%s: %+v, want done", jobType, res)
		}
	}
}
