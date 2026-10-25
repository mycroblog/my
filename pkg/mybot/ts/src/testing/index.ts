// SPDX-License-Identifier: MIT
// @mycroblog/mybot/testing: test bots of @mycroblog/mybot without a server (ADR-0094 §10), as mybottest does for the
// Go library. Server is a fake server of the client API behind a fetch; the test sends the bot notifications with
// inject and checks what it posted with posted or waitPosted. Its Clock moves only when the test moves it, so that a
// test runs the scheduled jobs (Bot.every) at once.
export { Clock } from "./clock.js";
export { domain, quietLogger, Server, token, type Post, type Reaction, type WaitOptions } from "./server.js";
