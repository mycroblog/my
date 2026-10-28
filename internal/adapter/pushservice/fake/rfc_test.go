// SPDX-License-Identifier: AGPL-3.0-only

package fake

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The example of RFC 8291 Appendix A, as the user agent sees it.
const (
	rfcPlaintext  = "When I grow up, I want to be a watermelon"
	rfcUAPrivate  = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcUAPublic   = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcAuthSecret = "BTBZMqHH6r4Tts7J_aSIgg" //nolint:gosec // G101: the example secret of RFC 8291
	rfcBody       = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func rfcUAKey(t *testing.T) *ecdh.PrivateKey {
	t.Helper()
	ua, err := ecdh.P256().NewPrivateKey(b64(t, rfcUAPrivate))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()); got != rfcUAPublic {
		t.Fatalf("the public key of ua_private = %s, want ua_public %s", got, rfcUAPublic)
	}
	return ua
}

func TestDecryptRFC8291AppendixA(t *testing.T) {
	ua := rfcUAKey(t)
	body := b64(t, rfcBody)
	got, err := Decrypt(body, ua, b64(t, rfcAuthSecret))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != rfcPlaintext {
		t.Errorf("Decrypt = %q, want %q", got, rfcPlaintext)
	}

	flip := func(i int) []byte {
		b := append([]byte(nil), body...)
		b[i] ^= 1
		return b
	}
	otherAuth := b64(t, rfcAuthSecret)
	otherAuth[0] ^= 1
	for _, tt := range []struct {
		name string
		body []byte
		auth []byte
		want string
	}{
		{"a short header", body[:20], nil, "shorter than the aes128gcm header"},
		{"a changed salt", flip(0), nil, "does not decrypt"},
		{"a record size below 18", append(append(append([]byte(nil), body[:16]...), 0, 0, 0, 17), body[20:]...), nil, "below 18"},
		{"a record larger than the record size", append(append(append([]byte(nil), body[:16]...), 0, 0, 0, 18), body[20:]...), nil, "more than one record"},
		{"a key ID of another length", append(append(append([]byte(nil), body[:20]...), 64), body[21:]...), nil, "key ID is 64 bytes"},
		{"a key ID off the curve", flip(21 + 64), nil, "not a P-256 public key"},
		{"a changed record", flip(len(body) - 1), nil, "does not decrypt"},
		{"another authentication secret", body, otherAuth, "does not decrypt"},
		{"no record", body[:21+65], nil, "shorter than its tag"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			auth := tt.auth
			if auth == nil {
				auth = b64(t, rfcAuthSecret)
			}
			_, err := Decrypt(tt.body, ua, auth)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Decrypt = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

// The example of RFC 8292 §2.4 (Figure 1).
const (
	rfcToken = "eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiJ9.eyJhdWQiOiJodHRwczovL3B1c2guZXhhbXBsZS5uZXQiLCJleHAiOjE0NTM1MjM3NjgsInN1YiI6Im1haWx0bzpwdXNoQGV4YW1wbGUuY29tIn0.i3CYb7t4xfxCDquptFOepC9GAu_HLGkMlMuCGSK2rpiUfnK9ojFwDXb1JrErtmysazNjjvW2L9OkSSHzvoD1oA" //nolint:gosec // G101: the example token of RFC 8292, expired in 2016
	rfcKey   = "BA1Hxzyi1RUM1b5wjxsn7nGxAszw2u61m164i3MrAIxHF6YK5h4SDYic-dRuU_RCPCfA5aq9ojSwk5Y2EmClBPs"
	rfcAud   = "https://push.example.net"
	rfcExp   = 1453523768
)

func TestVerifyVAPIDRFC8292Example(t *testing.T) {
	exp := time.Unix(rfcExp, 0)
	header := "vapid t=" + rfcToken + ", k=" + rfcKey
	v, err := VerifyVAPID(header, rfcAud, exp.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if v.Subject != "mailto:push@example.com" || !v.Expires.Equal(exp) || base64.RawURLEncoding.EncodeToString(v.Key) != rfcKey {
		t.Errorf("VerifyVAPID = %+v", v)
	}

	parts := strings.Split(rfcToken, ".")
	sig := b64(t, parts[2])
	sig[10] ^= 1
	badSig := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig)
	// The key of RFC 8291 Appendix A: a valid P-256 key that did not sign the token.
	otherKey := rfcUAPublic
	for _, tt := range []struct {
		name   string
		header string
		origin string
		now    time.Time
		want   string
	}{
		{"the header with padded k", "vapid t=" + rfcToken + ", k=" + rfcKey + "=", rfcAud, exp.Add(-time.Hour), ""},
		{"the scheme in capitals", "Vapid t=" + rfcToken + ",k=" + rfcKey, rfcAud, exp.Add(-time.Hour), ""},
		{"another origin", header, "https://push.example.org", exp.Add(-time.Hour), "aud is"},
		{"an expired token", header, rfcAud, exp, "expired"},
		{"exp more than 24 hours ahead", header, rfcAud, exp.Add(-24*time.Hour - time.Second), "more than 24 hours"},
		{"a bad signature", "vapid t=" + badSig + ", k=" + rfcKey, rfcAud, exp.Add(-time.Hour), "does not verify"},
		{"another key", "vapid t=" + rfcToken + ", k=" + otherKey, rfcAud, exp.Add(-time.Hour), "does not verify"},
		{"k off the curve", "vapid t=" + rfcToken + ", k=" + base64.RawURLEncoding.EncodeToString(make([]byte, 65)), rfcAud, exp.Add(-time.Hour), "not an uncompressed P-256"},
		{"no k", "vapid t=" + rfcToken, rfcAud, exp.Add(-time.Hour), "both t and k"},
		{"not a JWS", "vapid t=abc, k=" + rfcKey, rfcAud, exp.Add(-time.Hour), "compact serialization"},
		{"another algorithm", "vapid t=" + base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + parts[1] + "." + parts[2] + ", k=" + rfcKey, rfcAud, exp.Add(-time.Hour), "vapid needs ES256"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := VerifyVAPID(tt.header, tt.origin, tt.now)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("VerifyVAPID = %v, want it accepted", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("VerifyVAPID = %v, want an error containing %q", err, tt.want)
			}
		})
	}

	for _, h := range []string{"", "Bearer abc", "WebPush " + rfcToken} {
		if _, err := VerifyVAPID(h, rfcAud, exp.Add(-time.Hour)); !errors.Is(err, errNoVAPID) {
			t.Errorf("VerifyVAPID(%q) = %v, want errNoVAPID", h, err)
		}
	}
}
