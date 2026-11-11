// SPDX-License-Identifier: AGPL-3.0-only

package ops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/mycroblog/my/tools/agentkit/internal/gh"
)

// JobsOptions select what Jobs prints.
type JobsOptions struct {
	Run   int64         // the run to read; 0 reads the newest run of each workflow on the pull request's head
	Limit int           // jobs printed per run; 0 is 20
	Slow  time.Duration // a job longer than this lists its slowest steps; 0 is 3 minutes
	Now   time.Time     // the end of a job that still runs; zero is the present
}

// slowSteps is how many steps a slow job lists.
const slowSteps = 3

type span struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

// took is how long the job or step ran until now, and whether it started at all.
func (s span) took(now time.Time) (time.Duration, bool) {
	start, err := time.Parse(time.RFC3339, s.StartedAt)
	if err != nil {
		return 0, false
	}
	end, err := time.Parse(time.RFC3339, s.CompletedAt)
	if err != nil || s.Status != "completed" {
		end = now
	}
	if end.Before(start) {
		return 0, true
	}
	return end.Sub(start).Round(time.Second), true
}

// result is queued, running, skipped, failed, cancelled, or passed.
func (s span) result() string {
	if s.Status != "completed" {
		if s.StartedAt == "" || s.Status == "queued" || s.Status == "waiting" || s.Status == "pending" {
			return "queued"
		}
		return "running"
	}
	switch s.Conclusion {
	case "skipped":
		return "skipped"
	case "failure", "timed_out", "startup_failure", "action_required":
		return "failed"
	case "cancelled":
		return "cancelled"
	}
	return "passed"
}

type runJob struct {
	span
	Steps []span `json:"steps"`
}

type workflowRun struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	RunStartedAt string `json:"run_started_at"`
	UpdatedAt    string `json:"updated_at"`
}

// Jobs prints where the time of a pull request's CI goes: for the newest run of each workflow on its head (or
// for o.Run), one line with the run's wall time and its number of skipped jobs, then the jobs that ran, slowest
// first, with the slowest steps of each slow one. It reads once and does not wait for a run that still runs.
func Jobs(w io.Writer, n int, o JobsOptions) error {
	if o.Limit <= 0 {
		o.Limit = 20
	}
	if o.Slow <= 0 {
		o.Slow = 3 * time.Minute
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	runs := []int64{o.Run}
	if o.Run == 0 {
		p, err := viewPR(n)
		if err != nil {
			return err
		}
		runs = runs[:0]
		seen := map[int64]bool{}
		for _, c := range latest(p.StatusCheckRollup) {
			if id := c.runID(); id != 0 && !seen[id] {
				seen[id] = true
				runs = append(runs, id)
			}
		}
		sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
		if len(runs) == 0 {
			return fmt.Errorf("PR #%d: no workflow run on its head %s yet", n, shortSHA(p.HeadRefOid))
		}
	}
	for _, id := range runs {
		if err := runJobs(w, id, o); err != nil {
			return err
		}
	}
	return nil
}

func runJobs(w io.Writer, id int64, o JobsOptions) error {
	path := "repos/" + gh.Repo() + "/actions/runs/" + strconv.FormatInt(id, 10)
	var run workflowRun
	if err := gh.JSON(&run, "api", path); err != nil {
		return err
	}
	// One JSON object per job, over every page.
	out, err := gh.Run("api", "--paginate", path+"/jobs?per_page=100", "--jq", ".jobs[]")
	if err != nil {
		return err
	}
	var jobs []runJob
	skipped := 0
	for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
		var j runJob
		if err := dec.Decode(&j); err != nil {
			return fmt.Errorf("jobs of run %d: %w", id, err)
		}
		if j.result() == "skipped" {
			skipped++
			continue
		}
		jobs = append(jobs, j)
	}
	sort.SliceStable(jobs, func(a, b int) bool {
		da, _ := jobs[a].took(o.Now)
		db, _ := jobs[b].took(o.Now)
		return da > db
	})

	wall, _ := span{Status: run.Status, StartedAt: run.RunStartedAt, CompletedAt: run.UpdatedAt}.took(o.Now)
	state := span{Status: run.Status, Conclusion: run.Conclusion}.result()
	if run.Status != "completed" {
		state = "running"
	}
	fmt.Fprintf(w, "%s run %d (%s): %s wall, %d jobs ran, %d skipped\n", run.Name, id, state, wall, len(jobs), skipped)
	for i, j := range jobs {
		if i == o.Limit {
			fmt.Fprintf(w, "  … %d faster jobs (-n %d)\n", len(jobs)-i, len(jobs))
			break
		}
		d, _ := j.took(o.Now)
		fmt.Fprintf(w, "  %7s  %-9s  %s\n", d, j.result(), j.Name)
		if d <= o.Slow {
			continue
		}
		steps := append([]span(nil), j.Steps...)
		sort.SliceStable(steps, func(a, b int) bool {
			da, _ := steps[a].took(o.Now)
			db, _ := steps[b].took(o.Now)
			return da > db
		})
		for k, s := range steps {
			sd, started := s.took(o.Now)
			if k == slowSteps || !started {
				break
			}
			fmt.Fprintf(w, "           %7s  %s\n", sd, s.Name)
		}
	}
	return nil
}
