// SPDX-License-Identifier: AGPL-3.0-only
// The rules of the Oxlint plugin `restrict` (restrictions.js) on what the styles of the web client may say, which
// make the static guarantees of the design system (ADR-0171 §2, §3) hold without rendering a screen. Each is proved
// by restrictions.test.js on the fixtures of the layers.
//
// - token-colors: a colour is a token (ADR-0171 §2). It refuses a colour written out in a string of a file: a hex
//   colour, a colour function with its own channels (rgb(), hsl(), hwb(), lab(), lch(), oklab(), oklch(), color()),
//   and a named colour where a colour is expected (a colour property of a style object, a colour attribute of an SVG
//   element, the arbitrary value of a colour utility). `transparent`, `currentColor` and a `color-mix()` of tokens
//   are not colours of their own. colors.js applies the same to the CSS files. It does not see a colour put together
//   at run time.
//
// - layout-parts: only a layout part writes layout (ADR-0171 §3). The line it draws:
//   - A part's own box is what the box is by itself, whatever holds it and whatever it holds: its padding and its
//     fixed size (`w-`, `h-`, `size-`, `min-w-`, `min-h-`), each from a token, with `box-border` and `aspect-`. Every
//     part may write these: a button has its height and its inline padding.
//   - Layout is what places a box in what holds it, places its children against each other, or says what happens
//     when they do not fit: the display of a box (block, flex, grid, inline-flex…) and the flex and grid utilities of
//     a container and its items, alignment (`items-`, `justify-`, `self-`, `place-`, `content-`, `align-`), `gap-` and
//     `space-`, margin, a size that follows the container or the content (`w-full`, `h-auto`, `min-w-0`, a fraction)
//     and every upper bound (`max-w-`, `max-h-`), position (`relative`, `absolute`, `fixed`, `sticky`, `inset-`,
//     `top-`…, `z-`, `isolate`), overflow (`overflow-`, `overscroll-`, `truncate`, `line-clamp-`), `float-` and
//     `clear-`, `columns-`, a container query (`@container`, and any utility under an `@…:` variant), and a padding or
//     a size that is not a token (an arbitrary value, a number). The same properties in a `style` object and in an
//     arbitrary property (`[display:flex]`) are layout; there a width or a height is never a token.
//   - Colour, type (with how a text wraps), radius, border, shadow, opacity, cursor, focus ring, transition and
//     transform are neither: every part writes them. `hidden` and `sr-only` show or hide, `contents` gives a part
//     no box, and none of them places.
//   A viewport breakpoint is the other rule's. The classes are the ones of a `tv()` declaration, by slot, and the
//   ones a declaration gives to an element: in a `className` (an attribute, a property of an object, an assignment
//   to one, the `classList`), through the constants and the variables these name, with what a variable is given
//   after its declaration (`c += " flex"`), and through the functions of the file they call, by what those return.
//   A slot may write layout when every component that reads it is a layout part of layout-parts.js, a `className` or
//   a `style` when its declaration is. The rule knows a component as a function at the top of its file and does not
//   read another file: so a class crosses from one declaration to another only under a name that says it is one. A
//   parameter or a prop that a declaration gives to an element as a class is named `className` or `…ClassName`, and
//   what its callers give under that name, in this file or another, is read where they write it.
//
// - viewport-breakpoints: only the frame of a page decides by the viewport (ADR-0025 §4). It refuses, outside the
//   components layout-parts.js names as page frames, a class under a breakpoint variant (`tablet:`, `max-desktop:`,
//   `min-[40rem]:`, `portrait:`, an arbitrary `@media` of a width or a height), Tailwind's `container`, a
//   `matchMedia` of the viewport's width, height, aspect ratio or orientation written as a literal, and a read of
//   `innerWidth`. The viewport's height read for geometry (`innerHeight`) is not a breakpoint.
//
// layout-parts and viewport-breakpoints come with a baseline (styles-baseline.json): how many times each declaration
// of a file still writes against the rule, which may only shrink. A declaration fails with every finding in it when
// it has more than its baseline allows, and a file fails when its baseline allows more than it has, so that the
// baseline stays what the code is.

import { readFileSync } from "node:fs";
import { rawColors, namedColor } from "./colors.js";
import { LAYOUT_PARTS, PAGE_FRAMES } from "./layout-parts.js";

/** @typedef {{ type: string, [key: string]: any }} Node */
/** @typedef {{ text: string, node: Node }} ClassText */
/** @typedef {{ node: Node, what: string, why: string }} Finding */

const BASELINE = "web/devtools/src/styles-baseline.json";
/** @type {Record<string, Record<string, Record<string, number>>>} */
const baseline = JSON.parse(readFileSync(new URL("./styles-baseline.json", import.meta.url), "utf8"));

/** The viewport breakpoints of the theme (the `breakpoint` of @my/design-tokens), and the ones Tailwind names itself. */
export const BREAKPOINTS = ["tablet", "desktop", "wide"];
const SCREENS = new Set([...BREAKPOINTS, "sm", "md", "lg", "xl", "2xl"]);

/** How many bindings a name is followed through before it is taken as unknown. */
const MAX_DEPTH = 8;

/**
 * The path of a file from the repository's root (`web/design-system/src/chip.tsx`), as layout-parts.js and the
 * baseline name it. A fixture that restrictions.test.js lints as a copy (`.fixtures-<pid>-<time>/`) has the name of
 * its original.
 */
function fileKey(/** @type {string} */ filename) {
  const path = filename.replaceAll("\\", "/");
  const at = path.lastIndexOf("/web/");
  return (at < 0 ? path : path.slice(at + 1)).replace(/\/\.fixtures-\d+-\d+\//, "/fixtures/");
}

/** The expression under TypeScript's casts and assertions (`as`, `satisfies`, `!`, `<T>`) and parentheses. */
function unwrap(/** @type {Node} */ node) {
  let current = node;
  while (
    current.type === "TSAsExpression" ||
    current.type === "TSSatisfiesExpression" ||
    current.type === "TSNonNullExpression" ||
    current.type === "TSTypeAssertion" ||
    current.type === "ParenthesizedExpression"
  )
    current = current.expression;
  return current;
}

/** The name of a property key, or undefined when it is computed from something other than a literal. */
function keyName(/** @type {Node} */ property) {
  const key = property.key;
  if (!property.computed && key.type === "Identifier") return key.name;
  if (key.type === "Literal" && (typeof key.value === "string" || typeof key.value === "number"))
    return String(key.value);
  return undefined;
}

/** The variants of a class and its utility without its `!` and its sign: `tablet:hover:-mt-2!` as tablet, hover; mt-2. */
export function parseClass(/** @type {string} */ token) {
  const variants = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < token.length; i += 1) {
    const c = token[i];
    if (c === "[" || c === "(") depth += 1;
    else if (c === "]" || c === ")") depth -= 1;
    else if (c === ":" && depth === 0) {
      variants.push(token.slice(start, i));
      start = i + 1;
    }
  }
  const utility = token.slice(start).replace(/^!/, "").replace(/!$/, "").replace(/^-/, "");
  return { variants, utility };
}

const DISPLAY =
  /^(?:block|inline-block|inline|flex|inline-flex|grid|inline-grid|flow-root|list-item|table|inline-table|table-(?:caption|cell|column|column-group|footer-group|header-group|row-group|row|auto|fixed))$/;
const PLACEMENT =
  /^(?:flex|basis|grow|shrink|order|grid-cols|grid-rows|grid-flow|auto-cols|auto-rows|col|row|justify|items|self|place-content|place-items|place-self|gap|gap-x|gap-y|space-x|space-y|columns|float|clear)-./;
const CONTENT = /^content-(?:normal|center|start|end|between|around|evenly|baseline|stretch)$/;
const ALIGN = /^align-(?:baseline|top|middle|bottom|text-top|text-bottom|sub|super|[[(])/;
const MARGIN = /^m[xytbselr]?-./;
const POSITION = /^(?:static|fixed|absolute|relative|sticky|isolate|isolation-auto)$/;
const INSET = /^(?:inset|inset-x|inset-y|inset-s|inset-e|inset-bs|inset-be|top|bottom|start|end|left|right|z)-./;
const OVERFLOW = /^(?:(?:overflow|overscroll)(?:-[xy])?-.|truncate$|line-clamp-.)/;
const SIZE = /^(w|h|size|min-w|min-h|max-w|max-h)-(.+)$/;
const PADDING = /^p[xytbselr]?-(.+)$/;
/** The values of a size that follow the container, the content or the viewport. */
const FOLLOWING = new Set([
  "full",
  "screen",
  "auto",
  "fit",
  "min",
  "max",
  "svw",
  "lvw",
  "dvw",
  "svh",
  "lvh",
  "dvh",
  "lh",
]);

/** The properties of a style object that write layout; a custom property and the box's own padding do not. */
const LAYOUT_STYLE =
  /^(?:display|position|top|bottom|left|right|inset\w*|zIndex|width|height|(?:min|max)(?:Width|Height|InlineSize|BlockSize)|inlineSize|blockSize|margin\w*|gap|rowGap|columnGap|flex\w*|grid\w*|order|align(?:Items|Self|Content)|justify(?:Content|Items|Self)|place(?:Items|Self|Content)|overflow(?:X|Y|Inline|Block)?|overscrollBehavior\w*|float|clear|verticalAlign)$/;

/** Whether the value of a padding or a size utility is a token of the theme's spacing (a name, or its 0). */
const isToken = (/** @type {string} */ value) =>
  value === "0" || (/^[a-z][a-z0-9-]*$/.test(value) && !FOLLOWING.has(value));

/** What kind of layout a class writes, or null for one that writes none or the part's own box. */
export function layoutOf(/** @type {string} */ token) {
  const { variants, utility } = parseClass(token);
  if (variants.some((v) => v.startsWith("@") || (v.startsWith("[") && v.includes("@container"))))
    return "a container query";
  if (/^@container(?:\/.+)?$/.test(utility)) return "a container to query";
  if (DISPLAY.test(utility)) return "the display of a box";
  if (utility === "grow" || utility === "shrink" || PLACEMENT.test(utility)) return "the placement of children";
  if (CONTENT.test(utility) || ALIGN.test(utility)) return "an alignment";
  if (MARGIN.test(utility)) return "a margin";
  if (POSITION.test(utility)) return "a position";
  if (INSET.test(utility) && !/^inset-(?:ring|shadow)/.test(utility)) return "a position";
  if (OVERFLOW.test(utility)) return "an overflow";
  const size = SIZE.exec(utility);
  if (size) {
    const [, property = "", value = ""] = size;
    if (property.startsWith("max-")) return "an upper bound of a size";
    if (FOLLOWING.has(value) || /^\d+\/\d+$/.test(value)) return "a size that follows its container or its content";
    if (property.startsWith("min-") && value === "0") return "a size that follows its container or its content";
    return isToken(value) ? null : "a size that is not a token";
  }
  const padding = PADDING.exec(utility);
  if (padding && !isToken(padding[1] ?? "")) return "a padding that is not a token";
  // An arbitrary property: [display:flex], [grid-template-columns:…].
  const property = /^\[([a-z-]+):/.exec(utility)?.[1] ?? "";
  if (LAYOUT_STYLE.test(property.replace(/-([a-z])/g, (_, c) => c.toUpperCase())))
    return "a property of the box's place or size";
  return null;
}

/** The viewport's features of a media query: its width, its height, their ratio and the orientation. */
const VIEWPORT_MEDIA = /\b(?:(?:min-|max-)?(?:device-)?(?:width|height|aspect-ratio)|orientation)\b/;

/** Whether a class decides by the viewport: a breakpoint variant, or Tailwind's `container`. */
export function viewportOf(/** @type {string} */ token) {
  const { variants, utility } = parseClass(token);
  if (utility === "container") return true;
  return variants.some((v) => {
    if (SCREENS.has(v) || v === "portrait" || v === "landscape") return true;
    const [, limit, bound = ""] = /^(min|max|not)-(.+)$/.exec(v) ?? [];
    if (limit !== undefined && (SCREENS.has(bound) || (limit !== "not" && bound.startsWith("[")))) return true;
    return v.startsWith("[") && v.includes("@media") && VIEWPORT_MEDIA.test(v);
  });
}

/** The JSX attributes that take classes: `className`, and a part's class under another name (`inputClassName`). */
const CLASS_ATTRIBUTE = /^(?:class|className|classNames)$|ClassName$/;

/** The name of the declaration at the top of the file that holds `node`: a component, a hook, a constant. */
function ownerOf(/** @type {Node} */ node) {
  let top = node;
  let declarator;
  while (top.parent && top.parent.type !== "Program") {
    if (top.type === "VariableDeclarator") declarator = top;
    top = top.parent;
  }
  if (top.type === "ExportNamedDeclaration" || top.type === "ExportDefaultDeclaration") top = top.declaration ?? top;
  if (top.type === "FunctionDeclaration" || top.type === "ClassDeclaration") return top.id?.name ?? "default";
  if (top.type === "VariableDeclaration" && declarator?.id.type === "Identifier") return declarator.id.name;
  return "(module)";
}

/**
 * What a file's declarations write, filled in as its visitors read the file: the classes of each slot of each `tv()`
 * with the declarations that read the slot, the classes and the `style` objects written in each declaration at the
 * top of the file, and those declarations by name.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function collect(context) {
  const { sourceCode } = context;

  /** The variable that `name` refers to at `node`. */
  function resolve(/** @type {Node} */ node, /** @type {string} */ name) {
    for (let scope = sourceCode.getScope(node); scope; scope = scope.upper) {
      const variable = scope.set.get(name);
      if (variable) return variable;
    }
    return undefined;
  }

  /** The declarator of the `tv()` that the identifier `node` names, or undefined. */
  function tvOf(/** @type {Node} */ node) {
    if (node.type !== "Identifier") return undefined;
    const def = resolve(node, node.name)?.defs[0];
    return def?.type === "Variable" && isTV(def.node.init) ? def.node : undefined;
  }

  /** Whether `node` is a call of tailwind-variants' `tv`. */
  function isTV(/** @type {Node | null | undefined} */ node) {
    return node?.type === "CallExpression" && node.callee.type === "Identifier" && node.callee.name === "tv";
  }

  /** Whether the callee of a call reaches a `tv()`: `button(…)`, `button(…).base(…)`, `s.base(…)` of `s = button(…)`. */
  function callsTV(/** @type {Node} */ callee, /** @type {number} */ depth) {
    const expression = unwrap(callee);
    if (depth > MAX_DEPTH) return false;
    if (expression.type === "MemberExpression") return callsTV(expression.object, depth + 1);
    if (expression.type === "CallExpression") return callsTV(expression.callee, depth + 1);
    if (expression.type !== "Identifier") return false;
    if (tvOf(expression)) return true;
    const def = resolve(expression, expression.name)?.defs[0];
    return def?.type === "Variable" && def.node.init ? callsTV(def.node.init, depth + 1) : false;
  }

  /** The function a call's callee names, when it is one of this file: its declaration, or the function a constant holds. */
  function functionOf(/** @type {Node} */ callee) {
    const expression = unwrap(callee);
    if (expression.type !== "Identifier") return undefined;
    const def = resolve(expression, expression.name)?.defs[0];
    if (def?.type === "FunctionName") return def.node;
    const init = def?.type === "Variable" && def.node.init ? unwrap(def.node.init) : undefined;
    return init?.type === "ArrowFunctionExpression" || init?.type === "FunctionExpression" ? init : undefined;
  }

  /** The places, among a function's parameters, of those named as a class. */
  function classParameters(/** @type {Node} */ fn) {
    return fn.params.flatMap((/** @type {Node} */ parameter, /** @type {number} */ index) => {
      const id = parameter.type === "AssignmentPattern" ? parameter.left : parameter;
      return id.type === "Identifier" && CLASS_ATTRIBUTE.test(id.name) ? [index] : [];
    });
  }

  /** Whether a function stands at the top of the file: a component, or a function the components of the file call. */
  function atTop(/** @type {Node} */ fn) {
    let node = fn.parent;
    if (node?.type === "VariableDeclarator") node = node.parent?.parent;
    if (node?.type === "ExportNamedDeclaration" || node?.type === "ExportDefaultDeclaration") node = node.parent;
    return node?.type === "Program";
  }

  /**
   * The name a caller gives a parameter by: the key of a destructured prop (`{ panel: frame }` is `panel`), or the
   * parameter's own; undefined for the rest of the props, and for a parameter of a function inside another, which
   * takes what that one gives it.
   */
  function givenName(/** @type {any} */ def) {
    if (def?.type !== "Parameter" || !atTop(def.node)) return undefined;
    let binding = def.name;
    if (binding.parent?.type === "AssignmentPattern" && binding.parent.left === binding) binding = binding.parent;
    const holder = binding.parent;
    if (holder?.type === "RestElement") return undefined;
    if (holder?.type === "Property" && holder.value === binding) return keyName(holder);
    return def.name.name;
  }

  /** The parameters, by declaration, that reach an element as a class under a name that does not say so. */
  /** @type {Map<string, Finding[]>} */
  const misnamed = new Map();
  const reported = new Set();

  /** Holds a class that comes in through `name` to that name: `className`, or one that ends in `ClassName`. */
  function cameIn(/** @type {string | undefined} */ name, /** @type {Node} */ node, /** @type {string} */ owner) {
    if (name === undefined || CLASS_ATTRIBUTE.test(name) || reported.has(node)) return;
    reported.add(node);
    (misnamed.get(owner) ?? misnamed.set(owner, []).get(owner))?.push({
      node,
      what: name,
      why:
        `gives an element a class that comes in through \`${name}\`, a name that does not say so: the lint reads ` +
        "what a caller gives as a class by the name it gives it under, in this file and in every other. Name it " +
        "`className`, or a name that ends in `ClassName` (ADR-0171 §3).",
    });
  }

  /**
   * The class texts an expression may yield: each string and template written in it, through conditionals, arrays,
   * the arguments of a call, the constants and the variables it names with what they are given later, and what a
   * function of the file it calls returns. A call of a `tv()` yields the classes of its `class` option alone, since
   * its other options name variants. A parameter it reaches is held to its name (`cameIn`), in `owner`.
   * @param {Node | null | undefined} node
   * @param {ClassText[]} out
   * @param {Set<Node>} seen
   * @param {string} owner
   */
  function classTexts(node, out, seen, owner, depth = 0) {
    if (!node || depth > MAX_DEPTH * 4 || seen.has(node)) return out;
    seen.add(node);
    const expression = unwrap(node);
    const next = (/** @type {Node | null | undefined} */ n) => classTexts(n, out, seen, owner, depth + 1);
    switch (expression.type) {
      case "Literal":
        if (typeof expression.value === "string") out.push({ text: expression.value, node: expression });
        break;
      case "TemplateLiteral":
        out.push({
          text: expression.quasis.map((/** @type {Node} */ q) => q.value.cooked ?? "").join("\0"),
          node: expression,
        });
        // What stands alone between spaces is a class of its own; what is written into a class is a part of it.
        expression.expressions.forEach((/** @type {Node} */ part, /** @type {number} */ i) => {
          const before = expression.quasis[i]?.value.cooked ?? "";
          const after = expression.quasis[i + 1]?.value.cooked ?? "";
          if (/(?:^|\s)$/.test(before) && /^(?:\s|$)/.test(after)) next(part);
        });
        break;
      case "ArrayExpression":
        expression.elements.forEach(next);
        break;
      case "SpreadElement":
        next(expression.argument);
        break;
      case "ConditionalExpression":
        next(expression.consequent);
        next(expression.alternate);
        break;
      case "LogicalExpression":
        next(expression.left);
        next(expression.right);
        break;
      case "BinaryExpression":
        if (expression.operator === "+") {
          next(expression.left);
          next(expression.right);
        }
        break;
      case "ObjectExpression":
        for (const property of expression.properties) next(property.type === "Property" ? property.value : property);
        break;
      case "MemberExpression": {
        const property = expression.property;
        const name = !expression.computed
          ? property.name
          : property.type === "Literal" && typeof property.value === "string"
            ? property.value
            : undefined;
        // A class under a class's name is its giver's to write, and read where it is written.
        if (name !== undefined && CLASS_ATTRIBUTE.test(name)) break;
        const object = unwrap(expression.object);
        const def = object.type === "Identifier" ? resolve(object, object.name)?.defs[0] : undefined;
        if (name !== undefined && givenName(def) !== undefined) cameIn(name, expression, owner);
        else next(expression.object);
        break;
      }
      case "CallExpression":
        if (callsTV(expression.callee, 0)) {
          for (const argument of expression.arguments) {
            const options = unwrap(argument);
            if (options.type !== "ObjectExpression") continue;
            for (const property of options.properties)
              if (property.type === "Property" && /^class(?:Name)?$/.test(keyName(property) ?? ""))
                next(property.value);
          }
        } else {
          const fn = functionOf(expression.callee);
          if (fn === undefined) {
            if (expression.callee.type === "MemberExpression") next(expression.callee.object);
            expression.arguments.forEach(next);
          } else {
            // A function of the file: what it returns, and what it is given under a class's name.
            for (const index of classParameters(fn)) next(expression.arguments[index]);
            if (fn.type === "FunctionDeclaration") returned(fn.body, next);
            else next(fn);
          }
        }
        break;
      case "ArrowFunctionExpression":
      case "FunctionExpression":
        if (expression.body.type === "BlockStatement") returned(expression.body, next);
        else next(expression.body);
        break;
      case "Identifier": {
        const variable = resolve(expression, expression.name);
        const def = variable?.defs[0];
        if (def?.type === "Parameter") cameIn(givenName(def), expression, owner);
        else if (def?.type === "Variable" && def.node.id.type === "Identifier" && !isTV(def.node.init)) {
          if (seen.has(def.node)) break;
          seen.add(def.node);
          next(def.node.init);
          // What the variable is given after its declaration: `c += " flex"`.
          for (const reference of variable.references) if (reference.writeExpr) next(reference.writeExpr);
        }
        break;
      }
      default:
        break;
    }
    return out;
  }

  /** Gives `next` what each `return` of a block returns, in its branches too, not in a function inside it. */
  function returned(/** @type {Node} */ statement, /** @type {(n: Node) => unknown} */ next) {
    switch (statement.type) {
      case "ReturnStatement":
        if (statement.argument) next(statement.argument);
        break;
      case "BlockStatement":
        for (const s of statement.body) returned(s, next);
        break;
      case "IfStatement":
        returned(statement.consequent, next);
        if (statement.alternate) returned(statement.alternate, next);
        break;
      case "SwitchStatement":
        for (const c of statement.cases) for (const s of c.consequent) returned(s, next);
        break;
      default:
        break;
    }
  }

  /** The slots of a `tv()` and the class texts of each: `base` is the slot of one without slots. */
  function slotsOf(/** @type {Node} */ call) {
    /** @type {Map<string, ClassText[]>} */
    const slots = new Map();
    const seen = new Set();
    const add = (/** @type {string} */ slot, /** @type {Node} */ value) => {
      classTexts(value, slots.get(slot) ?? slots.set(slot, []).get(slot), seen, ownerOf(call));
    };
    /** A value that is the classes of the base, or an object of the classes of each slot. */
    const addValue = (/** @type {Node} */ value, /** @type {string[]} */ fallback) => {
      const v = unwrap(value);
      if (v.type === "ObjectExpression") {
        for (const p of v.properties) if (p.type === "Property") add(keyName(p) ?? "base", p.value);
      } else for (const slot of fallback) add(slot, v);
    };
    const config = call.arguments[0] && unwrap(call.arguments[0]);
    if (config?.type !== "ObjectExpression") return slots;
    for (const property of config.properties) {
      if (property.type !== "Property") continue;
      const name = keyName(property);
      const value = unwrap(property.value);
      if (name === "base") add("base", value);
      else if (name === "slots" && value.type === "ObjectExpression") {
        for (const p of value.properties) if (p.type === "Property") add(keyName(p) ?? "base", p.value);
      } else if (name === "variants" && value.type === "ObjectExpression") {
        for (const variant of value.properties) {
          const values = variant.type === "Property" ? unwrap(variant.value) : undefined;
          if (values?.type !== "ObjectExpression") continue;
          for (const p of values.properties) if (p.type === "Property") addValue(p.value, ["base"]);
        }
      } else if ((name === "compoundVariants" || name === "compoundSlots") && value.type === "ArrayExpression") {
        for (const element of value.elements) {
          const compound = element && unwrap(element);
          if (compound?.type !== "ObjectExpression") continue;
          /** @type {string[]} */
          let named = ["base"];
          for (const p of compound.properties) {
            const list = p.type === "Property" && keyName(p) === "slots" ? unwrap(p.value) : undefined;
            if (list?.type === "ArrayExpression")
              named = list.elements.flatMap((/** @type {Node} */ e) => (typeof e.value === "string" ? [e.value] : []));
          }
          for (const p of compound.properties)
            if (p.type === "Property" && /^class(?:Name)?$/.test(keyName(p) ?? "")) addValue(p.value, named);
        }
      }
    }
    return slots;
  }

  /** The slots of the `tv()` that the reference `id` to it reads, or "*" for all of them. */
  function slotsRead(/** @type {Node} */ id, /** @type {boolean} */ hasSlots) {
    const call = id.parent;
    if (call?.type !== "CallExpression" || call.callee !== id) return "*";
    let user = call.parent;
    while (user && unwrap(user) !== user) user = user.parent;
    if (user?.type === "MemberExpression" && !user.computed && unwrap(user.object) === call)
      return [user.property.name];
    if (user?.type === "VariableDeclarator" && user.init && unwrap(user.init) === call) {
      if (user.id.type === "ObjectPattern") {
        const names = user.id.properties.map((/** @type {Node} */ p) =>
          p.type === "Property" ? keyName(p) : undefined,
        );
        return names.includes(undefined) ? "*" : names;
      }
      if (user.id.type === "Identifier") {
        const names = [];
        for (const reference of resolve(user.id, user.id.name)?.references ?? []) {
          const member = reference.identifier.parent;
          if (reference.identifier === user.id) continue;
          if (member?.type !== "MemberExpression" || member.object !== reference.identifier || member.computed)
            return "*";
          names.push(member.property.name);
        }
        return names;
      }
    }
    return hasSlots ? "*" : ["base"];
  }

  /** @type {{ name: string, node: Node, slots: Map<string, ClassText[]>, readers: Map<string, Set<string>> }[]} */
  const declarations = [];
  /** @type {Map<string, ClassText[]>} */
  const written = new Map();
  /** @type {Map<string, Node[]>} */
  const styled = new Map();
  /** @type {Map<string, Node>} */
  const owners = new Map();
  const seenWritten = new Set();

  /** The style objects an expression may be, through conditionals and the constants it names. */
  function styleObjects(/** @type {Node | null | undefined} */ node, /** @type {Node[]} */ out, depth = 0) {
    if (!node || depth > MAX_DEPTH) return out;
    const expression = unwrap(node);
    if (expression.type === "ObjectExpression") {
      out.push(expression);
      for (const p of expression.properties) if (p.type === "SpreadElement") styleObjects(p.argument, out, depth + 1);
    } else if (expression.type === "ConditionalExpression") {
      styleObjects(expression.consequent, out, depth + 1);
      styleObjects(expression.alternate, out, depth + 1);
    } else if (expression.type === "LogicalExpression") {
      styleObjects(expression.left, out, depth + 1);
      styleObjects(expression.right, out, depth + 1);
    } else if (expression.type === "Identifier") {
      const def = resolve(expression, expression.name)?.defs[0];
      if (def?.type === "Variable" && def.node.id.type === "Identifier") styleObjects(def.node.init, out, depth + 1);
    }
    return out;
  }

  /** Reads what a declaration gives an element as a class. */
  function given(/** @type {Node} */ at, /** @type {Node | null | undefined} */ value) {
    const owner = ownerOf(at);
    classTexts(value, written.get(owner) ?? written.set(owner, []).get(owner), seenWritten, owner);
  }

  /** Whether `node` is in the configuration or the options of a `tv()`, whose classes its declaration holds. */
  function inTV(/** @type {Node} */ node) {
    for (let parent = node.parent; parent; parent = parent.parent)
      if (parent.type === "CallExpression" && (isTV(parent) || callsTV(parent.callee, 0))) return true;
    return false;
  }

  /** The name of the member a member expression reads, written as a name or as a string. */
  const memberName = (/** @type {Node} */ member) =>
    !member.computed
      ? member.property.name
      : member.property.type === "Literal" && typeof member.property.value === "string"
        ? member.property.value
        : undefined;

  return {
    visitors: {
      FunctionDeclaration(/** @type {Node} */ node) {
        if (!owners.has(ownerOf(node))) owners.set(ownerOf(node), node);
      },
      // A class given outside an attribute: `{ className: … }` (an element's props, spread or cloned).
      Property(/** @type {Node} */ node) {
        if (node.parent?.type !== "ObjectExpression" || !CLASS_ATTRIBUTE.test(keyName(node) ?? "")) return;
        if (!inTV(node)) given(node, node.value);
      },
      // `props.className = …`, `element.className = …`.
      AssignmentExpression(/** @type {Node} */ node) {
        const left = unwrap(node.left);
        if (left.type === "MemberExpression" && CLASS_ATTRIBUTE.test(memberName(left) ?? "")) given(node, node.right);
      },
      CallExpression(/** @type {Node} */ node) {
        const callee = unwrap(node.callee);
        // `element.classList.add(…)`, `element.setAttribute("class", …)`.
        if (callee.type === "MemberExpression" && !callee.computed) {
          const object = unwrap(callee.object);
          if (object.type === "MemberExpression" && memberName(object) === "classList")
            for (const argument of node.arguments) given(node, argument);
          const [name, value] = node.arguments;
          if (callee.property.name === "setAttribute" && name?.type === "Literal" && name.value === "class")
            given(node, value);
        }
        // A function of the file that takes a class under its name: what each caller gives it.
        const fn = functionOf(node.callee);
        if (fn !== undefined) for (const index of classParameters(fn)) given(node, node.arguments[index]);
      },
      VariableDeclarator(/** @type {Node} */ node) {
        if (node.id.type !== "Identifier") return;
        if (ownerOf(node) === node.id.name) owners.set(node.id.name, node);
        if (!isTV(node.init)) return;
        const slots = slotsOf(node.init);
        const hasSlots = (node.init.arguments[0]?.properties ?? []).some(
          (/** @type {Node} */ p) => p.type === "Property" && keyName(p) === "slots",
        );
        /** @type {Map<string, Set<string>>} */
        const readers = new Map();
        for (const reference of resolve(node.id, node.id.name)?.references ?? []) {
          const id = reference.identifier;
          if (id === node.id) continue;
          const read = slotsRead(id, hasSlots);
          for (const slot of read === "*" ? slots.keys() : read)
            (readers.get(slot) ?? readers.set(slot, new Set()).get(slot))?.add(ownerOf(id));
        }
        declarations.push({ name: node.id.name, node, slots, readers });
      },
      JSXAttribute(/** @type {Node} */ node) {
        if (node.name.type !== "JSXIdentifier" || !node.value) return;
        const value = node.value.type === "JSXExpressionContainer" ? node.value.expression : node.value;
        const owner = ownerOf(node);
        if (CLASS_ATTRIBUTE.test(node.name.name)) given(node, value);
        else if (node.name.name === "style") styleObjects(value, styled.get(owner) ?? styled.set(owner, []).get(owner));
      },
    },
    declarations,
    written,
    styled,
    owners,
    misnamed,
  };
}

/** The classes of a class text, each with the dynamic parts of a template as `\0`. */
const classesOf = (/** @type {string} */ text) => text.split(/\s+/).filter((token) => token !== "" && token !== "\0");

/** A class as a message shows it: the dynamic part of a template as `${…}`. */
const shown = (/** @type {string} */ token) => token.replaceAll("\0", "${…}");

/**
 * Gives `each` every class that a declaration outside `allowed` writes, with the key of the declaration and who it
 * is: a class of a slot of a `tv()` unless every reader of the slot is allowed (its key is `<tv>.<slot>`), and a
 * class written in a declaration that is not (its key is the declaration's name).
 * @param {{ declarations: { name: string, slots: Map<string, ClassText[]>, readers: Map<string, Set<string>> }[], written: Map<string, ClassText[]> }} classes
 * @param {Set<string>} allowed
 * @param {string} role What an allowed declaration is: "layout part", "page frame".
 * @param {(key: string, node: Node, token: string, who: string) => void} each
 */
function refusedClasses({ declarations, written }, allowed, role, each) {
  for (const { name, slots, readers } of declarations) {
    for (const [slot, texts] of slots) {
      const read = [...(readers.get(slot) ?? [])];
      const others = read.filter((reader) => !allowed.has(reader));
      if (read.length > 0 && others.length === 0) continue;
      const who =
        read.length === 0
          ? "no component reads it"
          : `${others.join(", ")} ${others.length > 1 ? "are" : "is"} no ${role}`;
      for (const { text, node } of texts)
        for (const token of classesOf(text)) each(`${name}.${slot}`, node, token, who);
    }
  }
  for (const [owner, texts] of written) {
    if (allowed.has(owner)) continue;
    for (const { text, node } of texts)
      for (const token of classesOf(text)) each(owner, node, token, `${owner} is no ${role}`);
  }
}

/**
 * Reports the findings of each declaration against the baseline of the file: all of a declaration's when it has more
 * than its baseline allows, and the baseline itself, at the declaration, when it allows more than there is.
 * @param {any} context
 * @param {Node} program
 * @param {string} rule
 * @param {Map<string, Finding[]>} findings
 * @param {Map<string, Node>} owners
 */
function settle(context, program, rule, findings, owners) {
  const allowed = baseline[rule]?.[fileKey(String(context.filename))] ?? {};
  for (const [key, found] of findings) {
    const limit = allowed[key] ?? 0;
    if (found.length <= limit) continue;
    const over = limit > 0 ? ` \`${key}\` has ${found.length} of these, and its baseline allows ${limit}.` : "";
    for (const f of found) context.report({ node: f.node, message: `\`${f.what}\` in \`${key}\` ${f.why}${over}` });
  }
  for (const [key, limit] of Object.entries(allowed)) {
    const count = findings.get(key)?.length ?? 0;
    if (count < limit)
      context.report({
        node: owners.get(key.split(".")[0] ?? key) ?? top(program),
        message:
          `The baseline of restrict/${rule} allows \`${key}\` ${limit} and it has ${count}: the baseline only ` +
          "shrinks, and stays what the code is. " +
          (count === 0 ? `Remove the entry from ${BASELINE}.` : `Set it to ${count} in ${BASELINE}.`),
      });
  }
}

/** Where a finding about the whole file is reported: its first statement. */
const top = (/** @type {Node} */ program) => program.body[0] ?? program;

/** Reports a name of layout-parts.js that is no function or constant at the top of the file. */
function known(
  /** @type {any} */ context,
  /** @type {Node} */ program,
  /** @type {string[]} */ names,
  /** @type {Map<string, Node>} */ owners,
) {
  for (const name of names)
    if (!owners.has(name))
      context.report({
        node: top(program),
        message:
          `\`${name}\` is named in web/devtools/src/layout-parts.js and is no component of this file: ` +
          "remove it there, or name the component that took its place.",
      });
}

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function createLayoutParts(context) {
  const file = fileKey(String(context.filename));
  const parts = new Set(LAYOUT_PARTS[file] ?? []);
  const { visitors, declarations, written, styled, owners, misnamed } = collect(context);
  const why = (/** @type {string} */ kind, /** @type {string} */ readers) =>
    `writes ${kind}, which is layout, and ${readers}: only a part that just places its children writes layout ` +
    "(ADR-0171 §3). Build it from the layout parts of @my/design-system (web/devtools/src/layout-parts.js names " +
    "them), or put the placement in one; a part's own box takes a padding and a fixed size from tokens.";

  return {
    ...visitors,
    "Program:exit"(/** @type {Node} */ program) {
      /** @type {Map<string, Finding[]>} */
      const findings = new Map();
      const add = (/** @type {string} */ key, /** @type {Finding} */ finding) =>
        (findings.get(key) ?? findings.set(key, []).get(key))?.push(finding);
      refusedClasses({ declarations, written }, parts, "layout part", (key, node, token, who) => {
        const kind = layoutOf(token);
        if (kind !== null) add(key, { node, what: shown(token), why: why(kind, who) });
      });
      for (const [owner, objects] of styled) {
        if (parts.has(owner)) continue;
        for (const object of objects)
          for (const property of object.properties) {
            const name = property.type === "Property" ? keyName(property) : undefined;
            if (name !== undefined && LAYOUT_STYLE.test(name))
              add(owner, {
                node: property,
                what: `style.${name}`,
                why: why("a property of the box's place or size", `${owner} is no layout part`),
              });
          }
      }
      // A class that comes in under another name, in a layout part too: its callers are other declarations.
      for (const [owner, found] of misnamed) for (const finding of found) add(owner, finding);
      settle(context, program, "layout-parts", findings, owners);
      known(context, program, LAYOUT_PARTS[file] ?? [], owners);
    },
  };
}

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function createViewportBreakpoints(context) {
  const file = fileKey(String(context.filename));
  const frames = new Set(PAGE_FRAMES[file] ?? []);
  const { visitors, declarations, written, owners } = collect(context);
  const why = (/** @type {string} */ who) =>
    `decides by the viewport, and ${who}: only the frame of a page reads the breakpoints (ADR-0025 §4, ADR-0171 §3). ` +
    "Lay the part out by its container (a container query in a layout part), or let the frame decide and pass the " +
    "answer in as a prop.";
  /** @type {Map<string, Finding[]>} */
  const findings = new Map();
  const add = (/** @type {string} */ key, /** @type {Finding} */ finding) =>
    (findings.get(key) ?? findings.set(key, []).get(key))?.push(finding);

  /** The text of a media query written as a literal, or undefined for one computed at run time. */
  function queryText(/** @type {Node | undefined} */ node) {
    if (node === undefined) return undefined;
    const expression = unwrap(node);
    if (expression.type === "Literal" && typeof expression.value === "string") return expression.value;
    if (expression.type === "TemplateLiteral")
      return expression.quasis.map((/** @type {Node} */ q) => q.value.cooked).join(" ");
    return undefined;
  }

  /** The owner of `node` when it is no page frame, to report what it reads. */
  const outside = (/** @type {Node} */ node) => {
    const owner = ownerOf(node);
    return frames.has(owner) ? undefined : owner;
  };
  return {
    ...visitors,
    CallExpression(/** @type {Node} */ node) {
      visitors.CallExpression(node);
      const callee = unwrap(node.callee);
      const named =
        (callee.type === "Identifier" && callee.name === "matchMedia") ||
        (callee.type === "MemberExpression" && !callee.computed && callee.property.name === "matchMedia");
      const feature = named ? VIEWPORT_MEDIA.exec(queryText(node.arguments[0]) ?? "")?.[0] : undefined;
      const owner = feature === undefined ? undefined : outside(node);
      if (owner !== undefined)
        add(owner, { node, what: `matchMedia(${feature})`, why: why(`${owner} is no page frame`) });
    },
    MemberExpression(/** @type {Node} */ node) {
      if (node.computed || node.property.name !== "innerWidth") return;
      const owner = outside(node);
      if (owner !== undefined) add(owner, { node, what: "innerWidth", why: why(`${owner} is no page frame`) });
    },
    "Program:exit"(/** @type {Node} */ program) {
      refusedClasses({ declarations, written }, frames, "page frame", (key, node, token, who) => {
        if (viewportOf(token)) add(key, { node, what: shown(token), why: why(who) });
      });
      settle(context, program, "viewport-breakpoints", findings, owners);
      known(context, program, PAGE_FRAMES[file] ?? [], owners);
    },
  };
}

/** The properties of a style object, in camel case, and the attributes of an SVG element that take a colour. */
const COLOR_PROPERTY =
  /^(?:color|background|backgroundColor|border\w*|outline|outlineColor|fill|stroke|boxShadow|textShadow|caretColor|accentColor|textDecoration|textDecorationColor|columnRule\w*|stopColor|floodColor|lightingColor)$/;
const COLOR_ATTRIBUTE =
  /^(?:color|fill|stroke|stop-color|stopColor|flood-color|floodColor|lighting-color|lightingColor)$/;

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function createTokenColors(context) {
  function report(/** @type {Node} */ node, /** @type {string} */ color) {
    context.report({
      node,
      message:
        `\`${color}\` is a colour outside the tokens: a colour comes from @my/design-tokens, whose combinations are ` +
        "checked for contrast in both themes (ADR-0171 §2). Use a colour of the theme (`bg-surface`, " +
        "`text-muted-text`, `var(--color-text)`), `currentColor` or `transparent`, or add the token it lacks to " +
        "@my/design-tokens.",
    });
  }
  function check(/** @type {Node} */ node, /** @type {string} */ text) {
    for (const color of rawColors(text)) report(node, color);
    const parent = node.parent;
    const named =
      (parent?.type === "Property" && parent.value === node && COLOR_PROPERTY.test(keyName(parent) ?? "")) ||
      (parent?.type === "JSXAttribute" &&
        parent.name.type === "JSXIdentifier" &&
        COLOR_ATTRIBUTE.test(parent.name.name));
    if (named) for (const color of namedColor(text)) report(node, color);
  }
  return {
    Literal(/** @type {Node} */ node) {
      if (typeof node.value !== "string") return;
      const parent = node.parent;
      // The source of an import and the key of a property name no colour.
      if (parent?.type === "ImportDeclaration" || parent?.type === "ImportExpression") return;
      if (parent?.type === "Property" && parent.key === node && parent.value !== node) return;
      check(node, node.value);
    },
    TemplateElement(/** @type {Node} */ node) {
      check(node, node.value.cooked ?? "");
    },
  };
}

export const rules = {
  "token-colors": {
    meta: {
      type: "problem",
      docs: { description: "Disallow a colour that is not a token of @my/design-tokens" },
      schema: [],
    },
    create: createTokenColors,
  },
  "layout-parts": {
    meta: {
      type: "problem",
      docs: { description: "Disallow layout in a part of @my/design-system that is no layout part" },
      schema: [],
    },
    create: createLayoutParts,
  },
  "viewport-breakpoints": {
    meta: {
      type: "problem",
      docs: { description: "Disallow a decision by the viewport outside the frame of a page" },
      schema: [],
    },
    create: createViewportBreakpoints,
  },
};
