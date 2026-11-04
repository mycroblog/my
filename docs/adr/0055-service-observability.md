# ADR-0055: メトリクスとトレース

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #645, #646, #814, #919

## Decision（決定）

計測は自前の `Meter` port と `Tracer` port に OpenTelemetry Go SDK のアダプタを付けて行う。
メトリクスのエクスポートの既定は `none` で、ターゲットごとに Prometheus（pull）・OTLP（push）・EMF（ログ）から選ぶ。
スケールの契機は基盤の指標を優先し、足りない分だけアプリが出す。基盤が資源（CPU・メモリ・台数・入口のリクエスト）を測り、アプリはプロセスの中の飽和を出す。
出来事の数はプロセスごとに持ってバックエンドで集計し、インスタンス全体の値（キューの滞留・利用者数・メディアの容量）は KV に置く 1 つのスナップショットを全プロセスで共有する。

トレースの既定（`traces_exporter`）も `none` で、有効にするときのエクスポートは OTLP/HTTP だけにする。ジョブの実行は新しいトレースの根にして、enqueue した span へ link する。
サーバーは OTLP の受け手の認証を持たず、Cloud Run と Lambda ではコレクター（Cloud Run のサイドカー、ADOT の Lambda のレイヤー）を経由して Cloud Trace・X-Ray へ送る。
`none` でもログには基盤から受けた trace ID を付けて、基盤のトレースと相関させる。

### 1. 計測の port（ADR-0002 の依存の向き）

- `internal/app/kernel` に `Meter` port を置く。持つのは Counter・UpDownCounter・Histogram・非同期の Gauge（収集のときに呼ぶ関数）・
  非同期の observable counter（収集のときに呼ぶ関数が、プロセスの CPU 時間やコネクションプールの待ちのように別の場所で持つ累積を返す。値はプロセスの再起動まで減らない）を作る口だけで、
  型は標準ライブラリだけで書く。OTel の型は port に出さない
- 計器（instrument）は、名前・単位・説明・**許すラベルのキーの一覧** を宣言してから使う。宣言にないラベルのキーを渡すのはプログラムの誤りとして、テストで落とす
- 実装はアダプタ `internal/adapter/telemetry/` に置く:
  - `none`（既定）: 何もしない。計測の呼び出しの費用はほぼない
  - `otel`: OpenTelemetry Go SDK（`go.opentelemetry.io/otel/sdk/metric`）に渡し、OTel のエクスポーター（Prometheus・OTLP）で出す
  - `emf`: 1 つのリクエスト・ジョブの終わりに、そこで記録した値を EMF の JSON 1 行として標準出力に書く（集計は CloudWatch が行う）
- observable counter は、Prometheus では `_total` の counter、OTLP では単調増加（monotonic）の Sum として出す。`emf` は収集を持たないので、書くときに前回書いてからの増分を書く
- 計測の多くは境界に置く: HTTP のミドルウェア（`internal/platform`）、ジョブの実行（`internal/app/queue` の runner が port を呼ぶ）、
  外向きの配送（`internal/app/federation` の配送のジョブ）、ストリームの接続（`internal/interface/clientapi`）。ドメイン（`internal/domain`）は計測しない
- システムの指標（§10）も同じ port とアダプタで出す

### 2. ターゲットごとのエクスポートの経路

設定は `metrics_exporter`（`none` | `prometheus` | `otlp` | `emf`）。既定は `none`。

| ターゲット | 設定 | 経路 | 追加の固定費 |
| --- | --- | --- | --- |
| Self-hosted（Docker Compose / microk8s） | `prometheus` | 公開のリスナーとは別のアドレス `metrics_listen`（既定 `127.0.0.1:9464`）で `/metrics` を出し、Prometheus 互換のサーバー（Prometheus・VictoriaMetrics など）が scrape する。KEDA はそこを読む | Prometheus 互換のサーバーの分だけ（運用者が選ぶ。M6 で計測） |
| Google Cloud（Cloud Run） | `none` | 基盤の指標（リクエスト数・遅延・インスタンス数、Cloud Tasks のキューの深さ）と、構造化ログ（§5）から作る **ログベースの指標** をデプロイのテンプレートが定義する | ログベースの指標の課金（要検証）。インスタンスベース課金にするなら `otlp` + コレクターのサイドカーも選べる |
| AWS（Lambda） | `emf` | 標準出力の EMF を CloudWatch Logs が取り出す。キューの深さと滞留時間は SQS の標準の指標（`ApproximateNumberOfMessagesVisible`・`ApproximateAgeOfOldestMessage`）を使う | カスタムメトリクスとログの取り込みの課金（要検証） |
| 任意（コレクターを動かせる構成） | `otlp` | OTLP/HTTP で OpenTelemetry Collector へ送る。宛先は OTel の標準の環境変数 `OTEL_EXPORTER_OTLP_ENDPOINT` などで設定する | コレクターの分 |

- `/metrics` は公開のリスナーには出さない。GoToSocial と同じく別のアドレスにし、認証は持たない（外に出すなら運用者がネットワークかプロキシで守る）
- OTLP は HTTP（protobuf）を使い、gRPC のエクスポーターは使わない（依存を小さくするため。gRPC は Google Cloud の SDK の間接依存として既にあるが、選ぶ理由がない）
- `emf` の名前空間は設定 `metrics_emf_namespace`（既定 `my`）。次元は §6 のラベルのうち、デプロイのテンプレートが選ぶ組み合わせだけにする（次元の組み合わせごとに課金されるため）

### 3. 保証するサービスの指標

名前は OTel の意味規約（semantic conventions）があるものはそれに従い、ないものは `my.` で始める。Prometheus では `.` が `_` になる。
「保証する」とは、`metrics_exporter` が `none` 以外のとき、どのターゲット・どの役割（`role`）でも、該当する処理をするプロセスがこの名前と単位で出すことをいう。

| 指標 | 種類・単位 | ラベル | 用途 |
| --- | --- | --- | --- |
| `http.server.request.duration` | Histogram・秒 | `http.request.method`、`http.route`（`ServeMux` のパターン。一致しないものは `other`）、`http.response.status_code`、`my.surface`（`client_api` / `activitypub` / `oauth` / `web` / `media` / `admin` / `payments`） | クライアント API・Inbox・Web の件数・遅延・エラー率 |
| `http.server.active_requests` | UpDownCounter・{request} | `my.surface` | 処理中のリクエスト数（Self-hosted の web の契機） |
| `my.inbox.activities` | Counter・{activity} | `activity.type`（既知の Activity の型、それ以外は `other`）、`my.outcome`（`accepted` / `duplicate` / `rejected` / `ignored` / `error`） | 連合の受信の件数と拒否の割合（ADR-0006） |
| `my.delivery.attempts` | Counter・{attempt} | `my.outcome`（`success` / `retry` / `gone` / `failed`） | 配送の成功率 |
| `my.delivery.duration` | Histogram・秒 | `my.outcome` | 配送の 1 回の HTTP の遅延 |
| `my.job.duration` | Histogram・秒 | `my.job.type`（登録済みのジョブの型）、`my.outcome`（`success` / `retry` / `dead`） | ジョブの処理時間と失敗 |
| `my.job.wait` | Histogram・秒 | `my.job.type` | 実行可能になってから処理が始まるまでの待ち（キューの遅延） |
| `my.queue.ready` | Gauge・{job} | `my.job.type` | 実行可能なジョブの数（DB キューだけ。§4・§7） |
| `my.queue.oldest_ready_age` | Gauge・秒 | `my.job.type`（ADR-0004 §17） | 最も古い実行可能なジョブの待ち時間（DB キューだけ。§4・§7） |
| `my.queue.dead` | Gauge・{job} | なし | 失敗して止まったジョブの数（NFR-OPS-3 と同じ値） |
| `my.timeline.delivery.lag` | Histogram・秒 | なし | 投稿の作成からホームタイムラインへの書き込み・ストリームへの publish までの遅延 |
| `my.stream.connections` | UpDownCounter・{connection} | `my.stream.kind`（ストリームの種類） | ストリーム（SSE）の接続数 |
| `my.accounts.registered`・`my.statuses.created` | Counter・{account}・{status} | `my.statuses.created` だけ `my.visibility`（ADR-0017 の 5 値） | 登録・投稿の業務の数 |
| `my.instance.users`・`my.instance.statuses` | Gauge | なし | ローカルの利用者数・投稿数（NodeInfo の usage と同じ値。§4） |
| `my.media.stored_bytes` | Gauge・バイト | `my.media.origin`（`local` / `remote_cache`） | メディアの容量。`local` はローカルのアップロードの現在の派生（`media_variants`）と処理待ちの原本（`uploaded_media`）の `file_size` の合計、`remote_cache` はリモートのメディアのキャッシュ（`remote_media_cache`）の合計（§4） |
| `my.errors.reported` | Counter | `my.error.source`・`my.error.kind` | 報告したエラーの数（ADR-0103 §8） |

- `my.surface = payments` は、決済の事業者が呼ぶ webhook の受け口 `POST /payments/webhooks/{provider}` の要求に付ける（ADR-0074 §3）
- DB から読む Gauge（`my.queue.*`・`my.instance.*`・`my.media.stored_bytes`）はインスタンス全体の値で、§4 のスナップショットから出す
- `emf` では、リクエストとジョブの終わりに書ける Counter・Histogram と、§4 のスナップショットの行だけを出す。プロセスごとの UpDownCounter（処理中のリクエスト・ストリームの接続）は基盤の指標（Lambda の同時実行数）で代える

### 4. 複数のプロセスでの値の集計と共有

web・worker を分けたり、レプリカ・サーバーレスのインスタンスが増減したりすると、1 つのインスタンスの値を複数のプロセスが持つ。値を 3 種に分け、それぞれの集計の場所を決める。

| 種類 | 指標 | 値の持ち主 | 集計の場所 |
| --- | --- | --- | --- |
| 出来事の数・分布 | Counter・Histogram（`http.server.request.duration`、`my.job.*`、`my.delivery.*`、`my.inbox.activities`、`my.accounts.registered` など） | 各プロセスの SDK のメモリ（起動からの累積） | バックエンド（PromQL の `sum(rate(...))`、Cloud Monitoring・CloudWatch のログベースの指標・EMF の集計） |
| プロセスの今の値 | UpDownCounter（`http.server.active_requests`、`my.stream.connections`） | 各プロセス | バックエンドで `sum`（Pod ごとの平均は `avg`） |
| インスタンス全体の今の値 | DB から読む Gauge（`my.queue.*`、`my.instance.*`、`my.media.stored_bytes`） | **スナップショット**（KV に 1 つ） | 計算は 1 回、各プロセスは同じ値を出す。バックエンドでは `max` で 1 つにする |

出来事の数は KV にも DB にも書かない。リクエスト・ジョブのたびに KV（既定の `kv = db` では DB の行）を書くと、DB の書き込みが利用量に比例して増え、Neon・Aurora の scale to zero を妨げる。
プロセスの再起動で累積が 0 に戻るのは、Prometheus の `rate` と OTel の累積の扱いが吸収する。業務の総数（利用者数・投稿数・容量）が要るときは、出来事の数を足し合わせず、スナップショットの Gauge を見る。

**スナップショット**（インスタンス全体の Gauge）:

- 計算する関数は 1 つで、DB に問い合わせて全部の値と計算した時刻を返す。結果は `kernel.KeyValueStore` の名前空間 `metrics` に 1 件で置き、TTL を付ける
- 各プロセスは収集（scrape・エクスポート）のとき、KV のスナップショットを読んで Gauge を出す。KV の読み取りはプロセスの中で 15 秒キャッシュする。DB には直接問い合わせない
- スナップショットを作り直すのは次の 2 つだけ:
  - **収集のとき古ければ**: KV の値が鮮度の上限（キューの値は 30 秒、利用者数・投稿数・容量は 15 分）を過ぎていたら、KV の `IncrBy` で TTL 付きのロックを取ったプロセスだけが計算し直して書く。
    取れなかったプロセスは古い値を出す。worker が 0 レプリカでも、scrape される web がキューの値を作れる（KEDA が worker を 0 から起こせる）
  - **定期ジョブ `metrics.snapshot`**（15 分ごと。`metrics_exporter` が `none` 以外のときだけ登録する）: 計算して KV に書き、同じ値を 1 行の構造化ログ（`emf` では EMF）に出す。
    scrape のない Cloud Run と Lambda では、この行がインスタンス全体の値の唯一の経路になる（ログベースの指標・EMF の Gauge）
- 値の鮮度はスナップショットの時刻で表し、`my.metrics.snapshot.age`（Gauge・秒）として出す。KV に値がない（期限切れ・まだない）ときは、ロックを取ったプロセスが計算する。取れなかったプロセスはその回の Gauge を出さない（0 と誤認させない）
- 整合性: 値は最大で鮮度の上限だけ古い。同じ時刻には全プロセスが同じスナップショットを出すので、プロセスの間で値が食い違わない。
  ロックの取り損ねと重複の計算は、同じ値を上書きするだけで害がない（冪等）
- `kv = memory` は KV がプロセスごとなので、各プロセスが自分で計算する。`kv = memory` は 1 プロセスの構成でだけ使う前提なので、これを許す

ターゲットごとの費用:

| ターゲット | スナップショットの経路 | 費用 |
| --- | --- | --- |
| Self-hosted（`kv = db`・`valkey`） | scrape のときの KV の読み取り（プロセスごとに 15 秒に 1 回）と、30 秒・15 分に 1 回の計算 | KV の読み取りは主キーの 1 行。計算は索引のある件数・最古の時刻と、15 分に 1 回の `file_size` の合計 |
| Cloud Run・Lambda | 定期ジョブ `metrics.snapshot` だけ（scrape がない） | 15 分に 1 回のジョブ（既存の定期ジョブのトリガーから enqueue するので、Cloud Scheduler・EventBridge Scheduler のジョブは増えない）とログ 1 行。DB を 15 分に 1 回起こす |

### 5. ログとの関係（NFR-OPS-1）

- HTTP のリクエストとジョブは、終わりに 1 行の構造化ログを出す（アクセスログを含む）。そのログは §3 の Histogram と Counter のラベルと値（`http.route`、`http.response.status_code`、`duration_seconds`、`my.job.type`、`my.outcome`、`wait_seconds`）を同じ名前で持つ
- Cloud Run ではこのログからログベースの指標（分布の指標を含む）を作る。`emf` はこの 1 行に `_aws` の指示を足したものにする。どちらもプロセスの中で集計しないので、リクエストベース課金の CPU の制限と、Lambda の凍結の影響を受けない

### 6. ラベルの規則（濃度と個人情報）

- ラベルの値は、コードで決まった有限の集合（ルートのパターン、ジョブの型、Activity の型、公開範囲、結果）だけにする
- 次のものはラベルにしない: 連合先のドメイン・ホスト、アカウント・投稿・ジョブの ID、URL・パス（パターンでない生の値）、クライアント IP、User-Agent、OAuth のアプリケーション
- 連合先ごとの状態（配送の失敗の続き、到達できないホスト）はメトリクスでなく DB と管理画面（NFR-OPS-3、[federation.md](../architecture/federation.md)）で見る
- 資源の属性（OTel の resource）は `service.name`（既定 `my`）、`service.version`、`my.role`（`web` / `worker` / `all`）と、基盤が与えるインスタンスの識別子だけにする

### 7. オートスケーリングの契機

| ターゲット | web の契機 | worker の契機 | アプリの指標の役割 |
| --- | --- | --- | --- |
| Cloud Run | 基盤のリクエストの同時実行数（と CPU） | Cloud Tasks の push がリクエストになるので、同じく同時実行数 | 監視とアラートだけ |
| AWS（Lambda） | 基盤の同時実行 | SQS のイベントソースの poller の自動の増減 | 監視とアラートだけ |
| microk8s（PostgreSQL・複数レプリカ） | HPA: `http.server.active_requests` の 1 Pod あたりの平均（KEDA の Prometheus スケーラー）。CPU の併用は §11 | KEDA の Prometheus スケーラー: `my.queue.oldest_ready_age` と `my.queue.ready` の合計 | 伸縮の契機 |
| Docker Compose・SQLite | 伸縮しない（SQLite はレプリカ 1 固定） | 同左 | 監視だけ |

- DB キューの `my.queue.*` はインスタンス全体の値で、どのプロセスが出しても同じになる。PromQL では `max` で 1 つにする。
  KEDA の PostgreSQL スケーラーでジョブの表を直接数える方法はスキーマに結びつくので採らない
- 具体的なしきい値は M6 のデプロイのテンプレートで決め、[deployment-targets.md](../architecture/deployment-targets.md) に書く

### 8. SLI と SLO

- SLI は §3 の指標で定義する: クライアント API の可用性（5xx でない割合）と遅延（ストリームを除く `client_api` の p95）、Inbox の受理の可用性、配送の成功率（`success` / (`success` + `failed`)）、ジョブの待ち（`my.job.wait` の p95）、タイムラインの配信の遅延
- SLO の目標値はサーバーには持たせず、デプロイのテンプレートのアラートの既定値として M6 で決める（運用者が上書きする）

### 9. トレース

NFR-OPS-2 の「任意で有効化」は、メトリクスは §2 の `otlp`、トレースは §13〜§19 で実現する。メトリクスとトレースは同じ OTel の SDK を共有する。

### 10. システムの指標: 基盤が測るもの、アプリが測るもの

| 種類 | 測る者 | 指標 |
| --- | --- | --- |
| コンテナの CPU・メモリ・台数・入口のリクエスト数と遅延 | 基盤（`経緯` の表） | アプリは重ねて出さない。Docker Compose と Lambda のメモリだけは、下のプロセスの指標で補う |
| プロセス | アプリ（`internal/platform`） | CPU 時間、常駐メモリ（RSS）、開いたファイル記述子の数と上限（Linux のみ）、起動時刻、役割・版の情報（`role`、ビルドの版） |
| Go のランタイム | アプリ（`internal/platform`） | OTel の意味規約の `go.memory.used`・`go.memory.limit`・`go.memory.allocated`・`go.memory.gc.goal`・`go.goroutine.count`・`go.processor.limit`・`go.config.gogc`・`go.schedule.duration`。GC の回数と停止時間（`go.memory.gc.cycles`・`go.memory.gc.pause.duration`）も出す |
| DB の接続のプール | DB アダプタ | `db.client.connection.count`（`state` = `idle` / `used`）・`db.client.connection.max`・`db.client.connection.pending_requests`・`db.client.connection.wait_time`・`db.client.connection.timeouts`。PostgreSQL は `pgxpool.Pool.Stat()`、SQLite は `sql.DB.Stats()` を観測のコールバックで読み、クエリごとの計測は足さない |
| キュー・KV・ストリーム・Blob のアダプタ | 各アダプタ（port の実装の境界） | 操作ごとの所要時間のヒストグラムと失敗の数（ラベルは `adapter`・`operation`・`outcome`）。常時の接続を持つもの（valkey-go、`LISTEN` の接続、SQS のロングポーリング、Pub/Sub の受信）は接続の数と再接続の数 |
| 外向き HTTP | `internal/adapter/httpclient` | `http.client.request.duration`（ラベルは用途 `HTTPPurpose`・メソッド・状態コードの区分・SSRF ガードによる拒否か）と `http.client.open_connections` |

- 指標の名前と単位は OpenTelemetry の意味規約に合わせる。runtime・DB・HTTP クライアントの規約の多くは Development の段階なので、規約が変わったら追う
  （[Go のランタイム](https://opentelemetry.io/docs/specs/semconv/runtime/go-metrics/)、[DB](https://opentelemetry.io/docs/specs/semconv/database/database-metrics/)、
  [HTTP](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/)）。独自の指標の接頭辞と命名は §3 に従う
- ラベルに相手のホスト・ドメイン・URL・アカウント・Blob のキーを入れない（相手のドメインごとの配送の状態はサービスの指標と管理画面、NFR-OPS-3 で扱う）。
  プロセスの識別（インスタンス ID・役割・版）はラベルでなく、§6 の資源の属性で行う
- 計測するのはアダプタと `internal/platform` だけで、`internal/domain` と `internal/app` はシステムの指標を知らない。アダプタは §1 の port で計測する
- Go のランタイムの指標は `go.opentelemetry.io/contrib/instrumentation/runtime`（Apache-2.0、意味規約の `go.*` を出す、[pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime)）で出す
- 有効化・エクスポートの経路・送る間隔は §2 の設定に従い、システムの指標だけを別に有効にする設定は作らない。無効のときは no-op の実装で、観測のコールバックも登録しない

### 11. スケールの契機（役割ごと）

アプリが出すシステムの指標は **スケールの契機にしない**。診断と警報（飽和の検出）と、台数の上限の見積もりに使う。

| ターゲット | web（`role = web`） | worker（`role = worker`） |
| --- | --- | --- |
| Cloud Run | CPU 使用率と同時リクエスト数（基盤の既定）。SSE のストリームは同時リクエスト数を占めるので、最大同時実行数はストリームの本数を見込んで決める（M6） | Cloud Tasks の push を受けるサービスとして、同じく CPU と同時リクエスト数。キューの配送の速さは Cloud Tasks のキューのレート制御で抑える |
| Lambda | 基盤の同時実行（指標の設定なし）。予約同時実行数で上限を決める | SQS のイベントソース（基盤）。最大同時実行数で上限を決める |
| ECS | `ECSServiceAverageCPUUtilization` の目標追跡。ALB を使うなら `ALBRequestCountPerTarget` を足してよい | pull のワーカーなので、キューの滞留（サービスの指標、§3）を CloudWatch の指標にして主にし、CPU を従にする |
| Kubernetes | HPA の CPU 使用率（metrics-server。コンテナに CPU の requests を必ず書く） | KEDA でキューの滞留（DB キューなら PostgreSQL スケーラーのクエリ、またはサービスの指標、§3）を主にし、CPU を従にする |
| Docker Compose / SQLite | スケールしない（`role = all` の 1 台） | 同左 |

- **メモリはスケールの契機にしない**。Go のヒープは負荷が下がってもすぐには返らず、台数を増やしても 1 台のメモリは減らない。Cloud Run はメモリでスケールしない。
  代わりに `go.memory.used` と `go.memory.limit` の比で警報を出す
- **`GOMEMLIMIT` は運営者が設定できる項目の一つ** とする（Go のランタイムの環境変数）。アプリは cgroup のメモリの上限から自動では決めず、設定がなければ Go の既定（上限なし）のまま動く。
  デプロイのテンプレート（M6）は設定できる値として置き、目安（コンテナのメモリの上限の約 90%）を添える
- **DB の接続は台数の上限を決める**。web と worker の台数の最大 × プールの最大接続数が DB の接続の上限（Neon・Aurora・Cloud SQL・PgBouncer）を超えないように、
  デプロイのテンプレートで台数の最大を決める。`db.client.connection.pending_requests` と `wait_time` は、上限に当たっていることの警報に使う
- `GOMAXPROCS` は Go 1.25 以降の既定（cgroup の CPU の上限に従う）に任せ、テンプレートで設定しない
- スケールの契機に使うキューの滞留・遅延の指標の定義は §3 が持つ

### 12. 死活・準備の検査（FR-802）との関係

- `/healthz` と `/readyz` は指標の値を見ない。プールの待ちや goroutine の数で readiness を落とすと、全台が一斉に外れて負荷が残りの台に寄るので、閾値での切り離しはしない
- readiness の失敗（DB に届かない）はログに残し、失敗の数をシステムの指標として出す
- pull のエンドポイントは §2 の `/metrics` で、worker の役割でも出し、ヘルスチェックの経路とは別のパスにする

### 13. トレースの port（ADR-0002 の依存の向き）

- `internal/app/kernel` に `Tracer` port を置く。型は標準ライブラリだけで書き、OTel の型を出さない:
  - `Start(ctx, SpanSpec, Attrs) (context.Context, Span)`・`Span.Set(key, value)`・`Span.SetName(name)`（ルートが決まってから HTTP の span の名前を付ける）・`Span.End(errorType string)`
  - `Inject(ctx) string`（現在の span の W3C `traceparent`。記録していなければ空）と `Linked(ctx, traceparent) context.Context`（次の `Start` に link を足す）、`Continue(ctx, traceparent) context.Context`（信頼したプロキシから受けた `traceparent` を次の `Start` の親にする。§15）
  - `LogIDs(ctx) (traceID, spanID string)`（ログの相関用。§18）
- span は §1 の計器と同じく、名前・種類（server / client / internal / consumer / producer）・**許す属性のキーの一覧** を `SpanSpec` で宣言してから使う。
  宣言にない属性のキーはアダプタが捨て、`testkit` の Tracer がテストを落とす
- アダプタは `internal/adapter/telemetry/` に置く:
  - `none`（既定）: span を記録しない。ただし基盤から受けた `traceparent` があれば、その trace ID を `LogIDs` で返す（§18）
  - `otel`: OTel Go SDK の `sdk/trace` と OTLP/HTTP のエクスポーター（`otlptracehttp`）。resource は §6 と同じ（`service.name`・`service.version`・`my.role`）
- span を作る場所は境界とアダプタに限る。ドメイン（`internal/domain`）は計測しない。§1 で計測のある場所と同じ:
  - HTTP のサーバー: `internal/platform` のミドルウェア（`accessLog` と同じ場所）。名前は `<method> <http.route>`
  - ジョブの実行: `internal/app/queue` の runner（`Meter.Begin` と同じ場所）。名前は `my.job.type`
  - 外向きの HTTP: `internal/adapter/httpclient`（`kernel.HTTPClient`）。連合の取得・配送、WebFinger、メディアのプロキシ、リンクのカード、Web Push
  - DB: `UnitOfWork.Tx` の 1 回の実行（再試行ごと）と、sqlc の 1 クエリ。PostgreSQL は pgx の `QueryTracer`、SQLite・libSQL は sqlc の `DBTX` を包むラッパーで作る（新しい依存なし）
  - Inbox の受理・配送の 1 回・ストリームの publish（`internal/app/federation`・`stream`）は、必要なら port で internal の span を足す

### 14. ジョブへの引き継ぎ（新しいトレース + link）

- `queue.Job`・`queue.NewJob` は `TraceParent`（W3C `traceparent` の文字列、空も可）を持つ。`JobQueue.Enqueue` と `Outbox.EnqueueTx` は、呼び出しの `ctx` から `Tracer.Inject` で埋める（呼び出し側は何もしない）
- 保存の場所: 表 `jobs`・`outbox_events`・`dead_jobs` の列 `trace_parent text`（null 許容。ADR-0023 の `schema/<dialect>.sql`）。
  これは状態でなくジョブの属性で、ADR-0020 の「状態を null 許容の時刻で表さない」に当たらない。outbox のリレーは列をそのまま次のキューへ渡す。
  Cloud Tasks・SQS の合図は `traceparent` を運ばない。ジョブの行にあるので、伝播は DB の列で足りる（ADR-0004 §9）
- runner はジョブの実行の span（種類 consumer）を **新しいトレースの根** として作り、`TraceParent` への link を持たせる。再試行は試行ごとに別の span で、属性 `my.job.attempt` を持つ
- サンプリングは link に従う: link 先が sampled ならジョブも記録し、そうでなければ §17 の比率で決める（OTel の `Sampler` は link を見られる）。これで記録したリクエストの続きのジョブは欠けない

### 15. 伝播の境界（信頼と漏れ）

| 境界 | 受け取る `traceparent` | 送る `traceparent` |
| --- | --- | --- |
| 公開のリスナー（クライアント API・Web・OAuth・Inbox・メディア） | 接続の相手が `trusted_proxies`（ADR-0019 §5）のときだけ親として使う。それ以外は捨てて新しいトレースを始める（W3C §3.4 の restart）。`tracestate` は常に捨てる | 応答に付けない |
| キューの push の受け口（Cloud Tasks の HTTP の受け口、Lambda の SQS のイベント） | 受け取らない。ジョブの行の `TraceParent` を link に使う（§14） | — |
| `kernel.HTTPClient`（連合・WebFinger・メディア・リンクのカード・Web Push） | — | **付けない**。リモートのサーバーに自分の trace ID・サンプリングの判断を渡さない（W3C §7.1）。client の span はプロセスの中だけで作る |
| 運用者が設定したクラウドの API（S3・GCS・Pub/Sub など） | — | 付けない（SDK の計測は入れない。後で必要になれば再検討する） |

- 受け取った sampled フラグにはそのまま従わず、§17 の親に従う sampler で判断する。信頼しない入口で restart するので、外から sampled を付けて記録の量を増やされない（W3C §7.2）

### 16. 属性の規則（個人情報）

§6 のラベルの規則を span の属性にも当てはめ、トレースは時系列でないので濃度の制限だけを次のように緩める:

- 許す: HTTP の `http.request.method`・`http.route`（パターン）・`http.response.status_code`・`my.surface`、ジョブの `my.job.type`・`my.job.attempt`・`my.outcome`、
  `activity.type`、DB の `db.system.name`・`db.operation.name`（sqlc のクエリ名）、`error.type`（エラーの種類の有限の名前）、
  外向きの HTTP の `server.address`（宛先のホスト名だけ。運用者が配送の失敗を調べるため）と `http.response.status_code`
- 入れない: アカウント・投稿・ジョブの ID、ユーザー名、URL の全体・パス・クエリ（`url.full`・`url.path`。Actor の URL はユーザー名を含む）、クライアント IP、User-Agent、
  OAuth のアプリケーション、SQL の本文とパラメーター（`db.query.text`）、エラーの文字列（URL・ユーザー名を含みうるので `error.type` だけにする）、リクエスト・応答の本文
- span のイベント（`AddEvent`）と例外の記録（`RecordError`）は port に出さない。ジョブ ID・リクエスト ID との対応は §18 のログで取る
- `Tracer` の `SpanSpec` の許可リストと、`testkit` のテストでこの規則を強制する

### 17. トレースのエクスポートの経路とサンプリング（NFR-COST）

設定（`internal/platform/config`、[configuration.md](../reference/configuration.md) を再生成）:

- `traces_exporter`: `none`（既定）| `otlp`。`otlp` は OTLP/HTTP（protobuf、gzip）で、宛先は OTel の標準の環境変数 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` などで決める（§2 の `metrics_exporter = otlp` と同じ流儀）
- `traces_sample_ratio`: 根の span を記録する割合（既定 0.01）。sampler は「親（信頼した入口の親、ジョブの link）に従い、親がなければ trace ID の比率」
- `traces_flush`: `batch`（既定。SDK の BatchSpanProcessor が周期的に送る）| `unit`（HTTP のリクエスト・ジョブの終わりに、そこで記録した span があれば送り切ってから返す）
- サーバーは OTLP の受け手の認証（Google の Application Default Credentials、AWS の SigV4）を持たない。Cloud Trace・X-Ray の受け口が求める認証は、ターゲットのコレクターが行う

| ターゲット | 設定 | 経路 | 費用 |
| --- | --- | --- | --- |
| 既定・小さな Self-hosted | `none` | ログの `request_id`・`job_id`・`trace_id`（§18） | 増えない |
| Self-hosted（Collector・Jaeger・Tempo などを運用者が動かす） | `otlp`・`batch` | OTLP/HTTP で受け手へ。tail sampling（エラーと遅いトレースを残す）は Collector で行える | 受け手の分 |
| Google Cloud（Cloud Run） | `none`、またはトレースを有効にするなら `otlp`・`unit` | `none` は Cloud Run の自動のトレース（無料）とログの相関。`otlp` は同じインスタンスの **OpenTelemetry Collector のサイドカー**（`localhost:4318`）へ送り、サイドカーが ADC で `telemetry.googleapis.com` へ送る | サイドカーの資源と span の量（要検証）。サイドカーが CPU の割り当てのない間も送れるよう、トレースを有効にする構成はインスタンスベース課金にする（`経緯` の Cloud Run の確認と同じ理由） |
| AWS（Lambda） | `none`、またはトレースを有効にするなら `otlp`・`unit` | **ADOT の Lambda のレイヤー**（Collector の拡張機能、`localhost:4318`）へ送り、レイヤーが SigV4 で `xray.<region>.amazonaws.com/v1/traces` へ送る | レイヤーの分の起動時間と、記録したトレース 100,000 件/月まで無料、以降は従量（要検証） |

- リクエストベース課金の Cloud Run と、凍結される Lambda では、BatchSpanProcessor の周期的な送信を当てにできない（`経緯` の Cloud Run の確認と同じ理由）。そこで `unit` でローカルのコレクターへ送り切る。
  送り先はループバックなので遅れは小さく、記録しないリクエスト（既定で 99%）は送信しない
- 既定の比率 0.01 は、1 日 10 万リクエストのサーバーで根のトレースが 1 日約 1,000 件（月約 3 万件）になり、X-Ray の無料枠に収まる見込みで選ぶ（span の数はトレースあたり十数個。要検証: M6 で実測）

### 18. ログとの相関（NFR-OPS-1）

- slog のハンドラー（`requestIDHandler`）は、`Tracer.LogIDs` が返す `trace_id`・`span_id` もログの全行に付ける。`request_id`・`job_id` も残す
- `traces_exporter = none` でも、信頼した入口（§15）の `traceparent` の trace ID をログに付ける。Cloud Run では自動のトレースとログが同じ trace ID でつながる
  （Cloud Logging が trace のフィールドとして解釈するには `logging.googleapis.com/trace` の形 `projects/<id>/traces/<trace_id>` が要る。デプロイのテンプレートでプロジェクト ID を渡すかは M6 で決める。要検証）

### 19. トレースの依存とライセンス

- トレースのための直接の依存は `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`（Apache-2.0、v1.47.0 は 2026-10-02 公開。OTLP のメトリクスのエクスポーターと同じ版の系列）だけ。
  `otel/trace`・`otel/sdk` は既にあり、W3C の propagator は `go.opentelemetry.io/otel/propagation` にある。contrib（`otelhttp`・AWS の X-Ray の propagator）は使わない
- すべて純 Go で、`CGO_ENABLED=0` を保つ。gRPC のエクスポーターは使わない（§2 と同じ）

### 覆る条件

- Cloud Run がアプリのカスタム指標での伸縮に対応したら、worker の契機をアプリの `my.job.wait` に寄せることを再検討する
- `otel` のアダプタが増やすバイナリのサイズや起動時間が、コールドスタートを目に見えて悪くする（M6 で計測）なら、`prometheus` を自前の出力にすることを再検討する
- ログベースの指標・EMF の費用が、想定の規模で無料枠を大きく超えるなら、Cloud Run と AWS の既定の経路を見直す
- 計測の仕組みを持たない（基盤のログからの抽出だけにする）と改めた場合は、システムの指標も基盤に寄せ、プロセスの中の飽和はログの定期の出力で補う
- Cloud Run のサービスが独自の指標でのスケールに対応した場合や、web の負荷が CPU に表れない（I/O 待ちが主で CPU が上がらないまま遅延が伸びる）ことが M6 の計測で分かった場合は、
  システムの指標（DB の接続の待ちや §3 の遅延）をスケールの契機にすることを見直す
- コレクターのサイドカー・ADOT のレイヤーの資源や起動時間の費用が、想定の規模で無視できない（M6 で計測）なら、アダプタが受け口の認証を持って直接送ることを再検討する
- OTel の messaging の意味規約が Stable になって、非同期の処理の既定を親子に変えたら、§14 を見直す
- 記録したリクエストの `unit` の遅れが利用者に見えるほど大きい（M6 で計測）なら、インスタンスベース課金の Cloud Run を `batch` に変える

## 経緯

NFR-OPS-1（JSON ログ、リクエスト ID / ジョブ ID の相関）、NFR-OPS-2（OpenTelemetry のトレース・メトリクスを任意で有効化）、NFR-OPS-3（管理画面でキューの滞留）、FR-802（liveness / readiness）を、NFR-COST（ほぼ $0、常駐ミドルウェアを必須にしない）と ADR-0002 の依存の向きの中で実現する。
サーバーが出していたのは slog の JSON ログと `request_id` だけで、決定者は、サービスの指標とシステムの指標の 2 つの方針がないとオートスケーリングの契機を作れないと指示した（#645、#646）。
そこで計測の仕組みとサービスの指標、システムの指標をそれぞれ決め、後から M4 の要件の監査で NFR-OPS-2 のトレースが未実現と分かって、同じ仕組みの上にトレースを足した（#814）。
決済の webhook の受け口ができると、webhook が届かないことや署名の設定の誤りに気付けるよう、`my.surface` に `payments` を加えた（#919）。

確かめた事実（メトリクスは 2026-10-20、トレースは 2026-10-21 確認）:

- **Cloud Run** のオートスケーリングは CPU 使用率（既定の目標 60%）とリクエストの同時実行数だけを見る。メモリでもアプリのカスタム指標でも伸縮せず、ゼロからの起動はリクエストだけが契機になる（[About instance autoscaling](https://docs.cloud.google.com/run/docs/about-instance-autoscaling)）
- **Cloud Run のリクエストベース課金** では CPU はリクエストの処理中だけ割り当てられ、公式の文書はバックグラウンドの監視エージェントをインスタンスベース課金を選ぶ理由に挙げる（[Billing settings](https://docs.cloud.google.com/run/docs/configuring/billing-settings)）。Prometheus のサイドカーも CPU が絞られると scrape が失敗しうる（[Cloud Run sidecar](https://docs.cloud.google.com/stackdriver/docs/managed-prometheus/cloudrun-sidecar)）。min instances 0 では、プロセスの中で集計して周期的に送る方式も、外から scrape する方式も当てにできない
- **Lambda の SQS のイベントソース** は、メッセージの量に応じて poller を自動で増やす（同時実行を 1 分あたり最大 300 ずつ、既定で最大 1,250。[SQS scaling](https://docs.aws.amazon.com/lambda/latest/dg/services-sqs-scaling.html)）
- **EMF** は標準出力の JSON（`_aws.CloudWatchMetrics`）から CloudWatch Logs がメトリクスを取り出す。1 イベントに MetricDefinition は 100 まで、DimensionSet は 30 キーまでで、次元の組み合わせごとに課金される（[EMF specification](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch_Embedded_Metric_Format_Specification.html)）
- **OpenTelemetry Go** のトレースとメトリクスは Stable、ライセンスは Apache-2.0、cgo は使わない（[OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)）
- **W3C Trace Context** の sampled フラグは推奨にすぎず、安全なネットワークの入口では restart でき、`tracestate` に個人を特定できる情報を入れてはならない（[Trace Context](https://www.w3.org/TR/trace-context/)）
- **OTel の messaging の意味規約**（Development）は、非同期の処理の span を作成時のコンテキストへの link にするのを既定とする（[Messaging spans](https://opentelemetry.io/docs/specs/semconv/messaging/messaging-spans/)）
- **Cloud Run** は受けたリクエストのトレースを自動で作り（最大 0.1 リクエスト/秒、無料）、`traceparent` を埋める（[Cloud Run tracing](https://docs.cloud.google.com/run/docs/trace)）。**Cloud Trace** は OTLP を `https://telemetry.googleapis.com` で受け、ADC で認証する（[Migrate to the OTLP endpoint](https://docs.cloud.google.com/trace/docs/migrate-to-otlp-endpoints)）
- **AWS X-Ray** は OTLP を HTTP だけで受け、認証は SigV4、1 リクエスト 5 MB・10,000 span まで（[CloudWatch OTLP endpoints](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch-OTLPEndpoint.html)）。SDK とデーモンは 2027-02-25 にサポートが終わる（[X-Ray SDK migration](https://docs.aws.amazon.com/xray/latest/devguide/xray-sdk-migration.html)）。記録したトレースの最初の 100,000 件/月は無料（[CloudWatch pricing](https://aws.amazon.com/cloudwatch/pricing/)）
- **他の実装**: GoToSocial は OTel Go SDK で計測し Prometheus の形式を別のポートで公開する（[GoToSocial Metrics](https://docs.gotosocial.org/en/latest/advanced/metrics/)）。Mastodon は Ruby のプロセスを `prometheus_exporter` で計測し、トレースだけを OTLP で送る（[Mastodon configuration](https://docs.joinmastodon.org/admin/config/)）

各ターゲットが標準で出す指標とスケールの契機（2026-10-20 確認）:

| ターゲット | 標準の指標（アプリの計測なし） | 標準のスケールの契機 | 独自の指標でのスケール |
| --- | --- | --- | --- |
| Cloud Run（サービス） | コンテナの CPU・メモリの使用率と割り当て、インスタンス数、起動の遅延、リクエスト数・遅延、最大同時リクエスト数、送受信バイト | CPU 使用率と同時リクエスト数（ともに既定の目標 60%） | サービスにはない。worker pool は CREMA（KEDA を使う別のサービス）で外部の指標からスケールできる |
| Lambda | `Invocations`・`Errors`・`Throttles`・`Duration`・`ConcurrentExecutions` ほか。メモリの使用量は標準の指標にない（実行ログの `REPORT` 行に最大使用量） | 同時実行数が要求に応じて増える。SQS のイベントソースはポーラーが自動で増える | 対象外 |
| ECS（Fargate） | サービスの `CPUUtilization`・`MemoryUtilization`、ALB の `RequestCountPerTarget` | Application Auto Scaling の目標追跡 | CloudWatch の独自の指標（アプリが書く必要がある） |
| Kubernetes（microk8s） | metrics-server を入れると Pod の CPU・メモリ | HPA の Resource 指標（コンテナの requests が要る） | Prometheus Adapter か KEDA（microk8s の community アドオン `keda`） |
| Docker Compose | なし（`docker stats` だけ） | なし（手で台数を変える） | — |

出典: [Cloud Run のモニタリング](https://docs.cloud.google.com/run/docs/monitoring)、[Lambda の指標の種類](https://docs.aws.amazon.com/lambda/latest/dg/monitoring-metrics-types.html)、[ECS の目標追跡](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/service-autoscaling-targettracking.html)、[Kubernetes の HorizontalPodAutoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)、[MicroK8s のアドオン](https://canonical.com/microk8s/docs/addons)、[KEDA の Prometheus スケーラー](https://keda.sh/docs/2.18/scalers/prometheus/)。

## 採らなかった案

計測の手段（メトリクス）:

- コアから OTel のメトリクス API を直接呼ぶ: API は no-op 前提の薄い層で、自前の port の重複がないのが最大の利点。ただし ADR-0002 に例外を作り、EMF のように OTel の集計を通さない出力を同じ呼び出しで扱えない。port は計器の宣言と 5 種の記録だけで小さい
- Prometheus のクライアントで計測し `/metrics` だけを公開する: pull の 1 経路だけになり、Cloud Run（リクエストベース課金）と Lambda では scrape を当てにできない。OTel は Prometheus の形式も出せるので、利点は採用案に含まれる
- アプリはメトリクスを持たず構造化ログだけを出す: 費用は最小だが、Self-hosted のキューの滞留のような Gauge を出せず KEDA の契機が作れない。1 リクエスト・1 ジョブを 1 行で表す考え方は §5 で使う

スケールの契機（メトリクス）:

- 基盤の指標だけに任せる: 実装も依存も費用もないが、Self-hosted の DB キューの滞留は基盤にない
- アプリの指標を全ターゲットの契機にする: 同じ指標で揃うが、Cloud Run はカスタム指標で伸縮せず、部品と費用が増える
- アプリのシステムの指標（DB の接続の待ち・goroutine 数）も契機にする: I/O 待ちで CPU が上がらない負荷に反応できるが、DB の接続の待ちで台数を増やすと、待ちの原因が DB 側にあるとき接続がさらに増えて悪化する

値の共有（§4）:

- 各プロセスが DB に問い合わせてキャッシュする: 最も単純だが、問い合わせがプロセスの数に比例し、計算の時刻がずれて値が食い違う
- 出来事の数を KV の `IncrBy` で合計する: プロセスをまたいで正確になるが、出来事のたびに KV（既定では DB）を書く。バックエンドが合計を出せるので要らない
- DB に current-value の表を持つ（ADR-0020）: 永続するが、失っても作り直せる値のために表とスキーマを増やす。TTL のある KV で足りる
- リーダーを選んで 1 プロセスだけが計算する: 選出の仕組みが要り、scale to zero や Lambda では成り立たない。KV のロックで同じことをより小さく実現できる
- OTel Collector などバックエンドで集約する: Counter・UpDownCounter はこの方式にした。DB から読む値の計算はアプリにしかできないので、Gauge には使えない

システムの指標の分担:

- 基盤の指標だけにする: 実装も費用もないが、DB の接続の待ち・goroutine・ヒープと上限・外向きの接続が見えない。Docker Compose ではほぼ何も見えず、Lambda ではメモリが標準の指標にない
- アプリがすべてを出す: どのターゲットでも同じ指標が出るが、CPU・メモリを重ねて測り、スケールをアプリの指標で行うには部品と費用が増える（NFR-COST-2）。コンテナの CPU の上限に対する使用率は cgroup を読まないと正しく出ない

トレース:

- コアから OTel のトレースの API を直接呼ぶ: port の重複がなく contrib をそのまま使えるが、ADR-0002 に例外を作り、§16 の許可リストを型で強制できない。port は開始・属性・終了・伝播の 4 つで小さい
- contrib の自動計測（`otelhttp`・DB ドライバの計測）を境界に差し込むだけにする: 実装は最小だが、`client.address`・`user_agent.original`・`url.path` を属性に入れて §16 に反し、外向きは既定で `traceparent` を宛先へ送って §15 に反する
- アプリはトレースを出さず、基盤のトレース（Cloud Run の自動のトレース、Lambda の `Active`）とログの相関だけにする: 費用は最小だが、DB・ジョブ・配送の内訳が見えず、Self-hosted には基盤のトレースがない。ログの相関の考え方は `none` の既定の挙動に取り込んだ
- ジョブの span を enqueue した span の子にする: 1 つのトレースで最も見やすいが、1 つの投稿が数百〜数千の配送のジョブに分かれ、再試行は数時間に及ぶ（ADR-0004）。トレースが際限なく大きくなり、受け手の上限（X-Ray は span の時刻が 14 日以内、1 リクエスト 10,000 span）と表示の使い勝手を損ねる
- ジョブへ引き継がない: 最も小さいが、NFR-OPS-1 の相関がリクエストとジョブの間で切れたままになる
- OTLP に加えて Cloud Trace・X-Ray の独自の形式のエクスポーターを持つ、ログに span を書いて基盤に取り込ませる: どちらも OTLP/HTTP を 1 つ持ってコレクターに差を吸収させる形より、サーバーが抱えるものが増える

`my.surface`:

- 決済の webhook を `web` に含める: 値が増えないが、`web` は Web クライアントのページの面で、webhook の失敗がページの要求の数に紛れる。呼ぶのが事業者のサーバーで性質も違う

## Consequences（影響）

- 良い影響
  - 既定（`none`）は同じ費用と挙動で、ほぼ $0 の構成を壊さない。ログに trace ID が付くぶん、Cloud Run の基盤のトレースとの相関が増える
  - 1 つの計測から Prometheus・OTLP・EMF に出せ、ターゲットを変えてもアプリの計測のコードは変わらない
  - サーバーレスでは伸縮を基盤に任せるので、アプリの指標が届かなくても伸縮が止まらない
  - ラベルの規則で、時系列の数が連合先・利用者の数に比例しない
  - スケールの設定がどのターゲットでも基盤の標準の機能で書け、常駐の収集器やアダプタを必須にしない（Kubernetes で worker を滞留でスケールするときだけ KEDA が要る）
  - DB の接続の待ち・goroutine の増加・ヒープと上限の比など、基盤から見えない飽和を見つけられる
  - リクエストからジョブ・配送まで link でたどれ、ファンアウトがトレースを肥大させない
  - トレースのヘッダーが連合先へ出ず、外部から記録の量を操作されない。属性の許可リストで個人情報が受け手に残らない
  - webhook の件数とエラー率を `my.surface = payments` で絞って見られる
- 悪い影響・トレードオフ
  - port と OTel の SDK の二重の層を持つ。port に OTel の機能（exemplar、ビューでのバケットの変更など）を足すたびに port を広げ、HTTP・DB の span は自前で書く。contrib の自動計測の恩恵（新しい意味規約への追従）を受けない
  - Cloud Run の既定の経路はログベースの指標で、プロセスごとの UpDownCounter（処理中のリクエスト・ストリームの接続）をアプリから出せない。基盤の指標で代える
  - `emf` はログの量を増やし、CloudWatch Logs の取り込みの費用が増える（要検証）
  - OTel の Prometheus エクスポーター（`go.opentelemetry.io/otel/exporters/prometheus`）は v0 系で、`prometheus/client_golang` を持ち込む
  - 連合先ごとの指標はメトリクスにないので、特定のホストへの配送の問題はログと管理画面で調べる
  - インスタンス全体の Gauge は最大で鮮度の上限（30 秒・15 分）だけ古い。Cloud Run と Lambda では 15 分ごとの値になり、DB を 15 分に 1 回起こす
  - CPU を契機にする web は、I/O 待ちで遅延が伸びても台数が増えないことがある。Cloud Run は同時リクエスト数で補えるが、ECS と Kubernetes は CPU の目標の調整が要る
  - Docker Compose では台数が増えないので、指標は警報と容量の見積もりにしか使えない
  - 意味規約の Development の段階の名前を使うので、規約が変わると名前の変更（v0 の破壊的変更、ADR-0023）が要る
  - Lambda はメモリの指標が標準にないので、プロセスの指標を送る費用（§2 の経路）がかかる。送らない運営者は `REPORT` のログで補う
  - ジョブの表に列が増える
  - link の方式は、トレースの表示で 1 つの画面にまとまらない。受け手が link をたどれるかは受け手次第（要検証）
  - 比率のサンプリング（head sampling）では、まれなエラーのトレースを取り逃しうる。確実に残すには Collector の tail sampling が要る
  - `unit` は記録したリクエストの応答を送信の分だけ遅らせる
  - `server.address` は連合先のホスト名を受け手に残す。1 人のサーバーの連合先は利用者の交友を示しうるので、運用者の受け手の扱いに依存する
  - Cloud Run と Lambda でトレースを有効にすると、コレクターのサイドカー・ADOT のレイヤーの資源と起動時間が増え、Cloud Run はインスタンスベース課金になる
  - `my.surface` の値が 1 つ増え、HTTP の指標の時系列が少し増える。課金が無効なら、この面には要求が来ない
- 後続で必要になる作業（M6）
  - Cloud Run のログベースの指標・CloudWatch のダッシュボードとアラーム・microk8s の KEDA の `ScaledObject` と Prometheus の設定をデプロイのテンプレートに入れる
  - [deployment-targets.md](../architecture/deployment-targets.md) のターゲットごとのスケールの設定と `GOMEMLIMIT` の記述をテンプレートと合わせる
  - デプロイのテンプレートの推奨の設定（Cloud Run の Collector のサイドカー、Lambda の ADOT のレイヤー）、Cloud Run のログの trace のフィールド、費用の実測を [deployment-targets.md](../architecture/deployment-targets.md) に書く

## Open Questions / 要検証

- 要検証: Cloud Monitoring のログベースの指標（ユーザー定義）と Cloud Logging の取り込み、CloudWatch のカスタムメトリクスと Logs の取り込みの料金と無料枠（料金ページを取得できなかった。M6 で確認する）
- 要検証: Cloud Run の基盤の指標（リクエスト数・遅延・インスタンス数）と Cloud Tasks のキューの深さの指標が無料で使えること
- 要検証: `otel` のアダプタ（SDK・Prometheus エクスポーター・OTLP/HTTP エクスポーター）が増やすバイナリのサイズとコールドスタートの時間
- 要検証: Lambda Web Adapter の下で、リクエストの終わりに書いた標準出力の EMF が、実行環境の凍結の前に CloudWatch Logs に届くこと
- Cloud Run で SSE のストリームを持つ web の最大同時実行数と CPU の目標の値（M6 の計測）
- Lambda Insights（メモリの指標を出す拡張）の費用と、§2 の経路で送る費用のどちらが安いか（要検証）
- Cloud Run の worker を worker pool（CREMA）に移し、キューの滞留でスケールするか（Cloud Tasks の push をやめることになるので、ADR-0004 に関わる。要検証）
- 要検証: Cloud Trace の料金と無料枠（2.5M span/月が無料、超過は $0.20/百万 span とされるが、料金ページを取得できなかった）、X-Ray の OTLP の受け口で取り込んだ span の課金（Transaction Search を有効にする必要の有無）
- 要検証: Cloud Run の Collector のサイドカーと ADOT の Lambda のレイヤーの資源・起動時間への影響と、ADOT のレイヤーが凍結の前に送り切ること
- 要検証: Cloud Run が公開のリクエストの `traceparent` を常に上書きするか（上書きするなら Cloud Run の前段を `trusted_proxies` として扱える）、Lambda Web Adapter がアプリへ `traceparent` / `X-Amzn-Trace-Id` を渡すか
- 要検証: Cloud Trace・X-Ray の表示で span の link をたどれるか
