<!-- source: supporter-payments-with-stripe f12ff4a0af71 -->
# 通过 Stripe 收取支持者付款

使用 `payments = stripe` 时，本站用户在 Stripe 托管的支付页面（Checkout）上订阅按月或按年的方案，方案对应的角色会随付款状态的变化而授予和移除。部署所用的配置项见[配置参考](../reference/configuration.md#payments)。关于整个计划（方案、角色、成员列表），参见 [supporters.md](supporters.md)。管理员在管理后台中启用该计划并创建方案。这个设置只是让服务器能够与 Stripe 通信。

一台服务器只能使用一个支付服务商。以后如果要切换到其他服务商，需要请现有的订阅者用新的服务商重新订阅。

<!-- source: what-the-server-does-at-stripe ca3e0d4f59f3 -->
## 服务器在 Stripe 上做什么

[订阅的工作方式](supporters.md#how-a-subscription-works)用一张图展示了支付页面、webhook、对账、方案的角色以及收入之间的关系。

- **方案和价格**：服务器通过 API 为每个方案创建一个 Product（ID 为 `my_plan_<plan ID>`，带有 `metadata.my_plan_id`），并为每个周期和币种各创建一个 Price（`lookup_key` 为 `my_<plan ID>_<month|year>_<currency>`）。Stripe 的 Price 无法修改金额，因此修改金额会创建一个新的 Price 并归档旧的。使用旧 Price 的订阅按旧金额续订。按年付款的折扣体现在年付 Price 的金额中；不使用优惠券。不要在控制台中编辑或删除这些 Product 和 Price。方案可以使用 [Stripe 最低收费金额列表](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts)中的币种，且其月付和年付金额都必须达到该最低金额：USD、AED、ARS、AUD、BRL、CAD、CHF、COP、CZK、DKK、EUR、GBP、HKD、HUF、IDR、ILS、INR、JPY、KRW、MXN、MYR、NOK、NZD、PHP、PLN、RON、RUB、SEK、SGD、THB 和 ZAR。服务器会拒绝其他任何币种。该列表适用于以账号结算币种收取的款项。需要 Stripe 换算的收费，在换算后必须达到结算币种的最低金额，而服务器不会检查这一点，因此当方案的币种与你的银行账户不同时，请把金额设得远高于最低金额。
- **订阅**：服务器创建一个 `mode=subscription` 的 Checkout 会话，并把用户的浏览器引导到其 URL（`checkout.stripe.com`）。支付页面一小时后过期。服务器的订阅 ID 会写入 `client_reference_id`、`metadata.my_subscription_id`，以及 Stripe 订阅的 `metadata.my_subscription_id`。不启用自动计税（Stripe Tax）。
- **重新开始**：一个待处理的订阅最多只有一个页面可以收款。用户在同一方案和周期上重新开始时，会回到那个仍然打开的页面，页面会再次以空白状态打开，除非它剩余时间不足 15 分钟。其他任何开始都会得到一个新页面，服务器会让先前的页面过期，之后那个页面会显示 Stripe 的过期提示。如果先前的页面已经付款，用户就不会得到页面：他们会被带到返回页面，看到该订阅。如果服务器无法让先前的页面过期，这次开始就会失败，用户可以再试一次。待处理的订阅关闭时（其方案被停用、其账号被删除），也会以同样的方式让它的页面过期。
- **第二个订阅**：两笔付款仍有可能同时发生，即两个页面在同一时刻提交。先到达服务器的订阅归用户所有。服务器会立即取消另一个订阅，并全额退还它所有已付的账单（包括续订），因为它没有为任何周期提供任何东西，退款原因为 `duplicate`，并记录日志 `a second subscription was canceled and refunded`。对已关闭的待处理订阅的付款也按同样方式处理，即使该付款恰好在关闭之前到达服务器。用户会收到 Stripe 的退款，并只看到一个订阅。Stripe 不会退还已退款收费的手续费，该手续费会体现在收入中。处于争议中的收费不会退款，服务器查看时尚未结算的付款（例如借记方式）也不会。已经有人在控制台中取消的订阅会保持原样，连同其收费，并且永远不会成为用户的订阅。**从不具备这一功能的版本升级服务器之前**，请在 Stripe 中查找拥有两个 `metadata.my_subscription_id` 相同的订阅的客户。第一次对账会取消未在使用的那个，并退还它收取的全部款项。如果你更愿意手动处理其中某些订阅，请在控制台中取消它们，服务器就不会再处理它们。
- **客户门户**：首次使用时，服务器通过 API 创建一个门户配置（`metadata.my=1`），只启用更新支付方式、账单历史和在周期结束时取消。方案变更在本服务器的页面上进行，而不是在门户中。在门户中取消，会在本服务器上显示为计划取消，用户可以在服务器的页面上撤销。在撤销之前，方案变更会被拒绝。
- **手动设置的取消日期**：门户设置的是订阅的 `cancel_at`，而不是 `cancel_at_period_end`，服务器两者都会读取。你在控制台中或通过 API 设置的 `cancel_at`，只要不晚于当前周期的结束，也同样算数。如果它早于周期结束，服务器会把该日期显示为订阅的结束日期，用户在那之前保留角色。更晚的 `cancel_at` 还不算取消：订阅会一直保持有效，直到包含该日期的那个周期。
- **方案变更**：升级以及从按月改为按年，会立即按比例收取差额（`proration_behavior=always_invoice`），并且只在该账单付款后才切换（`payment_behavior=pending_if_incomplete`）。降级以及从按年改为按月，通过 Subscription Schedule 在周期结束时切换。
- **方案变更失败**：变更的付款失败或卡片要求验证时，服务器会作废该账单，从而放弃这次变更，用户会看到错误。订阅保持原方案且有效，没有宽限期，也没有付款失败的通知。用户可以在门户中更新卡片后重试，只会被收费一次。如果付款仍在处理中，服务器不会干预，付款成功后方案就会切换。续订不受影响：续订失败会按下文所述进入宽限期。
- **回读状态**：服务器把 webhook 视为一个信号，并从 Stripe API 回读订阅、Checkout 会话和资金变动来决定状态。此外，每隔 `payments_reconcile_interval`（默认 6 小时）会进行一次对账，读取事件列表、未完成的 Checkout 会话、余额交易，并全面扫描所有订阅，修复服务器漏掉的任何 webhook。
- **收入**：服务器记录每笔余额交易的总额、手续费和净额。Billing 的 0.7% 等手续费以单独的 `stripe_fee` 交易出现，并从收入中扣除。`stripe_fee` 涵盖整个账号的手续费，因此**请使用专供本服务器使用的 Stripe 账号**。在同时用于其他销售的账号上，那些手续费也会从收入中扣除。
- **日期**：收入日期按 Stripe 账号的时区计算（控制台中“Settings > Business > Account details”下的时区，API 中的 `settings.dashboard.timezone`）。无法读取时，服务器按 UTC 计算，并每小时重新读取一次。

每个服务器进程每秒最多发送 20 个请求（测试模式下为 10 个）。[Stripe 的限制](https://docs.stripe.com/rate-limits)按账号计算：live 模式下每秒 100 个请求，沙盒中 25 个，每个端点每秒 25 个。同一服务器的各个进程共享这些限额，使用该账号的其他任何程序也一样。Stripe 返回 `429` 时，如果 `Retry-After` 指定的时长不超过 30 秒，服务器就等待该时长（没有该头时等待 1 秒），并最多重试 2 次；超出后交给任务重试。

Stripe 还按账号分配读取请求：30 天内平均每笔交易 500 次，且无论交易数量多少，每月至少 10,000 次。在默认间隔下，小型服务器上的对账远低于这个限额。即使对账没有发现任何问题，也仍会读取几页，因此在交易很少的账号上，最短的 15 分钟间隔（30 天内运行 2,880 次）会接近 10,000 次。Stripe 限制每个订阅每分钟最多 10 张、每天最多 20 张新账单，而立即收费的方案变更会产生一张新账单。

<!-- source: card-data-and-pci-dss f45a3c7d1846 -->
## 卡片数据与 PCI DSS

支付方式只在 Stripe 的 Checkout 和客户门户中输入，本服务器的 Web 客户端既不加载 Stripe 的脚本，也不加载 iframe。服务器只保存 Stripe 对象 ID（`cus_`、`sub_`、`price_`、`prod_`、`cs_`、`txn_`）、金额、周期和状态。它不保存卡号、卡号后四位、有效期或账单地址。Webhook 正文和 Stripe 的响应既不保存在日志中，也不保存在数据库中。因此，运营者的 PCI DSS 范围预计相当于使用托管支付页面的商户（SAQ A）。请在 Stripe 控制台的指引（“Settings > Compliance”）中确认适用的范围和自我评估问卷的类型。

<!-- source: 1-prepare-the-stripe-account 06eefed5779b -->
## 1. 准备 Stripe 账号

1. 创建一个 Stripe 账号，并在控制台中填写你的商业信息。如果你在日本销售，在“Settings > Business > Public information > Notice based on the Act on Specified Commercial Transactions”下登记你的商业信息披露页面的 URL，Checkout 下方就会出现一个链接。
2. 在“Settings > Business > Account details”下检查时区。收入日期按该时区计算。
3. 在“Settings > Billing > Subscriptions and emails > Manage failed payments”下，选择重试行为（Smart Retries）以及重试用完后的处理方式。移除角色前的宽限期（从首次失败起 7 天）由本服务器计算，因此 Stripe 的重试设置不会影响它。重试用完后，建议选择“Cancel the subscription”。
4. 要向订阅者发送收据，请在“Settings > Customer emails”下启用付款成功收据。

<!-- source: 2-create-an-api-key 95e2c858acca -->
## 2. 创建 API 密钥

建议在“Developers > API keys”下创建一个**受限密钥**（`rk_test_...` / `rk_live_...`）。私密密钥（`sk_test_...` / `sk_live_...`）也可以使用。密钥的前缀决定模式，测试密钥不会产生任何收费。可发布密钥（`pk_`）无法使用。

请给受限密钥以下权限。已在沙盒中用具有本表权限的受限密钥运行过订阅、方案变更、取消、对账和作废账单。退款、让页面过期和争议还没有用它运行过，也没有人确认过本表是能够工作的最小权限集合。

| 资源 | 权限 | 使用的 API |
| --- | --- | --- |
| Products | 写入 | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | 写入 | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | 写入 | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}`, `POST /v1/checkout/sessions/{id}/expire` |
| Customer portal | 写入 | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | 写入 | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules（Subscriptions 的一部分） | 写入 | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | 写入 | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments`, `POST /v1/invoices/{id}/void` |
| Charges, Refunds | 写入 | `GET /v1/charges/{id}`, `GET /v1/refunds`, `POST /v1/refunds` |
| Disputes | 读取 | 通过余额交易和收费读取 |
| Balance transaction sources | 读取 | `GET /v1/balance_transactions` |
| Events | 读取 | `GET /v1/events` |
| Account（Connect 之外的账号信息） | 读取 | `GET /v1/account`（时区） |

没有写入退款的权限时，服务器仍会取消第二个订阅，而 `duplicate_charge` 会一直留在警告中（参见[注册 webhook 端点](#3-register-the-webhook-endpoint)），直到能够退款为止。在这项权限被列出之前创建的密钥需要补上它。

通过环境变量 `MY_PAYMENTS_STRIPE_SECRET_KEY`（或在 `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE` 中指定文件路径）传递密钥。它不能写在配置文件中。如果密钥泄露，请在控制台中轮换密钥，并用新密钥重新部署。

<!-- source: 3-register-the-webhook-endpoint 1cbc2cf9ace3 -->
## 3. 注册 webhook 端点

在“Developers > Webhooks > Add endpoint”下注册以下内容。服务器不会通过 API 创建端点。

- URL：`https://<server domain>/payments/webhooks/stripe`
- 事件来源：“Your account”（不是 Connected accounts）
- 载荷样式：snapshot。服务器只读取正文中的 `id`、`type`、`livemode`、`created`、`data.object.id` 和 `data.object.object`，因此端点的 API 版本无关紧要。
- 要发送的事件（16 种）：

  - `checkout.session.completed`, `checkout.session.expired`
  - `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `customer.subscription.paused`, `customer.subscription.resumed`
  - `invoice.paid`, `invoice.payment_failed`
  - `charge.refunded`, `refund.created`, `refund.updated`
  - `charge.dispute.created`, `charge.dispute.closed`, `charge.dispute.funds_withdrawn`, `charge.dispute.funds_reinstated`

通过环境变量 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`（或 `_FILE`）传递端点的签名密钥（`whsec_...`）。

端点会验证签名（`Stripe-Signature` 中的 HMAC-SHA256）和时间戳（前后 5 分钟以内），并以 `400` 拒绝用 live 密钥收到的测试事件（反之亦然）。它按 ID 对每个事件只记录一次，把一个任务放入队列，并返回 `2xx`。无法写入数据库时，它返回 `503`，Stripe 会再次发送该事件。正文限制为 1 MiB。轮换签名密钥期间，Stripe 会同时用新旧两个密钥签名，因此只需把新密钥写入配置即可。

支付服务商的状态（模式、最后一个 webhook 到达的时间，以及最后一次对账的结果和失败种类）显示在管理后台 `/admin/supporters` 的“支付服务商”部分，也可以通过管理 API `GET /api/v1/admin/payment_status`（权限 `manage_supporters`）查看。该部分会说明如何修复每种对账失败。`warnings` 会列出 `no_webhook`（从未记录过任何 webhook）和 `reconciliation_corrected`（最后一次对账修复了某个订阅）。出现其中任何一个时，都应怀疑 webhook 配置有误。它还会列出 `duplicate_charge`：第二个订阅尚未被取消和退款。参见[运营](#7-operation)。该部分中的“立即对账”（`POST /api/v1/admin/payment_reconciliation/runs`）会立即对账，而不必等待计划的运行。它每个 UTC 小时最多运行一次（同一小时内的第二次调用返回 `429`），审计日志会记录 `payment_reconcile_queued`。

<!-- source: 4-configure-the-server 7b3f7d01839f -->
## 4. 配置服务器

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` 会验证配置。用构建标签 `no_payments_stripe` 构建的二进制文件会拒绝 `payments = stripe`（参见 [build-tags.md](../reference/build-tags.md)）。

<!-- source: 5-try-it-in-test-mode 94db104964c3 -->
## 5. 在测试模式下试用

先使用测试密钥（`rk_test_` / `sk_test_`，或沙盒密钥）。测试模式不会产生任何收费。测试密钥和 live 密钥各自拥有独立的 Product、Price、客户、订阅和 webhook 端点。要正式上线，请在 live 模式下重新注册端点，把密钥和签名密钥替换为 live 的，然后重新部署。方案的 Price 会在 live 账号中重新创建。

测试卡（[Stripe 文档](https://docs.stripe.com/testing)）：

| 卡号 | 行为 |
| --- | --- |
| `4242 4242 4242 4242` | 成功 |
| `4000 0000 0000 0341` | 可以绑定，但收费失败（用于检查付款失败和宽限期的开始） |
| `4000 0000 0000 9995` | 因余额不足而失败 |
| `4000 0000 0000 0259` | 付款后会产生争议 |

有效期填写任意未来日期，安全码填写任意 3 位数字。

要试用续订和付款失败，请使用[测试时钟](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage)。由 Checkout 创建的客户在创建时没有时钟，因此请先订阅，然后通过 API 为该客户创建时钟，即向 `POST /v1/test_helpers/test_clocks` 传入 `customer`（API 版本 `2026-05-27.dahlia` 或更高）。只有在 `frozen_time` 不在过去、账号没有配置 Automations、且客户最多有三个订阅时，Stripe 才允许这样做。之后客户无法脱离该时钟，删除时钟会删除该客户及其订阅。在控制台的“Billing > Test clocks”下推进时间，即可触发续订收费和失败。

时钟无法展示所有情况：

- **宽限期结束**：服务器把真实时间与 Stripe 报告的失败时间（`finalized_at`）进行比较，而后者是模拟的，领先于真实时间。订阅会进入宽限期，进一步推进时钟也不会让它结束。角色会一直保留，直到真实时间超过模拟失败时间之后 7 天，或者在账号设置为如此时，直到 Stripe 在重试用完后取消订阅。
- **全面扫描所有订阅**：Stripe 的订阅列表会排除使用时钟的订阅，除非请求中指定了时钟或客户，而服务器的扫描两者都不指定。只有 webhook 和事件列表会把它们的变化带给服务器。

<!-- source: 6-try-it-locally-stripe-cli 8a3e87b02ce0 -->
## 6. 在本地试用（Stripe CLI）

[Stripe CLI](https://docs.stripe.com/stripe-cli)（Apache-2.0）会把 Stripe 事件转发到你本机上的服务器。不需要公开的 URL。

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

把 `stripe listen` 输出的 `whsec_...` 填入 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`，然后启动服务器（端口要与你的 `listen` 设置一致）。

使用 Docker Compose 时，在 `deploy/docker/.env` 中设置 `MY_PAYMENTS=stripe`、`MY_PAYMENTS_STRIPE_SECRET_KEY` 和 `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET`，然后运行 `docker compose up -d`。

你可以在另一个终端中触发单个事件。

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

`stripe trigger` 创建的对象不带本服务器的订阅 ID，因此不会有任何订阅改变状态（服务器会回读它们，并忽略不属于自己订阅的内容）。要检查完整的订阅流程，请在 Web 客户端中订阅，并用测试卡付款。

<!-- source: 7-operation f70cd2967b9f -->
## 7. 运营

- 种类为 `permission` 的对账失败意味着密钥错误或缺少权限。服务器的日志行（`payment reconciliation failed`）中包含 Stripe 的消息，其中会指出需要添加的权限。
- 对账经常触及请求限制时，请提高 `payments_reconcile_max_requests` 或延长 `payments_reconcile_interval`。每次读取都计入 Stripe 的读取配额（参见[服务器在 Stripe 上做什么](#what-the-server-does-at-stripe)），因此在交易很少的账号上，优先选择更长的间隔。
- 对账停止超过 30 天时，服务器无法从 Stripe 的事件列表（只能回溯 30 天）中追上进度。这时由全面扫描所有订阅来修复状态。
- 警告中的 `duplicate_charge` 表示有用户付了两次款，而服务器未能完成对第二个订阅的取消和退款。日志行 `a second subscription could not be canceled and refunded` 会列出两个订阅和失败的种类，并在每次对账时再次出现，因为每次对账都会重试。如果种类是 `permission`，请添加写入退款的权限。否则请在控制台中退还该笔收费：下一次重试会发现已没有需要退款的款项，并清除该警告。“立即对账”会立即重试。
- 其他退款和拒付在 Stripe 控制台中处理。全额退款和拒付会立即移除角色并取消订阅，部分退款不会移除角色。赢得争议不会自动恢复角色。

<!-- source: items-to-verify 8960333fa514 -->
## 待验证事项

以下各项尚未在 Stripe 上（无论沙盒还是 live）实际试过，依据的是 Stripe 的文档。

- 使用已归档 Price 的订阅的续订，它应当保持在该 Price 上。
- 实际续订时的 `status_transitions.finalized_at`。服务器把它当作付款首次失败的时间，并从此开始计算宽限期。
- `stripe_fee` 交易能否归属到某个订阅。服务器不做尝试：它把这些手续费从整体收入中扣除。
- 受限密钥能否读取 `GET /v1/account` 的 `settings.dashboard.timezone`。读取失败不会让服务器停止，它会按 UTC 计算，因此正常运行的服务器不会表现出来。请检查收入日期是否落在账号的时区内。
- `GET /v1/invoice_payments` 能否按 `payment[payment_intent]` 筛选，它把一笔付款关联到其订阅。
- 全额退款、部分退款和拒付，从控制台操作到服务器上的状态。
- 全面扫描所有订阅能否修复服务器漏掉其状态的订阅。
- 对于使用待处理更新所不支持的支付方式（ACH、SEPA 和 Bacs 直接借记）付款的订阅，升级会产生什么结果。在确认之前，请在控制台中关闭这些支付方式。
- `cancel_at_period_end=true` 是否会取代位于之后周期的 `cancel_at`。
- 返回仍然打开的页面。服务器会把 `GET /v1/checkout/sessions/{id}` 返回的内容（`line_items`、`success_url`、`cancel_url`、`customer`、`customer_email`）与新请求进行比较，任何差异都会产生一个新页面。如果 Stripe 在保存 `success_url` 或 `customer_email` 时会改写它们，用户每次都会得到一个新页面并丢失已输入的内容，但只会付款一次。
- 在付款完成的同时让会话过期。Stripe 的文档说，不处于打开状态的会话会返回错误，服务器随后会读取该会话。错误的确切代码，以及卡片验证进行中时的状态，文档中都没有说明。
- 一个会话无法从两个标签页付款两次。一个会话对应一个订阅且只会完成一次，但文档没有说明两次在同一时刻提交的情况。
- 支付页面上的验证失败是否会留下一个指向服务器订阅的 `incomplete` 订阅。服务器会关联它并让用户保持待处理状态，而在另一个页面上的付款会作为第二个订阅被退款。
- 使用“Charges”和“Refunds”权限与上表一致的受限密钥时，第二个订阅的退款以及随后的资金变动。
