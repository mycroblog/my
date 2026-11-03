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

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/federation"
)

// runAdminRelay implements `my admin relay <subcommand>`: the relays the instance joins
// (ADR-0061, FR-724). The CLI acts as no user: what it does is recorded in the audit log as the
// command line's.
func runAdminRelay(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("admin relay", args, []string{"create", "follow", "unfollow", "remove", "list"}, stderr, adminRelayUsage)
	if code >= 0 || len(args) == 0 {
		return max(code, 0)
	}
	var (
		protocol string
		sendOnly bool
		nargs    = 1
	)
	if sub == "list" {
		nargs = 0
	}
	c, code := parseSubcommand("my admin relay "+sub, args[1:], nargs, stderr, adminRelayUsage, func(fs *flag.FlagSet) {
		if sub == "create" {
			fs.StringVar(&protocol, "protocol", "", "mastodon (the URL is the relay's inbox) or litepub (the URL is its actor); detected when omitted")
			fs.BoolVar(&sendOnly, "send-only", false, "send public posts to the relay without taking in what it relays")
		}
	})
	if code >= 0 {
		return code
	}
	if protocol != "" && !federation.RelayProtocol(protocol).Valid() {
		_, _ = fmt.Fprintf(stderr, "%s: -protocol must be mastodon or litepub\n", c.name)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var (
		v   appfederation.RelayView
		err error
	)
	switch sub {
	case "list":
		var vs []appfederation.RelayView
		if vs, err = app.Relays.List(ctx); err == nil {
			err = printRelays(stdout, vs)
		}
	case "create":
		v, err = app.Relays.Add(ctx, appfederation.RelayInput{
			Protocol: federation.RelayProtocol(protocol), URL: c.fs.Arg(0), Receive: !sendOnly,
		}, "")
		if err == nil {
			_, _ = fmt.Fprintf(stdout, "created %s relay %s (relay %s, inbox %s); its Follow is sent\n", v.Relay.Protocol, v.Relay.URL, v.Relay.ID, v.Relay.InboxURL)
		}
	case "follow":
		if v, err = app.Relays.Follow(ctx, c.fs.Arg(0), ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "sent a new Follow to relay %s\n", v.Relay.ID)
		}
	case "unfollow":
		if v, err = app.Relays.Unfollow(ctx, c.fs.Arg(0), ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "left relay %s\n", v.Relay.ID)
		}
	case "remove":
		if err = app.Relays.Remove(ctx, c.fs.Arg(0), ""); err == nil {
			_, _ = fmt.Fprintf(stdout, "removed relay %s\n", c.fs.Arg(0))
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", c.name, describeRelayError(err, c.fs.Arg(0)))
		return 1
	}
	return 0
}

// describeRelayError turns a failure of the relay use cases into a message for the operator.
func describeRelayError(err error, arg string) string {
	switch {
	case errors.Is(err, appfederation.ErrNotFound):
		return fmt.Sprintf("no relay has the ID %q", arg)
	case errors.Is(err, federation.ErrRelayNotJoinable):
		return "the relay is pending or accepted; leave it before joining it again"
	case errors.Is(err, federation.ErrRelayNotJoined):
		return "the relay is not pending or accepted"
	case errors.Is(err, appfederation.ErrRelayNotRemovable):
		return "the relay is pending or accepted; leave it before removing it"
	}
	return err.Error()
}

func printRelays(w io.Writer, vs []appfederation.RelayView) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tPROTOCOL\tURL\tSTATE\tSINCE\tRECEIVE\tTAKEN_24H\tDROPPED_24H\tSUSPENDED")
	for _, v := range vs {
		state, since := string(v.Status.State), v.Status.Since
		if state == "" {
			state, since = string(federation.RelayPending), v.Relay.CreatedAt
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%t\t%d\t%d\t%t\n", v.Relay.ID, v.Relay.Protocol, v.Relay.URL, state,
			since.Format(time.RFC3339), v.Relay.Receive, v.Taken, v.Dropped, v.Suspended)
	}
	return tw.Flush()
}

func adminRelayUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my admin relay list [-config <file>]
  my admin relay create [-protocol mastodon|litepub] [-send-only] [-config <file>] <url>
  my admin relay follow [-config <file>] <relay id>
  my admin relay unfollow [-config <file>] <relay id>
  my admin relay remove [-config <file>] <relay id>

The relays the instance joins. A relay passes the public posts of the
servers that join it to the others.

create registers a relay and sends the instance actor's Follow; the relay is
pending until it accepts or rejects it. <url> is the relay's inbox for a
mastodon relay (pub-relay, Activity-Relay) and its actor for a litepub one
(Pleroma and Akkoma's relays); without -protocol, a URL that serves an actor is
a litepub relay and any other a mastodon one. Only public posts go to a relay,
with a linked data signature, and the public posts it relays are fetched from
their origin into the federated and hashtag timelines, up to
relay_ingest_hourly_limit per hour, and kept for relay_status_retention.
-send-only takes in nothing from the relay.

unfollow leaves a pending or accepted relay with the Undo of its Follow, and
follow joins a relay that the instance left or that rejected it again. remove
deletes the registration of a relay that the instance left or that rejected it.
list shows every relay with its state and what it brought in during the last
24 hours. Every operation, and the relay's Accept or Reject, is recorded in the
audit log.
`)
}
