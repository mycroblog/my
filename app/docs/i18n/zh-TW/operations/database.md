<!-- source: database-initializing-upgrading-and-recreating b4189f003273 -->
# 資料庫：初始化、升級與重新建立

說明營運者如何建立資料庫、將它升級到新版本，以及（僅限開發與測試用實例）清除它。

結構描述（schema）存放在 `schema/` 中，每個模組一個目錄，每種資料庫一個檔案（`schema/<NN>-<module>/sqlite.sql` 與 `postgres.sql`）。伺服器執行檔內含其版本的結構描述，由 `my schema apply` 套用：每種部署方式都會在伺服器之前，以伺服器映像檔與伺服器的資料庫設定，將它作為一次性任務執行，因此一個實例只需要伺服器映像檔這一個映像檔。`my serve` 絕不會套用結構描述。在沒有結構描述的資料庫上啟動時，伺服器會在 `/readyz` 回應 `503`（請參閱[啟動伺服器](running.md#start-the-server)）。

結構描述變更時需要轉移的資料（一個欄位拆成兩個、某個值移到新的資料表），會由伺服器映像檔本身的 `my db upgrade` 改寫；它在套用結構描述之後，每個版本執行一次。完整的順序請見[升級](#upgrading)。

<!-- source: initializing-applying-the-schema 3b0a5f0defa1 -->
## 初始化（套用結構描述）

`my schema apply` 只會執行讓資料庫符合其版本結構描述所需的陳述式：在空的資料庫上會建立所有內容，在已是最新狀態的資料庫上則什麼也不做。執行幾次都是安全的，每次升級也都會再執行它。

<!-- source: with-my-schema-apply df4290f9f1c4 -->
### 使用 `my schema apply`

`my schema apply` 會讀取伺服器設定中的資料庫設定（`database`、`database_url` 與 `database_dev_url`，來自 `-config <file>` 或環境變數；不需要其他設定），並讓資料庫符合該執行檔版本的結構描述：

```sh
my schema apply -dry-run   # print the SQL statements it would run, and change nothing
my schema apply            # run them
```

- 它會印出所執行的 SQL 陳述式，並在一個交易中執行：只要有一個失敗，就不會留下任何變更。已符合結構描述的資料庫不會被變更（`ok: the database matches the schema; nothing to apply`）。
- 會刪除資料表或欄位的計畫會被拒絕，什麼都不會執行，除非指定了 `-allow-destructive`。只有在版本說明要求時、並在備份之後才指定它。為了變更 SQLite 資料表而重建該資料表，不算是刪除。
- 在 PostgreSQL 上，它會在同一個資料庫的暫時 schema 中建立用來比較的結構描述，並在結束時（包括失敗時）刪除它；角色需要在那裡建立 schema 的權限。若沒有這個權限，請把 `database_dev_url` 設為另一個有權限的資料庫（[`database_dev_url`](../reference/configuration.md#database_dev_url)）。對同一個資料庫的多次執行會互相等待，因此同時執行兩次也都會成功。
- 它支援 `database = sqlite` 與 `database = postgres`，當因錯誤或拒絕刪除而沒有套用任何內容時，會以 `1` 結束。

從 `v0.1.0` 開始，受支援版本之間的升級絕不會包含會遺失資料的差異；`v0.1.0` 之前的版本可能會刪除欄位與資料表，因此需要 `-allow-destructive`。每次升級前都請用 `-dry-run` 閱讀 SQL（請參閱[升級](#upgrading)）。

<!-- source: through-a-connection-pooler dd56827cda63 -->
### 透過連線池

`my schema apply` 必須直接連線到資料庫。它會持有工作階段層級的鎖，而交易模式的連線池（PgBouncer 的 transaction pooling、Neon 的 pooled 連線字串）不會保留這種鎖：透過這種連線池，同時執行兩次會卡住而不是失敗。請把直接連線的 URL 設為 `database_url`。`pool_max_conns` 與其他 `pool_*` 參數可以保留在其中，也可以保留在 `database_dev_url` 與 `stream_postgres_url` 中：不開啟連線池的指令會忽略它們。`my db upgrade`、`my db upgrade -check` 與主控台也使用同一個 URL。

`my serve` 可以使用交易模式的連線池，只要：

- 連線池能處理協定層級的預備陳述式（prepared statement），伺服器的每個查詢都會使用它：PgBouncer 1.21 或更新版本，且 `max_prepared_statements` 大於 0，Neon 的 pooled 連線字串就符合這個條件。在不支援的連線池後方，陳述式會隨機以 `prepared statement ... does not exist` 或 `already exists` 失敗：請在 URL 中加上 `default_query_exec_mode=exec`，這樣就能避開它們，代價是每個查詢多一次往返。
- `stream = postgres` 時，`stream_postgres_url` 要使用直接連線的 URL，因為 `LISTEN` 屬於工作階段。使用經過連線池的 URL 的監聽者雖然能連線，卻收不到任何東西。
- URL 中沒有 `search_path` 或其他連線池在連線時會拒絕的參數：除非 `ignore_startup_parameters` 列出了這些參數，否則 PgBouncer 會關閉這樣的連線。

<!-- source: docker-compose 7971a448363a -->
### Docker Compose

在 [`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml) 中，伺服器映像檔的三個一次性服務會依下列順序執行，每一個都要在前一個成功之後才會執行，最後才啟動伺服器（`my`）：

| 服務 | 執行內容 |
| --- | --- |
| `upgrade-check` | `my db upgrade -check`（空的資料庫會通過） |
| `schema` | `my schema apply`，對象為 `/data/my.db`，或在使用 `compose.postgres.yaml` 層時為其 PostgreSQL（[Docker Compose 檔案](running.md#docker-compose-files)） |
| `upgrade` | `my db upgrade` |

```sh
cd deploy/docker
docker compose up -d             # pulls the image of MY_VERSION; the three one-off services run in order, then my starts
curl -fsS localhost:8080/readyz  # ready when it answers {"status":"ok"}
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
```

- 只要三者中有一個失敗，`my` 就不會啟動。請用 `docker compose logs upgrade-check`（或 `schema`、`upgrade`）閱讀其輸出。
- 建立管理員的方法請見[管理員與角色](admin-cli.md)。

<!-- source: other-platforms 5f4cdac7fe7a -->
### 其他平台

不使用 Compose 時，請以與伺服器相同的資料庫設定，將伺服器映像檔的 `my schema apply` 作為一次性任務執行（Kubernetes Job 或 init 容器、Cloud Run 任務等）：

```sh
# SQLite: mount the volume that holds the database file at /data
docker run --rm -v my_data:/data -e MY_DATABASE=sqlite -e MY_DATABASE_URL=/data/my.db my schema apply

# PostgreSQL
docker run --rm -e MY_DATABASE=postgres -e MY_DATABASE_URL='postgres://my:...@db.example/my' my schema apply
```

<!-- source: upgrading 4996c9dc9019 -->
## 升級

升級到新版本時，一律以**新**版本的伺服器映像檔，依下列順序執行這些步驟：

1. **備份**資料庫（使用檔案系統 blob 儲存區時，也包括媒體）。請參閱[備份](#backing-up)。
2. **`my db upgrade -check`**。它不會變更任何東西。當新的結構描述會刪除資料庫尚未轉移的資料時，它會以 `1` 結束，並指出必須先經過哪個版本升級；這時請停下來，依照[跳過版本](#skipping-releases)進行。空的資料庫會通過。
3. **`my schema apply`**，與[初始化](#initializing-applying-the-schema)相同。請先用 `-dry-run` 檢查 SQL。
4. **`my db upgrade`**。它會執行該版本中資料庫尚未記錄的每個資料升級步驟，重建這些步驟指定的投影（projection），並記錄每個步驟。再次執行時什麼也不做。若某個步驟失敗，它會以 `1` 結束；已完成的步驟仍會保持記錄，再次執行時會從失敗的步驟繼續。
5. **啟動新的伺服器**。在該版本的每個資料升級步驟都被記錄之前，它會在 `/readyz` 回應 `503`，並在日誌中記錄缺少的步驟。

`my serve` 絕不會執行 `my db upgrade -check`、`my schema apply` 或 `my db upgrade`；它們都是一次性任務。對同一個資料庫一次只執行其中一個。

大多數版本沒有資料升級步驟：`my db upgrade -check` 會印出 `ok: the database can be upgraded to this release`，`my db upgrade` 則印出 `ok: no data upgrade step to run`。即便如此仍請兩者都執行，這樣程序就不必依賴閱讀每一份版本說明。版本說明的 **Upgrading** 一節會列出每個版本的步驟。

<!-- source: the-server-during-an-upgrade ef479675745b -->
### 升級期間的伺服器

不保證能零停機升級。在步驟 3 到 5 之間的短暫時間內，舊版伺服器仍能在新的結構描述上運作，但無法參與資料升級步驟。升級時請切換到[維護模式](maintenance.md)：在維護模式下，`my serve` 不會開啟任何資料庫連線，因此備份會保持一致，執行步驟期間也不會有任何寫入。請把維護時段控制在幾個小時以內（[多久](maintenance.md#how-long)）。

![由上到下的升級步驟，以及每個步驟中 /readyz 的回應：升級前為 200 且 status 為 ok，從進入維護模式到新版本啟動為止為 200 且 status 為 maintenance，之後再次為 200 且 status 為 ok。下方的清單說明每個步驟。](images/upgrade.svg)

從圖的上方往下：

- **舊版本**。伺服器照常執行，`/readyz` 回應 `200` 與 `{"status":"ok"}`。
- **維護模式**。以 `MY_MAINTENANCE=true` 重新部署。從這時起到新版本啟動為止，`/readyz` 會回應 `200` 與 `{"status":"maintenance"}`，因此負載平衡器會保留這些執行個體，同時沒有任何請求會到達資料庫。
- **1. 備份**資料庫與媒體。
- **2 到 4：新映像檔的一次性任務**。`my db upgrade -check` 不會變更任何東西；`my schema apply` 要在你閱讀過它的 `-dry-run` 之後執行；`my db upgrade` 執行資料升級步驟。
- **5. 啟動新版本**，不設定 `MY_MAINTENANCE`。當該版本的每個資料升級步驟都已記錄時，`/readyz` 會回應 `200` 與 `{"status":"ok"}`；仍有缺少的步驟（或尚未套用結構描述）時則回應 `503`。

使用 Docker Compose 時，`docker compose up` 會在一次性任務執行之前取代維護模式的容器，因此在步驟 2 到 4 期間，`/readyz` 不會有任何回應（[Docker Compose](#docker-compose-1)）。

<!-- source: checking-the-sql-with--dry-run ac385f82c7d5 -->
### 用 `-dry-run` 檢查 SQL

在步驟 3 套用任何東西之前，請執行 `my schema apply -dry-run`，閱讀它將會執行的內容。大多數版本包含的是新增（`CREATE TABLE`、`ADD COLUMN`、`CREATE INDEX`）。投影資料表也可能被重建，之後再由 `my db upgrade` 從其事件重新填入。對其他任何東西的 `DROP` 或型別變更，應該列在版本說明的 **Breaking changes** 一節中；若沒有列出，請在套用前停下來並回報。除非指定了 `-allow-destructive`，否則 `my schema apply` 會拒絕刪除資料表或欄位的計畫。

<!-- source: docker-compose-1 c4c380ec8daa -->
### Docker Compose

`docker compose up -d` 會自行依序執行步驟 2 到 5（請參閱[初始化](#docker-compose)中的表格）。只要其中一個失敗，新的伺服器就不會啟動。

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

`docker compose up` 會在一次性服務執行之前取代 `my` 容器，並且只在三者都成功之後才啟動新的容器。若其中一個失敗，`docker compose up` 會回報，伺服器則保持停止：這段期間請在前方放置[匯出的維護頁面](maintenance.md#docker-compose)。閱讀 `docker compose logs <service>`，然後修正原因後再次執行 `docker compose up -d`，或是[回復](#rolling-back)。磁碟區名稱 `my_data` 是 Compose 專案名稱（`my`）加上磁碟區名稱（`data`）。

升級時唯一要變更的是 `MY_VERSION`，而且不會有任何東西自動替你變更它：沒有 `latest` 標籤，`0.1` 也只會在 `v0.1` 之內移動。使用 `MY_VERSION=0.1` 時，步驟 3 只需要 `docker compose pull`，就會取得最新的修補版本。若改為從原始碼建置，請簽出新版本的標籤，在步驟 3 使用 `docker compose -f compose.yaml -f compose.build.yaml build`，並在其他步驟使用相同的 `-f` 選項。

<!-- source: kubernetes b423ca2c88f9 -->
### Kubernetes

在推出新映像檔之前，把步驟 2 到 4 作為一個 Job 執行。init 容器會一個接一個執行，每一個都要在前一個成功之後才執行，主容器則最後執行，因此只要失敗，Job 就會在下一個步驟之前停止。請給這些容器與 Deployment 相同的設定。

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

若 Job 失敗，`kubectl logs job/my-upgrade-v0-2-0 -c upgrade-check`（或 `-c schema`、`-c upgrade`）會顯示原因，而 Deployment 會維持在舊的映像檔。若在磁碟區上使用 SQLite，請在 Job 中掛載同一個磁碟區，並先停止 Deployment 的 Pod：SQLite 只接受一個寫入者。

<!-- source: cloud-run a6c3de3c814f -->
### Cloud Run

使用 OpenTofu 模組時，[在 Cloud Run 上部署](deploy-cloud-run.md#upgrading)中的指令碼會執行這些步驟。手動進行時：

Cloud Run 任務各自只執行一個容器，因此請為每個步驟建立一個任務，並以 `--wait` 依序執行，任務失敗時它也會失敗。請給這些任務與服務相同的環境變數與密鑰。

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

若要先檢查 SQL，請以 `--args=schema,apply,-dry-run` 部署 schema 任務並執行，閱讀它的日誌之後，再為了套用而重新部署它。

<!-- source: backing-up b597d3139c2b -->
### 備份

請在伺服器停止寫入之後（處於維護模式或已停止）、升級的任何步驟之前進行備份。[備份與還原](backup.md)完整說明了每個部分；就升級而言：

- **SQLite**：在沒有任何程序開啟資料庫檔案時，連同其 `-wal` 與 `-shm` 檔案一起複製。使用 Compose 時，上面對磁碟區執行的 `tar` 也涵蓋了 `/data/blobs` 中的媒體。
- **PostgreSQL**：以 `pg_restore` 能讀取的自訂格式執行 `pg_dump`。

  ```sh
  pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
  ```

  也可以使用託管資料庫本身的備份（Cloud SQL 的隨選備份、RDS 快照）；請在同一個時間點進行。
- **媒體**：檔案系統 blob 儲存區是一個要與資料庫一起複製的目錄。
- **KEK** 不在資料庫中。請把它保留在原處；還原的資料庫需要同一個 KEK。

<!-- source: rolling-back 59a40002cd86 -->
### 回復

沒有反向遷移。若要回到原本的狀態，請還原升級前的備份，然後啟動**舊**版本：

1. 停止新的伺服器（或將它切換到維護模式）。
2. 還原備份：使用 Compose 時，把封存檔解壓縮到清空後的磁碟區（`docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'rm -rf /data/* && tar xzf /backup/<archive> -C /data'`）；使用 PostgreSQL 時，執行 `pg_restore --clean --if-exists --dbname=<url> <dump>`。
3. 啟動舊版本的映像檔（使用 Compose 時，把 `MY_VERSION` 改回舊版本，然後執行 `docker compose up -d`）。

新版本在升級後寫入的所有內容都會遺失，所以請在升級後盡快回復，趁實例還沒有進行太多聯邦通訊之前。

<!-- source: skipping-releases 9c7038296a6d -->
### 跳過版本

每個版本都支援從同一個次要版本升級（例如從 `v0.2.1` 到 `v0.2.4`），以及從前一個次要版本的最新修補版本升級（從最新的 `v0.1.x` 到 `v0.2.y`）。修補版本絕不會移除欄位或資料表，因此在同一個次要版本之內，任何順序都可以。

若要跨越一個以上的次要版本，請依序經過每個次要版本的最新修補版本升級，每次都執行完整的程序。例如從 `v0.1.3` 到 `v0.4.0`：先升到最新的 `v0.1.x`，再到最新的 `v0.2.x`，再到最新的 `v0.3.x`，最後到 `v0.4.0`。`my db upgrade -check` 會阻止會遺失資料的跳躍，並指出必須經過的版本；在那個時間點，什麼都還沒有變更。

<!-- source: checking-and-rebuilding-projections 40b848afd40d -->
## 檢查與重建投影

有些資料表是投影：伺服器從其模組的事件與資源推導出它們（例如從帳號的審核處分推導出該帳號的限制）。worker 每天會檢查每個投影一次，並修正找到的任何差異。若要手動檢查或修正：

```sh
my projection list              # each projection with its module and tables
my projection verify [<name>]   # derive every projection, or the one named, and print the rows that differ; changes nothing
my projection rebuild [<name>]  # derive them the same way and write the difference
```

- `<name>` 是投影或其資料表之一。
- `verify` 會印出資料表中有、但來源推導不出來的列（`-`），以及來源推導得出、但資料表中沒有的列（`+`），只要有任何差異就以 `1` 結束。
- `rebuild` 每個交易最多寫入 500 列，因此讀取者絕不會看到被清空的投影。請在實例較空閒時執行：在重建期間變更的列會保留新的值，之後的 `verify` 會回報任何剩下的差異。

<!-- source: recreating-erasing-every-row 9ff993b99448 -->
## 重新建立（清除所有列）

重新建立是指清空資料庫並再次套用結構描述。**從 `v0.1.0` 開始，重新建立僅限開發與測試用的實例。絕對不要重新建立與其他伺服器聯邦的公開實例**。

所有列都會遺失且無法復原。以金鑰加密金鑰（KEK）包裝的帳號金鑰也會遺失，因此在相同主機名稱下重新建立的帳號會得到不同的金鑰，對其他伺服器而言是不同的 Actor，它與遠端的追蹤關係也不會恢復。無法套用結構描述變更的公開實例，應改為從備份[回復](#rolling-back)。

沒有任何 `my` 指令能清除資料庫：請用資料庫的工具移除它，然後再次套用結構描述。空的資料庫沒有記錄 `host` 與 `account_domain`，因此下次啟動時會記錄設定的值。

<!-- source: docker-compose-2 eba2326609d1 -->
### Docker Compose

```sh
cd deploy/docker
docker compose stop my                  # stop the server
docker run --rm -v my_data:/data busybox rm -f /data/my.db /data/my.db-wal /data/my.db-shm
docker compose up -d                    # the one-off services apply the schema to the empty database, then my starts
```

- 上傳與處理過的媒體（`/data/blobs`）會保留下來。若要連同資料庫一起清除，請用 `docker compose down -v` 移除具名磁碟區，然後執行 `docker compose up -d`。不加 `-v` 時，磁碟區會被保留。
- 若要先保留一份複本，請停止伺服器，並從磁碟區複製 `my.db`（連同其 `-wal` 與 `-shm` 檔案）。

<!-- source: postgresql 679b0e3a5b1f -->
### PostgreSQL

刪除資料庫，再以相同的擁有者重新建立一個空的資料庫（或使用託管資料庫的主控台），然後執行 `my schema apply`：

```sh
# stop every server and job that uses the database first
dropdb --maintenance-db="postgres://postgres:...@db.example/postgres" my
createdb --maintenance-db="postgres://postgres:...@db.example/postgres" --owner=my my
my schema apply
```
