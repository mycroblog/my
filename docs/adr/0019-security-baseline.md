# ADR-0019: セキュリティの基準と鍵の管理

- Status: Accepted
- Date: 2026-10-16
- Deciders: @rosylilly
- Issue: #34, #1842

## Decision（決定）

セキュリティの保護はアプリ内に組み込む（Go 標準ライブラリと少数の実績あるライブラリで、ポートの契約・HTTP ミドルウェア・ドメインサービスとして実装する。インフラ側の保護は任意の追加層）。以下を「ベースライン」とし、各機能の実装はこれに従う。数値はすべて設定で変更できる既定値。

### 1. SSRF ガード付き HTTPClient ポート（NFR-SEC-1）

すべての外部 HTTP（連合の取得・配送、WebFinger、メディアプロキシ、リンクプレビュー、Web Push 送信）は `kernel.HTTPClient` ポートを通す。
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

- **Argon2id**（`golang.org/x/crypto/argon2` の `IDKey`、BSD-3-Clause、pure Go）。代替アルゴリズムは用意しない。
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

認可は OAuth 2.0 Authorization Code + PKCE（ADR-0016）。[RFC 9700](https://www.rfc-editor.org/rfc/rfc9700)（OAuth 2.0 Security BCP）に従う。

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
- クッキーを使うのは、ログイン・認可同意・アカウント設定などサーバーが返すフォームと、[ADR-0090](0090-third-party-oauth-apps.md) のトークン更新（`POST /oauth/token/refresh`）だけ。これらには Go 標準の
  [`net/http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection)（Go 1.25 以降。`Sec-Fetch-Site` / `Origin` で非安全メソッドのクロスオリジン要求を拒否）を適用し、
  さらに `SameSite=Lax` で二重化する。Fetch Metadata を送らない古いブラウザ向けにはシンクロナイザートークンを併用する（出典: [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)）。
- Web クライアントでのトークン保管は [ADR-0090](0090-third-party-oauth-apps.md) に従う（アクセストークンはメモリ、リフレッシュトークンは HttpOnly の `__Host-` Cookie）。
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

OAuth のクライアントの登録などの制限は ADR-0090 §7 が足す。

- 超過時は `429 Too Many Requests` と `Retry-After` を返す。Inbox の `429` は送信側のリトライ（Mastodon は指数バックオフで再送する）に任せる。
- クライアント IP は、設定した **信頼するプロキシ**（CIDR のリスト）経由の場合だけ `X-Forwarded-For` / `Forwarded` の右端から信頼できる値を採る。既定は接続元 IP のみ。
- 公開の CDN のアドレスを信頼すると、同じ CDN のサーバーレス環境から呼ぶ第三者もその範囲から接続できる。そこでオリジンが公開のままの構成のために、**CDN が付ける秘密のヘッダを検査して、経由しないリクエストを拒否する** 設定を持つ（`origin_secret`、`origin_secret_header`、ローテーション中の `origin_secret_previous`）。HTTP を提供するすべてのプロセスで、保守モードでも、信頼するプロキシの判定より先に定数時間で比較する。ヘルスチェックと `/_internal/*` と Lambda の SQS イベントだけは CDN を通らないので除外し、秘密を示さないリクエストは除外の対象でも転送ヘッダと `traceparent` を信じない。詳細は [deployment-targets](../architecture/deployment-targets.md#cdn-を経由しないリクエストの拒否)。
- Inbox はレート制限の前にボディサイズ上限（1 MiB、`http.MaxBytesReader`）を適用し、ドメインブロック済みのドメインは署名検証より前に拒否する。

### 6. 秘密情報の保管とアクター秘密鍵の暗号化（NFR-SEC-7）

- **アクター秘密鍵**（RSA 2048 ビットと、RFC 9421 の署名と Object Integrity Proofs に使う Ed25519）は DB に **AES-256-GCM で暗号化して** 保存する（エンベロープ暗号化）。
  - 鍵暗号化鍵（KEK）は DB の外（環境変数・シークレットファイル・クラウドのシークレットストア）に置く。DB のダンプだけが漏れても秘密鍵は漏れない。
  - 暗号文には KEK の ID（バージョン）を付け、KEK のローテーション時は新旧両方を設定して順次再暗号化できるようにする。AAD（追加認証データ）にアクター ID と列名を入れ、別の行への付け替えを防ぐ。
  - `KeyManager` ポートを定義し、アダプタは `local`（設定の KEK を使う。既定）、任意で `gcpkms` / `awskms`（KEK を KMS に置く。§11）。
- **KEK を失うと全アクターの秘密鍵を失い、連合上の同一性が壊れる**（NFR-REL-4）。バックアップ手順の文書に「DB と KEK を別々に、両方バックアップする」ことを明記する。
- その他の秘密（DB 接続文字列、SMTP パスワード、VAPID 秘密鍵、外部 OAuth のクライアントシークレット）は環境変数で受け取り、
  Docker / Kubernetes の secret をファイルで渡せるよう `<NAME>_FILE` 形式（ファイルパスから読む）も受け付ける（ADR-0010）。
- 秘密を保持する型は `slog.LogValuer` を実装して `[REDACTED]` を出力し、ログ・エラーメッセージ・トレースに漏らさない。

### 7. HTTP 署名検証の制限

Inbox と、Authorized Fetch 有効時のオブジェクト GET に適用する（ADR-0006 の draft-cavage-12 / RFC 9421）。方式ごとの受信の細部（方式の選び方・必須の対象・時間窓）は ADR-0006 が決める。

- **時刻ずれ**: draft-cavage は署名の `Date` ヘッダ（または `(created)`）が **過去 12 時間より古い、または 1 時間より未来** なら拒否する。RFC 9421 は `created` が 1 時間より未来なら拒否し、`expires`（無ければ `created + 12 時間`）を 1 時間過ぎていたら拒否する。
  Mastodon の `EXPIRATION_WINDOW_LIMIT = 12.hours` と `CLOCK_SKEW_MARGIN = 1.hour` に合わせる（出典: Mastodon `app/controllers/concerns/signature_verification.rb`、要検証）。
- **署名必須のコンポーネント**: draft-cavage は `(request-target)`、`host`、`date`（または `(created)`）、POST では `digest`。RFC 9421 は `@method`、`@target-uri`（または `@path` + `@authority`、クエリ文字列があれば `@query`）、`created`、POST では `content-digest`。
  どれかが署名対象に含まれていなければ拒否する。
- **ダイジェスト必須**: POST は `Digest`（`SHA-256=`。`SHA-512` も受ける）または `Content-Digest`（RFC 9530、`sha-256` / `sha-512`）を **必須** とし、受信したボディから計算した値と照合する。
  ボディは 1 MiB の上限まで読んでから計算する。
- **アルゴリズム**: `rsa-sha256`（RSA 2048 ビット以上）、`hs2019` は鍵の種類から決める、RFC 9421 の `rsa-v1_5-sha256` / `rsa-pss-sha512` / `ed25519`。HMAC 系と 2048 ビット未満の RSA は拒否。
- **署名者と主体の一致**: `keyId` の公開鍵の `owner` がアクティビティの `actor` と一致し、かつ `keyId` のホストが `actor` のホストと一致すること。
  一致しない場合（リレーや転送されたアクティビティ）は署名を信用せず、オブジェクトを `id` から取り直して検証する。
- **公開鍵キャッシュ**: 取得した公開鍵は DB / KV に TTL **24 時間** でキャッシュする。検証に失敗したら、鍵のローテーションに備えて **1 回だけ** 再取得して再検証する。
  再取得は `keyId` ごとに 5 分に 1 回まで（§5）とし、偽の署名で外部取得を増幅させない。
- 未知のアクターからの `Delete`（アカウント削除の通知）は、鍵の取得が 404 / 410 になることが多いので鍵を取りに行かず、ローカルに対象が無ければ何もせず `202` を返す（取得の増幅を防ぐ）。
- 送信側の署名は draft-cavage-12（`rsa-sha256`）を既定とし、400 / 401 が返ったら RFC 9421 で 1 回だけ再送する double-knocking を行う（再送の規則と、成功した方式の記憶は ADR-0006）。

### 8. アップロードの安全性（NFR-SEC-6）

- **サイズ上限**（既定、設定可）: 画像 16 MiB、動画・音声 100 MiB（要検証: Mastodon 4.x の既定に合わせて調整）。`http.MaxBytesReader` で読み込み時に強制する。
- **形式の判定は中身で行う**: クライアントが申告した `Content-Type` とファイル名は信用しない。先頭バイトのシグネチャ（`http.DetectContentType` と、WebP / AVIF / MP4 など不足分の自前判定）で判定し、
  許可リスト（JPEG / PNG / GIF / WebP / AVIF / MP4 / WebM / MP3 / Ogg）にない形式と、申告と中身が食い違うものは拒否する。**SVG は受け付けない**（スクリプトを含められるため）。
  例外は `manage_settings` を持つ管理者が設定するサーバーのロゴとアイコンの SVG だけで、ADR-0059 §9 のサニタイズと §12 のヘッダーを条件にする。利用者のメディアに例外はない。
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

- `'unsafe-inline'` と `'unsafe-eval'` は使わない。**UI フレームワークは、このポリシーでビルド成果物が動くものを選ぶ。**
  リモートの画像・動画はメディアプロキシ（ADR-0005）経由なので、`img-src` に任意のリモートオリジンを入れる必要はない。
- `require-trusted-types-for` はブラウザの対応状況に差がある（要検証）。未対応のブラウザでは無視されるだけなので、付けたままにする。
- その他のヘッダ（Web クライアント・API・管理画面の全レスポンス。出典: [OWASP HTTP Headers Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Headers_Cheat_Sheet.html)）:

| ヘッダ | 値 |
| --- | --- |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains`（`preload` は運営者が設定で付ける） |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin`（外部リンクは `rel="noreferrer"` を付ける） |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=(), usb=()`（カメラ等が必要になったら `self` に緩める。音声チャットが有効なら `microphone=(self)`、ADR-0100） |
| `Cross-Origin-Opener-Policy` | `same-origin` |
| `Cross-Origin-Resource-Policy` | Web クライアント・API は `same-site`、公開メディア・ActivityPub の JSON は `cross-origin` |
| `X-Frame-Options` | `DENY`（`frame-ancestors` 非対応ブラウザ向け） |

- HTTPS 化はリバースプロキシ / ロードバランサ / `autocert` のどれでもよいが、アプリは HSTS を自分で付ける。`http` での提供は開発プロファイルのみ。
- Tauri アプリは `tauri.conf.json` の CSP に同じポリシーを設定する（`connect-src` はログイン先インスタンスのオリジン）。

### 10. 依存関係の脆弱性スキャン（NFR-SEC-8）

- 既知の脆弱性の検知と版更新の提案は Dependabot に任せる（ADR-0081）。CI のジョブ・タスクとしての脆弱性検査は持たない（`govulncheck` も `bun audit` も動かさない）。
- 静的解析: `golangci-lint` で `gosec`、`forbidigo`（`http.DefaultClient`・`math/rand` の暗号用途・`InsecureSkipVerify` の禁止）を有効にする。
- リリース時にコンテナイメージを Trivy 等で検査する（任意。後続 Issue）。

### 11. KMS のアダプタ

- `internal/adapter/keymanager/gcpkms`（`cloud.google.com/go/kms`）と `internal/adapter/keymanager/awskms`（aws-sdk-go-v2 の `kms`）。認証は ADC と SDK の既定の資格情報の連鎖。
  どちらの SDK も Apache-2.0 で、新しく入る推移的な依存は `cloud.google.com/go/longrunning`（Apache-2.0）だけ（gRPC・genproto・認証のモジュールは `stream/pubsub` で入っている）。
  `CGO_ENABLED=0` でビルドでき、`cmd/my` の大きさは `gcpkms` で約 1.2 MiB、`awskms` で約 0.7 MiB 増える（2026-10-29 に `kms` v1.36.1・`service/kms` v1.62.1 を足したビルドで計測）。
  ビルドタグ `no_gcpkms` / `no_awskms`
- `WrapKey` は KMS の Encrypt で 32 バイトの DEK を包み、`UnwrapKey` は Decrypt で開く。KEK は KMS の外に出ない。
  暗号文は封じた値のヘッダーの 2 バイトの長さに収まる（AWS KMS の `CiphertextBlob` は最大 6,144 バイト。2026-10-29 に [Encrypt](https://docs.aws.amazon.com/kms/latest/APIReference/API_Encrypt.html) で確認）
- **AAD**: Cloud KMS は `additionalAuthenticatedData` にそのまま渡す（ソフトウェアの鍵で最大 64 KiB）。AWS KMS は暗号化コンテキスト（文字列の map）に `{"my-aad": <aad の base64url>}` として渡す
  （大きさの上限は API の文書に書かれていないが、`kernel.AAD` の値は行の ID と列の名前で数十バイト）。
  暗号化コンテキストは CloudTrail のログに平文で残るが、AAD は秘密を含まない
  （2026-10-29 に [Cloud KMS encrypt](https://docs.cloud.google.com/kms/docs/reference/rest/v1/projects.locations.keyRings.cryptoKeys/encrypt)・[Encryption context](https://docs.aws.amazon.com/kms/latest/developerguide/encrypt_context.html) で確認）
- **鍵 ID**: `gcpkms:<CryptoKey のリソース名>`、`awskms:<鍵の ARN>`。
  - Cloud KMS は版を含めない。Decrypt は鍵の名前で呼ぶとサーバーが版を選び、ローテーションは古い版を無効にしないので、KMS 側の自動のローテーションで鍵 ID は変わらず、包み直しも要らない。
    古い版を無効化・破棄すると、その版で包んだ値は開けない（2026-10-29 に [Decrypt](https://docs.cloud.google.com/kms/docs/reference/rest/v1/projects.locations.keyRings.cryptoKeys/decrypt)・
    [Rotate a key](https://docs.cloud.google.com/kms/docs/rotate-key)・[Key version states](https://docs.cloud.google.com/kms/docs/key-states) で確認）。運用文書に「古い版を無効にする前に、版を替えた KMS の鍵へ `my kek rewrap` で移す」ことを書く
  - CryptoKey の名前は最長でも約 221 バイト（プロジェクト ID 30・鍵リングと鍵の ID 各 63・最長のロケーション 23）で、接頭辞 `gcpkms:` を足しても 255 に収まる（[Resource hierarchy](https://docs.cloud.google.com/kms/docs/resource-hierarchy) とリソースの作成の API の規則から計算）。
    ドメインの付いた古いプロジェクト ID のように 255 バイトを超える名前は、起動時の設定の検査で拒む
  - AWS KMS は起動時に `DescribeKey` で別名（alias）を ARN に直してから使う。別名は付け替えられるので鍵 ID にしない。対称鍵の Decrypt は暗号文から鍵を知れるが、違う鍵の取り違えを `IncorrectKeyException` で拒ませるため `KeyId` を渡す
    （2026-10-29 に [Decrypt](https://docs.aws.amazon.com/kms/latest/APIReference/API_Decrypt.html) で確認）
- **エラー**: 暗号文や AAD が合わない（AWS KMS の `InvalidCiphertextException`、Cloud KMS では `INVALID_ARGUMENT` の見込み）は `ErrDecrypt`。
  鍵 ID が今の鍵でも前の鍵でもないときは `ErrUnknownKey`。権限の不足（`PERMISSION_DENIED`・`AccessDeniedException`）・鍵の無効化（Cloud KMS では `FAILED_PRECONDITION` の見込み、AWS KMS の `DisabledException`）は設定の誤りとしてそのまま返し、
  上限（`RESOURCE_EXHAUSTED`・`ThrottlingException`）・一時的な失敗は再試行できるエラーにする
- **クォータ**: AWS KMS の対称鍵の暗号操作は us-east-1 でアカウント・リージョンあたり毎秒 100,000 回で、全操作で共有する（2026-10-29 に [Request quotas](https://docs.aws.amazon.com/kms/latest/developerguide/requests-per-second.html) で確認）。
  Cloud KMS はリージョンごとのトークンの量で数える（ソフトウェアの鍵の利用は毎分 6,000,000 トークン。[Quotas](https://docs.cloud.google.com/kms/quotas)）。§13 のキャッシュのもとでは、どちらにも遠く届かない

### 12. メディアプロキシの署名鍵を DB に封じて置く

- 32 バイトの乱数の鍵を、最初の起動で作って `Sealer` で封じ、インスタンスの秘密の表に置く（resource。行は名前で一意。ADR-0020）。起動時に 1 回開き、メモリに持つ
- `key_manager = local` も同じにする。KEK の HKDF から導出せず、鍵の出どころを 1 つにする。この置き場所へ移したとき、移す前に署名した URL は一度だけ無効になる（v0.1.0 の前なので許す。プロキシの URL は API が出し直す）
- 封じた鍵は `my kek rewrap` の対象の列に足す。KEK を替えても署名鍵は変わらない
- 署名鍵そのもののローテーションは、この ADR の対象外にする（独立の要件がない）

### 13. 包みを解いた DEK のキャッシュ

- `kernel` に `KeyManager` を包むデコレータを置き、`UnwrapKey` の結果を `(鍵 ID, 包んだ DEK, AAD)` のハッシュを鍵にキャッシュする。既定は 10 分・1,024 件で、設定で変えられる（0 で無効）
- `WrapKey` はキャッシュしない（新しい秘密を封じるときだけで、回数が少ない）
- `key_manager = local` では使わない（KEK がメモリにあり、キャッシュの意味がない）
- 安全面: キャッシュが持つのは DEK で、プロセスのメモリを読める攻撃者には、`local` では KEK そのものが、KMS では有効期間内の DEK が見える。KMS の構成でもこれは `local` より悪くならない。
  キャッシュの値はログ・エラーに出さない（`slog.LogValuer` で `[REDACTED]`）
- 呼び出しの見積り: 配送・取得で使う秘密（ローカルのアクターの秘密鍵、インスタンスのアクター）1 つにつき、プロセスあたり 10 分に 1 回。
  10 個のアクターが常に配送するサーバーで、プロセス 1 つなら月約 4.3 万回（30 日 × 144 回 × 10）で、AWS KMS では無料枠を超えた分の月 $0.07 ほど、無料枠のない Cloud KMS では月 $0.13 ほど。キャッシュがないと、配送・取得の数と同じだけ呼ぶ

### 14. KMS との移行

- `kernel` に、包むのは現在の `KeyManager`、開くのは鍵 ID の接頭辞（`local:` / `gcpkms:` / `awskms:`）で選んだ `KeyManager`、という振り分けを置く
- 設定: `key_manager` が現在。`kek_previous`（と、`key_manager` が `local` でないときの `kek`）は開くだけの `local` の KEK。
  `kms_key`（現在の KMS の鍵）と `kms_key_previous`（開くだけの同じ事業者の KMS の鍵のリスト）
- `local` から KMS へ: `key_manager = gcpkms`・`kms_key` を設定し、今の `kek` を `kek_previous` に移して起動し、`my kek rewrap` を流す。`my kek status` が 0 件になったら `kek_previous` を外す。逆も同じ手順
- 違う事業者の KMS への移行（Cloud KMS から AWS KMS）は、`local` を経由する 2 回の移行で行う。直接の移行は作らない（一度に 2 つの SDK の資格情報を持たせない）

### 覆す条件

- 運営者の大半がマネージド WAF / API Gateway を前提にでき、Self-hosted の最小構成を捨ててよい場合は、レート制限とヘッダ付与をインフラへ寄せる。
- 認証要件（SSO、WebAuthn、管理者の多要素認証）が v0 の範囲で大きく膨らみ、自前実装の保守が見合わない場合は、OAuth / 認証部分だけ外部 IdP を再検討する。
- Argon2id の既定パラメータで、1 vCPU の Cloud Run / Lambda でログインの p95 が 1 秒を超える場合は、OWASP の範囲内でパラメータを下げる（アルゴリズムは変えない）。
- インスタンスの秘密の表を置くことが、ほかのモジュールの表の持ち方と合わないと分かった場合は、メディアプロキシの署名鍵を別の設定 `media_proxy_key` に移す（§12）。
- 運用上、DEK をメモリに置く時間を 0 にする要件が出た場合は、キャッシュをやめ、配送の遅延と KMS の費用を受け入れる（§13）。

## 経緯

NFR-SEC-1〜8 は「何を守るか」を定めるが、「どう実装するか」は決まっていなかった。Fediverse サーバーは任意のリモートサーバーから送られる URL・HTML・メディア・署名付きリクエストを処理するので、SSRF・XSS・署名偽装・リソース枯渇の攻撃面が通常の Web アプリより広い。
これらは `HTTPClient`・`KeyValueStore` ポート、認証、メディア処理のインターフェイスに直接影響するので、M1 の実装の前に決めた。関連: NFR-PERF-4、NFR-REL-4、NFR-COST-2、NFR-PORT-1、ADR-0011。

判断基準は、全ターゲット（Self-hosted の最小構成でも Cloud Run / Lambda でも）で同じ保護が効くこと、運営者が何も設定しなくても安全で、緩める設定は明示的で起動時に警告を出すこと、Mastodon など主要実装が実際に送るものを拒否しないこと、1 vCPU / 1 GB RAM で DoS されにくいこと、OWASP・RFC・IANA など公開された基準に基づくこと、pure Go で AGPL-3.0 と両立し保守されている依存だけを使うこと。

Web クライアントのトークン保管は、当初リフレッシュトークンを IndexedDB に置くとしたが、ADR-0090 で HttpOnly の Cookie に移した。依存の脆弱性の検査は、当初 `govulncheck` を PR ごとの CI と日次実行で動かすとしたが、計算コストが高いので ADR-0081 で Dependabot Alerts に移した。送信の署名の順序は ADR-0006 が定め、ADR-0006 との食い違いを解消した。

KMS のアダプタは、M6 の Google Cloud・AWS のターゲットで KEK を環境変数やシークレットに置かず IAM で使わせるために決めた。`kernel.KeyManager`（`CurrentKeyID` / `WrapKey` / `UnwrapKey`）を KMS の Encrypt / Decrypt に写すのは素直だが、KEK から導出していたメディアプロキシの署名鍵、`kernel.Sealer.Open` が値を開くたびに `UnwrapKey` を呼ぶこと（配送と署名付きの取得は要求のたびにアクターの秘密鍵を開く。`account.Service.SigningKey`）、鍵 ID の形、`local` からの移行を決める必要があった。メディアプロキシの鍵の置き場所を変えると既存の URL の署名が変わるので、v0.1.0 より前に決めた（ADR-0023）。判断基準は、KEK も KEK から導出した鍵もプロセスの設定とディスクに置かない、配送を KMS の遅延と障害から守る、運営者に新しい秘密を持たせない、小さなサーバーで無料枠か月数十セントに収まる（NFR-COST）、`local` と KMS の間を秘密を失わずに移せる、SDK をアダプタと `cmd/` に閉じる、である。

料金（2026-10-29 確認）: AWS KMS は鍵 1 つ月 $1（ローテーションごとに月 $1 を足し、2 回目で上限）、対称鍵の要求 1 万回 $0.03、無料枠は全リージョンで月 2 万要求（[AWS KMS pricing](https://aws.amazon.com/kms/pricing/)）。
Cloud KMS はソフトウェアの鍵の版 1 つ月約 $0.06（有効・無効・破棄の予定の版を数え、破棄した版は無料）、暗号操作 1 万回 $0.03 で、無料枠は Autokey で作った鍵だけ（[Cloud KMS pricing](https://cloud.google.com/kms/pricing)。月額は時間あたりの単価と料金ページの例から換算）。

## 採らなかった案

- インフラに委ねる（WAF、API Gateway、リバースプロキシ、Egress プロキシ）: L3 / L4 / L7 の DDoS をアプリに届く前に止められ、Egress プロキシは設定ミスがあってもネットワーク層で止まる。ただし Self-hosted の最小構成で保護が消え、ターゲットごとに設定が違ってテストできず、固定費がかかり、`keyId` からのドメイン単位の制限や `id` とオリジンの一致はアプリ外でできない。運営者が**追加の層**として使うことは推奨する（デプロイ文書に記載）。
- 包括的なフレームワーク / 外部 IdP（Ory、Keycloak）: OAuth / OIDC・多要素認証・アカウント回復が成熟した実装で手に入るが、常時起動のサービスが増え、Keycloak はメモリが大きく、SSRF・サニタイズ・署名検証など連合固有の課題は解決しない。
- メディアプロキシの署名鍵を別の設定 `media_proxy_key`（と `_previous`）にして KMS のときに必須にする: 表が要らず独立にローテーションできるが、KMS を選んだのに環境変数の秘密が残る。
- KMS の MAC の操作（Cloud KMS の `MacSign`、AWS KMS の `GenerateMac`）で URL ごとに署名する: 鍵が KMS の外に出ないが、URL を出すたび・検証するたびに KMS を呼ぶ。
- `UnwrapKey` をキャッシュしない: DEK がメモリに残らないが、配送 1 件ごとに KMS の遅延と料金が乗り、KMS の障害で連合がすぐ止まる。
- 開いた秘密をモジュールの側でキャッシュする: 署名鍵をそのまま再利用でき `x509` の解析も省けるが、秘密の種類ごとに有効期間と消去の規則がばらつき、秘密鍵そのものがメモリに長く残る。
- 移行用の専用のコマンドで一度に包み直す: 設定の意味は単純だが、移行が一度きりの操作になり、途中で止まったときの再開と移行中に封じた値の扱いが要る。

## Consequences（影響）

- 良い影響
  - Self-hosted の最小構成でもクラウドでも同じ防御が効き、コントラクトテストで保証できる。
  - SSRF ガードを接続時に行うので、名前解決・リダイレクト・rebinding のどの経路でも内部ネットワークに到達できない。
  - DB の漏洩だけでは、アクター秘密鍵・パスワード・トークンのどれも直接は悪用できない。
  - 依存は `golang.org/x/crypto`・`bluemonday` だけで、いずれも pure Go・BSD-3-Clause（ADR-0002、ADR-0011 と両立）。
  - KMS の構成では、プロセスの設定とディスクに KEK も導出した鍵もない。KEK の漏洩の経路が IAM の権限に限られ、KMS の監査ログに使われ方が残る。
  - 配送の要求が KMS を待たない。KMS の短い障害は、キャッシュの有効期間の内なら配送を止めない。運営者は KMS の鍵を 1 つ用意するだけで、新しい秘密を持たない。
- 悪い影響・トレードオフ
  - セキュリティ機構の実装と保守を自前で持つ。特に SSRF ガード・署名検証・サニタイズは回帰テストへの投資が必須。インフラの WAF ほど大規模な攻撃（L3 / L4 の DDoS）には耐えられない。
  - Argon2id はログイン 1 回あたり約 19 MiB のメモリと数十 ms の CPU を使う（要検証）。同時実行数の制限でログインが待たされることがある。
  - `kv/db` でのレート制限は DB への書き込みを増やす。高負荷時は `kv/memory` や `kv/redis` が必要になる。
  - Mastodon と同じ許可リストに合わせるため、Misskey の MFM などリッチな表現は失われる（ActivityPub 上は HTML のフォールバックが来るので表示は可能）。
  - KEK の管理が運営者の新たな責務になる。失うと連合上の同一性を失う。
  - KMS の鍵に月額がかかる（AWS は月 $1〜$3、Cloud KMS は版ごとに月約 $0.06）。KMS は任意で、`local` が既定のまま。
  - キャッシュの有効期間の間、DEK がメモリに残る。無効にもできる。
  - キャッシュの有効期間を超える KMS の障害では、秘密を開けず、配送・ログインの 2 段階認証・webhook が失敗する（ジョブは再試行される）。起動時のメディアプロキシの鍵も開けず、起動が失敗する。
- 後続で必要になる作業（Issue 化する）
  - 漏洩パスワード照合、管理者の多要素認証（v0 以降でも可）

## Open Questions / 要検証

- Mastodon の現行のサニタイズ設定（`MASTODON_STRICT`）と、署名の時刻ずれ定数を最新ソースで確認する
- Mastodon 4.x のメディアサイズ上限・Inbox のボディ上限を確認し、既定値を揃える
- Argon2id（m = 19 MiB, t = 2, p = 1）の 1 vCPU（Cloud Run / Lambda / 1 GB VM）での所要時間とメモリの実測（M1）
- `kv/db` でのレート制限の書き込み負荷（SQLite / PostgreSQL、NFR-PERF-1 の想定負荷）
- Trusted Types と `require-trusted-types-for` の主要ブラウザでの対応状況
- Tauri でのリフレッシュトークンのセキュアストレージ（#31 の spike で確認）
- Cloud Run・Lambda からの 1 回の Decrypt の遅延（公式の文書に目安がない。M6 のデプロイで計測し、§13 のキャッシュの既定の有効期間を見直す）
- Cloud KMS の Decrypt が AAD の不一致と鍵の無効化に返すエラーのコード（KMS の文書に書かれていない。`INVALID_ARGUMENT`・`FAILED_PRECONDITION` は他のサービスの文書と実例による見込みで、実装の PR で実際の KMS に対して確かめる）
