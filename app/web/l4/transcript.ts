// SPDX-License-Identifier: AGPL-3.0-only
// Measures one L4 task (ADR-0110 §7) from the transcript of the agent: the stream-json output of
// `claude -p --output-format stream-json --verbose`, one JSON event per line, whose tool calls drive the browser
// through the Playwright MCP server. The steps are the calls that act on the page; the measures come from them, never
// from what the agent says about itself.
//
// A step visits a place: the page's address after the step (path and query) and the control it acted on, by its role
// and accessible name in the last ARIA snapshot (`link "Badges"`), so that toggling a switch is a place of its own
// although the address stays. Lostness (Smith, 1996) is √((N/S − 1)² + (R/N − 1)²) with S the steps, N the distinct
// places and R the steps of the L1 shortest path: 0 when the agent took the shortest path, above 0.4 when it got lost.
// A backtrack is a step that goes back in the history, or one that returns to an address visited before other than
// the one it left.

/** The prefix of the tools of the Playwright MCP server, named `playwright` in the MCP configuration of run.ts. */
export const mcpPrefix = "mcp__playwright__";

/** The tools of the Playwright MCP server that act on the page: each successful call is a step. */
export const stepTools: readonly string[] = [
  "browser_click",
  "browser_type",
  "browser_fill_form",
  "browser_select_option",
  "browser_press_key",
  "browser_navigate_back",
  "browser_drag",
  "browser_handle_dialog",
];

/** The tools that only read the page or wait, which the agent may call without taking a step. */
export const readTools: readonly string[] = ["browser_snapshot", "browser_wait_for", "browser_hover"];

/** One step of the path the agent took. */
export type Step = {
  /** The tool without the MCP prefix, as `browser_click`. */
  action: string;
  /** The control it acted on, as `switch "Admin"`, or the key it pressed; empty for going back. */
  target: string;
  /** The path and query of the page after the step. */
  url: string;
};

/** The measures of one task. */
export type Measures = {
  /** S: the steps. */
  steps: number;
  /** N: the distinct places of the steps. */
  distinct: number;
  /** The steps that went back to an earlier page. */
  backtracks: number;
  /** Lostness, rounded to 3 decimals; null when the agent took no step. */
  lostness: number | null;
};

/** What the transcript of one task tells. */
export type Transcript = {
  path: Step[];
  /** Whether the agent finished with the task done, by its structured output. */
  completed: boolean;
  /** The agent's summary of what it did or why it stopped. */
  summary: string;
  /** How the session ended: the `subtype` of its result event (`success`, `error_max_turns`, ...). */
  ending: string;
  /** The message of an API, authentication or transport error that ended the session (`is_error` of the result event). */
  apiError: string | null;
  /**
   * Why the agent never got a page: no browser tool call returned one (the browser failed to start, say). Then the
   * session measured nothing of the screen. The first browser error, or that the agent called no browser tool.
   */
  browserError: string | null;
  /** The cost of the session in US dollars, when the result event gives it. */
  costUSD: number | null;
};

type ToolUse = { name: string; input: Record<string, unknown> };

/** The text of a tool result's content, which is a string or a list of content blocks. */
function resultText(content: unknown): string {
  if (typeof content === "string") {
    return content;
  }
  if (!Array.isArray(content)) {
    return "";
  }
  return content
    .map((block: unknown) =>
      typeof block === "object" && block !== null && "text" in block && typeof block.text === "string"
        ? block.text
        : "",
    )
    .join("\n");
}

/** The path and query of an absolute URL, or the URL as it is when it is not one. */
function pathOf(url: string): string {
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:" ? parsed.pathname + parsed.search : url;
  } catch {
    return url;
  }
}

/** The controls of an ARIA snapshot by their ref: `- switch "Admin" [checked] [ref=e12]` gives e12 → `switch "Admin"`. */
export function snapshotRefs(snapshot: string): Map<string, string> {
  const refs = new Map<string, string>();
  for (const match of snapshot.matchAll(/^\s*- ([a-z][\w-]*)(?: "((?:[^"\\]|\\.)*)")?[^\n]*\[ref=([\w-]+)\]/gm)) {
    const [, role = "", name, ref = ""] = match;
    refs.set(ref, name === undefined ? role : `${role} "${name}"`);
  }
  return refs;
}

/** The target of a step: the controls it acted on, by the last snapshot, or what the agent called them. */
function targetOf(use: ToolUse, refs: ReadonlyMap<string, string>): string {
  const control = (input: Record<string, unknown>): string => {
    const ref =
      typeof input["target"] === "string" ? input["target"] : typeof input["ref"] === "string" ? input["ref"] : "";
    const element =
      typeof input["element"] === "string" ? input["element"] : typeof input["name"] === "string" ? input["name"] : "";
    return refs.get(ref) ?? (element || ref);
  };
  switch (use.name) {
    case "browser_press_key":
      return `key ${String(use.input["key"])}`;
    case "browser_navigate_back":
      return "";
    case "browser_fill_form": {
      const fields = Array.isArray(use.input["fields"]) ? (use.input["fields"] as Record<string, unknown>[]) : [];
      return fields.map(control).join(", ");
    }
    case "browser_handle_dialog":
      return `dialog ${use.input["accept"] === true ? "accept" : "dismiss"}`;
    case "browser_drag":
      return `${control({ target: use.input["startTarget"] ?? use.input["startRef"], element: use.input["startElement"] })} → ${control({ target: use.input["endTarget"] ?? use.input["endRef"], element: use.input["endElement"] })}`;
    default:
      return control(use.input);
  }
}

/** Reads the transcript of one task from the stream-json `output` of Claude Code. */
export function parseTranscript(output: string): Transcript {
  const uses = new Map<string, ToolUse>();
  let refs = new Map<string, string>();
  let url = "";
  const path: Step[] = [];
  const transcript: Transcript = {
    path,
    completed: false,
    summary: "",
    ending: "none",
    apiError: null,
    browserError: null,
    costUSD: null,
  };
  let gotPage = false;
  let firstBrowserError: string | null = null;
  for (const line of output.split("\n")) {
    if (line.trim() === "") {
      continue;
    }
    let event: Record<string, unknown>;
    try {
      event = JSON.parse(line) as Record<string, unknown>;
    } catch {
      continue;
    }
    const message = event["message"] as { content?: unknown } | undefined;
    const blocks = Array.isArray(message?.content) ? (message.content as Record<string, unknown>[]) : [];
    if (event["type"] === "assistant") {
      for (const block of blocks) {
        if (block["type"] === "tool_use" && typeof block["id"] === "string" && typeof block["name"] === "string") {
          const name = block["name"].startsWith(mcpPrefix) ? block["name"].slice(mcpPrefix.length) : block["name"];
          uses.set(block["id"], { name, input: (block["input"] ?? {}) as Record<string, unknown> });
        }
      }
    } else if (event["type"] === "user") {
      for (const block of blocks) {
        if (block["type"] !== "tool_result" || typeof block["tool_use_id"] !== "string") {
          continue;
        }
        const use = uses.get(block["tool_use_id"]);
        if (use === undefined) {
          continue;
        }
        const text = resultText(block["content"]);
        const page = /- Page URL: (\S+)/.exec(text)?.[1];
        if (page !== undefined) {
          url = pathOf(page);
        }
        if (block["is_error"] !== true && page !== undefined) {
          gotPage = true;
        } else if (!gotPage && firstBrowserError === null && (block["is_error"] === true || /^### Error/m.test(text))) {
          firstBrowserError = text.trim().split("\n").slice(0, 3).join(" ").slice(0, 500);
        }
        if (use.name === "browser_snapshot") {
          refs = snapshotRefs(text);
        }
        if (block["is_error"] !== true && stepTools.includes(use.name)) {
          path.push({ action: use.name, target: targetOf(use, refs), url });
        }
      }
    } else if (event["type"] === "result") {
      transcript.ending = typeof event["subtype"] === "string" ? event["subtype"] : "unknown";
      transcript.costUSD = typeof event["total_cost_usd"] === "number" ? event["total_cost_usd"] : null;
      const verdict = structuredVerdict(event);
      transcript.completed = transcript.ending === "success" && verdict.completed;
      transcript.summary = verdict.summary;
      if (event["is_error"] === true && transcript.ending === "success") {
        // Claude Code reports a failed API call (a 401 of the token exchange, say) as a successful session with is_error.
        const message = typeof event["result"] === "string" ? event["result"] : verdict.summary;
        transcript.apiError = message === "" ? "the session ended with an error" : message;
        transcript.completed = false;
      }
    }
  }
  if (!gotPage) {
    transcript.browserError = firstBrowserError ?? "the agent called no browser tool";
  }
  return transcript;
}

/**
 * Why the session measured nothing of the screen, or null when it did. run.ts records such a task as `error`, which
 * files no issue and makes the run exit non-zero: an API error, or a browser that never gave the agent a page.
 */
export function failureOf(transcript: Transcript): string | null {
  if (transcript.apiError !== null) {
    return `agent run failed: ${transcript.apiError}`;
  }
  if (transcript.browserError !== null) {
    return `the agent never got a page: ${transcript.browserError}`;
  }
  return null;
}

/** The agent's verdict: the structured output of the result event, or a JSON object at the end of its text. */
function structuredVerdict(event: Record<string, unknown>): { completed: boolean; summary: string } {
  let value: unknown = event["structured_output"];
  if (value === undefined && typeof event["result"] === "string") {
    const json = /\{[\s\S]*\}\s*$/.exec(event["result"])?.[0];
    try {
      value = json === undefined ? undefined : JSON.parse(json);
    } catch {
      value = undefined;
    }
  }
  if (typeof value !== "object" || value === null) {
    return { completed: false, summary: typeof event["result"] === "string" ? event["result"] : "" };
  }
  const record = value as Record<string, unknown>;
  return {
    completed: record["completed"] === true,
    summary: typeof record["summary"] === "string" ? record["summary"] : "",
  };
}

/** Measures `path` against the `shortest` path of the L1, starting at the address `start`. */
export function measure(path: readonly Step[], shortest: number, start: string): Measures {
  const steps = path.length;
  const distinct = new Set(path.map((step) => `${step.url}\n${step.target}`)).size;
  let backtracks = 0;
  const seen = new Set([start]);
  let previous = start;
  for (const step of path) {
    if (step.action === "browser_navigate_back" || (step.url !== previous && seen.has(step.url))) {
      backtracks++;
    }
    seen.add(step.url);
    previous = step.url;
  }
  if (steps === 0) {
    return { steps, distinct, backtracks, lostness: null };
  }
  const lostness = Math.sqrt((distinct / steps - 1) ** 2 + (shortest / distinct - 1) ** 2);
  return { steps, distinct, backtracks, lostness: Math.round(lostness * 1000) / 1000 };
}
