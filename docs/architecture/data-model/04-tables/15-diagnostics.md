### 4.15 diagnostics

報告されたエラーを指紋ごとにまとめたエラーのグループ（[ADR-0103](../../../adr/0103-error-reporting.md) §4 (2)・§7）。`internal/app/diagnostics` が所有します。
行に入るのは `ErrorReporter` が除去（ADR-0103 §3）を済ませた値だけで、アカウント・ユーザー名・IP アドレス・リクエストの本文を持ちません。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `error_groups` | リソース | `id`, `fingerprint`（`string(32)`。ADR-0103 §1 の指紋）, `source`（`enum(server, web)`）, `kind`（`enum(panic, error)`）, `error_type`（型の連鎖の最後の型）, `message`（除去した `message` の最初の行。200 文字まで）, `where_name`（起きた場所の有限の名前）, `first_version`（最初の 1 件の `service.version`）, `unauthenticated`（`bool`。最初の 1 件がログインしていない Web クライアントの報告）, `first_seen_at` | 一意 `(fingerprint)` |
| `error_group_counts` | 現在値 | `group_id`, `hour_start`（`timestamp`。UTC の 1 時間の始まり）, `occurrences`（`bigint`）, `last_seen_at`, `last_version` | PK `(group_id, hour_start)`。`occurrences > 0`。インデックス `(hour_start)`（古い行の削除） |
| `error_group_samples` | 現在値 | `group_id`（PK）, `error_type`（型の連鎖）, `message`（除去した全文）, `stack`（`text`。除去したスタック）, `request_id`・`trace_id`（なしは `''`）, `version`, `role`, `occurred_at` | 最新の 1 件だけ |
| `error_group_events` | イベント | `id`, `group_id`, `kind`（`enum(resolved, regressed, muted, unmuted)`）, `actor_kind`（`enum(user, cli, system)`）, `actor_user_id`（NULL 可 = 利用者でない）, `version`（`resolved` は解決にしたときの最後の 1 件の版、`regressed` は再発の版、ほかは `''`）, `occurred_at` | `(actor_kind = user) = (actor_user_id IS NOT NULL)`。`(kind = regressed) = (actor_kind = system)`。インデックス `(group_id, id DESC)` |
| `error_group_confirmations` | イベント | `group_id`（PK）, `confirmed_at` | `unauthenticated` のグループに、ログインした報告が初めて届いたこと。グループごとに 1 行 |
| `error_group_states` | 投影 | `group_id`（PK）, `state`（`enum(open, resolved, muted)`）, `resolved_version`（`resolved` のときの `error_group_events.version`、ほかは `''`）, `since`（今の状態になった時刻） | `state = resolved` でなければ `resolved_version = ''` |

- 書き込みは `ErrorReporter` の出力の 1 つ `diagnostics.Recorder` が行います。プロセスの中で指紋と版の組の最初の 1 件は、`Recorder` のゴルーチンがすぐに書きます（`Service.RecordFirst`）:
  - 短いトランザクションで `error_groups` に挿入します（指紋の一意の衝突は何もしない）。挿入したときは `diagnostics.error_group_created` を発行し、射影が `error_group_states` を `open` で作ります。
  - 挿入しなかったとき（既にあるグループ）は状態の行を読み（PostgreSQL は `FOR UPDATE`）、`resolved` で `resolved_version` と版が異なれば `regressed` のイベントを足して `diagnostics.error_group_regressed` を発行し、射影が `open` に戻します。
    同じ版での再発は数えるだけです（`diagnostics.Regresses`）。`muted` のグループは再発しません。
  - `unauthenticated` のグループにログインした報告が届いたときは、同じトランザクションで `error_group_confirmations` に挿入し（主キーの衝突は何もしない）、挿入したときだけ `diagnostics.error_group_confirmed` を発行します。
    ログインしていない呼び出しは新しいグループの通知を起こさない（ADR-0103 §6）ので、通知はログインした報告が作ったグループの `diagnostics.error_group_created` か、`diagnostics.error_group_confirmed` で行います（[§4.7](07-notification.md) の `admin_error_group`。webhook の `error_group.*` も同じ規則）。
    `diagnostics` の射影は `notification` と `webhook` の購読者より先に登録し、購読者が同じトランザクションで `error_group_states` を読めるようにします。
    プロセスの中の「最初の 1 件」は指紋・版・ログインの有無の組ごとです。
  - そのあと、トランザクションの外の 1 文ずつで `error_group_counts` に 1 を足し、`error_group_samples` をその 1 件にします。
- 2 件目からはプロセスの中で指紋と 1 時間ごとに数を貯め、60 秒ごととプロセスの終わり（`App.Close`）に、時間ごとの数の加算と最新の 1 件の置き換えを、それぞれトランザクションの外の 1 文の upsert で書きます。
  加算は `occurrences = occurrences + excluded.occurrences` なので、複数のプロセスの書き込みが重なっても 1 件を 1 回だけ数えます。標本は `occurred_at` が新しいときだけ置き換えます。
  どちらも指紋から `error_groups` を引いて書くので、グループの行がない指紋は何も書きません。
- 書き込みは報告の ctx の取り消しを外し（`context.WithoutCancel`。リクエスト ID などの値は残る）、書き込みごとに 5 秒の期限を付けます。応答の後の報告や、止まりかけたゴルーチンの panic の報告も書きます。
- 書けなかった数と標本は捨てます（正確な件数はメトリクスとログが持ちます）。最初の 1 件が書けなかったときは、同じ指紋と版の次の 1 件をもう一度すぐに書きます。指紋のない報告はまとめません。
- `resolve`・`mute`・`unmute` は、状態の行を読んで `diagnostics.Next` で次の状態を決め、変わるときだけイベントを足して `diagnostics.error_group_state_changed` を発行します（射影が新しい状態を書く）。
  変わる組み合わせは、`resolved` が `open`・`muted` から、`muted` が `open`・`resolved` から、`unmuted` が `muted` から `open` へ、`regressed` が `resolved` から `open` へ、です。
  `resolved` の `version` は、そのときの `error_group_samples.version`（なければ `first_version`）です。
- 定期のジョブ `diagnostics.prune_error_counts`（24 時間ごと）が、始まりが 90 日より前の時間の `error_group_counts` を 1 文で消します。
- `error_group_states` は各グループの最も新しいイベントから作り直せます: `resolved`・`muted` はそのまま、`unmuted`・`regressed` とイベントのないグループは `open` です（§9）。
