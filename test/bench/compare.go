// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
)

// printResult prints the levels and the statements per root of r as Markdown tables.
func printResult(w io.Writer, r *Result) {
	for _, run := range r.Runs {
		fmt.Fprintf(w, "\n## %s (%s data: %d people, %d posts)\n\n", run.Database, r.Options.Data, run.Data.Accounts, run.Data.Statuses)
		fmt.Fprintln(w, "| Clients | Requests/s | p50 ms | p95 ms | p99 ms | max ms | not 2xx |")
		fmt.Fprintln(w, "| ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
		for _, lv := range run.Levels {
			fmt.Fprintf(w, "| %d | %.0f | %.1f | %.1f | %.1f | %.1f | %d |\n", lv.Clients, lv.Throughput,
				lv.Latency.P50, lv.Latency.P95, lv.Latency.P99, lv.Latency.Max, lv.failures())
		}
		if run.Trace != nil {
			fmt.Fprintln(w, "\n| Root | Count | Statements | Transactions | DB ms | ms |")
			fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: |")
			for _, rs := range run.Trace.Roots {
				fmt.Fprintf(w, "| %s | %d | %.1f | %.1f | %.2f | %.2f |\n", rs.Root, rs.Count, rs.Statements, rs.Transactions, rs.DBMillis, rs.Millis)
			}
		}
	}
}

// compareCommand prints the results at paths a and b side by side.
func compareCommand(a, b string, w io.Writer) error {
	ra, err := readResult(a)
	if err != nil {
		return err
	}
	rb, err := readResult(b)
	if err != nil {
		return err
	}
	compare(w, ra, rb)
	return nil
}

func compare(w io.Writer, a, b *Result) {
	fmt.Fprintf(w, "a: %s%s, %s data, cores %s\nb: %s%s, %s data, cores %s\n", short(a.Commit), dirtyMark(a.Dirty), a.Options.Data, coresOf(a),
		short(b.Commit), dirtyMark(b.Dirty), b.Options.Data, coresOf(b))
	for _, ra := range a.Runs {
		i := slices.IndexFunc(b.Runs, func(r DBRun) bool { return r.Database == ra.Database })
		if i < 0 {
			continue
		}
		rb := b.Runs[i]
		fmt.Fprintf(w, "\n## %s\n\n", ra.Database)
		fmt.Fprintln(w, "| Clients | Requests/s a → b | p50 ms a → b | p95 ms a → b | p99 ms a → b |")
		fmt.Fprintln(w, "| ---: | ---: | ---: | ---: | ---: |")
		for _, la := range ra.Levels {
			j := slices.IndexFunc(rb.Levels, func(l Level) bool { return l.Clients == la.Clients })
			if j < 0 {
				continue
			}
			lb := rb.Levels[j]
			fmt.Fprintf(w, "| %d | %s | %s | %s | %s |\n", la.Clients, change(la.Throughput, lb.Throughput, "%.0f"),
				change(la.Latency.P50, lb.Latency.P50, "%.1f"), change(la.Latency.P95, lb.Latency.P95, "%.1f"),
				change(la.Latency.P99, lb.Latency.P99, "%.1f"))
		}
		if ra.Trace == nil || rb.Trace == nil {
			continue
		}
		fmt.Fprintln(w, "\n| Root | Statements a → b | DB ms a → b |")
		fmt.Fprintln(w, "| --- | ---: | ---: |")
		roots := map[string][2]*RootStats{}
		for k := range ra.Trace.Roots {
			r := &ra.Trace.Roots[k]
			p := roots[r.Root]
			p[0] = r
			roots[r.Root] = p
		}
		for k := range rb.Trace.Roots {
			r := &rb.Trace.Roots[k]
			p := roots[r.Root]
			p[1] = r
			roots[r.Root] = p
		}
		for _, name := range slices.Sorted(maps.Keys(roots)) {
			p := roots[name]
			if p[0] == nil || p[1] == nil {
				continue
			}
			fmt.Fprintf(w, "| %s | %s | %s |\n", name, change(p[0].Statements, p[1].Statements, "%.1f"),
				change(p[0].DBMillis, p[1].DBMillis, "%.2f"))
		}
	}
}

// change returns "a → b (+x%)".
func change(a, b float64, format string) string {
	s := fmt.Sprintf(format+" → "+format, a, b)
	if a != 0 {
		s += fmt.Sprintf(" (%+.0f%%)", (b-a)/a*100)
	}
	return s
}

func short(commit string) string { return commit[:min(12, len(commit))] }

func dirtyMark(d bool) string {
	if d {
		return " (dirty)"
	}
	return ""
}

func coresOf(r *Result) string {
	if r.Cores == "" {
		return fmt.Sprintf("all %d", r.CPUs)
	}
	return r.Cores
}
