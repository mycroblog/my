<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# リンクプレビュー（OGP / X Card / fediverse:creator）

ほかのサービスがこのサーバーのページについて表示するプレビューと、そのためにサーバーが返すもの、そして手で確認する手順を説明します。このサーバーが自身の投稿のリンクについて表示するカードは、
下の[投稿のリンクのカード](#cards-of-links-in-posts)にあります。ドメインからの取得を止める方法は[インスタンスの設定](instance-settings.md#link-card-domains)にあります。

<!-- source: what-the-server-returns bb9326f9729a -->
## サーバーが返すもの

`my serve` は、ランディングページ（`https://<host>/`）、インスタンスの情報のページ（`https://<host>/about`）、
公開している文書のページ（`/about/terms`、`/about/privacy`、`/about/commerce`）、リンクのカードのクローラーについてのページ（`/about/crawler`）、プロフィール
（`https://<host>/@alice`）、投稿（`https://<host>/@alice/status/<post ID>`）の HTML を、Web クライアントのシェルにメタタグを埋め込んで
返します。
HTML のリクエストは常にログインしていないものとして扱うので、プレビューには、ログインしていない訪問者に見えるものだけが表示されます。

| ページ | 応答 |
| --- | --- |
| ランディングページ（`/`）とインスタンスの情報のページ（`/about`） | 200。インスタンスを紹介するメタタグの入ったシェル（下の「ランディングページ」を参照） |
| リンクのカードのクローラーについてのページ（`/about/crawler`） | 200。インスタンスを紹介するメタタグの入ったシェル。題名は「リンクのカードのクローラー \| インスタンス名」 |
| 公開している文書のページ（`/about/terms`、`/about/privacy`、`/about/commerce`） | 200。インスタンスを紹介するメタタグの入ったシェル。題名は「文書名 \| インスタンス名」。公開していない文書のページには汎用のシェルを返します |
| `public` の投稿、有効なローカルのアカウントのプロフィール | 200。メタタグの入ったシェル |
| `unlisted` の投稿 | 200。メタタグの入ったシェルに、`<meta name="robots" content="noindex, noarchive">` と `X-Robots-Tag: noindex, noarchive` を加えたもの |
| `local`、`private`、`direct`、削除された、または存在しない投稿と、凍結、無効化、削除、または制限されたアカウントのプロフィールと投稿 | 404。汎用のシェルと `X-Robots-Tag: noindex, noarchive`。本文とヘッダーは、理由が何であっても同じです |
| `/@<old username>/status/<post ID>` | 301。いまのユーザー名の URL へ |

どのページにも、インスタンスの設定と持ち主の設定に従って、検索エンジンと AI のクローラーへの表明（`noindex` や `noai` などのメタタグと、
`X-Robots-Tag`、`Content-Usage`、`tdm-reservation`）が付きます
（[検索エンジンと AI のクローラー](crawlers.md)）。上の表の `noindex` は、設定にかかわらず必ず付きます。
`noindex` はリンクプレビューを止めません。

ActivityPub の `Accept` を付けたリクエストにはシェルを返しません。ActivityPub の側が応答します（プロフィールは
Actor の `id` で応答し、ログインしていない訪問者に見える投稿は、その `id` への 302 で応答します）。どの応答にも `Vary: Accept` が付きます。

<!-- source: meta-tags 00a4a8115b9e -->
### メタタグ

| タグ | 内容 |
| --- | --- |
| `og:title`、`<title>` | 「表示名 (@ユーザー名@アカウントのドメイン)」。表示名が空ならユーザー名 |
| `og:type` | プロフィールでは `profile`、投稿では `article` |
| `og:url`、`<link rel="canonical">` | 正規の URL（いまのユーザー名の URL） |
| `og:description` | 投稿の本文（プロフィールでは自己紹介）からタグを除いたプレーンテキストを、200 書記素で切ったもの。注意書きのある投稿では、注意書きの文章だけ |
| `og:image`、`og:image:alt` / `width` / `height` | 下の順で選んだ画像 |
| `og:site_name` | インスタンス名（テキスト `name`。どの言語でも同じです。ない場合は `host`。[名前とドメイン](instance-settings.md#name-and-domain)） |
| `og:locale`、`<html lang dir>` | 投稿の言語。設定されていない（`und`）場合は投稿者の既定の投稿の言語、それもなければインスタンスの `default_locale`。`Accept-Language` によっては変わりません |
| `article:published_time`、`article:author` | 投稿のみ。投稿の日時（UTC）と投稿者のプロフィールの URL |
| `profile:username` | プロフィールのみ。`username@account domain` |
| `twitter:card` | 画像がおよそ 2:1 より横長（幅 / 高さ >= 1.7）で、300x157 以上なら `summary_large_image`、それ以外は `summary` |
| `fediverse:creator` | `@username@account domain`。Mastodon はこれを「作者」として表示します |
| `<link rel="alternate" type="application/activity+json">` | AP の `id`（Actor か、投稿のオブジェクト） |

ランディングページのメタタグは、インスタンス自体を紹介します。

- `og:type` は `website`、`og:title`、`<title>`、`og:site_name` はインスタンス名、`og:url` と
  `<link rel="canonical">` は `https://<host>/` です。
- `og:description` は、インスタンスのテキスト `short_description` を 200 書記素で切ったものです。ない場合は出力しません。
- 言語（`og:locale`、`<html lang dir>`）はインスタンスの `default_locale` です。名前と説明は、どの言語でも同じ値です。
- `og:image` は下の 3 番（インスタンスのプレビュー画像）、それがなければ 4 番か 5 番です。
- アカウントに属さないので、`fediverse:creator` と ActivityPub の `alternate` は出力しません。
- インスタンスの情報のページ（`/about`）にも同じメタタグがあります。`og:title` と `<title>` は「{インスタンス名} について」
  （`server:preview.about_title`）、`og:url` と `<link rel="canonical">` は `https://<host>/about` です。
- 公開している文書のページ（`/about/terms`、`/about/privacy`、`/about/commerce`）にも同じメタタグがあります。
  `og:title` と `<title>` は「{文書名} | {インスタンス名}」（`server:preview.document_title`）、`og:url` と
  `<link rel="canonical">` はそのページのアドレスです。公開していない文書のページには汎用のシェルを返します。
- リンクのカードのクローラーについてのページ（`/about/crawler`）にも同じメタタグがあり、題名とアドレスも同じ形です。

`og:image` は次の順で選びます。

1. 投稿に添付された最初の画像（その代替テキストが `og:image:alt` に入ります）。センシティブに指定した投稿や
   注意書きのある投稿では使いません。
2. プロフィールのヘッダー画像、なければアバター（`og:image:alt` は「{name} のヘッダー画像」か「{name} のアバター」）。
3. インスタンスのプレビュー画像（インスタンスの設定 `preview_image`。`og:image:alt` はテキスト `preview_image_alt`、なければ「{インスタンス名} の
   イメージ画像」）。
4. インスタンス自身のロゴのリンクプレビュー用の版（[ロゴとアイコン](instance-settings.md#logo-and-icon)）。`https://<host>/brand/<revision>/preview-1200x630.png` にある
   1200x630 の PNG です（`og:image:alt` は「{インスタンス名} のイメージ画像」）。
5. 同梱の既定の画像（`https://<host>/preview-image.png`、1200x630 の PNG。インスタンス名を含まない汎用の絵で、
   `og:image:alt` はその絵を説明します）。

そのため、プレビューを表示するページには必ず `og:image` と `og:image:alt` があります。
1 から 3 の画像は処理済みのメディアの `full` の版で、幅と高さは処理で得た値です。
`preview_image` が処理中、処理に失敗した、または画像でない場合は、4 番（ロゴがなければ 5 番）を使います。
インスタンスの設定はリクエストのたびに読むので、`my instance set preview_image ...` は再起動なしで反映されます（手順は
[インスタンスの設定](instance-settings.md)を参照してください）。

既定の画像は、起動後の最初のリクエストで標準ライブラリーによって描かれ、プロセスの中に保持されます（約 10 KB）。
`Cache-Control: no-cache` と ETag を付けて配信します。

<!-- source: caching 5bc906452b10 -->
### キャッシュ

- 応答には `Cache-Control: public, max-age=60, stale-while-revalidate=300` が付きます。削除、公開範囲の変更、
  文書の公開や非公開は、最長でその間、共有キャッシュに残ります。
- ETag は、テンプレート、アセット、インスタンスの設定のバージョン、言語、投稿のバージョン
  （版）、実際の公開範囲、プロフィールのバージョン、選んだ画像から作ります（ランディングページ、
  情報のページ、文書のページ、クローラーについてのページでは、ページのアドレス、その題名、インスタンスの説明、選んだ画像）。
  `If-None-Match` には `304` を返します。
- 前に CDN を置く場合は、キャッシュキーに `Accept` を含めてください（AP の JSON と HTML が混ざらないようにするためです）。

<!-- source: manual-check bf2c7a86a5de -->
## 手での確認

アップグレードのあとやプレビューの設定を変えたあとは、各サービスを実機で確認してください。外から HTTPS で届くインスタンスが必要です（
プレビューのボットは `localhost` に届きません）。

<!-- source: preparation 9fd604dc1529 -->
### 準備

1. テスト用のアカウントで、次の投稿を用意します。
   - 横長の画像（たとえば 1200x630）と代替テキストのある `public` の投稿
   - 画像のない `public` の投稿（ヘッダー画像のあるアカウントから）
   - 本文が 200 文字より長く、絵文字（👨‍👩‍👧‍👦 など）を含む `public` の投稿
   - `unlisted` の投稿
   - `private` の投稿
   - 表示名に `"><script>alert(1)</script>` を含むアカウントの投稿
2. 各 URL の HTML を確認します。

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - ステータスが 200 で、上の表のメタタグがあること。`unlisted` の投稿には `noindex` があること。
   - `private` の投稿と存在しない ID が、`X-Robots-Tag: noindex, noarchive`（設定が AI を拒否しているときは `noai,
     noimageai` も）とともに同じ 404 の本文を返すこと。

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - 表示名の `<script>` が `&lt;script&gt;` とエスケープされていること。

<!-- source: x 73a2dffdfdbb -->
### X

1. 画像のある投稿の URL を、投稿の画面（または DM）に貼り付けます。
2. 大きな画像のカード（`summary_large_image`）に、題名、ドメイン、画像が表示されることを確認します。
3. 画像のない投稿の URL では、カードにヘッダー画像が表示されることを確認します。ヘッダー画像も
   アバターもないアカウントでは、大きなカードに既定の画像（`preview_image`、設定されていなければ同梱の画像）が表示されることを確認します。
4. 正方形のアバターだけを持つアカウントでは、カードが小さな画像のカード（`summary`）であることを確認します。

X はカードをキャッシュします。変更を確認するには、別の URL とみなされるよう、クエリ（`?v=2` など）を付けて URL を貼り付けてください。

<!-- source: mastodon 02868aebd430 -->
### Mastodon

1. ほかのインスタンスのアカウントから、画像のある投稿の URL を含む投稿をします。
2. 投稿の下に、題名、説明、画像のあるプレビューカードが表示されることを確認します。
3. カードの下に、テスト用のアカウント（`fediverse:creator`）が「作者」として表示され、その
   プロフィールに移動できることを確認します。

Mastodon は、カードを取得したサーバーごとにカードをキャッシュします。作り直すには、新しい URL（クエリ付き）で投稿してください。

<!-- source: discord 4df04642bb64 -->
### Discord

1. 画像のある投稿の URL をチャンネルに貼り付けます。
2. 埋め込みに、サイト名（インスタンス名）、題名、説明、大きな画像が表示されることを確認します。
3. 200 文字より長い投稿では、説明が `…` で終わり、絵文字が途中で切れていないことを確認します。

<!-- source: slack 9f1e524968f3 -->
### Slack

1. 画像のある投稿の URL をチャンネルに貼り付けます。
2. 展開されたプレビューに、サイト名、題名、説明、画像が表示されることを確認します。
3. `unlisted` の投稿の URL でもプレビューが表示されることを確認します（`noindex` は検索エンジン向けで、
   プレビューを止めません）。

Slack もプレビューをキャッシュします。変更を確認するには、URL にクエリを付けてください。

<!-- source: line ef9e1e682603 -->
### LINE

1. 画像のある投稿の URL をトークで送ります。
2. 題名、説明、画像のあるプレビューが表示されることを確認します。

LINE もプレビューをキャッシュします。変更を確認するには、URL にクエリを付けてください。

<!-- source: what-must-not-be-visible 187905fe778d -->
### 見えてはいけないもの

どのサービスでも、`private` の投稿の URL と存在しない投稿の URL が、どちらも同じ汎用のプレビュー
（インスタンス名だけか、何もなし）になり、本文、表示名、画像が表示されないことを確認してください。

<!-- source: cards-of-links-in-posts 6394a6bf5641 -->
## 投稿のリンクのカード

投稿のリンクのカードを表示するために、このサーバーのジョブはリンク先のページとその画像を取得し、そのことについてリンク先のサイトが示すものに従います。
ページ、画像、`robots.txt` へのリクエストは `my-linkcard/<version> (+https://<host>/)` です。メディアプロキシーのものを含め、サーバーのほかのリクエストはすべて `my/<version> (+https://<host>/)` のままです。
リクエストは Cookie も `Referer` も送らず、どのアカウントやどの投稿がそのページにリンクしたかを示すものも何も送りません。

<!-- source: the-page-for-site-operators 08a157cd9eac -->
### サイトの運営者向けのページ

`https://<host>/about/crawler` は、リンク先のサイトの運営者に、クローラーが何をリクエストし、何を送り、どのくらいの頻度で、どうすれば拒否できるかと、自分のドメインからの取得をやめるようあなたに頼めることを伝えます。アカウントがなくても誰でも読めます。インスタンスの情報のページ（`/about`）の「検索エンジンと AI」の節がこのページにリンクしています。`User-Agent` が示すのは `https://<host>/` だけです。

このページは、`link_cards` と `link_cards_ignore_robots_txt` から、このサーバーが何をするかを伝え、インスタンスの連絡先（`contact_account`、`contact_email`）を表示します。サイトが問い合わせる先があるよう、連絡先を設定してください。連絡先がないと、ページには、このサーバーが連絡先を公開していないと表示されます。
`link_cards_ignore_robots_txt` が有効な場合、ページには、サイトが `robots.txt` でこのサーバーを拒否できないと表示されます。

<!-- source: robotstxt e8d2deaef93d -->
### robots.txt

`robots.txt` での名前は `my-linkcard` で、大文字と小文字は区別しません。その名前のグループがなければ、RFC 9309 のとおり `*` のグループが適用されます。`Allow`、`Disallow`、`*`、`$` を読み、最も長く一致したものが優先され、同じ長さなら `Allow` が優先されます。`Crawl-delay` などのレコードは読みません。
`robots.txt` は 10 秒で 512 KiB まで読み、それより大きいファイルは読めた分を使います。サーバーは、オリジン（スキーム、ホスト、ポート）ごとに、`my-linkcard` 向けのルールだけを保持します。

| `robots.txt` への応答 | サーバーがすること | 保持する期間 |
| --- | --- | --- |
| 200 | ファイルに従います | 24 時間 |
| 429 以外の 4xx | ファイルなしとして取得します | 24 時間 |
| 429、5xx、ネットワークの障害、またはサーバーが拒否するリダイレクト | そのオリジンからは何も取得しません | 1 時間 |
| 5 回を超えるリダイレクト | ファイルなしとして取得します | 24 時間 |

- ページの URL は、投稿の公開範囲にかかわらず、リクエストする前に確認します。リダイレクト先の各 URL も、その URL のオリジンの `robots.txt` に照らして確認し、拒否された URL へのリダイレクトはそこで終わります。サーバーはそれをリクエストせず、カードも作りません。
- `robots.txt` が拒否した URL は、ほかのページの失敗と同じく、24 時間は再び取得しません。ファイルが読めなかっただけの場合は、ジョブの試行回数の上限まで、1 時間後にページを再び試します。
- 画像も、そのオリジンに照らして同じように確認します。拒否された画像は、画像のないカードになります。ファイルが読めなかっただけの場合は、前に記録した画像が残り、新しい画像は次の再取得まで含めません。
- 再取得（最後の取得から 14 日後に投稿がきっかけとなりえます）は、`robots.txt` がその URL を拒否していればカードを取り除きます。ファイルが読めなかっただけの場合、カードは残ります。
- サーバーはカードを見回って確認し直すことはしないので、ルールを変えたサイトのカードは、そのカードの次の再取得で消えます。

`link_cards_ignore_robots_txt`（[インスタンスの設定](instance-settings.md#settings-one-key-one-value)）は、これらをすべて無効にします。サーバーは `robots.txt` を取得も使用もしません。それでも `my-linkcard` と名乗り、リンク先のサイトは `robots.txt` で取得を拒否できなくなります。残るのは、そのページの指定と、[そのドメインのルール](instance-settings.md#link-card-domains)をあなたに頼むことだけで、[サイトの運営者向けのページ](#the-page-for-site-operators)がそれを伝えます。

<!-- source: directives-of-a-page a854ef2ae8ea -->
### ページの指定

サーバーは、取得したページの指定も読みます。`<head>` の中の `<meta name="robots">` と `<meta name="my-linkcard">`、そしてボットの名前のない、または `my-linkcard:` の付いた `X-Robots-Tag` です。ほかのボット向けの指定は適用しません。複数あるときは、厳しいほうが優先されます。
これらは `link_cards_ignore_robots_txt` にかかわらず適用します。

| 指定 | カード |
| --- | --- |
| `nosnippet`、`max-snippet:0` | 説明なし |
| `noimageindex`、`max-image-preview:none` | 画像なし。画像は取得しません |
| `noindex`、`nofollow`、`noarchive`、`none`、その他 | 変わりません |

題名、サイト名、作者は、どの指定でも残ります。それらを望まないサイトは、`robots.txt` で取得を拒否してください。
指定はページを取得したあとでなければわからないので、ページへのリクエストは止めません。再取得では、その時点でページにある指定を適用します。

<!-- source: limits-per-host 41838af70d25 -->
### ホストごとの制限

クローラーは、`link_cards_ignore_robots_txt` にかかわらず、各ホストに穏やかに接します。ホストとは URL の中の名前（ポートを除く）なので、リンク先のサイトとその画像のホストは別々の制限を持ちます。

- 1 つのホストからは一度に 1 つずつ取得し、次の取得は前の取得が終わってから少なくとも 1 秒後に始めます。1 回の取得とは、1 つのリクエストとそのリダイレクト、そして読む必要があるときは、それに必要な `robots.txt` です。ページとその画像は、同じホストにあっても 2 回の取得です。
- `429` か `503` は、それを返したホストからの取得を止めます。リダイレクト、画像、`robots.txt` への応答であっても同じです。止める期間はその `Retry-After`（秒数か日付）で、最短 1 秒、最長 1 時間です。`Retry-After` がないか読めない場合は 1 分です。
- 順番を待たなければならないジョブは、あとで戻ってきます。どのワーカーも待たず、ジョブの試行回数も使わないので、1 つのサイトへのリンクが一度に集まっても、すべてのカードがそろい、失敗するジョブはありません。1 つのホストのジョブは、およそ 1 秒に 1 つずつ起きます。
- ホストがクローラーを止めた URL は、カードを保ち、失敗として記憶されず、停止が終わると取得されます。1 つの URL について 24 時間以内にそのような応答が 5 回あると、そのページは `200` 以外のほかのページと同じく失敗とみなされ、画像はカードから除かれます。
- 画像を待っているカードは、ページを再びリクエストしないよう、ページから読んだものを最大 2 時間保持します。
- [ドメインのルール](instance-settings.md#link-card-domains)とドメインへの措置が先に適用されます。何も取得しないホストについては、何も待ちません。

制限はキーバリューストアにあり、すべて期限が切れます。取得の途中で止まったワーカーは、2 分以内にそのホストを解放します。
これらの数値は設定ではありません。制限の働きを見るには次のものを使います。

- `my.link_card.host_pauses` は、ホストが返したステータスごとに停止の回数を数えます。停止のたびに、`host`、`status`、`pause_seconds` とともに `INFO` で `link card host paused` をログに出力します。
- `my.job.type="content.fetch_link_card"` と `my.outcome="postponed"` の付いた `my.job.duration` は、あとで戻ってきた実行を数えます。大きな数は、少数のサイトへの多くのリンクか長い停止を意味し、失敗を意味するものではありません。
