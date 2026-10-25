// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// TransactionalMailPolicy is the retry policy of the jobs that send transactional email, such
// as a verification or a password reset (ADR-0086 §1): 6 runs, 30 seconds of backoff that
// doubles, about 15 minutes in all, since a late link is of little use. The jobs that send
// notification email take the runner's default policy.
var TransactionalMailPolicy = RetryPolicy{MaxAttempts: 6, BaseDelay: 30 * time.Second}

// MailResult returns the Result of a job that sent email and got err (ADR-0086 §1): Done for
// nil, Fail for a permanent rejection (kernel.ErrMailRejected, which kernel.ErrRecipientRejected
// wraps) or a message that is not valid (kernel.ErrInvalidMail), and Retry with the policy's
// backoff for anything else, which is temporary.
func MailResult(err error) Result {
	switch {
	case err == nil:
		return Done()
	case errors.Is(err, kernel.ErrMailRejected), errors.Is(err, kernel.ErrInvalidMail):
		return Fail(err)
	}
	return Retry(0, err)
}
