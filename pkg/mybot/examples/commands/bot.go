// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mycroblog/my/pkg/mybot"
)

// The limits of the dice command, so that a reply stays short.
const (
	maxDice  = 10
	maxSides = 1000
)

// newBot returns the command bot. It answers "@bot ping" with pong and "@bot dice 2d6" with the throws of two
// six-sided dice (one with "@bot dice"); "@bot help" lists the commands with their descriptions, the answer of
// the library. roll(n) returns a number from 1 to n.
func newBot(cfg mybot.Config, roll func(n int) int) (*mybot.Bot, error) {
	bot, err := mybot.New(cfg)
	if err != nil {
		return nil, err
	}
	bot.OnCommand("ping", "answer pong", func(c *mybot.Context) error {
		_, err := c.Reply("pong")
		return err
	})
	bot.OnCommand("dice", "throw dice, such as dice 2d6", func(c *mybot.Context) error {
		n, sides, err := parseDice(c.Args)
		if err != nil {
			_, err := c.Reply(err.Error())
			return err
		}
		throws := make([]string, n)
		sum := 0
		for i := range throws {
			v := roll(sides)
			sum += v
			throws[i] = strconv.Itoa(v)
		}
		text := fmt.Sprintf("🎲 %s", throws[0])
		if n > 1 {
			text = fmt.Sprintf("🎲 %s = %d", strings.Join(throws, " + "), sum)
		}
		_, err = c.Reply(text)
		return err
	})
	// A mention that is no command gets a hint.
	bot.OnMention(func(c *mybot.Context) error {
		_, err := c.Reply("I do not know that command. Try help.")
		return err
	})
	return bot, nil
}

// errDice is the answer to dice arguments that the bot does not take.
var errDice = fmt.Errorf("Write the dice as NdM, such as 2d6: up to %d dice of 2 to %d sides.", maxDice, maxSides) //nolint:staticcheck,revive // ST1005: the message is the bot's reply, a sentence

// parseDice reads the arguments of the dice command: "NdM" (N dice of M sides), "dM" (one die), or "" (one
// six-sided die).
func parseDice(args string) (n, sides int, err error) {
	args = strings.ToLower(strings.TrimSpace(args))
	if args == "" {
		return 1, 6, nil
	}
	count, s, ok := strings.Cut(args, "d")
	if !ok {
		return 0, 0, errDice
	}
	n = 1
	if count != "" {
		if n, err = strconv.Atoi(count); err != nil {
			return 0, 0, errDice
		}
	}
	if sides, err = strconv.Atoi(s); err != nil {
		return 0, 0, errDice
	}
	if n < 1 || n > maxDice || sides < 2 || sides > maxSides {
		return 0, 0, errDice
	}
	return n, sides, nil
}
