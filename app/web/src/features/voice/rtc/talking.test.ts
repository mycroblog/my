// SPDX-License-Identifier: AGPL-3.0-only
// Who is talking from the levels of the streams: the threshold with its hold (no flicker between words), a report only
// when the set changes, streams whose session comes later, the release of every meter, nothing without Web Audio, and
// the browser's meter on the one AudioContext of the page's calls.
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { FakeAudioContext, FakeLevelMeter, FakeRemoteAudio, FakeStream, resetFakeRtc } from "../test/fakeRtc";
import { browserEnvironment } from "./connect";
import { talkingHold, talkingSample, talkingThreshold, TalkingDetector } from "./talking";
import type { VoiceEnvironment } from "./types";

const environment: VoiceEnvironment = {
  createPeerConnection: () => {
    throw new Error("no connections here");
  },
  getMicrophone: () => Promise.reject(new Error("no microphone here")),
  createRemoteAudio: () => new FakeRemoteAudio(),
  createLevelMeter: (stream) => new FakeLevelMeter(stream),
};

const loud = talkingThreshold * 5;
const quiet = talkingThreshold / 4;

function stream(): MediaStream {
  return new FakeStream() as unknown as MediaStream;
}

function detector(env: VoiceEnvironment = environment) {
  const reports: string[][] = [];
  const talking = new TalkingDetector(env, (sessions) => reports.push([...sessions].sort()));
  return { talking, reports };
}

beforeEach(() => {
  resetFakeRtc();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the threshold and its hold", () => {
  test("talks from a loud sample, and stops only after the hold of quiet samples", async () => {
    const { talking, reports } = detector();
    const a = stream();
    talking.watch("a", "s1", a);
    FakeLevelMeter.of(a).value = loud;
    await vi.advanceTimersByTimeAsync(talkingSample);
    expect(reports).toEqual([["s1"]]);

    FakeLevelMeter.of(a).value = quiet;
    await vi.advanceTimersByTimeAsync(talkingHold - talkingSample);
    expect(reports).toEqual([["s1"]]);
    await vi.advanceTimersByTimeAsync(talkingSample);
    expect(reports).toEqual([["s1"], []]);
    talking.close();
  });

  test("a short pause between words does not stop the talking", async () => {
    const { talking, reports } = detector();
    const a = stream();
    talking.watch("a", "s1", a);
    const meter = FakeLevelMeter.of(a);
    for (let i = 0; i < 10; i++) {
      meter.value = loud;
      await vi.advanceTimersByTimeAsync(talkingSample);
      meter.value = quiet;
      await vi.advanceTimersByTimeAsync(talkingHold - talkingSample);
    }
    expect(reports).toEqual([["s1"]]);
    talking.close();
  });

  test("reports only when the set changes, with every talking session", async () => {
    const { talking, reports } = detector();
    const a = stream();
    const b = stream();
    talking.watch("a", "s1", a);
    talking.watch("b", "s2", b);
    FakeLevelMeter.of(a).value = loud;
    await vi.advanceTimersByTimeAsync(talkingSample * 5);
    FakeLevelMeter.of(b).value = loud;
    await vi.advanceTimersByTimeAsync(talkingSample * 5);
    expect(reports).toEqual([["s1"], ["s1", "s2"]]);
    talking.close();
    expect(reports.at(-1)).toEqual([]);
  });
});

describe("the streams", () => {
  test("a stream whose session comes later talks once it is named", async () => {
    const { talking, reports } = detector();
    const a = stream();
    talking.watch("a", undefined, a);
    FakeLevelMeter.of(a).value = loud;
    await vi.advanceTimersByTimeAsync(talkingSample);
    expect(reports).toEqual([]);
    talking.name("a", "s1");
    expect(reports).toEqual([["s1"]]);
    talking.close();
  });

  test("the same stream again keeps its meter; another one replaces it", () => {
    const { talking } = detector();
    const a = stream();
    talking.watch("a", "s1", a);
    talking.watch("a", "s1", a);
    expect(FakeLevelMeter.all).toHaveLength(1);
    const b = stream();
    talking.watch("a", "s1", b);
    expect(FakeLevelMeter.all.map((meter) => meter.closed)).toEqual([true, false]);
    talking.close();
  });

  test("a stream that goes away stops talking and releases its meter; with none left, nothing samples", async () => {
    const { talking, reports } = detector();
    const a = stream();
    talking.watch("a", "s1", a);
    FakeLevelMeter.of(a).value = loud;
    await vi.advanceTimersByTimeAsync(talkingSample);
    talking.unwatch("a");
    expect(reports).toEqual([["s1"], []]);
    expect(FakeLevelMeter.open()).toEqual([]);
    expect(vi.getTimerCount()).toBe(0);
  });

  test("closing releases every meter, and watches nothing after", () => {
    const { talking } = detector();
    talking.watch("a", "s1", stream());
    talking.watch("b", "s2", stream());
    talking.close();
    expect(FakeLevelMeter.open()).toEqual([]);
    expect(vi.getTimerCount()).toBe(0);
    talking.watch("c", "s3", stream());
    expect(FakeLevelMeter.all).toHaveLength(2);
  });

  test("without Web Audio nothing is measured and nobody talks", async () => {
    const { createLevelMeter: _, ...withoutMeter } = environment;
    for (const env of [withoutMeter, { ...environment, createLevelMeter: () => null }]) {
      const { talking, reports } = detector(env);
      talking.watch("a", "s1", stream());
      await vi.advanceTimersByTimeAsync(talkingSample * 5);
      expect(reports).toEqual([]);
      expect(vi.getTimerCount()).toBe(0);
      talking.close();
    }
  });
});

describe("the browser's meter", () => {
  test("measures the RMS on the one AudioContext of the calls, and releases its nodes", () => {
    vi.stubGlobal("AudioContext", FakeAudioContext);
    vi.stubGlobal(
      "Audio",
      class {
        muted = false;
        srcObject: unknown = null;
        play = () => Promise.resolve();
        pause = () => undefined;
      },
    );
    FakeAudioContext.all = [];
    const first = browserEnvironment();
    const second = browserEnvironment();
    const meter = first.createLevelMeter?.(stream());
    const other = second.createLevelMeter?.(stream());
    expect(FakeAudioContext.all).toHaveLength(1);
    const context = FakeAudioContext.all[0];
    const analyser = context?.analysers[0];
    if (meter == null || other == null || context === undefined || analyser === undefined) throw new Error("no meter");
    analyser.sample = 0.25;
    expect(meter.level()).toBeCloseTo(0.25);
    analyser.sample = -0.5;
    expect(meter.level()).toBeCloseTo(0.5);
    meter.close();
    other.close();
    expect(context.analysers.map((a) => a.connected)).toEqual([false, false]);
    expect(context.sources.map((s) => s.connected)).toEqual([false, false]);
  });
});
