# my

ActivityPub 互換のマイクロブログサーバー。Go 製。Google Cloud / AWS / セルフホスト（Docker, microk8s）で、
できる限り低コストに運営できることを目指す OSS です。

> **Status: v0 の開発中（M1: Walking Skeleton）。** 動くのは、管理者アカウントの作成と、他サーバーがアカウントを見つけるための
> WebFinger・Actor・NodeInfo だけです。投稿やフォローはまだできません。
> `v0.1.0` より前は DB の作り直しを含む破壊的変更が入ります。`v0.1.0` 以降は、版の間の更新でデータを引き継ぎます（[ADR-0087](docs/adr/0087-schema-changes-after-launch.md)）。

## 目指すもの

- 月額ほぼ $0〜$5 から運営できる（scale-to-zero、常駐ミドルウェア不要）
- RDB・キュー・ストレージ・実行環境を設定で切り替えられる
- Mastodon / Misskey / GoToSocial などと連合できる
- 最初から多言語対応
- Web（PWA）とモバイルアプリ

## クイックスタート（Docker Compose）

Docker と Docker Compose v2 で、SQLite の最小構成を起動します（[`deploy/docker/compose.yaml`](deploy/docker/compose.yaml)）。

```sh
cd deploy/docker
cp .env.example .env
# .env の MY_KEK に `openssl rand -base64 32` の出力を書く（DB とは別にバックアップする）
docker compose up -d --build
```

- `schema` サービスが Atlas Community Edition で `schema/sqlite/` のスキーマを名前付きボリュームの DB に適用して終了し、
  成功した後にサーバー（`role=all`）が起動します。`up` のたびに適用し直すので、更新後も同じ手順で起動します。
- サーバーは `127.0.0.1:8080` で待ち受けます（ポートは `.env` の `MY_HTTP_PORT`）。`curl localhost:8080/readyz` が
  `{"status":"ok"}` を返せば準備完了です。
- KEK は Docker secret として `/run/secrets/my_kek` に渡り、環境変数には出ません（[鍵暗号化鍵の管理](docs/operations/key-management.md)）。

管理者を作り、WebFinger と Actor を確かめます。パスワードは標準入力の 1 行目から読みます。

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<WebFinger の self リンクの UUID>
```

### ブラウザからログインする

`.env.example` の `MY_HOST=localhost:8080` と `MY_WEB_ORIGIN=http://localhost:8080` の組で、サインイン・クライアント API・メディアは
平文の HTTP の `http://localhost:8080` で動きます（ActivityPub の id は `https://localhost:8080/...` のままです）。
`MY_HTTP_PORT` を変えたときは、`MY_HOST` と `MY_WEB_ORIGIN` のポートも同じ値にします。

1. ブラウザで `http://localhost:8080/login` を開き、「ログインに進む」を押します。
2. サーバーのサインイン画面で、上で作った管理者のメールアドレスとパスワードを入れます。
3. Web クライアントに戻り、ホームのタイムラインが表示されます。

- サインインの Cookie は `Secure` 属性付きです。Chrome と Firefox は `http://localhost` でも受け付けますが、
  Safari（WebKit）は受け付けないため、手元では Chrome か Firefox を使います。
- 投稿の本文のハッシュタグやメンションのリンクは、連合の URL なので `https://` です。

### 公開する

- 公開するときは `.env` の `MY_HOST` を実際のドメインにし、`MY_WEB_ORIGIN` を消して（既定は `https://{MY_HOST}`）、
  TLS を終端するリバースプロキシ（Caddy など）を前に置きます。`MY_HOST` は最初の起動で DB に記録され、後から変えられません。
- リバースプロキシの背後では `.env` に `MY_TRUSTED_PROXIES` を設定し、ログインなどのレート制限を利用者の IP ごとにします
  （値の決め方は [デプロイターゲット](docs/architecture/deployment-targets.md#信頼するプロキシとクライアント-ip)）。
- `docker compose down` で止めます。`down -v` はデータのボリュームも消します。
- CI は同じ手順を `scripts/docker-smoke.sh`（`scripts/task.sh docker:smoke`）で、Docker の構成（`Dockerfile`・`.dockerignore`・`deploy/`）を変える PR と、遅い検査（リリースの関門・nightly）で実行します。

設定項目は [設定リファレンス](docs/reference/configuration.md)、運用は [サーバーの起動・停止](docs/operations/running.md) を参照してください。

## ドキュメント

- [ドキュメント目次](docs/README.md)
- [設計判断（ADR）一覧](docs/adr/README.md)
- [コントリビュートの手引き](CONTRIBUTING.md) / [コーディングエージェント向け](AGENTS.md)

## License

[GNU Affero General Public License v3.0](LICENSE)（[ADR-0011](docs/adr/0011-license.md)）。

例外として、次の範囲は MIT ライセンスです（[ADR-0093](docs/adr/0093-api-client-library-mykit.md) §2）。
クライアント API の仕様とそこから生成するクライアントライブラリを、利用者がライセンスの制約なしに使えるようにするためです。

- `api/spec/` と、そのバンドルの `api/openapi.yaml`
- `pkg/mykit/`（API クライアントライブラリ）
- `pkg/mybot/`（ボット制作ライブラリ）

それ以外（サーバーの実装、Web クライアントなど。サーバーと Web クライアントが `api/spec/` から生成するコードを含む）は AGPL-3.0-only のままです。
