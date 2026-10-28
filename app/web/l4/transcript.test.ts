// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { failureOf, measure, parseTranscript, snapshotRefs, type Step } from "./transcript.ts";

/** A stream-json transcript of Claude Code: each call is a tool use and its result. */
function transcript(
  calls: { name: string; input?: Record<string, unknown>; text: string; error?: boolean }[],
  result: Record<string, unknown>,
): string {
  const lines = [JSON.stringify({ type: "system", subtype: "init" })];
  for (const [i, call] of calls.entries()) {
    const id = `toolu_${String(i)}`;
    lines.push(
      JSON.stringify({
        type: "assistant",
        message: {
          content: [{ type: "tool_use", id, name: `mcp__playwright__${call.name}`, input: call.input ?? {} }],
        },
      }),
      JSON.stringify({
        type: "user",
        message: {
          content: [
            {
              type: "tool_result",
              tool_use_id: id,
              content: [{ type: "text", text: call.text }],
              is_error: call.error,
            },
          ],
        },
      }),
    );
  }
  lines.push(JSON.stringify({ type: "result", ...result }));
  return lines.join("\n");
}

const settings = [
  "### Page",
  "- Page URL: http://localhost:4000/settings",
  "### Snapshot",
  "```yaml",
  '- navigation "設定" [ref=e5]:',
  '  - link "プロフィール" [ref=e6] [cursor=pointer]:',
  '  - link "バッジとサポーターの一覧" [ref=e7]',
  "```",
].join("\n");
const badges = [
  "### Page",
  "- Page URL: http://localhost:4000/settings/badges",
  "### Snapshot",
  "```yaml",
  '- switch "Helper" [ref=e20]',
  "```",
].join("\n");
const moved = (path: string) => `### Page\n- Page URL: http://localhost:4000${path}\n### Snapshot\n- [Snapshot](x.yml)`;

describe("parseTranscript", () => {
  it("records the steps by the controls of the last snapshot and the verdict", () => {
    const parsed = parseTranscript(
      transcript(
        [
          { name: "browser_snapshot", text: settings },
          { name: "browser_click", input: { element: "Profile", target: "e6" }, text: moved("/settings/profile") },
          { name: "browser_navigate_back", text: moved("/settings") },
          { name: "browser_snapshot", text: settings },
          {
            name: "browser_click",
            input: { element: "a missing link", target: "e99" },
            text: "### Error",
            error: true,
          },
          { name: "browser_click", input: { element: "Badges", target: "e7" }, text: moved("/settings/badges") },
          { name: "browser_snapshot", text: badges },
          {
            name: "browser_click",
            input: { element: "Helper switch", target: "e20" },
            text: moved("/settings/badges"),
          },
          { name: "browser_press_key", input: { key: "Escape" }, text: moved("/settings/badges") },
        ],
        { subtype: "success", total_cost_usd: 0.12, structured_output: { completed: true, summary: "Shown." } },
      ),
    );
    expect(parsed.path).toEqual<Step[]>([
      { action: "browser_click", target: 'link "プロフィール"', url: "/settings/profile" },
      { action: "browser_navigate_back", target: "", url: "/settings" },
      { action: "browser_click", target: 'link "バッジとサポーターの一覧"', url: "/settings/badges" },
      { action: "browser_click", target: 'switch "Helper"', url: "/settings/badges" },
      { action: "browser_press_key", target: "key Escape", url: "/settings/badges" },
    ]);
    expect(parsed).toMatchObject({ completed: true, summary: "Shown.", ending: "success", costUSD: 0.12 });
  });

  it("counts each run of steps in the mailbox as one step and records them apart", () => {
    const mail = (path: string) =>
      `### Open tabs\n- 0: [App] (http://localhost:4000/)\n- 1: (current) [Mailpit] (http://127.0.0.1:8025${path})\n### Page\n- Page URL: http://127.0.0.1:8025${path}`;
    const parsed = parseTranscript(
      transcript(
        [
          { name: "browser_click", input: { element: "Send", target: "e3" }, text: moved("/auth/forgot_password") },
          { name: "browser_tabs", input: { action: "list" }, text: moved("/auth/forgot_password") },
          { name: "browser_tabs", input: { action: "select", index: 1 }, text: mail("/") },
          { name: "browser_click", input: { element: "the email", target: "e8" }, text: mail("/view/abc") },
          { name: "browser_click", input: { element: "the link", target: "f1e2" }, text: mail("/view/abc") },
          { name: "browser_tabs", input: { action: "select", index: 2 }, text: moved("/auth/reset_password?t=1") },
          {
            name: "browser_type",
            input: { element: "Password", target: "e4" },
            text: moved("/auth/reset_password?t=1"),
          },
        ],
        { subtype: "success", structured_output: { completed: true, summary: "Reset." } },
      ),
      "http://127.0.0.1:8025",
    );
    expect(parsed.path).toEqual<Step[]>([
      { action: "browser_click", target: "Send", url: "/auth/forgot_password" },
      { action: "mail", target: "", url: "/auth/reset_password?t=1" },
      { action: "browser_type", target: "Password", url: "/auth/reset_password?t=1" },
    ]);
    expect(parsed.mailPath).toEqual<Step[]>([
      { action: "browser_tabs", target: "tab select 1", url: "http://127.0.0.1:8025/" },
      { action: "browser_click", target: "the email", url: "http://127.0.0.1:8025/view/abc" },
      { action: "browser_click", target: "the link", url: "http://127.0.0.1:8025/view/abc" },
      { action: "browser_tabs", target: "tab select 2", url: "http://localhost:4000/auth/reset_password?t=1" },
    ]);
  });

  it("is not completed when the session ran out of turns", () => {
    const parsed = parseTranscript(
      transcript([], { subtype: "error_max_turns", structured_output: { completed: true, summary: "" } }),
    );
    expect(parsed).toMatchObject({ completed: false, ending: "error_max_turns" });
  });

  it("reports an API error of a session that ended with is_error as such, never as completed", () => {
    const parsed = parseTranscript(
      transcript([], {
        subtype: "success",
        is_error: true,
        result: "API Error: 401 authentication_error",
        structured_output: { completed: true, summary: "" },
      }),
    );
    expect(parsed).toMatchObject({ completed: false, apiError: "API Error: 401 authentication_error" });
  });

  it("has no API error for a normal session", () => {
    expect(parseTranscript(transcript([], { subtype: "success", is_error: false })).apiError).toBeNull();
  });

  it("reports a browser that never gave the agent a page", () => {
    const failed = [
      "### Error",
      "Error: async initializeServer: Target page, context or browser has been closed",
      "Browser logs:",
      "Chromium sandboxing failed!",
    ].join("\n");
    const parsed = parseTranscript(
      transcript(
        [
          { name: "browser_snapshot", text: failed, error: true },
          { name: "browser_snapshot", text: failed },
        ],
        { subtype: "success", structured_output: { completed: false, summary: "The browser did not start." } },
      ),
    );
    expect(parsed.browserError).toBe(
      "### Error Error: async initializeServer: Target page, context or browser has been closed Browser logs:",
    );
    expect(parsed.path).toEqual([]);
  });

  it("reports a session without a browser tool call as one that never got a page", () => {
    const parsed = parseTranscript(transcript([], { subtype: "success", structured_output: { completed: false } }));
    expect(parsed.browserError).toBe("the agent called no browser tool");
  });

  it("has no browser error once a call returned a page, whatever failed before or after", () => {
    const parsed = parseTranscript(
      transcript(
        [
          { name: "browser_click", input: { ref: "e9" }, text: "### Error\nRef e9 not found", error: true },
          { name: "browser_snapshot", text: settings },
          { name: "browser_click", input: { ref: "e7" }, text: "### Error\nTimeout", error: true },
        ],
        { subtype: "success", structured_output: { completed: false, summary: "" } },
      ),
    );
    expect(parsed.browserError).toBeNull();
  });

  it("reads a verdict at the end of the result text", () => {
    const parsed = parseTranscript(
      transcript([], { subtype: "success", result: 'Done.\n{"completed": true, "summary": "ok"}' }),
    );
    expect(parsed).toMatchObject({ completed: true, summary: "ok" });
  });
});

describe("failureOf", () => {
  it("fails a session whose browser never gave the agent a page, or that ended with an API error", () => {
    const noPage = parseTranscript(
      transcript([{ name: "browser_snapshot", text: "### Error\nChromium sandboxing failed!", error: true }], {
        subtype: "success",
        structured_output: { completed: false, summary: "" },
      }),
    );
    expect(failureOf(noPage)).toBe("the agent never got a page: ### Error Chromium sandboxing failed!");
    const api = parseTranscript(transcript([], { subtype: "success", is_error: true, result: "API Error: 401" }));
    expect(failureOf(api)).toBe("agent run failed: API Error: 401");
  });

  it("measures a session that got a page, done or not", () => {
    const parsed = parseTranscript(
      transcript([{ name: "browser_snapshot", text: settings }], {
        subtype: "error_max_turns",
        structured_output: { completed: false, summary: "" },
      }),
    );
    expect(failureOf(parsed)).toBeNull();
  });
});

describe("snapshotRefs", () => {
  it("names a control by its role and accessible name", () => {
    expect(snapshotRefs('- heading "A \\"B\\"" [level=1] [ref=e2]\n  - generic [ref=e3]:').get("e2")).toBe(
      'heading "A \\"B\\""',
    );
    expect(snapshotRefs("  - generic [ref=e3]:").get("e3")).toBe("generic");
  });
});

describe("measure", () => {
  const step = (url: string, target: string, action = "browser_click"): Step => ({ action, target, url });

  it("is 0 on the shortest path", () => {
    expect(measure([step("/settings/badges", "link B"), step("/settings/badges", "switch H")], 2, "/settings")).toEqual(
      {
        steps: 2,
        distinct: 2,
        backtracks: 0,
        lostness: 0,
      },
    );
  });

  it("grows with detours and counts the returns", () => {
    const path = [
      step("/settings/profile", "link P"),
      step("/settings", "", "browser_navigate_back"),
      step("/settings/badges", "link B"),
      step("/settings/profile", "link P"),
      step("/settings/badges", "link B"),
      step("/settings/badges", "switch H"),
    ];
    // S = 6, N = 4, R = 2: √((4/6 − 1)² + (2/4 − 1)²) = 0.601.
    expect(measure(path, 2, "/settings")).toEqual({ steps: 6, distinct: 4, backtracks: 3, lostness: 0.601 });
  });

  it("has no lostness without a step", () => {
    expect(measure([], 2, "/settings").lostness).toBeNull();
  });
});
