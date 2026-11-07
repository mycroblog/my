<!-- source: search-engines-and-ai-crawlers 648c61a01042 -->
# 検索エンジンと AI クローラー

サーバーが検索エンジンと AI クローラーに何を伝えるか、それを変える設定、そしてサーバーが返す内容を確かめる方法を説明します。

robots.txt とメタタグは、クローラーが従うことも従わないこともある意思表示であり、技術的にアクセスを止めるものではありません。止める必要があるときは、WAF やレート制限を使ってください。

<!-- source: settings 13dfb1b04948 -->
## 設定

| 範囲 | 設定 | 変更する場所 | 未設定のとき |
| --- | --- | --- | --- |
| サーバー | `search_indexing`（`allow` / `deny`） | 管理画面のサーバー設定の「検索とクローラー」ページにある「検索エンジンと AI」セクション、`my instance set`、`PATCH /api/v1/admin/settings`（[インスタンス設定](instance-settings.md)） | `deny` |
| サーバー | `ai_usage`（`allow` / `deny_training` / `deny`） | 同上 | `deny` |
| サーバー | `indexable_default`（新しいアカウントの `indexable`） | 同上 | `false` |
| サーバー | `security_contact`（脆弱性の報告先） | 同上 | 空（`security.txt` は返しません） |
| 個人 | `indexable`（検索に表示する） | プロフィール設定の「検索エンジンと AI」、`PATCH /api/v1/accounts/update_credentials` | 登録時の `indexable_default` |
| 個人 | `ai_opt_out`（AI による学習と利用を拒否する） | 同上 | `false` |

- ページの意思表示は、サーバーの設定とページの持ち主の設定のうち、厳しいほうになります。個人がサーバーより多くを許可することはできません。
- 検索エンジンにインデックスしてよいと伝えるのは、`search_indexing = allow` のサーバーで、`indexable = true` のアカウントのプロフィールと `public` の投稿だけです。
  検索されたいサーバーは、`search_indexing` と `indexable_default` の両方を変更します。`indexable_default` を変更する前に登録した人は、本人が `indexable` を選ぶまで対象になりません。
- 変更した設定は、再起動しなくても `/robots.txt`、ページ、メディアのレスポンスに反映されます。

<!-- source: what-the-server-returns 31584e03601b -->
## サーバーが返す内容

| レスポンス | 検索を拒否するとき | AI を拒否するとき |
| --- | --- | --- |
| プロフィールと `public` の投稿のページ | `<meta name="robots">` と `X-Robots-Tag` に `noindex, noarchive` | `noai, noimageai`、`Content-Usage: train-ai=n`（`deny` では `train-ai=n, ai-use=n`）、`tdm-reservation: 1`（ヘッダーとメタタグ） |
| `unlisted` の投稿、非表示または存在しないページの 404 ページ、リモートアカウントのページ、アプリの画面（`/home`、`/settings` など） | 設定にかかわらず `noindex, noarchive` | サーバーの設定に従う |
| `/`、`/about`、`/about/crawler` | サーバーの設定に従う | サーバーの設定に従う |
| `/media/`（このサーバーが配信する場合） | サーバーの設定に従う。持ち主ごとの意思表示はなし | サーバーの設定に従う。持ち主ごとの意思表示はなし |
| クライアント API、ActivityPub のドキュメント、`/.well-known/`、`/nodeinfo/`、サインインと OAuth の画面 | `X-Robots-Tag: noindex` | 付けない（robots.txt で十分なため） |

`noindex` はリンクプレビューを止めません（[リンクプレビュー](link-previews.md)を参照してください）。

<!-- source: robotstxt 636c85dabf63 -->
### `/robots.txt`

`GET /robots.txt` は、サーバーの設定と同梱の AI クローラーの一覧から組み立てられます（`text/plain; charset=utf-8`、`Cache-Control: public, max-age=3600`、ETag）。
運営者が独自のルールを加えることはできません。

- `ai_usage` が `deny_training` または `deny` のとき、学習用のクローラー（`GPTBot`、`ClaudeBot`、`CCBot`、`Google-Extended`、`Applebot-Extended` など）には `Disallow: /` を返します。
- `ai_usage` が `deny_training` または `deny` のとき、`Disallow: /` を持つ `User-agent: __usage_ai-training` グループで、どのクローラーがコンテンツを取得するかにかかわらず、すべてのパスについて AI の学習を望まないことを表明します
  （Internet-Draft の `draft-kelley-aipref-training-preference-signal`）。このグループはクローラーではなく用途を指すため、取得を止めるものではありません。`ai_usage = allow` ではこのグループを省き、何の意思も表明しません。`Allow: /` を書くと、`ai_opt_out` を設定した人のページも含めて、すべてのパスについて学習を認める意思を表明することになるためです。
- `ai_usage` が `deny` のとき、AI 検索、アシスタント、エージェントのクローラー（`OAI-SearchBot`、`ChatGPT-User`、`Claude-User`、`PerplexityBot` など）にも `Disallow: /` を返します。
- すべてのクローラー（`User-agent: *`）に対して拒否するのは `/api/`、`/oauth/`、`/auth/`、`/ap/`、`/media/proxy/` だけで、設定は `Content-Usage`（IETF AI Preferences のドラフト）と
  `Content-Signal`（Content Signals）の行で表明します。検索を拒否しても `Disallow: /` にはなりません。取得を止めると、クローラーが `noindex` を読めなくなり、
  robots.txt に従うリンクプレビューの取得も止まってしまうためです。
- サーバーの一覧や統計が動き続けるように、`/.well-known/` と `/nodeinfo` は拒否しません。
- `account_domain` がホストと別のときは、ホスト側が robots.txt を返します。ハンドルのドメインの robots.txt は運営者自身が管理します。

<!-- source: well-known 72d7e8cbbc33 -->
### `/.well-known/`

| パス | 返すとき | 内容 |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` が `allow` 以外のとき | `[{"location": "/*", "tdm-reservation": 1}]`（TDM Reservation Protocol）。`allow` では 404 |
| `/.well-known/security.txt` | `security_contact` が設定されているとき | `Contact`、`Expires`（レスポンスの時刻から 30 日後）、`Preferred-Languages`（`default_locale` と `en`）、`Canonical`（RFC 9116） |

<!-- source: information-page 0f89d6c3c3ce -->
### 情報ページ

インスタンスの情報ページ（`/about`）には、`GET /api/v1/instance` の `search_indexing` と `ai_usage` から、検索と AI が許可されているかどうかと、その意思表示の意味が表示されます。
その説明では、意思表示を日本の著作権法第 30 条の 4（このような意思表示が法的な拒否として扱われないことがあります）と、EU の DSM 指令第 4 条（テキストとデータのマイニングの機械可読な留保）に照らして位置づけています。
法的な効果を保証するものではありません。

<!-- source: this-servers-own-fetching 0cb9510e19e5 -->
## このサーバー自身による取得

ここまでの設定は、このサーバーをクロールするものについての設定です。このサーバーも、投稿内のリンクのカードを作るために `my-linkcard` としてほかのサイトをクロールします。その際は、[リンクプレビュー](link-previews.md#cards-of-links-in-posts)で説明するとおり、`robots.txt` とリンク先のページのディレクティブに従い、ホストごとのリクエスト数を制限します。
このクローラーは `link_cards`、`link_cards_ignore_robots_txt`、そしてドメインごとに[リンクカードのドメイン](instance-settings.md#link-card-domains)で設定します。

それらのサイトの運営者は、`/about` からリンクされている `https://<host>/about/crawler` でこのクローラーについて知ることができます（[サイト運営者向けのページ](link-previews.md#the-page-for-site-operators)）。`link_cards_ignore_robots_txt` がオンのときは、`robots.txt` で拒否することはできず、そのページにもそう書かれます。サイト運営者にできるのは、自分のページのディレクティブでカードを制限するか、自分のドメインについてのルールをあなたに頼むことだけです。

<!-- source: limits 93da7e422ee8 -->
## 制限事項

- 個人の `ai_opt_out` が表明されるのは、このサーバーが返すページだけです。メディアのキーには持ち主が含まれず、構成によっては CDN が直接配信するため、メディアにはサーバー全体の意思表示だけが付きます。
  ほかのサーバーが返すページでの意思表示は、それぞれのサーバーに任されます（独自の ActivityPub 拡張は送りません）。
- `ai_usage = deny` は、ChatGPT などのサービスの利用者がそのサービスにこのサーバーのページを読ませる操作に対しても、拒否を表明します。
- リモートアカウントの `indexable`（Mastodon の `toot:indexable`）は受け取って保存します。このサーバーでは、リモートアカウントのページは常に `noindex` です。
- `indexable` は、検索エンジンのあるサーバーで、その人の投稿をこのサーバー自身の[全文検索](search.md)に含めるかどうかも決めます。そこでは `search_visibilities` の公開範囲が対象になり、`local` を含めることもできます。`search_indexing` はその検索には影響せず、検索エンジンに伝える内容はこのページのとおりのままです。

<!-- source: serving-media-from-object-storage-or-a-cdn 98c7ab4994ab -->
## オブジェクトストレージや CDN からのメディアの配信

オブジェクトストレージや CDN がメディアを直接配信するときは、`/media/` のレスポンスはこのサーバーを通らず、上の表のヘッダーも付きません。
サーバー全体の意思表示は `/robots.txt` によって引き続きすべてに適用されます。ヘッダーも付けるには、CDN のレスポンスヘッダーのルールで `/media/*` に次を加えてください（`ai_usage = deny` の例）。

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

設定を変更したときは、それに合わせて CDN のルールも変更してください。`/media/proxy/` はこのサーバーが配信するため、ルールは必要ありません。

<!-- source: the-ai-crawler-list 675a37cb0caf -->
## AI クローラーの一覧

一覧は [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt)（MIT）の `robots.json` から生成され、バイナリに同梱されています。実行時には取得しないため、新しい一覧はアップグレードとともに入ります。

<!-- source: verify 1633f03abedb -->
## 確認する

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- 何も設定していないサーバーでは、`/robots.txt` に `User-agent: GPTBot`、`User-agent: __usage_ai-training`、`Content-Usage: search=n, train-ai=n, ai-use=n` が含まれ、
  プロフィールには `X-Robots-Tag: noindex, noarchive, noai, noimageai` が付きます。
- `search_indexing = allow` かつ `ai_usage = allow` のとき、`indexable` を選んだアカウントのプロフィールには `X-Robots-Tag` が付かず、`/.well-known/tdmrep.json` は 404 になります。
