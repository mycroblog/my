// SPDX-License-Identifier: AGPL-3.0-only

// One sample post of the catalog's post states (PostStates.tsx), outside any timeline: it keeps its status and answers
// its actions as the server would (answers.ts), so that it behaves while nothing is fetched.

import { Post, type PostAction, type PostTranslation } from "@my/app-components";
import { useState } from "react";
import { answered } from "./answers";
import { statusOf, translationOf, type TimelinePost } from "./fixtures";

/**
 * One sample post, with its status and whether it shows translated kept as the server and the reader leave them;
 * `translationAs` changes the translation the server answers (the catalog's notes of a translation it knows less of).
 */
export function SamplePost({
  post,
  lang,
  translationAs,
}: {
  post: TimelinePost;
  lang: "en" | "ja";
  translationAs?: ((t: PostTranslation) => PostTranslation) | undefined;
}) {
  const [status, setStatus] = useState(() => statusOf(post, post.id));
  const answer = translationOf(post, lang);
  const translation = answer && translationAs ? translationAs(answer) : answer;
  const [translated, setTranslated] = useState(post.translated === true);
  const onAction = (action: PostAction) => {
    if (action.type === "translate" || action.type === "original") {
      setTranslated(action.type === "translate");
      return;
    }
    setStatus((s) => (s.boost === undefined ? answered(s, action) : { ...s, boost: answered(s.boost, action) }));
  };
  return (
    <Post
      status={status}
      own={post.own}
      translatable={translation !== undefined}
      translation={translated ? translation : undefined}
      onAction={onAction}
    />
  );
}
