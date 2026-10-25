# メールの送信

運営者向けの手順です。メールの設計は [ADR-0086](../adr/0086-email-delivery.md)、設定項目は [設定リファレンス](../reference/configuration.md#mail) を参照してください。
メール確認とパスワードのリセットの利用者側の流れは [登録とメール確認](registration.md) にあります。

## 送るメールと設定

サーバーはメールを、リクエストの中では送りません。送る事実と同じトランザクションで用途ごとのジョブを積み、ジョブのワーカーが送ります（ADR-0086 §1）。
送信の失敗は登録やリセットの応答に出ません。利用者には「確認のメールを送りました」と表示し、届かなければ利用者が再送を求めます。

| 分類 | メール | ジョブの型 | 配信停止 |
| --- | --- | --- | --- |
| 取引（`transactional`） | メールアドレスの確認 | `identity.verification_email` | できない |
| 取引 | パスワードのリセット | `identity.password_reset_email` | できない |
| 取引 | パスワードの変更・削除の通知 | `identity.password_notice_email` | できない |
| 通知（`notification`） | モデレーターへの通知（利用者設定 `email_moderator_notifications`） | `notification.moderator_email` | できる |

サポーターの通知のメール（`notification.supporter_email`）も同じジョブのキューで送ります。

メールの送り方は設定 `mail` で選びます。

| `mail` | 動き |
| --- | --- |
| `none`（既定） | 送らず、本文（リンクを含む）をサーバーのログに `mail not sent (mail = none)` として出す。メールを使わない構成と開発用 |
| `smtp` | SMTP の提出（submission）で送る。STARTTLS を必須とし、証明書を検証してから認証・送信する |

`smtp` の設定は `mail_from`・`smtp_host`・`smtp_port`（既定 587）・`smtp_username`・`smtp_password` です。
`mail_from` は `noreply@social.example` のような裸のアドレスで、`From` は `"<サーバー名>" <mail_from>` になり、`Message-ID` のドメインと SPF・DKIM・DMARC の対象（下記）もこのドメインです。
`Reply-To` には、インスタンス設定の `contact_email` があればそれを付けます（[インスタンス設定](instance-settings.md)）。
すべてのメールに `Auto-Submitted: auto-generated` が付きます。

## 事業者ごとの設定例

秘密は環境変数かファイルで渡します（`MY_SMTP_PASSWORD` または `MY_SMTP_PASSWORD_FILE`）。設定ファイルには書かないでください。

### Amazon SES（SMTP インタフェース）

SES の SMTP インタフェースを `mail = smtp` で使います。SES の API を使うアダプタ（`mail = ses`）はまだなく、この構成では送信時の宛先の拒否だけを抑止として記録します（[メールが届かないアドレスの抑止](#メールが届かないアドレスの抑止)）。

1. SES で `mail_from` のドメインをアイデンティティとして検証し、Easy DKIM を有効にする（下記）。
2. 本番のアクセスを申請して、サンドボックスを出る。サンドボックスでは検証済みのアドレスにしか送れない。
3. SES の「SMTP settings」で SMTP 認証情報を作る（IAM のアクセスキーとは別の値）。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # SES のリージョンのエンドポイント
MY_SMTP_PORT=587                                       # STARTTLS
MY_SMTP_USERNAME=<SMTP 認証情報のユーザー名>
MY_SMTP_PASSWORD_FILE=/run/secrets/smtp_password
```

SES は SMTPUTF8 に対応せず、アドレスは ASCII（ドメインは Punycode）で送ります。SES は `Message-ID` と `Date` を自分の値で上書きします。

### 一般的な SMTP の提出

Postfix などの自前の SMTP サーバーや、メール配信の事業者が提出用に出しているサーバーを使うときです。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_SMTP_HOST=smtp.provider.example     # 証明書がこの名前で有効な STARTTLS 対応のサーバー
MY_SMTP_PORT=587
MY_SMTP_USERNAME=<ユーザー名>          # 認証のないサーバーでは空
MY_SMTP_PASSWORD_FILE=/run/secrets/smtp_password
```

STARTTLS を提供しないサーバーと、証明書が `smtp_host` の名前で検証できないサーバーには送りません。認証は STARTTLS のあとにだけ送ります。
設定を変えたら `my config check` で確かめてから起動し直します。

## SPF・DKIM・DMARC

Gmail と Yahoo は、1 日に多くのメールを送る送信者に SPF・DKIM・DMARC と、1 クリックの配信停止を求めます（ADR-0086 Decision Drivers 3）。少量でも、設定していないと迷惑メールに分類されやすくなります。
いずれも `mail_from` のドメイン（例: `social.example`）の DNS に設定します。値は事業者の案内に従ってください。

- **SPF**: 送信に使う事業者を許可する TXT レコードを、ドメインに 1 つだけ置く（例: SES の独自 MAIL FROM ドメインを使うなら、そのドメインに事業者の指定する `include:`）。
- **DKIM**: 事業者が出す公開鍵を DNS に置き、送信側の署名を有効にする。SES では Easy DKIM の CNAME レコード 3 つ。サーバー自身は署名しないので、署名は事業者が行う。
- **DMARC**: `_dmarc.social.example` の TXT レコード。**`p=none` から始め**、集計レポート（`rua=`）で SPF と DKIM の整合（From のドメインと一致すること）を確かめてから、`quarantine`、`reject` と強める。

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

SES で SPF の整合を取るには、独自の MAIL FROM ドメインを設定します（既定の MAIL FROM は `amazonses.com` で、From のドメインと一致しません）。DKIM が整合していれば DMARC は通ります。

## 送信の失敗を見る

送信の失敗はジョブのダッシュボードで見ます（`/admin/jobs`、権限 `manage_jobs`、ADR-0064。詳しくは [ジョブのキューと失敗したジョブ](running.md#ジョブのキューと失敗したジョブdead-job)）。
ジョブの型で絞ると、メールのジョブだけを見られます（管理 API なら `GET /api/v1/admin/jobs?type=identity.verification_email`）。
型は上の表のとおりです。

- **一時的な失敗**（接続・タイムアウト・4xx の応答）は再試行されます。取引のメールは 6 回（30 秒からの指数で、合計およそ 15 分）、通知のメールは既定の方針（10 回、およそ 4 時間）です。
  使い切ると失敗したジョブ（dead job）になります。`smtp_host` の誤り、認証の失敗、ネットワークの遮断はここに出ます。直したら `my job retry <id>` かジョブの画面で再実行します。
- **恒久的な拒否**（宛先や内容への 5xx）は再試行せず、すぐ失敗にします。
- 取引のメールは、要求から一定時間（リセット 15 分、確認 1 時間）を過ぎると、送らずに完了します。古いリンクが遅れて届かないようにするためで、失敗ではありません。
- ジョブの実行時に、アドレスが抑止されている、確認済みになった、通知の権限や設定を失った、などで送る理由がなければ、送らずに完了します。

ジョブのペイロードにはメールアドレス・本文・トークンを積まず、エラーの文にもそれらを入れません。ID、ジョブの型、SMTP の応答コードだけが残ります。
成功した送信は、1 回ごとの構造化ログ `job` の行（`my.job.type`）で辿れます。

## メールが届かないアドレスの抑止

宛先のアドレスが存在しないと SMTP サーバーが答えたとき（`RCPT TO` への 5xx で拡張ステータスが `5.1.x`）、サーバーはそのアドレスをハードバウンスとして記録し、以後の送信を止めます（ADR-0086 §5）。
`mail = smtp` で記録するのは、この送信時の拒否だけです。送信を受け付けたあとに返る非同期のバウンスや、苦情のフィードバックループは受けません。

| 種類 | 止めるメール |
| --- | --- |
| `hard_bounce` | 確認のメールを除くすべて。パスワードのリセットのメールも止まる |
| `complaint`（迷惑メールとしての報告） | 通知と告知のメール。取引のメールは送る |

- モデレーターは管理画面のアカウントのページ（`GET /api/v1/admin/accounts/{id}` の `email_suppression`）で、抑止の種類と起点を見ます。
- **解除**は 2 通りです。利用者が確認のメールを求めて、そのアドレスを確認し直すと、ハードバウンスの抑止は解けます（確認のメールは抑止中も送ります）。
  メールボックスを直したなど送ってよいと確かめたときは、モデレーターが `POST /api/v1/admin/accounts/{id}/email_suppression/lift`（権限 `manage_accounts`）で解除します。監査ログに `email_suppression_lifted` を記録します（[モデレーション](moderation.md)）。
- パスワードのリセットのメールが止まっている利用者には、管理者がリセットのリンクを発行して渡します（`my admin account password-reset-link @alice`）。
- 利用者は設定画面で、主アドレスにメールが届いていないことと、その時期を見ます。

## 配信停止

通知のメールには、配信停止のリンクと `List-Unsubscribe` ・`List-Unsubscribe-Post: List-Unsubscribe=One-Click` のヘッダ（RFC 8058）を付けます。メールクライアントの「配信を停止」ボタンがこれを使います。

- 配信停止の区分は、利用者設定のメールの項目と 1 対 1 です。今ある区分は `email_moderator_notifications`（モデレーターへの通知）です。
- リンクは `https://{host}/mail/unsubscribe?token=...` で、ログインは要りません。トークンはメールごとに発行し、DB にはハッシュだけを残し、90 日で失効します。
  - `GET` は確認の画面を出すだけで、設定は変えません（リンクを先読みするスキャナーで止まらないように）。ボタンで同じ `POST` を送ります。
  - `POST`（ワンクリック）は区分の設定を off にします。期限までは何度送っても結果は同じです。
- **取引のメールは配信停止にできません**。確認・リセット・パスワードの変更の通知は、止めると安全に関わる知らせが届かなくなるためです。ヘッダも付きません。
- 利用者は設定画面からも同じ設定を切り替えられます。

## 開発環境

`mail = none`（既定）では送らずにログへ出すので、確認のリンクはログから取れます。ローカルの SMTP サーバー（Mailpit）を開発環境に入れる作業は [#1026](https://github.com/mycroblog/my/issues/1026) で進めています。
