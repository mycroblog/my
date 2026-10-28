# ADR-0063: 分散トレースの方式

- Status: Accepted
- Date: 2026-10-21
- Deciders: @rosylilly
- Issue: #814

## Context（背景・課題）

決定者の指示（原文）: 「NFR-OPS-2: トレースの実現。ADR から。」

NFR-OPS-2 は「OpenTelemetry（トレース・メトリクス）は任意で有効化」と定める。メトリクスは [ADR-0055](0055-service-observability.md)（サービスの指標）と
[ADR-0056](0056-system-observability.md)（システムの指標）が `kernel.Meter` port と `internal/adapter/telemetry/` のアダプタ（`none` / `otel` / `emf`）で実現し、
ADR-0055 §9 はトレースを別の ADR に回している。この ADR はトレースの計測の手段・エクスポートの経路・コンテキストの伝播・サンプリング・属性の規則を決める。

今のサーバーの相関の手段は次の 2 つだけで、トレースはない:

- HTTP のリクエストは `request_id`（`X-Request-Id`。信頼できる値なら受け継ぎ、なければ UUIDv7）を持ち、slog の JSON ログの全行に付く（`internal/platform/log.go`・`http.go`）
- ジョブは 1 回の実行の終わりに `job_id`・`attempt` を持つ 1 行のログを出す（`internal/app/queue/runner.go`）。
  ただしジョブ（`queue.Job`、表 `jobs`・`outbox_events`・`dead_jobs`）は enqueue したリクエストの情報を何も持たないので、
  「この投稿の作成が、どの配送のジョブを生み、どれが遅れたか」をログからたどれない

関連する要件と決定:

- NFR-OPS-1（JSON ログ、リクエスト ID / ジョブ ID の相関）、NFR-OPS-2、NFR-COST-1・2（scale to zero、常駐のミドルウェアを必須にしない）、NFR-SEC-1（SSRF）
- [ADR-0002](0002-modular-monolith-hexagonal.md): コアは外部ライブラリを import しない（depguard）。[ADR-0055](0055-service-observability.md) §1 はこれに従い、OTel の型を port に出さない `kernel.Meter` を置いた
- [ADR-0004](0004-job-queue-abstraction.md)・[ADR-0003](0003-database-abstraction.md): ジョブは outbox を通って at-least-once で実行され、トランザクションは外部 I/O を持たない
- [ADR-0019](0019-security-baseline.md) §1: 連合の取得・配送など、リモートが影響する宛先への HTTP は `kernel.HTTPClient` を通る
- [ADR-0020](0020-data-modeling-principles.md): 表の分類と、状態を null 許容の時刻で表さない規則
- [deployment-targets.md](../architecture/deployment-targets.md): Cloud Run（リクエストベース課金・min instances 0）、AWS Lambda（Lambda Web Adapter）+ SQS、Self-hosted（Docker Compose / microk8s）

今決める理由: M4 の要件の監査で NFR-OPS-2 のトレースが未実現と分かった。ジョブの表とキューのアダプタ（Cloud Tasks・SQS は未実装）に伝播の口を足すなら、
M6 のデプロイのテンプレートより前に、ジョブのモデルとエクスポートの経路を決めておく必要がある。

### 一次情報で確かめた事実（2026-10-21 確認）

- **OpenTelemetry Go** のトレースの API と SDK は Stable で、ライセンスは Apache-2.0（ADR-0055 の確認）。`go.opentelemetry.io/otel/trace` と `go.opentelemetry.io/otel/sdk`（`sdk/trace` を含む）v1.47.0 はすでに `go.mod` にある。
  OTLP/HTTP のトレースのエクスポーター `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` v1.47.0 は 2026-10-02 公開で、
  `WithHTTPClient`（v1.36.0 から）で認証付きの `http.Client` を渡せ、`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` などの標準の環境変数を読む
  （[pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp)）
- **W3C Trace Context**（Recommendation、2021-11-23）: `traceparent` の sampled フラグは呼び出し側の推奨にすぎず、受け手は従わなくてよい（§3.2.2.5）。
  安全なネットワークの入口ではトレースを始め直す（restart）ことができ、sampled を付けた要求をそのまま続けるサービスは過負荷にされうる（§3.4・§7.2）。
  `tracestate` に個人を特定できる情報を入れてはならず（§6.2）、外部のシステムへ送らないことも勧める（§7.1）（[Trace Context](https://www.w3.org/TR/trace-context/)）
- **OTel の messaging の意味規約**（Development）: 非同期の処理（Process）の span は、メッセージを作ったときのコンテキストへの **link** を既定とし、
  作成のコンテキストを親にするのは単一のメッセージの場合の任意の例外とする（[Messaging spans](https://opentelemetry.io/docs/specs/semconv/messaging/messaging-spans/)）
- **Cloud Run** は受けたリクエストのトレースを自動で作り、`traceparent` を自動で埋める。サンプリングはインスタンスあたり最大 0.1 リクエスト/秒で設定できない。
  自動のトレースは課金されず、アプリが送る span は Cloud Trace の料金で課金される（[Cloud Run tracing](https://docs.cloud.google.com/run/docs/trace)）
- **Cloud Trace** は OTLP（http/protobuf・gRPC）を `https://telemetry.googleapis.com` で直接受け、Application Default Credentials で認証する。
  プロセスの中のエクスポーターからは、Cloud Trace 独自の形式より OTLP への移行を勧めている（[Migrate to the OTLP endpoint](https://docs.cloud.google.com/trace/docs/migrate-to-otlp-endpoints)）
- **AWS X-Ray** は OTLP を `https://xray.<region>.amazonaws.com/v1/traces` で受ける。HTTP だけで gRPC は受けず、認証は SigV4 で、1 リクエスト 5 MB・10,000 span まで
  （[CloudWatch OTLP endpoints](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch-OTLPEndpoint.html)）。
  X-Ray の SDK とデーモンは 2026-02-25 から保守モードで 2027-02-25 にサポートが終わり、AWS は OpenTelemetry への移行を勧める
  （[X-Ray SDK migration](https://docs.aws.amazon.com/xray/latest/devguide/xray-sdk-migration.html)）
- **Lambda** の X-Ray は `Active`（Lambda が自動で segment を送る。サンプリングは 1 リクエスト/秒 + 残りの 5% で設定できない）と `PassThrough`（既定。ヘッダーを渡すだけ）がある
  （[Lambda and X-Ray](https://docs.aws.amazon.com/lambda/latest/dg/services-xray.html)）
- **料金**: X-Ray は記録したトレースの最初の 100,000 件/月が無料（[CloudWatch pricing](https://aws.amazon.com/cloudwatch/pricing/)）。
  Cloud Trace の無料枠と単価（2.5M span/月が無料、超過は $0.20/百万 span とされる）は料金ページを取得できず **要検証**
- **他の実装**: Mastodon は OTLP のエクスポーターだけを持ち、`OTEL_EXPORTER_OTLP_ENDPOINT` がなければ計測そのものを無効にする（[Mastodon configuration](https://docs.joinmastodon.org/admin/config/)）。
  GoToSocial は設定 `tracing-enabled` と環境変数 `OTEL_TRACES_EXPORTER` で有効にする（[GoToSocial Tracing](https://docs.gotosocial.org/en/latest/advanced/tracing/)）。
  Misskey と Pleroma / Akkoma の公式の文書にはトレースの設定が見当たらない（要検証）

## Decision Drivers（判断基準）

1. **ほぼ $0 の構成を壊さない**（NFR-COST）: 既定ではトレースを出さず、今と同じ費用で動く。コレクターなどの追加の費用は、トレースを有効にした構成だけが負う
2. **個人情報と外部への漏れがない**: span の属性に利用者を特定できる値を入れない。トレースのヘッダーを連合先へ送らず、連合先・利用者から受けたヘッダーで計測を操作されない
3. **依存の向き**（ADR-0002）: ADR-0055 と同じく、コアは OTel を import せず port を通す
4. **リクエストからジョブ・配送までたどれる**: outbox を通るジョブと、Cloud Tasks・SQS のアダプタをまたいでコンテキストが続く
5. **ベンダーに縛られない**: 1 つの計測から OTLP のどの受け手（Collector・Cloud Trace・X-Ray・Jaeger・Tempo など）にも出せる
6. **実装と依存の小ささ**: `CGO_ENABLED=0`、AGPL と両立するライセンス、3 日以上前に公開された版、ADR-0055 の `otel` のアダプタと SDK を共有する

## Considered Options（選択肢）

計測の手段:

1. **A. 自前の小さな `Tracer` port + OpenTelemetry Go SDK のアダプタ**（ADR-0055 の `Meter` と同じ形）。HTTP・ジョブ・DB・外向きの HTTP の span は自前のミドルウェアとアダプタが作る
2. **B. コアから OpenTelemetry のトレースの API（`go.opentelemetry.io/otel/trace`）を直接呼ぶ**: API は既定で no-op。depguard の許可リストに足す
3. **C. OTel の contrib の自動計測（`otelhttp`・DB ドライバの計測）を境界に差し込むだけで、port を持たない**
4. **D. アプリはトレースを出さず、基盤のトレース（Cloud Run の自動のトレース、Lambda の `Active`）とログの相関だけにする**

ジョブへのコンテキストの引き継ぎ:

1. **a. ジョブの実行の span を enqueue した span の子にする**（1 つのトレースにまとめる）
2. **b. ジョブの実行を新しいトレースの根にし、enqueue した span への link を持たせる**（messaging の意味規約の既定）
3. **c. 引き継がない**（ジョブはログの `job_id` だけ）

エクスポートの経路: **OTLP/HTTP だけ** / OTLP に加えて Cloud Trace・X-Ray の独自の形式のエクスポーター / ログに span を書いて基盤に取り込ませる

## Decision（決定）

**決定: A（自前の `Tracer` port + OTel Go SDK のアダプタ）、ジョブは b（新しいトレース + link）、エクスポートは OTLP/HTTP だけ。**
サーバーは OTLP の受け手の認証を持たず、Cloud Run と Lambda ではコレクター（Cloud Run のサイドカー、ADOT の Lambda のレイヤー）を経由して Cloud Trace・X-Ray へ送る。
設定 `traces_exporter` の既定は `none` で、`none` でもログには基盤から受けた trace ID を付けて基盤のトレースと相関させる（D の利点を取り込む）。

### 1. 計測の port（ADR-0002 の依存の向き）

- `internal/app/kernel` に `Tracer` port を置く。型は標準ライブラリだけで書き、OTel の型を出さない:
  - `Start(ctx, SpanSpec, Attrs) (context.Context, Span)`・`Span.Set(key, value)`・`Span.SetName(name)`（ルートが決まってから HTTP の span の名前を付ける）・`Span.End(errorType string)`
  - `Inject(ctx) string`（現在の span の W3C `traceparent`。記録していなければ空）と `Linked(ctx, traceparent) context.Context`（次の `Start` に link を足す）、`Continue(ctx, traceparent) context.Context`（信頼したプロキシから受けた `traceparent` を次の `Start` の親にする。§3）
  - `LogIDs(ctx) (traceID, spanID string)`（ログの相関用。§6）
- span は ADR-0055 の計器と同じく、名前・種類（server / client / internal / consumer / producer）・**許す属性のキーの一覧** を `SpanSpec` で宣言してから使う。
  宣言にない属性のキーはアダプタが捨て、`testkit` の Tracer がテストを落とす
- アダプタは `internal/adapter/telemetry/` に置く:
  - `none`（既定）: span を記録しない。ただし基盤から受けた `traceparent` があれば、その trace ID を `LogIDs` で返す（§6）
  - `otel`: OTel Go SDK の `sdk/trace` と OTLP/HTTP のエクスポーター（`otlptracehttp`）。resource は ADR-0055 §6 と同じ（`service.name`・`service.version`・`my.role`）
- span を作る場所は境界とアダプタに限る。ドメイン（`internal/domain`）は計測しない。ADR-0055 §1 で計測のある場所と同じ:
  - HTTP のサーバー: `internal/platform` のミドルウェア（`accessLog` と同じ場所）。名前は `<method> <http.route>`
  - ジョブの実行: `internal/app/queue` の runner（`Meter.Begin` と同じ場所）。名前は `my.job.type`
  - 外向きの HTTP: `internal/adapter/httpclient`（`kernel.HTTPClient`）。連合の取得・配送、WebFinger、メディアのプロキシ、リンクのカード、Web Push
  - DB: `UnitOfWork.Tx` の 1 回の実行（再試行ごと）と、sqlc の 1 クエリ。PostgreSQL は pgx の `QueryTracer`、SQLite・libSQL は sqlc の `DBTX` を包むラッパーで作る（新しい依存なし）
  - Inbox の受理・配送の 1 回・ストリームの publish（`internal/app/federation`・`stream`）は、必要なら port で internal の span を足す

B を選ばない理由: 最も強い利点は port の重複がないことと、contrib の計測をそのまま使えることである。ただし ADR-0055 が `Meter` で避けた ADR-0002 の例外を作り、
§4 の属性の許可リストを型で強制できない。port は span の開始・属性・終了・伝播の 4 つで小さい。

C を選ばない理由: `otelhttp` のサーバーの計測は意味規約に従って `client.address`・`user_agent.original`・`url.path` などを属性に入れ、§4 の規則に反する。
外向きの `otelhttp` は既定で `traceparent` を宛先へ送り、§3 の規則に反する。境界の span は自前で書いても数か所で、属性を選べる。

D を選ばない理由: 費用は最も小さいが、基盤の span は HTTP の入口だけで、DB・ジョブ・配送の内訳が見えない。Self-hosted には基盤のトレースがない。
ただし D の考え方は `none` の既定の挙動（ログの相関）に取り込む。

### 2. ジョブへの引き継ぎ（b: 新しいトレース + link）

- `queue.Job`・`queue.NewJob` に `TraceParent`（W3C `traceparent` の文字列、空も可）を足す。`JobQueue.Enqueue` と `Outbox.EnqueueTx` は、呼び出しの `ctx` から `Tracer.Inject` で埋める（呼び出し側は何もしない）
- 保存の場所: 表 `jobs`・`outbox_events`・`dead_jobs` に列 `trace_parent text`（null 許容）を足す（ADR-0023 の `schema/<dialect>.sql`）。
  これは状態でなくジョブの属性で、ADR-0020 の「状態を null 許容の時刻で表さない」に当たらない。outbox のリレーは列をそのまま次のキューへ渡す。
  Cloud Tasks はタスクの HTTP の `traceparent` ヘッダー、SQS はメッセージ属性 `traceparent` で運ぶ（どちらもアダプタの実装のとき）
- runner はジョブの実行の span（種類 consumer）を **新しいトレースの根** として作り、`TraceParent` への link を持たせる。再試行は試行ごとに別の span で、属性 `my.job.attempt` を持つ
- サンプリングは link に従う: link 先が sampled ならジョブも記録し、そうでなければ §5 の比率で決める（OTel の `Sampler` は link を見られる）。これで記録したリクエストの続きのジョブは欠けない

a を選ばない理由: 1 つのトレースで最も見やすいが、1 つの投稿が数百〜数千の配送のジョブに分かれ、再試行は数時間に及ぶ（ADR-0004 の指数バックオフ）。
1 つのトレースが際限なく大きく長くなり、受け手の上限（X-Ray は span の時刻が 14 日以内、1 リクエスト 10,000 span）と表示の使い勝手を損ねる。
c を選ばない理由: NFR-OPS-1 の相関がリクエストとジョブの間で切れたままになる。

### 3. 伝播の境界（信頼と漏れ）

| 境界 | 受け取る `traceparent` | 送る `traceparent` |
| --- | --- | --- |
| 公開のリスナー（クライアント API・Web・OAuth・Inbox・メディア） | 接続の相手が `trusted_proxies`（ADR-0019 §5）のときだけ親として使う。それ以外は捨てて新しいトレースを始める（W3C §3.4 の restart）。`tracestate` は常に捨てる | 応答に付けない |
| キューの push の受け口（Cloud Tasks の HTTP の受け口、Lambda の SQS のイベント） | 認証を通ったものだけ link として使う（§2） | — |
| `kernel.HTTPClient`（連合・WebFinger・メディア・リンクのカード・Web Push） | — | **付けない**。リモートのサーバーに自分の trace ID・サンプリングの判断を渡さない（W3C §7.1）。client の span はプロセスの中だけで作る |
| 運用者が設定したクラウドの API（S3・GCS・Pub/Sub など） | — | 付けない（SDK の計測は入れない。§7 の後続で再検討） |

- 受け取った sampled フラグにはそのまま従わず、§5 の親に従う sampler で判断する。信頼しない入口で restart するので、外から sampled を付けて記録の量を増やされない（W3C §7.2）

### 4. 属性の規則（個人情報）

ADR-0055 §6 のラベルの規則を span の属性にも当てはめ、トレースは時系列でないので濃度の制限だけを次のように緩める:

- 許す: HTTP の `http.request.method`・`http.route`（パターン）・`http.response.status_code`・`my.surface`、ジョブの `my.job.type`・`my.job.attempt`・`my.outcome`、
  `activity.type`、DB の `db.system.name`・`db.operation.name`（sqlc のクエリ名）、`error.type`（エラーの種類の有限の名前）、
  外向きの HTTP の `server.address`（宛先のホスト名だけ。運用者が配送の失敗を調べるため）と `http.response.status_code`
- 入れない: アカウント・投稿・ジョブの ID、ユーザー名、URL の全体・パス・クエリ（`url.full`・`url.path`。Actor の URL はユーザー名を含む）、クライアント IP、User-Agent、
  OAuth のアプリケーション、SQL の本文とパラメーター（`db.query.text`）、エラーの文字列（URL・ユーザー名を含みうるので `error.type` だけにする）、リクエスト・応答の本文
- span のイベント（`AddEvent`）と例外の記録（`RecordError`）は port に出さない。ジョブ ID・リクエスト ID との対応は §6 のログで取る
- `Tracer` の `SpanSpec` の許可リストと、`testkit` のテストでこの規則を強制する

### 5. エクスポートの経路とサンプリング（NFR-COST）

設定（`internal/platform/config`、[configuration.md](../reference/configuration.md) を再生成）:

- `traces_exporter`: `none`（既定）| `otlp`。`otlp` は OTLP/HTTP（protobuf、gzip）で、宛先は OTel の標準の環境変数 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` などで決める（ADR-0055 の `metrics_exporter = otlp` と同じ流儀）
- `traces_sample_ratio`: 根の span を記録する割合（既定 0.01）。sampler は「親（信頼した入口の親、ジョブの link）に従い、親がなければ trace ID の比率」
- `traces_flush`: `batch`（既定。SDK の BatchSpanProcessor が周期的に送る）| `unit`（HTTP のリクエスト・ジョブの終わりに、そこで記録した span があれば送り切ってから返す）
- サーバーは OTLP の受け手の認証（Google の Application Default Credentials、AWS の SigV4）を持たない。Cloud Trace・X-Ray の受け口が求める認証は、ターゲットのコレクターが行う

| ターゲット | 設定 | 経路 | 費用 |
| --- | --- | --- | --- |
| 既定・小さな Self-hosted | `none` | ログの `request_id`・`job_id`・`trace_id`（§6） | 増えない |
| Self-hosted（Collector・Jaeger・Tempo などを運用者が動かす） | `otlp`・`batch` | OTLP/HTTP で受け手へ。tail sampling（エラーと遅いトレースを残す）は Collector で行える | 受け手の分 |
| Google Cloud（Cloud Run） | `none`、またはトレースを有効にするなら `otlp`・`unit` | `none` は Cloud Run の自動のトレース（無料）とログの相関。`otlp` は同じインスタンスの **OpenTelemetry Collector のサイドカー**（`localhost:4318`）へ送り、サイドカーが ADC で `telemetry.googleapis.com` へ送る | サイドカーの資源と span の量（要検証）。サイドカーが CPU の割り当てのない間も送れるよう、トレースを有効にする構成はインスタンスベース課金にする（ADR-0055 の確認と同じ理由） |
| AWS（Lambda） | `none`、またはトレースを有効にするなら `otlp`・`unit` | **ADOT の Lambda のレイヤー**（Collector の拡張機能、`localhost:4318`）へ送り、レイヤーが SigV4 で `xray.<region>.amazonaws.com/v1/traces` へ送る | レイヤーの分の起動時間と、記録したトレース 100,000 件/月まで無料、以降は従量（要検証） |

- リクエストベース課金の Cloud Run と、凍結される Lambda では、BatchSpanProcessor の周期的な送信を当てにできない（ADR-0055 の確認と同じ理由）。そこで `unit` でローカルのコレクターへ送り切る。
  送り先はループバックなので遅れは小さく、記録しないリクエスト（既定で 99%）は送信しない
- 既定の比率 0.01 は、1 日 10 万リクエストのサーバーで根のトレースが 1 日約 1,000 件（月約 3 万件）になり、X-Ray の無料枠に収まる見込みで選ぶ（span の数はトレースあたり十数個。要検証: M6 で実測）

### 6. ログとの相関（NFR-OPS-1）

- slog のハンドラー（今の `requestIDHandler`）は、`Tracer.LogIDs` が返す `trace_id`・`span_id` もログの全行に付ける。`request_id`・`job_id` は今のまま残す
- `traces_exporter = none` でも、信頼した入口（§3）の `traceparent` の trace ID をログに付ける。Cloud Run では自動のトレースとログが同じ trace ID でつながる
  （Cloud Logging が trace のフィールドとして解釈するには `logging.googleapis.com/trace` の形 `projects/<id>/traces/<trace_id>` が要る。デプロイのテンプレートでプロジェクト ID を渡すかは M6 で決める。要検証）

### 7. 依存とライセンス

- 新しい直接の依存は `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`（Apache-2.0、v1.47.0 は 2026-10-02 公開。ADR-0055 の OTLP のメトリクスのエクスポーターと同じ版の系列）だけ。
  `otel/trace`・`otel/sdk` は既にあり、W3C の propagator は `go.opentelemetry.io/otel/propagation` にある。contrib（`otelhttp`・AWS の X-Ray の propagator）は使わない
- すべて純 Go で、`CGO_ENABLED=0` を保つ。gRPC のエクスポーターは使わない（ADR-0055 §2 と同じ）

### 覆る条件

- コレクターのサイドカー・ADOT のレイヤーの資源や起動時間の費用が、想定の規模で無視できない（M6 で計測）なら、アダプタが受け口の認証を持って直接送ることを再検討する
- OTel の messaging の意味規約が Stable になって、非同期の処理の既定を親子に変えたら、§2 を見直す
- 記録したリクエストの `unit` の遅れが利用者に見えるほど大きい（M6 で計測）なら、インスタンスベース課金の Cloud Run を `batch` に変える

## Consequences（影響）

- 良い影響
  - 既定（`none`）は今と同じ費用で、ログに trace ID が付くぶんだけ Cloud Run の基盤のトレースとの相関が増える
  - リクエストからジョブ・配送まで、link でたどれる。ファンアウトがトレースを肥大させない
  - トレースのヘッダーが連合先へ出ず、外部から記録の量を操作されない。属性の許可リストで個人情報が受け手に残らない
  - サーバーは OTLP/HTTP を 1 つ持つだけで、受け口の認証とベンダーごとの差はコレクターが吸収する。Self-hosted は任意の OTLP の受け手を使える
- 悪い影響・トレードオフ
  - port と SDK の二重の層を持ち、HTTP・DB の span を自前で書く。contrib の自動計測の恩恵（新しい意味規約への追従）を受けない
  - ジョブの表 3 つに列が増え、Cloud Tasks・SQS のアダプタはヘッダー・属性を運ぶ必要がある
  - link の方式は、トレースの表示で 1 つの画面にまとまらない。受け手が link をたどれるか（Cloud Trace・X-Ray の表示）は受け手次第（要検証）
  - 比率のサンプリング（head sampling）では、まれなエラーのトレースを取り逃しうる。確実に残すには Collector の tail sampling が要る
  - `unit` は記録したリクエストの応答を送信の分だけ遅らせる
  - `server.address` は連合先のホスト名を受け手に残す。1 人のサーバーの連合先は利用者の交友を示しうるので、運用者の受け手の扱いに依存する
  - Cloud Run と Lambda でトレースを有効にすると、コレクターのサイドカー・ADOT のレイヤーの資源と起動時間が増え、Cloud Run はインスタンスベース課金になる
- 後続で必要になる作業（Issue 化する）
  - `kernel.Tracer` port と `SpanSpec`、`none` / `otel` のアダプタ、`testkit` の Tracer、設定（`traces_exporter`・`traces_sample_ratio`・`traces_flush`）と configuration.md の再生成
  - HTTP のミドルウェア・ジョブの runner・`kernel.HTTPClient`・`UnitOfWork.Tx`・クエリ（pgx の `QueryTracer`、SQLite・libSQL の `DBTX` のラッパー）の span
  - `queue.Job` の `TraceParent`、`schema/<dialect>.sql` の `jobs`・`outbox_events`・`dead_jobs` の `trace_parent`、DB キューと outbox のリレーの引き継ぎ、link に従う sampler
  - 入口の `traceparent` の信頼（`trusted_proxies`）と、ログの `trace_id`・`span_id`
  - Cloud Tasks・SQS のアダプタの実装のとき、`traceparent` のヘッダー・メッセージ属性
  - M6: デプロイのテンプレートの推奨の設定（Cloud Run の Collector のサイドカー、Lambda の ADOT のレイヤー）、Cloud Run のログの trace のフィールド、費用の実測を [deployment-targets.md](../architecture/deployment-targets.md) に書く
  - [overview.md](../architecture/overview.md) の port の表（`Meter` の行の「トレースは持たない」）を `Tracer` に合わせて直す

## Pros and Cons of the Options（詳細比較・任意）

| 観点 | A. port + OTel SDK | B. OTel API を直接 | C. contrib の自動計測 | D. 基盤のトレースとログだけ |
| --- | --- | --- | --- | --- |
| ADR-0002 との整合 | ◎ | △（depguard に例外） | △ | ◎ |
| 属性の許可リストの強制 | ◎（`SpanSpec`） | △（レビュー頼み） | ×（既定で IP・UA・パス） | ◎ |
| 連合先への `traceparent` の漏れ | ◎（付けない） | ○ | ×（既定で付ける） | ◎ |
| DB・ジョブ・配送の内訳 | ◎ | ◎ | ○ | × |
| Self-hosted | ◎ | ◎ | ◎ | ×（基盤のトレースがない） |
| 実装量 | 中 | 小 | 最小 | 最小 |

| 観点 | a. 親子 | b. 新しいトレース + link | c. 引き継がない |
| --- | --- | --- | --- |
| リクエストからジョブへたどれる | ◎（1 画面） | ○（link） | × |
| ファンアウト・長い再試行でのトレースの大きさ | ×（際限なし） | ◎ | ◎ |
| OTel の messaging の意味規約 | △（単一メッセージの例外） | ◎（既定） | — |

## Open Questions / 要検証

- 要検証: Cloud Trace の料金と無料枠（料金ページを取得できなかった）、X-Ray の OTLP の受け口で取り込んだ span の課金（Transaction Search を有効にする必要の有無）
- 要検証: Cloud Run の Collector のサイドカーと ADOT の Lambda のレイヤーの資源・起動時間への影響と、ADOT のレイヤーが凍結の前に送り切ること
- 要検証: Cloud Run が公開のリクエストの `traceparent` を常に上書きするか（上書きするなら Cloud Run の前段を `trusted_proxies` として扱える）、Lambda Web Adapter がアプリへ `traceparent` / `X-Amzn-Trace-Id` を渡すか
- 要検証: Cloud Trace・X-Ray の表示で span の link をたどれるか
