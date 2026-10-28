// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	domfederation "github.com/mycroblog/my/internal/domain/federation"
	domfinance "github.com/mycroblog/my/internal/domain/finance"
	domidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	domwebhook "github.com/mycroblog/my/internal/domain/webhook"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestProjectionsVerifyAndRebuild fills every projection through the services, on both
// dialects, and checks that each one verifies, that verify finds the rows that raw SQL breaks
// and changes nothing, and that rebuild restores them and keeps the fetch state of the remote
// actors' heads, which no event records, and the restriction rows whose end has passed. The
// scheduled job projection.verify corrects them.
func TestProjectionsVerifyAndRebuild(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()
			dbURL := newDatabase(t, true)
			if dialect == "postgres" {
				dbURL = newPostgresDatabase(t)
			}
			cfg := loadConfig(t, map[string]string{
				"MY_DB": dialect, "MY_DATABASE_URL": dbURL, "MY_WEB_ORIGIN": webOrigin, "MY_MAIL": "none", "MY_BILLING_SOURCES": "aws",
			})
			app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{
				Logger: slog.New(slog.DiscardHandler), BillingSources: map[string]appfinance.BillingSource{"aws": billingFixture()},
				// The actions that fillModeration issues end, and the expiry job judges them, by
				// this clock, after waits on the monotonic clock.
				Clock: newSteadyClock(),
			})
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			fillProjections(t, app)
			exec, count := rawSQL(t, dialect, dbURL)
			checkProjections(t, app, exec, count)
		})
	}
}

func fillProjections(t *testing.T, a *platform.App) {
	t.Helper()
	ctx := t.Context()
	ids := map[string]string{}
	for _, name := range []string{"alice", "bob", "carol"} {
		reg, err := a.Identity.RegisterLocalUser(ctx, identity.RegisterInput{
			Username: name, Password: "correct horse battery staple", DisplayName: name, Locale: "en",
		})
		if err != nil {
			t.Fatalf("RegisterLocalUser(%s): %v", name, err)
		}
		ids[name] = reg.AccountID
	}
	alice, bob, carol := ids["alice"], ids["bob"], ids["carol"]
	var dave string
	err := a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		d, err := a.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: "https://remote.example/users/dave", Domain: "remote.example", Type: "Person",
			PreferredUsername: "dave", InboxURL: "https://remote.example/users/dave/inbox",
			SharedInboxURL: "https://remote.example/inbox",
		})
		dave = d.ID
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	if _, err := a.Graph.Follow(ctx, bob, alice); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	post := func(author, text string) string {
		p, err := a.Content.Create(ctx, appcontent.CreateInput{
			AuthorAccountID: author, TextInput: appcontent.TextInput{Text: text}, Visibility: domcontent.VisibilityPublic,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return p.Status.ID
	}
	first := post(alice, "hi @bob #projections")
	gone := post(alice, "soon gone")
	hello := post(bob, "hello")
	for _, id := range []string{first, gone} {
		if _, err := a.Interactions.Favourite(ctx, bob, id); err != nil {
			t.Fatalf("Favourite: %v", err)
		}
	}
	if _, err := a.Interactions.Boost(ctx, bob, first); err != nil {
		t.Fatalf("Boost: %v", err)
	}
	if _, err := a.Content.Delete(ctx, alice, gone); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := a.Graph.Block(ctx, carol, bob); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if _, err := a.Graph.Mute(ctx, alice, carol, appgraph.MuteInput{HideNotifications: true}); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	fillRemotePoll(t, a, dave)
	fillModeration(t, a, ids, dave, hello)
	fillWebhooks(t, a)
	fillFinance(t, a)
	fillIdentity(t, a, bob)
}

// fillRemotePoll fills poll_tallies with a remote poll of dave and two observations of it.
func fillRemotePoll(t *testing.T, a *platform.App, dave string) {
	t.Helper()
	ctx := t.Context()
	at := time.Now().UTC().Add(-time.Hour)
	in := appcontent.RemoteStatusInput{
		URI: "https://remote.example/users/dave/statuses/1", AuthorAccountID: dave, AuthorDomain: "remote.example",
		Visibility: domcontent.VisibilityPublic, PublishedAt: at, RevisedAt: at,
		Bodies: []appcontent.RemoteBody{{Language: "und", HTML: "<p>cats or dogs?</p>"}},
		Poll:   &appcontent.RemotePollInput{Options: []string{"cats", "dogs"}, Votes: []int64{1, 2}},
	}
	err := a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := a.Content.IngestRemoteStatus(ctx, tx, in)
		return err
	})
	if err != nil {
		t.Fatalf("IngestRemoteStatus: %v", err)
	}
	in.Poll.Votes = []int64{3, 2}
	err = a.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		_, err := a.Content.ReviseRemoteStatus(ctx, tx, in)
		return err
	})
	if err != nil {
		t.Fatalf("ReviseRemoteStatus: %v", err)
	}
}

// fillWebhooks fills the webhook projections: an endpoint revised, with its secret rotated, and
// a failed attempt, as the guard refuses its loopback destination.
func fillWebhooks(t *testing.T, a *platform.App) {
	t.Helper()
	ctx := t.Context()
	settings := domwebhook.EndpointSettings{URL: "https://127.0.0.1/hook", Events: []domwebhook.EventType{domwebhook.EventStatusCreated}}
	c, err := a.Webhooks.Create(ctx, settings, "")
	if err != nil {
		t.Fatalf("Create the webhook = %v", err)
	}
	id := c.Endpoint.ID
	settings.Description = "Chat"
	if _, err := a.Webhooks.Update(ctx, id, settings, ""); err != nil {
		t.Fatalf("Update the webhook = %v", err)
	}
	if _, err := a.Webhooks.RotateSecret(ctx, id, ""); err != nil {
		t.Fatalf("RotateSecret = %v", err)
	}
	d, err := a.Webhooks.Test(ctx, id)
	if err != nil {
		t.Fatalf("Test the webhook = %v", err)
	}
	if res := a.Webhooks.Deliver(ctx, d.Delivery.ID, false); res.IsDone() {
		t.Fatalf("Deliver to loopback = %+v, want a failure", res)
	}
}

// billingFixture is a source with two costs this month.
func billingFixture() *testkit.FakeBillingSource {
	src := testkit.NewFakeBillingSource("aws", time.UTC)
	src.SetMonth(domfinance.YearMonthOf(domfinance.DateOf(time.Now().UTC())), testkit.FakeBillingMonth{Rows: []testkit.FakeBillingRow{
		{Service: "Amazon Simple Storage Service", ChargeType: domfinance.ChargeUsage, Amount: "1.25", Currency: "USD"},
		{Service: "Tax", ChargeType: domfinance.ChargeTax, Amount: "0.10", Currency: "USD"},
	}})
	return src
}

// fillFinance fills cost_item_heads: an item relabeled and ended, and one canceled; and
// imported_cost_heads with an import of billingFixture.
func fillFinance(t *testing.T, a *platform.App) {
	t.Helper()
	ctx := t.Context()
	if r := a.Finance.Import(ctx, "aws", "fill"); !r.IsDone() {
		t.Fatalf("Import = %v", r.Err())
	}
	start, err := domfinance.ParseDate("2026-04-10")
	if err != nil {
		t.Fatal(err)
	}
	in := appfinance.CreateInput{
		Recurrence: domfinance.Recurrence{Schedule: domfinance.ScheduleYears, Interval: 1}, StartsOn: start,
		Label: domfinance.Label{Name: "example.org", Provider: "Registrar", Category: domfinance.CategoryDomain},
		Price: domfinance.Money{Minor: 1650, Currency: "JPY"},
	}
	kept, err := a.Finance.Create(ctx, in, "")
	if err != nil {
		t.Fatalf("Create the cost item = %v", err)
	}
	in.Label.Name = "example.net"
	if _, err := a.Finance.Relabel(ctx, kept.ID, in.Label, ""); err != nil {
		t.Fatalf("Relabel = %v", err)
	}
	end := start.AddDays(1000)
	if _, err := a.Finance.SetEnd(ctx, kept.ID, &end, ""); err != nil {
		t.Fatalf("SetEnd = %v", err)
	}
	canceled, err := a.Finance.Create(ctx, in, "")
	if err != nil {
		t.Fatalf("Create the cost item = %v", err)
	}
	if err := a.Finance.Cancel(ctx, canceled.ID, ""); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
}

// fillModeration fills the moderation projections, status_sensitivity_overrides, and the feed
// restrictions: actions on accounts and domains (one lifted, one that has expired), a report
// assigned, resolved, and reopened, and an appeal pending and one decided.
func fillModeration(t *testing.T, a *platform.App, ids map[string]string, dave, hello string) {
	t.Helper()
	ctx := t.Context()
	alice, bob, carol := ids["alice"], ids["bob"], ids["carol"]
	issue := func(in appmoderation.AccountActionInput) moderation.Action {
		t.Helper()
		in.ReasonCategory = moderation.CategoryOther
		act, err := a.Moderation.IssueAccountAction(ctx, in)
		if err != nil {
			t.Fatalf("IssueAccountAction(%s) = %v", in.Kind, err)
		}
		return act
	}
	limit := issue(appmoderation.AccountActionInput{TargetAccountID: carol, Kind: moderation.ActionLimit, Duration: time.Hour})
	issue(appmoderation.AccountActionInput{TargetAccountID: dave, Kind: moderation.ActionForceSensitive, Duration: time.Millisecond})
	issue(appmoderation.AccountActionInput{TargetAccountID: bob, Kind: moderation.ActionMarkStatusesSensitive, StatusIDs: []string{hello}})
	lifted := issue(appmoderation.AccountActionInput{TargetAccountID: bob, Kind: moderation.ActionRestrictPosting})
	if err := a.Moderation.LiftAccountAction(ctx, lifted.ID, "", ""); err != nil {
		t.Fatalf("LiftAccountAction = %v", err)
	}
	warn := issue(appmoderation.AccountActionInput{TargetAccountID: alice, Kind: moderation.ActionWarn})
	for _, in := range []appmoderation.DomainActionInput{
		{Domain: "spam.example", Kind: moderation.DomainSuspend},
		{Domain: "remote.example", Kind: moderation.DomainLimit, Duration: time.Millisecond},
	} {
		if _, err := a.Moderation.IssueDomainAction(ctx, in); err != nil {
			t.Fatalf("IssueDomainAction(%s) = %v", in.Kind, err)
		}
	}
	// The force_sensitive action on dave and the limit of remote.example expire, and the expiry
	// job removes the account's row of the first; the second leaves a row of the timelines whose
	// end has passed, as does the limit on dave below, whose rows also stay in
	// account_restrictions, since the expiry job does not run again.
	// The wait starts after the actions are issued, so that a slow machine cannot make it too short.
	waitPast(time.Now().Add(5 * time.Millisecond))
	if _, err := a.Moderation.ExpireAccountRestrictions(ctx); err != nil {
		t.Fatalf("ExpireAccountRestrictions = %v", err)
	}
	issue(appmoderation.AccountActionInput{TargetAccountID: dave, Kind: moderation.ActionLimit, Duration: 20 * time.Millisecond})
	limitEnds := time.Now().Add(25 * time.Millisecond)

	r, _, err := a.Moderation.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/1", ReporterAccountID: dave, TargetAccountID: alice, Comment: "spam",
	})
	if err != nil {
		t.Fatalf("FileRemoteReport = %v", err)
	}
	if _, err := a.Moderation.AssignReport(ctx, r.ID, testkit.NamedID("someone"), ""); err != nil {
		t.Fatalf("AssignReport = %v", err)
	}
	if _, err := a.Moderation.ResolveReport(ctx, r.ID, moderation.OutcomeDismissed, ""); err != nil {
		t.Fatalf("ResolveReport = %v", err)
	}
	if _, err := a.Moderation.ReopenReport(ctx, r.ID, ""); err != nil {
		t.Fatalf("ReopenReport = %v", err)
	}
	resolved, _, err := a.Moderation.FileRemoteReport(ctx, appmoderation.RemoteReportInput{
		URI: "https://remote.example/flags/2", ReporterAccountID: dave, TargetAccountID: alice, Comment: "more spam",
	})
	if err != nil {
		t.Fatalf("FileRemoteReport = %v", err)
	}
	if _, err := a.Moderation.ResolveReport(ctx, resolved.ID, moderation.OutcomeActioned, ""); err != nil {
		t.Fatalf("ResolveReport = %v", err)
	}

	if _, err := a.Moderation.FileAppeal(ctx, carol, limit.ID, "I did nothing wrong."); err != nil {
		t.Fatalf("FileAppeal = %v", err)
	}
	appeal, err := a.Moderation.FileAppeal(ctx, alice, warn.ID, "Please reconsider.")
	if err != nil {
		t.Fatalf("FileAppeal = %v", err)
	}
	if appeal.Appeal == nil {
		t.Fatalf("FileAppeal = %+v, want the appeal", appeal)
	}
	if _, err := a.Moderation.DecideAppeal(ctx, appeal.Appeal.ID, moderation.AppealDecisionUpheld, "", ""); err != nil {
		t.Fatalf("DecideAppeal = %v", err)
	}
	// The limit on dave ends.
	waitPast(limitEnds)
}

// waitPast blocks until the monotonic clock is past t, a reading of time.Now, however long the
// machine took to get here. The App must tell the time with a steadyClock, which follows the
// monotonic clock, for its clock to be past t too.
func waitPast(t time.Time) {
	for d := time.Until(t); d > 0; d = time.Until(t) {
		time.Sleep(d)
	}
}

// fillIdentity fills the identity projections: a role grant, a revoked one, and sign-ups
// pending and approved.
func fillIdentity(t *testing.T, a *platform.App, bob string) {
	t.Helper()
	ctx := t.Context()
	if _, err := a.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "root", Email: "root@example.com", Password: "correct horse battery staple",
	}); err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	b, err := a.Accounts.FindByID(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	in := identity.RoleChangeInput{UserID: b.Local.UserID, RoleKey: domidentity.RoleModerator}
	if _, err := a.Identity.GrantRole(ctx, in); err != nil {
		t.Fatalf("GrantRole = %v", err)
	}
	if _, err := a.Identity.RevokeRole(ctx, in); err != nil {
		t.Fatalf("RevokeRole = %v", err)
	}
	if _, err := a.Identity.GrantRole(ctx, in); err != nil {
		t.Fatalf("GrantRole = %v", err)
	}
	if _, err := a.Instance.SetSetting(ctx, appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationApproval),
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"erin", "frank"} {
		s, err := a.Identity.SignUp(ctx, identity.SignUpInput{
			Username: name, Email: name + "@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("SignUp(%s) = %v", name, err)
		}
		if name != "frank" {
			continue
		}
		if _, err := a.Identity.DecideRegistration(ctx, identity.DecideInput{UserID: s.User.ID, Decision: domidentity.DecisionApproved}); err != nil {
			t.Fatalf("DecideRegistration = %v", err)
		}
	}
	// A relay joined from the CLI (ADR-0061); its Follow waits in the queue.
	if _, err := a.Relays.Add(ctx, appfederation.RelayInput{Protocol: domfederation.RelayMastodon, URL: "https://relay.example/inbox"}, ""); err != nil {
		t.Fatalf("Relays.Add = %v", err)
	}
}

// rawSQL returns functions that run SQL on the database directly, to break projections and to
// read a number.
func rawSQL(t *testing.T, dialect, dbURL string) (exec func(q string), count func(q string) int64) {
	t.Helper()
	if dialect == "postgres" {
		conn, err := pgx.Connect(t.Context(), dbURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		return func(q string) {
				if _, err := conn.Exec(t.Context(), q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}, func(q string) int64 {
				var n int64
				if err := conn.QueryRow(t.Context(), q).Scan(&n); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
				return n
			}
	}
	db, err := sql.Open("sqlite", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return func(q string) {
			if _, err := db.ExecContext(t.Context(), q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}, func(q string) int64 {
			var n int64
			if err := db.QueryRowContext(t.Context(), q).Scan(&n); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			return n
		}
}

func checkProjections(t *testing.T, a *platform.App, exec func(string), count func(string) int64) {
	t.Helper()
	ctx := t.Context()
	svc := a.Projections
	verifyAll := func() map[string]projection.Report {
		out := map[string]projection.Report{}
		for _, p := range svc.List() {
			r, err := svc.Verify(ctx, p)
			if err != nil {
				t.Fatalf("Verify(%s) = %v", p.Name, err)
			}
			out[p.Name] = r
		}
		return out
	}
	// Every projection verifies, and the commands filled the tables below.
	held := map[string]int{}
	for name, r := range verifyAll() {
		if r.Drifted() {
			t.Errorf("Verify(%s) after the commands = %+v, want no drift", name, r.Tables)
		}
		for _, d := range r.Tables {
			held[d.Table] = d.Rows
		}
	}
	for _, table := range []string{
		"account_handles", "account_profile_heads", "remote_account_heads", "follow_edges", "follow_counters",
		"block_edges", "status_heads", "status_mention_index", "status_counters", "account_post_counters", "local_post_counters",
		"boost_edges", "feed_entries", "feed_tag_entries", "feed_follow_edges", "feed_exclusions",
		"notification_exclusions", "notification_deleted_statuses", "poll_tallies",
		"account_restrictions", "domain_restrictions", "report_states", "appeal_states", "status_sensitivity_overrides",
		"feed_author_restrictions", "feed_domain_restrictions", "user_roles", "user_registration_states", "user_primary_emails",
		"webhook_endpoint_heads", "webhook_endpoint_failures", "relay_states", "cost_item_heads", "imported_cost_heads",
	} {
		if held[table] == 0 {
			t.Errorf("%s holds no rows", table)
		}
	}

	exec("UPDATE status_counters SET favourites_count = favourites_count + 5")
	exec("DELETE FROM feed_tag_entries")
	exec("DELETE FROM notification_exclusions WHERE reason = 'mute'")
	exec("INSERT INTO notification_deleted_statuses (status_id) VALUES ('" + testkit.NamedID("no-such-status") + "')")
	exec("UPDATE remote_account_heads SET inbox_url = 'https://elsewhere.example/inbox', consecutive_fetch_failures = 3")
	exec("DELETE FROM follow_counters")
	exec("UPDATE local_post_counters SET posts_count = posts_count + 1")
	exec("UPDATE poll_tallies SET votes = votes + 1")
	exec("DELETE FROM account_restrictions WHERE restriction = 'public_discovery'")
	exec("INSERT INTO domain_restrictions (domain, restriction, source_action_id) VALUES ('nowhere.example', 'federation', '" + testkit.NamedID("no-such-action") + "')")
	exec("UPDATE report_states SET assignee_user_id = NULL")
	exec("DELETE FROM appeal_states WHERE state = 'pending'")
	exec("DELETE FROM status_sensitivity_overrides")
	exec("DELETE FROM feed_author_restrictions WHERE effective_until IS NOT NULL")
	exec("DELETE FROM feed_domain_restrictions WHERE domain = 'spam.example'")
	exec("DELETE FROM user_roles")
	exec("UPDATE user_registration_states SET state = 'rejected' WHERE state = 'pending'")
	exec("DELETE FROM user_primary_emails")
	exec("DELETE FROM webhook_endpoint_heads")
	exec("UPDATE webhook_endpoint_failures SET consecutive_failures = consecutive_failures + 3")
	exec("UPDATE relay_states SET state = 'left'")
	exec("UPDATE cost_item_heads SET ends_on = NULL")
	exec("DELETE FROM imported_cost_heads WHERE service = 'Tax'")

	drifted := []string{}
	for _, p := range svc.List() {
		r, err := svc.Verify(ctx, p)
		if err != nil {
			t.Fatalf("Verify(%s) = %v", p.Name, err)
		}
		for _, d := range r.Tables {
			if !d.Empty() {
				drifted = append(drifted, d.Table)
			}
		}
	}
	slices.Sort(drifted)
	want := []string{
		"account_restrictions", "appeal_states", "cost_item_heads", "domain_restrictions", "feed_author_restrictions", "feed_domain_restrictions",
		"feed_tag_entries", "follow_counters", "imported_cost_heads", "local_post_counters", "notification_deleted_statuses", "notification_exclusions",
		"poll_tallies", "relay_states", "remote_account_heads", "report_states", "status_counters", "status_sensitivity_overrides",
		"user_primary_emails", "user_registration_states", "user_roles", "webhook_endpoint_failures", "webhook_endpoint_heads",
	}
	if !slices.Equal(drifted, want) {
		t.Fatalf("tables that verify reports = %v, want %v", drifted, want)
	}
	// Verify changed nothing.
	again, err := svc.Lookup("feed_tag_entries")
	if err != nil {
		t.Fatal(err)
	}
	if r, err := svc.Verify(ctx, again); err != nil || !r.Drifted() {
		t.Errorf("Verify(feeds) a second time = %v, %v; want drift", r.Drifted(), err)
	}

	// The scheduled job corrects every projection.
	if !slices.ContainsFunc(a.Scheduler.Schedules(), func(s queue.Schedule) bool { return s.Name == projection.JobVerify }) {
		t.Errorf("no %s schedule", projection.JobVerify)
	}
	if err := a.Jobs.Enqueue(ctx, queue.NewJob{Type: projection.JobVerify}); err != nil {
		t.Fatalf("Enqueue = %v", err)
	}
	if n, err := a.Runner.Drain(ctx); err != nil || n == 0 {
		t.Fatalf("Drain = %d, %v; want the job run", n, err)
	}
	for name, r := range verifyAll() {
		if r.Drifted() {
			t.Errorf("Verify(%s) after the job = %+v, want no drift", name, r.Tables)
		}
	}
	if n := count("SELECT COUNT(*) FROM remote_account_heads WHERE inbox_url = 'https://remote.example/users/dave/inbox' AND consecutive_fetch_failures = 3"); n != 1 {
		t.Errorf("remote heads with the snapshot's inbox and the 3 failures kept = %d, want 1", n)
	}
	// The rows whose end has passed stay, and the one that the expiry job removed stays away.
	for q, want := range map[string]int64{
		"SELECT COUNT(*) FROM account_restrictions WHERE restriction = 'media_sensitive'":                               0,
		"SELECT COUNT(*) FROM account_restrictions WHERE restriction = 'unsolicited_reach'":                             2,
		"SELECT COUNT(*) FROM feed_domain_restrictions WHERE domain = 'remote.example' AND effective_until IS NOT NULL": 1,
		"SELECT COUNT(*) FROM feed_author_restrictions WHERE effective_until IS NOT NULL":                               2,
	} {
		if n := count(q); n != want {
			t.Errorf("%s = %d, want %d", q, n, want)
		}
	}
}
