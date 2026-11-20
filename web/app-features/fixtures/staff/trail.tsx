// SPDX-License-Identifier: AGPL-3.0-only
// In the staff's screens, useAddressPages of staff/parts.tsx alone keeps a list's trail of pages in the address
// (#3279): a `trail` read or written as a member or a key is refused; a cursor read for a loader stays open.

type Search = Readonly<Record<string, string | undefined>>;
declare const search: Search;
declare const cursorOf: (search: Search) => string | undefined;

export const read = [
  search["trail"], // error: trail search
  (search as { trail?: string }).trail, // error: trail search
];

export const written: Search = {
  ...search,
  trail: "", // error: trail key
};

export const allowed = { cursor: cursorOf(search) };
