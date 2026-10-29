// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// DefaultCloudTasksDispatchDeadline is the default of queue_cloudtasks_dispatch_deadline, the
// default of Cloud Tasks.
const DefaultCloudTasksDispatchDeadline = 10 * time.Minute

// DefaultSQSEventsPath is the default of queue_sqs_events_path, the default of the Lambda Web
// Adapter's AWS_LWA_PASS_THROUGH_PATH, and DefaultSQSEventsBudget that of queue_sqs_events_budget.
const (
	DefaultSQSEventsPath   = "/events"
	DefaultSQSEventsBudget = 10 * time.Minute
)

// wakerQueueRequired are the settings that each queue with a waker requires.
var wakerQueueRequired = map[string][]string{
	"cloudtasks": {"queue_cloudtasks_queue", "queue_cloudtasks_service_account"},
	"sqs":        {"queue_sqs_queue_url"},
}

// wakerQueueChecks are the checks of the queues whose external queue only carries wake-up
// signals, cloudtasks and sqs (ADR-0120 §4, §6): the jobs are in the database, so they need a
// database that the processes share, and the slots of the periodic jobs are claimed in the KV,
// so they need one that the processes share too.
func wakerQueueChecks(vals map[string]string, present map[string]bool) []Issue {
	q := vals["queue"]
	required, ok := wakerQueueRequired[q]
	if !ok {
		return nil
	}
	var issues []Issue
	for _, k := range required {
		if !present[k] {
			issues = append(issues, Issue{Key: k, Missing: true, Message: "required when queue is " + q})
		}
	}
	if db := cmp.Or(vals["db"], "sqlite"); db != "postgres" && db != "libsql" {
		issues = append(issues, Issue{Key: "queue", Message: q + " keeps the jobs in the database and requires db = postgres or libsql, not " + db})
	}
	if kv := cmp.Or(vals["kv"], "db"); kv != "db" && kv != "valkey" {
		issues = append(issues, Issue{Key: "queue", Message: q + " claims the periods of the periodic jobs in the KV and requires kv = db or valkey, not " + kv})
	}
	return issues
}

// checkServiceURL accepts an absolute https URL without user info, path, query, or fragment.
func checkServiceURL(v string) error {
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.TrimSuffix(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must be an absolute https URL without a path, such as https://my-worker-abc123-an.a.run.app")
	}
	return nil
}

// checkDispatchDeadline accepts a dispatch deadline of Cloud Tasks (15s to 30m) that is longer
// than the lease of a job run, which bounds the run of one delivery.
func checkDispatchDeadline(v string) error {
	d, err := time.ParseDuration(v)
	if err != nil || d < 15*time.Second || d > 30*time.Minute {
		return errors.New("must be a Go duration from 15s to 30m, for example 10m")
	}
	if d <= queue.DefaultLease {
		return fmt.Errorf("must be longer than the %s lease of a job run", queue.DefaultLease)
	}
	return nil
}

// checkEventsPath accepts the path of the SQS events: an absolute path other than the root,
// without a query, a fragment, or a pattern of the router.
func checkEventsPath(v string) error {
	if len(v) < 2 || v[0] != '/' || strings.ContainsAny(v, "?#{} \t") || path.Clean(v) != v {
		return errors.New("must be an absolute path other than /, such as /events")
	}
	return nil
}

// checkEventsBudget accepts a budget of an SQS event longer than the lease of a job run and at
// most the 15-minute limit of a Lambda invocation.
func checkEventsBudget(v string) error {
	d, err := time.ParseDuration(v)
	if err != nil || d <= queue.DefaultLease || d > 15*time.Minute {
		return fmt.Errorf("must be a Go duration longer than the %s lease of a job run and at most 15m, for example 10m", queue.DefaultLease)
	}
	return nil
}
