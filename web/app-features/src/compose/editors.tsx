// SPDX-License-Identifier: AGPL-3.0-only

// The editors of a post's text and of its content warning (text.tsx), a chunk of their own fetched once, for the
// composer (composer.tsx) and the reply in place at the end of a conversation (in-place.tsx): drawn at once where the
// chunk came already, so that the text takes the focus inside the press that opened it (a phone opens its keyboard
// only then), else as it comes; one that could not be fetched tells its place, which says so once with Try again.

import { FeedNotice, TextButton } from "@my/design-system";
import { useState, type ComponentType } from "react";
import type { stringsIn } from "../locale";
import { ChunkBoundary, retryingLazy } from "./chunk";
import type { PostTextProps, WarningTextProps } from "./text";

type T = ReturnType<typeof stringsIn>["t"];

/**
 * The editor's module, fetched once: each call after the first gets the same promise, but after a failed fetch, which
 * the next tries again; and the module, once here.
 */
let loading: Promise<typeof import("./text")> | undefined;
let loaded: typeof import("./text") | undefined;
const loadText = () =>
  (loading ??= import("./text").then(
    (m) => {
      loaded = m;
      return m;
    },
    (error: unknown) => {
      loading = undefined;
      throw error;
    },
  ));

/** Fetches the editor's module with this one, ahead of the composer's first opening. */
export function preloadComposerText(): void {
  // A failed fetch is tried again by the next.
  loadText().catch(() => undefined);
}

const lazyPostText = retryingLazy(() => loadText().then((m) => m.PostText));
const lazyWarningText = retryingLazy(() => loadText().then((m) => m.WarningText));

/**
 * In the text's place when the editors' chunk could not be fetched, once for the whole form: that it could not be
 * loaded, and Try again, which fetches both editors again.
 */
export function EditorFailed({ t, retry }: { t: T; retry: () => void }) {
  return (
    <FeedNotice alert start action={<TextButton onPress={retry}>{t("compose.load_retry")}</TextButton>}>
      {t("compose.editor_failed")}
    </FeedNotice>
  );
}

/** An editor chosen once, as it mounts: drawn at once where its module came already, else as it comes. */
function Chosen<P extends object>({
  here,
  fetched,
  props,
}: {
  here: ComponentType<P> | undefined;
  fetched: () => ComponentType<P>;
  props: P;
}) {
  const [Editor] = useState(() => here ?? fetched());
  return <Editor {...props} />;
}

/** Nothing in a failed editor's place: the view says it once, for the whole form (`EditorFailed`). */
const nothing = () => null;

/** Tries both editors again, as one: they come in one chunk. */
export function retryEditors(): void {
  lazyPostText.retry();
  lazyWarningText.retry();
}

/**
 * The text's editor: drawn at once where its module came already, so that it takes the focus inside the press that
 * opened the composer (a phone opens its keyboard only then), else as it comes; one that could not be fetched tells
 * `onFailed`.
 */
export function PostText({ onFailed, ...props }: PostTextProps & { onFailed: () => void }) {
  return (
    <ChunkBoundary fallback={nothing} onRetry={retryEditors} onFailed={onFailed}>
      <Chosen here={loaded?.PostText} fetched={lazyPostText.get} props={props} />
    </ChunkBoundary>
  );
}

/** The content warning's editor, as the text's. */
export function WarningText({ onFailed, ...props }: WarningTextProps & { onFailed: () => void }) {
  return (
    <ChunkBoundary fallback={nothing} onRetry={retryEditors} onFailed={onFailed}>
      <Chosen here={loaded?.WarningText} fetched={lazyWarningText.get} props={props} />
    </ChunkBoundary>
  );
}
