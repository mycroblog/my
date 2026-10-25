// SPDX-License-Identifier: MIT
// The scheduled bot, which posts a greeting every morning: `bun examples/schedule/main.ts` with the environment of
// ../env.ts and MYBOT_TIME_ZONE, the IANA time zone of the morning, such as Asia/Tokyo (UTC by default).
import { optionsFromEnv, runUntilSignal } from "../env.ts";
import { createBot } from "./bot.ts";

await runUntilSignal(createBot(optionsFromEnv("mybot-example-schedule/1.0"), process.env["MYBOT_TIME_ZONE"] || "UTC"));
