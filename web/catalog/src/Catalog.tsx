// SPDX-License-Identifier: AGPL-3.0-only

// The catalog page, where the decider and an agent decide the tokens and the components with the notes of
// @my/devtools (ADR-0168). It has no tokens to show: the specimen and the swatches use literal values.

const sample = "The quick brown fox jumps over the lazy dog. 素早い茶色の狐がのろまな犬を飛び越える。";

const specimen = [
  { name: "Display", size: 32, weight: 700 },
  { name: "Title", size: 24, weight: 600 },
  { name: "Body", size: 16, weight: 400 },
  { name: "Caption", size: 13, weight: 400 },
];

const swatches = ["#111111", "#5f6368", "#ffffff", "#0b57d0", "#b3261e"];

const rgb = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));

// WCAG 2 relative luminance and contrast ratio.
const luminance = (hex: string) => {
  const [r = 0, g = 0, b = 0] = rgb(hex).map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const contrast = (a: string, b: string) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05);
};

const muted = { fontSize: 12, color: "#5f6368" } as const;

// One row per color, so that the colors compare down a column, with the contrast that decides whether a color can
// carry text.
function Swatches() {
  const cell = { padding: "8px 12px 8px 0", textAlign: "left", whiteSpace: "nowrap" } as const;
  const ratio = (c: string, on: string) => {
    const r = contrast(c, on);
    return `${r.toFixed(1)} ${r >= 4.5 ? "AA" : r >= 3 ? "AA large" : "—"}`;
  };
  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ borderCollapse: "collapse", fontSize: 14, width: "100%" }}>
        <thead>
          <tr style={{ ...muted, borderBottom: "1px solid #ccc" }}>
            <th style={cell} />
            <th style={cell}>Hex</th>
            <th style={cell}>RGB</th>
            <th style={cell}>on #ffffff</th>
            <th style={cell}>on #111111</th>
          </tr>
        </thead>
        <tbody>
          {swatches.map((c) => (
            <tr key={c} style={{ borderBottom: "1px solid #eee" }}>
              <td style={cell}>
                <div style={{ background: c, border: "1px solid #ccc", borderRadius: 4, height: 24, width: 40 }} />
              </td>
              <td style={cell}>
                <code>{c}</code>
              </td>
              <td style={cell}>{rgb(c).join(" ")}</td>
              <td style={cell}>{ratio(c, "#ffffff")}</td>
              <td style={cell}>{ratio(c, "#111111")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Catalog() {
  return (
    <main style={{ fontFamily: "system-ui, sans-serif", margin: "0 auto", maxWidth: 960, padding: 24 }}>
      <h1>Catalog</h1>
      <section aria-labelledby="type">
        <h2 id="type">Type</h2>
        {specimen.map((s) => (
          <div key={s.name} style={{ marginBottom: 16 }}>
            <div style={{ fontSize: 12, color: "#5f6368" }}>
              {s.name} · {s.size}px / {s.weight}
            </div>
            <p style={{ fontSize: s.size, fontWeight: s.weight, margin: 0 }}>{sample}</p>
          </div>
        ))}
      </section>
      <section aria-labelledby="color">
        <h2 id="color">Color</h2>
        <Swatches />
      </section>
    </main>
  );
}
