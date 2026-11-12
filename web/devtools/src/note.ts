// SPDX-License-Identifier: AGPL-3.0-only

// The shapes of a batch of notes (ADR-0168 §2), shared by the overlay and the dev server. The dev server writes each
// batch to .tmp/feedback/<id>.json, which `scripts/agentkit.sh feedback` reads and answers.

/** The path prefix of the dev server's endpoints. */
export const endpoint = "/__my-devtools";

/** The custom event of Vite's HMR channel that tells the overlay a batch file changed. */
export const updateEvent = "my-devtools:update";

/** The DOM event that switches an <Options> to a variant: detail {id, variant}. */
export const switchEvent = "my-devtools:switch";

/** The DOM event an <Options> dispatches when it mounts or switches, for the overlay to read the options again. */
export const optionsEvent = "my-devtools:options";

/** The attribute that marks the element of an <Options>, holding its id. */
export const optionsAttribute = "data-my-devtools-options";

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

interface Target {
  /** The element the note is on: for an area, the element at its centre. */
  tag: string;
  selector: string;
  text: string;
  /** The element's box, in page coordinates. */
  rect: Rect;
  /** The area the decider dragged over, in page coordinates, or null for a click on an element. */
  area: Rect | null;
}

export interface State {
  /** The URL path, search and hash of the page. */
  page: string;
  /** The route of the router; null while the page has no router (the catalog page). */
  route: string | null;
  viewport: { width: number; height: number; devicePixelRatio: number };
  theme: { page: string | null; system: "light" | "dark" };
  locale: string;
  direction: string;
  orientation: string | null;
  /** The signed-in account; null on a page without accounts. */
  account: string | null;
}

interface Reproduction {
  /** The E2E scenario and its step (ADR-0015 §5); null on a page that no scenario opens. */
  scenario: string | null;
  step: string | null;
}

interface Reply {
  kind: "fixed" | "question" | "options";
  text: string;
  /** For options, the id of the <Options> the agent placed on the page. */
  options?: string;
  at: string;
}

/** The variant the decider picked of an <Options>. */
interface Pick {
  options: string;
  variant: string;
  variants: string[];
}

/** What the overlay sends for one note; the dev server adds the id and the layer. */
export interface NoteInput {
  text: string;
  target: Target;
  /** The `file:line` of the element's JSX (data-source), or null when no development build placed one. */
  source: string | null;
  /** For an area, the sources of the elements in it. */
  sources: string[];
  state: State;
  reproduction: Reproduction;
  /** An outline of the accessibility tree of the element or area, one node per line. */
  accessibility: string;
  /** On a note that picks a variant, the pick. */
  pick?: Pick;
}

export interface Note extends NoteInput {
  id: string;
  /** The package of the source file: the layer the fix belongs to (ADR-0025 §2). */
  layer: string | null;
  reply?: Reply;
}

export interface Batch {
  id: string;
  sentAt: string;
  page: string;
  notes: Note[];
  /** When `agentkit feedback wait` handed the batch to an agent. */
  delivered?: string;
}

export interface BatchInput {
  page: string;
  notes: NoteInput[];
}
