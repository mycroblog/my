# ADR-0004: ジョブキューとワーカー（ポート・ダッシュボード・外部キュー・種類の選択・同時実行）

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #4, #815, #1837, #1996, #2003, #2004

## Decision（決定）

ジョブは DB の `jobs` を正とする。ハンドラはどのキューでも同じコードで、外部のキュー（Cloud Tasks・SQS）は DB のジョブを起こす合図だけを運ぶ。
処理能力は、`queue = db` では種類の指定とプロセス内の同時実行数で、外部のキューではキューの分割とプラットフォームの設定で分ける。

### 1. ジョブモデル

```text
Job {
  id: string            // UUIDv7
  type: string          // "federation.deliver", "federation.inbox", "media.process", ...
  payload: JSON         // 小さく保つ（大きいデータは DB / Blob に置いて参照）
  idempotencyKey?: string
  attempt: number
  runAt: timestamp      // 遅延実行
  orderingKey?: string  // 同一キーの直列化が必要な場合（例: 同一 Actor の Inbox）
}
```

### 2. ポート

- `JobQueue.enqueue(job | job[])` — 投入（遅延・冪等キー付き）
- `JobHandlerRegistry` — `type` → ハンドラ。ハンドラは `(ctx, job) => Result<Done | Retry(after) | Fail>` の純粋なアプリケーション関数
- `JobRunner` — アダプタ側。2 種類の実装形態を持つ:
  - **pull runner**: ループでデキュー → ハンドラ実行 → ack / 再投入（DB キュー）
  - **push receiver**: プラットフォームから渡された合図を処理する（Lambda の SQS イベント、Cloud Tasks の HTTP ターゲット `/_internal/jobs`。§10、§13）
- `Scheduler` — 定期ジョブの起動トリガー（Cloud Scheduler / EventBridge Scheduler / プロセス内 cron。§12）。
  トリガーは「該当ジョブを enqueue する」だけにし、実処理は JobQueue に流す。

### 3. セマンティクス

- **at-least-once**。ハンドラは冪等（Activity ID・冪等キーで重複排除）。
- リトライ: 指数バックオフ + ジッタ。配送は最大数日（例: 1m, 5m, 30m, 2h, 12h, 24h ×3）。
- **遅延上限を超えるバックオフは「再投入チェーン」**: 上限まで遅延 → 起動時に `runAt` 未到来ならそのまま再遅延。
- 最大試行回数超過は `dead_jobs` テーブル（DB）へ記録し、管理画面 / CLI から再実行できる（DLQ を持つサービスでも DB に寄せる）。
- 連合の配送・受信処理も独自キューを持たず、本ポートに一本化する。
- 実行は「早くとも `runAt` 以降」で、厳密な実行時刻は保証しない。

### 4. v0 で提供するアダプタ

`queue/db`（既定）、`queue/inline`（テスト・開発用の同期実行）、`queue/sqs`、`queue/cloudtasks`。
Redis / NATS / RabbitMQ 等はコミュニティアダプタとして受け入れ可能にする。

### 5. ダッシュボード: 試行の記録 `job_attempts`（イベント）

**成功しなかった試行（`retry` と `dead`）だけを記録する。** 成功した試行（`success`）は記録せず、構造化ログの `job` の行（`job_id` を持つ）が担う。ジョブが最後に成功すれば、ジョブは一覧から消え、その前の失敗の記録は保持期間まで残る。

| 列 | 内容 |
| --- | --- |
| `job_id` | ジョブの ID（`jobs.id` / `dead_jobs.id`。再実行しても同じ） |
| `attempt` | 試行の番号（1 から） |
| `job_type` | ジョブの型（絞り込みと、ジョブが消えた後の表示のため） |
| `outcome` | `retry`（もう一度試す）/ `dead`（失敗が確定した） |
| `error` | 失敗の理由。先頭の 2,000 バイトまで（UTF-8 の境界で切る） |
| `worker` | 試行したワーカーの名前 |
| `started_at`・`finished_at` | 試行の開始と終了 |

- 主キーは `(job_id, attempt, finished_at)` とし、同じ試行の記録を繰り返しても 1 行になる（冪等）。再実行（`RequeueDeadJob`）は試行回数を数え直すので、同じ `attempt` が 2 回現れうる。終了時刻で区別する
- 記録するのは runner（pull runner と push receiver が共有する結果の処理）で、ポート `queue.AttemptLog` を通す。キューのアダプタには依存しない（どのキューでも同じ DB の表に書く）
- 記録の失敗はジョブの結果を変えない（ログに出して続ける）。DB キューでは、`Release`・`Bury` と同じトランザクションで書く実装にしてよい

### 6. ダッシュボード: 保持期間・量の上限・個人情報

- **保持期間**: `finished_at` から設定 `job_attempt_retention`（既定 30 日）。失敗したジョブの `dead_job_retention` とは別の設定で、既定値を揃える。毎時の消去ジョブ `queue.purge_dead_jobs` が、古い順に 1 文 1,000 行・20 文まで消す（`delivery_attempts` と同じ）
- **量の上限**: 1 行は `error` の 2,000 バイトで上限が決まる。1 つのジョブの行数は再試行の方針の `MaxAttempts` で上限が決まる（再実行のたびに増えるが、各行は保持期間で消える）。行の総数は「保持期間 × 失敗の数」で、正常なジョブは増やさない
- **個人情報**: ペイロード（`payload`）とハンドラのログは記録しない。`error` はハンドラが返したエラーの文で、ハンドラはエラーにメールアドレス・IP アドレス・トークン・本文を入れない（`last_error` と同じ規則）。
  ペイロードは失敗したジョブの画面と同じく、管理者（`manage_jobs`）にだけ見せる

### 7. ダッシュボード: 未完了のジョブの一覧

- `internal/app/queue` の読み取りの口 `JobInspector`: 状態と型で絞り込んだジョブのページ（カーソル）と、状態 × 型の件数を返す。実装は `queue/db` と `queue/inline` で、取れるアダプタだけが実装する（`DeadJobs`・`BacklogReader` と同じく有無を判定する）。
  `queue = cloudtasks` / `sqs` のジョブは DB の `jobs` にある（§9）ので、一覧はすべてのキューで出る
- 状態は `jobs` の今の値から導く（列は足さない）:

  | 状態 | 条件（DB キュー） |
  | --- | --- |
  | 実行中（`running`） | リースが有効（`locked_until > now`） |
  | 待ち（`queued`） | 実行中でなく、`attempt = 0`（`run_at` が先なら予定の時刻を見せる） |
  | リトライ待ち（`retrying`） | 実行中でなく、`attempt > 0` |
  | 失敗（`dead`） | `dead_jobs` の行 |

- 管理 API の形はアダプタで変えない: 応答に、そのキューで読めるもの（`pending_listing` の可否）を持たせる

### 8. ダッシュボード: 管理 API と管理画面

- 権限は `manage_jobs`（読み取りは `admin:read`、再実行・削除は `admin:write`）
- `GET /admin/queue`: 状態 × 型の件数、最も古い実行可能なジョブの待ち、アダプタが読めるもの
- `GET /admin/jobs?state=&type=`: ジョブのページ（`state=dead` は `dead_jobs` の一覧を兼ねる）
- `GET /admin/jobs/{id}/attempts`: ジョブの試行の記録（新しい順）
- 失敗したジョブの再実行・削除もこの名前空間に置く
- 管理画面は `/admin/jobs` に 1 つのダッシュボードを置き、失敗したジョブはその「失敗」の絞り込みにまとめる

### 9. 外部のキュー: 合図（wake-up signal）

`queue = cloudtasks` / `sqs` は「DB のキュー + 合図の運び手」で、外部のキューはジョブを持たない。

- ジョブは `queue = db` と同じく `jobs` に入る。`EnqueueTx` は呼び手のトランザクションで `jobs` に書く（`outbox_events` とリレーは使わない）。
  `Enqueue` も `jobs` に書いてから合図を送る
- **合図はジョブの ID と、見るべき時刻だけを持つ**。ペイロード・型・キーは載せない。`traceparent` はジョブの行にあるので運ばない（[ADR-0055](0055-service-observability.md) §14 の伝播は DB の列で足りる）
- 合図はコミットの後に送る（uow のコミットのフック。`queue.AfterCommit` と同じ仕組み）。送れなかった合図・届かなかった合図は §11 の掃除が拾うので、送信の失敗はログに出すだけにする
- ポート `queue.Waker`（`Wake(ctx, []Wake{JobID, At})`。`Type` はプロセス内の振り分けにだけ使う。§24）を `internal/app/queue` に置き、`queue/cloudtasks` と `queue/sqs` が実装する。
  `At` が遅延の上限（SQS 15 分、Cloud Tasks 30 日）を超えるときは上限で送る（`queue.NextVisible`）
- Cloud Tasks のタスクには名前を付けない（名前の重複排除は削除後 24 時間も効き、同じジョブの次の合図を拒むため）。SQS は標準キューを使う
- 合図の重複は害がない（§10 の claim が 1 回にする）。合図の数はジョブの投入・リトライの数と同じで、外部のキューの課金もそれに比例する

### 10. 外部のキュー: push receiver

- 受け口は合図のジョブを **ID で claim** する。`queue.Source` の `ClaimJob(ctx, id, worker, now, lease)` を `queue/db` が実装する。
  `queue/db` の claim は、候補を選ぶ文（`PickJob`）・順序キーの advisory lock・ID を指定してリースを取る文（`ClaimJob`）に分かれているので、後の 2 つをそのまま使う。
  結果に応じて次のようにする

  | claim の結果 | すること |
  | --- | --- |
  | 取れた | ハンドラを動かし、pull runner と同じ結果の処理（`Complete` / `Release` / `Bury`・試行の記録・エラーの報告）をする。`Release` したらその `RunAt` で合図を送る |
  | まだ `RunAt` 前（遅延の上限で早く届いた） | `RunAt` で合図を送り直す（再投入のチェーン） |
  | 順序キーの前のジョブが残っている | 何もしない。前のジョブが `Complete` / `Bury` したとき、runner が同じキーの次のジョブに合図を送る |
  | 他の worker がリース中・行がない（完了済み・dead） | 何もしない |

- 結果を DB に記録できたら成功（Cloud Tasks には 2xx、SQS にはそのメッセージを `batchItemFailures` に入れない）を返す。**リトライはアプリが `RunAt` で行い、外部のキューの再試行には頼らない**。
  外部のキューの再試行は、受け口が落ちた・DB に届かなかったときにだけ働く（5xx・`batchItemFailures`）
- Cloud Tasks は 200〜299 だけを成功とし、それ以外の状態と無応答を `RetryConfig` で再試行する。429・503 では待ちを長くし、`Retry-After` を考慮する
  （2026-10-29 に [Task](https://docs.cloud.google.com/tasks/docs/reference/rest/v2/projects.locations.queues.tasks) で確認）
- ハンドラの context はリースで終わる。リースは全ハンドラで一律 5 分（`queue.DefaultLease`）で、プラットフォームの上限より短い。
  Cloud Tasks の HTTP ターゲットの `dispatchDeadline` は既定 10 分・15 秒〜30 分、Cloud Run のリクエストのタイムアウトは既定 5 分・最大 60 分（2026-10-29 に
  [Task](https://docs.cloud.google.com/tasks/docs/reference/rest/v2/projects.locations.queues.tasks)・[Request timeout](https://docs.cloud.google.com/run/docs/configuring/request-timeout) で確認）、
  Lambda のタイムアウトは最大 15 分。テンプレートは Cloud Run のタイムアウトと `dispatchDeadline` をリースより長く（10 分）にし、設定の検査はリースがそれらを超えないことを確かめる
- Lambda の SQS のイベントは複数のメッセージをまとめて渡す。受け口は 1 件ずつ順に処理し、残り時間がリースに足りなくなったら残りを `batchItemFailures` で返す
  （`ReportBatchItemFailures` を使う。2026-10-29 に [Handling errors for an SQS event source](https://docs.aws.amazon.com/lambda/latest/dg/services-sqs-errorhandling.html) で確認）。
  Lambda Web Adapter は pass-through の応答の **本文だけ** を JSON として Lambda の結果にし、状態とヘッダーを捨てる。本文が JSON として読めないと結果は `null`（バッチ全体の成功）になるので、
  受け口は必ず `Content-Type: application/json` の JSON を返し、Lambda Web Adapter は応答のストリーミングでない既定の呼び出しの形で使う
  （v1.1.0 の `lambda_http` 1.3.0 の `response.rs` で確認）

### 11. 外部のキュー: 掃除（合図の取りこぼしを拾う）

- 定期のジョブ `queue.wake` が、期限の来た（`run_at <= now - 猶予`）ジョブのうち、リースがない・切れていて順序キーで止められていないものに合図を送り直す。
  猶予（既定 2 分）は送ったばかりの合図との重なりを減らすため。重なっても §10 で 1 回になる
- 探し方は pull runner が毎回の claim で流す `PickJob` と同じ述語で、`LIMIT` を件数の上限にし `FOR UPDATE SKIP LOCKED` を外したもの。索引 `jobs_run_at` と、順序キーの確かめの `jobs_ordering_key_run_at` に乗る
- これでコミットとの間のプロセスの停止、合図の送信の失敗、リース中に落ちた worker のジョブを拾う。取りこぼしたジョブの遅れは掃除の周期（§12 のトリガーの周期、1 分）と猶予で決まる
- ルートがある構成では、掃除はルートごとに行う（§26）

### 12. 外部のキュー: Scheduler の外部トリガー

- 受け口に、合図とは別の **tick** がある。tick は (1) `queue.Scheduler.Tick` で期限の来た周期のジョブを積み、(2) §11 の掃除をする
- 周期の 2 回目の投入は、プロセスのメモリでなく KV（`KeyValueStore.CompareAndSwap`、TTL は周期）で防ぐ（冪等キー `schedule:<name>:<slot>` は未完了の間だけ効くため）。外部トリガーを使うときは `kv` を `db` か `valkey` にすることを設定の検査で求める
- Google Cloud: Cloud Scheduler の HTTP ターゲットで tick を 1 分ごとに 1 本（無料枠の 3 本のうち 1 本）。
- AWS: EventBridge Scheduler の `rate(1 minute)`（最小の単位）で、同じ SQS の標準キューに tick のメッセージを送る（templated target の `SendMessage`。`Input` が本文になり、最大 256 KB）。
  料金は 100 万回 $1.00 で月 1,400 万回まで無料なので、1 分ごと（月約 4.3 万回）は無料枠に収まる
  （2026-10-29 に [Templated targets](https://docs.aws.amazon.com/scheduler/latest/UserGuide/managing-targets-templated.html)・[Schedule types](https://docs.aws.amazon.com/scheduler/latest/UserGuide/schedule-types.html)・[EventBridge pricing](https://aws.amazon.com/eventbridge/pricing/) で確認）。
  Lambda を直接呼ぶ形もあるが、合図と同じ経路（SQS のイベント）に揃える
- 外部トリガーを使うプロセスはプロセス内の Scheduler を動かさない。Self-hosted（`queue = db`、常駐の worker）はプロセス内の Scheduler と pull runner を使い、Kubernetes の CronJob は要らない

### 13. 外部のキュー: 受け口と認証

- **Cloud Tasks・Cloud Scheduler**: 内部のエンドポイント `POST /_internal/jobs`（合図）と `POST /_internal/tick` で、アプリが OIDC の ID トークンを検証する。
  署名（Google の JWKS）・`iss`・`aud`（設定。既定は受け口の URL で、Cloud Tasks の `OidcToken.audience` の既定と同じ。2026-10-29 に [OidcToken](https://docs.cloud.google.com/tasks/docs/reference/rest/v2/OidcToken) で確認）・
  `email` が設定のサービスアカウントで `email_verified` が真であることを確かめる。JWKS の取得とキャッシュは `internal/adapter/oidc` を使う。
  Cloud Run の Invoker ロールでの認証は、worker を別のサービスに分けたときに足せる追加の守りとし、アプリの検証を省かない
- **Lambda**: Lambda Web Adapter は転送するすべての要求にヘッダー `x-amzn-request-context` を付ける。pass-through の要求では値が JSON の `null`、
  Function URL の要求では Function URL が埋めた要求の文脈の JSON で、クライアントが同じ名前のヘッダーを送っても Lambda Web Adapter が置き換える。
  受け口（設定 `queue_sqs_events_path`、既定 `/events`。`AWS_LWA_PASS_THROUGH_PATH` と同じ値）は、この値が `null` の要求だけを受け、それ以外を 404 にする
  （2026-10-29 に Lambda Web Adapter v1.1.0 の `src/lib.rs` と、それが使う `lambda_http` 1.3.0 の `request.rs`・`deserializer.rs` で確認）
- どちらでも、**合図は DB にあるジョブの ID を指すだけ** なので、認証を破られても新しいジョブを作れず、`RunAt` 前のジョブを早められない（§10 の表）。できるのは DB の読み取りを増やすことだけで、レート制限で抑える
- メンテナンスモード（[ADR-0088](0088-service-shutdown-modes.md)）では、両方の受け口が 503 を返し、外部のキューの再試行に任せる

### 14. 外部のキュー: 設定とビルド

- `queue = cloudtasks`: キューのパス（`projects/<p>/locations/<l>/queues/<q>`）・受け口の URL・OIDC のサービスアカウント・audience。認証は ADC
- `queue = sqs`: キューの URL・`queue_sqs_events_path`。認証は SDK の既定の資格情報の連鎖
- どちらも `db` は `postgres` か `libsql`（`jobs` を持つ DB）。ビルドタグ `no_cloudtasks` / `no_sqs`
- **SQS の pull（常駐の worker が SQS を長くポーリングする形）は作らない**。常駐の worker があるなら `queue = db` で足り、SQS を挟む利点がない

### 15. 種類の選択: 設定

- 設定のキーは 2 つで、同時に指定すると起動に失敗する。どちらも指定しなければ、すべての種類を実行する。
  - `worker_job_types`: このプロセスが実行する種類だけを並べる
  - `worker_job_types_except`: このプロセスが実行しない種類を並べる
- 値はカンマ区切りで、種類の名前（`federation.deliver`）か接頭辞（`federation.*`）を書く。`role = web` で指定すると起動に失敗する。`role = all` では使える。
- プロセスごとの違いは環境変数で与えられるので、`my serve` のフラグは足さない。
- 起動時に、プロセスが持つハンドラの一覧に照らして実行する種類の集合を求める。**どのハンドラにも当たらない名前や接頭辞があれば起動に失敗する**。誤字に気づくためで、種類の名前を変えたリリースでも、起動の時点で分かる。求めた一覧はログに出す。
- `worker_job_types_except` は、ハンドラの一覧から除いた集合に直してから使う。除外も、種類ごとに引く速い形で取れる。

種類の名前は運営者の設定に現れる。名前の変更はリリースノートに書く（v0 では設定のキーの変更と同じ扱い。ADR-0023）。

### 16. 種類の選択: claim

- 指定のないプロセスは `PickJob` のままで、速さも動作も変わらない。
- 指定のあるプロセスは、種類ごとに `(type, run_at, id)` を引いて候補を 1 件ずつ得て、`run_at` がいちばん早いものを取る。PostgreSQL は `unnest` と `LATERAL`、SQLite は `UNION ALL` か種類ごとの問い合わせで書く。
- 期限・リース・順序キーの条件は `PickJob` と同じで、PostgreSQL の advisory lock と 2 回目の確かめもそのまま使う。
- PostgreSQL では、種類ごとの先頭の行を `FOR UPDATE SKIP LOCKED` でトランザクションの間だけロックする。使わなかった行のロックは他のワーカーに飛ばされるだけで、待たせない。83 種類を多数のワーカーで同時に引く場合の競合は測っていない。
- `jobs` に索引 `jobs_type_run_at_id`（`(type, run_at, id)`）を持つ（両方の方言）。指定を使わない構成にも索引の書き込みのコストがかかる。`jobs_run_at` は指定のないプロセスと `queue.wake` の掃除が使うので残す。

### 17. 種類の選択: 誰も実行しない種類

種類を外すと、どのワーカーも実行しない種類が生まれうる。定期の後始末のジョブも対象になる。プロセスは他のプロセスの設定を知らないので、起動時には判定できない。**ジョブが待ち続けていることで検知する**。

- 指標 `my.queue.oldest_ready_age` に種類のラベルを持たせる。警告のルールの例を運用の文書に書く。
- ジョブの画面で、種類ごとの最も古い待ち時間を見せる。
- 指定のあるプロセスは起動時に、**自分が実行しない種類** をログに出し、他のプロセスが実行する必要があることを伝える。
- `/readyz` には反映しない。他のプロセスの設定の問題で、正常なワーカーを落とさないため。
- ワーカーの一覧を登録する仕組みは持たない。指標はワーカーがいない以外の滞りも検知するし、登録の表や KV の管理が増えるため。

### 18. 種類の選択: 順序キー

判定は種類を見ない。同じキーの前のジョブが残っていれば、種類が違っても待つ。順序を保つためで、前のジョブの種類を実行するワーカーが動いている限り、別のワーカーに分かれていても順序は守られる。

ただし、前のジョブの種類を誰も実行しないと、後ろのジョブも止まる。種類をまたいでキーを共有するのは次のとおりで、同じワーカーの組に置くよう運用の文書に書く。

| キー | 種類 |
| --- | --- |
| `like:` | `federation.fanout`、`federation.deliver`（お気に入りの配送だけ） |
| `voice_room:` | `voice.media_event`、`voice.apply` |
| `supporter_subscription:` | `supporters.sync`、`supporters.command` |

分けた結果、片方の種類が滞ると、同じキーのもう一方も待つ。

### 19. 種類の選択: 外部のキュー

`worker_job_types` と `worker_job_types_except` は **`queue = db` だけで使え**、`cloudtasks` / `sqs` / `inline` では起動に失敗する。

- 合図は ID だけを持つので、受け取ってからでないと種類が分からない。受け取って実行しない場合に合図を出し直すと、合図が回り続けて課金も増える。
- SQS の受信の API にメッセージを選ぶ引数はない（[ReceiveMessage](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_ReceiveMessage.html)）。Lambda のイベントのフィルターは、条件に合わないレコードを捨てる（[Event filtering](https://docs.aws.amazon.com/lambda/latest/dg/invocation-eventfiltering.html)）ので、合図を失う。
- 外部のキューでは、種類の組ごとにキューを分けられる（§23〜§30）。指定しなければ既定のキューだけで動き（§23）、受け口のパスは増やさず、関数やサービスを分けるかは運営者が選ぶ（§25）。Cloud Tasks はキューごとに送る速さと同時数を設定できる（[Configure queues](https://docs.cloud.google.com/tasks/docs/configuring-queues)）。
- 3 つの文書はいずれも 2026-11-03 に確認した。

### 20. 種類の選択: Scheduler

`role = all` / `worker` のプロセスは、指定の有無によらずプロセス内の Scheduler を動かす（外部トリガーを使うプロセスは除く。§12）。投入は実行するワーカーと関係なく、周期の投入は冪等キーで 1 回になる。

### 21. 種類の選択: 同時実行数

種類の指定は同時実行数を含めない。種類を分けてプロセスを増やせば、種類ごとのスケールアウトはできる。1 プロセス内の同時実行は §31〜§36 が決める。

### 22. 種類の選択: リース

リースは全種類で一律の 5 分（`queue.DefaultLease`）とする。遅いジョブと速いジョブを分けても設定を増やさない。

### 23. キューの分割: 設定

合図はヒントでしかなく、どの受け口でもどのジョブでも実行できるので、キューを分けるのに必要なのは送る側の振り分けだけになる。受け口の変更と、種類で絞った claim は要らない。

- 設定のキーは方言ごとに 1 つ。指定しなければ、すべての合図が既定のキューに行く。
  - `queue_sqs_routes`: `federation.*=https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-federation,notification.email=https://sqs.ap-northeast-1.amazonaws.com/123456789012/my-mail`
  - `queue_cloudtasks_routes`: `federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com`。`@` 以降の受け口の URL は省略でき、省略すると `queue_cloudtasks_url` を使う
- 左辺の書き方は §15 と同じ（種類の名前か `prefix.*`）。**どのハンドラにも当たらない左辺は起動に失敗する**。右辺の形式の誤り、同じ左辺の重複も起動に失敗する。
- いちばん具体的なものを選ぶ。完全な名前が接頭辞に勝ち、接頭辞は長いほうが勝つ。
- `queue = db` / `inline` で指定すると起動に失敗する。§19 の、外部のキューで `worker_job_types` を拒むことは変えない。

#### 設定する先

ルートは **合図を送るすべてのプロセス** に、同じ値で設定する。受け取る側が持っていても、受け取りには使わない。

DB のインスタンス設定には置かず、全プロセス共通の設定に書く。DB に置けば置き場所は 1 か所になるが、合図を送る処理が設定を読むようになり、キューの URL という構成の情報が管理画面の側に入る。送信の権限はどのみちプロセスごとに要る。

| 誰が | 何を | なぜ |
| --- | --- | --- |
| 運営者 | キュー・受け口・ルートの対応を決める。ルートは全プロセス共通の設定（同じ TOML や環境変数の組）に 1 か所で書く | 分けるかどうかは構成の判断で、インスタンスの管理画面の設定ではない |
| `role = web`、`all` のプロセス | ルートを持ち、送り分ける | **利用者の操作で積むジョブの合図は、ほとんどここから出る**。投稿の `federation.fanout` は `web` が積む |
| `role = worker` のプロセス | ルートを持ち、送り分ける | ジョブが積む次のジョブ、リトライ、順序キーの次のジョブの合図と、掃除（§26）はここから出る。`federation.deliver` は `worker` が積む |
| 受け口（Lambda、Cloud Run） | キューの合図を受ける。ルートは見ない | 実行するかは `ClaimJob` だけで決まる（§25）。ただし受け口も `worker` なので、次のジョブのために同じルートを持つ |
| 認証情報 | 全プロセスに、ルートのすべてのキューへの送信の権限を与える（`sqs:SendMessage`、`roles/cloudtasks.enqueuer`） | 権限がない送信は失敗してログに出て、掃除が 2 分後から拾う |

設定が欠けたときの動作は次のとおり。**ジョブは失われないが、キューを分けた目的が黙って崩れる。**

| 状況 | 結果 |
| --- | --- |
| `web` にルートがない | `web` が積むジョブの合図が既定のキューに行く。既定の受け口で実行され、分けた処理能力を使わず、絞った上限も効かない |
| `worker` にルートがない | ジョブが積む次のジョブと掃除の合図が既定のキューに行く。上と同じ |
| 権限がない | その種類の合図が毎回失敗し、実行が期限の 2 分後以降に遅れる。**既定のキューには回らない**（§24） |

食い違いに気づけるように次を持つ。

- 起動時に、ルートの一覧とその値のハッシュをログに出す。プロセス間で比べられる。
- `my config check` は `role` によらずルートの形式と左辺を確かめる。どの `role` も同じハンドラの一覧を持つので、`web` でも同じ判定になる。
- 運用の文書に、`role` ごとに別の設定を持つ構成（Cloud Run のサービス、Lambda の関数）で全部に同じ値を渡す例を書く。
- 指標 `my.queue.oldest_ready_age` では検知できない。ジョブは既定の受け口で動いて滞らないため。キューごとの滞留はプラットフォームの指標で見る。

### 24. キューの分割: 合図の送り分け

- `queue.Wake` は `Type` を持つ。**外に出す合図の本文はジョブの ID のままで、種類はプロセス内の振り分けにだけ使う**（§9 の「合図は ID と時刻だけを持つ」は変えない）。
- `Waker` を実装する振り分けの部品を `internal/app/queue` に置く。種類で分けて、キューごとの adapter の `Waker` に渡す。adapter は変えない。
- 合図を出す場所は 6 つ（投入の `wakesOf`、dead のジョブの再実行、早く届いた合図の送り直し、リトライ、順序キーの次のジョブ `NextJob`、掃除の `WakeSweeper`）で、種類が要るので、次の読み取りが種類も返す。`RequeueDeadJob`、`ClaimJob` の `ClaimNotDue`、`NextJobOfKey`、`ListDueJobs`。投入とリトライは種類を持っている。
- 送信の失敗はログに出して掃除に任せる。**他のキューには回さない**（絞っている種類が他のキューにあふれないように）。

### 25. キューの分割: 受け口

- **増やさない。** SQS は `queue_sqs_events_path`、Cloud Tasks は `/_internal/jobs` の 1 つが、どのキューの合図も受ける。認証は §13 のまま。
- 合図のキューに応じて処理を変えない（`PushReceiver.Receive`）。実行するかは `ClaimJob` だけで決まる。
- プラットフォームの構成は運営者が選ぶ。どちらも同じイメージで動く。

| | キューの分け方 | 処理能力の設定 |
| --- | --- | --- |
| AWS（1 つの関数） | キューごとに event source mapping | mapping ごとの最大同時実行数（2〜1,000、設定は無料）。関数の reserved concurrency は最大同時実行数の合計以上にする |
| AWS（関数を分ける） | キューごとに関数 | 関数ごとのメモリ・タイムアウト・reserved concurrency |
| Google Cloud（1 つのサービス） | ルートに URL を書かない | キューごとの `maxConcurrentDispatches` と `maxDispatchesPerSecond`。インスタンスの上限はサービスで共有する |
| Google Cloud（サービスを分ける） | ルートに別のサービスの URL を書く | 上に加えて、サービスごとの最大インスタンス数と同時実行数 |

- 分けたサービスは自分の URL を `queue_cloudtasks_url` に持つ。audience の既定は呼ばれた URL なので、§13 の検証で通る。
- URL をルートに書く形にし、Cloud Tasks のキューの `uriOverride` には頼らない。上書きしたときの OIDC の audience の扱いが確かめられず（要検証）、CMEK とも併用できないため。

### 26. キューの分割: 掃除と tick

- **掃除はルートごとに行う。** 全体で 500 件の上限のままだと、配送の滞りで古い合図が先頭を占め、失われたメールの合図が拾われなくなる。これは分けた目的に反する。
- ルートごとに 1 回 500 件まで、その種類の `DueJobs` を読んで、そのキューに送る。ルートのある種類は索引 `(type, run_at, id)`（§16）を引く。既定のキューの分は `type <> ALL(ルートの種類)` で読む。ハンドラのない種類も既定のキューに残すため。測定で 80 ms 程度の形だが、1 分に 1 回なので許容する。
- ルートがなければ、`DueJobs` を `run_at` の早い順に 1 回 500 件までの 1 回だけで、クエリも §11 のまま。
- tick はキューを問わず処理できる（SQS では同じキュー、Google Cloud では Cloud Scheduler から `/_internal/tick` へ届く）。SQS では、tick を混ぜるキューが滞ると周期のジョブも掃除も遅れるので、滞りうる種類を既定のキューから外すか、tick だけのキューを作ることを運用の文書に書く。

### 27. キューの分割: 割り当ての変更と未設定の種類

- どのルートにも当たらない種類は既定のキュー（`queue_sqs_queue_url` / `queue_cloudtasks_queue`）に行く。**新しいバージョンで増えた種類も既定のキューで動くので、止まらない。**
- 送り先を変えたとき、途中の合図は古いキューに残るが、その受け口が動いていれば実行される。古いキューを消すと合図は失われ、掃除が期限の 2 分後から拾う。SQS の遅延は最大 15 分だが、Cloud Tasks の合図は最大 30 日先の分が残っていることがある。どちらでも失われるのは合図だけで、ジョブは DB にある。
- ルートの変更をプロセスに順に反映している間は、合図の送り先が混ざるだけで、ジョブは失われない。食い違いが残り続ける場合の影響は §23 の「設定する先」のとおり。

### 28. キューの分割: 順序キー

順序は `ClaimJob` と `NextJob` が DB で守るので、キューを分けても変わらない。前のジョブが残っていれば合図は捨てられ、前のジョブが終わるときに次のジョブの種類のキューへ合図する。
§18 と違い、種類をまたぐキー（`like:`、`voice_room:`、`supporter_subscription:`）を同じ組に置く必要はない。ただし、前のジョブのキューが滞れば、同じキーの次のジョブも待つ。

### 29. キューの分割: 止まったキューの検知

ジョブは DB にあるので、§17 の `my.queue.oldest_ready_age`（種類のラベル）で同じように検知する。外部のキューの登録や、キューごとの指標の取得は足さない。
受け口のないキューには合図がたまり続けるので、運用の文書に、対応する受け口の確認手順とプラットフォームの指標（SQS の滞留、Cloud Tasks のタスク数）を書く。

### 30. キューの分割: 費用と上限

2026-11-03 に公式の文書で確認した。

- **SQS**: 最低料金はなく、すべての操作をリクエストとして数え、月 100 万リクエストは無料（[pricing](https://aws.amazon.com/sqs/pricing/)）。キュー単位の料金の記載はなく、キューの数の上限は [quotas](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/quotas-queues.html) に記載がない。100 万リクエストあたりの単価は取得した本文に含まれず **要検証**。
- **Lambda**: event source mapping の同時実行は既定で最大 1,250、[最大同時実行数](https://docs.aws.amazon.com/lambda/latest/dg/services-sqs-scaling.html)は 2〜1,000 で mapping ごと、設定は無料。関数に複数の SQS を結べる。最初は 5 つの並行で読み、低負荷でも 2 まで下げる。バッチの上限は標準キューで 10,000（[parameters](https://docs.aws.amazon.com/lambda/latest/dg/services-sqs-parameters.html)）、処理中のメッセージはキューあたり約 120,000（[visibility timeout](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html)）。
- **SQS の待ち受けの費用**: mapping はメッセージがなくてもキューを読み続けるので、**キューが増えるとリクエスト数が増え、無料枠を超えうる**。読む間隔は確かめられず、金額は **要検証**。実アカウントでの確認を実装の前提にする。
- **Cloud Tasks**: キューは 1 リージョン 1,000 まで、1 キューの送出は毎秒 500 件まで（「より高い速さには複数のキューを使う」）、予定は 30 日先まで（[quotas](https://docs.cloud.google.com/tasks/docs/quotas)）。キューごとに同時数と速さを設定できる（[configuring queues](https://docs.cloud.google.com/tasks/docs/configuring-queues)）が、それぞれの既定値と上限の記載は確かめられなかった。料金のページは本文を取得できず、キューそのものの料金と単価は **要検証**。
- **Cloud Run**: インスタンスあたりの同時実行は最大 1,000（[concurrency](https://docs.cloud.google.com/run/docs/about-concurrency)）、最大インスタンス数はサービス単位で設定でき既定 100（[max instances](https://docs.cloud.google.com/run/docs/configuring/max-instances)）、リクエストのタイムアウトは最大 60 分（[request timeout](https://docs.cloud.google.com/run/docs/configuring/request-timeout)）。

### 31. 同時実行: 設定

- 設定のキー `worker_job_concurrency`: 1 つのプロセスが同時に実行するジョブの数で、**既定は 1**。範囲は 1〜128（測った範囲）。1 のときは 1 件ずつの動作（`RunOnce` の繰り返し）になる。
- 種類ごとの数は持たない。種類ごとに変えたいときは、§15 の `worker_job_types` でプロセスを分け、プロセスごとに数を与える。
- `role = web` で 2 以上を指定すると起動に失敗する。`queue` が `db` 以外のときも同じ。
- **`worker_job_concurrency` が PostgreSQL のプールの最大以上なら、起動に失敗する**（§34）。

既定を 1 にするのは、実際のハンドラで本番の検証をしていないため。実際のインスタンスで確かめてから既定を上げることは、この決定を覆さずに別に決められる。

### 32. 同時実行: 動作

- スロットごとに `RunOnce` と同じ claim と実行を行う。ジョブごとに **空いたスロットが claim する** ので、claim 済みのジョブがスロットを待つことはなく、リースの意味は変わらない（先読みをしない）。
- ジョブが見つからなければ、1 つのスロットだけが `PollInterval` ごとにポーリングを続け、残りは待つ。ジョブが見つかれば、待っているスロットを起こす。
- スロットごとにワーカーの名前を持つ（claim と試行の記録に出る）。
- claim の並行と順序キーの扱いは、複数のワーカーの間と同じ（`PullRunner` は複数で 1 つの `Source` を共有してよい）。`queue/db` の変更は要らない。

### 33. 同時実行: ハンドラの安全性

82 種類のハンドラとその依存を読んだ範囲では、接続の待ち（§34）を除いて、同時実行で壊れるものは見つからなかった。ただし全行は読んでおらず、実行して確かめたのは試作の範囲（ハンドラは試作のもの。`-race` で 1 × 32 を 10 秒実行し、競合の報告なし。`Meter` と `Tracer` は付けていない）。

| 場所 | 状態 | 同時実行での扱い |
| --- | --- | --- |
| `internal/app/federation/breaker.go` | ホストごとの状態を持つ | mutex あり。プロセスごとの状態で、スロットで共有されるぶん早く開く |
| `internal/app/content/link_card_hosts.go` | ホストごとの fetch の制限 | KV の lock で守る。`linkCardHostHold` は 1 つの fetch だけのもの |
| `internal/adapter/media/native/native.go` | 画像の処理 | 既定で CPU の半分までの semaphore がある（後述） |
| `internal/adapter/httpclient/httpclient.go` | 1 ホスト 8 接続 | 1 つのクライアントを共有する。超えると待つ（後述） |
| `internal/adapter/jsonld/jsonld.go` | `@context` の読み込み | 読むたびに新しく解く。並行を前提に書かれている |
| `internal/adapter/mail/smtp/smtp.go` | メールの送信 | 接続を共有しない |
| `internal/app/kernel/keymanager_cache.go`、`internal/app/instance/overview.go`、`internal/app/content/remote_media.go`、`internal/app/uow/projector.go` | キャッシュと登録 | mutex または atomic あり |

複数のスロットを動かした統合テストを `-race` で回し、見つかったものを直す。

待ちの注意点が 2 つある。

- **画像の処理**: semaphore を待つ時間もリースに数える。スロットが CPU より多く、`media.process` ばかりがたまると、待っている間にリースが切れてやり直す可能性がある。測っていない。CPU を使う種類は、§15 で少ないスロットのプロセスに分けるよう運用の文書に書く。
- **同じホストへの集中**: 1 ホストの上限を超えたスロットは接続を待つ。止まっているホストへの配送でスロットが埋まると、他の種類も待つ。これはプロセスが 1 スロットのときも同じで、割合はスロットが増えても変わらないが、1 ホストの上限 8 はプロセス単位なので、プロセスを増やす場合より早く上限に当たる。

### 34. 同時実行: DB の接続

スロットの数に合わせて接続を自動で増やさない。`database_url` の `pool_max_conns` は運営者の設定で、外部の接続の上限（Cloud SQL、RDS、接続プール）を知っているのは運営者だから。

**`worker_job_concurrency` がプールの最大以上なら、起動に失敗する。** 警告では足りない。トランザクションを持ったまま別の接続を取るハンドラがあると、スロットがプールに届いたときに全員が互いを待ってハンドラの期限まで止まることを再現した（`経緯`）。プールの既定は「4 と CPU 数の大きいほう」（pgx の既定）なので、1 vCPU の既定なら 3 までで、それより多くするには `pool_max_conns` を上げる。
指定のないとき（既定の 1）は判定しない。`pool_max_conns=1` の構成を起動できなくしないためで、この構成は 2 本目を取った時点で止まる。この制限は `worker_job_concurrency` だけのもので、プッシュの受け口には適用しない（§36）。

SQLite と libSQL ではプールの最大がないので判定せず、32 を超えると起動時に警告のログを出す。SQLite の書き込みは 1 つずつで（`BEGIN IMMEDIATE`、`busy_timeout` 5 秒）、128 で処理量が下がったため。libSQL は測っていない。

#### トランザクションの中で別の接続を使う箇所

次の箇所を `tx` を通す形に直すことを実装の範囲に含める（#2009）。直しても起動時の判定は残す。claim と結果の記録も接続を要するので、スロットがプールに届く構成は遅くなるだけで得がなく、調べていない箇所が残る可能性もあるから。

調べ方の限界: `internal/app`、`internal/platform`、`internal/interface` の、`uow.Tx` を引数に持つ関数の中で、`ctx` という名前の引数を渡して `tx` を渡さない **直接の呼び出し** だけを機械的に探し、読んで確かめた（枝 `spike/2004-worker-concurrency` の `spike/2004/txscan`）。呼び出された先で別の接続を使うものは、呼び出しの箇所として挙がるが、`tx` を受け取った adapter の中で別の接続を使うものや、`ctx` 以外の名前の文脈を渡すものは見つけられない。実際のハンドラを同時に動かして確かめてはいない。

| 箇所 | 読むもの | 通る経路 |
| --- | --- | --- |
| `notification/projection.go` | `UnsolicitedReachRestricted`（`moderation/account_policy.go`） | **送り元のある通知のすべて**（`account_moved` を除く）。`federation.inbox`、`notification.post_subscribers` などのジョブと、お気に入りなどの HTTP 要求 |
| `content/remote_reaction.go` | `FindCustomEmojiByShortcode` | `federation.inbox`（他サーバーの絵文字を借りたリアクション） |
| `content/remote_interaction.go` | `FindBoostRetraction` | `federation.inbox`（ブーストの取り消し） |
| `content/remote_status.go` | `FindRevision`、`FindDeletionByURI` | `federation.inbox`（投稿の更新と削除） |
| `federation/quote_send.go` | `FindStatusByURI`、`remoteInbox` | 引用のイベントの投影。HTTP 要求と `federation.inbox` |
| `identity/staff.go` | `LocalAccountOfUser` | モデレーターへの通知の投影。HTTP 要求と、そこに至るジョブ |
| `identity/moderation.go`、`timeline/moderation.go` | `accounts.FindByID` | 処分と削除のイベントの投影。HTTP 要求と `account.execute_deletions` などのジョブ |
| `voice/records.go` | `HeldAccountIDs`、`ReportedVoiceRooms` | `voice.purge_records`（定期ジョブ） |
| `federation/reports.go` | `InstanceActor` | 通報の転送の投影。HTTP 要求 |
| `federation/outbound.go` | `ListCurrentSettings`（連合のモード） | 別名の変更の投影。HTTP 要求 |
| `account/alias.go`、`account/move.go`、`identity/external_link.go` | `HasAlias`、`FindMove`、`ListLoginProviders` | HTTP 要求だけ |

### 35. 同時実行: 停止とリース

- `stop` を閉じたら、どのスロットも新しいジョブを claim しない。実行中のジョブは `ShutdownTimeout`（既定 25 秒）まで続ける。その後に取り消し、`JobStopTimeout`（既定 3 秒）だけ待つ。
- リースは一律の 5 分で、ハンドラの期限は claim の前から数える（`RunOnce`）。
- 同時に止まるジョブの数はスロットの数だけ増える。取り消されたジョブは試行を 1 回使う。試作の強制終了でも、処理中の数だけリトライが記録された。また、結果の記録の上限 `RecordTimeout`（10 秒）は `JobStopTimeout` より長いので、取り消しの後の記録が間に合わないジョブはリースの期限まで待つ。スロットが増えるとこの数も増える。
- `ShutdownTimeout` は変えない。応答に時間がかかる種類を大きなスロットで動かすと、デプロイのたびに試行を使う数が増えることを運用の文書に書く。

### 36. 同時実行: プッシュの受け口

`queue = cloudtasks` と `sqs` では同時実行数はプラットフォームが決める（Cloud Run の同時要求数、Cloud Tasks のキューの同時数、Lambda の同時実行数）。`worker_job_concurrency` は使えず、2 以上で起動に失敗する。外部のキューで種類ごとに処理能力を分ける方法は §23〜§30 が決める。プールの判定は、プッシュの受け口には適用しない。

### 37. 覆る条件

- 成功した試行も DB で辿りたいという運用の要望が強い（ログの基盤なしで完了したジョブを調べたい）なら、すべての試行の記録を設定で選べるようにすることを再検討する
- ジョブの数が、DB の `jobs` の書き込み（1 ジョブに INSERT・claim・DELETE）を支えられないほど多いと分かった場合、対象の DB が ID での claim のトランザクションに耐えない場合（#1846）は、§9 のジョブを外部のキューに置かない形を見直す
- Lambda Web Adapter が pass-through の要求の印（`x-amzn-request-context` の `null`）を変えた場合は、§13 の守りを組み直す
- 本番に近い環境で、種類ごとに引く claim が速くならない場合や、索引の書き込みが投入を明らかに遅くする場合は、レーンの列（`採らなかった案`）と比べ直す
- 実アカウントでの確認で、待ち受けの費用がほぼ $0 の構成を壊すほど大きい場合（キューを分けるのを既定にはしないので、使う人だけの費用にとどまるが、文書で強く警告する）。または、滞ったキューで掃除の重複の合図が費用や処理を圧迫する場合（§26 の掃除の間隔や上限を見直す）
- 実際のハンドラの統合テストで安全に並行できない種類が多く見つかる場合や、本番に近い環境で処理量が伸びない場合は、プロセスを増やすだけの形と比べ直す

## 経緯

連合配送・Inbox 処理・メディア処理・通知・クリーンアップなど非同期処理が多く、キューは各クラウドのマネージドサービスに切り替えたい（G3、NFR-COST-2、NFR-REL-1, 2）。サービスごとに配信モデルが違う。DB キューは pull で遅延の上限がなく、PostgreSQL は `FOR UPDATE SKIP LOCKED`、SQLite は単一ワーカー前提。
SQS は pull（Lambda event source なら push 的）で遅延 15 分、可視性タイムアウト最大 12 時間、メッセージ最大 1 MiB、保持最大 14 日。Cloud Tasks は push で遅延 30 日、タスク名の重複排除は削除後最大 24 時間、キューあたり最大 500 件/秒、タスク最大 1 MiB、dispatch deadline 15 秒〜30 分。Pub/Sub には遅延配信がない（要検証: 本調査の対象外）。
料金は Cloud Tasks が月 100 万オペレーションまで無料（以降 $0.40 / 100 万）、SQS が月 100 万リクエストまで無料で、配送のリトライも 1 回に数える（いずれも 2026-10-16 時点の公式ドキュメント。出典は [deployment-targets.md](../architecture/deployment-targets.md)）。

最初は外部のキューにジョブそのものを積むアダプタを想定していた。M6 のターゲット（Cloud Run の min instances 0、Lambda + Lambda Web Adapter）の設計で、順序キー・未完了の間の冪等キー・`RunAt`・待ちのジョブの一覧を、外部のキューがそのままでは満たせないと分かった（Cloud Tasks と SQS 標準キューは順序なし、SQS は FIFO だけ `MessageGroupId` で順序を持つが遅延はキュー単位だけで、`MessageDeduplicationId` は 5 分。SQS の値は 2026-10-29 に [SendMessage](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_SendMessage.html) で確認）。そこで合図だけを運ぶ形（§9〜§14）にし、SQS 常駐ワーカーの pull と Kubernetes の CronJob をやめ、`outbox_events` とリレーを除いた。外部のキューでは未完了の一覧を出さない当初の判断（§7）も、これで不要になった。

ダッシュボード（NFR-OPS-3）は、失敗して止まったジョブしか見えず前の試行の理由も上書きされるという不足から足した。[ADR-0055](0055-service-observability.md) §4 の「出来事の数を DB に書かない」に触れないよう、失敗の数にしか比例しない記録にした。ダッシュボードに並ぶジョブ（未完了・失敗）の試行は成功しなかった試行がすべてなので、一覧に出るジョブの経過は欠けない。

種類の選択（NFR-PERF-2）は、配送がたまるとメールや通知も待たされ、ワーカーを増やしても全種類で分け合うことから決めた。手元の WSL2 の PostgreSQL 18.6 と SQLite 3.45.1 で、期限の来た 100 万件（`federation.deliver` が 90 万件で他より古い最悪の分布）を測った。**本番相当の環境ではない。** 1 回の claim は、指定なし 0.16 ms（12 ブロック）、`type = ANY` や `type <> ALL` を足すだけの形 約 80 ms（約 1.5 万ブロック、ジョブのない種類を待つポーリングは順次走査で 55 ms）、種類ごとに引く `LATERAL` は 1 / 3 / 83 種類で 0.18 / 0.20 / 0.31 ms。索引だけではプランが変わらず、**クエリの形を変える必要がある**。8 クライアントの負荷では毎秒約 100 件と約 5,600 件（桁だけを読む）。SQLite も 84〜104 ms と 0.07 ms 以下で、遅い claim は他の書き込みも止める。索引は 100 万件で 64 MB（`jobs_run_at` は 21 MB）で、投入の遅れは見えなかったが切り分けていない。詳細は [#1996 の報告](https://github.com/mycroblog/my/issues/1996) と枝 `spike/1996-worker-job-claim`。
決定者が外部のキューでも特定の種類を増やす手段を求めたので、キューの分割（§23〜§30）を足した。

同時実行（NFR-PERF-2）は、待ちの多いジョブでプロセスが丸ごと空くことから決めた。手元の WSL2（24 CPU、46 GB）の試作（枝 `spike/2004-worker-concurrency`）で、応答 200 ms のとき 32 × 1 は 127〜154 件/秒・747 MiB・32 接続、1 × 32 は 141〜153 件/秒・34 MiB・24 接続、1 × 128 は 328〜601 件/秒・41 MiB。応答 2 s も理論値に近い。本物の `my serve` は 1 プロセス 86 MiB で、32 プロセス約 2.7 GiB に対し 1 プロセス 32 スロット約 100 MiB と **推定** する。`pool_max_conns=4` では 1 × 128 が 185〜297 件/秒に下がり、`MaxConnsPerHost = 8` で 1 ホストに送ると 1 × 32 は 40 件/秒で止まる。SQLite は 1 × 32 で 158 件/秒だが 1 × 128 は 73〜106 件/秒に下がった。ジョブがないときのトランザクションは、128 スロットでスロットごとのポーリング毎秒 131、1 つのスロットだけ毎秒 8、1 つの claim のループ毎秒 2。
トランザクションを持ったまま 2 本目の接続を取るハンドラ（`pool_max_conns=4`、`content/remote_reaction.go` と同じ形）は、スロットが 3 までは動き、`hold` 5 ms の 4 スロット以上と `hold` 0 の 8 スロット以上で処理量がほぼ 0 になり、`context deadline exceeded` と `idle in transaction` の接続が残った。2 本目を取らないハンドラは 80 スロットでも止まらない。種類の指定と組み合わせても処理量は落ちない（例外は 1 種類を 128 スロットで取るときの 1 割強で、原因は切り分けていない）。詳細は [#2004 の報告](https://github.com/mycroblog/my/issues/2004)。

## 採らなかった案

- すべての試行を記録する: 完了したジョブも DB で辿れるが、成功のたびに書くと書き込みが利用量に比例し、Neon・Aurora の scale to zero を妨げる。
- 試行の記録を持たない: 実装は最小だが、前の試行の理由が残らず、ログの基盤のない Self-hosted では管理画面から辿れない。
- ハンドラのログも保存する: 経過は最も詳しいが、リモートのアカウント・投稿の URL や本文の断片を含みうる個人情報の写しが増え、行の量の上限も決めにくい。
- 未完了の一覧をすべてのアダプタに必須にする（`ListTasks`・`GetQueueAttributes`）: 見え方は揃うが、SQS は覗くと可視性タイムアウトが変わり、Cloud Tasks は呼び出しに費用がかかる。§9 でジョブが DB に来たので不要。
- ジョブを外部に積み、DB の補助の表で補う: ジョブの量が DB に縛られないが、持ち主の表・再投入のチェーン・一覧の代わりが要り、DB への書き込みも残り、不整合の扱いが大きい。ペイロードがクラウドに置かれ、受け口の認証を破られると任意のジョブを作れる。
- ジョブを外部に積み、足りない性質を諦める: 実装は最小だが、順序キーに頼る supporters・voice・データの取り込みが Google Cloud・AWS でだけ壊れる。受け口の認証を破られると任意のジョブを作れる。
- 種類の指定 + `PickJob` に条件を足すだけ: 変更は最小で、混ざった分布なら 1〜3 ms で済むが、たまったときに 1 回 80 ms かかり、処理量が 2 桁下がる。
- `jobs` のレーンの列: 名前が安定し、順序キーもまとめやすいが、列と投入の全経路の変更が要り、運営者の環境に合わせて分けられない。
- ワーカーの種類を選ばない（全体を増やすだけ）: コストはないが、配送の洪水の間、メールや通知は順番を待つ。
- 決め打ちのレーンごとのキュー: 運営者は種類の名前を知らなくてよいが、分け方は実際の負荷で決まり、種類の指定と 2 つの言葉を持つ。
- 1 つのキューのまま受け口が送り直す: キューも設定も増えないが、合図が回り続けて課金と実行回数が増える。
- キューを分けない: 実装は要らず受け口全体は増やせるが、増やした分を全種類で分け合い、滞りの順番待ちは解消しない。
- スロットの数だけ `PullRunner` を並べる: 試作は数行で同じ処理量が出るが、ジョブがないときのトランザクションがスロットに比例する（128 で毎秒 131）。
- 1 つの claim のループ: ポーリングは最少（毎秒 2）で単純だが、128 スロットで 88〜101 件/秒に留まり、32 スロットより遅い。
- 種類ごとの同時実行数: 1 プロセスで遅い種類と速い種類に別の数を与えられるが、空きの判定が要り、§15 でプロセスを分ければ同じことができる。
- プロセスを増やすだけ: 障害の範囲が小さく CPU の取り合いがない（CPU を使うジョブではこれが正しい選択のまま）が、待ちの多いジョブで同じ処理量に 20 倍以上のメモリを使う。

## Consequences（影響）

- どのターゲットでもハンドラは同一コードで、エントリポイントだけが異なる。順序キー・冪等キー・リトライ・`dead_jobs`・試行の記録・待ちのジョブの一覧が、すべてのキューで同じになる。遅延上限やバッチサイズの差はアダプタが吸収する。
- 管理画面だけで、キューの滞留・実行中のジョブ・リトライの経過・失敗の理由を追える。完了したジョブの経過は DB に残らず、構造化ログで `job_id` を検索する。
- 失敗が大量に続く障害（連合先の停止で配送が全部リトライする）のとき、記録の行も同じだけ増える。保持期間と毎時の消去で上限を持つが、障害の間は DB の書き込みが増える。`error` に個人情報を入れない規則は型では守れず、ハンドラのレビューとテストで守る。
- ペイロードと秘密がクラウドのキューに出ず、偽の合図の害は DB の読み取りに限られる。
- 外部のキューのアダプタは「合図を送る」だけの小さなものになり、受け口・掃除・tick は `internal/app/queue` で共有する。
- ジョブごとに DB への書き込みが残り、ジョブの量の上限は DB で決まる。
- 合図の取りこぼしは掃除の周期（1 分 + 猶予）まで遅れる。順序キーで止められたジョブは前のジョブの完了の合図を待ち、前のジョブの runner が合図を送る前に落ちると、掃除の周期だけ遅れる。
- 外部トリガーは KV に周期の記録を置くので、`kv = memory` では使えない。
- 種類の指定: 配送がたまっていても、メールや通知だけを別のワーカーで実行し、配送だけのワーカーを増やせる。指定しない構成の claim は変わらないが、索引の容量（100 万件で 64 MB）と、切り分けられていない書き込みのコストはかかる。
- 誤った組み合わせで誰も実行しない種類ができると、検知まではジョブが止まる。定期の後始末やメールも止まりうる。
- `worker_job_types` のプロセスは、新しいバージョンで増えた種類を実行しない。`worker_job_types_except` のプロセスは自動で実行する。リリースごとに確認が要る。種類の名前が設定の一部になり、変更が運営者に見える。
- 種類の指定は Cloud Run / Lambda の構成では使えない。libSQL は測っていない。SQLite と同じクエリを使うが、遠隔の場合の往復は `UNION ALL` なら 1 回、種類ごとの問い合わせなら種類の数だけかかる。
- キューの分割: Lambda と Cloud Run の構成でも、配送だけを増やす、メールを専用の枠で動かす、といった設計ができ、分け方を誤ってもジョブは失われない。新しい種類は既定のキューで動く。
- ルートの設定を、合図を送るすべてのプロセス（`web` を含む）に揃える手間があり、揃っていなくてもエラーにならず、分けた効果だけが黙って失われる。キューや mapping の作成も運営者の作業である。
- 処理能力の上限は DB にもある。1 つのルートの同時実行を増やすと DB の接続と書き込みが増え、他の種類を遅くしうる。接続数の設計は別に要る。
- 滞ったキューでは、掃除が 1 分ごとにルートあたり最大 500 件の重複した合図を送る。重複は害がないが、課金は増える。1 つの Cloud Run サービスのままでは、キューで絞ることはできても、種類ごとに最大インスタンス数は分けられない。
- 同時実行: 待ちの多いジョブの処理量を、メモリと DB 接続をあまり増やさずに増やせる。接続はプロセスを増やす場合より少ない（測定では 32 に対して 24 以下）。既定のままなら 1 件ずつで、ジョブがないときの負荷も増えない。
- 1 つのプロセスの障害で、実行中のジョブがスロットの数だけ止まり、リースの期限まで待つ。パニックはジョブごとに回復するが、メモリ不足で落ちると全部が巻き込まれる。CPU とメモリをスロットで奪い合う。スロットの実行の順序は保証しない。
- 1 ホスト 8 接続の上限はプロセス単位のままで、1 プロセスに集めるとその分だけ早く当たる。`pool_max_conns` を一緒に考えることになり、既定のプールのままでは 3 スロットまで（1 vCPU の場合）で、接続の数がプールに届く構成は起動できない。外部の接続の上限が低い環境では、スロットの上限もそこで決まる。

## Open Questions / 要検証

- Cloud Tasks の HTTP ターゲット認証（OIDC トークン）と Cloud Run の実機での疎通。公式ドキュメント上は、ID トークンを付与し、サービスアカウントに Cloud Run Invoker ロールを与える構成で可能
- SQLite DB キューで web と worker を同一プロセスにした時のロック競合
- §13 の `x-amzn-request-context` の値と §10 の応答の本文の扱いは、Lambda Web Adapter のソースから読んだもので、実機では見ていない。M6 のデプロイのスモークテストで、pass-through の要求と Function URL の要求の両方のヘッダーの値と、`batchItemFailures` が効くことを確かめる
- 本番に近い環境（小さなインスタンス、キャッシュなし）での claim の時間と、索引の書き込みのコスト
- SQS のリクエストの単価と、mapping の待ち受けが月に生むリクエスト数（実アカウントで確認）
- Cloud Tasks の料金（キューそのものの費用、単価、無料枠）、`maxConcurrentDispatches` と `maxDispatchesPerSecond` の既定値と上限
- キューの分割の未確認: Lambda と Cloud Run で分けたときの実際のスケーリング、ルートごとの掃除の所要時間、同じイメージを環境変数だけ変えて複数のサービスで動かす手順
- 実際のハンドラの統合テストで、同時実行で壊れる種類がないこと（§33）。§34 の表の外に 2 本目の接続がないこと
- 実際のインスタンスでの処理量とメモリ。既定を 1 より上げるかは、その結果で決める。実際のハンドラ（配送の署名、投稿の取り込みなど）の並行時の処理量とメモリ、実際のリモートのサーバーとの接続（TLS）、DNS、多数のホスト、キャッシュの乗らない小さなインスタンス、libSQL、ネットワーク越しの DB、長時間の動作、停止のときの実行中のジョブの数ごとの影響、2 本目の接続で止まる現象の本物のハンドラや実際のプッシュの受け口での再現、1 つのスロットだけのポーリングの毎秒 8 が 1 より多い理由（窓の中のスロットの再開と思われるが未確認）は測っていない
