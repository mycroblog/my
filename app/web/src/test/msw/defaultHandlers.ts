// SPDX-License-Identifier: AGPL-3.0-only
import * as modules from "./handlers";

/** The default handlers of every module listed in handlers.ts: the MSW server of Vitest and the worker of Storybook. */
export const defaultHandlers = Object.values(modules).flatMap((source) => source.handlers);
