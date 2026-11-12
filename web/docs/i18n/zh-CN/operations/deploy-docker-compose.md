<!-- source: deploying-with-docker-compose 29ec05aaf767 -->
# 使用 Docker Compose 部署

本文介绍如何用 Docker Compose 在一台机器上运行 `my`：这是最小的配置，只有一个服务器进程，数据库使用 SQLite，媒体文件放在命名卷中。PostgreSQL、对象存储和 TLS 都可以之后通过叠加的文件加上（[Docker Compose 文件](running.md#docker-compose-files)）。

你需要 Docker 和 Docker Compose v2。

<!-- source: start-the-server ba6cb34daaa0 -->
## 启动服务器

Compose 文件位于源代码的 [`deploy/docker`](../../deploy/docker/compose.yaml) 中。请从你所运行版本的标签获取它们，使其与镜像相符：

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose 会拉取 `MY_VERSION` 对应的镜像 `ghcr.io/mycroblog/my`。镜像没有 `latest` 标签，所以必须指明版本：某个版本可能需要执行一些步骤，而静默更新会跳过它们（[镜像的标签](running.md#get-the-image)）。如果要改为从检出的源代码构建镜像，请加上覆盖文件：`docker compose -f compose.yaml -f compose.build.yaml up -d --build`。
- `schema` 服务会运行服务器镜像中的 `my schema apply`，把命名卷中的数据库更新到该版本的模式，然后退出；它成功后服务器（`role=all`）才会启动。每次 `up` 都会重新应用模式，所以升级之后也用同样的方式启动服务器。
- 服务器监听 `127.0.0.1:8080`（端口由 `.env` 中的 `MY_HTTP_PORT` 指定）。当 `curl localhost:8080/readyz` 返回 `{"status":"ok"}` 时，服务器就绪。
- 密钥加密密钥（KEK）从 `secrets/kek` 以 Docker secret `/run/secrets/my_kek` 的形式传给服务器，绝不通过环境变量传递（[管理密钥加密密钥](key-management.md)）。
- [配置参考](../reference/configuration.md)中的其他所有设置都在 `.env` 中设置，其模板对它们有说明。

<!-- source: create-an-administrator c3d30824f7c0 -->
## 创建管理员

先创建一个管理员，然后用 WebFinger 查找该账号并获取它的 actor。密码从标准输入的第一行读取。

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

<!-- source: sign-in-from-a-browser 3028115543b0 -->
## 从浏览器登录

使用 `example.env` 中的 `MY_HOST=localhost:8080` 和 `MY_WEB_ORIGIN=http://localhost:8080` 这组设置时，登录、客户端 API 和媒体都可以在 `http://localhost:8080` 上通过普通 HTTP 使用（ActivityPub id 仍然是 `https://localhost:8080/...`）。如果你修改了 `MY_HTTP_PORT`，请把 `MY_HOST` 和 `MY_WEB_ORIGIN` 的端口改成相同的值。

1. 在浏览器中打开 `http://localhost:8080/login`（或者在 Web 客户端顶栏中按“登录”），会打开服务器的登录页面。
2. 在登录页面上，输入你在上面创建的管理员的电子邮件地址和密码。
3. 你会回到 Web 客户端，进入你的主页时间线。

- 登录 cookie 带有 `Secure` 属性。Chrome 和 Firefox 在 `http://localhost` 上接受它；Safari（WebKit）不接受，所以在本地请使用 Chrome 或 Firefox。
- 帖子正文中话题标签和提及的链接使用 `https://`，因为它们是联邦使用的 URL。

<!-- source: going-public be2475df6407 -->
## 对外公开

- 把 `.env` 中的 `MY_HOST` 设为你真实的域名，删除 `MY_WEB_ORIGIN`（它默认为 `https://{MY_HOST}`），并在服务器前面终止 TLS：叠加文件 `compose.caddy.yaml` 会为此运行 Caddy（[Docker Compose 文件](running.md#docker-compose-files)）。`MY_HOST` 在首次启动时存入数据库，之后无法更改。
- 如果服务器在你自己的反向代理之后，请在 `.env` 中设置 `MY_TRUSTED_PROXIES`，让登录等操作的速率限制按客户端 IP 生效，而不是作用于代理（[`trusted_proxies`](../reference/configuration.md#trusted_proxies)）。
- 在向用户开放服务器之前，请逐项检查[生产环境检查清单](production-checklist.md)。
- `docker compose down` 会停止整个栈。`down -v` 还会删除数据卷。

所有设置请参阅[配置参考](../reference/configuration.md)；日常运维请参阅[运行服务器](running.md)。
