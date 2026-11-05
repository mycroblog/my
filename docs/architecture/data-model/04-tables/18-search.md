### 4.18 search

全文検索の索引の再構築の進み具合（[ADR-0149](../../../adr/0149-full-text-search.md) §10）。`internal/app/search` が所有します。索引そのものは DB の外の検索エンジンにあり、`search = none` のサーバーはこの表に何も書きません。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `search_rebuild` | 現在値 | `id`（`integer`。常に 1）, `rebuild_id`, `mode`（`enum`: `clear` / `keep`）, `phase`（`enum`: `clearing` / `filling` / `finished`）, `after_status_id`（`string`。カーソル。最初は空）, `rate`（`int`）, `processed`, `failed`（`bigint`）, `last_error`（`text`）, `started_at`, `updated_at` | PK `id`、CHECK `id = 1` |
| `search_rebuild_failures` | 同上（子） | `rebuild_id`, `status_id`, `message`（`text`）, `failed_at` | PK `(rebuild_id, status_id)` |

- 行は最新の再構築の 1 行だけです。進み具合に履歴としての意味はないので、次の再構築が置き換え、`my search clear` が消します（規約 5）。
- `my search rebuild` は、終わっていない再構築がなければ行を書き、あれば同じ `mode` のときだけ続きから再開します。`mode = clear` の全消去はコマンドが行い、終えると `phase` を `filling` に進めるので、再開でもう一度消しません。
- ジョブ `search.rebuild` は再構築の ID とカーソルを持ち、1 回で 100 件を同期して、次のジョブと一緒に `phase` と `after_status_id` を同じトランザクションで進めます。更新は読んだ位置からだけ行い（`phase` と `after_status_id` の一致）、位置が違うジョブは何もせず終わるので、同じジョブが 2 回走っても鎖は分かれません。
- `search_rebuild_failures` は投稿の ID とエラーだけを持ち、投稿の文字列を持ちません。約 100 件で書くのをやめ、`failed` は数え続けます。
