<!-- source: backup-and-restore 845c6a2e72dd -->
# 备份与恢复

本文说明需要备份什么、如何在实例继续运行的情况下完成每项备份、如何从备份恢复实例，以及如何在真正需要之前演练恢复。升级时也要做备份，相关步骤见[升级](database.md#upgrading)。

<!-- source: what-to-back-up 916727936cca -->
## 需要备份什么

![需要备份的内容和恢复顺序：数据库和媒体一起备份，KEK 与它们分开备份；恢复时在服务器启动前放回数据库、媒体，以及使用同一个 KEK 的配置。下面的列表内容相同。](images/backup-restore.svg)

按图中从上到下的顺序：

- **数据库**（`database_url`：SQLite 或 PostgreSQL）最先备份，使用 `my db backup` 或 `pg_dump`，每天一次，并在每次升级前进行。
- **媒体**（`blob_fs_dir`，或 `s3`、`gcs` blob 存储的存储桶）紧接着备份，方式可以是复制、存储桶的版本或副本。这两份备份构成一代，放在一起保存，存放在服务器之外并加密。
- **KEK**（`MY_KEK` 或 `MY_KEK_FILE`）备份一次，每次轮换后再备份，连同 `kek_previous` 一起，并且绝不与数据库备份放在同一个地方。使用 `key_manager = gcpkms` 或 `awskms` 时，KEK 保留在 KMS 中：只要还保留着需要它的备份，就要让 `kms_key` 的密钥保持启用（[停用 AWS KMS 密钥](key-management.md#retire-an-aws-kms-key)）。
- **配置**（环境变量、`MY_CONFIG`、`.env` 和各项密钥）在发生变化时随部署文件一起复制，不包含 KEK。
- **恢复**要在空环境中按以下顺序进行：数据库，然后在服务器启动前放回使用相同键名的媒体，再放回使用同一个 KEK、`kek_previous` 和 `host` 的配置；然后启动服务器，检查 `/readyz`、`my projection verify` 和各 actor 的公钥（[恢复](#restoring)）。没有 KEK 时服务器无法启动；使用另一个 KEK 时，读取密钥会失败并报 `unknown key-encryption key`
  （[没有正确的 KEK 时](#without-the-right-kek)）。
- **不需要备份**的有：`kv = memory` 或 `valkey` 时的键值存储（缓存和速率限制）、任务（它们在数据库中）、日志和指标，以及搜索引擎的索引（`my search rebuild` 会重新生成）。

| 内容 | 存放位置 | 备份方法 | 没有备份时会丢失 |
| --- | --- | --- | --- |
| 数据库 | SQLite 文件（`database_url`）或 PostgreSQL 数据库 | [SQLite](#sqlite) 或 [PostgreSQL](#postgresql) | 一切：账号、帖子、关注关系、设置 |
| 媒体 | `fs` blob 存储的目录 `blob_fs_dir`，或 `s3` blob 存储的存储桶 `blob_s3_bucket`、`gcs` blob 存储的存储桶 `blob_gcs_bucket` | [媒体](#media)或[存储桶中的媒体](#media-in-a-bucket) | 上传的图片、视频和音频文件，以及缓存的远程文件 |
| KEK | 你保存 `MY_KEK` 或 `MY_KEK_FILE` 所指文件的地方（使用 Compose 时为 `secrets/kek`；绝不在数据库中） | [KEK](key-management.md#back-it-up)，与数据库分开 | 每个 actor 的私钥，以及随之而来的该 actor 在联邦宇宙中的身份，还有数据库中保存的其他密钥 |
| 配置 | 环境变量、配置文件（`MY_CONFIG`）、各项密钥（`MY_VAPID_PRIVATE_KEY`、邮件和支付的密钥）、使用 Compose 时的 `.env` | 随部署文件一起复制 | 数据不会丢失，但实例无法以相同方式启动；更改 `host` 会使所有联邦 URL 失效 |

不需要备份的内容：

- **键值存储**（`kv`）。`kv = db` 时它是数据库的一部分。`kv = memory` 或 `kv = valkey` 时，它保存缓存、速率限制计数器，以及短期状态，例如等待第二重验证的登录。用空的 Valkey 恢复后，缓存会重新填充，速率限制重新计数，进行中的登录或语音通话需要重新开始。
- **任务队列**（`queue`）。`queue = db` 和 `queue = cloudtasks` 时，任务是数据库的一部分，会随数据库一起恢复。Cloud Tasks 的任务只是为数据库中的任务传递信号；恢复后，服务器会在几分钟内再次为到期的任务发出信号。
- **日志和指标**，服务器不会读回它们。
- **搜索引擎的索引**（如果有的话）。`my search rebuild` 会从数据库重新生成它。如果你确实为搜索引擎做了快照，快照中包含帖子的文字，包括快照之后被删除的帖子，因此要像对待数据库备份一样对待它们（[全文搜索](search.md#backups)）。

Web Push 的 VAPID 密钥是配置中的一项密钥。换用新密钥会使所有现有订阅的推送通知停止（[Web Push VAPID 密钥](key-management.md#web-push-vapid-key)），所以要把它和配置放在一起保存。

<!-- source: how-often-and-where 17e55765e9a0 -->
## 备份频率和存放位置

- **数据库**：至少每天一次，并在每次升级前进行。保留多代备份，例如 7 份每日、4 份每周和 3 份每月备份，这样即使很晚才发现错误也仍然可以撤销。支持时间点恢复的托管 PostgreSQL 可以覆盖其保留期内的日子。
- **媒体**：每次都与数据库一起备份（见[数据库与媒体的顺序](#the-order-of-the-database-and-the-media)）。
- **KEK 和配置**：在它们变化时备份。KEK 轮换也是一种变化：在使用新 KEK 之前先备份它，并且只要 `kek_previous` 还列着旧 KEK，就继续保留它。
- **存放位置**：远离服务器及其磁盘，放在另一个故障域中（另一台机器、另一个区域、另一个服务商或账号）。放在服务器自身磁盘上的备份，会随该磁盘的损坏一起丢失。
- **加密**：数据库备份包含邮箱地址、密码哈希和私信；要对其进行静态加密（使用加密的存储桶，或在上传前用 `age` 或 `gpg` 加密文件）。其中的密钥（例如各 actor 的私钥）已用 KEK 加密，所以**绝不要把 KEK 和数据库备份放在同一个地方**：同时拿到两者的人可以读取所有密钥。
- **保留期限**：备份会保留用户已删除的数据，或你应要求已清除的数据。请按照隐私政策中写明的周期删除旧的备份。

<!-- source: backing-up 0c38c68033e4 -->
## 备份

备份期间服务器可以继续运行。如果要获得与某个时间点完全一致的备份（例如在升级前），请先切换到[维护模式](maintenance.md)：在维护模式下，`my serve` 不打开任何数据库连接，也不写入任何内容。

<!-- source: sqlite 7105c29de4c9 -->
### SQLite

`my db backup <file>` 在服务器继续运行的同时，把 SQLite 数据库的一致副本写入一个新文件。副本包含命令开始前已提交的所有写入（包括仍在 `-wal` 文件中的写入），不包含之后的写入。它是单个文件，由于省略了空闲页，比原文件小。该命令拒绝写入已经存在的文件，并且只在 `database = sqlite` 时可用。

```sh
my db backup /backup/my-$(date +%Y%m%d%H%M).db
```

使用 Docker Compose 时，在服务器的容器中运行它。副本写在容器内部，所以要把它写到数据卷中，再从那里移出来：

```sh
cd deploy/docker
docker compose exec -T my my db backup /data/backup.db
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'mv /data/backup.db /backup/my-$(date +%Y%m%d%H%M).db'
```

服务器运行时，不要用 `cp` 或 `tar` 复制数据库文件：单独的数据库文件会缺少仍在 `-wal` 文件中的写入，而在写入过程中复制的副本可能已损坏。没有任何进程打开数据库时（服务器已停止，或处于维护模式），直接复制是可以的；这时要连同 `-wal` 和 `-shm` 文件一起复制。

<!-- source: postgresql 45b14a5a4970 -->
### PostgreSQL

使用 `pg_dump` 的自定义格式，`pg_restore` 可以读取这种格式。它读取的是单个快照，所以服务器可以继续运行。

```sh
pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
```

使用 Compose 层 `compose.postgres.yaml` 时，在数据库的容器中运行它（用 `-f` 或在 `COMPOSE_FILE` 中指定该层）：

```sh
cd deploy/docker
docker compose exec -T postgres pg_dump --format=custom -U my my > my-$(date +%Y%m%d%H%M).dump
```

请使用与服务器主版本相同或更新的 `pg_dump`。

托管的 PostgreSQL 有自己的备份，对于大型数据库，这是更好的选择：

- **Cloud SQL** 和 **Amazon RDS** 每天自动备份；开启时间点恢复后，可以恢复到保留期内的任意一秒。请把两者都打开，将保留期设置为覆盖你想要的备份代数，并在每次升级前做一次按需备份（Cloud SQL 的按需备份、RDS 的快照）。
- 它们的恢复会创建一个新实例（或覆盖一个实例）：之后把 `database_url` 指向它。
- 这些备份留在服务商的账号中。也请时不时把 `pg_dump` 导出到账号之外的地方，以免丢失账号时连同数据一起丢失。

<!-- source: media a2445f4d5501 -->
### 媒体

`fs` blob 存储把所有文件保存在 `blob_fs_dir` 下（使用 Compose 时为 `/data/blobs`）：上传的文件在 `objects/` 中，正在写入的文件在 `tmp/` 中。可以用任何复制文件的工具（`tar`、`rsync`、卷或磁盘的快照）复制该目录，`tmp/` 除外。文件先写在 `tmp/` 下，再一步重命名到 `objects/` 中，因此在服务器运行时复制，`objects/` 中也绝不会出现只写了一半的文件。

```sh
docker run --rm -v my_data:/data -v "$PWD":/backup busybox \
  tar czf /backup/blobs-$(date +%Y%m%d%H%M).tar.gz -C /data --exclude=blobs/tmp blobs
```

用 `rsync` 复制到备份主机，第一次之后的复制量会很小，因为媒体文件一旦写入就不再改变。

<!-- source: media-in-a-bucket 882f3a0f9a2b -->
### 存储桶中的媒体

`s3` 和 `gcs` blob 存储把每个文件保存为存储桶 `blob_s3_bucket` 或 `blob_gcs_bucket` 中的一个对象，以其键命名：处理后的媒体在 `media/` 下，按收到时原样保存的上传文件在 `uploads/` 下，缓存的远程文件在 `remote/` 下。存储桶保持私有：由服务器读取文件并在 `/media/` 下提供。对象只有完整写入后才会出现，因此在服务器运行时复制，也绝不会得到只写了一半的文件。请从最便宜到最安全，分三层保护存储桶。

**版本控制。** 在服务支持的情况下（Amazon S3、Google Cloud Storage、MinIO 等），开启存储桶的对象版本控制。这样，被删除或被覆盖的对象会作为非当前版本保留，存储桶可以回到某次数据库备份的时刻。

```sh
aws s3api put-bucket-versioning --bucket my-media --versioning-configuration Status=Enabled
```

**生命周期规则。** 开启版本控制后，每个被删除的文件在被规则移除之前都会持续产生存储费用。添加一条规则，在超过你最旧的数据库备份的保留时间后删除非当前版本（按照[备份频率和存放位置](#how-often-and-where)中的备份代数，不计每月备份时为 35 天），这样你保留的每份数据库备份仍能找到它的文件，而用户删除的文件也不会保留得比隐私政策所说的更久。再添加一条规则，在一天后中止未完成的分段上传：服务器会自行中止大文件上传失败的情况，但在上传过程中停止的进程会留下已上传的分段。

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

**在另一个账号或区域中的副本。** 版本控制能防范操作失误，但不能防范丢失存储桶或拥有它的账号，而且服务器自己的凭据可以删除对象。请保留一份这些凭据无法触及的副本：

- **复制。** Amazon S3 可以把开启了版本控制的存储桶复制到另一个区域或另一个账号中的存储桶（跨区域复制或同区域复制，目标由另一个账号拥有）。目标存储桶也要开启版本控制，并且不要复制删除操作（保持删除标记复制关闭），以免实例上的删除波及副本。其他服务有各自的复制功能，例如 Cloud Storage 的双区域存储桶和 MinIO 的存储桶复制。
- **定期复制。** 没有复制功能时，或者要复制到另一个服务商时，请按数据库备份的周期、紧接在数据库备份之后复制存储桶，使用的凭据只能读取实例的存储桶、只能写入备份存储桶：

  ```sh
  rclone copy --checksum source:my-media backup:my-media-backup
  ```

  不带 `--delete` 的 `rclone copy` 和 `aws s3 sync` 只会添加新对象，从不从副本中删除对象。文件一旦写入就不再改变，所以第一次运行之后，每次复制只传输新文件。请按照保留策略的周期从副本中删除旧对象，或者在那里也开启版本控制和生命周期规则。

凭据不放进副本：存储桶中不含任何密钥，但配置中通过 `blob_s3_access_key_id`
和 `blob_s3_secret_access_key` 指定密钥，它们应与配置放在一起。`gcs` blob 存储不保存密钥：它使用机器或 Cloud Run 的服务账号。

<!-- source: the-order-of-the-database-and-the-media 52b56171d6d9 -->
### 数据库与媒体的顺序

**先备份数据库，再备份媒体**，并把两者作为一代放在一起保存。对于存储桶，在数据库备份之后运行的复制就是媒体备份；开启版本控制时，数据库备份时刻的版本就是媒体备份。

数据库记录了它所使用的媒体文件，而媒体副本是稍后片刻才取得的。在这片刻之间：

- 新上传的文件可能出现在媒体副本中，而数据库副本中没有对应的行。它们是没有被任何东西使用的文件；会占用空间，但没有害处。
- 在数据库备份之后被删除的文件（已删除帖子的媒体、从未附加到帖子的上传文件）在数据库副本中有记录，但不在媒体副本中。恢复后，该文件会返回 `404`，显示它的帖子会显示缺失的图片。

反过来的顺序更糟：在媒体备份和数据库备份之间上传的每个文件在恢复后都会丢失，包括新帖子的媒体。在维护模式下同时完成两项备份，可以避免这两种缺口。

<!-- source: the-kek-and-the-configuration b3d65ec5edb5 -->
### KEK 与配置

KEK 不在数据库中。请按照[管理密钥加密密钥](key-management.md#back-it-up)中的说明，与数据库备份分开备份一次，并在每次轮换后再次备份。把配置（环境变量、配置文件、`.env` 和密钥文件）的副本与部署文件放在一起保存，不包含 KEK。

<!-- source: restoring 4db94bba2bc4 -->
## 恢复

备份中保存着其实例的 `host` 和 `account_domain`。启动恢复后的数据库时要使用相同的值：值不同时，服务器会拒绝启动。

在空环境（新机器、新卷或新数据库）中按以下顺序恢复：数据库、媒体（目录或存储桶），然后是配置，使用备份时正在使用的**同一个 KEK**（以及相同的 `kek_previous`）。然后照常启动服务器。

<!-- source: sqlite-1 62820e765e7f -->
### SQLite

停止服务器，用备份文件替换数据库文件，如果还留有旧的 `-wal` 和 `-shm` 文件，就把它们删除。使用 Docker Compose 时，恢复到卷 `my_data` 中（现有的卷，或另一台机器上的新卷）：

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

服务器镜像以用户 `65532` 运行，该用户必须拥有卷中的文件（即上面的 `chown`）。

<!-- source: postgresql-1 4a3134816771 -->
### PostgreSQL

恢复到一个新的空数据库中，准备方式与新实例相同，但不要应用 schema：转储中已经包含它。

1. 创建角色和数据库。
2. 恢复转储。以超级用户身份恢复时，恢复的表保留其所有者（该角色必须存在）；以数据库所有者身份恢复时，加上 `--no-owner`。

   ```sh
   pg_restore --exit-on-error --dbname="postgres://postgres:...@db.example/my" my-202601010000.dump
   ```

3. 如果使用托管数据库自己的备份，请改用服务商的方式恢复，并把 `database_url` 指向恢复后的实例。

使用 Compose 层 `compose.postgres.yaml` 时，只在新卷上启动数据库，然后恢复到其中。容器已经创建好了角色和数据库：

```sh
cd deploy/docker
docker compose up -d --wait postgres
docker compose exec -T postgres pg_restore --exit-on-error --no-owner -U my -d my < my-202601010000.dump
docker compose up -d
```

如果要覆盖现有数据库进行恢复（回滚升级），请使用 `pg_restore --clean --if-exists`，如[回滚](database.md#rolling-back)所示。

<!-- source: media-1 4292939246df -->
### 媒体

在服务器启动前，把媒体副本解压到 `blob_fs_dir` 中，文件归服务器运行时所用的用户所有。

<!-- source: media-in-a-bucket-1 2e8d8c7703b4 -->
### 存储桶中的媒体

在服务器启动前，把对象恢复到一个空存储桶中，然后将 `blob_s3_bucket`（以及端点、区域和凭据）或 `blob_gcs_bucket` 设置为该存储桶。键必须保持不变：数据库按键来指定每个文件。

- 从另一个存储桶中的副本恢复时，把它复制回来：

  ```sh
  rclone copy --checksum backup:my-media-backup target:my-media-restored
  ```

- 从实例自身存储桶的版本恢复时，把数据库备份时刻的对象复制到一个新存储桶。`rclone` 用 `--s3-version-at` 读取那一时刻的版本：

  ```sh
  rclone copy --s3-version-at "2026-01-01 00:00:00" source:my-media target:my-media-restored
  ```

- 从副本恢复时，把 `blob_s3_bucket` 或 `blob_gcs_bucket` 指向副本，或者把它复制回原账号中的一个存储桶。

服务器的凭据需要能读取、写入和删除恢复后存储桶中的对象（Amazon S3 上为 `s3:GetObject`、`s3:PutObject`、`s3:DeleteObject`
和 `s3:AbortMultipartUpload`；Google Cloud Storage 上为该存储桶的角色 `roles/storage.objectUser`）。

<!-- source: starting-and-checking f656b59cc0ed -->
### 启动与检查

照常启动服务器。使用 Docker Compose 时，`docker compose up -d` 会先运行 `my db upgrade -check`、schema 和 `my db upgrade`；对于同一版本的备份，它们不会做任何更改。然后检查：

1. `/readyz` 返回 `200`，并且服务器日志中没有 `unknown key-encryption key` 错误。
2. `my projection verify` 没有报告差异（[检查和重建投影](database.md#checking-and-rebuilding-projections)）。
3. 你可以登录，并且最新的帖子及其媒体能正常显示。
4. **各 actor 的密钥没有改变**：某个 actor 的 `publicKey.publicKeyPem`
   （`curl -H 'Accept: application/activity+json' https://<host>/ap/actors/<id>`）与恢复前相同。其他服务器用该密钥验证实例的签名，所以密钥不变意味着联邦会从中断的地方继续。
5. 发帖、关注或取消关注能到达另一台服务器（另一个实例上的测试账号能看到）。

备份之后的写入会丢失：此后的帖子、关注和上传在本实例上都不复存在，而其他服务器可能仍保留着它们收到的活动。这在本实例上无法修复；请从你手头最新的备份恢复。

如果使用搜索引擎，恢复后运行 `my search rebuild`，不带 `--keep`，这样比备份更新的帖子会从索引中移除（[全文搜索](search.md#backups)）。

<!-- source: without-the-right-kek 2a5031eadf18 -->
### 没有正确的 KEK 时

- **没有 KEK**：服务器无法启动。`my serve` 以及其他读取配置的 `my` 命令会在此停止，并报
  `The required setting MY_KEK is not set.` 和 `kek: required when key_manager is local`。
- **使用另一个 KEK**：服务器能启动并响应 `/readyz`，但任何读取用旧 KEK 加密的密钥的操作都会失败，报
  `unknown key-encryption key` 以及旧 KEK 的密钥 ID（`local key manager: key "local:…" is not configured`）：向其他服务器的投递和带签名的获取（各 actor 的私钥）、使用双重验证的登录、Webhook 签名，以及外部登录服务商。之前签发的媒体代理 URL 也会失效。密码和帖子没有用 KEK 加密，所以用密码登录和阅读帖子仍然正常，这可能在一开始掩盖问题：请留意日志中的错误。
- 找到错误中所示密钥 ID 对应的 KEK，把它设为 `kek`（或放进 `kek_previous`），然后重启。没有 KEK 就无法恢复密钥；如果 KEK 永久丢失，就必须重新生成各 actor 的密钥，其他服务器会把每个 actor 视为同一账号使用了新密钥。

<!-- source: recovery-drill f9adfe0e0d28 -->
## 恢复演练

备份只有在恢复成功过一次之后，才能确认它是可用的。请在实例公开之前演练一次恢复，并在备份方式发生变化后再次演练（至少每几个月一次）：

1. 按上文所述为正在运行的实例做一次备份：先数据库，后媒体。
2. 在另一台机器上或另一个 Compose 项目中恢复它，使用从你的保存位置取出的 KEK 和配置（不要从正在运行的服务器上复制），并保持 `host` 不变，使各 actor 的 URL 一致。
3. 不要让这个副本参与联邦：让它与互联网隔离，或者在下面的检查通过后打开维护模式。
4. 按照[启动与检查](#starting-and-checking)逐项检查：登录、帖子和媒体，以及将几个 actor 的 `publicKey` 与正在运行的实例比较。
5. 分别在没有 KEK 和使用另一个 KEK 的情况下各启动一次，确认它会像[没有正确的 KEK 时](#without-the-right-kek)所说的那样失败。
6. 记下每一步花费的时间：这就是真正恢复时实例需要停机的时间。
