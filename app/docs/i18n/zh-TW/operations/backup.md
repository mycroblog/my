<!-- source: backup-and-restore 845c6a2e72dd -->
# 備份與還原

本頁說明要備份什麼、如何在實例持續運作的情況下進行各項備份、如何從備份還原實例，以及如何在真正需要之前演練還原。升級時也會進行備份；相關步驟請參閱[升級](database.md#upgrading)。

<!-- source: what-to-back-up 916727936cca -->
## 要備份什麼

![要備份的項目與還原的順序：資料庫和媒體一起備份，KEK 與它們分開備份；還原時，在伺服器啟動之前放回資料庫、媒體，以及搭配同一把 KEK 的設定。下方的列表說明了相同的內容。](images/backup-restore.svg)

依圖表由上而下：

- **資料庫**（`database_url`：SQLite 或 PostgreSQL）最先備份，使用 `my db backup` 或 `pg_dump`，每天一次，以及每次升級之前。
- **媒體**（`blob_fs_dir`，或 `s3`、`gcs` blob 儲存區的 bucket）緊接在資料庫之後備份，方式可以是複製、bucket 的版本或副本。這兩份備份屬於同一個世代，要放在一起，存放在伺服器以外的地方並加密。
- **KEK**（`MY_KEK` 或 `MY_KEK_FILE`）備份一次，之後每次輪替後連同 `kek_previous` 再備份，而且絕不要和資料庫備份放在同一個地方。使用 `key_manager = gcpkms` 或 `awskms` 時，KEK 留在 KMS 中：只要你還保留著需要它的備份，就要讓 `kms_key` 的金鑰保持啟用（[停用 AWS KMS 金鑰](key-management.md#retire-an-aws-kms-key)）。
- **設定**（環境、`MY_CONFIG`、`.env` 和密鑰）在變更時與部署檔案一起複製，但不包含 KEK。
- **還原**要在空的環境中依以下順序進行：資料庫，接著在伺服器啟動前放回使用相同鍵值的媒體，然後是搭配相同 KEK、`kek_previous` 和 `host` 的設定；之後啟動伺服器，並檢查 `/readyz`、`my projection verify` 和 actor 的公開金鑰（[還原](#restoring)）。沒有 KEK，伺服器就無法啟動；使用另一把 KEK 時，讀取密鑰會以 `unknown key-encryption key` 失敗（[沒有正確的 KEK 時](#without-the-right-kek)）。
- **不需要備份**的有：`kv = memory` 或 `valkey` 時的鍵值儲存區（快取和速率限制）、任務（它們在資料庫中）、日誌和指標，以及搜尋引擎的索引（`my search rebuild` 會重新建立）。

| 項目 | 存放位置 | 備份方式 | 沒有備份會失去什麼 |
| --- | --- | --- | --- |
| 資料庫 | SQLite 檔案（`database_url`）或 PostgreSQL 資料庫 | [SQLite](#sqlite) 或 [PostgreSQL](#postgresql) | 一切：帳號、貼文、追蹤、設定 |
| 媒體 | `fs` blob 儲存區的目錄 `blob_fs_dir`，或 `s3` blob 儲存區的 bucket `blob_s3_bucket`、`gcs` blob 儲存區的 `blob_gcs_bucket` | [媒體](#media)或 [bucket 中的媒體](#media-in-a-bucket) | 上傳的圖片、影片和音訊檔案，以及快取的遠端檔案 |
| KEK | 你存放 `MY_KEK` 或 `MY_KEK_FILE` 檔案的地方（使用 Compose 時為 `secrets/kek`；絕不在資料庫中） | [KEK](key-management.md#back-it-up)，與資料庫分開 | 每個 actor 的私密金鑰（以及隨之而來的 actor 在聯邦宇宙中的身分），還有資料庫儲存的其他密鑰 |
| 設定 | 環境變數、設定檔（`MY_CONFIG`）、密鑰（`MY_VAPID_PRIVATE_KEY`、郵件和付款的金鑰）、使用 Compose 時的 `.env` | 與部署檔案一起複製 | 資料不會遺失，但實例無法以相同方式啟動；變更 `host` 會破壞所有聯邦 URL |

不需要備份的項目：

- **鍵值儲存區**（`kv`）。`kv = db` 時它是資料庫的一部分。`kv = memory` 或 `kv = valkey` 時，它存放快取、速率限制計數器，以及短暫的狀態，例如正在等待第二個驗證因素的登入。以空的 Valkey 還原後，快取會重新填滿，速率限制會重新計算，而進行中的登入或語音通話必須重新開始。
- **任務佇列**（`queue`）。`queue = db` 和 `queue = cloudtasks` 時，任務是資料庫的一部分，會隨資料庫一起回來。Cloud Tasks 的 task 只負責為資料庫中的任務傳送訊號；還原後，伺服器會在幾分鐘內再次為到期的任務傳送訊號。
- **日誌和指標**，伺服器不會讀回它們。
- **搜尋引擎的索引**（如果有的話）。`my search rebuild` 會從資料庫重新建立它。如果你確實會為搜尋引擎建立快照，快照中包含貼文的文字，也包括快照之後才刪除的貼文，所以請把快照當作資料庫備份一樣看待（[全文搜尋](search.md#backups)）。

Web Push 的 VAPID 金鑰是設定中的密鑰。換成新的金鑰會讓所有現有訂閱都收不到推播通知（[Web Push VAPID 金鑰](key-management.md#web-push-vapid-key)），所以請將它和設定一起保管。

<!-- source: how-often-and-where 17e55765e9a0 -->
## 頻率與存放位置

- **資料庫**：至少每天一次，以及每次升級之前。請保留多個世代，例如 7 份每日、4 份每週和 3 份每月備份，讓較晚才發現的錯誤也還能復原。具備時間點復原功能的託管 PostgreSQL，可以涵蓋其保留期間內的天數。
- **媒體**：每次都與資料庫一起備份（請參閱[資料庫與媒體的順序](#the-order-of-the-database-and-the-media)）。
- **KEK 和設定**：在變更時備份。KEK 輪替也是一種變更：請在使用新的 KEK 之前先備份它，並在 `kek_previous` 仍列出舊 KEK 的期間保留舊的那一把。
- **存放位置**：遠離伺服器及其硬碟，放在另一個故障範圍（另一台機器、另一個區域、另一個服務商或帳號）。存放在伺服器自身硬碟上的備份，無法在該硬碟損毀時倖存。
- **加密**：資料庫備份中包含電子郵件地址、密碼雜湊和私訊；請將其靜態加密（使用加密的 bucket，或在上傳前以 `age` 或 `gpg` 加密檔案）。其中的密鑰（例如 actor 的私密金鑰）是以 KEK 加密的，所以**絕不要把 KEK 和資料庫備份放在同一個地方**：同時取得兩者的人就能讀取所有密鑰。
- **保留**：備份中會保留使用者已刪除的資料，或你應要求而清除的資料。請依照隱私權政策所述的時程刪除舊的世代。

<!-- source: backing-up 0c38c68033e4 -->
## 備份

備份期間伺服器可以繼續運作。若要取得與某個時間點完全一致的備份（例如升級之前），請先切換到[維護模式](maintenance.md)：在維護模式下，`my serve` 不會開啟任何資料庫連線，也不會寫入任何東西。

<!-- source: sqlite 7105c29de4c9 -->
### SQLite

`my db backup <file>` 會在伺服器持續運作的情況下，將 SQLite 資料庫的一致性副本寫入一個新檔案。副本包含指令開始前已提交的所有寫入（包括仍在 `-wal` 檔案中的寫入），不包含之後的寫入。它是單一檔案，而且因為省略了空閒頁面，會比原檔案小。指令會拒絕已經存在的檔案，而且只能在 `database = sqlite` 時使用。

```sh
my db backup /backup/my-$(date +%Y%m%d%H%M).db
```

使用 Docker Compose 時，請在伺服器的容器中執行。副本會寫在容器內，所以請寫到資料 volume，再從那裡移出：

```sh
cd deploy/docker
docker compose exec -T my my db backup /data/backup.db
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'mv /data/backup.db /backup/my-$(date +%Y%m%d%H%M).db'
```

伺服器執行期間，請不要用 `cp` 或 `tar` 複製資料庫檔案：單獨的檔案會缺少仍在 `-wal` 檔案中的寫入，而在寫入途中取得的副本可能會損毀。沒有任何行程開啟資料庫時（伺服器已停止，或處於維護模式），直接複製就沒問題；此時請將 `-wal` 和 `-shm` 檔案一起複製。

<!-- source: postgresql 45b14a5a4970 -->
### PostgreSQL

請以 `pg_restore` 能讀取的自訂格式使用 `pg_dump`。它讀取的是單一快照，所以伺服器可以繼續運作。

```sh
pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
```

使用 Compose 層 `compose.postgres.yaml` 時，請在資料庫的容器中執行（以 `-f` 或在 `COMPOSE_FILE` 中指定該層）：

```sh
cd deploy/docker
docker compose exec -T postgres pg_dump --format=custom -U my my > my-$(date +%Y%m%d%H%M).dump
```

請使用與伺服器相同主要版本或更新版本的 `pg_dump`。

託管的 PostgreSQL 有自己的備份機制，對於大型資料庫是更好的選擇：

- **Cloud SQL** 和 **Amazon RDS** 每天會自動備份；開啟時間點復原後，可以還原到保留期間內的任何一秒。請將兩者都開啟，將保留期間設定為涵蓋你想要的世代，並在每次升級前進行隨選備份（Cloud SQL 的隨選備份、RDS 快照）。
- 它們的還原會建立新的執行個體（或覆寫現有的）：之後請將 `database_url` 指向它。
- 這些備份留在服務商的帳號內。請不時另外匯出一份 `pg_dump` 到帳號以外的地方，以免失去帳號時連資料也一起失去。

<!-- source: media a2445f4d5501 -->
### 媒體

`fs` blob 儲存區將所有檔案存放在 `blob_fs_dir`（使用 Compose 時為 `/data/blobs`）之下：上傳的檔案在 `objects/`，正在寫入的檔案在 `tmp/`。請用任何能複製檔案的工具（`tar`、`rsync`、volume 或硬碟的快照）複製該目錄，但排除 `tmp/`。檔案會先寫在 `tmp/` 之下，再一次性地重新命名到 `objects/`，所以在伺服器執行期間取得的副本，`objects/` 中絕不會有寫到一半的檔案。

```sh
docker run --rm -v my_data:/data -v "$PWD":/backup busybox \
  tar czf /backup/blobs-$(date +%Y%m%d%H%M).tar.gz -C /data --exclude=blobs/tmp blobs
```

由於媒體檔案一旦寫入就不會再變更，以 `rsync` 備份到備份主機時，第一次之後的複製量都會很小。

<!-- source: media-in-a-bucket 882f3a0f9a2b -->
### bucket 中的媒體

`s3` 和 `gcs` blob 儲存區將每個檔案存成 bucket `blob_s3_bucket` 或 `blob_gcs_bucket` 中的物件，以其鍵值命名：處理後的媒體在 `media/` 之下，原樣收到的上傳檔案在 `uploads/` 之下，快取的遠端檔案在 `remote/` 之下。bucket 保持私有：由伺服器讀取檔案，並在 `/media/` 之下提供。物件只有在完整寫入後才會出現，所以在伺服器執行期間取得的副本絕不會有寫到一半的檔案。請以三層保護 bucket，從成本最低到最安全依序如下。

**版本控制**。 在服務有提供的地方（Amazon S3、Google Cloud Storage、MinIO 等），請開啟 bucket 的物件版本控制。這樣被刪除或覆寫的物件會以非目前版本保留下來，bucket 就能回到資料庫備份當時的狀態。

```sh
aws s3api put-bucket-versioning --bucket my-media --versioning-configuration Status=Enabled
```

**生命週期規則**。 開啟版本控制後，每個被刪除的檔案在被規則移除之前都會持續產生儲存費用。請新增一條規則，在超過你最舊的資料庫備份的保存天數後刪除非目前版本（以[頻率與存放位置](#how-often-and-where)中不含每月備份的世代來說是 35 天），讓你保留的每一份資料庫備份都還能找到它的檔案，而使用者刪除的檔案也不會保留得比隱私權政策所述更久。也請新增一條規則，在一天後中止未完成的分段上傳：伺服器會自行中止失敗的大型檔案上傳，但在上傳途中停止的行程會留下它的分段。

```json
{
  "Rules": [
    {
      "ID": "noncurrent-versions",
      "Status": "Enabled",
      "Filter": {},
      "NoncurrentVersionExpiration": { "NoncurrentDays": 35 },
      "AbortIncompleteMultipartUpload": { "DaysAfterInitiation": 1 }
    }
  ]
}
```

```sh
aws s3api put-bucket-lifecycle-configuration --bucket my-media --lifecycle-configuration file://lifecycle.json
```

**另一個帳號或區域中的副本**。 版本控制能防範失誤，卻無法防範失去 bucket 或擁有它的帳號，而且伺服器自己的憑證就能刪除物件。請保留一份這些憑證無法觸及的副本：

- **複寫**。 Amazon S3 可以將有版本控制的 bucket 複寫到另一個區域或另一個帳號中的 bucket（跨區域或同區域複寫，目的地由另一個帳號擁有）。目的地也請開啟版本控制，並且不要複寫刪除（關閉刪除標記的複寫），讓實例上的刪除不會影響到副本。其他服務有各自的複寫功能，例如 Cloud Storage 的雙區域 bucket 和 MinIO 的 bucket 複寫。
- **排程複製**。 在沒有複寫功能的情況下，或要複製到另一個服務商時，請依照資料庫備份的排程，在其之後立即複製 bucket，並使用只能讀取實例 bucket、只能寫入備份 bucket 的憑證：

  ```sh
  rclone copy --checksum source:my-media backup:my-media-backup
  ```

  不加 `--delete` 的 `rclone copy` 和 `aws s3 sync` 只會新增物件，絕不會從副本中移除物件。檔案一旦寫入就不會再變更，所以第一次執行之後，每次複製只會傳輸新的檔案。請依照你的保留政策排程從副本中刪除舊物件，或在那裡也開啟版本控制和生命週期規則。

憑證不要放進副本：bucket 本身不含任何密鑰，但設定在 `blob_s3_access_key_id` 和 `blob_s3_secret_access_key` 中指定了金鑰，它們應該和設定放在一起。`gcs` blob 儲存區不保存任何金鑰：它使用機器或 Cloud Run 的服務帳號。

<!-- source: the-order-of-the-database-and-the-media 52b56171d6d9 -->
### 資料庫與媒體的順序

請**先備份資料庫，再備份媒體**，並將兩者作為同一個世代放在一起。對 bucket 來說，在資料庫備份之後執行的複製或複寫就是媒體備份；開啟版本控制時，資料庫備份當時的版本就是媒體備份。

資料庫會記錄它所使用的媒體檔案名稱，而媒體副本會在稍後才取得。在這段時間內：

- 新上傳的檔案可能會出現在媒體副本中，但資料庫副本中沒有對應的資料列。這些是沒有被使用的檔案；它們會佔用空間，但沒有害處。
- 在資料庫備份之後才刪除的檔案（已刪除貼文的媒體、從未附加的上傳檔案）在資料庫副本中有記錄，但不在媒體副本中。還原後，該檔案會回應 `404`，原本顯示它的貼文會顯示一張遺失的圖片。

另一種順序更糟：在媒體備份與資料庫備份之間上傳的每個檔案，還原後都會遺失，包括新貼文的媒體。在維護模式下同時進行兩者備份，就能避免任何一種落差。

<!-- source: the-kek-and-the-configuration b3d65ec5edb5 -->
### KEK 與設定

KEK 不在資料庫中。請依照[管理金鑰加密金鑰](key-management.md#back-it-up)的說明，與資料庫備份分開備份一次，並在每次輪替後再備份。請將設定（環境、設定檔、`.env` 和密鑰檔案）的副本與部署檔案放在一起，但不包含 KEK。

<!-- source: restoring 4db94bba2bc4 -->
## 還原

備份中保存著其實例的 `host` 和 `account_domain`。請以相同的值啟動還原後的資料庫：值不同時，伺服器會拒絕啟動。

請在空的環境（新的機器、新的 volume 或新的資料庫）中依以下順序還原：資料庫、媒體（目錄或 bucket），然後是搭配備份當時所使用的**同一把 KEK**（以及相同的 `kek_previous`）的設定。接著照常啟動伺服器。

<!-- source: sqlite-1 62820e765e7f -->
### SQLite

停止伺服器，用備份檔案取代資料庫檔案，如果還留有舊的 `-wal` 和 `-shm` 檔案，請將其移除。使用 Docker Compose 時，還原到 volume `my_data`（現有的 volume，或另一台機器上新建的 volume）：

```sh
cd deploy/docker
docker compose down          # keeps the volume; on a new machine, `docker compose up --no-start` creates it
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c '
  rm -rf /data/my.db /data/my.db-wal /data/my.db-shm /data/blobs &&
  cp /backup/my-202601010000.db /data/my.db &&
  tar xzf /backup/blobs-202601010000.tar.gz -C /data &&
  chown -R 65532:65532 /data'
docker compose up -d
```

伺服器映像檔以使用者 `65532` 執行，該使用者必須擁有 volume 中的檔案（上面的 `chown`）。

<!-- source: postgresql-1 4a3134816771 -->
### PostgreSQL

請還原到一個新的空資料庫，準備方式與新實例相同，但不要套用結構描述：dump 中已包含它。

1. 建立角色和資料庫。
2. 還原 dump。以超級使用者身分還原時，還原的資料表會保留其擁有者（該角色必須存在）；以資料庫擁有者身分還原時，請加上 `--no-owner`。

   ```sh
   pg_restore --exit-on-error --dbname="postgres://postgres:...@db.example/my" my-202601010000.dump
   ```

3. 使用託管資料庫自身的備份時，請改用服務商的方式還原，並將 `database_url` 指向還原後的執行個體。

使用 Compose 層 `compose.postgres.yaml` 時，請只在新的 volume 上啟動資料庫，再還原到其中。容器已經建立了角色和資料庫：

```sh
cd deploy/docker
docker compose up -d --wait postgres
docker compose exec -T postgres pg_restore --exit-on-error --no-owner -U my -d my < my-202601010000.dump
docker compose up -d
```

若要改為覆蓋現有的資料庫來還原（回復升級），請如[回復](database.md#rolling-back)所示使用 `pg_restore --clean --if-exists`。

<!-- source: media-1 4292939246df -->
### 媒體

在伺服器啟動之前，將媒體副本解壓縮到 `blob_fs_dir`，並讓伺服器執行時的使用者擁有這些檔案。

<!-- source: media-in-a-bucket-1 2e8d8c7703b4 -->
### bucket 中的媒體

在伺服器啟動之前，將物件還原到一個空的 bucket，然後將 `blob_s3_bucket`（以及端點、區域和憑證）或 `blob_gcs_bucket` 設定為該 bucket。鍵值必須保持不變：資料庫是以鍵值來指定每個檔案。

- 從另一個 bucket 中的副本，複製回來：

  ```sh
  rclone copy --checksum backup:my-media-backup target:my-media-restored
  ```

- 從實例自身 bucket 的版本，將資料庫備份當時的物件複製到新的 bucket。`rclone` 會以 `--s3-version-at` 讀取該時間點的版本：

  ```sh
  rclone copy --s3-version-at "2026-01-01 00:00:00" source:my-media target:my-media-restored
  ```

- 從副本還原時，將 `blob_s3_bucket` 或 `blob_gcs_bucket` 指向該副本，或將其複製回原帳號中的 bucket。

伺服器的憑證需要能讀取、寫入和刪除還原後 bucket 中的物件（在 Amazon S3 上為 `s3:GetObject`、`s3:PutObject`、`s3:DeleteObject` 和 `s3:AbortMultipartUpload`；在 Google Cloud Storage 上為該 bucket 的角色 `roles/storage.objectUser`）。

<!-- source: starting-and-checking f656b59cc0ed -->
### 啟動與檢查

照常啟動伺服器。使用 Docker Compose 時，`docker compose up -d` 會先執行 `my db upgrade -check`、套用結構描述和 `my db upgrade`；對同一個版本的備份來說，這些不會變更任何東西。接著檢查：

1. `/readyz` 回應 `200`，而且伺服器日誌中沒有 `unknown key-encryption key` 錯誤。
2. `my projection verify` 回報沒有差異（[檢查與重建 projection](database.md#checking-and-rebuilding-projections)）。
3. 你可以登入，並且能看到最新的貼文及其媒體。
4. **actor 的金鑰沒有改變**：某個 actor 的 `publicKey.publicKeyPem`（`curl -H 'Accept: application/activity+json' https://<host>/ap/actors/<id>`）與還原前相同。其他伺服器會用這把金鑰驗證實例的簽章，所以金鑰沒有改變，代表聯邦會從中斷的地方繼續。
5. 貼文、追蹤或取消追蹤能送達另一台伺服器（另一個實例上的測試帳號看得到）。

備份之後所做的寫入會遺失：從那時起的貼文、追蹤和上傳在這個實例上都會消失，而其他伺服器可能仍保有它們已收到的活動。這無法從這裡修復；請從你手上最新的備份還原。

使用搜尋引擎時，請在還原後執行不加 `--keep` 的 `my search rebuild`，讓比備份更新的貼文離開索引（[全文搜尋](search.md#backups)）。

<!-- source: without-the-right-kek 2a5031eadf18 -->
### 沒有正確的 KEK 時

- **沒有 KEK**：伺服器無法啟動。`my serve` 和其他會讀取設定的 `my` 指令會在這裡停止，並顯示 `The required setting MY_KEK is not set.` 和 `kek: required when key_manager is local`。
- **另一把 KEK**：伺服器會啟動並回應 `/readyz`，但凡是讀取以舊 KEK 加密的密鑰的操作，都會以 `unknown key-encryption key` 和舊 KEK 的金鑰 ID（`local key manager: key "local:…" is not configured`）失敗：包括遞送到其他伺服器和帶簽章的擷取（actor 的私密金鑰）、使用兩步驟驗證的登入、webhook 簽章，以及外部登入提供者。先前發行的媒體代理 URL 也會失效。密碼和貼文並未以 KEK 加密，所以用密碼登入和閱讀貼文仍然可行，這可能一開始會讓問題不易察覺：請留意日誌中的這個錯誤。
- 找出錯誤中所指金鑰 ID 的 KEK，將其設定為 `kek`（或加入 `kek_previous`），然後重新啟動。沒有 KEK 就無法復原密鑰；如果 KEK 永久遺失，就必須重新產生 actor 的金鑰，而其他伺服器會把每個 actor 視為同一個帳號的新金鑰。

<!-- source: recovery-drill f9adfe0e0d28 -->
## 復原演練

備份只有在實際還原過之後，才能確定它可用。請在實例公開之前演練一次還原，並在備份方式變更之後再演練一次（至少每幾個月一次）：

1. 依上述方式備份正式運作中的實例：先資料庫，再媒體。
2. 在另一台機器上或另一個 Compose 專案中，使用從你保管之處取得的 KEK 和設定（而不是從正式伺服器複製）進行還原，並保持 `host` 不變，讓 actor 的 URL 一致。
3. 不要讓這份副本參與聯邦：讓它與網際網路隔離，或在下列檢查通過後開啟維護模式。
4. 依照[啟動與檢查](#starting-and-checking)逐項確認：登入、貼文和媒體，以及將幾個 actor 的 `publicKey` 與正式實例比較。
5. 分別在沒有 KEK 和使用不同 KEK 的情況下各啟動一次，確認它會如[沒有正確的 KEK 時](#without-the-right-kek)所述那樣失敗。
6. 記下每個步驟花了多久：這就是真正還原時實例會停擺的時間。
