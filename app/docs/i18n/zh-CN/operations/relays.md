<!-- source: federation-relays bbd5bb98dde3 -->
# 联邦中继

中继是一种服务器，它把加入它的各个服务器的公开帖子转交给其他加入的服务器。即使是关注不多的小实例，联邦时间线和话题标签时间线也能有内容，自己的公开帖子也能传得更远。不加入中继的实例不会因此增加任何数据行或任务。

<!-- source: join-a-relay 147b07e65534 -->
## 加入中继

使用管理区域中的“中继”（`/admin/relays`）。它需要权限 `manage_settings`，内置角色 `admin` 拥有该权限。“加入中继”会打开登记中继的表单。列表显示每个中继的状态，以及退出或重新加入它的按钮；选择一个中继会打开它的页面，显示它的地址、最近 24 小时内带来的内容，以及删除它的按钮。对应的 CLI 是 `my admin relay`（`my admin relay -help`）：

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. 登记中继的 URL。有两种协议：
   - **Mastodon**：中继的 **inbox URL**（例如 `https://relay.example/inbox`）。包括 pub-relay 和 Activity-Relay 在内的许多中继都接受这种方式。
   - **LitePub**：中继的 **actor URL**（例如 `https://relay.example/relay`）。这是 Pleroma 和 Akkoma 中继使用的协议。如果不选择协议（“自动检测”），当该 URL 提供 actor 文档时，服务器使用 LitePub，否则使用 Mastodon。
2. 登记之后，实例 actor 会向中继发送 `Follow`，状态为“等待中继回应”（`pending`）。中继接受后，状态变为“已加入”（`accepted`）；中继拒绝时，状态变为“已被中继拒绝”（`rejected`）。有些中继由运营者逐一手动批准请求，因此需要一段时间。
3. 如果登记时关闭“接收来自中继的帖子”，服务器只发送而不接收。要更改这一点，需要退出该中继后重新登记。

受到域名处置（`suspend`）的服务器，不能登记它的中继。如果已加入的中继所在的服务器后来被暂停，状态不会改变，但不再向该中继发送任何内容，也不再从它接收任何内容，管理区域会显示警告。

<!-- source: what-the-server-sends 1db52c508f9c -->
## 服务器发送的内容

- 只发送本站**公开**（`public`）帖子的创建、编辑和删除。设为 `unlisted`、仅关注者、私信或仅本站（`local`）的帖子，以及转发，都不会发送。要让某条帖子不进入中继，请将它发布为 `unlisted` 或 `local`。没有按用户退出的选项。
- 公开帖子的文档带有 Linked Data Signature（`RsaSignature2017`），因此 Mastodon 和 Misskey 能接受中继原样转交的帖子。
- 投递次数会增加“已加入的中继数 × 公开帖子数”（发往同一中继的投递会合并为一次）。

<!-- source: what-the-server-receives ff962150d1a7 -->
## 服务器接收的内容

- 已加入的中继带来的公开帖子，会先从其来源服务器重新获取并检查，然后才会收录。之后它会出现在联邦时间线和话题标签时间线中。它不会出现在主页时间线中，也不会产生通知。
- 每个中继每小时最多收录 `relay_max_activities_per_hour` 条帖子（默认 1,000）。超出限制的部分不会获取，直接丢弃。管理区域中该中继的页面和 `my admin relay list` 会显示最近 24 小时内收录了多少条、因超出限制丢弃了多少条。如果丢弃很多，请提高限制或重新考虑是否使用该中继。
- 如果帖子的域名或账号受到 `suspend`（联邦限制）或 `limit`（从公开场所移除的限制），该帖子不会被收录。
- 只通过中继到达的帖子，会在实例设置 `relay_status_retention_days`（默认 7）之后从公开视图中移除。本站用户回复、转发、喜欢或加入书签的帖子，以及本站用户关注的作者的帖子，与其他远程帖子的保留期相同（`remote_status_retention_days`）。
- 每收录一条帖子，需要从来源服务器获取一次（作者未知时还要获取一次 actor）。在每小时 1,000 条的限制下，每天最多获取 24,000 次。

这些设置见[配置参考](../reference/configuration.md#relay_max_activities_per_hour)。收录的结果也会以指标 `my.relay.activities` 导出（见[运行服务器](running.md)）。

<!-- source: leave-join-again-and-delete ea12da94f940 -->
## 退出、重新加入和删除

- “退出”会发送对 `Follow` 的 `Undo`，并将状态设为“已退出”（`left`）。此后不再向该中继发送任何内容，也不再从它接收任何内容。已经收录的帖子会在保留期结束时移除。
- “重新加入”会向拒绝了本服务器或本服务器已退出的中继发送新的 `Follow`。
- “删除”会移除已退出或被拒绝的中继的登记。删除之后，已经发送给该中继的帖子的编辑和删除仍会投递给它。

登记、加入、退出和删除，以及中继的接受或拒绝，都会记录在审计日志中，目标类型为“中继”（接受和拒绝记录为远程操作）。
