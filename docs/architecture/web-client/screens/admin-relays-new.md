---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.relays_.new.tsx", "app/web/src/features/admin/components/RelaySettings.tsx", "app/web/src/features/admin/hooks/useRelays.ts", "app/web/src/features/admin/api/relays.ts"]
notes: {"`https://social-relay.example/inbox` のリレーに参加する": "the demo server reaches no relay: the relay stays awaiting its answer"}
---

## リレーに参加する（`/admin/relays/new`）

`manage_settings` を持つ管理者が、連合のリレーを登録して参加を求めるページ（[ADR-0006](../../../adr/0006-federation-protocol-scope.md)、[§14](../14-admin.md)）。
[リレーの一覧](admin-relays.md)の「リレーに参加する」から来る。

### 1. 使う人と頻度

- 管理者。リレーを足すとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| `https://social-relay.example/inbox` のリレーに参加する | 4 |

手順は、メニューで一覧を開く 1 つ、「リレーに参加する」を押す 1 つ、URL を入れる 1 つ（方式は既定の「自動で判定」、受信は既定で受け取る）、「参加する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 見出し（`h1`）と「リレーの一覧へ戻る」。
2. フォーム: 公開投稿だけを送ることの説明、URL（必須、https、2048 バイトまで）、方式（`Select`。「自動で判定」・Mastodon・LitePub）、リレーから投稿を受け取るか（`Checkbox`、既定は受け取る）。

### 4. 主な操作と保存の仕方

- 主な操作は「参加する」。フォームはまとめて保存（`FormActionBar`）。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 参加する | 画面の遷移。登録したリレーのページ（見出しが結果を示す） |
| サーバーが URL を拒否（422 の `url`・`exists`・`domain_suspended`・`not_actor`） | その場。URL の欄に理由を出し、入力を残す |
| ほかの失敗 | その場。ボタンの隣（`role="alert"`）。入力を残す |

### 6. 入口と出口

- 入口: リレーの一覧の「リレーに参加する」。
- 出口: 参加したリレーのページ、「リレーの一覧へ戻る」。
