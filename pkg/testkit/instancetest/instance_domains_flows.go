// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"errors"
	"sync"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceDomainCases are the cases of InstanceFlows about the domains of the
// instance (ADR-0018): the first values are kept, and any other value is a mismatch.
var instanceDomainCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"RecordsDomainsOnce", instanceRecordsDomainsOnce},
	{"RefusesOtherDomains", instanceRefusesOtherDomains},
	{"CheckingDomainsRecordsNothing", instanceCheckingDomainsRecordsNothing},
	{"ConcurrentFirstDomainsAgree", instanceConcurrentFirstDomainsAgree},
}

var instanceTestDomains = instance.Domains{Host: "social.example.org", AccountDomain: "example.org"}

func instanceRecordsDomainsOnce(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	if _, err := e.h.Instance.FindDomains(ctx); !errors.Is(err, appinstance.ErrNotFound) {
		t.Fatalf("FindDomains of a new instance = %v, want ErrNotFound", err)
	}
	if err := e.service.EnsureDomains(ctx, instanceTestDomains); err != nil {
		t.Fatalf("EnsureDomains = %v", err)
	}
	recordedAt := e.clock.Now()
	e.clock.Advance(time.Hour)
	if err := e.service.EnsureDomains(ctx, instanceTestDomains); err != nil {
		t.Errorf("EnsureDomains again = %v", err)
	}
	got, err := e.h.Instance.FindDomains(ctx)
	if err != nil {
		t.Fatalf("FindDomains = %v", err)
	}
	if !got.Same(instanceTestDomains) || !got.RecordedAt.Equal(recordedAt) {
		t.Errorf("FindDomains = %+v, want the domains recorded at %v", got, recordedAt)
	}
}

func instanceRefusesOtherDomains(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	if err := e.service.EnsureDomains(ctx, instanceTestDomains); err != nil {
		t.Fatalf("EnsureDomains = %v", err)
	}
	for _, other := range []instance.Domains{
		{Host: "other.example.org", AccountDomain: instanceTestDomains.AccountDomain},
		{Host: instanceTestDomains.Host, AccountDomain: "other.example.org"},
	} {
		var mismatch *instance.DomainMismatchError
		if err := e.service.EnsureDomains(ctx, other); !errors.As(err, &mismatch) {
			t.Errorf("EnsureDomains(%+v) = %v, want a DomainMismatchError", other, err)
		} else if !mismatch.Recorded.Same(instanceTestDomains) || !mismatch.Configured.Same(other) {
			t.Errorf("mismatch = %+v, want recorded %+v and configured %+v", mismatch, instanceTestDomains, other)
		}
		if err := e.service.CheckDomains(ctx, other); !errors.As(err, &mismatch) {
			t.Errorf("CheckDomains(%+v) = %v, want a DomainMismatchError", other, err)
		}
	}
	if got, err := e.h.Instance.FindDomains(ctx); err != nil || !got.Same(instanceTestDomains) {
		t.Errorf("FindDomains after the refusals = %+v, %v", got, err)
	}
}

func instanceCheckingDomainsRecordsNothing(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	if err := e.service.CheckDomains(ctx, instanceTestDomains); err != nil {
		t.Fatalf("CheckDomains of a new instance = %v", err)
	}
	if _, err := e.h.Instance.FindDomains(ctx); !errors.Is(err, appinstance.ErrNotFound) {
		t.Errorf("FindDomains after CheckDomains = %v, want ErrNotFound", err)
	}
}

func instanceConcurrentFirstDomainsAgree(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() { errs[i] = e.service.EnsureDomains(ctx, instanceTestDomains) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("EnsureDomains %d = %v", i, err)
		}
	}
}
