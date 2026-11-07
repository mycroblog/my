---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.settings_.registration.tsx", "app/web/src/features/admin/components/PolicySettings.tsx", "app/web/src/features/admin/hooks/useServerPolicies.ts", "app/web/src/features/admin/api/settings.ts", "app/web/src/features/admin/components/ServerSettings.tsx", "app/ui/src/LocalNav.tsx"]
---

## サーバーの設定: 登録（`/admin/settings/registration`）

`manage_settings` を持つ管理者が、登録できる人、ログインと登録を IdP だけにするか、利用者が作る招待とその上限、アプリの登録の経路を決めるページ（FR-001、[ADR-0106](../../../adr/0106-external-login-providers.md) §14・§15、[ADR-0090](../../../adr/0090-third-party-oauth-apps.md) §1、[§14](../14-admin.md)）。
画面の名前の `h1` の下に、ページの間のリンクの並び（`LocalNav`。「全般」「アイコンとロゴ」「登録」「コンテンツ」「投稿の自動削除」「連合」「検索とクローラー」、サーバーが通話を出しているときだけ「通話」）を置き、今のページを `aria-current="page"` にする。管理のメニューは「サーバーの設定」の 1 項目のままで、どのページでも今いる項目になる。

### 1. 使う人と頻度

- サーバーを立ち上げた管理者。最初に登録のモードを決める。立ち上げのときに 1 回。
- 運営を続ける管理者。登録のモードや招待の上限を変える。月に数回まで。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 登録のモードを変える | 4 |
| 利用者が招待を作れるようにする | 4 |
| 招待 1 つで登録できる人数の上限を変える | 4 |

手順は、管理のメニューの「サーバーの設定」を開く 1 つ、ページの間のリンクで「登録」を開く 1 つ、欄を変える 1 つ、「設定を保存する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 登録: 登録のモード（`RadioGroup`）と、ログインと登録を IdP だけにする設定（チェックボックス）。
2. 利用者の招待: 招待を作れる人（`RadioGroup`）、1 人が持てる使える招待の数と招待 1 つで登録できる人数（`inputMode="numeric"` の `TextField`）、招待が使える最長の期間（`Select`）。
3. アプリ: 利用者がアプリを登録するか、Client ID Metadata Document、Dynamic Client Registration（チェックボックス）。

### 4. 主な操作と保存の仕方

- 主な操作は「設定を保存する」（ページに 1 つ、`primary`）。保存は、まとめて保存だけ。このページの設定のうち変えたものだけを `PATCH /admin/settings` でまとめて送り、すべて通るか何も変わらないかのどちらかで、変更は 1 つずつ監査ログに残る。ほかのページの設定は送らない。
- 保存のボタンは `FormActionBar` に置き、画面より長いときのためにバーを画面の下に固定する。範囲の外の数は欄のエラーにして送らない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| ログインを IdP だけにする設定（`login_providers_only`）をオンにして保存する | `alertdialog` で確かめる。ログインに使える IdP へのひも付けのない有効な利用者の数（`GET /admin/settings/login_providers_only/impact`）、管理者自身が含まれるならそのこと、止まるものと続くもの、元に戻す CLI のコマンドを言う。断らない。オフにするときは聞かない |
| 保存 | その場。`FormActionBar`（保存のボタンの横）に結果を出す |
| 変えずに保存 | その場。`FormActionBar` に保存するものがないこと |
| サーバーが拒否 | その場。`FormActionBar`（`role="alert"`）に理由を出し、入力を残す |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「サーバーの設定」から、ページの間のリンクの「登録」。
- 出口: ページの間のリンクのほかのページ、管理のメニュー。
