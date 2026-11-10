## 6. タイムラインのクエリ

v0 は fan-out-on-read（ADR-0013）です。`timeline` モジュールの投影だけを読みます。返す直前にドメインの `canView`（[公開範囲の閲覧判定](../visibility.md)）を再適用します（多重防御、NFR-SEC-4）。以下は方言に依存しない概略で、実際のクエリは `queries/<dialect>/timeline/timeline.sql` です。

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
- フォローしていない相手からの `direct` 投稿（自分宛て）は、ホームではなくダイレクトメッセージの一覧（§6.4）で読みます。
- **読み取り窓**: `:window_floor` は「基準時刻 − 窓」の UUIDv7 の下限です（§1.3）。基準時刻は現在時刻で、カーソルがあればカーソルの `id` の時刻です。
  窓の既定値は 7 日（`timeline.HomeWindow`）です。ページが埋まらなければ、窓を 4 倍（`timeline.HomeWindowGrowth`）に広げて読み直すことを 3 回（`timeline.HomeWindowWidenings`）まで行い、それでも埋まらなければ下限なしで読みます。

- **フォローしたハッシュタグ**（ADR-0013 §24〜§28）: ホームは、上のフォローの側の枝に、フォローしたハッシュタグの枝を足して `id` で合わせます。
  ハッシュタグの枝は、`feed_tag_follows` の閲覧者のタグごとに `feed_tag_entries (tag_id, entry_id)` の範囲を同じカーソルと読み取り窓で読み、`feed_entries` と結合します。

  ```sql
  SELECT e.*, array_agg(t.tag_id ORDER BY t.tag_id) AS tag_ids  -- SQLite は group_concat
  FROM feed_tag_entries t
  JOIN feed_entries e ON e.id = t.entry_id
  WHERE t.tag_id IN (SELECT tag_id FROM feed_tag_follows WHERE account_id = :viewer)
    AND t.visibility IN ('public', 'local')
    AND t.entry_id < :cursor AND t.entry_id >= :window_floor
    AND e.kind = 'post'
    AND t.author_account_id <> :viewer
    AND NOT EXISTS (SELECT 1 FROM feed_follow_edges f
          WHERE f.follower_account_id = :viewer AND f.followee_account_id = t.author_account_id)
    AND /* 作者の public_discovery・visibility_ceiling_private・profile_view、ドメインの public_discovery・profile_view（§6.3 のハッシュタグと同じ） */
    AND /* 閲覧者のブロック・ミュート（feed_exclusions）、ドメインのミュート（feed_domain_mutes）、言語フィルタ（ホームと同じ） */
  GROUP BY e.id
  ORDER BY e.id DESC
  LIMIT :limit + 1
  ```

  - 作者をフォローしている投稿と自分の投稿はフォローの側の枝だけが返すので、2 つの枝は重なりません。同じ投稿が複数のフォローしたタグを持つときは `GROUP BY` で 1 行にし、合ったタグをまとめて返します。
  - 2 つの枝をそれぞれ `ORDER BY id DESC LIMIT :limit + 1` で読み、アプリケーションで `id` の大きい順に合わせて `:limit + 1` 件に切ります（同じ `id` はフォローの側の行を残します）。読み取り窓を広げるとき（上の読み取り窓）は、両方の枝を同じ窓で読み直します。
  - 使うインデックス: `feed_tag_follows` の PK、`feed_tag_entries (tag_id, entry_id)` の PK か `(entry_id)`、`feed_entries` の PK。
  - ハッシュタグの枝の行は、`content.CanView` を `ContextHashtag` で当て直します（フォローの側の行は `ContextHome`）。作者をフォローしていなくても見えるのは、ハッシュタグのタイムラインで見える投稿だからです。
  - ハッシュタグの枝からだけ入った行には、合ったフォロー中のタグを付けて返します（ADR-0013 §28）。ストリームの新着の判定（`HomeEntry`）も同じ 2 つの枝を読み、配る相手の絞り込み（`HomeAudience`）は、作者のフォロワーに加えて、投稿のタグ（`public` / `local` のとき）をフォローしている人を `feed_tag_entries (entry_id)` と `feed_tag_follows` の PK で選びます。
  - **計測**（#2571）: 30 日で 30 万件の投稿（2 万人、2 割がローカル、1 割が `unlisted`）、3 割の投稿に 2,000 個のタグのどれか、5% に賑やかなタグ（1 日 500 件）。閲覧者はフォロー 1,000・フォローしたタグ 100、ほかに 500 人がフォロー 200・タグ 20 を持ちます。21 件の読み取りの中央値（21 回）です。

    | 読み方 | SQLite: フォローの枝 | SQLite: ハッシュタグの枝 | PostgreSQL: フォローの枝 | PostgreSQL: ハッシュタグの枝 |
    | --- | --- | --- | --- | --- |
    | 1 ページ目（窓 7 日）、賑やかなタグを含む | 5.4 ms | 22.9 ms | 12.1 ms | 6.2 ms |
    | 3 日前のカーソル（窓 7 日）、賑やかなタグを含む | 5.8 ms | 23.1 ms | 15.3 ms | 5.7 ms |
    | 下限なし、賑やかなタグを含む | 6.6 ms | 106.9 ms | 10.7 ms | 5.3 ms |
    | 1 ページ目（窓 7 日）、静かなタグ 100 | 5.5 ms | 7.0 ms | 10.6 ms | 5.6 ms |
    | 下限なし、静かなタグ 100 | 5.2 ms | 32.3 ms | 10.7 ms | 5.7 ms |

    - SQLite の計画は `SEARCH t USING INDEX sqlite_autoindex_feed_tag_entries_1 (tag_id=? AND entry_id>?)`（`LIST SUBQUERY` が `feed_tag_follows` の PK を `account_id=?` で読む）、`SEARCH e USING INDEX sqlite_autoindex_feed_entries_1 (id=?)`、`USE TEMP B-TREE FOR GROUP BY` です。
      タグごとに窓の中の範囲をすべて読んでから並べるので、読む行は窓の中のフォローしたタグの投稿の数（賑やかなタグで約 3,500 行、静かなタグ 100 で約 1,000 行）に比例します。
    - PostgreSQL の計画は `Index Scan Backward using feed_tag_entries_entry_id` で新しい順に読み、各行のタグを `feed_tag_follows_pkey`（`Memoize`）で確かめ、21 件で止まります。賑やかなタグで 173 行、静かなタグ 100 で 935 行を読みました（`EXPLAIN (ANALYZE, BUFFERS)`、それぞれ 0.8 ms と 1.8 ms）。
    - 両方の枝を足しても、窓の中の読み取りは 30 ms 以下で、ホームの予算（NFR-PERF-3 の p95 300 ms）に収まります。下限なしは、窓を広げてもページが埋まらないとき（フォローしたタグの投稿が少ないとき）だけ読みます。
      ハッシュタグの枝の窓を狭める・件数を限る・投影を作る手当て（ADR-0013 §29）は、今は要りません。

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

- 連合タイムラインは `kind = 'post' AND visibility = 'public'` の部分インデックスを使い、`feed_domain_restrictions`（`public_discovery`・`profile_view`）も除外します。
  閲覧者がミュートしたドメイン（`feed_domain_mutes`。サブドメインを含む）の投稿も除外します。ローカルタイムラインはリモートの投稿を含まないので、ドメインのミュートは関係しません。

### 6.3 ハッシュタグ・プロフィール・スレッド

- **ハッシュタグ（FR-203）**: `feed_tag_entries (tag_id, entry_id)` を `visibility IN ('public','local')`（未ログインは `public`）で絞り、言語のために `feed_entries` と結合します。
  作者の `public_discovery`・`visibility_ceiling_private`・`profile_view` は `feed_author_restrictions` で、作者のドメインの `public_discovery`・`profile_view` は `feed_domain_restrictions` で、閲覧者のブロック・ミュートは `feed_exclusions` で、閲覧者がミュートしたドメインは `feed_domain_mutes` で除外します。
- **プロフィール（FR-204）**: `feed_entries (author_account_id, id)` を使い、作者の投稿とブーストを読みます。
  許される公開範囲の集合は、閲覧者と作者の関係（本人・承認済みフォロワー・ブロック）と作者の制限を `canView`（`ContextProfile`）に公開範囲ごとに与えて求め、`IN (…)` に入れます。
  `direct` は本人なら全部、それ以外は自分宛てのもの（`feed_direct_recipients`）だけです。ブーストは、元投稿の作者を閲覧者が除外している・元投稿の作者やそのドメインに `profile_view` がある・元投稿の作者のドメインを閲覧者がミュートしているものを除きます。
  返信を除くときは `NOT is_reply`、ブーストを除くときは `kind = 'post'` で絞ります。言語フィルタは掛けません（プロフィールは作者のすべての言語を見せる）。
  ピン留めした投稿は `timeline` ではなく `content` が `status_pins` から読み、投稿ごとに `canView`（`ContextStatus`）を適用します。
- **スレッド（FR-106）**: 起点の投稿の会話の投稿を `status_conversations (conversation_id, status_id)` で一括取得し、行ごとに `canView` を適用します。祖先と子孫は `status_reply_links` でたどり、返信のつながりの無い投稿は出しません（[ADR-0154](../../adr/0154-conversations.md) §1）。
- **投稿の全文検索（FR-309）**: 候補はこの節のクエリではなく検索エンジンが返す投稿の ID で、`feed_entries` を読みません。投稿ごとに `canView`（`ContextSearch`）を適用し、ブロック・ミュートと制限はホームと同じ `feed_exclusions` と `feed_*_restrictions`、閲覧者がミュートしたドメインは作者のドメインとその親を `feed_domain_mutes` と照らして除外します（フォローしている相手も除外します。[ADR-0149](../../adr/0149-full-text-search.md) §8）。

### 6.4 ダイレクトメッセージ

- 一覧（FR-212）は、閲覧者が送った・受け取った `direct` の投稿を、会話（`status_conversations` の `conversation_id`）ごとにまとめたものです。フォローしているかによりません。UI の名前は `en` で「Direct messages」、`ja` で「DM」です（[ADR-0154](../../adr/0154-conversations.md) §6）。
- 一覧の投影は `timeline` モジュールが持ち、会話の ID を鍵にします（§4.6 の「ダイレクトメッセージ」）。宛先は `feed_direct_recipients` と同じ現在版のメンションから得ます。
- `content.conversation_merged` を受けたら、併合された会話の行を併合先へ移し、既読の境界と非表示の境界は 2 つのうち新しい方を残します。併合された会話の ID を指す要求には、併合先の会話で答えます（`content` の `ResolveConversation` で併合先をたどります）。

1 ページは、会話の数によらず次の決まった数のクエリで読みます。

```sql
-- 1. ページ: 閲覧者の会話を、閲覧者の最新の direct の投稿の新しい順に、非表示を除いて
SELECT c.conversation_id, c.last_entry_id, r.entry_id AS read_entry_id
FROM feed_conversations c
LEFT JOIN feed_conversation_reads r ON r.account_id = c.account_id AND r.conversation_id = c.conversation_id
WHERE c.account_id = :viewer AND c.last_entry_id < :cursor
  AND NOT EXISTS (SELECT 1 FROM feed_conversation_hides h
        WHERE h.account_id = c.account_id AND h.conversation_id = c.conversation_id AND h.entry_id >= c.last_entry_id)
ORDER BY c.last_entry_id DESC LIMIT :limit + 1;

-- 2. ページの会話ごとに、見られる最新の投稿と、そのうちほかの人の最新の投稿
SELECT p.conversation_id, max(e.id) AS last_entry_id,
       max(CASE WHEN e.author_account_id <> :viewer THEN e.id END) AS last_other_entry_id
FROM feed_conversation_participants p JOIN feed_entries e ON e.id = p.entry_id
WHERE p.account_id = :viewer AND p.conversation_id IN (:page)
  AND NOT EXISTS (/* 閲覧者のブロック・ミュート（feed_exclusions）を作者に */)
  AND NOT EXISTS (/* 作者の profile_view（feed_author_restrictions）、作者のドメインの profile_view（feed_domain_restrictions） */)
GROUP BY p.conversation_id;

-- 3. ページの会話ごとの相手: 閲覧者が当事者の投稿の、ほかの当事者
SELECT DISTINCT o.conversation_id, o.account_id
FROM feed_conversation_participants v
JOIN feed_conversation_participants o ON o.conversation_id = v.conversation_id AND o.entry_id = v.entry_id
WHERE v.account_id = :viewer AND v.conversation_id IN (:page) AND o.account_id <> :viewer;
```

- 2 の投稿は `feed_entries` を ID でまとめて読み、ほかのタイムラインと同じく `Facts` をページの行についてまとめて読んで、`canView`（`ContextDirectMessages`。`direct` だけを作者と宛先に見せる。[visibility.md](../visibility.md) §3）を再適用します。拒んだ会話はページから省きます。
  最新の投稿が隠れている（ミュート・ブロック・作者の制限）会話は、それより前の見られる投稿を見せ、見られる投稿が無い会話は省きます。省いてもページは詰めず、次のページのカーソルは読んだ最後の行の `last_entry_id` です。
- 並びは閲覧者の最新の `direct` の投稿（`last_entry_id`）の順で、見せる投稿がそれより前のこともあります。タイムラインの言語（FR-208）は掛けません。
- 未読は、2 の `last_other_entry_id` が既読の境界より新しいときです。閲覧者自身の投稿は未読にしません。
- 非表示の会話は、境界より新しい投稿が届くと `last_entry_id` が進んで一覧に戻ります。単体の取得（`GET /api/v1/conversations/{id}`）は非表示でも返します。
- 使うインデックス: `feed_conversations (account_id, last_entry_id)`、`feed_conversation_participants` の PK と `(conversation_id, entry_id)`、`feed_entries` の PK、`feed_exclusions` の PK。
- ストリーム（FR-207）: `direct` の投稿の作成をコミットした後、当事者ごとに `stream.KindConversation`（会話の ID）を出し、既読にした後は閲覧者に出します。受けたストリームは、その会話を一覧と同じ規則で読み直して `conversation` イベントで送ります。一覧が出さない会話は送りません。
- メンションの通知はこれまでどおりで、`direct` の投稿も宛先に `mention` の通知を作ります（`notification` の projector）。
