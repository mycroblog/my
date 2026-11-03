# ADR-0090: OAuth をサードパーティのアプリに開く

- Status: Accepted（§4 の一次のクライアントだけの操作に ADR-0143 が加える）
- Date: 2026-10-23
- Deciders: @rosylilly
- Issue: #1038

## Context（背景・課題）

[FR-003](../requirements/01-functional.md) と [ADR-0007](0007-client-api.md) は、認可に OAuth 2.0（Authorization Code + PKCE）を使い、
サードパーティのクライアントを受け入れられる設計にすると決めている。認可サーバーの中心はすでにある（`internal/app/identity/oauth.go`）:
PKCE S256 だけの Authorization Code、redirect URI の完全一致、1 回限りの認可コード、refresh token のローテーションと再利用時の系列ごとの失効
（[RFC 9700](https://www.rfc-editor.org/rfc/rfc9700) §4.14.2）、失効（RFC 7009）、細かいスコープ（`api/spec/openapi.yaml`）、同意の画面
（`internal/interface/authpages/authorize.go`）。

ただし、クライアントは起動時に登録される一次の Web クライアント（`my-web`、`oauth_clients.is_first_party`）だけで、
**サードパーティのアプリが `client_id` を得る手段がない**。次のものが同じ仕組みを必要とする:

- Tauri 2 のモバイルアプリ（FR-603、#31）。1 つのアプリが任意のインスタンスにつなぐので、各インスタンスにとっては事前に登録されていないクライアントになる。
- CLI・ボット（[ADR-0093](0093-api-client-library-mykit.md) の `pkg/mykit`、[ADR-0094](0094-bot-library-mybot.md) の `pkg/mybot`）と、利用者が作る連携サービス。

また v0 の破壊的変更の方針（[ADR-0087](0087-schema-changes-after-launch.md)）はクライアント API の安定性を決めず、この ADR に委ねている。サードパーティに開くなら、API の安定性の約束もあわせて決める必要がある。
M5（v0 リリース）で実運用を始める前に、登録の方法・ネイティブのリダイレクト・利用者による管理・濫用への対策を決める。
関連: FR-003、FR-006（bot フラグ）、FR-603、NFR-SEC-5（レート制限）、[ADR-0009](0009-internationalization.md)、[ADR-0016](0016-client-api-style.md)、
[ADR-0019](0019-security-baseline.md)、[ADR-0020](0020-data-modeling-principles.md)、[ADR-0030](0030-web-client-token-handling.md)、[ADR-0037](0037-audit-log.md)。

## Decision Drivers（判断基準）

1. **安全性**: 利用者のアカウントを奪う・なりすます・資源を食いつぶすアプリを作りにくいこと。OAuth の BCP（RFC 9700）と RFC 8252 に従うこと。
2. **公式のモバイルアプリと CLI が、インスタンスごとの手作業なしで使えること**（任意のインスタンスへの接続）。
3. **既存の OAuth のクライアント・ツールとの相互運用**: RFC 7591 や CIMD を前提にする汎用のクライアント（MCP のクライアントなど）が、そのまま登録できること。
4. **ほぼゼロコスト・小規模運用**（NFR-COST）: 未認証で増える行は上限と自動の削除で抑え、管理者の手作業を増やさない。
5. **利用者の制御**: 認可したアプリを見て取り消せること。管理者がアプリを止められること。
6. **標準への準拠と実装の小ささ**: 既存の認可サーバーを延長で済ませ、独自の仕組みを増やさない。
7. **サードパーティの開発者にとっての予測可能性**: API の変更が告知され、仕様が公開されていること。

## Considered Options（選択肢）

クライアントの登録の方法を軸に比べる（クライアントの種類・ネイティブ・スコープ・管理・API の安定性は、どれを選んでも Decision の内容をほぼそのまま使う）。

1. **利用者の設定の画面での登録 + Client ID Metadata Document（CIMD）+ 動的クライアント登録（[RFC 7591](https://www.rfc-editor.org/rfc/rfc7591)、未認証・上限つき）+ 管理者による無効化**
2. **利用者の設定の画面での登録 + CIMD + 管理者による無効化**（RFC 7591 なし）
3. **RFC 7591 を未認証で開くだけ + 管理者による無効化**（Mastodon の `POST /api/v1/apps` に相当）
4. **利用者の設定の画面での登録だけ**
5. **管理者による登録だけ**（利用者・アプリの開発者が管理者に依頼する）

## Decision（決定）

**1 を採用する。** 登録の経路を 3 つ持ち、どれも同じ認可サーバー（PKCE・ローテーション・失効）に載せる。
クライアント API（`/api/v1`）の安定性の約束はこの ADR の §8 が持つ（[ADR-0087](0087-schema-changes-after-launch.md) は DB スキーマ・設定・内部インターフェイスを扱い、クライアント API をこの ADR に委ねている）。

### 1. クライアントの登録

- **利用者による登録（設定の画面と client API）**: ログインしている利用者が、名前・サイトの URL・redirect URI・要求できるスコープの上限・種類（public / confidential）を入れてアプリを登録する。
  `oauth_clients.owner_user_id` に登録者を持つ。登録者は編集・削除・シークレットの再発行ができる。サーバーの連携・ボット・自作のクライアント向け。
- **Client ID Metadata Document**（[draft-ietf-oauth-client-id-metadata-document-02](https://datatracker.ietf.org/doc/draft-ietf-oauth-client-id-metadata-document/)、
  2026-07-06 更新の WG 草案で RFC ではない、2026-10-23 確認）: `client_id` が `https` の URL なら、事前登録なしに、その URL の JSON（`client_name`・`client_uri`・`logo_uri`・`redirect_uris` など）を取得してクライアントとして扱う。
  公式のモバイルアプリ・配布される CLI・Web のサードパーティのクライアント向け。
  - 取得は `kernel.HTTPClient`（ADR-0019 §1）を通す。草案に従い、リダイレクトを追わない、`client_id` の完全一致、応答は小さい上限（草案は 5 KB 程度を推奨）、エラーはキャッシュしない、HTTP のキャッシュ指示を尊重する。
  - CIMD のクライアントは public client だけにする（草案は共有シークレットを禁じる。`private_key_jwt` は受けない。§2）。
  - 取得した内容はキャッシュとして持ち（ADR-0020 の投影に近い扱い。再取得で作り直せる）、管理者の無効化はキャッシュではなく `client_id` の URL（とそのホスト）に対して記録する。
- **動的クライアント登録**（[RFC 7591](https://www.rfc-editor.org/rfc/rfc7591)）: `POST /oauth/register` を **認証なし** で受ける（RFC 7591 §3 は初期アクセストークンなしの登録を SHOULD としている、2026-10-23 に確認）。
  CIMD を扱えない汎用のクライアント・ツール向け。
  - 受けるのは public client（`token_endpoint_auth_method: "none"`）だけ。RFC 7591 は省略時の既定を `client_secret_basic` としているが、サーバーは要求されたメタデータを拒否・置き換えてよい（§2・§3.2.1、2026-10-23 に確認）ので、
    省略時は `none` に置き換えて応答で返し、`client_secret_basic` などを明示した要求は `invalid_client_metadata` で拒否する。シークレットを持つクライアントは利用者による登録で作る。
  - 受けるメタデータは `client_name`・`client_uri`・`logo_uri`・`redirect_uris`・`scope`・`grant_types`・`response_types`。`scope` は一次専用のもの（§4）を除いた範囲に置き換える。
    software statement は受けない。登録したクライアントの読み取り・更新・削除（RFC 7592）は提供しない。変えたいアプリは登録しなおす。
  - **使われないクライアントの自動の削除**: 動的登録のクライアントのうち、登録から 24 時間以内に認可コードが 1 度も交換されなかったもの、または有効なトークン系列がなく最後に使われてから 90 日たったものを、定期のジョブで削除する（既定。設定で変えられる）。
    削除はクライアントのイベントとして記録し、ジョブは冪等にする（ADR-0004）。
  - Mastodon の `POST /api/v1/apps` とは別物で、互換はしない（ADR-0007）。Mastodon 向けのクライアントはどのみち使えない。
- **管理者**: 新しい権限 `manage_oauth_apps` で、全クライアントの一覧（登録の経路で絞り込める）・無効化・無効化の取り消し、CIMD のホスト単位の拒否をする。無効化は監査ログ（ADR-0037）に残し、そのクライアントの全トークンを失効させる。
- **インスタンス設定**: 「利用者によるアプリの登録」「CIMD のクライアント」「動的クライアント登録」をそれぞれ有効・無効にできる。既定はすべて有効。すべて無効なら一次のクライアントだけが使える。

### 2. クライアントの種類とグラント

- **public client**（SPA・ネイティブ・CLI）: シークレットなし、PKCE S256 必須（現行どおり）。
- **confidential client**（サーバーで動く連携・ボット）: 利用者による登録のときだけ選べる。`client_secret` は登録・再発行のとき 1 度だけ表示し、SHA-256 のハッシュで保存する（ADR-0019 §4）。
  認証は `client_secret_basic` と `client_secret_post`。再発行は古いシークレットを即時に無効にする（猶予期間は持たない）。confidential client でも PKCE は必須にする（RFC 9700 §2.1.1）。
  CIMD と動的登録のクライアントは public client だけなので、confidential client は利用者による登録のものに限られる。
- **グラントは `authorization_code` と `refresh_token` だけ。** `client_credentials`（利用者のいないトークン）・`password`・implicit・デバイス認可（RFC 8628）は受けない。
  API の操作はすべて利用者として行う設計で、ボットもボット用のアカウントの利用者として認可する。
- **登録者自身へのトークン発行**: 利用者による登録のアプリは、設定の画面から登録者自身のアカウントへのトークンを、redirect なしに発行できる（ボット・スクリプト向け。Mastodon の「開発」の画面と同じ使い方）。
  発行されるのは通常のアクセストークンとリフレッシュトークンの組（ADR-0019 §4 の有効期限のまま。長命のトークンは発行しない）で、同意の画面の代わりに設定の画面でスコープを選ぶ。
  ボットは、ローテーションされたリフレッシュトークンを保存しながら更新する（[ADR-0094](0094-bot-library-mybot.md) の `Store`）。

### 3. ネイティブのアプリ（[RFC 8252](https://www.rfc-editor.org/rfc/rfc8252)）

- 許す redirect URI（いずれも利用者・動的登録で登録したもの、CIMD に記載したものと完全一致）:
  - **claimed `https`**（Universal Links / App Links）: 推奨。公式のモバイルアプリはプロジェクトのドメインの URL を使う。OS がドメインとアプリの対応を検証するので、ほかのアプリが横取りできない。
  - **private-use URI scheme**: 逆ドメイン表記（例: `com.example.app:/oauth/callback`、RFC 8252 §7.1）だけを許す。`http`・`https`・`javascript` など既知の危険なスキームは拒否する。
    スキームは OS が検証しないので、どのアプリも同じスキームを名乗れる。
  - **ループバック**: `http://127.0.0.1/...` と `http://[::1]/...`。照合のときポートは無視する（RFC 8252 §7.3）。`localhost` は許さない（§8.3）。
- `urn:ietf:wg:oauth:2.0:oob` は受けない。
- **private-use URI scheme とループバックは、CIMD ではなりすましを防げない**（動的登録では、どのアプリも任意の名前を名乗れる）。正規のアプリの `client_id` の URL を名乗り、同じスキームやループバックのポートで待つ別のアプリが、正規のアプリの名前の同意の画面からコードを受け取れる
  （[MCP の認可の仕様 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization) の Localhost Redirect URI Risks、2026-10-23 に確認）。
  そのため、CIMD・動的登録のクライアントの redirect URI が claimed `https` でないときは、同意の画面に redirect URI のホスト（またはスキーム）と「名乗っているアプリと別のアプリかもしれない」という注意を出す（§4）。
  CIMD の草案は、redirect URI を `client_id` と同じオリジンなどに制限してよいとしている（[draft -02 §8.1](https://www.ietf.org/archive/id/draft-ietf-oauth-client-id-metadata-document-02.html)、2026-10-23 に確認）が、ネイティブのアプリとループバックを使えなくなるので制限はしない。
- **公式のモバイルアプリ（#31）**: CIMD のクライアントとして、プロジェクトのサイトに置いた `client_id` の URL を使う。インスタンスにとっては一次のクライアントではないので、同意の画面を必ず出す（信頼済みとして同意を省く設定は持たない）。
  redirect URI には claimed `https` を使う。Tauri 2 の deep link プラグインは iOS の Universal Links・Android の App Links とカスタムスキームを受けられ、
  モバイルではリンクを設定に静的に書く必要がある（[Tauri の文書](https://v2.tauri.app/plugin/deep-linking/)、2026-10-23 に確認）。プロジェクトのドメインに `apple-app-site-association` と `assetlinks.json` を置く。
  OAuth のコールバックを実機で受けられるかは #31 の spike で確かめる。
- **CLI**（`pkg/mykit` を使うもの）: 配布されるものは CIMD + ループバック。個人のスクリプトは利用者による登録 + 登録者自身へのトークン発行。

### 4. スコープと同意の画面

- 現行のスコープの粒度をサードパーティにも使う。ただし次を **一次のクライアントだけのもの** にし、サードパーティには発行しない:
  - `admin` / `admin:read` / `admin:write`（管理の操作は一次の Web クライアントからだけ行う）
  - `write:supporters`（支払いを伴う）
  - アカウントの保安の操作（パスワード・二要素認証・メールアドレス・アカウントの削除・Move（[ADR-0089](0089-account-migration-move.md)）・認可したアプリの管理）は、スコープにかかわらず一次のクライアントだけにする。
- サードパーティが要求した一次専用のスコープは、黙って落とさず `invalid_scope` にする。
- ダイレクトメッセージ（公開範囲 `direct`）の読み取りは `read:statuses` から分けて `read:direct` にする。`read:statuses` では `direct` の投稿は読めない。広いスコープ `read` は引き続き `read:direct` を含む。
  `direct` の投稿の送信は `write:statuses` のまま分けない。
- **同意の画面**（ADR-0030 の Go のフォーム、文言は `server` 名前空間、ADR-0009）に出すもの: アプリの名前、出どころ（CIMD なら `client_id` のホスト名、利用者の登録なら「@登録者 が登録したアプリ」、動的登録なら「自己申告で登録されたアプリ」）、サイトの URL、
  要求するスコープの説明（現行の `scopeDescriptions`）、redirect URI のホスト。DM・投稿の書き込み・プロフィールの変更は注意の表示を付ける。
  redirect URI が claimed `https` でない CIMD・動的登録のクライアントには、§3 のなりすましの注意を付ける。
  **サードパーティのアプリはすべて「このサーバーが確認していないアプリ」と表示する**（確認の制度は持たない）。名前・ロゴはアプリの自己申告で、ホスト名・登録者だけが検証済みの情報だと分かる表示にする。
- 同意は毎回求め、記憶しない（同意の記録のテーブルを持たない）。

### 5. 利用者による管理

- 設定の画面と client API に「認可したアプリ」の一覧を置く: アプリ・出どころ・付与したスコープ・最初に認可した日時・最後に使った日時。取り消すとその利用者とクライアントの組の全トークン系列を失効させる（`oauth_token_revocations` の `user`）。
- データモデル（ADR-0020）:
  - 一覧は有効なトークン系列（`oauth_access_tokens` の `family_id`、失効のイベントがないもの）を利用者とクライアントで束ねて導く。一覧のためのテーブルは持たない。
  - **最後に使った日時** は現在値（規約 5）のテーブル `oauth_token_family_last_used`（系列ごと）に持ち、認証のたびではなく **1 時間に 1 回まで** 更新する（`KeyValueStore` で間引く）。毎リクエストの書き込みを避けるため精度を落とす。
  - 登録の経路（一次・利用者・CIMD・動的）は、クライアントの不変の属性として `oauth_clients` に持つ（`is_first_party` を置き換える）。
  - 管理者の無効化・登録者の削除・使われない動的登録のクライアントの自動の削除はクライアントのイベント（無効化・無効化の取り消し・削除と理由）として記録し、`oauth_clients` に状態のカラムを足さない。
    トークンの失効の理由に `client_disabled` と `client_deleted` を足す。
  - 動的登録の自動の削除の判定は、認可コードの交換（`oauth_authorization_code_redemptions`）・トークン系列・最後に使った日時から導き、判定のためのカラムを足さない。
  - CIMD の取得結果はキャッシュのテーブルに持つ。`oauth_clients` に CIMD の部分型を足すか別テーブルにするかは実装の issue で data-model 文書とあわせて決める。

### 6. メタデータと発見

- **Authorization Server Metadata**（[RFC 8414](https://www.rfc-editor.org/rfc/rfc8414)）を `/.well-known/oauth-authorization-server` で出す
  （`issuer`・各エンドポイント（`registration_endpoint` を含む）・`scopes_supported`（一次専用は除く）・`response_types_supported: ["code"]`・`grant_types_supported`・`code_challenge_methods_supported: ["S256"]`・
  `token_endpoint_auth_methods_supported`、CIMD を示す `client_id_metadata_document_supported`（[draft -02 §6](https://www.ietf.org/archive/id/draft-ietf-oauth-client-id-metadata-document-02.html)、2026-10-23 に確認））。
- **Protected Resource Metadata**（[RFC 9728](https://www.rfc-editor.org/rfc/rfc9728)）を `/.well-known/oauth-protected-resource` で出し、`401` の `WWW-Authenticate` に `resource_metadata` を付ける。どちらも静的な JSON で費用はほぼない。
- OpenID Connect（ID トークン・`/.well-known/openid-configuration`）は提供しない。

### 7. 濫用への対策（NFR-SEC-5、ADR-0019 §5 に足す）

| 対象 | キー | 既定（要検証: 運用で調整） |
| --- | --- | --- |
| API 全体（認証済み） | アカウント（現行。アプリをまたいで合算） | 1,500 / 5 分 |
| API 全体（クライアントごと） | クライアント（全利用者の合算） | 設定で指定。既定は無制限で、管理者が問題のクライアントに上限を付ける |
| アプリの登録（利用者） | 利用者 | 5 / 時、1 利用者あたり 25 件まで |
| 動的クライアント登録 | IP（IPv6 は `/64` に集約） | 5 / 時 |
| 動的クライアント登録（全体） | インスタンス | 100 / 時。超えたら `429` |
| CIMD の取得 | `client_id` の URL / ホスト | URL ごとに 1 / 5 分（キャッシュがあればそれを使う）、ホストごとに 30 / 時 |
| `/oauth/token` | クライアント + IP | 60 / 分 |

- スパムのアプリは、登録の上限・使われない動的登録のクライアントの自動の削除・管理者の無効化（ホスト単位を含む）・「確認していないアプリ」の表示で抑える。
  利用者による登録のアプリは、登録者が凍結・削除されたら無効にする。
- **bot フラグ（FR-006）とは独立**: どのアプリから投稿してもアカウントの bot フラグは変わらない。ボットを動かす利用者が自分で bot フラグを付ける（`pkg/mybot` の文書で案内する）。投稿にアプリの名前は出さない（連合にも出さない）。

### 8. クライアント API の安定性

v0 リリース（M5）以降、`/api/v1` に次を約束する。この節がクライアント API の安定性の方針で、DB スキーマ・設定・内部インターフェイスは [ADR-0087](0087-schema-changes-after-launch.md) に従う。

- 変更は追加を既定とする（フィールド・操作・スコープの追加、列挙値の追加は破壊的変更と見なさない。クライアントは未知の値を無視する）。
- 破壊的変更（削除・名前や型の変更・必須の追加・スコープの意味の変更）は v0 の間も許すが、
  OpenAPI で `deprecated: true` にし、応答に `Deprecation` ヘッダ（[RFC 9745](https://www.rfc-editor.org/rfc/rfc9745)、値は Structured Fields の Date（例: `@1688169599`）、2026-10-23 に確認）と
  `Sunset` ヘッダ（RFC 8594。`Deprecation` の日時より前にしない）を付け、**少なくとも 1 つの v0.x のマイナーリリースの間** は残してから削除する。
  リリースノートに「API の変更」の節を設ける。セキュリティの修正は猶予なしに変えてよい。
- バンドルした OpenAPI をサーバーが `/api/v1/openapi.yaml`（仮）で配り、文書のサイトにも載せる。
- v1.0 では `/api/v1` の破壊的変更をしない（必要なら `/api/v2`）。

### 9. 開発者向けの文書

英語（#1031 の方針）で、登録の 3 つの方法（利用者による登録・CIMD・動的登録。CIMD を優先して案内する）・認可の流れ（PKCE・ネイティブのリダイレクト・CIMD の書き方）・スコープ（一次専用のものを含む）・レート制限と `429`・API の安定性の約束を書く。
**Mastodon 向けのクライアント・ライブラリはそのままでは使えない**（`/api/v1/apps` も Mastodon の API もない。動的登録は RFC 7591 の `/oauth/register`）ことを冒頭に書く。

### FR-003 の書き換え

> FR-003 | OAuth 2.0（Authorization Code + PKCE）で一次のクライアントとサードパーティのアプリを認可できる。
> サードパーティのアプリは、利用者による登録・Client ID Metadata Document・動的クライアント登録（RFC 7591）で `client_id` を得る。利用者は認可したアプリを一覧して取り消せ、
> 管理者はアプリを無効にできる（[ADR-0090](../adr/0090-third-party-oauth-apps.md)） | M

### 見直す条件

- 未認証の動的登録がスパムに多用され、上限と自動の削除で抑えられない場合は、動的登録の既定を無効にするか、初期アクセストークン（RFC 7591 §3）を要求する形に変える。
- 主要なクライアントが CIMD に移り、動的登録が使われなくなった場合は、動的登録をやめる。
- CIMD が WG で大きく変わる・放棄される場合は、CIMD の扱いを見直す。
- 利用者による登録のアプリがスパムに多用され、上限と無効化で抑えられない場合は、管理者の承認制（選択肢 5 の要素）を設定で選べるようにする。

## Consequences（影響）

- 良い影響
  - 公式のモバイルアプリと配布される CLI が、インスタンスごとの登録なしに任意のインスタンスにつながる。
  - RFC 7591 だけに対応する汎用のクライアント・ツールも、そのまま登録できる。
  - 管理・支払い・アカウントの保安はサードパーティから触れないので、悪意のアプリの被害が投稿・閲覧の範囲に限られる。
  - 既存の認可サーバー（PKCE・ローテーション・失効）をそのまま使い、足すのは登録・照合・表示・管理の部分だけ。
- 悪い影響・トレードオフ
  - CIMD は RFC になっていない草案で、仕様の変更に追従する費用がある。クライアントの側はメタデータの JSON を置いて URL を `client_id` に使うだけなので、ライブラリの対応は要らない
    （`golang.org/x/oauth2` は PKCE の S256 を持つ。[`oauth4webapi`](https://github.com/panva/oauth4webapi) は RFC 8414 と RFC 9728 の発見を持つ。2026-10-23 に確認）。
  - private-use URI scheme とループバックの CIMD のクライアントは、なりすましを表示の注意でしか防げない（§3）。
  - 未認証の動的登録で `oauth_clients` の行が増える。上限と自動の削除で抑えるが、削除のジョブと上限の調整の運用が要る。動的登録の名前・ロゴは何の検証もない。
  - 登録の経路が 3 つあり、実装・試験・説明の量が増える。
  - CIMD の取得はサーバー側の外向きの HTTP で、SSRF の対策（ADR-0019 §1）とレート制限が要る。
  - 確認の制度がないので、名前・ロゴのなりすましは表示の工夫（ホスト名・登録者の表示）でしか防げない。
  - 最後に使った日時は 1 時間の精度しかない。
  - `read:direct` を分けるので、`read:statuses` だけのアプリには DM が見えない（DM を扱うアプリは `read:direct` を求める必要がある）。
  - `/api/v1` の非推奨の期間を守る分だけ、v0 の API の作り直しが遅くなる。
  - `admin` を一次専用にするので、サードパーティの管理ツールは作れない。
- 後続で必要になる作業（issue に分ける）
  1. クライアントの登録の経路の属性とイベント（`oauth_clients` の経路、無効化・削除のイベント、失効の理由の追加）と data-model 文書の identity の表
  2. 利用者によるアプリの登録（client API・設定の画面・confidential client のシークレット・登録者自身へのトークン発行）
  3. CIMD のクライアント（取得・キャッシュ・照合・ホスト単位の拒否）
  4. 動的クライアント登録（`POST /oauth/register`・メタデータの検証と置き換え・使われないクライアントの自動の削除のジョブ）
  5. ネイティブの redirect URI の検証（claimed `https`・private-use scheme・ループバックのポートの無視）
  6. 一次専用のスコープと操作の制限、`read:direct` の分割、同意の画面の表示（出どころ・redirect URI のホスト・なりすましの注意、i18n）
  7. 認可したアプリの一覧と取り消し（client API・設定の画面・最後に使った日時）
  8. 管理者の無効化（`manage_oauth_apps`・監査ログ）
  9. Authorization Server Metadata と Protected Resource Metadata、`WWW-Authenticate` の `resource_metadata`
  10. レート制限の追加（クライアントごと・利用者の登録・動的登録の IP と全体・CIMD の取得・`/oauth/token`）
  11. OpenAPI の公開と、非推奨のヘッダ・リリースノートの「API の変更」の節
  12. 英語の開発者向けの文書（#1031）
  13. 文書の更新: FR-003 の書き換え、ADR-0019 §5 の表、overview の認可の節、`api/spec/openapi.yaml` の securitySchemes の説明（一次専用のスコープ・`read:direct`）

## Pros and Cons of the Options（詳細比較・任意）

### 1. 利用者の登録 + CIMD + 動的登録（上限つき）+ 管理者の無効化（採用）

- 良い: 任意のインスタンスにつながるアプリ（モバイル・CLI）、1 つのインスタンス向けの連携、RFC 7591 だけを前提にする汎用のクライアントのすべてを賄う。
  CIMD を出せるクライアントには `client_id` のホストという検証済みの手がかりがあり、出せないクライアントも動的登録で使える。
- 悪い: 3 つの登録の経路を実装・説明する必要がある。未認証で行が増える口を持つので、上限と自動の削除の運用が要る。CIMD は草案。

### 2. 利用者の登録 + CIMD + 管理者の無効化（RFC 7591 なし）

- 良い（この選択肢を推す最も強い理由）: 任意のインスタンスにつながるアプリと 1 つのインスタンス向けの連携の両方を賄い、登録の行は認証済みの利用者だけが作る。
  `client_id` の URL のホストが、なりすましに対する唯一の検証済みの手がかりになる。Bluesky の AT Protocol OAuth は `client_id` を https のメタデータの URL にする同じ方式で（confidential client は `private_key_jwt`）、
  [MCP の認可の仕様 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization) も CIMD を SHOULD、RFC 7591 を後方互換のための MAY にしている（[atproto の文書](https://atproto.com/specs/oauth)と MCP の仕様、2026-10-23 に確認）。
- 悪い: RFC 7591 だけに対応する汎用のクライアント・ツールが使えない。CIMD は草案で、対応するクライアントがまだ限られる。

### 3. 未認証の動的クライアント登録（RFC 7591）だけ

- 良い（この選択肢を推す最も強い理由）: 標準化済みで、汎用の OAuth のライブラリ・ツールの多くが対応している。Mastodon と同じ使い勝手で、クライアントの開発者になじみがある。
  任意のインスタンスへの接続も、アプリが起動時に自分を登録すれば済む。
- 悪い: 未認証で `oauth_clients` が増えつづける（Mastodon の `POST /api/v1/apps` も認証不要、[Mastodon の文書](https://docs.joinmastodon.org/methods/apps/)、2026-10-23 確認）。
  登録された名前・ロゴは何の検証もないので、同意の画面で出どころを示せない。Mastodon の API と互換しない（ADR-0007）ので、Mastodon のクライアントになじみがあるという利点は実際には得られない。
  confidential client のための利用者による登録もないので、ボット・連携サービスの手段が欠ける。

### 4. 利用者の登録だけ

- 良い: 最も小さい実装。シークレットの管理も含めて自己完結する。
- 悪い: 公式のモバイルアプリが、利用者に「自分でアプリを登録して `client_id` を入れる」手順を強いる。配布されるクライアントは事実上作れない。

### 5. 管理者による登録だけ

- 良い: 管理者がすべてのアプリを把握・承認でき、スパムの心配がない。
- 悪い: インスタンスごとに管理者の手作業が要り、ほぼゼロコスト・小規模運用の目標と合わない。モバイルアプリが多数のインスタンスにつながれない。

## Open Questions / 要検証

- 要検証: CIMD の草案の今後の版での変更（WG 草案のため。実装の着手時に最新版と突き合わせる）。
- 要検証: Tauri 2 のアプリが Universal Links・App Links で OAuth のコールバックを実機で受けられるか（実機が要るため #31 の spike で確かめる）。
- 要検証: §7 のレート制限の既定値と、動的登録の自動の削除の期間（運用の計測が要る）。
