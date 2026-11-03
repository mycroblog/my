# ADR-0062: 投稿中のリンクのカード

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #652

## Context（背景・課題）

投稿の本文にある URL の先のページから、題名・説明・画像を取り出して投稿の下にカードとして出す仕組みがない（決定者の指示: 「URL 書き込み時の OGP などのカード埋め込み、及びその取得系の実装」）。
Mastodon・Misskey・Pleroma / Akkoma の利用者はこのカードを前提に URL を貼るので、M4（機能の完成度、[00-vision.md](../requirements/00-vision.md)）で扱う。要件は FR-117（[01-functional.md](../requirements/01-functional.md)）。

この ADR は **他所のページを取得して自サーバーの投稿にカードを出す** ことを扱う。逆向きの **自分のページ（`/@{acct}`、投稿）に OGP / X Card の meta を付けて他所にプレビューを出させる** ことは
[ADR-0028](0028-html-shell-link-previews-and-pwa.md) が決めており、この ADR は変えない。コードでも両者を混ぜないよう、ADR-0028 の側は「プレビュー」（`content.Previews`、`internal/interface/web`）、
この ADR の側は「リンクのカード」（`LinkCard`、API の `card`）と呼び分ける。

今決める理由と制約:

- 取得はリモートのサーバーや利用者が宛先を決める外部 HTTP なので、[ADR-0019](0019-security-baseline.md) §1 の SSRF ガード付き `kernel.HTTPClient` を通す。
  ポートにはすでに `HTTPPurposeHTML`（本文 1 MiB、全体 30 秒）がある（`internal/app/kernel/httpclient.go`）が、使う側の仕様（何を・いつ・どれだけ取るか）が決まっていない。
- 取得は相手のサイトに「この URL がいつ投稿されたか」を知らせ、画像の表示は閲覧者の IP を知らせ得る。プライバシーの扱いを先に決めないと、API の形とデータモデルが決まらない（NFR-SEC、FR-102 の公開範囲）。
- カードの画像はリモートのメディアで、[ADR-0005](0005-blob-storage-and-media.md) のメディアプロキシと FR-409 のキャッシュ方針、メディアの派生を決める ADR-0054（#644。メディアプロキシが静止版の `small` を作る）と整合させる必要がある。
- データは [ADR-0020](0020-data-modeling-principles.md) の分類に従い、リモートの投稿の保持は [ADR-0034](0034-remote-content-scope-and-retention.md)、ドメイン単位の処分は
  [data-model.md §5](../architecture/data-model/README.md) の制限（`media_fetch` など）と [ADR-0036](0036-moderation-federation-propagation.md) に従う。
- 後続の実装 #669 がこの決定を待っている。

## Decision Drivers（判断基準）

1. **安全**: SSRF・巨大な応答・悪意ある HTML / 画像・なりすまし（別の URL の題名を名乗る）を、ADR-0019 の既定の上に追加の穴を開けずに防ぐ
2. **プライバシー**: 閲覧者の IP を第三者に出さない。取得から利用者・投稿を特定させない
3. **外部 HTTP の量**: 取得が閲覧回数ではなく投稿と URL の数に比例し、人気の投稿が連合したときの集中（多数のサーバーが同時に同じ URL を取る）を和らげる（NFR-COST、ADR-0034 の判断基準 3）
4. **相互運用**: Mastodon・Misskey などが出す OGP / X Card / `fediverse:creator` を、利用者の期待どおりに読む
5. **実装と運用の小ささ**: 追加のミドルウェアや外部サービス（Summaly のプロキシなど）を必須にしない（NFR-COST-2）。依存は既存の `golang.org/x/net` の範囲に収める

## Considered Options（選択肢）

1. **サーバーが投稿時にジョブで取得し、URL ごとに共有する**（Mastodon 型）
   - ローカルの投稿の作成・編集と、取り込んだリモートの投稿の `Create` / `Update` の後にジョブ（[ADR-0004](0004-job-queue-abstraction.md)）で取得する。結果は URL ごとに保存して投稿から参照する。
   - Mastodon は `LinkCrawlWorker` を投稿の保存後に積み、リモートの投稿では最大 1 分のランダムな遅延を付ける。最初の URL を選び、メンション・ハッシュタグ・自サーバーの URL・添付のある投稿を除き、
     カードを URL で共有して 2 週間で取り直す（出典: [`fetch_link_card_service.rb`](https://github.com/mastodon/mastodon/blob/main/app/services/fetch_link_card_service.rb)、
     [`activitypub/activity/create.rb`](https://github.com/mastodon/mastodon/blob/main/app/lib/activitypub/activity/create.rb)、
     [`post_status_service.rb`](https://github.com/mastodon/mastodon/blob/main/app/services/post_status_service.rb)、2026-10-20 確認）。
2. **表示時にサーバーが取得する**（Misskey 型）
   - クライアントが投稿を表示するときに、自サーバーの API（Misskey は `/url`）に URL を渡し、サーバーが [Summaly](https://github.com/misskey-dev/summaly)（MIT）で取得して返す。
     Misskey はメモリに 1 時間・100 件、HTTP で `max-age=86400` のキャッシュを持ち、画像はメディアプロキシで包む（出典: [`UrlPreviewService.ts`](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/server/web/UrlPreviewService.ts)、2026-10-20 確認）。
   - Pleroma / Akkoma の `:rich_media` も、取得の契機こそ違うが、ホストの除外リスト（`ignore_hosts`、`ignore_tld`）とタイムアウトを設定に持つ
     （出典: [Pleroma Configuration Cheat Sheet](https://docs.pleroma.social/backend/configuration/cheatsheet/)。取得の契機は要検証）。
3. **カードを出さない**
   - GoToSocial は API の `card` を常に `null` で返す（出典: [`internaltofrontend.go`](https://codeberg.org/superseriousbusiness/gotosocial/src/branch/main/internal/typeutils/internaltofrontend.go) の `Card: nil, // TODO: implement cards`、2026-10-20 確認）。
4. **送信側のサーバーが作ったカードを信じる**
   - リモートの投稿が持ってきた題名・画像をそのまま出す。主要な実装でカードを連合に載せる広く使われた仕様はない（要検証）。

クライアントが直接取得する案は、CORS で読めないうえ閲覧者の IP を相手に出すので、選択肢に入れない。

## Decision（決定）

**選択肢 1（投稿時にサーバーがジョブで取得し、URL ごとに共有する）を採用する。**
取得は投稿と URL の数に比例し（判断基準 3）、閲覧者の IP は出ず（判断基準 2）、Mastodon と同じ見え方になる（判断基準 4）。
選択肢 2 は閲覧のたびに API を呼ぶので、キャッシュが外れるたびに外部 HTTP が閲覧に比例し、タイムラインの表示がカードの取得を待つ。選択肢 3 は FR-117 を満たさない。
選択肢 4 は、送信側が任意の題名と画像を任意の URL に付けられ、フィッシングに使えるので採らない。

### 1. 対象と契機

- **契機**: ローカルの投稿の作成・編集（`content` の投稿の版のイベント）と、ADR-0034 で取り込んだリモートの投稿の `Create` / `Update` の後に、ジョブ `content.fetch_link_card`（引数は投稿の ID）を outbox で積む。
  リモートの投稿のジョブは 0〜60 秒のランダムな遅延を付ける。多数のサーバーが同じ投稿を同時に受けて同じ URL を一斉に取る集中を散らすためで、Mastodon の `DISTRIBUTE_DELAY` と同じ幅にする。
- **選ぶ URL**: 投稿の現在の版の本文にある、次の条件を満たす **最初の 1 件**。
  - スキームが `http` / `https` で、ホストがある
  - メンションとハッシュタグのリンクではない。ローカルの投稿は `content.ParseText` の区分で、リモートの投稿はサニタイズ済み HTML の `a` の `class`（`mention`, `hashtag`, `u-url`）と `rel="tag"`、
    投稿のメンション・タグの URL との一致で除く
  - 自サーバーの URL ではない（自サーバーの投稿やプロフィールへのリンクにカードを出さない）
- **出さない投稿**: 添付（画像・動画・音声）や投票がある投稿、ブースト（ブーストされた投稿のカードを出す）。添付と並べると表示が重くなるためで、Mastodon も添付のある投稿は取らない。
- **公開範囲**:
  - **公開範囲によらず**（`public` / `unlisted` / `local` / `private` / `direct`）、カードがない・古いときに取得する。Mastodon と同じ見え方にする。
  - 取得は相手のサイトに「その URL がいつ投稿されたか」を知らせ、共有リンク・ワンタイムの URL では、GET で動く操作を実行させ得る。
    非公開の投稿でもこれを受け入れ、相手に伝わる情報を §4 の範囲に限ることで抑える。
- 編集で選ぶ URL が変わったら、ジョブを積み直してカードの参照を差し替える。URL がなくなればカードを外す。

### 2. 取得と解釈

- **HTTP**: `kernel.HTTPClient` に `HTTPPurposeHTML` で `GET` を送る。ADR-0019 §1 の宛先の拒否・リダイレクト 5 回・`https` から `http` への格下げの拒否・本文 1 MiB（展開後）・全体 30 秒をそのまま使う。
  - 要求ヘッダは `Accept: text/html, application/xhtml+xml`、`Accept-Language` はインスタンスの既定の言語（利用者の言語にしない。取得を利用者に結び付けないため）。Cookie と `Referer` は送らない。
  - 状態コード 200 と、`Content-Type` が `text/html` / `application/xhtml+xml` の応答だけを解釈する。
  - `http` の URL は、ADR-0019 §1 のスキームの制限（`https` のみ。開発用の設定を除く）に従い取得せず、カードを出さない。
- **解析**: `golang.org/x/net/html` のトークナイザで `<head>` の中だけを読み、`</head>` か `<body>` で止める。文字コードは `golang.org/x/net/html/charset`（`Content-Type` の `charset`、`<meta charset>`、推定の順）で UTF-8 にする。
  スクリプトは実行しない（JavaScript で meta を書き足すページはカードにならない）。
- **項目と優先順**（出典: [The Open Graph protocol](https://ogp.me/)、[X Cards markup](https://developer.x.com/en/docs/x-for-websites/cards/overview/markup)。X Card の項目名は要検証）:

  | 項目 | 優先順 | 上限 |
  | --- | --- | --- |
  | 題名 | `og:title` → `twitter:title` → `<title>` | 200 文字 |
  | 説明 | `og:description` → `twitter:description` → `<meta name="description">` | 500 文字 |
  | サイト名 | `og:site_name` → URL のホスト | 100 文字 |
  | 画像 | `og:image`（`og:image:secure_url` を優先）→ `twitter:image` | URL 2,048 バイト |
  | 画像の代替テキスト | `og:image:alt` → `twitter:image:alt` | 1,500 文字 |
  | 作者 | `article:author`（URL でない値）→ `<meta name="author">` | 100 文字 |
  | 公開日時 | `article:published_time`（RFC 3339 として読めるものだけ） | — |
  | 作者の Fediverse のアカウント | `fediverse:creator`（§5） | — |

  - 値はすべてプレーンテキストにする（HTML の実体参照を戻し、タグを除き、空白を 1 つにまとめる）。上限を超える分は文字（書記素）の境界で切る。
  - 相対 URL は応答の最終 URL を基準に解決し、`http` / `https` 以外は捨てる。
  - `og:url` は、最終 URL と同じオリジンのときだけカードの URL に使う（別のオリジンを名乗るページで、別のサイトのカードに見せかけられないようにする。Mastodon も同じオリジンに限る）。
  - 題名がなければカードを作らない（失敗として扱う）。
- **JSON-LD**: `<head>` の `<script type="application/ld+json">` を、スクリプトとして実行せず JSON として静的に読む（[JSON-LD 1.1](https://www.w3.org/TR/json-ld11/)、[schema.org](https://schema.org/)）。
  - 最上位またはその `@graph` にある `@type` が `NewsArticle` / `Article` / `BlogPosting` / `WebPage` の最初のオブジェクトだけを使う。`@context` の取得・展開はしない（外部 HTTP を増やさない）。
  - 使う項目は `headline`（題名）、`description`（説明）、`author.name`（作者。配列ならカンマと空白でつなぐ）、`datePublished`（公開日時）、`publisher.name`（サイト名）。
    上の表の項目がないときだけ使う（優先順は meta の後、`<title>` / `<meta name="description">` の前）。Mastodon は JSON-LD を meta より先に読むが、ここではページの作者が明示した meta を優先する。
  - JSON として読めないもの、1 つの `<script>` が 64 KiB を超えるもの、JavaScript で後から書き込まれるもの（取得した HTML にないもの）は使わない。上限と値の扱い（プレーンテキスト化）は表と同じ。
- **oEmbed と埋め込み**: oEmbed（[oembed.com](https://oembed.com/)）には対応しない（発見も取得もしない）。`twitter:player` / `og:video` による iframe・動画の埋め込みもしない。
  埋め込みは第三者の iframe とスクリプトを Web クライアントに入れることになり、閲覧者の IP と Cookie を相手に渡し、CSP（ADR-0019 §9）に `frame-src` を足す必要がある。
  動画のページも、題名・説明・サムネイルの静的なカードとして出す。Mastodon は `video` 型の oEmbed の HTML を iframe として出し、`rich` 型（スクリプトを使うもの）は拒否している。
- **失敗**: 取得・解釈の失敗（4xx / 5xx、タイムアウト、上限超過、HTML でない、題名がない）は、URL ごとに KV に 24 時間記録し、その間は同じ URL を取らない（ADR-0034 の取得の失敗の記録と同じ仕組み）。
- **同時取得の抑止**: 取得の前に URL ごとの短期のロック（KV、60 秒）を取り、別のジョブが取得中なら待たずに後で積み直す。ジョブは冪等で、同じ投稿のジョブが重なってもカードの行と参照の UPSERT で同じ結果になる。

### 3. 画像

- 画像の URL は、ほかのリモートのメディアと同じく **宣言値として記録し、クライアントにはメディアプロキシの URL で返す**（ADR-0005、[federation.md §9](../architecture/federation.md)）。
  クライアントが元のサイトの画像を直接読み込むことはない（閲覧者の IP を出さない。ADR-0019 §9 の `img-src` も広げない）。
- カードを作るジョブは、画像を `HTTPPurposeImage` で 1 回だけ取得し、形式を中身で判定し（ADR-0019 §8 の許可リスト、SVG は拒否）、寸法と blurhash を求めて記録する。
  形式・寸法の上限（ADR-0019 §8）に合わないものは画像なしのカードにする。取得した本体は保存しない（保存はメディアプロキシのキャッシュ方針 FR-409 に任せる）。
- 表示は ADR-0054（#644）のメディアプロキシの `small`（静止版の S）を使う。カードの画像の用途は投稿の添付と同じ S の大きさとし、アニメーション画像は最初のフレームの静止画にする。
  API は `small` の URL だけを返し、カードから原寸（`full`）は出さない。`remote_media_policy` が `all` のときは、ADR-0054 と同じく `remote_media_cache` に派生のキーで保存する。
- 画像は `media_files`（`origin = remote`）の部分型として記録する。所有者はアカウントではなくカードの URL なので、`remote_media`（所有者がアカウント）とは別の部分型にする（§6）。

### 4. プライバシー

- 取得はサーバーだけが行い、クライアントは自サーバーの API とメディアプロキシしか呼ばない。
- 相手のサイトに伝わるのは、サーバーの `User-Agent`、インスタンスの既定の言語、取得の時刻だけで、どの利用者・どの投稿かは伝えない。
- カードは URL ごとに共有するので、同じ URL を複数の投稿が貼っても取得は（再取得の間隔の内は）1 回で済む。
- カードの API の応答に、取得の時刻や失敗の理由など、運用者向けの情報は出さない。

### 5. `fediverse:creator`

- `<meta name="fediverse:creator" content="@user@domain">` を読み（複数あれば最初の 1 件）、カードを作るジョブの中で WebFinger（FR-401）でアカウントに解決する（10 秒まで。解決できなければ記録しない）。
- **検証できたときだけ** カードに作者のアカウントとして出す。検証は、解決した Actor の `attributionDomains`（Mastodon の `toot:attributionDomains`。
  出典: [Mastodon ActivityPub 仕様](https://docs.joinmastodon.org/spec/activitypub/)、2026-10-20 確認）が、ページの最終 URL のホストかその親ドメインを含むこと。
  検証できない宣言は捨てる（任意のページが任意のアカウントを作者と名乗れるため）。Mastodon も同じドメインの照合をし、照合できない作者は「未検証」として区別する。
- ローカルの利用者が自分の `attributionDomains` を設定して Actor に載せることは、この ADR の範囲外とし、後続の Issue にする。

### 6. データモデル（ADR-0020）

`content` モジュールが所有する。

| テーブル | 分類 | 主な列 | 備考 |
| --- | --- | --- | --- |
| `link_cards` | 現在値（キャッシュ） | `url`（PK。正規化した要求の URL）, `card_url`（最終 URL、または同じオリジンの `og:url`）, `title`, `description`, `site_name`, `author_name`, `published_at`（NULL 可）, `image_media_id`（NULL 可）, `creator_account_id`（NULL 可。§5 で検証できたもの）, `language`（NULL 可。`<html lang>`）, `fetched_at` | `remote_media_cache` と同じく、取得の結果を再取得で UPSERT する。インデックス `(fetched_at)` |
| `link_card_media` | リソース（部分型） | `media_id`（PK、`media_files` の部分型）, `remote_url`（一意）, `content_type`, `width`, `height`, `blurhash`, `description`（`og:image:alt`） | §3。リモートの添付の `remote_media` と違い、所有者のアカウントを持たない |
| `status_link_cards` | 投影 | `status_id`（PK）, `url` | 投稿の現在の版から選んだ URL。インデックス `(url)`。`link_cards` の行がない間はカードを出さない。再構築は投稿の版から選び直す |

- URL の正規化: スキームとホストを小文字にし、既定のポートとフラグメントを除く。クエリは並べ替えない（意味が変わり得るため）。
- **再取得**: 新しい投稿が URL を参照したとき、`fetched_at` から 14 日を過ぎていれば取り直す（Mastodon の 2 週間に合わせる）。定期的な一斉の取り直しはしない。
- **保持と消去**: 1 日 1 回のジョブ `content.purge_link_cards` が、`status_link_cards` から参照されなくなって `fetched_at` から 14 日を過ぎた `link_cards` と、どこからも参照されない `link_card_media` を消す（FR-803）。
  投稿の削除・消去（ADR-0034 の保持期間切れ、処分による削除）は、同じトランザクションで `status_link_cards` の行を消す。
- インスタンス設定 `link_cards_enabled`（`bool`、既定 `true`）を `instance_setting_revisions` の `key` に足す。偽ならジョブを積まず、API は `card` を返さない。

### 7. ドメイン単位の処分との関係

- カードの URL のホスト（またはその親ドメイン）に、ドメイン単位の `suspend` の制限（`federation`）があれば、取得しない・既存のカードも出さない。運用者が連絡を断ったドメインに、カードのためだけに接続しないため。
- 同じく `reject_media` の制限（`media_fetch`）があれば、画像なしのカードにする。`media_fetch` はメディアプロキシでも画像の URL のホストに当たる（federation.md §9）。
- 投稿の作者に `media_sensitive`（`force_sensitive`、メディアの強制センシティブ）があるか、投稿がセンシティブなら、カードの画像はセンシティブとして表示する（ぼかす）。
- CW（`spoiler_text`）がある投稿のカードは、本文と同じく CW を開くまで出さない（Web クライアントの表示の規則）。
- カードは連合しない（自サーバーの投稿を配送するときにカードの情報を載せない）ので、ADR-0036 の処分の伝播には関係しない。

### 8. クライアント API

`Status` に省略可能な `card`（`LinkCard`）を足す。カードがない投稿では省略する。ブーストの行では `boost` の側に付く。名前は自前の API の流儀（[ADR-0016](0016-client-api-style.md)）に従い、Mastodon の `PreviewCard` とは合わせない（[ADR-0007](0007-client-api.md)）。

| 項目 | 内容 |
| --- | --- |
| `url` | カードの URL（`card_url`） |
| `title` / `description` / `site_name` | §2 のプレーンテキスト（`description` と `site_name` は空文字があり得る） |
| `author_name` | 省略可 |
| `published_at` | 省略可 |
| `language` | 省略可（表示の `lang` に使う） |
| `image` | 省略可。`url`（メディアプロキシの `small` の URL）、`width`、`height`、`blurhash`、`description` |
| `author` | 省略可。§5 で検証できた作者の `Account` |

Web クライアントは、カード全体を `url` へのリンク（`rel="noopener noreferrer nofollow"`、`target="_blank"`）にし、題名・説明・サイト名は `dir="auto"` と `lang` を付けて出す（ADR-0009）。
カードの部品は独立して失敗し得る区画として `react-error-boundary` で囲む（[web-client.md](../architecture/web-client/README.md) の E1）。

### この決定を見直す条件

- 非公開の投稿の URL の取得で、ワンタイムの URL の消費などの実害が報告された場合（`private` / `direct` で取得を起こさない形に寄せる）
- 連合の集中で、相手のサイトへの取得が問題になる規模になった場合（遅延の幅を広げる、または選択肢 2 の表示時の取得に寄せる）
- 埋め込み（動画の再生など）を求める声が大きい場合（クリックで読み込む・許可したプロバイダだけの埋め込みを、別の ADR で CSP とともに決める）

## Consequences（影響）

- 良い影響
  - URL を貼った投稿が、Mastodon などと同じくカード付きで見え、FR-117 を満たす。
  - 取得は投稿と URL の数に比例し、閲覧回数に比例しない。閲覧者の IP は第三者に出ない。
  - 外部 HTTP は既存の `kernel.HTTPClient`（`HTTPPurposeHTML` / `HTTPPurposeImage`）とメディアプロキシの上に乗り、新しい依存も新しいミドルウェアも要らない。
- 悪い影響・トレードオフ
  - 投稿の直後はカードがなく、ジョブの完了後に現れる（リモートの投稿は最大 60 秒の遅延が加わる）。SSE（FR-207）で投稿の更新を流さない限り、再読み込みまで出ない。
  - `private` / `direct` の投稿に貼った URL も取得するので、相手のサイトはその URL がいつ投稿されたかを知り、GET で動く URL（ワンタイムのリンクなど）は消費され得る。
  - JSON-LD を読むので、HTML の解析の対象が `<script type="application/ld+json">` の JSON にも広がる（上限で抑える）。
  - JavaScript で meta を作るページ、`<head>` が 1 MiB を超えるページ、bot を拒否するページはカードにならない。
  - 埋め込みがないので、動画のリンクはその場で再生できない。
  - `http` の URL にはカードが出ない（ADR-0019 §1。Mastodon は `http` も取る）。
  - 取得は相手のサイトに投稿の時刻を（遅延の幅の精度で）知らせる。公開の投稿についてはこれを受け入れる。
- 後続で必要になる作業（Issue 化する）
  - #669 カードの取得と表示（ジョブ、`link_cards` / `link_card_media` / `status_link_cards`、`Status.card`、Web クライアントのカード、`link_cards_enabled`、データモデルとスキーマ、federation.md への追記）
  - カードの付いた投稿の更新を SSE で流す（FR-207 の実装に合わせる）
  - ローカルの利用者の `attributionDomains` の設定と Actor への出力
  - 運用者がカードを取らないホストを指定する除外リスト（Pleroma の `ignore_hosts`、Misskey の `urlPreviewDenyList` に当たるもの）。必要が出たら作る

## Pros and Cons of the Options（詳細比較・任意）

| 観点 | 1. 投稿時のジョブ | 2. 表示時の取得 | 3. 出さない | 4. 送信側を信じる |
| --- | --- | --- | --- | --- |
| 安全（なりすまし） | 自サーバーで解釈し、`og:url` を同じオリジンに限る | 同じ | 問題なし | 送信側が任意に名乗れる |
| 閲覧者の IP | 出さない（プロキシ） | 出さない（プロキシ） | — | 画像次第 |
| 外部 HTTP の量 | 投稿と URL に比例 | 閲覧とキャッシュの外れに比例 | なし | なし |
| 連合の集中 | 受信した全サーバーが取る（遅延で散らす） | 閲覧されたサーバーだけが取る | なし | なし |
| 表示の速さ | 保存済みを返す | 表示のたびに取得を待つことがある | — | 速い |
| 非公開の投稿 | 投稿時に 1 回取得する | 閲覧のたびに取得し得る | — | — |
| 実装 | ジョブ・テーブル 3 つ | API 1 つ・キャッシュ | なし | 仕様がない |

選択肢 2 の最も強い点は、受信した投稿のうち実際に閲覧されたものの URL だけを取るので、連合の集中が小さく、保存も要らないこと。
小さなサーバーでは閲覧される投稿が少なく、取得の総量は 1 より少ないこともある。それでも 1 を選ぶのは、タイムラインの表示を取得の待ちから切り離せることと、
URL ごとの取得を投稿時の 1 回にまとめられることを重く見るため。

## Open Questions / 要検証

- 要検証: 主要な実装が、カードを ActivityPub の文書に載せて連合する仕様（FEP など）を持たないこと。
- 要検証: Pleroma / Akkoma の `:rich_media` が取得する契機（投稿の受信時か、表示時か）。
- 要検証: 題名・説明の上限（200 / 500 文字）が、実際のサイトのカードを切りすぎないこと。#669 の実装で主要なサイトを確かめる。
