// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the email layout (ADR-0086 §2): tables and inline styles, which every mail client reads, never the
// components of @my/design-system. The type, the weights and the radii are the design tokens'; the colors are values
// of the template, which the server fills from its theme (ADR-0059 §2), light inline and dark in the one stylesheet
// of the head, which a client that follows the color scheme applies over them.
import { fontFamily, fontWeight, radius, rootPx, text, type TextName } from "@my/design-tokens";
import type { CSSProperties, ReactNode } from "react";
import { value, valueIf } from "./go";

/** A color of the template's `.Colors`: MailPalette of internal/app/kernel/mail_composer.go. */
type Paint = "Backdrop" | "Surface" | "Text" | "MutedText" | "Accent" | "OnAccent" | "AccentText" | "Rule";

/** What a look paints: the background, the text, and the top border of a rule. */
type Painted = { background?: Paint; color?: Paint; border?: Paint };

/** The looks of the parts, each a class that the dark scheme paints again. */
const looks = {
  backdrop: { background: "Backdrop" },
  card: { background: "Surface" },
  text: { color: "Text" },
  muted: { color: "MutedText" },
  button: { background: "Accent", color: "OnAccent" },
  link: { color: "AccentText" },
  rule: { border: "Rule" },
} as const satisfies Record<string, Painted>;

type Look = keyof typeof looks;

const paint = (scheme: "Light" | "Dark", p: Paint) => value(`$.Colors.${scheme}.${p}`);

/** The class and the light colors, inline, of a part that has `look`. */
function painted(look: Look, style: CSSProperties = {}): { className: string; style: CSSProperties } {
  const p: Painted = looks[look];
  const colors: CSSProperties = {};
  if (p.background !== undefined) colors.backgroundColor = paint("Light", p.background);
  if (p.color !== undefined) colors.color = paint("Light", p.color);
  if (p.border !== undefined) colors.borderTop = `1px solid ${paint("Light", p.border)}`;
  return { className: `mail-${look}`, style: { ...style, ...colors } };
}

/**
 * The stylesheet of the head: the color schemes the layout supports, and the dark colors of each look, which a client
 * that follows `prefers-color-scheme` applies over the inline light ones, with the dark logo in place of the light.
 */
export function schemeStyles(): string {
  const rules = Object.entries(looks).map(([look, p]: [string, Painted]) => {
    const decls: string[] = [];
    if (p.background !== undefined) decls.push(`background-color: ${paint("Dark", p.background)} !important;`);
    if (p.color !== undefined) decls.push(`color: ${paint("Dark", p.color)} !important;`);
    if (p.border !== undefined) decls.push(`border-top-color: ${paint("Dark", p.border)} !important;`);
    return `  .mail-${look} { ${decls.join(" ")} }`;
  });
  return [
    "",
    ":root { color-scheme: light dark; supported-color-schemes: light dark; }",
    "@media (prefers-color-scheme: dark) {",
    ...rules,
    "  .mail-logo-light { display: none !important; }",
    "  .mail-logo-dark { display: block !important; }",
    "}",
    "",
  ].join("\n");
}

/**
 * The system's fonts of the web client's stack (ADR-0086 §2): an email loads no font, so the stack keeps neither the
 * self-hosted fonts nor the theme's, which only a page that loads them has, and quotes a name in single quotes, as an
 * inline style in double quotes needs.
 */
const systemFonts = fontFamily.sans
  .split(", ")
  .filter((f) => !f.startsWith("var(") && !f.startsWith('"my '))
  .map((f) => f.replaceAll('"', "'"))
  .join(", ");

const px = (rem: number) => `${rem * rootPx}px`;

/** The type of a text step of the tokens. */
function textStep(name: TextName): CSSProperties {
  const t = text[name];
  return { fontFamily: systemFonts, fontSize: px(t.size), lineHeight: t.lineHeight, fontWeight: t.fontWeight };
}

/**
 * The start of a line in the email's direction, from the server: a value of the template where a keyword of CSS
 * stands, `left`, or `right` in a right-to-left locale.
 */
export const start = value("$.Start") as "left" | "right";
/** The email's direction, `ltr` or `rtl`, from the server. */
export const dir = value("$.Dir") as "ltr" | "rtl";

/** The width of the card, in px, which a wider window centres. */
export const cardWidth = 600;

/** A table of the layout: no border, no spacing, and the width of what holds it unless it says another. */
function Table({ children, width = "100%", style }: { children: ReactNode; width?: string; style?: CSSProperties }) {
  return (
    <table role="presentation" border={0} cellPadding={0} cellSpacing={0} width={width} style={style}>
      <tbody>{children}</tbody>
    </table>
  );
}

/** The body of the email, in the backdrop's color down to the window's end. */
export function Body({ children }: { children: ReactNode }) {
  return <body {...painted("backdrop", { margin: 0, padding: 0, wordSpacing: "normal" })}>{children}</body>;
}

/** The backdrop around the card, the whole width of the window, and the card on it. */
export function Card({ children }: { children: ReactNode }) {
  const backdrop = painted("backdrop");
  const card = painted("card", {
    borderRadius: px(radius.overlay),
    padding: "24px",
    direction: dir,
    textAlign: start,
  });
  return (
    <table
      role="presentation"
      border={0}
      cellPadding={0}
      cellSpacing={0}
      width="100%"
      lang={value("$.Lang")}
      dir={dir}
      {...backdrop}
    >
      <tbody>
        <tr>
          <td align="center" style={{ padding: "24px 16px" }}>
            <table
              role="presentation"
              border={0}
              cellPadding={0}
              cellSpacing={0}
              width="100%"
              style={{ maxWidth: `${cardWidth}px`, borderCollapse: "separate" }}
            >
              <tbody>
                <tr>
                  <td dir={dir} {...card}>
                    <Table>{children}</Table>
                  </td>
                </tr>
              </tbody>
            </table>
          </td>
        </tr>
      </tbody>
    </table>
  );
}

/** A line of text of the card, as a row: a paragraph in the text's color, or a muted line of the signature. */
export function Line({
  children,
  muted = false,
  align = start,
  space = "0 0 16px 0",
}: {
  children: ReactNode;
  muted?: boolean;
  align?: "left" | "right" | "center";
  space?: string;
}) {
  const look = painted(muted ? "muted" : "text", {
    ...textStep(muted ? "body" : "reading"),
    padding: space,
    textAlign: align,
    wordBreak: "break-word",
  });
  return (
    <tr>
      <td align={align} {...look}>
        {children}
      </td>
    </tr>
  );
}

/** A link of the text, in the color of links; `long` lets a long URL break anywhere. */
export function Link({ href, children, long = false }: { href: string; children: ReactNode; long?: boolean }) {
  const look = painted("link", long ? { wordBreak: "break-all" } : {});
  return (
    <a href={href} {...look}>
      {children}
    </a>
  );
}

/** The email's one action, a button centred in the card. */
export function Button({ href, children }: { href: string; children: ReactNode }) {
  const shape = { borderRadius: px(radius.control) };
  return (
    <tr>
      <td align="center" style={{ padding: "8px 0" }}>
        <table role="presentation" border={0} cellPadding={0} cellSpacing={0} style={{ borderCollapse: "separate" }}>
          <tbody>
            <tr>
              <td align="center" valign="middle" {...painted("button", shape)}>
                <a
                  href={href}

                  {...painted("button", {
                    ...shape,
                    ...textStep("reading"),
                    fontWeight: fontWeight.label,
                    lineHeight: 1.2,
                    display: "inline-block",
                    padding: "12px 24px",
                    textDecoration: "none",
                  })}
                >
                  {children}
                </a>
              </td>
            </tr>
          </tbody>
        </table>
      </td>
    </tr>
  );
}

/** The rule above the signature. */
export function Rule() {
  return (
    <tr>
      <td style={{ padding: "8px 0 16px 0" }}>
        <table role="presentation" border={0} cellPadding={0} cellSpacing={0} width="100%">
          <tbody>
            <tr>
              <td {...painted("rule", { fontSize: "1px", lineHeight: "1px" })}>&nbsp;</td>
            </tr>
          </tbody>
        </table>
      </td>
    </tr>
  );
}

/** A logo image: one of the two of the server's logo, light or dark. */
export function LogoImage({ dark = false, hidden = false }: { dark?: boolean; hidden?: boolean }) {
  return (
    <img
      src={value(".URL")}
      width={value(".Width")}
      height={value(".Height")}
      alt={value("$.Site.Name")}
      className={dark ? "mail-logo-dark" : valueIf("$.LogoDark", "mail-logo-light")}
      style={{ display: hidden ? "none" : "block", border: 0, maxWidth: "100%", height: "auto" }}
    />
  );
}
