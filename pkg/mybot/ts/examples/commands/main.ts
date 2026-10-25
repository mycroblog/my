// SPDX-License-Identifier: MIT
// The command bot, which answers help, ping, and dice: `bun examples/commands/main.ts` with the environment of
// ../env.ts.
import { optionsFromEnv, runUntilSignal } from "../env.ts";
import { createBot } from "./bot.ts";

await runUntilSignal(createBot(optionsFromEnv("mybot-example-commands/1.0")));
