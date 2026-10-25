// SPDX-License-Identifier: MIT
// @mycroblog/mybot: build bots for mycroblog/my on the API client @mycroblog/mykit (ADR-0094). The author of a bot
// writes the handlers of its events; the library receives the events, so that none is lost or answered twice, and
// keeps the bot polite by default. It mirrors the Go library github.com/mycroblog/my/pkg/mybot: the same handlers,
// options, defaults, idempotency keys, and state file.
export {
  AlreadyRunningError,
  Bot,
  defaultCatchUpLimit,
  defaultConcurrency,
  defaultPollInterval,
  defaultProcessedRetention,
  defaultShutdownGrace,
  NotBotError,
  ScopeError,
  type BotOptions,
  type Mode,
} from "./bot.js";
export { defaultMaxSendWait, defaultPostRate, defaultReplyLimits, defaultWriteRate, type Rate } from "./budget.js";
export {
  BudgetExceededError,
  Context,
  NoAccountError,
  NoStatusError,
  ReplyLimitError,
  replyIdempotencyKey,
  replyVisibility,
  type Handler,
  type Middleware,
} from "./context.js";
export {
  everyIdempotencyKey,
  NotScheduledError,
  PostedAlreadyError,
  type CatchUp,
  type EveryOptions,
  type EveryVisibility,
} from "./every.js";
export { defaultStatePath, FileStore } from "./filestore.js";
export { defaultHelp, defaultMiddleware, manners, recover, sendBudget, type Command } from "./handler.js";
export type { Logger } from "./logger.js";
export { MemoryStore, type Store } from "./store.js";
export { parseSchedule, Schedule, ScheduleError } from "./schedule.js";
export { plainText } from "./text.js";
