# DB の初期化と作り直し

運営者向けの手順です。要件は FR-801（[機能要件](../requirements/01-functional.md)）、方針は [ADR-0023](../adr/0023-declarative-schema-management.md) です。

DB の初期化は `my` のコマンドではなく、Atlas Community Edition でスキーマ `schema/<dialect>/`（モジュールごとのファイル）を適用して行います。
サーバーの単一バイナリには Atlas を組み込まず（ADR-0023）、Atlas とスキーマは別のイメージ（`Dockerfile` の target `schema`）に入っています。
`my serve` はスキーマを適用しません。未適用のまま起動すると readiness が 503 を返します（[起動する](running.md#start-the-server)）。

## 初期化する（スキーマを適用する）

`atlas schema apply` は、DB を目標のスキーマに合わせる差分だけを流します。空の DB には全体を作り、適用済みの DB では何もしません。
何度実行してもよく、更新のたびに同じ手順で適用し直します。

### Docker Compose

[`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml) では、一回限りの `schema` サービスが適用してから、サーバー（`my`）が起動します。
`scripts/task.sh docker:smoke`（`scripts/docker-smoke.sh`）が確かめているのも、この手順です。

```sh
cd deploy/docker
docker compose up -d --build     # schema が schema/sqlite/ を /data/my.db に適用して終わり、その後に my が起動する
curl -fsS localhost:8080/readyz  # {"status":"ok"} なら準備完了
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
```

- `schema` サービスが失敗すると `my` は起動しません。`docker compose logs schema` で Atlas の出力を確かめます。
- 管理者の作り方は [管理者とロール](admin-cli.md) にあります。

### スキーマのイメージを直接使う

Compose を使わない構成でも、同じイメージを一回限りのジョブ（Kubernetes の Job・initContainer、Cloud Run のジョブなど）として動かします。
イメージの入口は `atlas` で、スキーマはディレクトリ `/schema/sqlite` と `/schema/postgres` にあります（`--to` にディレクトリを渡すと、Atlas は中のファイルを名前の順に読みます）。

```sh
docker build -t my-schema --target schema .

# SQLite: DB のファイルを置くボリュームを /data にマウントする
docker run --rm -v my_data:/data my-schema schema apply \
  --url sqlite:///data/my.db --to file:///schema/sqlite \
  --dev-url 'sqlite://dev?mode=memory' --auto-approve
```

PostgreSQL では、次の 2 つが要ります。

- 適用の前に、準備 SQL `schema/postgres.bootstrap.sql`（拡張機能の作成。冪等）を対象の DB で `psql` などから実行する。
  スキーマのイメージには `psql` が入っていない。
- 差分の計算に使う空の開発用 DB（`--dev-url`）。対象とは別の DB にし、同じ準備 SQL を実行しておく。

```sh
psql -v ON_ERROR_STOP=1 "postgres://my:...@db.example/my" < schema/postgres.bootstrap.sql
psql -v ON_ERROR_STOP=1 "postgres://my:...@db.example/my_dev" < schema/postgres.bootstrap.sql
docker run --rm my-schema schema apply \
  --url 'postgres://my:...@db.example/my?search_path=public' --to file:///schema/postgres \
  --dev-url 'postgres://my:...@db.example/my_dev?search_path=public' --auto-approve
```

ここでの `--auto-approve` は、計算した差分の適用を確かめずに進めるフラグです。`v0.1.0` より前の版では、カラムの削除などデータを失う差分も適用します。`v0.1.0` 以降の版の間の更新では、データを失う差分は入りません（ADR-0087、ADR-0023）。
流れる SQL を先に見るには、`--auto-approve` の代わりに `--dry-run` を付けます。

### リポジトリから（開発）

`scripts/task.sh db:apply`（`DB_DIALECT=sqlite|postgres`、`DB_URL`・`DB_DEV_URL` で宛先を変える）が、固定版の Atlas（`bin/atlas`）で同じ適用を行います。
`scripts/task.sh db:diff` は流す SQL を表示するだけです。

## 作り直す（データを消す）

v0 の DB の初期化は、空にしてから適用し直すことです（FR-801）。差分の適用に失敗したときや、整合性を保てないときに使います（ADR-0023）。
**DB のデータはすべて消え、元に戻せません。** 鍵暗号化鍵（KEK）で包んだアカウントの鍵も消えるので、同じホスト名で作り直したアカウントは
別の鍵と別の Actor になり、連合先のフォロー関係は戻りません。公開して連合しているインスタンスでは使いません。

消す操作は、確認なしには実行されません。

- `atlas schema clean` は、消す内容（`DROP TABLE ...`）を表示してから「Are you sure?」と尋ね、`Apply` を選んだときだけ消す。
- 端末がない（`docker compose run -T`、CI、ジョブ）ときは、尋ねられずに終了コード 1 で終わり、何も消さない。
- 確かめずに消すのは、明示のフラグ `--auto-approve` を付けたときだけ。手順やスクリプトに書くときは、対象の URL を確かめてから付ける。

### Docker Compose

```sh
cd deploy/docker
docker compose stop my                  # サーバーを止める
docker compose run --rm schema schema clean --url sqlite:///data/my.db
#   消す内容が表示され、Apply を選ぶと消える（Abort で中止）
docker compose up -d                    # schema が空の DB に適用し直し、my が起動する
```

- アップロードと処理済みのメディア（`/data/blobs`）は残ります。DB と一緒に消すには、`docker compose down -v` で名前付きボリュームごと消してから `docker compose up -d` します。
  `-v` を付けなければボリュームは消えません。
- 先にバックアップを取る場合は、サーバーを止めてからボリュームの `my.db`（SQLite）を複製します。

### PostgreSQL・スキーマのイメージを直接使う

```sh
docker run --rm -it my-schema schema clean --url 'postgres://my:...@db.example/my?search_path=public'
```

`clean` は `search_path` のスキーマ（`public`）の中身を消します。拡張機能も消えることがあるので、その後は [初期化する](#初期化するスキーマを適用する) の準備 SQL から実行し直します。

### リポジトリから（開発）

`scripts/task.sh db:reset` は `atlas schema clean --auto-approve` の後に `scripts/task.sh db:apply` を実行します。確かめずに消すので、開発用の DB だけに使います。
