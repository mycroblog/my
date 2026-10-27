// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Handler runs one job. It is a plain application function: it must be idempotent (delivery
// is at-least-once) and it reports its outcome as a Result instead of an error, so that the
// same handler works under every runner (pull loop or push receiver).
//
// ctx ends when the job's lease is about to expire or the runner stops; a handler that sees
// ctx end should return Retry.
type Handler func(ctx context.Context, j Job) Result

type outcome int

const (
	outcomeDone outcome = iota + 1
	outcomeRetry
	outcomeFail
)

// Result is a handler's outcome: Done, Retry, or Fail. The zero Result is not valid; a runner
// treats it like Retry(0, nil).
type Result struct {
	outcome outcome
	after   time.Duration
	err     error
	// remote marks a failure that a remote server causes (RetryRemote).
	remote bool
}

// Done reports that the job finished; the runner removes it.
func Done() Result { return Result{outcome: outcomeDone} }

// Retry asks for another attempt after the given delay, or after the retry policy's backoff
// when after <= 0. cause, which may be nil, is recorded as the job's last error. Once the
// policy's attempts are used up, the job becomes a dead job instead.
func Retry(after time.Duration, cause error) Result {
	return Result{outcome: outcomeRetry, after: after, err: cause}
}

// RetryRemote is Retry for a failure that a remote server causes, such as a delivery that the
// inbox refused for now: once the attempts are used up, the dead job is not reported as an
// error to the operator (ADR-0103 §2), since nothing here is broken.
func RetryRemote(after time.Duration, cause error) Result {
	return Result{outcome: outcomeRetry, after: after, err: cause, remote: true}
}

// Fail reports that the job can never succeed; the runner moves it to the dead jobs at once.
func Fail(cause error) Result { return Result{outcome: outcomeFail, err: cause} }

// IsDone reports whether r is Done.
func (r Result) IsDone() bool { return r.outcome == outcomeDone }

// IsFail reports whether r is Fail.
func (r Result) IsFail() bool { return r.outcome == outcomeFail }

// IsRetry reports whether r asks for another attempt (Retry, or the invalid zero Result).
func (r Result) IsRetry() bool { return !r.IsDone() && !r.IsFail() }

// After returns the delay a Retry asked for; zero or less means the policy's backoff.
func (r Result) After() time.Duration { return r.after }

// Err returns the cause given to Retry or Fail.
func (r Result) Err() error { return r.err }

// IsRemote reports whether r is a RetryRemote.
func (r Result) IsRemote() bool { return r.remote }

// JobHandlerRegistry maps job types to handlers and their retry policies. The zero value is
// empty and ready to use. It is safe for concurrent use; register every handler at startup,
// before the runners start.
type JobHandlerRegistry struct {
	mu     sync.RWMutex
	byType map[string]registration
}

type registration struct {
	handler Handler
	policy  RetryPolicy
}

// Register sets the handler of jobType, using the runner's default retry policy. It panics
// when jobType is empty or already registered.
func (r *JobHandlerRegistry) Register(jobType string, h Handler) {
	r.RegisterWithPolicy(jobType, h, RetryPolicy{})
}

// RegisterWithPolicy sets the handler of jobType with its own retry policy; zero fields of
// policy take the runner's defaults. It panics when jobType is empty or already registered.
func (r *JobHandlerRegistry) RegisterWithPolicy(jobType string, h Handler, policy RetryPolicy) {
	if jobType == "" || h == nil {
		panic("queue: Register needs a job type and a handler")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byType[jobType]; dup {
		panic(fmt.Sprintf("queue: handler for %q registered twice", jobType))
	}
	if r.byType == nil {
		r.byType = make(map[string]registration)
	}
	r.byType[jobType] = registration{handler: h, policy: policy}
}

// Lookup returns the handler and retry policy registered for jobType.
func (r *JobHandlerRegistry) Lookup(jobType string) (Handler, RetryPolicy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byType[jobType]
	return reg.handler, reg.policy, ok
}
