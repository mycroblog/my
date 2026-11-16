// SPDX-License-Identifier: AGPL-3.0-only

// A refused write placed on its form: the server names the member of the request a 422 is about (app-data's
// refusedField), and a form declares which of its inputs answers to which member, so that what is wrong shows beside
// that input and the form's summary says only the problem's title. A member no input answers to (an ID, a header), or
// any other failure, stays the form's general error, in the server's words.

import { refusal, refusedField } from "@my/app-data";

/**
 * Which input of a form answers to which member of the request, by the member's name in the request: `text`,
 * `spoiler_text`. A member of a list is named with `[]` for its index (`poll.options[]`, `media[].description`), and
 * answers with a function of the index (the first one, in a list of lists).
 */
export type FieldMembers<K extends string> = Readonly<Record<string, K | ((index: number) => K | undefined)>>;

/** A refused write as its form shows it. */
export type Refused<K extends string> = {
  /** The form's summary: the server's words, its title alone when `field` holds them; undefined for no answer. */
  readonly reason: string | undefined;
  /** The input the refusal is about, and what is wrong with its value, to show beside it. */
  readonly field?: { readonly input: K; readonly text: string } | undefined;
};

/** The input of `members` that answers to the request's `member`, or undefined for none. */
export function inputOf<K extends string>(member: string, members: FieldMembers<K>): K | undefined {
  const exact = members[member];
  if (typeof exact === "string") return exact;
  const indices: number[] = [];
  const shape = member.replace(/\[(\d+)\]/g, (_, n: string) => {
    indices.push(Number(n));
    return "[]";
  });
  const found = members[shape];
  if (found === undefined) return undefined;
  if (typeof found === "string") return found;
  const [index] = indices;
  return index === undefined ? undefined : found(index);
}

/** `error` placed on the form whose inputs answer to `members`. */
export function placeRefusal<K extends string>(error: unknown, members: FieldMembers<K>): Refused<K> {
  const named = refusedField(error);
  const input = named === undefined ? undefined : inputOf(named.member, members);
  if (named === undefined || input === undefined) return { reason: refusal(error) };
  return { reason: named.title || undefined, field: { input, text: named.detail } };
}

/** What `refused` says beside `input`, or undefined when it is about another input or none. */
export const fieldError = <K extends string>(refused: Refused<K> | undefined, input: K): string | undefined =>
  refused?.field?.input === input ? refused.field.text : undefined;
