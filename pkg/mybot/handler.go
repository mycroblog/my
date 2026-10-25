// SPDX-License-Identifier: MIT

package mybot

import (
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mycroblog/my/pkg/mykit"
)

// Handler handles one event. The error it returns is logged; the event counts as processed all the same, so
// that a failing event is not retried for ever.
type Handler func(c *Context) error

// Middleware wraps a handler, such as to check who may run a command, or to log in a format of one's own.
type Middleware func(next Handler) Handler

// DefaultMiddleware is the chain that every handler runs in unless Config.NoDefaultMiddleware is set, outermost
// first: Recover, Manners, and SendBudget (ADR-0094 §4).
func DefaultMiddleware() []Middleware {
	return []Middleware{Recover(), Manners(), SendBudget()}
}

// Recover logs the error of the handler, and recovers and logs its panic, so that one event never stops the bot.
func Recover() Middleware {
	return func(next Handler) Handler {
		return func(c *Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					c.Logger().Error("mybot: the handler panicked", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
					err = nil
				}
			}()
			if err := next(c); err != nil {
				c.Logger().Error("mybot: the handler failed", "error", err)
			}
			return nil
		}
	}
}

// Manners skips the handler, the event counting as processed, for a sender whose account has the bot flag
// (unless Config.RespondToBots) and for a sender whom the bot replied to as often as Config.ReplyLimits allow
// (ADR-0094 §8). Events of the bot's own account never reach a handler, with or without it.
func Manners() Middleware {
	return func(next Handler) Handler {
		return func(c *Context) error {
			if c.Account != nil && c.Account.Bot && !c.bot.cfg.RespondToBots {
				c.Logger().Debug("mybot: skipped an event of a bot", "account", c.Account.Acct)
				return nil
			}
			if c.Account != nil && c.bot.cfg.ReplyLimits != nil && c.bot.replies.full(c.sender, c.bot.now()) {
				c.Logger().Info("mybot: skipped an event of a sender at the reply limit", "account", c.Account.Acct)
				return nil
			}
			return next(c)
		}
	}
}

// SendBudget makes the helpers of Context (Reply, React, Favourite, Follow) wait for the client-side send budget
// of Config.PostRate and Config.WriteRate, and for the reset of the server's rate limit when less than a tenth
// of it remains (ADR-0094 §6). A write that would wait longer than Config.MaxSendWait is dropped and logged.
func SendBudget() Middleware {
	return func(next Handler) Handler {
		return func(c *Context) error {
			c.budgeted = true
			return next(c)
		}
	}
}

// Command is a command that OnCommand registered.
type Command struct {
	// Name is the lowercase name of the command.
	Name string
	// Description is what the command does, for the answer of help.
	Description string

	handler Handler
}

// DefaultHelp formats the answer of the help command: a line for each command with its description.
func DefaultHelp(cmds []Command) string {
	var b strings.Builder
	for i, c := range cmds {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(c.Name)
		if c.Description != "" {
			b.WriteString(": ")
			b.WriteString(c.Description)
		}
	}
	return b.String()
}

// handlers are the handlers that a bot registered.
type handlers struct {
	mention, follow, followRequest, reaction, favourite, boost, quote, notification, status Handler
	commands                                                                                map[string]*Command
}

// OnMention registers the handler of the mentions of the bot, replies and direct posts included, that are no
// registered command.
func (b *Bot) OnMention(h Handler) { b.h.mention = h }

// OnCommand registers the handler of the mentions whose first word, after the mentions of the bot at their start,
// is name, compared without case: `@bot ping` runs the command ping, whose Context has Command "ping" and Args
// the rest of the text. description is the command's line in the answer of help, which the bot gives unless
// Config.DisableHelp is set or a command help is registered.
func (b *Bot) OnCommand(name, description string, h Handler) {
	name = strings.ToLower(name)
	if b.h.commands == nil {
		b.h.commands = map[string]*Command{}
	}
	b.h.commands[name] = &Command{Name: name, Description: description, handler: h}
}

// OnFollow registers the handler of new followers.
func (b *Bot) OnFollow(h Handler) { b.h.follow = h }

// OnFollowRequest registers the handler of the follow requests to a locked bot account.
func (b *Bot) OnFollowRequest(h Handler) { b.h.followRequest = h }

// OnReaction registers the handler of the emoji reactions to the bot's posts.
func (b *Bot) OnReaction(h Handler) { b.h.reaction = h }

// OnFavourite registers the handler of the favourites of the bot's posts.
func (b *Bot) OnFavourite(h Handler) { b.h.favourite = h }

// OnBoost registers the handler of the boosts of the bot's posts.
func (b *Bot) OnBoost(h Handler) { b.h.boost = h }

// OnQuote registers the handler of the quotes of the bot's posts.
func (b *Bot) OnQuote(h Handler) { b.h.quote = h }

// OnNotification registers the handler of every notification that no handler above takes, the types without a
// handler of their own among them.
func (b *Bot) OnNotification(h Handler) { b.h.notification = h }

// OnStatus registers the handler of the new posts of the home timeline, from the event stream alone: they are
// not caught up after a disconnection or a restart, and Mode Polling does not receive them. The bot's own posts
// are left out.
func (b *Bot) OnStatus(h Handler) { b.h.status = h }

// Use adds middleware inside the default chain, in the order given.
func (b *Bot) Use(mw ...Middleware) { b.middleware = append(b.middleware, mw...) }

// Commands are the registered commands, by name, with help when the bot answers it.
func (b *Bot) Commands() []Command {
	var cmds []Command
	for _, c := range b.h.commands {
		cmds = append(cmds, *c)
	}
	if _, ok := b.h.commands["help"]; !ok && !b.cfg.DisableHelp {
		cmds = append(cmds, Command{Name: "help", Description: "list the commands"})
	}
	slices.SortFunc(cmds, func(a, b Command) int { return strings.Compare(a.Name, b.Name) })
	return cmds
}

// route picks the handler of the event of c, filling in its command; nil when none takes it.
func (b *Bot) route(c *Context) Handler {
	if c.Notification == nil {
		return b.h.status
	}
	var h Handler
	switch c.Notification.Type {
	case mykit.NotificationTypeMention:
		if name, args, ok := parseCommand(c.Text, b.me); ok {
			if cmd, found := b.h.commands[name]; found {
				c.Command, c.Args = name, args
				return cmd.handler
			}
			if name == "help" && !b.cfg.DisableHelp && len(b.h.commands) > 0 {
				c.Command, c.Args = name, args
				return b.help
			}
		}
		h = b.h.mention
	case mykit.NotificationTypeFollow:
		h = b.h.follow
	case mykit.NotificationTypeFollowRequest:
		h = b.h.followRequest
	case mykit.NotificationTypeReaction:
		h = b.h.reaction
	case mykit.NotificationTypeFavourite:
		h = b.h.favourite
	case mykit.NotificationTypeBoost:
		h = b.h.boost
	case mykit.NotificationTypeQuote:
		h = b.h.quote
	}
	if h == nil {
		h = b.h.notification
	}
	return h
}

// help answers the help command with the registered commands.
func (b *Bot) help(c *Context) error {
	_, err := c.Reply(b.cfg.Help(b.Commands()))
	return err
}

// parseCommand reads the command of a mention's text: the first word after the mentions of me at its start,
// lowercase, and the rest of the text. It reports false when no word follows.
func parseCommand(text string, me *mykit.CredentialAccount) (name, args string, ok bool) {
	s := strings.TrimSpace(text)
	for {
		word, rest := cutWord(s)
		if word == "" {
			return "", "", false
		}
		if !mentionsMe(word, me) {
			return strings.ToLower(word), strings.TrimSpace(rest), true
		}
		s = rest
	}
}

// cutWord splits s at the end of its first word, after the spaces before it.
func cutWord(s string) (word, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if i := strings.IndexFunc(s, unicode.IsSpace); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

// mentionsMe reports whether word is a mention of me: @username or @username@domain, without case, and with the
// punctuation that may follow it (`@bot,`).
func mentionsMe(word string, me *mykit.CredentialAccount) bool {
	handle, ok := strings.CutPrefix(word, "@")
	if !ok || me == nil {
		return false
	}
	handle = strings.TrimRightFunc(handle, func(r rune) bool {
		return r != '_' && r < utf8.RuneSelf && unicode.IsPunct(r)
	})
	for _, h := range []string{me.Username, me.Acct, me.Username + "@" + me.Domain} {
		if h != "" && strings.EqualFold(handle, h) {
			return true
		}
	}
	return false
}
