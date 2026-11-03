// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// claudeModel is the one place that names the model. Its full name must be one that the Claude Code pinned in
// .github/workflows/release.yml knows.
const claudeModel = "claude-opus-5-5"

// maxBudgetUSD is the most a call may cost in Claude Code's own count, well above what the bounds of the input allow.
const maxBudgetUSD = "5"

// claudeCLI asks Claude Code, which authenticates by Workload Identity Federation from the environment, as in the L4
// task tests. The model gets no tool: nothing it reads can make it do anything but answer.
type claudeCLI struct {
	path    string
	timeout time.Duration
}

// cliResult is the part of Claude Code's `--output-format json` object that matters here.
type cliResult struct {
	IsError    bool   `json:"is_error"`
	Subtype    string `json:"subtype"`
	StopReason string `json:"stop_reason"`
	Result     string `json:"result"`
}

func (c claudeCLI) complete(ctx context.Context, systemPrompt, user string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	// An empty directory, so that no CLAUDE.md, settings, or hooks of the checkout load.
	dir, err := os.MkdirTemp("", "releaseprose")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	cmd := exec.CommandContext(ctx, c.path, "-p",
		"--model", claudeModel,
		"--effort", "high",
		"--max-turns", "1",
		"--max-budget-usd", maxBudgetUSD,
		"--output-format", "json",
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "",
		"--disable-slash-commands",
		"--no-session-persistence",
		"--system-prompt", systemPrompt,
	)
	cmd.Dir = dir
	cmd.Env = claudeEnv(os.Environ())
	cmd.Stdin = strings.NewReader(user)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	var res cliResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		if runErr != nil {
			return "", fmt.Errorf("%w: %s", runErr, tail(stderr.String()))
		}
		return "", fmt.Errorf("the output is not JSON: %w", err)
	}
	switch {
	case res.IsError:
		// A failure can say success in subtype, as "Not logged in" does.
		return "", fmt.Errorf("claude reported an error (%s): %s", res.Subtype, tail(res.Result))
	case res.Subtype != "success":
		return "", errors.New("claude stopped: " + res.Subtype)
	case res.StopReason == "max_tokens" || res.StopReason == "refusal":
		return "", errors.New("the answer stopped early: " + res.StopReason)
	case runErr != nil:
		return "", fmt.Errorf("%w: %s", runErr, tail(stderr.String()))
	}
	return res.Result, nil
}

// claudeEnv keeps what Claude Code needs to run and to authenticate, and no other token of the job.
func claudeEnv(environ []string) []string {
	var env []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if name == "PATH" || name == "HOME" || name == "TMPDIR" || strings.HasPrefix(name, "ANTHROPIC_") {
			env = append(env, kv)
		}
	}
	return env
}

func tail(text string) string {
	const limit = 500
	if len(text) > limit {
		return text[len(text)-limit:]
	}
	return text
}
