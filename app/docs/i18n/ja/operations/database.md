<!-- source: database-initializing-upgrading-and-recreating b4189f003273 -->
# データベース: 初期化、アップグレード、作り直し

データベースを作成する方法、新しいリリースへアップグレードする方法、そして開発用とテスト用のインスタンスに限り、データベースを消去する方法を説明します。

スキーマは `schema/` にあり、モジュールごとに 1 つのディレクトリーに、データベースごとのファイルを置いています（`schema/<NN>-<module>/sqlite.sql` と `postgres.sql`）。サーバーのバイナリーは自身のリリースのスキーマを持っており、
`my schema apply` がそれを適用します。どのデプロイでも、サーバーより先に一回限りのジョブとして、サーバーのイメージとサーバーの
データベース設定で実行します。そのため、インスタンスに必要なイメージはサーバーのイメージだけです。
`my serve` はスキーマを適用しません。スキーマのないデータベースで起動すると、サーバーは `/readyz` に `503` を返します（[サーバーを起動する](running.md#start-the-server)を参照してください）。

スキーマの変更で引き継がなければならないデータ（2 つに分かれる列や、新しいテーブルに移る値）は、サーバーのイメージ自身の
`my db upgrade` が書き換えます。これはスキーマを適用したあと、リリースごとに一度実行します。全体の順序は[アップグレード](#upgrading)にあります。

<!-- source: initializing-applying-the-schema 3b0a5f0defa1 -->
## 初期化（スキーマの適用）

`my schema apply` は、データベースをそのリリースのスキーマにするために必要な文だけを実行します。空のデータベースではすべてを作成し、
最新のデータベースでは何もしません。何度実行しても安全で、アップグレードのたびに再び実行します。

<!-- source: with-my-schema-apply df4290f9f1c4 -->
### `my schema apply` で適用する

`my schema apply` は、サーバーの設定のうちデータベースの設定（`database`、`database_url`、`database_dev_url`。
`-config <file>` か環境変数から読みます。ほかの設定は要りません）を読み、データベースをバイナリーのリリースのスキーマにします。

```sh
my schema apply -dry-run   # print the SQL statements it would run, and change nothing
my schema apply            # run them
```

- 実行する SQL 文を出力し、それらを 1 つのトランザクションで実行します。1 つが失敗すると、どの文も残りません。
  すでにスキーマと一致しているデータベースは変更しません（`ok: the database matches the schema; nothing to apply`）。
- テーブルや列を削除する計画は、`-allow-destructive` を指定しない限り拒否され、何も実行されません。このオプションは、
  リリースノートで求められたときに限り、バックアップのあとで指定してください。SQLite のテーブルを変更のために作り直すことは、削除とはみなしません。
- PostgreSQL では、比較に使うスキーマを同じデータベースの一時スキーマに作り、終わったとき（失敗したときも）に削除します。
  そのため、ロールにはそこでスキーマを作成する権限が必要です。権限がない場合は、権限のある別のデータベースを `database_dev_url` に設定してください
  （[`database_dev_url`](../reference/configuration.md#database_dev_url)）。1 つのデータベースに対する実行は互いに待ち合うので、2 つを同時に実行しても両方とも成功します。
- `database = sqlite` と `database = postgres` に対応しています。エラーや削除の拒否によって何も適用しなかったときは、`1` で終了します。

`v0.1.0` 以降、サポートされるリリース間のアップグレードに、データを失う差分が含まれることはありません。`v0.1.0` より前のリリースでは列やテーブルを
削除することがあり、`-allow-destructive` が必要です。アップグレードの前には毎回、`-dry-run` で SQL を読んでください（[アップグレード](#upgrading)を参照してください）。

<!-- source: through-a-connection-pooler dd56827cda63 -->
### コネクションプーラーを通す場合

`my schema apply` はデータベースに直接接続しなければなりません。セッション単位のロックを取るためで、トランザクションモードのプーラー（PgBouncer のトランザクションプーリング、Neon のプール接続文字列）はこのロックを保持しません。プーラーを通すと、2 つの同時実行が失敗せずに止まったままになりました。`database_url` には直接接続の URL を指定してください。`pool_max_conns` などの `pool_*` パラメーターは、その URL にも、`database_dev_url` と `stream_postgres_url` にも残しておいてかまいません。プールを開かないコマンドはそれらを無視します。`my db upgrade`、`my db upgrade -check`、コンソールも同じ URL を使います。

`my serve` は、次の条件を満たせばトランザクションモードのプーラーを使えます。

- プーラーがプロトコルレベルのプリペアドステートメントを扱えること。サーバーはすべてのクエリでこれを使います。PgBouncer 1.21 以降で `max_prepared_statements` を 0 より大きくしたものが該当し、Neon のプール接続文字列はこれを満たしています。これを扱えないプーラーの後ろでは、文が `prepared statement ... does not exist` や `already exists` で不規則に失敗します。その場合は URL に `default_query_exec_mode=exec` を加えてください。クエリごとに往復が 1 回増える代わりに、プリペアドステートメントを使わなくなります。
- `stream = postgres` のときは、`stream_postgres_url` に直接接続の URL を指定すること。`LISTEN` はセッションに属するためです。プールされた URL のリスナーは接続はしますが、何も受け取りません。
- URL に、`search_path` など、プーラーが接続時に拒否するパラメーターが含まれていないこと。PgBouncer は、`ignore_startup_parameters` に列挙されていない限り、そのような接続を閉じます。

<!-- source: docker-compose 7971a448363a -->
### Docker Compose

[`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml) では、サーバーのイメージによる一回限りのサービス 3 つがこの順に、それぞれ
前のサービスが成功したあとでだけ実行され、最後にサーバー（`my`）が起動します。

| サービス | 実行するもの |
| --- | --- |
| `upgrade-check` | `my db upgrade -check`（空のデータベースでは成功します） |
| `schema` | `my schema apply`。`/data/my.db` に対して、またはレイヤー `compose.postgres.yaml` を使うときはその PostgreSQL に対して実行します（[Docker Compose のファイル](running.md#docker-compose-files)） |
| `upgrade` | `my db upgrade` |

```sh
cd deploy/docker
docker compose up -d             # pulls the image of MY_VERSION; the three one-off services run in order, then my starts
curl -fsS localhost:8080/readyz  # ready when it answers {"status":"ok"}
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
```

- 3 つのうちどれかが失敗すると、`my` は起動しません。出力は `docker compose logs upgrade-check`（または `schema`、`upgrade`）で読めます。
- 管理者の作成については[管理者とロール](admin-cli.md)で説明しています。

<!-- source: other-platforms 5f4cdac7fe7a -->
### ほかのプラットフォーム

Compose を使わない場合は、サーバーのイメージの `my schema apply` を、一回限りのジョブ（Kubernetes の Job や init コンテナー、Cloud Run のジョブなど）として、
サーバーと同じデータベース設定で実行します。

```sh
# SQLite: mount the volume that holds the database file at /data
docker run --rm -v my_data:/data -e MY_DATABASE=sqlite -e MY_DATABASE_URL=/data/my.db my schema apply

# PostgreSQL
docker run --rm -e MY_DATABASE=postgres -e MY_DATABASE_URL='postgres://my:...@db.example/my' my schema apply
```

<!-- source: upgrading 4996c9dc9019 -->
## アップグレード

新しいリリースへのアップグレードでは、**新しい**リリースのサーバーのイメージで、必ず次の手順をこの順に実行します。

1. データベース（ファイルシステムの blob ストアを使う場合はメディアも）を**バックアップ**します。[バックアップ](#backing-up)を参照してください。
2. **`my db upgrade -check`**。何も変更しません。新しいスキーマが、データベースでまだ引き継がれていないデータを削除することになる場合は `1` で終了し、
   先に経由するべきリリースを示します。そのときは中止して、[リリースを飛ばす](#skipping-releases)に従ってください。
   空のデータベースは成功します。
3. **`my schema apply`**。[初期化](#initializing-applying-the-schema)と同じです。先に `-dry-run` で SQL を確認してください。
4. **`my db upgrade`**。リリースのデータアップグレード手順のうち、データベースにまだ記録されていないものをそれぞれ実行し、それらの手順が名指しする
   プロジェクションを作り直し、手順ごとに記録します。もう一度実行しても何もしません。手順が失敗すると `1` で終了します。完了した手順は
   記録されたまま残り、もう一度実行すると失敗した手順から再開します。
5. **新しいサーバーを起動します。** そのリリースのデータアップグレード手順がすべて記録されるまで、`/readyz` に `503` を返し、足りない手順をログに出力します。

`my serve` は `my db upgrade -check`、`my schema apply`、`my db upgrade` のどれも実行しません。これらは一回限りのジョブです。1 つのデータベースに対しては、同時に 1 つだけ実行してください。

ほとんどのリリースにはデータアップグレード手順がありません。そのとき `my db upgrade -check` は `ok: the database can be upgraded to this release` を、
`my db upgrade` は `ok: no data upgrade step to run` を出力します。それでも両方を実行してください。そうすれば、手順がすべてのリリースノートを読むことに依存しません。
各リリースの手順は、リリースノートの **Upgrading** の節に載っています。

<!-- source: the-server-during-an-upgrade ef479675745b -->
### アップグレード中のサーバー

無停止でのアップグレードは保証しません。前のサーバーは、手順 3 から 5 までの短い間、新しいスキーマの上でも動き続けますが、
データアップグレード手順には加われません。アップグレードの間は[メンテナンスモード](maintenance.md)に切り替えてください。メンテナンスモードの
`my serve` はデータベース接続を開かないので、バックアップの一貫性が保たれ、手順の実行中に何も書き込まれません。
メンテナンスの時間は数時間以内に収めてください（[どのくらいの長さか](maintenance.md#how-long)）。

![アップグレードの手順を上から下へ並べ、それぞれで /readyz が返すものを示した図。開始前は status ok の 200、メンテナンスモードから新しいリリースが起動するまでは status maintenance の 200、そのあとは再び status ok の 200。下の一覧が各手順を説明しています。](images/upgrade.svg)

図の上から順に説明します。

- **前のリリース。** サーバーは通常どおり動き、`/readyz` は `{"status":"ok"}` とともに `200` を返します。
- **メンテナンスモード。** `MY_MAINTENANCE=true` で再デプロイします。ここから新しいリリースが起動するまで、`/readyz` は `{"status":"maintenance"}` とともに
  `200` を返すので、ロードバランサーはインスタンスを保持したまま、何もデータベースに届きません。
- **1. データベースとメディアをバックアップ**します。
- **2 から 4: 新しいイメージの一回限りのジョブ。** `my db upgrade -check` は何も変更しません。`my schema apply` は、その `-dry-run` を
  読んだあとで実行します。`my db upgrade` がデータアップグレード手順を実行します。
- **5. `MY_MAINTENANCE` を外して新しいリリースを起動します。** リリースのデータアップグレード手順がすべて記録されると `/readyz` は `{"status":"ok"}` とともに `200` を返し、
  1 つでも足りない間（またはスキーマが適用されていない間）は `503` を返します。

Docker Compose では、`docker compose up` が一回限りのジョブの実行前にメンテナンスモードのコンテナーを置き換えるので、手順 2 から 4 の間は
`/readyz` に何も応答しません（[Docker Compose](#docker-compose-1)）。

<!-- source: checking-the-sql-with--dry-run ac385f82c7d5 -->
### `-dry-run` で SQL を確認する

手順 3 で何かを適用する前に、`my schema apply -dry-run` を実行し、実行される内容を読んでください。
ほとんどのリリースに含まれるのは追加（`CREATE TABLE`、`ADD COLUMN`、`CREATE INDEX`）です。プロジェクションのテーブルが作り直されることもあり、
その場合は `my db upgrade` がイベントから埋め直します。それ以外のものの `DROP` や型の変更は、リリースノートの
**Breaking changes** の節に載っているはずです。載っていなければ、適用する前に中止して報告してください。`my schema apply` は、テーブルや
列を削除する計画を、`-allow-destructive` を指定しない限り拒否します。

<!-- source: docker-compose-1 c4c380ec8daa -->
### Docker Compose

`docker compose up -d` は手順 2 から 5 を自動で順に実行します（[初期化](#docker-compose)の表を参照してください）。どれかが失敗すると、新しいサーバーは起動しません。

```sh
cd deploy/docker

# 1. Switch to the maintenance mode. --no-deps keeps the one-off services from running.
MY_MAINTENANCE=true docker compose up -d --no-deps --force-recreate my

# 2. Back up the volume (database and media) while the maintenance mode holds no connection.
docker run --rm -v my_data:/data -v "$PWD":/backup busybox tar czf /backup/my-data-$(date +%Y%m%d%H%M).tar.gz -C /data .

# 3. Set MY_VERSION in .env to the new version (0.2.0, or its digest form 0.2.0@sha256:... from the notes), and pull it.
docker compose pull

# 4. Check the SQL that the schema service will run.
docker compose run --rm --no-deps upgrade-check
docker compose run --rm --no-deps schema schema apply -dry-run

# 5. Upgrade: upgrade-check, schema, upgrade, then my starts again without the maintenance mode.
docker compose up -d
curl -fsS localhost:8080/readyz
```

`docker compose up` は一回限りのサービスの実行前に `my` コンテナーを置き換え、3 つすべてが成功したあとでだけ新しいコンテナーを起動します。
1 つが失敗すると、`docker compose up` がそれを報告し、サーバーは止まったままになります。その間は[書き出したメンテナンスページ](maintenance.md#docker-compose)を前に置いてください。
`docker compose logs <service>` を読み、原因を直して `docker compose up -d` をもう一度実行するか、[ロールバック](#rolling-back)してください。
ボリューム名 `my_data` は、Compose のプロジェクト名（`my`）にボリューム名（`data`）を続けたものです。

アップグレードで変えるのは `MY_VERSION` だけで、自動で変わることはありません。`latest` タグはなく、`0.1` は `v0.1` の中でだけ動きます。`MY_VERSION=0.1` の場合、手順 3 は `docker compose pull` だけで、最新のパッチを取得します。
代わりにソースからビルドする場合は、新しいリリースのタグをチェックアウトし、手順 3 で `docker compose -f compose.yaml -f compose.build.yaml build` を使い、ほかの手順でも同じ `-f` オプションを使ってください。

<!-- source: kubernetes b423ca2c88f9 -->
### Kubernetes

新しいイメージをロールアウトする前に、手順 2 から 4 を 1 つの Job として実行します。init コンテナーは 1 つずつ順に、それぞれ前のものが成功したあとでだけ実行され、
メインのコンテナーは最後に実行されるので、失敗すると次の手順の前で Job が止まります。コンテナーには Deployment と同じ設定を与えてください。

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-upgrade-v0-2-0
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      initContainers:
        - name: upgrade-check
          image: ghcr.io/mycroblog/my:0.2.0
          args: [db, upgrade, -check]
          envFrom: [{ secretRef: { name: my-config } }]   # the Deployment's settings
        - name: schema
          image: ghcr.io/mycroblog/my:0.2.0
          args: [schema, apply]
          envFrom: [{ secretRef: { name: my-config } }]
      containers:
        - name: upgrade
          image: ghcr.io/mycroblog/my:0.2.0
          args: [db, upgrade]
          envFrom: [{ secretRef: { name: my-config } }]
```

```sh
kubectl set env deployment/my MY_MAINTENANCE=true && kubectl rollout status deployment/my
# back up, and check the SQL with a one-off Pod of the new image: my schema apply -dry-run
kubectl apply -f my-upgrade-v0-2-0.yaml
kubectl wait --for=condition=complete --timeout=30m job/my-upgrade-v0-2-0
kubectl set image deployment/my my=ghcr.io/mycroblog/my:0.2.0
kubectl set env deployment/my MY_MAINTENANCE-
kubectl rollout status deployment/my
```

Job が失敗した場合は、`kubectl logs job/my-upgrade-v0-2-0 -c upgrade-check`（または `-c schema`、`-c upgrade`）で理由がわかり、Deployment は前のイメージのままです。
SQLite をボリュームに置いている場合は、同じボリュームを Job にマウントし、先に Deployment の Pod を止めてください。SQLite の書き込み手は 1 つだけです。

<!-- source: cloud-run a6c3de3c814f -->
### Cloud Run

OpenTofu のモジュールを使う場合は、[Cloud Run へのデプロイ](deploy-cloud-run.md#upgrading)のスクリプトがこれらの手順を実行します。手で行う場合は次のとおりです。

Cloud Run のジョブは 1 つにつき 1 つのコンテナーを実行するので、手順ごとにジョブを作り、`--wait` を付けて順に実行します。これを付けると、ジョブが失敗したときにコマンドも失敗します。
ジョブにはサービスと同じ環境変数とシークレットを与えてください。

```sh
gcloud run services update my --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
# back up (see below)

gcloud run jobs deploy my-upgrade-check --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=db,upgrade,-check --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-schema --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=schema,apply --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-upgrade --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=db,upgrade --set-env-vars=... --set-secrets=...

gcloud run jobs execute my-upgrade-check --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-schema --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-upgrade --region=asia-northeast1 --wait &&
  gcloud run deploy my --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 --remove-env-vars=MY_MAINTENANCE
```

先に SQL を確認するには、スキーマのジョブを `--args=schema,apply,-dry-run` でデプロイして実行し、そのログを読んでから、適用のために再デプロイしてください。

<!-- source: backing-up b597d3139c2b -->
### バックアップ

バックアップは、サーバーが書き込みをやめたあと（メンテナンスモードにするか、止めたあと）、アップグレードのどの手順よりも前に取ります。
各部分の詳細は[バックアップと復元](backup.md)で説明しています。アップグレードでは次のとおりです。

- **SQLite**: データベースのファイルを、その `-wal` と `-shm` のファイルとともに、どのプロセスも開いていない間にコピーします。
  Compose では、上のボリュームの `tar` が `/data/blobs` のメディアも含みます。
- **PostgreSQL**: `pg_restore` が読めるカスタム形式で `pg_dump` を取ります。

  ```sh
  pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
  ```

  マネージドデータベース自身のバックアップ（Cloud SQL のオンデマンドバックアップ、RDS のスナップショット）でもかまいません。同じ時点で取ってください。
- **メディア**: ファイルシステムの blob ストアは、データベースとともにコピーするディレクトリです。
- **KEK** はデータベースの中にはありません。いまある場所に保管しておいてください。復元したデータベースには同じ KEK が必要です。

<!-- source: rolling-back 59a40002cd86 -->
### ロールバック

逆向きのマイグレーションはありません。元に戻すには、アップグレードの前に取ったバックアップを復元し、**前の**リリースを起動します。

1. 新しいサーバーを止めます（またはメンテナンスモードに切り替えます）。
2. バックアップを復元します。Compose では、空にしたボリュームにアーカイブを展開します
   （`docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'rm -rf /data/* && tar xzf /backup/<archive> -C /data'`）。
   PostgreSQL では `pg_restore --clean --if-exists --dbname=<url> <dump>` を実行します。
3. 前のリリースのイメージを起動します（Compose では `MY_VERSION` を前のバージョンに戻し、`docker compose up -d` を実行します）。

アップグレードのあとに新しいリリースが書き込んだものはすべて失われます。そのため、ロールバックはアップグレードのすぐあと、インスタンスがあまり連合しないうちに行ってください。

<!-- source: skipping-releases 9c7038296a6d -->
### リリースを飛ばす

各リリースは、同じマイナーバージョンからのアップグレード（たとえば `v0.2.1` から `v0.2.4`）と、1 つ前のマイナーバージョンの最新のパッチリリースからのアップグレード
（最新の `v0.1.x` から `v0.2.y`）に対応しています。パッチリリースは列やテーブルを削除しないので、同じマイナーバージョンの中ではどの順でもかまいません。

2 つ以上のマイナーバージョンをまたぐ場合は、各マイナーバージョンの最新のパッチリリースを順に経由し、そのたびに手順全体を実行してください。
たとえば `v0.1.3` から `v0.4.0` へは、最新の `v0.1.x`、次に最新の `v0.2.x`、次に最新の `v0.3.x`、そして `v0.4.0` の順です。
データを失うような飛ばし方は `my db upgrade -check` が止め、経由するべきリリースを示します。その時点では何も変わっていません。

<!-- source: checking-and-rebuilding-projections 40b848afd40d -->
## プロジェクションの確認と作り直し

一部のテーブルはプロジェクションです。サーバーは、それをモジュールのイベントとリソースから導き出します（たとえば、アカウントの制限をその
モデレーションの操作から導きます）。ワーカーは 1 日に 1 回すべてのプロジェクションを確認し、見つけた差分を直します。手で確認したり直したりするには次のようにします。

```sh
my projection list              # each projection with its module and tables
my projection verify [<name>]   # derive every projection, or the one named, and print the rows that differ; changes nothing
my projection rebuild [<name>]  # derive them the same way and write the difference
```

- `<name>` はプロジェクションか、そのテーブルの 1 つです。
- `verify` は、テーブルにはあるがソースからは導き出されない行（`-`）と、ソースからは導き出されるがテーブルにない行（`+`）を出力し、
  差分があれば `1` で終了します。
- `rebuild` は 1 つのトランザクションで最大 500 行を書き込むので、読み手が空になったプロジェクションを目にすることはありません。インスタンスが静かなときに実行してください。
  作り直しの間に変わった行は新しい値を保ち、残った差分はあとの `verify` が報告します。

<!-- source: recreating-erasing-every-row 9ff993b99448 -->
## 作り直し（すべての行の消去）

作り直しとは、データベースを空にしてスキーマを適用し直すことです。**`v0.1.0` 以降、作り直しは開発用とテスト用のインスタンスに限ります。
ほかのサーバーと連合している公開のインスタンスは、決して作り直さないでください。**

すべての行が失われ、元に戻せません。鍵暗号化鍵（KEK）でラップしたアカウントの鍵も失われるので、同じホスト名で作り直したアカウントは
別の鍵を持ち、ほかのサーバーからは別の Actor になります。リモートとのフォロー関係も戻りません。
スキーマの変更を適用できない公開のインスタンスは、作り直す代わりにバックアップから[ロールバック](#rolling-back)します。

データベースを消去する `my` のコマンドはありません。データベースのツールで削除してから、スキーマを適用し直してください。
空のデータベースには `host` と `account_domain` が記録されていないので、次の起動で設定された値が記録されます。

<!-- source: docker-compose-2 eba2326609d1 -->
### Docker Compose

```sh
cd deploy/docker
docker compose stop my                  # stop the server
docker run --rm -v my_data:/data busybox rm -f /data/my.db /data/my.db-wal /data/my.db-shm
docker compose up -d                    # the one-off services apply the schema to the empty database, then my starts
```

- アップロードされたメディアと処理済みのメディア（`/data/blobs`）は残ります。データベースとともに消去するには、`docker compose down -v` で名前付きボリュームを削除してから、
  `docker compose up -d` を実行します。`-v` を付けなければボリュームは残ります。
- 先にコピーを取っておくには、サーバーを止めて、ボリュームから `my.db`（その `-wal` と `-shm` のファイルとともに）をコピーしてください。

<!-- source: postgresql 679b0e3a5b1f -->
### PostgreSQL

データベースを削除し、同じ所有者で空のデータベースを作り直してから（またはマネージドデータベースのコンソールで行ってから）、`my schema apply` を実行します。

```sh
# stop every server and job that uses the database first
dropdb --maintenance-db="postgres://postgres:...@db.example/postgres" my
createdb --maintenance-db="postgres://postgres:...@db.example/postgres" --owner=my my
my schema apply
```
