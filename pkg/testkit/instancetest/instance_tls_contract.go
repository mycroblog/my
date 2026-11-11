// SPDX-License-Identifier: AGPL-3.0-only

package instancetest

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/instance"
)

const tlsDirectory = "https://ca.example/directory"

// instanceRepoTLSAccounts checks that two registrations of the same directory at once keep one
// account, the one inserted first.
func instanceRepoTLSAccounts(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	_, err := r.i.FindTLSAccount(ctx, tlsDirectory)
	wantNotFound(t, "FindTLSAccount before an insert", err)
	accounts := []instance.TLSAccount{
		{ID: r.ids.NewID(), DirectoryURL: tlsDirectory, AccountURL: "https://ca.example/acct/1", SealedPrivateKey: []byte("sealed-1"), CreatedAt: r.now()},
		{ID: r.ids.NewID(), DirectoryURL: tlsDirectory, AccountURL: "https://ca.example/acct/2", SealedPrivateKey: []byte("sealed-2"), CreatedAt: r.now()},
	}
	inserted := make([]bool, len(accounts))
	errs := make([]error, len(accounts))
	var wg sync.WaitGroup
	for i, a := range accounts {
		wg.Go(func() {
			errs[i] = r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) (err error) {
				inserted[i], err = r.i.InsertTLSAccount(ctx, tx, a)
				return err
			})
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("InsertTLSAccount at once: %v", err)
		}
	}
	if inserted[0] == inserted[1] {
		t.Fatalf("InsertTLSAccount at once = %v, want exactly one inserted", inserted)
	}
	winner := accounts[slices.Index(inserted, true)]
	if got, err := r.i.FindTLSAccount(ctx, tlsDirectory); err != nil || !reflect.DeepEqual(got, winner) {
		t.Errorf("FindTLSAccount = %+v, %v; want %+v", got, err, winner)
	}
	other := instance.TLSAccount{ID: r.ids.NewID(), DirectoryURL: "https://staging.ca.example/directory", AccountURL: "https://staging.ca.example/acct/1", SealedPrivateKey: []byte("sealed-3"), CreatedAt: r.now()}
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		ok, err = r.i.InsertTLSAccount(ctx, tx, other)
		return err
	})
	if got, err := r.i.FindTLSAccount(ctx, other.DirectoryURL); !ok || err != nil || !reflect.DeepEqual(got, other) {
		t.Errorf("InsertTLSAccount of another directory = %t; FindTLSAccount = %+v, %v", ok, got, err)
	}
}

func (r *instanceRepo) tlsAttempt(t *testing.T, host, replaces string) instance.TLSOrderAttempt {
	t.Helper()
	a := instance.TLSOrderAttempt{ID: r.ids.NewID(), Host: host, DirectoryURL: tlsDirectory, Profile: "shortlived", ReplacesCertificateID: replaces, OccurredAt: r.now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.i.InsertTLSOrderAttempt(ctx, tx, a) })
	return a
}

func (r *instanceRepo) tlsCertificate(t *testing.T, a instance.TLSOrderAttempt, validity time.Duration) instance.TLSCertificate {
	t.Helper()
	now := r.now()
	c := instance.TLSCertificate{
		ID: r.ids.NewID(), AttemptID: a.ID, Host: a.Host, DirectoryURL: a.DirectoryURL, Serial: a.ID[len(a.ID)-6:],
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(validity), ChainPEM: "-----BEGIN CERTIFICATE-----\n" + a.ID + "\n-----END CERTIFICATE-----\n",
		SealedPrivateKey: []byte("sealed-" + a.ID), CreatedAt: now,
	}
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		ok, err = r.i.InsertTLSCertificate(ctx, tx, c)
		return err
	})
	if !ok {
		t.Fatalf("InsertTLSCertificate(%s) = false, want inserted", c.ID)
	}
	return c
}

func (r *instanceRepo) wantCurrentTLSCertificate(t *testing.T, host string, want instance.TLSCertificate) {
	t.Helper()
	if got, err := r.i.CurrentTLSCertificate(t.Context(), host); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("CurrentTLSCertificate(%s) = %+v, %v; want %+v", host, got, err, want)
	}
	if got, err := r.i.CurrentTLSCertificateID(t.Context(), host); err != nil || got != want.ID {
		t.Errorf("CurrentTLSCertificateID(%s) = %q, %v; want %q", host, got, err, want.ID)
	}
}

func (r *instanceRepo) wantUnresulted(t *testing.T, host string, after time.Time, want ...instance.TLSOrderAttempt) {
	t.Helper()
	got, err := r.i.ListUnresultedTLSOrderAttempts(t.Context(), host, after)
	if err != nil {
		t.Fatalf("ListUnresultedTLSOrderAttempts: %v", err)
	}
	if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
		t.Errorf("ListUnresultedTLSOrderAttempts(%s, %s) = %+v, want %+v", host, after, got, want)
	}
}

// instanceRepoTLSCertificates checks that the current certificate of a host is the last one
// issued that is not revoked, by created_at even when an older one ends later, and which attempts
// have no certificate.
func instanceRepoTLSCertificates(t *testing.T, r *instanceRepo) {
	ctx := t.Context()
	const host = "social.example"
	_, err := r.i.CurrentTLSCertificate(ctx, host)
	wantNotFound(t, "CurrentTLSCertificate before an insert", err)
	_, err = r.i.CurrentTLSCertificateID(ctx, host)
	wantNotFound(t, "CurrentTLSCertificateID before an insert", err)
	r.wantUnresulted(t, host, time.Time{})

	first := r.tlsCertificate(t, r.tlsAttempt(t, host, ""), 90*24*time.Hour)
	r.wantCurrentTLSCertificate(t, host, first)
	// A renewal with a shorter profile ends before the certificate it replaces.
	second := r.tlsCertificate(t, r.tlsAttempt(t, host, first.ID), 6*24*time.Hour)
	if !second.NotAfter.Before(first.NotAfter) {
		t.Fatal("the test needs the second certificate to end first")
	}
	r.wantCurrentTLSCertificate(t, host, second)
	// Another host's certificate is not this host's.
	elsewhere := r.tlsCertificate(t, r.tlsAttempt(t, "media.example", ""), 90*24*time.Hour)
	r.wantCurrentTLSCertificate(t, host, second)
	r.wantCurrentTLSCertificate(t, "media.example", elsewhere)

	// An attempt has one certificate.
	again := second
	again.ID = r.ids.NewID()
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		ok, err = r.i.InsertTLSCertificate(ctx, tx, again)
		return err
	})
	if ok {
		t.Error("InsertTLSCertificate of an attempt that has one = true, want false")
	}

	// A revoked certificate is never current; the one before it is again.
	revocation := instance.TLSCertificateRevocation{ID: r.ids.NewID(), CertificateID: second.ID, Reason: instance.TLSRevocationKeyCompromise, OccurredAt: r.now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.i.InsertTLSCertificateRevocation(ctx, tx, revocation); err != nil {
			return err
		}
		return r.i.InsertTLSCertificateRevocation(ctx, tx, revocation)
	})
	r.wantCurrentTLSCertificate(t, host, first)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.i.InsertTLSCertificateRevocation(ctx, tx, instance.TLSCertificateRevocation{
			ID: r.ids.NewID(), CertificateID: first.ID, Reason: instance.TLSRevocationOperator, ByUserID: r.ids.NewID(), OccurredAt: r.now(),
		})
	})
	_, err = r.i.CurrentTLSCertificate(ctx, host)
	wantNotFound(t, "CurrentTLSCertificate with every certificate revoked", err)
	_, err = r.i.CurrentTLSCertificateID(ctx, host)
	wantNotFound(t, "CurrentTLSCertificateID with every certificate revoked", err)

	// The attempts without a certificate: a failure recorded (twice), and one whose process died.
	r.wantUnresulted(t, host, time.Time{})
	failed := r.tlsAttempt(t, host, first.ID)
	failure := instance.TLSOrderFailure{AttemptID: failed.ID, ProblemType: "urn:ietf:params:acme:error:rateLimited", Detail: "too many", OccurredAt: r.now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.i.InsertTLSOrderFailure(ctx, tx, failure); err != nil {
			return err
		}
		return r.i.InsertTLSOrderFailure(ctx, tx, failure)
	})
	died := r.tlsAttempt(t, host, "")
	r.wantUnresulted(t, host, time.Time{}, failed, died)
	r.wantUnresulted(t, host, second.CreatedAt, failed, died)
	r.wantUnresulted(t, host, failed.OccurredAt, died)
	r.wantUnresulted(t, "media.example", time.Time{})

	// The latest attempts with their results, newest first.
	got, err := r.i.ListTLSOrderAttempts(ctx, host, 3)
	if err != nil {
		t.Fatalf("ListTLSOrderAttempts: %v", err)
	}
	want := []instance.TLSOrderAttemptResult{
		{Attempt: died},
		{Attempt: failed, Failure: &failure},
		{Attempt: instance.TLSOrderAttempt{
			ID: second.AttemptID, Host: host, DirectoryURL: tlsDirectory, Profile: "shortlived", ReplacesCertificateID: first.ID,
		}, CertificateID: second.ID},
	}
	if len(got) == 3 {
		want[2].Attempt.OccurredAt = got[2].Attempt.OccurredAt
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListTLSOrderAttempts(%s, 3) = %+v, want %+v", host, got, want)
	}
	if got, err := r.i.ListTLSOrderAttempts(ctx, "other.example", 3); err != nil || len(got) != 0 {
		t.Errorf("ListTLSOrderAttempts of a host without orders = %+v, %v", got, err)
	}

	// A certificate is superseded once a later one of its host is issued, revoked or not.
	for id, want := range map[string]bool{first.ID: true, second.ID: false, elsewhere.ID: false, r.ids.NewID(): false} {
		if got, err := r.i.TLSCertificateSuperseded(ctx, id); err != nil || got != want {
			t.Errorf("TLSCertificateSuperseded(%s) = %t, %v; want %t", id, got, err, want)
		}
	}
}
