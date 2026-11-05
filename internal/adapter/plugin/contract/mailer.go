// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"context"
	"errors"
	"fmt"

	mailplugin "github.com/mycroblog/my/internal/adapter/mail/plugin"
	mailerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/mailer/v1/mailerv1connect"
	"github.com/mycroblog/my/internal/app/kernel"
)

// mailerCases check a mailer plugin. A real plugin sends what it gets, so without --mail-to no call of the RPC is made at
// all (ADR-0146 §16).
var mailerCases = []Case{
	{
		Name: "send",
		Skip: withoutMailTo,
		Run:  mailerSend,
		// The server cannot see the message once the plugin has it, so the person who ran the test checks this one.
		Note: "the plugin took it; check that the message arrives with the headers Message-ID, Date, and Auto-Submitted: auto-generated",
	},
	{Name: "send_no_recipient", Skip: withoutMailTo, Run: mailerNoRecipient},
}

func withoutMailTo(env Env) string {
	if env.MailTo == "" {
		return "it sends real mail; --mail-to runs it and names the recipient"
	}
	return ""
}

// mailFrom is the sender of the cases: --mail-from, or the recipient, which sends the address a message of its own.
func mailFrom(env Env) string {
	if env.MailFrom != "" {
		return env.MailFrom
	}
	return env.MailTo
}

// mailerSend makes the server's call through the adapter. It sends one message to the address the flag names.
func mailerSend(ctx context.Context, env Env) error {
	mailer, err := mailplugin.New(env.Conn, mailFrom(env))
	if err != nil {
		return err
	}
	err = mailer.Send(ctx, kernel.MailMessage{
		Category: kernel.MailTransactional,
		To:       env.MailTo,
		FromName: "my plugin test",
		Subject:  "my plugin test — テスト",
		Text: "`my plugin test` sent this message to check a mail plugin. こんにちは。\n\n" +
			"The plugin must have added the headers Message-ID, Date, and Auto-Submitted: auto-generated.\n",
		HTML: "<p><code>my plugin test</code> sent this message to check a mail plugin. こんにちは。</p>" +
			"<p>The plugin must have added the headers Message-ID, Date, and Auto-Submitted: auto-generated.</p>",
	})
	if err == nil || errors.Is(err, kernel.ErrInvalidMail) || errors.Is(err, kernel.ErrMailRejected) {
		return err
	}
	return describeCallError(env, err)
}

// mailerNoRecipient sends what the adapter would refuse, which reaches nobody: the plugin must refuse it for good, since
// sending it again cannot help.
func mailerNoRecipient(ctx context.Context, env Env) error {
	res, err := mailerv1connect.NewMailerServiceClient(env.Conn.RPC).Send(ctx, &mailerv1.SendRequest{
		Category: mailerv1.Category_CATEGORY_TRANSACTIONAL, EnvelopeFrom: mailFrom(env), Subject: "my plugin test", Text: "No recipient.\n",
	})
	if err != nil {
		return describeCallError(env, err)
	}
	switch res.GetOutcome() {
	case mailerv1.Outcome_OUTCOME_INVALID, mailerv1.Outcome_OUTCOME_REJECTED:
		return nil
	}
	return fmt.Errorf("no recipient: the outcome is %s, not OUTCOME_INVALID or OUTCOME_REJECTED", res.GetOutcome())
}
