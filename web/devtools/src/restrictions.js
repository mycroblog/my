// SPDX-License-Identifier: AGPL-3.0-only
// The Oxlint JS plugin `restrict` (ADR-0025 §8): the layer rules of ADR-0025 §3 and §4 that a type cannot hold, enabled
// per path in .oxlintrc.json. restrictions.test.js lints the fixtures of web/devtools/fixtures/restrictions/ and of
// the layers' fixtures/ with Oxlint and compares its findings with the ones each fixture expects.
//
// - layer-imports: a layer imports only the layers below it (§3): design-tokens ← design-system ← app-components ←
//   app-features ← app, with app-data beside app-components, under app-features, and platform and rtc under app-data;
//   @my/i18n reaches the app alone. The layer of a file is the package of web/ it is in. It sees `import`, `export …
//   from` and `import()` of a package of the workspace by its name, and a relative path into another package of
//   web/ (web/brand, which holds files and no package, is open to all).
//
// - design-system-props: outside @my/design-system, no element of one of its components gets a `className` or a
//   `style` (§4). The `Omit` of each component's props refuses them only while the types hold; this also refuses them
//   through a cast, a spread of an object that holds them, and a component bound under another name in the same file.
//   It does not see, for want of other files and of types:
//   - a component that another package re-exports under another name (`export { Button as X }` in app-components);
//   - a wrapper that spreads its own props onto a component (`(p) => <Button {...p} />`), whose callers can pass them.

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

/** @param {any} context The rule's context, as Oxlint gives it to a JS plugin. */
function create(context) {
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

  /** The name of the element, as written: `Button`, `ui.Button`. */
  function elementName(/** @type {Node} */ name) {
    if (name.type === "JSXMemberExpression") return `${elementName(name.object)}.${name.property.name}`;
    return name.name;
  }

  function report(/** @type {Node} */ node, /** @type {string} */ prop, /** @type {string} */ component) {
    context.report({
      node,
      message:
        `<${component}> of @my/design-system gets \`${prop}\`: outside the package a component takes no class and ` +
        "no style (ADR-0025 §4). Use its props and variants, or add the variant it lacks to @my/design-system.",
    });
  }

  /**
   * Reports each `className` or `style` that the spread object `node` holds: an object literal, or a variable holding
   * one (with what is assigned to its members), through casts, conditionals and nested spreads.
   */
  function checkSpread(/** @type {Node} */ node, /** @type {string} */ component, /** @type {number} */ depth) {
    if (depth > MAX_DEPTH) return;
    const expression = unwrap(node);
    switch (expression.type) {
      case "ObjectExpression":
        for (const property of expression.properties) {
          if (property.type === "SpreadElement") checkSpread(property.argument, component, depth + 1);
          else {
            const name = keyName(property);
            if (name !== undefined && LAYOUT_PROPS.has(name)) report(property, name, component);
          }
        }
        return;
      case "ConditionalExpression":
        checkSpread(expression.consequent, component, depth + 1);
        checkSpread(expression.alternate, component, depth + 1);
        return;
      case "LogicalExpression":
        checkSpread(expression.left, component, depth + 1);
        checkSpread(expression.right, component, depth + 1);
        return;
      case "Identifier": {
        const variable = resolve(expression, expression.name);
        if (!variable) return;
        for (const def of variable.defs)
          if (def.type === "Variable" && def.node.id.type === "Identifier" && def.node.init)
            checkSpread(def.node.init, component, depth + 1);
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
          if (name !== undefined && LAYOUT_PROPS.has(name)) report(member, name, component);
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
      // A lower-case name is a DOM element, which takes a class and a style.
      if (name.type === "JSXIdentifier" && !/^[A-Z]/.test(name.name)) return;
      if (name.type === "JSXNamespacedName" || !isDesignSystem(name, 0)) return;
      const component = elementName(name);
      for (const attribute of node.attributes) {
        if (attribute.type === "JSXSpreadAttribute") checkSpread(attribute.argument, component, 0);
        else if (attribute.name.type === "JSXIdentifier" && LAYOUT_PROPS.has(attribute.name.name))
          report(attribute, attribute.name.name, component);
      }
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

  function check(/** @type {Node | null | undefined} */ source) {
    if (source?.type !== "Literal" || typeof source.value !== "string") return;
    const specifier = source.value;
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

export default {
  meta: { name: "restrict" },
  rules: {
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
        docs: { description: "Disallow a className or a style on a component of @my/design-system outside it" },
        schema: [],
      },
      create,
    },
  },
};
