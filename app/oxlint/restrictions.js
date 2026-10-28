// SPDX-License-Identifier: AGPL-3.0-only
// The Oxlint JS plugin `restrict` (ADR-0049): reports every node that matches one of the esquery selectors
// given as the rule's options, with that selector's message, like ESLint's no-restricted-syntax (which
// Oxlint lacks). The selectors are the project-specific restrictions of docs/architecture/web-client/README.md,
// written in app/.oxlintrc.json per path; restrictions.test.js proves that each one rejects a violation
// and accepts the intended alternative.
//
// The nine rules share one implementation and differ only in where .oxlintrc.json enables them, so that
// a path can drop one set of selectors (an override replaces a rule's options) without repeating the others:
// - syntax: every client source file.
// - svg: every client source file but the few that may draw SVG (G10).
// - feature: a feature's hooks and components.
// - pattern: a feature's components: build the screen from the pattern components of @my/ui (G5).
// - icons: every component of the kit and the client but stories and tests: an icon-only button has a tooltip (G12).
// - test: test files.
// - storage: every client source file but lib/storage and the tests (S11).
// - html: every client source file but PostContent and the tests: the Trusted Types sinks (A8).
// - classes: every file of web/src, tests included: no className and no class-composing helper (A4).

/** @typedef {{ selector: string, message: string }} Restriction */

const schema = {
  type: "array",
  items: {
    type: "object",
    properties: { selector: { type: "string" }, message: { type: "string" } },
    required: ["selector", "message"],
    additionalProperties: false,
  },
};

const rule = {
  meta: {
    type: "problem",
    docs: { description: "Disallow the syntax that the given esquery selectors match" },
    schema,
  },
  create(context) {
    /** @type {Record<string, (node: object) => void>} */
    const visitors = {};
    for (const { selector, message } of /** @type {Restriction[]} */ (context.options)) {
      const previous = visitors[selector];
      visitors[selector] = (node) => {
        previous?.(node);
        context.report({ node, message });
      };
    }
    return visitors;
  },
};

export default {
  meta: { name: "restrict" },
  rules: {
    syntax: rule,
    svg: rule,
    feature: rule,
    pattern: rule,
    icons: rule,
    test: rule,
    storage: rule,
    html: rule,
    classes: rule,
  },
};
