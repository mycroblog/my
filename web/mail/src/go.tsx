// SPDX-License-Identifier: AGPL-3.0-only

// The Go html/template syntax that the layout keeps (ADR-0086 §2): React renders the layout once, at build time, and
// the server fills it for each email. A value is the text of its action, `{{.Subject}}`, in a text node or an
// attribute; an action that wraps parts (`{{with .Logo}}…{{end}}`) and one of the layout's own HTML comments, which
// html/template would drop, are markers that `toTemplate` turns into their actions. A marker is an empty <template>,
// which HTML allows anywhere, a table's body too, so React renders it where the action stands.
import type { ReactNode } from "react";

/** The action that writes the value at `path` of the template's data, such as `$.Site.Name` or `.`. */
export const value = (path: string): string => `{{${path}}}`;

/** The actions that write `text` when the value at `path` is not empty, and nothing otherwise. */
export const valueIf = (path: string, text: string): string => `{{if ${path}}}${text}{{end}}`;

/** An action's text, kept in the marker's attribute until `toTemplate` writes it out. */
const Marker = ({ action }: { action: string }) => <template data-go={action} />;

/**
 * The parts that `action` wraps, such as `with .Logo` or `range .Paragraphs`, and the parts of its `{{else}}`: its
 * dot is the value it names, and `$` the template's data.
 */
export function Go({ action, children, otherwise }: { action: string; children: ReactNode; otherwise?: ReactNode }) {
  return (
    <>
      <Marker action={action} />
      {children}
      {otherwise === undefined ? null : (
        <>
          <Marker action="else" />
          {otherwise}
        </>
      )}
      <Marker action="end" />
    </>
  );
}

/**
 * One of the layout's own HTML comments, written by the frame's `comment` function: Outlook's conditional comments.
 * Its text is a constant of the layout, never data, and holds no backquote.
 */
export function Comment({ text }: { text: string }) {
  if (text.includes("`")) throw new Error(`a comment of the layout holds a backquote: ${text}`);
  return <Marker action={`comment \`<!--${text}-->\``} />;
}

const entities: Record<string, string> = { "&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": '"', "&#x27;": "'" };
const unescape = (s: string) => s.replace(/&(?:amp|lt|gt|quot|#x27);/g, (e) => entities[e] ?? e);

/**
 * The Go template of the markup React rendered: each marker becomes its action, and each action keeps the characters
 * React escaped, which html/template reads as Go's syntax (a quoted string, a comparison).
 */
export function toTemplate(markup: string): string {
  return markup
    .replace(/\{\{[\s\S]*?\}\}/g, (action) => unescape(action))
    .replace(/<template data-go="([^"]*)"><\/template>/g, (_, action: string) => `{{${unescape(action)}}}`);
}
