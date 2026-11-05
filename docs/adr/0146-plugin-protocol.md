# ADR-0146: 外部プラグインのプロトコルと基盤

- Status: Accepted
- Date: 2026-11-05
- Deciders: @rosylilly
- Issue: #2214（エピック #2228）

## Decision（決定）

覆る条件は §18 に書く。

### 1. 結論

`my` は、運営者が動かす別プロセスのプログラムやサービスへ、選んだポートと事前の判定を Connect で委譲する。

- `my` はアドレスへ接続するだけのクライアントで、プラグインのプロセスを管理しない
- 宛先は設定ファイルと環境変数だけで決める
- 契約は Go のインターフェイスと別の `v1` の proto で、MIT にする
- 既定では何も委譲せず、追加のプロセスも接続もない
- プロプライエタリのプラグインを認める設計で実装を進める。法務の確認は `v0.1.0` のタグの条件で、済むまで文書はライセンスについて何も述べない（§17）

### 2. 範囲と時期

委譲するのは次の種類（kind）で、エピック #2228 のポートとフックを `v0.1.0` の前にすべて作る。後から足すと proto・設定・スキーマの制約の互換を背負う点が増えるので、最初の版で揃える。proto は `v0.1.0` のタグで凍り、以後は §9 に従う。

| kind | ポート | 課題 |
| --- | --- | --- |
| `translator` | `translation` の翻訳のポート（[ADR-0148](0148-post-translation.md)） | #2217 |
| `billing` | `finance.BillingSource` | #2218 |
| `mailer` | `kernel.Mailer` | #2219 |
| `media` | `media.Processor`（`AVProcessor` を含む） | #2220 |
| `key_manager` | `kernel.KeyManager` | #2221 |
| フック | [ADR-0147](0147-decision-hooks.md) の各フック | #2222〜#2227 |

委譲の基準は、トランザクションの外で呼ぶこと、遅延を許すこと、既存の標準プロトコルがないこと、インターフェイスが小さく安定していること、失敗しても機能が縮退で済むこと、セキュリティの境界でないこと。`KeyManager` はこの最後の基準の意図的な例外で、理由は §14 に書く。DB とトランザクションに結びつくポートは委譲しない。

### 3. プロトコル

- **Connect を使う。** ライブラリは `connectrpc.com/connect/v2`（Apache-2.0）。1 つの proto から Connect・gRPC・HTTP + JSON の 3 つで話せる
- 複数のプロトコルを受けられるのはサーバーの側の性質で、ここでは **`my` がクライアント** である。プラグインが受けるものに `my` が合わせるので、kind ごとに `plugin_<kind>_protocol` を置く

| 値 | 送るもの | 相手 |
| --- | --- | --- |
| `json`（`media` 以外の既定） | Connect の unary を `application/json` で | Connect のライブラリのサーバー、および `net/http` と JSON だけのプラグイン |
| `proto`（`media` の既定） | Connect を `application/proto` で | Connect のライブラリのサーバー |
| `grpc` | gRPC（HTTP/2） | grpc-go など gRPC だけのサーバー |

- 既定が `json` なのは、これだけが 3 種類のうち 2 種類の相手に届き、小さなプラグインがライブラリなしで動く（#2216 の受け入れ条件）から。バイナリの Connect を JSON だけのプラグインへ送ると `415` になり、Connect を grpc-go へ送っても `415` になるので、設定の誤りは起動時の確認と §16 の CLI で出す
- JSON の形は proto3 の標準の対応に従う: `int64` は文字列、`bytes` は base64、列挙は名前。手書きのプラグイン向けに、運用文書に kind ごとの例を置く

検証（2026-11-05、Go 1.27.3・`connect/v2` v2.0.2・grpc-go v1.85.0 のスクラッチのテスト、`CGO_ENABLED=0`）: `net/http` と `encoding/json` だけのプラグインへ Unix ソケットの HTTP/1.1 で届く。余分な JSON のフィールドは無視される（`connectproto` の `DiscardUnknown`）。`WithGRPC` は grpc-go へ Unix ソケットの h2c で届く。

### 4. プロセスモデル

- `my` は `plugin_<kind>_address` に接続する。形は `unix:///run/my/mailer.sock`・`https://mailer.internal:8443`・`http://...` の 3 つ。サブプロセスは起動も監視もしない。運営者がサイドカーや systemd で動かす
- kind の選択は既存の設定の値を増やす: `mail = plugin`・`media = plugin`・`key_manager = plugin`・`billing_sources` の `plugin`。翻訳は ADR-0148、フックは ADR-0147 の選択に従う
- 選ばれていない kind のクライアントは作らない。プラグインを使わないサーバーは接続もプロセスも増えない
- ビルドタグ `no_plugins` で全体を外せる（既存の `no_*` と同じ）

### 5. 宛先

- `plugin_<kind>_address`・`_protocol`・`_token`・`_tls_ca_file`・`_tls_cert_file`・`_tls_key_file`・`_timeout` は設定ファイルと環境変数だけのキーで、インスタンスの設定の表にも管理画面にもクライアント API にも出さない。管理者のアカウントが盗まれても投稿や鍵の行き先を変えられない
- `_token` は秘密の設定で、`<NAME>_FILE` でも渡せる（ADR-0019 §6）。変更は再起動で反映する
- 宛先は検証時に DNS を解決せず、形だけ確かめる。ホスト名は接続ごとに解決する

### 6. 認証と輸送

認証は次の 3 つから選ぶ。

- **Unix ソケット**: ソケットのファイルの権限で守る
- **mTLS**: `_tls_cert_file` と `_tls_key_file`
- **bearer トークン**: `_token` を `Authorization: Bearer` で送る。**TLS か Unix ソケットの上でだけ** 許し、`http://` の宛先に `_token` があれば起動時に設定のエラーにする

`https://` のサーバー証明書は常に検証する（`_tls_ca_file` か OS の信頼する CA）。検証を切る設定は作らない。TLS の最小の版は Go の既定に従う。

平文の TCP（`http://`）を拒むのは **`key_manager`・`mailer`・投稿の本文を送るフックだけ** で、これらは起動時に設定のエラーにする。ほかの kind は平文の TCP を使える。

| kind | 運ぶもの | 平文の TCP |
| --- | --- | --- |
| `key_manager` | 平文の DEK | 拒否 |
| `mailer` | 確認のリンクなどの秘密、宛先 | 拒否 |
| `inbound.status`・`local.status` のフック | 投稿の本文 | 拒否 |
| `translator` | 読者が求めた投稿の本文（IRI とハンドルは送らない。ADR-0148 §5） | 許可 |
| `media` | メタデータの残るアップロード（位置情報を含みうる） | 許可 |
| `billing` | 費用の集計 | 許可 |
| `signup.check` のフック | メールアドレスと IP | 許可 |
| `inbound.follow` のフック | フォローの当事者 | 許可 |
| `media.check` のフック | 処理後の小さな版。原本は送らない | 許可 |

- 平文の TCP を使う kind は、起動時に kind と宛先を挙げて警告する。bearer を使えないので、相手の認証も暗号化もない。そのネットワークを守るのは運営者の責任で、運用文書にそう書く
- 線の理由: 拒むのは、漏れると別の被害に広がる秘密（DEK、確認のリンク）を運ぶ kind と、受信や投稿のたびに本文を流し続けるフックである。`translator` は読者が求めたときだけ本文を送り、`media` と合わせて出力を `my` が確かめる（§10、§13）。どちらも自前の LibreTranslate や別のマシンの変換のように閉じたネットワークで動かす構成が主で、そこに証明書の管理を必須にしない

**ループバックに例外を置かない。** `http://127.0.0.1` も平文の TCP で、上の表に従う。拒む kind はループバックでも拒む。同じホストの別のプロセスから読まれない理由にならず、例外を置くと判定が名前解決やネットワーク名前空間の話になる。これらのサイドカーは共有ボリュームの Unix ソケットで話す。許可する kind はループバックでそのまま動く。

### 7. SSRF ガードとの関係

プラグインの宛先へは `kernel.HTTPClient` を **通さない**。

- `kernel.HTTPClient` はプライベートのアドレスを拒むが、サイドカーや社内のプラグインはそこにある。§5 により宛先は運営者だけが決め、リモートのサーバーも利用者も影響できないので、AGENTS.md の SSRF の規則の対象（リモートのサーバーや利用者が影響できる宛先）に当たらない
- 手本は `internal/adapter/voice/livekit/livekit.go` と `internal/platform/voice.go` の `voiceHTTPClient`（[ADR-0101](0101-voice-chat-transport.md) §10）: adapter は `Options.HTTPClient` で受け、`internal/platform` が組み立て、リダイレクトを追わない。Connect の文書も RPC のクライアントにリダイレクトを追わせない設定を例示する
- lint は `http.DefaultClient`・`http.Get` などだけを禁じる（`.golangci.yml` の `forbidigo`）ので、専用の `*http.Client` は通る

voice との違いが 2 つある。

- `http.Client.Timeout` は使わず、期限は `context` で切る。`Timeout` はストリームの全体にかかり、§13 と合わない
- `http.Transport` は専用に作り、`Proxy` は `nil`、接続 5 秒・TLS 5 秒、`MaxConnsPerHost` は 8 とする

プラグインが返す値から `my` が URL を取りに行くことはしない。そうなる設計が要るなら、その URL は SSRF の規則の対象である。

### 8. proto の置き場と生成

**新しいトップレベルのディレクトリは要らない。** `api/plugin/` に置く。

```text
api/plugin/
  LICENSE                                     # MIT
  buf.yaml, buf.gen.yaml                      # SPDX は MIT
  my/plugin/<kind>/v1/<kind>.proto            # package my.plugin.<kind>.v1
internal/adapter/plugin/gen/...               # 生成物（Go、AGPL-3.0-only、コミットする。ADR-0081）
internal/adapter/plugin/client/               # 接続・認証・上限の共通の部分
internal/adapter/<port>/plugin/               # 各ポートの adapter（translator・billing・mail・media・keymanager）
internal/adapter/hook/connect/                # フックの adapter（#2222）
```

- 既存の `api/*` は `scripts/changed-contexts.sh` で `api` の文脈に入り、`tools/agentkit/context-map.yaml` の `api/**` にも含まれる
- パッケージは kind ごとに分け、共有の型を置かない。ある kind の `v2` がほかの kind に波及しない
- 生成は `buf`（Apache-2.0）と `protoc-gen-go`・`protoc-gen-connect-go`。`protoc` は要らない。`scripts/task.sh generate` の依存に `api:plugin-generate` を足し、`api:check` に生成物の鮮度・`buf lint`・§9 の `buf breaking` を足す
- `buf` は `go tool` で固定するが、`tools/go.mod` に足すと `google.golang.org/protobuf` が `v1.37.0-rc.1` へ上がる（2026-11-05 に `tools/go.mod` の複製で確認。`buf` v1.75.0、`go.mod` は 56 行増）。`protoc-gen-go` の版は製品の `protobuf` に揃えるので、`buf` だけ `tools/buf/go.mod` に分ける
- `go_package` は proto に書かず、`buf.gen.yaml` の managed mode で与える。プラグインの作者に `my` の内部のパスを見せない（2026-11-05 に確認）

**`api/plugin/` の MIT は次の 4 か所で揃える。** [ADR-0093](0093-api-client-library-mykit.md) §2 と AGENTS.md の MIT の例外が範囲を定め、残りは最初のファイルを足す #2216 が揃える。

1. `scripts/license-scope-check.sh`: `api/plugin/*` を `want=MIT required=1` にする
2. AGENTS.md の MIT の例外の一覧と、ルートの `README.md` のライセンスの節
3. ADR-0093 §2 の MIT の範囲
4. 生成物の見出し: `protoc-gen-go`・`protoc-gen-connect-go` は proto の先頭のコメントをそのまま写す（2026-11-05 に確認）ので、`SPDX-License-Identifier: MIT` が `internal/` に写って検査で落ちる。AGENTS.md の規則どおり生成物を AGPL-3.0-only にするため、生成のタスクが見出しの 1 行を書き換える

proto は well-known types も使わず（時間は `int64` のミリ秒）、ほかの proto を import しない。MIT の範囲に GPL 系の依存は入らない。

### 9. 版

proto の版は kind ごとの `v1` で、`my` の Go のインターフェイスとは別に動く。内部のインターフェイスを変えても adapter の写像で吸収する（AGENTS.md は v0.x で内部のインターフェイスの変更を許す）。

`v0.1.0` のタグより前は `v1` を自由に変えてよい。タグの後の `v1` で **許すのは、昨日のプラグインが無視しても正しく動き続ける変更だけ** である。

| 変更 | `v1` の中で |
| --- | --- |
| 新しい番号でフィールドを足す | 許す。無視されても意味が変わらないものに限る |
| 省略可能な RPC を足す | 許す。`unimplemented` のときの `my` の動作を proto のコメントに書く |
| 列挙の値を足す | proto のコメントで「知らない値」の扱いを定めた列挙に限り許す |
| 上限を緩める、期限を延ばす | 許す |
| フィールド・RPC・サービスの削除、改名 | 禁止。JSON の名前と Connect のパスも契約で、番号が同じでも壊れる |
| 型や番号の変更、番号の再利用 | 禁止 |
| プラグインが読まないと誤動作するフィールドの追加 | 禁止（`v2`） |
| 同じ値の意味の変更、`my` が課す上限を狭める変更 | 禁止（`v2`） |

- 検査は `buf breaking` の `FILE` の規則で、タグ `v0.1.0` 以降の最新のタグと比べる。2026-11-05 にディレクトリとの比較で、追加が通り、フィールドの改名と列挙の値の削除が落ちることを確かめた。意味の変更は機械で見つからないのでレビューで止める
- **`v2` の共存:** `api/plugin/my/plugin/<kind>/v2/` に新しいパッケージを足す。パス（`/my.plugin.mailer.v2.MailerService/Send`）が違うので、1 つのプラグインのサーバーが `v1` と `v2` を同時に出せる
  - `my` は版ごとに adapter を持ち、`plugin_<kind>_version` で選ぶ。既定は古い版のままで、`my` の新しい版が来ても黙って切り替わらない
  - 自動の検出はしない。`unimplemented` への自動のフォールバックは、障害と古いプラグインの区別がつかず、`key_manager` で黙って版が下がる
  - `v2` を出したら `v1` は少なくとも 2 つのマイナー版のあいだ残し、使っているあいだ起動時に警告し、リリースノートで消す版を告げる

### 10. 結果とエラーの写し方

- **ポートの結果はメッセージの `outcome` の列挙で返す。** Connect のエラーは運搬の失敗だけを表す。JSON だけのプラグインが Connect のエラーの詳細を組み立てずに済み、`BillingSource` の「失敗しても使った要求の数を返す」も表せる
- **列挙の 0 は `*_UNSPECIFIED` で、成功にしない。** 空の JSON の `{}` と、知らない列挙の名前は 0 に、知らない数値はそのまま届く（2026-11-05 に確認）。どれも成功と読まず、そのポートの一時的な失敗か不正な応答として扱う
- Connect のエラーは、ポートの一時的な失敗に写す（`my` のジョブが再試行する）。例外は §14
- **プラグインの自由文を受け取らない。** proto の応答に自由文のフィールドを置かず、Connect のエラーの `message` は捨てて `code` だけをログに出す。`Mailer` の「エラーとログに宛先・本文・相手の応答の文を出さない」はこれで構造的に守られる
- `my` の adapter は再試行しない。再試行は呼び出す側（ジョブ）の責務で、配送は at-least-once なので、プラグインは同じ呼び出しの重複に耐える（[ADR-0004](0004-job-queue-abstraction.md)）
- **プラグインの出力は信頼しない。** adapter はポートの契約を満たすことを確かめてから返す（翻訳のサニタイズ、請求の値の検査、§13 のメディアの検査）

### 11. Go のポートから proto への写像

`Name()` のような `context` もエラーもないメソッドは、呼び出しごとに通信できないので、設定か起動時の 1 回に写す。

| ポートのメソッド | proto / 写し方 |
| --- | --- |
| `BillingSource.Name()` | 固定の `plugin`。1 つの宛先が 1 つの source（複数の事業者はプラグインの中で集約し、`service` で分ける） |
| `BillingSource.Location()` | 設定 `plugin_billing_location`（IANA の名前）。起動を通信に依存させない |
| `BillingSource.Method()` | 固定の `plugin` |
| `BillingSource.Fetch` | unary `Fetch`。`months`（`YYYY-MM`）・`max_requests` → `outcome`（`ImportErrorKind` と同じ値に `OK`）・`lines`・`requests`・`bytes_billed` |
| `CostLine` | `amount` は 10 進の文字列のまま、`covered` は `YYYY-MM-DD` の 2 つ、`charge_type`・`category` は列挙、`final` |
| `Mailer.Send` | unary `Send`。`MailMessage` をそのままと `envelope_from`（`mail_from`）→ `outcome`（`SENT`・`INVALID`・`RECIPIENT_REJECTED`・`REJECTED`）・`enhanced_status` |
| `Processor.Process`・`Remux` | 双方向ストリーム。§13 |
| `Processor.Inspect`・`InspectAV`・`Preview` | **委譲しない。** §13 |
| `KeyManager.CurrentKeyID()` | unary `Describe` を起動時に呼んで保持する。`WrapKey` の応答の `key_id` で更新する |
| `KeyManager.WrapKey`・`UnwrapKey` | unary。`bytes` の `dek`・`aad` など。§14 |

- **`BillingSource`:** スキーマの `source` の `CHECK (source IN ('aws', 'gcp'))`（`12-finance.sql` の 4 つの表）と、API の `BillingImport` の `method` の列挙に値を足す。`v0.1.0` の前なので DB を作り直せる。表示の名前は設定 `plugin_billing_provider` で、`SourceProvider` が使う
- **`Mailer`:** `Send` はジョブからだけ呼ばれるので、遅延は要求に届かない。adapter は呼ぶ前に `ValidateMail` を通す。`Message-ID`・`Date`・`Auto-Submitted` はプラグインが付ける（契約に書き、§16 で確かめる）
- **`media`:** `Limits`・`AVLimits` は `int64` と `double`、時間はミリ秒、`Format`・`AVKind` は列挙。拒否の `outcome` は `UNSUPPORTED_FORMAT`・`UNSUPPORTED_CODEC`・`TOO_LARGE`・`DIMENSIONS`・`CORRUPT`・`TOO_LONG` で、`media.Rejected` がそのまま効く
- **`translator`:** 断片・原語（`und` を含む）・訳語 → 訳文・検出した言語・`outcome`。形は ADR-0148 の翻訳のポートの決定に従う

### 12. 期限と上限

既定は次のとおりで、期限は `plugin_<kind>_timeout` で変えられる。`connect/v2` の既定のメッセージの上限は読み書きとも 4 MiB（`connecthttp/option.go`。2026-11-05 に動作でも確認。同じサイトの Deployment の文書は既定で無制限と書いていて、v2.0.2 の実装と合わない）なので、既定に頼らず kind ごとに必ず指定する。grpc-go のサーバーも既定の受信の上限が 4 MiB なので、`my` が送るメッセージは 4 MiB を超えない。

| kind | 1 回の期限 | 送るメッセージ | 受けるメッセージ | 主な呼び出し元 |
| --- | --- | --- | --- | --- |
| `key_manager` | 5 秒 | 128 KiB | 128 KiB | 要求の中とジョブ |
| `mailer` | 30 秒（`smtp.DefaultTimeout` と同じ） | 2 MiB | 64 KiB | ジョブ |
| `billing` | 2 分 | 64 KiB | 4 MiB | ジョブ |
| `translator` | 10 秒（ADR-0148 が変えてよい） | 1 MiB | 1 MiB | ADR-0148 |
| `media` | 4 分 | 2 MiB（チャンクは 1 MiB） | 2 MiB | ジョブ |
| フック | ADR-0147 | 1 MiB | 64 KiB | ジョブ |

- `key_manager` の 128 KiB は、包んだ鍵の上限 65,535 バイト（`kernel.Sealer` の見出しの 2 バイト）の base64 が JSON に収まる大きさ
- ジョブの `context` の期限はリースの終わりで、リースは 5 分（`queue.DefaultLease`、`runner.go`）。**ジョブから呼ぶ kind の期限はこれより短くなければならず**、設定の検査で 5 分以上を拒む
- 期限は `Connect-Timeout-Ms` でプラグインに届く（2026-11-05 に確認。gRPC は仕様の `grpc-timeout`）。手書きのプラグインは無視してもよく、`my` は期限で切る
- 受信の上限は展開の後の大きさにかかる（圧縮の爆弾に効く）。`my` は送信を圧縮しない
- 超過は `resource_exhausted` になる（2026-11-05 に確認）。§10 のとおり一時的な失敗として数え、繰り返せばジョブの再試行の上限で止まる
- 同時の呼び出しは kind ごとのセマフォで絞る。`media` の既定は 2（要検証: M6 の実測で調整）で、ほかは `MaxConnsPerHost` の 8 まで

### 13. メディアの本体

**本体はストリームのチャンクで運ぶ。** 画像は 16 MiB、動画と音声は 100 MiB まであり（ADR-0019 §8）、1 つのメッセージにも、grpc-go の既定の 4 MiB にも収まらない。

- **RPC:** `Process` と `Remux` を双方向ストリームにする。使い方は半二重で、`my` は `head`（操作の引数）とチャンクを送りきってから `CloseSend` し、プラグインは `head`（`outcome`・寸法・再生時間・blurhash）のあとに、出力ごとの `part`（どの出力か・形式・寸法・バイト数）とチャンクを返す
- **HTTP/2 が要る。** Connect の双方向は HTTP/2 だけで、HTTP/1.1 では送信が `EOF` で失敗する。したがって `media` は `proto` か `grpc` だけを受け、`json` は設定のエラーにする。TLS は ALPN の h2、Unix ソケットと平文の TCP は h2c
- **上限:** チャンクは 1 MiB。入力の合計は `Limits.MaxBytes`・`AVLimits.MaxBytes` で、ポートが呼ばれる時点で守られている。`my` は受信を出力ごとと全体で数え、`part` の宣言か次の上限を超えたら読むのをやめて `ErrTooLarge` にする: `Process` の全体は `Limits.MaxBytes` の 2 倍、`Remux` は本体が `AVLimits.MaxBytes`、カバーが `Limits.MaxBytes`
- **メモリ:** ポートは今も `data []byte` を受けて `[]byte` を返すので、ストリームにしても `my` のメモリは減らない。増えもしない。ポートを `io.Reader` にする変更は別の課題とする

検証（2026-11-05）: Unix ソケットの h2c で 100 MiB を 1 MiB のチャンクで往復し、mTLS の h2 で bearer 付きの 32 MiB を往復した。`context` の期限で `deadline_exceeded` になる。grpc-go へも双方向で届く。

**委譲するのは `Process` と `Remux` だけ** とし、`Inspect`・`InspectAV`・`Preview` は組み込みの `media/native` のままにする。

- `Inspect`・`InspectAV` はアップロードの要求の中で動き（`content/media.go`）、`Preview` はメディアプロキシの読み取りの経路で動く（`remote_media.go`）。どれも遅延を許す経路ではない
- 重い変換という #2220 の目的は `Process` と `Remux` で足りる。`Preview` はプラグインとの出力の違いが問題にならない小さな変種である
- adapter は `Options` で組み込みの `media.Processor` を受け、`internal/platform` が組み立てる。adapter 同士の import は要らない

**出力の検査（ADR-0019 §8）:** `media.Sniff`・`SniffAV` で形式を判定し、ポートが定める形式（`Still` の large は JPEG か PNG、small と medium は WebP など）と `part` の宣言の両方に合うこと、組み込みの `Inspect`・`InspectAV` が同じ `Limits` で通すこと、箱の寸法に収まることを確かめる。`ContentType` と `Extension` はプラグインの値を使わず判定した形式から決める。合わない出力は `ErrCorrupt`、大きすぎる出力は `ErrTooLarge` で、`media.Rejected` により組み込みの拒否と同じくメディアの失敗になる。運搬の失敗は組み込みの一時的な失敗と同じく再試行する。

### 14. `KeyManager` の委譲とフェイルクローズ

**なぜ例外として委譲するか。**

- Vault や HSM に KEK を置きたい運営者がいて、製品ごとの adapter を `my` に持たない。HSM の PKCS#11 は C のライブラリの読み込みを要し、`CGO_ENABLED=0` と両立しない（要検証）ので、別プロセスの方が筋がよい
- ポートは 3 メソッドで小さく、`gcpkms`・`awskms` がすでにネットワーク越しの呼び出しにしている。トランザクションの外で呼ぶことも、遅延の見積もりも、DEK のキャッシュ（ADR-0019 §13）も同じ形で使える
- プラグインは内容の可否を判定しない。誤動作の結果は「開けない」で、何かを通してしまうことにならない。信頼の点でも、平文の DEK を見るのはクラウドの KMS を使うときの事業者と同じ位置である。ただし、これを裏切るプラグインは全部の秘密を読める。**プラグインは KEK と同じ信頼の範囲に置く** と運用文書に書く

**フェイルクローズ。** どの失敗でも、平文で保存せず、`local` にも別の鍵にも切り替えず、開けないものは開けないまま失敗する。

| 場面 | 動作 |
| --- | --- |
| 起動時の到達不能 | 30 秒まで再試行して（サイドカーの起動順のため）、だめなら起動を失敗させる。メディアプロキシの鍵を起動時に開くので（ADR-0019 §12）、今の KMS の構成と同じ |
| 起動時の自己検査 | ランダムな DEK で `WrapKey` と `UnwrapKey` の往復、違う AAD の `UnwrapKey` が `DECRYPT` になること、2 回包んだ結果が違うこと、包んだ結果が DEK と違うことを確かめ、落ちれば起動しない。AAD を無視するプラグインや素通しのプラグインをここで止める |
| 実行中の失敗 | Connect のエラーも期限切れも、知らない `outcome` も、そのままエラーを返す。要求は失敗し、ジョブは再試行する。ブレーカーで「通す」動作は持たない |
| 応答の不正 | `OK` なのに `wrapped` が空・上限超過、`key_id` が空や上限超過は失敗とする |

- **鍵 ID:** `plugin:` + プラグインの ID で、全体で 255 バイトまで。`kernel.KeyIDKind` は `plugin` を返すので、`NewKeyRouter` の移行（ADR-0019 §14）がそのまま使える。`local` からの移行も `local` への移行も同じ手順で、ほかの KMS との直接の移行は作らない
- **`UnwrapKey` の結果:** `outcome` の `UNKNOWN_KEY` は `ErrUnknownKey`、`DECRYPT` は `ErrDecrypt` に写す
- **止まるもの**（#2221 の運用文書に書く）: キャッシュの有効期間（既定 10 分）を超える障害では `Sealer` を使うものが失敗する。連合の配送と署名付きの取得（アクターの秘密鍵）、新しいアカウントの作成、TOTP・パスキー・外部ログインの処理の一部、webhook の配送と登録、`my kek rewrap`、再起動（起動が失敗する）。連合の配送はジョブの再試行で追いつく
- **ログ:** DEK・AAD・`wrapped` は出さない。`slog.LogValuer` の `[REDACTED]` に従う

### 15. 失敗したときの共通の動作

| kind | プラグインが落ちているとき |
| --- | --- |
| `translator` | 翻訳が失敗として返る。投稿の表示は変わらない |
| `billing` | 組み込みの source の失敗と同じ。過去の取り込みは変わらず、`unavailable` はジョブが再試行する |
| `mailer` | 一時的な失敗として再試行する。メールが遅れる |
| `media` | 組み込みの一時的な失敗と同じで、処理が遅れる。カスタム絵文字の登録のように `Process` を同期で呼ぶ経路（`custom_emoji_admin.go`）は、その要求がエラーになる |
| `key_manager` | §14 |
| フック | ADR-0147 |

起動時に落ちていても、`key_manager` 以外は起動を止めない（縮退で済む kind が、起動の順序に縛られない）。失敗はメトリクスに kind と Connect の `code` で出す。

### 16. コントラクトテスト

- `pkg/testkit` は AGPL-3.0-only なので、プラグインの作者には import させない。黒箱の CLI `my plugin test --kind <kind> <address>` を渡す。作者は `my` のバイナリを 1 つ動かすだけで、何もリンクしない
- `testkit` は `testing` を import するので、`cmd/my` の本番のビルドに入れない。CLI の事例は `internal/adapter/plugin/contract` の表に置き、結果を事例ごとに pass / fail で出す
- `my` の adapter は従来どおり `testkit` のコントラクト（`MailerContract`・`BillingSourceContract`・`MediaProcessorContract` など）を、フェイクのプラグイン（`internal/adapter/plugin/fakeplugin`）に対して通す。フェイクは Connect のものと、`net/http` と JSON だけのものの 2 つ
- **CLI の副作用を明示する。** プラグインの先は本物のことがある
  - `mailer` は `--mail-to` を要求し、指定がなければ送る事例を飛ばす
  - `billing` は `max_requests = 0` の事例だけを既定にし、課金される事例は `--billed` で足す
  - `key_manager` は使い捨ての DEK だけを使う
  - `media` はテストの画像だけを送る。位置情報を含む画像が除去されて返ることも確かめる
- 接続の設定は `plugin_<kind>_*` と同じ意味のフラグで渡す

### 17. ライセンスと法務の確認（要検証）

プロプライエタリのプラグインを認める設計で実装を進める。法務の確認は実装の条件ではなく、**`v0.1.0` のタグの条件** である（#2032）。

**確認が済むまで、文書はライセンスについて何も述べない。** `README.md`・`docs/operations/`・`docs/reference/`・CLI の説明に、プロプライエタリのプラグインを作れること、プラグインに AGPL が及ばないことを書かない。`api/plugin/` の `LICENSE` と SPDX はそのファイルのライセンスの表示で、この制限の外である。

**次の法的な判断は誰も確認していない。** この節のどれも法的な助言や結論ではなく、法務のレビューが見るべき問いである。

1. **別プロセスは別の著作物か。** FSF の GPL FAQ（[MereAggregation](https://www.gnu.org/licenses/gpl-faq.html#MereAggregation)、[GPLPlugins](https://www.gnu.org/licenses/gpl-faq.html#GPLPlugins)。2026-11-05 に読んだ）は、境界は通信の仕組みと内容の両方で決まり、最後は裁判所の問いであるとし、複雑なデータ構造をやり取りすると別プロセスでも一体とみなされうると書く。FSF の見解は法ではない。`my` 専用の proto で投稿や鍵を渡すプラグインがどちらに当たるか
2. **MIT の proto が AGPL の Go のポートの写しである点。** proto は AGPL-3.0-only のインターフェイスの形をなぞる。それを MIT で出す権利の確認が要る。リポジトリに CLA はなく（`CONTRIBUTING.md` に記載なし）、貢献者の権利の確認の範囲が問題になる
3. **proto から生成したコードの扱い。** 作者の側の生成物と Connect のランタイム（Apache-2.0）の組み合わせに `my` の AGPL が及ばないこと
4. **AGPL §13。** `my` を改変した運営者が負うソースの提供に、改変版と組み合わせて動くプラグインが入るか。利用者はプラグインと直接やり取りしない場合の扱い（FSF FAQ の [AGPLv3InteractingRemotely](https://www.gnu.org/licenses/gpl-faq.html#AGPLv3InteractingRemotely) は定義の一部しか答えない）
5. **一緒に配るとき。** プラグインと `my` を同じコンテナイメージや Compose のファイルで配ることが集積物に当たる条件
6. **`my plugin test`。** AGPL の CLI を黒箱で動かすことがプラグインに影響しないこと。CLI の事例やフィクスチャを作者が自分のテストに写した場合
7. **文書の書き方。** 確認の後に、プロプライエタリのプラグインを認めることを運用文書にどう書くか。ライセンスの解釈を約束しない文にできるか

`media.check` のフックの法務は ADR-0147 が扱う。

### 18. 覆す条件

- §17 のレビューで別プロセスのプロプライエタリのプラグインが危ないとなったら、プラグインのライセンスの条件を変えるか、対象を AGPL と両立するプラグインに絞る
- `connect/v2` の保守が止まる、または Apache-2.0 でなくなったら、`v1` か gRPC の直接の実装へ移す。プロトコルの契約は proto の側にあるので、プラグインに影響しない
- HTTP/2 を通せない配置が `media` の主な使い方になったら、§13 を URL の受け渡しに変える
- `key_manager` の起動時の自己検査が、実在の KMS の費用や権限と合わなければ、`--skip` ではなく検査の内容を変える

## 経緯

運営者の環境に合わせた翻訳エンジン・費用の取り込み元・メールの事業者・重い変換・鍵の置き場を、`my` の本体に足さずに差し替えたい。判定フックも同じ輸送に乗せる。FR の番号はまだなく、エピック #2228 の範囲がこの ADR の起点である（関連: ADR-0002、ADR-0019、NFR-SEC-1、NFR-SEC-7）。

プロプライエタリのプラグインを認めること、DB とトランザクションのポートを委譲しないこと、マイルストーン External Plugins を `v0.1.0` の前に全部作ること、法務の確認をタグの条件にして先に実装することは決定者の判断である。全部を前に作るのは、後からの追加が互換の破れ目になるからで、エピックの順序は作る順として読む。#2214 の推奨（Connect、接続だけのモデル、設定だけの宛先、3 つの認証と、`key_manager`・`mailer`・本文を送るフックでの平文の TCP の拒否、MIT の `v1`、黒箱の CLI）を起点に、Go のインターフェイスと依存を読んで具体化した。

形が決まった転機は 3 つある。

- **`my` がクライアントであること。** 「1 つの proto で 3 つのプロトコル」はサーバーの性質なので、JSON だけのプラグインへ届くには `my` が JSON を送る必要があり、§3 の設定と既定になった
- **メディアの本体。** #2220 は blob のキーか署名付き URL で渡す案だった。ポートと経路を読むと §13 の採らなかった理由があり、ストリームにした
- **MIT の proto と生成物。** 検査のスクリプトと生成の動作を確かめると、見出しのコピーで検査が落ちることが分かり、§8 の 4 か所になった

確認した一次情報（2026-11-05）: `connectrpc.com/connect/v2` v2.0.2 の `LICENSE`（Apache-2.0）・`go.mod`（ビルドの依存は `google.golang.org/protobuf` だけ、`go-cmp` はテスト用）・ソースに `import "C"` なし、[Connect Protocol Reference](https://connectrpc.com/docs/protocol/)、[Deployment & h2c](https://connectrpc.com/docs/go/deployment/)、[Migrating to v2](https://connectrpc.com/docs/go/migrating-to-v2/)。`CGO_ENABLED=0` でスクラッチのプログラムをビルドして動かし、Connect の分のバイナリの増加は約 0.8 MiB（protobuf と `net/http` を含む比較対象との差。`cmd/my` への追加では未計測）。`Apache-2.0` は `GO_ALLOWED_LICENSES` にある。[Lambda の quotas](https://docs.aws.amazon.com/lambda/latest/dg/gettingstarted-limits.html) の同期の上限は要求・応答とも 6 MB。

## 採らなかった案

- **gRPC だけ:** 実装の選択肢が最も広く、ストリームも素直だが、HTTP/2 が必須で、JSON だけの小さなプラグインが書けない。プラグインを Lambda に置く構成とも相性が悪い（要検証: Function URL と API Gateway の HTTP/2 と trailer の扱いは未読）。`protocol = grpc` として選べるので、相手としては失っていない
- **HTTP + JSON の自前の仕様（OpenAPI）:** `api/spec/` と同じ道具で書け、依存が増えないが、ストリームと版の検査（`buf breaking`）を自分で作ることになり、gRPC のプラグインを受けられない
- **`connect` の v1:** 保守が続き（Migrating to v2 に明記）、実績も長いが、これから書くコードが移行を背負う。v2 の最初の版は 2026-10-07 と新しいことはリスクとして受ける
- **サブプロセスの管理（`hashicorp/go-plugin` の形）:** 配布が簡単で寿命を `my` が握れるが、別のマシンで動かせず、Lambda や Cloud Run の 1 つのコンテナにプロセスを同居させることになり（要検証）、バイナリの更新の責任が `my` に来る
- **Go の `plugin` パッケージや Wasm:** 通信の費用がなく同じプロセスで動くが、`plugin` は cgo とビルドの完全な一致を要し（要検証）、AGPL との結合の問題が最も重い。Wasm は Vault や別のマシンでの変換という目的に合わない
- **管理画面で宛先を設定:** 運営者には楽だが、管理者のアカウントの盗難が投稿と鍵の行き先の変更になる
- **`media` の本体を blob のキーや署名付き URL で渡す（#2220 の案）:** メッセージが小さく、プラグインが HTTP だけで動き、Lambda にも置ける。ただし次の理由で足りない
  - `blob.Signer` は読むだけの URL で書く URL がなく、出力の戻り道に新しいポートのメソッドが要る
  - `Signer` は `blob.Store` と別の任意のインターフェイスで、`s3` と `gcs` だけが実装し、`fs` にはない（2026-11-05 に確認）
  - `Remux` の入力は blob にあるが、リンクカードの画像（`link_card.go`）と埋め込みのカバー（`AVOutput.Cover`）の `Process` の入力は blob にないので、一時的な blob と掃除が要る
  - 利用者の未処理のアップロードの URL が外に出て、バケットへの到達性がプラグインの配置を縛る
- **`media` を 2 つの RPC に分けて HTTP/1.1 に乗せる（クライアントストリームで送り、ジョブ ID でサーバーストリームを受ける）:** HTTP/1.1 で動くが、プラグインに状態が生まれ、ロードバランサーの後ろで同じインスタンスに届く保証が要る
- **`Inspect`・`Preview` も委譲:** ポートをまるごと委譲できて単純だが、アップロードの要求と読み取りの経路にプラグインの遅延と障害が入る
- **すべての kind で平文の TCP を拒む（`billing` を除く）:** 投稿の本文とアップロードが暗号化されずに流れることがなくなり、規則も 1 つで済むが、閉じたネットワークの LibreTranslate や変換の機械に証明書の管理を負わせる。決定者は #2214 の範囲にとどめた
- **拒む kind でもループバックの平文の TCP を許す:** サイドカーの設定が簡単で、Unix ソケットの共有ボリュームが要らないが、§6 の理由で判定が曖昧になる
- **`Vault` などの組み込みの adapter:** `KeyManager` の委譲のリスクを避けられ、1 製品なら小さい。ただし製品ごとに依存とテストが増え、HSM には届かない（Vault Transit の API は要検証）
- **`KeyManager` を委譲しない:** 最も広い障害と信頼の問題がなくなるが、KMS 以外の置き場の要望に答えられない。平文の KEK を環境変数に置く `local` に戻る運営者が増えるほうが悪い
- **テストを `pkg/testkit` で配る:** 再利用が簡単だが AGPL-3.0-only で、リンクした作者に及ぶ。MIT に切り出すと、ポートの型に依存しているので大きな移動になる

## Consequences（影響）

- 良い影響
  - 翻訳・費用・メール・変換・鍵の差し替えが `my` の依存を増やさず、プラグインなしのサーバーは何も変わらない
  - 契約が proto にあるので、内部のインターフェイスの変更がプラグインを壊さない。`buf breaking` で機械的に止まる
  - `net/http` と JSON だけの小さなプラグインが書ける（`media` を除く）
- 悪い影響・トレードオフ
  - `v0.1.0` の前に kind を全部作るので、最初のリリースまでの作業が増える。翻訳のポート（#2230）が前提になり、リリースの順序に影響する
  - `media` は HTTP/2 と Connect か gRPC のライブラリを要し、JSON だけのプラグインにも Lambda の同期の呼び出し（6 MB）にも置けない
  - `my` はプラグインのメタデータの除去を完全には確かめられない。形式と寸法は検査するが、位置情報の除去は §16 の CLI の事例と運営者の責任になる（ADR-0019 §8 が前提とする再エンコードをプラグインに任せる）
  - `translator` と `media` と `signup.check` を平文の TCP で動かすと、投稿の本文（提供元に許せば `private`・`direct` も）、位置情報の残るアップロード、メールアドレスと IP がネットワークに暗号化されずに流れ、相手の認証もない。起動時の警告と運用文書で伝えるだけで、`my` は止めない
  - `key_manager` のプラグインは KEK と同じ信頼を要し、障害は §14 の範囲に広がる。誤って使うと連合上の同一性に関わる（NFR-REL-4）
  - Connect のエラーの `message` を捨てるので、プラグインの障害の調査はプラグインのログに頼る
  - 依存が増える: `connect/v2`（`cmd/my` に）、`buf`（`tools/buf/` に）。`connect/v2` は最初の版から 1 か月ほどで、変更に追随する可能性がある
- 後続で必要になる作業（Issue 化する）
  - #2216: §3〜§8・§12・§16 の基盤。`scripts/license-scope-check.sh`、`README.md`、`tools/agentkit/context-map.yaml`、`docs/reference/` と運用文書、`agentkit docs inventory` の `my plugin test` とビルドタグ
  - #2217〜#2221: §11・§13・§14 の写像。#2218 はスキーマと API の列挙の変更を含む
  - §17 の法務のレビューの手配（このプロジェクトの作業者にはできない）。#2032 のタグの条件
  - `Processor` のポートを `io.Reader` にしてメモリの上限を下げる（任意）

## Open Questions / 要検証

- HSM の PKCS#11 と `CGO_ENABLED=0` の関係、Vault Transit の API（未読。§14 の理由づけに使っている）
- Lambda の Function URL・API Gateway の HTTP/2 と trailer の扱い（未読）、Cloud Run のサイドカーの共有ボリュームと ECS Fargate のボリュームでの Unix ソケット（未読。§6 の平文の TCP を拒む kind をサイドカーに置くときの前提。M6 のテンプレートで確かめる）
- `buf breaking` の `--against` に Git のタグを渡す形が CI の浅いクローンで動くか（確かめたのはディレクトリとの比較だけ）
- `tools/buf/go.mod` を分けたときの `scripts/go-work-check.sh` と Dependabot の扱い（未確認）
- `media` の同時実行の既定と 100 MiB のときのメモリの実測（スクラッチのテストの速度は同じプロセスの中で、本番の参考にならない）
- `connect/v2` の Deployment の文書と実装の既定の上限の食い違い（§12。実装を信じて常に指定する）
