// SPDX-License-Identifier: MIT
// The echo bot, which answers every mention with its text: `bun examples/echo/main.ts` with the environment of
// ../env.ts.
import { optionsFromEnv, runUntilSignal } from "../env.ts";
import { createBot } from "./bot.ts";

await runUntilSignal(createBot(optionsFromEnv("mybot-example-echo/1.0")));
