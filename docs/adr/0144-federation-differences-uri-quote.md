# ADR-0144: 連合の URI と引用の、ADR-0018・ADR-0069 と異なる決定

- Status: Proposed
- Date: 2026-11-03
- Deciders: @rosylilly
- Issue: #2079

## Context（背景・課題）

`v0.1.0` の前の連合の監査（#2027）で、Accepted の [ADR-0018](0018-federated-uris.md)（連合 URI）と [ADR-0069](0069-quote-posts.md)（引用投稿）が、コードと食い違っていると分かりました。URI と形式はタグの後に変えにくいので、いま片方に揃えます。Accepted の ADR の本文は書き換えられないので、違いをこの ADR に置きます。

監査で見つかり、決めるものは次のとおりです。

| 項目 | ADR の記述 | コード |
| --- | --- | --- |
| §1 削除された Actor の GET | ADR-0018 に記述がない。投稿は削除で `410` と `Tombstone` | `404`（`internal/interface/activitypub/actor.go`、`webfinger.go`） |
| §2 引用のフォールバック | ADR-0069 §3: 本文の**末尾**に `<span class="quote-inline"><br>RE: …</span>` | 本文の**先頭**に `<p class="quote-inline">RE: …</p>`（`internal/domain/federation/document.go`、federation.md §7.4） |
| §3 `/ap/*` への HTML の要求 | ADR-0018: Cookie のセッションの閲覧者に `canView` で権限があれば `303` | ブラウザをすべて未ログインとして扱う（`internal/interface/activitypub/object.go`、federation.md §10） |
| §4 引用元の公開範囲 | ADR-0069 §4: 引用元の公開範囲は自由。§3: リモートの投稿の引用には `QuoteRequest` を送る | `local` の投稿の IRI と作者が、作者のフォロワーのいないサーバーに届く（#2066） |
| §5 配信する URI | ADR-0018 の一覧: `/ap/activities/{uuid}`、`{object}/replies`・`/likes`・`/shares` | どれも配信していない |

確認した外部の事実は次のとおりです（2026-11-03 に確認）。

- Mastodon v4.7.4 の [`text_formatter.rb`](https://github.com/mastodon/mastodon/blob/v4.7.4/app/lib/text_formatter.rb) は、本文の前に `<p class="quote-inline">RE: …</p>` を付けます。
- [FEP-044f](https://codeberg.org/fediverse/fep/src/branch/main/fep/044f/fep-044f.md) の `<span class="quote-inline">` は Backward compatibility の節と例にあるもので、規範ではありません。例は、フォールバックの位置は埋め込みの表示位置に影響しないと述べます。
- Mastodon v4.7.4 は、削除を待つ Actor の GET に `403`、消去の終わった Actor の GET に `410` を返します（[`account_owned_concern.rb`](https://github.com/mastodon/mastodon/blob/v4.7.4/app/controllers/concerns/account_owned_concern.rb)、[`suspensions.rb`](https://github.com/mastodon/mastodon/blob/v4.7.4/app/models/concerns/account/suspensions.rb)）。リモートの WebFinger の `410` を受けると、そのアカウントを削除します（[`resolve_account_service.rb`](https://github.com/mastodon/mastodon/blob/v4.7.4/app/services/resolve_account_service.rb)）。
- このサーバーの受信は、リモートの Actor の `410` を `remote_deleted` にします（federation.md §6）。

## Decision Drivers（判断基準）

1. **`local` の約束を守ること**（ADR-0017）: 投稿の中身だけでなく、存在と作者も、外部のフォロワーのいないサーバーへ出さないこと。
2. **実際の相手と揃うこと**（ADR-0069 の判断基準 3）: 規範でない例より、動いている実装に合わせること。
3. **タグの後に残る決定から先に閉じること**: 形式と URI。応答のコードやルートの追加は、後から変えても id が変わりません。
4. **層の分離**（ADR-0002）: ActivityPub の入口が Web のセッションに依存しないこと。
5. **実装を増やさないこと**: 使われない分岐を持たないこと。

## Considered Options（選択肢）

### §1 削除された Actor

1. **A1: `404` のまま記録する** — 消えたことと、無かったことを区別しません。
2. **A2: `410` を返す** — 監査が提案した案で、`Delete` の配送が終わった後の Actor の GET に `410` と `Tombstone` を返します。`Delete` を受け損ねたサーバーが、消えたと分かります。投稿とも、ADR-0088 のクローズとも揃います。

### §2 引用のフォールバック

1. **B1: コードの形を記録する** — 先頭の `<p class="quote-inline">`（Mastodon の形）。
2. **B2: ADR-0069 の形に戻す** — 末尾の `<span>`（FEP-044f の例の形）。

### §3 HTML のリダイレクト

1. **C1: 未ログインとして扱うことを記録する** — 未ログインで閲覧できる投稿だけ `303`、それ以外は `404` です。
2. **C2: セッションを読む** — ADR-0018 のとおりです。ログイン中の人が、`local` の投稿の `/ap/` の URL を開けます。

### §4 引用元の公開範囲

1. **D1: `local`・`private`・`direct` の投稿からリモートの投稿を引用させない。`local` の投稿の引用は連合しない**
2. **D2: `QuoteRequest` だけを止める** — 引用はでき、状態は `pending` のままです。
3. **D3: ADR-0069 のまま** — 中身は守られているので、IRI と作者は漏れてもよいとします。

### §5 配信する URI

1. **E1: 公開・未収載のブーストの `Announce` だけを配信する。残りは `404` のままにする**
2. **E2: すべての活動を配信する** — ActivityPub §3.1 の「id は取得できる」に最も忠実です。

## Decision（決定）

> 推奨案です。Accepted にするのは人間の Decider だけです。

**A1・B1・C1・D1・E1 を推奨します。** B1・C1・D1・E1 は、監査の後に決定者が示した方向です。A1 は下の理由で、決定者の確認がいります。

### 1. 削除された Actor は `404`（A1）

削除されたローカルの Actor の GET と WebFinger は `404` です。削除された投稿の `410` と `Tombstone`、クローズしたサーバーの `410` は、そのままです。

- 決定者が記録するものとして挙げたのは `404` です。
- 受け損ねたサーバーに残るのは表示だけで、`Delete` は配送のリトライ（§7.2）と既知の shared inbox で届きます。
- A2 の利点は本物です。Mastodon は WebFinger の `410` でそのアカウントを消すので、`Delete` を受け損ねたサーバーの残骸を減らせます。`404` から `410` への変更は id を変えず、タグの後でも形式の互換を壊しません。そのため、いま決めなくても失うものが小さいと判断します。

**見直す条件**: 相互運用の確認（#2029）か運用で、削除された Actor が相手に残り続けると分かったとき。そのときは Actor の GET と WebFinger に `410` を足します（判断基準 2）。

### 2. 引用のフォールバックは先頭の `<p>`（B1）

`quote`（または方言）を付けた Note の本文の先頭に、`<p class="quote-inline">RE: <a href="…">…</a></p>` を付けます。ADR-0069 §3 の末尾の `<span>` を置き換えます。Mastodon v4.7.4 と同じ形で、FEP-044f の形は規範ではありません（判断基準 2）。

B2 の強みは FEP の例と一致することです。ただし読み手は `quote-inline` の class を見ており、位置に意味はありません。

### 3. HTML の要求はセッションを読まない（C1）

`/ap/*` に HTML を求める要求は未ログインとして扱い、未ログインで閲覧できる投稿だけを表示用 URL へ `303` で送ります。それ以外は `404` です。ADR-0018 の「閲覧者（Cookie のセッション）」を置き換えます。

- `/ap/*` はサーバーどうしの URI で、クライアントの UI からリンクしません（`url` が表示用 URL です）。ログイン中の人が `/ap/` の URL を手で開く場面のために、ActivityPub の入口へセッションの依存を足しません（判断基準 4・5）。
- 代償として、ログイン中の人でも `local` の投稿の `/ap/` の URL は `404` です。表示用 URL は開けます。

### 4. 引用元の公開範囲（D1）

ADR-0069 §3・§4 の「引用元の公開範囲は自由」を、次のとおり狭めます。

- `local`・`private`・`direct` の投稿は、リモートの投稿を引用できません。投稿の作成で断ります（`quoteTarget`）。`QuoteRequest` の `instrument` が、引用元の IRI を宛先の外へ知らせるためです。
- ローカルの `local` の投稿を `public` や `unlisted` で引用することは、引き続きできます。その Note を連合するときは、`quote`・併記のプロパティ・`quoteAuthorization`・フォールバックを除きます。ローカルの閲覧者には埋め込みを出します。
- その引用の `QuoteAuthorization` の GET は `404` です。

「外部へはフォロワーにしか届かない」（ADR-0017）は、投稿の中身だけでなく、その IRI と作者のハンドルも含むものとします（判断基準 1）。D2 は `pending` のまま終わらない引用を残し、D3 は判断基準 1 を説明で済ませるだけです。

### 5. 配信する URI（E1）

`/ap/activities/{uuid}` は、公開・未収載のブーストの `Announce` だけを、保存している事実から配信します。残りの活動・`/ap/objects/{id}/quote_request`・`{object}/replies`・`/likes`・`/shares` は `404` で、広告もしません。規則は federation.md §10 に書きます（#2068）。

ADR-0018 の一覧は「id の形」として残り、そこに配信の約束はありません。E2 は規範に最も近いですが、Like や Follow の取得を使う相手は確かめられておらず、公開範囲の判定を活動ごとに増やします（判断基準 5）。後からルートを足しても id は変わりません（判断基準 3）。

## Consequences（影響）

- 良い影響
  - ADR と federation.md とコードが一致します。
  - `local` の投稿の存在と作者が、フォロワーのいないサーバーへ出なくなります。
- 悪い影響・トレードオフ
  - `Delete` を受け損ねたサーバーは、削除された Actor を区別できません。投稿は `410`、Actor は `404` で揃いません。
  - `local`・`private`・`direct` の投稿から、リモートの投稿を引用できません。
  - `local` の投稿を引用した公開の投稿は、連合先ではただの投稿に見えます。
  - 活動の id の多くは取得できません（ActivityPub §3.1 の例外として扱います）。
- 後続で必要になる作業（Issue 化する）
  - #2066: §4 の実装。#2068: §5 の実装。
  - ADR-0018 と ADR-0069 の Status に、この ADR が置き換える部分を書きます（Accepted になるとき）。
  - A2 を選ぶなら、Actor の GET と WebFinger の `410` の issue を足します。

## Open Questions / 要検証

- 要検証: Misskey・Akkoma・GoToSocial が先頭の `<p class="quote-inline">` をどう表示するか。ソースもライブの相互運用も確かめていません（`federation-live` で確かめる）。
- 要検証: Mastodon 以外が、Actor の `404` と `410` をどう区別するか。
