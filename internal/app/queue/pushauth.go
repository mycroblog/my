// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"errors"
)

// ErrPushUnauthenticated reports a request to the endpoint of a PushReceiver or a Trigger whose
// credential does not prove that the configured platform service sent it.
var ErrPushUnauthenticated = errors.New("queue: the push request is not authenticated")

// PushAuthenticator checks the credential that a platform's queue or scheduler (Cloud Tasks,
// Cloud Scheduler) sends with each request to the endpoints of a PushReceiver and a Trigger
// (ADR-0120 §5). A signal only points at a job of the database queue, so a forged one can
// neither create a job nor run one before its RunAt; the check keeps everyone else from making
// the server read the database on their behalf.
type PushAuthenticator interface {
	// Authenticate checks the bearer token of a request to endpoint, the absolute URL that the
	// sender was told to call. It fails with an error wrapping ErrPushUnauthenticated when the
	// token does not prove the configured sender, and with another error when the check could
	// not be made (the issuer's keys out of reach), for the sender to try again.
	Authenticate(ctx context.Context, token, endpoint string) error
}
