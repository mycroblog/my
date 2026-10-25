// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

func TestMailResult(t *testing.T) {
	for _, c := range []struct {
		err               error
		done, fail, retry bool
	}{
		{nil, true, false, false},
		{fmt.Errorf("send: %w", kernel.ErrRecipientRejected), false, true, false},
		{fmt.Errorf("send: %w", kernel.ErrMailRejected), false, true, false},
		{fmt.Errorf("send: %w", kernel.ErrInvalidMail), false, true, false},
		{errors.New("smtp: RCPT TO: reply 451 4.3.0"), false, false, true},
	} {
		r := queue.MailResult(c.err)
		if r.IsDone() != c.done || r.IsFail() != c.fail || r.IsRetry() != c.retry || !errors.Is(r.Err(), c.err) {
			t.Errorf("MailResult(%v) = %+v", c.err, r)
		}
	}
	if !errors.Is(kernel.ErrRecipientRejected, kernel.ErrMailRejected) {
		t.Error("ErrRecipientRejected does not wrap ErrMailRejected")
	}
}
