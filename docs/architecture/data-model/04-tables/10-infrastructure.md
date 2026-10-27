### 4.10 基盤（アダプタ所有）

| テーブル | カラム | 一意 | インデックス |
| --- | --- | --- | --- |
| `outbox_events` | `id`, `job_type`, `payload`（`document`）, `idempotency_key`（NULL 可）, `ordering_key`（NULL 可）, `trace_parent`（NULL 可）, `run_at`, `created_at` | — | PK 順に読み、キュー投入後に削除 |
| `jobs`（`queue/db`） | `id`, `type`, `payload`（`document`）, `idempotency_key`（NULL 可）, `ordering_key`（NULL 可）, `trace_parent`（NULL 可）, `attempt`, `run_at`, `locked_by`（NULL 可）, `locked_until`（NULL 可）, `last_error`（NULL 可）, `created_at` | `(idempotency_key)` | `(run_at)`, `(ordering_key, run_at)` |
| `dead_jobs` | `id`, `type`, `payload`（`document`）, `idempotency_key`（NULL 可）, `ordering_key`（NULL 可）, `trace_parent`（NULL 可）, `attempt`, `last_error`, `created_at`, `failed_at` | — | `(failed_at)` |
| `job_attempts`（イベント） | `job_id`, `attempt`, `job_type`, `outcome`（`enum(retry, dead)`）, `error`（`text`、2,000 バイトまで）, `worker`, `started_at`, `finished_at` | PK `(job_id, attempt, finished_at)` | `(finished_at)` |
| `kv_entries`（`kv/db`） | `namespace`, `key`, `value`（`bytes`）, `expires_at`（NULL 可 = 無期限） | PK `(namespace, key)` | `(expires_at)` |
| `data_upgrade_runs`（イベント、`db/*`） | `step_key`（`string`）, `completed_at` | PK `(step_key)` | — |

- ジョブのペイロードはアダプタ間を渡る「解釈しない文書」なので `document` 型です（ドメインのデータは入れず、ID を渡します）。
- `outbox_events` は投入先のキューに渡す値（`run_at` を含む）をすべて持ち、リレーがそのまま `JobQueue.Enqueue` に渡します。
- `trace_parent` はジョブを投入した span の W3C `traceparent` で、ジョブの実行の span はこれへの link を持ちます（[ADR-0063](../../../adr/0063-distributed-tracing.md) §2）。
  状態でなくジョブの属性で、記録していない span から投入したジョブは NULL です。リレー・dead job への移動・再実行はそのまま引き継ぎます。
- `jobs` は未完了のジョブだけを持ちます。完了したジョブは削除し、恒久的に失敗したジョブは `dead_jobs` に移します。
  そのため `jobs (idempotency_key)` の一意制約は「未完了のジョブの間での一意」を表します。
- `dead_jobs` は管理 API から再実行するまで残り、`failed_at` から設定 `dead_job_retention`（既定 30 日）を過ぎた行を 1 時間ごとの消去ジョブ `queue.purge_dead_jobs` が
  `DeadJobs.DeleteDeadJobsBefore` で古い順に、1 回 1,000 件・20 回まで消します（FR-803）。続きは次の回が消します。
- `job_attempts` は【イベント】で、成功しなかったジョブの試行（`retry` と `dead`）を 1 試行 1 行で記録します（[ADR-0064](../../../adr/0064-queue-job-dashboard.md)）。
  成功した試行は記録しないので、書き込みは失敗の数にだけ比例します。runner が `queue.AttemptLog` を通して、キューのアダプタによらずアプリの DB に書き、記録の失敗はジョブの結果を変えません。
  ペイロードとハンドラのログは持たず、`error` はハンドラが返したエラーの先頭 2,000 バイト（UTF-8 の境界で切る）で、ハンドラはエラーにメールアドレス・IP アドレス・トークン・本文を入れません。
  dead job の再実行は試行回数を数え直すので、同じ `attempt` が 2 回現れることがあり、`finished_at` を主キーに含めて区別します。同じ試行を繰り返し記録しても 1 行です。
  `finished_at` から設定 `job_attempt_retention`（既定 30 日）を過ぎた行は、`queue.purge_dead_jobs` が dead job と同じ上限で古い順に消します。
- ジョブの状態（待ち・実行中・リトライ待ち）は列を持たず、`jobs` の今の値から導きます（`locked_until > now` なら実行中、そうでなく `attempt = 0` なら待ち、`attempt > 0` ならリトライ待ち）。
- `kv_entries` は `KeyValueStore` ポートの値です。`expires_at` を過ぎた行は無いものとして扱い、1 時間ごとの消去ジョブ `kv.cleanup` が `DeleteExpired` で物理削除します。
  `DeleteExpired` は 1 回（1 トランザクション）で消す件数に上限を取り、ジョブは上限の回数まで繰り返して、残りを次の回に回します（FR-803）。
  カウンタ（`IncrBy`、レート制限）は値を 10 進の文字列で持ち、1 文の upsert で加算します。
  `CompareAndSwap` は、期待した値（または期限内の行が無いこと）のときだけ書き換えか削除をし、PostgreSQL では条件付きの 1 文、SQLite では `BEGIN IMMEDIATE` の中の読み取りと書き込みで行います。
- `kv = memory` と `kv = valkey` は `kv_entries` を使わず、同じポートの値をプロセスのメモリか Valkey に持ちます。
  `kv/valkey` は 1 件を値と期限のハッシュにし、TTL のある件をサーバーの TTL と期限順のソート済み集合にも入れて、`kv.cleanup` がサーバーの消し残しを消します。
- `locked_by` と `locked_until` はワーカーのリース（ADR-0004）で、取得時に両方を設定し、結果の記録時に両方を NULL に戻します（`CHECK` で同時性を保証）。
  リースが切れた（`locked_until <= now`）ジョブは他のワーカーが取り直します。結果の記録は `(id, locked_by, attempt)` が一致するときだけ有効です。
- `ordering_key` が同じジョブは 1 件ずつ、投入順（`(created_at, id)`）に実行します（`internal/app/queue` の「Ordering keys」）。
  取得は、同じキーの他のジョブがリース中のとき、または自分より前に投入された同じキーのジョブが `jobs` に残っている（遅延中・リトライ待ちを含む）ときは、そのジョブを飛ばします。
  この判定はインデックス `(ordering_key, run_at)` で同じキーの行を引きます。
  PostgreSQL では同じキーのジョブを同時に取得する 2 つのトランザクションが互いのリースを見落とさないよう、取得はキーのトランザクション内 advisory lock を取ってから候補を再確認します。
- `data_upgrade_runs` は【イベント】で、`my db upgrade` が完了した更新の手順（[ADR-0087](../../../adr/0087-schema-changes-after-launch.md) §2・§4）を 1 手順 1 行で記録します。
  手順は `internal/app/dbupgrade` の `Step` で、キー・説明・実行の関数（冪等で、小さなトランザクションの繰り返し）・作り直す投影の名前を持ち、`internal/platform/setup_dbupgrade.go` に登録します。
  手順の実行と投影の作り直しが終わってから記録するので、途中で失敗した手順は次の `my db upgrade` でもう一度実行します。同じ手順を 2 回記録しても最初の行が残ります。
  contract を含む版は前提とする手順のキーと、足りないときに通る版を持ち、`my db upgrade --check`（`schema apply` の前）が記録に無ければ終了コード 1 で止まります。
  表の無い DB は新しい DB として通し、この表の無い DB（スキーマがこの表より古い）はすべての前提が足りないものとして扱います。
  contract の後も手順は実行の関数を外して登録を残すので、新しい DB でも記録が揃い、readiness は登録したすべての手順の記録を求めます。
