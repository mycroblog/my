# アーキテクチャ概要（基礎設計）

> この文書はサーバーの基礎設計で、Accepted な ADR に従います。判断の根拠は各 ADR を参照してください。
> サーバーの実装言語は Go です（[ADR-0001](../adr/0001-server-language-and-runtime.md)）。

## 1. 全体像

```text
                ┌──────────────── Clients ────────────────┐
                │ Web (PWA) │ Mobile (Tauri 2) │ 3rd-party │
                │           │                  │           │
                └─────┬─────┴────────┬─────────┴─────┬─────┘
                      │  Client API (OpenAPI, 独自)    │
Remote servers ──AP──►│                               │
(Mastodon, Misskey…)  ▼                               ▼
        ┌──────────────────── Entrypoints (ターゲット別) ─────────────────────┐
        │ HTTP handler │ Queue consumer (pull / push) │ Scheduler (cron)       │
        └───────┬──────┴──────────────┬───────────────┴──────────┬────────────┘
                ▼                     ▼                          ▼
        ┌─────────────── Interface layer (inbound adapters) ──────────────────┐
        │ Client API │ ActivityPub (inbox/outbox/actor/webfinger) │ Admin       │
        └───────┬──────────────────────────────────────────────────────────────┘
                ▼
        ┌─────────────── Application layer (use cases) ───────────────────────┐
        │ PostStatus, Follow, ProcessInboxActivity, DeliverActivity, ...       │
        └───────┬──────────────────────────────────────────────────────────────┘
                ▼
        ┌─────────────── Domain layer (pure) ─────────────────────────────────┐
        │ Account, Status, Relationship, Visibility policy, Timeline rules ... │
        └──────────────────────────────────────────────────────────────────────┘
                ▲  ports (interfaces) — application 層が定義
        ┌───────┴─────────── Outbound adapters（設定で選択）────────────────────┐
        │ DB: sqlite │ libsql │ postgres                                        │
        │ Queue: db │ sqs │ cloud-tasks │ inline                                │
        │ Blob: fs │ s3-compatible(S3/R2/GCS/MinIO)                             │
        │ KV/Cache: db │ memory │ valkey(任意。Redis 互換)                        │
        │ Mail: none │ smtp │ ses │ http-api   Push: webpush                    │
        │ Media: none │ native(pure Go) │ vips(cgo, 任意)                       │
        └──────────────────────────────────────────────────────────────────────┘
```

- **モジュラーモノリス + ヘキサゴナル（Ports & Adapters）**。[ADR-0002](../adr/0002-modular-monolith-hexagonal.md)
- 1 つのコードベースから、ターゲット別の **エントリポイント** が必要なアダプタだけを組み込んで起動する。
- Web とワーカーは同一成果物。役割（`web` / `worker` / `all`）は設定で切り替える。
- **クライアントの認可**: 自前の Web・モバイルのクライアントも、サードパーティのアプリも、同じ OAuth 2.0 の認可サーバー（Authorization Code + PKCE S256、リフレッシュトークンのローテーション）で `/api/v1` を呼ぶ。
  サードパーティは利用者による登録・Client ID Metadata Document・動的クライアント登録（RFC 7591）で `client_id` を得て、毎回の同意の画面を通る。管理者・支払い・アカウントの保安の操作は一次のクライアントだけが行える。
  [ADR-0090](../adr/0090-third-party-oauth-apps.md)、仕様は [client-api §4](client-api.md#4-認可oauth-20)、開発者向けの手引きは [third-party-apps](../reference/third-party-apps.md)。

## 2. モジュール（境界づけられたコンテキスト）

| モジュール | 責務 | 主な集約 / エンティティ |
| --- | --- | --- |
| `identity` | 利用者登録・認証・セッション・OAuth アプリ / トークン・2FA・パスキー | User, Session, OAuthApp, AccessToken, PasskeyCredential |
| `account` | ローカル / リモートのアカウント（Actor）とプロフィール | Account, ProfileField, ActorKey |
| `content` | 投稿・メディア・メンション・タグ・カスタム絵文字・投票 | Status, MediaAttachment, Tag, Emoji, Poll |
| `graph` | フォロー・フォローリクエスト・ブロック・ミュート | Follow, FollowRequest, Block, Mute |
| `timeline` | ホーム / ローカル / 連合 / タグ / プロフィールのタイムライン組み立て | （読み取りモデル） |
| `notification` | 通知の生成・取得・Web Push | Notification, PushSubscription |
| `federation` | AP の受信・配送・Actor/Object 取得、署名、ドメイン状態 | RemoteObjectCache, DeliveryAttempt, InstanceHealth |
| `moderation` | 通報・ドメインブロック・凍結・監査ログ | Report, DomainBlock, AuditLog |
| `instance` | インスタンス設定・規約・ルール・テーマ・お知らせ・NodeInfo | InstanceSettings |
| `webhook` | サーバー単位の webhook（宛先・署名・配送と再送、ADR-0060） | WebhookEndpoint, WebhookMessage, WebhookDelivery |
| `finance` | サーバーの維持費用（費用の項目・支払いの周期・改定・終了・取り消し、週・月・年の集計。集計は読み取りのときに計算する。ADR-0067）と、クラウドの利用料金の取り込み（見込み・確定・修正の版。ADR-0068）と、決済の事業者からの収入の記録と集計（`supporters` が `RecordIncome` で渡し、`finance` は加入者とプランの人数を `SupporterDirectory` で読む。ADR-0073） | CostItem, PriceRevision, ImportedRevision, IncomeMovement |
| `supporters` | 管理者が有効にしたときだけ動く課金ユーザー（サポーター。ADR-0072）。課金の有効化・プラン・事業者の価格・加入・加入の状態と猶予・掲載の選択。決済の事業者の事実を運ぶ経路（webhook の受信の記録とジョブ、定期の照合。ADR-0074 §3・§4）。加入の状態は事業者のスナップショットから `Syncer`（`supporters.sync`）が導き、プランのロールを `identity` の管理されたロールの操作で付け外しする | Plan, PlanRevision, Subscription, Transition, WebhookReceipt, ReconcileRun |
| `i18n` | ロケール解決・メッセージカタログ・整形 | （横断。`internal/i18n` の 1 パッケージで、`domain` / `app` の層を持たない） |

### 依存ルール

- `domain` は他のどの層にも依存しない（純粋関数・値オブジェクト中心）。
- モジュール間はアプリケーション層の公開 API（ユースケース / イベント）経由でのみ参照する。テーブルの直接参照は禁止。
- インターフェイス層（Client API / AP）はアプリケーション層のユースケースを呼ぶだけで、ドメインロジックを持たない。
- **公開範囲の判定（`canView(viewer, status)`）はドメイン層の 1 箇所に集約**（NFR-SEC-4、[公開範囲の閲覧判定](visibility.md)）。公開範囲は `public` / `local` / `unlisted` / `private` / `direct`（[ADR-0017](../adr/0017-visibility-local.md)）。
- import の方向は golangci-lint の depguard（`.golangci.yml`）で `scripts/task.sh check` と CI が検査する。§6 の各ディレクトリについて:

  | パッケージ | import してよいもの |
  | --- | --- |
  | `internal/domain/<module>` | 標準ライブラリと自モジュールの `domain` だけ |
  | `internal/app/<module>` | 標準ライブラリ、自モジュールの `domain`、各モジュールの `app`（モジュール間の唯一の経路）、`internal/i18n` |
  | `internal/app/kernel` | 標準ライブラリだけ（全モジュール共通のポート） |
  | `internal/app/uow` | 標準ライブラリだけ（`UnitOfWork` ポート・リトライ・同期 projector の登録先） |
  | `internal/app/blob`, `internal/app/media` | 標準ライブラリだけ（`BlobStore` / `MediaProcessor` ポート） |
  | `internal/app/stream` | 標準ライブラリ、`internal/app/kernel`（生存の印の KV）、`internal/app/queue`（後始末の定期ジョブ）（リアルタイム更新の `Publisher` / `Subscriber` / `Bus` / `Sweeper` ポート、プロセス内ブローカー、プロセス間の中継） |
  | `internal/app/projection` | 標準ライブラリ、`internal/app/uow`、`internal/app/queue`（投影の再構築と検証。各モジュールの再構築関数は composition root が渡す） |
  | `internal/app/metrics` | 標準ライブラリ、`internal/app/kernel`（KV と `Meter`）、`internal/app/queue`（キューの滞留と定期ジョブ）（インスタンス全体の指標のスナップショット、ADR-0055 §4。件数と容量を数える関数は composition root が渡す） |
  | `internal/i18n` | 標準ライブラリ、`locales`（埋め込みカタログ）、MessageFormat エンジンだけ（`app` と `interface` から使う末端のライブラリ） |
  | `internal/interface/*` | `app` と `domain`（`adapter` / `platform` は不可。`pkg/testkit` はテストからだけ可） |
  | `internal/adapter/*` | `app` のポートと `domain`（`interface` と `platform` は不可） |
  | `pkg/testkit` | `app` と `domain`（`interface` / `adapter` / `platform` は不可） |
  | `internal/platform`, `cmd/*` | すべて（composition root）。`cmd/` はどこからも import しない |

  `domain` と `app` で外部ライブラリを使うときは、`.golangci.yml` の allow に明示的に追加する。

## 3. ポート一覧

各ポートは、それを使うモジュールの `internal/app/<module>` に Go のインターフェイスとして定義する（§6）。
全モジュールが使う `Clock` / `IdGenerator`（Go 名 `IDGenerator`）と `KeyManager` は `internal/app/kernel` に置き、ドメインには `time.Time` と ID 文字列を値で渡す。
`UnitOfWork` も全モジュールと全 DB アダプタが共有するので `internal/app/uow` に置く。リポジトリポートは各モジュールの `app` に置き、
DB アダプタの実装は受け取った `uow.Tx` から自分のトランザクションを取り出す。
同期の購読者が他のモジュールの書きかけの行を読む必要があるときは、そのモジュールが `uow.Tx` を明示的に受け取る読み取り
（リポジトリと公開 API の `...Tx`）を持つ（webhook の `data`、ADR-0060）。
ジョブキューのポート（`JobQueue` / `Outbox` / `JobHandlerRegistry` / `JobRunner`）も全モジュールと全キューアダプタが共有するので `internal/app/queue` に置く。
ここにはリトライ方針（指数バックオフ + ジッタ）と、キューアダプタの `queue.Source` を駆動する pull runner、プロセス内の `Scheduler` も置く。
`Scheduler` は定期ジョブを周期ごとの冪等キー付きで enqueue するだけで、worker と all のロールで動く。
ファイルを持つモジュールとそのアダプタが共有する `BlobStore` は `internal/app/blob`（Go 名 `blob.Store`）、`MediaProcessor` は `internal/app/media`（Go 名 `media.Processor`）に置く。
リアルタイム更新（§4.4）の `Publisher` / `Subscriber` は、通知する側のモジュールとストリーミングのエンドポイントが共有するので `internal/app/stream` に置く。プロセス間で知らせを同報するブローカーのポート `Bus` と、残された資源を消す `Sweeper` も同じパッケージに置く。
Blob のキーはサーバーが生成し、`blob.ValidateKey` の構文（`[A-Za-z0-9._-]` のセグメントを `/` でつなぐ）に限る。`media/` で始まるキーだけが公開で、`/media/` で配信する。

| ポート | 役割 | 既定アダプタ（self-hosted） | 関連 ADR |
| --- | --- | --- | --- |
| `Repositories` / `UnitOfWork` | 永続化。書き込みは **短く再実行可能なトランザクション** 単位 | sqlite / postgres | [0003](../adr/0003-database-abstraction.md) |
| `JobQueue` | ジョブ投入（遅延・冪等キー付き） | db | [0004](../adr/0004-job-queue-abstraction.md) |
| `JobRunner` | ジョブ実行（pull ループ or push 受信） | db（pull） | [0004](../adr/0004-job-queue-abstraction.md) |
| `Scheduler` | 定期実行のトリガー | in-process cron | [0004](../adr/0004-job-queue-abstraction.md) |
| `BlobStore` | メディアの保存・署名付き URL | fs | [0005](../adr/0005-blob-storage-and-media.md) |
| `MediaProcessor` | 用途ごとの S / M / L の版・アニメーションの静止版と動く版・EXIF 除去 | native | [0005](../adr/0005-blob-storage-and-media.md), [0054](../adr/0054-media-variants-video-audio.md) |
| `KeyValueStore` | キャッシュ・レート制限・連合ライブラリ用 KV。任意の `valkey` は Valkey か Redis 互換のサーバーに置き、ADR-0003 と ADR-0019 の「Redis」の枠をこのアダプタで満たす | db | [0003](../adr/0003-database-abstraction.md), [0019](../adr/0019-security-baseline.md) |
| `HTTPClient` | 外部 HTTP（SSRF ガード・用途別の本文上限とタイムアウト・UA 付与） | httpclient（`net/http` + SSRF ガード） | [0006](../adr/0006-federation-protocol-scope.md), [0019](../adr/0019-security-baseline.md) |
| `Mailer` | メール送信 | none（ログ出力） | [0010](../adr/0010-configuration-and-packaging.md) |
| `HumanVerifier` | サインインと登録の CAPTCHA のトークンの検証（`HTTPClient` で siteverify に送る）。`captcha = none` では使わない（nil） | なし（`captcha = none`） | [0042](../adr/0042-captcha-human-verifier.md) |
| `PushSender` | Web Push 送信 | webpush | — |
| `Publisher` / `Subscriber` | リアルタイム更新の通知と購読（§4.4）。設定 `stream` が `hub` なら `stream.Hub`、それ以外は `stream.Relay` | in-process（`stream.Hub`、`stream = hub`） | [0016](../adr/0016-client-api-style.md), [0050](../adr/0050-stream-pubsub-across-processes.md) |
| `Bus` / `Sweeper` | プロセス間の知らせの同報（`stream.Relay` が使う）と、プロセスが残したブローカーの資源の後始末（§4.4）。アダプタは `postgres` / `valkey` / `sns` / `pubsub`（`sns` と `pubsub` は `Sweeper` も実装）。`stream.MemoryBroker` はテスト用 | なし（`stream = hub`）。複数プロセスは `postgres` | [0050](../adr/0050-stream-pubsub-across-processes.md) |
| `Clock` / `IdGenerator` | 時刻（UTC・ミリ秒）・ID 生成（テスト容易性のため注入） | system（システム時計 / `uuid.NewV7()`） | [0003](../adr/0003-database-abstraction.md) |
| `KeyManager` | 秘密（アクター秘密鍵など）を包む DEK の wrap / unwrap。暗号化は `kernel.Sealer` が行う | local（設定の KEK） | [0019](../adr/0019-security-baseline.md) |
| `SearchIndex` | アカウント・タグ検索（将来全文） | db（FTS） | [0009](../adr/0009-internationalization.md) |
| `Meter` | サービスとシステムの指標の計測。計器（Counter・UpDownCounter・Histogram・Gauge）を名前・単位・許すラベルのキーとともに宣言して記録し、`Begin` でリクエスト・ジョブの区切りを示す。型は標準ライブラリだけで、OTel の型を出さない。アダプタは `none` / `otel`（OpenTelemetry Go SDK。Prometheus の scrape と OTLP/HTTP）/ `emf`（CloudWatch Embedded Metric Format。Gauge は収集されないので、定期ジョブが `GaugeWriter` で書かせる）。ログは port でなく `log/slog` の JSON Lines（標準エラー）で、トレースは `Tracer` が持つ | none（`metrics_exporter = none`） | [0055](../adr/0055-service-observability.md), [0056](../adr/0056-system-observability.md) |
| `BillingSource` | クラウドの事業者の利用料金を請求月・サービス・費用の種類ごとに返す（`internal/app/finance`）。型は標準ライブラリと finance のドメインだけで、SDK の型を出さない。課金される要求の数を `MaxRequests` までに抑え、失敗を種類に分ける。事業者のアダプタは `internal/adapter/billing/<source>` に置き、テスト用の fake は `testkit.FakeBillingSource` | なし（`billing_sources` が空） | [0068](../adr/0068-billing-import.md) |
| `PaymentProvider` | 決済の事業者の側の事実（プランと価格・ホスト型の決済画面・顧客ポータル・プランの変更と解約の命令・webhook の検証・サブスクリプションのスナップショット・一覧・お金の動き）を返す（`internal/app/supporters`）。型は標準ライブラリだけで、SDK の型を出さない。webhook の検証は I/O をせず、失敗を種類に分ける。事業者のアダプタは `internal/adapter/payment/<provider>` に置き、テスト用の fake は `internal/adapter/payment/fake`。契約は `testkit.PaymentProviderContract` | なし（`payments_provider = none`） | [0074](../adr/0074-payment-provider-port.md) |
| `Tracer` | 分散トレースの span。span を名前・種類・許す属性のキー（`SpanSpec`）で宣言して始め、`Inject` で今の span の `traceparent` をジョブ（`queue.Job.TraceParent`）に書き、`Linked` でジョブの実行を link 付きの新しいトレースにし、`Continue` で信頼したプロキシの `traceparent` を親にし、`LogIDs` でログの行に `trace_id`・`span_id` を付ける。型は標準ライブラリだけで、OTel の型を出さない。span は HTTP のミドルウェア・ジョブの runner・`HTTPClient`・`UnitOfWork.Tx` とクエリ（`telemetry/dbtrace`、PostgreSQL は pgx の `QueryTracer`）・配送が作る。アダプタは `none` / `otel`（OpenTelemetry Go SDK の OTLP/HTTP） | none（`traces_exporter = none`。信頼したプロキシの trace ID をログに付けるだけ） | [0063](../adr/0063-distributed-tracing.md) |

各ポートには `pkg/testkit` にアダプタ非依存のコントラクトテスト（`testkit.<名前>Contract`）があり、そのポートのアダプタはすべてそれを通す（NFR-PORT-3）。
アダプタを包んで計測する `internal/adapter/telemetry/sysmetrics` のデコレータも、包むポートのコントラクトを通す。

## 4. 主要フロー

### 4.1 ローカル投稿 → 連合配送

連合の受信・配送・取得の詳細は [連合の設計](federation.md) にあります。

1. Client API の投稿作成 → `PostStatus` ユースケース
2. ドメインで検証（文字数・公開範囲・メンション解決）→ `Status` を生成
3. **1 回のトランザクション**（`UnitOfWork.Tx`）で `statuses` + 付随行を書き、同じトランザクションの同期 projector が `outbound_activities`（`Create`）と `outbox_events`（`federation.fanout` ジョブ）を書く（Transactional Outbox）
4. リレーが outbox のジョブをキューに渡す（コミットした分だけが届く）
5. ワーカー: `federation.fanout` が文書を 1 回だけ組み立て、公開範囲に応じて宛先 inbox を 500 件ずつ算出し（`local` はリモートフォロワーのみ）、**shared inbox 単位** の `deliveries` ごとに `federation.deliver{deliveryId}` を投入
6. `federation.deliver` が署名付き POST。失敗時はバックオフで再投入、ドメインが連続失敗したら一時停止

### 4.2 Inbox 受信

1. `POST /ap/inbox`（shared inbox）または `/ap/actors/{uuid}/inbox`（[ADR-0018](../adr/0018-federated-uris.md)）
2. 同期処理: サイズ制限 → 署名検証（鍵は KV / DB キャッシュ、未取得なら取得）→ ドメインブロック判定 → `202 Accepted`
3. `JobQueue.enqueue(ProcessInbox{activity, verifiedActor})`
4. ワーカー: Activity 種別ごとのハンドラ（冪等: Activity ID で重複排除）→ 通知生成など

### 4.3 タイムライン取得

- v0 は **fan-out-on-read**（読み取り時にフォロー関係と JOIN してページング）を既定とする。
  Redis 等の常駐ミドルウェアを不要にするため。規模が大きくなったら `home_feed` テーブルへの fan-out-on-write に切り替えられるよう、
  `TimelineReader` を内部インターフェイスにしておく。→ [ADR-0013](../adr/0013-timeline-strategy.md)

### 4.4 リアルタイム更新（SSE）

1. コマンドのトランザクションがコミットされると、`uow.Projectors` に登録したコミットフックが、そのトランザクションで発行したイベントを受け取る（[data-model §2.1](data-model/02-consistency.md#21-トランザクションの規律)）
2. `timeline` のフックは新しい投稿・ブーストの ID を、`notification` のフックは通知を受け取りうるアカウントを、`stream.Publisher` に知らせる。知らせるのは「何が変わったか」だけで、内容は載せない
3. `GET /api/v1/streaming` の各接続は `stream.Subscriber` で自分のアカウントを購読し、知らせを受けるたびにユースケースで読み直す。
   ホームに載るかは `timeline.Service.HomeEntry`（ホームのタイムラインと同じ条件と `CanView`）、通知は前回送ったものより新しい通知の一覧で決める
4. 結果を SSE のイベント（`update` は `Status`、`notification` は `Notification`）として送る。手順は [client-api.md §5](client-api.md#5-ストリーミング)

- 組み立て（`internal/platform`）は、設定 `stream`（[設定のリファレンス](../reference/configuration.md)）でブローカーを選ぶ。設定は推測せず、既定は `hub`（[ADR-0050](../adr/0050-stream-pubsub-across-processes.md) §3）
  - `hub`: `stream.Hub` だけで、**1 つのプロセスの中だけ** で知らせを配る。web のプロセスが複数ある構成、web と worker を分けた構成（Inbox の処理で作られた通知は worker で生まれる）では、
    別のプロセスで起きた変更がストリームに届かない。そのため `role` が `all` 以外のプロセスは、起動時に警告をログに出す
  - `postgres`（`db = postgres` のときだけ）/ `valkey` / `sns` / `pubsub`: `stream.Relay` とそのアダプタの `Bus`。プロセスごとに新しいプロセス ID を作り、`Relay` とアダプタで共有する。
    アダプタが `stream.Sweeper` を実装していれば（`sns`、`pubsub`）、`Relay` は KV に生存の印を書き、組み立ては `stream.sweep` を `Scheduler` に載せる
  - ターゲットごとのブローカーは [デプロイターゲット](deployment-targets.md#ストリームプロセス間の知らせ) のとおり
  - シャットダウン: web と all のロールは、シャットダウンの開始時にブローカーを閉じる（開いたストリームを終え、送信バッファの知らせを送り、listener を閉じてブローカーの資源を消す）。
    worker のロールはストリームを持たないので、実行中のジョブが終わってから閉じ、ジョブの知らせを最後まで送る。all のロールでシャットダウン中に終わったジョブの知らせは、ほかのプロセスに届かない
  - 2 つのプロセスの結合テスト（`internal/platform` の `TestStreamsAcrossProcesses`）: 同じ DB と同じブローカーの web と worker の 2 つの App で、worker で作った投稿と、worker が Inbox の処理で作った通知が、
    web のストリームに届くことを、メモリの `Bus`、`stream = postgres`（PR では pgmem）、`stream = pubsub`（プロセス内の Pub/Sub サーバー pstest）で確かめる
- プロセスをまたぐ配信は `stream.Relay` が行う。`Relay` は `Publisher` / `Subscriber` を実装し、プロセスの中の振り分け・上限・遅い購読者の切断は自分の `Hub` に任せ、
  プロセスの間はポート `stream.Bus`（すべてのプロセスへ同報し、受け取るだけ）で運ぶ
  - 知らせは JSON `{"v":1,"o":"<process id>","k":"home_entry","e":"<entry id>"}`（通知は `"k":"notifications","a":"<account id>"`、お知らせは `"k":"announcement","n":"<announcement id>"`）で、1 KiB を超える符号化は失敗させる。知らない `v` の知らせと、知らない種類の知らせは捨てる
  - `Publish` は自分の `Hub` にすぐ配り、有限の送信バッファ（1,024 件）に積む。送信の goroutine が `Bus.Send` を呼ぶ。あふれたときと送れなかったときは捨て、件数を `Relay.Stats` とログに出す。
    受けた知らせのうち、自分のプロセス ID のもの（発行時に配り済み）は捨てる
  - listen は遅延させる。ローカルの購読が 0 から 1 になったら `Bus.Listen` し、0 に戻って 60 秒たったら `Listener.Close` する。`Subscribe(ctx, accountID)` は listener の準備を最大 10 秒待ち、
    間に合わなければ `stream.ErrUnavailable`（ストリーミングのエンドポイントは 503 と `Retry-After`）
  - `Listener` が `Broken`（知らせを取りこぼしたかもしれない）になったら、そのプロセスのローカルの購読をすべて終える。クライアントは再接続して読み直す
  - `Bus` は at-least-once で、プロセスをまたぐ順序は保証しない。ストリーミングのハンドラはストリームごとに最近送ったホームの新着の ID を覚えて重複を捨てる
  - 後始末: listen 中の `Relay` は KV（`kernel.KeyValueStore`、名前空間 `stream.alive`）にプロセス ID の生存の印を 1 分ごとに TTL 5 分で書く。
    プロセスごとの資源（キュー・subscription）を作るブローカーは任意のポート `stream.Sweeper` を実装し、定期ジョブ `stream.sweep`（15 分ごと）が、作成から 5 分以上たち印のないプロセスの資源を消す
  - アダプタは `testkit.StreamBusContract` を、`Hub` と `Relay` は `testkit.StreamBrokerContract` を通す。プロセス内のメモリの `stream.MemoryBroker` はテスト用
- 配信は best effort で、再送しない。クライアントは再接続したらホームと通知を読み直す
- ホームの新着は、接続中の利用者ごとに 1 回 `HomeEntry` を読む。読み取りの回数は「新しい投稿数 × 接続数」に比例する

## 5. データ設計の原則

テーブル・カラム・制約・インデックスの詳細は [論理データモデル](data-model/README.md) を参照してください。
モデリングの規約（リソース・イベント・投影の分離、状態の直交分解、正規化）は [ADR-0020](../adr/0020-data-modeling-principles.md) です。

- ID は時系列ソート可能な **UUIDv7**（文字列表現）。API ではそのまま文字列で返す。
- 時刻は UTC で保存、API では RFC 3339。
- 方言差を吸収するため、型は「共通部分集合」で設計（真偽値は整数、JSON は TEXT 保存も許容、等）。
- トランザクションは短く再実行可能にし、内部で外部 I/O をしない。重複実行への耐性は「一意制約 + upsert」「冪等キー」で担保（[ADR-0003](../adr/0003-database-abstraction.md)）。
- マイグレーション履歴を持たず、方言ごとの宣言的なスキーマファイルを適用する。`v0.1.0` 以降はデータを失わない変更だけを許し、データの書き換えは `my db upgrade` で行う（[ADR-0087](../adr/0087-schema-changes-after-launch.md)）。

## 6. リポジトリ構成

サーバーは Go（[ADR-0001](../adr/0001-server-language-and-runtime.md)）、構成は [ADR-0010](../adr/0010-configuration-and-packaging.md) に従う。

```text
cmd/
  my/                # 単一バイナリ: serve / worker / cli（Self-hosted, Cloud Run, コンテナ）
  my-lambda/         # AWS Lambda エントリ（Lambda Web Adapter で my を流用できるなら不要）
internal/
  domain/<module>/   # 純粋なドメイン（identity, account, content, graph, ...）
  app/<module>/      # ユースケースとポート定義
  app/kernel/        # 全モジュール共通のポート（Clock / IDGenerator / KeyManager / KeyValueStore / Mailer / HTMLSanitizer / HTTPClient / HumanVerifier）、UUIDv7 ヘルパー、Sealer、RateLimiter
  app/uow/           # UnitOfWork ポート、トランザクションのリトライ、同期 projector の登録先
  app/queue/         # ジョブキューのポート、ハンドラの登録先、リトライ方針、pull runner、プロセス内 Scheduler
  app/blob/          # BlobStore ポートとキーの構文
  app/media/         # MediaProcessor ポート、形式の判定、上限の既定値
  app/stream/        # リアルタイム更新の Publisher / Subscriber / Bus / Sweeper ポート、プロセス内ブローカー Hub、プロセス間の中継 Relay、定期ジョブ stream.sweep
  app/projection/    # 投影の再構築と検証（my projection、定期ジョブ projection.verify）と、表を列に依存せず読み書きする Store ポート
  app/metrics/       # インスタンス全体の指標のスナップショット（KV の名前空間 metrics、定期ジョブ metrics.snapshot。ADR-0055 §4）
  interface/
    clientapi/       # 独自クライアント API（api/openapi.yaml から生成したインターフェイスを実装）。スタッフの操作（/admin/*、ADR-0058）の入口もここだけ
    activitypub/     # inbox / outbox / actor / webfinger / nodeinfo
    media/           # 公開メディア（Blob のキー media/...）の配信 /media/
    payments/        # 決済の事業者が呼ぶ webhook の受け口 POST /payments/webhooks/{provider}（クライアント API の外。ADR-0074 §3）
    authpages/       # ログイン・ログアウト・登録・メール確認の画面（html/template、JavaScript なし。ADR-0030。CAPTCHA を有効にしたときだけ、ログインと登録の画面がサービスのウィジェットを読む。ADR-0042。ログインの画面は、パスキーに対応したブラウザでだけパスキーのボタンを出す Web ビルドのスクリプトを読む。FR-004）
    web/             # Web クライアントの HTML の外枠（ランディングページ・インスタンスの情報ページ・プロフィール・投稿のリンクプレビューを含む）・ビルド成果物（dist/ を埋め込む）・Web アプリマニフェスト（ADR-0028）
  adapter/
    db/{sqlite,libsql,postgres}/
    queue/{db,inline,sqs,cloudtasks}/
    blob/{fs,s3}/
    media/{native,vips,none}/
    kv/{db,memory,valkey}/
    stream/postgres/ # stream.Bus の PostgreSQL LISTEN / NOTIFY（送信はプール、listener は専用の接続。ADR-0050）
    stream/valkey/   # stream.Bus の Valkey Pub/Sub（1 チャネルに PUBLISH、listener は専用の接続で SUBSCRIBE。ADR-0050）
    stream/sns/      # stream.Bus と stream.Sweeper の Amazon SNS → プロセスごとの SQS キュー（ADR-0050）
    stream/pubsub/   # stream.Bus と stream.Sweeper の Google Cloud Pub/Sub（プロセスごとの subscription。ADR-0050）
    valkey/          # Valkey（Redis 互換）への接続（valkey-go）。valkey_url を使うアダプタが共有する
    billing/aws/     # finance.BillingSource の AWS Cost Explorer（GetCostAndUsage の MONTHLY、SERVICE × RECORD_TYPE。ADR-0068 §6）
    billing/gcp/     # finance.BillingSource の Google Cloud（Cloud Billing の BigQuery への export を jobs.query で invoice.month × サービス × cost_type に集計、または予算の Pub/Sub 通知を pull して月ごとの合計の概算。ADR-0068 §7）
    payment/fake/    # supporters.PaymentProvider のテスト用の fake（事業者の側をメモリに持ち、webhook に HMAC で署名する。本番の binary に入らない）
    mail/{none,smtp,ses}/
    telemetry/{none,otel,emf}/ # kernel.Meter（ADR-0055）と kernel.Tracer（none・otel、ADR-0063）の実装。OpenTelemetry と Prometheus のライブラリはここだけが import する
    telemetry/dbtrace/ # DB のアダプタが共有するトランザクションとクエリの span（ADR-0063 §1）
    telemetry/sysmetrics/ # システムの指標の計器と、アダプタの境界で測るデコレーター（ADR-0056）
    captcha/siteverify/ # HumanVerifier の実装（Turnstile / hCaptcha / reCAPTCHA v2 のプリセット。ADR-0042）
    system/          # Clock / IDGenerator の本番実装（システム時計・UUIDv7）
    keymanager/local/ # KeyManager の実装（設定の KEK。KMS アダプタは後続）
    htmlsanitizer/   # HTMLSanitizer の実装（bluemonday。リモート HTML の許可リスト。federation.md §5）と、インスタンスの文書の許可リスト
    webauthn/        # identity.PasskeyCeremonies の実装（go-webauthn。パスキーの登録とログインの WebAuthn の手続き。FR-004）
    markdown/        # instance.DocumentRenderer の実装（goldmark。利用規約・プライバシーポリシーの Markdown を HTML にし、htmlsanitizer の文書の許可リストに通す）
  platform/          # 設定、composition root、ログ、HTTP サーバー
  i18n/              # メッセージ整形・ロケール解決（型付きアクセサは tools/i18n が生成）
pkg/
  testkit/           # コントラクトテスト・フィクスチャ・決定的なフェイク（独自アダプタ作者向けに公開）
    pgtest/          # テスト用の PostgreSQL（既定は pgmem、テンプレートの複製。ADR-0041）
api/                 # spec/（クライアント API の正。パス・components の項目ごとのファイル）、そのバンドル openapi.yaml（生成物）と lint 設定 redocly.yaml。規約は client-api.md
schema/              # 目標スキーマ sqlite/・postgres/（モジュールごとのファイル。Atlas で宣言的に適用）と準備 SQL postgres.bootstrap.sql（ADR-0023・ADR-0082）
queries/             # sqlc 用クエリ（方言別。設定は sqlc.yaml、生成先は internal/adapter/db/<dialect>/。queue/ は internal/adapter/queue/db 用）
locales/             # ICU MessageFormat カタログ（en がソース。Go と TS で共有。en-XA / ar-XB は生成物）
tools/               # 開発ツール（Go ツールの go.mod、i18n の生成器・検査を持つ Bun パッケージ tools/i18n）
app/                 # クライアントの Bun ワークスペース（アプリと共有パッケージ。ADR-0026）
  ui/                # 共有 UI キット @my/ui（React Aria Components を包む部品、デザイントークン、Storybook）
  web/               # Web クライアント（PWA、TypeScript）
  mobile/            # モバイルアプリ（Tauri 2）
deploy/
  docker/ k8s/ gcp/ aws/
  docker/federation-live/  # ライブの相互運用テスト（Mastodon・GoToSocial との compose とシナリオ。federation.md §12）
docs/
```
