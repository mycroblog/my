# 利用者登録（登録モード・招待・承認・メール確認）

運営者向けの手順です。要件は FR-001・FR-002（[機能要件](../requirements/01-functional.md)）、テーブルは [データモデル §4.1](../architecture/data-model/04-tables/01-identity.md#41-identity)、
トークンとレート制限は [ADR-0019 §4・§5](../adr/0019-security-baseline.md) です。

## 登録画面

利用者は `https://<host>/auth/sign_up` で、ユーザー名・メールアドレス・パスワードを入力して登録します。
登録すると、そのブラウザーはそのままログインした状態になります。
画面は JavaScript なしで動き、ログイン画面と同じく CSRF トークンとクロスオリジン要求の拒否で守られます（ADR-0030）。
ボット対策に CAPTCHA を選んだときは、登録とログインの画面にそのウィジェットを出し、JavaScript が要ります（[CAPTCHA](captcha.md)）。

登録モードはインスタンス設定 `registration_mode` で、CLI か Web クライアントの「サーバーの設定」（`/admin/settings`）で選びます（[インスタンス設定](instance-settings.md)）。変更は次の登録から効きます。

```sh
my instance set registration_mode invite
```

| モード | 画面 | 登録直後の状態 |
| --- | --- | --- |
| `open` | 誰でも登録できる | 承認済み。ただし `mail = none` ではモデレーターの承認待ち（下記） |
| `approval`（既定） | 「参加したい理由」（任意、1000 文字まで）も尋ねる | 承認待ち |
| `invite` | 招待コードが必須 | 承認済み |
| `closed` | 受け付けない旨だけを表示する | — |

- 同じネットワーク（IP アドレス。IPv6 は `/64`）からの登録は 30 分に 5 回までです。超えると `429` と `Retry-After` を返します。
- ユーザー名・メールアドレスが使われていれば、その項目にエラーを表示し、何も書き込みません（招待も消費しません）。
- 登録とログインのたびに、接続元の IP アドレス（ログインでは User-Agent も）を記録します。記録は 180 日（インスタンス設定 `retention_session_origins`・`retention_registration_origins`、90 日以上）で自動的に消えます。保全中のアカウントの記録は残ります（[ADR-0038](../adr/0038-moderation-personal-data-retention.md)、[記録の保持期間と保全](moderation.md#記録の保持期間と保全)）。
  プライバシーポリシーにはこの保持期間を書いてください。

## アプリを使えるようになる条件

承認待ち・却下された利用者や、メールアドレスを確認していない利用者も、ログイン画面にはログインできます。
ログイン後は、何を待っているか（モデレーターの承認・メールアドレスの確認）、または登録が承認されなかったことを表示し、元の画面には進みません。
アプリ（クライアント API の OAuth 認可とトークン発行）を使えるのは、承認済みで、メール確認が必要な構成ではメールアドレスを確認済みの利用者だけです。

`my admin create` で作った管理者は承認済み・確認済みとして扱います。

## メール確認

設定 `mail`（[設定リファレンス](../reference/configuration.md#mail)）で送信方法を選びます。
SMTP の設定例・DNS の設定・送信の失敗の見方は [メールの送信](email.md) にあります。

- `mail = smtp`: 登録時に確認メールを送ります。リンクの有効期限は 24 時間で、1 回だけ使えます。
  リンクを開くと確認ボタンの画面になり、ボタンを押すと確認済みになります（メールのリンクを先読みするスキャナーでトークンを消費しないため）。
  利用者はログイン後の画面から 1 時間に 3 回まで再送できます。
  SMTP サーバーへの接続は必ず STARTTLS で暗号化し、証明書を検証してから認証・送信します。STARTTLS を提供しないサーバーには送りません。
- `mail = none`（既定）: メールを送らず、本文（確認リンクを含む）をサーバーのログに `mail not sent (mail = none)` として出します。
  利用者にメールが届かないので、メール確認の代わりにモデレーターの承認を求めます（FR-002）。
  このため `open` モードの登録も承認待ちで始まり、メールアドレスが未確認でもアプリの利用は妨げません。

確認トークンはこのログ行のほかには記録しません（アクセスログはクエリ文字列を記録しません）。DB にはトークンの SHA-256 だけを保存します。

## パスワードのリセットと変更

- `mail = smtp`: ログインの画面の「パスワードを忘れたとき」でメールアドレスを入れると、そのアドレスを主アドレスにする利用者に、新しいパスワードを設定するリンクを送ります。
  画面の答えはアカウントの有無で変わりません。リンクは 1 時間・1 回だけ使え、その後にパスワードがほかの方法で変わると使えなくなります。
  要求は接続元の IP アドレスごとに 30 分に 5 回まで、メールは 1 つのアドレスに 1 時間に 3 通までです（それを超えた要求は何も送らず、同じ答えを返します）。CAPTCHA を有効にしていれば、このフォームでも検証します。
- `mail = none`: 画面はリンクを送れないことと、管理者に頼むことを伝えます。管理者は管理画面のアカウントのページか `my admin account password-reset-link @alice` でリンクを発行して本人に渡します（[モデレーション](moderation.md)）。
- 利用者はログインしたまま、設定の「パスワード」で現在のパスワードを入れて変えられます（間違いは利用者ごとに 1 時間 5 回まで）。
- リセットでも変更でも、新しいパスワードはその利用者のほかのセッションとアプリのトークンを失効させます（設定からの変更では、変更したアプリだけが残ります）。パスキーと二要素認証は残ります。
  `mail = smtp` では、変更したことを本人にメールで知らせます。

## 招待コード

```sh
my invite create                          # 1 回だけ使え、7 日で期限切れになる招待
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # 回数・期限の制限なし
my invite list                            # 新しい順。状態は usable / revoked / expired / used up
my invite revoke <code>
```

- `create` はコード（16 文字）と、コードを入力済みの登録画面のリンク（`https://<host>/auth/sign_up?invite=<code>`）を表示します。
- コードは大文字小文字・空白・ハイフンを区別せずに照合します。
- 同時に登録されても、使用回数の上限を超えて使われることはありません（`invite_redemptions` の一意制約）。
- 取り消したコード・期限切れのコード・使い切ったコードは、どれも同じエラー（使えない招待）になります。
- 招待は CLI のほか、Web クライアントのモデレーションの画面（「招待」、`/moderation/invites`）とクライアント API
  （`GET` / `POST /api/v1/admin/invites`・`POST /api/v1/admin/invites/{id}/revoke`）で作成・一覧・失効できます。どちらもロールの権限 `manage_invites` が要ります
  （組み込みの `moderator` は持たないので、任せるときはその権限を持つロールを作って与えます）。
  画面は作成した招待のコードと登録画面のリンクを出し、使える招待のリンクをコピーできます。登録モードが `invite` でないときは、招待では誰も登録できないことを画面に出します。
- 招待の作成と失効は、画面・API・CLI のどれからでも監査ログ（`invite_created`・`invite_revoked`）に残ります。対象の名前はコードの先頭 4 文字（`ABCD…`）で、コード全体は監査ログに写しません。
  失効済みの招待をもう一度失効しても何も変わらないので、記録しません。

## 承認と却下

```sh
my registration list                      # 承認待ち（待ちが長い順）。理由・メール確認の有無も表示する
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- 引数はローカルアカウントのユーザー名です（先頭の `@` は省けます）。
- 決定できるのは承認待ちの登録だけです。決定済みの登録や、登録画面を通らずに作られた利用者（`my admin create`）に対しては、何も変更せずに終了コード 1 で終わります。
- 終了コード 2 は引数の誤りです。

管理画面（クライアント API）からも同じ決定ができます。どれも `manage_accounts` 権限が要ります。

| 操作 | 内容 |
| --- | --- |
| `GET /api/v1/admin/registrations` | 承認待ちの登録（待ちが長い順）。登録の理由・メールアドレスと確認の有無・ローカルアカウントを返す |
| `POST /api/v1/admin/registrations/{user_id}/approve` | 承認する。本文 `{"note": "..."}` は省ける |
| `POST /api/v1/admin/registrations/{user_id}/reject` | 却下する。本文は `approve` と同じ |

- CLI と同じユースケースで、決定を `registration_decisions` と監査ログ（`registration_approved`・`registration_rejected`）に記録します。CLI の決定の操作者は `cli`、API の決定の操作者は決めたモデレーターです。
- 決定済みの登録をもう一度決定しても何も変わらず、今の状態を返します（同時に決定しても、決定は 1 つだけです）。登録画面を通らずに作られた利用者は 404 です。

## 登録の拒否リスト

メールアドレスのドメイン・メールアドレス・IP の範囲で、登録画面からの登録を拒否するか承認制に切り替えます（FR-719）。

```sh
my admin signup-block add -kind email_domain spam.example          # サブドメインにも効く。U-label も可
my admin signup-block add -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block add -kind email_hash spammer@example.com     # アドレスはハッシュだけを残す
my admin signup-block list -active
my admin signup-block lift <block id>
```

- `-severity reject`（既定）に当たる登録は受け付けず、`require_approval` に当たる登録は登録モードに関わらず承認待ちで始まります。複数に当たれば強いほうが効きます。登録画面は拒否の理由を示しません。
- `email_hash` は小文字にし、ローカル部の最初の `+` 以降を除いたアドレスで照合します。IP の範囲は登録元の IP（`registration_origins` と同じ値）で照合し、IP の分からない登録には効きません。
- 期限切れ・解除済みの拒否は効きません。追加と解除は監査ログ（`signup_block_added`・`signup_block_lifted`）に残ります。
- ローカルアカウントの BAN で `-block-email`（API は `block_email: true`）を付けると、その利用者のメールアドレスの `email_hash` の拒否を同時に作ります。

使い捨てメールのサービスのドメインのように数千件ある一覧は、`import` でファイル（`-` で標準入力）からまとめて足します。すべての値に同じ種類・強さ・コメント・期間が付きます。

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- 一覧は 1 行に 1 つの値（`add` の `<value>` と同じ形）か、最初の列を値とする CSV の行です。空行と `#` で始まる行は読み飛ばします。1 回に 10,000 件までです。
- 足せた件数と、足さなかった行の番号・値・理由を表示します。理由は `invalid`（その種類の値として正しくない）、`duplicate`（同じ一覧の前の行と正規形が同じ）、`already_blocked`（同じ種類・値で、同じかより強い拒否が効いている）です。
- 足した拒否は 1 件ずつ `add` と同じく監査ログ（`signup_block_added`）に残り、1 件ずつ解除できます。
- 数百件ずつのトランザクションで書くので、途中で失敗しても、それまでに足した分は残ります。同じ一覧をもう一度読み込めば、残りだけが足されます。
- 一覧はこのソフトウェアに同梱せず、URL からの定期的な取り込みもしません。公開の一覧（GitHub の disposable-email-domains など）の取得と更新は運営者が行い、更新した一覧をもう一度読み込みます。

管理画面（クライアント API）からも同じ操作ができます。どれも `manage_accounts` 権限が要ります。

| 操作 | 内容 |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | 拒否の一覧（新しい順）。`kind`・`active` で絞り込める |
| `POST /api/v1/admin/signup_blocks` | 拒否を追加する。本文は `kind`・`value`・`severity`・`comment`・`expires_in`（秒） |
| `POST /api/v1/admin/signup_blocks/import` | 一覧からまとめて追加する。本文は `kind`・`text`（一覧）・`severity`・`comment`・`expires_in`。応答は足した件数 `added` と足さなかった行 `skipped`（`line`・`value`・`reason`） |
| `GET /api/v1/admin/signup_blocks/{id}` | 拒否を 1 件返す |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | 拒否を解除する。解除済みなら何も変えない |
