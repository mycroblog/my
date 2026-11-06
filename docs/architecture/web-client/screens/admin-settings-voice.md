---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.settings_.voice.tsx", "app/web/src/features/admin/components/PolicySettings.tsx", "app/web/src/features/admin/hooks/useServerPolicies.ts", "app/web/src/features/admin/api/settings.ts", "app/web/src/features/admin/components/ServerSettings.tsx", "app/ui/src/LocalNav.tsx", "app/web/src/features/voice/hooks/useVoiceRooms.ts"]
---

## サーバーの設定: 通話（`/admin/settings/voice`）

`manage_settings` を持つ管理者が、通話のルームを開ける人、ログインしていない人への要約、参加者の記録を残す日数を決めるページ（[ADR-0100](../../../adr/0100-voice-chat-rooms.md) §2・§9・§13、[§14](../14-admin.md)）。サーバーが通話を出していない（`useVoiceAvailable`）ときは、ページの間のリンクに出さず、URL で開いてもフォームの代わりに通話を出していないことを言う。
画面の名前の `h1` の下に、ページの間のリンクの並び（`LocalNav`。「全般」「アイコンとロゴ」「登録」「コンテンツ」「連合」「検索とクローラー」、サーバーが通話を出しているときだけ「通話」）を置き、今のページを `aria-current="page"` にする。管理のメニューは「サーバーの設定」の 1 項目のままで、どのページでも今いる項目になる。

### 1. 使う人と頻度

- 通話を出したサーバーの管理者。通話を始めるときと、ルームを開ける人を見直すとき。年に数回まで。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| ルームを開ける人を権限のある人だけにする | 4 |
| 参加者の記録を残す日数を変える | 4 |

手順は、管理のメニューの「サーバーの設定」を開く 1 つ、ページの間のリンクで「通話」を開く 1 つ、欄を変える 1 つ、「設定を保存する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 通話: ルームを開ける人（`RadioGroup`）、ログインしていない人への要約（チェックボックス）、参加者の記録を残す日数（`inputMode="numeric"` の `TextField`。範囲は `GET /admin/settings` の `minimum`・`maximum`）。

### 4. 主な操作と保存の仕方

- 主な操作は「設定を保存する」（ページに 1 つ、`primary`）。保存は、まとめて保存だけ。このページの設定のうち変えたものだけを `PATCH /admin/settings` でまとめて送り、すべて通るか何も変わらないかのどちらかで、変更は 1 つずつ監査ログに残る。ほかのページの設定は送らない。
- 保存のボタンは `FormActionBar` に置き、画面より長いときのためにバーを画面の下に固定する。範囲の外の数は欄のエラーにして送らない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |

| 保存 | その場。`FormActionBar`（保存のボタンの横）に結果を出す |
| 変えずに保存 | その場。`FormActionBar` に保存するものがないこと |
| サーバーが拒否 | その場。`FormActionBar`（`role="alert"`）に理由を出し、入力を残す |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「サーバーの設定」から、ページの間のリンクの「通話」。
- 出口: ページの間のリンクのほかのページ、管理のメニュー。
