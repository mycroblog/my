// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { inputOf } from "../field-errors";
import { articleMembers } from "./article-model";

test("a refused member of an article, new, edited or a draft, names its input", () => {
  const named = {
    title: "title",
    "article.title": "title",
    text: "text",
    summary: "summary",
    "article.summary": "summary",
    "cover.description": "cover",
    "article.cover.description": "cover",
    tags: "tags",
    "tags[0]": "tags",
    "tags[4]": "tags",
    "article.tags": "tags",
    "article.tags[2]": "tags",
  };
  for (const [member, input] of Object.entries(named))
    expect([member, inputOf(member, articleMembers)]).toEqual([member, input]);
});

test("the members of an article no input answers to stay the form's", () => {
  for (const member of ["language", "article.language", "visibility", "cover", "cover.id", "Idempotency-Key"])
    expect([member, inputOf(member, articleMembers)]).toEqual([member, undefined]);
});
