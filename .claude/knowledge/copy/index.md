# copy: index

The copy role writes the text users read (ADR-0169 §2). Read this page, then `decisions.md` and `exemplars.md`.

- The tone and the punctuation: web-client §16.5, G19 to G22 (`docs/architecture/web-client/16-look-states-and-wording.md`).
- `scripts/task.sh i18n:lint` holds the en and ja catalogs to the rules of the copy lint (`tools/i18n/src/copy.ts`).
  A finding blocks, is waived with its reason in `tools/i18n/waivers.json` (at most three new waivers a change), or
  is answered. A word line of `decisions.md` is one of its rules.
- A correction of the decider becomes a line of `decisions.md`; the same correction a second time, a proposal to turn
  the line into a check (ADR-0169 §4). Add, merge or delete lines; never rewrite a file.
