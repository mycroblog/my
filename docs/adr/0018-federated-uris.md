# ADR-0018: 連合 URI は ID ベースで `/ap/` 名前空間に置き、表示用 URL と分離する

- Status: Accepted（投稿の `url` とその振り分けの規則を ADR-0053（投稿の表示用 URL） が置き換える。URI 一覧に ADR-0051（カスタム絵文字）の `/ap/emojis/{uuid}` が加わる。削除された Actor の応答、`/ap/*` への HTML の要求、配信する URI を ADR-0144 が定める）
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #36

## Context（背景・課題）

ActivityPub の `id`（Actor・投稿・アクティビティ・コレクションの URI）は連合先のデータベースに保存され、識別子として使われ続ける。
形式を変えると、連合先では別アカウント・別投稿として扱われる。そのため [ADR-0012](0012-v0-breaking-change-policy.md) は、
v0 でも連合 URI だけは安易に変えないとしている。M1 で Actor を公開する前に形式を決めておく必要がある。

関連する要件と決定:

- FR-401（WebFinger / host-meta）、FR-403（Actor と各コレクション）、FR-305（`@user@host` / URL 指定での解決）、FR-006（プロフィール）
- [ADR-0006](0006-federation-protocol-scope.md): Mastodon / Misskey / GoToSocial / Akkoma との相互運用
- [ADR-0017](0017-visibility-local.md): 公開範囲 `local` を追加。オブジェクト URI への GET は署名者のアクセス権を確かめ、権限が無ければ 404 を返す
- [ADR-0007](0007-client-api.md): Mastodon Client API とは互換させない。このため、HTML 側の URL を Mastodon に合わせる必要はない
- [ADR-0003](0003-database-abstraction.md) / overview: ID は UUIDv7（36 文字の文字列）
- NFR-SEC-4（公開範囲の判定はドメイン層の単一の関数で行う）、NFR-COST（CDN で配信コストを抑える）

### 主要実装の方式比較

2026-10-16 に各実装のソース・公式ドキュメントで確認した（出典は末尾）。

| 観点 | Mastodon | GoToSocial | Misskey | Pleroma / Akkoma |
| --- | --- | --- | --- | --- |
| Actor `id` | 従来: `/users/{username}`。4.5.0 以降の新規ローカルアカウントは `/ap/users/{数値ID}`（`id_scheme`、既定は numeric） | `/users/{username}` | `/users/{id}`（aid 系 ID） | `/users/{nickname}` |
| プロフィール URL（`url`） | `/@{username}` | `/@{username}` | `/@{username}` | `/users/{nickname}`（フロントエンドは `/@{nickname}` も受ける） |
| 投稿 `id` | `/users/{username}/statuses/{snowflake}`（新方式は `/ap/users/{id}/statuses/{id}`） | `/users/{username}/statuses/{ULID}` | `/notes/{id}` | `/objects/{UUID}` |
| 投稿 URL（`url`） | `/@{username}/{id}` | `/@{username}/statuses/{ULID}` | `/notes/{id}`（AP と同じ URL を Accept で振り分け） | `/notice/{flake id}` |
| `Create` の `id` | `{投稿 id}/activity` | `{投稿 id}/activity` 相当（要検証） | `/notes/{id}/activity` | `/activities/{UUID}` |
| その他のアクティビティ | 投稿配下・`#likes/…` などフラグメント | `/users/{username}/follow/{ULID}` `/liked/{ULID}` など Actor 配下 | `/likes/{id}` `/follows/{follower}/{followee}` | `/activities/{UUID}` |
| コレクション | `/users/{u}/{inbox,outbox,followers,following,collections/featured}`、shared inbox `/inbox` | `/users/{u}/{inbox,outbox,followers,following,collections/featured}` | `/users/{id}/{inbox,outbox,followers,following,collections/featured}`、`/inbox` | `/users/{n}/{inbox,outbox,followers,following}`、`/inbox` |
| 公開鍵 `id` | `{actor}#main-key` | `{actor}/main-key` | `{actor}#main-key`（`/users/{id}/publickey` も提供） | `{actor}#main-key` |
| ユーザー名の変更 | 不可（`Move` による引っ越しのみ）。数値 ID 方式は将来の改名を可能にするための変更 | 不可 | 不可（ID ベースなので AP 上の制約ではない） | 不可 |
| HTML と AP JSON | 別 URL。数値 ID の AP URL に HTML で来たら `/@username` へリダイレクト（4.5.x） | 別 URL。`/users/{u}` に HTML で来たら `/@{u}` へリダイレクト | 同一 URL を `Accept` で振り分け、`Vary: Accept` を付ける | 同一 URL を `Accept` で振り分け |
| ドメイン分離（ハンドルと配信） | `LOCAL_DOMAIN`（ハンドル）と `WEB_DOMAIN`（配信）。後から変更不可 | `account-domain` と `host`。初回起動前に決め、連合後は変更不可 | 非対応（要検証） | `Pleroma.Web.WebFinger` の `domain` と Endpoint の `host`。後から変更不可 |

分かったこと:

- 主要実装はどれも、相手の URI の形式を前提にしない。Actor は WebFinger の `self` リンクと `id` で解決される。
  このため、パスの形式は相互運用にほぼ影響しない（Mastodon 自身も 4.5 で形式を変えた）。
- ユーザー名を `id` に含める方式（Mastodon の従来方式・GoToSocial・Pleroma）では、改名すると `id` が変わるため、どの実装も改名を提供していない。
- ドメインを分離する方式はどの実装でも「初回連合の前に固定し、後から変えない」という制約付き。
  ハンドル側ドメインの `/.well-known/webfinger`（と host-meta / nodeinfo）を配信ドメインへリダイレクトする。

## Decision Drivers（判断基準）

1. **`id` の不変性**: 一度公開した `id` は、改名・設定変更・実装変更があっても変わらない（ADR-0012）
2. **相互運用性**: Mastodon / Misskey / GoToSocial / Akkoma と問題なく連合できる（ADR-0006）
3. **アクセス制御を 1 か所に集める**: どの URI も「パース → 実体の取得 → `canView`」の 1 経路で処理し、権限が無いときと存在しないときで応答（ステータス・ヘッダー）を同じにする（ADR-0017, NFR-SEC-4）
4. **配信経路と CDN の扱いやすさ**: AP 向けのリクエストを、PWA の静的配信（ADR-0008）とパスのプレフィックスで分けられる（NFR-COST）
5. **ユーザー名を変更できる余地**: v0 で機能として提供するかは別として、URI の形式が改名を妨げない
6. **人間にとっての読みやすさ・デバッグのしやすさ**

## Considered Options（選択肢）

1. **ユーザー名ベース（Mastodon 従来方式 / GoToSocial）**: `/users/{username}`、`/users/{username}/statuses/{id}`
2. **ID ベース + `/ap/` 名前空間（Mastodon 4.5 の新方式に近い）**: `/ap/actors/{uuid}`、`/ap/objects/{uuid}`、`/ap/activities/{uuid}`
3. **ID ベース・プレフィックスなし（Misskey 方式）**: `/users/{uuid}`、`/notes/{uuid}`。HTML と AP を同じ URL で振り分ける
4. **表示用 URL を `id` に兼用**: `/@{username}`、`/@{username}/{id}` を `Accept` で HTML / AP に振り分ける

## Decision（決定）

**選択肢 2 を採用する。** AP の `id` には UUIDv7 だけを使い、`/ap/` 名前空間に置く。人間向けの表示用 URL（`url`）は別に持ち、こちらは変えてよいものとして扱う。

### URI 一覧（`{host}` = 配信ドメイン、`{uuid}` = UUIDv7 の 36 文字小文字表記）

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
| プロフィール（HTML、`url`） | `https://{host}/@{username}` | `https://social.example.org/@alice` |
| 投稿（HTML、`url`） | `https://{host}/@{username}/{uuid}` | `https://social.example.org/@alice/0192f3b0-2c3d-7e4f-8a00-112233445566` |
| ハッシュタグ（HTML、`Hashtag.href`） | `https://{host}/tags/{name}` | `https://social.example.org/tags/fediverse` |
| WebFinger の `subject` | `acct:{username}@{account_domain}` | `acct:alice@example.org` |

規則:

- `id` は https・小文字・末尾スラッシュなし・クエリなし（コレクションのページ `?page=…` を除く）。パスにユーザー名を含めない。
- 受信したアクティビティ中の自サーバーの URI は、正規表現 `^/ap/(actors|objects|activities)/{uuid}` で厳密にパースし、HTTP 取得せずに DB を引く。
- `Create` の `id` は投稿から決まる値（`{object}/activity`）にして保存しない。`Update` は編集ごとに別のアクティビティなので `/ap/activities/{uuid}` を使う。
- メディアの URL は `id` ではない（Blob / CDN のドメインに置いてよい。ADR-0005）。

### ドメイン

- 設定は `host`（配信ドメイン。`id` はすべてこのドメイン）と `account_domain`（ハンドルのドメイン。省略時は `host`）の 2 つ。
- 2 つの値は初回起動時に DB に記録する。設定ファイルの値と食い違ったら起動を拒否する（連合した後の変更事故を防ぐ）。
- 分離する場合、`account_domain` の `/.well-known/webfinger`、`/.well-known/host-meta`、`/.well-known/nodeinfo` を `host` へリダイレクトする（CORS ヘッダー付き）。
  `host` 側の WebFinger は `acct:alice@example.org` と `acct:alice@social.example.org` の両方に答え、`subject` は常に `account_domain` 側のハンドルで返す。

### HTML と AP JSON の振り分け

- `/ap/*` は AP 用。`Accept` が `application/activity+json` または `application/ld+json; profile="https://www.w3.org/ns/activitystreams"` なら JSON（`Content-Type: application/activity+json`）を返す。
  HTML を求めるリクエストには、閲覧者（Cookie のセッション）に `canView` で権限があるときだけ表示用 URL へ `303` でリダイレクトし、権限が無ければ 404 を返す。
- 表示用 URL（`/@alice`、`/@alice/{uuid}`）に AP の `Accept` で来たら、対応する `id` へ `302` でリダイレクトする（URL を貼ってのリモート検索、FR-305 用）。
- 振り分けを行うレスポンスにはすべて `Vary: Accept` を付ける。
- `/@{任意の名前}/{uuid}` は投稿 ID だけで引く。名前が現在のユーザー名と違えば正しい URL へリダイレクトする（改名後も古いリンクが切れない）。

### アクセス制御（ADR-0017）

- `/ap/objects/{uuid}`（`/activity` と各サブコレクションも含む）は、リクエストの HTTP 署名から閲覧者を決め、`canView` を通す。
  `public` / `unlisted` は署名なしでも返す（Authorized Fetch が無効のとき）。`local` / `private` / `direct` は署名者に権限があるときだけ返す。
- **権限が無いときと存在しないときは、同じ 404、同じヘッダーで返す**（`Cache-Control: no-store` を含む）。応答の違いから存在が漏れないようにする。
- outbox が列挙するのは `public` / `unlisted` の投稿だけ。`followers` / `following` は、利用者の設定に応じて件数だけを返すこともできる。
- UUIDv7 は推測されにくいが、時刻を含む。推測されにくいことをアクセス制御の根拠にはしない。

### CDN キャッシュ

- `/ap/*` と PWA の静的ファイルはパスのプレフィックスで分け、CDN・リバースプロキシのルールを単純にする。
- 匿名で閲覧できる Actor と `public` / `unlisted` の投稿だけ、`Cache-Control: public, max-age=60`（値は要調整）を付けてよい。
  それ以外（Authorized Fetch が有効なとき、`local` 以下の公開範囲、404）は `private, no-store`。
- `Vary: Accept` を正しく扱わない CDN があり得る（要検証）。その場合は CDN 側で `/ap/*` をキャッシュ対象外にする設定をデプロイテンプレートの既定にする。

### ユーザー名の変更

- `id` にユーザー名を含めないので、改名しても `id` は変わらない。改名時は `Update(Person)` で新しい `preferredUsername` と `url` を配送する。
- 古いユーザー名は、一定期間（既定 30 日、要調整）他人が取得できないよう予約する。その間、`/@old` は `/@new` へリダイレクトする。古いハンドルでの WebFinger には答えない（なりすましの混乱を避ける）。
- 改名を v0 の機能として提供するかは別に決める（この ADR は改名を妨げない形式を決めるだけ）。

### この決定を見直す条件

- 主要実装のいずれかが `/ap/` 配下の ID ベース URI と相互運用できない（例: パスの形式を前提にした処理がある）ことが分かった場合
- 運用者がユーザー名入りの URL を強く求め、改名を恒久的に提供しないと決めた場合（選択肢 1 が有力になる）

## Consequences（影響）

- 良い影響
  - 改名・表示用 URL の変更・HTML ルーティングの変更が `id` に影響しない。ADR-0012 の「連合の識別子は変えない」を守りやすい。
  - 自サーバーの URI は種類と UUID だけで引けるので、パースと `canView` の経路が 1 本になる。
  - `/ap/` プレフィックスで AP の配信と PWA の静的配信を分けられる。CDN・リバースプロキシの設定が単純になる。
  - Mastodon 4.5 の新方式（`/ap/users/{id}`）と構造が近く、相互運用で問題になりにくい。
- 悪い影響・トレードオフ
  - `id` が長く（UUID 36 文字）、人間には読みにくい。デバッグでは表示用 URL や管理画面を併用する。
  - `id` と `url` の 2 系統を持つので、ルーティングと双方向のリダイレクトを実装する必要がある。
  - UUIDv7 は作成時刻を含むので、`id` から投稿・アカウントの作成時刻が分かる（Mastodon の snowflake も同じ）。
  - ドメインの分離は初回起動時に固定され、後から変えられない。セットアップ手順で強く警告する必要がある。
- 後続の作業（Issue 化する）
  - URI 生成・パースを 1 つのパッケージにまとめ、ラウンドトリップのテストを書く
  - WebFinger / host-meta / nodeinfo と、ドメイン分離時のリダイレクト設定例（nginx / Caddy / Cloud CDN）
  - `canView` と統合した AP オブジェクト取得のテーブル駆動テスト（404 の応答が同じであることも検証する）
  - 主要 4 実装との相互運用テストに、`/ap/` 形式の Actor の解決・フォロー・投稿の取得を加える（ADR-0006）
  - ユーザー名変更機能を提供するかの判断（要件への追加）

## Pros and Cons of the Options（詳細比較）

### 1. ユーザー名ベース

- 良い: 読みやすく、URI を推測できる。Mastodon の従来方式・GoToSocial と同じで、実績が最も多い。
- 悪い: 改名すると `id` が変わるので、改名機能を実質的に提供できない。Mastodon もこれを理由に 4.5 で数値 ID 方式を導入した。
  パスがユーザー名から決まるので、名前の正規化（大文字小文字・Unicode）の扱いも `id` の安定性に影響する。

### 2. ID ベース + `/ap/` 名前空間（採用）

- 良い: 判断基準 1・3・4・5 をすべて満たす。Mastodon の新方式と同じ考え方。
- 悪い: 読みにくい。表示用 URL との 2 系統を持つ実装コストがかかる。

### 3. ID ベース・プレフィックスなし（Misskey 方式）

- 良い: 改名に強い。URL が 1 つで済み、`Accept` で振り分けるだけでよい。Misskey で実績がある。
- 悪い: `/users/…` や `/notes/…` が PWA のルーティングと同じ名前空間に入り、静的配信と AP の振り分けをパス単位でできない。
  同じ URL で HTML と JSON を返すため、すべてのレスポンスが `Vary: Accept` に依存し、CDN の扱いが難しくなる。

### 4. 表示用 URL を `id` に兼用

- 良い: URL が 1 種類で最も単純。ブラウザで `id` を開くと、そのまま人間向けのページになる。
- 悪い: ユーザー名を含むので、改名で `id` が変わる（選択肢 1 と同じ問題）。表示用 URL の設計を変えるだけでも連合の識別子が壊れる。

## Open Questions / 要検証

- [ ] Mastodon / Misskey / GoToSocial / Akkoma が、`preferredUsername` が変わった `Update(Person)` を受け取ったときにハンドルを更新するか（改名機能の前提）
- [ ] 主要 CDN（Cloud CDN、CloudFront、Cloudflare）の `Vary: Accept` の扱い。デプロイテンプレートの既定値を決めるため
- [ ] Misskey にハンドルと配信のドメイン分離の設定があるか（比較表の確認）
- [ ] GoToSocial の `Create` アクティビティの `id` の形式（比較表の確認）
- [ ] インスタンス Actor の WebFinger 上のハンドル（Mastodon の `acct:{host}@{host}` に合わせるか）
- [ ] UUID をそのまま使うか、短い表記（Crockford Base32 の 26 文字など）にするか。推奨案は DB と同じ 36 文字表記で変換を持たない

### 出典（2026-10-16 確認）

- Mastodon: [config/routes.rb](https://github.com/mastodon/mastodon/blob/main/config/routes.rb)、[CHANGELOG 4.5.0「Add support for numeric-based URIs for new local accounts」](https://github.com/mastodon/mastodon/blob/main/CHANGELOG.md)、[LOCAL_DOMAIN / WEB_DOMAIN](https://docs.joinmastodon.org/admin/config/)
- GoToSocial: [internal/uris](https://codeberg.org/superseriousbusiness/gotosocial/src/branch/main/internal/uris/uri.go)、[Host and account domain](https://docs.gotosocial.org/en/latest/advanced/host-account-domain/)
- Misskey: [ActivityPubServerService.ts](https://github.com/misskey-dev/misskey/blob/develop/packages/backend/src/server/ActivityPubServerService.ts)
- Akkoma / Pleroma: [Akkoma router.ex](https://akkoma.dev/AkkomaGang/akkoma/src/branch/develop/lib/pleroma/web/router.ex)、[How to serve another domain for WebFinger](https://docs.pleroma.social/backend/configuration/how_to_serve_another_domain_for_webfinger)
