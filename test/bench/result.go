// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"
)

// Result is what a run writes to result.json.
type Result struct {
	Commit  string    `json:"commit"`
	Dirty   bool      `json:"dirty"`
	Started time.Time `json:"started"`
	// CPUs is the number of processors of the machine; Cores the server's (-cores), empty for all.
	CPUs    int      `json:"cpus"`
	Cores   string   `json:"cores,omitempty"`
	Options Options  `json:"options"`
	Runs    []DBRun  `json:"runs"`
	Errors  []string `json:"errors,omitempty"`
}

// Options are the flags of the run that shape what it measures.
type Options struct {
	Data           string `json:"data"`
	Clients        []int  `json:"clients"`
	Requests       int    `json:"requests"`
	Warmup         int    `json:"warmup"`
	Writes         int    `json:"writes_per_hundred"`
	ProfileSeconds int    `json:"profile_seconds"`
	TraceRequests  int    `json:"trace_requests"`
}

// DBRun is the measurement on one database.
type DBRun struct {
	Database    string       `json:"database"`
	Data        DataInfo     `json:"data"`
	SeedSeconds float64      `json:"seed_seconds"`
	Levels      []Level      `json:"levels"`
	Trace       *TraceResult `json:"trace,omitempty"`
	Plans       []Plan       `json:"plans,omitempty"`
}

// DataInfo is what the seed made.
type DataInfo struct {
	Accounts   int `json:"accounts"`
	Follows    int `json:"follows"`
	Statuses   int `json:"statuses"`
	Favourites int `json:"favourites"`
}

// Level is the measurement at one number of concurrent clients.
type Level struct {
	Clients    int            `json:"clients"`
	Requests   int            `json:"requests"`
	Seconds    float64        `json:"seconds"`
	Throughput float64        `json:"requests_per_second"`
	Latency    Latency        `json:"latency_ms"`
	Routes     []RouteLatency `json:"routes"`
	Statuses   map[int]int    `json:"statuses"`
	Profiles   []string       `json:"profiles,omitempty"`
}

// Latency holds percentiles of a latency in milliseconds.
type Latency struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

// RouteLatency is the latency of one route of a level.
type RouteLatency struct {
	Route   string  `json:"route"`
	Count   int     `json:"count"`
	Errors  int     `json:"errors"`
	Latency Latency `json:"latency_ms"`
}

// TraceResult is what the traced requests show, per root of a trace.
type TraceResult struct {
	Requests int         `json:"requests"`
	Roots    []RootStats `json:"roots"`
	// Queries are the queries of every traced root by the time they took, most first.
	Queries []QueryStats `json:"queries"`
}

// RootStats is the mean trace of one root: a request's route or a job's type.
type RootStats struct {
	Root         string  `json:"root"`
	Count        int     `json:"count"`
	Statements   float64 `json:"statements"`
	Transactions float64 `json:"transactions"`
	DBMillis     float64 `json:"db_ms"`
	Millis       float64 `json:"ms"`
	// Queries are the statements of a mean one by query name, most first.
	Queries []QueryCount `json:"queries"`
}

// QueryCount is how often one root runs one query.
type QueryCount struct {
	Name       string  `json:"name"`
	PerRequest float64 `json:"per_root"`
}

// QueryStats is the time one query took across the traced roots.
type QueryStats struct {
	Name        string  `json:"name"`
	Calls       int     `json:"calls"`
	TotalMillis float64 `json:"total_ms"`
	MeanMillis  float64 `json:"mean_ms"`
}

// Plan is the plan of one query on the run's database.
type Plan struct {
	Name  string `json:"name"`
	SQL   string `json:"sql,omitempty"`
	Plan  string `json:"plan,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeResult(path string, r *Result) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func readResult(path string) (*Result, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the caller names the result to read
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

// latencyOf returns the percentiles of ds, which it sorts.
func latencyOf(ds []time.Duration) Latency {
	if len(ds) == 0 {
		return Latency{}
	}
	slices.Sort(ds)
	return Latency{P50: quantile(ds, 0.50), P95: quantile(ds, 0.95), P99: quantile(ds, 0.99), Max: ms(ds[len(ds)-1])}
}

// quantile returns the q-quantile of the sorted ds in milliseconds, by the nearest rank.
func quantile(sorted []time.Duration, q float64) float64 {
	i := int(float64(len(sorted))*q+0.5) - 1
	i = max(0, min(i, len(sorted)-1))
	return ms(sorted[i])
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
