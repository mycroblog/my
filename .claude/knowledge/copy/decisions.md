# copy: decisions

One line each: `- <decision> — <why> — <from: the decider's correction or choice> — <YYYY-MM-DD> — held N / missed N`.
A word line, `- <locale>[, to users]: write "<term>", avoid` and the patterns in code spans[`; write` ...], is also a
rule of the copy lint; `to users` spares admin strings.

- en: write "sign in" / "sign-in" / "sign out", avoid `\b[Ll]og(?:ged|ging|s)?[ -]?ins?\b(?! providers?\b)`, `\b[Ll]og(?:ged|ging|s)?[ -]?outs?\b`; write "Create an account", avoid `\bCreate account\b` — one name each ("Login provider" stays) — glossary (c24c48015) — 2026-10-28 — held 0 / missed 0
- en: write "event", avoid `\b[Gg]atherings?\b` — only the code says gathering — ADR-0161 §2 — 2026-11-10 — held 0 / missed 0
- en: write "tip", avoid `\b[Dd]onat(?:e|es|ed|ing|ions?)\b`, `\b[Tt]ax[ -]deductible\b` — a one-off payment for nothing back, no donation or deduction; "support" is the plans' — glossary (a22e2f0ee) — 2026-11-11 — held 0 / missed 0
- ja: write "ログイン" / "ログアウト", avoid `サインイン`, `サインアウト`; write "アカウントを作成", avoid `アカウントを作[らりるれろっ]`; write "試してください", avoid `お試し`; write "モバイル", avoid `電話`; write "チップ", avoid `寄付`, `カンパ`, `投げ銭`, `控除`; write "イベント", avoid `催し`, `集い` — one name each, as en — glossary (c24c48015, ed7cb4c4f, a22e2f0ee), ADR-0161 §2 — 2026-11-11 — held 0 / missed 0
- Short, flat, to the user: no jokes, no blame (not 無効な入力です) — bad states too — web-client G19 (a73e41238) — 2026-10-17 — held 0 / missed 0
- An error: what happened, why, what to do next (もう一度試す); a field error, how to fix the value; a 404 too — else it blames — web-client G16, G19 (a73e41238) — 2026-10-17 — held 0 / missed 0
- Unknown cause: reload or update the app, then try again — a step — the decider (#3108) — 2026-11-16 — held 0 / missed 0
- An empty area: the fact (まだ投稿はありません), what appears there, the next action — a first view — web-client G14 (a73e41238) — 2026-10-17 — held 0 / missed 0
- One state, one pattern: <対象>を読み込めませんでした: {reason}, then もう一度試す — learned once — web-client G18 (a73e41238) — 2026-10-17 — held 0 / missed 0
- A confirmation asks (この投稿を削除しますか？), says if it can be undone, and labels buttons with verbs (削除, キャンセル), not yes / no — the verb is the act — web-client G22 (a73e41238) — 2026-10-17 — held 0 / missed 0
- In en, sentence case, a button or item starts with a verb, names no input (Click, Tap); in ja, です・ます, a button or item ends in する or a noun (投稿する, フォロー) — any device — web-client G20 (a73e41238) — 2026-10-17 — held 0 / missed 1
- en, to users: write what the user sees and does, avoid `\bAPIs?\b`, `\bADRs?\b`, `\b[Ss]chemas?\b`, `\b[Rr]oles?\b`, `\bbytes?\b`, `\b[Cc]onversations?\b` — a system word says nothing to a user — the decider (#3091, #3090) — 2026-11-16 — held 0 / missed 2
- ja, to users: write what the user sees and does, avoid `API`, `ADR`, `スキーマ`, `ロール`, `バイト`, `会話` — as en; DM, messages (not 会話, 投稿) — the decider (#3091, #3090) — 2026-11-16 — held 0 / missed 2
- ja: write "タイトル", avoid `(?<!\p{Script=Han})題`; write "カバー画像", avoid `カバー(?!画像)`; write "閉鎖", avoid `クローズ`, `(?:サーバー|サービス|運営|提供)(?:の|を|が|は)?終了`; write "管理者", avoid `運営者`; write "予告", avoid `告知`; write "アカウントをもとに戻す", avoid `再有効化`; write "BAN", avoid `追放`; write "サポーターのプラン", avoid `サポーター制度`, `課金の?機能`; write "連携", avoid `(?:ひも|紐)[付づ]` — one word each; undo stays 元に戻す; a request is 承認 / 断る (not 拒否) — the decider (#3091, #3108, #3113) — 2026-11-16 — held 0 / missed 2
- What cannot be undone ends 「…。取り消せません。」 in ja (en "This cannot be undone."), not 元に戻せません / もとに戻せません — もとに戻す is reactivating an account; a word rule once settings.apps.*, settings.invites.revoke.about and server:mail.auto_delete_limit.detail follow — the review brief of #3203 — 2026-11-18 — held 0 / missed 2
- ja: write "違法な内容", avoid `違法なコンテンツ` — a report's category has the same words for the reporter and the moderators — the review of #3259 (staff.report.category) — 2026-11-20 — held 0 / missed 1
- In en, what a report is about is "this" (Why are you reporting this?), never "it": it may be a person — the review of #3259 (appcomponents report.*) — 2026-11-20 — held 0 / missed 1
- In ja, a handle where the reader acts on accounts (moderation, the report panel, voice, events, settings, profile) is bare (`{handle} への通報`, `担当: {handle}`), not `{handle} さん`; さん stays with a person told in the feed (notifications, a post's context lines) — the tools already wrote handles bare — the review of #3259 (staff.report.*) — 2026-11-20 — held 0 / missed 1
- A report's category has the same names for the reporter and the moderators (en "Something else", not "Other") — one name each — the review of #3259 (staff.report.category) — 2026-11-20 — held 0 / missed 1
