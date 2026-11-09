# ADR-0103: エラーの報告（ErrorReporter port・指紋・報告先・運営者への通知）

- Status: Accepted
- Date: 2026-10-27
- Deciders: @rosylilly
- Issue: #1432, #1430, #1434, #1449

## Decision（決定）

エラーは「スタックと指紋を持つ 1 つの出来事」として port に渡し、
(1) 必ずログに 1 行書き、(2) DB のエラーのグループに数えて新しいグループと再発を管理者の通知と webhook で知らせ、(3) 設定したときだけ外部（Sentry 互換・OTLP）へ転送する。
ADR-0055 §16（span に `RecordError` とエラーの文字列を出さない）は変えない。エラーの報告は span と別の信号で、除去の規則（§3）を通ったものだけを出す。

### 1. port とエラーの出来事（ADR-0002）

- `internal/app/kernel` に `ErrorReporter` port を置く。型は標準ライブラリだけで書く:
  - `Report(ctx, ErrorEvent)`: 出来事を渡す。戻り値はなく、報告の失敗は報告しない（自分自身のエラーで再帰しない）
  - `Recover(ctx, where string)`: `defer` で呼び、panic を `ErrorEvent` にして報告し、再び panic させない（HTTP・ジョブ・goroutine の境界で使う）
- `ErrorEvent` が持つもの:
  - `Source`: `server` / `web`
  - `Kind`: `panic` / `error`
  - `Type`: エラーの型の連鎖（`errors.Unwrap` でたどった Go の型名。例 `*fmt.wrapError > *pgconn.PgError`）と、panic の値の型
  - `Message`: エラーの文字列（§3 で除去してから外へ出す）
  - `Stack`: panic は `debug.Stack()`。エラーは報告した地点のスタック。境界（アダプタの戻り値など）で `kernel.WithStack(err)` を使うと、作った地点のスタックを持たせられる。DB のアダプタの `Wrap`・`NotFound` は、それを呼んだリポジトリのメソッドから始まるスタックを付ける（`kernel.WithStackSkip`）。
    取ったスタックは、取る仕組み自身（`debug.Stack`・`NewErrorEvent`・`WithStack`）のフレームを含めず、呼び出し元から始める。面のエラーをまとめて報告するヘルパー（`activitypub` の `report`、クライアント API の `responseError` など）は `kernel.NewErrorEventSkip` で自分も除き、ヘルパーを呼んだ地点から始める。Error Reporting も §1 の指紋もスタックの先頭を読むので、先頭が報告の仕組みだと起きた場所の違うエラーが 1 つのグループに入る
  - `Where`: 起きた場所の有限の名前。HTTP は `my.surface` と `http.route`（パターン）、ジョブは `my.job.type`、goroutine は起動したときの名前（例 `stream.valkey.subscribe`）
  - 相関: `request_id`・`job_id`・`attempt`・`trace_id`・`span_id`（ADR-0055 §18）
  - 版と役割: `service.version`・`my.role`・設定 `errors_environment`（既定 `production`）
- 指紋（fingerprint）: `Source`・`Where`・`Type` の最後の型・スタックのうち本リポジトリ（`github.com/mycroblog/my/`）の関数名の上位 5 つ（行番号は含めない）から SHA-256 を作り、16 進の先頭 32 文字にする。
  本リポジトリの関数がスタックにないときは、§3 で正規化した `Message` を代わりに使う。行番号を含めないので、無関係な行の追加では指紋が変わらない

### 2. 報告するもの・しないもの

| 報告する | 報告しない（メトリクスとログのまま） |
| --- | --- |
| HTTP のハンドラーの panic と、ハンドラーが返したエラーによる 5xx（`clientapi`・`authpages`・ActivityPub・OAuth・管理・メディア） | 4xx（入力の誤り・認可・レート制限）、メンテナンスモードの 503 |
| ジョブの panic と `dead`（再試行を使い切った失敗） | ジョブの `retry`（一時的な失敗。使い切ったら `dead` で報告する） |
| goroutine の panic（§5） | 連合先が原因の失敗（配送の `retry`・`gone`・`failed`、Inbox の署名の拒否、リモートの取得の失敗） |
| 起動の失敗（設定・DB・アダプタの初期化） | readiness の失敗（ADR-0055 §12 のとおり指標とアラート） |
| Web クライアントの捕まえていないエラー（§6） | 利用者のネットワークの切断、ブラウザの拡張機能が出したエラー（スタックに自分の origin のファイルがないもの） |

- 外部の adapter（メール・blob・決済・SFU など）の失敗は、それによってジョブが `dead` になるか 5xx を返したときだけ報告する。一時的な失敗は ADR-0055 の `my.adapter.*` の指標で見る

### 3. 個人情報の除去と量の上限

- 除去（scrub）は port の実装の入口で 1 回行い、ログ・DB・外部のどの出力にも除去した後の値だけを渡す（リクエスト・ジョブの 1 行のログの `error` は変えない）:
  - `Message` の URL・メールアドレス・`@user@host` の形・IP アドレス・UUID と ULID・10 桁以上の数字を `<url>`・`<email>`・`<acct>`・`<ip>`・`<id>`・`<n>` に置き換え、1 KiB で切る
  - `Stack` は `debug.Stack()` の形を保ち、関数の行の末尾の引数の並び（`(0x1, {0x55d070?, …})` のような 16 進の値）を、Go がインライン化したフレームに出す形と同じ `(...)` に置き換える。
    `goroutine N [running]:` の行・ファイルの位置・`+0x` のオフセットは残し、報告の仕組み自身のフレーム（`runtime/debug.Stack`・`Recover`・`panic(...)`）は削る。16 KiB で切る。
    ビルドは `-trimpath` なので、ファイルの位置はモジュールからの相対で、ビルドしたマシンのパスを含まない
  - リクエストの本文・ヘッダー・クエリ・クッキー・アカウントの ID・ユーザー名・IP・User-Agent は持たない（ADR-0055 §6 と同じ）
- 量の上限: プロセスごとに、同じ指紋は 60 秒に 1 回だけ外部へ転送し、残りは数だけ数える。外部への転送は全体で毎分 60 件まで。上限を超えた分は DB の数とログには残る

### 4. 出力の経路

**(1) ログ（常に）**: 出来事ごとに 1 行の JSON を stderr に書く。Cloud Error Reporting が拾う形にする（他の基盤では普通の JSON の 1 行として扱える）:

```json
{"time":"…","level":"ERROR","severity":"ERROR","msg":"error report",
 "@type":"type.googleapis.com/google.devtools.clouderrorreporting.v1beta1.ReportedErrorEvent",
 "serviceContext":{"service":"my","version":"<service.version>"},
 "my.error.fingerprint":"…","my.error.source":"server","my.error.kind":"panic","my.error.where":"client_api GET /api/v1/statuses/{id}",
 "request_id":"…","trace_id":"…","span_id":"…",
 "message":"<Type>: <除去した Message>\ngoroutine 1 [running]:\ngithub.com/mycroblog/my/internal/app/status.(*Service).Get(...)\n\tgithub.com/mycroblog/my/internal/app/status/get.go:42 +0x32\n…"}
```

- `message` は Google の Go のライブラリと同じく、ヘッダーとスタックを改行 1 つでつなぐ。短いフィールドを前に、`message` を最後に置く（CloudWatch Logs の異常検知が行の先頭 1,500 文字だけを見るため）
- `serviceContext.version` には版だけを入れる（service と版の組はグループの数の上限に効く）。`context.httpRequest`・`context.user` は個人情報になるので出さない。
  `logging.googleapis.com/trace` の形（`projects/<id>/traces/<trace_id>`）は ADR-0055 §18 の未決の点と同じくデプロイのテンプレートで決める
- Error Reporting は `jsonPayload` の全フィールドからスタックを探すので、ほかのログの行にはスタックを書かない
- すべてのログの行に、slog の `ReplaceAttr` で Cloud Logging の `severity` を足す（`DEBUG`・`INFO`・`WARN`→`WARNING`・`ERROR`、`ERROR` より上→`CRITICAL`）。
  `level` も残す（アラートとログベースの指標の例が読む）。キーが重なる `warnReadiness` の属性は `severity` でなく `finding_severity` にする

**(2) エラーのグループ（常に。DB）**: §7 の表に指紋ごとに数え、新しいグループと、解決したグループの再発を §8 で知らせる。

- 書き込みはトランザクションの外の短い 1 文の upsert にする。プロセスの中で指紋ごとに数を貯め、最初の 1 件はすぐ、残りは 60 秒ごと（とプロセスの終わり）にまとめて書く。
  リクエストベース課金の基盤で貯めた数が失われることは受け入れる（正確な件数はメトリクスとログが持つ）
- DB に書けないとき（DB の障害そのものが原因のとき）は、捨ててログにだけ残す

**(3) 外部への転送（設定 `errors_exporter`、既定 `none`）**:

| 値 | 送り先 | 実装 |
| --- | --- | --- |
| `none` | 送らない | — |
| `sentry` | Sentry 互換の DSN（設定 `errors_sentry_dsn`、秘密）: Sentry（SaaS・セルフホスト）、GlitchTip、Datadog Error Tracking の Sentry 用の DSN | `internal/adapter/errorreport/sentry`。`sentry-go` を使わず、envelope（`sentry_version=7`）を自前で組み立てて送る小さなクライアント（DSN の解析・イベント・envelope・状態コード・レート制限で 300 行ほど）。イベントは `event_id`・`timestamp`・`platform: go`・`level`・`environment`・`release`・`fingerprint`（§1 の指紋）・`exception`（型・除去した文言・スタックのフレーム。本リポジトリのフレームは `in_app`）・`contexts.trace`（§1 の `trace_id`・`span_id`）・タグ `service`（Datadog に必須）・`my.role`・`my.error.where`・`error.fingerprint` を持ち、`server_name`・`user`・`request` を持たない。429 の `Retry-After` と `X-Sentry-Rate-Limits` の `error` のカテゴリに従って止め、再送しない |
| `otlp` | OpenTelemetry Collector など（宛先は OTel の標準の環境変数） | `internal/adapter/errorreport/otlp`。`otel/sdk/log` と `otlploghttp` で、例外の意味規約の LogRecord を送る: イベント名 `exception`、深刻度 `ERROR`（プロセスが落ちるものは `FATAL`）、`exception.type`・除去した `exception.message`・`exception.stacktrace`（§3 の形）・`my.error.fingerprint`・`my.error.where`。`SetErr` は除去の前の文言を入れるので使わない。trace_id・span_id は ctx の span から付ける。resource は ADR-0055 の `otel` のアダプタと同じものを使う。span のイベントには出さない（ADR-0055 §16、OTel の非推奨化） |

- 転送はトランザクションの外で、送信のキューを持つ非同期の送信者が 1 件ずつ送る（キューの上限 1,000 件。あふれたら捨てて数える）。`otlp` も BatchProcessor でなく SimpleProcessor をこのキューから呼び、捨てた数と成否を同じ指標で数える。
  プロセスの終わりに最大 2 秒待って送り切る
- `otlp` で受けた側がエラーをまとめるかはバックエンドによる（SigNoz は span のイベントだけ、Datadog は `error.stack` への写し替えが要る）。Sentry・GlitchTip・Datadog には `sentry` を使うよう文書で勧める
- 宛先は運営者が設定するサービスの API なので、`kernel.HTTPClient`（SSRF のガード）でなく、アダプタの中の HTTP クライアント・エクスポーターのものを使う（AGENTS.md の外向きの HTTP の規則）
- `sentry-go` を使わない理由: §1〜§3 の除去・指紋・量の上限・送信のキュー・成否の数を自前で持つので、SDK に残る利点はプロトコルの実装だけである。
  一方で SDK は版ごとに API を壊し、環境変数の暗黙の読み込みと `git` の実行、hostname の自動の付与、`kernel.Go` を通らない goroutine を持ち、送信の成否を返さない。プロトコルの版 7 と envelope は安定している
- 転送の成否は指標 `my.errors.exported`（Counter、`my.outcome` = `success` / `dropped` / `failed`）で見る

### 5. goroutine の起動

- `kernel.Go(ctx, name, reporter, fn)` を置き、`fn` の panic を `Recover` で報告してから、プロセスの終了の手順（`serve.go` の停止）に渡す。panic の握りつぶしで壊れた状態のまま動き続けないようにする
- `internal/` と `cmd/` の `go` 文はこの関数を使う（テストの補助のパッケージは除く）。`go` 文を禁止する検査（golangci-lint の規則か `agentkit` の検査）を足すかは実装の issue で決める

### 6. Web クライアントのエラー

- 報告の API `POST /api/v1/client_errors`（クライアント API・ADR-0016 の部分集合。ログインしていなくても呼べる）。本文は 16 KiB まで:
  `name`・`message`・`stack`（ブラウザの文字列）・`release`（ビルドの版）・`route`（TanStack Router のルートの ID。`router.state.matches.at(-1)?.routeId` で、URL でなくパターン）・
  `kind`（`route_error` / `boundary_error` / `uncaught` / `unhandledrejection`）。`Content-Type: application/json` だけを受ける（他の origin からの `text/plain` の単純なリクエストでの水増しを preflight で止める）
- サーバーは本文を `Source = web` の `ErrorEvent` にして §3 を通す。スタックは V8 の `at fn (url:line:col)` と Firefox・Safari の `fn@url:line:col` の 2 つの形を読む。
  最小化した関数名とチャンクのハッシュはリリースごとに変わるので、指紋は `name`・正規化した `message`・`route`・ハッシュを除いた最初の自分のチャンクの名前から作る。応答は常に `204`（失敗の理由を返さない）
- 乱用の対策: IP ごとのレート制限（毎分 10 件、ADR-0019 のレート制限の仕組み）。ログインしていない呼び出しは数とログに残すが、新しいグループの通知（§8）を起こさない
- 捕捉の入り口:
  - 主: TanStack Router の `defaultOnCatch`（ルートの `onCatch` は既定を上書きするので、ルートで `onCatch` を書くときも報告を呼ぶ）。notFound・redirect は報告しない
  - `react-error-boundary` の `onError`（ルートの外の境界）
  - `createRoot` の `onUncaughtError`（境界の外のもの）と `unhandledrejection`。`onUncaughtError` を置くと既定の `reportError` が呼ばれないので、window の `error` と二重に数えない
  - デプロイの後のチャンクの欠け（`vite:preloadError`。再読み込みの処理が受ける）と、自分の origin のファイルがスタックにないエラーは送らない
- 送り方: 同じエラーは画面の表示 1 回につき 1 回だけ、`fetch` の `keepalive` で `Authorization` を付けて送る（ログインした人からの報告を区別する）。
  ページを離れるときだけ `navigator.sendBeacon`（`application/json` の `Blob`）を使い、これはヘッダーを付けられないので「ログインしていない呼び出し」になる。
  keepalive の 64 KiB の枠は通話の退出の送信と分け合うので、本文の上限 16 KiB を守る
- 送り先は自分のサーバーだけで、CSP の `connect-src` を増やさない。ソースマップを使った復元は持たない（§10）

### 7. データモデル（ADR-0020）

モジュール `diagnostics`（`internal/domain/diagnostics`・`internal/app/diagnostics`）が所有する。スキーマは `schema/NN-diagnostics/<dialect>.sql`。

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

- 権限 `manage_errors`（ADR-0070 の権限の分け方。`admin` のロールに含める）を持つ。管理画面のエラーの一覧と、通知・webhook の購読に要る
- 管理者の通知（FR-712）の種類 `admin_error_group`: `manage_errors` を持つ人に、新しいグループ（`server` か、ログインした利用者からの `web`）と `regressed` を知らせる。
  インスタンス全体で 1 時間に 10 件までとし、超えた分は「ほかに N 件」の 1 件にまとめる。`muted` のグループは知らせない
- instance webhook（ADR-0060）のイベント `error_group.created`・`error_group.regressed`（要る権限 `manage_errors`）。`data` は管理 API の `ErrorGroup` の表現にする。
  Slack・Discord・n8n などへは ADR-0060 の仕組みのまま流せる
- 管理画面 `/admin/errors`: グループの一覧（状態・`source`・`where`・最後の型・`message`・24 時間と 7 日の件数・最初と最後の時刻・最初と最後の版）、
  詳細（最新の 1 件のスタック・`request_id`・`trace_id`・時間ごとの件数）と、解決にする・ミュートする・ミュートを外す操作。ログの検索の手がかり（`request_id`）を写せるようにする
- 指標 `my.errors.reported`（Counter、`my.error.source`・`my.error.kind`）を ADR-0055 §3 に足し、`alerts.yml` に「報告が急に増えた」アラートを置く

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

### 11. この決定を見直す条件

- 組み込みのグループの DB の書き込みが、小さな構成の DB の費用か負荷に目立って効くと分かった場合は、グループを持たず §4 (1) のログと外部への転送だけにする
- Sentry が envelope の形式か認証の版（`sentry_version=7`）を変えた場合、またはソースマップやセッションなど自前で持つと大きくなる機能が要る場合は、`sentry-go` を使うアダプタにする

## 経緯

決定者は、Pull 型の Observability はあるが Sentry や Datadog へ送るなどの Push 型の機能保証が足りないとして、エラー発生時に運営者が気づいて改善できる形の提案を求めた。メトリクス（ADR-0055）、トレース（ADR-0055）、構造化ログ（NFR-OPS-1）、外形監視（ADR-0088）、Prometheus のアラートはどれも件数と割合を見る仕組みで、次の穴があった: しきい値に届かないエラーに気づけない、スタックがどこにも残らない、同じ原因をまとめる指紋がない、recover しない goroutine の panic が理由を残さない、Web クライアントのエラーが届かない、システムの出来事を知らせる経路がない、Cloud Logging が `level`・`msg` を読まず深刻度が `DEFAULT` になる。M5 で v0.1.0 を公開して運用を始めるので、初日から外部のサービスなしで気づける必要があり、v0.1.0 からはスキーマの変更に互換が要る（ADR-0023）ので表を公開の前に決めた。

判断の軸は、外部のサービスなしで気づけること（NFR-COST）、個人情報を外へ出さないこと（ADR-0019・ADR-0055 §16）、原因の場所まで案内すること、ノイズで埋もれないこと、コアは port だけを呼ぶこと（ADR-0002）、`CGO_ENABLED=0` で AGPL と両立し、build tag で外せること（NFR-OPS-1〜3、NFR-COST-1・2）だった。

形を決めた外部の事実は次のとおりで、2026-10-27 に一次資料で確かめた（詳細は spike #1434 の報告）。

- Cloud Error Reporting は `jsonPayload` の `stack_trace`・`exception`・`message` の順にスタックを探し、`@type` を `ReportedErrorEvent` にすると必須の項目（`serviceContext.service`・`message`）があるものと扱う。グルーピングは型と上位 5 フレームで指紋を指定できず、グループの保持は 30 日、通知は 60 分に 20 件まで、service と版とグループの組は 10,000 まで（[Formatting errors in logs](https://docs.cloud.google.com/error-reporting/docs/formatting-error-messages)、[Grouping](https://docs.cloud.google.com/error-reporting/docs/grouping-errors)、[Notifications](https://docs.cloud.google.com/error-reporting/docs/notifications)、[Quotas](https://docs.cloud.google.com/error-reporting/quotas)）。
- Cloud Logging は `severity`・`message` を LogEntry に移し、`level`・`msg` を読まない（[special fields](https://docs.cloud.google.com/logging/docs/agent/configuration#special-fields)）。CloudWatch Logs の異常検知は各行の先頭 1,500 文字だけを見て、解決・再発の状態はない（[Log anomaly detection](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/LogsAnomalyDetection.html)）。
- Sentry は `POST {base}/api/{project}/envelope/` に `X-Sentry-Auth` で受け、`Event.fingerprint` が既定のグルーピングを置き換え、event の item は 1 MiB まで（[Envelopes](https://develop.sentry.dev/sdk/data-model/envelopes/)、[Event payloads](https://develop.sentry.dev/sdk/data-model/event-payloads/)）。`sentry-go` v0.50.0 は v0.48〜v0.50 の毎回に破壊的な変更があり、環境変数の読み込みと `git describe`、`ServerName` の hostname、`Transport.SendEvent` が成否を返さないことをソースで確認した。GlitchTip（MIT）は `/envelope/` を扱う。Datadog は専用の DSN（`https://<TOKEN>@sentry-intake.<DD_SITE>/1`）で受け、全イベントに `service` のタグが要り、`error.fingerprint` でまとめる（[Datadog: Sentry SDK](https://docs.datadoghq.com/error_tracking/guides/sentry_sdk/)）。
- OTel の例外は LogRecord の属性として記録し（`exception.*`）、span の例外の規約は Deprecated（[Exceptions in logs](https://opentelemetry.io/docs/specs/semconv/exceptions/exceptions-logs/)）。`otlploghttp` は v0.23.0 で v0.x の間は破壊的な変更がありうる。`Record.SetErr` は `exception.message` を `err.Error()` から作り、BatchProcessor はあふれると古いものから上書きし捨てた数を数えにくい。
- ブラウザ: React 19 の `onUncaughtError` の既定は `reportError`、`onCaughtError` は `console.error` だけ（[createRoot](https://react.dev/reference/react-dom/client/createRoot)）。`sendBeacon` はヘッダーを付けられず、keepalive の本文は同じ fetch group で合計 64 KiB まで（[Beacon](https://w3c.github.io/beacon/)、[Fetch](https://fetch.spec.whatwg.org/)）。
- 他の実装: Misskey は設定 `sentryForBackend`・`sentryForFrontend` で Sentry へ直接送り、Mastodon と GoToSocial は OTLP のトレースだけでエラーの報告はない。

Cloud Run の実機の確認（#1449）で、Error Reporting が 47 件の出来事を、報告するヘルパーのフレーム（`activitypub` の `report`）を場所とする 1 つのグループにまとめていた。中身は起きた場所の違う DB のエラーだった。DB のアダプタはスタックを付けず、スタックは報告した地点で取られ、取る仕組みとヘルパーのフレームから始まっていた。スタックを起きた場所から始めることにした（§1、#2659）。

## 採らなかった案

- コアから `sentry-go` を直接呼び、グルーピングと通知を Sentry に任せる: グルーピング・再発の検知・画面・ソースマップ・通知の規則が完成していて Datadog も同じ SDK と DSN で受けられるが、外部のサービスを使わない運営者には何も届かず、コアが SDK を import して ADR-0002 に例外を作る。Sentry 互換の転送で利点の多くを含められる。
- ログだけにしてグルーピングと通知は基盤に任せる: 費用は最も小さく Cloud Run では Error Reporting が無料で知らせるが、保持は 30 日、指紋を指定できず通知は毎時 20 件までで、AWS には解決・再発の状態がなく Self-hosted には基盤の仕組みがない。スタック付きの 1 行のログは §4 (1) に取り込んだ。
- OpenTelemetry だけにする: どのバックエンドにも出せるが、コレクターなどが要り既定の構成では何も届かない。転送の 1 つ（`otlp`）に取り込んだ。

## Consequences（影響）

- 外部のサービスがなくても、新しいエラーと再発がアプリ内の通知と webhook で運営者に届く。Cloud Run ではログの 1 行から Error Reporting も使える。
- スタック・起きた場所・版・`request_id`・`trace_id` から、ログとトレースへたどれる。Sentry・GlitchTip・Datadog（Sentry 用の DSN）・OTLP のバックエンドのどれにも、設定だけで転送できる。goroutine の panic が理由を残してから止まる。
- **悪い点**:
  - モジュール・5 つの表・権限・通知の種類・webhook のイベント・クライアント API・管理画面が増え、実装と保守の量が増える。
  - エラーごとに DB へ書く（貯めて 60 秒ごと）。DB の障害のときはグループに数えられない。
  - 除去は正規表現の置き換えで、文字列に埋め込まれた個人情報をすべて消せる保証はない。外部への転送は運営者が選んだときだけにし、文書に書く。
  - 指紋は関数名から作るので、関数の名前の変更やリファクタリングで別のグループになる。
  - Sentry の envelope のクライアントを自前で保守する。Sentry・GlitchTip・Datadog の取り込みの違いは、spike の模擬のサーバーでの契約テストと、利用する運営者の確認に頼る。
  - Cloud Error Reporting のグループ（指紋を指定できない）と DB のグループ（§1 の指紋）は、同じエラーを別の単位でまとめることがある。
- 後続: Cloud Run の実機で、§3 の引数を `(...)` にしたスタックを Error Reporting が読み、グループにすることを確かめる（#1449）。

## Open Questions / 要検証

- 引数を `(...)` にしたスタックを Cloud Error Reporting が読むか（#1449 で Cloud Run の実機で確かめる）
- Datadog の Sentry 用の DSN の提供の段階（preview か GA か）、Sentry の `fingerprint` とタグ `error.fingerprint` のどちらがグルーピングに効くか、ログの取り込みの料金が別にかかるか
- GlitchTip の通知が「新しい issue」「再発」だけを対象にできるか（しきい値の通知だけか）
