// SPDX-License-Identifier: MIT

package mybot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/mycroblog/my/pkg/mykit"
)

// ReplyIdempotencyKey is the Idempotency-Key of the n-th reply (from 1) that a handler posts for the event
// source: a notification ID, or for OnStatus the ID of the post (ADR-0094 §6). The server keeps the keys of each
// account, so the same event processed twice (after a restart in the middle of its handler) posts each reply
// once. Use it when posting with mykit directly:
//
//	api.CreateStatusWithResponse(ctx, &mykit.CreateStatusParams{IdempotencyKey: ptr(mybot.ReplyIdempotencyKey(id, 1))}, req)
func ReplyIdempotencyKey(source string, n int) string {
	return "mybot:reply:" + source + ":" + strconv.Itoa(n)
}

// ReplyVisibility is the visibility of a reply to a post of visibility v: never wider than v (ADR-0094 §8), and
// unlisted for a public post, so that a bot's replies stay out of the public timelines.
func ReplyVisibility(v mykit.Visibility) mykit.Visibility {
	switch v {
	case mykit.VisibilityDirect, mykit.VisibilityPrivate, mykit.VisibilityLocal, mykit.VisibilityUnlisted:
		return v
	default:
		return mykit.VisibilityUnlisted
	}
}

// Errors of the helpers of Context.
var (
	// ErrNoStatus is the error of Reply, React, and Favourite for an event without a post, such as a follow.
	ErrNoStatus = errors.New("mybot: the event has no post")
	// ErrNoAccount is the error of Follow for an event without an account.
	ErrNoAccount = errors.New("mybot: the event has no account")
	// ErrReplyLimit is the error of Reply when the replies to the sender reached a limit of Config.ReplyLimits.
	// The reply was not sent.
	ErrReplyLimit = errors.New("mybot: the replies to this sender reached their limit")
)

// Context is what a handler receives: the event, its post and its sender, the plain text and the command of a
// mention, and helpers that answer within the defaults of the library. It is the context.Context of the handler
// too, cancelled when the bot stops and the grace period is over.
type Context struct {
	context.Context

	// Notification is the notification of the event; nil for OnStatus.
	Notification *mykit.Notification
	// Status is the post of the event: the post that mentions the bot, the bot's post that was reacted to,
	// favourited, boosted, or quoted, the quote, or the post of OnStatus. Nil for an event without one.
	Status *mykit.Status
	// Account is the sender: the account that acted. Nil for a notification without one.
	Account *mykit.Account
	// Text is the content of Status as plain text (PlainText), "" without a Status.
	Text string
	// IsReply reports whether Status replies to a post.
	IsReply bool
	// IsDirect reports whether Status is a direct post.
	IsDirect bool
	// Command is the lowercase name of the command of OnCommand, "" otherwise.
	Command string
	// Args is the text after the command's name, with the spaces around it trimmed.
	Args string
	// Job is the name of the scheduled job (Every) that the Context runs, "" for an event.
	Job string
	// ScheduledAt is the time of the run of the scheduled job, which may be earlier than now for a run that
	// catches up; the zero time for an event.
	ScheduledAt time.Time

	bot *Bot
	// source is the ID that the idempotency keys of the replies derive from.
	source string
	// sender is the key of Account for the reply limits and the order of processing.
	sender  string
	replies int
	// every is the scheduled job of the run, and posted reports its Post.
	every  *everyJob
	posted bool
	// budgeted makes the helpers wait for the send budget (the SendBudget middleware).
	budgeted bool
}

// Me is the bot's own account, as verifyCredentials returned it when the bot started.
func (c *Context) Me() *mykit.CredentialAccount { return c.bot.me }

// API is the client of the server, for what the helpers do not cover. Writes through it skip the send budget and
// the reply limits: use ReplyIdempotencyKey for the posts.
func (c *Context) API() *mykit.API { return c.bot.api }

// Logger is the logger of the bot, with the event's attributes.
func (c *Context) Logger() *slog.Logger {
	l := c.bot.logger
	switch {
	case c.Notification != nil:
		l = l.With("notification", c.Notification.Id, "type", string(c.Notification.Type))
	case c.Status != nil:
		l = l.With("status", c.Status.Id)
	case c.Job != "":
		l = l.With("job", c.Job, "scheduled_at", c.ScheduledAt)
	}
	return l
}

// Reply posts text as a reply to Status. The reply mentions the sender alone, never the other accounts that the
// post mentions; its visibility is ReplyVisibility of the post's; and it carries the Idempotency-Key
// ReplyIdempotencyKey(event, n) for the n-th reply of the handler, so that processing the event again posts it
// once. It counts toward the reply limits of the sender and waits for the send budget.
func (c *Context) Reply(text string) (*mykit.Status, error) {
	st := c.Status
	if st == nil {
		return nil, ErrNoStatus
	}
	c.replies++
	key := ReplyIdempotencyKey(c.source, c.replies)
	body := text
	if c.Account != nil && c.Account.Id != c.bot.me.Id {
		body = "@" + c.Account.Acct + " " + text
	}
	limited := c.bot.cfg.ReplyLimits != nil && c.Account != nil
	var at time.Time
	if limited {
		at = c.bot.now()
		if !c.bot.replies.take(c.sender, at) {
			return nil, ErrReplyLimit
		}
	}
	if err := c.waitBudget(sendPost); err != nil {
		if limited {
			c.bot.replies.untake(c.sender, at)
		}
		return nil, err
	}
	inReplyTo := st.Id
	resp, err := c.bot.api.CreateStatusWithResponse(c, &mykit.CreateStatusParams{IdempotencyKey: &key}, mykit.CreateStatusRequest{
		Text:        body,
		Visibility:  ReplyVisibility(st.Visibility),
		InReplyToId: &inReplyTo,
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("mybot: createStatus answered %s", resp.Status())
	}
	return resp.JSON200, nil
}

// React reacts to Status with emoji: a Unicode emoji, or the shortcode of a custom emoji of the server. Reacting
// again with the same emoji changes nothing, so it needs no idempotency key. It waits for the send budget.
func (c *Context) React(emoji string) error {
	if c.Status == nil {
		return ErrNoStatus
	}
	if err := c.waitBudget(sendWrite); err != nil {
		return err
	}
	_, err := c.bot.api.ReactToStatusWithResponse(c, c.Status.Id, mykit.ReactToStatusRequest{Emoji: emoji})
	return err
}

// Favourite favourites Status. A second favourite changes nothing. It waits for the send budget.
func (c *Context) Favourite() error {
	if c.Status == nil {
		return ErrNoStatus
	}
	if err := c.waitBudget(sendWrite); err != nil {
		return err
	}
	_, err := c.bot.api.FavouriteStatusWithResponse(c, c.Status.Id)
	return err
}

// Follow follows the sender (for a locked account, requests to). Following again changes nothing. It waits for
// the send budget.
func (c *Context) Follow() error {
	if c.Account == nil {
		return ErrNoAccount
	}
	if err := c.waitBudget(sendWrite); err != nil {
		return err
	}
	_, err := c.bot.api.FollowAccountWithResponse(c, c.Account.Id, mykit.FollowAccountRequest{})
	return err
}

// waitBudget waits for the send budget of kind when the SendBudget middleware is in the chain. A wait longer than
// Config.MaxSendWait drops the write, logged, with ErrBudgetExceeded.
func (c *Context) waitBudget(kind sendKind) error {
	if !c.budgeted {
		return nil
	}
	wait, ok := c.bot.budget.reserve(kind, c.bot.now(), c.bot.cfg.MaxSendWait)
	if !ok {
		c.Logger().Warn("mybot: dropped a write that would wait too long for the send budget", "max_wait", c.bot.cfg.MaxSendWait)
		return ErrBudgetExceeded
	}
	if wait <= 0 {
		return nil
	}
	return c.bot.sleep(c, wait)
}
