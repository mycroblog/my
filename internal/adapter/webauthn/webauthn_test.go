// SPDX-License-Identifier: AGPL-3.0-only

package webauthn_test

import (
	"encoding/json"
	"testing"

	"github.com/mycroblog/my/internal/adapter/webauthn"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestPasskeyCeremoniesContract(t *testing.T) {
	identitytest.PasskeyCeremoniesContract(t, func(t *testing.T, rp identitytest.PasskeyRelyingParty) appidentity.PasskeyCeremonies {
		c, err := webauthn.New(webauthn.Options{RPID: rp.ID, DisplayName: rp.Name, Origins: rp.Origins})
		if err != nil {
			t.Fatal(err)
		}
		return c
	})
}

func TestTimeoutDefaultsToTheCeremonyTTL(t *testing.T) {
	c, err := webauthn.New(webauthn.Options{RPID: "social.example", DisplayName: "social.example", Origins: []string{"https://social.example"}})
	if err != nil {
		t.Fatal(err)
	}
	options, _, err := c.BeginSignIn()
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Timeout int64 `json:"timeout"`
	}
	if err := json.Unmarshal(options, &o); err != nil || o.Timeout != appidentity.PasskeyCeremonyTTL.Milliseconds() {
		t.Errorf("timeout = %d ms, %v; want %d", o.Timeout, err, appidentity.PasskeyCeremonyTTL.Milliseconds())
	}
}

// A development server or the E2E run on localhost over plain HTTP, which browsers treat as
// a secure context, with the web client on another port.
func TestRunsOnLocalhost(t *testing.T) {
	c, err := webauthn.New(webauthn.Options{RPID: "localhost", DisplayName: "localhost:8080", Origins: []string{"http://localhost:8080", "http://localhost:5173"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.BeginSignIn(); err != nil {
		t.Fatal(err)
	}
}

func TestNewRejectsAnInvalidRelyingParty(t *testing.T) {
	if _, err := webauthn.New(webauthn.Options{RPID: "", DisplayName: "x", Origins: []string{"https://social.example"}}); err == nil {
		t.Error("New without a relying party ID succeeded")
	}
}
