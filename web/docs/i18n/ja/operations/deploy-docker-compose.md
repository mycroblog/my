<!-- source: deploying-with-docker-compose 29ec05aaf767 -->
# Docker Compose でデプロイする

Docker Compose を使って 1 台のマシンで `my` を動かす方法を説明します。サーバーのプロセスが 1 つで、データベースは SQLite、メディアのファイルは名前付きボリュームに置く、いちばん小さな構成です。PostgreSQL、オブジェクトストレージ、TLS は、あとから重ねるファイルで追加します（[Docker Compose のファイル](running.md#docker-compose-files)）。

Docker と Docker Compose v2 が必要です。

<!-- source: start-the-server ba6cb34daaa0 -->
## サーバーを起動する

Compose のファイルは、ソースの [`deploy/docker`](../../deploy/docker/compose.yaml) にあります。イメージと合うように、動かすリリースのタグから取得してください。

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose は `MY_VERSION` のイメージ `ghcr.io/mycroblog/my` を取得します。`latest` タグはないので、バージョンを指定する必要があります。リリースによっては、黙って更新すると飛ばされてしまう手順が必要になることがあるためです（[イメージのタグ](running.md#get-the-image)）。代わりにチェックアウトしたソースからイメージをビルドするには、上書きのファイルを加えます: `docker compose -f compose.yaml -f compose.build.yaml up -d --build`。
- `schema` サービスはサーバーのイメージの `my schema apply` を実行し、名前付きボリュームにあるデータベースをそのリリースのスキーマにしてから終了します。サーバー（`role=all`）は、それが成功してから起動します。スキーマは `up` のたびに適用されるので、アップグレードのあとも同じ方法でサーバーを起動できます。
- サーバーは `127.0.0.1:8080` で待ち受けます（ポートは `.env` の `MY_HTTP_PORT` です）。`curl localhost:8080/readyz` が `{"status":"ok"}` を返せば準備ができています。
- 鍵暗号化鍵（KEK）は、環境変数ではなく、`secrets/kek` から Docker のシークレット `/run/secrets/my_kek` としてサーバーに渡ります（[鍵暗号化鍵の管理](key-management.md)）。
- [設定リファレンス](../reference/configuration.md)にあるそのほかの設定は、すべて `.env` で設定します。テンプレートにそれぞれの説明があります。

<!-- source: create-an-administrator c3d30824f7c0 -->
## 管理者を作る

管理者を作り、WebFinger でそのアカウントを調べて、アクターを取得します。パスワードは標準入力の 1 行目から読み込まれます。

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

<!-- source: sign-in-from-a-browser 3028115543b0 -->
## ブラウザーからログインする

`example.env` にある `MY_HOST=localhost:8080` と `MY_WEB_ORIGIN=http://localhost:8080` の組み合わせでは、ログイン、クライアント API、メディアが `http://localhost:8080` の素の HTTP で動きます（ActivityPub の ID は `https://localhost:8080/...` のままです）。`MY_HTTP_PORT` を変えた場合は、`MY_HOST` と `MY_WEB_ORIGIN` のポートもそれに合わせて変えてください。

1. ブラウザーで `http://localhost:8080/login` を開きます（または Web クライアントの上部のバーで **ログイン** を押します）。サーバーのログイン画面が開きます。
2. ログイン画面で、上で作った管理者のメールアドレスとパスワードを入力します。
3. Web クライアントのホームタイムラインに戻ります。

- ログインの Cookie は `Secure` です。Chrome と Firefox は `http://localhost` でもこれを受け入れますが、Safari（WebKit）は受け入れません。ローカルでは Chrome か Firefox を使ってください。
- 投稿の本文にあるハッシュタグとメンションのリンクは、連合で使う URL なので `https://` になります。

<!-- source: going-public be2475df6407 -->
## 公開する

- `.env` の `MY_HOST` を実際のドメインにし、`MY_WEB_ORIGIN` を削除して（既定値は `https://{MY_HOST}` です）、サーバーの前段で TLS を終端してください。そのためにはレイヤー `compose.caddy.yaml` が Caddy を動かします（[Docker Compose のファイル](running.md#docker-compose-files)）。`MY_HOST` は最初の起動時にデータベースに保存され、あとから変えることはできません。
- 自分で用意したリバースプロキシの後ろに置く場合は、`.env` に `MY_TRUSTED_PROXIES` を設定してください。そうすると、ログインのレート制限などがプロキシではなくクライアントの IP アドレスごとにかかります（[`trusted_proxies`](../reference/configuration.md#trusted_proxies)）。
- サーバーを利用者に公開する前に、[本番運用前のチェックリスト](production-checklist.md)を確認してください。
- `docker compose down` でスタックが止まります。`down -v` はデータのボリュームも削除します。

すべての設定は[設定リファレンス](../reference/configuration.md)を、日々の運用は[サーバーの運用](running.md)を参照してください。
