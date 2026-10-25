// SPDX-License-Identifier: AGPL-3.0-only

// Package ses implements the mailer port on the Amazon SES v2 API (mail = ses, ADR-0086 §6):
// each message goes out with one SendEmail call as raw content, the message in RFC 5322 form
// that package message assembles, the same as mail/smtp writes. SES does not offer SMTPUTF8,
// so the addresses are 7-bit ASCII (the domain in Punycode) and the sender's name an encoded
// word. SES replaces the Message-ID and the Date of the message with its own.
//
// The credentials come from the SDK's default chain (the execution role, the environment, the
// shared files), so the server holds no SMTP password. The role needs ses:SendEmail on the
// identity of the sender and, with a configuration set, on the configuration set.
//
// MessageRejected (SES refused the content for good) fails with kernel.ErrMailRejected; every
// other error, such as TooManyRequestsException, SendingPausedException,
// MailFromDomainNotVerifiedException, AccountSuspendedException, or a request that does not
// reach SES, is a temporary failure that passes once the operator fixes the account or the
// sending rate drops. SES accepts a message to an address on the account's suppression list
// and reports it later as a bounce, so no error is kernel.ErrRecipientRejected. The errors name
// the error code of SES, never its message, which may repeat the recipient's address.
package ses
