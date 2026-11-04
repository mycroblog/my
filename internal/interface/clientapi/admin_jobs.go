// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The queue dashboard (ADR-0004): the state of the job queue, its jobs by state, the records of
// the runs that did not succeed, and running a dead job again. Every operation needs the
// manage_jobs permission.

// QueueDashboard reads the job queue for the queue dashboard, beside Options.DeadJobs.
type QueueDashboard struct {
	// Adapter is the queue the server runs (the queue setting).
	Adapter string
	// Inspector lists the pending jobs; nil for a queue that cannot, which no queue of the server
	// is today (ADR-0004 §7).
	Inspector queue.JobInspector
	// Attempts reads the records of the runs that did not succeed; nil lists none.
	Attempts queue.AttemptLog
	// Clock tells the time the states of the jobs are read at; nil is the system clock.
	Clock kernel.Clock
}

func (d QueueDashboard) now() time.Time {
	if d.Clock == nil {
		return kernel.Normalize(time.Now())
	}
	return d.Clock.Now()
}

// encodeTimeCursor returns the cursor of the page after the item at (t, key): a dead job (its
// failure time and ID), a pending job (its run_at and ID), or an attempt (its finish time and
// number).
func encodeTimeCursor(t time.Time, key string) *Cursor {
	c := base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + " " + key))
	return &c
}

func decodeTimeCursor(c string) (time.Time, string, bool) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", false
	}
	at, key, ok := strings.Cut(string(b), " ")
	if !ok || key == "" {
		return time.Time{}, "", false
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return time.Time{}, "", false
	}
	return t, key, true
}

// GetAdminQueue serves GET /api/v1/admin/queue.
func (s *Server) GetAdminQueue(ctx context.Context, _ GetAdminQueueRequestObject) (GetAdminQueueResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageJobs); err != nil {
		return nil, err
	} else if p != nil {
		return GetAdminQueuedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	body := AdminQueue{Adapter: s.o.Queue.Adapter, PendingListing: s.o.Queue.Inspector != nil, Counts: []AdminJobCount{}, Waits: []AdminJobWait{}}
	if s.o.Queue.Inspector != nil {
		sum, err := s.o.Queue.Inspector.SummarizeJobs(ctx, s.o.Queue.now())
		if err != nil {
			return nil, fmt.Errorf("clientapi: summarize jobs: %w", err)
		}
		for _, c := range sum.Counts {
			body.Counts = append(body.Counts, AdminJobCount{State: AdminJobState(c.State), Type: c.Type, Count: c.Count})
		}
		if !sum.OldestReadyAt.IsZero() {
			at := sum.OldestReadyAt.UTC()
			body.OldestReadyAt = &at
		}
		for _, t := range slices.Sorted(maps.Keys(sum.OldestReadyByType)) {
			body.Waits = append(body.Waits, AdminJobWait{Type: t, OldestReadyAt: sum.OldestReadyByType[t].UTC()})
		}
	}
	cache := cacheNoStore
	return GetAdminQueue200JSONResponse{Body: body, Headers: GetAdminQueue200ResponseHeaders{CacheControl: &cache}}, nil
}

// ListAdminJobs serves GET /api/v1/admin/jobs: the pending jobs, the earliest run_at first, or
// the dead jobs, most recently failed first. The cursor is the time and ID of the last job of the
// previous page.
func (s *Server) ListAdminJobs(ctx context.Context, req ListAdminJobsRequestObject) (ListAdminJobsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageJobs); err != nil {
		return nil, err
	} else if p != nil {
		return ListAdminJobsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	limit, ok := pageLimit(req.Params.Limit)
	if !ok {
		return ListAdminJobsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "limit")), nil
	}
	var f queue.JobFilter
	if req.Params.State != nil {
		if !req.Params.State.Valid() {
			return ListAdminJobsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
		}
		f.State = queue.JobState(*req.Params.State)
	}
	if req.Params.Type != nil {
		f.Type = *req.Params.Type
	}
	var (
		at    time.Time
		after string
	)
	if req.Params.Cursor != nil {
		if at, after, ok = decodeTimeCursor(*req.Params.Cursor); !ok || !kernel.ValidID(after) {
			return ListAdminJobsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
	}
	var page AdminJobPage
	var err error
	if f.State == queue.JobStateDead {
		page, err = s.deadJobPage(ctx, f.Type, queue.DeadJobPosition{FailedAt: at, ID: after}, limit)
	} else {
		page, err = s.pendingJobPage(ctx, f, queue.JobPosition{RunAt: at, ID: after}, limit)
	}
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return ListAdminJobs200JSONResponse{Body: page, Headers: ListAdminJobs200ResponseHeaders{CacheControl: &cache}}, nil
}

func (s *Server) deadJobPage(ctx context.Context, jobType string, after queue.DeadJobPosition, limit int) (AdminJobPage, error) {
	jobs, err := s.o.DeadJobs.ListDeadJobs(ctx, jobType, after, limit+1)
	if err != nil {
		return AdminJobPage{}, fmt.Errorf("clientapi: list dead jobs: %w", err)
	}
	jobs, last := trim(jobs, limit)
	page := AdminJobPage{Items: make([]AdminJob, 0, len(jobs))}
	for _, j := range jobs {
		failedAt := j.FailedAt.UTC()
		page.Items = append(page.Items, AdminJob{
			Id: j.ID, Type: j.Type, State: AdminJobStateDead, Payload: string(j.Payload), Attempts: j.Attempt,
			IdempotencyKey: optionalString(j.IdempotencyKey), OrderingKey: optionalString(j.OrderingKey),
			LastError: optionalString(j.LastError), CreatedAt: j.CreatedAt.UTC(), FailedAt: &failedAt,
		})
	}
	if last >= 0 {
		page.NextCursor = encodeTimeCursor(jobs[last].FailedAt, jobs[last].ID)
	}
	return page, nil
}

func (s *Server) pendingJobPage(ctx context.Context, f queue.JobFilter, after queue.JobPosition, limit int) (AdminJobPage, error) {
	page := AdminJobPage{Items: []AdminJob{}}
	if s.o.Queue.Inspector == nil {
		return page, nil
	}
	jobs, err := s.o.Queue.Inspector.ListJobs(ctx, s.o.Queue.now(), f, after, limit+1)
	if err != nil {
		return AdminJobPage{}, fmt.Errorf("clientapi: list jobs: %w", err)
	}
	jobs, last := trim(jobs, limit)
	for _, j := range jobs {
		runAt := j.RunAt.UTC()
		item := AdminJob{
			Id: j.ID, Type: j.Type, State: AdminJobState(j.State), Payload: string(j.Payload), Attempts: j.Attempt,
			IdempotencyKey: optionalString(j.IdempotencyKey), OrderingKey: optionalString(j.OrderingKey),
			LastError: optionalString(j.LastError), RunAt: &runAt, Worker: optionalString(j.Worker), CreatedAt: j.CreatedAt.UTC(),
		}
		if !j.LockedUntil.IsZero() {
			until := j.LockedUntil.UTC()
			item.LockedUntil = &until
		}
		page.Items = append(page.Items, item)
	}
	if last >= 0 {
		page.NextCursor = encodeTimeCursor(jobs[last].RunAt, jobs[last].ID)
	}
	return page, nil
}

// ListAdminJobAttempts serves GET /api/v1/admin/jobs/{id}/attempts: the records of the runs of
// the job that did not succeed, the most recently finished first. The cursor is the finish time
// and number of the last record of the previous page.
func (s *Server) ListAdminJobAttempts(ctx context.Context, req ListAdminJobAttemptsRequestObject) (ListAdminJobAttemptsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageJobs); err != nil {
		return nil, err
	} else if p != nil {
		return ListAdminJobAttemptsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	limit, ok := pageLimit(req.Params.Limit)
	if !ok {
		return ListAdminJobAttemptsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "limit")), nil
	}
	var after queue.AttemptPosition
	if req.Params.Cursor != nil {
		at, n, ok := decodeTimeCursor(*req.Params.Cursor)
		attempt, err := strconv.Atoi(n)
		if !ok || err != nil || attempt < 1 {
			return ListAdminJobAttemptsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "cursor")), nil
		}
		after = queue.AttemptPosition{FinishedAt: at, Attempt: attempt}
	}
	page := AdminJobAttemptPage{Items: []AdminJobAttempt{}}
	if s.o.Queue.Attempts != nil {
		attempts, err := s.o.Queue.Attempts.ListAttempts(ctx, req.Id, after, limit+1)
		if err != nil {
			return nil, fmt.Errorf("clientapi: list attempts of job %s: %w", req.Id, err)
		}
		attempts, last := trim(attempts, limit)
		for _, a := range attempts {
			page.Items = append(page.Items, AdminJobAttempt{
				Attempt: a.Attempt, Type: a.JobType, Outcome: AdminJobAttemptOutcome(a.Outcome), Error: a.Error, Worker: a.Worker,
				StartedAt: a.StartedAt.UTC(), FinishedAt: a.FinishedAt.UTC(),
			})
		}
		if last >= 0 {
			page.NextCursor = encodeTimeCursor(attempts[last].FinishedAt, strconv.Itoa(attempts[last].Attempt))
		}
	}
	cache := cacheNoStore
	return ListAdminJobAttempts200JSONResponse{Body: page, Headers: ListAdminJobAttempts200ResponseHeaders{CacheControl: &cache}}, nil
}

// RetryAdminJob serves POST /api/v1/admin/jobs/{id}/retry.
func (s *Server) RetryAdminJob(ctx context.Context, req RetryAdminJobRequestObject) (RetryAdminJobResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageJobs); err != nil {
		return nil, err
	} else if p != nil {
		return RetryAdminJobdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err := s.o.DeadJobs.RequeueDeadJob(ctx, req.Id)
	if errors.Is(err, queue.ErrDeadJobNotFound) {
		return RetryAdminJobdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: requeue dead job %s: %w", req.Id, err)
	}
	return RetryAdminJob202Response{}, nil
}
