// SPDX-License-Identifier: AGPL-3.0-only
// Render helpers for tests (web-client/README.md T2, T4). They render inside the app's real providers and set
// up user-event before rendering, as the user-event docs recommend.
import { type AnyRouter, createMemoryHistory, type RouterHistory } from "@tanstack/react-router";
import { render, type RenderOptions } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { onTestFinished } from "vitest";
import { App, AppI18nProvider, ExternalLinks } from "../App";
import { ConfigProvider, type RuntimeConfig } from "../lib/config";
import { type I18n, loadI18n } from "../lib/i18n";
import type { RouterContext } from "../lib/query";
import { createAppRouter } from "../router";
import { createAppServices } from "../lib/services";

/** The runtime config tests run with, as the server would write it into the shell. */
const testConfig: RuntimeConfig = {
  apiOrigin: "http://localhost:3000",
  instanceName: "my example",
  accountDomain: "my.example",
  defaultLocale: "en",
};

/** Renders a component inside the config, i18n and external link providers, in `locale` (the source locale by default). */
export async function renderWithProviders(ui: ReactNode, { locale = "en", ...options }: Omit<RenderOptions, "wrapper"> & { locale?: string } = {}) {
  const i18n = await loadI18n(locale);
  const user = userEvent.setup();
  const result = render(ui, {
    ...options,
    wrapper: ({ children }) => (
      <ConfigProvider config={testConfig}>
        <AppI18nProvider i18n={i18n}>
          <ExternalLinks>{children}</ExternalLinks>
        </AppI18nProvider>
      </ConfigProvider>
    ),
  });
  return { user, i18n, ...result };
}

export type RenderAppOptions = {
  /** The address the app opens at; the home timeline by default (the root address is the landing page without a session). */
  path?: string;
  locale?: string;
  /** Creates the router; createAppRouter (every route of the app) by default. */
  createRouter?: (options: RouterContext & { history: RouterHistory }) => AnyRouter;
};

/**
 * Renders the whole app (an integration test, T4): the real router on a memory history, a fresh
 * QueryClient and session as main.tsx makes them (the session resumes from the refresh endpoint, which
 * answers "signed out" by default), and MSW for the network. Wait for the first page with a `findBy` query.
 */
export async function renderApp({ path = "/home", locale = "en", createRouter = createAppRouter }: RenderAppOptions = {}) {
  const i18n: I18n = await loadI18n(locale);
  /** Where the app sent the browser away to (the authorization screen); jsdom cannot load it. */
  const leftTo: URL[] = [];
  /** How many times the app loaded the page again (the end of a maintenance); jsdom cannot. */
  const reloads = { count: 0 };
  const services = createAppServices(testConfig, i18n, {
    leaveApp: (url) => leftTo.push(url),
    reload: () => {
      reloads.count++;
    },
  });
  onTestFinished(() => {
    services.session.dispose();
  });
  void services.session.restore();
  const router = createRouter({ ...services, history: createMemoryHistory({ initialEntries: [path] }) });
  const user = userEvent.setup();
  const result = render(<App config={testConfig} {...services} router={router} />);
  return { user, i18n, router, leftTo, reloads, ...services, ...result };
}
