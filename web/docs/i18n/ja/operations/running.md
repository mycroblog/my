<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# サーバーの運用: 起動、停止、ヘルスチェック

サーバーの起動と停止、イメージの入手、そして稼働中の監視 (メトリクス、トレース、ジョブキュー、ヘルスチェック) の方法を説明します。

<!-- source: start-the-server 93f89c7d7b66 -->
## サーバーを起動する

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` は設定を検証し、データベース、ジョブキュー、鍵管理のアダプターを組み立てて、`listen` で HTTP を受け付けます。
既定の `tls = off` では、そこで素の HTTP/1.1 を話します。前段に、HTTPS (クライアント向けには HTTP/2 や HTTP/3 も) を終端して HTTP/1.1 で転送するリバースプロキシかロードバランサーを置いてください。
プロキシなしで HTTPS を配信するには、[サーバー自身による HTTPS の配信](#serving-https-from-the-server)を参照してください。
リアルタイム更新 (`GET /api/v1/streaming`、ローカルタイムラインと連合タイムラインのストリーム、通話のルームのストリーム) は長く続くレスポンスで、30 秒ごと (通話のルームは 15 秒ごと) にハートビートのコメントを送り、何もない間も開いたままです。
プロキシはこれらのレスポンスをバッファリングしてはいけません (nginx の場合、サーバーが `X-Accel-Buffering: no` を送ります)。また、プロキシのアイドルタイムアウトや読み取りタイムアウトはハートビートの間隔より長くしてください。
スキーマの適用とデータのアップグレードの手順は実行しません。起動する前に `my schema apply` を、続けて `my db upgrade` を実行してください ([データベース](database.md)を参照。アップグレードでは[アップグレード](database.md#upgrading)の手順の中で両方を実行します)。`my db upgrade` がそのリリースのすべての手順を記録するまで、`/readyz` は `503` を返します。
Docker Compose の構成 (`deploy/docker/compose.yaml`。[Docker Compose でのデプロイ](deploy-docker-compose.md)を参照) では、一度だけ動く `schema` と `upgrade` のサービスが、サーバーの起動前に `my schema apply` と `my db upgrade` を実行します。
スキーマが適用されていなくてもプロセスは起動しますが、readiness は 503 を返し、スキーマができるまでほかには何も提供しません。

データベースを使う最初の `my serve` や `my` のコマンドが、`host` と `account_domain` をデータベースに記録します。設定が記録された値と異なると、`my serve` は両方の値の組を添えて `start failed` をログに出し、終了コード 1 で終わります。コマンドも同じエラーを表示します。例外は、確認しない `my schema apply` と `my db backup`、そしてデータベースを読まないメンテナンスモードです。`my config check -instance` と `my db upgrade -check` は確認しますが、何も記録しません。
起動時にデータベースを読めなかったプロセスは、読めるようになるまで ready になりません。それまではジョブを実行せず、ヘルスチェック (`/healthz`、`/readyz`、`/health/live`、`/health/ready`、`/api/v1/instance/health`) を除くすべてのリクエストに `503` (`Retry-After` 付き) で応えます。`my schema apply` の前に起動したサーバーも同じように動きます。そのあと異なる値を見つけると、同じエラーで停止します。

`production` プロファイルでは、`my config check` は有効な設定が本番のインスタンスに向いているかも確認します。ほかのサーバーから届かない `host` や、ランダムでない KEK などのエラーは終了コード 1 になり、`mail = none` などの警告はそうなりません。
`my serve` は起動時に両方を警告としてログに出しますが、そのまま起動します。そのため、`my config check` はデプロイの手順の 1 つとして実行してください。
[本番前のチェックリスト](production-checklist.md)に、確認項目と、公開前に手で確かめることがまとめてあります。

| `role` | HTTP | ジョブワーカー |
| --- | --- | --- |
| `all` (既定) | すべてのルート | 動く |
| `web` | すべてのルート | 動かない |
| `worker` | ヘルスチェックだけ (プラットフォームのプローブ用) | 動く |

ログは JSON Lines として標準エラーに出ます。どの行にも `level` (`DEBUG`、`INFO`、`WARN`、`ERROR`) と `severity` があります。後者は同じレベルを Cloud Logging が読む形で表したものです (`DEBUG`、`INFO`、`WARNING`、`ERROR`、そしてプロセスを止めるエラーには `CRITICAL`)。
リクエストの処理中に書かれたログには `request_id` が付き、同じ ID がレスポンスヘッダー `X-Request-Id` で返されます。
リクエストの `X-Request-Id` が英数字と `-_.:` からなる 128 文字以内なら、ロードバランサーが割り当てた ID としてそのまま使います。
トレースの中のログ (リクエストとジョブ) には `trace_id` と `span_id` も付きます ([トレース](#traces)を参照)。

HTTP リクエストとジョブの実行は、それぞれ終わったときに 1 行を書きます。この行は下の[メトリクス](#metrics)のラベルと同じ名前と値を持つので、
`metrics_exporter = none` でも、プラットフォームはこれらの行からログベースの指標 (カウンターと分布) を作れます。

| 行 (`msg`) | フィールド |
| --- | --- |
| `http request` (`INFO`。ヘルスチェックは `DEBUG`) | `http.request.method`、`http.route`、`my.surface`、`http.response.status_code` (数値) は `http.server.request.duration` のラベルと同じ値です。ほかに `duration_seconds` (秒数)、`url.path` (クエリ文字列を除いたパス)、`http.response.body.size`、`request_id` があります。ルートに届かなかったリクエスト (ヘルスチェック、ルーティングの前に拒否されたリクエスト) には `http.route` と `my.surface` がなく、メトリクスも数えません。サーバーが応える前にクライアントが去ったリクエストは、ハンドラーがパニックした場合を除き、5xx ではなくステータス `499` になります。クライアントは何も受け取っていないので、サーバーのエラーとしては数えません |
| `job` (`INFO`: 成功と `postponed`、`WARN`: `retry`、`ERROR`: `dead`) | `my.job.type` と `my.outcome` は `my.job.duration` のラベルと同じ値です。ほかに `duration_seconds` (`my.job.duration`)、`wait_seconds` (`my.job.wait`)、`job_id`、`attempt`、失敗時の `error` があります |
| `error report` (`ERROR`。プロセスを止める失敗は `CRITICAL`) | 報告するエラー 1 件につき 1 行 ([エラーの報告](#error-reports)) |

`metrics_exporter = none` のときは、`http request` と `job` の行に、アダプターとデータベースプールの[システムのメトリクス](#system-metrics)も載ります。
リクエストやジョブがキュー、キーバリュー、ストリーム、blob の操作を実行したときは、`my.adapter.operations.total`、`my.adapter.errors.total`、そして `my.adapter` ごとのエラーである `my.adapter.errors` (`{"kv.valkey": 2}` など) です。
プールに上限があるときは、上限に対する使用中の接続である `db.client.connection.usage` です。0 でないときは、`my.db.client.connection.waits`、`db.client.connection.wait_time` (秒)、`db.client.connection.timeouts` で、
そのプロセスの前の行からの増分なので、行をまたいで合計すればプロセス全体の値になります。

<!-- source: error-reports b70c69ddef24 -->
### エラーの報告

各プラットフォームでこれらの行をどこで読むか、「エラー」画面、通知、エラートラッカーへの送信は、[エラーの報告](error-reporting.md)のガイドで説明しています。

サーバーは、運営者の対応が必要なエラーを、1 件ごとに `error report` の 1 行として報告します。

- HTTP ハンドラー、ジョブ、バックグラウンドのタスクでのパニック
- クライアント API、管理 API、ログインのページ、ActivityPub、OAuth、メディアの 5xx レスポンスの原因になったエラー
- 最終的に失敗したジョブ (`dead`): 再試行を使い果たしたか、もう一度実行するのを拒否したもの。相手のサーバーが失敗し続けたためにあきらめた配送は報告しません
- 起動の失敗: 無効な設定や、準備できないデータベースやアダプター

4xx レスポンス、ジョブの再試行、メンテナンスモード、readiness チェックの失敗、クライアントが去ったリクエスト、ほかのサーバーが原因の失敗は報告しません。これらはメトリクスとほかのログ行で扱います。

リクエストのクライアントが去ったとき、そのリクエストについてサーバーが出すほかのエラー行は、`ERROR` ではなく `INFO` で書かれます。そのほとんどは、リクエストが取り消されたことを伝えるだけだからです。

この行は Google Cloud Error Reporting が読む形です。Cloud Run では、設定なしで報告が Error Reporting にエラーとしてまとめて表示されます。ほかのプラットフォームでは、ただの JSON の行です。

| フィールド | 値 |
| --- | --- |
| `@type`、`serviceContext` | Error Reporting のイベントの種類と、`{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 同じエラーなら発生のたびに同じになる 16 進数 32 桁。発生元、場所、エラーの型、スタックにあるサーバーの関数から、行番号を除いて作ります |
| `my.error.source`、`my.error.kind` | `server` (または `web`) と、`panic` または `error` |
| `my.error.where` | 起きた場所。リクエストなら `<surface> <method> <route>` (`client_api GET /api/v1/statuses/{id}` など)、ジョブならジョブの種類、バックグラウンドのタスクならタスクの名前、起動の失敗なら `startup` |
| `my.role`、`deployment.environment.name` | プロセスの `role` と `errors_environment` |
| `request_id`、`job_id`、`attempt`、`trace_id`、`span_id` | 同じエラーのリクエストやジョブの行を見つけるための ID (ある場合) |
| `context.httpRequest` | リクエストのエラーの場合: `method`、`url` (`/api/v1/statuses/{id}` のようなルートで、クライアントが要求したパスではありません)、応答したステータスである `responseStatusCode` (500) |
| `message` | 行の最後にあり、`<type>: <message>`、改行、Go のスタック |

報告がプロセスを出る前に、メッセージの中の URL、メールアドレス、`@user@host` のハンドル、IP アドレス、UUID と ULID、10 桁以上の数は、それぞれ `<url>`、`<email>`、`<acct>`、`<ip>`、`<id>`、`<n>` に置き換えられ、1 KiB に切り詰められます。
スタックはファイルの位置を残しますが、関数の行の引数の値は残さず (`(...)` と表示されます)、16 KiB に切り詰められます。リクエストの本文、ヘッダー、クエリ、Cookie、アカウント、ユーザー名、IP アドレス、User-Agent を含む報告はありません。
スタックを含む行はほかにないので、Error Reporting がスタックを見つけるのはこれらの行だけです。

フィールド名は `.` を含む 1 つのキーで、入れ子ではありません。Cloud Logging のフィルタや抽出では引用符で囲んでください (たとえば `jsonPayload."http.route"`、分布の値なら `EXTRACT(jsonPayload.duration_seconds)`)。
ラベルとして使うフィールドは、有限の集合の値だけをとります。`url.path`、`request_id`、`job_id` がラベルになることはありません。

`metrics_exporter = emf` のときは、この 2 つの行がそのまま EMF の行になります。`http request` の行は `http.server.request.duration` の値を、`job` の行は `my.job.duration` と `my.job.wait` の値を持ち (キーはメトリクスの名前です)、
さらに `_aws` ディレクティブ、`service.name`、`service.version`、`my.role`、`service.namespace` (`host`) を持ちます。この 3 つのメトリクスはほかの行には書かれません。`metrics_emf_dimensions` で選んだフィールドは、EMF の求めるとおり文字列として書かれます (たとえばディメンション `http.response.status_code` は `"200"`)。
ログは標準エラーに、ほかの EMF の行は標準出力に出ます。ECS のログドライバー `awslogs` は両方を CloudWatch Logs に送るので、メトリクスは両方から取り出されます。

<!-- source: docker-compose-files 9f0633524036 -->
## Docker Compose のファイル

`deploy/docker/` には、インスタンスをデプロイするためのものが入っています。`compose.yaml` は単独で動き、ほかのファイルはその上に重ねます。

| ファイル | 加えるもの | 使うとき |
| --- | --- | --- |
| `compose.yaml` | サーバーと、一度だけ動くデータベースのサービス。SQLite とメディアのファイルはボリューム `data` に置きます | 常に |
| `compose.postgres.yaml` | コンテナで動く PostgreSQL。ボリューム `postgres` に置きます | このマシンで PostgreSQL を使いたいとき。別の場所にある PostgreSQL サーバーを使うなら、このファイルは使わず、`.env` に `MY_DATABASE` と `MY_DATABASE_URL` を設定します |
| `compose.s3.yaml` | S3 互換のバケットに置くメディア。`COMPOSE_PROFILES=garage` なら、ボリューム `garage` に置く 1 ノードの [Garage](https://garagehq.deuxfleurs.fr/) のストアも加えます | メディアが 1 台のディスクに収まらなくなったとき、またはバケットに置きたいとき。プロファイルがなければ、ストアは `MY_BLOB_S3_*` の設定で指定する外部のものです |
| `compose.caddy.yaml` | ポート 80 と 443 で TLS 証明書を扱う Caddy | `MY_HOST` が公開のドメインで、このマシンでほかに TLS を終端するものがないとき |
| `compose.build.yaml` | チェックアウトからイメージをビルドする | 自分でビルドするとき |

データベースとメディアのストアは、最初に起動する前に選んでください。ファイルを変えても、データはひとりでには移りません
(移す必要があるものは[バックアップ](backup.md)にあります)。

**ファイルを重ねる**には、その順に `-f` オプションで指定するか、`.env` に一度だけ書きます。そうすると、以降のすべての `docker compose` コマンド
(`exec`、`logs`、`pull`、ほかのガイドにあるもの) が同じファイルを使います。

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

ディレクトリにある `compose.override.yaml` という名前のファイルは、`-f` も `COMPOSE_FILE` もファイルを指定していないときだけ読まれます。
それらを使うときは、このファイルも並べてください。

**設定。** `example.env` を `.env` にコピーします。Compose は `.env` を読んでファイルの変数を埋め、ファイル全体をサーバーのコンテナにも渡します。
そのため、[設定リファレンス](../reference/configuration.md)のどの設定も、そこに書けば効きます。
テンプレートにはすべての設定が、それぞれ一文の説明付きで並んでいます。

- 鍵暗号化鍵 (KEK) は `.env` には入れず、ファイル `secrets/kek` に置きます。コンテナはそれを Docker のシークレット
  `/run/secrets/my_kek` として読むので、環境変数になることはありません。`.env` に `MY_KEK` の行があるとサーバーは止まります。
  サーバーは `MY_KEK` と `MY_KEK_FILE` を同時には受け付けないためです ([鍵の管理](key-management.md))。
- サーバーはユーザー `65532` として動き、Compose はファイルをその権限のままマウントするので、ファイルはほかのユーザーから
  読めなければなりません。`example.env` のコマンドのように、自分だけが入れるディレクトリに置いてください。
- ほかのシークレットは環境変数としてコンテナに届きます。代わりにファイルとして渡すには、自分の `compose.override.yaml` で
  シークレットとしてマウントし、その `<NAME>_FILE` を設定します。`compose.yaml` が鍵についてそうしているのと同じです。
- `.env` にはシークレットが入っています。`chmod 600 .env` を実行し、バックアップと一緒に写しを保管してください。
- ファイルがそのポートにつなぐので、`MY_LISTEN` は `:8080` のままにします。ただし、サーバー自身が HTTPS を配信する場合は別です
  ([サーバー自身による HTTPS の配信](#serving-https-from-the-server))。

**Caddy での TLS。** `MY_HOST` には、DNS がこのマシンを指し、インターネットからポート 80 と 443 に届くドメインを設定し、
`.env` から `MY_WEB_ORIGIN` を削除します。Caddy は最初のリクエストで証明書をひとりでに取得し、更新します。
サーバーは Docker の既定のアドレスプールからの `X-Forwarded-For` を信頼します。プールが異なるなら `MY_TRUSTED_PROXIES` を設定してください。

**同梱の Garage。** `COMPOSE_PROFILES=garage` での `compose.s3.yaml` には、`example.env` の先頭に並ぶ値が必要です。
一度だけ動くサービス `garage-init` が、`up` のたびにストアを準備します。サーバーがメディアを自分で配信するので、ストアは Compose のネットワークの
中にとどまります。1 ノードは 1 台のディスクです。ボリューム `garage` かバケットをバックアップしてください ([バックアップ](backup.md#media-in-a-bucket))。

<!-- source: serving-https-from-the-server 45ec9231a41e -->
## サーバー自身による HTTPS の配信

`my` は自分で HTTPS を配信できます。そのため、小さなマシンでは、ドメインとポート 80・443 があれば、リバースプロキシなしで 1 つのバイナリがサーバー全体を動かせます。
`tls = acme` なら、Let's Encrypt などの ACME 認証局から証明書を取得します。`tls = files` なら、あなたが用意し、certbot などの別のツールで最新に保つ証明書と鍵を配信します。

```sh
MY_TLS=acme
MY_TLS_ACME_AGREE_TERMS=true            # you agree to the terms of service of the certificate authority
# MY_TLS_ACME_EMAIL=ops@social.example  optional contact of the account
# MY_LISTEN=:443          the default with tls other than off
# MY_TLS_HTTP_LISTEN=:80  the default; empty opens no port 80
```

```sh
MY_TLS=files
MY_TLS_CERT_FILE=/etc/letsencrypt/live/social.example/fullchain.pem
MY_TLS_KEY_FILE=/etc/letsencrypt/live/social.example/privkey.pem
```

- **ポート。** サーバーは `listen` (`tls` が `off` 以外になると既定は `:443`) で HTTPS を配信します。`tls_http_listen` (既定は `:80`) では、`GET` と `HEAD` のリクエストを `https` の同じパスへ `301` で転送し、それ以外のメソッドには `400` で応えます。リクエストの本文が平文で流れることはありません。
- **ホスト。** サーバーは `host` と、異なる場合は `web_origin` のホストを配信します。それ以外の名前に対するハンドシェイクは、証明書なしで失敗し、サーバーが証明書を注文することもありません。
- **接続。** TLS 1.2 以降で、暗号スイートは Go の既定のもの、HTTP/2 と HTTP/1.1 に対応し、リクエストヘッダーは最大 64 KiB、HTTP/2 の接続あたりの同時ストリームは最大 100 です。タイムアウトは素の HTTP と同じです。サーバーは引き続きすべてのレスポンスに `Strict-Transport-Security` を付けます ([セキュリティヘッダー](#security-headers))。
- **ロール。** TLS で待ち受けるのは `web` と `all` のロールだけです。`worker` は、ヘルスチェックを `listen` (既定は `:8080`) の素の HTTP で提供し、ポート 80 は開きません。
- **クライアントのアドレス。** 前段にプロキシがなければ、接続の相手がクライアントです。`trusted_proxies` は空のままにしてください。前段の CDN が `https` でサーバーに接続する場合にだけ設定します。
- **メンテナンスモード。** `my serve -maintenance` は `tls` も証明書も読まず、`listen` (設定しなければ `:8080`) で素の HTTP で応えます。メンテナンス中は `MY_LISTEN` とファイアウォールをそのポートに合わせるか、メンテナンス用にリバースプロキシを残してください。

<!-- source: certificates-from-an-acme-certificate-authority 52037a75d641 -->
### ACME 認証局からの証明書

`tls = acme` では、サーバーは、証明書を持たない各ホストの証明書を、`tls_acme_directory` の認証局 (既定は Let's Encrypt) に注文します。

- **始める前に。** `host` はサーバーを指す DNS 名で、ポート 443 と 80 にインターネットから届く必要があります。認証局は、その名前にサーバーが接続してその名前を管理していることを確かめます。確認には `listen` (TLS-ALPN-01 チャレンジ) か `tls_http_listen` (HTTP-01 チャレンジ) で応えるので、`tls_http_listen` を空にしても、ポート 443 だけで動きます。ワイルドカード証明書と DNS レコードによる検証は使えません。
- **最初の起動。** サーバーは証明書を待たずに待ち受けを始め、そのあと注文します。ホストが証明書を得るまで、そのホストへのハンドシェイクは失敗します。Let's Encrypt では、最初の証明書は通常 1 分以内に届きます。ログには、ホストごとに `obtained a TLS certificate` と出て、注文が失敗したときは理由とともに `the TLS certificate could not be obtained` と出ます。
- **更新。** サーバーは、認証局が証明書ごとに示す時刻 (ACME Renewal Information) に更新します。示された期間の中のランダムな時刻を一度選び、期間が変わらない間はそれを保ち、認証局の `Retry-After` が示す間隔で、少なくとも 12 時間ごとに問い合わせ直します。証明書を早めに入れ替える必要のある認証局が期間を動かせるようにするためです。認証局がそのような期間を示さないときは、証明書の有効期間の 3 分の 2 が過ぎたところで更新し、有効期間が 10 日未満の証明書では半分が過ぎたところで更新します。更新では、置き換える証明書を認証局に伝えるので、Let's Encrypt のレート制限の対象外になります。更新の時刻が来るまで現在の証明書が使われ続け、更新に失敗したときは、最初の注文のときと同じように待ちます。
- **失敗した注文。** 注文に失敗すると、同じホストの次の注文までサーバーは待ちます。最初の失敗の後は 1 分、2 回目の後は 10 分、3 回目の後は 100 分、その後は 1 日です。認証局の制限を超えないようにするためです。待ち時間はデータベースに保存され、再起動しても残ります。原因 (DNS、ファイアウォール、ポート) を直せば、待ち時間が終わったところで次の注文が出ます。
- **保管場所。** 認証局のアカウント、証明書、すべての注文の記録はデータベースに保存されます。秘密鍵は、ほかのシークレットと同じように鍵暗号化鍵で封じられます ([鍵の管理](key-management.md))。データベースのバックアップにはこれらが含まれ、再起動したサーバーは、注文せずに保存された証明書をすぐに配信します。鍵暗号化鍵を変更すると、これらは包み直されます。新しい証明書は注文されません。
- **複数のプロセス。** データベースと `kv` ストアを共有するプロセスは協調します。あるホストの証明書は 1 つのプロセスが注文し、ほかのプロセスは待ってからそれを読み込みます。認証局が接続したプロセスが、どれでもその確認に応えます。`kv = db` か `kv = valkey` を使ってください。`kv = memory` は、1 つのプロセスがすべてを動かす (`role = all`) 場合を除いて拒否されます。証明書を取得したプロセスは [`stream`](../reference/configuration.md#stream) のブローカー経由でほかのプロセスに知らせ、数秒のうちにそれが配信されます。そのため、`tls = acme` の `web` プロセスは、ブローカーを待ち受け続けます。`stream = hub` のときや、通知が失われたときでも、すべてのプロセスはデータベースから現在の証明書を 5 分ごと、証明書が更新の時刻に達してからは 1 分ごとに読むので、更新は古い証明書の期限よりずっと前に全プロセスに届きます。
- **プロファイル。** `tls_acme_profile` は、認証局が提供するプロファイル (Let's Encrypt の `shortlived` など) を選びます。空なら認証局の既定です。プロファイルを変更すると、次の更新はそれに従い、そこで得た証明書は、置き換える証明書より先に期限が切れる場合でも配信されます。

Let's Encrypt の本番環境の制限を使い切らずに試すには、まずステージング環境から始めます。ステージング環境は、ブラウザーが信頼しない証明書を、はるかに高い制限で発行します。

```sh
MY_TLS_ACME_DIRECTORY=https://acme-staging-v02.api.letsencrypt.org/directory
```

ログに証明書を取得したと出て、`curl -kv https://social.example/` が `(STAGING)` の証明書を示したら、この設定を削除して再起動します。サーバーは本番環境に新しいアカウントを登録し、そこで証明書を注文します。ステージングの証明書はデータベースに残り、配信されなくなります。

<!-- source: certificates-from-files 35f16ea23b33 -->
### ファイルからの証明書

`tls = files` では、`tls_cert_file` にサーバーの証明書と、それに続く中間証明書 (certbot の `fullchain.pem`) を入れ、`tls_key_file` にその鍵を入れます。証明書は `host` を含んでいる必要があります。異なる場合の `web_origin` のホストも、証明書がそれを含んでいれば配信されます。

サーバーは 1 分ごとに 2 つのファイルの更新時刻を調べ、どちらかが変わると、再起動せずに読み込み直します。まだ対応していない組み合わせ (新しい鍵より先に新しい証明書が置かれた場合など) は、次の確認まで前の証明書を使い続け、理由はログに出ます。

<!-- source: commands-and-metrics de97a3993e6b -->
### コマンドとメトリクス

`my tls` は、サーバーの設定を使って、データベースに届くどのマシンからでも証明書を表示・操作します ([管理 CLI](admin-cli.md#tls-certificates))。

```sh
my tls status                                          # each host: certificate, renewal, latest orders
my tls renew -host social.example                      # order now, without waiting
my tls revoke -host social.example -reason key_compromise
```

- **`status`** は、ホストごとに、現在の証明書 (シリアル番号、発行者、有効期限)、更新される時刻 (認証局が示す期間、または有効期間から導いた時刻)、証明書を得られなかった注文が連続した回数、直近の注文とそのエラーを表示します。`tls = files` では、`tls_cert_file` の証明書を表示します。
- **`renew`** は、更新の時刻と失敗した注文の後の待ち時間を飛ばして、今すぐ証明書を注文します。失敗した注文の原因を直したら実行してください。認証局の確認には動いているサーバーが応えるので、サーバーが起動している必要があります。各プロセスは数秒のうちに新しい証明書を配信します (`stream = hub` では 5 分以内)。
- **`revoke`** は、まず現在の証明書を置き換える証明書を注文し、そのあと認証局に現在の証明書の失効を求めて記録します。どのプロセスも、数秒のうちに (`stream = hub` では 5 分以内に)、失効した証明書の配信をやめます。秘密鍵がほかの人に知られたかもしれないときは `key_compromise` を、それ以外は `superseded` か `operator` を使います。置き換えの証明書を注文できないときでも失効は進みます。その場合、プロセスはそのホストの前の証明書が失効していなければそれを配信し、失効していれば、サーバーが新しい証明書を得るまで何も配信しません。
- `renew` と `revoke` には、`tls = acme` と、`memory` 以外の `kv` が必要です。認証局が接続するプロセスが、確認への応答を `kv` ストアから読むためです。`tls = files` では拒否されます。ファイルを書くツールで、証明書を更新または失効させてください。

[`metrics_exporter`](#metrics) を設定していれば、`my.tls.certificate.expiry` が 7 日未満になったときと、`my.tls.order.consecutive_failures` が 3 以上になったときにアラートを出してください ([アラート](alerts.md#mytlscertificateexpiring))。更新の失敗は、証明書が切れる何週間も前に現れます。

<!-- source: checks-and-running-it 983028d27675 -->
### チェックと実行

`my config check` は、`tls = acme` のとき、同意されていない利用規約、IP アドレスや `localhost`、ドットのない名前である `host`、`https` の URL ではない `tls_acme_directory`、`all` 以外の `role` での `kv = memory` を、エラーとして報告します。`tls = files` のときは、読めない証明書や鍵、証明書と合わない鍵、`host` を含まない・期限が切れている・まだ有効でない証明書をエラーとして報告し、残りが 14 日未満なら警告します。`tls` が `off` 以外で `trusted_proxies` が設定されていると警告します。`tls = off` で `listen` がループバック以外のアドレスのポート 80 か 443 のときも、前段で TLS を終端するものがないように見えるので警告します。`trusted_proxies` が空のそのようなサーバーが、ループバック以外のアドレスから素の HTTP のリクエストを受け取ると、サーバーは起動ごとに 1 回、同じ警告をログに出します。

**systemd。** 1024 未満のポートには、root 以外のユーザーなら capability が必要です。サービスにそれだけを与えてください。

```ini
[Service]
User=my
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

`tls = files` では、そのユーザーが証明書のファイルを読める必要もあります。certbot の `live` と `archive` のディレクトリは root だけが読めるので、サービスのグループにアクセスを与えるか (ディレクトリに `chgrp` と `chmod g+rx`、鍵に `g+r`)、デプロイフックでサービスが読めるディレクトリにファイルをコピーしてください。

**Docker。** Docker 20.10 以降は、コンテナが capability なしで 1024 未満のポートを開けるので、イメージはそのまま動きます。イメージは `MY_LISTEN=:8080` を設定しているので、`.env` で `MY_LISTEN=:443` を設定し、`compose.override.yaml` で両方のポートを公開します (`ports: ["80:80", "443:443"]`)。`tls = files` では、`MY_TLS_CERT_FILE` と `MY_TLS_KEY_FILE` が指す場所に証明書のファイルを読み取り専用でマウントします。サーバーはユーザー `65532` として動くので、そのユーザーが読める必要があります。`tls = acme` ではマウントするものはありません。証明書はデータベースにあります。
同じポートを使ってしまう `compose.caddy.yaml` は含めないでください。`network_mode: host` ではこの話は成り立ちません。コンテナがマシンのネットワークを共有するので、コンテナの外のサービスと同じ capability が必要です。

**Caddy との比較。** `my` から HTTPS を配信すると、HTTP/3、レスポンスの圧縮 (サーバーは Web クライアントのファイルを圧縮して送りますが、それ以外は圧縮しません)、独自のアクセスログ (サーバーの `http request` のログ行は残ります)、DNS で検証する証明書 (ワイルドカード証明書を含む) を使えなくなります。これらのどれかが必要なら、`compose.caddy.yaml` かほかのプロキシを使い続けてください。

<!-- source: get-the-image 123f0112b538 -->
## イメージを入手する

サーバーのイメージは `ghcr.io/mycroblog/my` です。公開されていて、`linux/amd64` と `linux/arm64` に対応し、すべてのアダプターを含みます。

| タグ | 指すもの | 使いどころ |
| --- | --- | --- |
| `0.1.0` | そのバージョン (変わりません) | バージョンを指定するとき |
| `0.1` | `v0.1` の最新のパッチ | `up` のたびにスキーマを適用する Docker Compose |
| `0.1.0-rc.1` | そのプレリリース | リリース前に試すとき |
| `nightly` | ナイトリーのテストに通った `main` の最新のコミット。変更があった夜ごとに動きます | プレリリースの前に変更を試すとき (捨てても構わないインスタンスで) |

`latest` と `0` はありません。どちらもマイナーバージョンをまたいでしまいますが、新しいマイナーバージョンへのアップグレードは毎回[一つ前のバージョンを経由](database.md#skipping-releases)しなければならないからです。どのコマンドでもバージョンを指定してください。`docker pull ghcr.io/mycroblog/my` だけでは `manifest unknown` で失敗します。
パッチリリースでも `my schema apply` と `my db upgrade` が必要なことがあるので、Docker Compose 以外では、バージョンごとに[アップグレード](database.md#upgrading)の手順に従ってください。本番では、リリースノートにあるダイジェストで固定します。

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

**`nightly` にはアップグレードの道筋がありません。** アップグレードを約束するのはリリースされたバージョンの間だけです。ある `nightly` のイメージから次のイメージへ、あるいは `nightly` からリリースへ移るとき、スキーマと `my db upgrade` のデータの手順は、どのリリースも通らない形で変わることがあるので、データベースは作り直すものと考えてください。本番では決して動かさないでください。`my version` はそれがどのコミットかを表示します (`v0.1.0-rc.5.0.20261109182600.g7a789720a1b2` のように)。最新のリリース (`v0.1.0-rc.5`) より後のバージョン、UTC のコミットの時刻、そして `g` の後にコミットの最初の 12 文字が続きます。1 つのビルドを使い続けるには、そのダイジェストで固定してください (`ghcr.io/mycroblog/my:nightly@sha256:...`)。古いビルドはタグを失いますが、ダイジェストなら引き続き pull できます。

動かす場所:

- **Docker Compose**: `deploy/docker/compose.yaml` は `.env` の `MY_VERSION` のイメージを動かします ([Docker Compose のファイル](#docker-compose-files))。代わりにチェックアウトからビルドするには、オーバーライド `compose.build.yaml` を加え (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`)、`MY_VERSION` を自分で決めた名前にします。この名前はローカルのイメージの名前になり、`my version` が表示するものでもあります。
- **Google Cloud と AWS**: OpenTofu のモジュールは、`ghcr.io` のイメージを、`ref` が指すリリースのままで動かします ([OpenTofu でのデプロイ](deploy-opentofu.md#the-root-modules))。
- **Kubernetes**: `ghcr.io` からそのまま pull します。

<!-- source: verify-an-image 76b9a1293edb -->
### イメージを検証する

リリースされたイメージにはそれぞれ、Sigstore で署名された来歴 (provenance) と SBOM があります。来歴は、ダイジェストとそれをビルドしたワークフローおよびコミットを結び付けます。[GitHub CLI](https://cli.github.com/) を使い、コマンドが求めるなら `ghcr.io` にログインして、次を実行します。

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

最初のコマンドは、イメージがこのリポジトリのリリースのワークフローでビルドされたものでなければ失敗します。`nightly` のイメージは、代わりに `--signer-workflow mycroblog/my/.github/workflows/nightly.yml` で検証してください。表示されるコミットはそのバージョンのタグのコミットで、イメージのラベル `org.opencontainers.image.revision` も同じコミットを指しています。

<!-- source: get-the-binary 2608f7bdd54d -->
## バイナリを入手する

コンテナなしで `my` を動かすために、各リリースにはイメージのバイナリが、`linux/amd64` と `linux/arm64` 向けに `my_<version>_linux_<arch>.tar.gz` として添付されています。
Web クライアントを内蔵した、静的リンクの単一のバイナリです。アーカイブには `LICENSE`、`licenses/` のライセンス表記、README も入っています。ほかのオペレーティングシステム向けのビルドはありません。

```sh
curl -fsSLO https://github.com/mycroblog/my/releases/download/v0.1.0/my_v0.1.0_linux_amd64.tar.gz
tar xzf my_v0.1.0_linux_amd64.tar.gz my
./my version
```

設定はイメージと同じです ([設定リファレンス](../reference/configuration.md))。バージョンごとに[アップグレード](database.md#upgrading)に従ってください。スキーマは何も適用してくれません。

<!-- source: verify-a-binary 03daabe0c940 -->
### バイナリを検証する

アーカイブにはそれぞれ、イメージと同じように、Sigstore で署名された来歴があり、リリースは `SHA256SUMS` にアーカイブの SHA-256 を一覧にしています。

```sh
gh attestation verify my_v0.1.0_linux_amd64.tar.gz -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
sha256sum --ignore-missing -c SHA256SUMS
```

<!-- source: publish-the-source-code-agpl-30 2e87c44ffb2f -->
## ソースコードを公開する (AGPL-3.0)

`my` のライセンスは AGPL-3.0-only です。AGPL-3.0 の第 13 条は、サーバーを動かしてネットワーク越しに利用者に使わせる人に、
動いているプログラムの対応するソースコードを入手する方法をその利用者に提供することを求めています。これは、アカウントを持つ人だけでなく、サーバーとやり取りするすべての人に当てはまります。

ソースを公開している場所は、設定 `source_url` (環境変数 `MY_SOURCE_URL`。[設定リファレンス](../reference/configuration.md#source_url)を参照) で示します。

| 値 | 意味 |
| --- | --- |
| 未設定 | 既定値の `https://github.com/mycroblog/my`。`my` を変更せずに動かすときに使います。その場合、リリースの `GET /instance` はそのバージョンのソース `https://github.com/mycroblog/my/tree/v0.1.0` を、NodeInfo はリポジトリを示します |
| `https:` の URL | ソースを読める場所 (フォークのリポジトリ、特定のバージョンのタグやアーカイブなど) |
| `mailto:` の URI | ソースを請求するメールアドレス (たとえば `mailto:source@example.org`。`?subject=...` も使えます) |

それ以外の値 (`http:`、`git@...`、相対パス、ユーザー情報や `#` を含む URL、アドレスのない `mailto:` など) では `my config check` が失敗し、サーバーは起動しません。

値は次の場所に表示されます。

- クライアント API の `GET /instance` の `source_url`
- Web クライアントのインスタンス情報のページ (`/about`) の「ソースコード」の欄と、ランディングページ (`/`) のリンク
- NodeInfo (`/nodeinfo/2.1`) の `metadata.sourceCode`。`https:` の URL は `software.repository` としても公開されます
  (NodeInfo 2.1 のスキーマでは `software.repository` に `http(s)` の URL しか使えないので、`mailto:` の値は `metadata.sourceCode` にだけ表示されます)

イメージには、`my` のライセンスが `/usr/share/doc/my/LICENSE` に、バイナリに含まれるサードパーティのコードとデータのライセンス表記が `/usr/share/doc/my/licenses/` に入っています。
`go-modules.md` は Go のモジュール、`npm-packages.md` と `npm-packages-sw.md` は Web クライアントの npm パッケージ、そのほかに、同梱のデータとフォントのライセンスです。
イメージやバイナリを配布するときは、これらのファイルも一緒に渡してください。

<!-- source: when-you-fork-or-modify-my be83256fe6a3 -->
### `my` をフォークしたり変更したりするとき

変更した `my` を動かす (パッチを当てる、フォークをビルドする、依存関係を差し替える) と、既定の URL はもう動いているもののソースを指しません。
`source_url` を、変更を含む、動いているバージョンのソースがある場所に変えてください。リポジトリの最新の状態ではなく、動いているバージョンに合うコミット、タグ、アーカイブを指すと、利用者は確実に同じソースを入手できます。
変更していない公式のリリースを動かすなら、既定値のままにしてください。

<!-- source: when-you-use-mailto 11155a8b71ed -->
### `mailto:` を使うとき

- 求められたら、対応するソースコード (ビルドに必要なスクリプトを含む) をすぐに渡せるようにしておいてください。渡す準備をせずにアドレスを示すだけでは、義務を果たしたことになりません。
- サーバーの運営をやめたり担当者が替わったりしても使えるアドレスを使ってください。個人の受信箱より、運営用の共有アドレスのほうが適しています。
- 請求がスパムフィルタで落ちないようにしてください。
- 公開してよいアドレスを使ってください。情報のページ、ランディングページ、NodeInfo で誰でも見られます。

<!-- source: metrics a5fd8092d408 -->
## メトリクス

設定 `metrics_exporter` (既定値 `none`) で、サービスのメトリクスを出力します。関連する設定は[設定リファレンス](../reference/configuration.md)にあります。

| `metrics_exporter` | 出力先 | 向いている環境 |
| --- | --- | --- |
| `none` | 何も出力しません。計測のコストはほとんどかかりません。インスタンス全体の値は 15 分ごとに `metrics snapshot` のログ 1 行として、アダプターとプールの値はリクエストとジョブの行に出力されます | Google Cloud (代わりにプラットフォームのメトリクスとログベースの指標を使います。[Google Cloud へのデプロイ](deploy-google-cloud.md#monitoring-and-alerts)) |
| `prometheus` | `metrics_listen` (既定値 `127.0.0.1:9464`) の `GET /metrics`。公開の `listen` とは別のリスナーで、認証はなく、どの `role` でも動きます | セルフホスト (Prometheus 互換のサーバーや KEDA が収集します) |
| `otlp` | OTLP/HTTP で OpenTelemetry Collector に送ります。送り先は `OTEL_EXPORTER_OTLP_ENDPOINT` などの OTel の標準の環境変数で決まります | Collector を動かせる構成 |
| `emf` | CloudWatch Embedded Metric Format の JSON の行。リクエストとジョブのメトリクスは上の終了時のログ行に載り、ほかのものはリクエストやジョブが終わったときに、ラベルの組ごとに 1 行ずつ標準出力に書かれます | AWS ([AWS へのデプロイ](deploy-aws.md#monitoring-and-alerts)) |

メトリクスの一覧は次のとおりです。Prometheus では、名前の `.` が `_` になり、単位の接尾辞 (`_seconds`、カウンターなら `_total`) が付きます。

| メトリクス | 種類 | ラベル |
| --- | --- | --- |
| `http.server.request.duration` | ヒストグラム、秒 | `http.request.method`、`http.route` (ルートのパターンのパス。一致するものがなければ `other`)、`http.response.status_code`、`my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | カウンター | `activity.type` (既知の Activity の種類、または `other`)、`my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`)、`my.inbox.rejection` (`rejected` のときだけ。署名や完全性の証明がない、無効、検証できない、または鍵を取得できなかったときは `signature`。停止されたドメインは `suspended`。`federation_off`。`rate_limited`。有効な `id`、`type`、`actor`、`object` のないアクティビティは `invalid`) |
| `my.relay.activities` | カウンター | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving`: リレーが運んだアクティビティを取り込んだか、捨てたならその理由。[連合のリレー](relays.md)を参照) |
| `my.move.activities` | カウンター | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`: 受け取った `Move` を記録したか、捨てたならその理由) |
| `my.delivery.attempts` | カウンター | `my.outcome` (`success` / `retry` / `gone` / `failed`。`gone` には 410 と、サーバーが利用できないか停止されているために飛ばした配送が含まれます) |
| `my.delivery.duration` | ヒストグラム、秒 | `my.outcome` |
| `my.ux.duration` | ヒストグラム、秒 | `my.ux.metric` (`lcp` / `inp` / `long_frame` / `transition` / `press` / `call_rtt` / `call_jitter`)、`my.ux.name` (ルート、操作の種類、アクション、または通話のトランスポート。[UX の計測](error-reporting.md#ux-measurements)) |
| `my.ux.call.packet_loss` | ヒストグラム、パーセント | `my.ux.name` (通話のトランスポート) |
| `my.plugin.failures` | カウンター | `my.plugin.kind` ([プラグイン](plugins.md)の種類)、`my.plugin.code` (失敗の Connect のコード) |
| `my.plugin.slot_wait` | ヒストグラム、秒 | `my.plugin.kind` (同時に動くその種類の呼び出しの中で、呼び出しが順番を待った時間) |
| `my.hook.decisions` | カウンター | `hook`、`mode`、`outcome` ([判定フック](decision-hooks.md)が記録した判定) |
| `my.hook.failures` | カウンター | `hook`、`outcome` (`timeout`、`error`、`invalid`、`breaker_open`) |
| `my.hook.duration` | ヒストグラム | `hook` (プラグインの呼び出し 1 回あたりの秒数) |
| `my.job.duration` | ヒストグラム、秒 | `my.job.type` (登録されたジョブの種類、または `other`)、`my.outcome` (`success` / `retry` / `dead` / `postponed`: `postponed` は、ホストを待つリンクカードの取得のように、何も処理せずジョブの試行回数も使わない実行です) |
| `my.job.wait` | ヒストグラム、秒 | `my.job.type` |
| `my.link_card.host_pauses` | カウンター | `http.response.status_code` (`429` / `503`)。あるホストからのリンクカードの取得を一時停止するたびに 1。[リンクのプレビュー](link-previews.md#limits-per-host)を参照 |
| `my.projection.drifts` | カウンター | `my.projection` (プロジェクションの名前)。定期の確認でプロジェクションの差分を見つけるたびに 1。[プロジェクションの確認と作り直し](database.md#checking-and-rebuilding-projections)を参照 |
| `my.translation.requests` | カウンター | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`)。閲覧者が見られる投稿の翻訳、またはモデレーターが求めた通報の投稿やコメントの翻訳の要求 1 件ごとに 1。投稿者自身の言葉、保存した翻訳、または事業者から応えたか、あるいは投稿を受け取ってよい事業者がない、月ごとの上限に達した、その言語を翻訳する事業者がない、使える翻訳を返した事業者がない、のいずれかで拒否したかを表します。 |
| `my.translation.lock_waits` | カウンター | なし。同じ投稿を同じ言語に翻訳している別のリクエストを、事業者に問い合わせる代わりに待ったリクエスト 1 件ごとに 1 |
| `my.translation.characters` | カウンター | `my.provider` (翻訳の事業者の ID)。事業者に送った投稿と通報のコメントの文字数で、マーカーを除き、失敗した呼び出しも含みます。[月ごとの上限](instance-settings.md#admin-screens-and-the-client-api)が数えるのはこの値で、事業者の請求とは異なることがあります |
| `my.timeline.delivery.lag` | ヒストグラム、秒 | なし (ローカルの投稿が作られてから、ホームタイムラインに書き込まれ、ストリームに通知されるまで) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`、ローカルタイムラインと連合タイムラインのストリームは `timeline`、通話のルームのストリームは `voice`) |
| `my.stream.signals` | カウンター | `my.stream.kind` (`user` / `timeline`)。開いているストリームが処理したシグナル 1 件ごとに 1。クライアントに何かを送ったかどうかは問いません |
| `my.accounts.registered` | カウンター | なし |
| `my.statuses.created` | カウンター | `my.visibility` |
| `my.billing.imports` | カウンター | `my.billing.source` (`aws` / `gcp` / `plugin`)、`my.outcome` (`success` / `failed`)、`error.type` (`failed` のときだけ。失敗の種類で、[クラウドの利用料金の取り込み](billing-import.md)のとおり)。再試行や、翌日の取り込みを待つ失敗も含め、すべての試行を数えます |
| `my.supporter.tip_checkouts` | カウンター | `my.outcome` (`started` / `rate_limited` / `paid` / `expired`)。プロバイダーのページを得た投げ銭の開始ごと、上限によって拒否された開始ごと、支払われたか支払われずに終わった投げ銭ごとに 1。対応するアラートは[投げ銭](supporters.md#tips)を参照 |
| `my.voice.turn.credentials` | カウンター | `my.voice.turn` (`coturn` / `cloudflare`)、`my.outcome` (`issued` / `reused` / `failed`)。通話の参加者がルームに参加するときや接続を更新するときに求めた TURN の認証情報 1 件ごとに 1。`reused` は同じ接続に以前発行した Cloudflare の認証情報をもう一度返したこと、`failed` は Cloudflare に届かないか拒否されたため、参加者がエラーを受け取って接続できなかったことを表します |
| `my.errors.exported` | カウンター | `my.outcome` (`success` / `dropped` / `failed`: `errors_exporter` のサービスへのエラーの報告 1 件ごとに 1。`dropped` は、すでに 1,000 件の報告が待っていたか、サービスがサーバーに送る速度を落とすよう求めたために、報告を送らなかったことを表します。サーバーは、もう一度送るまでサービスが求めるだけ待ちます。`failed` は、サービスに届かないか、サービスが報告を拒否したことを表します。報告は一度だけ送り、再試行はしません。停止の開始から 2 秒たってもまだ待っている報告は失われます) |
| `my.queue.ready` | ゲージ | `my.job.type` (データベースのキューにある、実行できるジョブの数) |
| `my.queue.oldest_ready_age` | ゲージ、秒 | `my.job.type` (データベースのキューで、種類ごとに最も古い実行できるジョブが待っている時間。実行できるジョブのない種類には値がありません) |
| `my.queue.dead` | ゲージ | なし |
| `my.instance.users`、`my.instance.statuses` | ゲージ | なし (NodeInfo と同じ、ローカルのユーザー数と投稿数) |
| `my.media.stored_bytes` | ゲージ、バイト | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | ゲージ、バイト | なし (データベースの大きさ。PostgreSQL は現在のスキーマのリレーション、SQLite は先行書き込みログを除いたデータベースのファイル) |
| `my.billing.import.failing` | ゲージ | `my.billing.source` (その取り込み元の最新の取り込みが失敗していれば 1、そうでなければ 0) |
| `my.federation.instances` | ゲージ | `my.federation.instance.state` (`available`: まだ配送しているが、最後に応答してから少なくとも 1 日は失敗したサーバー。`unavailable`: 配送を止めているサーバー) |
| `my.metrics.snapshot.age` | ゲージ、秒 | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | カウンター | `my.error.source` (`server` / `web`)、`my.error.kind` (`panic` / `error`)。[エラーの報告](#error-reports) 1 件ごとに 1 |
| `my.tls.certificate.expiry` | ゲージ、秒 | `host` (`tls = acme` または `tls = files` で、このプロセスがそのホストのために配信している証明書の期限までの時間。各プロセスが自分の値を報告するので、古い証明書のまま残ったプロセスが分かります。[コマンドとメトリクス](#commands-and-metrics)を参照) |
| `my.tls.order.attempts` | カウンター | `host`、`result` (`issued` / `failed`)。ACME 認証局への証明書の注文 1 件ごとに、注文したプロセスが 1 を数えます。`my tls renew` と `my tls revoke` も含みます |
| `my.tls.order.consecutive_failures` | ゲージ | `host` (そのホストの現在の証明書より後の注文のうち、証明書を得られなかったもの。次の注文までの待ち時間を決める数) |
| `my.tls.rounds` | カウンター | なし。`tls = acme` で、証明書マネージャーがホストを一巡するたびに 1。数えるのが止まったら、マネージャーが止まっています |

ヘルスチェック (`/healthz`、`/readyz`、`/health/live`、`/health/ready`) は計測しません。リモートのドメイン、アカウント、URL、クライアントの IP を含むラベルはありません。
`my.queue.*`、`my.instance.*`、`my.media.stored_bytes`、`my.db.size`、`my.federation.instances`、`my.billing.import.failing` はインスタンス全体の値です。どのプロセスも、キーバリューストアに保存された 1 つのスナップショットから同じ値を報告します。

<!-- source: system-metrics 973069859f11 -->
### システムのメトリクス

プロセス、Go のランタイム、リソースへの接続についてのメトリクスも、同じ `metrics_exporter` で出力します。
コンテナの CPU、メモリ、インスタンス数、受信したリクエストはプラットフォームのメトリクスに任せます。これらのメトリクスは、プラットフォームからは見えないプロセス内部の飽和を扱います。
スケーリングのきっかけには使わず、アラートとインスタンス数の上限の見積もりに使ってください。

| メトリクス | 種類 | ラベル |
| --- | --- | --- |
| `go.memory.used`、`go.memory.limit`、`go.memory.gc.goal` | ゲージ、バイト | `go.memory.type` (`stack` / `other`) は `go.memory.used` だけにあります。`go.memory.limit` は `GOMEMLIMIT` が設定されているときだけ出ます |
| `go.memory.allocated`、`go.memory.allocations` | カウンター | なし |
| `go.goroutine.count`、`go.processor.limit`、`go.config.gogc` | ゲージ | なし |
| `go.schedule.duration`、`go.memory.gc.pause.duration` | ヒストグラム、秒 | なし (実行可能になってから実行されるまでの待ち時間と、GC の停止時間) |
| `go.memory.gc.cycles` | カウンター | なし |
| `process.cpu.time` | Observable Counter、秒 (起動からの合計) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | ゲージ、バイト | なし (常駐メモリ、RSS) |
| `process.unix.file_descriptor.count`、`my.process.file_descriptor.limit` | ゲージ | なし (開いているファイル記述子とその上限) |
| `process.uptime` | ゲージ、秒 | なし |
| `db.client.connection.count` | ゲージ | `db.client.connection.pool.name` (`postgres` / `sqlite`)、`db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | ゲージ | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`、`my.db.client.connection.waits`、`db.client.connection.timeouts` | Observable Counter (起動からの合計。秒または回数) | `db.client.connection.pool.name`。`timeouts` は PostgreSQL だけです |
| `my.adapter.operation.duration` | ヒストグラム、秒 | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`)、`my.operation`、`my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (`valkey` の接続と、`stream.<stream>` の購読者) |
| `my.adapter.reconnects` | カウンター | `my.adapter` |
| `http.client.request.duration` | ヒストグラム、秒 | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots` / `domain_list`)、`http.request.method`、`my.http.status_class` (`2xx` など)、`error.type` (`forbidden_destination` は SSRF ガードによる拒否) |
| `http.client.open_connections` | UpDownCounter | なし |
| `my.readiness.failures` | カウンター | なし |
| `my.readiness.shared` | カウンター | なし。すでに実行中のチェックの結果を、新しく始める代わりに受け取った readiness のリクエスト 1 件ごとに 1 |
| `my.http.origin_refused` | カウンター | なし。CDN を通らずに来たために拒否したリクエスト 1 件ごとに 1 ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn))。オリジンのアドレスへの探査ではゆっくり増えます。トラフィックが止まるのと同時に跳ね上がったら、CDN がヘッダーを送らなくなっています。メンテナンスモードは何も計測しないので、そのときはアクセスログにしか表れません |

- Go のランタイムのメトリクス (`go.*`) を出力するのは `prometheus` と `otlp` だけです。`emf` では、定期的なスナップショットのジョブが、プロセスと接続プールのゲージと Observable Counter を書きます。
- プールの待ちと CPU 時間は Observable Counter です。プールとカーネルが起動からの合計を持ち、収集のときに読み取ります。Prometheus では `_total` のカウンター (`process_cpu_time_seconds_total` など)、OTLP では単調な合計になり、増え方は PromQL の `rate()` で見ます。
  `emf` には合計のカウンターがないので、前回書いてからの増分を書きます (CloudWatch の Sum 統計で足し合わせてください)。
- キューの `claim` は、ジョブを取ったときか失敗したときだけ計測します。待っているワーカーのポーリングは何も記録しません。
- `db.client.connection.pending_requests` は出力しません。pgx と `database/sql` の統計は待っている数を報告しないので、待ちの回数と時間で代わりにします。
- `go.memory.used` の合計と `go.memory.limit` の比、そして `db.client.connection.wait_time` の増加 (プールの上限に達している) にアラートを設定してください。

<!-- source: memory-limit-gomemlimit 66320a6c7b76 -->
### メモリの上限 (`GOMEMLIMIT`)

`GOMEMLIMIT` は運営者が設定する Go のランタイムの環境変数で、アプリケーションが決めるものではありません。
設定しなければ Go は既定値 (上限なし) で動き、コンテナのメモリの上限に近づいてもヒープを積極的には回収しません。
コンテナにメモリの上限があるときは、その上限の 90% ほどに設定してください (たとえば 512 MiB のコンテナなら `GOMEMLIMIT=460MiB`)。OpenTofu のモジュールは、サービスごとにそのサイズから設定します。
`GOMAXPROCS` は設定しないでください。Go の既定値がコンテナの CPU の上限に従います。

<!-- source: traces 15eb90e8584a -->
## トレース

設定 `traces_exporter` (既定値 `none`) で、分散トレースを出力します。関連する設定は[設定リファレンス](../reference/configuration.md)にあります。

| スパン | 名前 | 属性 |
| --- | --- | --- |
| HTTP リクエスト (サーバー) | `<method> <http.route>` (ルートに届かなかったリクエストは `<method>`) | `http.request.method`、`http.route`、`http.response.status_code`、`my.surface`。5xx は失敗です |
| ジョブの実行 (コンシューマー) | ジョブの種類 (`my.job.type`) | `my.job.type`、`my.job.attempt`、`my.outcome`。再試行はそれぞれ別のスパンです |
| トランザクション | `my.db.transaction` | `db.system.name`。再試行はそれぞれ別のスパンです |
| クエリ (クライアント) | sqlc のクエリ名 (`InsertJobs` など。それ以外は `BEGIN` など SQL の最初の単語) | `db.system.name`、`db.operation.name` |
| 配送 (`my.delivery`) | `my.delivery` | `activity.type`、`my.outcome` |
| 外向きの HTTP (クライアント) | `<method>` | `http.request.method`、`server.address` (送り先のホスト名だけ)、`http.response.status_code`。4xx と 5xx は失敗です |

失敗したスパンには `error.type` (`timeout` のような有限の名前、またはステータスコード) があります。属性には、アカウント・投稿・ジョブの ID、URL、パスやクエリ、クライアントの IP、User-Agent の値、SQL の文、エラーの文字列は決して含まれません。
`server.address` によってリモートのサーバーのホスト名が受信側に残るので、受信側がそれをどれだけの期間保存し、誰が見られるかはあなたが決めます。ヘルスチェックと、`origin_secret` で拒否したリクエストはスパンを作りません。

- ジョブの実行は、それをキューに入れたスパン (ふつうは HTTP リクエスト) の子ではありません。キューに入れたスパンへのリンクを持つ、**新しいトレースのルート**です。ビューアーでは、リクエストのトレースからリンクをたどってください。
- 受け取った `traceparent` が親になるのは、接続してきた相手が `trusted_proxies` に含まれ、さらに [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) を使うならリクエストがそのシークレットを持っているときだけです。そうでなければ捨てて、新しいトレースを始めます。`tracestate` は常に捨てます。外向きの HTTP (連合、WebFinger、メディア、Web Push など) には `traceparent` を付けません。
- サンプリング: リクエストは、信頼するプロキシからの親が記録されていたか、ジョブのリンク先が記録されていたなら記録します。そうでなければ `traces_sample_ratio` (既定値 0.01) の割合で記録します。割合を下げても、記録したリクエストの続きのジョブがトレースを失うことはありません。

| 対象 | 設定 | 出力先 |
| --- | --- | --- |
| 既定、小規模なセルフホスト | `traces_exporter = none` | スパンは出力しません。ログの `request_id` と `job_id` をたどってください。信頼するプロキシが `traceparent` を渡すなら、その `trace_id` と `span_id` がログに加わります |
| セルフホスト (Jaeger、Tempo、OpenTelemetry Collector など) | `traces_exporter = otlp`、`traces_flush = batch` | `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (または `OTEL_EXPORTER_OTLP_ENDPOINT`、既定値 `http://localhost:4318`) の受信側に OTLP/HTTP で送ります。認証のヘッダーは `OTEL_EXPORTER_OTLP_HEADERS` に書きます。エラーや遅いトレースを確実に残すには、Collector のテールサンプリングを使ってください |
| Google Cloud | `traces_exporter = none` | Cloud Trace が、Cloud Run がリクエストごとに記録するトレースを表示します: [Google Cloud へのデプロイ](deploy-google-cloud.md#traces) |
| AWS | `traces_exporter = otlp`。すべてのタスクにある Collector へ | X-Ray: [X-Ray へのトレース](deploy-aws.md#traces-to-x-ray) |

<!-- source: profiles d19dd2bf22e2 -->
## プロファイル

[システムのメトリクス](#system-metrics)は、プロセスがどれだけのメモリと CPU を使っているかを示します。Go のランタイムのプロファイルは、何がそれを使っているかを示します。
`pprof_listen` は、`/debug/pprof/` の下でプロファイルを提供するリスナーのアドレスで、どの `role` でも使えます。既定では空で、リスナーは開きません。

```sh
MY_PPROF_LISTEN=127.0.0.1:6060 my serve

go tool pprof -sample_index=inuse_space http://127.0.0.1:6060/debug/pprof/heap   # what holds the heap now
go tool pprof -sample_index=alloc_space http://127.0.0.1:6060/debug/pprof/heap   # what has allocated since the start
go tool pprof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'             # where the CPU goes, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=1'                       # every goroutine, by where it waits
go tool pprof 'http://127.0.0.1:6060/debug/pprof/mutex?seconds=30'               # where goroutines wait on locks, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/heap?debug=1' | grep '^# '               # the runtime's memory statistics
```

リスナーには認証がなく、プロファイルはプロセスの内部を見せ、取得には CPU を使います。ループバックに置いてトンネル (`ssh -L 6060:127.0.0.1:6060 <host>`、`kubectl port-forward`) 経由で接続し、使い終わったら設定を外してください。
ヒーププロファイルが扱うのは Go のヒープだけです。プロセスの常駐メモリには、読み込まれたバイナリのページ、スタック、ランタイム自身の構造も含まれます。それらはメモリの統計で分かります。
ブロックとミューテックスのプロファイルは、`seconds` を付けたリクエストが取得しているあいだだけサンプリングし、付けなければ空です。

<!-- source: stop-the-server 37d4c0cc3d97 -->
## サーバーを停止する

SIGTERM か SIGINT を受け取ると、サーバーは穏やかに停止します。

1. 新しい接続の受け付けとジョブの取得をやめます。
2. 処理中のリクエストと実行中のジョブが終わるのを最大 25 秒待ちます。
3. 25 秒たつと、残りのリクエストとジョブのコンテキストを取り消し、接続を閉じます。
   そのあと、取り消したジョブに結果を記録する時間をさらに最大 3 秒与えてから、終了コード 1 で終わります (合計で最大 28 秒。Kubernetes と ECS の既定の猶予期間 30 秒に収まります)。
   取り消したジョブは再試行されます。その 3 秒のうちに止まらないジョブや、途中で強制終了されたジョブは放棄され、リースが切れるとワーカーがもう一度取り上げるので、2 回実行されることがあります。

サーバーのバックグラウンドのタスクの 1 つでパニックが起きても同じ停止が始まり、サーバーが壊れた状態で動き続けることはありません。
ログの `serve failed` の行がそのタスクを示し、プラットフォームやプロセスマネージャーが再起動できるように、プロセスは終了コード 1 で終わります。

データベース、キーバリューストア、キューの作業をするときは、停止する代わりにメンテナンスモード (`MY_MAINTENANCE=true`) で再起動してください。[メンテナンス](maintenance.md)を参照してください。

<!-- source: the-job-queue-and-failed-dead-jobs 1fd5aaccbff5 -->
## ジョブキューと失敗した (dead) ジョブ

[ジョブ](../reference/jobs.md)に、すべてのジョブの種類について、何をするか、何がきっかけで始まるか、どう再試行するか、もう一度実行すると何が起きるかが載っています。

管理画面のジョブ (管理 → ジョブ、`/admin/jobs`、権限 `manage_jobs`) には 3 つのページがあります。

- **キューの状態** (`/admin/jobs`): まず失敗したジョブの数とそこへのリンク。次に状態 (待機中、実行中、リトライ待ち、失敗) ごとの数で、それぞれその状態のジョブへつながります。続いて、期限が来た最も古いジョブが待っている時間と、種類と状態ごとの数、種類ごとにジョブを待たせている時間の表です。
- **ジョブの一覧** (`/admin/jobs/list`): 終わっていないジョブ (待機中、実行中、リトライ待ち) か、1 つの状態、1 つの種類のジョブを 1 ページずつ表示します。種類、試行回数、予定または失敗した時刻、状態、最後のエラーの最初の行を表示します。
- **ジョブ** (`/admin/jobs/<id>`、一覧の種類から開きます): 失敗したジョブの **「再実行」** (実行すると、ジョブが失敗する前にしたことを繰り返すことがあるので、先に確認します)、ジョブのすべての情報 (ID とキーはそれぞれコピーボタン付き)、最後のエラーとペイロード (それぞれコピーボタン付き)、成功しなかった試行の表 (新しいものから。結果、終了時刻、ワーカー、エラー)、そして同じ種類のジョブへのリンクです。

管理 API は `GET /api/v1/admin/queue`、`GET /api/v1/admin/jobs?state=&type=`、`GET /api/v1/admin/jobs/{id}`、`GET /api/v1/admin/jobs/{id}/attempts`、`POST /api/v1/admin/jobs/{id}/retry` です。
完了したジョブはもうキューにないので、そのページは見つかりません。

- 待機中、実行中、リトライ待ちのジョブを一覧できるのは、サーバーがジョブを保持するキューだけです。`queue = db`、`queue = cloudtasks` (ジョブはデータベースにも入っています)、`queue = inline` です。失敗したジョブと試行の履歴は、どのキューでもアプリケーションのデータベースにあるので、常に一覧できます。
- 試行の履歴には、成功しなかった試行 (再試行と、最終的に失敗した試行) だけが残ります。成功した実行は、構造化された `job` のログ行 (`job_id` を持ちます) でたどってください。
  履歴は、試行が終わってから `job_attempt_retention` (既定値 30 日。[設定](../reference/configuration.md#job_attempt_retention)を参照) がたつと、1 時間ごとの掃除で削除されます。
  履歴にはペイロードもハンドラーのログも含まれず、エラーメッセージは最初の 2,000 バイトに切り詰められます。ジョブのハンドラーは、メールアドレス、IP アドレス、トークン、投稿の本文をエラーメッセージに含めません。
  メールのジョブの種類に絞った見方は、[メールの送信](email.md)を参照してください。

ハンドラーが最終的な失敗と判断したジョブと、再試行を使い果たしたジョブは、データベースのキュー (`queue = db` と `queue = cloudtasks`) で dead ジョブになります。
`my job` で一覧、再試行、削除ができます。ジョブのページの「失敗」の状態と管理 API でも、一覧と再試行ができます。

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- 同じ冪等キーのジョブがすでに待っているなら、そのジョブが再試行するジョブの代わりになります。ジョブのハンドラーは冪等なので、同じジョブを何度戻しても安全です。
- 不明な ID は表示され、コマンドは終了コード 1 で終わります。ほかの ID はそのまま処理されます。終了コード 2 は使い方の誤りを表します。
- 誰も再試行しない dead ジョブは、失敗してから `dead_job_retention` (既定値 30 日。[設定](../reference/configuration.md#dead_job_retention)を参照) がたつと、1 時間ごとの掃除で削除されます。
- `queue = inline` の dead ジョブはサーバーのプロセスのメモリにあるので、`my job` では扱えません (終了コード 1)。

<!-- source: splitting-workers-by-job-type 89ced36a3c80 -->
### ジョブの種類ごとにワーカーを分ける

既定では、どのワーカーもすべてのジョブの種類を、期限が来た順に、プロセスごとに 1 件ずつ実行します ([複数のジョブを同時に実行する](#running-several-jobs-at-once)を参照)。そのため配送が集中するとメールや通知が遅れ、ワーカーを足してもその時間はすべての種類で分け合うことになります。`queue = db` なら、一部の種類に専用のワーカーを割り当てられます。

- [`worker_job_types`](../reference/configuration.md#worker_job_types): このプロセスはこれらの種類だけを実行します。
- [`worker_job_types_except`](../reference/configuration.md#worker_job_types_except): このプロセスはこれら以外のすべての種類を実行します。

どちらも、名前 (`federation.deliver`) か接頭辞 (`federation.*`) で書いた[ジョブの種類](../reference/jobs.md)のカンマ区切りの一覧です (`my job types` で表示でき、どれにも一致しない名前は `my config check` が受け付けません)。`MY_WORKER_JOB_TYPES` などで、プロセスごとに設定します。1 つのプロセスに両方は設定できません。どのジョブの種類にも一致しない名前や接頭辞は起動を止めるので、リリースで名前が変わった種類や、設定していない設定があるときだけ存在する種類にも気づけます。`role = web` や、`queue = cloudtasks` または `inline` では使えず、やはり起動に失敗します。そこではシグナルが実行するジョブを指定するので、プロセスがジョブを選べないからです。

起動時に、どちらかの設定を持つプロセスは、飛ばす種類をログに出します。プロセスはほかのプロセスの設定を見られないので、**どの種類もいずれかのプロセスが実行するようにしてください**。そうしないと、そのジョブはいつまでも待ち続けます。定期的な掃除とメールもジョブの種類です。定期的なジョブはどのプロセスも起動しますが、どのプロセスがそれを実行するかはこれらの設定で決まります。

何を分けるかは、種類の名前ではなくメトリクスから決めてください。`my.job.wait` (Prometheus では `my_job_wait_seconds`) は、種類ごとにジョブが始まるまで待った時間を示します。

```promql
histogram_quantile(0.95, sum by (my_job_type, le) (rate(my_job_wait_seconds_bucket[15m])))
```

`sum by (my_job_type) (rate(my_job_duration_seconds_sum[1h]))` は、種類ごとに使っているワーカーの時間を示し、これはおおよそその種類に必要なプロセスの数でもあります。時間の大半を使ってほかの種類を待たせている種類が、移すべき種類です。

よくある 3 つの構成を挙げます。プロセスの数は自由に決めてください。

| 構成 | プロセス |
| --- | --- |
| 配送がほかを遅らせないようにする | 配送用に `worker_job_types = federation.fanout,federation.deliver`、それ以外のすべて用に `worker_job_types_except = federation.fanout,federation.deliver`。配送が集中している間は、最初のグループにプロセスを足します。 |
| 検索のバックエンドがほかを遅らせないようにする | 検索のジョブ用に `worker_job_types = search.*`、それ以外のすべて用に `worker_job_types_except = search.*` ([検索がほかのジョブを遅らせないようにする](search.md#keeping-search-from-delaying-other-jobs))。 |
| メールと通知を速く届ける | これらの種類用に `worker_job_types = identity.verification_email,identity.password_reset_email,identity.password_notice_email,identity.closure_email,identity.deactivation_email,notification.email,notification.moderator_email,notification.supporter_email,notification.push,notification.post_subscribers,instance.notify_announcement,voice.notify_started`、そしてすべて用に設定のないワーカー。このワーカーは手が空いているときにこれらの種類も手伝います。設定していない設定の種類は外してください。そうしないと起動が止まります。`notification.push` には `vapid_private_key` が、`notification.email`、`notification.moderator_email`、`notification.supporter_email` には `mail` が必要です。[設定によって決まるジョブ](../reference/jobs.md#jobs-that-depend-on-settings)を参照してください。 |

`identity.*` のような接頭辞は、その種類を持つコードのモジュールであって、仕事の種類ではありません。`identity.*` はデータのエクスポート、インポート、期限切れのジョブも含み、`instance.notify_announcement` と `voice.notify_started` は含まないので、1 つの仕事の種類のための構成では種類を 1 つずつ書きます。

次の種類は、同じ設定のワーカーにまとめてください。1 つの順序キーのジョブは種類に関係なく順に実行されるので、前のジョブを誰も実行しなければあとのジョブも待ち、前のジョブのグループが詰まっていればあとのジョブもそれを待ちます。

| 種類 | 順序を保つもの |
| --- | --- |
| `federation.fanout`、`federation.deliver` | いいねとその取り消しの配送 |
| `voice.apply_media_event`、`voice.apply` | 1 つの通話のルームのイベント |
| `supporters.sync`、`supporters.command` | 1 つのサブスクリプションの変更 |

アップグレードするとき:

- `worker_job_types` のプロセスは、新しいバージョンで加わった種類を実行しません。リリースノートと、ログにある飛ばした種類を確認してください。`worker_job_types_except` のプロセスはそれらを実行するので、「それ以外のすべて」を受け持たせるにはこちらのほうが安全です。
- 名前の変わった種類は起動を止めるので、先にリリースノートを読んでください。

分割を展開するには、まず新しいワーカーを起動し、そのあと古いワーカーに設定を加えます。元に戻すには、片方のグループを止める前に、もう片方のグループから設定を外します。ジョブはキューに残るので何も失われず、しばらく誰も実行しない種類は待つだけです。

**どのプロセスも実行しない種類を見つける。** そのような種類のジョブは待ち続けるので、その種類の `my.queue.oldest_ready_age` が増え、最も長く待っている種類についてアラート `MyQueueBacklog` と `MyQueueStalled` が発火します。どの種類かを知る、または種類ごとにアラートを出すには:

```promql
max by (my_job_type) (my_queue_oldest_ready_age_seconds) > 1800
```

管理画面のジョブのページにも、種類ごとに同じ待ち時間が表示されます。どちらかの設定を持つワーカーは、ハンドラーがまったくない種類のジョブ (メトリクスでは `other` として数えます) にも手を付けないので、設定のないワーカーがなければ、そのようなジョブは dead ジョブにならずに待ち続けます。

<!-- source: running-several-jobs-at-once 2a1137a12e8f -->
### 複数のジョブを同時に実行する

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) は、1 つのプロセスが同時に実行するジョブの数を 1 から 128 の間で決めます。`queue = db` で使えます。Cloud Tasks ではプラットフォームが決めます。既定は 1 件ずつで、1 を設定しても同じ動きです。

`federation.deliver` のように、ほかのサーバーを待つ時間が長いジョブのために上げてください。そうしたジョブは 1 件あたりのメモリもデータベースの接続も少ないので、1 つのプロセスで多くのプロセス分の仕事ができます。応答を 200 ms 待つだけのジョブでのローカルのテストでは、スロット 32 の 1 プロセスが、スロット 1 の 32 プロセスと同じだけ処理し、メモリはそのごく一部しか使いませんでした。そのジョブはほかに何もしなかったので、自分のインスタンスで測ってください。`worker_job_types` と組み合わせ、ジョブの種類ごとに、それぞれの数を持つプロセスを割り当ててください。

次の制約を覚えておいてください。

- **データベースの接続**: ジョブは、取得されるとき、自分の処理、結果の記録に接続を必要とします。この設定はプールの最大値より小さくなければならず、そうでないとサーバーは起動しません。最大値は `database_url` の `pool_max_conns` で、既定では 4 と CPU 数の大きいほうなので、vCPU が 1 つなら、上げるまでは 3 までです。データベースや接続プーラーが、すべてのプロセスの接続を受け付けるか確認してください。
- **SQLite**: トランザクションを 1 つずつ書き込みます。ローカルのテストでは、SQLite は 32 より 128 のほうが遅くなりました。
- **CPU を多く使うジョブ**: スロットは 1 つのプロセスの CPU とメモリを分け合います。`media.process` は既定で CPU 数の半分を上限として順番を待ち、その待ち時間はジョブの 5 分のリースに含まれるので、`media.process` でいっぱいのプロセスではリースを超えてやり直しになることがあります。そうしたジョブには、スロットの少ないプロセスを割り当ててください。
- **1 度に 1 つのサーバー**: プロセスはどの 1 つのホストにも最大 8 本の接続しか張らないので、スロットがいくつあっても、1 つのプロセスが 1 つのサーバーに配送する速さは 8 本の接続の分を超えません。配送の大半が少数の大きなサーバーに向かうなら、スロットを増やしても効かないところで、プロセスを増やすと効きます。
- **詰まったサーバーはスロットを占める**: 遅いサーバーを待っているスロットはほかのジョブに使えないので、配送はメールや通知と分けてください。
- **デプロイは試行回数を使う**: サーバーが停止するとき、`ShutdownTimeout` (既定で 25 秒) を過ぎても実行中のジョブは取り消され、取り消されたジョブはそれぞれ試行を 1 回使います。遅いジョブを実行するスロットが多いほど、デプロイのたびに使う試行が増えます。結果を時間内に記録できなかった取り消されたジョブは、リースが切れるのを待ってから再び実行されます。
- **クラッシュはすべてのスロットを巻き込む**: プロセスのメモリが尽きると、そのすべてのジョブがリースが切れるのを待ちます。

上げるかどうかは、`my.job.type` ごとの `my.job.wait` を見て決めます。ある種類のジョブが長く待ち、`my.job.duration` の大半がほかのサーバーを待つ時間で、CPU とメモリが低いままなら、スロットを増やすと効くはずです。上げても待ちが高いままなら、もう一度上げる前に、上のデータベースの接続とホストの上限を確認してください。

期限が来たジョブがない間は、1 つのスロットがポーリングしてほかは待つので、何もしていないプロセスがデータベースにかけるコストは、1 件ずつ実行するプロセスと同じです。

<!-- source: waiting-for-a-database-connection 795c0fd7f769 -->
### データベースの接続を待つ

`database = postgres` では、どの文もトランザクションも、まずプロセスのプールの空いた接続を待ちます。
[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (既定値 `60s`) はその待ち時間だけを制限し、ほかには何も制限しません。すでに接続を得た長いクエリや
トランザクションは、どれだけ時間がかかっても実行を続けます。`0` は上限なしで待ちます。SQLite には待つべき接続の上限がないので、そこで設定すると起動時に失敗します。

待ち時間が尽きると:

- リクエストには `Retry-After: 5` 付きの `503 Service Unavailable` で応えます。
- ジョブは 30 秒後にもう一度実行され、試行回数を使いません。そのため、忙しいデータベースがジョブを dead ジョブにすることはありません。
- メトリクス `db.client.connection.timeouts` がそれを数え、アラート [`MyDBPoolWaiting`](alerts.md#mydbpoolwaiting) が発火します。

既定値は、サーバーの書き込みタイムアウトの 60 秒です。それより長く待ったリクエストはどのみち応答を送れないので、既定値では、まだ成功できたはずのリクエストを失敗させることはありません。
値はインスタンスの動き方から選んでください。

- **下げる**と、負荷をすばやく逃がせます。クライアントはリクエストを開いたままにせず、`Retry-After` のあとに再試行します。いちばん遅い通常のトランザクションより長くしておかないと、短い集中で `503` を返し始めます。
- **先にプールの大きさを決めてください。** プロセスには、同時に実行する処理の数とほぼ同じ数の接続が必要です。`database_url` の `pool_max_conns` の既定値は 4 と CPU 数の大きいほうで、すべてのプロセスを合わせて、データベースの接続の上限を下回らなければなりません。
  - [`worker_job_concurrency`](#running-several-jobs-at-once) を使うと、スロットも接続を待ちます。この設定はプールの最大値より小さくしておく必要があります。
  - Google Cloud では、1 つのインスタンスが多くのリクエストを同時に受けます。[外部の PostgreSQL](deploy-google-cloud.md#an-external-postgresql)がプールの大きさを決めます。
- **`503` や `db.client.connection.timeouts` が頻繁に出る**なら、プールが小さすぎるか、クエリが遅くなっています。タイムアウトを長くしても、それを隠すだけです。[`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated) に従ってください。

<!-- source: cloud-tasks f14d9721ba24 -->
### Cloud Tasks

[Google Cloud へのデプロイ](deploy-google-cloud.md)では、以下のすべてを OpenTofu のモジュールで準備します。

`queue = cloudtasks` では、ジョブはデータベース (`database = postgres`) に残り、Google Cloud Tasks はジョブごとにサーバーを起こすだけです。

- サーバーは、保存、再試行、または戻すジョブのそれぞれについて、トランザクションがコミットされたあとで `queue_cloudtasks_queue` にタスクを作ります。タスクは、
  ジョブの期限の時刻に、ジョブの ID を本文とし、`queue_cloudtasks_service_account` の OIDC の ID トークンを付けて `POST {queue_cloudtasks_url}/_internal/jobs` を呼びます。
  30 日より先に期限が来るジョブは、30 日の時点で通知され、そのタスクが届いたときにもう一度通知されます。
- タスクを受け取ったプロセスはジョブを実行し、結果を記録したら `204` で応えます。ジョブがすでに実行済みか、期限が来ていないときも同じです。
  失敗したジョブは、Cloud Tasks ではなくサーバーが、次の試行の時刻に再試行します。Cloud Tasks がタスクを再試行するのは、応答が
  `5xx` (データベースに届かなかった) か `401` (サーバーが拒否したトークン) のときだけなので、その `retryConfig` は調整する必要がありません。
- テーブルをポーリングするプロセスも、定期的なジョブを自分で実行するプロセスもありません。Cloud Scheduler のジョブが、同じサービスアカウントの OIDC トークン (設定されていれば、
  `queue_cloudtasks_audience` のオーディエンス付き) で、既定では毎分 (`* * * * *`)、`POST {queue_cloudtasks_url}/_internal/tick` を呼びます。tick のたびに、周期が来た定期的なジョブを、インスタンス全体で 1 回だけキューに入れます (周期は KV で確保するので、
  `kv` は `db` か `valkey` でなければなりません)。また、期限から 2 分を超えても実行されていないジョブをもう一度通知し、失われた通知を回復します。
- `/_internal/jobs` と `/_internal/tick` は `role = all` か `worker` のプロセスが提供し、`queue_cloudtasks_service_account` のために Google が署名し、
  確認済みのメールアドレスを持ち、唯一のオーディエンスが `queue_cloudtasks_audience` (既定では呼ばれた URL) であるトークンだけを受け付けます。それ以外のリクエストは `401` です。
- 1 回の配信で 1 つのジョブを、最大で 5 分のリースの間実行します。`queue_cloudtasks_dispatch_deadline` (既定値 10 分) と、Cloud Run のサービスのリクエストのタイムアウトは、
  それより長くしておいてください。
- サーバー自身のサービスアカウントには、キューに対する `roles/cloudtasks.enqueuer` と、`queue_cloudtasks_service_account` に対する `roles/iam.serviceAccountUser` が必要です。
  認証にはアプリケーションのデフォルト認証情報 (Application Default Credentials) を使います。

<!-- source: how-late-periodic-jobs-run 775fb52d1df8 -->
### 定期的なジョブが遅れる時間

定期的なジョブは、トリガーの周期 1 回分までと、プラットフォームが起動するのにかかる時間だけ、予定の時刻より遅れて実行されます。イベントのリマインダーと開始の通知がどれだけ遅れて届くか、そしてイベントの通話のルームがいつ開くかは、これで決まります。`gathering.queue_due` がそれらを開始するからです。リマインダーは、表にある間隔の分まで遅れます。値はスケジューラーのコードと各プラットフォームのトリガーから導いたもので、動いているサーバーで測ったものではありません。

| プラットフォーム | 定期的なジョブを起動するもの | イベントのリマインダーや開始の想定される遅れ |
| --- | --- | --- |
| セルフホストと [Docker Compose](deploy-docker-compose.md) (`queue = db`) | `role = all` か `worker` の各プロセスにあるスケジューラーが、プロセスの起動時から 1 分ごとに確認し、サーバー全体で周期ごとに 1 回ずつジョブをキューに入れます | 最大 1 分と、ワーカーがジョブを取るまでの約 1 秒 |
| [AWS](deploy-aws.md) (`queue = db`) | 同じスケジューラーが、すべてのタスクで動きます | 最大 1 分と、約 1 秒 |
| [Google Cloud](deploy-google-cloud.md) (`queue = cloudtasks`) | Cloud Scheduler が `queue.tick_schedule` (既定では毎分) で `/_internal/tick` を呼び、そのあと Cloud Tasks がジョブごとにサーバーを起こします | 最大 1 分と、動いているインスタンスがないときはインスタンスの起動 (コールドスタート。たいてい数秒)。`tick_schedule` を長くすると、その間隔まで ([工夫で減らせる部分](deploy-google-cloud.md#where-care-reduces-it)) |

イベントが始まった後に届くことになるリマインダーは送られず、イベントが終わった後の開始の通知も送られません。
そのため、tick が 5 分より長いと、いちばん短いリマインダーが失われることがあります。

<!-- source: routing-job-types-to-their-own-queues f4098db6215c -->
### ジョブの種類を専用のキューに振り分ける

`queue = cloudtasks` では、ワーカーは実行するジョブを選べません。誰が通知を受け取るかはプラットフォームが決めるので、`worker_job_types` は `queue = db` でしか使えないからです。
一部のジョブの種類に専用の処理能力を持たせるには、その通知を専用のキューに送り、そのキューの処理能力をプラットフォームで設定します。
振り分けを設定しない限り、何も変わりません。

[`queue_cloudtasks_routes`](../reference/configuration.md#queue_cloudtasks_routes): `federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com,notification.email=projects/p/locations/l/queues/my-mail`。
`@` と URL のない振り分けは `queue_cloudtasks_url` を呼びます。

左辺は[ジョブのリファレンス](../reference/jobs.md)にあるジョブの種類か、接頭辞です。名前は接頭辞より、長い接頭辞は短い接頭辞より優先されます。
どの振り分けにも一致しない種類は、あとのリリースで加わるものも含めて既定のキュー (`queue_cloudtasks_queue`) を使うので、実行され続けます。
`my config check` は振り分けの形を確認します。どのジョブの種類にも一致しない左辺は、起動時に `my serve` を止めます。

**`role = web` も含め、すべてのプロセスに同じ振り分けを設定してください。** 通知を送るのはジョブを保存するプロセスです。投稿の最初のジョブなど、利用者の操作の大半では web のプロセスが、
ジョブが保存するジョブ、再試行、順序キーの次のジョブ、失われた通知の掃き出しではワーカーが送ります。
Google Cloud では、`workers` がワーカーごとに専用のキューとサービスを与え、どのサービスにも同じ振り分けと権限を与えます。
[ジョブをワーカーに分ける](deploy-opentofu.md#splitting-jobs-into-workers)を参照してください。

振り分けが異なるプロセスがあっても、エラーにはならず、ジョブも失われません。その通知は別のキューに行き、ジョブはそこで、設定した処理能力の外で実行されます。
そのジョブは待たないので、`my.queue.oldest_ready_age` にはこれが表れません。プロセスを比べるために、各プロセスは起動時に、その `routes` と `routes_hash` を含む行をログに出します。
ハッシュはどこでも同じでなければなりません。通知がどこに行くかを知るには、プラットフォームによる各キューのメトリクスを見てください。

**権限。** どのプロセスの認証情報にも、振り分けのすべてのキューに送る権限 `roles/cloudtasks.enqueuer` が必要です。
送信に失敗するとログに出し、別のキューには試しません。制限した種類があふれてほかに流れ込まないようにするためです。そのジョブは、2 分後に掃き出しが通知を送り直したときに、遅れて始まります。

**処理能力。** どの受信側もどのジョブでも実行するので、どこまで分けるかを選べます。

| プラットフォーム | 1 つの受信側 | 別々の受信側 |
| --- | --- | --- |
| Google Cloud | 1 つのサービス。キューごとにそれぞれの `maxConcurrentDispatches` と `maxDispatchesPerSecond` があり、サービスのインスタンスの上限は共有します | キューごとのサービス。振り分けの URL で指定し、それぞれにインスタンスの上限と同時実行数があります。それぞれ `queue_cloudtasks_url` を自分の URL に設定します |

キューには既定のキューと同じ準備、つまりキューに対するロールが必要です。
tick が必要なのは既定のキューだけです。

**キューが増やすもの。** キューは 1 つごとに、作り、見張り、支払うものが 1 つ増えます。それがどれほど重いかは、インスタンスの大きさによります。
すべてではなく、`my.job.wait` で待っているのを確かめたものを分けてください。

**掃き出し。** 掃き出しは毎分、期限から 2 分を超えても実行されていないジョブを、振り分けごとに最大 500 件、既定のキューで最大 500 件探し、もう一度通知します。
そのため、追いつけないキューには毎分最大 500 件の重複した通知が届きます。それでジョブが 2 回実行されることはありませんが、料金のかかるメッセージであり、滞留が解消すると止まります。

**順序キー**は引き続き守られます。何を実行してよいかはデータベースが決めます。1 つのキーのジョブが別々のキューにあることはありますが、止まったキューはそのキーのあとのジョブも遅らせます。

**キューを加える、移す、取り除く。**

1. キューとその受信側を作り、すべてのプロセスにそのキューへ送る権限を与えます。
2. すべてのプロセスで振り分けを変えます。異なっている間は、通知はどちらかのキューに行き、ジョブは両方の場所で実行されます。
3. キューを取り除くには、まずすべての場所で振り分けからそのキューを外し、空になるのを待ってから削除します。削除したキューにある通知は失われますが、ジョブは失われません。ジョブが期限から 2 分たつと、掃き出しがもう一度通知します。
   Cloud Tasks のタスクは 30 日先までのジョブの通知を保持できるので、待つ期間より後に期限が来るジョブは、その時が来たときに掃き出しが通知するだけです。

<!-- source: health-checks e268d13fe7f2 -->
## ヘルスチェック

| パス | 種類 | 200 を返すとき |
| --- | --- | --- |
| `GET /healthz`、`GET /health/live` | liveness | プロセスが HTTP に応えられるとき。データベースなどの依存先は見ません (そのため、データベースが遅くてもプロセスは再起動されません) |
| `GET /readyz`、`GET /health/ready` | readiness | データベースに届き、スキーマが適用されているとき |
| `GET /api/v1/instance/health` | 死活監視、ステータスページ、クライアント (`web` の役割が提供します) | readiness と同じです。結果はプロセスの中に 60 秒キャッシュされます |

- Cloud Run は `z` で終わるパスのいくつか (`/healthz` を含む) を予約していて、自分で `404` を返します。その背後では、同じ応答を返す `/health/live` と `/health/ready` を呼んでください。
- どれでも 200 の本文は `{"status":"ok"}` です。
- `/readyz` はリクエストのたびにデータベースを確認し、同時に来たリクエストは 1 回の確認を共有します。
- `/api/v1/instance/health` は、正常なら 200 と `{"status":"ok","checked_at":"<RFC 3339>"}` を、失敗なら 503 と `{"status":"unavailable","checked_at":...}` を返します (`Cache-Control: no-store`)。
  どれだけ頻繁に監視されたり要求されたりしても、各プロセスがデータベースに触れるのは最大で 1 分に 1 回で、アクセストークンもレート制限も見ません。死活監視はプローブ (`/readyz` や `/health/ready`) ではなく、このエンドポイントに向けてください。
  サービスごとの設定、監視の間隔と費用、メンテナンスの扱いは、[死活監視のガイド](monitoring.md)を参照してください。
- 3 つとも、アクセスログには Debug レベルで書き、トレースのスパンは作りません。
- readiness を満たさないときは、RFC 9457 の `application/problem+json` の本文を持つ 503 を返します。
  その `title` は `Accept-Language` に従って翻訳され、内部の詳細は含みません。原因 (接続のエラーや、ないテーブルの名前) はログに出ます。
- 1 回の readiness の確認は 3 秒で打ち切ります。
- readiness には、設定した `host` と `account_domain` がデータベースにあるか、プロセスがそれを記録できたことも必要です。
- **スキーマが適用されている**とは、そのデータベースのスキーマのファイル (`schema/<NN>-<module>/<dialect>.sql`) が `CREATE TABLE` で宣言するすべてのテーブルがあることです
  (SQLite は `sqlite_schema` で、PostgreSQL は接続の `current_schema()` で確認します)。`my schema apply` はスキーマ全体を適用するので、テーブルがないなら、スキーマが適用されていないか、バイナリより古いということです。
  列とインデックスの違いは確認しません。`my schema apply -dry-run` で、何が変わるかを表示できます。

<!-- source: requests-that-bypass-the-cdn 2cca4127af07 -->
## CDN を通らないリクエスト

サーバーが CDN の後ろにあり、サーバー自身のアドレスも公開されたままのときは、`origin_secret` と `origin_secret_header` を設定して、CDN を通らずに来たものを拒否させてください。ヘルスチェックには引き続き届きます。[CDN を通らずに来たリクエストを拒否する](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)を参照してください。

<!-- source: security-headers 8c5e82848149 -->
## セキュリティのヘッダー

どのレスポンスにも `Strict-Transport-Security`、`X-Content-Type-Options`、`Referrer-Policy`、`Permissions-Policy`、`Cross-Origin-Opener-Policy`、`Cross-Origin-Resource-Policy: same-site`、`X-Frame-Options` が付き、
ドキュメントでないレスポンスには `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'` も付きます。
ActivityPub の JSON や公開のメディアなど、ほかの値が必要なレスポンスでは、ハンドラーがこれらを上書きします。
`Permissions-Policy` は、カメラ、位置情報、支払い、USB を拒否します。マイクも拒否しますが、通話が有効な間 (`voice` が `off` でないとき) は例外で、
ブラウザが通話で話せるように、サーバー自身のオリジンのページにだけマイクを許可します (`microphone=(self)`)。
Web クライアントの HTML には、nonce を使わないドキュメントの CSP (`script-src 'self'` など) が付きます。
これは Trusted Types を必須にし (`require-trusted-types-for 'script'`)、作れるポリシーをクライアントの 2 つに限ります (`trusted-types my-server-sanitized-html my-service-worker`)。

<!-- source: web-client 0a0b62acc010 -->
## Web クライアント

`role` が `web` か `all` のプロセスは、バイナリに埋め込まれた Web クライアントを提供します。

| パス | 内容 | `Cache-Control` |
| --- | --- | --- |
| ほかのどのルートにも一致しない `GET` (`/`、`/@alice`、`/@alice/status/{id}`、`/settings` など) | SPA の HTML シェル。インスタンスの設定は `<script type="application/json" id="config">` で渡します | `public, max-age=60, stale-while-revalidate=300`、ETag |
| `GET /assets/*` | ハッシュの付いた名前のビルドのファイル | `public, max-age=31536000, immutable`、ETag |
| `GET /manifest.webmanifest` | インスタンスの設定から作った Web アプリのマニフェスト | `public, max-age=60, stale-while-revalidate=300`、ETag |
| `GET /sw.js`、`/icon.svg` など、ビルドの直下にあるファイル | そのまま提供します | `no-cache`、ETag |
| `GET /licenses/*` | ビルドが同梱するもののライセンス (npm のパッケージの表示と、npm 以外の同梱データの `LICENSE-Unicode.txt` と `OFL.txt`)。拡張子に関係なく `text/plain; charset=utf-8` として提供します | `no-cache`、ETag |
| `GET /robots.txt`、`/.well-known/tdmrep.json`、`/.well-known/security.txt` | インスタンスの設定から作ります ([検索エンジンと AI のクローラー](crawlers.md)を参照) | `public, max-age=3600`、ETag |

- `/api`、`/oauth`、`/auth`、`/.well-known`、`/nodeinfo`、`/ap`、`/media`、`/uploads`、`/theme` (サーバーのテーマのスタイルシート)、`/licenses` の下のパスと、`/robots.txt` は、決してシェルを返しません。ほかのどのルートにも一致しなければ、404 の problem を返します。
- シェルは ActivityPub のコンテントネゴシエーションのあとで提供します。シェルのパスを ActivityPub の `Accept` で要求すると 404 を返します (投稿のページは、下で説明するようにリダイレクトします)。
  シェルには常に `Vary: Accept` が付きます。前段に CDN を置くときは、`Accept` の種類 (ActivityPub か HTML か) をキャッシュキーに含めるか、シェル、`/ap/*`、`/@*` をキャッシュから外してください。
  CDN から提供するのは `/assets/*` だけにしてください。
- ビルドのテキストファイル (スクリプト、スタイルシート、SVG 画像、ライセンスの表示) は、圧縮した形だけが埋め込まれています。Brotli を受け付けるリクエストには `Content-Encoding: br` で、gzip を受け付けるリクエストには `Content-Encoding: gzip` で返し、それ以外のリクエストには展開して返します。いずれも `Vary: Accept-Encoding` を付け、エンコーディングごとの ETag を付けます。
  前段のプロキシは、これらのために自分で圧縮する必要はありません。CDN は `/assets/*` のキャッシュキーに `Vary: Accept-Encoding` (またはエンコーディング) を残してください。
- シェルとサインインの画面には、ページが最初に読み込むスタイルシートとエントリーのスクリプトを指す `Link` ヘッダーが付きます。Cloudflare (Early Hints の機能) などの CDN は、これをもとに独自のアーリーヒントを作れます。
  [`web_early_hints`](../reference/configuration.md#web_early_hints) = `true` なら、サーバーも `103 Early Hints` のレスポンスで先にこれを送り、サーバーがページを用意する間にブラウザーがファイルを取得できるようにします。103 を中継できないプロキシはリクエストを失敗させることがあるので、既定ではオフです。
  - Google Cloud の外部アプリケーションロードバランサーは 502 か 503 (`backend_interim_response_not_supported`) で応えます。その背後ではオフのままにしてください。Cloud Run のデプロイもそうしています。
  - nginx は 1.29.0 から、`early_hints` を設定すれば中継します。
  - Caddy の `reverse_proxy` は中継します。`encode` ディレクティブは 2.8 より前のリリースでは 103 を落とします。
  - ブラウザーが 103 を使うのは、移動先のページだけです。Chrome と Edge は、HTTP/2 と HTTP/3 でだけ使います。
- Web クライアントなしでビルドしたバイナリ (先にクライアントをビルドせずに、ただ `go build` したもの) は、起動時に警告をログに出し、スクリプトのないシェルを提供します。

<!-- source: federation-endpoints 7aaee8c9da42 -->
## 連合のエンドポイント

`role` が `web` か `all` のプロセスは、次のエンドポイントに応えます。ほかの Fediverse のサーバーは、これらを使ってあなたのアカウントを見つけ、表示します。

| パス | 内容 |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger (RFC 7033)。`resource` は `acct:alice@{account_domain}` (`acct:alice@{host}` も使えます)、Actor の `id`、または `/@alice` の URL です。`subject` は常に `account_domain` 側のハンドルです |
| `GET /.well-known/host-meta`、`/.well-known/host-meta.json` | WebFinger の URI テンプレート (XRD / JRD) |
| `GET /.well-known/nodeinfo`、`GET /nodeinfo/2.1` | NodeInfo 2.1。`metadata.sourceCode` は設定 `source_url` で、`https:` の URL なら `software.repository` も設定します ([ソースコードを公開する](#publish-the-source-code-agpl-30)を参照) |
| `GET /ap/actors/{uuid}` | Actor (`publicKey` は RSA の `#main-key`)。HTML を好むリクエストには `/@{username}` への 303 を返します |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | コレクション |
| `POST /ap/actors/{uuid}/inbox`、`POST /ap/inbox` | ローカルの各アクターの inbox と共有 inbox。リクエストには HTTP 署名が必要です |
| `GET /@{username}` | ActivityPub の `Accept` なら Actor の `id` への 302。そうでなければ Web クライアントの HTML (下記を参照) |
| `GET /@{username}/status/{uuid}` | ActivityPub の `Accept` なら、ログインしていない訪問者が見られる投稿はその `id` (`/ap/objects/{uuid}`) への 302、見られないなら 404。そうでなければ Web クライアントの HTML |

- インスタンスの Actor のハンドルは `{account_domain}@{account_domain}` です (Mastodon と同じ形)。
- 存在しないか表示できない (停止された、無効にされた、削除された、またはリモートの) アカウントには、常に同じ 404 を返します (`Cache-Control: no-store`)。
- 匿名のレスポンスには `Cache-Control: public, max-age=60` が付きます。`Accept` によって変わるレスポンスには `Vary: Accept` が付きます。
  `Vary: Accept` を扱えない CDN では、`/ap/*` と `/@*` をキャッシュから外してください。

<!-- source: separate-handle-and-delivery-domains b1c23d994d0e -->
### ハンドルと配送のドメインを分ける

`account_domain` が `host` と異なるとき、`account_domain` 側の `/.well-known/webfinger`、`/.well-known/host-meta`、`/.well-known/nodeinfo` へのリクエストは、301 で `host` にリダイレクトされます (`Access-Control-Allow-Origin: *` 付き)。
`account_domain` へのリクエストをこのサーバーに向けているなら、サーバー自身がそのリダイレクトを返します。
ほかのサーバーやリバースプロキシが代わりに応えるなら、パスとクエリ文字列を保ったまま `https://{host}` にリダイレクトしてください。
