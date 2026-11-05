### 4.11 webhook

サーバー単位の webhook（FR-722、[ADR-0060](../../../adr/0060-instance-webhooks.md)）。`internal/app/webhook` が所有します。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `webhook_endpoints` | リソース | `id`, `created_by_user_id`（NULL 可 = CLI）, `created_at` | |
| `webhook_endpoint_revisions` | イベント（版） | `id`, `endpoint_id`, `url`（`string(2048)`）, `description`（`string(200)`。NULL 可 = なし）, `revised_by_user_id`（NULL 可 = CLI）, `revised_at` | インデックス `(endpoint_id, id DESC)` |
| `webhook_endpoint_revision_events` | イベント（版の子） | `revision_id`, `event_type`（`enum`。購読できるイベント） | PK `(revision_id, event_type)`。インデックス `(event_type)` |
| `webhook_secrets` | イベント | `id`, `endpoint_id`, `sealed_secret`（`bytes`。`kernel.Sealer` で封をした 32 バイト）, `created_by_user_id`（NULL 可）, `created_at` | インデックス `(endpoint_id, id DESC)` |
| `webhook_endpoint_state_changes` | イベント | `id`, `endpoint_id`, `state`（`enum(enabled, disabled, deleted)`）, `reason`（`enum(admin, gone, failing)`）, `changed_by_user_id`（NULL 可。`reason = admin` のときだけ値を持てる）, `changed_at` | インデックス `(endpoint_id, id DESC)` |
| `webhook_messages` | リソース | `id`（= `webhook-id`）, `event_type`（`enum`。購読できるイベントと `webhook.test`）, `body`（`bytes`。送る本文そのもの）, `created_at` | インデックス `(created_at)` |
| `webhook_deliveries` | リソース | `id`, `message_id`, `endpoint_id`, `created_at` | 一意 `(message_id, endpoint_id)`。インデックス `(endpoint_id, id DESC)` |
| `webhook_delivery_attempts` | イベント | `id`, `delivery_id`, `outcome`（`enum(succeeded, http_error, timeout, connection_error, forbidden_destination)`）, `status_code`（NULL 可。`succeeded` と `http_error` のときだけ。`CHECK`。100〜599 の外の答えは HTTP の答えとして扱わず `connection_error` で記録する）, `duration_ms`, `attempted_at` | インデックス `(delivery_id, id)` |
| `webhook_endpoint_heads` | 投影 | `endpoint_id`（PK）, `revision_id`, `state`, `secret_id` | インデックス `(state)` |
| `webhook_endpoint_failures` | 投影 | `endpoint_id`（PK）, `first_failure_attempt_id`, `first_failed_at`, `consecutive_failures` | 行がある = 最後の成功の後に失敗が続いている |

- 宛先の版・秘密・状態の変化は、どれも `webhook.endpoint_changed` を発行し、その射影が `webhook_endpoint_heads` を書きます。
  試行は `webhook.attempt_recorded` を発行し、射影が失敗なら `webhook_endpoint_failures` の連続回数を足し、成功なら行を消します。
  `first_failed_at` は最初の失敗の試行の時刻の写しで、その試行の行が 14 日で消えたあとも自動の無効化の判定（72 時間と 20 回）に使えるように持ちます。
- 他のモジュールのドメインイベント（通報・登録・アカウント・処分・異議・ドメインの処分・公開投稿・カスタム絵文字）は、そのトランザクションの中で `webhook` の購読者が受け、
  購読している有効な宛先があるときだけ `webhook_messages` と宛先ごとの `webhook_deliveries` を書いて、`webhook.deliver` ジョブを Outbox で投入します（冪等キーは配送の ID）。
  本文の `data` は、同じ対象に管理 API が返す表現です。`webhook` の購読者はイベントのトランザクションを渡して、クライアント API が管理 API の表現を組み立てる関数
  （`webhook.Presenter`）を呼びます。その読み取りは各モジュールの `uow.Tx` を受け取る読み取り（`...Tx`）を通るので、イベントのトランザクションが書いた行を見ます。
  宛先が無ければ何も書きません。`body` は送る JSON をそのまま持つ「解釈しない文書」（規約 4）です。
- 削除は `state = deleted` の状態の変化で、行は消しません（監査ログと配送の履歴が指すため）。削除した宛先は一覧に出ず、何も受け取りません。
- `webhook_messages` と、その配送・試行は、作成から 14 日（固定）で 1 時間ごとの消去ジョブ `webhook.purge_messages` が消します（FR-803）。本文は通報のコメントなどの個人データを含みます。
- `url` は Discord や Slack のように経路やクエリに秘密を含むことがあるので、ログと監査ログ（`target_label`）にはホスト名と説明だけを出します。
