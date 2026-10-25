// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/mycroblog/my/pkg/mybot"
)

// newBot returns the echo bot: it replies to every mention with the mention's text, without the mentions at its
// start. The reply mentions the sender alone and is never more visible than the mention (the manners of mybot).
func newBot(cfg mybot.Config) (*mybot.Bot, error) {
	bot, err := mybot.New(cfg)
	if err != nil {
		return nil, err
	}
	bot.OnMention(func(c *mybot.Context) error {
		text := withoutMentions(c.Text)
		if text == "" {
			return nil
		}
		_, err := c.Reply(text)
		return err
	})
	return bot, nil
}

// withoutMentions drops the mentions (the words that start with @) at the start of text.
func withoutMentions(text string) string {
	text = strings.TrimSpace(text)
	for strings.HasPrefix(text, "@") {
		_, rest, _ := strings.Cut(text, " ")
		text = strings.TrimSpace(rest)
	}
	return text
}
