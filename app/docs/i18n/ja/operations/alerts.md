<!-- source: alerts-and-runbook 2c201450ab32 -->
# アラートとランブック

サーバー自身のメトリクスで何をアラートすべきか、それぞれのアラートの緊急度、そしてアラートが発生したときに確認することと行うことを説明します。サーバーを外から監視する方法（死活監視とステータスページ）は[サーバーを外から監視する](monitoring.md)にあります。
停止しているサーバーからはメトリクスが届かないので、そちらも設定してください。

<!-- source: getting-the-metrics 58d8055fe44d -->
## メトリクスを取得する

`metrics_exporter` を設定すると、サーバーはメトリクスをエクスポートします（[設定リファレンス](../reference/configuration.md#metrics_exporter)）。
すべてのメトリクスとそのラベルは[運用ガイド](running.md)に一覧があります。以下のアラートは Prometheus 向けに書かれており、名前は Prometheus から見える名前です。ドットはアンダースコアになり、単位とカウンターの接尾辞が付きます
（`my.queue.oldest_ready_age` は `my_queue_oldest_ready_age_seconds`、`my.delivery.attempts` は `my_delivery_attempts_total` になります）。

| 環境 | 設定 | アラート |
| --- | --- | --- |
| セルフホスト（Docker Compose、Kubernetes、VM） | `metrics_exporter = prometheus` | Prometheus が `metrics_listen` の `GET /metrics` をスクレイプし、Alertmanager がアラートを振り分けます。以下のルールはそのまま使えます |
| OpenTelemetry Collector を使うすべての環境 | `metrics_exporter = otlp` | Collector が好きなバックエンドに転送します。単位と種類の接尾辞を付ける Prometheus 互換のバックエンド（Prometheus 自身の OTLP レシーバーは既定でそうします）なら、同じ名前になります |
| Google Cloud（Cloud Run） | `metrics_exporter = none` | 稼働時間チェック、プラットフォームのメトリクス、ログベースの指標に対する Cloud Monitoring のアラートポリシー（[Cloud Run](#google-cloud-cloud-run) を参照） |
| AWS（ECS） | `metrics_exporter = emf` | CloudWatch Logs が取り出したメトリクスに対する CloudWatch アラーム（[AWS ECS](#aws-ecs) を参照） |

<!-- source: prometheus-and-alertmanager 8e8dcaa334e9 -->
### Prometheus と Alertmanager

ルールは [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml) にあります。このファイルを
`prometheus.yml` と同じ場所にコピーして読み込みます。

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` の既定値は `127.0.0.1:9464` で、同じホストの Prometheus からしか届きません。Docker Compose や Kubernetes では
  `MY_METRICS_LISTEN=0.0.0.0:9464` を設定し、ポートはプライベートネットワークの中にとどめてください。このリスナーには認証がありません。
- すべてのプロセスをスクレイプしてください（`role = web` と `role = worker` のプロセスはそれぞれ自分のリスナーを持ちます）。キュー、メディア、データベースのサイズ、
  リモートのサーバー、利用料金の取り込み、スナップショットのゲージは、どのプロセスでもインスタンス全体の同じ値を持つため、ルールはその最大値を使います。
- どのルールにも `severity` ラベルがあります。**`page`** は今すぐ人が必要なもの（利用者に影響が出ている、またはまもなく出る）、**`ticket`** は業務時間まで待てるものです。
  Alertmanager でそれぞれを別の受信先に振り分けてください。各ルールの `runbook_url` は、下の対応するセクションを指しています。
- しきい値は小さなサーバー向けの出発点です。1 週間ほど値を観察し、しきい値と `for` の期間をあなたのトラフィックに合わせて調整してください。
  `MyRemoteMediaCacheOverLimit`、`MyMediaStorageLarge`、`MyDatabaseStorageLarge` には、あなた自身の上限に合わせて設定すべきサイズが入っています。
- メンテナンスモードの間、サーバーはメトリクスを提供せず `503` を返すため、`MyTargetDown` と HTTP のアラートが発生します。
  始める前に、その時間帯のアラートを Alertmanager でサイレンスしてください（[メンテナンス](maintenance.md#steps)）。

ルールを変えたら、Prometheus に付属する `promtool` でファイルを検査し、その単体テスト（`alerts.test.yml`）を実行してください。

```sh
cd deploy/monitoring/prometheus
promtool check rules alerts.yml
promtool test rules alerts.test.yml
```

<!-- source: google-cloud-cloud-run 72c348585d24 -->
### Google Cloud（Cloud Run）

`metrics_exporter = none` の場合は、Cloud Run と Cloud Logging にすでにあるものからアラートを作ります。

- **HTTP のエラーとレイテンシ**: レスポンスコードのクラスごとの Cloud Run のリクエスト数と、サービスのリクエストのレイテンシのメトリクス。
- **エラー**: `error report` の行は Error Reporting に送られ、そこでグループにまとめられ、新しいエラーや再発したエラーを通知できます（[エラーの報告](error-reporting.md#google-cloud-cloud-run)）。
- **ジョブとリクエスト**: サーバーは HTTP リクエストごと（`msg` = `http request`）とジョブの実行ごと（`msg` = `job`）に 1 行のログを、
  メトリクスと同じラベル付きで書き出します。これらからログベースの指標を作ります。たとえば失敗したジョブには `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"` のカウンター、
  レイテンシには `jsonPayload."my.surface"="client_api"` で絞り込んだ `jsonPayload.duration_seconds` の分布を使います。
- **キューの滞留**: `queue = cloudtasks` ではジョブはデータベースにあるため、ジョブの画面とジョブ実行のログの行で滞留が分かります。
  Cloud Tasks のキューの深さのメトリクスは、配送を待っているシグナルを数えます。
- **データベース**: Cloud SQL のディスク使用率と接続のメトリクス、またはデータベースの事業者の使用量アラート。
- **費用**: アラートのしきい値を設定した Cloud Billing の予算（[費用と予算](#cost-and-budget)）。

モジュール [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) は、これらを下の表の名前のポリシーとして作り、違う方法で扱うアラートや表現できないアラートを一覧にしています。このモジュールはまだ一度も適用されたことがありません。トラフィックがあるときだけ生じるメトリクスは、ゼロまでスケールするサービスでは途切れるため、その欠落に対するアラートは誤って発生します。アイドル状態で停止したサーバーに気づくのは、このモジュールの稼働時間チェックです。

<!-- source: aws-ecs 609e5bf632ef -->
### AWS ECS

`metrics_exporter = emf` の場合、CloudWatch Logs はサーバーのログの行を名前空間 `metrics_emf_namespace` のメトリクスに変換します。名前はドット区切りのもの
（`my.job.duration`、`my.queue.oldest_ready_age`）です。リクエストとジョブの所要時間のメトリクスはリクエストとジョブのログの行から作られ、
ディメンションは `metrics_emf_dimensions` に挙げたラベルになります。失敗したジョブでアラームを出すには `my.outcome` を、5xx でアラームを出すには
`http.response.status_code` を含めてください。インスタンス全体のゲージ（キュー、メディア、データベースのサイズ、リモートのサーバー、利用料金の取り込み、スナップショット）は、定期的なスナップショットのジョブが書き出します。Go ランタイムのメトリクス（`go.*`）は
EMF では書き出されないため、代わりに ECS サービスの CPU とメモリのメトリクスを使ってください。

- **HTTP のエラー**: Application Load Balancer の 5xx とリクエスト数のメトリクス。
- **キューの滞留**: スナップショットのジョブが書き出す `my.queue.oldest_ready_age`。
- **データベース**: RDS や Aurora のストレージと接続のメトリクス、またはデータベースの事業者の使用量アラート。
- **メール**（`mail = ses`）: CloudWatch にある SES の評価のメトリクス（バウンス率と苦情率）。
- **費用**: アラートを設定した AWS Budgets の予算（[費用と予算](#cost-and-budget)）。

モジュール [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) は、これらを下の表の名前のアラームとして作り、同じように一覧を持っています。このモジュールはまだ一度も適用されたことがありません。

<!-- source: the-alerts d76842ca02b8 -->
## アラートの一覧

緊急度: **page** = 今すぐ対応する、**ticket** = 業務時間内に対応する。ルールの列の「–」は、それに対するメトリクスがまだないことを示します。説明のとおりに確認してください。

| 分野 | アラート | シグナル | しきい値 | 緊急度 | 考えられる原因 |
| --- | --- | --- | --- | --- | --- |
| キュー | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | 5 分超が 10 分続く | ticket | 急増（人気の投稿、リレー）に対してワーカーが少ない、遅いジョブ、止まったワーカー |
| キュー | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | 30 分超が 5 分続く | page | ワーカーが動いていない（クラッシュした、デプロイされていない、`role = web` だけ）、ある種類のジョブを実行するワーカーがない、ワーカーがデータベースで詰まっている |
| キュー | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | 1 時間に 10 件超が 15 分続く | ticket | ジョブのバグ、アダプターの設定の誤り（メール、ブロブ、利用料金）、拒否するリモートのサービス |
| キュー | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | キューの部分が 5 分超、合計の部分が 1 時間超の状態が 15 分続く | ticket | KV ストアに届かない、スナップショットのデータベースのクエリが失敗するかタイムアウトする |
| 連合 | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_outcome` が `retry` または `failed` の `my_delivery_attempts_total` | 試行の 50% 超が 30 分続く | ticket | 停止しているサーバーに多くのフォロワーがいる、大きなサーバーが停止している、外向きのネットワークの問題 |
| 連合 | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | 同上 | 試行の 95% 超が 15 分続く | page | 外向きのネットワークや DNS が壊れている、外向きの通信が遮断されている、サーバーの鍵や `host` が誤っている |
| 連合 | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 1 日前より 5 増える | ticket | 閉鎖したサーバーやこのサーバーをブロックしたサーバー、このサーバー側のネットワークの問題が何日も続いた |
| 連合 | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | POST の 25% 超が 30 分続く | ticket | 署名の検証の失敗（時計のずれ、鍵の取得の失敗）、スパマーや停止したドメインの再試行、レート制限 |
| 連合 | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | POST の 10% 超が 30 分続く | ticket | サーバーの時計がずれている、送信者の鍵の取得に失敗する、鍵を変えたサーバー |
| 連合 | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | POST の 5% 超が 15 分続く | ticket | データベースの障害、インボックスのバグ |
| エラー | [`MyErrorReportsIncreasing`](#myerrorreportsincreasing) | `my_errors_reported_total` | 15 分間に 10 件超かつ前日の平均の 3 倍が 15 分続く | ticket | リリースで入った新しいバグ、新しい形で失敗する依存先、壊れた設定 |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | `http_server_request_duration_seconds_count` の 5xx | 5% 超が 10 分続く | page | データベースや KV の停止、問題のあるリリース、メンテナンスモード |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | 同上 | 1% 超が 30 分続く | ticket | 失敗する 1 つのルート、ときどき失敗するアダプター |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | クライアント API の p95（ストリーミングを除く） | 300 ms 超が 30 分続く | ticket | データベースの飽和や遠さ、遅いクエリ、CPU の不足、コールドスタート |
| リソース | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 0 が 5 分続く | page | プロセスの停止、クラッシュの繰り返し、メンテナンスモード、スクレイプの経路の故障 |
| リソース | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | 5 分間に少しでも増える | page | データベースに届かない、またはスキーマが適用されていない |
| リソース | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 1 時間に 3 回の再起動 | ticket | メモリ不足による強制終了（OOM）、パニック、失敗する liveness プローブ |
| リソース | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | 90% 超が 15 分続く | ticket | 負荷に対して `GOMEMLIMIT` が低い、開いているストリームが多い、リーク |
| リソース | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | 90% 超が 15 分続く | ticket | トラフィックに対してプールが小さい、遅いクエリが接続を占有している |
| リソース | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`、`db_client_connection_timeouts_total` | 1 秒あたり 0.5 秒超の待ち、または 1 件でもタイムアウトが 10 分続く | ticket | 上と同じ。データベース自身の接続数の上限に達している |
| リソース | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | プランの上限から余裕を引いた値（例: 8 GiB）超が 1 時間続く | ticket | 投稿とキャッシュしたリモートのコンテンツの増加。プランを上げるか保存期間を短くする時期 |
| リソース | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | 80% 超が 15 分続く | ticket | 開いているストリームと接続が多い、`ulimit -n` が低い |
| リソース | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | 1 つのアダプターの操作の 5% 超が 15 分続く | ticket | Valkey、ブロブのディレクトリ、ストリームのバックエンドの障害 |
| リソース | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | `remote_media_cache_max_bytes` の 1.2 倍超が 3 時間続く | ticket | 1 時間ごとの掃除が動いていない、または失敗する |
| リソース | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | 計画したサイズ（例: 50 GiB）超が 1 時間続く | ticket | アップロードの増加。プランや予算を上げる時期 |
| メール | [`MyEmailFailing`](#myemailfailing) | `my_outcome="dead"` のメールのジョブの `my_job_duration_seconds_count` | 1 時間に 1 件でも | ticket | SMTP や SES の設定の誤り、アカウントの停止や送信枠の超過、恒久的に拒否されたアドレス |
| メール | [`MyEmailRetrying`](#myemailretrying) | 同上、`my_outcome="retry"` | 実行の 50% 超が 15 分続く | ticket | メールサーバーに届かない、ログインを拒否される、送信のレート制限 |
| 費用 | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 1 が 6 時間続く | ticket | 権限の不足、課金のエクスポートが設定されていない、リクエストの上限に達した、事業者が利用できない |
| 費用 | 予算の超過 | – | – | – | トラフィック、ストレージ、または設定の誤りで計画より費用がかかっている（[下記](#cost-and-budget)） |

<!-- source: where-to-look 9ef1d37c4f05 -->
## 確認する場所

以下のツールは、このあとのランブックのほとんどの項目に出てきます。

- **ジョブの画面**（「管理」→「ジョブ」、`/admin/jobs`、`manage_jobs` が必要）: 状態（「待ち」「実行中」「リトライ待ち」「失敗」）ごと、種類ごとの件数と、
  いちばん長く待っているジョブ。その **「ジョブ」** のページ（`/admin/jobs/list`）は 1 つの状態のジョブを一覧にし、各ジョブのページには最後のエラーと成功しなかった試行が表示されます。待っているジョブが一覧に出るのは、データベースに保持されるキュー
  （`queue = db` と `cloudtasks`）だけです。失敗したジョブと試行は、どのキューでも一覧に出ます。
- **`my job` コマンド**（サーバーの設定がある、`queue = db` と `cloudtasks` のホストで）:
  `my job list` は失敗したジョブを新しい順に一覧にし、`my job retry <id>...` はそれらをキューに戻し、`my job delete <id>...` はそれらを削除します。
  ジョブはもう一度実行しても安全です。
- **リモートのサーバーの画面**（「モデレーション」→「連合先のサーバー」、`/moderation/instances`、`manage_domains` が必要）: 配送が届かなかったサーバーと、
  配送が停止しているサーバーを、その理由と日時とともに表示します。「配送を再開する」で、そのサーバーを再び利用できる状態にします。
- **ログ**: 標準エラーに出力される JSON の行。HTTP リクエストはそれぞれ `msg` が `http request` の行で終わり、ジョブの実行はそれぞれ `msg` が `job` の行で終わります
  （成功は `INFO`、再試行は `WARN`、最終的に失敗したジョブは `ERROR` で、`error`、`job_id`、`my.job.type` が付きます）。
  リクエストの中の行にはその `request_id` が付き、クライアントからは `X-Request-Id` として見えます。
  - Docker Compose: `docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging: `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights: ``filter msg = "job" and `my.outcome` = "dead"``
- **エラーの報告**: `error report` の行と、エラーの画面（「管理」→「エラー」、`/admin/errors`、`manage_errors` が必要）（[エラーの報告](error-reporting.md)）。
- **トレース**（`traces_exporter = otlp` のとき）: ログの行には `trace_id` が付きます。トレースのバックエンドで開くと、遅いリクエストがどこで時間を使ったかが分かります。
- **メンテナンスモード**（[メンテナンス](maintenance.md)）: すべてのリクエストにメンテナンスのページを返し、どのアダプターにも触れません。データベースや
  KV ストアを修復する間に使ってください。

<!-- source: runbook e60e811fcff0 -->
## ランブック

<!-- source: myqueuebacklog e625ad2bb49f -->
### MyQueueBacklog

ジョブ（配送、通知、メール、タイムラインへの配信）が 5 分を超えてワーカーを待っています。

確認すること:

- ジョブの画面: どの種類が待っているか、多くが再試行中かどうか。1 つの種類の急増（フォロワーの多い投稿のあとの `federation.deliver`、
  リレーのアクティビティ）は自然に解消します。待ち時間が減っていくことを確かめてください。
- ワーカーのプロセス（`role = all` または `role = worker`）が動いていて、そのログに `job` の行が出ていること。
- 同時に `MyDBPoolSaturated` や `MyAPILatencyHigh` が出ていないか。出ていれば、ワーカーはデータベースを待っています。

対処:

- ワーカーのプロセス（`role = worker`）を増やすか、今のプロセスに CPU を多く割り当てます。
- 1 つの種類のジョブが遅い場合は、`job` のログの行（`duration_seconds`）でそれを見つけ、その種類のエラーを探します。

<!-- source: myqueuestalled c92f00f06f95 -->
### MyQueueStalled

実行できる状態のいちばん古いジョブが 30 分を超えて待っています。何も処理されていないか、ほとんど処理されていません。

確認すること:

- `role = all` または `role = worker` で動いているプロセスが少なくとも 1 つあり、クラッシュを繰り返していないこと（`MyProcessRestarting`）。
- ワーカーのログに、どのジョブでも繰り返されるエラー（データベース、KV、パニック）がないか。
- `MyNotReady`: データベースに届かないワーカーはジョブを取れません。
- `max by (my_job_type) (my_queue_oldest_ready_age_seconds)`: ある種類だけが待っていてほかは待っていない場合は、その種類を実行するワーカーがあることを確かめます（[ジョブの種類ごとにワーカーを分ける](running.md#splitting-workers-by-job-type)）。

対処:

- ワーカーを起動または再起動します。止まったワーカーが持っていたジョブは、リースが切れるとキューに戻ります。もう一度実行しても安全です。
- データベースが原因なら、[MyNotReady](#mynotready) に従ってください。

<!-- source: mydeadjobsincreasing b8e928621ac7 -->
### MyDeadJobsIncreasing

1 時間に 10 件を超えるジョブが最終的に失敗しています。再試行の回数を使い切ったか、ジョブが再実行を拒否しました。

確認すること:

- 「失敗」で絞り込んだジョブの画面: 種類と最後のエラー。ジョブの試行には、途中のすべてのエラーが表示されます。
- `my.outcome` = `dead` の `job` のログの行にある同じエラー。

対処:

- 設定の誤り（メール、ブロブ、利用料金の取り込み、Webhook の送信先）: 直してから、ジョブの画面（「再実行」）か `my job retry <id>...` でジョブをもう一度実行します。
- なくなったサーバーへの配送は、約 4 日間の再試行のあとで最終的に失敗します。これは想定どおりで、何もする必要はありません。
- 誰も再試行しない失敗したジョブは、`dead_job_retention`（既定は 30 日）のあとで削除されます。再試行しないものは `my job delete` で削除してください。
- バグに見えるエラー: ジョブの種類とエラーを添えて報告してください（個人データは含まれません）。

<!-- source: mymetricssnapshotstale c74fe15e6934 -->
### MyMetricsSnapshotStale

キューとストレージのゲージは、プロセスが KV ストアを通じて共有する 1 つのスナップショットから来ます。それが古くなっているため、キューとストレージのアラートは
古い値を見ており、鳴らないままになることがあります。

確認すること:

- ログに、メトリクスのスナップショットや KV ストアに関するエラーがないか。
- `kv.valkey` または `kv.db` の `MyAdapterErrors` と、データベースの状態。

対処:

- KV ストアやデータベースの接続を修復します。スナップショットは次の収集で計算し直されます。定期的なスナップショットのジョブも 15 分ごとに実行されます。

<!-- source: mydeliveryfailureshigh 709985dfbf2b -->
### MyDeliveryFailuresHigh

ほかのサーバーへの配送の試行の半分を超えて失敗しています。ある程度の失敗は普通のことです。サーバーはなくなることがあり、サーバーはそれらへの再試行を約 4 日間続けます。

確認すること:

- リモートのサーバーの画面: 失敗が少数のサーバーに集中している（それらが停止している。何もすることはない）か、多くのサーバーに散らばっている（このサーバーを疑う）か。
- `error_type` ごとの `http_client_request_duration_seconds_count`: タイムアウト、DNS のエラー、または `forbidden_destination`（SSRF ガードが送信先を拒否した）。

対処:

- 少数のサーバーでの失敗: 何もしません。異なる 7 日間に失敗すると、そのサーバーへの配送は停止され、再開するために毎日そのサーバーが確認されます。
- どこでも失敗している: [MyDeliveryAllFailing](#mydeliveryallfailing) に従ってください。

<!-- source: mydeliveryallfailing c81fcf407cb9 -->
### MyDeliveryAllFailing

ほとんどすべての配送が失敗しています。これはほぼ必ず、このサーバーの側の問題です。

確認すること:

- サーバーのホストやコンテナーから、外向きの HTTPS が通ること（DNS、外向きのファイアウォール、NAT、プロキシ）。
- `federation.deliver` の `job` のログの行にあるエラー: TLS のエラー、タイムアウト、多くのサーバーからの `401`（このサーバーの署名を検証できない）。
- ドメインや鍵を変えたあと: サーバーが一度連合したら `host` は変えてはならず、KEK は鍵を封印したときのものでなければなりません
  （[KEK の管理](key-management.md)）。

対処:

- 外向きのネットワークを修復します。失敗した試行はバックオフ（1 分、5 分、30 分、2 時間、…）を挟んで再試行されるため、配送は自然に追いつきます。
- 配送が再試行の回数を使い切った場合は、失敗した `federation.deliver` のジョブをジョブの画面からもう一度実行します。
- 障害の間にリモートのサーバーが停止された場合は、次に成功した配送、署名付きのリクエスト、または毎日の確認で再開されます。リモートのサーバーの画面で
  「配送を再開する」を押すこともできます。

<!-- source: myremoteserverspaused 17c3ec51fb53 -->
### MyRemoteServersPaused

この 1 日で、5 つ以上のサーバーへの配送が停止されました。サーバーへの配送が停止されるのは、異なる 7 日間に配送が失敗したときなので、週に数件の停止は
普通のことです。サーバーは閉鎖されるものだからです。一度に多くのサーバーが停止された場合は、多くのサーバーが同時に何日も失敗したことを意味します。`my_federation_instance_state` が
`available` の `my_federation_instances` は、まだ配送しているもののどこかの日に失敗したサーバーを数えます。そこでの急増は停止に先立って起こります。

このメトリクスにはサーバーごとのラベルがありません（ラベルがリモートのドメインを持つことはありません）。確認すること:

- リモートのサーバーの画面: 「配送停止中」のタブに停止されたサーバーが、その日時と理由とともに一覧になります。それらに共通点（同じホスティング事業者、同じソフトウェア）があるか、
  ばらばらか。
- 過去 1 週間の `MyDeliveryFailuresHigh` と `MyDeliveryAllFailing`: このサーバー側のネットワークの問題が何日も続くと、多くのサーバーが停止されます。

対処:

- なくなったサーバー: 何もしません。停止されたサーバーは毎日確認され、応答すれば自然に再開されます。
- このサーバー側の問題のあと: 修復してから、気にかけているサーバーについて「配送を再開する」を押すか、毎日の確認を待ちます。
- 運営者がこのサーバーをブロックしていると伝えてきたサーバー: こちら側でできることはありません。意図してあるサーバーとの連合をやめるには、ドメインへの処分を使います
  （「モデレーション」→「ドメインへの処分」）。

<!-- source: myinboxrejectionshigh ee5a244ccf12 -->
### MyInboxRejectionsHigh

ほかのサーバーがインボックスに POST するアクティビティの 4 分の 1 を超えて拒否されています。

確認すること:

- `my_inbox_rejection` ごとの `my_inbox_activities_total{my_outcome="rejected"}`: `signature`（[MyInboxSignatureFailures](#myinboxsignaturefailures) に従う）、
  `suspended`（再試行を続ける停止されたドメイン）、`federation_off`（連合が無効になっている）、`rate_limited`（レート制限を超えたクライアントの IP またはサーバー）、
  `invalid`（有効な `id`、`type`、`actor`、`object` のないアクティビティ）。
- インボックスのルート（`my.surface` = `activitypub`）の `http request` のログの行には、`http.response.status_code`
  （`401`、`403`、`429`、`400`）ごとの同じ内訳とクライアントのアドレスがあり、リクエストがどこから来ているかを知りたいときに使えます。

対処:

- 停止されたドメインからの拒否や、連合を無効にしているときの拒否は、何もする必要はありません。意図どおりに拒否されています。
- 1 つのサーバーからのレート制限されたリクエスト: スパマーか急増です。続くようならそのドメインを停止します。
- 1 つのサーバーからの無効なアクティビティ: そのサーバーのソフトウェアが、このサーバーが読めないものを送っています。そのサーバーのソフトウェアとバージョンを添えて報告してください。

<!-- source: myinboxsignaturefailures 85aa65f3247c -->
### MyInboxSignatureFailures

ほかのサーバーがインボックスに POST するアクティビティの 10% を超えて、署名や完全性の証明がない、または検証できない、
あるいは送信者の鍵を取得できなかったために拒否されています。

確認すること:

- サーバーの時計: 署名は一定の時間の範囲の外では拒否されます。ホストやコンテナーランタイムの NTP を確認してください。
- 送信者の鍵の取得: `MyDeliveryAllFailing` と、`error_type` ごとの `http_client_request_duration_seconds_count{my_http_purpose="json"}`
  （外向きのネットワーク、DNS、SSRF ガード）。
- 拒否が多くのサーバーから来ているか 1 つのサーバーから来ているか（ステータスが `401` の `http request` の行）: 鍵を変えたサーバーや、このサーバーが受け付けない
  方法で署名するサーバーは、そのサーバーにだけ影響します。

対処:

- 時計か外向きのネットワークを直します。ほかのサーバーは配送を再試行するので、1 日程度のうちに直れば何も失われません。
- 1 つのサーバーが失敗する: そのサーバーの運営者に、またはそのサーバーのソフトウェアとバージョンを添えてこのプロジェクトに報告してください。

<!-- source: myinboxerrors e3cf874c925e -->
### MyInboxErrors

インボックスが 5% を超えるアクティビティに `500` を返しています。送信者は再試行しますが、アクティビティは遅れます。

**確認すること:** インボックスのルートの `http request` とエラーのログの行にあるエラーと、データベースの状態（`MyNotReady`、`MyDBPoolWaiting`）。

**対処:** データベースを直すか、エラーをバグとして報告します。送信者が再試行するので、再送する必要はありません。

<!-- source: myerrorreportsincreasing deed4980cc8c -->
### MyErrorReportsIncreasing

サーバーが、通常の 3 倍を超える割合でエラー（パニック、5xx の原因となったエラー、失敗したジョブ）を報告しています。

確認すること:

- 直近 1 時間の `error report` の行を `my.error.fingerprint` でまとめたもの: 最も多いもの、その `my.error.where`、`message` の見出しの行。
  Cloud Run では、Google Cloud Error Reporting に同じグループが表示されます。
- 同じ時期に、リリース、設定の変更、または失敗する依存先（`MyNotReady`、`MyAdapterErrors`）が始まっていないか。
- 同じ `request_id` や `job_id` のリクエストやジョブの行。

対処:

- 失敗する依存先や設定の誤り: 直します。失敗したジョブは、そのあとでジョブの画面からもう一度実行できます。
- 問題のあるリリース: 1 つ前のイメージに戻します。
- バグ: その行の見出しとスタックを添えて報告してください。どちらも個人データが取り除かれています。

<!-- source: myhttperrorshigh dd55e9ba0dfd -->
### MyHTTPErrorsHigh

すべてのリクエストの 5% を超えて 5xx が返っています。利用者にはエラーが見えています。

確認すること:

- メンテナンスモードではないか（`503` は想定どおり。次回はアラートをサイレンスしてください）。
- `MyNotReady`（データベース）、`MyAdapterErrors`（KV、ブロブ、ストリーム）、最近のリリース。
- `http.response.status_code` が 500 以上の `http request` の行を `http.route` ごとに見たものと、同じ `request_id` のエラーの行。

対処:

- 失敗している依存先を修復します。データベースの作業が必要なら、利用者にエラーではなく分かりやすいページが見えるよう、メンテナンスモードに切り替えます。
- 問題のあるリリース: 1 つ前のイメージに戻します。
- 長引くようなら利用者に知らせます（[障害を利用者に知らせる](#telling-users-about-an-incident)）。

<!-- source: myhttperrorselevated 34d53d12ea4d -->
### MyHTTPErrorsElevated

30 分にわたって、リクエストの 1% を超えて 5xx が返っています。たいていは 1 つのルートか 1 つのアダプターです。

**確認すること:** 5xx の `http request` の行を `http.route` ごとに見たものと、それらのリクエストのエラーの行。**対処:** 依存先を直すか、エラーをバグとして報告します。

<!-- source: myapilatencyhigh 81b66aee9099 -->
### MyAPILatencyHigh

クライアント API の 95 パーセンタイルが 300 ms を超えています（ウォームな状態で 20 件の投稿のホームタイムラインのページの目標値）。ストリーミングのエンドポイントは除いています。

確認すること:

- `http_route` ごとの `http_server_request_duration_seconds`: 1 つのルートだけが遅いのか、すべてが遅いのか。
- データベース: `MyDBPoolWaiting`、データベースの CPU とスロークエリログ、サーバーとデータベースの間のネットワークの距離。
- プロセス: CPU のスロットリング、`MyMemoryNearLimit`（メモリが逼迫した状態での GC）、`go_schedule_duration_seconds`（CPU を待っているゴルーチン）。
- 有効にしていれば、遅いリクエストのトレース。

対処:

- データベースやサーバーのリソースを増やすか、両者を近くに置きます。
- ゼロまでスケールしたあとのコールドスタートは、静かなサーバーでパーセンタイルを押し上げます。これは想定どおりです。

<!-- source: mytargetdown e0308f141452 -->
### MyTargetDown

Prometheus がプロセスをスクレイプできません。

確認すること:

- プロセスが動いていること: `docker compose ps`、`kubectl get pods`、またはプラットフォームのコンソール。最後のログの行から、停止した理由が分かります。
- メンテナンスモードではないか（メトリクスは提供されません。想定どおりです）。
- `metrics_exporter = prometheus` になっていて、`metrics_listen` に Prometheus から届くこと。

対処:

- プロセスを再び起動します。起動しない場合は、ログに理由があります。設定の誤り（`my config check` で分かります）、データベースに届かない、スキーマが適用されていない、などです。

<!-- source: mynotready 1092418fbba2 -->
### MyNotReady

レディネスチェックが失敗しています。データベースに届かないか、そのテーブルがこのバージョンと合っていません。ロードバランサーはそのプロセスへのトラフィックの送信をやめ、
外部向けのヘルスエンドポイントは `unavailable` を返します。

確認すること:

- レディネスチェックのログの行: 接続のエラー（ネットワーク、認証情報、データベースの停止や接続数の枯渇）、または足りないテーブル。
- データベースの事業者のステータスページとコンソール: 動いているか、ストレージが足りないか、メンテナンス中か。
- アップグレードのあと: 新しいバージョンを起動する前にスキーマを適用したか（[データベースの初期化](database.md)）。

対処:

- データベースが停止している、または届かない: 復旧するか、フェイルオーバーします。作業の間は、メンテナンスモードで利用者に分かりやすいページを見せられます。
- データベースのストレージが足りない: プランを上げるか、空きを作ります（[MyDatabaseStorageLarge](#mydatabasestoragelarge)）。
- スキーマが適用されていない: 同じバージョンの `my schema apply` を実行すると、プロセスは自然にレディになります。
- データの消失や破損: バックアップから復元します。バックアップの鍵を封印した KEK は残しておいてください（[KEK の管理](key-management.md)）。

<!-- source: myprocessrestarting d96b041c2ff0 -->
### MyProcessRestarting

1 時間にプロセスが 3 回再起動しました。

確認すること:

- プラットフォームが示す再起動の理由: `OOMKilled`（メモリ）、失敗した liveness プローブ、または終了コード。
- 各再起動の直前のログの行に、パニックや致命的な設定の誤りがないか。

対処:

- メモリ不足: `GOMEMLIMIT` をコンテナーのメモリ上限の約 90% に設定するか、コンテナーにメモリを多く割り当てます（[MyMemoryNearLimit](#mymemorynearlimit)）。
- パニック: その前後のログの行を添えて報告してください。
- データベースが別の `host` や `account_domain` のものだとログに出ている: 記録されている値を設定に書くか、`database_url` を正しいデータベースに向けます。これらの値は変えられません（[あとから変えられない設定](production-checklist.md#settings-you-cannot-change-later)）。

<!-- source: mymemorynearlimit 438190b311c9 -->
### MyMemoryNearLimit

Go ランタイムが `GOMEMLIMIT` の 90% を超えて使っています。上限に近づくとガベージコレクターがずっと頻繁に動くようになり、CPU とレイテンシを消費します。
上限を超えると、プラットフォームがプロセスを強制終了することがあります。このアラートには `GOMEMLIMIT` が必要です。設定していない場合は、代わりにコンテナーのメモリのメトリクスを監視してください。

**確認すること:** `http_server_active_requests` と開いているストリーム（`my_stream_connections`）、そして負荷が横ばいなのにメモリが増え続けていないか（リーク）。

**対処:** コンテナーのメモリを増やし、それに合わせて `GOMEMLIMIT` も上げるか、プロセスを増やします。負荷に連動しない一定の増加は報告してください。

<!-- source: mydbpoolsaturated a737e35a86f5 -->
### MyDBPoolSaturated

あるプロセスのデータベース接続の 90% を超えて使われています。まもなく、リクエストが接続を待つようになります（`MyDBPoolWaiting`）。

**確認すること:** トラフィックが増えたのか、クエリが遅くなったのか（データベースのスロークエリログ、`my_adapter_operation_duration_seconds`）。

対処:

- `database_url` でプールのサイズを上げます（PostgreSQL では `pool_max_conns`）。すべてのプロセスの合計がデータベースの接続数の上限を下回るようにしてください。
- 多くのプロセスがデータベースを共有している場合は、その前にコネクションプーラーを置きます。

<!-- source: mydbpoolwaiting eacb1d75cd54 -->
### MyDBPoolWaiting

処理が空いたデータベース接続を待っているか、[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) のあとで待つのをやめています（タイムアウトは PostgreSQL だけ）。リクエストは `503` を返し、ジョブはあとでもう一度実行されます。[MyDBPoolSaturated](#mydbpoolsaturated) に従ってください。
データベース自体が接続数の上限に達していないことも確認してください。

<!-- source: mydatabasestoragelarge edf767115918 -->
### MyDatabaseStorageLarge

データベースが計画より大きくなっています。`my_db_size_bytes` は、PostgreSQL のデータベースのサイズ（`pg_database_size`。事業者のストレージの
上限が数えるもの）、または SQLite のファイルのサイズ（ログ先行書き込みのファイルを除く）です。これは障害ではなく増加です。ルールのしきい値は、データベースのプランのストレージの
上限、または SQLite のファイルを置くボリュームの上限から余裕を引いた値にしてください。そのボリュームには、ログ先行書き込みのファイルとバックアップのための空きも必要です。

対処:

- プランやボリュームを大きくします。
- リモートの投稿は、インスタンスの設定 `remote_status_retention_days` のあとで削除されます。これを下げると増加が緩やかになります。
- 削除された行の領域はどちらのデータベースでも再利用されますが、サイズが小さくなることはまれです。PostgreSQL はテーブルの領域を保持し、SQLite は空きページを
  `VACUUM` まで（サーバーを止めた状態で `sqlite3` で実行します）ファイルの中に残します。

<!-- source: myfiledescriptorsnearlimit 8a3112c11968 -->
### MyFileDescriptorsNearLimit

プロセスが、ファイルディスクリプターの上限の 80% を超えて開いています。上限を超えると、新しい接続が失敗します。

**対処:** 上限（`ulimit -n`、コンテナーランタイムの `nofile`）を上げるか、プロセスを増やします。何が占有しているかは `my_stream_connections` と
`http_client_open_connections` で確認してください。

<!-- source: myadaptererrors fcf1d91a0f01 -->
### MyAdapterErrors

1 つのアダプターの操作の 5% を超えて失敗しています: `kv.valkey`（Valkey）、`kv.db`（KV のテーブル）、`blob.fs`（メディアのディレクトリ）、
`queue.db`（キューのテーブル）、または `stream.<backend>`（プロセス間の通知）。

**確認すること:** そのアダプターのエラーの行と、その背後のサービス。Valkey が動いていて届くこと、メディアのボリュームがマウントされ、書き込めて、いっぱいでないこと。

**対処:** そのサービスを直します。失敗したジョブの操作は再試行されます。失敗したストリームの通知は、ライブの更新を遅らせるだけです。

<!-- source: myremotemediacacheoverlimit dea5d39a7027 -->
### MyRemoteMediaCacheOverLimit

リモートのメディアのコピーが `remote_media_cache_max_bytes` を超えたままです。通常は 1 時間ごとの掃除が、期限切れのコピーと最近最も使われていないコピーを削除します。
ルールのしきい値は、あなたの `remote_media_cache_max_bytes` の値にしてください。

**確認すること:** ジョブの画面とログに掃除のジョブの失敗がないかと、ブロブストレージの `MyAdapterErrors`。

**対処:** 失敗の原因を直すか、キャッシュのサイズやインスタンスのリモートのメディアのポリシーを下げます。

<!-- source: mymediastoragelarge ff5e5906636c -->
### MyMediaStorageLarge

メディアのファイルが計画より大きくなっています。これは障害ではなく増加です。ルールのしきい値は、ストレージのプランや予算が許す値にしてください。

**対処:** プランや予算を上げるか、アップロードの上限（`media_max_image_bytes`、`media_max_av_bytes`）を下げます。

<!-- source: myemailfailing f7a0c64c5c60 -->
### MyEmailFailing

直近 1 時間に、メールのジョブが最終的に失敗しました。確認、パスワードの再設定、パスワードに関するお知らせ、通知、または閉鎖のメールです。
確認や再設定のメールを待っている利用者は、手続きを終えられません。

**確認すること:** メールのジョブの種類（末尾が `_email`）で絞り込んだジョブの画面と、最後のエラー。[メールの送信](email.md#look-at-send-failures)に
エラーとその意味の一覧があります。

対処:

- SMTP のホスト、ポート、認証情報の誤り、または SES のアカウントや権限の問題: 直してから、失敗したジョブをもう一度実行します。
  リンクの有効期間より古い確認や再設定のメールは、再試行しても送信されずに完了するため、それらの利用者には新しいメールを申し込むよう伝えてください。
- 恒久的に拒否されたアドレスは送信停止になります。何もする必要はありません。

<!-- source: myemailretrying 40d3ed1a76f5 -->
### MyEmailRetrying

ほとんどのメールの送信が失敗して再試行を待っています。メールサーバーに届かないか、ログインを拒否されるか、送信の頻度を制限されています。再試行は、
トランザクションメールでは約 15 分、通知では約 4 時間続き、そのあとで最終的に失敗します。そうなる前に [MyEmailFailing](#myemailfailing) に従ってください。

<!-- source: cost-and-budget 5a63bdd2754d -->
### 費用と予算

サーバーは費用についてアラートを出しません。請求を最初に知る事業者の側で、アラート付きの予算を設定してください。

- AWS: AWS Budgets のコスト予算を作り、たとえば月額の 80% と 100% でアラートを出し、このサーバーのコスト配分タグで絞り込みます。
- Google Cloud: プロジェクトの Cloud Billing の予算を作り、50%、90%、100% にアラートのしきい値を設定します。同じ予算の通知を、
  費用の取り込みに使うこともできます（[クラウドの利用料金を取り込む](billing-import.md)）。

取り込んだ請求と月初からの見積もりは、「管理」→「費用」→「利用料金の取り込み」に表示されます。失敗した取り込みはそこに記録され、翌日の取り込みで再試行されます。
[`MyBillingImportFailing`](#mybillingimportfailing) は、その月の数値が古くなる前に知らせます。

<!-- source: mybillingimportfailing da997f400ac4 -->
### MyBillingImportFailing

ある課金ソースの利用料金の最新の取り込みが失敗し、そのあと 6 時間、成功した取り込みがありません。取り込みは 1 日に 1 回実行されます。スロットリングと
利用できない状態は数時間のうちに再試行され、そのほかの失敗は翌日の取り込みを待つため、それまで費用の画面には古い数値が表示されます。

確認すること:

- 「管理」→「費用」→「利用料金の取り込み」: そのソースの最後の取り込みとそのエラー。
- `error_type` ごとの `my_billing_imports_total{my_outcome="failed"}`、または `msg` が `billing import failed` の `WARN` のログの行とその `source` と
  `error_kind`。[クラウドの利用料金を取り込む](billing-import.md)に、種類とそれぞれの直し方の一覧があります。

対処:

- 原因（権限、課金のエクスポート、リクエストの上限）を直してから、翌日を待たずに「管理」→「費用」→「利用料金の取り込み」で「今すぐ取り込む」を押します。

Prometheus がない場合（`metrics_exporter = none` の Cloud Run）は、代わりに `billing import failed` の行に対するログベースのアラートを作ってください。

<!-- source: telling-users-about-an-incident e8cb0ca13fe8 -->
## 障害を利用者に知らせる

1. **アラートを確認済みにして**、ほかの当番の人に誰かが対応していることを知らせます。
2. 利用者に影響が出たらすぐに**ステータスページに投稿します**（`status_page_url`。[サーバーを外から監視する](monitoring.md#a-status-page)を参照）。
   何が壊れているか、いつからか、次にいつ投稿するかを書きます。ステータスページはこのサーバーが落ちていても届き、Web クライアントからリンクされています。
3. 利用者がまだサーバーに届く場合（停止ではなく劣化している場合）は、**お知らせを投稿します**。「管理」→「お知らせ」で本文を書いて公開します。
   お知らせは全員のホームタイムラインの上に表示されます。サーバーを止める必要がある場合は、代わりに `maintenance_message` を付けてメンテナンスモードを使います。
4. 何も変わっていなくても、約束した時刻に両方を**更新します**。
5. **終了**: ステータスページで障害を解決済みにし、お知らせを取り下げ（または短い続報を公開し）、記憶が新しいうちに何が起こり、
   何を変えたかを書き留めます。
