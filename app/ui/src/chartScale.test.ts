// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import {
  labelledPeriods,
  linePath,
  markedPoints,
  niceStep,
  periodPosition,
  pointPosition,
  valuePosition,
  valueTicks,
} from "./chartScale";

describe("niceStep", () => {
  it("rounds the raw step up to 1, 2 or 5 × 10ⁿ", () => {
    expect(niceStep(10, 4)).toBe(5);
    expect(niceStep(100, 4)).toBe(50);
    expect(niceStep(7, 4)).toBe(2);
    expect(niceStep(3.6, 4)).toBe(1);
    expect(niceStep(0.9, 4)).toBe(0.5);
    expect(niceStep(1234, 4)).toBe(500);
    expect(niceStep(80, 4)).toBe(20);
  });

  it("is 1 for an empty span", () => {
    expect(niceStep(0, 4)).toBe(1);
  });
});

describe("valueTicks", () => {
  it("starts at zero and ends at the first tick at or above the largest value", () => {
    expect(valueTicks([3, 12, 7])).toEqual([0, 5, 10, 15]);
    expect(valueTicks([1234, 980])).toEqual([0, 500, 1000, 1500]);
  });

  it("reaches below zero for a negative value", () => {
    expect(valueTicks([-30, 45])).toEqual([-40, -20, 0, 20, 40, 60]);
  });

  it("is 0 to 1 without values or with only zeros", () => {
    expect(valueTicks([])).toEqual([0, 0.5, 1]);
    expect(valueTicks([0, 0])).toEqual([0, 0.5, 1]);
    expect(valueTicks([null, null])).toEqual([0, 0.5, 1]);
  });

  it("leaves out missing values", () => {
    expect(valueTicks([null, 8, null])).toEqual([0, 2, 4, 6, 8]);
  });

  it("has no binary noise in decimal steps", () => {
    expect(valueTicks([0.3])).toEqual([0, 0.1, 0.2, 0.3]);
  });
});

describe("valuePosition", () => {
  it("puts the lowest tick at the bottom and the highest at the top", () => {
    const ticks = [0, 5, 10];
    expect(valuePosition(0, ticks)).toBe(100);
    expect(valuePosition(10, ticks)).toBe(0);
    expect(valuePosition(2.5, ticks)).toBe(75);
  });

  it("measures from a negative lowest tick", () => {
    expect(valuePosition(0, [-10, 0, 10])).toBe(50);
  });
});

describe("periodPosition", () => {
  it("puts each period in the middle of its band, the oldest at the start of the line", () => {
    expect(periodPosition(0, 4, "ltr")).toBe(12.5);
    expect(periodPosition(3, 4, "ltr")).toBe(87.5);
  });

  it("puts the newest period on the left in right-to-left text", () => {
    expect(periodPosition(3, 4, "rtl")).toBe(12.5);
    expect(periodPosition(0, 4, "rtl")).toBe(87.5);
  });

  it("centres a single period", () => {
    expect(periodPosition(0, 1, "ltr")).toBe(50);
    expect(periodPosition(0, 1, "rtl")).toBe(50);
  });
});

describe("pointPosition", () => {
  it("spans the whole box, mirrored in right-to-left text", () => {
    expect(pointPosition(0, 3, "ltr")).toBe(0);
    expect(pointPosition(2, 3, "ltr")).toBe(100);
    expect(pointPosition(2, 3, "rtl")).toBe(0);
    expect(pointPosition(0, 1, "rtl")).toBe(50);
  });
});

describe("labelledPeriods", () => {
  it("labels every period when they fit", () => {
    expect(labelledPeriods(3)).toEqual([0, 1, 2]);
    expect(labelledPeriods(0)).toEqual([]);
  });

  it("labels the first, the last and every k-th period between them", () => {
    expect(labelledPeriods(7)).toEqual([0, 2, 4, 6]);
    expect(labelledPeriods(24)).toEqual([0, 6, 12, 18, 23]);
    expect(labelledPeriods(30)).toEqual([0, 8, 16, 24, 29]);
  });

  it("drops a period too close to the last one", () => {
    expect(labelledPeriods(14)).toEqual([0, 4, 8, 13]);
    expect(labelledPeriods(10, 4)).toEqual([0, 3, 6, 9]);
  });
});

describe("linePath", () => {
  it("joins the points with lines", () => {
    expect(
      linePath([
        { x: 0, y: 100 },
        { x: 50, y: 25.5 },
        { x: 100, y: 0 },
      ]),
    ).toBe("M0 100 L50 25.5 L100 0");
  });

  it("breaks the line at a missing point and leaves out a lone point", () => {
    expect(
      linePath([{ x: 0, y: 10 }, { x: 25, y: 20 }, null, { x: 75, y: 30 }, null, { x: 90, y: 1 }, { x: 100, y: 2 }]),
    ).toBe("M0 10 L25 20 M90 1 L100 2");
  });

  it("rounds coordinates to three decimals", () => {
    expect(
      linePath([
        { x: 1 / 3, y: 0 },
        { x: 2 / 3, y: 0 },
      ]),
    ).toBe("M0.333 0 L0.667 0");
  });
});

describe("markedPoints", () => {
  it("marks the newest value and the lone points", () => {
    expect(markedPoints([1, 2, 3])).toEqual([2]);
    expect(markedPoints([1, null, 3, null, 4, 5, null])).toEqual([0, 2, 5]);
    expect(markedPoints([7])).toEqual([0]);
    expect(markedPoints([null, null])).toEqual([]);
  });
});
