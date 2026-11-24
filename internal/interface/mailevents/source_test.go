// SPDX-License-Identifier: AGPL-3.0-only

package mailevents

import (
	"net/http"
	"strings"
	"testing"
)

// complaintFrom is exampleComplaint with its mail.source replaced; newEnv's instance sends from
// john@example.com, the source of the example.
func complaintFrom(source string) string {
	return strings.Replace(exampleComplaint, `"source":"john@example.com"`, `"source":"`+source+`"`, 1)
}

// complaintWithoutMail is exampleComplaint without its mail object.
const complaintWithoutMail = `{
  "notificationType":"Complaint",
  "complaint":{
    "complainedRecipients":[{"emailAddress":"richard@example.com"}],
    "complaintFeedbackType":"abuse",
    "timestamp":"2016-01-27T14:59:38.237Z",
    "feedbackId":"000001378603177f-18c07c78-fa81-4a58-9dd1-fedc3cb8f49a-000000"
  }
}`

// A notification of mail another sender sent, as when instances share an SES configuration set
// or SNS topic, is acknowledged and suppresses nothing, even for a local account's address.
func TestIgnoresTheMailOfAnotherSender(t *testing.T) {
	for name, message := range map[string]string{
		"complaint of another address":        complaintFrom("sender@example.com"),
		"bounce of another address":           exampleSuppressed,
		"same address on another domain":      complaintFrom("john@example.org"),
		"local part in another case":          complaintFrom("John@example.com"),
		"no mail object":                      complaintWithoutMail,
		"empty source":                        complaintFrom(""),
		"source that is not an address":       complaintFrom("not an address"),
		"display name of ours, other address": complaintFrom("john@example.com <sender@example.com>"),
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, newRecorder("richard@example.com", "recipient@example.com"))
			m := notification(message)
			e.signer.sign(t, m)
			if code := e.postMessage(t, m); code != http.StatusOK {
				t.Fatalf("status %d, want 200", code)
			}
			if len(e.rec.rows) != 0 {
				t.Errorf("recorded %+v, want nothing", e.rec.rows)
			}
		})
	}
}

// A notification of mail this instance sent is recorded, its source compared as an address.
func TestRecordsTheMailOfThisInstance(t *testing.T) {
	for name, message := range map[string]string{
		"same address":           exampleComplaint,
		"with a display name":    complaintFrom("John Doe <john@example.com>"),
		"domain in another case": complaintFrom("john@EXAMPLE.com"),
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, newRecorder("richard@example.com"))
			m := notification(message)
			e.signer.sign(t, m)
			if code := e.postMessage(t, m); code != http.StatusOK {
				t.Fatalf("status %d, want 200", code)
			}
			if _, ok := e.rec.rows["000001378603177f-18c07c78-fa81-4a58-9dd1-fedc3cb8f49a-000000"]; !ok || len(e.rec.rows) != 1 {
				t.Errorf("recorded %+v, want the complaint", e.rec.rows)
			}
		})
	}
}
