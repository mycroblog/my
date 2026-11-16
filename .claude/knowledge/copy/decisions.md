# copy: decisions

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.
A word line, `- <locale>: write "<term>", avoid` and the patterns in code spans, is also a rule of the copy lint.

- en: write "sign in" / "sign-in" / "sign out", avoid `\b[Ll]og(?:ged|ging|s)?[ -]?ins?\b(?! providers?\b)`, `\b[Ll]og(?:ged|ging|s)?[ -]?outs?\b` — one name each ("Login provider" stays) — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- en: write "Create an account", avoid `\bCreate account\b` — the action's label — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- en: write "event", avoid `\b[Gg]atherings?\b` — only the code says gathering — ADR-0161 §2 — 2026-11-10 — held 0 / missed 0
- en: write "tip", avoid `\b[Dd]onat(?:e|es|ed|ing|ions?)\b`, `\b[Tt]ax[ -]deductible\b` — a one-off payment for nothing back, no donation or deduction; "support" is the plans' — glossary (a22e2f0ee) — 2026-11-11 — held 0 / missed 0
- ja: write "モバイル", avoid `電話` — the narrow screen — glossary (ed7cb4c4f) — 2026-10-30 — held 0 / missed 0
- ja: write "チップ", avoid `寄付`, `カンパ`, `投げ銭`, `控除` — as en tip — glossary (a22e2f0ee) — 2026-11-11 — held 0 / missed 0
- ja: write "ログイン" / "ログアウト", avoid `サインイン`, `サインアウト` — one name each — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "アカウントを作成", avoid `アカウントを作[らりるれろっ]` — the action's label — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "試してください", avoid `お試し` — as もう一度試す — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- ja: write "イベント", avoid `催し`, `集い` — as en event — ADR-0161 §2 — 2026-11-10 — held 0 / missed 0
- Short, flat, to the user: no jokes, no blame (not 無効な入力です) — read in the bad states too — web-client G19 (a73e41238) — 2026-10-17 — held 0 / missed 0
- An error: what happened, why, what to do next (もう一度試す); a field error, how to fix the value — else it blames — web-client G16, G19 (a73e41238) — 2026-10-17 — held 0 / missed 0
- An error of unknown cause: reload or update the app, then try again — the next step — the decider (#3108) — 2026-11-16 — held 0 / missed 0
- An empty area: the fact (まだ投稿はありません), what appears there, the next action — a new user's first view — web-client G14 (a73e41238) — 2026-10-17 — held 0 / missed 0
- One state, one pattern: <対象>を読み込めませんでした: {reason}, then もう一度試す — learned once — web-client G18 (a73e41238) — 2026-10-17 — held 0 / missed 0
- A confirmation asks (この投稿を削除しますか？), says if it can be undone, and labels buttons with verbs (削除, キャンセル), not yes / no — the verb is the act — web-client G22 (a73e41238) — 2026-10-17 — held 0 / missed 0
- In en, sentence case, a button or item starts with a verb, names no input (Click, Tap); in ja, です・ます, a button or item ends in する or a noun (投稿する, フォロー) — any device — web-client G20 (a73e41238) — 2026-10-17 — held 0 / missed 0
- UI words are what the user sees and does, never an ADR, schema or API: タイトル (not 題), カバー画像 (not カバー), what the account can do (not ロール), no byte counts; DM, messages (not conversation, 会話, 投稿); a 404 says what to do — an unmet word says nothing — the decider (#3091, #3090) — 2026-11-16 — held 0 / missed 2
- In ja, closing is 閉鎖 (not 終了, クローズ); 管理者, 予告 (not 運営者, 告知); アカウントをもとに戻す (not 再有効化; undo stays 元に戻す); BAN; サポーター by context (のプラン, の募集), not サポーター制度, 課金の機能; 連携 (not ひも付け) — one word — the decider (#3108) — 2026-11-16 — held 0 / missed 1
