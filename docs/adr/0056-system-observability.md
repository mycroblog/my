# ADR-0056: システムメトリクスの可観測性

- Status: Accepted
- Date: 2026-10-20
- Deciders: @rosylilly
- Issue: #646

## Context（背景・課題）

サーバーが出す観測の情報は、slog の JSON ログと `request_id` だけ（`internal/platform/log.go`）で、プロセスや資源の指標を出していない。
[overview §3](../architecture/overview.md#3-ポート一覧) は `Logger` / `Tracer` を「OTel 任意」と書くが、計測の仕組みはまだない。
決定者は、サービスの指標とシステムの指標の 2 つの方針がないとオートスケーリングなどの契機を作れない、と指示している（#646）。

この ADR はシステムの指標（プロセス・Go のランタイム・依存する資源への接続）を扱う。
利用者から見た振る舞いの指標（リクエスト・ジョブ・配送の数と遅延、キューの滞留）と、計測の仕組み（ライブラリ・port・エクスポートの経路・有効化の設定）は
ADR-0055 サービスメトリクスの可観測性（#645）が決める。この ADR はその仕組みに乗り、何を測るか・誰が測るか・どれをスケールの契機にするかだけを決める。

関係する要件と現状:

- NFR-OPS-1（JSON ログ）、NFR-OPS-2（OpenTelemetry は任意で有効化）、NFR-OPS-3（管理画面でキューの滞留を確認）、FR-802（liveness / readiness）
- NFR-PERF-2（web 層はステートレスで水平スケールし、worker は同じバイナリで役割を切り替える）、NFR-COST-1 / NFR-COST-2（アイドル時ほぼ $0、常駐ミドルウェアを必須にしない）
- 役割は `role = all | web | worker`（`internal/platform/config`）。worker の役割はヘルスチェックだけを HTTP で返す（`internal/platform/http.go`）
- `/healthz` は HTTP を返せるかだけ、`/readyz` は DB に届きスキーマが揃っているかだけを 3 秒の上限で見る（FR-802）
- 資源への接続: PostgreSQL は `pgxpool.Pool`（`internal/adapter/db/postgres`）、SQLite は `database/sql`（`internal/adapter/db/sqlite`）、
  KV・ストリームは valkey-go / pgx の `LISTEN` / SNS + SQS / Pub/Sub、Blob は fs / S3、外向き HTTP は `kernel.HTTPClient` の実装
  （`internal/adapter/httpclient`、用途 `HTTPPurpose` ごとの上限）
- ターゲット（[deployment-targets.md](../architecture/deployment-targets.md)）: Google Cloud は Cloud Run（web と Cloud Tasks の push を受ける worker）、
  AWS は Lambda（Web Adapter と SQS のイベントソース）、Self-hosted は Docker Compose / microk8s。
  同じコンテナイメージは ECS（Fargate）でも動き、ALB の前提は信頼するプロキシの節にある。デプロイのテンプレートは M6 で作る
- go.mod の Go は 1.27 で、Go 1.25 から Linux の `GOMAXPROCS` の既定値は cgroup の CPU の上限に従う（[Go 1.25 Release Notes](https://go.dev/doc/go1.25#runtime)）。
  `GOMEMLIMIT` は自動では決まらない

今決める理由: M4 で計測を実装する前に、ADR-0055 と範囲を分け、web・worker のスケールの契機を各ターゲットの仕組みに合わせて決めておく必要がある。
M6 のデプロイのテンプレートは、この ADR のスケールの設定をそのまま書く。

## Decision Drivers（判断基準）

1. **ターゲットが標準で使える契機に合わせる**: 追加の部品なしにスケールの契機にできるのは、各基盤が自分で測る指標だけである（下記の事実）
2. **ほぼ $0 を壊さない**（NFR-COST-1 / NFR-COST-2）: 計測のために常駐の収集器・アダプタを必須にしない。無効のときは計測の費用がほぼゼロ
3. **飽和を見つけられる**: 基盤からは見えない、プロセスの中の詰まり（DB の接続の待ち、goroutine の増加、ヒープと上限、外向きの接続）を出す
4. **依存の向き**（ADR-0002）: システムの指標はアダプタと `internal/platform` が測り、ドメインとアプリケーション層に計測を持ち込まない
5. **ラベルの濃度が有界**: 相手のドメイン・アカウント・URL をラベルにしない
6. **ADR-0055 と仕組みを分けない**: 同じライブラリ・同じ経路・同じ有効化の設定で出す

### 各ターゲットが標準で出す指標とスケールの契機（2026-10-20 に公式文書で確認）

| ターゲット | 標準の指標（アプリの計測なし） | 標準のスケールの契機 | 独自の指標でのスケール |
| --- | --- | --- | --- |
| Cloud Run（サービス） | コンテナの CPU・メモリの使用率と割り当て、インスタンス数、起動の遅延、リクエスト数・遅延、最大同時リクエスト数、送受信バイト | CPU 使用率（既定の目標 60%）と同時リクエスト数（最大同時実行数に対する既定の目標 60%）。**メモリではスケールしない** | サービスにはない。Prometheus / OTel のサイドカーで独自の指標を Cloud Monitoring に書ける。worker pool は CREMA（KEDA を使う別のサービス）で外部の指標からスケールできる |
| Lambda | `Invocations`・`Errors`・`Throttles`・`Duration`・`ConcurrentExecutions` ほか。SQS のイベントソースの指標は opt-in（`PolledEventCount` など）。メモリの使用量は標準の指標にない（実行ログの `REPORT` 行に最大使用量） | 同時実行数が要求に応じて増える（関数ごと・リージョンごとの上限まで）。SQS のイベントソースはポーラーが自動で増える | 対象外（実行環境の数を指標で決めない） |
| ECS（Fargate） | サービスの `CPUUtilization`・`MemoryUtilization`（AWS/ECS）、ALB の `RequestCountPerTarget` | Application Auto Scaling の目標追跡で `ECSServiceAverageCPUUtilization`・`ECSServiceAverageMemoryUtilization`・`ALBRequestCountPerTarget` | CloudWatch の独自の指標で目標追跡・ステップスケーリングができる（アプリが CloudWatch に書く必要がある） |
| Kubernetes（microk8s） | metrics-server を入れると Pod の CPU・メモリ（`metrics.k8s.io`）。microk8s では `metrics-server` アドオン | HPA の Resource 指標（CPU・メモリ、コンテナの requests に対する使用率。requests がないと動かない） | `custom.metrics.k8s.io` / `external.metrics.k8s.io` を出すアダプタ（Prometheus Adapter）か KEDA（microk8s の community アドオン `keda`）が要る |
| Docker Compose | なし（`docker stats` だけ） | なし（手で台数を変える） | — |

出典:
[Cloud Run のインスタンスの自動スケーリング](https://docs.cloud.google.com/run/docs/about-instance-autoscaling)、
[Cloud Run のモニタリング](https://docs.cloud.google.com/run/docs/monitoring)、
[Cloud Run のモニタリングの概要](https://docs.cloud.google.com/run/docs/monitoring-overview)、
[Cloud Run worker pool の外部の指標による自動スケーリング](https://docs.cloud.google.com/run/docs/configuring/workerpools/crema-autoscaling)、
[Lambda の指標の種類](https://docs.aws.amazon.com/lambda/latest/dg/monitoring-metrics-types.html)、
[Application Auto Scaling の CloudWatch の指標](https://docs.aws.amazon.com/autoscaling/application/userguide/monitoring-cloudwatch.html)、
[ECS の目標追跡](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/service-autoscaling-targettracking.html)、
[Kubernetes の HorizontalPodAutoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)、
[MicroK8s のアドオン](https://canonical.com/microk8s/docs/addons)、
[KEDA の PostgreSQL スケーラー](https://keda.sh/docs/2.18/scalers/postgresql/)。

## Considered Options（選択肢）

### 分担

1. **基盤の指標だけに任せる**: アプリはシステムの指標を出さない
2. **基盤が資源（CPU・メモリ・台数・入口のリクエスト）を、アプリがプロセスの中の飽和（ランタイム・接続のプール・アダプタ・外向き HTTP）を出す**
3. **アプリがすべてを出す**: CPU・メモリもアプリが測り、スケールもアプリの指標で行う

### スケールの契機

- a. **基盤の契機だけを使う**: web は CPU（Cloud Run は同時リクエスト数も）、worker は push なら基盤の配送、pull ならキューの滞留（ADR-0055 のサービスの指標）
- b. **アプリのシステムの指標（DB の接続の待ち・goroutine 数）もスケールの契機にする**

## Decision（決定）

**分担は選択肢 2、スケールの契機は a** とする。

### 1. 分担: 基盤が測るもの、アプリが測るもの

| 種類 | 測る者 | 指標 |
| --- | --- | --- |
| コンテナの CPU・メモリ・台数・入口のリクエスト数と遅延 | 基盤（上表） | アプリは重ねて出さない。Docker Compose と Lambda のメモリだけは、下のプロセスの指標で補う |
| プロセス | アプリ（`internal/platform`） | CPU 時間、常駐メモリ（RSS）、開いたファイル記述子の数と上限（Linux のみ）、起動時刻、役割・版の情報（`role`、ビルドの版） |
| Go のランタイム | アプリ（`internal/platform`） | OTel の意味規約の `go.memory.used`・`go.memory.limit`・`go.memory.allocated`・`go.memory.gc.goal`・`go.goroutine.count`・`go.processor.limit`・`go.config.gogc`・`go.schedule.duration`。GC の回数と停止時間（`go.memory.gc.cycles`・`go.memory.gc.pause.duration`）も出す |
| DB の接続のプール | DB アダプタ | `db.client.connection.count`（`state` = `idle` / `used`）・`db.client.connection.max`・`db.client.connection.pending_requests`・`db.client.connection.wait_time`・`db.client.connection.timeouts`。PostgreSQL は `pgxpool.Pool.Stat()`、SQLite は `sql.DB.Stats()` を観測のコールバックで読み、クエリごとの計測は足さない |
| キュー・KV・ストリーム・Blob のアダプタ | 各アダプタ（port の実装の境界） | 操作ごとの所要時間のヒストグラムと失敗の数（ラベルは `adapter`・`operation`・`outcome`）。常時の接続を持つもの（valkey-go、`LISTEN` の接続、SQS のロングポーリング、Pub/Sub の受信）は接続の数と再接続の数 |
| 外向き HTTP | `internal/adapter/httpclient` | `http.client.request.duration`（ラベルは用途 `HTTPPurpose`・メソッド・状態コードの区分・SSRF ガードによる拒否か）と `http.client.open_connections` |

- 指標の名前と単位は OpenTelemetry の意味規約に合わせる。runtime・DB・HTTP クライアントの規約の多くは Development の段階なので、規約が変わったら追う
  （[Go のランタイム](https://opentelemetry.io/docs/specs/semconv/runtime/go-metrics/)、[DB](https://opentelemetry.io/docs/specs/semconv/database/database-metrics/)、
  [HTTP](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/)）。独自の指標の接頭辞と命名は ADR-0055 に従う
- ラベルに相手のホスト・ドメイン・URL・アカウント・Blob のキーを入れない（相手のドメインごとの配送の状態はサービスの指標と管理画面、NFR-OPS-3 で扱う）。
  プロセスの識別（インスタンス ID・役割・版）は、ラベルでなく ADR-0055 の仕組みがプロセスを識別する方法に従う
- 計測するのはアダプタと `internal/platform` だけで、`internal/domain` と `internal/app` はシステムの指標を知らない。
  アダプタが計測の道具を受け取る方法（port か、OTel の `MeterProvider` を直接か）は ADR-0055 に従う
- Go のランタイムの指標の実装は ADR-0055 のライブラリに合わせる。OpenTelemetry の Go SDK なら `go.opentelemetry.io/contrib/instrumentation/runtime`
  （Apache-2.0、意味規約の `go.*` を出す、[pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime)）、
  Prometheus のクライアントなら `collectors.NewGoCollector` と `collectors.NewProcessCollector` を使う（要検証: ADR-0055 の選択が決まってから版と依存を確かめる）
- 有効化・エクスポートの経路・送る間隔は ADR-0055 の設定に従い、システムの指標だけを別に有効にする設定は作らない。無効のときは no-op の実装で、観測のコールバックも登録しない

### 2. スケールの契機（役割ごと）

アプリが出すシステムの指標は **スケールの契機にしない**。診断と警報（飽和の検出）と、台数の上限の見積もりに使う。

| ターゲット | web（`role = web`） | worker（`role = worker`） |
| --- | --- | --- |
| Cloud Run | CPU 使用率と同時リクエスト数（基盤の既定）。SSE のストリームは同時リクエスト数を占めるので、最大同時実行数はストリームの本数を見込んで決める（M6） | Cloud Tasks の push を受けるサービスとして、同じく CPU と同時リクエスト数。キューの配送の速さは Cloud Tasks のキューのレート制御で抑える |
| Lambda | 基盤の同時実行（指標の設定なし）。予約同時実行数で上限を決める | SQS のイベントソース（基盤）。最大同時実行数で上限を決める |
| ECS | `ECSServiceAverageCPUUtilization` の目標追跡。ALB を使うなら `ALBRequestCountPerTarget` を足してよい | pull のワーカーなので、キューの滞留（サービスの指標、ADR-0055）を CloudWatch の指標にして主にし、CPU を従にする |
| Kubernetes | HPA の CPU 使用率（metrics-server。コンテナに CPU の requests を必ず書く） | KEDA でキューの滞留（DB キューなら PostgreSQL スケーラーのクエリ、またはサービスの指標、ADR-0055）を主にし、CPU を従にする |
| Docker Compose / SQLite | スケールしない（`role = all` の 1 台） | 同左 |

- **メモリはスケールの契機にしない**。Go のヒープは負荷が下がってもすぐには返らず、台数を増やしても 1 台のメモリは減らない。Cloud Run はメモリでスケールしない。
  代わりに `go.memory.used` と `go.memory.limit` の比で警報を出す
- **`GOMEMLIMIT` は運営者が設定できる項目の一つ** とする（Go のランタイムの環境変数）。アプリは cgroup のメモリの上限から自動では決めず、設定がなければ Go の既定（上限なし）のまま動く。
  デプロイのテンプレート（M6）は設定できる値として置き、目安（コンテナのメモリの上限の約 90%）を添える
- **DB の接続は台数の上限を決める**。web と worker の台数の最大 × プールの最大接続数が DB の接続の上限（Neon・Aurora・Cloud SQL・PgBouncer）を超えないように、
  デプロイのテンプレートで台数の最大を決める。`db.client.connection.pending_requests` と `wait_time` は、上限に当たっていることの警報に使う
- `GOMAXPROCS` は Go 1.25 以降の既定（cgroup の CPU の上限に従う）に任せ、テンプレートで設定しない
- スケールの契機に使うキューの滞留・遅延の指標の定義は ADR-0055 が持つ

### 3. 死活・準備の検査（FR-802）との関係

- `/healthz` と `/readyz` は指標の値を見ない。プールの待ちや goroutine の数で readiness を落とすと、全台が一斉に外れて負荷が残りの台に寄るので、閾値での切り離しはしない
- readiness の失敗（DB に届かない）は今のとおりログに残し、失敗の数をシステムの指標として出す
- 指標を引き出す形（pull）のエンドポイントを置くかとその経路は ADR-0055 が決める。置くなら worker の役割でもヘルスチェックと同じ HTTP で返し、
  ヘルスチェックの経路とは別のパスにする

### この決定を見直す条件

- ADR-0055 が計測の仕組みを持たない（基盤のログからの抽出だけにする）と決めた場合は、選択肢 1 に寄せ、プロセスの中の飽和はログの定期の出力で補う
- Cloud Run のサービスが独自の指標でのスケールに対応した場合や、web の負荷が CPU に表れない（I/O 待ちが主で CPU が上がらないまま遅延が伸びる）ことが M6 の計測で分かった場合は、
  b（DB の接続の待ちや ADR-0055 の遅延をスケールの契機にする）を見直す

## Consequences（影響）

- 良い影響
  - スケールの設定がどのターゲットでも基盤の標準の機能で書け、常駐の収集器やアダプタを必須にしない（Kubernetes で worker を滞留でスケールするときだけ KEDA が要る）
  - DB の接続の待ち・goroutine の増加・ヒープと上限の比など、基盤から見えない飽和を見つけられる
  - 計測はアダプタと `internal/platform` に閉じ、ドメインとアプリケーション層は変わらない
- 悪い影響・トレードオフ
  - CPU を契機にする web は、I/O 待ちで遅延が伸びても台数が増えないことがある。Cloud Run は同時リクエスト数で補えるが、ECS と Kubernetes は CPU の目標の調整が要る
  - Docker Compose では台数が増えないので、指標は警報と容量の見積もりにしか使えない
  - 意味規約の Development の段階の名前を使うので、規約が変わると名前の変更（v0 の破壊的変更、ADR-0012）が要る
  - Lambda はメモリの指標が標準にないので、プロセスの指標を送る費用（ADR-0055 の経路）がかかる。送らない運営者は `REPORT` のログで補う
- 後続で必要になる作業（Issue にする）
  - Go のランタイムとプロセスの指標の計測（ADR-0055 の仕組みの実装の後）
  - DB アダプタ（PostgreSQL・SQLite・libSQL）の接続のプールの指標
  - キュー・KV・ストリーム・Blob のアダプタの操作の指標と、常時の接続の数・再接続の数
  - `internal/adapter/httpclient` の外向き HTTP の指標
  - readiness の失敗の数の指標
  - [overview.md](../architecture/overview.md) の観測性の行と、[deployment-targets.md](../architecture/deployment-targets.md) のターゲットごとのスケールの設定と `GOMEMLIMIT` の記述（M6 のテンプレートと合わせる）

## Pros and Cons of the Options（詳細比較・任意）

### 選択肢 1: 基盤の指標だけ

- 良い: 実装も依存も費用もない。CPU・メモリ・台数・入口のリクエストは基盤が出す
- 悪い: DB の接続の待ち・goroutine・ヒープと上限・外向きの接続など、遅延の原因になるプロセスの中の詰まりが見えない。Docker Compose ではほぼ何も見えない。
  Lambda ではメモリが標準の指標にない

### 選択肢 2: 基盤 + プロセスの中の飽和（採用）

- 良い: 重ねて出さず、基盤から見えないものだけを足す。DB のプールの指標は既存の統計を読むだけで、クエリの経路に費用を足さない
- 悪い: アダプタごとに計測を足す作業がある。ADR-0055 の仕組みに依存する

### 選択肢 3: アプリがすべて

- 良い: どのターゲットでも同じ指標が出る
- 悪い: CPU・メモリを重ねて測る。スケールをアプリの指標で行うには、Cloud Run のサービスではできず、ECS は CloudWatch への書き込み、Kubernetes はアダプタが要り、
  費用と部品が増える（NFR-COST-2）。コンテナの CPU の上限に対する使用率は、プロセスの中からは cgroup を読まないと正しく出ない

### スケールの契機 b: システムの指標もスケールの契機にする

- 良い: I/O 待ちで CPU が上がらない負荷にも反応できる
- 悪い: DB の接続の待ちで台数を増やすと、DB の接続がさらに増えて悪化する（待ちの原因が DB 側にあるとき）。基盤ごとに独自の指標の部品が要る

## Open Questions / 要検証

- ADR-0055 が選ぶライブラリ（OpenTelemetry の Go SDK か Prometheus のクライアントか）に応じた、ランタイム・プロセスの指標のパッケージの版と依存（cgo なし、AGPL と両立するライセンス、公開から 3 日以上）
- Cloud Run で SSE のストリームを持つ web の最大同時実行数と CPU の目標の値（M6 の計測）
- Lambda Insights（メモリの指標を出す拡張）の費用と、ADR-0055 の経路で送る費用のどちらが安いか（要検証）
- Cloud Run の worker を worker pool（CREMA）に移し、キューの滞留でスケールするか（Cloud Tasks の push をやめることになるので、ADR-0004 に関わる。要検証）
