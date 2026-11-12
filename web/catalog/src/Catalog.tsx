// SPDX-License-Identifier: AGPL-3.0-only

// The catalog page, where the decider and an agent decide the tokens and the components with the notes of
// @my/devtools (ADR-0168). It shows the tokens of @my/design-tokens.

import {
  color,
  contrast,
  contrastPairs,
  fontFamily,
  rootPx,
  text,
  type ColorName,
  type Scheme,
  type TextName,
} from "@my/design-tokens";

// The same en/ja sample for every step of the type scale.
const sample: Record<TextName, [string, string]> = {
  tagline: ["Start small, grow together.", "小さく始めて、みんなで育てる。"],
  title: ["What happened on your server today", "今日のサーバーのできごと"],
  body: [
    "mycroblog is a microblog for the fediverse. Write a short post, follow people on other servers, and reply to them as if they lived next door. It starts small and grows with its community, one server at a time.",
    "mycroblog は、フェディバースのためのマイクロブログです。短い投稿を書き、ほかのサーバーの人をフォローして、隣に住んでいるかのように返信できます。小さく始めて、コミュニティとともに、ひとつのサーバーずつ育っていきます。",
  ],
  caption: ["Posted 3 minutes ago · Public · 12 replies", "3分前に投稿 · 公開 · 返信 12件"],
};

// Sizes are in rem, so that they follow the reader's font size; hairlines stay in px.
const rem = (px: number) => `${px / rootPx}rem`;

// Text needs 4.5:1 (3:1 when large).
const verdict = (r: number, minimum = 4.5) => `${r.toFixed(1)} ${r >= minimum ? "AA" : r >= 3 ? "AA large" : "—"}`;

// The page itself is light: its labels use the light muted text.
const label = { fontSize: rem(12), color: color["muted-text"].light } as const;
const cell = { padding: `${rem(8)} ${rem(12)} ${rem(8)} 0`, textAlign: "left", whiteSpace: "nowrap" } as const;
const table = { borderCollapse: "collapse", fontSize: rem(14), width: "100%" } as const;
const head = { ...label, borderBottom: "1px solid #ccc" } as const;
const row = { borderBottom: "1px solid #eee" } as const;

function TypeSpecimen() {
  return (
    <div>
      <div style={{ ...label, marginBottom: rem(16) }}>Latin: system sans · Japanese: gothic · root {rootPx}px</div>
      {Object.entries(text).map(([name, step]) => (
        <div key={name} style={{ marginBottom: rem(24) }}>
          <div style={{ ...label, marginBottom: rem(4) }}>
            {name} · {step.size}rem ({step.size * rootPx}px) / {step.lineHeight} / {step.fontWeight}
          </div>
          {sample[name as TextName].map((line) => (
            <p
              key={line}
              style={{
                fontSize: `${step.size}rem`,
                fontWeight: step.fontWeight,
                letterSpacing: "letterSpacing" in step ? step.letterSpacing : undefined,
                lineHeight: step.lineHeight,
                margin: `0 0 ${rem(8)}`,
              }}
            >
              {line}
            </p>
          ))}
        </div>
      ))}
    </div>
  );
}

const of = (scheme: Scheme) => (name: ColorName) => color[name][scheme];

// The accent as a fill with its text, as links in body text, and as a focus ring, with a caption in muted text.
function Panel({ scheme }: { scheme: Scheme }) {
  const c = of(scheme);
  const button = {
    background: c("accent"),
    border: 0,
    borderRadius: 6,
    color: c("on-accent"),
    font: "inherit",
    fontWeight: 600,
    padding: `${rem(8)} ${rem(16)}`,
  } as const;
  const link = { color: c("accent-text") };
  return (
    <div
      style={{
        background: c("surface"),
        border: "1px solid #ccc",
        borderRadius: 8,
        color: c("text"),
        flex: `1 1 ${rem(280)}`,
        fontSize: `${text.body.size}rem`,
        lineHeight: text.body.lineHeight,
        padding: rem(16),
      }}
    >
      <div style={{ fontSize: rem(12), marginBottom: rem(12), color: c("muted-text") }}>{scheme}</div>
      <div style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: rem(16), marginBottom: rem(16) }}>
        <button type="button" style={button}>
          Post
        </button>
        <button type="button" style={{ ...button, outline: `2px solid ${c("accent-text")}`, outlineOffset: 2 }}>
          Post (focused)
        </button>
      </div>
      <p style={{ margin: 0 }}>
        Reply to{" "}
        <a href="#color" style={link}>
          @alice@example.social
        </a>{" "}
        or read{" "}
        <a href="#color" style={link}>
          the guide
        </a>
        . <span lang="ja">詳しくは</span>
        <a href="#color" lang="ja" style={link}>
          ガイド
        </a>
        <span lang="ja">をどうぞ。</span>
      </p>
      <p
        style={{
          color: c("muted-text"),
          fontSize: `${text.caption.size}rem`,
          lineHeight: text.caption.lineHeight,
          margin: `${rem(8)} 0 0`,
        }}
      >
        {sample.caption[0]}
        <br />
        <span lang="ja">{sample.caption[1]}</span>
      </p>
    </div>
  );
}

const swatch = (c: string) => (
  <div style={{ background: c, border: "1px solid #ccc", borderRadius: 4, height: rem(24), width: rem(40) }} />
);

// One row per color, so that the colors compare down a column, with the contrast on the surface of each scheme.
function Swatches() {
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={table}>
        <thead>
          <tr style={head}>
            <th style={cell}>Token</th>
            <th style={cell} />
            <th style={cell}>Light</th>
            <th style={cell}>on surface</th>
            <th style={cell} />
            <th style={cell}>Dark</th>
            <th style={cell}>on surface</th>
          </tr>
        </thead>
        <tbody>
          {Object.entries(color).map(([name, c]) => (
            <tr key={name} style={row}>
              <td style={cell}>
                <code>{name}</code>
              </td>
              <td style={cell}>{swatch(c.light)}</td>
              <td style={cell}>
                <code>{c.light}</code>
              </td>
              <td style={cell}>{verdict(contrast(c.light, color.surface.light))}</td>
              <td style={cell}>{swatch(c.dark)}</td>
              <td style={cell}>
                <code>{c.dark}</code>
              </td>
              <td style={cell}>{verdict(contrast(c.dark, color.surface.dark))}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// The pairs a check holds, with their ratio in each scheme.
function Pairs() {
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={table}>
        <thead>
          <tr style={head}>
            <th style={cell}>Pair</th>
            <th style={cell}>Minimum</th>
            <th style={cell}>Light</th>
            <th style={cell}>Dark</th>
          </tr>
        </thead>
        <tbody>
          {contrastPairs.map(({ fg, bg, minimum }) => (
            <tr key={`${fg}-${bg}`} style={row}>
              <td style={cell}>
                <code>{fg}</code> on <code>{bg}</code>
              </td>
              <td style={cell}>{minimum}:1</td>
              <td style={cell}>{verdict(contrast(color[fg].light, color[bg].light), minimum)}</td>
              <td style={cell}>{verdict(contrast(color[fg].dark, color[bg].dark), minimum)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Catalog() {
  return (
    <main style={{ fontFamily: fontFamily.sans, margin: "0 auto", maxWidth: rem(960), padding: rem(24) }}>
      <h1>Catalog</h1>
      <section aria-labelledby="type">
        <h2 id="type">Type</h2>
        <TypeSpecimen />
      </section>
      <section aria-labelledby="color">
        <h2 id="color">Color</h2>
        <div style={{ display: "flex", flexWrap: "wrap", gap: rem(16), marginBottom: rem(24) }}>
          <Panel scheme="light" />
          <Panel scheme="dark" />
        </div>
        <h3>Tokens</h3>
        <Swatches />
        <h3>Contrast pairs</h3>
        <Pairs />
      </section>
    </main>
  );
}
