// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// runJob implements `my job <subcommand>` (FR-801): list the dead jobs of the database queue,
// run them again, or delete them. The admin API and the web client list and retry them too.
func runJob(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("job", args, []string{"list", "retry", "delete"}, stderr, jobUsage)
	if code >= 0 {
		return code
	}
	name := "my job " + sub
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { jobUsage(stderr) }
	c := subcommand{name: name, fs: fs, file: fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")}
	limit := 50
	if sub == "list" {
		fs.IntVar(&limit, "limit", limit, "number of dead jobs to show, most recently failed first")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	switch {
	case sub == "list" && fs.NArg() > 0:
		_, _ = fmt.Fprintf(stderr, "%s: want 0 arguments, got %d\n\n", name, fs.NArg())
		jobUsage(stderr)
		return 2
	case sub == "list" && limit <= 0:
		_, _ = fmt.Fprintf(stderr, "%s: -limit must be positive\n", name)
		return 2
	case sub != "list" && fs.NArg() == 0:
		_, _ = fmt.Fprintf(stderr, "%s: want the IDs of one or more dead jobs (see my job list)\n\n", name)
		jobUsage(stderr)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	if app.Config.Queue == "inline" || app.DeadJobs == nil {
		// The inline queue keeps its dead jobs in the memory of the serving process.
		_, _ = fmt.Fprintf(stderr, "%s: queue %q keeps no dead jobs that this command can reach; only queue = db, cloudtasks, and sqs do\n", name, app.Config.Queue)
		return 1
	}
	if sub == "list" {
		jobs, err := app.DeadJobs.ListDeadJobs(ctx, "", queue.DeadJobPosition{}, limit)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return 1
		}
		printDeadJobs(stdout, jobs)
		return 0
	}
	act, done := app.DeadJobs.RequeueDeadJob, "requeued"
	if sub == "delete" {
		act, done = app.DeadJobs.DeleteDeadJob, "deleted"
	}
	// Each ID is its own operation, so that one unknown ID does not stop the others.
	failed := false
	for _, id := range fs.Args() {
		err := act(ctx, id)
		switch {
		case errors.Is(err, queue.ErrDeadJobNotFound):
			_, _ = fmt.Fprintf(stderr, "%s: no dead job has the ID %q\n", name, id)
			failed = true
		case err != nil:
			_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", name, id, err)
			failed = true
		default:
			_, _ = fmt.Fprintf(stdout, "%s dead job %s\n", done, id)
		}
	}
	if failed {
		return 1
	}
	return 0
}

func printDeadJobs(w io.Writer, jobs []queue.DeadJob) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTYPE\tATTEMPTS\tFAILED\tERROR")
	for _, j := range jobs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", j.ID, j.Type, j.Attempt, j.FailedAt.UTC().Format(time.RFC3339), oneLine(j.LastError))
	}
	_ = tw.Flush()
}

func jobUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my job list [-config <file>] [-limit <n>]
  my job retry [-config <file>] <id>...
  my job delete [-config <file>] <id>...

Dead jobs are the jobs of the database queue (queue = db) that failed for good or
used up their attempts (ADR-0004). list shows them, most recently failed first,
with their type, number of runs, failure time, and last error. retry enqueues
each named dead job again with a fresh attempt count, to run now; when a job
with the same idempotency key is pending, that job stands in for it. delete
removes each named dead job without running it. Dead jobs that nobody retries
are deleted after dead_job_retention (FR-803). The exit code is 1 when any ID
names no dead job; the other IDs are still handled.
`)
}
