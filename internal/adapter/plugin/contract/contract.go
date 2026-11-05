// SPDX-License-Identifier: AGPL-3.0-only

// Package contract is what `my plugin test` runs against a plugin (ADR-0146 §16): a table of cases per kind. A plugin's
// author runs the binary and links nothing, and pkg/testkit, which imports testing, stays out of the production build.
//
// A kind's cases go in a file of their own and one entry of the table. The plugin may be a real one, so a case with a
// side effect beyond the call runs only when a flag of Env asks for it.
package contract

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	hookv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
)

// Env is what a case runs with.
type Env struct {
	// Conn reaches the plugin, keeping the plugin's error messages for the report.
	Conn *client.Conn
	// Protocol is the protocol that Conn speaks.
	Protocol string
	// Billed runs the cases of the billing kind that may cost money (--billed).
	Billed bool
	// MailTo is the recipient of the cases of the mailer kind that send real mail (--mail-to); empty skips them. MailFrom
	// is their sender (--mail-from).
	MailTo, MailFrom string
	// TranslateFrom and TranslateTo are the languages of the cases of the translator kind (--translate-from, --translate-to),
	// for a plugin that has only some; empty takes en and ja.
	TranslateFrom, TranslateTo string
}

// Case is one contract case.
type Case struct {
	// Name is a word for the report.
	Name string
	// Skip, when set and not empty for this Env, says why the case does not run.
	Skip func(Env) string
	// Note, when set, says what a person still has to check after the case passes.
	Note string
	// Run returns what the plugin did wrong, or nil.
	Run func(ctx context.Context, env Env) error
}

// table holds the cases of each kind, which follow the common ones.
var table = map[string][]Case{
	kind.Probe:         probeCases,
	kind.Billing:       billingCases,
	kind.Mailer:        mailerCases,
	kind.Translator:    translatorCases,
	kind.InboundStatus: hookCases(kind.InboundStatus, hookv1.Verdict_VERDICT_ALLOW, hookv1.Verdict_VERDICT_DROP, hookv1.Verdict_VERDICT_FLAG, hookv1.Verdict_VERDICT_MARK_SENSITIVE),
	kind.InboundFollow: hookCases(kind.InboundFollow, hookv1.Verdict_VERDICT_ALLOW, hookv1.Verdict_VERDICT_DROP, hookv1.Verdict_VERDICT_FLAG),
	kind.SignupCheck:   hookCases(kind.SignupCheck, hookv1.Verdict_VERDICT_ALLOW, hookv1.Verdict_VERDICT_REQUIRE_APPROVAL, hookv1.Verdict_VERDICT_REJECT),
	kind.MediaCheck:    hookCases(kind.MediaCheck, hookv1.Verdict_VERDICT_ALLOW, hookv1.Verdict_VERDICT_MARK_SENSITIVE, hookv1.Verdict_VERDICT_FLAG),
	kind.LocalStatus:   hookCases(kind.LocalStatus, hookv1.Verdict_VERDICT_ALLOW, hookv1.Verdict_VERDICT_FLAG, hookv1.Verdict_VERDICT_REJECT),
}

// noCases is the reason of a kind without an entry in the table.
const noCases = "this version has no contract cases of the kind"

// Outcome is the result of a case.
type Outcome string

// The outcomes of a case.
const (
	Pass Outcome = "PASS"
	Fail Outcome = "FAIL"
	Skip Outcome = "SKIP"
)

// Result is one line of the report.
type Result struct {
	Case    string
	Outcome Outcome
	// Detail says why a case failed or was skipped, or what to check after it passed.
	Detail string
}

// Passed reports whether no result failed.
func Passed(results []Result) bool {
	for _, r := range results {
		if r.Outcome == Fail {
			return false
		}
	}
	return true
}

// Run runs the cases of the kind in order and calls report with each result as it comes. A failure to reach the plugin
// ends the run: every other case would say the same.
func Run(ctx context.Context, k kind.Kind, env Env, report func(Result)) []Result {
	var results []Result
	emit := func(r Result) {
		results = append(results, r)
		report(r)
	}
	if err := reach(ctx, env); err != nil {
		emit(Result{Case: "reachable", Outcome: Fail, Detail: err.Error()})
		return results
	}
	emit(Result{Case: "reachable", Outcome: Pass})

	cases, ok := table[k.Name]
	if !ok {
		emit(Result{Case: k.Name, Outcome: Skip, Detail: noCases})
		return results
	}
	for _, c := range cases {
		if c.Skip != nil {
			if reason := c.Skip(env); reason != "" {
				emit(Result{Case: c.Name, Outcome: Skip, Detail: reason})
				continue
			}
		}
		if err := c.Run(ctx, env); err != nil {
			emit(Result{Case: c.Name, Outcome: Fail, Detail: err.Error()})
			continue
		}
		emit(Result{Case: c.Name, Outcome: Pass, Detail: c.Note})
	}
	return results
}

// reach sends a request to a path no plugin serves, which has no effect on a real one. Any HTTP answer shows that the
// address, the TLS, and the HTTP version work; a refusal of the credentials fails.
func reach(ctx context.Context, env Env) error {
	ctx, cancel := context.WithTimeout(ctx, env.Conn.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(env.Conn.BaseURL, "/")+"/my.plugin.contract/Reachable", http.NoBody)
	if err != nil {
		return err
	}
	if env.Protocol == kind.ProtocolGRPC {
		req.Header.Set("Content-Type", "application/grpc")
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := env.Conn.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("no answer: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	switch res.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("the plugin refused the credentials (HTTP %d)", res.StatusCode)
	}
	return nil
}
