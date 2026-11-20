// SPDX-License-Identifier: AGPL-3.0-only
// The Oxlint JS plugin `restrict` (ADR-0025 §8): the layer rules of ADR-0025 §3 and §4 that a type cannot hold, enabled
// per path in .oxlintrc.json. restrictions.test.js lints the fixtures of web/devtools/fixtures/restrictions/ and of
// the layers' fixtures/ with Oxlint and compares its findings with the ones each fixture expects. The rules of the
// styles (token-colors, layout-parts, viewport-breakpoints) are in styles.js.
//
// - layer-imports: a layer imports only the layers below it (§3): design-tokens ← design-system ← app-components ←
//   app-features ← app, with app-data beside app-components, under app-features, and platform and rtc under app-data;
//   @my/i18n reaches the app alone. The layer of a file is the package of web/ it is in. It sees `import`, `export …
//   from` and `import()` of a package of the workspace by its name, and a relative path into another package of
//   web/ (web/brand, which holds files and no package, is open to all). A story (`*.stories.tsx`) and a sample module
//   beside it (`*.samples.ts(x)`), which only the Storybook host loads, may also import @my/e2e/fake, the fake API
//   (ADR-0171 §5), and @my/devtools/options, the live options the decider picks from (ADR-0168 §1), and nothing else
//   of either package. A test (`*.test.ts(x)`) and a module of a
//   package's `test/` directory, which only the test runner loads, may also import @my/e2e/fake, and nothing else of
//   it.
//
// - design-system-props: outside @my/design-system, no element of one of its components gets a `className` or a
//   `style` (§4). The `Omit` of each component's props refuses them only while the types hold; this also refuses them
//   through a cast, a spread of an object that holds them, and a component bound under another name in the same file.
//   It does not see, for want of other files and of types:
//   - a component that another package re-exports under another name (`export { Button as X }` in app-components);
//   - a wrapper that spreads its own props onto a component (`(p) => <Button {...p} />`), whose callers can pass them.
//   In the layers above it (app-components, app-features, app) no element gets either, of the DOM or a component of
//   their own, and no class is written otherwise: no `className` or `style` assigned, no `classList`, no
//   `setAttribute("class" or "style")`, and no import of a library that composes classes (tailwind-variants,
//   tailwind-merge, clsx). Only the design system writes a class, and its stylesheet holds the classes of its own
//   files alone. A story and a test may give an element of the DOM a class and a style, to stand a specimen in a
//   width.
//
// - platform-globals: in a layer other than @my/platform, nothing checks the browser, the PWA or the OS (ADR-0028
//   §17): no `navigator` or other global of the platform (as a global or a member of any object), no `"x" in window`
//   (or self, globalThis, document and their members), and no `matchMedia` of a platform media feature written as a
//   literal (display-mode, pointer, hover). It does not see a query computed at run time.
//
// - e2e-queries: in @my/e2e, no element is found by its position (`.nth()`, `.first()`, `.last()`), its markup
//   (`.locator()` with a CSS or XPath selector written in the file, `$`, `$$`) or a test ID (`getByTestId`), what a
//   reader of the screen cannot see. It refuses a method of those names on any object, and does not see a selector
//   held in a variable.
//
// - e2e-flaky: in @my/e2e, none of the patterns that made the E2E tests flaky, each refused with the incidents it
//   comes from and what to write instead: a one-shot read of the page (`.textContent()`, `.isVisible()`, `.count()`…,
//   and `expect(await …)`), a name or a text matched as a substring (a string without `exact: true`, a RegExp not
//   anchored `^…$`), a checkbox set by its state (`.check()`, `.uncheck()`, `.setChecked()`), and motion
//   (`reducedMotion: "no-preference"`, and a playwright.config.ts whose own `use` does not reduce it). Under specs/
//   (the specs and specs/screens/) it also refuses a wait for time and a read of the clock (`waitForTimeout`,
//   `setTimeout`, `setInterval`, `Date.now()`, `new Date()`), a `timeout` option and a forced action (`force: true`);
//   the demo server and the seed of src/ poll for readiness and remove their files by force. It refuses the methods by
//   name on any object, and sees a name or a RegExp written in the file, not one held in a variable.
//
// - switch-in-form, settings-sections, heading-size, settings-count: the checks of ADR-0110 §6 that a type cannot
//   hold, in the layers above @my/design-system: no switch in a form that has a submit button; a settings screen
//   takes sections alone, and no control stands beside a section outside any; a heading of the second level takes no
//   `size`; a group of the settings' menu and a section are written with seven things at most. Each reads the JSX one
//   file writes out: what a list is mapped to and what another file renders, `settingsFindings` of
//   @my/design-system/test/settings reads from the rendered tree. (That a control which saves at once takes its save
//   state is a required prop of `Switch` and `SettingRow`.)
//
// - zone-dates: in the web client, only zone.tsx of @my/design-system reads the device's clock and builds a formatter
//   of dates (ADR-0009 §3): no `Intl.DateTimeFormat`, no method of a Date that reads, sets or writes out the device's
//   wall clock, no Date built from the parts of one or from a date and time written with no zone
//   (`new Date("2026-11-16T09:00")`, `Date.parse("…")`), and no new Date written out as it is (`String(new Date())`, in
//   a template, `.toString()`). It refuses the methods by name on any object. It has no types, so it does not see a
//   date and time held in a variable (`new Date(value)`, `Date.parse(value)`: the API's timestamps, which name their
//   zone, are read so), nor a Date held in one and written out by coercion (`String(date)`, `${date}`,
//   `date.toString()`, `"" + date`).
//
// - pager-parts: outside @my/design-system, no button pages a list (#3279): a `Button`, `TextButton` or `LinkButton`
//   whose words are a message of the catalog that pages or grows a list (`t("….more")`, `_more`, `.earlier`, `.later`,
//   `.next`, `.previous`; a thread's fold of earlier posts, a disclosure, aside) is a pager of its own. The design
//   system's Pager (Previous and Next, Later and Earlier) and ShowMore are the one look of both kinds. It reads the
//   message ID written in the element's children, not one held in a variable.
//
// - page-trail: in the staff's screens (a file under staff/ of @my/app-features but its parts.tsx), no list keeps the
//   trail of its pages in the address itself (#3279): no `trail` read or written as a member or a key. useAddressPages
//   of staff/parts.tsx is the one place that keeps a list's cursor and trail.
//
// - html-sinks: in the web client, no string reaches the browser as markup (#3295). The shell requires Trusted Types
//   and lets two policies be created (ADR-0019 §9; internal/interface/web/shell.go), so the browser throws on an HTML
//   sink given a string, and the screen breaks (#3272, #3262). It refuses a write of `innerHTML`, `outerHTML` and
//   `srcdoc` (an assignment, `setAttribute("srcdoc", …)`), a call of `insertAdjacentHTML`, `DOMParser`, and the props
//   `dangerouslySetInnerHTML` and `srcDoc` (an attribute, or a key of an object of props). A read of `innerHTML` is
//   no sink. It refuses the members by name on any object.

import { rules as styles } from "./styles.js";

const DESIGN_SYSTEM = /^@my\/design-system(?:\/|$)/;
const LAYOUT_PROPS = new Set(["className", "style"]);
/** How many bindings a name is followed through (`const A = B`, `const B = C`…) before it is taken as unknown. */
const MAX_DEPTH = 8;

/** @typedef {{ type: string, [key: string]: any }} Node */

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
  if (key.type === "Literal" && typeof key.value === "string") return key.value;
  if (key.type === "TemplateLiteral" && key.expressions.length === 0) return key.quasis[0].value.cooked;
  return undefined;
}

/** The libraries that compose classes, which only @my/design-system imports. */
const CLASS_LIBRARIES = /^(?:tailwind-variants|tailwind-merge|clsx|classnames)(?:\/|$)/;

/** The layers above @my/design-system, by their directories of web/: no element of theirs gets a class or a style. */
const ABOVE = new Set(["app-components", "app-features", "app"]);

/** The name of the element, as written: `Button`, `ui.Button`. */
function elementName(/** @type {Node} */ name) {
  if (name.type === "JSXMemberExpression") return `${elementName(name.object)}.${name.property.name}`;
  if (name.type === "JSXNamespacedName") return `${name.namespace.name}:${name.name.name}`;
  return name.name;
}

/**
 * How a file's names reach @my/design-system: `resolve`, the variable a name refers to at a node, and
 * `isDesignSystem`, whether an expression is a binding of the package or a member of one.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function bindings(context) {
  const { sourceCode } = context;

  /** The variable that `name` refers to at `node`. */
  function resolve(/** @type {Node} */ node, /** @type {string} */ name) {
    for (let scope = sourceCode.getScope(node); scope; scope = scope.upper) {
      const variable = scope.set.get(name);
      if (variable) return variable;
    }
    return undefined;
  }

  /** Whether `variable` is bound to @my/design-system: imported from it, or assigned from what is. */
  function fromDesignSystem(/** @type {any} */ variable, /** @type {number} */ depth) {
    if (!variable || depth > MAX_DEPTH) return false;
    return variable.defs.some((/** @type {any} */ def) => {
      if (def.type === "ImportBinding") return DESIGN_SYSTEM.test(def.parent.source.value);
      // `const Alias = Button`, `const Alias = ui.Button as typeof Button`, `const { Button: Alias } = ui`.
      if (def.type === "Variable" && def.node.init) return isDesignSystem(def.node.init, depth + 1);
      return false;
    });
  }

  /** Whether the expression `node` is a binding of @my/design-system or a member of one (`ui.Button`, `Menu.Item`). */
  function isDesignSystem(/** @type {Node} */ node, /** @type {number} */ depth) {
    const expression = unwrap(node);
    if (expression.type === "Identifier" || expression.type === "JSXIdentifier")
      return fromDesignSystem(resolve(expression, expression.name), depth);
    if (expression.type === "MemberExpression" || expression.type === "JSXMemberExpression")
      return isDesignSystem(expression.object, depth);
    return false;
  }

  return { resolve, isDesignSystem };
}

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function create(context) {
  const filename = String(context.filename).replaceAll("\\", "/");
  const above = ABOVE.has(webDir(filename) ?? "") && !STORY_FILE.test(filename) && !TEST_FILE.test(filename);
  const { resolve, isDesignSystem } = bindings(context);

  const writesNone =
    "a layer above it writes no class and no style (ADR-0025 §4), and its stylesheet holds no class written here. " +
    "Compose the parts of @my/design-system by their props, or add the part or the variant it lacks to " +
    "@my/design-system.";

  /** Reports `prop` given to `component`, a part of @my/design-system (`ds`) or an element above it. */
  function report(
    /** @type {Node} */ node,
    /** @type {string} */ prop,
    /** @type {string} */ component,
    /** @type {boolean} */ ds,
  ) {
    context.report({
      node,
      message: ds
        ? `<${component}> of @my/design-system gets \`${prop}\`: outside the package a component takes no class and ` +
          "no style (ADR-0025 §4). Use its props and variants, or add the variant it lacks to @my/design-system."
        : `<${component}> above @my/design-system gets \`${prop}\`: ${writesNone}`,
    });
  }

  /** Reports a class or a style written above @my/design-system outside an element's props. */
  function reportWritten(/** @type {Node} */ node, /** @type {string} */ what) {
    context.report({ node, message: `\`${what}\` writes a class or a style above @my/design-system: ${writesNone}` });
  }

  /** The name of the member a member expression reads, written as a name or as a string. */
  const memberName = (/** @type {Node} */ member) =>
    !member.computed
      ? member.property.name
      : member.property.type === "Literal" && typeof member.property.value === "string"
        ? member.property.value
        : undefined;

  /**
   * Reports each `className` or `style` that the spread object `node` holds: an object literal, or a variable holding
   * one (with what is assigned to its members), through casts, conditionals and nested spreads.
   */
  function checkSpread(
    /** @type {Node} */ node,
    /** @type {string} */ component,
    /** @type {boolean} */ ds,
    /** @type {number} */ depth,
  ) {
    if (depth > MAX_DEPTH) return;
    const expression = unwrap(node);
    switch (expression.type) {
      case "ObjectExpression":
        for (const property of expression.properties) {
          if (property.type === "SpreadElement") checkSpread(property.argument, component, ds, depth + 1);
          else {
            const name = keyName(property);
            if (name !== undefined && LAYOUT_PROPS.has(name)) report(property, name, component, ds);
          }
        }
        return;
      case "ConditionalExpression":
        checkSpread(expression.consequent, component, ds, depth + 1);
        checkSpread(expression.alternate, component, ds, depth + 1);
        return;
      case "LogicalExpression":
        checkSpread(expression.left, component, ds, depth + 1);
        checkSpread(expression.right, component, ds, depth + 1);
        return;
      case "Identifier": {
        const variable = resolve(expression, expression.name);
        if (!variable) return;
        for (const def of variable.defs)
          if (def.type === "Variable" && def.node.id.type === "Identifier" && def.node.init)
            checkSpread(def.node.init, component, ds, depth + 1);
        // `props.className = …`, `props["style"] = …`.
        for (const reference of variable.references) {
          const member = reference.identifier.parent;
          if (member?.type !== "MemberExpression" || member.object !== reference.identifier) continue;
          const assignment = member.parent;
          if (assignment?.type !== "AssignmentExpression" || assignment.left !== member) continue;
          const name = member.computed
            ? member.property.type === "Literal" && typeof member.property.value === "string"
              ? member.property.value
              : undefined
            : member.property.name;
          if (name !== undefined && LAYOUT_PROPS.has(name)) report(member, name, component, ds);
        }
        return;
      }
      default:
        return;
    }
  }

  return {
    JSXOpeningElement(/** @type {Node} */ node) {
      const name = node.name;
      if (name.type === "JSXNamespacedName") return;
      // A lower-case name is a DOM element; it and a component of the file's own layer take a class and a style below
      // the layers above the design system.
      const dom = name.type === "JSXIdentifier" && !/^[A-Z]/.test(name.name);
      const ds = !dom && isDesignSystem(name, 0);
      if (!ds && !above) return;
      const component = elementName(name);
      for (const attribute of node.attributes) {
        if (attribute.type === "JSXSpreadAttribute") checkSpread(attribute.argument, component, ds, 0);
        else if (attribute.name.type === "JSXIdentifier" && LAYOUT_PROPS.has(attribute.name.name))
          report(attribute, attribute.name.name, component, ds);
      }
    },
    // `element.className = …`, `element.style = …`.
    AssignmentExpression(/** @type {Node} */ node) {
      const left = unwrap(node.left);
      const name = left.type === "MemberExpression" ? memberName(left) : undefined;
      if (above && name !== undefined && LAYOUT_PROPS.has(name)) reportWritten(left, `.${name} =`);
    },
    // `element.classList.add(…)`, `element.setAttribute("class", …)`.
    CallExpression(/** @type {Node} */ node) {
      if (!above) return;
      const callee = unwrap(node.callee);
      if (callee.type !== "MemberExpression") return;
      const object = unwrap(callee.object);
      if (object.type === "MemberExpression" && memberName(object) === "classList")
        reportWritten(callee, `classList.${memberName(callee) ?? "…"}()`);
      const [attribute] = node.arguments;
      if (
        memberName(callee) === "setAttribute" &&
        attribute?.type === "Literal" &&
        (attribute.value === "class" || attribute.value === "style")
      )
        reportWritten(node, `setAttribute("${attribute.value}")`);
    },
    ImportDeclaration(/** @type {Node} */ node) {
      const source = node.source.value;
      if (above && typeof source === "string" && CLASS_LIBRARIES.test(source)) reportWritten(node.source, source);
    },
  };
}

/** Each layer of the web client, by its directory under web/, and the layers below it, which it may import. */
const BELOW = {
  "design-tokens": [],
  "design-system": ["design-tokens"],
  "app-components": ["design-system", "design-tokens"],
  platform: [],
  rtc: [],
  "app-data": ["platform", "rtc"],
  "app-features": ["app-components", "app-data", "design-system", "design-tokens", "platform", "rtc"],
  app: ["app-features", "app-components", "app-data", "design-system", "design-tokens", "platform", "rtc", "i18n"],
};

/** The directory of web/ that holds files and no package, which every package may read. */
const SHARED_FILES = "brand";

/** A story, or a sample module beside it: what only the Storybook host loads. */
const STORY_FILE = /\.(?:stories\.tsx|samples\.tsx?)$/;
/** A test, or a module of a `test/` directory: what only the test runner loads. */
const TEST_FILE = /(?:\.test\.tsx?|\/test\/[^/]+\.tsx?)$/;
/** The fake API (ADR-0171 §5), which a story and a test may import besides the layers below their own. */
const FAKE_API = "@my/e2e/fake";
/** What a story may import besides: the fake API, and live options. */
const STORY_FAKES = new Set([FAKE_API, "@my/devtools/options"]);

/** The directory under web/ of a path (`app` of `/…/web/app/src/main.tsx`), or undefined outside web/. */
function webDir(/** @type {string} */ path) {
  const at = path.lastIndexOf("/web/");
  return at < 0 ? undefined : path.slice(at + "/web/".length).split("/")[0];
}

/** Resolves `/`-separated `relative` against the directory `from`, as a module path does. */
function join(/** @type {string} */ from, /** @type {string} */ relative) {
  const parts = from.split("/");
  for (const part of relative.split("/")) {
    if (part === "..") parts.pop();
    else if (part !== ".") parts.push(part);
  }
  return parts.join("/");
}

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function createLayerImports(context) {
  const filename = String(context.filename).replaceAll("\\", "/");
  const own = webDir(filename);
  if (own === undefined || !Object.hasOwn(BELOW, own)) return {};
  const below = new Set(BELOW[/** @type {keyof typeof BELOW} */ (own)]);
  const story = STORY_FILE.test(filename);
  const test = TEST_FILE.test(filename);

  function check(/** @type {Node | null | undefined} */ source) {
    if (source?.type !== "Literal" || typeof source.value !== "string") return;
    const specifier = source.value;
    if (story && STORY_FAKES.has(specifier)) return;
    if (test && specifier === FAKE_API) return;
    let target;
    let relative = false;
    if (specifier.startsWith("@my/")) target = specifier.slice("@my/".length).split("/")[0];
    else if (specifier.startsWith(".")) {
      target = webDir(join(filename.slice(0, filename.lastIndexOf("/")), specifier));
      relative = true;
    }
    if (target === undefined || target === own || target === SHARED_FILES) return;
    if (relative) {
      context.report({
        node: source,
        message:
          `@my/${own} imports @my/${target} through a relative path: a package of the workspace imports another by ` +
          `its name, a layer only the layers below it (ADR-0025 §3). Import it as @my/${target}` +
          (below.has(target) ? "." : `, or move what it needs into a layer below @my/${own}.`),
      });
      return;
    }
    if (below.has(target)) return;
    context.report({
      node: source,
      message:
        `@my/${own} imports @my/${target}, which is not below it: a layer imports only the layers below it ` +
        "(ADR-0025 §3): design-tokens ← design-system ← app-components ← app-features ← app, with app-data beside " +
        `app-components under app-features, and @my/i18n for the app alone. Move what @my/${own} needs into a layer ` +
        `below it, or let a layer above it pass it in (props, a provider, or the app's wiring).`,
    });
  }

  return {
    ImportDeclaration: (/** @type {Node} */ node) => check(node.source),
    ExportNamedDeclaration: (/** @type {Node} */ node) => check(node.source),
    ExportAllDeclaration: (/** @type {Node} */ node) => check(node.source),
    ImportExpression: (/** @type {Node} */ node) => check(node.source),
  };
}

/** The globals of the platform: each is read in @my/platform alone, through a global or a member of any object. */
const PLATFORM_GLOBALS = new Set(["navigator", "PublicKeyCredential", "Notification", "PushManager"]);
/** The global objects whose members a check of the platform's features (`"ontouchstart" in window`) reads. */
const GLOBAL_OBJECTS = new Set(["window", "self", "globalThis", "document"]);
/**
 * The media features of the platform: the display mode of the PWA and the input devices. The viewport's (width,
 * orientation) belong to the design system and the app (ADR-0025 §4), the reader's preferences (prefers-*) to the
 * design system, and stay open.
 */
const PLATFORM_MEDIA = /\b(?:display-mode|(?:any-)?pointer|(?:any-)?hover)\b/;
/** The node types under which an identifier is a type, not a value; the casts around a value are not among them. */
const VALUE_WRAPPERS = new Set(["TSAsExpression", "TSSatisfiesExpression", "TSNonNullExpression", "TSTypeAssertion"]);

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function createPlatformGlobals(context) {
  const own = webDir(String(context.filename).replaceAll("\\", "/"));
  if (own === undefined || own === "platform" || !Object.hasOwn(BELOW, own)) return {};
  const { sourceCode } = context;
  const fix = BELOW[/** @type {keyof typeof BELOW} */ (own)].includes("platform")
    ? "Ask @my/platform whether the browser can (add what it lacks there), and hold the screen for when it cannot."
    : `@my/${own} cannot import @my/platform: let the app ask it and pass the answer in (a prop or a provider).`;

  function report(/** @type {Node} */ node, /** @type {string} */ what) {
    context.report({
      node,
      message:
        `\`${what}\` checks the platform outside @my/platform: only it reads navigator, the platform's globals and ` +
        `media features (ADR-0028 §17). ${fix}`,
    });
  }

  /** Whether the identifier `node` names a global as a value: not a type, a key, or a binding of the file. */
  function isGlobal(/** @type {Node} */ node) {
    const parent = node.parent;
    if (parent?.type.startsWith("TS") && !VALUE_WRAPPERS.has(parent.type)) return false;
    if (parent?.type === "MemberExpression" && parent.property === node && !parent.computed) return false;
    if ((parent?.type === "Property" || parent?.type === "PropertyDefinition") && parent.key === node)
      if (!parent.computed && !parent.shorthand) return false;
    for (let scope = sourceCode.getScope(node); scope; scope = scope.upper) {
      const variable = scope.set.get(node.name);
      if (variable && variable.defs.length > 0) return false;
    }
    return true;
  }

  /** Whether `node` is a global object or a member of one (`window`, `document.documentElement`). */
  function onGlobalObject(/** @type {Node} */ node) {
    const expression = unwrap(node);
    if (expression.type === "Identifier") return GLOBAL_OBJECTS.has(expression.name) && isGlobal(expression);
    if (expression.type === "MemberExpression") return onGlobalObject(expression.object);
    return false;
  }

  /** The text of a media query written as a literal, or undefined for one computed at run time. */
  function queryText(/** @type {Node | undefined} */ node) {
    if (node === undefined) return undefined;
    const expression = unwrap(node);
    if (expression.type === "Literal" && typeof expression.value === "string") return expression.value;
    if (expression.type === "TemplateLiteral")
      return expression.quasis.map((/** @type {Node} */ q) => q.value.cooked).join(" ");
    return undefined;
  }

  return {
    Identifier(/** @type {Node} */ node) {
      if (PLATFORM_GLOBALS.has(node.name) && isGlobal(node)) report(node, node.name);
    },
    MemberExpression(/** @type {Node} */ node) {
      const name = node.computed
        ? node.property.type === "Literal" && typeof node.property.value === "string"
          ? node.property.value
          : undefined
        : node.property.name;
      if (name !== undefined && PLATFORM_GLOBALS.has(name)) report(node.property, `.${name}`);
    },
    BinaryExpression(/** @type {Node} */ node) {
      if (node.operator !== "in" || node.left.type !== "Literal" || typeof node.left.value !== "string") return;
      if (onGlobalObject(node.right)) report(node, `"${node.left.value}" in`);
    },
    CallExpression(/** @type {Node} */ node) {
      const callee = unwrap(node.callee);
      const named =
        (callee.type === "Identifier" && callee.name === "matchMedia" && isGlobal(callee)) ||
        (callee.type === "MemberExpression" && !callee.computed && callee.property.name === "matchMedia");
      if (!named) return;
      const query = queryText(node.arguments[0]);
      const feature = query === undefined ? undefined : PLATFORM_MEDIA.exec(query)?.[0];
      if (feature !== undefined) report(node, `matchMedia(${feature})`);
    },
  };
}

/** The Playwright calls that find an element by its position, its markup or a test ID: what a reader cannot see. */
const POSITIONAL = new Set(["nth", "first", "last"]);
const MARKUP = new Set(["$", "$$", "getByTestId"]);

/** Whether `node` is a string written in the file: a literal, a template, or a concatenation of them. */
function isString(/** @type {Node | undefined} */ node) {
  if (node === undefined) return false;
  const expression = unwrap(node);
  if (expression.type === "Literal") return typeof expression.value === "string";
  if (expression.type === "TemplateLiteral") return true;
  if (expression.type === "BinaryExpression" && expression.operator === "+")
    return isString(expression.left) || isString(expression.right);
  return false;
}

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function createE2EQueries(context) {
  function report(/** @type {Node} */ node, /** @type {string} */ what) {
    context.report({
      node,
      message:
        `\`${what}\` finds an element by what a reader cannot see. Query by role and accessible name ` +
        "(getByRole, getByLabel, getByText), narrowed by the region or the item it is in, and put the operations of a " +
        "screen in @my/e2e for the specs to share.",
    });
  }
  return {
    CallExpression(/** @type {Node} */ node) {
      const callee = unwrap(node.callee);
      if (callee.type !== "MemberExpression" || callee.computed || callee.property.type !== "Identifier") return;
      const name = callee.property.name;
      if (POSITIONAL.has(name)) report(callee.property, `.${name}()`);
      else if (MARKUP.has(name)) report(callee.property, `.${name}()`);
      else if (name === "locator" && isString(node.arguments[0])) report(callee.property, ".locator(selector)");
    },
  };
}

/** The Playwright reads that take the page's state once, without retrying until it settles. */
const ONE_SHOT_READS = new Set([
  "textContent",
  "innerText",
  "innerHTML",
  "isVisible",
  "isHidden",
  "isChecked",
  "isEnabled",
  "isDisabled",
  "count",
  "getAttribute",
  "inputValue",
  "allTextContents",
  "allInnerTexts",
]);
/** The queries whose first argument is the text to match, and which match a substring of it unless `exact`. */
const TEXT_QUERIES = new Set(["getByLabel", "getByText", "getByPlaceholder", "getByTitle", "getByAltText"]);
/** The calls that set a checkbox or a radio by its state, not as a person does. */
const SET_CHECKED = new Set(["check", "uncheck", "setChecked"]);
/** The calls that wait for time to pass. */
const TIMERS = new Set(["setTimeout", "setInterval"]);
/** Why a spec does not wait for time or read the clock. */
const timing =
  "waits for time to pass or reads the clock, which flakes on a slower or faster run (66d4f3db6, 45883c030, #1588). " +
  "Wait for the state itself with a retrying assertion or expect.poll, and take times from the scenario.";

/** The value of the property `name` of the object literal `node`, or undefined without one. */
function propertyValue(/** @type {Node | undefined} */ node, /** @type {string} */ name) {
  if (node === undefined) return undefined;
  const object = unwrap(node);
  if (object.type !== "ObjectExpression") return undefined;
  for (const property of object.properties)
    if (property.type === "Property" && keyName(property) === name) return unwrap(property.value);
  return undefined;
}

/** Whether `node` is the literal `value`. */
const isLiteral = (/** @type {Node | undefined} */ node, /** @type {unknown} */ value) =>
  node?.type === "Literal" && node.value === value;

/**
 * Whether the pattern of a regular expression matches the whole name: it starts with `^`, ends with an unescaped `$`,
 * and has no alternative (`|`) outside a group, which would anchor one side alone.
 */
function anchored(/** @type {string} */ pattern) {
  if (!pattern.startsWith("^") || !pattern.endsWith("$")) return false;
  let depth = 0;
  let inClass = false;
  for (let i = 0; i < pattern.length; i += 1) {
    const c = pattern[i];
    if (c === "\\") {
      // The final `$` escaped (`\$`) matches a dollar sign, not the end.
      if (i === pattern.length - 2) return false;
      i += 1;
    } else if (inClass) inClass = c !== "]";
    else if (c === "[") inClass = true;
    else if (c === "(") depth += 1;
    else if (c === ")") depth -= 1;
    else if (c === "|" && depth === 0) return false;
  }
  return true;
}

/**
 * The pattern of a regular expression written in the file: a literal (`/^Save$/`) or `new RegExp` of a string or a
 * template (whose interpolations stand for some text), else undefined.
 */
function regexPattern(/** @type {Node | undefined} */ node) {
  if (node === undefined) return undefined;
  const expression = unwrap(node);
  if (expression.type === "Literal" && expression.regex) return expression.regex.pattern;
  if (
    expression.type === "NewExpression" &&
    expression.callee.type === "Identifier" &&
    expression.callee.name === "RegExp"
  ) {
    const source = expression.arguments[0] && unwrap(expression.arguments[0]);
    if (source?.type === "Literal" && typeof source.value === "string") return source.value;
    if (source?.type === "TemplateLiteral")
      return source.quasis.map((/** @type {Node} */ q) => q.value.cooked).join("x");
  }
  return undefined;
}

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function createE2EFlaky(context) {
  const filename = String(context.filename).replaceAll("\\", "/");
  // The specs and the operations of the screens (specs/screens/) wait for states alone; the demo server and the seed
  // (src/) poll for readiness, and the configuration sets the limits of a run.
  const specs = /\/web\/e2e\/(?:[^/]+\/)*specs\//.test(filename);
  const config = filename.endsWith("/playwright.config.ts");
  let reducesMotion = false;

  function report(/** @type {Node} */ node, /** @type {string} */ what, /** @type {string} */ why) {
    context.report({ node, message: `\`${what}\` ${why}` });
  }

  const oneShot =
    "reads the page once, before it may have settled, and does not retry (0ab550727, 07d058a31, cce3243e7). Assert " +
    "on the locator with a retrying assertion (toHaveText, toHaveCount, toHaveAttribute, toBeVisible, toBeChecked, " +
    "toHaveValue), or wait for a computed value with expect.poll.";
  const substring =
    "matches any name that holds the text, and finds another element once one with a longer name shows " +
    "(cce3243e7, b6632bd23, #1403).";

  /** Checks the text or the name `node` that a query matches: a string with `exact: true`, a RegExp anchored. */
  function checkName(
    /** @type {Node} */ at,
    /** @type {string} */ query,
    /** @type {Node | undefined} */ node,
    /** @type {Node | undefined} */ options,
  ) {
    if (node === undefined) return;
    const pattern = regexPattern(node);
    if (pattern !== undefined) {
      if (!anchored(pattern))
        report(
          at,
          `${query}(/unanchored/)`,
          `${substring} Anchor the RegExp as /^…$/, with no alternative outside a group.`,
        );
      return;
    }
    if (isString(node) && !isLiteral(propertyValue(options, "exact"), true))
      report(
        at,
        `${query}(string)`,
        `${substring} Add \`exact: true\`, or match the whole name with an anchored RegExp.`,
      );
  }

  return {
    CallExpression(/** @type {Node} */ node) {
      const callee = unwrap(node.callee);
      if (callee.type === "Identifier") {
        if (callee.name === "expect" && node.arguments[0] && unwrap(node.arguments[0]).type === "AwaitExpression")
          report(node.arguments[0], "expect(await)", oneShot);
        else if (specs && TIMERS.has(callee.name)) report(callee, `${callee.name}()`, timing);
        return;
      }
      if (callee.type !== "MemberExpression" || callee.computed || callee.property.type !== "Identifier") return;
      const name = callee.property.name;
      const object = unwrap(callee.object);
      if (ONE_SHOT_READS.has(name)) report(callee.property, `.${name}()`, oneShot);
      else if (SET_CHECKED.has(name))
        report(
          callee.property,
          `.${name}()`,
          "sets the state of a checkbox or a radio, not as a person does, and fails when the control re-renders on " +
            'the change (6bd4f2c17, 5946eba33). Click the control or its label (getByRole("radio", { name, ' +
            "exact: true }).click()), then assert with toBeChecked().",
        );
      else if (name === "getByRole")
        checkName(callee.property, name, propertyValue(node.arguments[1], "name"), node.arguments[1]);
      else if (TEXT_QUERIES.has(name)) checkName(callee.property, name, node.arguments[0], node.arguments[1]);
      else if (specs && name === "waitForTimeout") report(callee.property, ".waitForTimeout()", timing);
      else if (specs && TIMERS.has(name) && object.type === "Identifier") report(callee.property, `${name}()`, timing);
      else if (specs && name === "now" && object.type === "Identifier" && object.name === "Date")
        report(callee, "Date.now()", timing);
    },
    NewExpression(/** @type {Node} */ node) {
      if (specs && node.callee.type === "Identifier" && node.callee.name === "Date" && node.arguments.length === 0)
        report(node, "new Date()", timing);
    },
    Property(/** @type {Node} */ node) {
      const name = keyName(node);
      const value = unwrap(node.value);
      if (specs && name === "force" && isLiteral(value, true))
        report(
          node,
          "force:true",
          "skips the checks that the element is visible, stable and takes the click, so the click lands on an element " +
            "still moving or covered (6bd4f2c17, 5946eba33). Wait for the state that lets a person click it with a " +
            "retrying assertion, then click without force.",
        );
      else if (specs && name === "timeout")
        report(
          node,
          "timeout:",
          "stretches one wait past the defaults, which hides a state the spec does not wait for and flakes again " +
            "under load (66d4f3db6, #309, #405). Wait for the state that comes first with a retrying assertion, or " +
            "change the limits of every test in playwright.config.ts.",
        );
      else if (name === "reducedMotion") {
        // The `use` of the configuration itself (defineConfig({ use }), export default { use }, const config = { use }),
        // not a project's.
        const use = node.parent?.parent;
        const top = use?.parent?.parent;
        if (
          isLiteral(value, "reduce") &&
          use?.type === "Property" &&
          keyName(use) === "use" &&
          (top?.type === "CallExpression" ||
            top?.type === "ExportDefaultDeclaration" ||
            top?.type === "VariableDeclarator")
        )
          reducesMotion = true;
        if (isLiteral(value, "no-preference"))
          report(
            node,
            "reducedMotion",
            "is no-preference, so the specs assert on a screen in motion; every wait for an animation flaked again, " +
              "and running without motion settled them (b511410df). Keep the motion reduced, as playwright.config.ts " +
              "sets it, and test the motion in the stories.",
          );
      }
    },
    "Program:exit"(/** @type {Node} */ node) {
      if (!config || reducesMotion) return;
      const exported = node.body.find((/** @type {Node} */ s) => s.type === "ExportDefaultDeclaration") ?? node;
      report(
        exported,
        "use.reducedMotion",
        'is not "reduce": with motion the specs assert on screens mid-transition, and every wait for an animation ' +
          'flaked again until the E2E ran without motion (b511410df). Set `use: { reducedMotion: "reduce" }`.',
      );
    },
  };
}

/** The value of a JSX attribute that a lint cannot read: an expression that is no literal. */
const UNREAD = Symbol("unread");

/**
 * The value of the attribute `name` of an opening element as the file writes it: a string or a number, `true` for an
 * attribute without a value, UNREAD for an expression that is no literal, and undefined without the attribute.
 */
function attribute(/** @type {Node} */ opening, /** @type {string} */ name) {
  for (const a of opening.attributes) {
    if (a.type !== "JSXAttribute" || a.name.type !== "JSXIdentifier" || a.name.name !== name) continue;
    if (a.value === null) return true;
    if (a.value.type === "Literal") return a.value.value;
    if (a.value.type === "JSXExpressionContainer") {
      const expression = unwrap(a.value.expression);
      if (expression.type === "Literal") return expression.value;
      if (expression.type === "TemplateLiteral" && expression.expressions.length === 0)
        return expression.quasis[0].value.cooked;
    }
    return UNREAD;
  }
  return undefined;
}

/**
 * The elements a JSX parent holds as the file writes them: its element children, through fragments, parentheses,
 * conditions (`a && <X />`, `a ? <X /> : <Y />`) and arrays written out. `most` is how many of them one render shows
 * at most (one branch of a condition); what an expression computes (a list mapped to elements) is not among them.
 * `past(limit)` is the element with which they pass `limit`, the last of them when they do not.
 * @returns {{ elements: Node[], most: number, past: (limit: number) => Node | undefined }}
 */
function heldElements(/** @type {Node[]} */ children) {
  /** @type {Node[]} */
  const elements = [];
  /**
   * Each child that holds elements: the first of them, and how many it shows at most.
   * @type {{ first: Node, most: number }[]}
   */
  const slots = [];
  /** @returns {number} How many elements `node` shows at most. */
  function visit(/** @type {Node | null | undefined} */ node) {
    if (!node) return 0;
    const expression = unwrap(node);
    switch (expression.type) {
      case "JSXElement":
        elements.push(expression);
        return 1;
      case "JSXFragment":
        return expression.children.reduce((/** @type {number} */ sum, /** @type {Node} */ c) => sum + visit(c), 0);
      case "JSXExpressionContainer":
        return visit(expression.expression);
      case "ArrayExpression":
        return expression.elements.reduce((/** @type {number} */ sum, /** @type {Node} */ c) => sum + visit(c), 0);
      case "LogicalExpression":
        return Math.max(visit(expression.left), visit(expression.right));
      case "ConditionalExpression":
        return Math.max(visit(expression.consequent), visit(expression.alternate));
      default:
        return 0;
    }
  }
  let most = 0;
  for (const child of children) {
    const before = elements.length;
    const shown = visit(child);
    most += shown;
    const first = elements[before];
    if (first !== undefined) slots.push({ first, most: shown });
  }
  function past(/** @type {number} */ limit) {
    let sum = 0;
    for (const slot of slots) {
      sum += slot.most;
      if (sum > limit) return slot.first;
    }
    return slots.at(-1)?.first;
  }
  return { elements, most, past };
}

/** The parts of @my/design-system that are a section of a settings screen. */
const SECTION_PARTS = new Set(["SettingsSection", "StatusSummary"]);
/** The parts of @my/design-system the reader acts on, and the rows that hold one: none stands outside a section. */
const CONTROL_PARTS = new Set([
  "BareTextArea",
  "Button",
  "ChannelCell",
  "ChannelRow",
  "ChannelTable",
  "CheckField",
  "Chip",
  "ChipLink",
  "ChoiceField",
  "ChoiceList",
  "ChoiceSelect",
  "DateTimeField",
  "IconButton",
  "LinkButton",
  "MenuButton",
  "NumberField",
  "SearchField",
  "Select",
  "SettingRow",
  "SettingsEmpty",
  "SettingsList",
  "SettingsListRow",
  "Switch",
  "TagInput",
  "TextButton",
  "TextField",
  "TextLink",
]);
/** The elements of the DOM the reader acts on. */
const CONTROL_ELEMENTS = new Set(["a", "button", "input", "select", "textarea"]);
/** What is sent as a whole: a form of the DOM, the design system's, and a settings screen (with its submit bar). */
const FORM_PARTS = new Set(["Form", "SettingsScreen"]);
/** The most entries of a group of the settings' menu, and the most a section holds (ADR-0110 §6). */
const SETTINGS_MOST = 7;

/**
 * How a rule of the settings reads a JSX element's name: `part`, the name of the part of @my/design-system it is
 * (`Switch` of `ui.Switch`), and `tag`, the element of the DOM it is; undefined for anything else.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function elements(context) {
  const { isDesignSystem } = bindings(context);
  return {
    part(/** @type {Node} */ name) {
      if (name.type === "JSXNamespacedName") return undefined;
      if (name.type === "JSXIdentifier" && !/^[A-Z]/.test(name.name)) return undefined;
      if (!isDesignSystem(name, 0)) return undefined;
      return name.type === "JSXMemberExpression" ? name.property.name : name.name;
    },
    tag: (/** @type {Node} */ name) =>
      name.type === "JSXIdentifier" && !/^[A-Z]/.test(name.name) ? name.name : undefined,
  };
}

/**
 * switch-in-form (ADR-0110 §6): no switch (the `Switch` of @my/design-system, or any `role="switch"`) stands in a
 * form that has a submit button. A form is a `<form>`, the design system's `Form`, or a `SettingsScreen`; its submit
 * button is anything of `type="submit"`, or a `SubmitBar`. It sees a form and a switch written in one file.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createSwitchInForm(context) {
  const { part, tag } = elements(context);
  /** @type {{ node: Node, submit: boolean, switches: { node: Node, what: string }[] }[]} */
  const forms = [];
  return {
    JSXElement(/** @type {Node} */ node) {
      const name = node.openingElement.name;
      if (tag(name) === "form" || FORM_PARTS.has(part(name) ?? "")) forms.push({ node, submit: false, switches: [] });
    },
    JSXOpeningElement(/** @type {Node} */ node) {
      const form = forms.at(-1);
      if (form === undefined) return;
      const name = part(node.name);
      if (name === "SubmitBar" || attribute(node, "type") === "submit") form.submit = true;
      if (name === "Switch") form.switches.push({ node, what: "<Switch>" });
      else if (attribute(node, "role") === "switch") form.switches.push({ node, what: 'role="switch"' });
    },
    "JSXElement:exit"(/** @type {Node} */ node) {
      const form = forms.at(-1);
      if (form?.node !== node) return;
      forms.pop();
      if (form.submit)
        for (const found of form.switches)
          context.report({
            node: found.node,
            message:
              `\`${found.what}\` stands in a form that has a submit button: a switch saves as it is pressed, and a ` +
              "form is saved as a whole when it is sent, so one screen would save in two ways (ADR-0110 §1 P1, §6). " +
              "In a form, use a CheckField; or let every control of the screen save at once, each with its save " +
              "state, and drop the submit button.",
          });
      // What a form inside another holds is the outer form's too.
      const outer = forms.at(-1);
      if (outer === undefined) return;
      outer.submit ||= form.submit;
      if (!form.submit) outer.switches.push(...form.switches);
    },
  };
}

/**
 * settings-sections (ADR-0110 §5, §6): a settings screen takes sections alone, and a section and a control outside
 * any section do not stand under one parent. It refuses, among the children a `SettingsScreen` is written with, an
 * element of the DOM and a part of @my/design-system that is no section (`SettingsSection`, `StatusSummary`); and,
 * under any other parent, a control (a control of the design system, a row that holds one, a control of the DOM)
 * beside a section. A component of the file's own layer passes: what it renders is another file's.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createSettingsSections(context) {
  const { part, tag } = elements(context);
  /** @type {Set<Node>} */
  const reported = new Set();

  function report(/** @type {Node} */ element, /** @type {string} */ why) {
    if (reported.has(element)) return;
    reported.add(element);
    context.report({
      node: element.openingElement,
      message: `\`<${elementName(element.openingElement.name)}>\` ${why}`,
    });
  }

  function check(/** @type {Node} */ parent) {
    const held = heldElements(parent.children).elements;
    const names = held.map((element) => ({
      element,
      part: part(element.openingElement.name),
      tag: tag(element.openingElement.name),
    }));
    if (parent.type === "JSXElement" && part(parent.openingElement.name) === "SettingsScreen") {
      for (const child of names)
        if (child.tag !== undefined || (child.part !== undefined && !SECTION_PARTS.has(child.part)))
          report(
            child.element,
            "stands in a settings screen outside any section: a screen of the settings takes sections alone " +
              "(SettingsSection, a StatusSummary first), so that everything on it stands under a heading that names " +
              "it (ADR-0110 §5, §6). Put it in a SettingsSection; what ends the screen is its `foot`.",
          );
      return;
    }
    if (!names.some((child) => child.part !== undefined && SECTION_PARTS.has(child.part))) return;
    for (const child of names)
      if (
        (child.part !== undefined && CONTROL_PARTS.has(child.part)) ||
        (child.tag !== undefined && CONTROL_ELEMENTS.has(child.tag))
      )
        report(
          child.element,
          "stands beside a section, in none: a section under a heading and a control outside any section do not " +
            "share a parent, or the reader cannot tell what the control belongs to (ADR-0110 §6). Put it in a " +
            "section, under a heading of its own if it belongs to none of these.",
        );
  }

  return { JSXElement: check, JSXFragment: check };
}

/**
 * heading-size (ADR-0110 §6, G5): above @my/design-system, a heading of the second level takes no `size`: an `<h2>`,
 * or any element given `level={2}`.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createHeadingSize(context) {
  return {
    JSXOpeningElement(/** @type {Node} */ node) {
      if (attribute(node, "size") === undefined) return;
      const name = elementName(node.name);
      const level = attribute(node, "level");
      if (name !== "h2" && level !== 2 && level !== "2") return;
      context.report({
        node,
        message:
          `\`<${name}>\` is a heading of the second level and gets \`size\`: a section's heading looks as its level ` +
          "says, on every screen alike (ADR-0110 §6). Drop `size`; a heading that must look otherwise stands at " +
          "another level, or is a part @my/design-system lacks.",
      });
    },
  };
}

/**
 * settings-count (ADR-0110 §6): a group of the settings' menu is written with seven entries at most, and a section
 * with seven things to do at most, its `action` among them. It counts the elements written out as children (one
 * branch of a condition), not what an expression computes: `settingsFindings` of @my/design-system/test/settings
 * counts those in the rendered tree. It reports the child with which the group or the section passes seven (the last
 * one, where the section's own action is the eighth).
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createSettingsCount(context) {
  const { part } = elements(context);
  return {
    JSXElement(/** @type {Node} */ node) {
      const name = part(node.openingElement.name);
      if (name !== "SettingsGroup" && name !== "SettingsSection") return;
      const group = name === "SettingsGroup";
      const own = !group && attribute(node.openingElement, "action") !== undefined ? 1 : 0;
      const held = heldElements(node.children);
      const count = held.most + own;
      if (count <= SETTINGS_MOST) return;
      context.report({
        node: held.past(SETTINGS_MOST)?.openingElement ?? node.openingElement,
        message: group
          ? `\`<SettingsGroup>\` holds ${count} entries: a group of the settings' menu has ${SETTINGS_MOST} at most, ` +
            "more than a reader takes in at a glance (ADR-0110 §6). Split it into two groups, each under a heading."
          : `\`<SettingsSection>\` holds ${count} things to do, its action among them: a section has ` +
            `${SETTINGS_MOST} at most (ADR-0110 §6). Split it into two sections, each under a heading.`,
      });
    },
  };
}

/** The methods of a Date that read the device's clock and calendar. */
const LOCAL_READS = new Set([
  "getFullYear",
  "getYear",
  "getMonth",
  "getDate",
  "getDay",
  "getHours",
  "getMinutes",
  "getSeconds",
  "getMilliseconds",
  "getTimezoneOffset",
]);
/** The methods of a Date that set it by the device's clock and calendar. */
const LOCAL_WRITES = new Set([
  "setFullYear",
  "setYear",
  "setMonth",
  "setDate",
  "setHours",
  "setMinutes",
  "setSeconds",
  "setMilliseconds",
]);
/** The methods that write a date or a time out in the device's zone (and a number in the device's locale). */
const LOCAL_TEXTS = new Set([
  "toLocaleString",
  "toLocaleDateString",
  "toLocaleTimeString",
  "toDateString",
  "toTimeString",
]);

/**
 * zone-dates (ADR-0009 §3): in the web client, every date and time is shown in the reader's time zone, which
 * zone.tsx of @my/design-system alone reads. It refuses, outside that file, an `Intl.DateTimeFormat` (constructed or
 * called), a method of a Date that reads or sets the device's clock (`getHours`, `setDate`…), one that writes a date
 * out in the device's zone (`toLocaleString`…), a Date built from the parts of a wall clock (`new Date(2026, 0,
 * 1)`) or from a date and time written out with no zone (`new Date("2026-11-16T09:00")`, `Date.parse` of one), and a
 * new Date written out as it is, which is the device's wall clock in English (`String(new Date())`, a new Date in a
 * template, `new Date().toString()`). It refuses the methods by name on any object, a number's `toLocaleString` too.
 * What it cannot see, for want of types: a date and time held in a variable (`new Date(value)`, `Date.parse(value)`,
 * which is how the API's timestamps, zoned, are read), and a Date held in a variable and written out by coercion
 * (`String(date)`, a template, `date.toString()`, a concatenation).
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createZoneDates(context) {
  const { sourceCode } = context;
  const fix =
    "Format with `dateFormat(locale, useTimeZone(), options)` of @my/design-system, read a clock with `wallTime` and " +
    "build a moment from one with `instantAt` (zone.tsx); a layer below the design system takes the zone as an argument.";

  function report(/** @type {Node} */ node, /** @type {string} */ what, /** @type {string} */ why) {
    context.report({ node, message: `\`${what}\` ${why} ${fix}` });
  }

  /** Whether the identifier `node` is the global of its name: no binding of the file shadows it. */
  function isGlobal(/** @type {Node} */ node) {
    for (let scope = sourceCode.getScope(node); scope; scope = scope.upper) {
      const variable = scope.set.get(node.name);
      if (variable && variable.defs.length > 0) return false;
    }
    return true;
  }

  /** Whether `node` is `Intl.DateTimeFormat`. */
  function isDateTimeFormat(/** @type {Node} */ node) {
    const callee = unwrap(node);
    if (callee.type !== "MemberExpression") return false;
    const object = unwrap(callee.object);
    const name = callee.computed
      ? callee.property.type === "Literal"
        ? callee.property.value
        : undefined
      : callee.property.name;
    return name === "DateTimeFormat" && object.type === "Identifier" && object.name === "Intl" && isGlobal(object);
  }

  const ownFormat =
    "is a formatter of its own, which shows the device's time zone to a reader who chose another (ADR-0009 §3).";
  const noZone =
    "reads a date and time written with no zone on the device's wall clock, not in the reader's time zone (ADR-0009 §3).";
  const writtenOut =
    "writes a moment out as the device's wall clock, in English, not in the reader's time zone and language (ADR-0009 §3).";

  /** Whether `node` is the global `Date`. */
  function isDate(/** @type {Node} */ node) {
    const callee = unwrap(node);
    return callee.type === "Identifier" && callee.name === "Date" && isGlobal(callee);
  }

  /** Whether `node` builds a Date (`new Date(…)`). */
  function isNewDate(/** @type {Node} */ node) {
    const built = unwrap(node);
    return built.type === "NewExpression" && isDate(built.callee);
  }

  /** The text of `node` when it is a string written in the file: a literal, or a template with no expression. */
  function written(/** @type {Node | undefined} */ node) {
    const arg = node && unwrap(node);
    if (!arg) return undefined;
    if (arg.type === "Literal" && typeof arg.value === "string") return arg.value;
    if (arg.type === "TemplateLiteral" && arg.expressions.length === 0) return arg.quasis[0]?.value.cooked ?? undefined;
    return undefined;
  }

  /**
   * Whether the date `text` is read on the device's wall clock: it names no zone (no `Z`, no offset, no GMT or UTC at
   * its end), and is not a date alone in the form of ISO 8601 (2026-11-16), which the language reads as UTC.
   */
  function zoneless(/** @type {string | undefined} */ text) {
    if (text === undefined || text.trim() === "") return false;
    if (/^\d{4}(-\d{2}(-\d{2})?)?$/.test(text.trim())) return false;
    return !/(Z|[+-]\d{2}:?\d{2}|GMT|UTC)$/i.test(text.trim());
  }

  return {
    NewExpression(/** @type {Node} */ node) {
      if (isDateTimeFormat(node.callee)) report(node, "Intl.DateTimeFormat", ownFormat);
      if (!isDate(node.callee)) return;
      if (node.arguments.length > 1)
        report(
          node,
          "Date(y,m,…)",
          "builds a moment from the parts of the device's wall clock, not of the reader's time zone (ADR-0009 §3).",
        );
      else if (zoneless(written(node.arguments[0]))) report(node, 'Date("…")', noZone);
    },
    TemplateLiteral(/** @type {Node} */ node) {
      for (const expression of node.expressions)
        if (isNewDate(expression)) report(expression, "${new Date(…)}", writtenOut);
    },
    CallExpression(/** @type {Node} */ node) {
      if (isDateTimeFormat(node.callee)) {
        report(node, "Intl.DateTimeFormat", ownFormat);
        return;
      }
      const callee = unwrap(node.callee);
      if (callee.type === "Identifier" && callee.name === "String" && isGlobal(callee)) {
        if (node.arguments.length === 1 && isNewDate(node.arguments[0]))
          report(node, "String(new Date(…))", writtenOut);
        return;
      }
      if (callee.type !== "MemberExpression") return;
      const name = callee.computed
        ? callee.property.type === "Literal" && typeof callee.property.value === "string"
          ? callee.property.value
          : undefined
        : callee.property.name;
      if (name === undefined) return;
      if (name === "parse" && isDate(callee.object)) {
        if (zoneless(written(node.arguments[0]))) report(node, 'Date.parse("…")', noZone);
        return;
      }
      if (name === "toString" && isNewDate(callee.object)) {
        report(callee.property, "new Date(…).toString()", writtenOut);
        return;
      }
      if (LOCAL_READS.has(name))
        report(
          callee.property,
          `.${name}()`,
          "reads the device's clock and calendar, not the reader's time zone (ADR-0009 §3).",
        );
      else if (LOCAL_WRITES.has(name))
        report(
          callee.property,
          `.${name}()`,
          "sets a moment by the device's clock and calendar, not the reader's time zone (ADR-0009 §3).",
        );
      else if (LOCAL_TEXTS.has(name))
        report(
          callee.property,
          `.${name}()`,
          "writes a date out in the device's time zone, not the reader's (ADR-0009 §3); a number is written with " +
            "`new Intl.NumberFormat(locale).format(n)`.",
        );
    },
  };
}

/** The buttons a list's pager would be written with, by their names. */
const PAGER_BUTTONS = new Set(["Button", "TextButton", "LinkButton"]);
/** A message of the catalog that pages a list or grows it. */
const PAGING_MESSAGE = /(?:[._]more|\.earlier|\.later|\.next|\.previous)$/;
/**
 * The messages of that shape that page nothing: a thread's fold of the posts between its first and the closest ones,
 * which shows posts already read, as a disclosure does.
 */
const NOT_PAGING = new Set(["thread.earlier"]);

/**
 * pager-parts (#3279): outside @my/design-system, a Button, TextButton or LinkButton whose words are a paging message.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createPagerParts(context) {
  /** The message IDs written in `node` as the first argument of a call of `t` (`t("…")`, `strings.t("…")`). */
  const messages = (/** @type {Node} */ node, /** @type {string[]} */ found) => {
    if (node === null || typeof node !== "object") return found;
    if (Array.isArray(node)) {
      for (const n of node) messages(n, found);
      return found;
    }
    if (node.type === "JSXElement") return found;
    if (node.type === "CallExpression") {
      const callee = node.callee;
      const named =
        (callee.type === "Identifier" && callee.name === "t") ||
        (callee.type === "MemberExpression" && !callee.computed && callee.property.name === "t");
      const first = node.arguments[0];
      if (named && first?.type === "Literal" && typeof first.value === "string") found.push(first.value);
    }
    for (const key of Object.keys(node))
      if (key !== "parent" && key !== "loc" && key !== "range") messages(node[key], found);
    return found;
  };
  return {
    JSXElement(/** @type {Node} */ node) {
      const name = elementName(node.openingElement.name);
      if (!PAGER_BUTTONS.has(name)) return;
      const paging = messages(node.children, []).find((id) => PAGING_MESSAGE.test(id) && !NOT_PAGING.has(id));
      if (paging === undefined) return;
      context.report({
        node: node.openingElement,
        message:
          `\`<${name}>\` says "${paging}" and pages a list of its own: every paged list has the design system's one ` +
          "pager (#3279). Use Pager of @my/design-system for a list that is a screen's subject (Previous and Next, " +
          "Later and Earlier with `periods`), ShowMore with useAddedFocus for one that grows in place.",
      });
    },
  };
}

/**
 * page-trail (#3279): in the staff's screens but staff/parts.tsx, no `trail` read or written as a member or a key.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createPageTrail(context) {
  const file = String(context.filename ?? context.getFilename?.() ?? "");
  if (!/\/staff\//.test(file) || /\/staff\/parts\.tsx$/.test(file)) return {};
  const report = (/** @type {Node} */ node) => {
    context.report({
      node,
      message:
        "`trail` keeps a list's pages in the address outside the one hook (#3279). Read the page with " +
        "useAddressPages of staff/parts.tsx (its `cursor`, `position` and `pager(next)` for the Pager), and " +
        "start a list again with `filtered`.",
    });
  };
  return {
    MemberExpression(/** @type {Node} */ node) {
      const key = node.property;
      if (
        (!node.computed && key.type === "Identifier" && key.name === "trail") ||
        (node.computed && key.type === "Literal" && key.value === "trail")
      )
        report(node);
    },
    Property(/** @type {Node} */ node) {
      if (node.parent?.type === "ObjectExpression" && keyName(node) === "trail") report(node);
    },
  };
}

/** The members whose write hands a string to the browser as markup. */
const HTML_MEMBERS = new Set(["innerHTML", "outerHTML", "srcdoc"]);
/** The props of React that do. */
const HTML_PROPS = new Set(["dangerouslySetInnerHTML", "srcDoc", "srcdoc"]);

/**
 * html-sinks (#3295): no HTML sink in the web client.
 * @param {any} context The rule's context, as Oxlint gives it to a JS plugin.
 */
function createHtmlSinks(context) {
  const { sourceCode } = context;
  function report(/** @type {Node} */ node, /** @type {string} */ what) {
    context.report({
      node,
      message:
        `\`${what}\` hands the browser a string as markup: the shell requires Trusted Types and lets only the ` +
        "policies my-server-sanitized-html and my-service-worker be created (ADR-0019 §9), so the browser throws " +
        "here and the screen breaks (#3272, #3262). Build the element with React or the DOM (`createElement`, " +
        "`textContent`, `replaceChildren`); a text the server rendered is read as data by html.ts of " +
        "@my/app-components and drawn with React; a script's URL goes through the policy of push.ts of @my/platform.",
    });
  }

  /** Whether the identifier `node` is the global of its name as a value: no binding of the file, no type, no key. */
  function isGlobal(/** @type {Node} */ node) {
    const parent = node.parent;
    if (parent?.type.startsWith("TS") && !VALUE_WRAPPERS.has(parent.type)) return false;
    if (parent?.type === "MemberExpression" && parent.property === node && !parent.computed) return false;
    if ((parent?.type === "Property" || parent?.type === "PropertyDefinition") && parent.key === node)
      if (!parent.computed && !parent.shorthand) return false;
    for (let scope = sourceCode.getScope(node); scope; scope = scope.upper) {
      const variable = scope.set.get(node.name);
      if (variable && variable.defs.length > 0) return false;
    }
    return true;
  }

  /** The name of the member a member expression reads, written as a name or as a string. */
  const memberName = (/** @type {Node} */ member) =>
    !member.computed
      ? member.property.name
      : member.property.type === "Literal" && typeof member.property.value === "string"
        ? member.property.value
        : undefined;

  return {
    AssignmentExpression(/** @type {Node} */ node) {
      const left = unwrap(node.left);
      if (left.type !== "MemberExpression") return;
      const name = memberName(left);
      if (name !== undefined && HTML_MEMBERS.has(name)) report(left, `.${name} =`);
    },
    CallExpression(/** @type {Node} */ node) {
      const callee = unwrap(node.callee);
      if (callee.type !== "MemberExpression") return;
      const name = memberName(callee);
      if (name === "insertAdjacentHTML") report(callee.property, ".insertAdjacentHTML()");
      const [attribute] = node.arguments;
      if (
        name === "setAttribute" &&
        attribute?.type === "Literal" &&
        typeof attribute.value === "string" &&
        attribute.value.toLowerCase() === "srcdoc"
      )
        report(node, 'setAttribute("srcdoc")');
    },
    Identifier(/** @type {Node} */ node) {
      if (node.name === "DOMParser" && isGlobal(node)) report(node, "DOMParser");
    },
    MemberExpression(/** @type {Node} */ node) {
      if (memberName(node) === "DOMParser") report(node.property, ".DOMParser");
    },
    JSXAttribute(/** @type {Node} */ node) {
      if (node.name.type === "JSXIdentifier" && HTML_PROPS.has(node.name.name)) report(node, node.name.name);
    },
    Property(/** @type {Node} */ node) {
      const name = node.parent?.type === "ObjectExpression" ? keyName(node) : undefined;
      if (name !== undefined && HTML_PROPS.has(name)) report(node, name);
    },
  };
}

export default {
  meta: { name: "restrict" },
  rules: {
    ...styles,
    "html-sinks": {
      meta: {
        type: "problem",
        docs: { description: "Disallow an HTML sink, which the shell's Trusted Types refuse a string" },
        schema: [],
      },
      create: createHtmlSinks,
    },
    "pager-parts": {
      meta: {
        type: "problem",
        docs: { description: "Disallow a button that pages or grows a list outside @my/design-system" },
        schema: [],
      },
      create: createPagerParts,
    },
    "page-trail": {
      meta: {
        type: "problem",
        docs: { description: "Disallow a trail of pages kept in the address outside the staff's useAddressPages" },
        schema: [],
      },
      create: createPageTrail,
    },
    "zone-dates": {
      meta: {
        type: "problem",
        docs: {
          description:
            "Disallow a date formatter of its own and a read of the device's clock outside zone.tsx of @my/design-system",
        },
        schema: [],
      },
      create: createZoneDates,
    },
    "settings-count": {
      meta: {
        type: "problem",
        docs: {
          description: "Disallow more than seven entries in a group of the settings' menu and in a section",
        },
        schema: [],
      },
      create: createSettingsCount,
    },
    "heading-size": {
      meta: {
        type: "problem",
        docs: { description: "Disallow a size on a heading of the second level above @my/design-system" },
        schema: [],
      },
      create: createHeadingSize,
    },
    "settings-sections": {
      meta: {
        type: "problem",
        docs: {
          description: "Disallow what is no section in a settings screen, and a control beside a section outside any",
        },
        schema: [],
      },
      create: createSettingsSections,
    },
    "switch-in-form": {
      meta: {
        type: "problem",
        docs: { description: "Disallow a switch in a form that has a submit button" },
        schema: [],
      },
      create: createSwitchInForm,
    },
    "e2e-flaky": {
      meta: {
        type: "problem",
        docs: {
          description:
            "Disallow the patterns that made the E2E tests flaky: one-shot reads, substring names, waits for time, forced and state-setting actions, and motion",
        },
        schema: [],
      },
      create: createE2EFlaky,
    },
    "e2e-queries": {
      meta: {
        type: "problem",
        docs: { description: "Disallow a query of an element by position, markup or test ID in the E2E tests" },
        schema: [],
      },
      create: createE2EQueries,
    },
    "platform-globals": {
      meta: {
        type: "problem",
        docs: { description: "Disallow a check of the browser, the PWA or the OS outside @my/platform" },
        schema: [],
      },
      create: createPlatformGlobals,
    },
    "layer-imports": {
      meta: {
        type: "problem",
        docs: { description: "Disallow an import of a layer of the web client that is not below the importing one" },
        schema: [],
      },
      create: createLayerImports,
    },
    "design-system-props": {
      meta: {
        type: "problem",
        docs: {
          description:
            "Disallow a className or a style on a component of @my/design-system outside it, and on any element above it",
        },
        schema: [],
      },
      create,
    },
  },
};
