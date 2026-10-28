// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { measure, parseTranscript, snapshotRefs, type Step } from "./transcript.ts";

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

  it("reads a verdict at the end of the result text", () => {
    const parsed = parseTranscript(
      transcript([], { subtype: "success", result: 'Done.\n{"completed": true, "summary": "ok"}' }),
    );
    expect(parsed).toMatchObject({ completed: true, summary: "ok" });
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
