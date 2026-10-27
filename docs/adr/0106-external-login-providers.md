# ADR-0106: 管理者が設定する外部 IdP でのログイン

- Status: Proposed
- Date: 2026-10-27
- Deciders: @rosylilly
- Issue: #1576

## Context（背景・課題）

いまのログイン手段は、メールアドレスとパスワード（FR-002）、パスキー（FR-004）、その上の TOTP（FR-005）だけである。
GitHub や Google、組織の IdP（Keycloak・Authentik・Microsoft Entra など）のアカウントでログイン・登録したいという要望に応える手段がない。
管理者が再起動なしに IdP を足せるようにし、利用者がそれでログイン・登録・既存アカウントへのひも付けをできるようにする。
該当する FR がないので、FR-010（管理者が設定する外部 IdP でのログインと登録）を足す。

これは [ADR-0090](0090-third-party-oauth-apps.md) の逆向きである。ADR-0090 はこのサーバーを OAuth の認可サーバーにする決定で、
本 ADR はこのサーバーを外部 IdP のクライアント（Relying Party）にする決定である。

この決定は、既存の Accepted ADR の次の規約の上に立つ。

- ログインと認可同意はサーバーが返すページ（`internal/interface/authpages`）で行い、Cookie のセッションを作る。Web クライアントはそのあと
  自前の `/oauth/authorize` でトークンを得る（ADR-0019 §4、ADR-0030）。ログイン時にセッションを作り直す（セッション固定攻撃対策）。
- 外部への HTTP は SSRF ガード付きの `kernel.HTTPClient` を通す（ADR-0019 §1）。内部の宛先は運営者の設定の許可リストでだけ許す先例がある
  （ADR-0060 の `webhook_allowed_destinations`）。
- 秘密情報は `kernel.Sealer` で封をして DB に置く（ADR-0019 §6）。管理者が設定する宛先と秘密は、リソース・版・封をした秘密・状態の変化・投影で持つ
  （ADR-0060 のデータモデル、ADR-0020）。
- 管理操作は監査ログに残す（ADR-0037）。権限は `internal/domain/identity` の `Permission` で表す（ADR-0058、ADR-0070）。
- 利用者は最後のログイン手段を失えない（`ErrLastSignInMethod`、FR-004 の実装）。
- 設定ファイルとインスタンス設定は、再起動なしに変えるかどうかで分ける（ADR-0010、`docs/operations/instance-settings.md`）。

主要な IdP の性質（2026-10-27 に公式文書で確認）:

| | OpenID Connect の IdP（Google・GitLab・Keycloak など） | GitHub（OAuth App） | Mastodon（4.3 以降） |
| --- | --- | --- | --- |
| 設定の発見 | `<issuer>/.well-known/openid-configuration`（OpenID Connect Discovery 1.0） | なし。エンドポイントは固定（`https://github.com/login/oauth/authorize` など） | `/.well-known/oauth-authorization-server`（RFC 8414）。4.3 より前は 404 |
| クライアントの登録 | IdP の管理画面で登録する | GitHub の設定で登録する | `POST /api/v1/apps` で認証なしに登録できる（サーバーの設定の開発の画面でも登録できる） |
| 利用者の識別子 | ID トークンの `sub`。`iss` と組で一意（OpenID Connect Core 1.0 §2） | `GET /user` の数値の `id`。`login`（ユーザー名）は変えられる | `GET /api/v1/accounts/verify_credentials` の `id`。`profile` scope で読める。4.4 からは `/oauth/userinfo` もある |
| ID トークン | 発行する。JWKS の鍵で署名を検証する | 発行しない（OIDC の Provider ではない） | 発行しない（OIDC の Provider ではない） |
| メールアドレス | `email` と `email_verified` の claim（`email` scope） | `GET /user/emails` の `primary` かつ `verified` のもの。要る scope は `user:email`（要検証） | 得られない |
| PKCE | 多くが対応。Discovery の `code_challenge_methods_supported` で分かる | `S256` に対応（2025-07 から。必須ではない）。GitHub Enterprise Server は版によって未対応 | メタデータの `code_challenge_methods_supported` で分かる |

出典: [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)、
[OpenID Connect Discovery 1.0](https://openid.net/specs/openid-connect-discovery-1_0.html)、
[Authorizing OAuth apps（GitHub Docs）](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps)、
[PKCE support for OAuth and GitHub App authentication（GitHub Changelog, 2025-07-14）](https://github.blog/changelog/2025-07-14-pkce-support-for-oauth-and-github-app-authentication/)、
[REST API endpoints for emails（GitHub Docs）](https://docs.github.com/en/rest/users/emails)、
[OAuth API methods（Mastodon documentation）](https://docs.joinmastodon.org/methods/oauth/)、
[OAuth Scopes（Mastodon documentation）](https://docs.joinmastodon.org/api/oauth-scopes/)、
[Mastodon 4.4 for Developers（Mastodon Blog）](https://blog.joinmastodon.org/2025/07/mastodon-4-4-for-devs/)、
[OpenID Connect（Sign in with Google）](https://developers.google.com/identity/openid-connect/openid-connect)。

## Decision Drivers（判断基準）

1. アカウントを乗っ取られないこと: IdP の設定やメールアドレスの一致を足がかりに、他人のアカウントへログインできないこと
2. 管理の権限を持つ人（とその乗っ取られたアカウント）が、IdP の設定を内部ネットワークへの踏み台（SSRF）にできないこと
3. 再起動なしに、管理画面から IdP を足し、無効にし、外せること
4. 多くの IdP を少ない実装で扱えること（OIDC は標準に乗り、標準に乗らないものだけを個別に持つ）
5. 既存の認証の仕組み（セッション、TOTP、登録モード、レート制限、最後のログイン手段、ADR-0030 のトークン）をそのまま使えること
6. IdP を設定しないサーバーに費用も設定も増えないこと（NFR-COST）

## Considered Options（選択肢）

IdP の種類について:

1. **汎用の OIDC と、OIDC でない IdP のコード上のプリセット（まず GitHub と Mastodon）**
2. **汎用の OAuth 2.0**: 管理者が認可・トークン・userinfo のエンドポイントと、識別子・メールアドレスを取り出すフィールドを指定する
3. **プリセットだけ**: GitHub・Google など、コードに持つ IdP だけを選べる

設定の置き場所について:

- (a) DB（管理 API・管理画面・CLI で変える）
- (b) 設定ファイル（`MY_*` と設定ファイル。変えるには再起動が要る）

## Decision（決定）

> 推奨案である。Accepted にするのは人間の Decider のみ。

**IdP の種類は選択肢 1、置き場所は (a) を推奨する。**

### 1. IdP の種類

- `oidc`: 管理者は `issuer` を入れ、サーバーは Discovery で認可・トークン・JWKS のエンドポイントを得る。
  ID トークンは JWKS の鍵で署名を検証し、`iss`・`aud`・`exp`・`iat`・`nonce` を確かめる（OpenID Connect Core 1.0 §3.1.3.7）。
  `alg` は Discovery の `id_token_signing_alg_values_supported` のうち `RS256`・`ES256`・`EdDSA` だけを受け、`none` と HMAC は受けない。
  利用者の識別子は `sub` とする。
- `github`: エンドポイントはコードに固定する。識別子は `GET /user` の数値の `id`（文字列にして保存する）とし、`login` は表示にだけ使う。
  メールアドレスは `GET /user/emails` の `primary` かつ `verified` のものを使う。GitHub Enterprise Server は対象にしない。
- `mastodon`: 管理者が指定した 1 つの Mastodon サーバーを IdP にする。サーバーごとに 1 つの IdP を作り、利用者が任意のサーバーを入力する形は持たない。
  - 管理者はサーバーのドメインを入れる。サーバーは RFC 8414 のメタデータからエンドポイントを得て、`POST /api/v1/apps` でクライアントを登録し
    （`redirect_uris` は §2 のコールバック、scope は `profile`）、得た `client_id` と `client_secret` を保存する。相手のサーバーが登録を
    制限しているときは、管理者が相手の画面で登録した `client_id` と `client_secret` を入れる。
  - メタデータがない（4.3 より前の）サーバーは断る。`profile` より広い scope（`read:accounts` など）は求めない。
  - 識別子は `verify_credentials` の `id` とし、`acct` と `display_name` は表示と登録画面の初期値にだけ使う。メールアドレスは得られないので、
    この IdP からの登録は既存のメール検証（FR-002）を求める。
  - 相手のサーバーのドメインに処分（FR-702）がかかっている間は、その IdP でのログイン・登録・ひも付けを断る。
- どれも Authorization Code フローで、`state` と PKCE（`S256`）を使う。`oidc` は `nonce` も使う。
  Discovery が `S256` を示さない IdP でも PKCE のパラメータは送る（受けない IdP は無視する）。
- ほかの OIDC でない IdP（Discord など）を足すときは、`github` や `mastodon` と同じく、識別子とメールアドレスの取り出し方をコードに書いたプリセットとして足す。
  管理者がエンドポイントとフィールドを指定する汎用の OAuth 2.0 は持たない。

### 2. 管理と保存

- IdP は DB に置き、identity モジュールが所有する。テーブルは ADR-0060 の webhook と同じ形にする（名前は data-model に書くときに決める）。
  - リソース: IdP（ID、作った人、作った時刻）
  - イベント（版）: 種類（`oidc` / `github` / `mastodon`）、表示名、`issuer`（`mastodon` ではサーバーの URL）、`client_id`、追加の scope、ポリシー（§4）。最新の版が現在の値
  - イベント: `client_secret`。`kernel.Sealer` で封をする。回転は行を足す
  - イベント: 有効化・無効化・削除
  - 投影: 現在の版・秘密・状態
- 管理 API（`/api/v1/admin/login_providers`）、管理画面、CLI（`my login-provider`）で作り、変え、無効にし、消す。
  CLI は Web の管理画面へ入る前の最初の設定にも使える。
- 権限は新しい `manage_login_providers` とし、既定では組み込みの `admin` ロールだけが持つ。IdP を足せる人は、その IdP で誰を登録させるかを
  決められ、`manage_settings` より重いからである。操作はすべて監査ログに残す。`client_secret` は作ったときと回転したときに入力を受けるだけで、
  API でも画面でも返さない。
- コールバックの URL は `https://<host>/auth/external/<IdP の ID>/callback` に固定し、管理画面に表示する。管理者はこれを IdP 側に登録する（`mastodon` はサーバーが §1 の登録で渡す）。
- 有効な IdP が 1 つもなければ、ログイン画面は外部ログインのボタンを出さず、外部への要求も起きない。

### 3. ログインの流れ

1. `GET /auth/external/<id>/start`: `state`、`nonce`、PKCE の `code_verifier` と、戻り先（既存の `return_to` の扱いに従う）を封をした短命の
   `__Host-` Cookie に入れ、IdP の認可エンドポイントへリダイレクトする。
2. `GET /auth/external/<id>/callback`: Cookie を開いて `state` を照合し（一度だけ使える）、コードをトークンに交換し、§1 の方法で識別子を得る。
   IdP のアクセストークンとリフレッシュトークンは保存しない。
3. `(IdP の ID, 識別子)` にひも付いた利用者がいれば、その利用者でログインする。既存のログインと同じく、IP ごとのレート制限、
   停止中のアカウントの拒否、セッションの作り直し、ログイン元の記録を通す。
   **TOTP を設定した利用者には、外部ログインのあとも第二要素を求める**（パスワードのログインと同じ challenge の流れ）。
   第二要素は IdP の多要素認証の有無にかかわらず、このサーバーのアカウントの設定に従う。
4. ひも付いた利用者がいなければ、IdP の `allow_signup` と登録モード（FR-001）が許すときだけ、登録の画面に進む。
   ユーザー名・規約への同意・（設定していれば）CAPTCHA を求め、`approval` なら承認待ち、`invite` なら招待コードを求め、`closed` なら断る。
   IdP が検証済みと示したメールアドレス（`email_verified` が真、または GitHub の `verified`）は検証済みとして登録し、それ以外は既存のメール検証を求める。
   登録の結果は、パスワードを持たず外部のひも付けだけを持つ利用者になる。
5. **メールアドレスが一致しても、既存のアカウントに自動でひも付けない。** ひも付けは、ログインした利用者がアカウント設定から、既存の確認
   （パスワードかパスキー）を済ませて IdP を経由したときだけ作る。ひも付けた識別子がすでに別の利用者にあれば断る。
6. 利用者はひも付けを一覧して外せる。外すことで最後のログイン手段がなくなるなら断る（`ErrLastSignInMethod` の判定に外部のひも付けを数える）。

### 4. IdP ごとのポリシー

- `allow_signup`（既定は偽）: その IdP から新しく登録できるか
- `allow_link`（既定は真）: 既存のアカウントにひも付けられるか
- 利用者を絞る条件（任意）。登録とひも付けだけでなく、ログインのたびに確かめ、満たさなければ断る。
  - `oidc`: ID トークンの claim と、許す値のリストの組。例えば Google Workspace の組織の人だけに絞るには `hd`（組織のドメイン）を使う。
    Google は、組織に属するかをメールアドレスのドメインで判断せず `hd` で確かめるよう求めているので、メールアドレスのドメインでは絞らない。
    認可要求にも同じ値を `hd` として付けるが、これはアカウントを選ぶ画面の補助で、判断は ID トークンの claim で行う。
  - `github`: 所属する Organization の許可リスト（`read:org` scope）
  - `mastodon`: 条件を持たない（IdP にしたサーバーの利用者すべてが対象）

### 5. IdP への HTTP

- Discovery、JWKS、トークンの交換、GitHub の API は、新しい用途 `kernel.HTTPPurposeLoginProvider` で `kernel.HTTPClient` を通す。
  リダイレクトはたどらず、応答の大きさに上限を置く。Discovery と JWKS は応答のキャッシュ指示に従い、上限を置いて使い回す。
  JWKS にない `kid` の ID トークンを受けたときは、間隔を空けて 1 度だけ取り直す。
- プライベートネットワーク上の IdP は、運営者の設定 `login_provider_allowed_destinations`（ホストか CIDR の許可リスト）にあるときだけ許す。
  管理の権限だけでは内部に届かない（ADR-0060 の (c) と同じ）。

### 6. 表示と i18n

- ログイン画面と登録画面のボタンの文言（「GitHub でログイン」など）は `locales/en`・`locales/ja` に置く。`oidc` のボタンは管理者が付けた表示名を
  そのまま出し、翻訳しない。
- アカウント設定に「ひも付けたアカウント」、管理画面に IdP の一覧と編集を足す（docs/architecture/web-client/ に従う）。

### この推奨を覆す条件

- 対象にしたい IdP の多くが OIDC でも GitHub 型のプリセットでも扱えず、プリセットを足し続けるほうが汎用の OAuth 2.0 より重いと分かった場合は、
  識別子のフィールドを固定の候補から選ぶ形の汎用 OAuth 2.0 を検討する。
- 運営者から、IdP の設定を Infrastructure as Code で配りたいという要望が強い場合は、設定ファイルから DB へ流し込む CLI（`my login-provider apply`）を足す。

## Consequences（影響）

- 良い影響
  - Google・GitLab・Microsoft Entra・Keycloak・Authentik など OIDC の IdP を、コードを足さずに管理画面から設定できる。GitHub も使える。
  - 姉妹インスタンスなど、管理者が信頼する Mastodon サーバーのアカウントでログイン・登録させられる。管理者はドメインを入れるだけで済む。
  - 識別子だけでひも付け、メールアドレスで自動ではひも付けないので、IdP 側でメールアドレスを詐称されても既存のアカウントは乗っ取られない。
  - 外部ログインは Cookie のセッションを作るところで終わるので、クライアント API とトークン（ADR-0030、ADR-0090）は変わらない。
  - IdP を設定しないサーバーには、行も外部への要求も増えない。
- 悪い影響・トレードオフ
  - 管理者が IdP を無効にしたり消したりすると、その IdP でしかログインしない利用者はログインできなくなる。救済はパスワードのリセット
    （FR-002）と管理者が発行するリンク（`manage_login_recovery`）に頼る。運用文書にこの影響を書き、管理画面は無効にする前にひも付いた利用者の数を示す。
  - IdP の多要素認証と、このサーバーの TOTP が二重になる利用者がいる。
  - JWT の検証と OIDC のクライアントのために依存を足す見込みがある（ライセンスの確認が要る）。
  - SAML と CAS の IdP は扱わない。
- 後続で必要になる作業（Issue に分ける）
  - FR-010 の追記、data-model §4.1 と `schema/<dialect>/01-identity.sql` の IdP とひも付けのテーブル
  - `manage_login_providers` の権限、監査ログの `action` と `target_kind`、管理 API（`api/spec`）・CLI・管理画面
  - `kernel.HTTPPurposeLoginProvider`、`login_provider_allowed_destinations` の設定、OIDC の Discovery・JWKS・ID トークンの検証
  - authpages のログイン・登録・コールバックの流れ、GitHub と Mastodon のプリセット
  - アカウント設定のひも付けの一覧・追加・解除、`ErrLastSignInMethod` の判定の拡張
  - 運用文書 `docs/operations/login-providers.md` と `tools/agentkit/context-map.yaml` の更新

## Pros and Cons of the Options（詳細比較・任意）

### 1. 汎用の OIDC とプリセット（推奨）

- 良い: OIDC の IdP は Discovery と ID トークンの検証という標準の手順で扱え、どの claim が識別子かで迷わない（Driver 1, 4）。
  標準に乗らない IdP の扱いはコードにあり、テストされている。
- 悪い: OIDC でない IdP を足すたびにコードの変更とリリースが要る。

### 2. 汎用の OAuth 2.0

- 良い（最も強い論点）: どの OAuth 2.0 のサービスでも、リリースを待たずに管理者が足せる。Discord・Slack・Mastodon などをすぐ使える。
- 悪い: 識別子のフィールドを管理者が選ぶので、変えられる値（ユーザー名やメールアドレス）を選ぶと、別人が同じ値を名乗って既存の利用者として
  ログインできる（Driver 1）。ID トークンの署名のような応答の検証がなく、userinfo の応答をそのまま信じることになる。

### 3. プリセットだけ

- 良い: 管理者の入力が最も少なく、IdP ごとの癖をコードで吸収できる。
- 悪い: 組織の Keycloak・Authentik など、自前の IdP を使えない。身内向けのサーバーという主要な使い方を満たさない（Driver 4）。

### 置き場所の (a)・(b)

- (a) DB（推奨）: 再起動なしに足せ（Driver 3）、変更が監査ログに残る。秘密は `kernel.Sealer` で封をする。
- (b) 設定ファイル: 秘密を DB に置かず、Infrastructure as Code で配れる。ただし、変えるたびに再起動が要り、管理画面から設定できないので
  決定者の要望（管理者が増やせる）を満たさない。

## Open Questions / 要検証

- GitHub の `GET /user/emails` に要る scope が `user:email` で足りるか（現行の文書の記述が一覧の操作について明確でない）
- Go の OIDC クライアントと JWT の検証に使うライブラリ（`github.com/coreos/go-oidc/v3` と `golang.org/x/oauth2` など）のライセンスと、
  `kernel.HTTPClient` を差し込めるか。標準ライブラリだけで書くほうが小さく済むか
- 外部ログインだけの利用者が、アカウントの削除や移転（FR-008, FR-009）の確認に何を使うか（IdP での再認証で足りるか、パスワードかパスキーの設定を求めるか）
