### 4.4 content

#### 投稿【リソース】と版【イベント】

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `statuses` | リソース | `id`, `uri`, `url`（NULL 可。リモートの投稿の表示用 URL だけを持ち、ローカルの投稿の表示用 URL は読むときに作る。ADR-0053）, `author_account_id`, `is_local`（`bool`）, `visibility`（`enum(public, local, unlisted, private, direct)`）, `in_reply_to_uri`（NULL 可）, `conversation_uri`（NULL 可）, `published_at` | 一意 `(uri)`。インデックス `(author_account_id, id)`, `(in_reply_to_uri)`, `(conversation_uri, id)`。CHECK `visibility <> 'local' OR is_local` |
| `local_statuses` | リソース（部分型） | `status_id`（PK）, `author_account_id`, `client_id`（NULL 可）, `idempotency_key`（NULL 可） | 一意 `(author_account_id, idempotency_key)` |
| `status_revisions` | イベント（版） | `id`, `status_id`, `seq`（`int`、1 始まり）, `spoiler_text`, `sensitive`（`bool`）, `language`（`lang`）, `revised_at` | 一意 `(status_id, seq)` |
| `status_revision_bodies` | イベント（版の子） | `revision_id`, `language`（`lang`）, `content_html` | PK `(revision_id, language)`。AP の `contentMap` を正規化したもの。`content` だけのものは版の `language` で 1 行（NFR-I18N-5） |
| `status_revision_sources` | イベント（部分型） | `revision_id`（PK）, `source_text`, `source_format`（`enum(text_plain)`） | ローカル投稿の入力ソース |
| `status_revision_attachments` | イベント（版の子） | `revision_id`, `position`（`int`、0〜3）, `media_id`, `description`（`text`、NULL 可） | PK `(revision_id, position)`。代替テキストは版ごと（編集で直せる）。ローカルの投稿・編集が添付できるのは、投稿者の `uploaded_media` で最新の `media_processing_results` が `ready` のものだけ（content のユースケースが検査する） |
| `status_revision_mentions` | イベント（版の子） | `revision_id`, `account_id` | PK `(revision_id, account_id)` |
| `status_revision_tags` | イベント（版の子） | `revision_id`, `tag_id` | PK `(revision_id, tag_id)` |
| `status_revision_emojis` | イベント（版の子） | `revision_id`, `emoji_id` | PK `(revision_id, emoji_id)`。インデックス `(emoji_id)`（リモートの絵文字の消去） |
| `remote_status_revisions` | イベント（部分型） | `revision_id`（PK）, `activity_uri`（NULL 可。版を運んだ `Create` / `Update`。取得で作った版は NULL）, `html_policy_version`（`int`）, `omitted_attachment_count`（`int`、既定 0） | リモートのみ。本文をサニタイズしたポリシーの版（ADR-0019 §2）。`omitted_attachment_count` は 4 件を超えて記録しなかった添付の数。版の `revised_at` は AP の `updated`（無ければ `published`） |
| `status_deletions` | イベント | `status_id`（PK）, `uri`, `author_account_id`, `origin`（`enum(author, moderation, remote, account_deletion, retention, auto_delete)`）, `moderation_action_id`（NULL 可）, `deleted_at` | 部分一意 `(uri) WHERE origin <> 'retention'`、インデックス `(uri)`。tombstone を兼ねる（遅れて届いた `Create` / `Update` で復活させない。GET には `410 Gone`） |

- **公開範囲・作者・返信先・公開日時は投稿の不変の事実** です。変わり得る本文・CW・言語・添付・メンション・タグは版に置きます（FR-109）。
- 返信先は `in_reply_to_uri`（不変の事実）だけを持ちます。親の `status_id` は `statuses (uri)` で引けるため保存しません。
  親を後から取得しても、投稿の行を更新する必要はありません。スレッド表示を速くするために、投影 `status_reply_links` を持ちます。
- ローカル投稿も `uri` を生成して保存します（ADR-0018 の規約）。
- リモートの投稿の公開範囲は、宛先（`to` / `cc`）から決めます（[federation.md §4](../../federation.md#4-as2-の正規化)）。
- `origin = retention`（保持期間による消去、[ADR-0034](../../../adr/0034-remote-content-scope-and-retention.md)）の削除は、同じ URI の取得し直しを妨げません。それ以外の削除は tombstone として、遅れて届いた `Create` / `Update` と取得を拒みます。
  取得し直した投稿は新しい `id` の行になり、後でまた消えることがあるので、1 つの URI が `retention` の削除を複数持てます。tombstone になる削除は URI ごとに 1 つです（部分一意）。URI で削除を引くときは tombstone を優先し、無ければ最新の `retention` の削除を返します。
  `retention` の削除しか無い URI への `Delete` は、未知のオブジェクトへの `Delete` として `remote_object_tombstones` を残します。

#### 投稿の投影

| テーブル | カラム | 制約 / 用途 |
| --- | --- | --- |
| `status_heads` | `status_id`（PK）, `revision_id`, `seq`, `last_revised_at` | 現在版（`seq` が最大の版）。`seq > 1` なら編集済み |
| `status_reply_links` | `status_id`（PK）, `parent_status_id`, `parent_author_account_id` | インデックス `(parent_status_id, status_id)`（返信一覧）。親が既知になった時点で作る |
| `status_mention_index` | `account_id`, `status_id` | PK `(account_id, status_id)`、インデックス `(status_id)`（編集・削除での置き換え）。現在版のメンション（`direct` の閲覧判定・メンション通知） |
| `status_sensitivity_overrides` | `status_id`, `source_action_id` | PK `(status_id, source_action_id)`、インデックス `(source_action_id)`。`mark_statuses_sensitive` による強制センシティブ。処分と同じトランザクションで書き、解除で処分の行を消す。行がある投稿の添付は、閲覧者を問わずセンシティブとして表示する（OGP のプレビューにも画像を出さない） |
| `status_counters` | `status_id`（PK）, `replies_count`, `boosts_count`, `favourites_count`（`bigint`） | §2.3 |
| `account_post_counters` | `account_id`（PK）, `statuses_count`（`bigint`、1 以上）, `last_status_at` | §2.3。投稿が 0 件のアカウントは行を持たない |
| `local_post_counters` | `account_id`（PK）, `posts_count`（`bigint`、1 以上） | §2.3。NodeInfo の `usage.localPosts`（全行の合計）。数えるものが 0 件のアカウントは行を持たない |

- 投稿の投影は **削除されていない投稿だけ** を対象にします。削除イベントを記録したトランザクションで、その投稿の `status_heads`・`status_mention_index`・`status_counters`・`status_reply_links`（返信としての行と、親としての行の両方）を消します。削除された投稿への遅れた版は投影に反映しません。
- `status_reply_links` は、返信と親の両方が既知で、どちらも削除されていないときに 1 行です。返信が先に届いた場合は、親の投稿のトランザクションで `statuses (in_reply_to_uri)` から返信を引いてつなぎます。
- 投稿の作成・編集・削除の projector は、投影を読む前に投稿のキーでトランザクションを直列化します（`Repository.LockStatusKeys`）。
  作成と削除は投稿の `uri` と `in_reply_to_uri`、編集と削除は投稿の `id` をキーにし、キーを決まった順に取ります。
  PostgreSQL は READ COMMITTED なので、トランザクションスコープのアドバイザリロックを取ります。SQLite は書き込みトランザクションが `BEGIN IMMEDIATE` で直列なので何もしません。
  これが無いと、同時に作られた親と返信が互いを見落としてつながらず、削除と同時の編集が削除済みの投稿の `status_heads`・`status_mention_index` を作り直します。
- `replies_count` は、つながった返信のうち公開範囲が `public` / `local` / `unlisted` のものを数えます。`private` / `direct` の返信は、件数からも存在が分からないように数えません。
- `local_post_counters` はローカルアカウントごとに、NodeInfo の `usage.localPosts` に数える投稿の件数を持ちます。
  Mastodon（`account_stats.statuses_count` のローカルアカウントの合計）と GoToSocial に合わせ、削除されていない `direct` 以外のローカルの投稿と、取り消されていない、削除されていない投稿へのローカルのブーストを数えます。
  - 両者はブーストを投稿として保存するので、ブーストも数えます。
  - NodeInfo は誰でも読めるので、要求のたびに投稿を数えず、ローカルアカウントの数の行を合計します。
    行をインスタンスで 1 つにしないのは、投稿のたびに同じ行を更新して書き込みが競合しないようにするためです。
  - 投稿の削除では、その投稿へのローカルのブーストの分も、ブーストした各アカウントから引きます。アカウントの消去（§8）では、消すブーストの分を引きます。
- `account_post_counters` は削除されていない投稿の件数と、その中で最新の `published_at` です。後から届いた古いリモート投稿では `last_status_at` は戻りません。投稿を削除したときは、残りの投稿から `last_status_at` を取り直します。
- 再構築（§9）は、各投影を空にして `statuses`・`status_revisions` と子テーブル・`status_deletions`（`local_post_counters` は加えて `boosts`・`boost_retractions`）から 1 つの SQL で作り直します。projector と同じ規則を SQL で書いたもので、両者の一致をコントラクトテストの性質テストで確かめます。

#### ローカル投稿のコマンド

ローカルの投稿・編集・削除は `internal/app/content` の `Service`（`Create` / `Edit` / `Delete`）が行います。

- **本文**: 利用者が書いたプレーンテキストを `status_revision_sources` にそのまま保存し、サーバーが HTML を生成して `status_revision_bodies` に保存します（ADR-0019 §2）。
  - 生成する HTML は `p`、`br`、`span[class]`、`a[href, class, rel, target]` だけで、テキストはすべてエスケープします。リンクには `rel="nofollow noopener noreferrer"` と `target="_blank"` を付けます。
  - メンション `@user`（`@user@{account_domain}`・`@user@{host}` も同じ）は、`account` の公開 API で引いたローカルの `active` なアカウントをリンクにし、版の `status_revision_mentions` に記録します。リンク先は `https://{host}/@{username}` です。
  - それ以外のドメインのメンション `@user@domain` は、そのハンドル（`account_handles`）を持つリモートの `active` なアカウントをリンクにし、同じく記録します。リンク先は Actor の `url`（最新の `remote_account_snapshots.profile_url`）、無ければ Actor の `uri` です。
  - 未知のハンドルは、トランザクションの前に `federation` の `RemoteActors.ResolveHandle`（WebFinger → Actor の取得 → 書き込み、[federation.md §6.2](../../federation.md#62-actor-とハンドル)）で解決します。1 つの本文で解決するのは 5 件まで、合計 10 秒までです。解決できないハンドル、`active` でないアカウントのハンドルはテキストのままです。
  - ハッシュタグ `#name` は、NFKC + 小文字化した名前で `tags` を引き（無ければ作り）、版の `status_revision_tags` に記録します。リンク先は `https://{host}/tags/{正規化した名前}` です。
  - `http` / `https` の URL だけを自動リンクにします。URL は空白・非 ASCII 文字・`<>"` などで終わり、末尾の句読点と対応しない閉じ括弧は含めません。
- **文字数**: 本文と CW の文字数（Unicode のコードポイント数）の合計が、インスタンス設定 `max_status_length` 以下でなければなりません。設定はコマンドごとに `instance` の公開 API から読みます。
- **公開範囲**: 要求された公開範囲を、`RestrictionPolicy` が返す上限で切り詰めて `statuses.visibility` に記録します（§5.4）。`post` の制限があれば投稿と編集を拒否します。削除は制限されません。
- **言語**: 要求が無ければ利用者設定の `default_post_language`、それも無ければ `und` です。
- **返信**: 返信先は、投稿者が `CanView` で見られる削除されていない投稿だけです。見られない・存在しない・削除済みは同じエラーにします。`conversation_uri` は返信先のものを引き継ぎ、返信でない投稿は自分の `uri` です。
- **冪等キー**: `Idempotency-Key` を受け取ったら `local_statuses (author_account_id, idempotency_key)` で同じ作者の投稿を引き、あればそれを返します。同時に届いた再送は一意制約で 1 つだけが成功し、残りは成功した投稿を返します。削除済みの投稿のキーはエラーにします。
- **編集**: 現在版の `seq + 1` で新しい版を作ります（FR-109）。公開範囲・作者・返信先は変わりません。
- **削除**: `status_deletions`（`origin = author`）を記録し、同じトランザクションで投影から外します。

コマンドは、記録したイベントに応じて次のドメインイベントを 1 つ発行します（`tx.Emit`、§2.1）。
購読する projector はイベントの内容（投稿の行・版と子テーブルの値・削除の行）だけで投影を更新します。

| ドメインイベント | 発行するとき | 載せるもの | `content` の投影 | 想定する購読者 |
| --- | --- | --- | --- | --- |
| `content.status_created` | 投稿 | 投稿、最初の版 | `status_heads`、`status_mention_index`、`status_reply_links`、`status_counters`、`account_post_counters` | `feed_entries`・`feed_tag_entries`・`feed_direct_recipients`、通知 `mention` |
| `content.status_revised` | 編集 | 投稿、新しい版 | `status_heads`、`status_mention_index` | `feed_entries`（言語）・`feed_tag_entries`・`feed_direct_recipients`、通知 `mention`（編集で増えたメンション） |
| `content.status_deleted` | 削除 | 投稿、削除の行 | すべての投稿の投影から外す、`account_post_counters` 減算 | `feed_*` から外す、`notification_deleted_statuses` |

#### リモート投稿のコマンド

受信した `Create` / `Update` / `Delete` の投稿は、`federation` のハンドラ（[federation.md §3.4](../../federation.md#34-アクティビティごとのハンドラ)）が、取り込み範囲（[ADR-0034](../../../adr/0034-remote-content-scope-and-retention.md)）を判定してから `content` の公開 API（`IngestRemoteStatus` / `ReviseRemoteStatus` / `DeleteRemoteStatus`）を 1 つのトランザクションの中で呼びます。
ローカル投稿と同じドメインイベント（`content.status_created` / `content.status_revised` / `content.status_deleted`）を発行するので、投稿の投影・`feed_*`・メンションの通知は同じ projector で更新されます。

- **投稿の行**: `id` は `published` の時刻の UUIDv7（§1.3）、`is_local` は偽です。公開範囲は宛先から決め（`local` にはならない）、`url` は表示用 URL が `id` と異なるときだけ持ちます。
  `conversation_uri` は、返信でなければ自分の `uri`、既知の親への返信なら親の `conversation_uri`、親が未知の返信なら NULL（スレッドが分からない）です。
- **版**: 本文は言語ごとにサニタイズして `status_revision_bodies` に保存し、版の `language` は文書の最初の言語です（`contentMap` も `content` も無ければ `und` の空の本文 1 行）。CW はプレーンテキスト、`remote_status_revisions` に版を運んだアクティビティとサニタイザのポリシーの版を記録します。
  メンションは `Mention` のタグと宛先のローカルの Actor で、未知のリモートの Actor は 1 つの投稿につき 5 件・合計 10 秒まで取得します。ハッシュタグは `tags` を正規化した名前で引き（無ければ作り）、添付は 4 件まで `remote_media` に宣言値を記録し、それを超えた添付の数を `remote_status_revisions.omitted_attachment_count` に記録します（クライアントは「+N」と元の投稿へのリンクを出す）。
- **編集**: `updated`（無ければ `published`）が現在版の `revised_at` より新しいときだけ、`seq + 1` の版を作ります。古い `Update` と同じ `Update` の再処理は版を作りません。
- **削除**: `status_deletions`（`origin = remote`）を記録します。削除済みの投稿の `Delete` は何もしません。
- **カスタム絵文字**: リモートの絵文字は `(shortcode, domain)` で引き、`domain` は作者のホスト、`uri` は `Emoji` の `id` が `domain` と同じホストのときだけ記録します（ほかのサーバーの絵文字の行を奪わせないため。ADR-0051 §1）。
  画像は作者を所有者とする `remote_media` で、記録した画像と異なる URL が届いたら新しい `custom_emoji_images` の行にします。shortcode が 128 文字を超える絵文字と、同じ文書の 2 つ目以降の同じ shortcode は読みません。
  クライアント API は、1 ページ分の投稿とアカウントの絵文字を `content` の公開 API（`MediaService.CustomEmojis`）で 1 回にまとめて読み、現在の画像をメディアプロキシの URL で返します。`domain` か画像のホストに `media_fetch` の制限がある絵文字は返しません（FR-702）。
- **投票**: `Question` の選択肢（20 件まで、プレーンテキスト）を版の `polls` / `poll_options` に、受信した票数を `poll_tally_observations` に記録します。リモートの票は取り消されないので、合計が最新の観測より少ない観測（遅れて届いた古い集計）と同じ票数の観測は記録しません。どの順で `Update` が届いても、集計は最も多い票数になります。
  観測を記録したらドメインイベント `content.poll_tally_observed` を発行し、同期 projector が `poll_tallies` を最新の観測に置き換えます。版を作らない `Update`（Mastodon が票数の変化で送る `Question` の `Update`）も、現在版の投票の観測を記録します。

#### リモート投稿の保持期間

1 時間ごとの消去ジョブ `content.purge_remote_statuses` が、保持期間を過ぎたリモートの投稿を消します（[ADR-0034](../../../adr/0034-remote-content-scope-and-retention.md)、FR-803）。

- 対象は、公開から設定 `remote_status_retention`（既定 90 日）を過ぎ、ローカルからの参照が無いリモートの投稿です。参照は、削除されていないローカルの返信、削除されていないローカルの投稿を含むスレッド（同じ `conversation_uri`）、取り消されていないローカルのブースト、お気に入り、ブックマーク、ピン留め、その投稿についての通知です。
  通知は `notification` の公開 API（`StatusNotified`）で、同じトランザクションの中で調べます。
- 設定 `remote_status_max_retention`（既定は上限なし）を設定すると、公開からその期間を過ぎたリモートの投稿は、参照があっても対象です。
- 通報か投稿単位の処分が指す投稿（`report_statuses`・`moderation_action_statuses`）と、効いている保全（§4.3）のある作者の投稿は、最大の期間を過ぎても対象にしません（ADR-0038）。判定は `moderation` の公開 API（`StatusRetained`）で行い、その記録が消えた後の回で対象になります。
- 対象の投稿ごとに `status_deletions`（`origin = retention`）を記録して `content.status_deleted` を発行し（投影・`feed_*` から外れ、通知は隠れる）、同じトランザクションで投稿の行・版と子テーブル・投票（選択肢・回答・観測・集計）・リモートのアカウントのブーストを物理削除します。
  `status_deletions`、ローカルのアカウントのブースト・お気に入り・ブックマーク・ピン留めは残し、読み取りでは対象の投稿を削除済みとして扱います（一覧から外れる）。
- 1 トランザクションで消すのは 100 件まで、1 回のジョブで 10 トランザクションまでです。続きはカーソル（投稿の `id`）を持った次のジョブが読みます。
  リモートの投稿の `id` は公開日時の UUIDv7（§1.3）なので、走査は主キーの範囲（期限の UUIDv7 の下限より前）で止まります。
- リレーだけが運んだ投稿（[ADR-0061](../../../adr/0061-federation-relays.md)）は、取り込みのトランザクションで `relayed_statuses` に記録します。
  1 時間ごとの消去ジョブ `content.purge_relayed_statuses` が、`relayed_statuses` にあり、公開から設定 `relay_status_retention`（既定 7 日）を過ぎ、上と同じくローカルからの参照が無く、作者をローカルのアカウントがフォローしていない投稿を、同じ規則（`origin = retention`、保全と通報の対象を残す）で消します。
  フォローは `graph` の公開 API（フォロワーの数）でトランザクションの外で調べ、消すトランザクションで候補を読み直して、その間に参照されたものを残します。走査は `relayed_statuses` の主キーで行い、残ったものは `content.purge_remote_statuses` の対象になります。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `relayed_statuses` | イベント | `status_id`（PK）, `relay_id`（参照を持たない ID。リレーの登録を消しても残す）, `received_at` | 投稿の物理削除（保持期間の消去）と一緒に消す |

#### 投稿の自動削除【現在値】

ローカルの利用者は、公開から指定した日数を過ぎた自分の投稿を自動で削除できます（FR-118）。設定は `content` が持ちます。

| テーブル | カラム | 制約 / インデックス |
| --- | --- | --- |
| `status_auto_delete_policies` | `account_id`, `enabled`（`bool`）, `min_age_days`（`int`、1〜3650）, `keep_pinned` / `keep_bookmarked` / `keep_favourited` / `keep_media` / `keep_polls`（`bool`）, `keep_min_boosts` / `keep_min_favourites`（`int`、0〜1,000,000。0 は閾値なし）, `updated_at` | PK `(account_id)`、部分インデックス `(account_id) WHERE enabled` |
| `status_auto_delete_kept_visibilities` | `account_id`, `visibility`（`enum`） | PK `(account_id, visibility)` |

- 行がなければ既定の設定（無効、30 日、ピン留め・自分のブックマーク・自分のお気に入り・`direct` を残す）です。保存は設定全体の置き換えで、残す公開範囲の行も入れ替えます。
- 1 時間ごとのジョブ `content.auto_delete_statuses` が、有効な設定をアカウントの順に 20 件ずつ読み（続きはカーソル（アカウントの ID）を持った次のジョブ）、アカウントごとに、公開から `min_age_days` 日を過ぎた削除されていないローカルの投稿を古い順に消します。
  例外は、ピン留め、本人のブックマーク・お気に入り、現在版の添付・投票、残す公開範囲、`status_counters` のブースト数・お気に入り数が閾値以上の投稿です。例外の判定は投稿を読む SQL の中で行うので、残る投稿は消す対象として読みません。
  ローカルの投稿の `id` は公開日時の UUIDv7 なので、走査は `(author_account_id, id)` のインデックスを期限の UUIDv7 の下限で止めます。
- 投稿ごとに `status_deletions`（`origin = auto_delete`）を記録して `content.status_deleted` を発行します。投影からの除外・通知・`Delete` の配送は本人の削除と同じで、行の物理削除も本人の削除と同じく消去ジョブ（§8）に任せます。
- 1 トランザクションで消すのは 20 件まで、1 回の実行で 1 アカウント 100 件までです。どの投稿も `Delete` を連合先へ送るので、古い投稿の多い設定は数時間に分けて消し、配送の量を抑えます（ADR-0004）。
- `active` でないアカウント（無効化・削除予定・削除済み）の投稿は消しません。ブーストは投稿ではないので対象外です。
- 保存していない設定が今消す投稿の数（`POST /preferences/auto_delete/preview`）は、ジョブと同じ SQL を 1 つの短い読み取りのトランザクションで 1,001 件まで読んで数えます（1,000 件で止め、超えたことを返す）。設定の有効・無効は数に関係しません。古い 3 件を例として返します。

#### ブースト

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `boosts` | リソース | `id`, `uri`（`Announce` の IRI）, `booster_account_id`, `status_id`, `visibility`（ローカルのブーストは元投稿の値を写す。リモートのブーストは `Announce` の宛先から決めた公開範囲と元投稿の公開範囲の狭いほう。不変）, `is_local`（ブーストした人がローカルか）, `boosted_at` | 一意 `(uri)`。インデックス `(status_id)`（件数の再集計）。CHECK `visibility <> 'local' OR is_local` |
| `boost_retractions` | イベント | `boost_id`（PK）, `origin`（`enum(booster, status_deleted, moderation, remote)`）, `retracted_at` | |
| `boost_edges` | 投影 | `booster_account_id`, `status_id`, `boost_id` | PK `(booster_account_id, status_id)`、インデックス `(status_id)`（投稿の削除での除去）。有効なブーストは 1 人 1 投稿につき 1 つ。取り消し後の再ブーストは新しい `boosts` 行 |

- `local` 投稿のブーストは連合しません（ADR-0017）。写した `visibility = 'local'` がその印です。`local` 投稿のブーストはこのサーバーのアカウントだけが作るので、`boosts.is_local` は常に真です（CHECK）。配送ジョブは作りません（`content.Boost.Federates`）。
- ブーストできるのは、閲覧できる（`canView`）投稿のうち `public` / `local` / `unlisted` のものです。`private` は作者本人だけが自分のフォロワーへブーストでき、`direct` は誰もブーストできません。
- `boost_edges` は、取り消されていないブーストのうち、元投稿が削除されていないものを持ちます。投稿の削除イベントを記録したトランザクションで、その投稿の行を消します（取り消しイベントは作りません）。
- `status_counters.boosts_count` は `boost_edges` の行を挿入・削除したときだけ増減し、`favourites_count` は `favourites` の行を実際に挿入・削除したときだけ増減します（§2.3）。再構築は、取り消されていない `boosts` と `favourites` の件数を数えます。
- 同じ組への同時のブーストは、後の方が `boost_edges` の主キーで再実行され、既存のブーストを返します。同時の取り消しは、後の方が `boost_retractions` の主キーで再実行され、何もしません。
- **リモートのブーストとお気に入り**: 受信した `Announce` / `Like` / `Undo` は、`federation` のハンドラ（[federation.md §3.4](../../federation.md#34-アクティビティごとのハンドラ)）が `content` の公開 API（`RecordRemoteBoost` / `RetractRemoteBoost` / `RecordRemoteFavourite` / `RemoveRemoteFavourite`）で記録します。ローカルのコマンドと同じドメインイベントを発行するので、件数・`feed_entries`・通知は同じ projector で更新されます。
  - リモートのアカウントがブーストできるのは、閲覧できる投稿のうち `Boostable` なものです。ローカルの投稿は、そのアカウントが ActivityPub で取得したときに見られるもの（`canView` の `ContextObjectFetch`）で、`local` の投稿は誰もブーストできません（ADR-0017）。公開範囲が `direct` になる（宛先が公開でもフォロワーでもない）`Announce` は記録しません。
  - ブーストの `id` は `Announce` の `published`（無い・未来なら受信時刻）の UUIDv7、`boosts.uri` は `Announce` の `id` です。`Undo Announce` は `boost_retractions`（`origin = remote`）を記録します。
  - リモートのアカウントがお気に入りできるのは、閲覧できる（`ContextObjectFetch`）ローカルの投稿だけです。`favourites.uri` は `Like`（または `EmojiReact`）の `id` です。そのため、リモートの投稿のお気に入りは常にローカルのアカウントのもので、`Like` の配送の対象です。
  - `Undo Like` は、`favourites (uri)` で引き、無ければ埋め込みの `Like` の `object` が指す投稿への主体のお気に入りを消します。

ブースト・お気に入りのコマンドは、行を実際に書いたときだけ次のドメインイベントを 1 つ発行します（`tx.Emit`、§2.1）。ブックマークとピン留めは本人だけのものなので、イベントを発行しません。

| ドメインイベント | 発行するとき | `content` の投影 | 想定する購読者 |
| --- | --- | --- | --- |
| `content.boosted` | ブーストの作成 | `boost_edges`、`boosts_count` 加算 | `feed_entries`（`kind = boost`）、通知 `boost`、`Federates` なら `Announce` の配送 |
| `content.boost_retracted` | 有効なブーストの取り消し | `boost_edges` から削除、`boosts_count` 減算 | `feed_entries` から削除、連合したブーストなら `Undo Announce` の配送 |
| `content.favourited` | お気に入りの追加 | `favourites_count` 加算 | 通知 `favourite`、リモートの投稿なら `Like` の配送 |
| `content.unfavourited` | お気に入りの削除 | `favourites_count` 減算 | リモートの投稿なら `Undo Like` の配送 |
| `content.reacted` | 絵文字リアクションの追加 | `status_reaction_counts` の絵文字の件数を加算 | 通知 `reaction`、リモートの投稿なら絵文字付きの `Like` の配送 |
| `content.unreacted` | 絵文字リアクションの削除 | `status_reaction_counts` の絵文字の件数を減算 | リモートの投稿なら `Undo Like` の配送 |

#### 絵文字リアクション

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `reactions` | リソース | `id`, `account_id`, `status_id`, `emoji_key`（Unicode 絵文字なら fully-qualified の形の絵文字、カスタム絵文字なら `custom_emojis.id`）, `custom_emoji_id`（NULL 可。Unicode 絵文字なら NULL）, `uri`（`Like` / `EmojiReact` の IRI）, `created_at` | 一意 `(account_id, status_id, emoji_key)`, `(uri)`。インデックス `(status_id, emoji_key, id)`（絵文字で絞った一覧）, `(status_id, id)`（一覧）, `(custom_emoji_id)`（リモートの絵文字の消去）。CHECK `custom_emoji_id IS NULL OR custom_emoji_id = emoji_key` |
| `status_reaction_counts` | 投影 | `status_id`, `emoji_key`, `custom_emoji_id`（NULL 可）, `count`（`bigint`、1 以上） | PK `(status_id, emoji_key)`。インデックス `(status_id, count DESC, emoji_key)`（多い順の上位 20） |

- 絵文字リアクション（FR-116、ADR-0052）は、お気に入りとは別の資源です。1 人が 1 投稿に絵文字ごとに 1 つ付けられ、取り消すと行を消して `content.unreacted` を発行します（取り消した後の付け直しは新しい行）。
- `status_reaction_counts` は削除されていない投稿のリアクションを絵文字ごとに数え、`reactions` の行を実際に挿入・削除したときだけ増減します。件数が 0 になった行は消し、投稿の削除イベントでその投稿の行を消します。再構築は、削除されていない投稿の `reactions` を絵文字ごとに数えます。
- Unicode 絵文字は、Unicode の `emoji-test.txt`（`scripts/task.sh emoji:generate` が生成する `internal/domain/content/emoji_table.gen.go`）の fully-qualified の形に正規化してから `emoji_key` にします。異体字セレクタ（U+FE0F）の有無が違う同じ絵文字は、同じ件数に数えます。
- **リモートのリアクション**: 受信した `EmojiReact` と、絵文字の付いた `Like`（[federation.md §3.4](../../federation.md#34-アクティビティごとのハンドラ)）を、`content` の公開 API（`RecordRemoteReaction` / `RemoveRemoteReaction`）で記録します。リモートのアカウントがリアクションできるのは、お気に入りと同じく閲覧できる（`ContextObjectFetch`）ローカルの投稿だけで、交流の禁止（`interact`）の制限があれば記録しません。リアクションの `id` は活動の `published`（無い・未来なら受信時刻）の UUIDv7 です。
  - カスタム絵文字は、投稿のカスタム絵文字と同じく `custom_emojis` と `custom_emoji_images` に取り込み、画像はリアクションした人のリモートのメディアです。ドメインは `Emoji` の `id` のホスト（`id` が無ければリアクションした人のドメイン）で、第三のサーバーの絵文字はそのサーバーの絵文字になります。
  - `Undo` は、`reactions (uri)`、`favourites (uri)` の順に引き、どちらにも無ければ埋め込みの活動の投稿と絵文字で主体のリアクション（またはお気に入り）を消します。
- **ローカルの人のリアクションと `Like` の枠**（ADR-0052 §1、§2）: ローカルの人は 1 投稿にお気に入りかリアクション 1 つのどちらか一方だけを持ちます。`content.Interactions.React` は、同じトランザクションで同じ投稿へのお気に入り（`content.unfavourited`）と別の絵文字のリアクション（`content.unreacted`）を消してから行を挿入し（`content.reacted`）、`Favourite` も同じくリアクションを消してからお気に入りを挿入します。同じ絵文字で付け直しても何も変えません。`Unreact` はその人のリアクションを消します。
  - 使える絵文字は、Unicode 絵文字（fully-qualified の形に正規化）と、このサーバーの有効な（`custom_emoji_settings.enabled`）画像のあるカスタム絵文字です。リモートのカスタム絵文字は使えません。閲覧の可否・ブロック・交流の禁止（`interact`）はお気に入りと同じ規則です。
  - リモートの投稿へのリアクションは、イベントにカスタム絵文字の `Emoji` タグの材料（ショートコード・画像）を持たせ、`federation` が絵文字付きの `Like` と `Undo Like` を組み立てて保存します（[federation.md §7.1](../../federation.md#71-記録)）。

#### メディア

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `media_files` | リソース | `id`, `owner_account_id`（NULL 可。リンクのカードの画像は NULL）, `origin`（`enum(upload, remote)`）, `created_at` | インデックス `(owner_account_id, id)` |
| `uploaded_media` | リソース（部分型） | `media_id`（PK）, `blob_key`, `content_type`, `file_size`（`bigint`） | |
| `remote_media` | リソース（部分型） | `media_id`（PK）, `owner_account_id`（`media_files` の値を写す）, `remote_url`, `declared_content_type`（NULL 可。AP の `mediaType`）, `declared_media_type`（`enum(image, gifv, video, audio, unknown)`）, `purpose`（`enum(attachment, avatar, header, emoji)`。記録した用途で、メディアプロキシの `small` の大きさを決める）, `width` / `height`（NULL 可）, `blurhash`（NULL 可）, `focus_x` / `focus_y`（`real`、-1.0〜1.0、NULL 可。範囲外の宣言は取り込み時に丸める）, `description`（`text`、NULL 可） | 一意 `(owner_account_id, remote_url)`。受信した文書が宣言した値で、ファイルは取得しない（[federation.md §9](../../federation.md#9-リモートメディア)） |
| `media_processing_results` | イベント | `id`, `media_id`, `purpose`（`enum(attachment, avatar, header, emoji)`）, `outcome`（`enum(ready, failed)`）, `media_type`（`enum(image, gifv, video, audio, unknown)`）, `width` / `height` / `duration_ms`（NULL 可）, `blurhash`（NULL 可）, `processed_at` | インデックス `(media_id, id DESC)`。最新行が現在の処理結果 |
| `media_variants` | イベント（結果の子） | `result_id`, `size`（`enum(small, medium, large)`）, `motion`（`enum(still, animated)`）, `blob_key`, `content_type`, `file_size`（`bigint`）, `width` / `height`（`int`） | PK `(result_id, size, motion)`（[ADR-0054](../../../adr/0054-media-variants-video-audio.md)） |
| `media_focus` | 現在値 | `media_id`（PK）, `focus_x` / `focus_y`（`real`、-1.0〜1.0） | 行がなければ中央 |
| `media_descriptions` | 現在値 | `media_id`（PK）, `description`（`text`、空でない、1,500 文字まで） | 行がなければ説明なし。投稿が添付するときに `status_revision_attachments.description` へ写す |
| `remote_media_cache` | 現在値（キャッシュ） | `url`（PK。メディアプロキシの `small` は `{URL}#small-{purpose}-{箱}`、箱が切り抜くときは焦点も加える。[federation.md §9](../../federation.md#9-リモートメディア)）, `blob_key`（`remote/{url の SHA-256}-{乱数}`。取得し直すたびに新しいキー）, `content_type`（中身で判定した形式）, `file_size`（`bigint`）, `fetched_at`, `expires_at`, `last_accessed_at`（1 時間単位で更新） | FR-409, NFR-COST-4。インデックス `(expires_at)`, `(last_accessed_at)`。メディアプロキシが書き、`media.cleanup_remote` が期限切れと容量超過の分を消す（federation.md §9） |
| `media_blob_deletions` | 現在値（作業の一覧） | `blob_key`（PK）, `delete_after` | インデックス `(delete_after)`。メディアのファイルとリモートのメディアのキャッシュの Blob で、`delete_after` 以降に消すもの。Blob ストアはキーを列挙できないので、どの Blob も書く前から消したあとまでいずれかの行にキーがある。書く側は書く前にキーを猶予（24 時間）付きで記録し、Blob を指す行を記録するトランザクションで消す。失敗した・競合に負けた書き込みは書いた Blob を消し、消せなかったものは猶予のあとにクリーンアップが消す。行を消す側は同じトランザクションでキーを今の時刻で記録し、そのあと Blob を消してから行を消す。`media.cleanup` と `media.cleanup_remote` が期限の来たものを消す |
| `link_card_media` | リソース（部分型） | `media_id`（PK）, `remote_url`, `content_type`（中身で判定した形式）, `width` / `height`（`int`）, `blurhash`, `description`（`og:image:alt`。無ければ空） | 一意 `(remote_url)`。リンクのカードの画像（[ADR-0062](../../../adr/0062-link-preview-cards.md) §3）。所有者はアカウントではなくカードの URL で、`media_files.owner_account_id` は空。1 回だけ取得して形式・寸法・blurhash を記録し、本体は保存しない。メディアプロキシが添付の画像と同じ規則で中継する |

#### リンクのカード

[ADR-0062](../../../adr/0062-link-preview-cards.md) の、投稿中のリンクの先のページから作るカードです（FR-117、[federation.md §9.1](../../federation.md#91-リンクのカード)）。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `link_cards` | 現在値（キャッシュ） | `url`（PK。正規化した要求の URL）, `card_url`（最終 URL、または同じオリジンの `og:url`）, `title`, `description`, `site_name`, `author_name`（無ければ空）, `published_at`（NULL 可）, `image_media_id`（NULL 可。`link_card_media`）, `creator_account_id`（NULL 可。`attributionDomains` で検証できた `fediverse:creator`）, `language`（NULL 可。`<html lang>`）, `fetched_at` | インデックス `(fetched_at)`, `(image_media_id)`。取得のたびに UPSERT する。URL ごとに共有し、取得から 14 日を過ぎたら、その URL を新しく参照した投稿の取り込みで取り直す |
| `status_link_cards` | 投影 | `status_id`（PK）, `url` | インデックス `(url)`。削除されていない投稿の現在の版から選んだ URL。`link_cards` の行がない間はカードを出さない |

- `status_link_cards` は、投稿の作成・編集のイベントの投影が書き、削除のイベントと消去のトランザクションが消します。カードの取得は投影が積むジョブ `content.fetch_link_card` が行います。
- 1 日 1 回のジョブ `content.cleanup_link_cards` が、どの投稿からも参照されず取得から 14 日を過ぎた `link_cards` の行と、どのカードも使わない `link_card_media`（と `media_files` の行）を消します（FR-803）。

#### 引用

[ADR-0069](../../../adr/0069-quote-posts.md) の、投稿が引用する投稿と、その承認です（FR-114、[federation.md §3.4](../../federation.md#34-アクティビティごとのハンドラ)）。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `status_quotes` | リソース | `status_id`（PK）, `quoted_uri`（引用先の投稿の URI。まだ保存していない投稿でもよい）, `form`（`enum(fep_044f, legacy)`） | インデックス `(quoted_uri)`。引用先は投稿の作成時に決まり、変わらない |
| `status_quote_events` | イベント | `id`, `status_id`, `kind`（`enum(accepted, rejected, revoked, detached)`）, `authorization_uri`（`accepted` が検証した、`revoked` が削除を受けた `QuoteAuthorization`。ほかは NULL。自分の投稿の引用の承認も NULL）, `occurred_at` | インデックス `(status_id, id)` |
| `status_quote_states` | 投影 | `status_id`（PK）, `state`（`enum(pending, accepted, rejected, revoked, legacy, detached)`）, `authorization_uri`（`accepted` のときだけ） | インデックス `(authorization_uri)` |
| `status_quote_policies` | 現在値 | `status_id`（PK）, `policy`（`enum(public, followers, nobody)`）, `updated_at` | ローカルの投稿は作成のときに必ず持ち、作者がいつでも変えられる。リモートの投稿は `interactionPolicy.canQuote` を宣言したものだけが持ち、`Update` のたびに置き換える |
| `quote_authorizations` | リソース | `id`, `quoting_uri`（引用元。ローカルでもリモートでもよい）, `quoting_account_id`, `quoted_status_id`（ローカルの引用先）, `created_at` | 一意 `(quoting_uri, quoted_status_id)`、インデックス `(quoted_status_id, id)`。このサーバーが出した承認で、IRI は `https://{host}/ap/quote_authorizations/{id}`（保存しない） |
| `quote_authorization_revocations` | イベント | `authorization_id`（PK）, `revoked_at` | 承認の取り消し。取り消した承認の IRI は 410 を返す |

- `status_quote_states` は、引用の記録（`content.quote_recorded`）と承認の変化（`content.quote_changed`）のイベントの投影が `content.QuoteStateOf` の規則で書きます。
  `fep_044f` の引用は `pending` から、`legacy` の引用は `legacy` から始まり、`legacy` の引用は承認では変わりません。`revoked` は、承認した `QuoteAuthorization` の削除だけが起こします。`detached`（編集で引用を外した）は最後の状態です。
- 引用先の削除・ブロック・ミュート・閲覧の可否は状態に持たず、表示のたびに閲覧者ごとに判定します（`content.Reader`）。
- 誰が引用できるかは `content.MayQuote` の 1 か所で決めます（投稿の作成、`Status.quotable`、リモートの `QuoteRequest` への答え、方言の引用の表示）。閲覧できない投稿と `direct` は引用できず、`private` は作者が `private` / `direct` で引用するときだけ、`local` はリモートからは引用できません。
- ローカルの投稿を引用すると、引用先がローカルの別の作者の投稿ならすぐに `quote_authorizations` を記録して承認し、自分の投稿なら承認だけを記録します。リモートの投稿は、引用の許可を宣言していれば `QuoteRequest` の答えを待ち（`pending`）、宣言していなければ `legacy` です。
- 引用の表（`status_quotes`・`status_quote_events`・`status_quote_states`・`status_quote_policies`）と、引用先の投稿への承認（`quote_authorizations` と取り消し）は、投稿の物理削除（保持期間の消去・アカウントの消去）と一緒に消します。

- アップロードは同期で受け付けます。形式は中身から判定し（申告された `Content-Type` とファイル名は信用しない）、サイズと寸法の上限は全体をデコードする前にヘッダで検査します（ADR-0019 §8）。
  `uploaded_media.content_type` は判定した形式で、`blob_key`（`uploads/<id>`）は受け取ったままの非公開のファイルです。
- 処理はジョブ `media.process` が非同期に行います。アップロードの用途（`purpose`。API の `MediaUpload.purpose`、既定 `attachment`）の大きさで、再エンコードでメタデータ（EXIF の位置情報など）を落とした `media_variants` を作り（[ADR-0054](../../../adr/0054-media-variants-video-audio.md) §1〜§3）、
  処理結果ごとの公開のキー（`media/<id>/<result_id>/<size>.<ext>`、アニメーションの動く版は `media/<id>/<result_id>/<size>-animated.<ext>`）に書き、結果を記録してから受け取ったままのファイルを Blob から削除します（位置情報を残さない）。
  キーは処理結果を含むので同じキーが別の中身を指すことはなく、`Cache-Control: public, max-age=31536000, immutable` で配ります。
  - 静止版は `small` / `medium` / `large` の 3 つです。`large` は不透明なら JPEG、透過があれば PNG で、`small` / `medium` は WebP です。アニメーション（GIF、アニメーション WebP）は、最初のフレームの静止版と、動く版（`large` は GIF、`small` / `medium` はアニメーション WebP）の 6 つです。
  - 用途ごとの大きさ: 投稿の画像は面積 640×360 / 1920×1080 / 3840×2160 まで、アイコンは焦点（`media_focus`、なければ中央）に合わせて正方形に切り抜いた 96 / 192 / 400 px、ヘッダーは 750×250 / 1500×500 / 3000×1000 の箱、カスタム絵文字は 64 / 128 / 256 px の箱に収めます。元より大きくはしません。
  - 別の用途で使う（投稿の画像をアバターにするなど）と、`content.MediaService.CheckProfileImage` が新しい用途の `media.process` を投入し、現在の結果の `large`（アニメーションなら動く版）から処理し直して新しい結果を記録します。それまでは前の結果の版を表示します。古い結果の Blob は残ります。
  ジョブは冪等です。最新の結果が同じ用途のファイルは受け取ったままのファイルの削除だけを繰り返し、消去済みのファイルは何もしません。初めての処理で処理できない中身は `outcome = failed`（版なし）の結果として記録します。処理し直しが失敗したときは記録せず、前の結果が残ります。
- 動画・音声（投稿の添付だけ）はトランスコードしません（[ADR-0054](../../../adr/0054-media-variants-video-audio.md) §6）。アップロードのときに container を解析して codec・長さ・寸法を検査し（上限は設定 `media_max_av_bytes`・`media_max_video_duration`・`media_max_audio_duration`、[設定リファレンス](../../../reference/configuration.md)）、
  クライアントが送ったポスター（API の `thumbnail`）を `uploads/<id>.poster` に置きます。`media.process` は、メタデータを落として並べ直した（remux）ファイルを `size = large`・`motion = animated` の版（`media/<id>/<result_id>/large-animated.<ext>`）にし、
  ポスター（なければファイルに埋め込まれたカバー画像）を投稿の画像として処理した静止版 3 つと blurhash を同じ結果に入れます。ポスターのない動画・音声は静止版を持ちません。結果の `media_type` は `video` / `audio`、`duration_ms` は長さで、受け取ったままのファイルとポスターは処理のあとに消します。
  動画・音声はほかの用途（アバター・ヘッダー・カスタム絵文字）に使えません。
- `width` / `height` は EXIF の向きを適用した元画像の寸法（動画は回転を適用した表示の寸法、音声は NULL）です（処理し直しても元の寸法を写す）。`media_variants` の寸法は各版の実寸です。
- 未添付のアップロードを読めるのも、説明（`media_descriptions`）と焦点（`media_focus`）を変えられるのも、アップロードした本人だけです。
  版が添付したあとは説明を版の `status_revision_attachments.description` で持つので、ファイル単体の説明・焦点は変えられません（投稿を編集して直す）。
- 未添付のアップロード（どの版からも参照されず、`account_profile_revisions` のアバター・ヘッダーにも使われていない `media_files`）は、
  作成から `media_unattached_ttl`（既定 24 時間）後に、1 時間ごとの消去ジョブ `media.cleanup` が行と Blob ごと削除します（FR-803）。
  アバター・ヘッダーの参照は `account` モジュール、インスタンスのプレビュー画像（`preview_image`、§4.9）の参照は `instance` モジュールの公開 API で問い合わせます。削除は同じトランザクションで添付の有無を再検査します。
  行を消すトランザクションで Blob のキーを `media_blob_deletions` に記録し、そのあと Blob を消すので、Blob の削除に失敗してもキーが残り、次の実行が消し直します。
- アップロードと `media.process` は、Blob を書く前にキーを `media_blob_deletions` に猶予付きで記録し、`uploaded_media` や処理結果を記録するトランザクションで外します。
  書き込みの失敗（オブジェクトを置いたあとの取り消しを含む）、結果の記録の失敗、同じジョブの並行する実行に負けたときは、書いた Blob を消します。消せなかったもの・処理の途中で止まったものは、猶予のあとに `media.cleanup` が消します。

#### タグ・カスタム絵文字・投票

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `tags` | リソース | `id`, `name`（最初に使われた表記）, `name_normalized`（NFKC + 小文字化）, `created_at` | 一意 `(name_normalized)`（PostgreSQL は `COLLATE "C"`、SQLite は既定の `BINARY` でバイト順。ハッシュタグの前方一致の検索が範囲で引く、ADR-0092 §1） |
| `custom_emojis` | リソース | `id`, `shortcode`, `domain`（ローカルは `''`）, `uri`（NULL 可。ローカルは常に NULL で、`id` から `https://{host}/ap/emojis/{id}` を導く）, `created_by_user_id`（NULL 可。作った利用者。承認で作った絵文字は申請者、管理画面で作った絵文字と複製はその管理者、CLI とリモートの絵文字は NULL。変えない）, `created_at` | 一意 `(shortcode, domain)`, `(uri)`。インデックス `(created_by_user_id, id)`（作成者の絵文字と上限の数え上げ） |
| `custom_emoji_images` | イベント（版） | `id`, `emoji_id`, `media_id`, `revised_at` | インデックス `(emoji_id, id DESC)`, `(media_id)`。最新の行が現在の画像。ローカルは管理者がアップロードした `media_files`（CLI と承認した申請はインスタンスの Actor の `media_files`。申請は申請者のアップロードの処理結果を複製する）、リモートは `remote_media` |
| `custom_emoji_requests` | リソース | `id`, `kind`（`enum(create, image)`）, `requester_user_id`, `emoji_id`（NULL 可。`image` の対象）, `shortcode`（`image` は対象の写し）, `media_id`（申請者のローカルのアカウントのアップロード）, `created_at` | CHECK `(kind = 'create') = (emoji_id IS NULL)`。インデックス `(requester_user_id, id)`, `(media_id)`。終わった申請も消さない（ADR-0070 §7） |
| `custom_emoji_request_outcomes` | イベント | `id`, `request_id`, `outcome`（`enum(approved, rejected, withdrawn, lapsed)`）, `decided_by_user_id`（NULL 可。CLI と `lapsed`。取り下げは申請者）, `reason`（NULL 可、500 文字まで。`rejected` だけ）, `emoji_id`（`approved` だけ。作った・変えた絵文字）, `decided_at` | 一意 `(request_id)`。行のない申請が審査中 |
| `custom_emoji_pending_requests` | 投影 | `request_id`（PK）, `requester_user_id`, `reserved_shortcode`（`create` だけ）, `emoji_id`（`image` だけ） | 一意 `(reserved_shortcode)`, `(emoji_id)`。インデックス `(requester_user_id)`。申請と同じトランザクションで作り、結果の行と同じトランザクションで消す。`custom_emoji_requests` のうち結果の行のないものから作り直せる |
| `custom_emoji_settings` | 現在値 | `emoji_id`（PK）, `enabled`, `visible_in_picker`（`bool`）, `category`（NULL 可、64 文字まで） | 行が無ければ有効・ピッカーに出す・カテゴリなし（既定に戻すと行を消す）。リモートの絵文字は `enabled` だけを使う |
| `custom_emoji_aliases` | 現在値 | `emoji_id`, `alias`（shortcode と同じ文字種） | PK `(emoji_id, alias)`。ローカルだけ。ピッカーの検索に使う |
| `polls` | イベント（版の子） | `id`, `revision_id`, `multiple`（`bool`）, `hide_totals`（`bool`。ローカルの投票で、締め切りまで選択肢ごとの票数を隠す）, `closes_at`（NULL 可） | 一意 `(revision_id)`。投票を作り直す編集は新しい版の新しい投票になる |
| `poll_options` | イベント（子） | `poll_id`, `position`, `title` | PK `(poll_id, position)` |
| `poll_votes` | イベント | `id`, `poll_id`, `account_id`, `position`, `uri`（NULL 可）, `voted_at` | 一意 `(poll_id, account_id, position)`, `(uri)`。インデックス `(account_id)`（消去）。複数選択の回答は選択肢ごとに 1 行 |
| `poll_tally_observations` | イベント | `id`, `poll_id`, `voters_count`（NULL 可）, `observed_at` | 投票の集計値。リモートの投票は相手が `Create` / `Update` で送ったもの、ローカルの投票は投稿したとき（0）と回答のたびに `poll_votes` を数え直したもの |
| `poll_tally_observation_counts` | イベント（子） | `observation_id`, `position`, `votes` | PK `(observation_id, position)` |
| `poll_tallies` | 投影 | `poll_id`, `position`, `votes`, `voters_count`（NULL 可。投票者数の分からない複数選択のリモートの投票） | PK `(poll_id, position)`。ローカルもリモートも最新の観測から |

- ローカルの絵文字（ADR-0051）: shortcode は `[A-Za-z0-9_]{2,30}` で、大文字と小文字を区別し、変えられません。投稿とプロフィールを保存するとき、本文・CW・表示名・自己紹介・補足情報の
  `:shortcode:`（前後が英数字と `:` でないもの）から有効なローカルの絵文字だけを版の `status_revision_emojis` / `account_profile_revision_emojis` に記録します。
  無効にした絵文字は表示・連合・ピッカーから外れ、削除は `custom_emojis`・`custom_emoji_images`・設定・別名の行を同じトランザクションで消します（過去の版の参照は残し、読み取りで飛ばします）。
  絵文字の画像のアップロードは、版が添付していなくても未使用のアップロードの掃除と、アカウントの消去で消しません。削除した絵文字の画像は未使用のアップロードになり、掃除が Blob ごと消します。
- 絵文字の作成の申請（ADR-0070 §2〜§4）: `create_custom_emojis` を持つ利用者は、新しい絵文字（`create`）か、自分が作った絵文字の画像の差し替え（`image`）を申請し、`manage_custom_emojis` を持つ人が承認・却下します。
  申請は絵文字ではないので、承認まで絵文字の読み取りの経路（ピッカー・本文からの抽出・`GET /ap/emojis/{id}`・リアクション・管理の一覧）のどれにも現れません。
  審査中の `create` は shortcode を予約し（`reserved_shortcode` の一意と、ローカルの絵文字の作成のときの照合）、`image` は 1 つの絵文字に 1 つまでです。審査中の申請の画像は申請者の未添付のアップロードのままで、未使用のアップロードの掃除はそれを残します（`content.PendingCustomEmojiRequestMedia`）。アカウントの消去は消します。
  承認は同じトランザクションで結果の行を書き、`custom_emojis`（作成者は申請者）・`custom_emoji_images` を作ります。画像は申請者のアップロードの処理結果をインスタンスの Actor のアップロードとして複製したもので、申請者のアップロードは未使用に戻ります。承認のときに申請者が `create_custom_emojis` を持たないか、shortcode がローカルの絵文字にあれば承認は失敗します。
  作成者は、`create_custom_emojis` を持ち、アカウントが使える間だけ、自分の絵文字の別名・カテゴリ・ピッカーに出すかを変え、削除できます。絵文字の削除は、その絵文字の審査中の `image` の申請を `lapsed` で閉じます。
  作成数の上限はポート `content.CustomEmojiQuota` が返し（プランのロールは `supporters.Service.EmojiLimit`、ADR-0072 §11）、その UTC の暦月（1 日 00:00 UTC から翌月 1 日 00:00 UTC まで）に作成者が出した `create` の申請のうち、承認された（`approved`）ものと審査中のものの数が上限以上なら新しい `create` を断ります（ADR-0124）。月は申請の `created_at` で決め、月をまたいで承認された申請は出した月に数えます。却下・取り下げ・失効した申請は数えず、承認した絵文字を後で削除しても数は減りません。`image` の申請と管理画面・CLI で作った絵文字は数えません。上限なし（既定）のときは数えません。使った数の API は次のリセットの時刻（翌月 1 日 00:00 UTC）も返します。
- リモートの絵文字の消去（ADR-0051 §1、FR-803）: 1 日ごとのジョブ `content.purge_remote_emojis` が、どの投稿の版（`status_revision_emojis`）・プロフィールの版（`account_profile_revision_emojis`、`account` の公開 API `IsEmojiReferenced`）・
  リアクション（`reactions.custom_emoji_id`）からも参照されないリモートの絵文字を消します。保持期間（下の「リモート投稿の保持期間」）とプロフィールの整理で版が消えた後に絵文字を残さないためです。
  管理者が無効にした絵文字は、参照が無くても行・画像・設定ごと残します。消すと次に届いたときに有効として記録し直され、`(shortcode, domain)` に対する無効化が失われるからです。
  絵文字ごとに短いトランザクションで、`custom_emojis`・`custom_emoji_images`・設定・別名の行と、画像の `remote_media` / `media_files`（ほかの絵文字の画像・添付・プロフィールのアバターとヘッダーが使っていないもの）を消します。
  `content` の参照は削除の文の中で調べ直します。実行中に届いた文書が消した絵文字を参照したときは、読み取りが参照を飛ばし、次にその絵文字が届いたときに記録し直します。
  1 回のジョブは 100 件ずつ 10 回まで読み、続きはカーソル（絵文字の `id`）を持った次のジョブが読みます。リモートのメディアのキャッシュの複製は期限で消えます。
- リモートの投票で `votersCount` が無いときは、単一選択なら票数の合計を投票者数にし、複数選択なら（誰も投票していなければ 0、それ以外は）NULL にします。
- 投票への回答（FR-112）: ローカルのアカウントは、閲覧できる削除されていない投稿の現在版の投票に、締め切り（`closes_at`）の前に 1 回だけ回答します（`content.Interactions.VotePoll`。交流の禁止 `interact` の対象）。選んだ選択肢ごとに `poll_votes` を 1 行書き、同じトランザクションで既に回答した行が無いことを確かめます。回答は取り消しません。
  リモートの投票への回答は、行ごとに `uri`（`https://{host}/ap/objects/{id}`）を持ち、ドメインイベント `content.poll_voted` で `federation` が選択肢ごとの `Note` を投票の作者に送ります（[federation.md §7.1](../../federation.md#71-記録)）。リモートの投票の票数は相手が送る集計（`poll_tallies`）のままで、回答は相手が次に送る `Update` の集計に入ります。
- ローカルの投票の作成（FR-112）: 投稿（`content.Service.Create`）は投票を 1 つ持てます。選択肢は互いに異なる 2〜4 個（前後の空白を除き、それぞれ 50 文字以内）、期間は 5 分〜30 日（`closes_at` は投稿の時刻に足して秒に切り捨てる）で、複数選択と `hide_totals` を選べます。添付のある投稿には付けられません。
  投稿のトランザクションで、版の `polls` / `poll_options` と、すべての選択肢が 0 票・投票者 0 人の観測を記録します。投票のある投稿は編集できません（回答はその版の投票のものなので、版を作り直すと回答が離れるため）。
- ローカルの投票の集計: ローカルのアカウントの回答（`VotePoll`）とリモートのアカウントの回答（`content.Interactions.RecordRemotePollVote`。`Question` への返信の `Note` の `name` が選択肢の題、[federation.md §3.4](../../federation.md#34-アクティビティごとのハンドラ)）は、
  同じトランザクションで投票の行をロックし（PostgreSQL は `SELECT … FOR UPDATE`。SQLite は書き込みのトランザクションが 1 つずつ）、`poll_votes` を選択肢ごとと投票者ごとに数え直した観測を記録して `content.poll_tally_observed` を発行します。
  リモートの回答は `Note` の `id` を `uri` に持つので、同じ `Note` を 2 回受けても 1 票です。単一選択の投票に同じアカウントが 2 つ目の選択肢を送ったときと、締め切り後・無い選択肢への回答は捨てます。
- 投票の締め切り（FR-112）: 締め切りは時刻だけで決まり（`closes_at` を過ぎた投票は回答を受け付けない）、締め切りを表す行は持ちません。ローカルの投票を投稿したときと、ローカルのアカウントがリモートの投票に回答したときに、
  同じトランザクションで `closes_at` に動くジョブ `content.close_poll`（冪等キーは投票ごと）を outbox に入れます。ジョブは投票が投稿の現在版のものであることを確かめ、回答したアカウントを読んで、ドメインイベント `content.poll_closed` を発行します。
  `notification` はこのイベントで、投票の作者（差出人なし）とローカルの回答者（差出人は作者）に `poll_closed` を作り（冪等キーは投票ごと）、`federation` はローカルの投票の最後の集計の `Update` を送ります。
- タグのモデレーション（使用禁止・トレンド除外）は、v0 では持ちません。必要になったら `tag_moderation_actions` を同じ型で追加します。

#### 利用者ごとの関係【現在値】

| テーブル | カラム | 制約 / インデックス |
| --- | --- | --- |
| `favourites` | `account_id`, `status_id`, `uri`（`Like` の IRI）, `created_at`, `id` | PK `(account_id, status_id)`、一意 `(uri)`、`(account_id, id)`、`(status_id, id)` |
| `bookmarks` | `account_id`, `status_id`, `created_at`, `id` | PK `(account_id, status_id)`、`(account_id, id)`、`(status_id)`（保持期間の判定） |
| `status_pins` | `account_id`, `status_id`, `slot`（`int`、1〜5）, `created_at`, `id` | PK `(account_id, status_id)`、一意 `(account_id, slot)`、`(account_id, id)` |

- お気に入り・ブックマーク・ピン留めは、閲覧できる（`canView`）削除されていない投稿にだけ付けられます。閲覧できない投稿は、存在しない投稿と同じく見つからないものとして扱います。
- どれも `INSERT … ON CONFLICT DO NOTHING` と `DELETE` で書き、実際に行が変わったかどうかを返します。繰り返しても結果は変わりません。
- リモートのアカウントの `status_pins` は、そのアカウントの `featured` コレクションの写しです（[federation.md §6.2](../../federation.md#62-actor-とハンドル)、ADR-0044）。Actor の取得・取り直しのたびに `content.Interactions.ReplaceRemotePins` が 1 つのトランザクションで全部を消して入れ直し（`id` はコレクションの順を保つ）、ローカルのピン留めのコマンドは書きません。上限と `slot` の規則はローカルと同じです。
- ローカルのアカウントのピン留めは自分の投稿だけで、`direct` は除きます。1 アカウント 5 件までで、各ピン留めは空いている最小の `slot` に入ります。一意 `(account_id, slot)` と `slot` の CHECK が上限をスキーマで保証するので、同時のピン留めでも 5 件を超えません。同時の挿入に `slot` を取られたら（行が増えず、そのピン留めも無く、空きがあるとき）次の空きで挿入し直し、空きがなければ上限超過として拒否します。
- 投稿が削除されても行はすぐには消さず、消去ジョブが物理削除します（§8）。削除された投稿は `status_counters` を持たないので、件数は変わりません。
