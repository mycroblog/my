// SPDX-License-Identifier: AGPL-3.0-only

// A settings screen for its tests: drawn alone on a fake server, in the main landmark as the shell places it, with the
// address a route would give it (the fragment a login provider came back with, the way back, where the app leaves
// for), counted for the test to read. What a test reads of the screen is in ./security.ts.

import { instanceQuery } from "@my/app-data";
import type { FakeApi, FakeOptions } from "@my/e2e/fake";
import { screen } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { onTestFinished } from "bun:test";
import type { ReactNode } from "react";
import { fakeData, renderFeature, type FakeData } from "./feature";

/** What a route gives a settings screen, and what the screen did with it. */
export type Address = {
  /** The fragment of the address, without its `#`. */
  fragment: string;
  /** How many times the screen asked for its way back. */
  backs: number;
  /** How many times the screen said it had read the fragment, which the route then takes off the address. */
  fragmentsRead: number;
  /** The addresses the screen left for by a load of the page, in their order. */
  left: string[];
  /** How many times the screen said the server ended the reader's session with what they did on it. */
  sessionsEnded: number;
};

/** The props of a settings screen that come from its route, counted in `address`. */
export const addressProps = (address: Address) => ({
  fragment: address.fragment,
  onBack: () => {
    address.backs += 1;
  },
  onFragmentRead: () => {
    address.fragmentsRead += 1;
  },
  leave: (href: string) => {
    address.left.push(href);
  },
  onSessionEnded: () => {
    address.sessionsEnded += 1;
  },
});

/** An address with `fragment`, at which nothing happened yet. */
const addressAt = (fragment = ""): Address => ({
  fragment,
  backs: 0,
  fragmentsRead: 0,
  left: [],
  sessionsEnded: 0,
});

/**
 * Draws a settings screen on the server `on`, in the main landmark as the shell places it, once the shell read the
 * instance, and returns the reader's hands. The server's answers reach the screen when they come, not inside an act()
 * of the test, so React is told so until the test ends, and a test waits for what it asserts.
 */
async function drawnSettings({ data }: FakeData, settings: ReactNode, locale = "en"): Promise<UserEvent> {
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  // React Aria's announcer outlives a test file and keeps what a busy button said: the next file starts without it.
  onTestFinished(() => {
    for (const log of screen.queryAllByRole("log")) log.replaceChildren();
  });
  await data.queryClient.query(instanceQuery(data));
  return renderFeature(data, <main>{settings}</main>, locale);
}

/** How the reader's account stands on the fake server (the fake's `access`). */
export type Access = NonNullable<FakeOptions["access"]>;

/** What a test gives a settings screen besides the account. */
export type Given = {
  /** The fragment of the screen's address, which may name a login provider of the fake. */
  fragment?: (fake: FakeApi) => string;
  /** What the test does to the server before the screen's first request: the scripts of its first reads. */
  before?: (on: FakeData) => void;
  /** The reader's language, English by default. */
  locale?: string;
  /** What the server is besides the account: its registration mode, a one-person server and its reader. */
  server?: Omit<FakeOptions, "access">;
};

/**
 * Draws the settings screen that `make` builds from its address, on a server whose account stands as `access` says,
 * and returns the server, the address and the reader's hands; the screen's reads may not have answered yet.
 */
export async function settingsOn(
  access: Access,
  make: (props: ReturnType<typeof addressProps>) => ReactNode,
  { fragment, before, locale, server }: Given = {},
): Promise<FakeData & { address: Address; user: UserEvent }> {
  const on = fakeData({ ...server, access });
  before?.(on);
  const address = addressAt(fragment?.(on.fake));
  const user = await drawnSettings(on, make(addressProps(address)), locale);
  return { ...on, address, user };
}
