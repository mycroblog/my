### 4.8 federation

設計は [連合の設計](../../federation.md) にあります。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `received_activities` | イベント | `activity_uri`（PK）, `actor_uri`, `type`, `signature_scheme`（`enum(rfc9421, cavage)`）, `trust`（`enum(signed, refetch)`。署名者と `actor` が一致しない、または `id` のホストが `actor` のホストと異なれば `refetch`）, `received_at` | インデックス `(received_at)`（保持期間後に削除） |
| `received_activity_documents` | イベント | `activity_uri`（PK）, `document`（`document`） | 原本。非同期の処理が読む。障害調査とフィクスチャ採取のため短期保持（#44） |
| `remote_object_tombstones` | イベント | `uri`（PK）, `actor_uri`, `activity_uri`, `received_at` | 未知のオブジェクトへの `Delete` と、未知の対象への `Undo`。`uri` は `actor_uri` と同じオリジンのものだけ（[federation §3.3](../../federation.md#33-順序と冪等性)）。後から届いた `Create` / `Update` / `Follow` などを捨てる。インデックス `(received_at)`（30 日で削除）、`(activity_uri)`（`remote_follow_tombstones` を使い終えたかの判定） |
| `remote_follow_tombstones` | イベント | `activity_uri`（PK。`Undo`）, `actor_uri`（フォローした人）, `object_uri`（埋め込みの `Follow` の `object`）, `received_at` | 未知の `Follow` への `Undo` を、埋め込みの `Follow` の 2 人の組で記録する。後から届いた同じ組の最初の `Follow` を捨て、その URI を同じ `activity_uri` で `remote_object_tombstones` に記録する（その行があれば使い終えた tombstone）。インデックス `(actor_uri, object_uri, received_at)`、`(received_at)`（30 日で削除） |
| `outbound_activities` | イベント | `id`, `activity_uri`, `type`, `actor_account_id`, `object_type`（`enum(status, boost, favourite, follow, block, account, report, poll_vote, relay, quote, refusal, move)`）, `object_id`（`favourite` と `poll_vote` は投稿の ID、`move` は `account_moves` の ID）, `created_at` | 一意 `(activity_uri)`。インデックス `(object_type, object_id, id)` |
| `outbound_activity_documents` | イベント | `outbound_activity_id`（PK）, `document`（`document`）, `built_at` | 配送する本文。展開ジョブが 1 回だけ組み立て（投稿の `Delete` と `Like` / `Undo Like` は記録のトランザクションで組み立てる）、すべての配送とリトライが同じ本文を送る。保持期間（30 日）後に削除 |
| `deliveries` | イベント | `id`, `outbound_activity_id`, `inbox_url`（shared inbox に集約済み）, `domain`, `created_at` | 一意 `(outbound_activity_id, inbox_url)` |
| `delivery_attempts` | イベント | `id`, `delivery_id`, `outcome`（`enum(succeeded, retryable_failure, permanent_failure, skipped)`）, `signature_scheme`（`enum(rfc9421, cavage)`、NULL 可 = 応答が無い）, `http_status`（`int`、NULL 可）, `error`（`text`、NULL 可）, `attempted_at` | インデックス `(delivery_id, id)`。保持期間（30 日）後に削除（NFR-OPS-3） |
| `outbound_fanout_completions` | イベント | `outbound_activity_id`（PK）, `completed_at` | 展開が最後のバッチを積んだ（送らずに終えた展開を含む）。ローカルの Actor の `Delete` と `Move` だけ記録する（サービスのクローズが待ち、点検が数える。[federation.md §15](../../federation.md#15-サービスの終了)） |
| `closure_deletes` | イベント | `account_id`（PK）, `outbound_activity_id`（一意）, `recorded_at` | サービスのクローズの `Delete`（Actor）。アカウントごとに 1 行で、ページの再実行でも増えない（[ADR-0088](../../../adr/0088-service-shutdown-modes.md) §3） |
| `remote_instances` | リソース | `domain`（PK）, `first_seen_at` | |
| `instance_nodeinfo_observations` | イベント | `id`, `domain`, `software_name`, `software_version`, `observed_at` | インデックス `(domain, id DESC)` |
| `instance_availability_events` | イベント | `id`, `domain`, `kind`（`enum(marked_unavailable, marked_available)`）, `reason`（`enum(failure_days, delivery_succeeded, inbound_request, probe_succeeded, operator)`）, `occurred_at` | インデックス `(domain, id DESC)`。配送先として扱うかの判断の記録（管理画面で理由を示す） |
| `instance_health` | 現在値 | `domain`（PK）, `state`（`enum(available, unavailable)`）, `since`, `failure_days`（`int`。直近の連続した失敗の、異なる UTC の日の数）, `last_failure_day`（`string(8)`、`YYYYMMDD`。失敗が無ければ空文字） | インデックス `(state)`。`delivery_attempts` と `instance_availability_events` から更新する。成功時の更新は、`failure_days > 0` か `state = unavailable` のときだけ行う |
| `relays` | リソース | `id`, `protocol`（`enum(mastodon, litepub)`）, `url`（登録した URL。`mastodon` は inbox、`litepub` は Actor）, `inbox_url`（配送先）, `receive`（`bool`）, `created_by_user_id`（NULL 可 = CLI）, `created_at` | 一意 `(url)`。`left` か `rejected` のときだけ、子の行と一緒に消せる（[ADR-0061](../../../adr/0061-federation-relays.md)） |
| `relay_events` | イベント | `id`, `relay_id`, `kind`（`enum(follow_sent, accepted, rejected, left)`）, `follow_activity_uri`（`follow_sent` が送った `Follow` の `id`。ほかは対象の `Follow`）, `relay_actor_uri`（`accepted` の主体。ほかは NULL）, `actor_user_id`（NULL 可 = CLI か受信）, `occurred_at` | 一意 `(relay_id, follow_activity_uri, kind)`（同じ答えの再処理で増えない）。インデックス `(relay_id, id DESC)`。CHECK `(kind = 'accepted') = (relay_actor_uri IS NOT NULL)` |
| `relay_states` | 投影 | `relay_id`（PK）, `state`（`enum(pending, accepted, rejected, left)`）, `follow_activity_uri`, `relay_actor_uri`（NULL 可）, `since` | `relay_events` の最新の行から作る。インデックス `(follow_activity_uri)`（`Accept` / `Reject` の照合）、`(relay_actor_uri)`（受信の署名者の照合） |

- **配送先の記録** は `deliveries` が兼ねます。
  - 投稿の `Update` / `Delete` は、その投稿の `Create` の `outbound_activities` に紐づく `deliveries` の inbox に送ります（§7）。
- `relay` の活動はリレーの参加と脱退の `Follow` / `Undo` と、インスタンス Actor への `Follow` に答える `Accept` / `Reject` です（[federation §13](../../federation.md#13-リレー)）。主体はインスタンス Actor、`object_id` はリレーの ID（リレーでない相手の `Follow` への `Reject` は相手のアカウントの ID）です。
  本文は `relay_events` を書くトランザクションで組み立てて保存し、宛先の inbox は展開ジョブのペイロードが持ちます（登録を消した後でも届く）。
  リレーの状態の遷移（`follow_sent` は状態が無いか `rejected` / `left` から `pending`、`accepted` は同じ `Follow` の `pending` から、`rejected` は同じ `Follow` の `pending` / `accepted` から、`left` は同じ `Follow` の `pending` / `accepted` から）は `federation.RelayStatus.Next` が決め、行を読んでから書くまで `relays` の行ロックを取ります（PostgreSQL は `SELECT ... FOR UPDATE`、SQLite は書き込みが直列）。
- `report` の活動は通報の転送の `Flag` です。主体（`actor_account_id`）はインスタンス Actor、`object_id` は通報（`reports`）の ID で、`forwarded` の通報だけが持ちます（[federation §11](../../federation.md#11-モデレーションとの連携)）。
- お気に入りは現在値で、取り消すと行が消えます。そこで `favourite` の活動は、お気に入りを決める組（`actor_account_id` と投稿）で記録し、`object_id` は投稿の ID です。
  `Like` / `Undo Like` の本文はイベントの値（お気に入りの `uri`・投稿の `uri`）から記録のトランザクションで組み立てるので、行が消えた後の `Undo Like` も `Like` の `id` を埋め込めます。
- `instance_health` は【現在値】です。`delivery_attempts` と受信・probe の成功から更新しますが、成功は間引いて記録しないので、イベントから作り直せません（§9）。
- `instance_health` は、`retryable_failure` を記録した日が `last_failure_day` と違えば `failure_days` を 1 増やし、7 に達したら `instance_availability_events`（`marked_unavailable`）を記録して `unavailable` にします。
  成功・署名付きの受信・probe の成功は `failure_days` を 0 に戻し、`unavailable` なら `marked_available` を記録します（[federation.md §7.3](../../federation.md#73-配送とリトライ)）。
  `permanent_failure` と `skipped` は `instance_health` を変えません。projector は同じドメインへの同時の更新を失わないよう、行を読んでから書くまで行ロックを取ります（PostgreSQL は `SELECT ... FOR UPDATE`、SQLite は書き込みが直列）。
- `delivery_attempts.signature_scheme` が NULL の行は、応答を得ていない試行です（送らなかった `skipped` とサーキットブレーカーが開いていた `retryable_failure`、接続・TLS・タイムアウトの失敗、`HTTPClient` が拒否した宛先）。
- 受信の記録（`received_activities`）は 30 日、原本は 7 日で削除します（[ADR-0034](../../../adr/0034-remote-content-scope-and-retention.md)、FR-803）。
  1 時間ごとの消去ジョブ `federation.purge` が、原本、受信の記録、`remote_follow_tombstones`（30 日）、`remote_object_tombstones`（30 日）、`outbound_activity_documents`（30 日）、`delivery_attempts`（30 日）の順に、1 文あたり 1,000 行・1 テーブルあたり 20 文まで、古い順に消します。続きは次の回が消します。
  受信の記録は原本が消えたものだけを消すので、原本が受信の記録より長く残ることはありません。`outbound_activity_documents` と `delivery_attempts` の走査は、主キーの UUIDv7 の範囲（期限の下限より前）で止まります。
  `remote_follow_tombstones` を `remote_object_tombstones` より先に消すので、使い終えた tombstone が、使い終えた印（同じ `activity_uri` の `remote_object_tombstones`。tombstone より新しい）より長く残って `Follow` をもう一度捨てることはありません。
