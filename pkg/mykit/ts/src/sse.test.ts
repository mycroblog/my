// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { EventStreamParser, EventTooLargeError, maxEventLength, type ServerSentEvent } from "./sse.js";

function parseAll(...pieces: string[]): ServerSentEvent[] {
  const parser = new EventStreamParser();
  return [...pieces.flatMap((piece) => parser.push(piece)), ...parser.end()];
}

describe("EventStreamParser", () => {
  test("dispatches an event at a blank line, with its type and data", () => {
    expect(parseAll('event: update\ndata: {"id":"1"}\n\n')).toEqual([{ event: "update", data: '{"id":"1"}' }]);
  });

  test("skips comments, such as the server's heartbeat", () => {
    expect(parseAll(": connected\n\n: heartbeat\n\nevent: notification\ndata: x\n\n")).toEqual([
      { event: "notification", data: "x" },
    ]);
  });

  test("joins data lines with LF and names an event without a type message", () => {
    expect(parseAll("data: one\ndata: two\n\n")).toEqual([{ event: "message", data: "one\ntwo" }]);
  });

  test("removes only one space after the colon, and takes a field without a colon as empty", () => {
    expect(parseAll("data:no space\ndata:  two spaces\ndata\n\n")).toEqual([
      { event: "message", data: "no space\n two spaces\n" },
    ]);
  });

  test("does not dispatch an event without data, and resets the type after each event", () => {
    expect(parseAll("event: update\n\ndata: x\n\n")).toEqual([{ event: "message", data: "x" }]);
  });

  test("accepts CRLF, CR and LF line ends, also when a CRLF is split between pieces", () => {
    expect(parseAll("event: a\r\ndata: 1\r", "\n\r\nevent: b\rdata: 2\r\r")).toEqual([
      { event: "a", data: "1" },
      { event: "b", data: "2" },
    ]);
  });

  test("assembles an event that arrives in many pieces", () => {
    expect(parseAll("ev", "ent: up", "date\nda", "ta: {}", "\n", "\n")).toEqual([{ event: "update", data: "{}" }]);
  });

  test("ignores a leading byte order mark, and fields it does not use", () => {
    expect(parseAll("﻿id: 7\nretry: 10\ndata: x\n\n")).toEqual([{ event: "message", data: "x" }]);
  });

  test("throws at an event whose data lines add up to more than maxEventLength, or at a longer line", () => {
    const line = `data: ${"x".repeat(1 << 20)}\n`;
    const parser = new EventStreamParser();
    expect(() => {
      for (let i = 0; i <= maxEventLength >> 20; i++) parser.push(line);
    }).toThrow(EventTooLargeError);
    expect(() => new EventStreamParser().push("x".repeat(maxEventLength + 1))).toThrow(EventTooLargeError);
    expect(parseAll(line.repeat(2), "\n")).toHaveLength(1);
  });

  test("discards an event that the stream ended before completing", () => {
    expect(parseAll("data: complete\n\ndata: cut off\n")).toEqual([{ event: "message", data: "complete" }]);
  });
});
