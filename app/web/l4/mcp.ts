// SPDX-License-Identifier: AGPL-3.0-only
// The Playwright MCP server the agent drives (ADR-0114): its command line, which gives the agent ARIA snapshots and
// no images, and a minimal MCP client over its standard input and output that the dry run uses to check, without
// the agent, that the server opens the start page signed in.
import { type ChildProcess, spawn } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";

/** The Playwright MCP server, pinned. Its browser is the Chromium of @playwright/test (`executablePath`). */
const playwrightMCP = "@playwright/mcp@0.0.83";

/** The arguments of `npx` that start the server for one task. */
export function mcpCommand(options: {
  /** The directory of the task: the init page and the server's output files go there. */
  dir: string;
  /** The absolute URL the browser opens before the agent's first call. */
  startURL: string;
  /** The storage state of the signed-in user (cookies and local storage). */
  storageState: string;
  /** The Chromium executable. */
  executablePath: string;
}): string[] {
  // The init page runs on every new page of the browser, before the first tool call returns: it opens the start,
  // so the agent never types an address, and waits until the client has drawn the screen's heading, so that the
  // first snapshot shows the screen rather than the loading state. Playwright MCP loads it with require().
  const initPage = join(options.dir, "init-page.cjs");
  writeFileSync(
    initPage,
    [
      "exports.default = async ({ page }) => {",
      `  await page.goto(${JSON.stringify(options.startURL)});`,
      '  await page.locator("h1").first().waitFor({ timeout: 30000 });',
      "};",
      "",
    ].join("\n"),
  );
  return [
    "-y",
    playwrightMCP,
    "--headless",
    "--isolated",
    "--storage-state",
    options.storageState,
    "--executable-path",
    options.executablePath,
    "--init-page",
    initPage,
    // ARIA snapshots only: no image ever reaches the agent.
    "--image-responses",
    "omit",
    "--viewport-size",
    "1280x900",
    "--output-dir",
    join(options.dir, "mcp-output"),
  ];
}

/** A client of one MCP server over stdio (newline-delimited JSON-RPC). */
export class MCPClient {
  private readonly process: ChildProcess;
  private readonly pending = new Map<number, (message: Record<string, unknown>) => void>();
  private nextID = 0;
  private buffer = "";

  constructor(command: string, args: readonly string[], cwd: string) {
    this.process = spawn(command, args, { cwd, stdio: ["pipe", "pipe", "inherit"] });
    this.process.stdout?.setEncoding("utf8");
    this.process.stdout?.on("data", (chunk: string) => {
      this.buffer += chunk;
      for (let end = this.buffer.indexOf("\n"); end >= 0; end = this.buffer.indexOf("\n")) {
        const line = this.buffer.slice(0, end);
        this.buffer = this.buffer.slice(end + 1);
        if (line.trim() === "") {
          continue;
        }
        const message = JSON.parse(line) as Record<string, unknown>;
        const resolve = typeof message["id"] === "number" ? this.pending.get(message["id"]) : undefined;
        if (resolve !== undefined) {
          this.pending.delete(message["id"] as number);
          resolve(message);
        }
      }
    });
  }

  private request(method: string, params: Record<string, unknown>): Promise<Record<string, unknown>> {
    const id = ++this.nextID;
    const reply = new Promise<Record<string, unknown>>((resolve) => this.pending.set(id, resolve));
    this.process.stdin?.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
    return reply;
  }

  /** Initializes the session. */
  async start(): Promise<void> {
    await this.request("initialize", {
      protocolVersion: "2025-06-18",
      capabilities: {},
      clientInfo: { name: "my-l4", version: "0" },
    });
    this.process.stdin?.write(`${JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" })}\n`);
  }

  /** The names of the server's tools. */
  async tools(): Promise<string[]> {
    const reply = await this.request("tools/list", {});
    const result = reply["result"] as { tools?: { name: string }[] } | undefined;
    return (result?.tools ?? []).map((tool) => tool.name);
  }

  /** Calls `name` and returns the text of its result; a tool error throws. */
  async call(name: string, args: Record<string, unknown> = {}): Promise<string> {
    const reply = await this.request("tools/call", { name, arguments: args });
    const result = reply["result"] as { content?: { type: string; text?: string }[]; isError?: boolean } | undefined;
    const content = result?.content ?? [];
    if (content.some((block) => block.type === "image")) {
      throw new Error(`${name} returned an image, which the agent must never get`);
    }
    const text = content.map((block) => block.text ?? "").join("\n");
    if (result?.isError === true || reply["error"] !== undefined) {
      throw new Error(`${name} failed: ${text || JSON.stringify(reply["error"])}`);
    }
    return text;
  }

  stop(): void {
    this.process.kill("SIGTERM");
  }
}
