// SPDX-License-Identifier: AGPL-3.0-only

// The text of the self-hosted Noto Sans CJK (cjk.css, ADR-0009 §5): the same Han characters in each language, which
// draws them with its region's glyphs, beside kana and Hangul; and a page in Latin alone, which loads no slice of Han.
// In the story tests, Regions checks the region each sample's stack names, and that of a Han character under each
// other tag of a region and in a non-CJK language inside a CJK one, and Code that code keeps the mono stack; the E2E
// tests read the font that drew each.

import { text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { expect } from "storybook/test";
import { ContentBody } from "./content";

/** A line in each language of the regions, with the Han characters whose glyphs differ by region: 直 骨 角 写 誤. */
const samples = [
  ["en", "Straight bone: 直 骨 角 写 誤"],
  ["ja", "直線の骨と角を写す誤り"],
  ["zh-CN", "直线的骨与角写错了"],
  ["zh-TW", "直線的骨與角寫錯了"],
  ["ko", "직선 直 骨 角 寫 誤 한국어"],
] as const;

const body: CSSProperties = { fontSize: `${text.body.size}rem`, lineHeight: text.body.lineHeight };
const caption: CSSProperties = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
};

function CjkSheet() {
  return (
    <div style={{ display: "grid", gap: "0.5rem" }}>
      <div style={caption}>Noto Sans CJK (SIL OFL 1.1), by the language of the text</div>
      {samples.map(([lang, line]) => (
        <div key={lang} style={{ display: "grid", gap: "0.125rem" }}>
          <code style={caption}>{lang}</code>
          <span lang={lang} data-lang={lang} style={body}>
            {line}
          </span>
        </div>
      ))}
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "CJK", tags: ["themes"] } satisfies Meta;
export default meta;

type Region = "JP" | "SC" | "TC" | "KR";
const allRegions: readonly Region[] = ["JP", "SC", "TC", "KR"];

/**
 * The regions of Noto Sans CJK that an element's stack names, and whether the face of the element's region that
 * claims each of its Han characters has loaded. The computed font-family has its custom properties substituted.
 */
async function regionOf(element: HTMLElement): Promise<{ named: Region[]; loaded: boolean }> {
  const { fontFamily, fontSize } = getComputedStyle(element);
  const named = allRegions.filter((r) => fontFamily.includes(`"my Noto Sans CJK ${r}"`));
  const [region] = named;
  if (region === undefined) return { named, loaded: false };
  const font = `${fontSize} "my Noto Sans CJK ${region}"`;
  const han = [...(element.textContent.match(/\p{Script=Han}/gu) ?? [])].join("");
  await document.fonts.load(font, han);
  return { named, loaded: han !== "" && document.fonts.check(font, han) };
}

/** The same characters in each region's language, drawn with that region's glyphs. */
export const Regions: StoryObj = {
  render: () => <CjkSheet />,
  play: async ({ canvasElement }) => {
    // Each sample names its region's Noto, and no other: en the page's default region, JP (the page's lang, en or
    // ar-XB, is no CJK language).
    const regions = [
      ["en", "JP"],
      ["ja", "JP"],
      ["zh-CN", "SC"],
      ["zh-TW", "TC"],
      ["ko", "KR"],
    ] as const;
    for (const [lang, region] of regions) {
      const sample = canvasElement.querySelector<HTMLElement>(`[data-lang="${lang}"]`);
      if (sample === null) throw new Error(`no sample in ${lang}`);
      await expect({ lang, ...(await regionOf(sample)) }).toEqual({ lang, named: [region], loaded: true });
    }

    // A Han character under the other tags of the regions, and in a non-CJK language inside a CJK one, which keeps
    // the region around it.
    const probes = [
      ["zh-Hans", undefined, "SC"],
      ["zh-Hant", undefined, "TC"],
      ["zh-HK", undefined, "TC"],
      ["zh-MO", undefined, "TC"],
      ["en", "zh-TW", "TC"],
      ["fr", "ko", "KR"],
      ["en", "zh-CN", "SC"],
    ] as const;
    for (const [lang, around, region] of probes) {
      const outer = document.createElement("div");
      if (around !== undefined) outer.lang = around;
      const probe = document.createElement("span");
      probe.lang = lang;
      probe.textContent = "直";
      outer.append(probe);
      canvasElement.append(outer);
      try {
        await expect({ lang, around, ...(await regionOf(probe)) }).toEqual({
          lang,
          around,
          named: [region],
          loaded: true,
        });
      } finally {
        outer.remove();
      }
    }
  },
};

/** Whether an element's stack is the mono stack: the mono override, then the system's monospace fonts. */
function isMono(element: Element): boolean {
  const { fontFamily } = getComputedStyle(element);
  return fontFamily.includes("ui-monospace") && !fontFamily.includes("system-ui");
}

/**
 * Code in a post's body, inline and as a block, with Han and an emoji, also where the code or a part of it has a
 * language: each draws with the mono stack, whose Noto Sans CJK is of the code's region.
 */
export const Code: StoryObj = {
  render: () => (
    <ContentBody lang="ja">
      <p>
        <code data-probe="inline">const 茶 = "🍵";</code> と{" "}
        <code data-probe="code-lang" lang="zh-CN">
          直线
        </code>
      </p>
      <pre data-probe="block">
        <code>
          {"// 直 "}
          <span data-probe="part-lang" lang="ko">
            한국어 直
          </span>
        </code>
      </pre>
    </ContentBody>
  ),
  play: async ({ canvasElement }) => {
    const regions = [
      ["inline", "JP"],
      ["code-lang", "SC"],
      ["block", "JP"],
      ["part-lang", "KR"],
    ] as const;
    for (const [probe, region] of regions) {
      const element = canvasElement.querySelector<HTMLElement>(`[data-probe="${probe}"]`);
      if (element === null) throw new Error(`no ${probe}`);
      await expect({ probe, mono: isMono(element), ...(await regionOf(element)) }).toEqual({
        probe,
        mono: true,
        named: [region],
        loaded: true,
      });
    }
  },
};

/** Running text in Latin alone, which draws with Noto Sans and loads no slice of Noto Sans CJK. */
export const LatinOnly: StoryObj = {
  render: () => (
    <p style={body}>
      A post in English draws its Latin letters with Noto Sans and loads no Han. Zażółć gęślą jaźń. Їжак.
    </p>
  ),
};
