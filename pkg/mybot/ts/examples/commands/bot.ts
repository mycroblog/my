// SPDX-License-Identifier: MIT
import { Bot, type BotOptions } from "@mycroblog/mybot";

/** The limits of the dice command, so that a reply stays short. */
const maxDice = 10;
const maxSides = 1000;

/** The answer to dice arguments that the bot does not take. */
export const diceUsage = `Write the dice as NdM, such as 2d6: up to ${maxDice} dice of 2 to ${maxSides} sides.`;

/** A throw of a die of n sides: a number from 1 to n. */
export type Roll = (n: number) => number;

const randomRoll: Roll = (n) => Math.floor(Math.random() * n) + 1;

/**
 * The command bot. It answers "@bot ping" with pong and "@bot dice 2d6" with the throws of two six-sided dice (one
 * with "@bot dice"); "@bot help" lists the commands with their descriptions, the answer of the library.
 */
export function createBot(options: BotOptions, roll: Roll = randomRoll): Bot {
  const bot = new Bot(options);
  bot.onCommand("ping", "answer pong", async (ctx) => {
    await ctx.reply("pong");
  });
  bot.onCommand("dice", "throw dice, such as dice 2d6", async (ctx) => {
    const dice = parseDice(ctx.args);
    if (dice === undefined) {
      await ctx.reply(diceUsage);
      return;
    }
    const throws = Array.from({ length: dice.n }, () => roll(dice.sides));
    const sum = throws.reduce((a, b) => a + b, 0);
    await ctx.reply(throws.length === 1 ? `🎲 ${sum}` : `🎲 ${throws.join(" + ")} = ${sum}`);
  });
  // A mention that is no command gets a hint.
  bot.onMention(async (ctx) => {
    await ctx.reply("I do not know that command. Try help.");
  });
  return bot;
}

/**
 * Reads the arguments of the dice command: "NdM" (N dice of M sides), "dM" (one die), or "" (one six-sided die);
 * undefined for others.
 */
export function parseDice(args: string): { n: number; sides: number } | undefined {
  const s = args.trim().toLowerCase();
  if (s === "") return { n: 1, sides: 6 };
  const m = /^(\d*)d(\d+)$/u.exec(s);
  if (m === null) return undefined;
  const n = m[1] === "" || m[1] === undefined ? 1 : Number(m[1]);
  const sides = Number(m[2]);
  if (n < 1 || n > maxDice || sides < 2 || sides > maxSides) return undefined;
  return { n, sides };
}
