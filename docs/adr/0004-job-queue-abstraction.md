# ADR-0004: ジョブキューとスケジューラの抽象化

- Status: Accepted（Cloud Tasks・SQS のアダプタの形、push の受け口とその認証、Scheduler の外部トリガーを ADR-0120（外部のジョブキューは DB のジョブを起こす合図だけを運ぶ）が決める。SQS の常駐 worker の pull は作らない）
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #4

## Context

連合配送・Inbox 処理・メディア処理・通知・クリーンアップなど、非同期処理が多い。
キューは各クラウドのマネージドサービスに切り替えたい（G3）が、サービスごとに **配信モデル** が異なる:

| サービス | 配信モデル | 遅延上限 | 備考 |
| --- | --- | --- | --- |
| DB キュー（自前） | pull（ポーリング） | 無制限 | PG は `FOR UPDATE SKIP LOCKED`、SQLite は単一ワーカー前提 |
| Amazon SQS | pull（Lambda event source なら push 的） | 15 分 | 可視性タイムアウト（最大 12 時間）、メッセージ最大 1 MiB、保持最大 14 日 |
| Google Cloud Tasks | push（HTTP ターゲット） | 30 日 | タスク名で重複排除（削除後最大 24 時間）、レート制御（キューあたり最大 500 件/秒）、タスク最大 1 MiB、dispatch deadline 15 秒〜30 分 |
| Google Pub/Sub | push / pull | — | 遅延配信なし（要検証: 本調査の対象外） |

料金は Cloud Tasks が月 100 万オペレーションまで無料（以降 $0.40 / 100 万）、SQS が月 100 万リクエストまで無料。配送のリトライも 1 オペレーション / 1 リクエストとして数える。

SQS と Cloud Tasks の値は 2026-10-16 時点の公式ドキュメントによる（出典は [deployment-targets.md](../architecture/deployment-targets.md)）。

## Decision Drivers

- 常駐ミドルウェアなしで動くこと（NFR-COST-2）
- push / pull どちらのモデルでも同じジョブハンドラが動くこと
- at-least-once・冪等性・長期バックオフ（NFR-REL-1, 2）

## Decision

### ジョブモデル

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

### ポート

- `JobQueue.enqueue(job | job[])` — 投入（遅延・冪等キー付き）
- `JobHandlerRegistry` — `type` → ハンドラ。ハンドラは `(ctx, job) => Result<Done | Retry(after) | Fail>` の純粋なアプリケーション関数
- `JobRunner` — アダプタ側。2 種類の実装形態を持つ:
  - **pull runner**: ループでデキュー → ハンドラ実行 → ack / 再投入（DB キュー、SQS 常駐ワーカー）
  - **push receiver**: プラットフォームから渡されたメッセージを処理（Lambda SQS イベント、Cloud Tasks の HTTP ターゲット `/_internal/jobs`。OIDC 等で認証）
- `Scheduler` — 定期ジョブの起動トリガー（Cloud Scheduler / EventBridge Scheduler / Kubernetes CronJob / プロセス内 cron）。
  トリガーは「該当ジョブを enqueue する」だけにし、実処理は JobQueue に流す。

### セマンティクス

- **at-least-once**。ハンドラは冪等（Activity ID・冪等キーで重複排除）。
- リトライ: 指数バックオフ + ジッタ。配送は最大数日（例: 1m, 5m, 30m, 2h, 12h, 24h ×3）。
- **遅延上限を超えるバックオフは「再投入チェーン」**: 上限まで遅延 → 起動時に `runAt` 未到来ならそのまま再遅延。
- 最大試行回数超過は `dead_jobs` テーブル（DB）へ記録し、管理画面 / CLI から再実行できる（DLQ を持つサービスでも DB に寄せる）。
- 連合の配送・受信処理も独自キューを持たず、本ポートに一本化する。

### v0 で提供するアダプタ

`queue/db`（既定）、`queue/inline`（テスト・開発用の同期実行）、`queue/sqs`、`queue/cloudtasks`。
Redis / NATS / RabbitMQ 等はコミュニティアダプタとして受け入れ可能にする。

## Consequences

- どのターゲットでもハンドラは同一コードで、エントリポイントだけが異なる。
- 遅延上限やバッチサイズの差はアダプタが吸収するが、厳密な実行時刻は保証しない（「早くとも runAt 以降」）。
- Transactional Outbox（ADR-0002）と組み合わせ、enqueue 失敗時も outbox から再投入できるようにする。

## Open Questions / 要検証

- [ ] Cloud Tasks の HTTP ターゲット認証（OIDC トークン）と Cloud Run の実機での疎通。公式ドキュメント上は、ID トークンを付与し、サービスアカウントに Cloud Run Invoker ロールを与える構成で可能
- [ ] SQLite DB キューで web と worker を同一プロセスにした時のロック競合
