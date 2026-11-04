# ADR-0057: 検索エンジンと AI クローラーの表明

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #647, #808

## Decision（決定）

表明は `robots.txt` を軸にし、`noindex` と IETF AI Preferences と TDMRep を重ねる。`noai` / `noimageai` も付ける。サーバーと個人の両方に設定を持ち、実際の表明は両者のうち制限の強いほうにする。

### 1. 設定

| 単位 | 設定 | 値 | 既定 |
| --- | --- | --- | --- |
| サーバー（インスタンス設定、`instance_setting_revisions` のキー） | `search_indexing` | `allow` / `deny` | 未設定なら `deny` |
| サーバー | `ai_usage` | `allow` / `deny_training`（学習を拒否）/ `deny`（学習と AI での利用を拒否） | 未設定なら `deny` |
| サーバー | `indexable_default` | 新規アカウントの `indexable` の既定（`true` / `false`） | 未設定なら `false` |
| サーバー | `security_contact` | `mailto:` か `https:` の URI。空なら `security.txt` を返さない | 空 |
| 個人（`account_profile_revisions`） | `indexable`（既存） | `true` / `false` | 登録時に `indexable_default` の値 |
| 個人 | `ai_opt_out` | `true` で学習と AI での利用を拒否 | `false`（サーバーに従う） |
| 個人 | `discoverable`（既存） | ディレクトリとおすすめへの掲載。クローラーの表明には使わない | `false` |

- あるページの実際の表明は、**サーバーの設定と、そのページの持ち主の設定のうち制限の強いほう**にする（IETF AI Preferences の「最も制限の強いものが勝つ」と同じ）。
  - 検索: `search_indexing = allow` かつ持ち主の `indexable = true` のときだけ索引を許す。
  - `indexable` は「公開投稿とプロフィールを検索（このサーバーの検索、外部の検索エンジン）に載せてよい」を表す。外部の検索エンジン用の設定を別に持たない。
  - AI: サーバーの `ai_usage` と、持ち主の `ai_opt_out` から決める。個人はサーバーより広く許可できない。
- 設定の無いサーバーは、検索エンジンの索引と、AI の学習と利用をすべて拒否し、新規アカウントの `indexable` も `false` にする。運営者は管理画面でそれぞれを許可の側に変えられる。
  - `indexable_default` を変えても、既存のアカウントの `indexable` は変わらない（登録時の既定にだけ効く）。
- 管理画面とクライアントは、サーバーが拒否しているときに個人の設定が効かないことを示す（設定の画面に説明を出す）。

### 2. robots.txt

`GET /robots.txt` を Go が動的に返す（`text/plain; charset=utf-8`、`Cache-Control: public, max-age=3600`、ETag）。
外枠と同じく `reservedPrefixes` に入れ、SPA の外枠は返さない。`account_domain` を分けている構成では、`host` 側で返す（ハンドル側は運営者の領分）。
次の例は設定の無いサーバー（`search_indexing = deny`、`ai_usage = deny`）のもの。

```text
# AI の学習のクローラー（ai_usage が deny_training か deny のとき）
User-agent: GPTBot
User-agent: ClaudeBot
User-agent: CCBot
User-agent: Google-Extended
User-agent: Applebot-Extended
# …同梱の一覧の「学習」のもの
Disallow: /

# AI の検索・利用者の依頼のクローラー（ai_usage が deny のとき）
User-agent: OAI-SearchBot
User-agent: ChatGPT-User
User-agent: Claude-SearchBot
User-agent: Claude-User
User-agent: PerplexityBot
# …同梱の一覧の「AI での利用」のもの
Disallow: /

User-agent: *
Content-Usage: search=n, train-ai=n, ai-use=n
Content-Signal: search=no, ai-train=no, ai-input=no
Disallow: /api/
Disallow: /oauth/
Disallow: /auth/
Disallow: /ap/
Disallow: /media/proxy/
Allow: /
```

- `User-agent: *` の群は、検索エンジンの拒否にも `Disallow: /` を使わない。取得を止めると `noindex` を読めず URL だけが索引に残ることがあり、
  リンクプレビューの取得者の一部（`Twitterbot` など）は robots.txt に従うので、ADR-0028 のプレビューが止まる。検索の拒否は `noindex` で表す。
- `Content-Usage` と `Content-Signal` の値はサーバーの設定から作る（`search_indexing = allow` なら `search=y` / `search=yes`）。
  未知の行は RFC 9309 のパーサーが無視するので、草案の行を足しても既存のクローラーを壊さない。
- `/.well-known/` と `/nodeinfo` は拒否しない。統計の収集やサーバーの一覧（FR-402）を止めないため。
- AI クローラーの一覧は `ai-robots-txt/ai.robots.txt` の `robots.json`（MIT、AGPL-3.0 と両立）から作り、**用途（学習 / AI での利用）で分類した表をリポジトリに同梱する**。
  - 実行時には取得しない。リンクプレビューの取得者（`facebookexternalhit` `Twitterbot` `Discordbot` `Slackbot` など）と、汎用の検索エンジン（`Googlebot` `Bingbot` `Applebot`）は表に入れない。
  - 更新はスクリプトで差分を作って PR にする（月に 1 度程度、取り込む元の版を記録する）。
- 運営者が規則を書き足す手段は設けない。robots.txt はサーバーの設定と同梱の一覧だけから作る。

### 3. ページの meta と HTTP ヘッダー

| 応答 | 検索を拒否するとき | AI を拒否するとき |
| --- | --- | --- |
| プロフィールと `public` の投稿の外枠（ADR-0028） | `<meta name="robots" content="noindex, noarchive">` と `X-Robots-Tag: noindex, noarchive` | `Content-Usage`（`train-ai=n` か `train-ai=n, ai-use=n`）、`tdm-reservation: 1`、meta と `X-Robots-Tag` に `noai, noimageai` |
| `unlisted` の投稿、見せない・存在しないページの 404 | 常に `noindex` | サーバーの設定に従う |
| リモートのアカウントのページ・アプリの画面（`/home` `/settings` `/admin` など） | 常に `noindex` | サーバーの設定に従う |
| サポーターの一覧（`/about/supporters` の外枠と `GET /api/v1/instance/supporters`、[ADR-0070](0070-permission-split-emoji-finance.md) §13） | 常に `noindex, noarchive`（meta と `X-Robots-Tag`。API は問題の応答も） | 外枠はサーバーの設定に従う。API は付けない |
| `/`（汎用の外枠） | サーバーの設定に従う | サーバーの設定に従う |
| `/media/`（このサーバーが返すとき） | `search_indexing = deny` のとき `X-Robots-Tag: noindex`。持ち主ごとの表明はしない | サーバーの設定に従う。持ち主ごとの表明はしない |
| クライアント API・AP の JSON・`/.well-known/` | `X-Robots-Tag: noindex` | 付けない（robots.txt で足りる） |

- 外枠の ETag には、表明の値（サーバーの設定と持ち主の設定）を含める（ADR-0028 の「投稿の版」と同じ扱い）。
- `noindex` はリンクプレビューを止めない（ADR-0028 と [link-previews.md](../operations/link-previews.md) のとおり）。
- メディアは持ち主がキーに無く、CDN が直接返す構成もあるので、**持ち主ごとの AI の拒否はメディアに及ばない**。サーバー単位の表明は robots.txt で全体に効く。
  CDN でヘッダーを付ける方法をデプロイ文書に書く。

### 4. `/.well-known/`

| パス | 返す条件 | 中身 |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` が `allow` 以外 | `[{"location": "/*", "tdm-reservation": 1}]`。`ai_usage = allow` のときは `tdm-reservation: 0` ではなく 404（何も表明しない） |
| `/.well-known/security.txt` | `security_contact` があるとき | `Contact`、`Expires`（応答の時刻から 30 日後）、`Preferred-Languages`（既定の言語と `en`）、`Canonical` |
| 既存の `webfinger` `host-meta` `host-meta.json` `nodeinfo` | 変えない | — |

- `ai.txt` と `/.well-known/` の AI 用の独自の文書は出さない。大手のクローラーが読むという文書が無く、同名の提案が並立しているため。

### 5. サーバーの情報ページ

- サーバーの情報ページ（FR-718 の情報ページ）に、検索と AI の許否の現在の設定と、その表明の意味を出す。
  - 文言は ICU メッセージとして `locales/en` と `locales/ja` に置き（ADR-0009）、設定の値で出し分ける。法的な効果を保証する書き方はしない。
  - 法的な説明（`client:about.crawlers.legal`）は個別の国の法令に触れず、`ja` は「この表明が法的にどう扱われるかは国によって異なり、多くの事業者はこうした表明に自主的に従っています。この文章は法的な助言ではありません。」とする。
  - `en` は同じ内容の "What such a statement means in law depends on the country, and many operators follow such statements voluntarily. This text is not legal advice." とする。
- クライアント API のインスタンス情報に `search_indexing` と `ai_usage` を載せ、情報ページはそれを読んで表示する。

### 6. 連合

- Actor の `indexable` と `discoverable` は、Mastodon の定義のまま送る。
- リモートの Actor の `indexable` を正規化で読み、保存する。
- リモートのアカウントの投稿とプロフィールは、このサーバーで外部の検索エンジンに出さない（上表の `noindex`）。
  このサーバーの検索（将来の全文検索）は、`indexable = true` のリモートのアカウントの公開投稿だけを対象にする。
- 個人の `ai_opt_out` を AP の拡張で連合先に伝えることはしない。共通の語彙が無く、受け手が従う見込みも無い。
  連合先が返すページでの表明は連合先の設定に任せる。この限界を設定の画面の説明に書く。

### 7. 見直す条件

- IETF AI Preferences が RFC になり、主要な AI の事業者が `Content-Usage` に従うと表明したら、User-agent の一覧の保守をやめて `Content-Usage` を軸にする。
- Fediverse で AI の拒否を表す共通の AP の語彙（FEP など）が広まったら、Actor に拡張を足して連合先にも伝える形に切り替える。

## 経緯

個人単位・サーバー単位での検索インデックス拒否と AI クロール拒否の明示、正確な robots.txt の配置、必要な well-known の掲示が求められた（#647。関連: FR-006、FR-401、FR-402、FR-705、[ADR-0017](0017-visibility-local.md)、[ADR-0018](0018-federated-uris.md)、[ADR-0028](0028-html-shell-link-previews-and-pwa.md)）。当時は `/robots.txt` の経路がなく SPA の外枠の HTML が 200 で返り、`discoverable` / `indexable` はページにも検索にも効かず、リモートの Actor の `indexable` は捨てられていた。

判断の基準は、本人が拒んだものがサーバーの設定でも連合先での表示でも許可に変わらないこと、主要なクローラーが文書で「従う」としている手段を軸にして標準化の途中の手段は安く足せるものだけ足すこと、リンクプレビュー・存在を漏らさない 404・連合の取得を止めないこと、AI クローラーの一覧の保守を運営者に求めないこと、`indexable` / `discoverable` の連合での意味を Mastodon の定義とずらさないことである。

判断の元にした手段の状態（2026-10-20 に一次情報で確認）:

- `robots.txt` は標準（[RFC 9309](https://www.rfc-editor.org/rfc/rfc9309)）で、主要な検索エンジンと AI の各社が従うと明記する。`noindex` は事実上の標準（[Google](https://developers.google.com/search/docs/crawling-indexing/robots-meta-tag)）。
- AI 用の User-agent は各社の文書による（[Google](https://developers.google.com/crawling/docs/crawlers-fetchers/google-common-crawlers)、[OpenAI](https://developers.openai.com/api/docs/bots)、[Anthropic](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler)）。`Google-Extended` は取得をせず使用の可否だけを表す。`ChatGPT-User` は利用者の操作なので robots.txt が効かないことがある。一覧は [ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt)（MIT）が持つ。
- IETF AI Preferences（[vocab](https://datatracker.ietf.org/doc/draft-ietf-aipref-vocab/)、[attach](https://datatracker.ietf.org/doc/draft-ietf-aipref-attach/)）は草案で、衝突したら最も制限の強い表明が勝つ。[Content Signals](https://blog.cloudflare.com/content-signals-policy/) は Cloudflare の方針。[TDMRep](https://www.w3.org/community/reports/tdmrep/CG-FINAL-tdmrep-20240510/) は W3C Community Group の Final Report で、W3C 標準ではない。どれも大手のクローラーの対応は未確認（要検証）。
- `noai` / `noimageai` は非標準で（[IPTC](https://metawatch.iptc.org/ai-policy/noai-meta/)）、Misskey・GoToSocial が出す。`ai.txt` は同名の提案が並立し（[draft](https://www.ietf.org/ietf-ftp/internet-drafts/draft-car-ai-txt-wellknown-00.txt)）、大手のクローラーが読むという文書はない。`security.txt` は標準（[RFC 9116](https://www.rfc-editor.org/rfc/rfc9116)）。
- 主要実装（2026-10-20 にソースで確認）: Mastodon は静的な robots.txt と利用者の `noindex`、Actor の `discoverable` と `indexable` を持つ。Misskey は動的な robots.txt と `preventAiLearning`。GoToSocial は ai.robots.txt の一覧をそのまま拒否し、常に `noai, noimageai` を出す。

情報ページの説明は当初、日本の著作権法 30 条の 4 と EU DSM 指令 4 条（機械可読な TDM の留保）に対する位置づけを含むとしていた。人による文言の確認（#808）で、決定者が個別の法令に触れない §5 の文を指定した。

## 採らなかった案

- `robots.txt` と `noindex` だけ: 実装は最小だが、IETF AI Preferences が RFC になったときに追いかける必要があり、機械可読の留保を求める場面で弱い。
- ai.robots.txt の一覧をそのまま `Disallow: /`: 保守が最も楽で一覧が最も広いが、`facebookexternalhit` や `Applebot` まで止め、ADR-0028 のリンクプレビューと矛盾する。
- サーバーの設定だけ: 単純だが、個人単位の表明ができない。
- 個人の設定がサーバーより優先: 個人が広く許可できるが、サーバー全体を拒否したい運営者の意図（robots.txt はサーバー全体にしか効かない）と矛盾する。
- 外部の検索エンジン用に `noindex` を別に持つ: Mastodon と同じ細かさだが、設定が増え、`indexable = true` かつ `noindex` のような分かりにくい組み合わせが生まれる。
- Actor に独自の拡張（`noai` など）を足す: 連合先に届く可能性があるが、受け手の実装がない語彙で、ADR-0006 の互換拡張の範囲を広げる。
- 既定をコードで固定する: 設定は少ないが、検索に載せたい運営者のサーバーで利用者全員が個別に許可する必要がある。
- 情報ページに法令への言及を残して文だけ短くする: 国ごとの位置づけを読めるが、決定者の確認した文言と異なる。

## Consequences（影響）

- `/robots.txt` が正しく返り、AI クローラーの拒否と検索の許否がサーバーと個人の両方で明示される。既存の `indexable` がページと検索に効き、Mastodon の定義と同じ意味で送受信される。リンクプレビューと連合の取得は止まらない。
- `indexable` の 1 つの設定で、連合での意味（公開投稿の索引）と外部の検索エンジンの扱いがそろう。Mastodon の利用者が想定する「検索エンジンには出るが全文検索には出ない」状態は作れない。
- robots.txt も meta も従うかどうかはクローラー次第で、技術的な遮断ではない。遮断はこの ADR の対象外（WAF・レート制限）。
- 同梱の AI クローラーの一覧は古くなる。更新の PR を定期的に作る必要がある。
- 設定の無いときの `ai_usage = deny` は、ChatGPT などで利用者がこのサーバーのページを読ませる操作も拒む表明になる。
- 設定の無いサーバーは検索エンジンに載らない。検索に載せたい運営者は `search_indexing` と `indexable_default` の両方を変える必要があり、`indexable_default` を変える前に登録した利用者は自分で `indexable` を選ぶまで載らない。
- 持ち主ごとの AI の拒否はメディアと連合先のページに及ばない。
- `noai` / `noimageai`・`Content-Usage`・`Content-Signal`・TDMRep は読み手が少ないか未確認で、効果は限られる。表明の手段が多く、テストの対象が増える。
- 情報ページは、表明の法的な扱いが国によって異なることと、法的な助言ではないことだけを伝え、国ごとの法的な位置づけは読めない。FR-725 の情報ページに出す内容もこれに合わせる。

## Open Questions / 要検証

- IETF AI Preferences と Content Signals に対応すると表明した大手のクローラーがあるか（要検証）。
- Tauri のアプリ（`/.well-known/assetlinks.json` と `apple-app-site-association`）と `/.well-known/change-password` は、それぞれの機能の Issue で扱う。
