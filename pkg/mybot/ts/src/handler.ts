// SPDX-License-Identifier: MIT
import type { components } from "@mycroblog/mykit";
import { type Handler, internal, type Middleware } from "./context.js";

type CredentialAccount = components["schemas"]["CredentialAccount"];

/** Logs what the handler throws, so that one event never stops the bot. */
export function recover(): Middleware {
  return async (ctx, next) => {
    try {
      await next();
    } catch (error) {
      ctx.logger.error("mybot: the handler failed", { error });
    }
  };
}

/**
 * Skips the handler, the event counting as processed, for a sender whose account has the bot flag (unless the
 * respondToBots option) and for a sender whom the bot replied to as often as the replyLimits option allows (ADR-0094
 * §8). Events of the bot's own account never reach a handler, with or without it.
 */
export function manners(): Middleware {
  return async (ctx, next) => {
    const { runtime, sender } = ctx[internal];
    if (ctx.account?.bot === true && !runtime.respondToBots) {
      ctx.logger.debug("mybot: skipped an event of a bot", { account: ctx.account.acct });
      return;
    }
    if (ctx.account !== undefined && runtime.replyLimits !== null && runtime.replies.full(sender, runtime.now())) {
      ctx.logger.info("mybot: skipped an event of a sender at the reply limit", { account: ctx.account.acct });
      return;
    }
    await next();
  };
}

/**
 * Makes the helpers of Context (reply, react, favourite, follow) wait for the client-side send budget of the postRate
 * and writeRate options, and for the reset of the server's rate limit when less than a tenth of it remains (ADR-0094
 * §6). A write that would wait longer than the maxSendWait option is dropped and logged.
 */
export function sendBudget(): Middleware {
  return async (ctx, next) => {
    ctx[internal].budgeted = true;
    await next();
  };
}

/**
 * The chain that every handler runs in unless the noDefaultMiddleware option is set, outermost first: recover,
 * manners, and sendBudget (ADR-0094 §4).
 */
export function defaultMiddleware(): Middleware[] {
  return [recover(), manners(), sendBudget()];
}

/** Wraps handler in middleware, the first outermost. */
export function chain(middleware: readonly Middleware[], handler: Handler): Handler {
  return (ctx) => {
    const step = async (i: number): Promise<void> => {
      const mw = middleware[i];
      if (mw === undefined) {
        await handler(ctx);
        return;
      }
      await mw(ctx, () => step(i + 1));
    };
    return step(0);
  };
}

/** A command that onCommand registered. */
export type Command = {
  /** The lowercase name of the command. */
  readonly name: string;
  /** What the command does, for the answer of help. */
  readonly description: string;
};

/** Formats the answer of the help command: a line for each command with its description. */
export function defaultHelp(commands: readonly Command[]): string {
  return commands.map((c) => (c.description === "" ? c.name : `${c.name}: ${c.description}`)).join("\n");
}

/** ASCII punctuation (Unicode category P), but '_', which may follow a mention: `@bot,`. */
const trailingPunctuation = /[!"#%&'()*,\-./:;?@[\\\]{}]+$/;

/** Whether word is a mention of me: @username or @username@domain, without case, and with the punctuation that may follow it. */
function mentionsMe(word: string, me: CredentialAccount | undefined): boolean {
  if (!word.startsWith("@") || me === undefined) return false;
  const handle = word.slice(1).replace(trailingPunctuation, "").toLowerCase();
  return [me.username, me.acct, `${me.username}@${me.domain}`].some((h) => h !== "" && h.toLowerCase() === handle);
}

/**
 * Reads the command of a mention's text: the first word after the mentions of me at its start, lowercase, and the
 * rest of the text. It is null when no word follows.
 */
export function parseCommand(text: string, me: CredentialAccount | undefined): { name: string; args: string } | null {
  let s = text.trim();
  for (;;) {
    const m = /^\s*(\S+)([\s\S]*)$/.exec(s);
    if (m === null) return null;
    const word = m[1] ?? "";
    const rest = m[2] ?? "";
    if (!mentionsMe(word, me)) return { name: word.toLowerCase(), args: rest.trim() };
    s = rest;
  }
}
