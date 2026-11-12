<!-- source: captcha-bot-protection-turnstile--hcaptcha--recaptcha 2c065ac1d2fb -->
# CAPTCHA によるボット対策 (Turnstile / hCaptcha / reCAPTCHA)

CAPTCHA は既定ではオフで、設定 [`captcha`](../reference/configuration.md#captcha) は `none` です。このときログイン・登録・パスワード再設定のページは JavaScript なしで動き、第三者には何も送りません。
CAPTCHA のサービスを選ぶと、そのサービスのウィジェットが 3 つのフォームに表示されます。ログイン (`/auth/sign_in`)、登録 (`/auth/sign_up`)、パスワード再設定リンクの申請
(`/auth/forgot_password`、メールを送るサーバーだけ) です。サーバーは、ウィジェットが作ったトークンをサービスの siteverify API で検証します。
一部のフォームだけを保護することはできません。

<!-- source: choosing-a-service 22ddd3cc639f -->
## サービスを選ぶ

| `captcha` | サービス | 費用 | 利用者がすること |
| --- | --- | --- | --- |
| `turnstile` (推奨) | Cloudflare Turnstile | 無料、上限なし | 通常は何もしない。必要なときだけチェックボックス |
| `hcaptcha` | hCaptcha | 無料枠あり | チェックボックス。必要なときは画像の課題も |
| `recaptcha` | Google reCAPTCHA v2 (チェックボックス) | 月 10,000 回の評価まで無料 (下記を参照) | チェックボックス。必要なときは画像か音声の課題も |

reCAPTCHA v3 (スコアのみ) と reCAPTCHA Enterprise には対応していません。

<!-- source: creating-keys 38cd93394638 -->
## キーを作る

どのサービスでも、公開の**サイトキー**と、サーバーだけが持つ**シークレットキー**の 2 つのキーが発行されます。ドメインには、設定 `host` のホスト名 (ポートを除く) を登録してください。
siteverify の応答にあるホスト名が `host` と一致しないとき、サーバーはトークンを拒否します。

- **Turnstile**: Cloudflare のダッシュボードで「Turnstile」にウィジェットを追加し、ホスト名を登録します。ウィジェットのモードは「Managed」を選びます。
- **hCaptcha**: hCaptcha のダッシュボードでサイトを追加し、ホスト名を登録します。シークレットキーはアカウントの設定にあります。
- **reCAPTCHA**: Google reCAPTCHA の管理コンソールで、種類「Challenge (v2)」の「I'm not a robot」チェックボックスを選び、ドメインを登録します。
  キーは Google Cloud のプロジェクトに属します。

各サービスのテスト用キー (常に成功するもの) は、ウィジェットが表示されてトークンを作ることを確かめるのに役立ちます。
ただし、テスト用キーでの siteverify は決まったホスト名 (Turnstile は `example.com`、hCaptcha は `dummy-key-pass` など) を返すので、それを使うとこのサーバーでのログインと登録は「もう一度確認してください」で失敗します。

<!-- source: configuring 3301db5c6286 -->
## 設定する

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- シークレットキーは、環境変数 `MY_CAPTCHA_SECRET_KEY` か、ファイルを指す `MY_CAPTCHA_SECRET_KEY_FILE` でだけ渡してください。ログには `[REDACTED]` と表示されます。
- `captcha` を選んでキーが欠けていると、`my config check` が問題を報告し、サーバーは起動しません。
- サーバーは起動時にキーを試しません。サービスがシークレットキーやサイトキーを拒否すると、最初の検証で ERROR
  「the CAPTCHA service refused the secret key or the site key」がログに出ます。直すまで、リクエストは下の「サービスに届かないとき」と同じように扱われます。
- 変更はサーバーを再起動すると反映されます。

<!-- source: how-verification-works c5cc12dd3769 -->
## 検証のしくみ

- 順番は、レート制限、CAPTCHA の検証、そしてパスワードの確認・登録の処理・再設定メールの送信です。レート制限を超えたリクエストはサービスに送りません。
- 有効な招待コードのある登録も検証します。
- 検証に失敗したログインは、クライアントの IP アドレスのログイン失敗として数えます。アカウントの失敗としては数えないので、誰かがほかの人を締め出すことはできません。
  検証に失敗した登録は登録の試行として数えます ([登録](registration.md)のレート制限を参照)。再設定リンクの申請も同じように申請の試行として数えます。
- Turnstile では、ウィジェットがアクション名 (`sign_in` / `sign_up` / `password_reset`) を持ち、サーバーは応答のアクションが同じであることを確かめます。あるフォームのトークンをほかのフォームでは使えません。
- サーバーは利用者の IP アドレス (`remoteip`) をサービスに送りません。トークンは 1 回しか使えないので、サーバーは再試行しません。タイムアウトは 10 秒です。

<!-- source: when-the-service-is-unreachable fe6a6d5915ea -->
### サービスに届かないとき

サービスに届かない、タイムアウトする、5xx を返す、またはキーを拒否するとき:

| フォーム | 扱い |
| --- | --- |
| 登録と再設定リンクの申請 | 「しばらくしてから試してください」で拒否します (`503`)。そうしないと、ボットにメールや登録を起こされるおそれがあるためです。 |
| ログイン | CAPTCHA なしで通し、サーバーは WARN「the CAPTCHA service could not check a sign-in; letting it through」をログに出します。レート制限とパスワードの確認はそのまま適用されます。 |

ログインを通すのは、サービスの障害やキーの誤りで、管理者を含む全員が締め出されないようにするためです。WARN が出続けるときは、サービスの状況とキーを確認してください。

<!-- source: recaptcha-free-tier 4f413b07dbb9 -->
### reCAPTCHA の無料枠

月 10,000 回の評価という無料枠を超えると、reCAPTCHA の siteverify は**常に成功します** (その間 CAPTCHA は何も守りません)。
サーバーは超過を検出できないので、Google Cloud のコンソールで使用量を確認してください。

<!-- source: pages-and-content-security-policy b0cf9741b2ce -->
## ページと Content-Security-Policy

- ウィジェットの言語はページのロケールに、テーマはブラウザの配色 (ライトまたはダーク) に従います。
  Turnstile のウィジェットは自分で配色に従います。hCaptcha と reCAPTCHA は、ブラウザが送るクライアントヒント `Sec-CH-Prefers-Color-Scheme` で選ばれ、それを送らないブラウザにはライトのテーマになります。
- ウィジェットの隣に、どのサービスが確認を行うか、サービスが IP アドレスとブラウザの情報を受け取ることが書かれ、サービスのプライバシーポリシーへのリンクがあります
  (hCaptcha と reCAPTCHA では利用規約へのリンクも)。reCAPTCHA では、これが Google の求める一文「This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply.」も兼ねます。
- JavaScript のないブラウザでは、確認に JavaScript が必要だというメッセージを `<noscript>` で表示します。CAPTCHA を解けない人は登録もログインもできず、
  `my admin create` でアカウントを作る、しばらく `captcha = none` にする、といったほかの方法でしか助けられません。
- `Content-Security-Policy` (CSP) が広がるのは、CAPTCHA が有効なときのログイン・登録・パスワード再設定の申請のページだけです。`'unsafe-inline'` と `'unsafe-eval'` は決して加えません。
  Web クライアント、クライアント API、同意画面、メールアドレスの確認などのページの CSP は変わりません。

| `captcha` | 加わるソース |
| --- | --- |
| `turnstile` | `script-src`、`frame-src`: `https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`、`frame-src`、`style-src`、`connect-src`: `https://hcaptcha.com`、`https://*.hcaptcha.com` |
| `recaptcha` | `script-src`: `https://www.google.com/recaptcha/`、`https://www.gstatic.com/recaptcha/`。`frame-src`: `https://www.google.com/recaptcha/`、`https://recaptcha.google.com/recaptcha/`。`connect-src`: `https://www.google.com/recaptcha/` |

サーバーの前にあるリバースプロキシや CDN が CSP を上書きするときは、この 3 つのページに同じソースを加えてください。

<!-- source: what-to-write-in-the-privacy-policy d601071ebd4c -->
## プライバシーポリシーに書くこと

CAPTCHA を有効にすると、ログイン・登録・パスワード再設定の申請のページを開いた人のブラウザはサービスに直接つながり、サービスは IP アドレス、ブラウザの情報、Cookie を受け取ります。
インスタンスのプライバシーポリシー (`my instance set privacy_policy`、[インスタンスの設定](instance-settings.md)を参照) はあなたが書くもので、サーバーは何も書き足しません。[プライバシーポリシーのひな形](privacy-policy-template.md)にはサービスの行があります。少なくとも次のことを書いてください。

- どのサービス (事業者の名前) を、なぜ使うのか。ログイン・登録・パスワード再設定の申請が自動で行われるのを防ぐためです。
- サービスに何が届くのか (IP アドレス、ブラウザと端末の情報、Cookie、ウィジェットの操作)。そして、それを送るのはログイン・登録・パスワード再設定の申請のページだけであること。
- サービスのプライバシーポリシーへのリンク (Turnstile: <https://www.cloudflare.com/privacypolicy/>、hCaptcha: <https://www.hcaptcha.com/privacy>、reCAPTCHA: <https://policies.google.com/privacy>)。
- reCAPTCHA では、Google のプライバシーポリシーと利用規約 (<https://policies.google.com/terms>) が適用されること。
