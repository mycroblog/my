# ADR-0055: サービスメトリクスの可観測性

- Status: Accepted（§3 の `my.surface` の値の一覧を ADR-0075（my.surface に payments を加える） が置き換える）
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #645

## Context（背景・課題）

決定者の指示（原文）: 「Observability についての方針を2つ建てる。1つはサービスメトリクスとしての Observability を保証するもの。1つはシステムメトリクスとしての Observability を保証するもの。この2つがないとオートスケーリングなどの契機が作れない。」

この ADR は **サービスの指標**（利用者から見た振る舞い: リクエスト・連合・ジョブ・配送・ストリーム・業務の数）と、
システムの指標の ADR（[ADR-0056](0056-system-observability.md) システムメトリクスの可観測性、#646）と共有する **計測の手段**（ライブラリ、port、エクスポートの経路、ラベルの規則）を決める。
プロセス・Go のランタイム・CPU・メモリ・DB の接続プール・アダプタの資源の指標は ADR-0056 が扱う。

今のサーバーが出すのは slog の JSON ログと `request_id` だけである（`internal/platform/log.go`）。メトリクスも `/metrics` もトレースもない。
[overview §3](../architecture/overview.md) は `Logger` / `Tracer` の port を「JSON stdout / OTel 任意」と書くが、`Tracer` の port はない。
OpenTelemetry Go（`go.opentelemetry.io/otel`・`otel/metric`・`otel/sdk` v1.47.0）は、Google Cloud の SDK の間接依存としてすでに `go.mod` にある。

関連する要件と決定:

- NFR-OPS-1（JSON ログ、リクエスト ID / ジョブ ID の相関）、NFR-OPS-2（OpenTelemetry のトレース・メトリクスを任意で有効化）、
  NFR-OPS-3（管理画面でキューの滞留・失敗ジョブ・連合先の状態を確認）、FR-802（liveness / readiness）
- NFR-COST（ほぼ $0 の構成）と [deployment-targets.md](../architecture/deployment-targets.md): Cloud Run はリクエストベース課金・min instances 0、
  AWS は Lambda（Lambda Web Adapter）+ SQS、Self-hosted は Docker Compose / microk8s
- [ADR-0002](0002-modular-monolith-hexagonal.md): `internal/domain` は標準ライブラリだけ、`internal/app` は自モジュールのドメインと他モジュールの `internal/app` と `internal/i18n` だけを import する（depguard の strict）。
  コアが外部ライブラリを呼ぶなら port を通す
- [ADR-0004](0004-job-queue-abstraction.md): キューは DB・Cloud Tasks・SQS・inline のアダプタで、ジョブは at-least-once

今決める理由: オートスケーリングの契機は、どの指標をどの経路で出すかが決まらないと作れない。M6 のデプロイのテンプレート（Cloud Run・AWS・microk8s）は、
ここで決める指標の名前と経路を前提に、スケールの設定とダッシュボードを書く。

### 一次情報で確かめた事実（2026-10-20 確認）

- **Cloud Run のオートスケーリング** は CPU 使用率（既定の目標 60%）とリクエストの同時実行数だけを見る。メモリでは伸縮せず、ゼロからの起動はリクエストだけが契機になる。
  アプリが出すカスタム指標では伸縮しない（[About instance autoscaling](https://docs.cloud.google.com/run/docs/about-instance-autoscaling)）
- **Cloud Run のリクエストベース課金** では CPU はリクエストの処理中だけ割り当てられる。公式の文書は「バックグラウンドで動く前提の OpenTelemetry などの監視エージェント」をインスタンスベース課金を選ぶ理由に挙げる
  （[Billing settings](https://docs.cloud.google.com/run/docs/configuring/billing-settings)）。
  Managed Service for Prometheus のサイドカーも、CPU が絞られると scrape が失敗しうるとして、インスタンスベース課金を推奨する（[Cloud Run sidecar](https://docs.cloud.google.com/stackdriver/docs/managed-prometheus/cloudrun-sidecar)）。
  つまり **min instances 0 のリクエストベース課金では、プロセスの中で集計して周期的に送る方式も、外から scrape する方式も当てにできない**
- **Lambda の SQS のイベントソース** は、メッセージの量に応じて poller を自動で増やす（同時実行を 1 分あたり最大 300 ずつ、既定で最大 1,250）。
  キューの滞留でのスケールは基盤が行い、アプリの指標は要らない（[Configuring scaling behavior for SQS event source mappings](https://docs.aws.amazon.com/lambda/latest/dg/services-sqs-scaling.html)）
- **CloudWatch の Embedded Metric Format（EMF）** は、標準出力に書いた JSON のログ（`_aws.CloudWatchMetrics`）から CloudWatch Logs がメトリクスを取り出す。
  1 イベントに MetricDefinition は 100 まで、DimensionSet は 30 キーまで。次元の組み合わせごとにカスタムメトリクスが 1 つでき、課金される
  （[EMF specification](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch_Embedded_Metric_Format_Specification.html)）
- **KEDA** の Prometheus スケーラーは PromQL の結果（単一の値）としきい値 `threshold`・`activationThreshold` で伸縮し、PostgreSQL スケーラーは SQL の結果で伸縮する
  （[Prometheus scaler](https://keda.sh/docs/2.18/scalers/prometheus/)、[PostgreSQL scaler](https://keda.sh/docs/2.18/scalers/postgresql/)）
- **OpenTelemetry Go** のトレースとメトリクスは Stable、ログは Release candidate（[OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)）。
  ライセンスは Apache-2.0（[open-telemetry/opentelemetry-go](https://github.com/open-telemetry/opentelemetry-go)）で、AGPL-3.0 と組み合わせられる。cgo は使わない
- **他の実装**: GoToSocial は OpenTelemetry Go SDK で計測し、Prometheus の形式を別のポート（既定 `localhost:9464/metrics`）で公開する。認証はなく、リバースプロキシで守る
  （[GoToSocial Metrics](https://docs.gotosocial.org/en/latest/advanced/metrics/)）。
  Mastodon は Ruby のプロセスを `prometheus_exporter` で計測し（`MASTODON_PROMETHEUS_EXPORTER_ENABLED`、詳細な指標は別のフラグで任意）、
  トレースだけを OTLP で送る（`OTEL_EXPORTER_OTLP_ENDPOINT`、[Mastodon configuration](https://docs.joinmastodon.org/admin/config/)）

## Decision Drivers（判断基準）

1. **ほぼ $0 の構成を壊さない**（NFR-COST）: 計測のために常駐のプロセス・サイドカー・インスタンスベース課金を必須にしない。指標を出さない既定の構成は今と同じ費用で動く
2. **オートスケーリングの契機を各ターゲットで作れる**: 基盤が伸縮の判断に使える形と経路で、必要な指標が届く
3. **依存の向き**（ADR-0002）: コアは外部ライブラリを import しない。計測は port を通し、port の実装はアダプタに置く
4. **ラベルの濃度と個人情報**: 時系列の数が利用者・連合先の数に比例しない。個人を特定できる値をラベルにしない
5. **ベンダーに縛られない**: 1 つの計測から Prometheus・OTLP・CloudWatch のどれにも出せる
6. **実装と依存の小ささ**: `CGO_ENABLED=0` のまま、ライセンスが AGPL と両立し、3 日以上前に公開された版だけを使う

## Considered Options（選択肢）

計測のライブラリと port の形:

1. **A. 自前の小さな `Meter` port + OpenTelemetry Go SDK のアダプタ**: コアと境界の層は port だけを呼び、アダプタが OTel の SDK に渡す。エクスポートは OTel のエクスポーターと、EMF の自前の出力
2. **B. コアから OpenTelemetry のメトリクス API（`go.opentelemetry.io/otel/metric`）を直接呼ぶ**: OTel の API は既定で no-op になるよう設計されており、ライブラリから呼ばれる前提のもの。depguard の許可リストに足す
3. **C. Prometheus のクライアント（`prometheus/client_golang`）で計測し、`/metrics` だけを公開する**
4. **D. アプリはメトリクスを持たず、構造化ログだけを出す**: 集計は基盤（Cloud Logging のログベースの指標、CloudWatch の EMF・メトリクスフィルタ、Loki など）に任せる

オートスケーリングの契機:

1. **a. 基盤が持つ指標に任せる**（Cloud Run の同時実行数、Lambda の SQS のイベントソース）。アプリの指標は監視とアラートにだけ使う
2. **b. アプリの指標（キューの滞留、処理中のリクエスト数）を全ターゲットの契機にする**
3. **c. 基盤が持つ指標を優先し、基盤が持たないもの（Self-hosted の DB キューの滞留）だけをアプリが出す**

## Decision（決定）

**決定: A（自前の `Meter` port + OpenTelemetry Go SDK のアダプタ）と、契機は c（基盤の指標を優先し、足りない分だけアプリが出す）。**
エクスポートの既定は `none` で、ターゲットごとに Prometheus（pull）・OTLP（push）・EMF（ログ）から選ぶ。
出来事の数はプロセスごとに持ってバックエンドで集計し、インスタンス全体の値（キューの滞留・利用者数・メディアの容量）は KV に置く 1 つのスナップショットを全プロセスで共有する。トレースはこの ADR では保証しない。

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
- ADR-0056 は同じ port とアダプタでシステムの指標を足す（Go のランタイム・プロセスの指標を OTel の contrib から足すかは ADR-0056 が決める）

B を選ばない理由: OTel の API は no-op を前提にした薄い層で、B の最も強い利点は自前の port の重複がないことである。
ただし ADR-0002 の「コアは外部ライブラリを import しない」に例外を作り、EMF のように OTel の集計を通さない出力を同じ呼び出しで扱えない。port は計器の宣言と 5 種の記録だけで小さく、重複は少ない。

C を選ばない理由: pull の 1 経路だけになり、Cloud Run（リクエストベース課金）と Lambda では scrape を当てにできない。OTel は Prometheus の形式も出せるので、C の利点は A に含まれる。

D を選ばない理由: 費用は最も小さいが、Self-hosted のキューの滞留のような「その時点の値」（Gauge）を出せず、KEDA の契機が作れない。
ただし D の考え方（1 リクエスト・1 ジョブを 1 行のログで表す）は、後述の Cloud Run の既定の経路として使う。

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
| `http.server.request.duration` | Histogram・秒 | `http.request.method`、`http.route`（`ServeMux` のパターン。一致しないものは `other`）、`http.response.status_code`、`my.surface`（`client_api` / `activitypub` / `oauth` / `web` / `media` / `admin`） | クライアント API・Inbox・Web の件数・遅延・エラー率 |
| `http.server.active_requests` | UpDownCounter・{request} | `my.surface` | 処理中のリクエスト数（Self-hosted の web の契機） |
| `my.inbox.activities` | Counter・{activity} | `activity.type`（既知の Activity の型、それ以外は `other`）、`my.outcome`（`accepted` / `duplicate` / `rejected` / `ignored` / `error`） | 連合の受信の件数と拒否の割合（ADR-0033） |
| `my.delivery.attempts` | Counter・{attempt} | `my.outcome`（`success` / `retry` / `gone` / `failed`） | 配送の成功率 |
| `my.delivery.duration` | Histogram・秒 | `my.outcome` | 配送の 1 回の HTTP の遅延 |
| `my.job.duration` | Histogram・秒 | `my.job.type`（登録済みのジョブの型）、`my.outcome`（`success` / `retry` / `dead`） | ジョブの処理時間と失敗 |
| `my.job.wait` | Histogram・秒 | `my.job.type` | 実行可能になってから処理が始まるまでの待ち（キューの遅延） |
| `my.queue.ready` | Gauge・{job} | `my.job.type` | 実行可能なジョブの数（DB キューだけ。§4・§7） |
| `my.queue.oldest_ready_age` | Gauge・秒 | なし | 最も古い実行可能なジョブの待ち時間（DB キューだけ。§4・§7） |
| `my.queue.dead` | Gauge・{job} | なし | 失敗して止まったジョブの数（NFR-OPS-3 と同じ値） |
| `my.timeline.delivery.lag` | Histogram・秒 | なし | 投稿の作成からホームタイムラインへの書き込み・ストリームへの publish までの遅延 |
| `my.stream.connections` | UpDownCounter・{connection} | `my.stream.kind`（ストリームの種類） | ストリーム（SSE）の接続数 |
| `my.accounts.registered`・`my.statuses.created` | Counter・{account}・{status} | `my.statuses.created` だけ `my.visibility`（ADR-0017 の 5 値） | 登録・投稿の業務の数 |
| `my.instance.users`・`my.instance.statuses` | Gauge | なし | ローカルの利用者数・投稿数（NodeInfo の usage と同じ値。§4） |
| `my.media.stored_bytes` | Gauge・バイト | `my.media.origin`（`local` / `remote_cache`） | メディアの容量。`local` はローカルのアップロードの現在の派生（`media_variants`）と処理待ちの原本（`uploaded_media`）の `file_size` の合計、`remote_cache` はリモートのメディアのキャッシュ（`remote_media_cache`）の合計（§4） |

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

他の案と比べた理由:

- **各プロセスが DB に問い合わせてプロセスの中でキャッシュする**: 最も単純だが、DB への問い合わせがプロセスの数に比例し、プロセスごとに計算の時刻がずれて値が食い違う
- **出来事の数を KV の `IncrBy` で全体の合計として持つ**: プロセスをまたいで正確な合計になるが、出来事のたびに KV（既定では DB）を書く。バックエンドが合計を出せるので要らない
- **DB に現在値（current-value）の表を持ち、ジョブが更新する**（ADR-0020）: 永続するが、メトリクスのための表とスキーマを増やす。値は失っても作り直せるので、TTL のある KV で足りる
- **リーダーを選んで 1 プロセスだけが計算する**: リーダー選出の仕組みが要り、scale to zero や Lambda では成り立たない。KV のロックで同じことをより小さく実現できる
- **OTel Collector などバックエンド側で集約・保存する**: Counter・UpDownCounter はこの方式（バックエンドで集計）にする。DB から読む値の計算はアプリにしかできないので、Gauge には使えない

### 5. ログとの関係（NFR-OPS-1）

- HTTP のリクエストとジョブは、終わりに 1 行の構造化ログを出す（今のアクセスログを含む）。そのログは §3 の Histogram と Counter のラベルと値（`http.route`、`http.response.status_code`、`duration_seconds`、`my.job.type`、`my.outcome`、`wait_seconds`）を同じ名前で持つ
- Cloud Run ではこのログからログベースの指標（分布の指標を含む）を作る。`emf` はこの 1 行に `_aws` の指示を足したものにする。どちらもプロセスの中で集計しないので、リクエストベース課金の CPU の制限と、Lambda の凍結の影響を受けない

### 6. ラベルの規則（濃度と個人情報）

- ラベルの値は、コードで決まった有限の集合（ルートのパターン、ジョブの型、Activity の型、公開範囲、結果）だけにする
- 次のものはラベルにしない: 連合先のドメイン・ホスト、アカウント・投稿・ジョブの ID、URL・パス（パターンでない生の値）、クライアント IP、User-Agent、OAuth のアプリケーション
- 連合先ごとの状態（配送の失敗の続き、到達できないホスト）はメトリクスでなく DB と管理画面（NFR-OPS-3、[federation.md](../architecture/federation.md)）で見る
- 資源の属性（OTel の resource）は `service.name`（既定 `my`）、`service.version`、`my.role`（`web` / `worker` / `all`）と、基盤が与えるインスタンスの識別子だけにする

### 7. オートスケーリングの契機（c）

| ターゲット | web の契機 | worker の契機 | アプリの指標の役割 |
| --- | --- | --- | --- |
| Cloud Run | 基盤のリクエストの同時実行数（と CPU） | Cloud Tasks の push がリクエストになるので、同じく同時実行数 | 監視とアラートだけ |
| AWS（Lambda） | 基盤の同時実行 | SQS のイベントソースの poller の自動の増減 | 監視とアラートだけ |
| microk8s（PostgreSQL・複数レプリカ） | HPA: `http.server.active_requests` の 1 Pod あたりの平均（KEDA の Prometheus スケーラー）。CPU を併用するかは ADR-0056 | KEDA の Prometheus スケーラー: `my.queue.oldest_ready_age` と `my.queue.ready` の合計 | 伸縮の契機 |
| Docker Compose・SQLite | 伸縮しない（SQLite はレプリカ 1 固定） | 同左 | 監視だけ |

- DB キューの `my.queue.*` はインスタンス全体の値で、どのプロセスが出しても同じになる。PromQL では `max` で 1 つにする。
  KEDA の PostgreSQL スケーラーでジョブの表を直接数える方法はスキーマに結びつくので採らない
- 具体的なしきい値は M6 のデプロイのテンプレートで決め、[deployment-targets.md](../architecture/deployment-targets.md) に書く

### 8. SLI と SLO

- SLI は §3 の指標で定義する: クライアント API の可用性（5xx でない割合）と遅延（ストリームを除く `client_api` の p95）、Inbox の受理の可用性、配送の成功率（`success` / (`success` + `failed`)）、ジョブの待ち（`my.job.wait` の p95）、タイムラインの配信の遅延
- SLO の目標値はサーバーには持たせず、デプロイのテンプレートのアラートの既定値として M6 で決める（運用者が上書きする）

### 9. トレース

この ADR はトレースを保証しない。NFR-OPS-2 の「任意で有効化」は、メトリクスはこの ADR の `otlp`、トレースは別の ADR で扱う。
OTel を計測の基盤に選ぶので、トレースを足すときも同じ SDK とエクスポーターを使える。

### 覆る条件

- Cloud Run がアプリのカスタム指標での伸縮に対応したら、worker の契機をアプリの `my.job.wait` に寄せることを再検討する
- `otel` のアダプタが増やすバイナリのサイズや起動時間が、コールドスタートを目に見えて悪くする（M6 で計測）なら、`prometheus` を自前の出力にすることを再検討する
- ログベースの指標・EMF の費用が、想定の規模で無料枠を大きく超えるなら、Cloud Run と AWS の既定の経路を見直す

## Consequences（影響）

- 良い影響
  - 既定（`none`）は今と同じ費用と挙動で、ほぼ $0 の構成を壊さない
  - 1 つの計測から Prometheus・OTLP・EMF に出せ、ターゲットを変えてもアプリの計測のコードは変わらない
  - サーバーレスでは伸縮を基盤に任せるので、アプリの指標が届かなくても伸縮が止まらない
  - ラベルの規則で、時系列の数が連合先・利用者の数に比例しない
- 悪い影響・トレードオフ
  - port と OTel の SDK の二重の層を持つ。port に OTel の機能（exemplar、ビューでのバケットの変更など）を足すたびに port を広げる必要がある
  - Cloud Run の既定の経路はログベースの指標で、プロセスごとの UpDownCounter（処理中のリクエスト・ストリームの接続）をアプリから出せない。基盤の指標で代える
  - `emf` はログの量を増やし、CloudWatch Logs の取り込みの費用が増える（要検証）
  - OTel の Prometheus エクスポーター（`go.opentelemetry.io/otel/exporters/prometheus`）は v0 系で、`prometheus/client_golang` を間接に持ち込む（要検証: 実装のときの版とバイナリのサイズ）
  - 連合先ごとの指標はメトリクスにないので、特定のホストへの配送の問題はログと管理画面で調べる
  - インスタンス全体の Gauge は最大で鮮度の上限（30 秒・15 分）だけ古い。Cloud Run と Lambda では 15 分ごとの値になり、DB を 15 分に 1 回起こす
- 後続で必要になる作業（Issue 化する）
  - `kernel.Meter` port、`none` / `otel` / `emf` のアダプタ、設定（`metrics_exporter`・`metrics_listen`・`metrics_emf_namespace`）と [configuration.md](../reference/configuration.md) の再生成
  - HTTP のミドルウェア・ジョブの runner・配送・Inbox・ストリーム・業務の数への計測と、リクエスト・ジョブの終わりの構造化ログのフィールドの統一（§5）
  - スナップショット（§4）: 計算の関数、KV の名前空間 `metrics` とロック、定期ジョブ `metrics.snapshot`、`my.metrics.snapshot.age`
  - DB キューの `my.queue.*` と、メディアの容量 `my.media.stored_bytes` の問い合わせ
  - M6: Cloud Run のログベースの指標・CloudWatch のダッシュボードとアラーム・microk8s の KEDA の `ScaledObject` と Prometheus の設定をデプロイのテンプレートに入れる
  - [overview.md](../architecture/overview.md) の port の表（`Logger` / `Tracer`）を `Meter` に合わせて直す

## Pros and Cons of the Options（詳細比較・任意）

| 観点 | A. port + OTel SDK | B. OTel API を直接 | C. Prometheus のクライアント | D. ログだけ |
| --- | --- | --- | --- | --- |
| ADR-0002 との整合 | ◎ | △（depguard に例外） | △（port を作るなら A と同じ） | ◎ |
| Cloud Run（リクエストベース課金）・Lambda | ○（ログ・EMF の経路） | ○ | ×（scrape できない） | ◎ |
| Self-hosted の KEDA の契機（Gauge） | ◎ | ◎ | ◎ | × |
| ベンダー中立（Prometheus / OTLP / CloudWatch） | ◎ | ○（EMF は別） | △ | ○ |
| 実装量 | 中 | 小 | 小 | 最小 |
| 依存 | OTel SDK（既に間接依存） | OTel API・SDK | client_golang | なし |

## Open Questions / 要検証

- 要検証: Cloud Monitoring のログベースの指標（ユーザー定義）と Cloud Logging の取り込み、CloudWatch のカスタムメトリクスと Logs の取り込みの料金と無料枠（料金ページを本調査で取得できなかった。M6 で確認する）
- 要検証: Cloud Run の基盤の指標（リクエスト数・遅延・インスタンス数）と Cloud Tasks のキューの深さの指標が無料で使えること
- 要検証: `otel` のアダプタ（SDK・Prometheus エクスポーター・OTLP/HTTP エクスポーター）が増やすバイナリのサイズとコールドスタートの時間
- 要検証: Lambda Web Adapter の下で、リクエストの終わりに書いた標準出力の EMF が、実行環境の凍結の前に CloudWatch Logs に届くこと
