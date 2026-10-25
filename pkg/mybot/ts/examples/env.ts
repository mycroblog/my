// SPDX-License-Identifier: MIT
// The part of the examples' main.ts files that reads the environment and runs a bot until a signal stops it.
//
// The environment: MYBOT_SERVER (such as https://example.com), MYBOT_CLIENT_ID, MYBOT_CLIENT_SECRET, and
// MYBOT_REFRESH_TOKEN, the app and the token that the settings page of the server issued to the bot's own account,
// and MYBOT_STATE, the state file (./mybot-state.json by default).
import { type Bot, type BotOptions, defaultStatePath, FileStore } from "@mycroblog/mybot";

/** Reads the options of a bot from the environment, with userAgent such as "echobot/1.0". */
export function optionsFromEnv(userAgent: string): BotOptions {
  const env = (name: string): string => {
    const v = process.env[name];
    if (v === undefined || v === "") throw new Error(`set ${name}`);
    return v;
  };
  return {
    server: env("MYBOT_SERVER"),
    oauth: { clientId: env("MYBOT_CLIENT_ID"), clientSecret: env("MYBOT_CLIENT_SECRET") },
    refreshToken: env("MYBOT_REFRESH_TOKEN"),
    store: new FileStore(process.env["MYBOT_STATE"] || defaultStatePath),
    userAgent,
  };
}

/** Runs bot until SIGINT or SIGTERM, then lets it shut down gracefully. */
export async function runUntilSignal(bot: Bot): Promise<void> {
  const controller = new AbortController();
  const stop = (): void => {
    controller.abort();
  };
  process.once("SIGINT", stop);
  process.once("SIGTERM", stop);
  await bot.run({ signal: controller.signal });
}
