# ADR-0086: メール送信のジョブ化・テンプレート・配信停止

- Status: Accepted（§2 の「外部の画像を読まない」に、サーバーのロゴの例外を ADR-0116（サーバーのロゴとアイコンを SVG で設定する）が加える）
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1025

## Context（背景・課題）

サーバーは次のメールを送る。Mailer ポート（`internal/app/kernel/mailer.go`）と、アダプタ `mail/none`（ログに出す）・`mail/smtp`（STARTTLS 必須）があり、`mail/ses` はパッケージの雛形だけがある。

| メール | 要件 | 送り方 |
| --- | --- | --- |
| メールアドレスの確認・再送 | FR-002 | リクエストの中で、コミットの後に同期で `Mailer.Send` |
| パスワードのリセット | FR-002 | 同上 |
| パスワードの変更・削除の通知 | FR-002 | 同上（失敗はログだけ） |
| モデレーターへの通知 | FR-712 | outbox → ジョブ `JobModeratorEmail`（利用者設定 `email_moderator_notifications`） |
| サポーターの通知（支払いの失敗など） | FR-910–FR-917 | outbox → ジョブ `JobSupporterEmail` |

v0 のリリース（M5）に向けて、次が欠けている。

- **送り方**: 同期の送信は、SMTP が遅い・一時的に落ちているとリクエストが遅れ・失敗し、再送もない。ジョブで送るものも、宛先の恒久的な拒否と一時的な失敗を区別せず、どちらも再試行する
- **内容**: 本文はプレーンテキストだけで、件名と本文を各ユースケースが個別に組み立てている。共通の外枠（サーバー名・連絡先・設定へのリンク）、`From` の表示名、`Reply-To`、`List-Unsubscribe`（RFC 8058）、`Auto-Submitted`（RFC 3834）がない
- **配信の停止**: 通知のメールを止めるには、ログインして設定画面を開くしかない
- **到達性**: SPF・DKIM・DMARC の運用文書と、バウンス・苦情の扱いがない。届かないアドレスに送り続ける
- **SES**: AWS のターゲット（M6）の `mail/ses` が未実装

ジョブの規約は ADR-0004（at-least-once・冪等なハンドラ）、トランザクションの規約は ADR-0003（外部 I/O をしない）、ジョブの失敗の見え方は ADR-0064、データの形は ADR-0020、生成物のコミットは ADR-0081 が決めている。
[ADR-0084](0084-maintenance-mode.md)（メンテナンスモード）・[ADR-0088](0088-service-shutdown-modes.md)（サービスの終了）は全利用者へメールを送ることがあり、この ADR の送り方と区分に乗る。

## Decision Drivers（判断基準）

1. **秘密を残さない**: 確認・リセットのリンクの秘密を、ジョブのペイロード（SQS・Cloud Tasks ではクラウドに置かれる）・`dead_jobs`・ログに残さない（ADR-0019、ADR-0064 §2）
2. **リクエストを外部 I/O から切り離す**: SMTP の遅延・障害が登録やリセットのリクエストを遅らせない。再送がある（ADR-0003・ADR-0004）
3. **到達性**: 主要なメールボックス事業者の送信者要件を満たし、届かないアドレスに送り続けない。Gmail はすべての送信者に SPF か DKIM を、1 日 5,000 通を超えて Gmail に送る送信者に SPF・DKIM・DMARC（`From` のドメインとの整合）と、マーケティング・購読したメールのワンクリック配信停止（`List-Unsubscribe-Post: List-Unsubscribe=One-Click`）と本文の配信停止のリンクを求める。Yahoo も同様で、DMARC は `p=none` 以上、配信停止は 2 日以内に反映する（2026-10-23 に [Gmail](https://support.google.com/a/answer/81126)・[Yahoo](https://senders.yahooinc.com/best-practices/) で確認）。どちらも取引のメールは配信停止を求める対象に挙げていない
4. **見た目**: Mastodon・Misskey と並べて見劣りしない、サーバー名を出した HTML のメールを送る。RTL・ダークモードでも崩れない
5. **i18n と既存のカタログ**: ICU のメッセージ（ADR-0009）を使い、`server*` 名前空間の制約（リッチテキストのタグ・日時の引数を使わない）を守る
6. **単一バイナリ**: 実行時に JavaScript を動かさず、`CGO_ENABLED=0` を保つ
7. **運用の少なさ**: 小さなサーバーの運営者が、SMTP の事業者を 1 つ設定するだけで送れる。受信の口は、それを使う構成（SES）でだけ開く

## Considered Options（選択肢）

論点ごとに選択肢を挙げる。

### 1. 送り方

- **1A. すべてのメールを用途ごとのジョブにし、ペイロードは ID だけ、秘密はハンドラが発行する**。
  ハンドラが実行時に宛先・ロケール（ADR-0009 §2）を引き、トークンを発行して、組み立てて送る
- **1B. 汎用の送信ジョブ `mail.send` に、組み立て済みのメッセージを積む**。秘密を含む本文は Sealer（ADR-0019 §6）で封じる
- **1C. 同期の送信を残し、失敗したときだけジョブで再送する**

### 2. 本文の形式

- **2A. プレーンテキストだけ**（`text/plain; charset=UTF-8`）。共通の外枠をテキストで足す
- **2B. `multipart/alternative` でテキストと HTML の両方を送る**。HTML は MJML から作る

### 3. バウンス・苦情

- **3A. 送信時の恒久的な拒否だけを扱い、非同期のバウンス・苦情は送信事業者の抑止リストに任せる**
- **3B. 届かないアドレスを自前で記録し、送信を止める**。送信時の宛先の拒否と、SES の通知（SNS）を記録する

### 4. `mail/ses`

- **4A. M6 で SES v2 の API（`SendEmail` の Raw content）で作る**。M5 では SES の SMTP インタフェースを `mail/smtp` で使える
- **4B. 作らない**。SES は SMTP インタフェースだけで使い、`mail = ses` の値を消す

## Decision（決定）

**1A・2B・3B・4A を採用する。** 以下を規約とする。

### 1. 送信はすべてジョブで行う

- リクエスト・ユースケースは `Mailer.Send` を直接呼ばない。メールを送る事実と同じトランザクションで、outbox に用途ごとのジョブを積む（ADR-0004）。
  `Mailer` を呼ぶのはジョブのハンドラだけにする
- **ペイロードは ID と分類だけ**（利用者・メールアドレス・通知の ID、要求の時刻）。宛先のアドレス・本文・秘密は積まない。ハンドラが実行時に、宛先・ロケール・サーバー名を引いて組み立てる。
  ADR-0064 の失敗したジョブの画面にペイロードが出ても個人情報・秘密を含まない
- **秘密はハンドラが発行する**。確認・リセットのジョブは、実行のたびに新しいトークンを発行して（`user_tokens` に hash を INSERT）、コミットしてから送る。
  秘密はメールの本文にだけ現れ、ペイロード・DB・ログには残らない。再試行や重複の実行では、別の有効なリンクを持つメールがもう 1 通届きうるが、どちらのリンクも 1 回だけ使え、期限で失効するので害はない
- **実行時の再確認**: ハンドラは送る前に、まだ送る理由があるかを確かめる（アドレスが未確認のまま主たるアドレスか、通知の権限と設定がまだあるか、アドレスが抑止されていないか（§5））。なければ送らずに完了する
- **冪等キー**: ジョブの型と、送る理由になった行の ID（通知の ID、確認の要求の ID）で作る。同じ要求から 2 通のジョブは積まない
- **期限**: 取引のメール（確認・リセット）は要求から一定時間（リセットは 15 分、確認は 1 時間）を過ぎたら、送らずに完了する。古いリセットのメールが遅れて届くのを防ぐ
- **再試行**: ポートのエラーを分ける。
  - `kernel.ErrRecipientRejected`（宛先のアドレスが存在しない: SMTP の `RCPT TO` への 5xx で拡張ステータスが `5.1.x`）。`ErrMailRejected` を包み、§5 の記録の対象になる
  - `kernel.ErrMailRejected`（それ以外の恒久的な拒否: `RCPT TO` / `DATA` へのその他の 5xx、SES v2 の `MessageRejected`（内容が不正））
  - それ以外は一時的な失敗（接続・タイムアウト・4xx、SES の `TooManyRequestsException`・`SendingPausedException`・`MailFromDomainNotVerifiedException` など、運営者が直せば通るもの）
  - 恒久的な拒否と `ErrInvalidMail` は `queue.Fail`、一時的な失敗は `queue.Retry`。再試行の方針は、取引のメールが 6 回（30 秒からの指数、合計およそ 15 分）、通知のメールが既定の方針（10 回、合計およそ 4 時間）
- **エラーの文**: ハンドラが返すエラーには、メールアドレス・トークン・本文を入れない（ADR-0064 §2）。ジョブの型・通知の ID・SMTP の応答コードだけを入れる
- 登録の応答は、送信の成否を返さない（「確認のメールを送りました」と表示し、届かなければ再送を求める）。送信の失敗はジョブのダッシュボード（`/admin/jobs`、ADR-0064）で見る

### 2. 組み立て（テンプレートと外枠）

- すべてのメールを `multipart/alternative` で送り、`text/plain` と `text/html`（どちらも UTF-8）を同じ内容で持つ
- 件名・本文の文は `locales/<locale>/server*.json` の ICU メッセージで持ち、生成した型付きアクセサで整形する（ADR-0009）。日時は Go 側で整形した文字列を引数に渡す。メッセージに HTML を書かない（段落は別のメッセージに分ける）
- **共通の外枠** を `internal/app/kernel` に置く組み立て器（`MailComposer` 相当）が付ける。ユースケースは「分類・宛先・ロケール・件名・段落の列・任意の行動（ボタンの文と URL）」を渡すだけにする:
  - 末尾の署名: サーバーの名前、サーバーの URL、`contact_email`（設定されていれば）、「このメールが届く理由」の 1 文
  - 通知・告知のメールには、設定画面へのリンクと配信停止のリンク（§4）
- **HTML のテンプレートは MJML から作る**:
  - 外枠の MJML（レイアウトは 1 つ。段落・ボタン・署名の差し込み口を持つ）をソースとしてリポジトリに置き、ビルド時に Bun で MJML をコンパイルして Go の `html/template` のファイルを作る。生成したファイルはコミットし（Go が読む生成物、ADR-0081）、`embed` でバイナリに入れる。生成が古くないかは既存の生成物の検査と同じく CI で確かめる
  - 実行時に JavaScript を動かさず、`CGO_ENABLED=0` を保つ。MJML は MIT ライセンスで、使う版は 5.4.1（2026-09-10 公開。2026-10-23 に [GitHub のリポジトリ](https://github.com/mjmlio/mjml) と npm で確認。リポジトリは保守が続いている）。開発時の依存として Bun のワークスペースに入れる
  - 差し込む値はすべて `html/template` の文脈に応じたエスケープを通す。利用者の HTML・Markdown を埋め込まない。URL は `https` のものだけを渡す
  - `<html lang dir>` をロケールから付ける（MJML の `<mjml lang dir>` で出力される）。外部のフォント・画像を読まない（コンパイル時に `fonts: {}` を渡し、システムのフォントの並びを使う）。明暗は `color-scheme` と `prefers-color-scheme` のスタイルで扱う
  - 2026-10-23 に、MJML 5.4.1 を Bun 1.4.3 でコンパイルし、`{{.X}}` の差し込み口が本文・属性・`<html lang dir>` に残ること、生成物を Go の `html/template` が読み、値をエスケープし `javascript:` の URL を無効化すること、`fonts: {}` で Google Fonts への参照が消えることを手元で確かめた
- テキストの版は同じ入力から Go の `text/template` で作る
- `Mailer` ポートの `MailMessage` に、分類（`transactional` / `notification` / `announcement`）・HTML の本文・`From` の表示名・`Reply-To`・追加のヘッダ（`List-Unsubscribe` 系だけを許す）を足す。アダプタはヘッダの注入を `ValidateMail` で拒否し続ける

### 3. ヘッダ

| ヘッダ | 値 |
| --- | --- |
| `From` | `"<サーバー名>" <mail_from>`（RFC 2047 で符号化。サーバー名がなければ host） |
| `Reply-To` | インスタンス設定 `contact_email`。未設定なら付けない（`mail_from` は `noreply@` を想定） |
| `Message-ID` | `<UUIDv7@mail_from のドメイン>`。SES は自分の `Message-ID` と `Date` で上書きする（2026-10-23 に [SendRawEmail](https://docs.aws.amazon.com/ses/latest/APIReference/API_SendRawEmail.html) で確認）ので、SES では付け替わった値になる |
| `Auto-Submitted` | すべてのメールに `auto-generated`（自動応答のループを防ぐ） |
| `List-Unsubscribe` / `List-Unsubscribe-Post` | 通知（`notification`）と告知（`announcement`）のメールだけ。`<https://{host}/mail/unsubscribe?token=...>` と `List-Unsubscribe=One-Click` |

確認・リセット・パスワードの変更の通知・支払いの失敗は取引のメールで、配信停止の対象にしない（止めると安全や契約に関わる知らせが届かない）。Mastodon も `List-Unsubscribe` 系のヘッダを通知のメールにだけ付ける（[notification_mailer.rb](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/mailers/notification_mailer.rb#L77-L81)、2026-10-23 に確認）。

### 4. 配信の停止

- 配信停止の区分は、利用者設定のメールの項目と 1 対 1 にする。今の区分は `email_moderator_notifications` だけで、通知のメールを足すときは区分と設定の列を一緒に足す。告知の区分は、そのメールを作るときに設定を足す。ADR-0088 の閉鎖の予告と持ち出しの期限のメールは、見落とすとデータを失うので取引のメールとして送る
- 配信停止の **トークンはメールごとにハンドラが発行する**（`user_tokens` の新しい用途 `email_unsubscribe`、区分を持ち、期限 90 日、hash だけを保存）。秘密の扱いは §1 と同じ
- エンドポイント `/mail/unsubscribe`（authpages、ログイン不要）:
  - `POST`（RFC 8058 のワンクリック。本文 `List-Unsubscribe=One-Click`）: トークンを検証し、その区分の設定を off にして `200` を返す。トークンは期限まで何度使ってもよい（冪等）
  - `GET`: 確認の画面を出し、ボタンで同じ `POST` を送る。リンクを先読みするスキャナーで止まらないよう、`GET` では変更しない
- 設定の変更は今の設定の更新と同じ経路（現在値、ADR-0020 規約 5）で行い、新しい表は作らない

### 5. バウンス・苦情

届かないアドレス・苦情を受けたアドレスを identity モジュールが記録し、送信を止める。

- **記録の出どころ**
  - すべてのアダプタ: 送信時の宛先の拒否（`ErrRecipientRejected`）。ハンドラがジョブを `Fail` にするのと一緒に、ハードバウンスとして記録する
  - `mail/ses`: SES の通知を SNS の HTTPS の購読で受ける（下記）。`Permanent` のバウンス（`General`・`Suppressed`・`OnAccountSuppressionList` などの副種別）をハードバウンス、苦情を苦情として記録する。`Transient`・`Undetermined` のバウンスは記録しない
  - `mail/smtp`: 送信時の拒否だけを記録する。非同期の DSN・フィードバックループは受けない（受信のメールボックスを持たない）
- **データの形**（ADR-0020。identity が所有し、`schema/<dialect>/01-identity.sql` に置く）

  | 表 | 分類 | 列 |
  | --- | --- | --- |
  | `email_delivery_failures` | イベント | `id`・`email_address_id`・種類（`hard_bounce` / `complaint`）・出どころ（`rejection` / `ses`）・`feedback_key`（一意。SES の `feedbackId`、送信時の拒否はジョブの ID と試行）・拡張ステータス（`5.1.1` など、任意）・`occurred_at` |
  | `email_suppression_lifts` | イベント | `id`・`email_address_id`・理由（`reverified` / `moderator`）・`lifted_at` |
  | `email_suppressions` | 投影 | `email_address_id`（主キー）・種類・起点の失敗の ID。最後の解除より後の失敗から導く。再構築のコマンドを持つ |

  - イベントにはアドレスの文字列を持たず、`user_email_addresses` を参照する（規約 6）。SES の通知のアドレスは正規化して `user_email_addresses` と照合し、一致しなければ何も記録しない
  - `feedback_key` の一意制約で、同じ通知を何度受けても 1 行になる（SNS の再送・ジョブの重複に強い）
- **送信への効き方**（§1 の実行時の再確認で判定する）
  - ハードバウンス: そのアドレスへのすべてのメールを送らない。ただし利用者が自分で求めた確認のメールの再送は送り、確認が済んだら解除のイベント（`reverified`）を記録する。パスワードのリセットは送らず、管理者が発行するリンク（FR-002）で代える
  - 苦情: 通知と告知のメールを送らない。取引のメールは送る
  - モデレーターは管理画面のアカウントの詳細で抑止の状態を見て、解除（`moderator`）できる。利用者は設定画面で、アドレスに届いていないことを見る
- **SES の通知の受け口**（M6、`mail/ses` と一緒に作る）
  - `POST /mail/events/ses`（interface 層の新しいパッケージ。決済の Webhook（ADR-0074 §3）と同じく、クライアント API ではない）。設定 `ses_sns_topic_arn` があるときだけ開き、ないときは `404`
  - SES の構成セットのイベントの送り先（SNS）か、アイデンティティの通知のどちらからでも受ける（本文の `eventType` / `notificationType` の両方を読む。2026-10-23 に [SES の通知の内容](https://docs.aws.amazon.com/ses/latest/dg/notification-contents.html) で確認）
  - **署名の検証**: 処理の前に必ず検証する。`TopicArn` が設定と一致しないものは拒否する。`SignatureVersion` は `2`（SHA256withRSA）だけを受け付け、運営者はトピックの属性 `SignatureVersion` を `2` にする（既定は `1`）。
    署名の証明書は、`SigningCertURL` が `https` で、ホストが `TopicArn` のリージョンの `sns.<region>.amazonaws.com` のときだけ取得し、KV にキャッシュする。
    署名する文字列は種類ごとに決まった項目を名前の順に並べる（2026-10-23 に [SNS の署名の検証](https://docs.aws.amazon.com/sns/latest/dg/sns-verify-signature-of-message.html)・[例のコード](https://docs.aws.amazon.com/sns/latest/dg/sns-example-code-endpoint-java-servlet.html)・[SHA256 の告知](https://aws.amazon.com/about-aws/whats-new/2022/09/amazon-sns-supports-message-signatures-based-sha256-hashing) で確認）
  - 証明書の取得と購読の確認（`SubscriptionConfirmation` の `SubscribeURL` への `GET`。ホストの条件は証明書と同じ）は、SSRF を防ぐ `kernel.HTTPClient` を通す。トランザクションの外で行う
  - 通知は検証のあと、1 つの短いトランザクションでイベントを INSERT し、投影を同期で更新する（ADR-0020 規約 7）。本文の上限を先に掛ける
  - SES の通知は 1 通に複数の宛先を含みうる。サーバーは 1 通に 1 宛先で送るので、苦情の宛先は特定できる（2026-10-23 に SES の通知の内容の文書で確認）

### 6. `mail/ses`

- **M6** で SES v2 の API（`sesv2.SendEmail`、Raw content）で作る。認証は SDK の既定の資格情報の連鎖（IAM ロール）で、SMTP のパスワードを持たない。設定はリージョン・任意の構成セット（configuration set）・§5 の `ses_sns_topic_arn`
- **M5** の間は、SES を使う運営者は SES の SMTP インタフェース（STARTTLS、587）を `mail = smtp` で使う。運用文書に例を載せる。この構成では送信時の拒否だけを記録する
- `mail/ses` は `testkit.MailerContract` を通す。恒久的な拒否を `ErrMailRejected` に写す
- SES は SMTPUTF8 に対応せず、アドレスは 7 ビットの ASCII（ドメインは Punycode）、`From` の表示名は MIME の encoded-word で書く（2026-10-23 に [SendRawEmail](https://docs.aws.amazon.com/ses/latest/APIReference/API_SendRawEmail.html) で確認）。`mail/smtp` と同じ組み立てで満たす
- SES のアカウントの抑止リスト（2019-11-25 以降のアカウントで既定で有効）に載ったアドレスへの送信は、受け付けたうえで送られず、同期のエラーにならない。その送信は `OnAccountSuppressionList` のバウンスとして SNS で届き、§5 で記録する。Gmail は苦情を SES に返さない（2026-10-23 に [SES の抑止リストの文書](https://docs.aws.amazon.com/ses/latest/dg/sending-email-suppression-list.html) で確認）

### 7. 運用文書

`docs/operations/email.md` を足す: 事業者ごとの設定例（SES の SMTP・SES の API、一般的な SMTP の提出）、`mail_from` のドメインの SPF・DKIM・DMARC（`p=none` から始めて整合を確かめる）、SES の SNS トピックと購読の作り方（`SignatureVersion` を `2` にする）、送信の失敗の見方（`/admin/jobs` のメールのジョブの型で絞る、ADR-0064）と抑止の見方・解除、開発環境の Mailpit（#1026）。

### 覆る条件

- 主要なメールクライアントで MJML の出力の表示が崩れる（RTL・ダークモード）と分かり、外枠の修正で直らなければ、HTML の作り方を見直す
- SMTP だけの構成で、非同期のバウンスを受けないことが送信の評判を損なうと分かったら、DSN の受信（受信のメールボックスか、事業者ごとの Webhook）を足す
- 秘密をハンドラで発行すると、ジョブの実行が遅れる環境（Cloud Tasks の遅延など）でリンクの期限の扱いが不自然になると分かったら、1B（Sealer で封じたペイロード）を再検討する

## Consequences（影響）

- 良い影響
  - SMTP の障害が登録・リセットのリクエストを遅らせず、失敗は再試行され、ジョブのダッシュボードで見える
  - 秘密がメールの本文にしか現れない。SQS・Cloud Tasks のアダプタでもクラウドに秘密を置かない
  - ヘッダと配信停止で、通知のメールが Gmail・Yahoo の送信者要件（Decision Drivers 3）に沿う
  - すべてのメールが同じ外枠・ヘッダ・見た目を持ち、ユースケースは文と行動だけを書く
  - 届かないアドレス・苦情を受けたアドレスへ送り続けず、モデレーターと利用者が状態を見られる
- 悪い影響・トレードオフ
  - 登録の直後に「送れなかった」と利用者に返せない。届かなければ利用者が再送を求める
  - ジョブの重複・再試行で、確認・リセットのメールが 2 通届くことがある（どちらのリンクも有効）
  - 配信停止のトークンの行がメールの数だけ増える（期限で消す。通知のメールの量は小さい）
  - MJML と Bun の生成の手順が 1 つ増え、外枠を変えるたびに生成物の差分が出る。HTML の表示をメールクライアントごとに試す手間がかかる
  - 抑止の表 3 つと、署名の検証を持つ受信の口が増える。SNS の署名の検証は自前で書く（公開鍵の取得と検証の誤りは偽の苦情で送信を止められる危険になる）
  - SMTP の構成では非同期のバウンスを受けないので、送信を受け付けた後に返るバウンスは記録されない
- 後続で必要になる作業（次の issue に分けて作る）
  1. ジョブ化: 確認・リセット・パスワードの通知をジョブにし、秘密の発行をハンドラに移す。`ErrRecipientRejected`・`ErrMailRejected` と再試行の方針、`mail/smtp` の応答コードの分類、`testkit.MailerContract` の拡張
  2. 組み立てとヘッダ: `MailMessage` の拡張、共通の外枠（テキスト）、`From`・`Reply-To`・`Message-ID`・`Auto-Submitted`、`locales/en`・`locales/ja` のメッセージ
  3. HTML: MJML の外枠、Bun の生成のタスクと CI の検査、`html/template` の埋め込み、明暗・RTL の表示の確認
  4. 配信停止: `user_tokens` の用途 `email_unsubscribe`（スキーマ）、`/mail/unsubscribe` の `GET` / `POST`、`List-Unsubscribe` 系のヘッダ
  5. 抑止: `email_delivery_failures`・`email_suppression_lifts`・`email_suppressions`（スキーマと投影・再構築）、送信時の拒否の記録、ハンドラでの判定、管理画面と設定画面の表示、モデレーターによる解除
  6. `mail/ses`（M6）: SES v2 の API、設定、コントラクトテスト、`POST /mail/events/ses`（SNS の署名の検証・購読の確認・通知の記録）
  7. 運用文書 `docs/operations/email.md`
  - 開発環境と結合テストの Mailpit は #1026 で別に進める

## Pros and Cons of the Options（詳細比較・任意）

### 1. 送り方

- **1A**: 良い — 秘密がペイロードに載らない。宛先・ロケールが実行時の値になる（ADR-0009 §2 と一致）。今のモデレーター・サポーターのジョブと同じ形。
  悪い — 用途ごとにハンドラを書く。秘密の発行がハンドラの中の DB 書き込みになり、ジョブの実行がトランザクションを 1 つ持つ
- **1B**: 良い — 送信の処理が 1 か所で、再試行・エラーの分類・計測をまとめやすい（Mastodon は Devise のメールも管理者への通知も ActionMailer の `deliver_later` でジョブにする。[user.rb](https://github.com/mastodon/mastodon/blob/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/models/user.rb#L448)、2026-10-23 に確認）。トークンの発行は今の流れのまま。
  悪い — 組み立て済みの本文とアドレスがペイロードに載り、失敗したジョブの画面と SQS・Cloud Tasks に個人情報が残る。秘密を Sealer で封じても、KEK を持つ者には読め、`dead_jobs` の保持期間（30 日）残る。ロケールが投入時に固定される
- **1C**: 良い — 多くの場合すぐ届き、送れなかったことを応答で伝えられる（Misskey はキューを通さず、リクエストの処理の中で nodemailer で送る。リセットでは送信を待たない。[request-reset-password.ts](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/server/api/endpoints/request-reset-password.ts#L94)、2026-10-23 に確認）。
  悪い — リクエストが SMTP の遅延に引きずられ、送信と再送の 2 つの経路を持つ。再送のジョブには結局秘密を渡す必要がある

### 2. 本文の形式

- **2A**: 良い — 作る・試す量が最小。HTML のエスケープ・メールクライアントごとの表示差・ダークモードの問題がない。読み上げに強い。
  悪い — 見た目を整えられない。長い URL がそのまま出る。RTL の言語の表示はメールクライアントの双方向の処理に任せる
- **2B**: 良い — 見た目・ボタン・サーバー名を出せる。Mastodon（[layouts/mailer.html.haml](https://github.com/mastodon/mastodon/tree/131aabcaf1f1f13e0615f8ad2748ab05f966e2c9/app/views/layouts)）・Misskey（[EmailService.ts](https://github.com/misskey-dev/misskey/blob/2dd2e114b72dc256bdfa0ce88a8387f5cf6bd348/packages/backend/src/core/EmailService.ts#L148-L157)）もテキストと HTML の両方を送る（2026-10-23 に確認）。MJML はメールクライアントごとの差（表のレイアウト・Outlook の条件付きコメント）を吸収した HTML を出す。`lang`・`dir` を文書に付けられる。
  悪い — テンプレートを 2 系統持ち、生成の手順と開発時の依存が増える。明暗・RTL・クライアントごとの表示の試験が要る

### 3. バウンス・苦情

- **3A**: 良い — 受信の口（SNS の HTTPS エンドポイント、DSN を受けるメールボックス）を持たない。作る量が最小。
  悪い — 事業者が抑止しない環境では、届かないアドレスへの送信が続く。SES の抑止リストに載った送信はジョブの上で成功に見え、利用者もモデレーターもアドレスに届かないことを知らない
- **3B**: 良い — 送信の評判を自分で守れる。どの事業者でも送信時の拒否は記録でき、SES では非同期のバウンス・苦情まで記録できる。モデレーターと利用者に「届かないアドレス」を見せられる。
  悪い — 表 3 つ・受信の口・SNS の署名の検証を作り、保守する。SMTP の構成では送信を受け付けた後のバウンスを拾えない

### 4. `mail/ses`

- **4A**: 良い — IAM ロールで認証でき、長期の SMTP の資格情報を持たない（Lambda・ECS で自然）。構成セットで SNS への通知を付けられる。aws-sdk-go-v2 は `queue/sqs` で既に依存している。
  悪い — アダプタとコントラクトテストを 1 つ増やす
- **4B**: 良い — 作るものがない。悪い — SMTP の資格情報（IAM ユーザーから派生）を秘密として管理する。Lambda から SMTP の接続を毎回張る

## Open Questions / 要検証

- 要検証: MJML の出力（RTL・ダークモード）が主要なメールクライアント（Gmail・Outlook・Apple Mail）で崩れないか。実際のクライアントでの確認が要る（後続の issue 3 で確かめる）
- 取引のメールの期限（リセット 15 分・確認 1 時間）と再試行の回数が妥当か。実装の issue で値を確かめる
