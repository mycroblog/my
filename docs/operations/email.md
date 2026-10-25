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
| 取引 | サービスの終了（予告・日付の変更・取り消し・閉鎖日の 7 日前と前日・クローズ・持ち出しの期限の 7 日前） | `identity.closure_email` | できない |
| 通知（`notification`） | モデレーターへの通知（利用者設定 `email_moderator_notifications`） | `notification.moderator_email` | できる |

サポーターの通知のメール（`notification.supporter_email`）も同じジョブのキューで送ります。

サービスの終了のメール（[ADR-0088](../adr/0088-service-shutdown-modes.md) §2）は、主たるアドレスの確認が済んだ全利用者へ送ります。
見落とすとデータを失うので取引のメールとして送り、配信停止のリンクを付けず、苦情でも止めません（ハードバウンスでは止まります）。
予告・日付の変更・取り消し・クローズは、その記録と同じトランザクションで `identity.closure_email_fanout` を積みます。
閉鎖日の 7 日前と前日、持ち出しの期限の 7 日前は、1 時間ごとの `identity.closure_email_reminders` が送る時点を決めます。予告の時点で過ぎていた時点の分は送りません（予告のメールが日付を伝えるため）。
展開のジョブは利用者ごとに `service_closure_emails` の行を記録し、記録できた利用者にだけ `identity.closure_email` を積むので、ジョブが何度動いても同じメールは 1 人に 1 回です。
送る前に、そのメールがまだ有効か（取り消された予告や、変更前の日付のメールではないか）を確かめます。

メールの送り方は設定 `mail` で選びます。

| `mail` | 動き |
| --- | --- |
| `none`（既定） | 送らず、本文（リンクを含む）をサーバーのログに `mail not sent (mail = none)` として出す。メールを使わない構成と開発用 |
| `smtp` | SMTP の提出（submission）で送る。STARTTLS を必須とし、証明書を検証してから認証・送信する |
| `ses` | Amazon SES v2 の API（`SendEmail` の Raw content）で送る。認証は AWS SDK の既定の資格情報の連鎖（IAM ロールなど）で、SMTP のパスワードを持たない |

`smtp` の設定は `mail_from`・`smtp_host`・`smtp_port`（既定 587）・`smtp_username`・`smtp_password`、`ses` の設定は `mail_from`・`ses_region`・`ses_configuration_set` です。
`mail_from` は `noreply@social.example` のような裸のアドレスで、`From` は `"<サーバー名>" <mail_from>` になり、`Message-ID` のドメインと SPF・DKIM・DMARC の対象（下記）もこのドメインです。
`Reply-To` には、インスタンス設定の `contact_email` があればそれを付けます（[インスタンス設定](instance-settings.md)）。
すべてのメールに `Auto-Submitted: auto-generated` が付きます。
`smtp` と `ses` は同じ形のメールを組み立てます。SMTPUTF8 を使わないので、アドレスは ASCII で送り、国際化ドメインは Punycode に直します。ローカル部（`@` の前）が ASCII でないアドレスには送れず、そのジョブは失敗します。

## 事業者ごとの設定例

秘密は環境変数かファイルで渡します（`MY_SMTP_PASSWORD` または `MY_SMTP_PASSWORD_FILE`）。設定ファイルには書かないでください。

### Amazon SES（API）

AWS で動かすときは `mail = ses` で SES v2 の API を使います。資格情報は AWS SDK の既定の連鎖（Lambda・ECS の実行ロール、環境変数、共有の設定ファイル）から取るので、秘密の設定はありません。

1. SES で `mail_from` のドメインをアイデンティティとして検証し、Easy DKIM を有効にする（下記）。
2. 本番のアクセスを申請して、サンドボックスを出る。サンドボックスでは検証済みのアドレスにしか送れない。
3. 実行ロールに、`mail_from` のアイデンティティ（構成セットを使うならその構成セットにも）への `ses:SendEmail` を許可する。

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_SES_REGION=ap-northeast-1          # アイデンティティを検証したリージョン。空なら AWS_REGION
MY_SES_CONFIGURATION_SET=my-events    # 任意。イベントの送り先を持つ構成セット
```

`ses_configuration_set` を設定すると、すべてのメールにその構成セットを適用します。SES はメールの `Message-ID` と `Date` を自分の値で上書きします。
送信時に SES が同期で宛先を拒むことはありません。アカウントの抑止リストに載ったアドレスへの送信も受け付けたうえで送られず、あとから通知（SNS）でバウンスとして届きます。
バウンスと苦情を記録するには、次の SES の通知を SNS で受ける設定をします。

### SES の通知（SNS）を受ける

SES のバウンスと苦情を SNS のトピックに出し、そのトピックの HTTPS の購読でサーバーの `POST /mail/events/ses` に届けます（ADR-0086 §5）。
受け口は `ses_sns_topic_arn` を設定したときだけ開き、設定しなければ `404` を返します。

1. SES のアイデンティティと同じリージョンに、SNS の標準トピックを作る。
2. トピックの属性 `SignatureVersion` を `2`（SHA256withRSA）にする。既定の `1` の署名は受け付けず、`400` を返す。
3. トピックのアクセスポリシーで、`ses.amazonaws.com` に自分のアカウントからの `SNS:Publish` を許可する（SES のコンソールでトピックを作ると付く）。
4. SES の通知の送り先にトピックを指定する。次のどちらでもよい（サーバーは本文の `notificationType` と `eventType` の両方を読む）。
   - アイデンティティの通知: `mail_from` のアイデンティティの「Bounce」と「Complaint」のフィードバックの通知を、このトピックにする。
   - 構成セットのイベントの送り先: `ses_configuration_set` の構成セットに、イベントの種類 `BOUNCE` と `COMPLAINT` で、このトピックを送り先にする。
5. `MY_SES_SNS_TOPIC_ARN` にトピックの ARN を設定し、`my config check` で確かめてから起動し直す。
6. トピックに、プロトコル `https`、エンドポイント `https://<サーバーのホスト>/mail/events/ses` の購読を作る。**raw message delivery は有効にしない**（署名のない本文は拒否する）。

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # 起動し直してから次の購読を作る

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

サーバーは購読の確認（`SubscriptionConfirmation`）を受けると、その `SubscribeURL` を `GET` して購読を確認し、ログに `SNS subscription of the SES notifications confirmed` を出します。
`aws sns list-subscriptions-by-topic` で購読が `PendingConfirmation` でなくなったことを確かめます。

受け口は、メッセージを使う前に次をすべて確かめ、どれかが違えば `400` を返して何も記録しません。

- `TopicArn` が `ses_sns_topic_arn` と一致する。
- `SignatureVersion` が `2` で、署名が正しい。
- 署名の証明書の URL（`SigningCertURL`）が `https` で、ホストがトピックのリージョンの `sns.<region>.amazonaws.com`。購読の確認の `SubscribeURL` も同じホストに限る。

証明書は SSRF を防ぐ外向きの HTTP クライアントで取得し、KV に 24 時間キャッシュします。本文は 1 MiB まで読み、超えると `413` です。
証明書が取得できない、購読の確認に失敗した、記録できなかったときは `503` を返し、SNS の再送に任せます。メンテナンスモードの間も `503` です。

記録するのは次の通知です。宛先のアドレスを主アドレスと照合し（大文字と小文字は区別せず、Punycode のドメインは Unicode の形でも照合する）、一致しなければ何もしません。

| 通知 | 記録 |
| --- | --- |
| バウンスで `bounceType` が `Permanent`（`General`・`NoEmail`・`Suppressed`・`OnAccountSuppressionList` などの副種別） | `hard_bounce` |
| 苦情（`complaintFeedbackType` が `not-spam` のものを除く） | `complaint` |
| バウンスで `Transient`・`Undetermined`、配達・送信などのほかの通知 | 記録しない（`200` を返す） |

SES の `feedbackId` を記録の一意のキーにするので、SNS が同じ通知を何度送っても記録は 1 つです。

### Amazon SES（SMTP インタフェース）

AWS の外で動かすときなど、IAM の資格情報を渡せない構成では、SES の SMTP インタフェースを `mail = smtp` で使います。この構成では送信時の宛先の拒否だけを抑止として記録します（[メールが届かないアドレスの抑止](#メールが届かないアドレスの抑止)）。

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

SES は `Message-ID` と `Date` を自分の値で上書きします。

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
- `mail = ses` では、SES の `MessageRejected`（内容の拒否）が恒久的な拒否です。`TooManyRequestsException`（送信の速度の上限）・`SendingPausedException`・`MailFromDomainNotVerifiedException`・`AccountSuspendedException`、SES に届かない要求などは一時的な失敗で、アカウントや権限を直せば再試行で通ります。
  エラーの文には SES のエラーコードだけが残ります。
- 取引のメールは、要求から一定時間（リセット 15 分、確認 1 時間）を過ぎると、送らずに完了します。古いリンクが遅れて届かないようにするためで、失敗ではありません。
- ジョブの実行時に、アドレスが抑止されている、確認済みになった、通知の権限や設定を失った、などで送る理由がなければ、送らずに完了します。

ジョブのペイロードにはメールアドレス・本文・トークンを積まず、エラーの文にもそれらを入れません。ID、ジョブの型、SMTP の応答コードだけが残ります。
成功した送信は、1 回ごとの構造化ログ `job` の行（`my.job.type`）で辿れます。

## メールが届かないアドレスの抑止

宛先のアドレスが存在しないと SMTP サーバーが答えたとき（`RCPT TO` への 5xx で拡張ステータスが `5.1.x`）、サーバーはそのアドレスをハードバウンスとして記録し、以後の送信を止めます（ADR-0086 §5）。
`mail = smtp` で記録するのは、この送信時の拒否だけです。送信を受け付けたあとに返る非同期のバウンスや、苦情のフィードバックループは受けません。
`mail = ses` の送信では SES が同期で宛先を拒まないので、送信時の記録はなく、恒久的なバウンスと苦情を SES の通知から記録します（[SES の通知（SNS）を受ける](#ses-の通知snsを受ける)）。

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

## HTML の版

すべてのメールは `multipart/alternative` で、同じ内容の `text/plain` と `text/html`（どちらも UTF-8）を持ちます（ADR-0086 §2）。`mail = none` のログに出るのはテキストの版だけです。

- 外枠のレイアウトは 1 つで、段落・ボタン・署名（サーバー名・URL・連絡先・理由・配信停止のリンク）を持ちます。`<html lang dir>` は受信者のロケールから付き、`ar` などでは右から左に組みます。
- 外部のフォント・画像・スタイルシートを読みません。色はデザイントークンの既定値で、明暗は `color-scheme` と `prefers-color-scheme` で切り替わります。管理者のテーマの上書きはメールに効きません。
- リンクにするのは `https` の URL だけです。`web_origin` が `http://{host}` の開発環境のように URL が `http` のときは、ボタンや配信停止のリンクの代わりに URL を文字で出し、署名のサーバーの URL は省きます。
- 外枠のソースは `tools/mail/frame.mjml` です。`scripts/task.sh mail:gen` が MJML（Bun）でコンパイルして Go の `html/template` の `internal/app/kernel/mail_frame.gen.html` を作り、これをコミットしてバイナリに埋め込みます。サーバーの実行時に JavaScript は動きません。
  コミットした生成物が古いと、`scripts/task.sh mail:gen-check`（Merge Queue の `server-static`）が失敗します。

## 開発環境

`mail = none`（既定）では送らずにログへ出すので、確認のリンクはログから取れます。
実際に送られるメール（ヘッダ・HTML の版・リンク）を見るときは、開発用の SMTP サーバー [Mailpit](https://mailpit.axllent.org/)（MIT）に `mail = smtp` で送ります。
Mailpit は受けたメールを誰にも届けずに保持し、Web UI（`http://127.0.0.1:8025`）と REST API で見せます。

- `mail = smtp` は STARTTLS と証明書の検証を省かないので、Mailpit は `scripts/mailpit-cert.sh` が作る開発用の CA の証明書（`mailpit`・`localhost`・`127.0.0.1` 向け、`.tmp/mailpit`）で STARTTLS を求め、`my serve` はその CA を `SSL_CERT_FILE` で信頼します。
  `SSL_CERT_FILE` はシステムのルート証明書を置き換えるので、その `my serve` から外のサーバーへの HTTPS（連合など）は通りません。開発の確認だけに使います。
- イメージ（`axllent/mailpit`）は、結合テストと同じものをタグとダイジェストで固定しています（`deploy/docker/mailpit/compose.yaml`）。

Docker Compose の構成（[`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)）に足すときは、上書きの [`compose.mailpit.yaml`](../../deploy/docker/compose.mailpit.yaml) を重ねます。`my` の `mail` を `smtp` にし、宛先を Mailpit にします。

```sh
scripts/mailpit-cert.sh
cd deploy/docker
docker compose -f compose.yaml -f compose.mailpit.yaml up -d --build
```

手元で動かす `my serve` から送るときは、Mailpit だけを起動して（`scripts/task.sh docker:mailpit`、止めるのは `docker compose -f deploy/docker/mailpit/compose.yaml down`）、次の設定で起動します。

```sh
MY_MAIL=smtp MY_MAIL_FROM=noreply@localhost MY_SMTP_HOST=localhost MY_SMTP_PORT=1025 \
  SSL_CERT_FILE=.tmp/mailpit/ca.pem my serve
```

どちらも、パスワードのリセットを求める（`/auth/forgot_password`）か、`open` モードで登録すると、メールが Mailpit の UI に届きます。UI で開いたメールのリンクを、ブラウザで開きます。

`mail = smtp` の結合テストも Mailpit を相手にします（`scripts/task.sh go:test-mail`、Docker が要る）。`pkg/testkit/mailpittest` がテストごとにコンテナを起動し、
アダプタが送ったメールのヘッダ・エンコード・本文と STARTTLS・認証の組み合わせを確かめ、登録の確認とパスワードのリセットを、Mailpit が受けたメールのリンクで通します。
CI では遅い検査の組（`slow-suite.yml` の `mail-integration`、リリース前と夜間）で回り、PR と Merge Queue では SMTP サーバーのテスト内のスタンドインで同じアダプタを確かめます。
