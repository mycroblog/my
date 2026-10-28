// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { issueBody, issueTitle, lostByScreen, skippedSummary, skippedTasks } from "./report.ts";
import type { RunResult, TaskResult } from "./results.ts";

const base: TaskResult = {
  screen: "badges",
  screenTitle: "バッジとサポーターの一覧",
  task: "バッジを出す",
  shortest: 2,
  start: "/settings",
  status: "measured",
  completed: true,
  ending: "success",
  summary: "Turned the Helper badge on.",
  steps: 4,
  distinct: 3,
  backtracks: 1,
  lostness: 0.43,
  costUSD: 0.1,
  path: [
    { action: "browser_click", target: 'link "プロフィール"', url: "/settings/profile" },
    { action: "browser_navigate_back", target: "", url: "/settings" },
  ],
  mailPath: [],
};

const result: RunResult = {
  commit: "0123456789abcdef0123456789abcdef01234567",
  startedAt: "2026-10-28T18:00:00Z",
  dryRun: false,
  model: "sonnet",
  threshold: 0.4,
  selection: { mode: "all" },
  cap: 10,
  tasks: [
    base,
    { ...base, task: "外す", lostness: 0.4 },
    { ...base, screen: "settings-menu", screenTitle: "設定のメニュー", lostness: 0.2 },
    { ...base, screen: "settings-menu", screenTitle: "設定のメニュー", status: "unavailable", lostness: null },
  ],
};

describe("lostByScreen", () => {
  it("keeps the measured tasks over the threshold, by screen", () => {
    const lost = lostByScreen(result);
    expect([...lost.keys()]).toEqual(["badges"]);
    expect(lost.get("badges")?.map((task) => task.task)).toEqual(["バッジを出す"]);
  });
});

describe("not completed and unavailable tasks", () => {
  const supporters = { ...base, screen: "supporters", screenTitle: "サポーター" };
  const mixed: RunResult = {
    ...result,
    tasks: [
      { ...base, task: "ok", lostness: 0.1 },
      { ...supporters, task: "支える", completed: false, ending: "gave up", lostness: 0.1 },
      {
        ...supporters,
        task: "出来ない",
        status: "unavailable",
        completed: false,
        lostness: null,
        reason: "no supporter fixture",
      },
      { ...base, task: "落ちた", status: "error", completed: false, lostness: null },
    ],
  };

  it("files a task that ended without completion even under the threshold", () => {
    const lost = lostByScreen(mixed);
    expect([...lost.keys()]).toEqual(["supporters"]);
    expect(lost.get("supporters")?.map((task) => task.task)).toEqual(["支える"]);
  });

  it("marks it not completed with its path and shortest path", () => {
    const [task] = lostByScreen(mixed).get("supporters") ?? [];
    const body = issueBody(mixed, task === undefined ? [] : [task], "", "mycroblog/my");
    expect(body).toContain("### 支える (not completed)");
    expect(body).toContain("L1 shortest path: 2 step(s)");
    expect(body).toContain("Path taken:");
  });

  it("leaves a completed task over the threshold unmarked", () => {
    expect(issueBody(result, [base], "", "mycroblog/my")).not.toContain("(not completed)");
  });

  it("lists unavailable and failed tasks in the summary instead of issues", () => {
    expect(skippedSummary(skippedTasks(mixed))).toEqual([
      "- unavailable: supporters: 出来ない (no supporter fixture)",
      "- error: badges: 落ちた",
    ]);
  });
});

describe("issueTitle", () => {
  it("names the screen and its L1 file", () => {
    expect(issueTitle(base)).toBe("L4: lost on バッジとサポーターの一覧 (screens/badges.md)");
  });
});

describe("issueBody", () => {
  it("lists each task's measures, the path taken, and the L1 shortest path", () => {
    const body = issueBody(result, [base], "https://github.com/mycroblog/my/actions/runs/1", "mycroblog/my");
    expect(body).toContain("([run](https://github.com/mycroblog/my/actions/runs/1), artifact `l4-results`)");
    expect(body).toContain(
      "https://github.com/mycroblog/my/blob/0123456789abcdef0123456789abcdef01234567/docs/architecture/web-client/screens/badges.md#2-作業",
    );
    expect(body).toContain("### バッジを出す");
    expect(body).toContain(
      "Lostness: **0.43** (steps S = 4, distinct places N = 3, L1 shortest path R = 2), backtracks: 1",
    );
    expect(body).toContain('1. `click` link "プロフィール" → `/settings/profile`\n2. `navigate_back` → `/settings`');
    expect(body).not.toContain("mailbox");
  });

  it("lists the steps in the mailbox apart from the path", () => {
    const task: TaskResult = {
      ...base,
      path: [{ action: "mail", target: "", url: "/auth/reset_password" }],
      mailPath: [{ action: "browser_click", target: 'link "Reset"', url: "http://127.0.0.1:8025/view/1" }],
    };
    const body = issueBody(result, [task], "", "mycroblog/my");
    expect(body).toContain("1. `mail` → `/auth/reset_password`");
    expect(body).toContain(
      "Steps in the mailbox (Mailpit's web UI), each run counted as the one step `mail` above: 1\n\n" +
        '1. `click` link "Reset" → `http://127.0.0.1:8025/view/1`',
    );
  });
});
