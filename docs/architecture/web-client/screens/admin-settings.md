---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.settings.tsx", "app/web/src/features/admin/components/ServerSettings.tsx", "app/web/src/features/admin/hooks/useServerGeneral.ts", "app/web/src/features/admin/api/settings.ts", "app/ui/src/LocalNav.tsx"]
---

## サーバーの設定: 全般（`/admin/settings`）

`manage_settings` を持つ管理者が、サーバーの名前・説明・連絡先とサムネイルを決めるページ（FR-705、[§14](../14-admin.md)）。サーバーの設定の画面の最初のページで、ほかのページ（[アイコンとロゴ](admin-settings-branding.md)・[登録](admin-settings-registration.md)・[コンテンツ](admin-settings-content.md)・[連合](admin-settings-federation.md)・[検索とクローラー](admin-settings-discovery.md)・[通話](admin-settings-voice.md)）は子のルート。
画面の名前の `h1` の下に、ページの間のリンクの並び（`LocalNav`。「全般」「アイコンとロゴ」「登録」「コンテンツ」「連合」「検索とクローラー」、サーバーが通話を出しているときだけ「通話」）を置き、今のページを `aria-current="page"` にする。管理のメニューは「サーバーの設定」の 1 項目のままで、どのページでも今いる項目になる。

### 1. 使う人と頻度

- サーバーを立ち上げた管理者。最初に名前と説明を決める。立ち上げのときに 1 回。
- 運営を続ける管理者。連絡先や説明を変える。月に数回まで。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| サーバーの名前を変える | 3 |
| 短い説明を変える | 3 |
| 連絡先のメールアドレスを変える | 3 |

手順は、管理のメニューの「サーバーの設定」を開く 1 つ、欄を変える 1 つ、「保存する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 名前: 名前の欄（名前がドメインの代わりに出ることの説明）。
2. 説明: 短い説明と説明。
3. 連絡先とサムネイル: 連絡先のアカウントとメールアドレス、サムネイル（`h3`）と画像の説明。

### 4. 主な操作と保存の仕方

- 主な操作は「保存する」（ページに 1 つ、`primary`）。3 つの区画は 1 つのフォームで、保存はまとめて保存だけ。
  変えた設定（`contact_account`・`contact_email`・`thumbnail`）を `PATCH /admin/settings` でまとめて送り、続けて変えた文（名前・説明・画像の説明）を `PUT /admin/instance_texts/{key}` で送る。
- 画像は選んだとたんにアップロードするが、サムネイルになるのは保存したとき。
- 保存のボタンは `FormActionBar` に置き、画面より長いときのためにバーを画面の下に固定する。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 保存 | その場。`FormActionBar`（保存のボタンの横）に結果を出す。外枠の名前・ランディングページ・サーバーの画面は `GET /instance` を取り直して新しい値を出す |
| 変えずに保存 | その場。`FormActionBar` に保存するものがないこと |
| 101 文字以上か空白だけの名前、上限を超えた説明で保存 | その場。欄のエラーにして送らない |
| 画像を選ぶ | その場。アップロードの間はボタンが待ちの状態になり、終わるとプレビューが出る。失敗はボタンの下（`role="alert"`） |
| サーバーが拒否 | その場。`FormActionBar`（`role="alert"`）に理由を出し、入力を残す |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「サーバーの設定」。
- 出口: ページの間のリンクのほかのページ、管理のメニュー。保存した名前は、外枠の先頭・ランディングページ・サーバーの画面（ログインなど）で確かめられる。
