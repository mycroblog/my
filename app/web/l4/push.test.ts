// SPDX-License-Identifier: AGPL-3.0-only
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Script } from "node:vm";
import { describe, expect, it } from "vitest";
import { mcpCommand } from "./mcp.ts";
import { postconditionCode, readPushState, turnPushOnCode } from "./push.ts";

describe("the init page of a push task", () => {
  it("installs the shim and grants notifications only for a push task", () => {
    const dir = mkdtempSync(join(tmpdir(), "l4-push-"));
    const base = { dir, startURL: "http://localhost:8787/settings", executablePath: "/chromium", sandbox: true };
    const plain = mcpCommand(base);
    expect(plain).not.toContain("--grant-permissions");
    expect(readFileSync(join(dir, "init-page.cjs"), "utf8")).not.toContain("__e2ePush");

    const state = join(dir, "push-subscription.json");
    const args = mcpCommand({ ...base, push: { origin: "https://localhost:8930", ca: "/ca.pem", state } });
    expect(args.join(" ")).toContain("--grant-permissions notifications");
    const initPage = readFileSync(join(dir, "init-page.cjs"), "utf8");
    for (const binding of ["__e2ePushSubscribe", "__e2ePushGetSubscription", "__e2ePushUnsubscribe"]) {
      expect(initPage).toContain(binding);
    }
    expect(initPage).toContain("PushManager.prototype.subscribe");
    expect(initPage).toContain(JSON.stringify(state));
    // Playwright MCP loads it with require(): it must be plain JavaScript.
    expect(() => new Script(`(function (exports, require) {${initPage}})`)).not.toThrow();
  });
});

describe("the code the harness runs in the browser", () => {
  it("is plain JavaScript for browser_run_code_unsafe", () => {
    for (const code of [
      turnPushOnCode("http://localhost:8787"),
      postconditionCode("http://localhost:8787", '{"type":"mention"}', "id-1"),
    ]) {
      expect(() => new Script(`(${code})`)).not.toThrow();
    }
    expect(turnPushOnCode("http://localhost:8787")).toContain('"http://localhost:8787/settings/notifications"');
  });
});

describe("readPushState", () => {
  it("reads the subscription of the state file, and none without the file", () => {
    const dir = mkdtempSync(join(tmpdir(), "l4-push-"));
    const file = join(dir, "push-subscription.json");
    expect(readPushState(file)).toBeUndefined();
    const state = { subscription: { endpoint: "https://localhost:8930/push/a" }, applicationServerKey: "k" };
    writeFileSync(file, JSON.stringify(state));
    expect(readPushState(file)).toEqual(state);
  });
});
