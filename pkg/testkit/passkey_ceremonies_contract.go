// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// PasskeyRelyingParty is the relying party that PasskeyCeremoniesContract configures the
// ceremonies under test with.
type PasskeyRelyingParty struct {
	// ID is the relying party ID: the instance's host name.
	ID string
	// Name names the instance in the browser's prompts.
	Name string
	// Origins are the origins that may run the ceremonies: the server's and the web client's.
	Origins []string
}

// PasskeyCeremoniesFactory returns the ceremonies under test for rp.
type PasskeyCeremoniesFactory func(t *testing.T, rp PasskeyRelyingParty) appidentity.PasskeyCeremonies

// The contract's relying party and user.
var (
	pcRP = PasskeyRelyingParty{
		ID: "social.example", Name: "social.example",
		Origins: []string{"https://social.example", "https://app.social.example"},
	}
	pcAlice = appidentity.PasskeyUser{Handle: []byte("019a0000-0000-7000-8000-000000000001"), Name: "alice@social.example"}
)

// PasskeyCeremoniesContract verifies an appidentity.PasskeyCeremonies adapter with a software
// authenticator that answers as a browser's PublicKeyCredential.toJSON does (W3C Web
// Authentication Level 3), without the network. It checks that the registration options ask
// for a discoverable credential with user verification and no attestation, for the relying
// party and the user, excluding the given credentials, with a timeout no longer than
// PasskeyCeremonyTTL; that the sign-in options ask for a discoverable credential with user
// verification; that a registration and a sign-in from each allowed origin verify and return
// the credential, its public key, and its backup eligibility, looking the credential up by
// its ID; and that a response from another origin, without user verification, for another
// user or another ceremony's challenge, signed by another key, with a changed backup
// eligibility, of an unknown credential, or that is not a credential at all fails with
// ErrInvalidPasskey, while an error of the lookup is returned as it is.
func PasskeyCeremoniesContract(t *testing.T, newCeremonies PasskeyCeremoniesFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, c appidentity.PasskeyCeremonies)
	}{
		{"RegistrationOptions", pcRegistrationOptions},
		{"SignInOptions", pcSignInOptions},
		{"RegistersAndSignsIn", pcRegistersAndSignsIn},
		{"RegistrationRejectsWhatDoesNotVerify", pcRegistrationRejects},
		{"SignInRejectsWhatDoesNotVerify", pcSignInRejects},
		{"SignInReturnsLookupErrors", pcSignInReturnsLookupErrors},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, newCeremonies(t, pcRP))
		})
	}
}

var pcB64 = base64.RawURLEncoding

// pcCBOR encodes the few CBOR items (RFC 8949) that an attestation object and a COSE key use.
type pcCBOR []byte

func (c pcCBOR) head(major byte, n uint64) pcCBOR {
	switch {
	case n < 24:
		return append(c, major<<5|byte(n))
	case n < 1<<8:
		return append(c, major<<5|24, byte(n))
	default:
		return binary.BigEndian.AppendUint16(append(c, major<<5|25), uint16(n)) //nolint:gosec // G115: test data under 64 KiB
	}
}

func (c pcCBOR) int(n int) pcCBOR {
	if n < 0 {
		return c.head(1, uint64(-1-n))
	}
	return c.head(0, uint64(n))
}

func (c pcCBOR) bytes(b []byte) pcCBOR { return append(c.head(2, uint64(len(b))), b...) }
func (c pcCBOR) text(s string) pcCBOR  { return append(c.head(3, uint64(len(s))), s...) }
func (c pcCBOR) mapOf(n int) pcCBOR    { return c.head(5, uint64(n)) } //nolint:gosec // G115: small maps

// Authenticator data flags (WebAuthn §6.1).
const (
	pcFlagUP = 0x01
	pcFlagUV = 0x04
	pcFlagBE = 0x08
	pcFlagBS = 0x10
	pcFlagAT = 0x40
	// pcSynced are the flags of a synced passkey with user verification.
	pcSynced = pcFlagUP | pcFlagUV | pcFlagBE | pcFlagBS
)

// pcAuthenticator is a software passkey: one P-256 key of one credential.
type pcAuthenticator struct {
	rpID   string
	key    *ecdsa.PrivateKey
	credID []byte
	flags  byte
	count  uint32
}

func newPCAuthenticator(t *testing.T) *pcAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	return &pcAuthenticator{rpID: pcRP.ID, key: key, credID: id, flags: pcSynced}
}

func (a *pcAuthenticator) coseKey(t *testing.T) []byte {
	t.Helper()
	pub, err := a.key.PublicKey.Bytes() // 0x04 || X || Y
	if err != nil {
		t.Fatal(err)
	}
	// kty EC2, alg ES256, crv P-256, x, y (RFC 9053 §7.1).
	return pcCBOR{}.mapOf(5).int(1).int(2).int(3).int(-7).int(-1).int(1).int(-2).bytes(pub[1:33]).int(-3).bytes(pub[33:])
}

func (a *pcAuthenticator) authData(t *testing.T, flags byte, attested bool) []byte {
	t.Helper()
	rp := sha256.Sum256([]byte(a.rpID))
	d := append(rp[:], flags)
	a.count++
	d = binary.BigEndian.AppendUint32(d, a.count)
	if attested {
		d = append(d, make([]byte, 16)...)                          // AAGUID
		d = binary.BigEndian.AppendUint16(d, uint16(len(a.credID))) //nolint:gosec // G115: 16 bytes
		d = append(d, a.credID...)
		d = append(d, a.coseKey(t)...)
	}
	return d
}

func pcClientData(t *testing.T, typ, challenge, origin string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": origin, "crossOrigin": false})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// pcChallenge reads the challenge of the options.
func pcChallenge(t *testing.T, options []byte) string {
	t.Helper()
	var o struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(options, &o); err != nil || o.Challenge == "" {
		t.Fatalf("options %s have no challenge: %v", options, err)
	}
	return o.Challenge
}

func pcJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// register answers a registration's options from origin.
func (a *pcAuthenticator) register(t *testing.T, options []byte, origin string) []byte {
	t.Helper()
	cd := pcClientData(t, "webauthn.create", pcChallenge(t, options), origin)
	att := pcCBOR{}.mapOf(3).text("fmt").text("none").text("attStmt").mapOf(0).text("authData").bytes(a.authData(t, a.flags|pcFlagAT, true))
	return pcJSON(t, map[string]any{
		"id": pcB64.EncodeToString(a.credID), "rawId": pcB64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": pcB64.EncodeToString(cd), "attestationObject": pcB64.EncodeToString(att), "transports": []string{"internal"},
		},
		"clientExtensionResults": map[string]any{}, "authenticatorAttachment": "platform",
	})
}

// signIn answers a sign-in's options from origin for the user handle, with flags.
func (a *pcAuthenticator) signIn(t *testing.T, options, handle []byte, flags byte, origin string) []byte {
	t.Helper()
	cd := pcClientData(t, "webauthn.get", pcChallenge(t, options), origin)
	ad := a.authData(t, flags, false)
	hash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), hash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return pcJSON(t, map[string]any{
		"id": pcB64.EncodeToString(a.credID), "rawId": pcB64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": pcB64.EncodeToString(cd), "authenticatorData": pcB64.EncodeToString(ad),
			"signature": pcB64.EncodeToString(sig), "userHandle": pcB64.EncodeToString(handle),
		},
		"clientExtensionResults": map[string]any{}, "authenticatorAttachment": "platform",
	})
}

// SoftwarePasskey is a software authenticator for a test through a real server: one credential of
// the relying party, whose answers verify on the real WebAuthn ceremonies (the adapter of
// PasskeyCeremoniesContract), as a browser at Origin would send them.
type SoftwarePasskey struct {
	a      *pcAuthenticator
	origin string
}

// NewSoftwarePasskey returns a new credential of the relying party rpID, at origin.
func NewSoftwarePasskey(t *testing.T, rpID, origin string) *SoftwarePasskey {
	t.Helper()
	a := newPCAuthenticator(t)
	a.rpID = rpID
	return &SoftwarePasskey{a: a, origin: origin}
}

// At returns the same credential answering from another origin.
func (p *SoftwarePasskey) At(origin string) *SoftwarePasskey {
	return &SoftwarePasskey{a: p.a, origin: origin}
}

// Register answers the options of a registration with the credential.
func (p *SoftwarePasskey) Register(t *testing.T, options []byte) []byte {
	t.Helper()
	return p.a.register(t, options, p.origin)
}

// SignIn answers the options of a sign-in or a confirmation as the user with handle.
func (p *SoftwarePasskey) SignIn(t *testing.T, options, handle []byte) []byte {
	t.Helper()
	return p.a.signIn(t, options, handle, p.a.flags, p.origin)
}

// pcRegister registers a's credential for alice from origin and returns it as the identity
// module stores it.
func pcRegister(t *testing.T, c appidentity.PasskeyCeremonies, a *pcAuthenticator, origin string) identity.PasskeyCredential {
	t.Helper()
	options, state, err := c.BeginRegistration(pcAlice, nil)
	if err != nil {
		t.Fatal(err)
	}
	created, err := c.FinishRegistration(pcAlice, state, a.register(t, options, origin))
	if err != nil {
		t.Fatalf("FinishRegistration = %v", err)
	}
	return identity.PasskeyCredential{
		ID: "p1", UserID: string(pcAlice.Handle), CredentialID: created.CredentialID, PublicKey: created.PublicKey,
		BackupEligible: created.BackupEligible, Name: "phone",
	}
}

func pcRegistrationOptions(t *testing.T, c appidentity.PasskeyCeremonies) {
	excluded := []byte{1, 2, 3, 4}
	options, state, err := c.BeginRegistration(pcAlice, [][]byte{excluded})
	if err != nil {
		t.Fatal(err)
	}
	if len(state) == 0 {
		t.Error("BeginRegistration returned no state")
	}
	var o struct {
		Challenge string `json:"challenge"`
		RP        struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"rp"`
		User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
		PubKeyCredParams []struct {
			Type string `json:"type"`
			Alg  int    `json:"alg"`
		} `json:"pubKeyCredParams"`
		AuthenticatorSelection struct {
			ResidentKey      string `json:"residentKey"`
			UserVerification string `json:"userVerification"`
		} `json:"authenticatorSelection"`
		Attestation        string `json:"attestation"`
		ExcludeCredentials []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"excludeCredentials"`
		Timeout int64 `json:"timeout"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		t.Fatalf("registration options %s: %v", options, err)
	}
	if o.RP.ID != pcRP.ID || o.RP.Name == "" {
		t.Errorf("rp = %+v, want the ID %q and a name", o.RP, pcRP.ID)
	}
	if o.User.ID != pcB64.EncodeToString(pcAlice.Handle) || o.User.Name != pcAlice.Name {
		t.Errorf("user = %+v, want alice's handle in base64url and her name", o.User)
	}
	if challenge, err := pcB64.DecodeString(o.Challenge); err != nil || len(challenge) < 16 {
		t.Errorf("challenge = %q, want at least 16 random bytes in base64url (WebAuthn §13.4.3)", o.Challenge)
	}
	es256 := false
	for _, p := range o.PubKeyCredParams {
		es256 = es256 || (p.Type == "public-key" && p.Alg == -7)
	}
	if !es256 {
		t.Errorf("pubKeyCredParams = %+v, want ES256", o.PubKeyCredParams)
	}
	if o.AuthenticatorSelection.ResidentKey != "required" || o.AuthenticatorSelection.UserVerification != "required" {
		t.Errorf("authenticatorSelection = %+v, want a discoverable credential and user verification", o.AuthenticatorSelection)
	}
	if o.Attestation != "" && o.Attestation != "none" {
		t.Errorf("attestation = %q, want none", o.Attestation)
	}
	if len(o.ExcludeCredentials) != 1 || o.ExcludeCredentials[0].ID != pcB64.EncodeToString(excluded) || o.ExcludeCredentials[0].Type != "public-key" {
		t.Errorf("excludeCredentials = %+v, want the excluded credential", o.ExcludeCredentials)
	}
	if o.Timeout <= 0 || o.Timeout > appidentity.PasskeyCeremonyTTL.Milliseconds() {
		t.Errorf("timeout = %d ms, want at most %d", o.Timeout, appidentity.PasskeyCeremonyTTL.Milliseconds())
	}
	// Each ceremony has its own challenge.
	again, _, err := c.BeginRegistration(pcAlice, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pcChallenge(t, again) == o.Challenge {
		t.Error("two registrations share a challenge")
	}
}

func pcSignInOptions(t *testing.T, c appidentity.PasskeyCeremonies) {
	options, state, err := c.BeginSignIn()
	if err != nil {
		t.Fatal(err)
	}
	if len(state) == 0 {
		t.Error("BeginSignIn returned no state")
	}
	var o struct {
		Challenge        string `json:"challenge"`
		RPID             string `json:"rpId"`
		UserVerification string `json:"userVerification"`
		AllowCredentials []any  `json:"allowCredentials"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		t.Fatalf("sign-in options %s: %v", options, err)
	}
	if o.RPID != pcRP.ID || o.UserVerification != "required" || len(o.AllowCredentials) != 0 {
		t.Errorf("sign-in options = %s, want the relying party ID, user verification, and no allowed credentials", options)
	}
	if challenge, err := pcB64.DecodeString(o.Challenge); err != nil || len(challenge) < 16 {
		t.Errorf("challenge = %q, want at least 16 random bytes in base64url", o.Challenge)
	}
}

func pcRegistersAndSignsIn(t *testing.T, c appidentity.PasskeyCeremonies) {
	for _, origin := range pcRP.Origins {
		t.Run(origin, func(t *testing.T) {
			a := newPCAuthenticator(t)
			stored := pcRegister(t, c, a, origin)
			if string(stored.CredentialID) != string(a.credID) || len(stored.PublicKey) == 0 || !stored.BackupEligible {
				t.Fatalf("registered credential = %+v, want the authenticator's ID, a public key, and backup eligibility", stored)
			}
			options, state, err := c.BeginSignIn()
			if err != nil {
				t.Fatal(err)
			}
			var looked []byte
			got, err := c.FinishSignIn(state, a.signIn(t, options, pcAlice.Handle, pcSynced, origin), func(id []byte) (identity.PasskeyCredential, error) {
				looked = id
				return stored, nil
			})
			if err != nil || got.ID != stored.ID || string(looked) != string(a.credID) {
				t.Fatalf("FinishSignIn = %+v, %v (looked up %x)", got, err, looked)
			}
		})
	}
	// A device-bound passkey is not backup eligible.
	a := newPCAuthenticator(t)
	a.flags = pcFlagUP | pcFlagUV
	if stored := pcRegister(t, c, a, pcRP.Origins[0]); stored.BackupEligible {
		t.Error("a device-bound passkey is registered as backup eligible")
	}
}

func pcRegistrationRejects(t *testing.T, c appidentity.PasskeyCeremonies) {
	origin := pcRP.Origins[0]
	for _, tc := range []struct {
		name   string
		answer func(a *pcAuthenticator, options []byte) []byte
	}{
		{"another origin", func(a *pcAuthenticator, o []byte) []byte { return a.register(t, o, "https://evil.example") }},
		{"no user verification", func(a *pcAuthenticator, o []byte) []byte {
			a.flags = pcFlagUP
			return a.register(t, o, origin)
		}},
		{"another challenge", func(a *pcAuthenticator, _ []byte) []byte {
			other, _, err := c.BeginRegistration(pcAlice, nil)
			if err != nil {
				t.Fatal(err)
			}
			return a.register(t, other, origin)
		}},
		{"not JSON", func(*pcAuthenticator, []byte) []byte { return []byte("[]") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options, state, err := c.BeginRegistration(pcAlice, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.FinishRegistration(pcAlice, state, tc.answer(newPCAuthenticator(t), options)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
				t.Errorf("FinishRegistration = %v, want ErrInvalidPasskey", err)
			}
		})
	}
	t.Run("another user", func(t *testing.T) {
		options, state, err := c.BeginRegistration(appidentity.PasskeyUser{Handle: []byte("bob"), Name: "bob@social.example"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.FinishRegistration(pcAlice, state, newPCAuthenticator(t).register(t, options, origin)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
			t.Errorf("FinishRegistration for another user = %v, want ErrInvalidPasskey", err)
		}
	})
}

func pcSignInRejects(t *testing.T, c appidentity.PasskeyCeremonies) {
	origin := pcRP.Origins[0]
	a := newPCAuthenticator(t)
	stored := pcRegister(t, c, a, origin)
	find := func([]byte) (identity.PasskeyCredential, error) { return stored, nil }
	for _, tc := range []struct {
		name   string
		answer func(options []byte) []byte
		find   func([]byte) (identity.PasskeyCredential, error)
	}{
		{"another origin", func(o []byte) []byte { return a.signIn(t, o, pcAlice.Handle, pcSynced, "https://evil.example") }, find},
		{"no user verification", func(o []byte) []byte { return a.signIn(t, o, pcAlice.Handle, pcFlagUP|pcFlagBE|pcFlagBS, origin) }, find},
		{"another user's handle", func(o []byte) []byte { return a.signIn(t, o, []byte("someone-else"), pcSynced, origin) }, find},
		{"another key", func(o []byte) []byte {
			other := newPCAuthenticator(t)
			other.credID = a.credID
			return other.signIn(t, o, pcAlice.Handle, pcSynced, origin)
		}, find},
		{"a changed backup eligibility", func(o []byte) []byte { return a.signIn(t, o, pcAlice.Handle, pcFlagUP|pcFlagUV, origin) }, find},
		{"another challenge", func([]byte) []byte {
			other, _, err := c.BeginSignIn()
			if err != nil {
				t.Fatal(err)
			}
			return a.signIn(t, other, pcAlice.Handle, pcSynced, origin)
		}, find},
		{"an unknown credential", func(o []byte) []byte { return a.signIn(t, o, pcAlice.Handle, pcSynced, origin) }, func([]byte) (identity.PasskeyCredential, error) {
			return identity.PasskeyCredential{}, appidentity.ErrNotFound
		}},
		{"not JSON", func([]byte) []byte { return []byte("{") }, find},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options, state, err := c.BeginSignIn()
			if err != nil {
				t.Fatal(err)
			}
			if got, err := c.FinishSignIn(state, tc.answer(options), tc.find); !errors.Is(err, appidentity.ErrInvalidPasskey) {
				t.Errorf("FinishSignIn = %+v, %v; want ErrInvalidPasskey", got, err)
			}
		})
	}
}

func pcSignInReturnsLookupErrors(t *testing.T, c appidentity.PasskeyCeremonies) {
	a := newPCAuthenticator(t)
	pcRegister(t, c, a, pcRP.Origins[0])
	options, state, err := c.BeginSignIn()
	if err != nil {
		t.Fatal(err)
	}
	broken := errors.New("database down")
	_, err = c.FinishSignIn(state, a.signIn(t, options, pcAlice.Handle, pcSynced, pcRP.Origins[0]), func([]byte) (identity.PasskeyCredential, error) {
		return identity.PasskeyCredential{}, broken
	})
	if !errors.Is(err, broken) || errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("FinishSignIn with a failing lookup = %v, want %v and not ErrInvalidPasskey", err, broken)
	}
}
