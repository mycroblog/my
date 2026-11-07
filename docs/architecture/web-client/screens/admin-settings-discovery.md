---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.settings_.discovery.tsx", "app/web/src/features/admin/components/PolicySettings.tsx", "app/web/src/features/admin/hooks/useServerPolicies.ts", "app/web/src/features/admin/api/settings.ts", "app/web/src/features/admin/components/ServerSettings.tsx", "app/ui/src/LocalNav.tsx", "app/web/src/lib/fullTextSearch.ts"]
---

## サーバーの設定: 検索とクローラー（`/admin/settings/discovery`）

`manage_settings` を持つ管理者が、検索エンジンと AI のクローラーに示すこと、セキュリティの連絡先、投稿の検索が対象にする投稿を決めるページ（[ADR-0149](../../../adr/0149-full-text-search.md) §5・§16、[§14](../14-admin.md)）。投稿の検索の区画は、サーバーに全文検索があるとき（`usePostSearchAvailable`）だけ出す。
画面の名前の `h1` の下に、ページの間のリンクの並び（`LocalNav`。「全般」「アイコンとロゴ」「登録」「コンテンツ」「投稿の自動削除」「連合」「検索とクローラー」、サーバーが通話を出しているときだけ「通話」）を置き、今のページを `aria-current="page"` にする。管理のメニューは「サーバーの設定」の 1 項目のままで、どのページでも今いる項目になる。

### 1. 使う人と頻度

- サーバーを立ち上げた管理者。最初にクローラーへの方針を決める。立ち上げのときに 1 回。
- 運営を続ける管理者。全文検索を入れたときに検索の対象を決める。年に数回まで。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 検索エンジンに索引を許す | 4 |
| 投稿の検索の対象を公開の投稿と「このサーバー」の投稿の両方にする | 4 |

手順は、管理のメニューの「サーバーの設定」を開く 1 つ、ページの間のリンクで「検索とクローラー」を開く 1 つ、欄を変える 1 つ、「設定を保存する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 検索エンジンと AI: 索引を許すか（`RadioGroup`）、新しいアカウントが索引を許すか（チェックボックス）、AI の学習と利用（`RadioGroup`）、セキュリティの連絡先（`TextField`）。
2. 投稿の検索（サーバーに全文検索があるときだけ）: `search_visibilities` を公開の投稿・公開範囲が「このサーバー」の投稿・その両方から選ぶ。`unlisted`・`private`・`direct` は出さず、空も選べない。狭めるとその投稿が検索から消えること、広げても過去の投稿は索引を再構築するまで見つからないことを言う。

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

- 入口: 管理のモードのメニューの「サーバーの設定」から、ページの間のリンクの「検索とクローラー」。
- 出口: ページの間のリンクのほかのページ、管理のメニュー。
