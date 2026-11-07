<!-- source: sending-email 941ecea58cd9 -->
# メールの送信

サーバーが送るメール、送信の設定の仕方、失敗と配信停止の扱い方を説明します。
設定の一覧は [設定のリファレンス](../reference/configuration.md#mail) にあります。
メールアドレスの確認とパスワードの再設定の、利用者から見た流れは [登録とメールアドレスの確認](registration.md) にあります。

<!-- source: what-is-sent-and-how-to-configure-it 3f387cd1afcc -->
## 送るものと設定の仕方

サーバーがリクエストの中でメールを送ることはありません。メールのきっかけとなる事実と同じトランザクションで目的ごとにジョブを登録し、ジョブワーカーが送ります。送信の失敗は登録や再設定への応答には表れません。
利用者には「確認のメールを送りました」と伝え、届かなければ利用者がもう一度求めます。

| 分類 | メール | ジョブの種類 | 利用者が配信を停止できるか |
| --- | --- | --- | --- |
| トランザクション (`transactional`) | メールアドレスの確認 | `identity.verification_email` | いいえ |
| トランザクション | パスワードの再設定 | `identity.password_reset_email` | いいえ |
| トランザクション | パスワードの変更または削除のお知らせ | `identity.password_notice_email` | いいえ |
| トランザクション | アカウントの無効化のお知らせ（削除の日付つき） | `identity.deactivation_email` | いいえ |
| トランザクション | サービスの終了（告知、日付の変更、取り消し、終了日の 7 日前と 1 日前、終了、エクスポートの期限の 7 日前） | `identity.closure_email` | いいえ |
| 通知 (`notification`) | モデレーターへのお知らせ（お知らせの種類ごとに、既定でオン） | `notification.moderator_email` | はい |
| 通知 (`notification`) | 15 分間未読のままだった通知: メンション、フォロー、ブースト、ほかの利用者からのそのほかの通知、モデレーターからのお知らせ（種類ごとに、既定でオフ） | `notification.email` | はい |

サポーター向けの通知メール（`notification.supporter_email`）も同じジョブキューを通ります。

<!-- source: which-notifications-are-emailed 76c2b871762c -->
### メールで送る通知

各利用者は、設定の **通知** のページ（Web クライアントでは `/settings/notifications`）で、通知の種類ごとにアプリ内とメールで受け取るかを選びます。
「メール」の列が表示されるのは、サーバーがメールを送る間だけです。アプリ内でオフにした種類はそもそも作られないため、メールでもプッシュでも送られません。

| 種類 | 既定のメール | 送るとき |
| --- | --- | --- |
| ほかの利用者からの通知（メンション、新しいフォロワー、フォローリクエスト、承認されたフォローリクエスト、ブースト、お気に入り、絵文字リアクション、引用、終了した投票、フォローしているアカウントの引っ越し、通話） | オフ | 通知の 15 分後に、まだ未読（既読にされておらず、消されていない）で、まだ見られる状態で、利用者がまだその種類をメールで受け取る設定のときだけ。通知 1 件につき 1 通 |
| モデレーターからのお知らせ（アカウントへのモデレーションの対応、解決した通報、決定した異議申し立て、停止によって失われたフォロー、決定したカスタム絵文字の申請） | オフ | 上と同じ |
| モデレーターへのお知らせ（新しい通報、承認待ちの登録、異議申し立て、無効になった Webhook、カスタム絵文字の申請、制限されたアカウントの引っ越し、終了日の経過、新しいエラーと繰り返すエラー）。その権限を持つロールの人に送ります | オン | すぐに。読まれるのを待ちません |
| サービスの終了と、サポーターのプランのお知らせ | 常に | すぐに。オフにはできません |

通知メールは、確認済みになった利用者のメインのアドレスにだけ送り、メールを止めたアドレス（下を参照）には送りません。ほかの利用者からの通知のメールの件名は、その Web Push のメッセージと同じ文言で、メールは投稿、プロフィール、または通知へのリンクを含みます。

サービスの終了のメールは、メインのアドレスが確認済みのすべての利用者に送ります。1 通でも見逃すとデータを失うおそれがあるため、トランザクションのメールとして送り、配信停止のリンクを付けず、苦情でも止めません（ハードバウンスでは止まります）。
告知、日付の変更、取り消し、終了は、それぞれの記録と同じトランザクションで `identity.closure_email_fanout` を登録します。終了日の 7 日前と 1 日前、エクスポートの期限の 7 日前のリマインダーをいつ送るかは、1 時間ごとの `identity.queue_closure_reminders` が決めます。告知の時点ですでに時刻を過ぎていたリマインダーは送りません。告知のメールに日付が書かれているためです。
ファンアウトのジョブは利用者ごとに `service_closure_emails` の行を記録し、記録できた利用者についてだけ `identity.closure_email` を登録するため、ジョブが何度走っても、各利用者は各メールを 1 回だけ受け取ります。ジョブは送る前に、メールがまだ有効であること（取り消した告知や、その後に変わった日付のためのものでないこと）を確かめます。

設定 `mail` で、メールの送り方を選びます。

| `mail` | 動作 |
| --- | --- |
| `none`（既定値） | 何も送らず、サーバーのログにメッセージの分類とともに `mail not sent (mail = none)` を書きます。`profile = development` では、その行にアドレス、件名、本文（リンクを含む）も入ります。メールを使わない構成と開発のためのものです |
| `smtp` | SMTP のサブミッションで送ります。STARTTLS を必須とし、認証と送信の前に証明書を検証します |
| `ses` | Amazon SES v2 の API（raw の内容での `SendEmail`）で送ります。AWS SDK の既定の認証情報チェーン（たとえば IAM ロール）で認証し、SMTP のパスワードを持ちません |
| `plugin` | HTTP の API しか提供しないメールサービスのために、各メッセージを自分で動かす [プラグイン](#a-plugin-mail--plugin) に渡します |

`smtp` の設定は `mail_from`、`mail_smtp_host`、`mail_smtp_port`（既定値 587）、`mail_smtp_username`、`mail_smtp_password` です。
`ses` の設定は `mail_from`、`mail_ses_region`、`mail_ses_configuration_set` です。`plugin` の設定は `mail_from` と `plugin_mailer_address`、および [接続先](plugins.md#the-destination) のそのほかの設定です。
`mail_from` は `noreply@social.example` のような素のアドレスです。`From` は `"<server name>" <mail_from>` になり、`mail_from` のドメインは `Message-ID` のドメインでもあり、SPF、DKIM、DMARC が適用されるドメインでもあります（下を参照）。
インスタンスの設定 `contact_email` を設定しているときは、それが `Reply-To` になります（[インスタンスの設定](instance-settings.md)）。
すべてのメールに `Auto-Submitted: auto-generated` を付けます。
`smtp` と `ses` は同じメッセージを組み立てます。SMTPUTF8 は使わないため、アドレスは ASCII で送り、国際化ドメインは Punycode に変換します。ローカル部（`@` の前）が ASCII でないアドレスには送れず、そのジョブは失敗します。

<!-- source: examples-by-provider 7ab129a02c43 -->
## 事業者ごとの例

シークレットは環境変数またはファイル（`MY_MAIL_SMTP_PASSWORD` または `MY_MAIL_SMTP_PASSWORD_FILE`）で渡してください。設定ファイルには書かないでください。

<!-- source: amazon-ses-api 1cef4371e61c -->
### Amazon SES (API)

AWS で動かすときは、`mail = ses` で SES v2 の API を使います。認証情報は AWS SDK の既定のチェーン（ECS のタスクロール、環境変数、共有の設定ファイル）から得るため、設定するシークレットはありません。

1. SES で、`mail_from` のドメインを ID として検証し、Easy DKIM を有効にします（下を参照）。
2. サンドボックスを出るため、本番アクセスを申請します。サンドボックスでは検証済みのアドレスにしか送れません。
3. 実行ロールに、`mail_from` の ID（と、使う場合は設定セット）に対する `ses:SendEmail` を許可します。

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

`mail_ses_configuration_set` を設定すると、すべてのメールがその設定セットを使います。SES はメールの `Message-ID` と `Date` を独自の値で上書きします。
SES は送信の時点で受信者を同期的に拒否することはありません。アカウントのサプレッションリストにあるアドレスへの送信も受け付けたうえで配信せず、バウンスはあとから通知（SNS）として届きます。
バウンスと苦情を記録するには、下の SES の通知が SNS を通して届くよう設定してください。

<!-- source: a-plugin-mail--plugin 65b46feaf1cf -->
### プラグイン (`mail = plugin`)

HTTP の API しか提供しないメールサービスには、種類 `mailer` の [プラグイン](plugins.md) を動かし、そのサービスを呼び出させます。

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

この値では `plugin_mailer_address` が必須です。メッセージには確認のリンクのようなシークレットが含まれることがあるため、この種類はループバックでも `http://` を拒否します。Unix ソケットか `https://` を使ってください。サーバーはプラグインを呼び出さずに起動するため、プラグインが停止していても起動し、メールはそのジョブの中で待ちます。

`plugin_mailer_protocol = json`（既定値）では、プラグインに必要なのは HTTP サーバーと JSON ライブラリだけです
（[ライブラリを使わないプラグイン](plugins.md#a-plugin-with-no-library)）。`Send` は最長 30 秒、または 5 分未満の `plugin_mailer_timeout` まで続きます。リクエストは最大 2 MiB、レスポンスは最大 64 KiB です。

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

サーバーはプラグインを呼び出す前にメッセージを確かめるため、これらのどのフィールドからもヘッダーを注入することはできません。リクエストはメッセージの部品を持ち、プラグインはサービスが必要とするものを組み立てます。

- **プラグインは 3 つのヘッダーを加えます:** `envelopeFrom` のドメインで、メッセージごとに一意の `Message-ID`、`Date`、そして自動応答がループするのを防ぐ `Auto-Submitted: auto-generated` です。独自の `Message-ID` と `Date` を書くサービスなら、その 2 つはサービスに任せられます。
- `From` は `fromName` と `envelopeFrom` です。`fromName`、`replyTo`、`html`、`headers` はないことがあります。`headers` が持つのは 2 つの `List-Unsubscribe` のヘッダーだけです。
- アドレスは UTF-8 です。サービスが ASCII を必要とするプラグインはドメインを Punycode に変換し、送れないローカル部には `OUTCOME_INVALID` で応答します。
- 失敗したジョブは再試行されるため、同じメッセージが 2 回届くことがあります。メッセージをログに書かないでください。

`outcome` は、メッセージがどうなったかを伝えます。

| `outcome` | 意味 | サーバーの動作 |
| --- | --- | --- |
| `OUTCOME_SENT` | サービスがメッセージを受け取った | ジョブを完了します |
| `OUTCOME_INVALID` | そのままでは送れない | ジョブをすぐに失敗させます |
| `OUTCOME_RECIPIENT_REJECTED` | アドレスが存在しない | ジョブをすぐに失敗させ、[アドレスへの送信を止めます](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | サービスがほかの理由で恒久的に拒否した | ジョブをすぐに失敗させます |
| `OUTCOME_TEMPORARY` | サービスが一時的に拒否した、または応答しなかった | ジョブを再試行します |
| それ以外、またはなし | | ジョブを再試行します |

HTTP のエラー、または対応するステータスを伴う Connect の `{"code":"unavailable"}` も、一時的な失敗です。
`enhancedStatus` は任意で、`5.1.1` のような拒否の拡張ステータスコードです。サーバーは、拒否のクラス 5 のコードと `OUTCOME_TEMPORARY` のクラス 4 のコードを保持し、それ以外は無視します。プラグインからの文言は何も保持しないため、そこに理由を入れないでください。理由はプラグイン自身のログに書きます。

サーバーがバウンスを知るのは、この応答からだけです。プラグインのサービスのあとからのバウンスと苦情を受けるエンドポイントはないため、サービスがあとから報告するバウンスでアドレスへの送信が止まることはありません。

プラグインは `my` のバイナリでテストします。

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

1 つ目はプラグインが応答することだけを確かめ、何も送りません。2 つ目は `--mail-to` に実際のメッセージを送り、さらに受信者のないリクエストをプラグインに送ります。プラグインはこれを拒否しなければなりません。プラグインに渡ったあとのメッセージはサーバーからは見えないため、届いたメッセージを見てください。そのヘッダーには `Message-ID`、`Date`、`Auto-Submitted: auto-generated` が入っていなければなりません。
[プラグインをテストする](plugins.md#test-a-plugin) を参照してください。

<!-- source: receive-ses-notifications-sns e21b650ab5fe -->
### SES の通知を受け取る (SNS)

SES のバウンスと苦情は SNS のトピックに送られ、そのトピックの HTTPS のサブスクリプションがサーバーの `POST /mail/events/ses` に届けます。
このエンドポイントは `mail_ses_sns_topic_arn` を設定したときだけ開き、それ以外のときは `404` を返します。

1. SES の ID と同じリージョンに、標準の SNS のトピックを作成します。
2. トピックの属性 `SignatureVersion` を `2`（SHA256withRSA）にします。既定の `1` の署名は受け付けず、サーバーは `400` を返します。
3. トピックのアクセスポリシーで、自分のアカウントからの `ses.amazonaws.com` による `SNS:Publish` を許可します（SES のコンソールでトピックを作成すると、これが加えられます）。
4. SES の通知をトピックに向けます。方法は次のどちらでもかまいません（サーバーは本文の `notificationType` と `eventType` の両方を読みます）。
   - ID の通知: `mail_from` の ID の「Bounce」と「Complaint」のフィードバック通知に、このトピックを使います。
   - 設定セットのイベントの送信先: `mail_ses_configuration_set` の設定セットの送信先として、イベントの種類 `BOUNCE` と `COMPLAINT` についてこのトピックを加えます。
5. `MY_MAIL_SES_SNS_TOPIC_ARN` にトピックの ARN を設定し、`my config check` で確かめて再起動します。
6. トピックに、プロトコル `https`、エンドポイント `https://<server host>/mail/events/ses` のサブスクリプションを作成します。
   **raw メッセージの配信は有効にしないでください**（署名のない本文は拒否されます）。

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

サーバーはサブスクリプションの確認（`SubscriptionConfirmation`）を受け取ると、その `SubscribeURL` に `GET` を送ってサブスクリプションを確認し、`SNS subscription of the SES notifications confirmed` をログに書きます。
`aws sns list-subscriptions-by-topic` で、サブスクリプションが `PendingConfirmation` でなくなったことを確かめてください。

エンドポイントは、メッセージを使う前に次のすべてを確かめます。どれかを満たさなければ `400` を返し、何も記録しません。

- `TopicArn` が `mail_ses_sns_topic_arn` と等しい。
- `SignatureVersion` が `2` で、署名が正しい。
- 署名の証明書の URL（`SigningCertURL`）が `https` で、そのホストがトピックのリージョンの `sns.<region>.amazonaws.com` である。サブスクリプションの確認の `SubscribeURL` も同じホストでなければなりません。

証明書は SSRF を防ぐ送信用の HTTP クライアントで取得し、KV ストアに 24 時間キャッシュします。本文は 1 MiB まで読み、それより大きいものには `413` を返します。
証明書を取得できないとき、サブスクリプションの確認に失敗したとき、または記録を書けないときは、エンドポイントは `503` を返し、再試行を SNS に任せます。メンテナンスモードの間も `503` を返します。

記録する通知は次のとおりです。受信者のアドレスをメインのアドレスと照合し（大文字と小文字を区別せず、Punycode のドメインは Unicode の形でも一致します）、一致するものがなければ何もしません。

| 通知 | 記録する種類 |
| --- | --- |
| `bounceType` が `Permanent` のバウンス（`General`、`NoEmail`、`Suppressed`、`OnAccountSuppressionList` など、サブタイプは問いません） | `hard_bounce` |
| 苦情（`complaintFeedbackType` が `not-spam` のものを除く） | `complaint` |
| `Transient` または `Undetermined` のバウンスと、配信や送信などそのほかの通知 | 記録しません（`200` を返します） |

SES の `feedbackId` が記録の一意のキーなので、SNS が同じ通知を何度送っても、記録は 1 つです。

<!-- source: amazon-ses-smtp-interface f46a7a562966 -->
### Amazon SES (SMTP インターフェース)

AWS の外で動かすときなど、IAM の認証情報を与えられないときは、`mail = smtp` で SES の SMTP インターフェースを使います。この構成で送信停止として記録されるのは、送信の時点での受信者の拒否だけです
（[メールを受け取れないアドレスへの送信の停止](#suppression-of-addresses-that-cannot-receive-email)）。

1. SES で、`mail_from` のドメインを ID として検証し、Easy DKIM を有効にします（下を参照）。
2. サンドボックスを出るため、本番アクセスを申請します。サンドボックスでは検証済みのアドレスにしか送れません。
3. SES の「SMTP settings」で SMTP の認証情報を作成します（IAM のアクセスキーとは別のものです）。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

SES は `Message-ID` と `Date` を独自の値で上書きします。

<!-- source: a-general-smtp-submission-server 62921988a793 -->
### 一般的な SMTP のサブミッションサーバー

Postfix のような自前の SMTP サーバーや、メール配信事業者のサブミッションサーバーに使います。

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

STARTTLS を提供しないサーバーや、`mail_smtp_host` の名前で証明書を検証できないサーバーには送りません。認証情報は STARTTLS のあとでだけ送ります。
設定を変えたら、`my config check` で確かめて再起動します。

<!-- source: spf-dkim-and-dmarc d6b36a4f4d74 -->
## SPF、DKIM、DMARC

Gmail と Yahoo は、1 日に多くのメールを送る送信者に SPF、DKIM、DMARC、ワンクリックでの配信停止を求めています。少ない量でも、これらのないメールは迷惑メールに分類されやすくなります。
すべて、`mail_from` のドメイン（例: `social.example`）の DNS に、事業者から示された値で設定してください。

- **SPF**: 送信に使う事業者を許可する TXT レコードを、ドメインに 1 つ置きます（たとえば SES のカスタム MAIL FROM ドメインを使う場合は、事業者が指定する `include:` をそのドメインに置きます）。
- **DKIM**: 事業者が発行する公開鍵を DNS に置き、送信側で署名を有効にします。SES では Easy DKIM の 3 つの CNAME レコードです。サーバー自体は署名しないため、署名は事業者が行います。
- **DMARC**: `_dmarc.social.example` の TXT レコードです。**`p=none` から始め**、集計レポート（`rua=`）で SPF と DKIM がアラインしている（From のドメインと一致している）ことを確かめてから、`quarantine`、`reject` へと厳しくします。

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

SES で SPF をアラインさせるには、カスタム MAIL FROM ドメインを設定します（既定の MAIL FROM は `amazonses.com` で、From のドメインと一致しません）。DKIM がアラインしていれば、DMARC は通ります。

<!-- source: look-at-send-failures ab9c4f356d3b -->
## 送信の失敗を見る

送信の失敗はジョブのページ（`/admin/jobs/list`、権限 `manage_jobs`。[ジョブキューと失敗したジョブ](running.md#the-job-queue-and-failed-dead-jobs) を参照）で見ます。
ジョブの種類で絞り込むと、メールのジョブだけを見られます（管理 API では `GET /api/v1/admin/jobs?type=identity.verification_email`）。
種類は上の表にあります。

- **一時的な失敗**（接続のエラー、タイムアウト、4xx の応答）は再試行します。トランザクションのメールは 6 回試み（30 秒から始まる指数バックオフで、合計約 15 分）、通知のメールは既定の方針（10 回、約 4 時間）に従います。試行を使い切ると、ジョブは失敗したジョブ（dead job）になります。誤った `mail_smtp_host`、認証の失敗、ネットワークの遮断はここに表れます。原因を直したら、`my job retry <id>` またはジョブの画面からもう一度実行します。
- **恒久的な拒否**（受信者または内容に対する 5xx）は再試行せず、すぐに失敗します。
- `mail = ses` では、SES の `MessageRejected`（内容が拒否された）は恒久的な拒否です。`TooManyRequestsException`（送信レートの上限）、`SendingPausedException`、`MailFromDomainNotVerifiedException`、`AccountSuspendedException`、SES に届かないリクエストは一時的な失敗で、アカウントや権限を直せば再試行が成功します。
  エラーの文言に残るのは SES のエラーコードだけです。
- `mail = plugin` では、応答の [`outcome`](#a-plugin-mail--plugin) で決まります。到達できないプラグインやエラーを返すプラグインは一時的な失敗で、[`my.plugin.failures`](running.md) に数えられます。エラーの文言に残るのは結果と拡張ステータスコードだけなので、失敗の理由はプラグインのログにあります。
- 要求からの制限時間（再設定は 15 分、確認は 1 時間）を過ぎたトランザクションのメールは、送らずに完了します。古いリンクが遅れて届かないようにするためのもので、失敗ではありません。
- ジョブが走ったときに送る理由がない（アドレスへの送信が止まっている、その間に確認済みになった、利用者が通知の権限や設定を失った）ときは、送らずに完了します。

ジョブのペイロードはメールアドレス、本文、トークンを持たず、エラーの文言も同様です。残るのは ID、ジョブの種類、SMTP の応答コードだけです。
送信の成功は、実行ごとの構造化された `job` のログ行（`my.job.type`）で追えます。

<!-- source: suppression-of-addresses-that-cannot-receive-email 18b4750f1531 -->
## メールを受け取れないアドレスへの送信の停止

SMTP サーバーが受信者のアドレスは存在しないと伝えたとき（`RCPT TO` に対する、拡張ステータス `5.1.x` の 5xx）、サーバーはそのアドレスをハードバウンスとして記録し、そのアドレスへの送信を止めます。
`mail = plugin` では、`OUTCOME_RECIPIENT_REJECTED` が同じハードバウンスを記録し、ほかのものは記録しません。
`mail = smtp` で記録するのは、この送信の時点での拒否だけです。SMTP サーバーがメールを受け付けたあとに戻ってくる非同期のバウンスや、苦情のフィードバックループは受け取りません。
`mail = ses` では、SES が受信者を同期的に拒否することはないため、送信の時点では何も記録せず、恒久的なバウンスと苦情を SES の通知から記録します
（[SES の通知を受け取る (SNS)](#receive-ses-notifications-sns)）。

| 種類 | 止めるメール |
| --- | --- |
| `hard_bounce` | 確認のメールを除くすべて。パスワードの再設定のメールも止まります |
| `complaint`（迷惑メールとして報告された） | 通知とお知らせのメール。トランザクションのメールは引き続き送ります |

- モデレーターは、管理画面のアカウントの「管理」のページで、送信の停止の種類ときっかけを見られます（`GET /api/v1/admin/accounts/{id}` の `email_suppression`）。
- 停止を **解除** する方法は 2 つあります。利用者が確認のメールを求めてアドレスをもう一度確認すると、ハードバウンスによる停止は解除されます（アドレスへの送信が止まっている間も確認のメールは送ります）。利用者は、クライアント API（`POST /api/v1/accounts/email/verification`、1 時間に 3 回まで）またはログインの状態のページから求めます。
  メールボックスが直ったなど、送ってよいことを確かめたときは、モデレーターが `POST /api/v1/admin/accounts/{id}/email_suppression/lift`（権限 `manage_accounts`）で解除します。これは監査ログに `email_suppression_lifted` を記録します（[モデレーション](moderation.md)）。
- パスワードの再設定のメールが止まっている利用者には、管理者が再設定のリンクを発行して渡します（`my admin account password-reset-link @alice`）。
- 利用者は設定の画面で、メインのアドレスにメールが届いていないことと、その時期を見られます。

<!-- source: unsubscribing 2806dbccc409 -->
## 配信停止

通知のメールには、配信停止のリンクと、ヘッダー `List-Unsubscribe` と `List-Unsubscribe-Post:
List-Unsubscribe=One-Click`（RFC 8058）を付けます。メールクライアントの「配信停止」のボタンはこれらを使います。

- 配信停止の分類は、通知の設定のうち利用者がメールを選べる行と 1 対 1 に対応します。たとえば `mention` や `admin_report` です（モデレーターへのお知らせの種類ごとに 1 つ、モデレーターからのお知らせには `moderation`）。
- リンクは `https://{host}/mail/unsubscribe?token=...` で、ログインは不要です。トークンはメールごとに発行し、データベースにはそのハッシュだけを保持し、90 日で期限切れになります。
  - `GET` は確認の画面を表示するだけで、何も変えません（リンクを先読みするスキャナーがメールを止めないようにするためです）。ボタンは同じ `POST` を送ります。
  - `POST`（ワンクリック）は、通知の設定のその行のメールをオフにし、ほかには何もしません。トークンが期限切れになるまでは、もう一度送っても同じ結果になります。
- **トランザクションのメールは配信停止できません。** 確認、再設定、パスワードの変更のお知らせを止めると、セキュリティに関わるお知らせが届かなくなるためです。これらのメールにはこれらのヘッダーも付けません。
- 利用者は、設定の **通知** のページで同じ設定を切り替えられます。

<!-- source: the-html-version 2ca008b1a48a -->
## HTML 版

すべてのメールは `multipart/alternative` で、同じ内容を `text/plain` と `text/html`（どちらも UTF-8）で持ちます。`development` のプロファイルで `mail = none` のときは、テキスト版だけをログに書きます。

- 外側のレイアウトは 1 つで、段落、ボタン、署名（サーバー名、URL、連絡先、メールを送った理由、配信停止のリンク）を持ちます。`<html lang dir>` は受信者のロケールから決まり、`ar` などの言語は右から左に配置されます。
- 外部のフォントやスタイルシートは読み込みません。色は既定のデザイントークンで、ライトとダークは `color-scheme` と `prefers-color-scheme` に従います。管理者のテーマの上書きはメールに影響しません。
- 画像は読み込みません。ただし、インスタンス自身のロゴを設定しているとき（[ロゴとアイコン](instance-settings.md#logo-and-icon)）は例外です。
  そのとき HTML 版はロゴ（`https://<host>/brand/...`。メールクライアントがダークの配色に従う場合はダークのロゴ）から始まり、URL はすべての受信者で同じなので、誰がメールを開いたかは何も伝わりません。外部の画像を読み込まないクライアントは、代わりにサーバー名（画像の代替テキスト）を表示します。ロゴがないとき、またはサーバーの URL が `https` でないときは、メールに画像は入りません。
- リンクになるのは `https` の URL だけです。`web_origin` が `http://{host}` の開発の構成のように URL が `http` のときは、URL をボタンや配信停止のリンクではなくテキストとして書き、署名のサーバーの URL は省きます。
- レイアウトはバイナリに含まれているため、実行時にサーバーで JavaScript が走ることはありません。
