### 4.5 graph

#### フォロー

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `follows` | リソース | `id`, `uri`（`Follow` の IRI。ローカルのフォローは `https://{host}/ap/activities/{id}`、ADR-0018）, `follower_account_id`, `followee_account_id`, `requested_at` | 一意 `(uri)`。フォローし直すと新しい行になる（過去のフォロー関係が残る） |
| `follow_acceptances` | イベント | `follow_id`（PK）, `accepted_by`（`enum(auto, followee, remote)`）, `activity_uri`（NULL 可。受信した `Accept`）, `accepted_at` | |
| `follow_rejections` | イベント | `follow_id`（PK）, `rejected_by`（`enum(followee, remote, system)`）, `activity_uri`（NULL 可）, `rejected_at` | |
| `follow_terminations` | イベント | `follow_id`（PK）, `reason`（`enum(unfollowed, removed_by_followee, blocked, account_deleted, moderation, moved)`）, `terminated_at` | 承認前の取り下げ（`Undo Follow`）も `unfollowed`。`moved` はフォローしていた相手の移転で、移転先へのフォローに置き換えたもの（ADR-0089） |
| `follow_resends` | イベント | `uri`（PK。もう一度届いた `Follow` の `id`）, `follow_id`, `received_at` | 承認済みのフォローに、別の `id` の `Follow` がもう一度届いたこと。`follows` の行は増やさない |
| `follow_edges` | 投影 | `follower_account_id`, `followee_account_id`, `follow_id`, `state`（`enum(pending, active)`）, `since`（`pending` は申請、`active` は承認の日時） | PK `(follower_account_id, followee_account_id)`。インデックス `(followee_account_id, state, follow_id)`（フォロワー・フォローリクエストの一覧・配送先の展開）、`(follower_account_id, state, follow_id)`（フォロー中の一覧）。一覧は `follow_id` のキーセットで新しい順に引く。拒否・終了で行を消す |
| `follow_preferences` | 現在値 | `follow_id`（PK）, `show_boosts` | フォローの作成時に既定値（`show_boosts = true`）で作る。新しい投稿の通知はフォローの設定ではなく、`notification` の購読（§4.7）です |
| `follow_counters` | 投影 | `account_id`（PK）, `followers_count`, `following_count`（`bigint`、0 以上） | `active` のフォローだけを数える。両方 0 になったら行を消す（行が無い = 0） |

- 鍵アカウントへのフォロー（FR-302）は `follows` を作り、承認待ちの間は `follow_edges.state = pending` です。
  鍵アカウントかどうかは、フォローされる側の現在のプロフィール版の `locked` で決め、`account` のアプリケーション層の公開 API から読みます。
  鍵アカウントでなければ、同じトランザクションで `follow_acceptances`（`accepted_by = auto`）も記録します。
- 拒否は承認待ちのフォローだけ、フォロワーの削除（`removed_by_followee`）は `active` のフォローだけが対象です。
- `follow_edges` の行があるペアへのフォローは何もしません。同時に 2 つのフォローが作られた場合は、投影の主キーで後の方を再実行させ、既存のフォローを返します。
- リモートへのフォローは、相手が鍵アカウントでなくても承認待ち（`graph.follow_requested`）で始まり、`Accept` を受け取ったら `follow_acceptances`（`accepted_by = remote`）を記録して投影を `active` にします。
- リモートからのフォロー（`graph.ReceiveFollow`）の `uri` は受け取った `Follow` の `id` で、同じ `uri` の `Follow` をもう一度受け取っても何もしません。
  承認済みのフォローがあるペアから別の `id` の `Follow` が届いたら、相手のサーバーはフォローが成立していないとみなしています（Mastodon は `Accept` が届くまで `Follow` を送り直す）。
  `follow_resends` にその `id` を記録して `graph.follow_resent` を発行し、その `Follow` への `Accept` を送り直します。承認待ちのペアへの `Follow` は何もしません。
  どちらかがブロックしていれば、`follows` を作って同じトランザクションで `follow_rejections`（`rejected_by = system`）を記録し、相手に `Reject` を送ります。
- アカウントの移転（`account.moved`、ADR-0089）は、ジョブ `graph.migrate_follows`（冪等キーは移転元と移転先の組、100 件ずつのページ）がローカルの関係を移します（`graph.Service.MigrateFollows`）。
  移転元を承認済みでフォローしている利用者は、移転先をブロックも、移転先からブロックもされておらず、移転先のドメインをミュートしていなければ、移転元へのフォローの `follow_preferences` を写した移転先へのフォロー（リモートか鍵アカウントなら承認待ち）を作り、移転元へのフォローを `moved` で終えます。そうでなければ移転元へのフォローを残します。
  どちらでも `graph.follow_migrated` を発行します。移転元をブロックしている利用者は移転先もブロックし、ミュートしている利用者は、移転先をまだミュートしていなければ同じ `hide_notifications`・`expires_at` で移転先をミュートします。
  利用者ごとに短いトランザクションで、移ったフォローは移転元に残らず、既にあるブロック・ミュートは変えないので、再実行しても何も増えません。
- 受け取った `Accept` / `Reject` / `Undo` が指すフォローは、`follows (uri)`（無ければ `follow_resends (uri)`）で引き、送り主の側が合わなければ、活動が埋め込む `Follow` の 2 人の組の承認待ち・承認済みのフォローで引きます。終了済みのフォローへの `Accept` / `Reject` / `Undo` は何もしません。

フォロー・ブロック・ミュートのコマンドは、記録したイベントや現在値に応じて次のドメインイベントを発行します（`tx.Emit`、§2.1）。
フォローとミュートのコマンドは 1 つ（ミュートの解除は、ミュートがあったときだけ）、ブロックは終了したフォローごとの `graph.follow_terminated` と `graph.blocked` を発行します。
購読する projector はイベントの内容（`follows` の行と、承認・拒否・終了の行、設定、`blocks` の行と取り消しの行、ミュートの値）だけで投影を更新します。

| ドメインイベント | 発行するとき | `graph` の投影 | 想定する購読者 |
| --- | --- | --- | --- |
| `graph.followed` | 鍵アカウントでない相手へのフォロー（自動承認） | `follow_edges`（`active`）、`follow_counters` 加算 | `feed_follow_edges`、通知 `follow` |
| `graph.follow_requested` | 鍵アカウントへのフォロー | `follow_edges`（`pending`） | 通知 `follow_request` |
| `graph.follow_accepted` | フォローリクエストの承認 | `follow_edges` を `active` に、`follow_counters` 加算 | `feed_follow_edges`、通知 `follow_accepted` |
| `graph.follow_rejected` | フォローリクエストの拒否 | `follow_edges` から削除 | — |
| `graph.follow_terminated` | フォロー解除・申請の取り下げ・フォロワーの削除など（終了前の状態を載せる） | `follow_edges` から削除、`active` だったら `follow_counters` 減算 | `feed_follow_edges` |
| `graph.follow_resent` | 承認済みのフォローへの、別の `id` の `Follow` の受信 | — | `Accept` の配送 |
| `graph.follow_preferences_changed` | フォローごとの設定の変更 | — | `feed_follow_edges`（`show_boosts`） |
| `graph.follow_migrated` | 移転したアカウントのローカルのフォロワーごとに、フォローを移転先へ移したか、残したとき（`outcome` が `followed` / `requested` / `blocking` / `blocked_by` / `domain_muted`） | — | 通知 `account_moved` |
| `graph.blocked` | ブロック（終了したフォローごとの `graph.follow_terminated`（`reason = blocked`）の後に発行する） | `block_edges` に追加 | `feed_exclusions`・`notification_exclusions`（両方向）、`Block` の配送 |
| `graph.unblocked` | ブロックの解除 | `block_edges` から削除 | `feed_exclusions`・`notification_exclusions`、`Undo Block` の配送 |
| `graph.muted` | ミュート（設定・期限の置き換えを含む。置き換えた後のミュートを載せる） | — | `feed_exclusions`（期限つき）、`notification_exclusions`（`hide_notifications` 付きのときだけ） |
| `graph.unmuted` | ミュートの解除と、期限切れのミュートの削除（ミュートが無ければ発行しない） | — | `feed_exclusions`、`notification_exclusions` |
| `graph.domain_muted` | ドメインのミュート（すでにミュートしていれば発行しない） | — | `feed_domain_mutes`、`notification_domain_exclusions` |
| `graph.domain_unmuted` | ドメインのミュートの解除（ミュートが無ければ発行しない） | — | `feed_domain_mutes`、`notification_domain_exclusions` |

#### ブロック・ミュート

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `blocks` | リソース | `id`, `uri`（`Block` の IRI。ローカルのブロックは `https://{host}/ap/activities/{id}`、ADR-0018。`id` の無い `Block` を受信したときだけ NULL）, `blocker_account_id`, `blocked_account_id`, `created_at` | 一意 `(uri)`。ブロックし直すと新しい行になる（過去のブロックが残る） |
| `block_retractions` | イベント | `block_id`（PK）, `retracted_at` | |
| `block_edges` | 投影 | `blocker_account_id`, `blocked_account_id`, `block_id` | PK `(blocker_account_id, blocked_account_id)`。インデックス `(blocked_account_id)`、`(blocker_account_id, block_id)`（ブロック中の一覧。`block_id` のキーセットで新しい順に引く）。取り消しで行を消す |
| `mutes` | 現在値 | `account_id`, `target_account_id`, `hide_notifications`（`bool`）, `expires_at`（NULL 可。NULL は解除まで）, `created_at` | PK `(account_id, target_account_id)`。`expires_at > created_at`。インデックス `(account_id, created_at, target_account_id)`（ミュート中の一覧。`(created_at, target_account_id)` のキーセットで新しい順に引く）、`(expires_at)`（期限切れの削除） |
| `account_domain_mutes` | 現在値 | `account_id`, `domain`（正規化したホスト。IDNA の A ラベルの小文字）, `created_at` | PK `(account_id, domain)`（FR-304。一覧はドメインの順に PK で引く）。1 アカウント 1000 件まで |

- ブロックは連合（`Block` / `Undo Block`）やフォロー関係の終了と連動し、嫌がらせの調査にも使うので、イベントとして持ちます。
- ブロックすると、同じトランザクションで 2 人の間のフォローを両方向とも（承認待ちも含めて）`follow_terminations`（`reason = blocked`）で終了します。
  解除してもフォローは戻りません。ブロックが有効な間は、どちらの向きのフォローも作れません。
- フォロー・承認・拒否・終了とブロックは、2 人の組（順序なし）単位でトランザクションを直列化します（`Repository.LockPair`）。
  PostgreSQL は READ COMMITTED なので、トランザクションスコープのアドバイザリロックを取ります。SQLite は書き込みトランザクションが `BEGIN IMMEDIATE` で直列なので何もしません。
  これが無いと、同時に走ったフォローとブロックが互いを見落とし、ブロック中のフォローが残ります。同じ承認待ちのフォローへの承認と拒否（または取り下げ）が両方記録され、`follow_counters` がずれます。
  - 承認・拒否・終了の projector は、コマンドが読んだ `state` の `follow_edges` の行だけを更新・削除し、行が無ければ再実行させます（再実行は変わった状態を読み直す）。組のロックを取らない経路（アカウントの消去）でも、結果は 1 つだけになり、カウンタは一度だけ動きます。システムが即座に拒否したフォロー（`rejected_by = system`）は行を持たないので対象外です。
- API からのフォローは、指定された設定（`show_boosts`）を同じトランザクションで記録します（`graph.FollowWith`）。API の `notify` は `notification` の購読で、フォローの後に別のトランザクションで読み書きします（§4.7）。新しいフォローは指定された設定で始まり、既にあるフォローは指定された設定だけを変えます。途中で失敗したら何も残しません。
- ローカルのアカウントは、ローカルのアカウントもリモートのアカウントもブロックできます（存在すれば有効でなくてもよい）。リモートのアカウントへのブロックは `federation` が `Block` / `Undo Block` として配送します。
- リモートのアカウントがローカルのアカウントをブロックしたことは、受信した `Block` から `graph.ReceiveBlock` が記録します（`blocks.uri` は `Block` の `id`）。同じ `uri` の `Block` の再受信と、ブロックが有効な組への `Block` は何もしません。`Undo Block` は `graph.RetractRemoteBlock` が、`uri`（`id` の無い `Block` は 2 人の組）で引いて取り消します。
- ミュートは本人だけに効き、履歴にも意味がないので現在値です（ADR-0020 規約 5）。ミュートし直すと設定・期限・`created_at` を置き換えます。
- 期限付きのミュートは、読み取り時に `expires_at > 現在時刻` で絞り込むので、期限が来た時点で書き込み無しに無効になります。
  期限切れの行は、1 時間ごとの消去ジョブ `graph.cleanup_mutes` が、1 トランザクションあたり件数を区切って削除し、削除した行ごとに `graph.unmuted` を発行します（FR-803）。
- ミュートの対象はリモートのアカウントでもかまいません。`graph` 自身はミュートを読み取り時に `expires_at` で絞り込み、他のモジュールの投影（`feed_exclusions`、`notification_exclusions`）のために `graph.muted` / `graph.unmuted` を発行します。
- ドメインのミュート（FR-304）は、リモートのドメインとそのサブドメインのアカウントの投稿と通知を、ミュートした本人からだけ隠します。運営者のドメインへの処分（§4.3）と違って、ほかの利用者にも連合にも何も変えず、相手のドメインにも伝えません。
  本人だけに効き履歴にも意味がないので、アカウントのミュートと同じく現在値です。期限はありません。このサーバー自身のドメインはミュートできません。
  `graph` は他のモジュールの投影（`feed_domain_mutes`、`notification_domain_exclusions`）のために `graph.domain_muted` / `graph.domain_unmuted` を発行します。
  - フォローしているアカウントは、ドメインをミュートしてもホームのタイムラインに残ります（フォローは本人が選んだものなので、ミュートより優先します）。ホームからは、そのドメインの投稿のブーストだけを除きます。
  - ローカル以外のタイムライン（連合・ハッシュタグ）からはそのドメインの投稿を除き、プロフィールのタイムラインからはそのドメインの投稿のブーストを除きます（そのドメインのアカウント自身のプロフィールは、投稿をそのまま見せます）。
  - 通知は、フォローしているかにかかわらず、差出人がそのドメインのアカウントなら作らず、作成済みのものも読み取り時に隠します（§4.7）。
