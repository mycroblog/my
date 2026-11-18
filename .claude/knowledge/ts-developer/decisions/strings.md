# ts-developer: decisions, strings

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.

- A message is a whole sentence in every case it has: a case with something unknown (a provider's name, a browser nothing is recorded of) gets a key of its own (`….unknown`), never a fragment of the catalog put into another message as a value; and a key is named for where its words are read, so the words of a part that several screens share stand under that part's name (`settings.confirm.*`), not under the first screen that used it — a fragment in a sentence cannot be translated (ja showed a stray space, "Sign out Unknown browser?"), and another screen had to read "security" keys for a dialog of its own — the copy role's review on #3205 — 2026-11-18 — held 0 / missed 1
