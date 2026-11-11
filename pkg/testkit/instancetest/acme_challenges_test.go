// SPDX-License-Identifier: AGPL-3.0-only

package instancetest_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
)

func TestACMEChallengesAnswerFromTheKV(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	clock := testkit.NewClock(time.Now())
	kv := testkit.NewKeyValueStore(clock)
	// Two processes share the KV: one presents, the other answers.
	presenter, answerer := instance.NewACMEChallenges(kv), instance.NewACMEChallenges(kv)
	http01 := instance.ACMEChallenge{Type: instance.ACMEChallengeHTTP01, Host: "social.example", Token: "tok", KeyAuthorization: "tok.thumb"}
	alpn := instance.ACMEChallenge{Type: instance.ACMEChallengeTLSALPN01, Host: "Social.Example.", KeyAuthorization: "alpn.thumb"}
	for _, ch := range []instance.ACMEChallenge{http01, alpn} {
		if err := presenter.Present(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := answerer.HTTP01(ctx, "tok"); err != nil || got != "tok.thumb" {
		t.Errorf("HTTP01 = %q, %v", got, err)
	}
	if _, err := answerer.HTTP01(ctx, "other"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("HTTP01 of an unknown token = %v, want ErrKeyNotFound", err)
	}
	c, err := answerer.TLSALPN01Certificate(ctx, "social.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := instancetest.CheckTLSALPN01Certificate(c.Leaf, instance.ACMEChallenge{Host: "social.example", KeyAuthorization: "alpn.thumb"}); err != nil {
		t.Error(err)
	}
	if _, err := answerer.TLSALPN01Certificate(ctx, "unknown.example"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("TLSALPN01Certificate of an unknown host = %v, want ErrKeyNotFound", err)
	}
	// The answers expire after ACMEChallengeTTL, and CleanUp removes them.
	if err := presenter.CleanUp(ctx, http01); err != nil {
		t.Fatal(err)
	}
	if _, err := answerer.HTTP01(ctx, "tok"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("HTTP01 after CleanUp = %v", err)
	}
	clock.Advance(instance.ACMEChallengeTTL + time.Second)
	if _, err := answerer.TLSALPN01Certificate(ctx, "social.example"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("TLSALPN01Certificate after the TTL = %v", err)
	}
	if err := presenter.Present(ctx, instance.ACMEChallenge{Type: "dns-01", Host: "social.example"}); err == nil {
		t.Error("Present of a dns-01 challenge = nil, want an error")
	}
}
