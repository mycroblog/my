// SPDX-License-Identifier: AGPL-3.0-only

package cloudtasks_test

import (
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/oidc"
	"github.com/mycroblog/my/internal/adapter/queue/cloudtasks"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestAuthenticator(t *testing.T) {
	t.Parallel()
	provider := testkit.NewFakeOIDCProvider(t)
	key := testkit.NewFakeOIDCSigner(t, "RS256")
	provider.AddKey("tasks", "RS256", key)
	ecKey := testkit.NewFakeOIDCSigner(t, "ES256")
	provider.AddKey("tasks-ec", "ES256", ecKey)
	other := testkit.NewFakeOIDCSigner(t, "RS256")
	clock := testkit.NewClock(now)
	client := oidc.New(oidc.Options{HTTP: provider, Clock: clock})
	newAuth := func(audience string) *cloudtasks.Authenticator {
		return cloudtasks.NewAuthenticator(cloudtasks.AuthOptions{
			OIDC: client, ServiceAccount: serviceAccount, Audience: audience,
			JWKSURI: testkit.FakeOIDCIssuer + "/jwks", Issuers: []string{testkit.FakeOIDCIssuer},
		})
	}
	valid := map[string]any{
		"iss": testkit.FakeOIDCIssuer, "aud": jobsURL, "sub": "1234567890",
		"email": serviceAccount, "email_verified": true,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	with := func(k string, v any) map[string]any {
		c := maps.Clone(valid)
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return c
	}
	sign := func(c map[string]any) string { return testkit.SignFakeIDToken(t, "RS256", "tasks", key, c) }

	if err := newAuth("").Authenticate(t.Context(), sign(valid), jobsURL); err != nil {
		t.Errorf("Authenticate(valid token, default audience) = %v, want nil", err)
	}
	if err := newAuth("https://my.example").Authenticate(t.Context(), sign(with("aud", "https://my.example")), jobsURL); err != nil {
		t.Errorf("Authenticate(valid token, configured audience) = %v, want nil", err)
	}
	if err := newAuth("").Authenticate(t.Context(), sign(with("email_verified", "true")), jobsURL); err != nil {
		t.Errorf(`Authenticate(email_verified "true") = %v, want nil`, err)
	}

	for name, token := range map[string]string{
		"signature":           testkit.SignFakeIDToken(t, "RS256", "tasks", other, valid),
		"alg ES256":           testkit.SignFakeIDToken(t, "ES256", "tasks-ec", ecKey, valid),
		"alg none":            testkit.SignFakeIDToken(t, "none", "", nil, valid),
		"iss":                 sign(with("iss", "https://accounts.example")),
		"aud":                 sign(with("aud", "https://my.example/_internal/tick")),
		"email":               sign(with("email", "someone@my-project.iam.gserviceaccount.com")),
		"no email":            sign(with("email", nil)),
		"email_verified":      sign(with("email_verified", false)),
		"no email_verified":   sign(with("email_verified", nil)),
		"expired":             sign(with("exp", now.Add(-2*time.Minute).Unix())),
		"issued in future":    sign(with("iat", now.Add(10*time.Minute).Unix())),
		"not a JWS":           "Bearer",
		"audience configured": sign(valid), // aud is the URL, not the configured audience
	} {
		auth := newAuth("")
		if name == "audience configured" {
			auth = newAuth("https://my.example")
		}
		if err := auth.Authenticate(t.Context(), token, jobsURL); !errors.Is(err, queue.ErrPushUnauthenticated) {
			t.Errorf("Authenticate(%s) = %v, want ErrPushUnauthenticated", name, err)
		}
	}

	provider.Refuse()
	unreachable := cloudtasks.NewAuthenticator(cloudtasks.AuthOptions{
		OIDC: oidc.New(oidc.Options{HTTP: provider, Clock: clock}), ServiceAccount: serviceAccount,
		JWKSURI: testkit.FakeOIDCIssuer + "/jwks", Issuers: []string{testkit.FakeOIDCIssuer},
	})
	err := unreachable.Authenticate(t.Context(), sign(valid), jobsURL)
	if err == nil || errors.Is(err, queue.ErrPushUnauthenticated) {
		t.Errorf("Authenticate(JWKS out of reach) = %v, want an error other than ErrPushUnauthenticated", err)
	}
}
