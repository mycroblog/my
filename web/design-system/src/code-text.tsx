// SPDX-License-Identifier: AGPL-3.0-only

// Code as a reader follows it when it wraps (a payload, an endpoint, an IP address, a key): it breaks at a space or
// after a slash, a question mark, an ampersand, an equals sign, a pipe, a comma, or a colon that does not start a
// URL's `//` (`notification.email:01J9…`), and inside a run of other characters only
// where the run is longer than its line, never at a hyphen of a date or inside a word that fits. In a block, each line
// stands on its own, and a wrapped line's continuation hangs under its own line's indent, two characters further
// in, so that it never reads as a line of its own. The text is unchanged for copying and for assistive technology.

import type { CSSProperties, ReactNode } from "react";

/** A run between two places a line may break: the characters a reader keeps together, with the mark that ends it. */
const RUNS = /\s+|(?:[^\s/?&=|,:]|:(?=\/))*(?:[/?&=|,]|:(?!\/))+|(?:[^\s/?&=|,:]|:(?=\/))+/g;

/** A run kept whole on its line, unless it is longer than the line, where it breaks inside. */
function CodeRun({ children }: { children: string }) {
  return <span className="inline-block max-w-full indent-0 align-top wrap-anywhere">{children}</span>;
}

/** A line of a block, its continuation hung `indent` characters in, then the hang. */
function CodeLine({ indent, children }: { indent: number; children: ReactNode }) {
  return (
    <span
      className="block ps-[calc(var(--code-indent)*1ch+var(--content-code-hang))] -indent-[var(--content-code-hang)]"
      style={{ "--code-indent": indent } as CSSProperties}
    >
      {children}
    </span>
  );
}

/** `text` in runs, each kept whole where it fits. */
export function codeRuns(text: string): ReactNode {
  const runs = text.match(RUNS) ?? [];
  // A text with no place to break stays as it is: like every text, it breaks inside only where it is longer than its
  // line.
  if (runs.length <= 1) return text;
  return runs.map((run, i) => (/^\s+$/.test(run) ? run : <CodeRun key={i}>{run}</CodeRun>));
}

/**
 * A value in the code's face among words (a job's type, a route, a version): isolated left to right, as code reads in
 * a right-to-left page too, and broken as code is, after a slash or a colon, never inside a word that fits.
 */
export function CodeText({ children }: { children: string }) {
  return (
    <bdi dir="ltr" className="font-mono">
      {codeRuns(children)}
    </bdi>
  );
}

/** A block of code, line by line, each wrapped line hung under its own indent. */
export function CodeLines({ code }: { code: string }) {
  return code.split("\n").map((line, i) => {
    const expanded = line.replace(/\t/g, "  ");
    const text = expanded.trimStart();
    // An empty line keeps its height.
    return (
      <CodeLine key={i} indent={expanded.length - text.length}>
        {text === "" ? " " : codeRuns(text)}
      </CodeLine>
    );
  });
}
