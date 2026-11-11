# ADR-0163: TLS 証明書の ACME による自動取得と DB への封じた保存

- Status: Accepted
- Date: 2026-11-11
- Deciders: @rosylilly
- Issue: #2823（spike。[報告](https://github.com/mycroblog/my/issues/2823#issuecomment-6786802481)）

## Decision（決定）

**`my` が自分で HTTPS を待ち受ける設定 `tls = off | acme | files`（既定 `off`）を足す。`acme` は `github.com/mholt/acmez/v3` の上に `my` の小さな管理部を書き、証明書・その鍵・ACME のアカウント鍵は `instance` モジュールの表に置き、鍵だけを `Sealer` で封じる。発行の試みと失敗はイベントとして DB に残し、再試行の待ちはそこから導く。KV は発注のリースとチャレンジの値だけに使う。TLS のハンドシェイクはメモリの証明書だけを読み、ほかのプロセスへの更新の知らせは `Bus` で送り、定期の DB の確認で取りこぼしを拾う。**

覆す条件は §12 に書く。

### 1. 位置づけ

- 小さな VPS で、1 つのバイナリとドメインとポート 80・443 だけで始められるようにする（ADR-0019 §9 の HTTPS 化の手段の 1 つ）。
- 既定は `off`。今と同じく平文の HTTP で待ち受け、リバースプロキシ・ロードバランサ・CDN が TLS を終端する。Cloud Run・ALB・Cloudflare の前段があるターゲットは `off` のまま使う。
- `files` は運営者が用意した証明書と鍵のファイルを読む（certbot など、ほかの仕組みで取る場合）。
- 証明書を扱うのは `role` が `web` か `all` のプロセスだけ。`worker` は TLS で待ち受けない。

### 2. ライブラリ

- ACME のクライアントは [`github.com/mholt/acmez/v3`](https://github.com/mholt/acmez)（Apache-2.0、v3.1.7、2026-09-25 公開）を使う。
  RFC 8555 に加えて ARI（RFC 9773 の `renewalInfo` と newOrder の `replaces`）とプロファイルの選択に対応する。リンクされる新しいモジュールはこれ 1 つで（ほかは `golang.org/x/crypto`・`x/net` で既に入っている）、
  `cmd/my` の増分は spike の管理部を含めて約 0.41 MiB（`CGO_ENABLED=0`、`-trimpath`、2026-11-11 に計測）。
- 証明書の管理部（いつ発注するか、どこに置くか、どのプロセスが発注するか）は `my` が持つ。保存の形（§4）とプロセス間の調整（§5・§6）を、ライブラリのファイル的な保存の抽象に合わせず、ADR-0020 の表と既存のポートで書くためである。
- 置き場所: 証明書の状態と判断（表、待ちの計算、リース、知らせ）は `internal/app/instance`、ACME の通信はポートの背後のアダプタ `internal/adapter/acme`、待ち受けと `tls.Config` の組み立ては `internal/platform`。
  ビルドタグ `no_acme` で外せる（ADR-0010 の形）。
- CA への HTTP は、運営者が設定する宛先なので `kernel.HTTPClient` を通さず、`internal/platform` が渡すリダイレクトに従わない専用の `*http.Client` を使う（ADR-0146 §7 と同じ規則）。

### 3. チャレンジとポート

- **TLS-ALPN-01（443）と HTTP-01（80）の両方を既定で有効にする。** 80 を開けない環境（`tls_http_listen` を空にする）では TLS-ALPN-01 だけで取れる。DNS-01（ワイルドカード）は扱わない。
- ポート 80 は、`/.well-known/acme-challenge/{token}` に答え、それ以外は `GET`・`HEAD` を `https://` の同じパスへ 301 で送り、ほかのメソッドは 400 にする（本文を平文で受け取らない）。
- チャレンジの値（HTTP-01 のキー認可、TLS-ALPN-01 のチャレンジ）は、CA の検証がどのプロセスに届いても答えられるよう、共有の `KeyValueStore` に TTL 10 分で置く。
  TLS-ALPN-01 の証明書は `acme-tls/1` の ALPN のハンドシェイクのときだけ KV から作る。通常のハンドシェイクは KV を読まない。
- root でない利用者でポート 80・443 を開く:
  - systemd: `AmbientCapabilities=CAP_NET_BIND_SERVICE` と `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`。
  - コンテナ: Docker 20.10 以降は、コンテナのネットワーク名前空間で特権ポートを権限なしに開ける既定の sysctl を付ける（[20.10 のリリースノート](https://docs.docker.com/engine/release-notes/20.10/)、moby#41030。2026-11-11 確認）。
    イメージ（distroless の `nonroot`）はそのままで、`ports: ["80:80", "443:443"]` を公開すればよい。`network_mode: host` では効かないので、そのときは `cap_add: [NET_BIND_SERVICE]` に加えて実行ファイルのケーパビリティが要る（要検証）。
  - Kubernetes / microk8s（M6）は Ingress で終端する前提で、この ADR の対象外。

### 4. 保存: `instance` モジュールの表（ADR-0020）

| 表 | 分類 | 列 | 備考 |
| --- | --- | --- | --- |
| `tls_acme_accounts` | リソース | `id`, `directory_url`, `account_url`, `sealed_private_key`, `created_at` | 一意 `(directory_url)`。CA ごとに 1 つ。2 つのプロセスが同時に登録したら `ON CONFLICT DO NOTHING` のあと読み直して 1 つに揃える |
| `tls_order_attempts` | イベント | `id`, `host`, `directory_url`, `profile`, `replaces_certificate_id`（NULL 可 = 初回。任意の関連）, `occurred_at` | リースを持つプロセスが発注する直前に書く |
| `tls_order_failures` | イベント（試みの結果の部分型） | `attempt_id`（PK）, `problem_type`（RFC 8555 の problem の `type`、またはクライアント側の失敗の分類）, `detail`（`string(1000)`）, `occurred_at` | |
| `tls_certificates` | リソース（試みの結果の部分型） | `id`, `attempt_id`（一意）, `host`, `directory_url`, `serial`, `not_before`, `not_after`, `chain_pem`, `sealed_private_key`, `created_at` | INSERT のみ |
| `tls_certificate_revocations` | イベント | `id`, `certificate_id`, `reason`（`enum(key_compromise, superseded, operator)`）, `revoked_by_user_id`（NULL 可 = CLI）, `occurred_at` | `my tls revoke` が CA に失効を求めたあと書く |

- **ホストの現在の証明書は、失効していないもののうち最後に発行したもの**（`created_at` の降順）で、索引 `(host, created_at)` を引く問い合わせにする。`not_after` の最大で選ばない: プロファイルを短くした更新（90 日から 45 日・6 日）は、置き換える証明書より先に切れる。
  件数はホストあたり年に十数件なので、投影は作らない。管理画面が一覧を要るようになったら、そのとき投影を足す。
- **封じるのは鍵だけ。** チェーンは公開の情報なので平文で持つ。AAD は `kernel.AAD("tls_certificates", id, "sealed_private_key")` と `kernel.AAD("tls_acme_accounts", id, "sealed_private_key")`。
  2 つの列は `my kek rewrap`・`my kek status` の対象の列（`internal/platform/setup_rewrap.go` の `sealedColumns`）に足す。新しい仕組みは要らない。
- **KEK を替えても再発行しない。** 鍵は rewrap で包み直す。アカウント鍵を失うと別のアカウントになり、ARI の更新（`replaces` は同じアカウントに限る）と、アカウントにひもづく CA の上限の調整を失うためである。
- DB に置く理由: 起動に DB は欠かせないので、証明書の置き場所が増やす故障の箇所がない。発行の履歴をイベントとして残せる。バックアップ（`my db backup`）にそのまま入る。

### 5. KV と DB の分担

- **DB（失ってはいけないもの）**: 証明書・鍵・アカウント、発注の試みと失敗の履歴。再試行の待ち（§7）は履歴から導くので、再起動でも KV の喪失でも戻らない。
- **KV（失ってよいもの）**:
  - 発注のリース `acme.order/<host>`: `CompareAndSwap` で乱数のトークンを TTL 10 分で書き、3 分ごとに延ばし、終わったら消す。延ばせなかったら発注の context を止める。
    死んだプロセスのリースは TTL で切れる。リースを取ったら DB を読み直し、ほかのプロセスが更新済みでないか、待ちの最中でないかを確かめてから発注する。
  - チャレンジの値（§3）。
- KV を失ったとき: リースが消えれば 2 つ目のプロセスが同時に発注しうるが、結果は証明書が 1 枚多いだけで（重複の証明書は 7 日に 5 枚まで）、待ちは DB にあるので失敗の連打にはならない。チャレンジの値が消えればその検証が失敗し、失敗として記録されて待ちに入る。
- `kv = db` と `kv = valkey` はどちらも TTL 付きの `CompareAndSwap` を持ち、リースに足りる（spike で `kv/db` の SQLite で確かめた）。`kv = memory` はプロセスごとなので、`tls = acme` で `role` が `all` でなければ設定の検査で誤りにする（チャレンジに別のプロセスが答えられない）。
- DB のロック（PostgreSQL の advisory lock）は使わない。発注の数秒〜数分のあいだ接続かトランザクションを持ち続け、外部 I/O をトランザクションに入れない規則（ADR-0003）に反する。SQLite にはない。DB に行で持つリースは `kv = db` そのものである。
- ARI の窓は DB にも KV にも置かず、各プロセスのメモリに持つ（CA にいつでも聞き直せる。認証の要らない GET で、発注の上限に数えない）。

### 6. メモリの証明書と、ほかのプロセスへの知らせ

- **ハンドシェイク（`GetCertificate`）はメモリだけを読む。** DB・KMS・KV に触れない（`acme-tls/1` の検証のハンドシェイクを除く）。ホストごとに解析済みの `tls.Certificate` を持つ。
  封じた鍵を開く（KMS の `UnwrapKey`）のは、証明書を読み込むときだけ: 起動時、自分が更新したとき、知らせを受けたとき、定期の確認で新しい行を見つけたとき。
  spike では 1 万回のハンドシェイクで `UnwrapKey` は 0 回、2 プロセスで発行と更新を 1 回ずつ通して計 6 回だった。ADR-0019 §13 のキャッシュは要らない（あっても害はない）。
- **更新・置き換え・失効のあと、そのプロセスは `Bus` に知らせを送る**。種類 `tls_certificate`、中身はホストと証明書の ID だけ（秘密を含まない。`MaxPayloadSize` の 1,024 バイトに収まる）。
  受けたプロセスは、そのホストの現在の証明書を DB から読み直す。知らせは「読み直せ」の合図で、中身の ID を入れない: 現在の証明書は DB が決めるので、遅れて届いた知らせや 2 度届いた知らせが古い証明書を戻すことはない。
- `Bus` は best effort である（`postgres` は接続が切れている間の知らせを失い、`valkey` は高々 1 回、`sns`・`pubsub` は少なくとも 1 回。ADR-0050 §1、`internal/app/stream/bus.go`）。だから次の 2 つで補う。
  - **定期の確認**: 5 分ごとに、ホストごとに現在の証明書の ID だけを DB に問い合わせ、違えば読み込む（索引の 1 行の読み取り）。メモリの証明書が更新の時期（§7）を過ぎたら、確認を 1 分ごとにする。
    期限切れまで待たない: 更新は残り 3 分の 1 の時点で始まるので、知らせが全部失われても、証明書が切れる数週間前に新しい証明書へ移る。
  - **リスナーが壊れたら**（`Listener.Broken`）、知らせを取りこぼしたかもしれないので、全ホストを読み直してから聞き直す。
- `Relay` は購読がある間だけ聞くので、`tls = acme` の `web` のプロセスでは、TLS の知らせのための購読を常に 1 つ持たせる（`Relay` に、ストリームの利用者でない内部の購読を足す）。1 つのプロセスが持てる `Bus` のリスナーは 1 つなので、TLS 用に別のリスナーは開かない。
- `stream = hub`（1 プロセス）は `Bus` がなく、知らせる相手もいない。更新したプロセスがそのままメモリを替える。定期の確認は、`my` のコマンド（`my tls revoke` など）が DB を変えたときのために残す。

### 7. 更新の時期と再試行

- **ARI（RFC 9773）を第一にする**: CA の `suggestedWindow` の中で一様に時刻を 1 つ選び、窓が変わるまでその時刻を保つ（確認のたびに選び直すと時刻が逃げ続ける。spike で起きた）。
  `Retry-After` は 1 分〜1 日に丸め、少なくとも 12 時間ごとに聞き直す（Let's Encrypt の [Integration Guide](https://letsencrypt.org/docs/integration-guide/) は 1 日 2 回以上を勧める。2026-11-11 確認）。
- ARI がない CA では、有効期間の 3 分の 2 が過ぎたら更新する。10 日未満の証明書は半分で更新する（同ガイド）。
- 更新の発注には `replaces` に前の証明書を付ける。Let's Encrypt では ARI による更新はすべての上限から外れる（[Rate Limits](https://letsencrypt.org/docs/rate-limits/)、2026-11-11 確認）。
- **再試行の待ち**: 前の証明書のあとに結果のない試み（失敗、または発注中にプロセスが死んで結果のないもの）が n 件あれば、最後の試みから 1 分（n = 1）・10 分・100 分・以後 1 日（同ガイドの例）。
  最初の 1 時間の 3 回は「識別子あたり 1 時間に 5 回の認可の失敗」を超えず、1 日 1 回なら「1,152 回の連続した失敗でアカウントを止める（1 日 1 回ずつ戻る）」に届かない（Rate Limits、2026-11-11 確認）。
- 運営者は `my tls renew -host <host>` で待ちを飛ばして今すぐ発注できる（ガイドの「管理者が早めの再試行を求められること」）。
- プロファイルは設定 `tls_acme_profile`（既定は空 = CA の既定）。Let's Encrypt の既定の `classic` は 2027-02-10 に 64 日、2028-02-16 に 45 日になる（[From 90 to 45](https://letsencrypt.org/2025/12/02/from-90-to-45)、2026-11-11 確認）。ARI に従うので、短くなっても設定は変えなくてよい。
- 初回の発行は起動を止めない。443 で待ち受けてから（TLS-ALPN-01 にそれが要る）発注し、取れるまでそのホストのハンドシェイクは失敗する。
- OCSP は扱わない（Let's Encrypt は 2025-08-06 に OCSP を止めた。[OCSP Service Has Reached End of Life](https://letsencrypt.org/2025/08/06/ocsp-service-has-reached-end-of-life)）。2 つ目の CA への切り替えは v0 では作らない（§12）。

### 8. 証明書を取るホスト

- 取るのは `host` と、`web_origin` のホストが `host` と違いこのサーバーが配るときのそのホストだけ。設定から決め、実行中に増やさない。
- SNI が一覧にないハンドシェイクは証明書を返さずに失敗させ、発注しない（任意の SNI で CA の上限を使わせない）。オンデマンドの発行は作らない。
- `host` が IP アドレス・`localhost`・ドットのない名前なら、`tls = acme` を設定の検査で誤りにする。

### 9. インターネットに直接出すときの HTTP サーバー

- 今の `http.Server` の `ReadHeaderTimeout`（10 秒）・`ReadTimeout`（60 秒）・`WriteTimeout`・`IdleTimeout`（120 秒）は、そのまま遅いクライアントへの守りになる。
- `MaxHeaderBytes` を既定の 1 MiB から 64 KiB に下げる。HTTP/2 は `http.Server.HTTP2` の `MaxConcurrentStreams` を 100 にする（Go の既定は 250。`net/http/internal/http2/server.go`）。HTTP/2 の rapid reset（CVE-2023-44487）は Go の側で抑えている（要検証: 版）。
- `tls.Config` は `MinVersion: tls.VersionTLS12`、暗号スイートは Go の既定。HSTS は今どおりアプリが付ける（ADR-0019 §9）。
- 前にプロキシがなければ、接続元のアドレスが利用者のアドレスである。`trusted_proxies` は空のままにする（CDN を前に置いて CDN から HTTPS で受けるときだけ設定する）。`tls != off` で `trusted_proxies` があれば設定の検査が警告する。
- Caddy と比べて失うもの: HTTP/3、応答の圧縮（`my` は圧縮しない。ビルド済みの資産の事前圧縮は別の作業）、アクセスログ、DNS-01 とワイルドカード。運営者の文書に書く。

### 10. 設定

| キー | 既定 | 意味 |
| --- | --- | --- |
| `tls` | `off` | `off` / `acme` / `files` |
| `listen` | `tls = off` で `:8080`、それ以外で `:443` | `tls != off` では TLS で待ち受ける |
| `tls_http_listen` | `:80` | HTTP-01 と `https` への転送。空で開かない（TLS-ALPN-01 だけになる） |
| `tls_acme_directory` | Let's Encrypt の本番の URL | ステージングの URL は文書に書く |
| `tls_acme_agree_terms` | `false` | CA の利用規約に同意する。`acme` では `true` が必須 |
| `tls_acme_email` | 空 | 連絡先。任意（Let's Encrypt は 2025-06-04 に期限切れの通知メールをやめた。[告知](https://letsencrypt.org/2025/06/26/expiration-notification-service-has-ended)） |
| `tls_acme_profile` | 空 | 発注のプロファイル |
| `tls_cert_file`・`tls_key_file` | 空 | `files` で必須。1 分ごとに更新時刻を見て、変わったら読み直す |

`my config check` は次を確かめる。

- `acme`: `tls_acme_agree_terms = true`、`host` が DNS の名前（§8）、`tls_acme_directory` が `https` の URL、`kv = memory` かつ `role != all` でないこと（§5）。
- `files`: 2 つのファイルが読めて鍵が証明書に合い、証明書が `host` を含み、切れていないこと。残り 14 日未満なら警告。
- `tls = off` で、`listen` がループバックでないアドレスのポート 80 か 443 なら「TLS を終端するものがないように見える」と警告する。
  実行時は、`tls = off`・`trusted_proxies` が空で、ループバックでない接続元から平文の要求が来たら、起動ごとに 1 度だけ同じ警告をログに出す。

### 11. メトリクスと運用

- `my.tls.certificate.expiry`（Prometheus: `my_tls_certificate_expiry_seconds`）: ゲージ。属性 `host`。メモリにある証明書の `NotAfter` までの秒。プロセスごとに出すので、古い証明書のまま残ったプロセスが分かる。
- `my.tls.order.attempts`（`my_tls_order_attempts_total`）: カウンタ。属性 `host`・`result`（`issued` / `failed`）。
- `my.tls.order.consecutive_failures`（`my_tls_order_consecutive_failures`）: ゲージ。属性 `host`。§7 の n。
- 警報の例: 残り 7 日未満、または連続した失敗が 3 以上。
- `my tls status`: ホストごとに現在の証明書（発行者・シリアル・期限）、次の更新の時刻、最近の試みと失敗を出す。`my tls renew -host`、`my tls revoke -host -reason`。
- 運営者の文書: `docs/operations/running.md` に「HTTPS を `my` 自身で配る」節（設定、ポート、systemd と Docker、ステージングでの試し方、失うもの）、本番のチェックリストの「Reachable over `https`」の確かめ方、`key-management.md` に封じた列が増えること。

### 12. 覆す条件

- 2 つ目の CA への自動の切り替えや、DNS-01（ワイルドカード）が v0 で要るなら、管理部を自分で持たず `certmagic` を使う案を見直す。
- `Relay` に常時の内部の購読を足すことが、ストリームの設計（ADR-0050）と合わないと分かったら、知らせをやめて定期の確認（1 分ごと）だけにする。
- 管理画面で証明書の一覧を頻繁に引くようになったら、ホストごとの現在の証明書の投影を足す。

## 経緯

`my` は平文の HTTP だけを話し、HTTPS はリバースプロキシ（Docker Compose では `compose.caddy.yaml` の Caddy）に任せていた。1 つのバイナリで小さく始めるという価値（vision）のため、VPS でプロキシなしに始められるよう #2823 で調べた。

最初の案は、証明書と鍵とアカウント鍵を `BlobStore` の非公開の接頭辞に置き、値ごとに `Sealer` で封じるものだった。spike でこれを `autocert.Cache` として実装し、Pebble で動くことを確かめた（キーは `blob.ValidateKey` を通り、AAD を Blob のキーにすれば値の付け替えは開けない）。
そのうえで、発行の履歴を見たいこと、`my kek rewrap` に乗らないこと、起動時の故障の箇所が増えることから、置き場所を DB に移し、状態をイベントとして持つ形にした。KV は失ってよいものだけに絞った。
最後に、ハンドシェイクを DB と KMS から切り離すメモリのキャッシュと、`Bus` での知らせを足した。

spike のコードと計測（コミットへのリンク）は [#2823 の報告](https://github.com/mycroblog/my/issues/2823#issuecomment-6786802481) にある。

## 採らなかった案

- **`BlobStore` に封じて置く**: どのロールも同じ場所を読め、スキーマを変えずに済む。だが `BlobStore` に一覧の操作がなく、`my kek rewrap`・`my kek status` に新しい経路が要る。起動時に Blob が読めないという、DB にはない故障が増える。履歴を持てない。DB に入れても大きさは証明書 1 枚で数 KiB で、DB を重くしない。
- **`golang.org/x/crypto/acme/autocert`**: 既に依存にあり、`cmd/my` の増分は約 0.28 MiB と最小。だが ARI と `replaces` がない（golang/go#60958 は 2026-11-11 時点で open）。プロセス間の排他がなく（コードに「分散環境では競合は避けられない」とある）、各プロセスが更新する。
  失敗の再試行は 30〜60 分ごとに上限なく続く。キャッシュの値は鍵とチェーンをまとめた不透明なバイト列で、鍵だけを封じることも履歴を持つこともできない。
- **`github.com/caddyserver/certmagic`**（v0.25.6）: ARI、ストレージを通した分散ロック、30 日までの指数的な再試行、複数の CA を持ち、最も揃っている。だが `cmd/my` が約 2.7 MiB 増え、`zap`・`miekg/dns`・`libdns`・`blake3` など 10 のモジュールが入る（ライセンスはすべて AGPL と両立: MIT・Apache-2.0・BSD-3-Clause・CC0）。
  `Storage` はディレクトリを持つファイルの木（`List`・`Stat`・`Lock`）を前提にしており、DB に置くと意味を持たない汎用のキーと値の表になって ADR-0020 規約 4 に合わない。証明書のキャッシュと更新の予定もライブラリが持ち、§6 の知らせと二重になる。
- **再試行の待ちと ARI の窓を KV に置く**: DB の書き込みが減る。だが Valkey の追い出しや再起動で待ちが消えると、失敗している CA へすぐに発注し直し、上限を使い切りうる。
- **DB の advisory lock**: §5 のとおり、外部 I/O の間ロックを持つことになり、SQLite にない。
- **Caddy だけに任せる（今のまま）**: 最も成熟し、HTTP/3 と圧縮もある。運営者の選択肢として残すが、1 つのバイナリで始めるという目的を満たさない。

## Consequences（影響）

- 良い: VPS で `my` だけで HTTPS を始められる。証明書の状態は DB の 1 か所にあり、バックアップと KEK の運用（rewrap・status）にそのまま乗る。発行と失敗の履歴を CLI と管理画面で見られる。
- 良い: ハンドシェイクは DB・KMS に触れないので、KMS の費用と遅延は証明書の読み込みの回数（起動と更新）だけにかかる。
- 悪い: ACME の管理部（予定、待ち、リース、知らせ）を自分で保守する。CA の振る舞いの変化（プロファイル、上限、ARI）に追従する責任が `my` にある。
- 悪い: Caddy の HTTP/3・圧縮・アクセスログを失う。`Relay` に内部の購読を足す変更が要る。
- 後続の作業: `instance` の表と封じた列（`sealedColumns`）、`internal/adapter/acme` とポート、`internal/platform` の待ち受けと設定（`docs/reference/configuration.md` の生成）、`Relay` の内部の購読と `tls_certificate` の種類、メトリクス、`my tls` のコマンド、`docs/operations/` の更新、Pebble を使う結合テスト（CI では `pebble` を Go のツールとして起動）。

## Open Questions / 要検証

- 期限切れから時間のたった証明書の行（と封じた鍵）を消すか。件数は小さいが、使わない鍵を残すことになる。消すなら ADR-0020 規約 5 の例外として、保持期間を決める。
- `account_domain` が `host` と違い、同じ `my` がそのドメインの WebFinger の転送も配る構成で、そのドメインの証明書も取るか（今は §8 のとおり取らない）。
- EAB（External Account Binding）を要する CA（ZeroSSL、Google Trust Services）の設定を v0 に入れるか。
- `network_mode: host` のコンテナでポート 80・443 を開く手順（§3）。
- Go の HTTP/2 の rapid reset への対策が入った版（§9）。
