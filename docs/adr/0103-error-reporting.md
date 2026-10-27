# ADR-0103: エラーの報告（ErrorReporter port・指紋・報告先・運営者への通知）

- Status: Proposed
- Date: 2026-10-27
- Deciders: @rosylilly
- Issue: #1432

## Context（背景・課題）

決定者の指示（原文）: 「現在 Pull 型の Observability については機能提供できていると思いますが、Sentry や Datadog などのサービスや管理系へエラーレポートを送るなどの Push 型の Observability については機能保証が足りないと思っています。（中略）どういう形式の何を提供すればエラー発生時にきちんと運営者が気づき、改善していけるかを検討して機能開発の提案をしてください」

メトリクス（[ADR-0055](0055-service-observability.md)・[ADR-0056](0056-system-observability.md)）、トレース（[ADR-0063](0063-distributed-tracing.md)）、
構造化ログ（NFR-OPS-1）、外形監視（[ADR-0085](0085-uptime-monitoring-status-page.md)）、Prometheus のアラートと runbook（`docs/operations/alerts.md`）はある。
どれも「件数と割合」を見る仕組みで、1 つ 1 つのエラーを運営者に届け、原因の場所まで案内する仕組みはない。今のコードには次の穴がある:

- **しきい値に届かないエラーは誰も気づかない**: 5xx のアラートは割合（1% が 30 分など）で鳴る。利用者の少ないインスタンスでは、ある画面が毎回 500 を返しても鳴らないことがある
- **場所が分からない**: スタックトレースをどこにも残さない。HTTP の panic（`internal/platform/http.go` の `recoverPanics`）とジョブの panic（`internal/app/queue/runner.go`）は panic の値だけを、
  ハンドラーのエラー（`clientapi` の `responseError`、`authpages` の `internalError`）とジョブの失敗は `err.Error()` の文字列だけをログに書く
- **まとまらない**: 同じ原因のエラーを 1 つにまとめる指紋（fingerprint）がなく、初めて出たエラーとリリースの後に再発したエラーを区別できない
- **落ちたプロセスは理由を残さない**: recover しない goroutine（`go func()` が 12 か所ほど）で panic すると、プロセスが落ち、外からは再起動（`MyProcessRestarting`）しか見えない
- **Web クライアントのエラーは届かない**: `AppLayout.tsx` の `ErrorBoundary` は `fallback={null}` で何も報告せず、`window.onerror`・`unhandledrejection` も拾わない
- **知らせる経路がない**: instance webhook（[ADR-0060](0060-instance-webhooks.md)）と管理者の通知（FR-712）にシステムの出来事がない。Sentry・Datadog などへ送る口もない
- **Cloud Logging が深刻度を読めない**: slog の JSON は `level` を書き、Cloud Logging の構造化ログの特別なフィールド `severity` を書かない（要検証: Cloud Run の構造化ログの文書）

関連する要件と決定:

- NFR-OPS-1（JSON ログ・相関）、NFR-OPS-2（OpenTelemetry は任意で有効化）、NFR-OPS-3（管理画面で失敗ジョブ・状態を確認）、NFR-COST-1・2（scale to zero、常駐のミドルウェアを必須にしない）
- [ADR-0002](0002-modular-monolith-hexagonal.md): コアは外部ライブラリを import しない
- [ADR-0019](0019-security-baseline.md): 内部の情報（エラーの文言・ホスト名）を応答に出さない。リモートが影響する宛先への HTTP は `kernel.HTTPClient` を通る
- [ADR-0020](0020-data-modeling-principles.md): 表の分類、状態を null 許容の時刻で表さない
- ADR-0055 §6・ADR-0063 §4: ラベルと span の属性に個人情報と無限の値を入れない。ADR-0063 §4 は `RecordError` とエラーの文字列を span に出さない
- ADR-0060: instance webhook のイベント・署名・配送。FR-712: 管理者へのアプリ内の通知

今決める理由: M5 で v0.1.0 を公開し、決定者のインスタンスの運用を始める。運用の初日から、外部のサービスを契約していなくても「新しいエラーが出た」ことに気づける必要がある。
また v0.1.0 からはスキーマの変更に互換が要る（[ADR-0087](0087-schema-changes-after-launch.md)）ので、エラーを記録する表は公開の前に決めておく。

### 一次情報で確かめた事実（2026-10-27 確認）

- **Google Cloud Error Reporting** は Cloud Logging の構造化ログからエラーを拾う。`jsonPayload` の `stack_trace`・`exception`・`message` の順にスタックトレースを探し、
  `@type` を `type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent` にすると `message` だけでも拾う。
  `ReportedErrorEvent` は `serviceContext.service`（必須）・`serviceContext.version`・`message`（必須）・`context.reportLocation`（スタックがないとき必須）を持つ
  （[Formatting errors in logs](https://docs.cloud.google.com/error-reporting/docs/formatting-error-messages)）。
  Cloud Run ではスタックトレースを含み、深刻度が未設定か `ERROR` 以上のログからエラーを作る（[Go の設定](https://docs.cloud.google.com/error-reporting/docs/setup/go)）。
  受け付ける Go のスタックの形（`debug.Stack` の形）は文書に明記がなく **要検証**。Error Reporting そのものの料金は公式の料金ページを確かめられず **要検証**（ログの取り込みと保存は Cloud Logging の料金）
- **sentry-go** は MIT で、v0.50.0 は 2026-10-08 公開（[pkg.go.dev](https://pkg.go.dev/github.com/getsentry/sentry-go)）
- **Datadog Error Tracking** は Sentry の SDK のイベントを Datadog 専用の DSN（`https://<TOKEN>@sentry-intake.<DD_SITE>/1`）で受ける。試験済みの Go の SDK は `sentry-go` v0.32.0 で、
  エラーのイベントとメッセージだけを受け（トレース・添付・セッションは受けない）、全イベントに `service` のタグが要る。Error Tracking for Logs を有効にする必要がある
  （[Datadog: Sentry SDK](https://docs.datadoghq.com/error_tracking/guides/sentry_sdk/)）。提供の段階（preview か GA か）と料金は文書になく **要検証**
- **GlitchTip** は Sentry の取り込みの API を実装し、Sentry の SDK を DSN の差し替えだけで使えるとされ、ライセンスは MIT とされる（二次情報。**要検証**: GlitchTip のリポジトリの LICENSE）
- **OpenTelemetry の例外の意味規約**: 例外は LogRecord の属性として記録する（SHOULD）。属性 `exception.type`・`exception.message`・`exception.stacktrace` は Stable で、
  `exception.message` は機微な情報を含みうると注記されている。span のイベントとしての記録は既定のまま残り、`OTEL_SEMCONV_EXCEPTION_SIGNAL_OPT_IN` でログへ移す段階にある
  （[Exceptions in logs](https://opentelemetry.io/docs/specs/semconv/exceptions/exceptions-logs/)）。
  OTel Go のログの SDK（`go.opentelemetry.io/otel/sdk/log`）と OTLP/HTTP のログのエクスポーター（`otlploghttp`）の安定性と版は **要検証**

## Decision Drivers（判断基準）

1. **外部のサービスがなくても気づける**: 既定の設定・追加の費用なしで、新しいエラーと再発が運営者に届く（NFR-COST）。外部のサービスは選べる追加にする
2. **個人情報を外へ出さない**: 報告に URL・ユーザー名・メールアドレス・IP・投稿の本文を入れない。外部へ送る前に必ず除去する（ADR-0019・ADR-0063 §4 と同じ考え方）
3. **原因の場所まで案内する**: スタック・起きた場所（ルートのパターン・ジョブの型）・版・`request_id`・`trace_id` から、ログとトレースへたどれる
4. **ノイズで埋もれない**: 同じ原因は 1 つにまとめ、自分たちの不具合でない出来事（連合先の失敗・4xx）は報告しない。エラーの嵐でも通知と送信の量は上限を持つ
5. **依存の向きとベンダーに縛られない**（ADR-0002）: コアは port だけを呼び、報告先はアダプタで差し替える
6. **実装と依存の小ささ**: `CGO_ENABLED=0`、AGPL と両立するライセンス、build tag で外せる

## Considered Options（選択肢）

1. **A. 自前の `ErrorReporter` port + 組み込みのエラーのグループ（DB）と通知 + 外部への転送のアダプタ（Sentry 互換・OTLP）**
2. **B. コアから Sentry の SDK（`sentry-go`）を直接呼び、グルーピングと通知は Sentry（互換のサービス）に任せる**
3. **C. ログだけ**: エラーの出来事をスタック付きの構造化ログ 1 行で書き、グルーピングと通知は基盤（Cloud Error Reporting、CloudWatch のメトリクスフィルター、Loki のアラート）に任せる
4. **D. OpenTelemetry だけ**: 例外を OTel のログ（または span のイベント）として OTLP で出し、エラーの追跡はバックエンド（Datadog・Grafana・Honeycomb など）に任せる

## Decision（決定）

> Proposed の間は「推奨案」として書く。Accepted にするのは人間の Decider のみ。

**推奨案: A。** エラーは「スタックと指紋を持つ 1 つの出来事」として port に渡し、
(1) 必ずログに 1 行書き、(2) DB のエラーのグループに数えて新しいグループと再発を管理者の通知と webhook で知らせ、(3) 設定したときだけ外部（Sentry 互換・OTLP）へ転送する。
ADR-0063 §4（span に `RecordError` とエラーの文字列を出さない）は変えない。エラーの報告は span と別の信号で、除去の規則（§3）を通ったものだけを出す。

B を選ばない理由: B の最も強い利点は、グルーピング・再発の検知・画面・ソースマップ・通知の規則が完成していて、Datadog も同じ SDK と DSN で受けられることである。
ただし外部のサービスを使わない運営者には何も届かず（判断基準 1）、コアが SDK を import して ADR-0002 に例外を作る。A は Sentry 互換の転送を持つので、B の利点の多くを含む。

C を選ばない理由: 費用は最も小さく、Cloud Run では Error Reporting がほぼそのまま使える。ただし AWS と Self-hosted には同じ水準のグルーピングと通知が基盤にない。
C の考え方（スタック付きの 1 行のログ、Error Reporting が拾う形）は A の §4 の既定の出力として取り込む。

D を選ばない理由: どのバックエンドにも出せるが、コレクターなどが要り、既定の構成では何も届かない。A の転送のアダプタの 1 つ（`otlp`）として取り込む。

### 1. port とエラーの出来事（ADR-0002）

- `internal/app/kernel` に `ErrorReporter` port を置く。型は標準ライブラリだけで書く:
  - `Report(ctx, ErrorEvent)`: 出来事を渡す。戻り値はなく、報告の失敗は報告しない（自分自身のエラーで再帰しない）
  - `Recover(ctx, where string)`: `defer` で呼び、panic を `ErrorEvent` にして報告し、再び panic させない（HTTP・ジョブ・goroutine の境界で使う）
- `ErrorEvent` が持つもの:
  - `Source`: `server` / `web`
  - `Kind`: `panic` / `error`
  - `Type`: エラーの型の連鎖（`errors.Unwrap` でたどった Go の型名。例 `*fmt.wrapError > *pgconn.PgError`）と、panic の値の型
  - `Message`: エラーの文字列（§3 で除去してから外へ出す）
  - `Stack`: panic は `debug.Stack()`。エラーは報告した地点のスタック。境界（アダプタの戻り値など）で `kernel.WithStack(err)` を使うと、作った地点のスタックを持たせられる
  - `Where`: 起きた場所の有限の名前。HTTP は `my.surface` と `http.route`（パターン）、ジョブは `my.job.type`、goroutine は起動したときの名前（例 `stream.valkey.subscribe`）
  - 相関: `request_id`・`job_id`・`attempt`・`trace_id`・`span_id`（ADR-0063 §6）
  - 版と役割: `service.version`・`my.role`・設定 `errors_environment`（既定 `production`）
- 指紋（fingerprint）: `Source`・`Where`・`Type` の最後の型・スタックのうち本リポジトリ（`github.com/mycroblog/my/`）の関数名の上位 5 つ（行番号は含めない）から SHA-256 を作り、16 進の先頭 32 文字にする。
  本リポジトリの関数がスタックにないときは、§3 で正規化した `Message` を代わりに使う。行番号を含めないので、無関係な行の追加では指紋が変わらない

### 2. 報告するもの・しないもの

| 報告する | 報告しない（メトリクスとログのまま） |
| --- | --- |
| HTTP のハンドラーの panic と、ハンドラーが返したエラーによる 5xx（`clientapi`・`authpages`・ActivityPub・OAuth・管理・メディア） | 4xx（入力の誤り・認可・レート制限）、メンテナンスモードの 503 |
| ジョブの panic と `dead`（再試行を使い切った失敗） | ジョブの `retry`（一時的な失敗。使い切ったら `dead` で報告する） |
| goroutine の panic（§5） | 連合先が原因の失敗（配送の `retry`・`gone`・`failed`、Inbox の署名の拒否、リモートの取得の失敗） |
| 起動の失敗（設定・DB・アダプタの初期化） | readiness の失敗（ADR-0056 §3 のとおり指標とアラート） |
| Web クライアントの捕まえていないエラー（§6） | 利用者のネットワークの切断、ブラウザの拡張機能が出したエラー（スタックに自分の origin のファイルがないもの） |

- 外部の adapter（メール・blob・決済・SFU など）の失敗は、それによってジョブが `dead` になるか 5xx を返したときだけ報告する。一時的な失敗は ADR-0056 の `my.adapter.*` の指標で見る

### 3. 個人情報の除去と量の上限

- 除去（scrub）は port の実装の入口で 1 回行い、ログ・DB・外部のどの出力にも除去した後の値だけを渡す（今のリクエスト・ジョブの 1 行のログの `error` は今のまま）:
  - `Message` の URL・メールアドレス・`@user@host` の形・IP アドレス・UUID と ULID・10 桁以上の数字を `<url>`・`<email>`・`<acct>`・`<ip>`・`<id>`・`<n>` に置き換え、1 KiB で切る
  - `Stack` は関数名とファイルの位置だけで、引数の値（Go のスタックの 16 進の引数）を消す
  - リクエストの本文・ヘッダー・クエリ・クッキー・アカウントの ID・ユーザー名・IP・User-Agent は持たない（ADR-0055 §6 と同じ）
- 量の上限: プロセスごとに、同じ指紋は 60 秒に 1 回だけ外部へ転送し、残りは数だけ数える。外部への転送は全体で毎分 60 件まで。上限を超えた分は DB の数とログには残る

### 4. 出力の経路

**(1) ログ（常に）**: 出来事ごとに 1 行の JSON を stderr に書く。Cloud Error Reporting が拾う形にする（他の基盤では普通の JSON の 1 行として扱える）:

```json
{"level":"ERROR","severity":"ERROR","msg":"error report","@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent",
 "message":"<Type>: <除去した Message>\n\n<Stack>","serviceContext":{"service":"my","version":"<service.version>"},
 "my.error.fingerprint":"…","my.error.source":"server","my.error.kind":"panic","my.error.where":"client_api GET /api/v1/statuses/{id}",
 "request_id":"…","trace_id":"…"}
```

- あわせて、すべてのログの行に Cloud Logging の `severity`（`DEBUG`・`INFO`・`WARNING`・`ERROR`）を足す。`level` は今のまま残す（アラートとログベースの指標の例が使っている）

**(2) エラーのグループ（常に。DB）**: §7 の表に指紋ごとに数え、新しいグループと、解決したグループの再発を §8 で知らせる。

- 書き込みはトランザクションの外の短い 1 文の upsert にする。プロセスの中で指紋ごとに数を貯め、最初の 1 件はすぐ、残りは 60 秒ごと（とプロセスの終わり）にまとめて書く。
  リクエストベース課金の基盤で貯めた数が失われることは受け入れる（正確な件数はメトリクスとログが持つ）
- DB に書けないとき（DB の障害そのものが原因のとき）は、捨ててログにだけ残す

**(3) 外部への転送（設定 `errors_exporter`、既定 `none`）**:

| 値 | 送り先 | 実装 |
| --- | --- | --- |
| `none` | 送らない | — |
| `sentry` | Sentry 互換の DSN（設定 `errors_sentry_dsn`、秘密）: Sentry（SaaS・セルフホスト）、GlitchTip、Datadog Error Tracking の Sentry 用の DSN | `internal/adapter/errorreport/sentry`。`sentry-go` の `sentry.NewClient` をグローバルな hub なしで使い、既定の統合（integrations）を切り、`BeforeSend` で §3 を通ったことを確かめる。タグ `service`（Datadog に必須）・`environment`・`release`・`my.role`・`my.error.where` を付ける。build tag `no_sentry` で外せる |
| `otlp` | OpenTelemetry Collector など（宛先は OTel の標準の環境変数） | `internal/adapter/errorreport/otlp`。OTel の例外の意味規約に従う LogRecord（`exception.type`・`exception.message`・`exception.stacktrace`、深刻度 `ERROR`、span のコンテキスト）を OTLP/HTTP で送る。ADR-0055 の `no_otel` で外れる |

- 転送はトランザクションの外で、送信のキューを持つ非同期の送信者が行う（キューの上限 1,000 件。あふれたら捨てて数える）。プロセスの終わりに最大 2 秒待って送り切る
- 宛先は運営者が設定するサービスの API なので、`kernel.HTTPClient`（SSRF のガード）でなく、アダプタの中の SDK・エクスポーターの HTTP クライアントを使う（AGENTS.md の外向きの HTTP の規則）
- 転送の成否は指標 `my.errors.exported`（Counter、`my.outcome` = `success` / `dropped` / `failed`）で見る

### 5. goroutine の起動

- `kernel.Go(ctx, name, reporter, fn)` を置き、`fn` の panic を `Recover` で報告してから、プロセスの終了の手順（`serve.go` の停止）に渡す。panic の握りつぶしで壊れた状態のまま動き続けないようにする
- `internal/` と `cmd/` の `go` 文はこの関数に置き換える。`go` 文を禁止する検査（golangci-lint の規則か `agentkit` の検査）を足すかは実装の issue で決める

### 6. Web クライアントのエラー

- 報告の API `POST /api/v1/client_errors` を足す（クライアント API・ADR-0029 の部分集合。ログインしていなくても呼べる）。本文は 16 KiB まで:
  `name`・`message`・`stack`（ブラウザの文字列）・`release`（ビルドの版）・`route`（TanStack Router のルートの ID。URL でなくパターン）・`kind`（`error_boundary` / `onerror` / `unhandledrejection`）
- サーバーは本文を `Source = web` の `ErrorEvent` にして §3 を通す。指紋はスタックの自分の origin のファイルの関数名（なければ正規化した `message`）と `route` から作る。
  応答は常に `204`（失敗の理由を返さない）
- 乱用の対策: IP ごとのレート制限（毎分 10 件、ADR-0019 のレート制限の仕組み）。ログインしていない呼び出しは数とログに残すが、新しいグループの通知（§8）を起こさない
- クライアントは `ErrorBoundary` の `onError`・`window.onerror`・`unhandledrejection` で捕まえ、同じエラーは画面の表示 1 回につき 1 回だけ、`navigator.sendBeacon`（なければ `fetch` の `keepalive`）で送る。
  自分の origin のファイルがスタックにないエラーは送らない
- 送り先は自分のサーバーだけで、CSP の `connect-src` を増やさない。ソースマップを使った復元は持たない（§10）

### 7. データモデル（ADR-0020）

新しいモジュール `diagnostics`（`internal/domain/diagnostics`・`internal/app/diagnostics`）が所有する。スキーマは `schema/<dialect>/NN-diagnostics.sql`。

| テーブル | 分類 | 内容 |
| --- | --- | --- |
| `error_groups` | リソース | 指紋（一意）・`source`・`kind`・最後の型・除去した `message` の先頭・`where`・最初に見た時刻と版 |
| `error_group_counts` | 現在値 | グループと 1 時間の区切りごとの件数・その時間の最後の時刻と版（upsert で足す）。90 日より古い行は定期のジョブで消す |
| `error_group_samples` | 現在値 | グループごとの最新の 1 件: 除去したスタックと `message`・`request_id`・`trace_id`・版・役割 |
| `error_group_events` | イベント | 管理者の操作と自動の出来事: `resolved`（解決にした）・`regressed`（解決の後に再び起きた）・`muted`・`unmuted`、行った人（`actor_kind = system` を含む） |
| `error_group_states` | 投影 | イベントから作るグループの今の状態（`open` / `resolved` / `muted`） |

- 新しいグループの作成と `regressed` の記録は、同じ短いトランザクションでドメインイベントを発行し、§8 の通知と webhook が同期の購読者として受ける（ADR-0060 と同じ経路）
- `regressed` は、`resolved` のグループに、解決にしたときより新しい版（`service.version` が異なる）で再び起きたときに記録する。同じ版での再発は数えるだけにする（デプロイ前の残りを再発と呼ばない）

### 8. 運営者への通知と管理画面

- 新しい権限 `manage_errors`（ADR-0070 の権限の分け方。`admin` のロールに含める）を足す。管理画面のエラーの一覧と、通知・webhook の購読に要る
- 管理者の通知（FR-712）の新しい種類 `admin_error_group`: `manage_errors` を持つ人に、新しいグループ（`server` か、ログインした利用者からの `web`）と `regressed` を知らせる。
  インスタンス全体で 1 時間に 10 件までとし、超えた分は「ほかに N 件」の 1 件にまとめる。`muted` のグループは知らせない
- instance webhook（ADR-0060）の新しいイベント `error_group.created`・`error_group.regressed`（要る権限 `manage_errors`）。`data` は管理 API の `ErrorGroup` の表現にする。
  Slack・Discord・n8n などへは ADR-0060 の仕組みのまま流せる
- 管理画面 `/admin/errors`: グループの一覧（状態・`source`・`where`・最後の型・`message`・24 時間と 7 日の件数・最初と最後の時刻・最初と最後の版）、
  詳細（最新の 1 件のスタック・`request_id`・`trace_id`・時間ごとの件数）と、解決にする・ミュートする・ミュートを外す操作。ログの検索の手がかり（`request_id`）を写せるようにする
- 指標 `my.errors.reported`（Counter、`my.error.source`・`my.error.kind`）を ADR-0055 §3 に足し、`alerts.yml` に「報告が急に増えた」アラートを足す

### 9. 設定

| キー | 既定 | 内容 |
| --- | --- | --- |
| `errors_exporter` | `none` | 外部への転送（`none` / `sentry` / `otlp`）。ログと DB のグループは設定に関係なく常に動く |
| `errors_sentry_dsn` | なし | `errors_exporter = sentry` の DSN（秘密） |
| `errors_environment` | `production` | 報告に付ける環境の名前（`staging` など） |

### 10. この決定の範囲の外

- ソースマップを使った Web クライアントのスタックの復元（ビルドの成果物にソースマップを含めて、サーバーか Sentry で復元する）。必要になったら別の ADR にする
- `pkg/mykit`・`pkg/mybot` を使うクライアントのエラーの報告
- 外部のサービスの課題の管理（Sentry の issue の解決を DB のグループへ戻す同期）

### この決定を見直す条件

- 組み込みのグループの DB の書き込みが、小さな構成の DB の費用か負荷に目立って効くと分かった場合は、グループを持たず §4 (1) のログと外部への転送だけにする（C に寄せる）
- `sentry-go` の依存かライセンスが変わり、AGPL と両立しなくなった場合は、Sentry の envelope の形式を自前で送るアダプタにする

## Consequences（影響）

- 良い影響:
  - 外部のサービスがなくても、新しいエラーと再発がアプリ内の通知と webhook で運営者に届く。Cloud Run ではログの 1 行から Error Reporting も使える
  - スタック・起きた場所・版・`request_id`・`trace_id` から、ログとトレースへたどれる
  - Sentry・GlitchTip・Datadog（Sentry 用の DSN）・OTLP のバックエンドのどれにも、設定だけで転送できる
  - goroutine の panic が理由を残してから止まる
- 悪い影響・トレードオフ:
  - 新しいモジュール・5 つの表・権限・通知の種類・webhook のイベント・クライアント API・管理画面を足すので、実装と保守の量が増える
  - エラーごとに DB へ書く（貯めて 60 秒ごと）。DB の障害のときはグループに数えられない
  - 除去は正規表現の置き換えで、文字列に埋め込まれた個人情報をすべて消せる保証はない。外部への転送は運営者が選んだときだけにし、文書に書く
  - 指紋は関数名から作るので、関数の名前の変更やリファクタリングで別のグループになる
  - `sentry-go` の依存が増える（build tag `no_sentry` で外せる）
- 後続で必要になる作業（Epic #1430 の Sub-issue にする）:
  - port・`ErrorEvent`・指紋・除去・ログの出力（`severity` を含む）と、HTTP・ジョブ・起動の捕捉
  - `kernel.Go` と既存の `go` 文の置き換え
  - `diagnostics` モジュールの表・グループの集計・期限切れの削除
  - 権限 `manage_errors`・管理者の通知・webhook のイベント・管理 API と管理画面
  - Web クライアントの報告の API と捕捉
  - `sentry` と `otlp` のアダプタ
  - 運営者の文書（`docs/operations/` のエラーの報告の章、`alerts.md`、`docs/reference/webhooks.md`、設定のリファレンス）と `context-map.yaml` の項目

## Pros and Cons of the Options（詳細比較・任意）

| 観点 | A | B | C | D |
| --- | --- | --- | --- | --- |
| 外部のサービスなしで気づける | ◎（通知・webhook・画面） | ×（何も届かない） | △（Cloud Run の Error Reporting だけ） | × |
| 追加の費用 | DB の小さな書き込み | 外部のサービスの料金 | ログの料金 | コレクターとバックエンド |
| 個人情報の管理 | port の入口で 1 回除去 | SDK の `BeforeSend` を全経路で正しく使う必要 | ログに書く前に除去 | エクスポートの前に除去 |
| グルーピングの質 | 自前の指紋（単純） | Sentry の成熟したもの | 基盤による | バックエンドによる |
| 依存の向き（ADR-0002） | 守る | コアが SDK を import | 守る | 守る（port を通すなら） |
| 実装の量 | 大 | 小 | 小 | 中 |

## Open Questions / 要検証

- 権限: 新しい `manage_errors` にするか、既存の `manage_jobs`（ジョブ画面の最後のエラーを見る権限）に含めるか（推奨: 新しい `manage_errors`。スタックとエラーの文言はジョブより広い）
- Cloud Error Reporting が `debug.Stack()` の形の Go のスタックを `message` から読めるか、`stack_trace` のフィールドに分ける必要があるか（実装の issue で Cloud Run で確かめる）
- Cloud Logging が `jsonPayload` の `severity` を深刻度として読むことの確認と、Error Reporting の料金
- Datadog の Sentry 用の DSN の提供の段階・料金と、`sentry-go` の新しい版（v0.50 系）での動作
- GlitchTip のライセンス（MIT）の一次情報
- OTel Go のログの SDK と `otlploghttp` の安定性と版（2 週間以上前の版）
