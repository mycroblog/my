### 4.6 timeline

`timeline` モジュールは **自前の投影だけ** を読みます（ADR-0002 の例外は不要）。どれも `content` / `graph`（制限は `moderation`）のイベントを購読する同期 projector が更新します（§2.1）。

| テーブル | カラム | 制約 / 用途 | 元になるイベント |
| --- | --- | --- | --- |
| `feed_entries` | `id`（= 投稿の `status_id` またはブーストの `boost_id`）, `kind`（`enum(post, boost)`）, `status_id`, `author_account_id`（投稿者またはブーストした人）, `author_domain`, `original_author_account_id`（元投稿の作者。投稿なら `author_account_id` と同じ）, `original_author_domain`, `visibility`, `is_local`（投稿者またはブーストした人がローカルか）, `language`（`lang`。現在版の言語）, `revision_seq`（`int`。現在版の `seq`）, `is_reply`（`bool`。投稿が返信か。ブーストは常に偽） | PK `(id)`。インデックス `(author_account_id, id)`、`(status_id)`（版・削除での更新）、部分インデックス `(id) WHERE is_local AND kind = 'post' AND visibility IN ('public','local')`（ローカル）、`(id) WHERE kind = 'post' AND visibility = 'public'`（連合） | 投稿・版（言語）・削除・ブースト・ブースト取り消し |
| `feed_direct_recipients` | `entry_id`, `recipient_account_id` | PK `(recipient_account_id, entry_id)`、インデックス `(entry_id)`。`direct` 投稿の宛先（現在版のメンション） | 投稿・版・削除 |
| `feed_tag_entries` | `tag_id`, `entry_id`, `author_account_id`, `visibility`, `is_local`, `revised_at`（現在版の `revised_at`） | PK `(tag_id, entry_id)`、インデックス `(entry_id)`、部分インデックス `(tag_id, revised_at) WHERE visibility IN ('public','local','unlisted')`（`tag_usage.last_used_at`）。現在版のタグ。公開範囲は不変なので非正規化してよい（FR-203） | 投稿・版・削除 |
| `tag_usage` | `tag_id`, `statuses_count`（`bigint`、1 以上）, `last_used_at` | PK `(tag_id)`。タグごとの `public` / `local` / `unlisted` の投稿の数と、それらの `feed_tag_entries.revised_at` の最大。ハッシュタグの前方一致の検索（FR-119, FR-306, ADR-0092 §1） | 投稿・版・削除（`feed_tag_entries` と同じ更新） |
| `feed_follow_edges` | `follower_account_id`, `followee_account_id`, `show_boosts` | PK `(follower_account_id, followee_account_id)`。`active` のフォローだけ | フォロー（自動承認）・承認・終了、フォローごとの設定 |
| `feed_exclusions` | `viewer_account_id`, `target_account_id`, `reason`（`enum(block, blocked_by, mute)`）, `expires_at`（NULL 可。ミュートの期限） | PK `(viewer_account_id, target_account_id, reason)` | ブロック（両方向の 2 行）・ブロックの解除・ミュート・ミュートの解除 |
| `feed_domain_mutes` | `viewer_account_id`, `domain` | PK `(viewer_account_id, domain)`。サブドメインにも効く（FR-304） | ドメインのミュート・解除 |
| `feed_author_restrictions` | `account_id`, `restriction`（`enum(profile_view, visibility_ceiling_private, public_discovery)`）, `effective_until`（NULL 可） | PK `(account_id, restriction)`（複数の処分の和集合をまとめ、最も遅い期限を持つ） | 制限・ライフサイクル（凍結・削除済みは `profile_view` として表す。削除は最終なので、削除済み（`deleted` / `remote_deleted`）の `profile_view` は `effective_until` が NULL） |
| `feed_domain_restrictions` | `domain`, `restriction`（同上）, `effective_until`（NULL 可） | PK `(domain, restriction)`。サブドメインにも効く | ドメイン単位の制限 |

- ドメイン（`author_domain` / `original_author_domain`）はローカルなら `''`、リモートなら投稿・ブーストの URI のホストです。ドメイン単位の制限の照合に使います。
- ブーストの行は、元投稿の行から作者・ドメイン・言語・版を写します。元投稿の版が進むと、元投稿とそのブーストの行の `language` と `revision_seq` をまとめて更新します（`revision_seq` 以下の版は無視します）。
- 削除された投稿の行は、投稿・そのブースト・宛先・タグをまとめて消します。削除された投稿への遅れた版やブーストは反映しません。
- `tag_usage` は `feed_tag_entries` を置き換える・消すのと同じ更新で増減します（ADR-0092 §1）。`public` / `local` / `unlisted` の投稿のタグだけを数え、`private` / `direct` だけのタグは名前から投稿の存在が漏れるので行を持ちません。
  数が 0 になったタグは行を消します。`last_used_at` は、数えている投稿の `feed_tag_entries.revised_at` の最大を、更新のたびに部分インデックスから読み直します（編集・削除で外れたタグも、残る投稿の時刻に戻ります）。
  再構築は `feed_*` と同じく `timeline.RebuildProjections` が空にしてから作り直すので、増減の結果と一致します。アカウントの消去（§8）で消すエントリのタグも数から引きます。
  - ハッシュタグの前方一致の検索（`GET /tags/search`）は、`content` の `tags.name_normalized` の範囲（`>= q AND < q || U+10FFFF`、PostgreSQL は `COLLATE "C"`）を索引の順に最大 100 行読み、`tag_usage` に行のあるタグだけを、完全一致 → `statuses_count` の多い順 → `last_used_at` の新しい順 → 名前の順に並べます。`tags` は JOIN しません。
- `feed_entries` の挿入は、同じ `id` があれば失敗させます（投稿とブーストの ID は一度しか挿入されないので、projector の二重登録を検出できます）。
- `feed_entries` は元の公開範囲を持ちます。サイレンスによる上限（§5.4）は、読み取り時に `feed_author_restrictions` と合成して適用します。
  データを書き換えないので、解除すれば元に戻ります。
- `feed_author_restrictions` は、アカウントの削除（`account_lifecycle_events` の `deleted` / `remote_deleted`）で `profile_view` の行を持ちます。
- `feed_author_restrictions` / `feed_domain_restrictions` の処分による行は、`moderation` のイベント `moderation.account_restrictions_changed` / `moderation.domain_restrictions_changed` から作ります。
  イベントは処分と解除のたびに、その後に有効な制限（制限ごとに最も遅い期限）を載せるので、projector はそのアカウント・ドメインの 3 つの制限を、載っているものは書き、載っていないものは消します。
- 将来 fan-out-on-write に切り替える場合も、`home_feed`（閲覧者 × エントリ）をこのモジュールの投影として追加するだけで済みます（ADR-0013）。

#### リスト

フォロー中の人を、持ち主だけに見えるリストにまとめます（FR-211、[ADR-0122](../../../adr/0122-lists.md)）。リストの表は `timeline` が持ち、書くのも読むのも `timeline` だけです。
メンバーの条件（承認済みのフォロー）は自前の `feed_follow_edges` で確かめ、`graph` の公開 API は呼びません。

| テーブル | 分類 | カラム | 制約 / 用途 |
| --- | --- | --- | --- |
| `lists` | リソース | `id`, `owner_account_id`, `created_at` | PK `(id)`、インデックス `(owner_account_id, id)`（持ち主のリストの一覧と数） |
| `list_names` | 現在値 | `list_id`, `name`（書記素で 1〜50） | PK `(list_id)`。名前の変更は上書きし、履歴を持たない（ADR-0020 規約 5） |
| `list_members` | 現在値 | `list_id`, `member_account_id`, `owner_account_id`（リストの持ち主の写し。不変）, `added_at` | PK `(list_id, member_account_id)`、インデックス `(owner_account_id, member_account_id)`（フォローの終了で外す・あるアカウントが入っているリスト）、`(member_account_id)`（消去） |

- 持ち主のコマンドが書きます。メンバーに足せるのは、持ち主の `feed_follow_edges` に行がある（`active` のフォローの）相手だけです。承認待ちの相手は足せません。
- 上限は 1 人 50 リスト・1 リスト 1,000 メンバーで、超える作成・追加は断ります。メンバーの数は挿入の後に数えるので、入っている相手を足し直しても上限で断られません。
- リストの削除は、名前とメンバーの行を同じトランザクションで消します。
- `graph.follow_terminated`（`active` だったフォローの終了。理由を問わない）の projector は、`feed_follow_edges` の行を消すのと同じ更新で、`list_members` から持ち主 = フォローした人・メンバー = フォローされた人の行を消します。フォローし直しても戻しません。移転でフォローが移るときも、移転元のフォローの終了として外れます。
- ブロック・ミュートはメンバーのまま残し、リストのタイムラインの読み取りで除きます。
- リストのタイムラインは新しい投影を作らず、`feed_entries` を `list_members` で絞って読みます（§6、ADR-0013）。
- リストは投影ではないので、`timeline.RebuildProjections` は空にしません。再構築した `feed_follow_edges` に行のないメンバーだけを消します（§9）。
- アカウントの消去（§8）は、本人のリスト（名前とメンバーを含む）と、ほかの人のリストの本人のメンバーの行を消します。
