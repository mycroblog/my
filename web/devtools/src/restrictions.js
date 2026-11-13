// SPDX-License-Identifier: AGPL-3.0-only
// The Oxlint JS plugin `restrict` (ADR-0025 §8): the layer rules of ADR-0025 §4 that a prop type cannot hold, enabled
// per path in .oxlintrc.json. restrictions.test.js lints the fixtures of web/devtools/fixtures/restrictions/ with
// Oxlint and compares its findings with the ones each fixture expects.
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

export default {
  meta: { name: "restrict" },
  rules: {
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
