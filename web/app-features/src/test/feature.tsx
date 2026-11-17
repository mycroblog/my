// SPDX-License-Identifier: AGPL-3.0-only

// A feature for its component tests (ADR-0171 §2, §5): the app's data on a fake API of its own (@my/e2e/fake), read
// as a person of its seed or as a visitor, and the feature rendered under the providers the app places, in a locale,
// on the DOM the test file's kind registered. A test loads what the screen's route loads, finds what the feature shows
// by role and name, presses and types as a reader does, and scripts the fake's answers.

import { AppDataProvider, createAppData, type AppData } from "@my/app-data";
import { I18nProvider } from "@my/design-system";
import { createFakeApi, fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { render } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";

/**
 * A fake API of its own (`fake.script(route, …)` holds, fails or changes its answers, `fake.send` pushes an event of
 * its stream) and the app's data on it, without retries. The reader is the fake's (`viewer`, yuki by default), or a
 * visitor (`visitor: true`).
 */
export function fakeData(options: FakeOptions = {}): { fake: FakeApi; data: AppData } {
  const fake = createFakeApi(options);
  const data = createAppData(createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: fake.serve }));
  return { fake, data };
}

/**
 * Renders `children` on `data` under the providers the app places, in `locale`, and returns the reader's hands:
 * `user.click`, `user.keyboard`, `user.type`, `user.tab`.
 */
export function renderFeature(data: AppData, children: ReactNode, locale = "en"): UserEvent {
  render(
    <AppDataProvider data={data}>
      <I18nProvider locale={locale}>{children}</I18nProvider>
    </AppDataProvider>,
  );
  return userEvent.setup();
}
