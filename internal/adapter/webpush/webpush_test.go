// SPDX-License-Identifier: AGPL-3.0-only

package webpush_test

import (
	"crypto/ecdh"
	"encoding/base64"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/webpush"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/notificationtest"
)

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The example of RFC 8291 Appendix A.
const (
	rfcPlaintext  = "When I grow up, I want to be a watermelon"
	rfcASPrivate  = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcUAPublic   = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcSalt       = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcAuthSecret = "BTBZMqHH6r4Tts7J_aSIgg" //nolint:gosec // G101: the example secret of RFC 8291
	rfcBody       = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func TestEncryptMatchesRFC8291(t *testing.T) {
	as, err := ecdh.P256().NewPrivateKey(b64(t, rfcASPrivate))
	if err != nil {
		t.Fatal(err)
	}
	ua, err := ecdh.P256().NewPublicKey(b64(t, rfcUAPublic))
	if err != nil {
		t.Fatal(err)
	}
	got, err := webpush.Encrypt([]byte(rfcPlaintext), ua, b64(t, rfcAuthSecret), as, b64(t, rfcSalt))
	if err != nil {
		t.Fatal(err)
	}
	if enc := base64.RawURLEncoding.EncodeToString(got); enc != rfcBody {
		t.Errorf("Encrypt =\n %s\nwant\n %s", enc, rfcBody)
	}
}

// TestPushSenderContract runs the port's contract against a fake push service.
func TestPushSenderContract(t *testing.T) {
	notificationtest.PushSenderContract(t, func(t *testing.T, s notificationtest.PushService) appnotification.PushSender {
		t.Helper()
		priv, pub, err := webpush.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		key, err := webpush.ParsePrivateKey(priv)
		if err != nil {
			t.Fatal(err)
		}
		sender, err := webpush.New(webpush.Options{PrivateKey: key, Subject: "https://social.example", HTTP: s.HTTP, Clock: s.Clock})
		if err != nil {
			t.Fatal(err)
		}
		if sender.ServerKey() != pub {
			t.Fatalf("ServerKey = %s, want the public key of GenerateKey %s", sender.ServerKey(), pub)
		}
		return sender
	})
}

func TestNewRequiresItsOptions(t *testing.T) {
	key, err := webpush.ParsePrivateKey(rfcASPrivate)
	if err != nil {
		t.Fatal(err)
	}
	clock := testkit.NewClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	for name, o := range map[string]webpush.Options{
		"no key":         {Subject: "https://social.example", HTTP: nil, Clock: clock},
		"no subject":     {PrivateKey: key, Clock: clock},
		"no HTTP client": {PrivateKey: key, Subject: "mailto:admin@social.example", Clock: clock},
	} {
		if _, err := webpush.New(o); err == nil {
			t.Errorf("New with %s succeeded", name)
		}
	}
}

func TestParsePrivateKey(t *testing.T) {
	for _, bad := range []string{"", "not base64!", base64.RawURLEncoding.EncodeToString(make([]byte, 31)), base64.RawURLEncoding.EncodeToString(make([]byte, 32))} {
		if _, err := webpush.ParsePrivateKey(bad); err == nil {
			t.Errorf("ParsePrivateKey(%q) succeeded", bad)
		}
	}
	if _, err := webpush.ParsePrivateKey(rfcASPrivate + "="); err != nil {
		t.Errorf("ParsePrivateKey with padding: %v", err)
	}
}
