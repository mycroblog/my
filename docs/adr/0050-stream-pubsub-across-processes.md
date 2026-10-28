# ADR-0050: リアルタイム更新のプロセス間 Pub/Sub

- Status: Accepted（§5 のアダプタの契約テストを main への push で回す部分を ADR-0079（CI の検査を Merge Queue に一本化する） が置き換える）
- Date: 2026-10-19
- Deciders: @rosylilly
- Issue: #196

## Context（背景・課題）

リアルタイム更新（`GET /api/v1/streaming`、FR-207、[ADR-0016](0016-client-api-style.md)）は、コミットフックが `stream.Publisher` に知らせを出し、
各ストリームが `stream.Subscriber` で購読する（[overview §4.4](../architecture/overview.md#44-リアルタイム更新sse)）。
今の唯一の実装 `stream.Hub` は 1 つのプロセスの中だけで配るため、次の構成では知らせが届かない（#196）。

- web のプロセスが複数ある構成（Cloud Run の複数インスタンス、Lambda の複数の実行環境、k8s のレプリカ）
- web と worker を分けた構成（Inbox の処理で作られた通知は worker で生まれる）

決定者は「PubSub のアダプタを増やして設計し、実装しなさい。対象は AWS の Queue や GCP 上の類似サービス、あるいは valkey とします。」と指示している。
アダプタの選び方は ADR-0002（ポートとアダプタ）、ADR-0004（クラウドのマネージドサービスへの切り替え）、NFR-COST-1 / NFR-COST-2（scale to zero、常駐ミドルウェアを必須にしない）に従う必要がある。
Valkey の接続（`internal/adapter/valkey`）と設定は、KV の Valkey アダプタ（`kv = valkey`、#582）と共有する。

知らせの性質は次のとおりで、これが要件の上限を決める。

- 知らせは「何が変わったか」（投稿・ブーストの ID、通知を受けうるアカウントの ID）だけで、内容を持たない。受けたプロセスがユースケースで読み直し、公開範囲は `CanView`（ドメインの単一の方針関数）で決まる
- 配信は best effort。再接続したクライアントはホームと通知を読み直す
- 通知は「前回送ったものより新しい通知の一覧」を読み直すので、重複にも順序の入れ替わりにも強い。ホームの新着は、同じ ID が 2 度届くと同じ投稿を 2 度送る

## Decision Drivers（判断基準）

1. どのターゲットでも、すべての web プロセスがすべての知らせを受ける（fan-out）。損失はあってよいが、損失に気づいたら利用者が読み直せること
2. scale to zero を壊さない（NFR-COST-1）。ストリームを開いていないプロセスは、ブローカーに対してコストも接続も持たない
3. 常駐ミドルウェアを必須にしない（NFR-COST-2）。Self-hosted は RDB だけで複数プロセスに配れる
4. 公開範囲の判定をドメインの方針関数から出さない。ブローカーには内容を載せない
5. コアはクラウドの SDK を import しない（ADR-0002）。アダプタはポートのコントラクトテストを通し、毎 PR のテストはアカウントも Docker も要らない（ADR-0041 と同じ考え方）
6. 依存は AGPL-3.0 と両立し、`CGO_ENABLED=0` でビルドでき、公開から 3 日以上たった版

## Considered Options（選択肢）

全体の形:

1. **プロセス単位の放送のポート `stream.Bus` と、共通の中継 `stream.Relay`（`Hub` + `Bus`）**: アダプタは「すべてのプロセスへ同報し、受け取る」ことだけを実装する。購読の振り分け・上限・遅い購読者の切断は `Hub` のまま
2. 各アダプタが `Publisher` / `Subscriber` を丸ごと実装する: ブローカーの機能（アカウントごとのチャネルなど）を直接使えるが、振り分けと上限をアダプタごとに書き直す
3. ブローカーを置かず、DB の知らせテーブルを各プロセスがポーリングする: どこでも動くが、DB を起こし続け（Neon の scale to zero、Aurora の 0 ACU が効かない）、書き込みも増える

アカウントごとのチャネル（購読をアカウント単位にする）は採らない。ホームの新着はもともと全購読者に配るうえ、クラウドではプロセスごとに作る購読の資源をアカウント単位に増やせない。

ブローカー（ターゲットごと）:

| 候補 | fan-out の作り方 | 毎月の固定費（2026-10-19 確認） | 判定 |
| --- | --- | --- | --- |
| PostgreSQL `LISTEN` / `NOTIFY` | 各プロセスが専用の接続で `LISTEN` | 0（今の RDB） | Self-hosted の PostgreSQL 構成の既定 |
| Valkey Pub/Sub | 各プロセスが 1 チャネルを `SUBSCRIBE` | Self-hosted は 0（コンテナ）。ElastiCache Serverless は約 $6.13〜、Memorystore は約 $23〜 | 任意（Valkey を既に動かしている構成） |
| Amazon SNS → プロセスごとの SQS キュー | 標準トピックに、プロセスごとのキューを購読させる | 0（無料枠内） | AWS の既定 |
| Amazon SQS だけ | — | — | 不適。1 件を 1 つの消費者が受け取る point-to-point で、同じキューを複数のプロセスが読むと分け合う |
| Amazon EventBridge | ルールごとのターゲット（最大 5、変更不可） | カスタムイベント $1.00 / 100 万（無料枠なし） | 不適。プロセスを動的に足せない |
| AWS AppSync Events / IoT Core / API Gateway WebSocket | 配信 1 件ごとに課金 | 無料枠は 12 か月のみ | 不適。ブラウザ向けで、プロセスの同報には割高 |
| Google Cloud Pub/Sub（プロセスごとの subscription） | プロセスごとに subscription を作る | 0（無料枠内） | GCP の既定 |
| Pub/Sub の push / Eventarc | 1 件が 1 つの HTTP リクエストになり、Cloud Run の 1 インスタンスに届く | — | 不適（全インスタンスに届かない） |
| Firestore のリアルタイムリスナー | 知らせを文書として書き、全インスタンスが listen | 書き込み 1 + 読み取り N | 不適。Pub/Sub より高く、文書の形に合わない |
| NATS core | 各プロセスが subject を購読 | Self-hosted は 0 | 採らない。Valkey と役割が重なる（コミュニティアダプタは受け入れる） |

## Decision（決定）

選択肢 1（`stream.Bus` + `stream.Relay`）を採り、`Bus` のアダプタとして `postgres`、`valkey`、`sns`（SNS + SQS）、`pubsub`（Google Cloud Pub/Sub）を用意する。
SQLite（単一プロセス）と `role = all` の 1 プロセス構成は、今の `Hub` のまま。

### 1. ポートとセマンティクス

`internal/app/stream` に次を置く（コアの型で、SDK は import しない）。

```go
// Bus broadcasts payloads to every process that listens on it.
type Bus interface {
    // Send hands payload to the broker for every listening process, including this one.
    Send(ctx context.Context, payload []byte) error
    // Listen starts receiving and returns once the listener is ready: a payload sent after
    // Listen returns reaches receive, unless the listener breaks.
    Listen(ctx context.Context, receive func(payload []byte)) (Listener, error)
}

// Listener is one process's open listener.
type Listener interface {
    // Broken is closed when the listener may have missed payloads (connection lost,
    // subscription deleted) and stops receiving.
    Broken() <-chan struct{}
    // Close stops receiving and deletes the per-process resources (queue, subscription).
    Close(ctx context.Context) error
}
```

`stream.Relay` は `Publisher` と `Subscriber` を実装し、`Hub` と `Bus` を束ねる。

- **fan-out**: 知らせは、そのとき listen しているすべてのプロセスに届く。プロセスの中の振り分け（通知はそのアカウントの購読へ、ホームの新着は全購読へ）、上限、遅い購読者の切断は `Hub` が行う
- **発行**: `Publish` は自分のプロセスの `Hub` に直ちに配り、同時に符号化した知らせを有限の送信バッファ（既定 1,024 件）に積む。送信の goroutine が `Bus.Send` を呼ぶ。
  バッファがあふれたときと送信に失敗したときは捨てて、件数をログとメトリクスに出す。`Publish` はブロックせず、失敗を呼び出し元に返さない（`uow.CommitHook` から呼べる今の約束のまま）
- **受信**: 自分が出した知らせ（発行元のプロセス ID が自分）は捨てる（自分の `Hub` には発行時に配っている）。それ以外を `Hub.Publish` に渡す
- **遅延 listen**: ローカルの購読が 0 から 1 になったときに `Listen` し、0 に戻ってから一定時間（既定 60 秒）たったら `Close` する。
  worker だけのプロセスとストリームを持たないプロセスは、送るだけでブローカーを購読しない（Driver 2）
- **購読の開始**: `Subscribe(ctx, accountID)` は listener の準備ができるまで待つ（上限 10 秒）。間に合わなければ `ErrUnavailable` を返し、ストリーミングのエンドポイントは 503 と `Retry-After` を返す。
  クライアントは今の再接続の手順で読み直す
- **損失**: listener が `Broken` になったら、そのプロセスのローカルの購読をすべて終える（`Done` を閉じる）。クライアントは再接続してホームと通知を読み直す。遅い購読者を切るときと同じ扱いで、「取りこぼしたかもしれないストリームは開いたままにしない」
- **重複と順序**: アダプタによっては重複する（SQS 標準・Pub/Sub は at-least-once）。プロセスをまたぐ順序は保証しない（同じプロセスの中の順序は `Hub` が保つ）。
  ストリーミングのハンドラは、ストリームごとに最近送ったホームの新着の ID（数百件）を覚えて重複を捨てる。通知は今の「前回より新しい一覧」で重複にも順序にも強い
- **形式と大きさ**: 知らせは JSON `{"v":1,"o":"<process id>","k":"home_entry","e":"<entry id>"}`（通知は `"k":"notifications","a":"<account id>"`）。
  上限 1 KiB を符号化で強制する（PostgreSQL の `NOTIFY` の 8,000 バイト未満、SNS の 256 KiB、Pub/Sub の 10 MB のいずれにも収まる）。`v` が知らない版の知らせは捨てる（ローリングデプロイ中の混在に備える）
- **公開範囲**: 知らせは ID だけで内容を載せない。受けたプロセスは今のとおり `timeline.Service.HomeEntry` と通知の一覧で読み直し、公開範囲は `CanView` で決まる。
  ブローカーが見るのは投稿・アカウントの ID だけで、接続は TLS（Valkey / クラウド）または DB の接続の上で行う
- **上限**: アカウントごとのストリームの上限（8）はプロセスごとのまま。全体で数えるのは別の課題

### 2. アダプタ

#### `stream/postgres`（PostgreSQL `LISTEN` / `NOTIFY`）

- 送信: プールの接続で `SELECT pg_notify('<channel>', $1)`（自動コミット）。受信: プールとは別の専用の接続で `LISTEN <channel>` し、pgx v5 の `WaitForNotification` で受ける。接続が切れたら `Broken`
- `LISTEN` はセッションに結びつくので、トランザクションモードのプーラー（PgBouncer の transaction pooling、Neon のプール接続、Supabase の 6543 番）では使えない。
  そのときは直接の接続先を `stream_postgres_url` で別に指定する
- Neon は、アイドルの `LISTEN` の接続ではコンピュートを起こし続けないが、scale to zero でリスナーを切る。遅延 listen のため、切られるのはストリームがないときだけで、切られたら `Broken` として扱う
- `NOTIFY` するトランザクションはコミット時にクラスタ全体で直列化される（PostgreSQL のソースの `async.c`。利用者向けの文書には書かれていない）。知らせは 1 件ずつの短い文で、v0 の規模では問題にならないと見込む（要検証: 負荷試験）
- Aurora DSQL での可否は公式の文書で確認できない（サポートする SQL の一覧に `LISTEN` / `NOTIFY` がない）。DSQL では使わない前提にする
- 依存: 既存の pgx v5（MIT）。追加のコストはない

#### `stream/valkey`（Valkey Pub/Sub）

- 1 つのチャネル `<valkey_key_prefix>stream` に `PUBLISH` し、専用の接続で `SUBSCRIBE` する。クラスタモードでも通常の `PUBLISH` はクラスタ全体に届く。ElastiCache Serverless は内部で sharded Pub/Sub を使うが、1 チャネルなので影響しない
- Valkey の Pub/Sub は at-most-once で、切断中の知らせは失われる。valkey-go は自動で再接続・再購読するが、その間の損失は分からないので、切断（`SetPubSubHooks` の待ちチャネルが閉じる）を `Broken` として扱う
- 知らせは小さく、`client-output-buffer-limit pubsub 32mb 8mb 60`（既定）に当たるのは、受信の goroutine が止まったときだけ。そのときは Valkey が切断し、`Broken` になる
- 接続は KV と同じ `internal/adapter/valkey` で作り、設定 `valkey_url`（`valkey://` / `valkeys://`、認証と DB 番号を含む）・`valkey_tls_ca_file`・`valkey_key_prefix` を共有する
- クライアント: valkey-go（Apache-2.0、pure Go）。版は公開から 3 日を過ぎた v1.0.78。KV のアダプタと同じ版を使う
- クラウドでは既定にしない
  - ElastiCache Serverless for Valkey は最小 100 MB の課金で約 $6.13 / 月（$0.084 / GB 時間、us-east-1）。VPC のエンドポイントでは Lambda を VPC に入れる必要があり、外向きに NAT が要るうえ、**VPC の中の Lambda の Function URL はレスポンスストリーミングに対応しない**。
    公開エンドポイント（Valkey 9.0 以上、IAM 認証のみ、追加料金なし）なら VPC は要らないが、提供リージョンと valkey-go の IAM 認証の対応は要検証
  - Memorystore for Valkey は最小の shared-core-nano でも $0.0318 / 時間（約 $23 / 月）で SLA がなく、Private Service Connect と Cloud Run の Direct VPC egress が要る。GCP に scale to zero の Valkey は見つからなかった

#### `stream/sns`（Amazon SNS 標準トピック → プロセスごとの SQS キュー）

- SQS は point-to-point なので、全プロセスへの同報には SNS のトピックからプロセスごとのキューへ配る形にする
- トピック（標準）はデプロイのテンプレートが作る。送信は `sns:Publish`（Raw の本文）
- `Listen` で作るもの
  - キュー `<stream_sqs_queue_prefix>-<process id>-<n>`（80 文字以内。`n` はそのプロセスの `Listen` の連番）。`MessageRetentionPeriod` 60 秒（最小）、`ReceiveMessageWaitTimeSeconds` 20 秒、タグ付き
  - キューのポリシー: `sns.amazonaws.com` に `sqs:SendMessage` を許し、`aws:SourceArn` をトピックの ARN に限る
  - トピックの購読（`Protocol` は `sqs`、`RawMessageDelivery=true`）。同じアカウントなので自動で確認される
- 受信は 20 秒のロングポーリング（最大 10 件）と `DeleteMessageBatch`。`Close` で `Unsubscribe` と `DeleteQueue` を呼ぶ。キューの削除は最大 60 秒かかり、同じ名前は 60 秒使えないので、名前はプロセス ID と `Listen` の連番で毎回新しくする（`Relay` は `Broken` の後に同じプロセスで listen し直す）
- 後始末
  - Lambda Web Adapter は外部拡張として登録されるので、実行環境の終了時にプロセスは SIGTERM を受け、約 2 秒の猶予で `Close` を試みる
  - 異常終了や凍結のまま捨てられた実行環境の分は、定期ジョブ `stream.sweep`（ADR-0004 の Scheduler → JobQueue）が消す。
    listen 中のプロセスは KV（`kernel.KeyValueStore`）に TTL 5 分の生存の印を 1 分ごとに書き、ジョブは接頭辞でキューとトピックの購読を一覧して、印のない古いもの（作成から 5 分以上）を消す
  - 凍結から戻ったプロセスのキューが消されていたら、受信が `QueueDoesNotExist` で失敗して `Broken` になり、次の購読で作り直す
- Lambda での形: 実行環境は 1 度に 1 リクエストを扱うので、SSE の接続 1 本が実行環境 1 つ、つまりキュー 1 つになる。関数のタイムアウト（最大 15 分）でストリームは切れ、クライアントが再接続する
- コスト（2026-10-19 確認、us-east-1）
  - SNS: API リクエストは月 100 万まで無料、以降 $0.50 / 100 万（64 KB ごとに 1 リクエスト）。SQS への配信は課金されない
  - SQS: 全顧客に月 100 万リクエスト無料、以降 $0.40 / 100 万。空の受信も 1 リクエスト。listen 中のプロセス 1 つで約 129,600 回 / 月（20 秒ごと）なので、常時開いたストリーム約 7 本分までが無料枠
  - 知らせ 1 件は SNS 1 リクエスト。月 10 万件でも無料枠に収まる
- IAM（実行ロール）: `sns:Publish`、`sns:Subscribe`（`sns:Protocol` = `sqs` と `sns:Endpoint` でキューの接頭辞に限る）、`sns:Unsubscribe`、`sns:ListSubscriptionsByTopic`、
  キューの接頭辞の ARN に対する `sqs:CreateQueue`・`sqs:TagQueue`・`sqs:SetQueueAttributes`・`sqs:GetQueueAttributes`・`sqs:GetQueueUrl`・`sqs:ReceiveMessage`・`sqs:DeleteMessage`・`sqs:DeleteQueue`、`sqs:ListQueues`。
  `sns:Unsubscribe` の対象の資源の種類は要検証
- 依存: aws-sdk-go-v2 の `service/sns`・`service/sqs`（Apache-2.0）。`queue/sqs` と同じ SDK

#### `stream/pubsub`（Google Cloud Pub/Sub、プロセスごとの subscription）

- 1 つの subscription に複数の購読者がいると知らせを分け合うので、プロセス（Cloud Run のインスタンス）ごとに subscription を作る
- トピックはデプロイのテンプレートが作る。`Listen` で subscription `<stream_pubsub_subscription_prefix>-<process id>-<n>` を作る（`n` はそのプロセスの `Listen` の連番。Pub/Sub の削除は世代を指定できないので、壊れた listener の遅れた `Close` が次の listener の subscription を消さないよう、名前を毎回新しくする）
  - `expirationPolicy.ttl` 1 日（最小）、`messageRetentionDuration` 10 分（最小）、ack の期限 10 秒
  - streaming pull で受け、受けたらすぐ ack する。`Close`（SIGTERM の 10 秒の猶予を含む）で subscription を消す
- 後始末: 消し損ねた subscription は 1 日使われなければ Pub/Sub が消す。トピックあたり 10,000 の上限に近づかないよう、`stream.sweep` も同じ生存の印で消す
- Cloud Run のリクエストベース課金では、CPU はリクエストの処理中だけ割り当てられ、リクエストの外の goroutine は止まる。SSE のストリームはリクエストなので、
  遅延 listen と組み合わせれば「ストリームがある間だけ受ける」形になる。1 つのインスタンスは複数のリクエストを並行に扱う（concurrency）ので、subscription はインスタンスにつき 1 つ
- コスト（2026-10-19 確認）: スループット $40 / TiB、毎月 10 GiB まで無料。リクエストごとに最低 1 KB で数える。
  subscription 3 つなら知らせ約 268 万件 / 月までが無料枠で、以降 100 万件ごとに約 $0.15。保持を 1 日以下にするので保存料はかからない。subscription の作成・削除に料金はない
- IAM: トピックに `pubsub.topics.publish` と `pubsub.topics.attachSubscription`、プロジェクトに `pubsub.subscriptions.create`・`pubsub.subscriptions.delete`・`pubsub.subscriptions.consume`・`pubsub.subscriptions.list`（カスタムロール。`roles/pubsub.editor` は広すぎる）。`list` は `stream.sweep` が、プロセス ID と作成時刻を持つラベルで古い subscription を探すのに使う
- 依存: `cloud.google.com/go/pubsub/v2`（Apache-2.0。v1 は非推奨で、2026-12-31 までの修正のみ）

### 3. ターゲットごとの既定

`stream` の設定（下記）は推測しない（`db` と同じ）。既定は `hub` で、`role` が `all` 以外のときに `hub` なら起動時に警告する。

| ターゲット | `stream` |
| --- | --- |
| Self-hosted（SQLite、または 1 プロセスの `role = all`） | `hub` |
| Self-hosted（PostgreSQL で複数プロセス・レプリカ） | `postgres`。Valkey を動かしている構成（`kv = valkey`）は `valkey` も可 |
| Google Cloud（Cloud Run） | `pubsub` |
| AWS（Lambda） | `sns` |

### 4. 設定

| 設定 | 内容 |
| --- | --- |
| `stream` | `hub`（既定）/ `postgres` / `valkey` / `sns` / `pubsub` |
| `stream_postgres_url` | `LISTEN` に使う直接の接続先（既定は `database_url`）。`stream = postgres` は `db = postgres` のときだけ |
| `valkey_url`（`_FILE` 可）、`valkey_tls_ca_file`、`valkey_key_prefix` | `kv = valkey` と共有（必須の条件に `stream = valkey` を足す） |
| `stream_sns_topic_arn`、`stream_sqs_queue_prefix` | AWS。認証とリージョンは SDK の既定の連鎖 |
| `stream_pubsub_topic`、`stream_pubsub_subscription_prefix` | GCP（`projects/<p>/topics/<t>`） |

### 5. テスト

- `pkg/testkit` に 2 つのコントラクトを置く
  - `StreamBusContract`: 同じブローカーの 2 つの `Bus`（2 つのプロセスに見立てる）で、全 listener への同報、`Listen` が返った後の送信が届くこと、自分にも届くこと、`Close` で止まって資源が消えること、ブローカーに届かないときも `Send` が時間内に返ること、切断で `Broken` になること（テストで切断を起こせるアダプタ）を確かめる
  - `StreamBrokerContract`: `Publisher` / `Subscriber`（`Hub` と、メモリの `Bus` の上の `Relay`）の振り分け・上限・遅い購読者の切断・`Broken` での購読の終了
- アダプタごとのテスト

| アダプタ | 毎 PR（アカウント・Docker なし） | main への push と nightly |
| --- | --- | --- |
| `postgres` | pgmem（ADR-0041。pgmem は複数接続の `LISTEN` / `NOTIFY` をうたう。実装で確かめる） | `make go-test-postgres` の `postgres:18-alpine` |
| `valkey` | miniredis（MIT、pure Go、`PUBLISH` / `SUBSCRIBE` と RESP3）。公開から 3 日を過ぎた v2.39.0 | testcontainers-go の `modules/valkey`（MIT）と `valkey/valkey:9.1.2-alpine` |
| `pubsub` | `cloud.google.com/go/pubsub/v2/pstest`（Go のプロセス内の偽サーバー、Apache-2.0） | Pub/Sub エミュレータ（`google-cloud-cli:<版>-emulators` を testcontainers-go の `modules/gcloud/pubsub` で起動。アカウント不要。エミュレータは期限切れと保持期間に対応しない） |
| `sns` | アダプタが使う SNS / SQS の操作を狭いインターフェイスにし、testkit のメモリの偽物で確かめる | Floci 2.1.0（MIT、`floci/floci:2.1.0`。キューの `Policy`、`CreatedTimestamp`、SNS → SQS と `RawMessageDelivery`、削除済みキューの `QueueDoesNotExist` に対応）のコンテナ |

- LocalStack は 2026-03-23 のリリースから起動に認証トークン（アカウント）が要り、商用の CI は有料なので使わない。goaws v0.5.4 はキューの `Policy` を受け付けず `CreatedTimestamp` を返さないので使わない
- 2 プロセスの結合テスト（`internal/platform`）: 同じ DB と同じ `Bus` を使う 2 つの App を立て、片方で投稿したものと、`role = worker` 側で Inbox を処理して作られた通知が、もう片方のストリームに届くことを確かめる（`postgres` を pgmem で、と `Relay` のメモリの `Bus` で）
- クラウドの資源の後始末（SIGTERM、`stream.sweep`、Pub/Sub の期限切れ）は、M6 のデプロイのスモークテストで確かめる

## Consequences（影響）

- 良い影響
  - 複数プロセス・web と worker の分離のどちらでも、ストリームにすべての更新が届く
  - アダプタは「同報して受ける」だけを実装するので小さい。振り分け・上限・公開範囲の扱いは 1 か所のまま
  - ストリームのないプロセスはブローカーに何も持たず、scale to zero と無料枠を壊さない
  - Self-hosted の PostgreSQL 構成は、ミドルウェアを足さずに複数プロセスにできる
- 悪い影響・トレードオフ
  - プロセスをまたぐ配信は best effort のまま。ブローカーの切断のたびに、そのプロセスのストリームが一斉に切れて再接続と読み直しが起きる
  - 購読の開始が listener の準備（SQS のキューの作成と購読、Pub/Sub の subscription の作成）を待つので、プロセスで最初のストリームの開始が数秒遅れうる（要検証: 実測）
  - AWS では SSE の接続 1 本が Lambda の実行環境 1 つを最大 15 分占有する。ブローカーとは別に、Lambda の実行時間がストリームの本数 × 時間で増える（例: 512 MB で 1 本を 1 か月開き続けると約 131 万 GB 秒で、無料枠 40 万 GB 秒を超える）
  - クラウドではプロセスごとの資源を作り消すので、後始末の定期ジョブと IAM の権限が要る
  - アカウントごとのストリームの上限はプロセスごとで、全体では「上限 × プロセス数」になる
- 後続で必要になる作業（#196 の実装として PR を分ける）
  1. ポート: `stream.Bus` / `Listener`、知らせの符号化、`stream.Relay`、`Subscribe(ctx, ...)` と `ErrUnavailable`（503）、ストリームでのホームの新着の重複排除、メモリの `Bus`、`StreamBusContract` と `StreamBrokerContract`
  2. `stream/postgres` とそのテスト
  3. `stream/valkey` とそのテスト（`internal/adapter/valkey` の接続と設定を共有）
  4. `stream/pubsub` とそのテスト
  5. `stream/sns` とそのテスト、`stream.sweep` ジョブ
  6. 設定（スキーマ、`make config-docs`）、`internal/platform` の組み立て、`hub` で `role` が `all` 以外のときの警告
  7. 2 プロセスの結合テスト
  8. 文書: overview の §3 のポートの表と §4.4、client-api.md §5（503、上限）、deployment-targets.md（ブローカーの行・費用・IAM）

## Open Questions / 要検証

- AWS（Lambda）でストリームを既定で有効にするか。上記の Lambda の実行時間を避けるなら、Lambda ではストリームを無効にしてクライアントのポーリングに任せる設定が要る
- Self-hosted の `db = postgres` で、`stream` の既定を `postgres` にするか（今の推奨案は `db` と同じく推測しない）
- ElastiCache Serverless の公開エンドポイントの提供リージョンと、valkey-go の IAM 認証の対応（対応すれば AWS で `valkey` も選べる）
- `NOTIFY` のコミット時の直列化が、知らせの多い構成で投稿の書き込みを遅らせないか（負荷試験）

出典（2026-10-19 確認）:
[Valkey Pub/Sub](https://valkey.io/topics/pubsub/) /
[valkey.conf（client-output-buffer-limit）](https://raw.githubusercontent.com/valkey-io/valkey/unstable/valkey.conf) /
[valkey-go](https://github.com/valkey-io/valkey-go) /
[testcontainers-go Valkey module](https://golang.testcontainers.org/modules/valkey/) /
[miniredis](https://github.com/alicebob/miniredis) /
[PostgreSQL 18 NOTIFY](https://www.postgresql.org/docs/18/sql-notify.html) /
[PostgreSQL 18 LISTEN](https://www.postgresql.org/docs/18/sql-listen.html) /
[PostgreSQL async.c（REL_18_STABLE）](https://github.com/postgres/postgres/blob/REL_18_STABLE/src/backend/commands/async.c) /
[PgBouncer features](https://www.pgbouncer.org/features.html) /
[Neon: LISTEN / NOTIFY](https://neon.com/guides/pub-sub-listen-notify) /
[Neon connection pooling](https://neon.com/docs/connect/connection-pooling) /
[Neon compute lifecycle](https://neon.com/docs/introduction/compute-lifecycle) /
[Supabase: connecting to Postgres](https://supabase.com/docs/guides/database/connecting-to-postgres) /
[Aurora DSQL supported SQL](https://docs.aws.amazon.com/aurora-dsql/latest/userguide/working-with-postgresql-compatibility-supported-sql-features.html) /
[pgmem architecture](https://github.com/shibukawa/pgmem/blob/main/.knowledge/concept/architecture.md) /
[SNS pricing](https://aws.amazon.com/sns/pricing/) /
[SNS quotas](https://docs.aws.amazon.com/general/latest/gr/sns.html) /
[SNS raw message delivery](https://docs.aws.amazon.com/sns/latest/dg/sns-large-payload-raw-message-delivery.html) /
[SNS → SQS subscription](https://docs.aws.amazon.com/sns/latest/dg/subscribe-sqs-queue-to-sns-topic.html) /
[SNS API permissions](https://docs.aws.amazon.com/sns/latest/dg/sns-access-policy-language-api-permissions-reference.html) /
[SQS pricing](https://aws.amazon.com/sqs/pricing/) /
[SQS FAQ](https://aws.amazon.com/sqs/faqs/) /
[SQS queue quotas](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/quotas-queues.html) /
[SQS CreateQueue](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_CreateQueue.html) /
[SQS DeleteQueue](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_DeleteQueue.html) /
[SQS visibility timeout](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html) /
[SQS API permissions](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-api-permissions-reference.html) /
[EventBridge quotas](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-quota.html) /
[EventBridge pricing](https://aws.amazon.com/eventbridge/pricing/) /
[AppSync pricing](https://aws.amazon.com/appsync/pricing/) /
[IoT Core pricing](https://aws.amazon.com/iot-core/pricing/) /
[API Gateway pricing](https://aws.amazon.com/api-gateway/pricing/) /
[ElastiCache pricing](https://aws.amazon.com/elasticache/pricing/) /
[ElastiCache supported commands](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/SupportedCommands.html) /
[ElastiCache serverless public endpoints](https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/serverless-public-endpoints-chapter.html) /
[Lambda VPC internet access](https://docs.aws.amazon.com/lambda/latest/dg/configuration-vpc-internet.html) /
[Lambda response streaming](https://docs.aws.amazon.com/lambda/latest/dg/configuration-response-streaming.html) /
[Lambda execution environment lifecycle](https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtime-environment.html) /
[Lambda quotas](https://docs.aws.amazon.com/lambda/latest/dg/gettingstarted-limits.html) /
[Lambda Web Adapter](https://github.com/awslabs/aws-lambda-web-adapter) /
[LocalStack 2026.03.0](https://blog.localstack.cloud/localstack-for-aws-release-2026-03-0/) /
[LocalStack pricing](https://www.localstack.cloud/pricing) /
[goaws](https://github.com/Admiral-Piett/goaws) /
[MiniStack](https://github.com/ministackorg/ministack) /
[Floci](https://github.com/floci-io/floci) /
[aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2) /
[Pub/Sub basics](https://docs.cloud.google.com/pubsub/docs/pubsub-basics) /
[Pub/Sub subscription overview](https://docs.cloud.google.com/pubsub/docs/subscription-overview) /
[Pub/Sub subscription properties](https://docs.cloud.google.com/pubsub/docs/subscription-properties) /
[Pub/Sub quotas](https://docs.cloud.google.com/pubsub/quotas) /
[Pub/Sub pricing](https://cloud.google.com/pubsub/pricing) /
[Pub/Sub access control](https://docs.cloud.google.com/pubsub/docs/access-control) /
[Pub/Sub emulator](https://docs.cloud.google.com/pubsub/docs/emulator) /
[testcontainers-go gcloud/pubsub](https://pkg.go.dev/github.com/testcontainers/testcontainers-go/modules/gcloud/pubsub) /
[cloud.google.com/go/pubsub（v1 の非推奨）](https://pkg.go.dev/cloud.google.com/go/pubsub) /
[cloud.google.com/go/pubsub/v2](https://pkg.go.dev/cloud.google.com/go/pubsub/v2) /
[Cloud Run billing settings](https://docs.cloud.google.com/run/docs/configuring/billing-settings) /
[Cloud Run background activity](https://docs.cloud.google.com/run/docs/tips/general) /
[Cloud Run container contract](https://docs.cloud.google.com/run/docs/container-contract) /
[Memorystore for Valkey pricing](https://cloud.google.com/memorystore/valkey/pricing) /
[Memorystore for Valkey node specification](https://docs.cloud.google.com/memorystore/docs/valkey/instance-node-specification) /
[Cloud Run Direct VPC egress](https://docs.cloud.google.com/run/docs/configuring/vpc-direct-vpc) /
[Firestore realtime listeners](https://docs.cloud.google.com/firestore/native/docs/query-data/listen) /
[Eventarc overview](https://docs.cloud.google.com/eventarc/docs/overview)
