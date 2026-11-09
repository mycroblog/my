# ADR-0160: 記事（Markdown の長い投稿）を投稿の一種として持つ

- Status: Proposed
- Date: 2026-11-09
- Deciders: @rosylilly
- Issue: #2590

## Decision（決定）

決定者は §1〜§12 のすべてを決めた。Status を Accepted にするのは決定者である。
ほかのサーバーの受け取り方はソースコードを読んで確かめたもの（§9）で、実地の確認は #2592 で行う。そこで違いが見つかれば、§5 の上限と §9 の文書の形を見直す。

### 1. 記事は投稿の一種である

- 記事は、題・要約・カバー画像を持ち、本文を Markdown で書く長い投稿である。ブログや日記のように読まれ、ほかのサーバーへも届く。
- 記事は `statuses` の 1 行で、版（`status_revisions`）に記事の子の行（題・要約・カバー）を足したものである。公開範囲・編集の履歴・リアクション・お気に入り・ブースト・引用される・返信される・タイムライン・通報・処分・削除は、投稿と同じ経路で働く。
- 投稿か記事かは作るときに決まり、編集で変わらない。

### 2. テーブル（data-model §4.4「記事」）

所有するモジュールは `content` である。JSON は使わない（ADR-0020 規約 4）。

| テーブル | 分類 | 中身 |
| --- | --- | --- |
| `status_articles` | リソース（部分型） | `status_id`（PK）, `author_account_id`。記事である投稿。インデックス `(author_account_id, status_id)` はプロフィールの記事の一覧と記事だけのフィード（§11）に使う |
| `status_revision_articles` | イベント（版の子） | `revision_id`（PK）, `title`, `summary`, `summary_origin`（`enum(author, body)`） |
| `status_revision_article_covers` | イベント（版の子） | `revision_id`（PK）, `media_id`, `description` |
| `status_revision_article_images` | イベント（版の子） | `revision_id`, `position`, `media_id`, `description`。本文の中の画像 |
| `status_revision_sources` | イベント（部分型） | `source_format` を `enum(text_plain, text_markdown)` に広げる |
| `draft_articles` | 現在値 | `draft_id`（PK）, `title`, `summary`, `cover_media_id`（NULL 可）, `cover_description`（NULL 可） |
| `draft_article_tags` | 現在値 | `draft_id`, `position`, `name`。エディタで付けたハッシュタグ |
| `draft_article_images` | 現在値 | `draft_id`, `media_id`。本文が参照するアップロード |

- 種類を列ではなく部分型のテーブルの有無で表す（ADR-0020 規約 4）。`status_articles` は投稿の不変の事実で、すべての版が `status_revision_articles` を持つことを `content` のユースケースが守る。
- 記事の版は `status_revision_attachments`・投票・CW（`spoiler_text` は空）を持たない。本文の HTML は投稿と同じ `status_revision_bodies` に置く。
- 下書きは ADR-0156 のテーブルを共有し、記事の下書きは `drafts` に `draft_articles` の行を足したものである。本文の Markdown は `draft_contents.source_text` に置く。予約（ADR-0156 §5）は記事にも同じに働く。
  `media.purge_unattached` は `draft_article_images` と `draft_articles.cover_media_id` の参照も添付として数える。
- `summary_origin = body` は、作者が要約を空にしたので本文から作った要約である（§6）。編集で本文を変えると作り直す。

### 3. 書ける人

- 既定では誰でも記事を書ける。運営者はインスタンス設定 `article_authoring`（`enum(everyone, permission)`、既定 `everyone`）で、権限 `create_articles` を持つロールに限れる。
- `create_articles` は、課金のプランに結び付けられるロールが持てる権限（FR-912）に加える。
- 限ったあとも、既にある記事は消さず、編集もできる。新しく書けないだけである（処分の `post` の制限は投稿と同じに記事の作成と編集を拒む）。

### 4. Markdown の描画

- 書いたときに 1 回だけ描画し、HTML を `status_revision_bodies` に保存する。読むたびに描画しない。原文は `status_revision_sources`（`text_markdown`）に残す。
- 描画は `internal/adapter/markdown`（goldmark）が、記事用のポート（`content` の記事の描画）を実装して行う。CommonMark と、GFM の表・取り消し線・自動リンクを使う。
- 原文の HTML は描画しない（goldmark のエスケープに任せ、残ったものはサニタイザが落とす）。段落の中の改行は `<br>` にする。見出しは 1 段下げ、`#` が `<h2>`、`#####` 以下は `<h6>` になる（記事の題がページの `<h1>` である）。
- 描画した HTML は、記事用の許可リスト（`internal/adapter/htmlsanitizer`、投稿や規約の文書とは別）に通す。
  - 要素: `p` `br` `hr` `h2`〜`h6` `strong` `em` `del` `code` `pre` `blockquote` `ul` `ol` `li` `a` `img` `table` `thead` `tbody` `tr` `th` `td` `span`
  - 属性: `a[href rel class target]`、`img[src alt width height loading]`、`ol[start]`、`th[align]`・`td[align]`、`span[class]`（`mention` `hashtag` `invisible` `ellipsis` だけ）
  - `a` には投稿と同じく `rel="nofollow noopener noreferrer"` と `target="_blank"` を付ける。`href` のスキームはリモートの HTML と同じ許可リストである。
- 画像はこのサーバーの、書いた本人のアップロードで、処理が `ready` のものだけを `<img>` にし（`src` は `large` の版の公開 URL、寸法と代替テキスト付き、`loading="lazy"`）、`status_revision_article_images` に記録する。
  それ以外の画像（外部の URL、他人のアップロード）は、代替テキストを文字にしたリンクにする。読む人のブラウザが外部へ画像を取りに行かず、読む人のアドレスが漏れないようにするためである。
- メンション `@user@domain` は投稿と同じ規則で解決したアカウントへのリンクにするが、`status_revision_mentions` に記録しない（§7）。本文の `#name` はハッシュタグにせず、文字のままである。コードの中は解決しない。
- カスタム絵文字の `:shortcode:` は、題・要約・本文から投稿と同じ規則で `status_revision_emojis` に記録する。
- エディタのプレビューは、保存しない同じ描画（`POST /articles/preview`）で作る。プレビューと公開した記事は同じ HTML になる。

### 5. 上限と制約

| 項目 | 値 |
| --- | --- |
| 本文 | 描画した HTML の UTF-8 のバイト数がインスタンス設定 `max_article_bytes`（既定かつ上限 100,000）以下 |
| 本文の画像 | インスタンス設定 `max_article_images`（既定 20）まで。添付の 4 件とは別 |
| 題 | 1〜200 文字（コードポイント）、プレーンテキスト |
| 要約 | 0〜500 文字、プレーンテキスト。空なら本文から作る（§6） |
| ハッシュタグ | エディタで 10 件まで |
| 公開範囲 | `public` / `local` / `unlisted` / `private`。`direct` は選べない |

- 本文の上限を原文ではなく描画した HTML で数えるのは、受け取る側が数えるのが HTML だからである。このサーバーの受信の上限は本文 HTML 100 KiB（federation.md §4）、Akkoma はリモートの投稿を 100,000 文字で捨てる（§9 の表）。100,000 バイト以下の HTML はどちらも超えない。日本語の本文ならおよそ 3 万字である。プレビューは今のバイト数と上限を返す。
- 投票・添付（カバーと本文の画像を使う）・CW の文字は持たない。記事は返信にも、ほかの投稿の引用にもならない。記事への返信と記事の引用は投稿と同じにできる。
- 投稿の自動削除（FR-118）に、記事を残す例外 `keep_articles`（`status_auto_delete_policies` の `bool`、既定 真）を加える。サーバーの上限（ADR-0151）との関係はほかの例外と同じである。

### 6. 要約とセンシティブ

- 記事の `summary` はいつも要約で、CW ではない。記事は CW の文字を持たない。
- センシティブな記事は `sensitive = true` の版で、題と要約を見せて本文とカバーを畳む。
- 要約が空なら、描画した本文のテキスト（見出し・コード・画像の代替テキストを除く）の先頭から 200 文字を、文か語の切れ目で切って `…` を付けたものにする（`summary_origin = body`）。連合の文書にもこの要約を載せる。

### 7. メンションとハッシュタグ

- 本文のメンションはリンクだけで、誰にも通知しない。`status_revision_mentions`・`status_mention_index` に入れず、連合の文書の `tag` に `Mention` を載せず、宛先（`cc`）にも加えない。
- ハッシュタグはエディタで付けたものだけで、`status_revision_tags` に記録し、ハッシュタグのタイムラインと検索に投稿と同じく載る。本文の `#name` からは取らない。

### 8. クライアント API

- 書くのは `POST /articles`・`PUT /articles/{id}`（題・要約・カバー・本文の Markdown・ハッシュタグ・言語・公開範囲・センシティブ・引用の許可）と、`POST /articles/preview` である。投票・添付・CW を持たない形なので、`POST /statuses` とは別の要求にする（ADR-0016）。削除は `DELETE /statuses/{id}` である。
- 読むのは投稿と同じ `Status` で、記事なら `article`（題・要約・カバー）を持つ。タイムライン・通知・ストリーミング・検索の結果は本文を返さない。本文は `GET /statuses/{id}` が返す。編集の履歴（`GET /statuses/{id}/history`）は版ごとの題・要約・本文を返す。
- 下書きは ADR-0156 §9 の `/drafts` に `article` の部分を足す。`pkg/mykit` は同じ操作を持つ（ADR-0093）。

### 9. 送信

- 記事は `Article` として送る。`name`（題）・`summary`（要約。空なら本文から作ったもの）・`content` と `contentMap`（描画した HTML）・`source`（`content` に Markdown の原文、`mediaType` に `text/markdown`）・`url`（表示用 URL）・`sensitive`（常に付ける）・`image`（カバー。`Image` で `url`・`mediaType`・`name`）・`tag`（`Hashtag` とカスタム絵文字の `Emoji`）を載せる。`attachment` は載せない。
- `Create`・`Update`・`Delete`・宛先・`@context`・引用の許可は投稿と同じ組み立て（`federation.notes`）を通り、型だけが変わる。
- 2026-11-09 にソースコードを読んで確かめた、ほかのサーバーの受け取り方（実地の確認は #2592）:

| サーバー | 受け取った `Article` の見え方 | 上限 | `source`（`text/markdown`） |
| --- | --- | --- | --- |
| Mastodon | `CONVERTED_TYPES`（`Image Audio Video Article Page Event`）として、`name` の `<h2>`・`summary`・`url` へのリンクだけの投稿にする。`summary` を CW にしない（`processed_spoiler_text` は空）。本文は読まない | JSON の取得は 1 MiB（`Request#body_with_limit`）。受信の本文の上限は要検証 | 読まない |
| Misskey | `validPost` に `Article` を含み、普通のノートにする。`content` の HTML を MFM に変え、`summary` を CW にする。`name` は投票の回答の照合にだけ使う | ノートの本文を 8,192 文字（`DB_MAX_NOTE_TEXT_LENGTH`）で切る | `text/x.misskeymarkdown` だけを原文として読む |
| GoToSocial | 型で分けず、`content` を全文で持つ。`summary`（無ければ `name`）を CW にする | 要検証 | 読まない（要検証） |
| Akkoma | `Article` を `Note`・`Page` と同じ検証で受け、`content` を全文で持つ。`summary` を CW（subject）にする（要検証） | `remote_limit`（既定 100,000 文字）を超える投稿は捨てる | `text/x.misskeymarkdown` だけを描画し直す |

出典: [Mastodon `app/lib/activitypub/activity.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity.rb)・[`parser/status_parser.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/parser/status_parser.rb)・[`app/lib/request.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/request.rb)、
[Misskey `core/activitypub/type.ts`・`models/ApNoteService.ts`・`const.ts`](https://github.com/misskey-dev/misskey/tree/develop/packages/backend/src)、
[GoToSocial `internal/typeutils/astointernal.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/branch/main/internal/typeutils/astointernal.go)、
Akkoma の `article_note_page_validator.ex` と `config/config.exs`（本家 akkoma.dev は自動取得を拒むため、GitHub の派生 [BDX-town/Akkoma](https://github.com/BDX-town/Akkoma) で読んだ。本家との差は要検証）。

- Mastodon の読者には題・要約・リンクが記事への入口になり、要約が CW にならないので記事は畳まれない。GoToSocial と Akkoma の読者には要約が CW として見え、本文はその下に畳まれる。Misskey の読者には 8,192 文字までの本文が要約の CW の下に見える。どれも本 ADR の側では変えられないので、要約を常に送ることで、どの読者にも中身の分かる入口を残す。

### 10. 受信

- 受信した `Article` は、変換せず記事として取り込む。`name`（無ければ `nameMap` の最初の値。どちらも無ければ空の題）を題、`summary`（同じく `summaryMap`）をプレーンテキストの要約（CW ではない）、`content` / `contentMap` を本文、`image` の最初の画像をカバーにする。
- 本文は §4 の記事用の許可リストでサニタイズする。`h1` は `h2` に下げる。`img` はどのホストのものでも `remote_media`（`purpose = attachment`）に宣言値を記録し、`max_article_images` 件まで `status_revision_article_images` に入れて、表示ではメディアプロキシの URL に置き換える。超えた分は代替テキストのリンクにする。
- 要約が空なら §6 と同じに本文から作る。メンション・ハッシュタグ・カスタム絵文字・添付・公開範囲・`sensitive` は投稿と同じに読む。メンションは投稿と同じに記録して通知する。宛先を決めたのは送った側で、通知しないのはローカルの記事の本文だけである（§7）。
- `Page`・`Event`・`Image`・`Video`・`Audio` は今のまま変換して取り込む。
- 本文 HTML の上限（100 KiB）は投稿と同じである。

### 11. 一覧とフィード

- プロフィールに記事の一覧を持つ。`status_articles (author_account_id, status_id)` をキーセットで読み、公開範囲は投稿と同じポリシー関数で判定する。
- Atom のフィード（#2584）は記事を含め、題・要約・リンクを載せる。各プロフィールは記事だけのフィード（`/@{username}/articles.atom`）も持つ。載せる規則（ローカルの `public` だけ、本人のフィードの設定）は #2584 と同じである。
- HTML のシェル（ADR-0028）は記事の URL で `og:type=article`・題・要約・カバーを出す。本文はシェルに入れない（§12）。

### 12. 最初の版に入れないもの

カレンダー、「1 年前の今日」、サーバーが描画する HTML のシェルの中の本文、共同編集、埋め込み、記事の中の投票と動画、有料の記事。

## 経緯

投稿はプレーンテキストで既定 500 文字までである（FR-101）。ブログや日記のように読める長い文章を Markdown で書き、ほかのサーバーへも届けたい。サーバーの文書は既に `internal/adapter/markdown`（goldmark）と `internal/adapter/htmlsanitizer` で描画している。受信した `Article` は題・要約・リンクだけに変換し、本文を持たない（Mastodon の `CONVERTED_TYPES` と同じ）ので、WriteFreely などの記事をここでは読めない。

決定者は、記事を投稿の一種（`statuses` の 1 行と版の子）とし、下書きを ADR-0156 のテーブルで共有すること、既定で誰でも書け `create_articles` で限れること、本文のメンションは通知しないこと、ハッシュタグをエディタで付けること、要約を CW にしないこと、受信した `Article` を全文で読むこと、Atom のフィードに記事を含めること、最初の版に入れないものを決めた。
そこから、記事であることを部分型のテーブルで表すこと、描画を書いたときに 1 回にすること、画像をこのサーバーのアップロードに限ること、上限を描画した HTML で数えることを詰めた。
続けて決定者は、上限の値（§5）、本文の画像を `attachment` に載せないこと、受信した記事のメンションを投稿と同じに通知すること、全文で読むのは `Article` だけで `Page` は変換のままにすること、`keep_articles` を既定で有効にすること、API の形（§8）を決め、読む時間は持たないことにした。
ほかのサーバーの受け取り方はソースコードを読んで確かめた内容で決め、実地の確認（#2592）で違いが見つかれば見直す。

上限の数え方は、ほかのサーバーの受け取り方をソースコードで調べて決めた（§9 の表）。Akkoma は 100,000 文字を超える投稿を捨て、このサーバー自身も本文 HTML 100 KiB を超える文書を読まないので、原文ではなく HTML で数える。

## 採らなかった案

- 記事を投稿とは別のリソース（`articles` のテーブル）にする: 記事に要らない投稿の列（投票・添付）から自由になるが、公開範囲・編集の履歴・リアクション・タイムライン・通報・削除をすべて二重に持つ。決定者の判断（§1）に反する。
- 種類を `statuses` の列（`kind`）で持つ: 一覧の絞り込みが 1 つの表で済むが、ADR-0020 規約 4 に反し、記事にしかない値が投稿の行に NULL として並ぶ。
- 読むたびに Markdown を描画する: 描画の規則を変えたときに古い記事にも効くが、読み取りのたびに CPU を使い、プレビューと公開の結果が描画の版でずれ得る。原文が残るので、規則を変えたときに描画し直すことはできる。
- 原文の HTML を許す（サニタイザに任せる）: 書き手の自由は増えるが、許可リストの外の書き方が黙って消え、プレビューと読者の見え方が食い違う。投稿も HTML を書かせない（ADR-0019 §2）。
- 外部の画像を `<img>` のまま、またはメディアプロキシ経由で出す: 書き手が手元の画像を使えるが、そのままでは読者のアドレスが外部へ漏れ、プロキシ経由でも書き手が任意の URL をサーバーに取りに行かせられる。アップロードすれば済む。
- 本文の上限を原文の文字数（`max_status_length` と同じ数え方）にする: 書き手に分かりやすいが、表やリンクの多い記事は HTML が膨らみ、受け取る側の上限を超えて黙って捨てられる。
- 本文の画像を `attachment` にも載せる（FEP-b2b8 の勧め）: 本文の `<img>` を落とすサーバーでも画像が見えるが、Mastodon は 4 件までの画像を題と要約の下に並べ、全文を持つサーバーでは本文と添付に同じ画像が二重に出る。
- 記事を `Note` として送る: どのサーバーでも本文が見えるが、Mastodon は 500 文字を超える `Note` もそのまま長い投稿として流し、題と要約の区別が失われる。FEP-b2b8 も長い文章に `Note` を使わないよう勧める。
- `preview`（FEP-b2b8 の要約の `Note`）を載せる: FEP は勧めるが、調べた 4 つのサーバーのどれも読まず、文書が大きくなるだけである。
- 読む時間を持つ（書いたときに本文から数え、カードに出す）: 記事を開く前に長さが分かるが、要らないと決めた。言語ごとの読む速さの目安も根拠が弱い。
- 受信した `Page`（Lemmy の投稿など）も記事として全文で読む: 全文を読める相手が増えるが、`Page` は記事以外の用途にも使われ、題と要約の扱いが送る側ごとに違う。`Article` だけにする。
- 本文のメンションを投稿と同じく通知する: 書き手が人を呼べるが、長い記事の中の言及は呼びかけではないことが多く、決定者の判断（§7）に反する。

## Consequences（影響）

- 良い: 記事は投稿の経路をそのまま使うので、公開範囲・処分・通報・削除・連合・タイムラインが二重にならない。下書きと予約も ADR-0156 の仕組みで動く。
- 良い: WriteFreely などから届く `Article` をこのサーバーで全文読める。
- 良い: 本文の HTML は保存した値だけで、プレビューと公開が一致する。画像は自分のサーバーからだけで、読者のアドレスは外部へ出ない。
- 悪い: Mastodon の読者は記事を題・要約・リンクでしか見られない。GoToSocial・Akkoma・Misskey の読者には要約が CW のように見え、Misskey では長い記事が 8,192 文字で切れる。こちらからは変えられない。
- 悪い: 日本語の本文はおよそ 3 万字までで、本 ADR の上限は受け取る側の都合で決まる。
- 悪い: メンションを通知しないので、記事で名前を出された人はそれに気づかない。
- 悪い: 受信した記事の画像を `remote_media` に記録するので、画像の多い記事を多く受け取るとメディアの行が増える。
- 後続の作業:
  - #2591: §2 のテーブル（`schema/<dialect>/` の content のファイル）、§4 の描画と許可リスト、§5 の上限と設定、§3 の権限と設定、§8 の API と `pkg/mykit`、`keep_articles`、FR の追加と FR-912 の権限の一覧、設定の文書。
  - #2592: §9 の `Article` の組み立てと、ほかのサーバーでの実地の確認（結果は federation.md と本 ADR の §9 に書く）。
  - #2593: §10 の受信。
  - #2594: §2 の下書きの子テーブルと予約。
  - #2595: §11 のフィード。#2596: エディタ。#2597: 記事のカード・記事のページ・プロフィールの一覧・`og:type=article`。
  - 全文検索（M5）が記事の題・要約・本文を索引するかは、検索の ADR が決める。

## Open Questions / 要検証

- 実地の確認（#2592）で、§9 の表とソースコードを読んで確かめた受け取り方に違いが見つかれば、§5 の上限と §9 の文書の形を見直す。
