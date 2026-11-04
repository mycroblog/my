# ADR-0018: 連合 URI は ID ベースで `/ap/` 名前空間に置き、表示用 URL と分離する

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #36, #643, #2079

## Decision（決定）

AP の `id` には UUIDv7 だけを使い、`/ap/` 名前空間に置く。人間向けの表示用 URL（`url`）は別に持ち、こちらは変えてよいものとして扱う。

### 1. URI 一覧（`{host}` = 配信ドメイン、`{uuid}` = UUIDv7 の 36 文字小文字表記）

| 対象 | URI | 例 |
| --- | --- | --- |
| Actor（Person / Service / インスタンス Actor） | `https://{host}/ap/actors/{uuid}` | `https://social.example.org/ap/actors/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f` |
| 公開鍵 | `{actor}#main-key` | `https://social.example.org/ap/actors/0192f3a4-…-1a2b3c4d5e6f#main-key` |
| inbox / outbox | `{actor}/inbox`、`{actor}/outbox` | `…/ap/actors/0192f3a4-…/inbox` |
| followers / following | `{actor}/followers`、`{actor}/following` | `…/ap/actors/0192f3a4-…/followers` |
| ピン留め（`featured`）/ 注目タグ（`featuredTags`） | `{actor}/featured`、`{actor}/featured-tags` | `…/ap/actors/0192f3a4-…/featured` |
| shared inbox | `https://{host}/ap/inbox` | `https://social.example.org/ap/inbox` |
| 投稿（Note / Question など） | `https://{host}/ap/objects/{uuid}` | `https://social.example.org/ap/objects/0192f3b0-2c3d-7e4f-8a00-112233445566` |
| 投稿の `Create` | `{object}/activity` | `…/ap/objects/0192f3b0-…/activity` |
| 投稿の replies / likes / shares | `{object}/replies` など | `…/ap/objects/0192f3b0-…/replies` |
| その他のアクティビティ（Follow / Like / Announce / Block / Undo / Update / Delete / Accept / Reject / Flag） | `https://{host}/ap/activities/{uuid}` | `https://social.example.org/ap/activities/0192f3c1-…` |
| カスタム絵文字（ADR-0051） | `https://{host}/ap/emojis/{uuid}` | — |
| 引用の承認（ADR-0069 §1） | `https://{host}/ap/quote_authorizations/{uuid}` | — |
| プロフィール（HTML、`url`） | `https://{host}/@{username}` | `https://social.example.org/@alice` |
| 投稿（HTML、`url`） | `https://{host}/@{username}/status/{uuid}`（§8） | `https://social.example.org/@alice/status/0192f3b0-2c3d-7e4f-8a00-112233445566` |
| ハッシュタグ（HTML、`Hashtag.href`） | `https://{host}/tags/{name}` | `https://social.example.org/tags/fediverse` |
| WebFinger の `subject` | `acct:{username}@{account_domain}` | `acct:alice@example.org` |

規則:

- `id` は https・小文字・末尾スラッシュなし・クエリなし（コレクションのページ `?page=…` を除く）。パスにユーザー名を含めない。
- 受信したアクティビティ中の自サーバーの URI は、`internal/interface/activitypub/uris.go` の `parseLocalURI` が接頭辞（`/ap/actors/` など）と `kernel.ValidID` で厳密にパースし、HTTP 取得せずに DB を引く。プロフィールの URL（`/@{username}`）も受ける。
- `Create` の `id` は投稿から決まる値（`{object}/activity`）にして保存しない。`Update` は編集ごとに別のアクティビティなので `/ap/activities/{uuid}` を使う。
- メディアの URL は `id` ではない（Blob / CDN のドメインに置いてよい。ADR-0005）。
- この一覧は `id` の形を決めるもので、配信の約束ではない。配信する URI は §16。

### 2. ドメイン

- 設定は `host`（配信ドメイン。`id` はすべてこのドメイン）と `account_domain`（ハンドルのドメイン。省略時は `host`）の 2 つ。
- 2 つの値は初回起動時に DB に記録する。設定ファイルの値と食い違ったら起動を拒否する（連合した後の変更事故を防ぐ）。
- 分離する場合、`account_domain` の `/.well-known/webfinger`、`/.well-known/host-meta`、`/.well-known/nodeinfo` を `host` へリダイレクトする（CORS ヘッダー付き）。
  `host` 側の WebFinger は `acct:alice@example.org` と `acct:alice@social.example.org` の両方に答え、`subject` は常に `account_domain` 側のハンドルで返す。

### 3. HTML と AP JSON の振り分け

- `/ap/*` は AP 用。`Accept` が `application/activity+json` または `application/ld+json; profile="https://www.w3.org/ns/activitystreams"` なら JSON（`Content-Type: application/activity+json`）を返す。
  HTML を求めるリクエストは未ログインとして扱い、未ログインで閲覧できる投稿だけを表示用 URL へ `303` でリダイレクトし、それ以外は 404 を返す（§14）。
- 表示用 URL（`/@alice`、`/@alice/status/{uuid}`）に AP の `Accept` で来たら、対応する `id` へ `302` でリダイレクトする（URL を貼ってのリモート検索、FR-305 用。投稿は §8 の確認を通す）。
- 振り分けを行うレスポンスにはすべて `Vary: Accept` を付ける。
- `/@{任意の名前}/status/{uuid}` は投稿 ID だけで引く。名前が現在のユーザー名と違えば正しい URL へ `301` でリダイレクトする（§8）。

### 4. アクセス制御（ADR-0017）

- `/ap/objects/{uuid}`（`/activity` と各サブコレクションも含む）は、リクエストの HTTP 署名から閲覧者を決め、`canView` を通す。
  `public` / `unlisted` は署名なしでも返す（Authorized Fetch が無効のとき）。`local` / `private` / `direct` は署名者に権限があるときだけ返す。
- **権限が無いときと存在しないときは、同じ 404、同じヘッダーで返す**（`Cache-Control: no-store` を含む）。応答の違いから存在が漏れないようにする。これは公開範囲で隠すもののための規則で、削除された Actor は §12 のとおり `410` を返す。
- outbox が列挙するのは `public` / `unlisted` の投稿だけ。`followers` / `following` は、利用者の設定に応じて件数だけを返すこともできる。
- UUIDv7 は推測されにくいが、時刻を含む。推測されにくいことをアクセス制御の根拠にはしない。

### 5. CDN キャッシュ

- `/ap/*` と PWA の静的ファイルはパスのプレフィックスで分け、CDN・リバースプロキシのルールを単純にする。
- 匿名で閲覧できる Actor と `public` / `unlisted` の投稿だけ、`Cache-Control: public, max-age=60`（値は要調整）を付けてよい。
  それ以外（Authorized Fetch が有効なとき、`local` 以下の公開範囲、404）は `private, no-store`。
- `Vary: Accept` を正しく扱わない CDN があり得る（要検証）。その場合は CDN 側で `/ap/*` をキャッシュ対象外にする設定をデプロイテンプレートの既定にする。

### 6. ユーザー名の変更

- `id` にユーザー名を含めないので、改名しても `id` は変わらない。改名時は `Update(Person)` で新しい `preferredUsername` と `url` を配送する。
- 古いユーザー名は、一定期間（既定 30 日、要調整）他人が取得できないよう予約する。その間、`/@old` は `/@new` へリダイレクトする。古いハンドルでの WebFinger には答えない（なりすましの混乱を避ける）。
- 改名を v0 の機能として提供するかは別に決める（この ADR は改名を妨げない形式を決めるだけ）。

### 7. この決定を見直す条件

- 主要実装のいずれかが `/ap/` 配下の ID ベース URI と相互運用できない（例: パスの形式を前提にした処理がある）ことが分かった場合
- 運用者がユーザー名入りの URL を強く求め、改名を恒久的に提供しないと決めた場合（ユーザー名ベースの案が有力になる）
- 主要実装のいずれかが、`url` のパスの形を前提にした処理（例: `/@{username}/{id}` を Mastodon の形とみなす推測）で、§8 の形の URL を解決できないと分かった場合
- 公開済みのインスタンスが増え、v0 の間でも古い `url` のリダイレクトを持つべきだと決定者が判断した場合

### 8. 投稿の表示用 URL

| 対象 | URL | 例 |
| --- | --- | --- |
| ローカルの投稿（HTML、Note の `url`） | `https://{host}/@{username}/status/{uuid}` | `https://social.example.org/@alice/status/0192f3b0-2c3d-7e4f-8a00-112233445566` |
| リモートの投稿（Web クライアントの経路） | `/@{username@domain}/status/{id}` | `/@bob@remote.example/status/0192f3b0-…` |

- 投稿は ID だけで引く。`{username}` は読みやすさのためだけにあり、現在のユーザー名と違えば、正しい URL へ 301 でリダイレクトする（改名後も古いリンクが切れない）。
- AP の `Accept`（§3 の判定）で `/@{username}/status/{uuid}` に来たら、投稿の `id`（`/ap/objects/{uuid}`）へ 302 でリダイレクトする（FR-305。Mastodon の検索に URL を貼ると投稿が引ける）。
  リダイレクトの前に `canView` で匿名の閲覧者が見られるかを確かめ、見られなければ外枠の 404 と同じに扱う（ADR-0017）。
- ローカルの投稿の `url` は保存せず、読み出すときに作者の現在のユーザー名と投稿 ID から `StatusURL` で作る。
  改名と URL の形の変更が、保存済みの値に残らないようにするため。`statuses.url` はリモートの投稿の `url` だけを持つ。
- AP の `id` は `https://{host}/ap/objects/{uuid}` のままで変わらない。
- OGP 付きの外枠（ADR-0028）は `GET /@{acct}/status/{id}` で返す。

### 9. `status` と `statuses`

- `status` を使う。「この投稿」を指す 1 件のページで、X と同じ語順なので、共有された URL から投稿だと読み取りやすい。
- `statuses` は REST の「コレクションの下の要素」の形で、GoToSocial と Mastodon のクライアント API（`/api/v1/statuses/{id}`）と同じ。
  `/@{username}/statuses` を投稿一覧のページにする余地が残るが、投稿一覧はプロフィール（`/@{username}`）そのものなので、使い道が無い。
- どちらも AP の相互運用には影響しない。

### 10. `/@{username}/` の下の名前空間

- `/@{acct}/` の直下の第 1 区間は、次の規則の固定の語だけにする。可変の値（ID、タグ名など）を第 1 区間に置かない。
  可変の値は語の下（`/@{acct}/status/{id}`、`/@{acct}/tagged/{tag}` など）に置く。
- 語は小文字の ASCII 英字・数字・ハイフン・アンダースコア（`[a-z][a-z0-9_-]*`）。単数か複数かは、そのページが 1 件を指すか一覧かで決める。
- 予約語の一覧（現在のページと、将来のページ）:

  | 語 | ページ | 状態 |
  | --- | --- | --- |
  | `status` | 投稿の詳細（`status/{id}`）。`status/{id}/…` も投稿の下のページ用に予約する | 実装済み |
  | `followers` | フォロワー一覧 | 実装済み |
  | `following` | フォロー一覧 | 実装済み |
  | `media` | メディア付きの投稿 | 予約 |
  | `favorite` | お気に入り | 予約 |
  | `with_replies` | 返信を含む投稿 | 予約 |
  | `tagged` | ハッシュタグで絞った投稿（`tagged/{tag}`） | 予約 |
  | `featured` | ピン留めした投稿 | 予約 |

- 一覧の置き場所: Web クライアントの経路のファイル（`app/web/src/routes/@{$acct}_.*.tsx`）が実体で、
  語を足すときは [web-client の経路の説明](../architecture/web-client/09-app-shell.md) の一覧を更新する。
  Go が知るのは、サーバーが役割を持つ語（外枠の meta タグと AP のリダイレクトを返す `status`）だけで、それ以外の語は汎用の外枠を返す。
  語を足すのに ADR は要らないが、第 1 区間に可変の値を置く変更はこの ADR の決定の変更を要する。

### 11. 既存の `/@{username}/{uuid}`

- 互換のためのリダイレクトは持たない（ADR-0023）。`/@{username}/{uuid}` は予約語でない第 1 区間なので、Web クライアントの「ページが無い」になる。
- 連合先が保存した古い `url` と、外部に貼られた古いリンクは切れる。投稿の識別（`id`）は変わらないので、連合先での返信・ブースト・お気に入りは影響を受けない。
  公開済みのインスタンスがあれば、利用者に URL の変更を告知する（v0 の試験運用の前提、ADR-0023）。

### 12. 削除された Actor は `410`

削除されたローカルの Actor の GET は `410 Gone` と `Tombstone`（`id` は Actor の URI）、WebFinger は `410` を返す。`/@{username}` への ActivityPub の要求も同じ。消去の後も同じ。存在しないユーザーは `404` のまま。

- `Delete` を受け損ねたサーバーの残骸を減らせる。Mastodon は WebFinger の `410` でそのアカウントを消し、このサーバーの受信も Actor の `410` を `remote_deleted` にする。
- §4 の「権限が無いときと存在しないときは同じ `404`」は、公開範囲で隠すもののためのもの。Actor は公開で、削除は `Delete` で知らせているので、`410` で新しく漏れるものはない。
- `410` にするのは `deleted` を記録した時点からで、配送の終わりを待たない。待つには配送の終わりを記録する必要があり、配送中に `410` を受けたサーバーの結果は `Delete` と同じ。配送中の Actor はもともと取得できないので、`Delete` の署名の検証で悪くなることはない。

ユーザーの Actor が凍結中や無効化中（ADR-0089）なら、削除されるまでは最小の文書を返す。

### 13. 引用のフォールバックは先頭の `<p>`

`quote`（または方言）を付けた Note の本文の先頭に、`<p class="quote-inline">RE: <a href="…">…</a></p>` を付ける（ADR-0069 §3 の送信の形）。Mastodon v4.7.4 と同じ形で、FEP-044f の形は規範ではない。

### 14. HTML の要求はセッションを読まない

`/ap/*` に HTML を求める要求は未ログインとして扱い、未ログインで閲覧できる投稿だけを表示用 URL へ `303` で送る。それ以外は `404`。

- `/ap/*` はサーバーどうしの URI で、クライアントの UI からリンクしない（`url` が表示用 URL）。ログイン中の人が `/ap/` の URL を手で開く場面のために、ActivityPub の入口へセッションの依存を足さない（ADR-0002 の層の分離、実装を増やさないこと）。
- 代償として、ログイン中の人でも `local` の投稿の `/ap/` の URL は `404` になる。表示用 URL は開ける。

### 15. 引用元の公開範囲

引用元の公開範囲は次のとおり制限する（ADR-0069 §4 と同じ規則）。

- `local`・`private`・`direct` の投稿は、リモートの投稿を引用できない。投稿の作成で断る（`quoteTarget`）。`QuoteRequest` の `instrument` が、引用元の IRI を宛先の外へ知らせるため。
- ローカルの `local` の投稿を `public` や `unlisted` で引用することは、引き続きできる。その Note を連合するときは、`quote`・併記のプロパティ・`quoteAuthorization`・フォールバックを除く。ローカルの閲覧者には埋め込みを出す。
  - 除くのは引用元の公開範囲によらない。`local` や `private` の引用元の Note も、引用先の作者のフォロワーがいないサーバーへ届くため。作者が自分の `local` の投稿を引用するときも同じ。
  - 引用先がこのサーバーの投稿で、行がもう無く公開範囲が分からないときも、同じように除く。
- その引用の `QuoteAuthorization` の GET は `404`。承認を取り消した後も、引用先を削除した後も同じ。引用先の行がもう無い承認は公開範囲が分からないので、ほかの承認と同じ `410` の `Tombstone` で、示すのは承認自身の URI だけ。

「外部へはフォロワーにしか届かない」（ADR-0017）は、投稿の中身だけでなく、その IRI と作者のハンドルも含むものとする。

### 16. 配信する URI

`/ap/activities/{uuid}` は、公開・未収載のブーストの `Announce` だけを、保存している事実から配信する。残りの活動・`/ap/objects/{id}/quote_request`・`{object}/replies`・`/likes`・`/shares` は `404` で、広告もしない。規則は federation.md §10 に書く。

§1 の一覧は「id の形」として残り、そこに配信の約束はない。後からルートを足しても id は変わらない。

## 経緯

ActivityPub の `id` は連合先のデータベースに保存され、識別子として使われ続ける。形式を変えると別アカウント・別投稿として扱われるので、M1 で Actor を公開する前に形式を決めた（FR-401, FR-403, FR-305, FR-006、ADR-0023）。2026-10-16 に主要実装のソースを比べたところ、どの実装も相手の URI の形式を前提にせず（Actor は WebFinger の `self` リンクと `id` で解決される）、パスの形式は相互運用にほぼ影響しなかった。ユーザー名を `id` に含める実装はどれも改名を提供できず、Mastodon も 4.5 で数値 ID の `/ap/users/{id}` を導入していた。ドメインの分離はどの実装でも「初回連合の前に固定」という制約付きだった。アクセス制御を 1 経路に集める（ADR-0017, NFR-SEC-4）こと、AP と PWA の静的配信をパスで分ける（ADR-0028, NFR-COST）ことも合わせて、今の形にした。

投稿の表示用 URL は当初 `/@{username}/{uuid}` だった。`/@user/favorite` や `/@user/media` のようなページを足すと投稿 ID とページ名が同じ階層に並び、Go と Web クライアントの両方が UUIDv7 の正規表現で区別し続けることになるので、M4 でそれらのページを足す前に、決定者の指示で `/@{username}/status/{id}` に移した（FR-106, FR-204）。`url` は表示用の値で、連合先は投稿を `id` で識別するので、AP の相互運用には影響しない。

`v0.1.0` の前の連合の監査（#2027）で、ADR とコードの食い違いが見つかり、URI と形式はタグの後に変えにくいので片方に揃えた。§13・§14 は当時のコードに揃え、§12・§15・§16 はコードを変えた。§12 の削除された Actor は当時 `404`、§14 は Cookie のセッションの閲覧者に `canView` で権限があれば `303`、§16 はどの活動も配信していなかった。§15 は `local` の投稿の IRI と作者が、作者のフォロワーのいないサーバーに届く問題（#2066）から狭めた。確認した外部の事実（2026-11-03）: Mastodon v4.7.4 の `text_formatter.rb` は本文の前に `<p class="quote-inline">RE: …</p>` を付ける。[FEP-044f](https://codeberg.org/fediverse/fep/src/branch/main/fep/044f/fep-044f.md) の `<span class="quote-inline">` は Backward compatibility の節と例にあるもので規範ではない。Mastodon v4.7.4 は削除を待つ Actor の GET に `403`、消去の終わった Actor の GET に `410` を返し、リモートの WebFinger の `410` でそのアカウントを削除する。

## 採らなかった案

- URI: ユーザー名ベース（`/users/{username}`）: 読みやすく実績が最も多いが、改名で `id` が変わり、名前の正規化も `id` の安定性に影響する。
- URI: ID ベース・プレフィックスなし（Misskey）: URL が 1 つで済むが、PWA のルーティングと同じ名前空間に入り、すべてのレスポンスが `Vary: Accept` に依存して CDN の扱いが難しくなる。
- URI: 表示用 URL を `id` に兼用: 最も単純だが、改名や表示用 URL の設計変更で連合の識別子が壊れる。
- 投稿の URL: `/@{username}/statuses/{id}`（GoToSocial）: 衝突は無くクライアント API の語と揃うが、1 件のページに複数形を使い、`/@{username}/statuses` に置くページも無い。
- 投稿の URL: `/@{username}/{id}` のまま予約語で区別（Mastodon）: 最も短く既存の URL が切れないが、ID の形式とページ名に暗黙の制約がかかり、Go と Web クライアントが正規表現を持ち続ける。
- 投稿の URL: `/status/{id}`（Misskey・Akkoma に近い）: 衝突も改名のリダイレクトも無いが、URL から作者が読めず、トップレベルの経路を消費し、`/@{username}` の下のページ構成（ADR-0028）と揃わない。
- 削除された Actor: `404` のまま: 何も変えずに済み、後からでも変えられるが、変更は小さく、受け損ねの残骸はタグの後も増え続ける。
- 引用のフォールバック: 末尾の `<span>`（FEP-044f の例の形）: FEP の例と一致するが、読み手は `quote-inline` の class を見ており、位置に意味はない。
- HTML の要求: セッションを読む: ログイン中の人が `local` の投稿の `/ap/` の URL を開けるが、ActivityPub の入口が Web のセッションに依存する。
- 引用元の公開範囲: `QuoteRequest` だけを止める: 引用はできるが、`pending` のまま終わらない引用が残る。
- 引用元の公開範囲: 制限せず自由のままにする: 中身は守られるが、IRI と作者の漏れを説明で済ませるだけになる。
- 配信する URI: すべての活動を配信: ActivityPub §3.1 の「id は取得できる」に最も忠実だが、Like や Follow の取得を使う相手は確かめられておらず、公開範囲の判定を活動ごとに増やす。

## Consequences（影響）

- 改名・表示用 URL の変更・HTML ルーティングの変更が `id` に影響しない。自サーバーの URI は種類と UUID だけで引けるので、パースと `canView` の経路が 1 本になる。
- `/ap/` プレフィックスで AP の配信と PWA の静的配信を分けられる。Mastodon 4.5 の新方式（`/ap/users/{id}`）と構造が近く、相互運用で問題になりにくい。
- `id` が長く（UUID 36 文字）、人間には読みにくい。デバッグでは表示用 URL や管理画面を併用する。
- `id` と `url` の 2 系統を持つので、ルーティングと双方向のリダイレクトを実装する必要がある。
- UUIDv7 は作成時刻を含むので、`id` から投稿・アカウントの作成時刻が分かる（Mastodon の snowflake も同じ）。
- ドメインの分離は初回起動時に固定され、後から変えられない。セットアップ手順で強く警告する必要がある。
- プロフィールの下にページを足しても投稿 ID と衝突せず、Go の外枠の振り分けが固定の語の照合で済む。ローカルの投稿の `url` を保存しないので、改名や URL の形の変更で保存済みの値を書き換える必要が無い。
- URL が `/status` の 7 文字だけ長くなる。既に配った `url` と外部に貼られたリンクが切れ、連合先の「元のページを開く」が 404 になる。
- Mastodon とは URL の形が違う。Mastodon の `ResolveURLService` は取得が 404 のときだけ URL の形から `uri` を推測するので、取得が成功する新しい URL ではこの推測に頼らない。
- 削除された Actor と投稿が、どちらも `410` で揃う。`410` を受けた実装はそのアカウントを消し、戻せないので、`deleted` より前の状態では返さない。
- `local` の投稿の存在と作者が、フォロワーのいないサーバーへ出なくなる。代わりに `local`・`private`・`direct` の投稿からリモートの投稿を引用できず、`local` の投稿を引用した公開の投稿は連合先ではただの投稿に見える。
- 活動の id の多くは取得できない（ActivityPub §3.1 の例外として扱う）。
- ユーザー名変更機能を提供するかの判断（要件への追加）が残る。

## Open Questions / 要検証

- Mastodon / Misskey / GoToSocial / Akkoma が、`preferredUsername` が変わった `Update(Person)` を受け取ったときにハンドルを更新するか（改名機能の前提）。
- 主要 CDN（Cloud CDN、CloudFront、Cloudflare）の `Vary: Accept` の扱い。デプロイテンプレートの既定値を決めるため。
- 要検証: Misskey・Akkoma・GoToSocial が先頭の `<p class="quote-inline">` をどう表示するか。ソースもライブの相互運用も確かめていない（`federation-live` で確かめる）。
- 要検証: Mastodon 以外が、Actor の `410` をどう扱うか。
