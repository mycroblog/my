// SPDX-License-Identifier: AGPL-3.0-only

package contract_test

import (
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/contract"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	mailerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
)

func runMailer(t *testing.T, o fakeplugin.Options, mailTo, mailFrom string) map[string]contract.Result {
	t.Helper()
	k, _ := kind.ByName(kind.Mailer)
	h, _ := fakeplugin.JSON(o)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, KeepDetail: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	out := map[string]contract.Result{}
	contract.Run(t.Context(), k, contract.Env{Conn: conn, Protocol: k.Protocol, MailTo: mailTo, MailFrom: mailFrom}, func(r contract.Result) { out[r.Case] = r })
	return out
}

// Without --mail-to no request reaches the plugin's RPC at all.
func TestMailerSendsNothingWithoutMailTo(t *testing.T) {
	box := new(fakeplugin.Mailbox)
	results := runMailer(t, fakeplugin.Options{Mailbox: box}, "", "")
	for _, name := range []string{"send", "send_no_recipient"} {
		if r := results[name]; r.Outcome != contract.Skip || !strings.Contains(r.Detail, "--mail-to") {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if n := len(box.Requests()); n != 0 {
		t.Errorf("%d requests without --mail-to", n)
	}
}

func TestMailerCases(t *testing.T) {
	box := new(fakeplugin.Mailbox)
	results := runMailer(t, fakeplugin.Options{Mailbox: box}, "me@example.com", "noreply@social.example")
	for _, name := range []string{"send", "send_no_recipient"} {
		if r := results[name]; r.Outcome != contract.Pass {
			t.Errorf("%s: %+v", name, r)
		}
	}
	// What the plugin must add can be seen only on the message, so the case tells the person to look.
	for _, header := range []string{"Message-ID", "Date", "Auto-Submitted"} {
		if !strings.Contains(results["send"].Detail, header) {
			t.Errorf("the note of send lacks %s: %q", header, results["send"].Detail)
		}
	}
	got := box.Delivered()
	if len(got) != 1 || got[0].GetTo() != "me@example.com" || got[0].GetEnvelopeFrom() != "noreply@social.example" {
		t.Fatalf("delivered = %v", got)
	}
	for _, header := range []string{"Message-ID", "Date", "Auto-Submitted"} {
		if !strings.Contains(got[0].GetText(), header) {
			t.Errorf("the message lacks %s: %q", header, got[0].GetText())
		}
	}

	// The sender falls back to the recipient.
	box = new(fakeplugin.Mailbox)
	runMailer(t, fakeplugin.Options{Mailbox: box}, "me@example.com", "")
	if got := box.Delivered(); len(got) != 1 || got[0].GetEnvelopeFrom() != "me@example.com" {
		t.Errorf("delivered = %v", got)
	}
}

// A plugin's mistake fails the case, with the reason.
func TestMailerCasesFail(t *testing.T) {
	for name, tc := range map[string]struct {
		options fakeplugin.Options
		mailTo  string
		want    map[string]string
	}{
		"no outcome": {
			options: fakeplugin.Options{Mailbox: new(fakeplugin.Mailbox), Broken: true}, mailTo: "me@example.com",
			want: map[string]string{"send": "outcome", "send_no_recipient": "OUTCOME_UNSPECIFIED"},
		},
		"no RPC": {
			mailTo: "me@example.com",
			want:   map[string]string{"send": "does the plugin serve this RPC", "send_no_recipient": "does the plugin serve this RPC"},
		},
		"a refused recipient": {
			options: fakeplugin.Options{Mailbox: &fakeplugin.Mailbox{Answers: map[string]*mailerv1.SendResponse{
				"me@example.com": {Outcome: mailerv1.Outcome_OUTCOME_RECIPIENT_REJECTED},
			}}},
			mailTo: "me@example.com",
			want:   map[string]string{"send": "recipient rejected"},
		},
		"a recipient that is not an address": {
			options: fakeplugin.Options{Mailbox: new(fakeplugin.Mailbox)}, mailTo: "not an address",
			want: map[string]string{"send": "bare address"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			results := runMailer(t, tc.options, tc.mailTo, "")
			for c, want := range tc.want {
				if r := results[c]; r.Outcome != contract.Fail || !strings.Contains(r.Detail, want) {
					t.Errorf("%s: %+v, want a failure with %q", c, r, want)
				}
			}
		})
	}
}
