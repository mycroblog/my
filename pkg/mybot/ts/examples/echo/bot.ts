// SPDX-License-Identifier: MIT
import { Bot, type BotOptions } from "@mycroblog/mybot";

/** The mentions (the words that start with @) at the start of a text, and the spaces after them. */
const leadingMentions = /^(?:@\S*\s*)+/u;

/**
 * The echo bot: it replies to every mention with the mention's text, without the mentions at its start. The reply
 * mentions the sender alone and is never more visible than the mention (the manners of mybot).
 */
export function createBot(options: BotOptions): Bot {
  const bot = new Bot(options);
  bot.onMention(async (ctx) => {
    const text = withoutMentions(ctx.text);
    if (text !== "") await ctx.reply(text);
  });
  return bot;
}

/** Drops the mentions at the start of text. */
export function withoutMentions(text: string): string {
  return text.trim().replace(leadingMentions, "");
}
