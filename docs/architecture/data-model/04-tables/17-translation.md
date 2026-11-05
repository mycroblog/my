### 4.17 translation

投稿の翻訳のキャッシュと、提供元に送った文字数（[ADR-0148](../../../adr/0148-post-translation.md) §9・§11）。`internal/app/translation` が所有します。
翻訳は版から作った派生物で、消してよく、提供元の出力は決定的でないので投影の再構築の保証を満たしません。[§9](../09-projection-rebuild.md) の再構築の表には載りません。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `status_translations` | 現在値（キャッシュ） | `id`, `status_id`, `revision_id`, `target_language`（`lang`）, `provider`（`string`。提供元を足してもスキーマが変わらないよう CHECK を付けない）, `source_language`（`lang`。提供元が検出した言語）, `content_html`（`text`。サニタイズ済み）, `spoiler_text`（`text`）, `created_at` | UNIQUE `(revision_id, target_language, provider)`、INDEX `(status_id)`, `(created_at)` |
| `status_translation_attachments` | 同上（子） | `translation_id`, `position`（版の `status_revision_attachments.position`）, `description`（`text`） | PK `(translation_id, position)`。代替テキストのある添付だけ |
| `status_translation_poll_options` | 同上（子） | `translation_id`, `position`（版の投票の選択肢の位置）, `title`（`text`） | PK `(translation_id, position)` |
| `translation_account_usage` | 現在値 | `account_id`, `month`（`string`。UTC の `YYYY-MM`）, `characters`（`bigint`） | PK `(account_id, month)`、INDEX `(month)` |
| `translation_server_usage` | 現在値 | `month`（`string`。UTC の `YYYY-MM`）, `characters`（`bigint`） | PK `month` |

- キーが版なので、編集すれば新しい版になり、古い翻訳には当たりません。子の行も版の位置を指します。
- 同じ投稿を読める人の間で行を共有します。サービスは行を返す前に必ず `content.CanView` を通します（`content.Reader.Readable`）。
- 行の保存は `translation.Service` のトランザクションで、投稿の削除を待たせるロック（`content.Reader.HoldLive`）を取り、削除されていないことを確かめてから書きます。投稿の削除が先なら保存は失敗し、保存が先なら削除の投影が行を消します。提供元の呼び出しはトランザクションの外です。同じ翻訳の呼び出しは KV のロック（`translation.lock`、`CompareAndSwap`、2 分）で 1 回にし、ロックが失われたときは一意制約が 2 つ目を捨てます。
- `content.StatusDeleted` を受ける投影が、削除と同じトランザクションで投稿の翻訳を消します。削除のすべての経路（作者・モデレーション・リモートの `Delete`・自動削除・リモートの投稿の保持期間・リレーの投稿の保持期間・アカウントの消去）がこのイベントを発行します。通報や処分の証拠として行が残る投稿でも、翻訳は消します。作者の凍結では消さず、閲覧の判定が止めます。
- 無効にした提供元や許可を外した提供元の行は返しません。行は保持期間まで残るので、有効に戻せばまた返します。
- 1 日 1 回のジョブ `translation.purge_cache` が、作成から 30 日を過ぎた行を子の行と一緒に消します。読んでも延ばしません。利用量の行は翌々月に消します（今月と先月の行だけを残します）。
- アカウントの消去は `translation_account_usage` の行を消します（`translation.Eraser`）。`translation_server_usage` にアカウントの情報はないので残します。
