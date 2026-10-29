// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// DefaultCloudTasksDispatchDeadline is the default of queue_cloudtasks_dispatch_deadline, the
// default of Cloud Tasks.
const DefaultCloudTasksDispatchDeadline = 10 * time.Minute

// cloudTasksChecks are the checks of queue = cloudtasks (ADR-0120 §4, §6): the jobs are in the
// database, so it needs a database that the processes share, and the slots of the periodic jobs
// are claimed in the KV, so it needs one that they share too.
func cloudTasksChecks(vals map[string]string, present map[string]bool) []Issue {
	if vals["queue"] != "cloudtasks" {
		return nil
	}
	var issues []Issue
	for _, k := range []string{"queue_cloudtasks_queue", "queue_cloudtasks_service_account"} {
		if !present[k] {
			issues = append(issues, Issue{Key: k, Missing: true, Message: "required when queue is cloudtasks"})
		}
	}
	if db := cmp.Or(vals["db"], "sqlite"); db != "postgres" && db != "libsql" {
		issues = append(issues, Issue{Key: "queue", Message: "cloudtasks keeps the jobs in the database and requires db = postgres or libsql, not " + db})
	}
	if kv := cmp.Or(vals["kv"], "db"); kv != "db" && kv != "valkey" {
		issues = append(issues, Issue{Key: "queue", Message: "cloudtasks claims the periods of the periodic jobs in the KV and requires kv = db or valkey, not " + kv})
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
