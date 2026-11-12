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
- ja: write "試してください", avoid `お試し` — matches the retry button もう一度試す (web-client G16, G18) — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "イベント", avoid `催し`, `集い` — users read イベント; only the code says gathering — ADR-0161 §2 — 2026-11-10 — held 0 / missed 0
