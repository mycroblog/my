// SPDX-License-Identifier: MIT

/**
 * Receives the log of a bot: `console`, the default, or anything with the same four methods. The bot calls each with
 * a message and an object of attributes, such as `logger.warn("mybot: ...", { notification: "...", error })`.
 */
export type Logger = {
  debug(message: string, ...data: unknown[]): void;
  info(message: string, ...data: unknown[]): void;
  warn(message: string, ...data: unknown[]): void;
  error(message: string, ...data: unknown[]): void;
};

/** Whether value is a plain object of attributes. */
function isAttributes(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && Object.getPrototypeOf(value) === Object.prototype;
}

/** A Logger that adds attrs to the attributes of every call to logger. */
export function withAttributes(logger: Logger, attrs: Record<string, unknown>): Logger {
  const call =
    (level: keyof Logger) =>
    (message: string, ...data: unknown[]): void => {
      const [first, ...more] = data;
      if (isAttributes(first)) {
        logger[level](message, { ...attrs, ...first }, ...more);
      } else {
        logger[level](message, attrs, ...data);
      }
    };
  return { debug: call("debug"), info: call("info"), warn: call("warn"), error: call("error") };
}
