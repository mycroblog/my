# copy: decisions

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.
A word line, `- <locale>: write "<term>", avoid` and the patterns in code spans, is also a rule of the copy lint.

- en: write "sign in" / "sign-in", avoid `\b[Ll]og(?:ged|ging|s)?[ -]?ins?\b(?! providers?\b)` — one name for starting a session; "Login provider", the admin feature, stays — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- en: write "sign out", avoid `\b[Ll]og(?:ged|ging|s)?[ -]?outs?\b` — one name for ending a session — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- en: write "Create an account", avoid `\bCreate account\b` — the label of the action that creates an account — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- en: write "event", avoid `\b[Gg]atherings?\b` — users read event; only the code says gathering — ADR-0161 §2 — 2026-11-10 — held 0 / missed 0
- en: write "tip", avoid `\b[Dd]onat(?:e|es|ed|ing|ions?)\b`, `\b[Tt]ax[ -]deductible\b` — a one-time payment with nothing in return, never a donation or a deduction; "support" is the supporter plans' — glossary (a22e2f0ee) — 2026-11-11 — held 0 / missed 0
- ja: write "モバイル", avoid `電話` — the narrow screen with the bottom tab bar ("phone" in en) — glossary (ed7cb4c4f) — 2026-10-30 — held 0 / missed 0
- ja: write "チップ", avoid `寄付`, `カンパ`, `投げ銭`, `控除` — a one-time payment with nothing in return, never a donation or a deduction — glossary (a22e2f0ee) — 2026-11-11 — held 0 / missed 0
- ja: write "ログイン", avoid `サインイン` — one name for starting a session — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "ログアウト", avoid `サインアウト` — one name for ending a session — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "アカウントを作成", avoid `アカウントを作[らりるれろっ]` — the label and wording of the action that creates an account — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "試してください", avoid `お試し` — matches the retry button もう一度試す — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "イベント", avoid `催し`, `集い` — users read イベント; only the code says gathering — ADR-0161 §2 — 2026-11-10 — held 0 / missed 0
- Short, flat, to the user: no jokes, no blame (not 無効な入力です) — copy is read in the bad states too — web-client G19 (a73e41238) — 2026-10-17 — held 0 / missed 0
- An error says what happened, why, and what to do next (もう一度試す); a field error, how to fix the value — an error the user cannot act on only blames — web-client G16, G19 (a73e41238) — 2026-10-17 — held 0 / missed 0
- An empty area states the fact (まだ投稿はありません), what appears there, and the next action — it is a new user's first view — web-client G14 (a73e41238) — 2026-10-17 — held 0 / missed 0
- The same kind of state has the same pattern: <対象>を読み込めませんでした: {reason}, then もう一度試す — one pattern is learned once — web-client G18 (a73e41238) — 2026-10-17 — held 0 / missed 0
- A confirmation asks a question (この投稿を削除しますか？), says whether it can be undone, and labels its buttons with verbs (削除, キャンセル), never yes / no — a verb says what happens — web-client G22 (a73e41238) — 2026-10-17 — held 0 / missed 0
- In en, sentence case, and a button or item starts with a verb and names no input (Click, Tap); in ja, です・ます, and a button or item ends in する or a noun (投稿する, フォロー) — the label is the action on any device — web-client G20 (a73e41238) — 2026-10-17 — held 0 / missed 0
