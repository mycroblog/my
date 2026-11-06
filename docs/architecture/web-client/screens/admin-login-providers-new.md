---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.login-providers_.new.tsx", "app/web/src/features/admin/components/LoginProviderSettings.tsx", "app/web/src/features/admin/components/loginProviderParts.tsx", "app/web/src/features/admin/hooks/useLoginProviders.ts", "app/web/src/features/admin/api/loginProviders.ts"]
---

## プロバイダーを追加（`/admin/login-providers/new`）

`manage_login_providers` を持つ管理者が、外部ログインのプロバイダーを設定するページ（[ADR-0106](../../../adr/0106-external-login-providers.md)、[§14](../14-admin.md)）。
設定するとすぐ有効になるので、ログインプロバイダーだけのログインがそれで始まるときは、送る前に警告のダイアログで確かめる。

### 1. 使う人と頻度

- 管理者。外部ログインを始めるとき、プロバイダーを足すとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| GitHub のログインプロバイダーを追加する（クライアント ID は demo-id、シークレットは demo-secret） | 7 |

手順は、メニューで一覧を開く 1 つ、「プロバイダーを追加」を押す 1 つ、種類の「GitHub」を選ぶ 1 つ、表示名・クライアント ID・クライアントシークレットを入れる 3 つ、「設定する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

1. 見出し（`h1`「プロバイダーを追加」）と「ログインプロバイダーの一覧へ戻る」。
2. フォーム: 種類（`RadioGroup`: OpenID Connect・GitHub・Discord・Mastodon）と、種類に応じた項目（表示名・Issuer かサーバー・クライアント ID・クライアントシークレット・追加の scope・「新規登録を許可する」と「アカウントへのひも付けを許可する」・条件）。

### 4. 主な操作と保存の仕方

- 主な操作は「設定する」。フォームはまとめて保存（`FormActionBar`）。種類が取らない項目は送らない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 設定する | 画面の遷移。設定したプロバイダーのページ。そこにコールバック URL を IdP に登録することの説明とコピーのボタンがある |
| ログインプロバイダーだけのログインが始まる | その場。警告の `ConfirmDialog`（ログインできなくなる人の数と、自分が含まれるか）。確かめたら送る |
| 条件の行の片方だけ・サーバーの拒否（422） | その場。その項目に理由を出し、入力を残す |

### 6. 入口と出口

- 入口: 一覧の「プロバイダーを追加」。
- 出口: 設定したプロバイダーのページ、「ログインプロバイダーの一覧へ戻る」。
