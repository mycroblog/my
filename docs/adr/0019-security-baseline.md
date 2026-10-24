# ADR-0019: セキュリティベースライン

- Status: Accepted（§5 の制限に ADR-0090（OAuth をサードパーティのアプリに開く）の制限が加わる。一部を ADR-0032（§7 の送信の署名の既定）、ADR-0046（§10 の govulncheck） が置き換える）
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #34

## Context（背景・課題）

非機能要件 NFR-SEC-1〜8（[02-non-functional.md](../requirements/02-non-functional.md)）は「何を守るか」を定めているが、
「どう実装するか」を定めた ADR がない。Fediverse サーバーは **任意のリモートサーバーから送られてくる URL・HTML・メディア・署名付きリクエスト** を処理するため、
SSRF・XSS・署名偽装・リソース枯渇の攻撃面が通常の Web アプリより広い。

実装着手（M1）前に方針を決める理由:

- `HttpClient` ポート（[ADR-0006](0006-federation-protocol-scope.md)）、`KeyValueStore` ポート、認証・OAuth（[ADR-0007](0007-client-api.md)、ADR-0016）、
  メディア処理（[ADR-0005](0005-blob-storage-and-media.md)）の **インターフェイス設計に直接影響する**。後付けするとポートの契約が変わる。
- [ADR-0001](0001-server-language-and-runtime.md)（Accepted）で Go + `CGO_ENABLED=0` に決まり、Cloudflare Workers は対象外になった。
  NFR-SEC-3 の「ランタイムで Argon2id が使えない場合の代替」は不要になったので、パラメータを確定できる。
- Web クライアントの CSP は UI フレームワーク選定（[ADR-0008](0008-web-and-mobile-clients.md) の別 Issue）の制約になる。

関連: NFR-SEC-1〜8、NFR-PERF-4（Inbox は署名検証まで同期）、NFR-REL-4（リストア後も連合鍵が変わらない）、NFR-COST-2（常時起動ミドルウェアを必須にしない）、
NFR-PORT-1（コアはクラウド SDK に依存しない）、[ADR-0011](0011-license.md)（依存は AGPL-3.0 と両立するライセンスに限る）、
ADR-0016（クライアント API の様式）・ADR-0017（公開範囲 `local`）。ADR-0016 / 0017 と、独自クライアント API に改訂した ADR-0007 は執筆時点で未マージ（`docs/own-client-api-and-local-visibility` ブランチ）。

## Decision Drivers（判断基準）

1. **全ターゲットで同じ保護が効くこと**: Self-hosted の最小構成（バイナリ + SQLite）でも、Cloud Run / Lambda でも同じ防御が働く（G2, NFR-PORT）
2. **安全側の既定値**: 運営者が何も設定しなくても安全。緩める設定は明示的で、起動時に警告を出す
3. **連合の相互運用性**: Mastodon などの主要実装が実際に送ってくるものを拒否しない（G4）
4. **低コスト・低フットプリント**: 1 vCPU / 1 GB RAM で DoS されにくい（NFR-COST-2, NFR-COST-3）
5. **出典のあるパラメータ**: OWASP・RFC・IANA など公開された基準に基づき、根拠を追える
6. **依存の少なさとライセンス**: pure Go（`CGO_ENABLED=0`）・AGPL-3.0 と両立・保守されているライブラリだけを使う

## Considered Options（選択肢）

1. **アプリ内に組み込む**: Go 標準ライブラリと少数の実績あるライブラリで、ポートの契約・HTTP ミドルウェア・ドメインサービスとして実装する。インフラ側の保護は任意の追加層
2. **インフラに委ねる**: WAF（Cloud Armor / AWS WAF）、API Gateway のレート制限、リバースプロキシ（Caddy / nginx）のヘッダ付与、Egress プロキシ（Smokescreen 等）による SSRF 対策
3. **包括的なフレームワーク / 外部 IdP を採用する**: 認証・OAuth・セッションを Ory（Kratos / Hydra）や Keycloak に任せ、アプリはトークン検証だけ行う

## Decision（決定）

**1（アプリ内に組み込む）を採用する。** 以下を「ベースライン」とし、各機能の実装はこれに従う。数値はすべて設定で変更できる既定値。

### 1. SSRF ガード付き HttpClient ポート（NFR-SEC-1）

すべての外部 HTTP（連合の取得・配送、WebFinger、メディアプロキシ、リンクプレビュー、Web Push 送信）は `HttpClient` ポートを通す。
`net/http` の `http.DefaultClient` を直接使うことは lint（`forbidigo`）で禁止する。

**拒否するアドレス**（出典: [IANA IPv4 / IPv6 Special-Purpose Address Registry](https://www.iana.org/assignments/iana-ipv4-special-registry/)、2026-10-16 確認）。
実装は「`netip.Addr.IsGlobalUnicast()` が真」かつ「下表のどれにも含まれない」場合だけ許可する許可リスト方式とし、表は `netip.Prefix` の定数として持つ。

| 区分 | IPv4 | IPv6 |
| --- | --- | --- |
| 未指定・"this network" | `0.0.0.0/8` | `::/128` |
| ループバック | `127.0.0.0/8` | `::1/128` |
| プライベート | `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` | `fc00::/7`（ULA） |
| CGNAT（Shared Address Space） | `100.64.0.0/10` | — |
| リンクローカル | `169.254.0.0/16` | `fe80::/10` |
| 旧サイトローカル | — | `fec0::/10` |
| IETF 予約・ベンチマーク・文書用 | `192.0.0.0/24`, `192.0.2.0/24`, `198.18.0.0/15`, `198.51.100.0/24`, `203.0.113.0/24` | `2001::/23`, `2001:db8::/32`, `100::/64`（discard） |
| 6to4 リレー・マルチキャスト・予約・ブロードキャスト | `192.88.99.0/24`, `224.0.0.0/4`, `240.0.0.0/4`, `255.255.255.255/32` | `ff00::/8` |
| IPv4 を埋め込む IPv6 | — | `::ffff:0:0/96`（IPv4-mapped）、`64:ff9b::/96`・`64:ff9b:1::/48`（NAT64）、`2002::/16`（6to4）。**埋め込まれた IPv4 を取り出して IPv4 の表で再検査**し、拒否対象なら拒否 |

- クラウドのメタデータエンドポイントは上の表で塞がる: `169.254.169.254`（AWS / GCP / Azure）、`fd00:ec2::254`（AWS IMDS の IPv6、ULA に含まれる）、
  `100.100.100.200`（Alibaba、CGNAT に含まれる）。加えてホスト名 `metadata.google.internal`・`metadata` と、末尾が `.internal` / `.local` / `.localhost` のホスト名は名前解決前に拒否する。
- IP リテラルの URL（`https://10.0.0.1/`、10 進表記 `http://2130706433/` など）も `netip` で正規化してから同じ判定にかける。
- **DNS rebinding 対策**: 判定は名前解決後ではなく **接続直前** に行う。`net.Dialer{Control: guard}` を `http.Transport.DialContext` に設定し、
  `Control(network, address string, c syscall.RawConn)` が受け取る **実際に接続する IP:port** を `netip.ParseAddrPort` → `Unmap()` して検査する。
  Happy Eyeballs で複数アドレスを試す場合も、試行ごとに `Control` が呼ばれる。事前の名前解決結果を信用しないので、TTL 0 の rebinding でも破れない。
- スキームは `https` のみ（開発用に `http` を許可する設定あり）。ポートは既定で `443` と `80` のみ。
- `http.Transport.Proxy` は既定で `nil`（`HTTP_PROXY` 環境変数を無視）。運営者が Egress プロキシを設定した場合はプロキシ側で同等の制限をかける前提とし、起動時に警告を出す。
- **リダイレクト**: `CheckRedirect` で最大 5 回。各ホップでスキーム・ポート・ホスト名の拒否リストを再検査する（IP の検査は各ホップの接続時に `Control` で再度行われる）。
  `https` → `http` へのダウングレードは拒否。ActivityPub オブジェクトの取得では、最終 URL のオリジンと取得した `id` のオリジンが一致しない場合は採用しない。
- **タイムアウト**（`http.Transport` と `context`）: 接続 5 秒、TLS ハンドシェイク 5 秒、レスポンスヘッダ 10 秒、リクエスト全体 30 秒（メディアプロキシのストリーミングは 60 秒）。
  アイドル接続 90 秒、ホストあたりの同時接続数に上限（`MaxConnsPerHost` 既定 8）。
- **レスポンスサイズ上限**（`io.LimitReader` で上限 + 1 バイトまで読み、超えたらエラー。`Transport` が自動で gzip 展開した後の **展開後サイズ** に適用するので圧縮爆弾も防ぐ）:
  ActivityPub / WebFinger / NodeInfo の JSON 1 MiB、リンクプレビューの HTML 1 MiB、リモート画像 16 MiB、リモート動画・音声 100 MiB（要検証: Mastodon の上限に合わせて調整）。
  `Content-Length` が上限を超える場合は本文を読まずに打ち切る。
- 開発・テスト用に `HTTP_ALLOW_PRIVATE_NETWORKS=true`（`httptest` のループバックサーバーを使うため）を用意する。有効時は起動ログに警告を出し、本番プロファイルでは起動を拒否する。
- `testkit` に SSRF ガードのコントラクトテスト（上表の各レンジ、IPv4-mapped、NAT64、10 進表記、リダイレクトで内部に向かうケース、rebinding するスタブ DNS）を置き、全アダプタが通ることを必須にする。

### 2. リモート HTML のサニタイズ（NFR-SEC-2）

- ライブラリ: [`github.com/microcosm-cc/bluemonday`](https://github.com/microcosm-cc/bluemonday)（BSD-3-Clause、pure Go）。
- ポリシーは **Mastodon の `Sanitize::Config::MASTODON_STRICT` に合わせた許可リスト** にする（出典: Mastodon `lib/sanitize_ext/sanitize_config.rb`、要検証: 実装時に最新版と突き合わせる）。
  - 要素: `p`, `br`, `span`, `a`, `del`, `s`, `pre`, `blockquote`, `code`, `b`, `strong`, `u`, `i`, `em`, `ul`, `ol`, `li`, `ruby`, `rt`, `rp`
  - 属性: `a[href]`, `a[rel]`, `a[class]`, `span[class]`, `ol[start]`, `ol[reversed]`, `li[value]`。`class` は Mastodon / microformats が使う値（`mention`, `hashtag`, `u-url`, `h-card`, `invisible`, `ellipsis` 等）だけ残す
  - `href` のスキーム: `http`, `https`, `dat`, `dweb`, `ipfs`, `ipns`, `ssb`, `gopher`, `xmpp`, `magnet`, `gemini`。それ以外（`javascript:`, `data:` 等）は除去
  - `a` には `rel="nofollow noopener noreferrer"` と `target="_blank"` を強制する
  - 見出し（`h1`〜`h6`）は `strong` を含む `p` に、許可外のブロック要素は `p` に変換し、本文を失わないようにする
- **取り込み時にサニタイズし、サニタイズ済み HTML を保存する**（表示のたびにサニタイズしない）。ポリシーを変えたときに再サニタイズできるよう、ポリシーのバージョンを列に持つ。
- 表示名・ユーザー名・投票の選択肢・メディアの代替テキストなど、HTML を許さないフィールドはプレーンテキストとして扱い、HTML タグを除去する。
- ローカル投稿の HTML はサーバーがプレーンテキストから生成する（エスケープ + リンク・メンション・ハッシュタグの自動リンク）。利用者が HTML を直接書くことはない。
- Web クライアントはサニタイズ済み HTML 以外を `innerHTML` に入れない。カスタム絵文字はクライアントで DOM 要素として差し込む。二重の防御として CSP（§9）と Trusted Types を使う。

### 3. パスワードのハッシュ化（NFR-SEC-3）

- **Argon2id**（`golang.org/x/crypto/argon2` の `IDKey`、BSD-3-Clause、pure Go）。Workers の CPU 制約は ADR-0001 で前提から外れたため、代替アルゴリズムは用意しない。
- 既定パラメータ: **m = 19 MiB（19456 KiB）、t = 2、p = 1**、ソルト 16 バイト（`crypto/rand`）、出力 32 バイト。
  出典: [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)（Argon2id の最低構成の 1 つ、2026-10-16 確認）。
  [RFC 9106](https://www.rfc-editor.org/rfc/rfc9106) §4 の推奨（m = 2 GiB または 64 MiB）は 1 GB RAM の VM（NFR-COST-3）では同時ログインでメモリが枯渇するため採らない。
  余裕のある環境向けに、OWASP の別構成（m = 46 MiB, t = 1, p = 1 など）へ設定で切り替えられる。
- 保存形式は PHC 文字列（`$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`）。ログイン成功時に保存済みパラメータが現在の設定より弱ければ再ハッシュする。
- 比較は `crypto/subtle.ConstantTimeCompare`。存在しないアカウントでもダミーのハッシュ計算を行い、応答時間からアカウントの有無を推測させない。
- メモリ枯渇型の DoS を防ぐため、同時に実行するハッシュ計算をセマフォで制限する（既定: `GOMAXPROCS` 個）。待ちが溢れたら `503` を返す。
- パスワードの長さは 8 文字以上・最大 128 文字（[OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)）。
  合成規則（記号必須など）は課さない。漏洩パスワードの照合（HIBP の k-anonymity API 等）は任意機能として後続 Issue で検討する。

### 4. セッション・OAuth トークンの保管と CSRF

認可は OAuth 2.0 Authorization Code + PKCE（ADR-0007、ADR-0016）。[RFC 9700](https://www.rfc-editor.org/rfc/rfc9700)（OAuth 2.0 Security BCP）に従う。

- **トークンは平文で保存しない。** アクセストークン・リフレッシュトークン・認可コード・セッション ID・メール確認 / パスワードリセットのトークンは、
  `crypto/rand` の 256 ビット乱数を base64url にしたものを発行し、DB には **SHA-256 ハッシュだけ** を保存する（高エントロピーなので低速ハッシュは不要）。
  照合はハッシュ値の一意インデックスで引く。トークンには種別プレフィックス（例: `my_at_`, `my_rt_`）を付け、シークレットスキャナで検出しやすくする。
- 有効期限の既定: 認可コード 60 秒・一回限り（[RFC 6749 §4.1.2](https://www.rfc-editor.org/rfc/rfc6749#section-4.1.2) は最大 10 分を推奨）、アクセストークン 1 時間、リフレッシュトークン 30 日。
  リフレッシュトークンは使うたびにローテーションし、**使用済みのものが再利用されたらそのトークン系列をすべて失効** させる（RFC 9700 §4.14）。
- PKCE は `S256` のみ受け付け、`plain` は拒否する。`redirect_uri` は登録済みの値と完全一致で照合する。
- OAuth クライアントのシークレット（機密クライアントの場合）も SHA-256 ハッシュで保存する。
- ログイン・認可同意画面のセッションクッキー: `__Host-` プレフィックス、`Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/`。ログイン時にセッション ID を再発行する（セッション固定攻撃対策）。

**CSRF の方針**:

- クライアント API（`/api/v1/...`）は **`Authorization: Bearer` ヘッダでだけ認証** し、クッキーでは認証しない。ブラウザがクレデンシャルを自動送信しないので、API は CSRF の対象にならない。
  CORS は自前 Web クライアントのオリジン（設定値）だけを `Access-Control-Allow-Origin` に返し、`Access-Control-Allow-Credentials` は付けない。
- クッキーを使うのは、ログイン・認可同意・アカウント設定などサーバーが返すフォームと、[ADR-0030](0030-web-client-token-handling.md) のトークン更新（`POST /oauth/token/refresh`）だけ。これらには Go 標準の
  [`net/http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection)（Go 1.25 以降。`Sec-Fetch-Site` / `Origin` で非安全メソッドのクロスオリジン要求を拒否）を適用し、
  さらに `SameSite=Lax` で二重化する。Fetch Metadata を送らない古いブラウザ向けにはシンクロナイザートークンを併用する（出典: [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)）。
- Web クライアントでのトークン保管は [ADR-0030](0030-web-client-token-handling.md) に従う（アクセストークンはメモリ、リフレッシュトークンは HttpOnly の `__Host-` Cookie）。
  Tauri ではリフレッシュトークンを OS のセキュアストレージ（Keychain / Keystore）に置く（要検証: #31 の spike で確認）。

### 5. レート制限（NFR-SEC-5）

- **実装場所**: HTTP アダプタ層のミドルウェア（ルートごとに制限名を宣言）と、ログインなどアプリケーション層の明示的な呼び出しの 2 か所。コアは `RateLimiter` インターフェイスだけに依存する。
- **ストア**: `KeyValueStore` ポートに **原子的な「加算して TTL を設定」操作**（`IncrBy(key, n, ttl) (int64, error)`）を追加し、その上に GCRA（Generic Cell Rate Algorithm）またはスライディングウィンドウを実装する。
  既定アダプタは `kv/db`（追加ミドルウェア不要、NFR-COST-2）、単一ノードでは `kv/memory`、任意で `kv/redis`。DB への書き込み負荷が問題になる場合は memory に切り替える（複数インスタンス時は制限が緩くなることを許容）。
- **キーと既定値**（要検証: M1 の負荷計測で調整）:

| 対象 | キー | 既定 |
| --- | --- | --- |
| API 全体（未認証） | IP（IPv6 は `/64` に集約） | 300 リクエスト / 5 分 |
| API 全体（認証済み） | アカウント | 1,500 リクエスト / 5 分 |
| 投稿・メディアアップロード | アカウント | 投稿 300 / 3 時間、アップロード 30 / 30 分 |
| ログイン | IP、および対象アカウント | IP 5 回 / 分、アカウント 10 回 / 時（失敗のみ数える） |
| 二要素認証のコード（`login.second_factor`。ログインの 2 段目と設定での確認） | 利用者 | 10 回 / 時（失敗のみ数える。ログインの 2 段目の失敗は IP のログイン失敗にも数える） |
| アカウント登録・パスワードリセット | IP | 5 回 / 30 分 |
| Inbox | IP、および **リモートドメイン**（署名の `keyId` のホスト） | ドメイン 2,000 / 5 分、IP 1,000 / 5 分 |
| 公開鍵の再取得（§7） | `keyId` | 1 回 / 5 分 |

- 超過時は `429 Too Many Requests` と `Retry-After` を返す。Inbox の `429` は送信側のリトライ（Mastodon は指数バックオフで再送する）に任せる。
- クライアント IP は、設定した **信頼するプロキシ**（CIDR のリスト）経由の場合だけ `X-Forwarded-For` / `Forwarded` の右端から信頼できる値を採る。既定は接続元 IP のみ。
- Inbox はレート制限の前にボディサイズ上限（1 MiB、`http.MaxBytesReader`）を適用し、ドメインブロック済みのドメインは署名検証より前に拒否する。

### 6. 秘密情報の保管とアクター秘密鍵の暗号化（NFR-SEC-7）

- **アクター秘密鍵**（RSA 2048 ビット。将来 Ed25519 を追加）は DB に **AES-256-GCM で暗号化して** 保存する（エンベロープ暗号化）。
  - 鍵暗号化鍵（KEK）は DB の外（環境変数・シークレットファイル・クラウドのシークレットストア）に置く。DB のダンプだけが漏れても秘密鍵は漏れない。
  - 暗号文には KEK の ID（バージョン）を付け、KEK のローテーション時は新旧両方を設定して順次再暗号化できるようにする。AAD（追加認証データ）にアクター ID と列名を入れ、別の行への付け替えを防ぐ。
  - `KeyManager` ポートを定義し、アダプタは `local`（設定の KEK を使う。既定）、任意で `gcpkms` / `awskms`（KEK を KMS に置く）。KMS アダプタは v0 では作らず、後続 Issue とする。
- **KEK を失うと全アクターの秘密鍵を失い、連合上の同一性が壊れる**（NFR-REL-4）。バックアップ手順の文書に「DB と KEK を別々に、両方バックアップする」ことを明記する。
- その他の秘密（DB 接続文字列、SMTP パスワード、VAPID 秘密鍵、外部 OAuth のクライアントシークレット）は環境変数で受け取り、
  Docker / Kubernetes の secret をファイルで渡せるよう `<NAME>_FILE` 形式（ファイルパスから読む）も受け付ける（ADR-0010）。
- 秘密を保持する型は `slog.LogValuer` を実装して `[REDACTED]` を出力し、ログ・エラーメッセージ・トレースに漏らさない。

### 7. HTTP 署名検証の制限

Inbox と、Authorized Fetch 有効時のオブジェクト GET に適用する（ADR-0006 の draft-cavage-12 / RFC 9421）。

- **時刻ずれ**: 署名の `Date` ヘッダ（RFC 9421 では `created`）が **過去 12 時間より古い、または 1 時間より未来** なら拒否する。
  Mastodon の `EXPIRATION_WINDOW_LIMIT = 12.hours` と `CLOCK_SKEW_MARGIN = 1.hour` に合わせる（出典: Mastodon `app/controllers/concerns/signature_verification.rb`、要検証）。
  RFC 9421 の `expires` があればそれも検査する。
- **署名必須のコンポーネント**: draft-cavage は `(request-target)`、`host`、`date`、POST では `digest`。RFC 9421 は `@method`、`@target-uri`（または `@path` + `@authority`）、`created`、POST では `content-digest`。
  どれかが署名対象に含まれていなければ拒否する。
- **ダイジェスト必須**: POST は `Digest`（`SHA-256=`）または `Content-Digest`（RFC 9530、`sha-256` / `sha-512`）を **必須** とし、受信したボディから計算した値と照合する。
  ボディは 1 MiB の上限まで読んでから計算する。
- **アルゴリズム**: `rsa-sha256`（RSA 2048 ビット以上）、`hs2019` は鍵の種類から決める、RFC 9421 の `rsa-v1_5-sha256` / `rsa-pss-sha512` / `ed25519`。HMAC 系と 2048 ビット未満の RSA は拒否。
- **署名者と主体の一致**: `keyId` の公開鍵の `owner` がアクティビティの `actor` と一致し、かつ `keyId` のホストが `actor` のホストと一致すること。
  一致しない場合（リレーや転送されたアクティビティ）は署名を信用せず、オブジェクトを `id` から取り直して検証する。
- **公開鍵キャッシュ**: 取得した公開鍵は DB / KV に TTL **24 時間** でキャッシュする。検証に失敗したら、鍵のローテーションに備えて **1 回だけ** 再取得して再検証する。
  再取得は `keyId` ごとに 5 分に 1 回まで（§5）とし、偽の署名で外部取得を増幅させない。
- 未知のアクターからの `Delete`（アカウント削除の通知）は、鍵の取得が 404 / 410 になることが多いので鍵を取りに行かず、ローカルに対象が無ければ何もせず `202` を返す（取得の増幅を防ぐ）。
- 送信側の署名は draft-cavage-12（`rsa-sha256`）を既定とし、受信側が RFC 9421 を要求した場合は double-knocking で切り替える（ADR-0006）。

### 8. アップロードの安全性（NFR-SEC-6）

- **サイズ上限**（既定、設定可）: 画像 16 MiB、動画・音声 100 MiB（要検証: Mastodon 4.x の既定に合わせて調整）。`http.MaxBytesReader` で読み込み時に強制する。
- **形式の判定は中身で行う**: クライアントが申告した `Content-Type` とファイル名は信用しない。先頭バイトのシグネチャ（`http.DetectContentType` と、WebP / AVIF / MP4 など不足分の自前判定）で判定し、
  許可リスト（JPEG / PNG / GIF / WebP / AVIF / MP4 / WebM / MP3 / Ogg）にない形式と、申告と中身が食い違うものは拒否する。**SVG は受け付けない**（スクリプトを含められるため）。
- **寸法の上限**: 全体をデコードする前に `image.DecodeConfig` で寸法を読み、幅・高さ各 8,192 px 以下かつ総画素数 33 MP 以下でなければ拒否する（解凍爆弾対策）。GIF / WebP のアニメーションはフレーム数にも上限を設ける。
- **EXIF の除去**: 画像は `MediaProcessor` で **再エンコード** して保存する（再エンコードでメタデータは落ちる）。JPEG は EXIF の `Orientation` を先に画素へ適用してから除去する。
  `media/none` アダプタでも EXIF / XMP / ICC 以外のメタデータを除去する軽量処理は必須（ADR-0005）。クライアント側の除去は保険であり、サーバー側を省略する理由にしない。
- **保存と配信**: Blob のキーはサーバーが生成（UUIDv7）し、利用者のファイル名をパスに使わない。配信時は `X-Content-Type-Options: nosniff`、
  判定済みの `Content-Type`、`Content-Security-Policy: default-src 'none'; sandbox` を付ける。可能ならメディアは本体とは別のオリジン（例: `media.example.com`）から配る。
- デコード処理はタイムアウト付き・同時実行数に上限のあるワーカーで実行し、パニックはジョブ単位で回収する。

### 9. CSP とセキュリティヘッダ（Web クライアント）

Web クライアント（PWA）を配るレスポンスに付ける既定の CSP（出典: [OWASP Content Security Policy Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html)、[MDN CSP](https://developer.mozilla.org/docs/Web/HTTP/CSP)）:

```text
default-src 'none';
script-src 'self';
style-src 'self';
img-src 'self' blob: <media-origin>;
media-src 'self' blob: <media-origin>;
font-src 'self';
connect-src 'self' <api-origin>;
manifest-src 'self';
worker-src 'self';
form-action 'self';
frame-ancestors 'none';
base-uri 'none';
object-src 'none';
require-trusted-types-for 'script';
upgrade-insecure-requests
```

- `'unsafe-inline'` と `'unsafe-eval'` は使わない。**UI フレームワーク選定（ADR-0008 の別 Issue）は、このポリシーでビルド成果物が動くことを判断基準に加える。**
  リモートの画像・動画はメディアプロキシ（ADR-0005）経由なので、`img-src` に任意のリモートオリジンを入れる必要はない。
- `require-trusted-types-for` はブラウザの対応状況に差がある（要検証）。未対応のブラウザでは無視されるだけなので、付けたままにする。
- その他のヘッダ（Web クライアント・API・管理画面の全レスポンス。出典: [OWASP HTTP Headers Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Headers_Cheat_Sheet.html)）:

| ヘッダ | 値 |
| --- | --- |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains`（`preload` は運営者が設定で付ける） |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin`（外部リンクは `rel="noreferrer"` を付ける） |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=(), usb=()`（カメラ等が必要になったら `self` に緩める） |
| `Cross-Origin-Opener-Policy` | `same-origin` |
| `Cross-Origin-Resource-Policy` | Web クライアント・API は `same-site`、公開メディア・ActivityPub の JSON は `cross-origin` |
| `X-Frame-Options` | `DENY`（`frame-ancestors` 非対応ブラウザ向け） |

- HTTPS 化はリバースプロキシ / ロードバランサ / `autocert` のどれでもよいが、アプリは HSTS を自分で付ける。`http` での提供は開発プロファイルのみ。
- Tauri アプリは `tauri.conf.json` の CSP に同じポリシーを設定する（`connect-src` はログイン先インスタンスのオリジン）。

### 10. 依存関係の脆弱性スキャン（NFR-SEC-8）

- Go: [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)（BSD-3-Clause）を `make check` と CI で実行する。govulncheck は **実際に呼ばれるシンボル** だけを報告するので、報告があれば CI を失敗させる。
  加えて CI で日次実行し、新しい脆弱性の公開を main への変更が無くても検知する。
- 静的解析: `golangci-lint` で `gosec`、`forbidigo`（`http.DefaultClient`・`math/rand` の暗号用途・`InsecureSkipVerify` の禁止）を有効にする。
- TypeScript（Web / Tauri）: `bun audit`（要検証: 対応状況と CI での終了コード）で既知の脆弱性を検査する。
- 依存更新は Dependabot（または Renovate）で自動 PR 化する。ライセンス検査（ADR-0011）と同じ CI ジョブにまとめる。
- リリース時にコンテナイメージを Trivy 等で検査する（任意。後続 Issue）。

### 覆す条件

- 運営者の大半がマネージド WAF / API Gateway を前提にでき、Self-hosted の最小構成を捨ててよい場合は、レート制限とヘッダ付与を 2（インフラ）へ寄せる。
- 認証要件（SSO、WebAuthn、管理者の多要素認証）が v0 の範囲で大きく膨らみ、自前実装の保守が見合わない場合は、OAuth / 認証部分だけ 3（外部 IdP）を再検討する。
- Argon2id の既定パラメータで、1 vCPU の Cloud Run / Lambda でログインの p95 が 1 秒を超える場合は、OWASP の範囲内でパラメータを下げる（アルゴリズムは変えない）。

## Consequences（影響）

- 良い影響
  - Self-hosted の最小構成でもクラウドでも同じ防御が効き、コントラクトテストで保証できる。
  - SSRF ガードを接続時に行うので、名前解決・リダイレクト・rebinding のどの経路でも内部ネットワークに到達できない。
  - DB の漏洩だけでは、アクター秘密鍵・パスワード・トークンのどれも直接は悪用できない。
  - 依存は `golang.org/x/crypto`・`golang.org/x/vuln`・`bluemonday` だけで、いずれも pure Go・BSD-3-Clause（ADR-0001、ADR-0011 と両立）。
- 悪い影響・トレードオフ
  - セキュリティ機構の実装と保守を自前で持つ。特に SSRF ガード・署名検証・サニタイズは回帰テストへの投資が必須。
  - Argon2id はログイン 1 回あたり約 19 MiB のメモリと数十 ms の CPU を使う（要検証）。同時実行数の制限でログインが待たされることがある。
  - `kv/db` でのレート制限は DB への書き込みを増やす。高負荷時は `kv/memory` や `kv/redis` が必要になる。
  - Mastodon と同じ許可リストに合わせるため、Misskey の MFM などリッチな表現は失われる（ActivityPub 上は HTML のフォールバックが来るので表示は可能）。
  - リフレッシュトークンを IndexedDB に置くので、XSS が起きた場合の被害は BFF 方式より大きい。CSP と Trusted Types で緩和する前提。
  - KEK の管理が運営者の新たな責務になる。失うと連合上の同一性を失う。
- 後続で必要になる作業（Issue 化する）
  - `HttpClient` ポートの SSRF ガード実装とコントラクトテスト（`testkit`）
  - `KeyValueStore` ポートへの `IncrBy` 追加とレート制限ミドルウェア
  - `KeyManager` ポート（`local`）とアクター秘密鍵の暗号化、KEK ローテーション手順、バックアップ文書の更新
  - HTML サニタイズポリシー（Mastodon との差分テスト用フィクスチャ）
  - パスワードハッシュとログインのレート制限、トークン発行・ハッシュ保存・リフレッシュトークンのローテーション
  - HTTP 署名検証（時刻ずれ・ダイジェスト・鍵キャッシュ）とフィクスチャ
  - アップロード検証（形式判定・寸法・EXIF 除去）
  - セキュリティヘッダ / CSP ミドルウェアと、UI フレームワーク選定 Issue への CSP 要件の追記
  - CI への `govulncheck`・`gosec`・`bun audit`・Dependabot の追加
  - KMS アダプタ（`gcpkms` / `awskms`）、漏洩パスワード照合、管理者の多要素認証（v0 以降でも可）

## Pros and Cons of the Options（詳細比較・任意）

### 1. アプリ内に組み込む（採用）

- 良い: 全ターゲットで同じ挙動（Driver 1）。安全な既定値をコードで保証できる（Driver 2）。追加の常時起動コンポーネントが不要（Driver 4）。
- 悪い: 実装・保守の負担。インフラの WAF ほど大規模な攻撃（L3 / L4 の DDoS）には耐えられない。

### 2. インフラに委ねる

- 良い（最も強い論点）: WAF・CDN は L3 / L4 / L7 の DDoS をアプリに届く前に止められ、アプリでは不可能な規模の防御ができる。
  Egress プロキシによる SSRF 対策は言語やライブラリに依存せず、設定ミスがあってもネットワーク層で止まる。コードも減る。
- 悪い: Self-hosted の最小構成（バイナリ + SQLite）には WAF もプロキシも無く、保護が消える（Driver 1）。
  ターゲットごとに設定方法が違い、テストできない。Cloud Armor / AWS WAF は固定費がかかる（NFR-COST-1）。
  連合固有の判断（署名の `keyId` からのドメイン単位の制限、ActivityPub の `id` とオリジンの一致）はアプリ外ではできない。
- 結論: この決定と矛盾しないので、運営者が **追加の層** として使うことは推奨する（デプロイ文書に記載）。

### 3. 包括的なフレームワーク / 外部 IdP

- 良い: OAuth / OIDC・多要素認証・アカウント回復などが成熟した実装で手に入る。
- 悪い: Ory / Keycloak は常時起動のサービスを増やし（NFR-COST-2）、Keycloak は JVM でメモリが大きい（NFR-COST-3）。
  SSRF・サニタイズ・署名検証など連合固有の課題は解決しない。

## Open Questions / 要検証

- [ ] Mastodon の現行のサニタイズ設定（`MASTODON_STRICT`）と、署名の時刻ずれ定数を最新ソースで確認する
- [ ] Mastodon 4.x のメディアサイズ上限・Inbox のボディ上限を確認し、既定値を揃える
- [ ] Argon2id（m = 19 MiB, t = 2, p = 1）の 1 vCPU（Cloud Run / Lambda / 1 GB VM）での所要時間とメモリの実測（M1）
- [ ] `kv/db` でのレート制限の書き込み負荷（SQLite / PostgreSQL、NFR-PERF-1 の想定負荷）
- [ ] Trusted Types と `require-trusted-types-for` の主要ブラウザでの対応状況、選定する UI フレームワークでの動作
- [ ] `bun audit` の CI での使い方（終了コード・除外設定）
- [ ] Tauri でのリフレッシュトークンのセキュアストレージ（#31 の spike で確認）
