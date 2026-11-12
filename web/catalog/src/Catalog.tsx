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
        <ul style={{ display: "flex", flexWrap: "wrap", gap: 16, listStyle: "none", padding: 0 }}>
          {swatches.map((c) => (
            <li key={c} style={{ width: 96 }}>
              <div style={{ background: c, border: "1px solid #ccc", borderRadius: 4, height: 64 }} />
              <code style={{ fontSize: 12 }}>{c}</code>
            </li>
          ))}
        </ul>
      </section>
    </main>
  );
}
