// SPDX-License-Identifier: MIT
import { Bot, type BotOptions } from "@mycroblog/mybot";

/**
 * The scheduled bot: it posts a greeting at 9:00 every morning in the IANA time zone timeZone (UTC by default). The
 * post is unlisted, the default of the scheduled posts. With catchUp "latest", a bot that was stopped at 9:00 posts
 * that morning's greeting once when it starts again, and never a pile of the mornings it missed.
 */
export function createBot(options: BotOptions, timeZone = "UTC"): Bot {
  const bot = new Bot(options);
  bot.every(
    "0 9 * * *",
    async (ctx) => {
      await ctx.post(greeting(ctx.scheduledAt ?? Date.now(), timeZone));
    },
    { name: "morning", timeZone, catchUp: "latest" },
  );
  return bot;
}

/** The text of the post of the morning at (milliseconds since the Unix epoch) in timeZone. */
function greeting(at: number, timeZone: string): string {
  const parts = new Intl.DateTimeFormat("en-US", { timeZone, weekday: "long", day: "numeric", month: "long", year: "numeric" }).formatToParts(at);
  const part = (type: Intl.DateTimeFormatPartTypes): string => parts.find((p) => p.type === type)?.value ?? "";
  return `Good morning! Today is ${part("weekday")}, ${part("day")} ${part("month")} ${part("year")}.`;
}
