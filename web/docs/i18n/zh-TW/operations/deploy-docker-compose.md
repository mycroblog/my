<!-- source: deploying-with-docker-compose 29ec05aaf767 -->
# 用 Docker Compose 部署

本頁說明如何用 Docker Compose 在一台機器上執行 `my`：這是最小的配置，一個伺服器程序搭配 SQLite，媒體檔案放在具名磁碟區中。PostgreSQL、物件儲存和 TLS 之後可以用其他檔案疊加上去（[Docker Compose 檔案](running.md#docker-compose-files)）。

你需要 Docker 和 Docker Compose v2。

<!-- source: start-the-server ba6cb34daaa0 -->
## 啟動伺服器

Compose 檔案位於原始碼的 [`deploy/docker`](../../deploy/docker/compose.yaml)。請從你執行的版本的標籤取得這些檔案，讓它們與該版本的映像檔相符：

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose 會拉取 `MY_VERSION` 指定的 `ghcr.io/mycroblog/my` 映像檔。映像檔沒有 `latest` 標籤，所以必須指定版本：某個版本可能需要一些步驟，而無聲無息的更新會略過它們（[映像檔的標籤](running.md#get-the-image)）。如果要改為從簽出的原始碼建置映像檔，請加上覆寫檔：`docker compose -f compose.yaml -f compose.build.yaml up -d --build`。
- `schema` 服務會執行伺服器映像檔的 `my schema apply`，把具名磁碟區中的資料庫更新到該版本的結構描述後結束；伺服器（`role=all`）會在它成功之後啟動。每次 `up` 都會重新套用結構描述，所以升級之後也用同樣的方式啟動伺服器。
- 伺服器在 `127.0.0.1:8080` 上監聽（連接埠是 `.env` 中的 `MY_HTTP_PORT`）。當 `curl localhost:8080/readyz` 回傳 `{"status":"ok"}` 時，伺服器就準備好了。
- 金鑰加密金鑰（KEK）會從 `secrets/kek` 以 Docker secret `/run/secrets/my_kek` 的形式傳給伺服器，絕不會透過環境變數傳遞（[管理金鑰加密金鑰](key-management.md)）。
- [設定參考](../reference/configuration.md)中的其他所有設定都在 `.env` 中設定，其範本會說明每一項設定。

<!-- source: create-an-administrator c3d30824f7c0 -->
## 建立管理員

建立一個管理員，然後用 WebFinger 查詢這個帳號並取得它的 actor。密碼會從標準輸入的第一行讀取。

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

<!-- source: sign-in-from-a-browser 3028115543b0 -->
## 從瀏覽器登入

使用 `example.env` 中的 `MY_HOST=localhost:8080` 和 `MY_WEB_ORIGIN=http://localhost:8080` 這組設定時，登入、用戶端 API 和媒體都能透過純 HTTP 在 `http://localhost:8080` 上運作（ActivityPub id 仍然是 `https://localhost:8080/...`）。如果你變更了 `MY_HTTP_PORT`，請把 `MY_HOST` 和 `MY_WEB_ORIGIN` 的連接埠改成一致。

1. 在瀏覽器開啟 `http://localhost:8080/login`（或按網頁用戶端頂端列的 **登入**），它會開啟伺服器的登入畫面。
2. 在登入畫面輸入你在上面建立的管理員的電子郵件地址和密碼。
3. 你會回到網頁用戶端的首頁時間軸。

- 登入用的 cookie 帶有 `Secure`。Chrome 和 Firefox 在 `http://localhost` 上會接受它；Safari（WebKit）不會，所以在本機請使用 Chrome 或 Firefox。
- 貼文內文中的主題標籤和提及的連結使用 `https://`，因為它們是聯邦用的 URL。

<!-- source: going-public be2475df6407 -->
## 對外公開

- 把 `.env` 中的 `MY_HOST` 設成你真正的網域，移除 `MY_WEB_ORIGIN`（預設為 `https://{MY_HOST}`），並在伺服器前端終止 TLS：`compose.caddy.yaml` 這一層會為此執行 Caddy（[Docker Compose 檔案](running.md#docker-compose-files)）。`MY_HOST` 會在第一次啟動時儲存到資料庫，之後無法變更。
- 如果在你自己的反向代理後方執行，請在 `.env` 中設定 `MY_TRUSTED_PROXIES`，讓登入等速率限制依用戶端 IP 套用，而不是套用到代理伺服器上（[`trusted_proxies`](../reference/configuration.md#trusted_proxies)）。
- 在對使用者開放伺服器之前，請逐項檢查[正式環境檢查清單](production-checklist.md)。
- `docker compose down` 會停止整個堆疊。`down -v` 還會刪除資料磁碟區。

所有設定請參閱[設定參考](../reference/configuration.md)；日常營運請參閱[執行伺服器](running.md)。
