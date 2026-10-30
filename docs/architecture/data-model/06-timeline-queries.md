## 6. タイムラインのクエリ

v0 は fan-out-on-read（ADR-0013）です。`timeline` モジュールの投影だけを読みます。返す直前にドメインの `canView`（[公開範囲の閲覧判定](../visibility.md)）を再適用します（多重防御、NFR-SEC-4）。以下は方言に依存しない概略で、実際のクエリは `queries/<dialect>/timeline.sql` です。

共通の規則:

- ページングは `id` のキーセット（§1.3）で、`:limit + 1` 件を読んで次のページの有無を決めます。次のページのカーソルは、`canView` で除いた行も含めて読んだ最後の行の `id` です（除いた結果、ページが `:limit` 件より少なくなることがあります）。
- 期限つきの除外・制限は、`expires_at` / `effective_until` が NULL か `:now` より後のものだけが効きます。
- ドメイン単位の制限は、そのドメインとサブドメイン（`author_domain = d.domain` または `author_domain` が `'.' || d.domain` で終わる）に効きます。
- **言語フィルタ（FR-208）**: 言語の一覧を渡すと `language IN (:languages)` に加えて `language = 'und'`（不明）も通します。空なら絞り込みません。一覧は呼び出し側が `identity` の `user_timeline_languages` から渡します（`timeline` は `identity` のテーブルを読みません）。
- `canView` の再適用: 投稿は投稿として、ブーストは「ブーストした人の、元投稿と同じ公開範囲の投稿」として文脈ごとに判定し、さらに元投稿を単体の取得として判定します。閲覧者との関係（承認済みのフォロー・ブロック・宛先か）と作者の制限は、ページの行について `feed_*` からまとめて読みます。ミュートは `canView` の外の絞り込みですが、同じ段階でも除きます。

### 6.1 ホーム（FR-201）

```sql
SELECT e.*
FROM feed_entries e
WHERE e.author_account_id IN (
        SELECT followee_account_id FROM feed_follow_edges WHERE follower_account_id = :viewer
        UNION ALL SELECT :viewer)
  AND e.id < :cursor
  AND e.id >= :window_floor                       -- uuidv7_floor(基準時刻 - 読み取り窓)
  AND (e.kind = 'post' OR e.author_account_id = :viewer OR EXISTS (SELECT 1 FROM feed_follow_edges f
         WHERE f.follower_account_id = :viewer AND f.followee_account_id = e.author_account_id AND f.show_boosts))
  AND (e.visibility <> 'direct' OR e.author_account_id = :viewer
       OR EXISTS (SELECT 1 FROM feed_direct_recipients d
                  WHERE d.recipient_account_id = :viewer AND d.entry_id = e.id))
  AND NOT EXISTS (SELECT 1 FROM feed_exclusions x
         WHERE x.viewer_account_id = :viewer
           AND x.target_account_id IN (e.author_account_id, e.original_author_account_id)
           AND (x.expires_at IS NULL OR x.expires_at > :now))
  AND NOT EXISTS (SELECT 1 FROM feed_author_restrictions r
         WHERE r.account_id IN (e.author_account_id, e.original_author_account_id)
           AND r.restriction = 'profile_view'
           AND (r.effective_until IS NULL OR r.effective_until > :now))
  AND NOT EXISTS (/* feed_domain_restrictions の profile_view を author_domain と original_author_domain に */)
  AND /* 言語フィルタ */
ORDER BY e.id DESC
LIMIT :limit + 1
```

- 使うインデックス: `feed_entries (author_account_id, id)`、`feed_follow_edges (follower_account_id, followee_account_id)`、`feed_exclusions` の PK。
- フォローしている相手の投稿は、`public` / `local` / `unlisted` / `private` のどれでも表示できます（閲覧者はフォロワーかつローカル）。自分の投稿とブーストも表示します。
- ブーストは、ブーストした人と元投稿の作者（`original_author_account_id`）の両方について、ブロック・ミュート・制限で除外します。閲覧者がミュートしたドメイン（`feed_domain_mutes`）の投稿のブーストも除外しますが、フォローしているそのドメインのアカウントの投稿は除外しません（§4.5）。
- フォローしていない相手からの `direct` 投稿（自分宛て）は、ホームではなく会話一覧で `feed_direct_recipients (recipient_account_id, entry_id)` から読みます。
- **読み取り窓**: `:window_floor` は「基準時刻 − 窓」の UUIDv7 の下限です（§1.3）。基準時刻は現在時刻で、カーソルがあればカーソルの `id` の時刻です。
  窓の既定値は 7 日（`timeline.HomeWindow`）です。ページが埋まらなければ、窓を 4 倍（`timeline.HomeWindowGrowth`）に広げて読み直すことを 3 回（`timeline.HomeWindowWidenings`）まで行い、それでも埋まらなければ下限なしで読みます。

- **リスト（FR-211）**: ホームと同じ条件（読み取り窓・`show_boosts`・`direct` の宛先・ブロックとミュート・`profile_view`・ドメインのミュート・言語）を、作者（投稿者またはブーストした人）をそのリストのメンバーに限って当てます。
  作者の集合は `list_members` を `feed_follow_edges` と結合した `IN (SELECT member_account_id … WHERE list_id = :list AND owner_account_id = :viewer)` で、閲覧者自身の投稿とブーストは入れません（`e.author_account_id <> :viewer`）。
  リストの持ち主でない閲覧者には、クエリの前にリストが無いものとして答えます。

### 6.2 ローカル / 連合（FR-202）

```sql
SELECT e.* FROM feed_entries e
WHERE e.is_local AND e.kind = 'post'
  AND e.visibility IN ('public', 'local')          -- 未ログインは 'public' だけ（ADR-0017）
  AND e.id < :cursor
  AND NOT EXISTS (SELECT 1 FROM feed_author_restrictions r
        WHERE r.account_id = e.author_account_id
          AND r.restriction IN ('public_discovery', 'visibility_ceiling_private', 'profile_view')
          AND (r.effective_until IS NULL OR r.effective_until > :now))
  AND NOT EXISTS (SELECT 1 FROM feed_exclusions x  -- 閲覧者のブロック・ミュート（未ログインなら無し）
        WHERE x.viewer_account_id = :viewer AND x.target_account_id = e.author_account_id
          AND (x.expires_at IS NULL OR x.expires_at > :now))
  AND /* 言語フィルタ */
ORDER BY e.id DESC LIMIT :limit + 1;
```

- 連合タイムラインは `kind = 'post' AND visibility = 'public'` の部分インデックスを使い、`feed_domain_restrictions`（`public_discovery`・`visibility_ceiling_private`・`profile_view`）も除外します。
  閲覧者がミュートしたドメイン（`feed_domain_mutes`。サブドメインを含む）の投稿も除外します。ローカルタイムラインはリモートの投稿を含まないので、ドメインのミュートは関係しません。

### 6.3 ハッシュタグ・プロフィール・スレッド

- **ハッシュタグ（FR-203）**: `feed_tag_entries (tag_id, entry_id)` を `visibility IN ('public','local')`（未ログインは `public`）で絞り、言語のために `feed_entries` と結合します。
  作者の `public_discovery`・`visibility_ceiling_private`・`profile_view` は `feed_author_restrictions` と `feed_domain_restrictions`（作者のドメイン）で、閲覧者のブロック・ミュートは `feed_exclusions` で、閲覧者がミュートしたドメインは `feed_domain_mutes` で除外します。
- **プロフィール（FR-204）**: `feed_entries (author_account_id, id)` を使い、作者の投稿とブーストを読みます。
  許される公開範囲の集合は、閲覧者と作者の関係（本人・承認済みフォロワー・ブロック）と作者の制限を `canView`（`ContextProfile`）に公開範囲ごとに与えて求め、`IN (…)` に入れます。
  `direct` は本人なら全部、それ以外は自分宛てのもの（`feed_direct_recipients`）だけです。ブーストは、元投稿の作者を閲覧者が除外している・元投稿の作者やそのドメインに `profile_view` がある・元投稿の作者のドメインを閲覧者がミュートしているものを除きます。
  返信を除くときは `NOT is_reply`、ブーストを除くときは `kind = 'post'` で絞ります。言語フィルタは掛けません（プロフィールは作者のすべての言語を見せる）。
  ピン留めした投稿は `timeline` ではなく `content` が `status_pins` から読み、投稿ごとに `canView`（`ContextStatus`）を適用します。
- **スレッド（FR-106）**: `statuses (conversation_uri, id)` で一括取得し、行ごとに `canView` を適用します。親子関係は `status_reply_links` を使います。
