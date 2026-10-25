// SPDX-License-Identifier: MIT

package main

import (
	"time"

	"github.com/mycroblog/my/pkg/mybot"
)

// newBot returns the scheduled bot: it posts a greeting at 9:00 every morning in the IANA time zone timeZone
// ("" for UTC). The post is unlisted, the default of the scheduled posts. With CatchUp Latest, a bot that was
// stopped at 9:00 posts that morning's greeting once when it starts again, and never a pile of the mornings it
// missed.
func newBot(cfg mybot.Config, timeZone string) (*mybot.Bot, error) {
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, err
	}
	bot, err := mybot.New(cfg)
	if err != nil {
		return nil, err
	}
	err = bot.Every("0 9 * * *", func(c *mybot.Context) error {
		_, err := c.Post(greeting(c.ScheduledAt.In(loc)))
		return err
	}, mybot.EveryOptions{Name: "morning", TimeZone: timeZone, CatchUp: mybot.Latest})
	if err != nil {
		return nil, err
	}
	return bot, nil
}

// greeting is the text of the post of the morning at.
func greeting(at time.Time) string {
	return "Good morning! Today is " + at.Format("Monday, 2 January 2006") + "."
}
