<!-- source: full-text-search-of-posts fc16f55b56ea -->
# 帖子全文搜索

本页说明帖子全文搜索保存了什么、如何开启和更改它、如何重建索引，以及它对日志和备份有哪些要求。

<!-- source: choosing-an-engine 9cfd51361e0a -->
## 选择引擎

帖子全文搜索需要一个由你自己运行或租用的搜索引擎。由 [`search`](../reference/configuration.md#search) 选择：

- `none`，默认值：没有全文搜索。服务器不连接任何引擎，不排入搜索任务，客户端也不提供搜索帖子的方式。在这样的服务器上，每个 `my search` 命令都会说明这一点，并且什么都不做。
- `elasticsearch`、`opensearch` 或 `meilisearch`：使用对应的引擎，设置见[搜索引擎](search-engines.md)中该引擎的部分。
- `plugin`：你自己的程序，位于 `plugin_search_address`，在任何其他引擎中维护索引，例如 Groonga 或用 Sudachi 构建的索引（[用插件搜索帖子](plugins.md#searching-posts)）。

账号和话题标签的搜索基于数据库，不受此设置影响。

服务器只管理它写入、筛选、排序和分页所用的字段。引擎如何分析文本、如何排列结果、索引有多大，由你通过引擎的设置文件调整；[搜索引擎](search-engines.md)说明了使用内置设置时各引擎在日语中能找到什么，以及如何更改。

<!-- source: what-the-index-holds 5e0884482ea8 -->
## 索引保存的内容

数据库始终是唯一可信的数据来源，索引只是一份可以丢弃并重建的副本。引擎只返回帖子 ID：服务器会从数据库重新读取每条帖子并应用通常的可见范围规则，因此落后的索引可能导致某条帖子搜不到，但不会显示不该看到的帖子。

同时满足以下所有条件的帖子会进入索引：

- 它没有被删除。
- 它的可见范围是 `public` 或 `local`，并且包含在 [`search_visibilities`](instance-settings.md) 中。无论如何设置，`unlisted`、`private` 和 `direct` 帖子都不会被索引。
- 它的作者开启了 `indexable`，该设置在[搜索引擎和 AI 爬虫](crawlers.md)中说明。
- 它的作者以及作者所在的域名，都没有受到使其不被发现的限制。

索引保存每条帖子当前版本的正文、内容警告和替代文本，以及作者和语言。它不保存投票、个人资料或旧版本。

`indexable_default` 默认为 `false`，因此刚开启搜索的服务器，在用户自己选择 `indexable` 之前，能搜到的内容很少。

<!-- source: turning-it-on 3d88170680a2 -->
## 开启

1. 运行引擎，并设置 `search` 及其后端的设置。
2. 如果你单独运行 worker，确定[它们的布局](#keeping-search-from-delaying-other-jobs)。
3. 配置[你的日志](#the-query-appears-in-urls)。
4. 如果引擎由他人运行，把它加入你的[隐私文档](#your-privacy-documents)。
5. 重启服务器，此后新帖子会进入索引。
6. 运行 `my search rebuild` 加入现有的帖子。

重建期间搜索可以使用，但在重建结束之前结果不完整。

<!-- source: rebuilding-the-index 8a8ea41dfc98 -->
## 重建索引

```sh
my search rebuild              # empty the index, then add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second
my search status               # the progress and the failures
my search clear                # empty the index and end any rebuild
```

重建一开始，命令就会返回，其余工作由 worker 以 `search.rebuild` 任务完成（[任务参考](../reference/jobs.md#full-text-search)），因此请保持 worker 运行。每个任务处理 100 条帖子。

- **速率。** 默认每秒 20 条，最多 1000 条，计入查看过的每条 `public` 和 `local` 帖子，包括不符合条件的帖子。按默认速率，一百万条帖子大约需要 14 小时。这为其他任务和数据库留出了余量，因此只有在服务器资源充裕时才提高它。
- **运行哪一种。** 默认方式会移除所有不该在索引中的内容：例如删除未送达而遗留的文档，或从备份恢复数据库后遗留的文档。在它结束之前，搜索能找到的内容会变少。[添加可见范围](#changing-search_visibilities)之后，如果不需要移除任何内容并且希望搜索保持完整，请使用 `--keep`。`--keep` 永远不会移除已不在数据库中的帖子的文档。
- **中断了？** 再次运行同一个命令。它会从最后处理的帖子继续，不会再次清空索引。加上 `--rate` 可以更改速率。worker 重启时，重建也会自动继续。
- **另一种模式。** 重建未完成期间，另一种模式的命令会被拒绝，因为它不会按其所说的那样执行。要以任一模式从头开始，请先运行 `my search clear`，再运行你需要的命令。
- **运行两次**得到的索引相同。重建期间创建、编辑或删除的帖子最终也会是正确的，因为它们自己的任务仍在运行，而每个任务读取的都是帖子当时的状态。

`my search status` 显示后端、`search_visibilities`，以及最近一次重建的阶段（`clearing`、`filling` 或 `finished`）、已处理和失败的帖子数、速率和最后一个错误。

出现问题时：

- **引擎宕机。** 如果连续三条帖子失败，任务会停在原处，记录错误并重试。用完重试次数的重建会留下一个失败的任务（`my job list`），状态中的 `updated` 也不再变化。引擎恢复后，运行同一个命令继续。
- **引擎拒绝某条帖子。** 该帖子会被记为失败，重建继续进行。状态最多列出其中 20 条。修复原因后，运行 `my search rebuild --keep` 再次尝试所有帖子。

<!-- source: changing-search_visibilities 4adf54d02fe9 -->
## 更改 `search_visibilities`

去掉某个可见范围，会自动从索引中移除其帖子。添加某个可见范围，在你运行 `my search rebuild --keep` 之前只能找到新帖子。

**添加 `local` 之前，请先告知你的用户。** 此后，`indexable` 会覆盖所有已开启它的人的 `local` 帖子，重建也会加入他们以前的 `local` 帖子，而不会再次征求他们的同意。个人资料设置中写明了 `indexable` 在你的服务器上覆盖哪些内容，但很早以前就选择了它的人，除非你告诉他们，否则不会看到这一点。

<!-- source: switching-off-or-changing-the-engine 538a78a629e3 -->
## 关闭或更换引擎

当服务器改为 `search = none` 或换用其他引擎时，它会失去与旧索引的连接。旧索引不再被清理，因此此后删除的帖子仍会留在其中，包括其文本。

1. 停止服务器及其 worker，确保下一步之后没有任何程序写入索引。
2. 运行 `my search clear`。
3. 更改 `search`，然后启动服务器。
4. 如果使用新引擎，运行 `my search rebuild`。

如果你已经切换，或者无法停止服务器，请在引擎本身中删除旧索引。设置更改之后，`my search clear` 就无法访问它了。改为 `none` 之后，更改之前排入的任务不会做任何事。

**从 `none` 恢复。** 运行不带 `--keep` 的 `my search rebuild`：服务器没有记录在没有引擎期间删除了什么，只有清空索引才能移除这些帖子。如果离开时有未完成的重建，请先运行 `my search clear`，因为继续执行的重建不会清空索引。

<!-- source: keeping-search-from-delaying-other-jobs c01b5c259acf -->
## 避免搜索拖慢其他任务

默认情况下，一个 worker 一次运行一个任务，因此即使每次调用引擎都有 10 秒的限制，缓慢的引擎仍会与 `federation.inbox` 分享该 worker 的时间。繁忙的服务器应把它们分开：

- 运行一个 `worker_job_types = search.*` 的 worker 进程，其他进程使用 `worker_job_types_except = search.*`（[按任务类型拆分 worker](running.md#splitting-workers-by-job-type)）。
- 使用 `queue = cloudtasks` 时，用[队列路由](running.md#routing-job-types-to-their-own-queues)把 `search.*` 类型发送到它们自己的队列。

小型服务器可以让它们待在一起，这时搜索任务与联邦共享时间。

<!-- source: the-query-appears-in-urls 488e2c1e91dc -->
## 查询会出现在 URL 中

`GET /api/v1/statuses/search` 在其 URL 中以 `?q=` 携带用户搜索的内容。服务器自己的 `http request` 日志行记录的是路径和路由，而不是查询字符串，跟踪和错误报告也会将其省略。不过，位于服务器前面的组件可能会记录它，而搜索内容可能透露一个人的很多信息。请检查你的部署中的每一层，要么让查询不进入其日志，要么把该日志当作个人数据处理，并相应地管理访问权限和保留期。

| 层 | 是否记录查询？ | 怎么做 |
| --- | --- | --- |
| Cloud Run | 是。它的请求日志会自动创建，`httpRequest.requestUrl` 包含查询。 | 按下文排除这些日志。 |
| Google Cloud 负载均衡器 | 只有在为后端服务开启日志记录时才记录。新建的后端服务默认关闭，使用相同的字段。 | 保持关闭，或按下文排除这些日志。 |
| AWS Application Load Balancer | 只有在开启访问日志时才记录，默认关闭。请求行包含客户端发送的原始 URL。它无法删除一行中的部分内容。 | 保持关闭，或为其设置较短的生命周期规则并限制访问。 |
| CloudFront | 只有在开启标准日志记录时才记录，位于字段 `cs-uri-query` 中。 | 使用标准日志记录 v2 时，选择字段时不要选该字段。 |
| ECS | 没有自己的请求日志：其日志只包含服务器写入的内容。 | 无需操作。 |
| Cloudflare | 只有在使用 Logpush 时才记录，位于字段 `ClientRequestURI` 中。 | 改为选择 `ClientRequestPath`。 |
| Caddy | 只有在使用 `log` 指令时才记录，`compose.caddy.yaml` 中没有该指令。 | 如果添加了该指令，按下文删除该参数。 |
| 其他反向代理 | 通常会：它们的默认格式会记录整个请求行。 | 对此路由只记录不含查询的路径，或关闭其访问日志。 |

在 Google Cloud 上，以下命令让这两种日志都不进入项目的 `_Default` 存储桶。请在你自己的每个接收器中添加同样的排除项：

```sh
gcloud logging sinks update _Default \
  --add-exclusion='name=my-search-queries,filter=httpRequest.requestUrl:"/api/v1/statuses/search"'
```

在 Caddy 中：

```caddyfile
social.example {
  log {
    format filter {
      request>uri query {
        delete q
      }
    }
  }
  reverse_proxy my:8080
}
```

<!-- source: backups cb3b40906192 -->
## 备份

索引保存着帖子的文本，因此需要与数据库同样的谨慎对待：谁能访问它、它保存在哪里、保存多久。

需要留意的是引擎的快照。从索引中删除的帖子，会留在之前拍摄的每个快照中，这与数据库备份的情况相同，因此请让快照的保留期与你的[数据库备份](backup.md)相同。

恢复时不需要快照：`my search rebuild` 会根据数据库重新生成索引。从备份恢复数据库之后，请运行不带 `--keep` 的该命令，让比备份更新的帖子离开索引。

<!-- source: your-privacy-documents 660de0e2a0c4 -->
## 你的隐私文档

如果引擎运行在其他公司的服务上，该公司会收到 `search_visibilities` 所覆盖的帖子的文本，如果你包含了 `local` 帖子，也包括它们。请在[隐私政策](privacy-policy-template.md)和[处理活动记录](processing-record-template.md)中把它列为接收方。你自己运行的引擎不是新的接收方。

如果你保留了[包含查询的日志](#the-query-appears-in-urls)，也请说明你对它们的决定。

<!-- source: closing-the-service 343ec69df255 -->
## 关闭服务

服务器在关闭时不会自行删除数据，因此请在丢弃数据库和媒体的同时，丢弃索引和引擎的快照（[关闭服务](service-closure.md#7-discarding-the-db-blobs-and-backups)）。
