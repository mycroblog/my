# ADR-0106: 登録とログインの方法: 外部 IdP・CAPTCHA・招待・IdP だけの制限

- Status: Accepted
- Date: 2026-10-27
- Deciders: @rosylilly
- Issue: #1576, #490, #1617, #1955

## Decision（決定）

メールアドレスとパスワード（FR-002）とパスキー（FR-004）に加えて、次を決める。管理者が設定する外部 IdP でのログインと登録（§1〜§6）、サインインと登録のボット対策の CAPTCHA（§7〜§13）、利用者による招待の作成と招待の使える登録モード（§14〜§19）、ログインと登録を外部 IdP だけに制限する設定（§20〜§25）。

### 1. IdP の種類

- `oidc`: 管理者は `issuer` を入れ、サーバーは Discovery で認可・トークン・JWKS のエンドポイントを得る。
  ID トークンは JWKS の鍵で署名を検証し、`iss`・`aud`・`exp`・`iat`・`nonce` を確かめる（OpenID Connect Core 1.0 §3.1.3.7）。
  `alg` は Discovery の `id_token_signing_alg_values_supported` のうち `RS256`・`ES256`・`EdDSA` だけを受け、`none` と HMAC は受けない。
  利用者の識別子は `sub` とする。
- `github`: エンドポイントはコードに固定する。識別子は `GET /user` の数値の `id`（文字列にして保存する）とし、`login` は表示にだけ使う。
  メールアドレスは `GET /user/emails` の `primary` かつ `verified` のものを使い、そのために `user:email` scope を求める（`GET /user` に scope は要らない）。
  `read:org` は Organization の条件（§4）を設定した IdP だけが加える。GitHub Enterprise Server は対象にしない。
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
- `discord`: discord.com のエンドポイントはコードに固定する。識別子は数値の Discord ID。
- ほかの OIDC でない IdP を足すときは、`github`・`mastodon`・`discord` と同じく、識別子とメールアドレスの取り出し方をコードに書いたプリセットとして足す。
  管理者がエンドポイントとフィールドを指定する汎用の OAuth 2.0 は持たない。

### 2. 管理と保存

- IdP は DB に置き、identity モジュールが所有する。テーブルは ADR-0060 の webhook と同じ形にする（名前は data-model）。
  - リソース: IdP（ID、作った人、作った時刻）
  - イベント（版）: 種類（`oidc` / `github` / `mastodon` / `discord`）、表示名、`issuer`（`mastodon` ではサーバーの URL）、`client_id`、追加の scope、ポリシー（§4）。最新の版が現在の値
  - イベント: `client_secret`。`kernel.Sealer` で封をする。回転は行を足す
  - イベント: 有効化・無効化・削除
  - 投影: 現在の版・秘密・状態
- 管理 API（`/api/v1/admin/login_providers`）、管理画面、CLI（`my login-provider`）で作り、変え、無効にし、消す。
  CLI は Web の管理画面へ入る前の最初の設定にも使える。
- 権限は `manage_login_providers` とし、既定では組み込みの `admin` ロールだけが持つ。IdP を足せる人は、その IdP で誰を登録させるかを
  決められ、`manage_settings` より重いからである。操作はすべて監査ログに残す。`client_secret` は作ったときと回転したときに入力を受けるだけで、
  API でも画面でも返さない。
- コールバックの URL は `https://<host>/auth/external/<IdP の ID>/callback` に固定し、管理画面に表示する。管理者はこれを IdP 側に登録する（`mastodon` はサーバーが §1 の登録で渡す）。
- 有効な IdP が 1 つもなければ、ログイン画面は外部ログインのボタンを出さず、外部への要求も起きない。

### 3. ログインの流れ

#### 3.1 開始

`GET /auth/external/<id>/start`: `state`、`nonce`、PKCE の `code_verifier` と、戻り先（既存の `return_to` の扱いに従う）を封をした短命の
`__Host-` Cookie に入れ、IdP の認可エンドポイントへリダイレクトする。

#### 3.2 コールバック

`GET /auth/external/<id>/callback`: Cookie を開いて `state` を照合し（一度だけ使える）、コードをトークンに交換し、§1 の方法で識別子を得る。
IdP のアクセストークンとリフレッシュトークンは保存しない。

#### 3.3 ひも付いた利用者のログイン

`(IdP の ID, 識別子)` にひも付いた利用者がいれば、その利用者でログインする。既存のログインと同じく、IP ごとのレート制限、
停止中のアカウントの拒否、セッションの作り直し、ログイン元の記録を通す。
**TOTP を設定した利用者には、外部ログインのあとも第二要素を求める**（パスワードのログインと同じ challenge の流れ）。
第二要素は IdP の多要素認証の有無にかかわらず、このサーバーのアカウントの設定に従う。

#### 3.4 新しい登録

ひも付いた利用者がいなければ、IdP の `allow_signup` と登録モード（FR-001）が許すときだけ、登録の画面に進む。
ユーザー名・規約への同意・（設定していれば）CAPTCHA を求め、`approval` で招待がなければ承認待ち、`invite` なら招待コードを求め、`closed` なら断る（招待の扱いは §17）。
IdP が検証済みと示したメールアドレス（`email_verified` が真、または GitHub の `verified`）は検証済みとして登録し、それ以外は既存のメール検証を求める。
登録の結果は、パスワードを持たず外部のひも付けだけを持つ利用者になる。

#### 3.5 既存のアカウントへのひも付け

**メールアドレスが一致しても、既存のアカウントに自動でひも付けない。** ひも付けは、ログインした利用者がアカウント設定から、既存の確認
（§3.7 の `Confirmation`）を済ませて IdP を経由したときだけ作る。ひも付けた識別子がすでに別の利用者にあれば断る。

#### 3.6 ひも付けの一覧と解除

利用者はひも付けを一覧して外せる。外すことで最後のログイン手段がなくなるなら断る（`ErrLastSignInMethod` の判定に外部のひも付けを数える）。

#### 3.7 本人確認

本人確認（`Confirmation`。パスキーの登録、パスワードの変更と削除、アカウントの削除と移転、ひも付けの追加など）に、パスワードとパスキーに加えて
**ひも付けた IdP での再認証** を足す。サーバーは確認する操作を封をした Cookie に記録して IdP へ送り、戻ってきた識別子が確認する利用者の
ひも付けと一致したときに、その確認を一度だけ使えるものとして通す。誤った再認証は、パスワードやパスキーの誤りと同じ制限に数える。

- `oidc`: `max_age=0` と `prompt=login` を付け、ID トークンの `auth_time` が送った時刻より後であることを確かめる。
- `mastodon`: `force_login=true` を付ける。
- `github`: 再入力を強いる手段がないので、戻ってきたことは GitHub にログインしていることだけを示す。そのため、ほかの手段で確認できる利用者には
  画面で先にほかの手段を勧める。

### 4. IdP ごとのポリシー

- `allow_signup`（既定は偽）: その IdP から新しく登録できるか
- `allow_link`（既定は真）: 既存のアカウントにひも付けられるか
- 利用者を絞る条件（任意）。登録とひも付けだけでなく、ログインのたびに確かめ、満たさなければ断る。
  - `oidc`: ID トークンの claim と、許す値のリストの組。例えば Google Workspace の組織の人だけに絞るには `hd`（組織のドメイン）を使う。
    Google は、組織に属するかをメールアドレスのドメインで判断せず `hd` で確かめるよう求めているので、メールアドレスのドメインでは絞らない。
    認可要求にも同じ値を `hd` として付けるが、これはアカウントを選ぶ画面の補助で、判断は ID トークンの claim で行う。
  - `github`: 所属する Organization の許可リスト（`read:org` scope）
  - `mastodon`: 条件を持たない（IdP にしたサーバーの利用者すべてが対象）
  - `discord`: 所属する Discord サーバー（guild）の許可リスト（`guilds` scope）

### 5. IdP への HTTP

- Discovery、JWKS、トークンの交換、GitHub の API は、新しい用途 `kernel.HTTPPurposeLoginProvider` で `kernel.HTTPClient` を通す。
  リダイレクトはたどらず、応答の大きさに上限を置く。Discovery と JWKS は応答のキャッシュ指示に従い、上限を置いて使い回す。
  JWKS にない `kid` の ID トークンを受けたときは、間隔を空けて 1 度だけ取り直す。
- Discovery・JWKS・ID トークンの検証は、ライブラリを足さず標準ライブラリ（`crypto/rsa`・`crypto/ecdsa`・`crypto/ed25519`）で書く
  （identity の `OIDCClient` ポートと `internal/adapter/oidc`）。`github.com/coreos/go-oidc/v3`（Apache-2.0、`github.com/go-jose/go-jose/v4` に依存）と
  `golang.org/x/oauth2`（BSD-3-Clause）はライセンスでは使えるが、`kernel.HTTPClient` は context に入れた `*http.Client` の `RoundTripper` に
  包み直さないと差し込めず、go-oidc の JWKS は応答のキャッシュ指示に従わず、`kid` が見つからないたびに取り直し、`nonce` と `auth_time` は呼び出し側で確かめる必要がある。
  この節の要件を満たすには結局ライブラリの外で書く部分が大きく、受ける `alg` が 3 つに限られるので、標準ライブラリだけのほうが小さく済む。
  トークンの交換も `golang.org/x/oauth2` を使わず、`kernel.HTTPClient` へのフォームの `POST` として書く。
- プライベートネットワーク上の IdP は、運営者の設定 `login_provider_allowed_destinations`（ホストか CIDR の許可リスト）にあるときだけ許す。
  管理の権限だけでは内部に届かない（ADR-0060 の `webhook_allowed_destinations` と同じ）。

### 6. 表示と i18n

- ログイン画面と登録画面のボタンの文言（「GitHub でログイン」など）は `locales/en`・`locales/ja` に置く。`oidc` のボタンは管理者が付けた表示名を
  そのまま出し、翻訳しない。
- アカウント設定に「ひも付けたアカウント」、管理画面に IdP の一覧と編集を置く（docs/architecture/web-client/ に従う）。

### 7. CAPTCHA: ポートとアダプター

- 対応するサービスは Turnstile・hCaptcha・reCAPTCHA v2 で、運営者が選ぶ。既定は「使わない」。推奨は Turnstile（無料で上限がなく、多くの利用者に課題を出さない）。
- `internal/app/kernel` に `HumanVerifier` ポートを置く。`Verify(ctx, token, action string) error` だけを持ち、
  トークンが無効・期限切れ・使用済み・別のホスト名や別の操作で発行されたものなら `ErrHumanCheckFailed` を、
  サービスに届かない・タイムアウト・5xx・秘密鍵が無効（`invalid-input-secret`）のときは `ErrHumanCheckUnavailable` を包んで返す。
  CAPTCHA を使わないときは `HumanVerifier` を渡さない（nil）。
- アダプターは `internal/adapter/captcha/siteverify` の 1 つで、サービスごとの違い（検証 API の URL、フォームの欄の名前、スクリプトの URL、CSP に足すオリジン、言語の指定方法）を
  プリセットの表として持つ。`pkg/testkit` に `HumanVerifierContract` を置き、`httptest` の siteverify スタブで成功・失敗・期限切れ・ホスト名の不一致・5xx・タイムアウト・大きすぎる応答を検査する。
- ウィジェットの描画に要るもの（サービス、サイトキー、スクリプトの URL、欄の名前、CSP のオリジン）は値の型 `kernel.HumanCheckWidget` として、`internal/platform` が `authpages` に渡す。
- 対応しないもの: reCAPTCHA v3（スコアだけで課題を出せず、閾値で落とされた人に代わりの経路がない）と reCAPTCHA Enterprise の `createAssessment`（API キーとプロジェクトが要る別の API）。
  必要になったら同じポートの別アダプターとして ADR を書く。自前ホストの proof-of-work も同じポートで後から足せる。

### 8. CAPTCHA: 設定

デプロイの設定（[設定リファレンス](../reference/configuration.md)）に足す。インスタンスの設定（DB）には置かない。秘密鍵をインスタンスの設定に置かないため（ADR-0019 §6）。

| キー | 既定 | 必須 | 秘密 | 内容 |
| --- | --- | --- | --- | --- |
| `captcha` | `none` | no | no | `none` / `turnstile` / `hcaptcha` / `recaptcha`（v2 チェックボックス） |
| `captcha_site_key` | なし | `captcha` が `none` 以外のとき | no | ウィジェットに渡す公開のサイトキー |
| `captcha_secret_key` | なし | `captcha` が `none` 以外のとき | yes | siteverify に送る秘密鍵。`MY_CAPTCHA_SECRET_KEY` か `MY_CAPTCHA_SECRET_KEY_FILE` だけで受け取る |

- 「鍵が設定されていれば自動で有効にする」ことはしない。アダプターは常に明示的なキーで選ぶ（設定リファレンスの規約）。
  `captcha` を選んだのに鍵が欠けていれば `my config check` が問題として挙げ、サーバーは起動しない。
- 起動時に siteverify へ試しに問い合わせることはしない（起動を外部サービスに依存させない、Cloud Run / Lambda のコールドスタートを延ばさない）。
  秘密鍵が誤っていれば最初の検証で `invalid-input-secret` が返るので、ERROR のログを出し、`ErrHumanCheckUnavailable` として扱う（§10）。
- 秘密鍵は `slog.LogValuer` で `[REDACTED]` を出す型で持つ。

### 9. CAPTCHA: 検証する場所と順序

- 検証は `identity` のユースケース（`SignIn`・`SignUp`）の中で、トランザクションの外で行う（ADR-0003）。入力（`SignInInput`・`SignUpInput`）にトークンを足す。
  画面ではなくユースケースで検証するので、別の入口を足しても検査を忘れない。
- 順序: レート制限 → CAPTCHA の検証 → パスワードの照合（Argon2id）/ 登録の処理。外部の呼び出しより安いレート制限を先にし、高価なハッシュ計算をボットにさせない。
- CAPTCHA を有効にすると、サインインと登録の両方で必ず検証する。片方だけにかける設定は持たない。
- 有効な招待コードでの登録でも検証する。招待コードは漏れたり奪われたりしうるので、それだけでは人であることの証明にならない。
- 検証の失敗は、サインインでは IP のログイン失敗として数え（アカウントの失敗としては数えない。他人のアカウントをロックさせないため）、登録では登録の試行として数える。
- 応答の検査: `success` が真、`hostname` が設定の `host` と一致すること。Turnstile ではウィジェットに `data-action`（`sign_in` / `sign_up`）を付け、応答の `action` が一致することも検査する（あるフォームで得たトークンを別のフォームで使わせない）。
- `remoteip` は送らない。ブラウザはウィジェットを読むときにサービスへ直接つながっているので、サーバーから IP を重ねて渡さない（データの最小化）。hCaptcha には推奨どおり `sitekey` を送る。
- 外部 HTTP は `kernel.HTTPClient` を通す。用途 `HTTPPurposeHumanCheck` を足し、本文の上限 64 KiB、全体のタイムアウト 10 秒とする。
  トークンは 1 回限りなので再試行しない（Turnstile の `idempotency_key` も使わない）。

### 10. CAPTCHA: 検証できないときの扱い

| 場面 | トークンが不正（`ErrHumanCheckFailed`） | サービスに届かない・秘密鍵が無効（`ErrHumanCheckUnavailable`） |
| --- | --- | --- |
| 登録 | 拒否し、フォームを「確認をやり直してください」と出し直す | 拒否し（fail closed）、「しばらくしてからやり直してください」と出す |
| サインイン | 拒否し、フォームを出し直す | **通す**（fail open）。WARN のログを出し、レート制限とパスワードの照合はそのまま行う |

- サインインを fail open にするのは、サービスの障害や鍵の誤りで管理者を含む全員が締め出されないため。
  サービスを落とせない攻撃者には得がなく、総当たりはレート制限が止める。
- reCAPTCHA は無料枠を超えると siteverify が成功を返す。運営者向けの文書に書き、超過の検出はこのサーバーでは行わない。

### 11. CAPTCHA: 画面と CSP

- ウィジェットはサインインと登録のフォーム、およびパスワードリセットのリンクを要求するフォーム（`/auth/forgot_password`。メールを送るサーバーでだけ表示する、FR-002）にだけ出す。サーバーが描画するページはいまは JavaScript なしで動くが、CAPTCHA を有効にしたときだけ、この 3 つのページはサービスのスクリプトを読む。
  リセットのリンクの要求は `identity` の `RequestPasswordReset` で、レート制限の後、メールを送る前に検証する。メールを送る操作なので、サービスに届かないときは登録と同じく拒否する（fail closed）。Turnstile の `data-action` は `password_reset`。
  `<noscript>` で「この確認には JavaScript が要る」ことを知らせる。
- ウィジェットの言語はページのロケールに合わせ（Turnstile は `data-language`、hCaptcha と reCAPTCHA はスクリプトの `hl`）、テーマはページに合わせる（`data-theme`）。
- ウィジェットの近くに「このページは <サービス名> でボットかどうかを確認します。<サービス名> のプライバシーポリシー」の文と、サービスのプライバシーポリシーへのリンクを出す（en / ja のメッセージ、ADR-0009）。
  reCAPTCHA では、バッジの代わりに「This site is protected by reCAPTCHA」と Google のプライバシーポリシー・利用規約を示す表示（reCAPTCHA FAQ）を兼ねる。
- CSP は、CAPTCHA を有効にしたときのサインイン・登録・パスワードリセットのリンクの要求のページだけ、`contentSecurityPolicy` に §13 の表のオリジンを足す（`script-src`・`frame-src`、hCaptcha では `style-src`・`connect-src`、reCAPTCHA では `connect-src` も）。
  `'unsafe-inline'`・`'unsafe-eval'` は足さない。Web クライアント・API・同意画面など他のレスポンスの CSP は変えない。
- `Permissions-Policy`・`Cross-Origin-Opener-Policy: same-origin`・`frame-ancestors 'none'` はそのまま（ウィジェットは iframe の中で動き、こちらのページを埋め込まない）。

### 12. CAPTCHA: プライバシーと運営者の文書

- サービスは利用者の IP アドレス・ブラウザの情報・Cookie を受け取る。reCAPTCHA は Google のプライバシーポリシーと利用規約の対象になる。
- 運営者向けに `docs/operations/captcha.md` を置き、サービスごとの鍵の作り方、設定、CSP、無料枠、プライバシーポリシー（#489）に書くべきこと（どのサービスに何が渡るか、その目的）を説明する。
  プライバシーポリシーの本文は運営者が書くもので、サーバーは自動で書き足さない。

### 13. CAPTCHA: 各サービスの仕様（2026-10-19 に公式文書で確認）

| | Cloudflare Turnstile | hCaptcha | reCAPTCHA v2（チェックボックス） |
| --- | --- | --- | --- |
| 検証 API | `POST https://challenges.cloudflare.com/turnstile/v0/siteverify` | `POST https://api.hcaptcha.com/siteverify` | `POST https://www.google.com/recaptcha/api/siteverify` |
| 送るもの | `secret`, `response`, 任意で `remoteip`, `idempotency_key` | `secret`, `response`, 任意で `remoteip`, `sitekey` | `secret`, `response`, 任意で `remoteip` |
| 返るもの | `success`, `challenge_ts`, `hostname`, `error-codes`, `action`, `cdata` | `success`, `challenge_ts`, `hostname`, `error-codes` | `success`, `challenge_ts`, `hostname`, `error-codes` |
| フォームの欄 | `cf-turnstile-response` | `h-captcha-response` | `g-recaptcha-response` |
| トークン | 1 回限り、発行から 300 秒 | 1 回限り、既定 120 秒 | 1 回限り、発行から 2 分（要検証） |
| CSP に足すオリジン | `https://challenges.cloudflare.com`（`script-src`, `frame-src`） | `https://hcaptcha.com`, `https://*.hcaptcha.com`（`script-src`, `frame-src`, `style-src`, `connect-src`） | `https://www.google.com/recaptcha/`, `https://www.gstatic.com/recaptcha/`（`script-src`）、`https://www.google.com/recaptcha/`, `https://recaptcha.google.com/recaptcha/`（`frame-src`）、`https://www.google.com/recaptcha/`（`connect-src`） |
| 費用 | 無料（上限なし） | 無料枠あり | Google Cloud のプロジェクトに属し、月 10,000 評価まで無料。超過すると siteverify は **成功（スコア 0.9）を返す**（fail open） |
| 利用者の操作 | 多くはなし（必要なときだけチェックボックス） | 画像の課題。テキストの課題・アクセシビリティ用 Cookie | チェックボックス、必要なときに画像・音声の課題 |

出典: [Turnstile server-side validation](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/)、[hCaptcha developer guide](https://docs.hcaptcha.com/)、
[reCAPTCHA verify](https://developers.google.com/recaptcha/docs/verify)、[reCAPTCHA FAQ（CSP）](https://developers.google.com/recaptcha/docs/faq)、
[reCAPTCHA Classic から Enterprise への移行](https://cloud.google.com/recaptcha/docs/migration-overview)。

3 つとも「ブラウザのウィジェットがトークンをフォームに入れ、サーバーが秘密鍵とトークンを siteverify に POST して JSON を受け取る」同じ形をしている
（hCaptcha と Turnstile は reCAPTCHA の API に合わせて作られている）。
reCAPTCHA v3 は課題を出さずにスコア（0.0〜1.0）だけを返し、Enterprise は API キーとプロジェクト ID で `createAssessment` を呼ぶ別の API になる。

### 14. 招待: 権限とインスタンス設定

- 権限 `create_invites` を置く。区分は `member`（ADR-0070 §1）。自分の招待を作り、自分の招待を一覧して取り消すことだけを許し、他人の招待やサーバー全体の設定は変えられないので、区分の条件を満たす。
  組み込みのロールでは `admin` だけが持つ。プランのロールに付けられる（ADR-0072 の許可リストは区分 `member` の権限）。
- インスタンス設定 `invite_creators`（`instance_settings`、`manage_settings`）: `everyone`（ログイン済みで有効なローカルの誰でも）か `permitted`（`create_invites` を持つ人だけ）。既定は `permitted`。
- 利用者が招待を作れるのは、`invite_creators` が `everyone` か、`create_invites` を持つとき。凍結・制限中・登録の承認待ちのアカウントは作れない。
- `manage_invites` を持つスタッフは、これまでどおりすべての招待を作り、一覧し、取り消せる。スタッフの招待に §15 の上限は効かない。

### 15. 招待: 利用者の招待の上限

上限はインスタンス設定（`manage_settings`）で変えられる。利用者が作る招待にだけ効く。

| 設定 | 意味 | 既定 | 範囲 |
| --- | --- | --- | --- |
| `member_invite_max_active` | 1 人が同時に持てる使える招待（取り消し・期限切れ・使い切りでないもの）の数 | 5 | 0〜100（0 は作れない） |
| `member_invite_max_uses` | 1 つの招待で登録できる人数の上限 | 10 | 1〜1,000 |
| `member_invite_max_lifetime_seconds` | 招待の有効期間の上限 | 30 日 | 1 時間〜365 日 |

- 利用者は回数と期間を上限の中で選ぶ。既定は 1 回・7 日（上限がそれより小さいときは上限）。無制限（`max_uses` や期限なし）は選べない。
- 上限を下げても、すでにある招待は変えない。新しく作るときだけ検査する。

### 16. 招待: 作成者の資格の確認

- 利用者が作った招待は、登録のたびに作成者がまだ §14 の条件を満たすかを確かめる。満たさなければ、その招待は使えない招待（既存の `ErrInvalidInvite`。取り消し・期限切れ・使い切りと同じ答え）として断る。
  条件を外れる場合: `create_invites` やプランのロールを失った、`invite_creators` が `permitted` に戻った、凍結・制限・削除された。
- 招待の行は変えない。資格が戻れば（再加入など）、期限の中ならまた使える。ADR-0070 §3 の絵文字の作成者と同じく、操作のたびに決める。
- スタッフの招待（作成者が `manage_invites` で作った招待と CLI の招待）は、これまでどおり作成者を確かめない。どちらの経路で作ったかは招待の行に記録する（列は data-model）。

### 17. 招待: 使える登録モード

- 招待は `open`・`approval`・`invite` のどれでも作れて使える。`closed` では作れず、使えない（`closed` は運営者のみ）。
- `invite`: これまでどおり、使える招待が要り、すぐ承認する。
- `approval`: 招待は任意。使える招待で登録した人は、承認待ちにせずすぐ承認する（招待した人が承認を肩代わりする）。招待なしの登録は、これまでどおり承認待ちになる。
- `open`: 招待は任意。招待で登録しても扱いは招待なしと同じで、誰の招待で来たかだけが記録される。
- どのモードでも、入力された招待が使えない（取り消し・期限切れ・使い切り・§16 の作成者の資格なし・コードがない）ときは、招待なしとして通さずに `ErrInvalidInvite` で断る。打ち間違いに気づけるようにするため。
- 登録の制限（`/moderation/signup-blocks` のメールアドレスのドメインと IP の制限）が承認待ちにする登録は、招待があっても承認待ちにする。
- 登録の画面は、どのモードでも招待コードの欄と `?invite=<code>` を受ける（`invite` では必須、ほかは任意）。

### 18. 招待: 画面と API

- 利用者は設定の中の「招待」（`/settings/invites`）で、自分の招待を作り、一覧し（状態: 使える・取り消し・期限切れ・使い切り・作成者の資格なし）、リンクをコピーし、取り消す。ナビゲーションの項目は増やさない（ADR-0070 §1 の `/settings/custom-emojis` と同じ）。
  §14 の条件を満たさない利用者には、この画面への入口を出さない。
- クライアント API: 自分の招待の `GET` / `POST /api/v1/invites` と `POST /api/v1/invites/{id}/revoke`。管理の `/api/v1/admin/invites` は変えず、応答に作成者を足す。
- モデレーション: `/moderation/invites` に作成者を出す。`/moderation/accounts/{id}` に、そのアカウントが使った招待と招待した人（作成者）を出す（`manage_accounts`。招待の行と登録の申請の `InviteID` からたどる）。

### 19. 招待: 監査ログ

- 利用者の招待の作成と取り消しも、スタッフと同じ `invite_created` / `invite_revoked` で記録する。実行者はその利用者。対象名は今と同じくコードの先頭 4 文字。
- 招待での登録は、登録の申請と承認の記録（既存）に招待の ID が残るので、新しい監査の action は足さない。

### 20. IdP だけの制限: 規則

外部 IdP でのログインと登録に限るインスタンス設定 `login_providers_only`（真偽値、既定 `false`）を置く。

**制限中 = 設定が `true` かつ、ログインに使える IdP がひとつ以上ある**

規則は `identity.Service.LoginProvidersOnly` の 1 か所にある。ログインに使える IdP は、ログインの画面にボタンが出るもの（`ExternalLoginProviders`）と同じで、無効・削除の IdP とドメインの措置の下にある Mastodon サーバーは数えない。
設定と IdP はリクエストごとに読むので、再起動なしに変わる。設定の変更は他の設定と同じく監査ログに残る。スキーマは、設定のキーの CHECK 制約だけが変わる。

### 21. IdP だけの制限: 断るもの

制限中は、次のものをサービスで断り（`ErrLoginProvidersOnly`）、画面はそれを出さない。答えは入力によらず同じで、アカウントを引く前に断る。レート制限にも数えない。

- パスワードでのログイン（`SignIn`）。パスキーでのログイン（`SignInWithPasskey`）。
- パスワードかパスキーで始めたログインの 2 段目（`CompleteSignIn`）。チャレンジに 1 段目が何だったかを持たせ、IdP で始めたものは通す。
- パスワードでの登録（`SignUp`）。IdP での登録は、IdP の `allow_signup` と登録のモードに従って続く。
- パスワードのリセットの依頼（`RequestPasswordReset`）と、リセットのリンクでの設定（`ResetPassword`）。キューに残ったリセットのメールは送らない。リンクは使い切らず、制限が終われば有効期限まで使える。
- パスワードの設定と変更（`ChangePassword`）。パスキーの追加（`BeginPasskeyRegistration`・`FinishPasskeyRegistration`）。どちらもログインに使えない資格を作るだけであり、パスワードの変更はほかの端末のセッションを切って、その端末を締め出す。

### 22. IdP だけの制限: 残すもの

- 既存のセッション、アクセストークン、リフレッシュトークン。
- ログイン中のパスワードとパスキーの**確認**（`confirm`）。パスワードだけの利用者はこれで IdP をひも付ける。
- IdP でのログイン・登録・ひも付けと、その後の TOTP。
- パスワードとパスキーの削除（`RemovePassword`・`RemovePasskey`）。
- 管理者のリセットのリンクの発行（`IssuePasswordResetLink`）。制限を解く直前に要ることがあるので発行は続け、今は使えないことを CLI の出力と API の応答（`login_providers_only`）と管理画面で言う。`my admin create` も続け、そのパスワードでログインできないことを警告する。

### 23. IdP だけの制限: 最後のログイン手段

`ErrLastSignInMethod` の判定は次のとおり。

- パスワードとパスキーの削除では、制限中もパスワードとパスキーを今までどおり手段に数える。制限が終われば戻る手段であり、ひも付けの確認に使う手段でもあるからである。
- ひも付けの解除では、制限中はパスワードとパスキーを手段に数えない。ログインに使える IdP への最後のひも付けは解除できない。無効な IdP へのひも付けは、誰もログインさせないので解除でき、今までの判定に従う。

### 24. IdP だけの制限: 警告と回復

- 有効にするときは断らず、警告して管理者に任せる。管理画面の確認のダイアログと `my instance set` の出力が、ログインに使える IdP へのひも付けのない有効なローカルの利用者の数を言う。管理画面は、そこに管理者自身が含まれることをはっきり言う。数は `GET /api/v1/admin/settings/login_providers_only/impact`（`manage_settings`）が返す。
- 設定がオンで使える IdP がないときは、IdP の設定（作ると同時に有効になる）と有効化が制限をその場で始める。管理画面は、その操作のときだけ実行の前に確認のダイアログを出し、その IdP へのひも付けのない有効な利用者の数と、管理者自身が含まれることを言う。数と始まるかどうかは同じ API が返す（`provider_id` で有効にする IdP を指定する。権限は `manage_settings` か `manage_login_providers`）。断らない。
- `my login-provider create`・`enable`・`disable`・`delete` は、設定がオンのときに制限の今の状態を出す。
- 締め出された管理者は、サーバーのホストで `my instance set login_providers_only false` を実行して戻す。
- 有効な IdP がなくなると制限は自動で働かなくなる。パスワードもパスキーもない利用者は、今までどおりリセットのリンクがいる。

### 25. IdP だけの制限: 登録のモードと公開する情報

制限中で、新規登録を許す IdP がひとつもないときは、登録のモードを `closed` と読む（`GET /instance` の `registrations.mode`、NodeInfo の `openRegistrations`、ログインの画面の登録へのリンク、招待の作成）。設定の値は変えない。
`GET /instance` の `login_providers_only` が、クライアントに制限中かどうかを伝える。招待のリンクの招待コードは、IdP を経由して登録のフォームまで運ぶ。

### この決定を見直す条件

- 対象にしたい IdP の多くが OIDC でも GitHub 型のプリセットでも扱えず、プリセットを足し続けるほうが汎用の OAuth 2.0 より重いと分かった場合は、識別子のフィールドを固定の候補から選ぶ形の汎用 OAuth 2.0 を検討する。
- 運営者から、IdP の設定を Infrastructure as Code で配りたいという要望が強い場合は、設定ファイルから DB へ流し込む CLI（`my login-provider apply`）を足す。
- 利用者の招待からのスパムの登録が目立つ場合は、招待で登録した人の初期の制限（投稿数など）や、招待した人への責任のひも付け（招待した人の凍結で招待された人を確認待ちにする）を検討する。
- パスワードだけの利用者の締め出しが許容できなくなったときは、ひも付けのための猶予のモードを検討する。
- 組織が、パスキーのログインを IdP と別に許す必要を示したときは、パスワードだけを止める形を検討する。

## 経緯

ログイン手段がメールアドレスとパスワード、パスキー、その上の TOTP（FR-005）だけだった。GitHub や Google、組織の IdP でのログイン・登録の要望に応えるため、管理者が再起動なしに IdP を足せるようにした（FR-010）。ADR-0090 の逆向きで、このサーバーが IdP のクライアントになる。IdP の設定やメールアドレスの一致を足がかりにアカウントを乗っ取られないこと、IdP の設定を内部ネットワークへの踏み台にできないことを優先した。ログイン・同意のページ（ADR-0019 §4、ADR-0090）、`kernel.HTTPClient`（ADR-0019 §1）、`kernel.Sealer`（ADR-0019 §6）、監査ログ（ADR-0058）、権限（ADR-0058、ADR-0070）、最後のログイン手段（`ErrLastSignInMethod`）の既存の規約の上に載せた。IdP の性質は 2026-10-27 に公式文書で確かめた: [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)、[Discovery 1.0](https://openid.net/specs/openid-connect-discovery-1_0.html)、[GitHub の OAuth apps](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps)、[GitHub の PKCE](https://github.blog/changelog/2025-07-14-pkce-support-for-oauth-and-github-app-authentication/)、[GitHub の emails](https://docs.github.com/en/rest/users/emails)、[Mastodon の OAuth](https://docs.joinmastodon.org/methods/oauth/)、[Mastodon の scope](https://docs.joinmastodon.org/api/oauth-scopes/)、[Mastodon 4.4](https://blog.joinmastodon.org/2025/07/mastodon-4-4-for-devs/)、[Google の OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect)。

CAPTCHA は、レート制限（ADR-0019 §5）が多数の IP から少しずつ来るスパム登録やパスワードスプレーを止めないので足した。reCAPTCHA を使いたいという要望があり、3 つのサービスが同じ siteverify の形をしていたので、同じ実装で選べるようにした（§13 の出典: [Turnstile](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/)、[hCaptcha](https://docs.hcaptcha.com/)、[reCAPTCHA verify](https://developers.google.com/recaptcha/docs/verify)、[reCAPTCHA FAQ](https://developers.google.com/recaptcha/docs/faq)、[reCAPTCHA の移行](https://cloud.google.com/recaptcha/docs/migration-overview)）。

招待はスタッフだけが作れ、`invite` モードでしか使えなかった。サポーターの特典にもできるよう、ADR-0100 の `voice_room_starters` と `start_voice_rooms` と同じ形（設定と区分 `member` の権限）で利用者にも開き、`open` と `approval` でも使えるようにした。

IdP だけの制限は、組織の IdP だけを入口にして退職者の扱いや多要素認証の方針を IdP に任せたいサーバーのためのもので、§1〜§6 が IdP を足すだけなのに対して、ほかの入口を止める。パスキーも止める。IdP をひも付けていない既存の利用者は警告して管理者に任せ、有効な IdP がなくなれば自動で戻す。

## 採らなかった案

- 汎用の OAuth 2.0: リリースを待たずどのサービスも足せるのが最も強い論点だが、識別子のフィールドを管理者が選ぶので、変えられる値を選ぶと別人が既存の利用者としてログインでき、userinfo の応答をそのまま信じることになる。
- プリセットだけの IdP: 管理者の入力が最も少ないが、組織の Keycloak・Authentik など自前の IdP を使えない。
- IdP の設定を設定ファイルに置く: 秘密を DB に置かず Infrastructure as Code で配れるが、変えるたびに再起動が要り、管理画面から設定できない。
- reCAPTCHA v2 だけを組み込む: 実装は最小だが、Google への依存とデータ送信を全運営者に強い、無料枠を超えると黙って成功を返す。ほかのサービスとの差はプリセットの表だけで、縛る理由が小さい。
- 自前ホストの proof-of-work（[ALTCHA](https://altcha.org/)、[mCaptcha](https://mcaptcha.org/)）: 第三者にデータを送らないのが最も強い論点だが、CPU を持つボットは通り、要望の reCAPTCHA を満たさない。ポートがあるので後から足せる。
- CAPTCHA を使わない: 第三者に依存しないが、承認制は管理者の手間が登録数に比例し、開放した登録とパスワードスプレーには効かない。
- `manage_invites` をプランのロールにも付ける: 新しい権限が要らないが、全員の招待を一覧して取り消せるスタッフの権限で、区分 `member` の条件を満たさず、お金でスタッフの権限を買えない決まり（ADR-0072）に反する。
- 利用者の招待の上限をコードに固定する: 設定が増えないが、サーバーごとに変えられない。スタッフと同じ上限なしでは、`everyone` のときに 1 人が無制限の招待を作れる。
- 作成者が資格を失っても招待を期限まで使える: 登録のときの読み取りが増えないが、凍結した利用者やプランをやめた利用者の招待で登録が続く。
- IdP だけの制限でパスワードだけ止め、パスキーを残す: パスキーは推測に強く締め出しを減らすが、入口を IdP に集める目的に合わない。
- ひも付けのための猶予のモード: パスワードだけの利用者が自分で移行でき締め出しがいちばん少ないが、パスワードが入口として残り、ログインできても何もできないセッションの状態を新しく作る。
- パスキーと未連携の利用者には制限をかけない: 締め出しは起きないが、既存の利用者のパスワードの入口が残り、制限にならない。

## Consequences（影響）

- 良い影響
  - Google・GitLab・Microsoft Entra・Keycloak・Authentik など OIDC の IdP を、コードを足さずに管理画面から設定できる。GitHub も使える。
  - 姉妹インスタンスなど、管理者が信頼する Mastodon サーバーのアカウントでログイン・登録させられる。
  - 識別子だけでひも付け、メールアドレスで自動ではひも付けないので、IdP 側でメールアドレスを詐称されても既存のアカウントは乗っ取られない。
  - 外部ログインは Cookie のセッションを作るところで終わるので、クライアント API とトークン（ADR-0090）は変わらない。IdP を設定しないサーバーには、行も外部への要求も増えない。
  - 多数の IP から来るスパム登録とパスワードスプレーに、レート制限とは別の層ができる。CAPTCHA の既定は `none` なので、自前ホストや閉じたネットワークのインスタンスは何も変わらない。
  - サポーターの特典に招待を足せる。`approval` のサーバーで、信頼できる利用者の招待なら承認の手間が減る。誰の招待で来たかをモデレーターが確かめられる。
  - 組織の IdP だけを入口にでき、入口の管理が IdP に一本化する。
- 悪い影響・トレードオフ
  - 管理者が IdP を無効にしたり消したりすると、その IdP でしかログインしない利用者はログインできなくなる。救済はパスワードのリセット（FR-002）と管理者が発行するリンク（`manage_login_recovery`）に頼る。運用文書にこの影響を書き、管理画面は無効にする前にひも付いた利用者の数を示す。
  - IdP の多要素認証と、このサーバーの TOTP が二重になる利用者がいる。
  - GitHub での再認証は、GitHub にログイン中の端末なら入力なしに通るので、パスワードやパスキーより弱い本人確認になる。
  - JWT の検証と OIDC のクライアントを依存を足さずに自前で持つので、署名の検証とクレームの確認の正しさは、このリポジトリのコントラクトテスト（`testkit.OIDCClientContract`）で守る。
  - SAML と CAS の IdP は扱わない。
  - CAPTCHA を有効にすると、サインインと登録のページが第三者のスクリプトに依存し、そのページの CSP が広がる。サービスのスクリプトが侵害されればそのページは影響を受ける。
  - CAPTCHA を解けない人（視覚・認知の障害、JavaScript を無効にしている人、サービスに届かない国や組織）は登録できない。招待コードがあっても同じで、運営者に連絡して CAPTCHA を外してもらうほかない。
  - サインインの fail open は、サービスの障害中は CAPTCHA の保護がないことを意味する。サービスの仕様（検証 API・CSP のオリジン）が変わったらプリセットを直す必要がある。
  - `approval` の承認を招待で省けるので、`everyone` にすると承認制の意味が弱まる。運営者向けの文書にこの関係を書く。
  - 登録のたびに作成者の資格を確かめるので、招待での登録の読み取りが 1 回増える。
  - **IdP だけの制限中、セッションのない利用者は、管理者が動くまで締め出される。** これがこの決定の害である。IdP をひも付けていないパスワードだけの利用者は、セッションが切れるともう入れない。これを承知で警告に留めた。
  - 同じ理由で、IdP のないパスワードだけの利用者が `disable_login`・`suspend`・`ban` を受けてセッションを失うと、ログインの画面からの異議申し立てに届かない。
  - **アプリのトークンは生きていてブラウザの Cookie のセッションだけ切れた利用者は、制限中はひも付けに進めない。** ひも付けの開始はブラウザのセッションを確かめる（ログイン CSRF の対策、§3.5）。Cookie のセッションとリフレッシュトークンはどちらも 30 日で、リフレッシュで延びるのはトークンだけなので、これは実際に起きる。このまま受け入れ、運営者は、制限を有効にする前に利用者へ知らせる。
  - 管理者が CLI のないホスティングだけを持つと、自分を締め出したときに戻す道がない。IdP を無効にする API にも届かない。
  - IdP の障害の間は、制限中のサーバーには誰も新しくログインできない。設定をオフにするか IdP を無効にすれば戻る。
  - 有効な IdP を足すと、設定がオンなら制限がその場で始まる。管理画面は実行の前に警告し、CLI は実行の後に状態を出す。

## Open Questions / 要検証

- `GET /user/emails` に要る GitHub の scope が `user:email` であること（要検証: §1 はこれを求めるが、公式文書で確かめ切れていない）
- 3 つのサービスのウィジェットが、`'unsafe-inline'` のない `style-src` と `require-trusted-types-for` のない現在の認証ページの CSP で動くか（各サービスのテスト用の鍵を使い、ブラウザで確かめる）
