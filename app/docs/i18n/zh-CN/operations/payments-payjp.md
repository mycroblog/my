<!-- source: supporter-payments-with-payjp 1f782a11b125 -->
# 使用 PAY.JP 收取支持者付款

设置 `payments = payjp` 后，本站用户可以在这台服务器的付款页面订阅按月或按年的方案，方案的角色会随付款状态的变化而授予或移除。
部署时使用的密钥见[配置参考](../reference/configuration.md#payments)。关于整个计划（方案、角色、成员列表），请参阅 [supporters.md](supporters.md)。
启用计划和创建方案由管理员在管理页面中完成。这项设置只是让服务器能够与 PAY.JP 通信。

一台服务器只能使用一个支付服务商。要从 Stripe 切换到 PAY.JP（或反过来），需要请现有订阅者使用新的支付服务商重新订阅。

<!-- source: what-the-server-does-at-payjp a8abbf012e10 -->
## 服务器在 PAY.JP 上做什么

PAY.JP 有 API v1 和 v2。v2 还没有定期扣款 API，因此服务器使用 **API v1** 的定期扣款（Plan 和 Subscription）。v1 既没有托管付款页面，也没有客户门户，因此由这台服务器提供相应的页面来代替。

[订阅的运作方式](supporters.md#how-a-subscription-works)用一张图说明了付款页面、webhook、对账、方案的角色和收入之间的关系。

- **Plan**：服务器为每个价格创建一个 Plan，其 ID 为 `my_<plan ID>_<month|year>_<amount>`（`metadata.my_plan_id`）。PAY.JP 的 Plan 无法更改金额，因此更改金额时会创建新的 Plan，旧的 Plan 不会删除，而是加上 `metadata.my_archived=1`。旧 Plan 上的订阅按原金额续订。
  不要在 PAY.JP 的管理界面中编辑或删除这些 Plan。唯一支持的货币是日元，金额为 50 到 9,999,999 日元。
- **订阅**：服务器把用户的浏览器引导到自己的付款页面（`/payments/pages/payjp/checkout`）。该页面显示 payjp.js 的银行卡输入框（PAY.JP 的 iframe），并在创建令牌时进行 3-D Secure 认证（iframe 方式）。
  服务器只接受已完成 3-D Secure 的令牌（`three_d_secure_status` 为 `verified` 或 `attempted`），将该卡作为默认卡添加到客户（首次使用 ID `my_<the server's subscription ID>`），并创建一个在 `metadata.my_subscription_id` 中带有服务器订阅 ID 的订阅。首次扣款在创建订阅时进行。
  页面 URL 的有效期很短，并带有服务器签名的状态（自创建起 1 到 2 小时内有效）。
  用户离开页面后重新开始时，无论是同一方案还是其他方案，都会为当前要订阅的方案获得一个新 URL。之前的 URL 在过期前仍可使用，最先完成付款的页面会生成唯一的订阅，其他页面则会转到这个订阅。如果之前的某个页面已经付款，用户会转到返回界面，不会获得新的 URL。
  已签名的 URL 无法撤回，因此在待处理的订阅关闭后（其方案已停用、账号已删除），页面仍然可以付款。在这样的页面上付款不会买到任何东西：服务器会删除该订阅并全额退还其扣款（PAY.JP 在售出后 180 天内接受退款）。
  API v1 不接受幂等键，因此在页面的一次提交创建订阅期间，服务器会在数据库中把这次结账标记为进行中：第二次提交（双击、另一个标签页，或位于同一负载均衡器后的另一个服务器进程）最多等待 30 秒，然后转到第一次提交创建的订阅。如果持有标记的进程已停止，标记会在 5 分钟后失效。
- **银行卡与付款记录页面**（代替 Stripe 的客户门户）：订阅者在自己的订阅界面打开 `/payments/pages/payjp/portal`，更换银行卡或查看最近的付款（URL 在 1 小时后过期）。表单开头显示 PAY.JP 客户的电子邮件地址，即首次付款时输入的地址，而不是之后更改的账号地址。
  更换银行卡会恢复因付款失败而暂停的订阅，并立即扣款。取消和更改方案在这台服务器的订阅界面中进行。
- **付款失败**：续订扣款失败时，PAY.JP 会暂停（`paused`）该订阅，并且**不会重试**。服务器把暂停视为逾期付款，从暂停时起保留角色 7 天的宽限期。如果订阅者在此期间更换了银行卡，订阅会恢复；否则 7 天后移除角色（在 PAY.JP 一侧，订阅保持暂停，不会扣款）。
- **更改方案**：升级时，会把当前周期剩余部分按比例计算的差额作为一次性扣款收取（低于 50 日元不扣款），替换方案，并在周期结束时收取新方案的全额（周期的起点不变）。如果差额扣款被拒绝，方案不会更改。
  降级以及从按年改为按月，会通过 `next_cycle_plan` 在周期结束时切换。
- **回读状态**：服务器把 webhook 当作信号，从 PAY.JP API 回读订阅和资金变动来确定状态。
  此外，每隔 `payments_reconcile_interval` 进行一次的对账会读取事件列表、资金变动并遍历所有订阅，修复服务器漏收的 webhook。
- **收入**：服务器为每笔付款记录总额、手续费（根据扣款的 `fee_rate` 计算，不足 1 日元的部分舍去）和净额。退款和拒付会把退还的手续费记为负的手续费。
  当一个结算期结束并出具结算单时，结算单上的手续费与服务器计算结果之间的差额会记为一笔手续费变动（其 ID 为结算单的 ID），使该期的手续费合计与结算单一致。
  结算单涵盖整个账户的手续费，因此**请使用专供这台服务器使用的 PAY.JP 账户**。
- **日期**：收入的日期按日本时间计算。

服务器每秒最多发送 20 个请求（测试模式下为 2 个）。这在 PAY.JP 的速率限制之内（使用私钥的请求，正式模式下每秒 30 个，测试模式下每秒 2 个）。收到 `429` 时，服务器先等待 1 秒，之后每次加倍，最多重试 3 次。
在测试模式下，单次对账很慢（达到 1,000 个请求的上限约需 8 分钟）。后续任务会接着处理，无需任何操作。

<!-- source: card-data-and-pci-dss e7247e5fea0f -->
## 银行卡数据与 PCI DSS

卡号、有效期和安全码只在 payjp.js 显示的 PAY.JP iframe 中输入。这台服务器的页面和脚本从不接触它们。服务器只接收 payjp.js 创建的令牌 ID，以及 3-D Secure 所需的持卡人姓名和电子邮件地址。
服务器只保存 PAY.JP 的对象 ID（`cus_`、`sub_`、`ch_`、Plan ID、事件 ID）、金额、周期和状态。它不保存卡号、卡号后四位或有效期（银行卡页面只是从 PAY.JP 读取卡号后四位、有效期和客户的电子邮件地址并显示出来）。webhook 的正文和 PAY.JP 的响应既不保存在日志中，也不保存在数据库中。

付款页面由这台服务器的源提供，因此在构建时考虑了 PCI DSS v4 对付款页面脚本的管理（要求 6.4.3）和篡改检测（要求 11.6.1）。

- 页面只加载 `https://js.pay.jp/v2/pay.js` 和一段嵌入在页面中、带有 nonce 的小脚本。页面既不包含 Web 客户端的打包文件，也不包含任何分析或广告脚本。
- 页面始终带有自己的 Content-Security-Policy（`script-src` 只有 nonce 和 `https://js.pay.jp`，`frame-src` 只有 `https://js.pay.jp` 和 `https://api.pay.jp`，`form-action 'self'`，`frame-ancestors 'none'`）、`Referrer-Policy: no-referrer`、`Cache-Control: no-store` 和 `X-Content-Type-Options: nosniff`。没有任何设置可以更改 CSP。
- 模板和脚本嵌入在服务器的二进制文件中，运行时无法更改。只接受来自同源的表单提交。

因此，运营者的适用范围预计是把付款输入交给 PAY.JP iframe 的商户（SAQ A）。请根据 PAY.JP 的指引和各卡组织的要求，确认适用的范围和自我评估问卷的类型。
确认时，记录以下几点会有帮助：付款页面的 URL（`/payments/pages/payjp/checkout` 和 `/portal`）、页面加载的脚本列表（上述两个）、页面的 CSP，以及到服务器的通信使用 TLS（反向代理的配置）。

<!-- source: 1-prepare-the-payjp-account b01b53a61e40 -->
## 1. 准备 PAY.JP 账户

1. 创建 PAY.JP 账户，并从测试模式开始。在正式模式下扣款需要提交正式使用申请并通过审核。
2. 在 PAY.JP 控制台中查看“API settings > 3-D Secure”。服务器每次创建令牌时都会要求 3-D Secure，因此你也可以选择对所有令牌创建都要求 3-D Secure。
3. 商业交易信息披露页面在这台服务器的管理页面中设置（参见 [supporters.md](supporters.md)）。

<!-- source: 2-keys-and-the-webhook-token 7bee0ef9256a -->
## 2. 密钥与 webhook 令牌

在 PAY.JP 控制台的“API”下，查看以下两个密钥。密钥的前缀决定模式，测试密钥不会产生任何扣款。两个密钥必须处于同一模式。

| 密钥 | 示例 | 用途 |
| --- | --- | --- |
| 私钥 | `sk_test_...` / `sk_live_...` | 服务器用它调用 API。它也是签名页面 URL 的密钥的来源 |
| 公钥 | `pk_test_...` / `pk_live_...` | 付款页面和银行卡页面上的 payjp.js |

通过环境变量 `MY_PAYMENTS_PAYJP_SECRET_KEY` 传入私钥（或在 `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE` 中传入文件路径）。私钥不能写在配置文件中。
如果私钥泄露，请在控制台中重新签发，并使用新密钥重新部署。重新部署前已打开的付款页面和银行卡页面的 URL 将失效。

<!-- source: 3-register-the-webhook 1afaf50fdaef -->
## 3. 注册 webhook

在 PAY.JP 控制台的“Settings > Webhook”下添加一个接收地址。测试模式和正式模式需要分别注册。

- URL：`https://<server domain>/payments/webhooks/payjp`
- 模式：与密钥相同

PAY.JP 的 webhook 用账户的**来源令牌**（`whook_...`，位于请求头 `X-Payjp-Webhook-Token` 中）标识发送方。通过环境变量 `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN`（或 `_FILE`）传入控制台中显示的令牌。
端点以恒定时间比较令牌，并以 `400` 拒绝用正式密钥收到的测试事件（反之亦然）。由于没有签名或时间戳，任何知道令牌的人都可以发送通知，但服务器从不根据通知的内容写入状态，只会从 PAY.JP 回读状态。

服务器使用以下事件（其他类型会被接受但不记录）。

- `subscription.created`、`subscription.updated`、`subscription.deleted`、`subscription.paused`、`subscription.resumed`、`subscription.canceled`、`subscription.renewed`
- `charge.succeeded`、`charge.failed`、`charge.refunded`
- `dispute.created`
- `statement.created`

PAY.JP 只会每 3 分钟重发一次通知，最多 3 次，超过之后的通知要等你在控制台中重新发送才会送达。对账会修复漏收的通知，因此**建议把 `payments_reconcile_interval` 设为 1 小时**。
支付服务商的状态（模式、最后一次收到 webhook 的时间，以及最后一次对账的结果）显示在管理页面 `/admin/supporters` 的“支付服务商”部分。

<!-- source: 4-configure-the-server b0d24c7a2da0 -->
## 4. 配置服务器

```sh
MY_PAYMENTS=payjp
MY_PAYMENTS_PAYJP_SECRET_KEY=sk_test_...
MY_PAYMENTS_PAYJP_PUBLIC_KEY=pk_test_...
MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN=whook_...
# The recommended reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=1h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` 会验证配置。使用构建标签 `no_payments_payjp` 构建的二进制文件会拒绝 `payments = payjp`（参见 [build-tags.md](../reference/build-tags.md)）。

<!-- source: 5-try-it-in-test-mode f6ebbada0e0e -->
## 5. 在测试模式下试用

先使用测试密钥。测试模式和正式模式的 Plan、客户、订阅、webhook 和令牌各自独立。要切换到正式环境，请在正式模式下注册 webhook，把密钥和令牌替换为正式的，然后重新部署。各方案的 Plan 会在正式账户中重新创建。

测试卡（[PAY.JP 文档](https://pay.jp/docs/testcard)）：

| 卡号 | 行为 |
| --- | --- |
| `4242 4242 4242 4242` | 成功 |
| `4000 0000 0008 0319` | 可以登记银行卡，但扣款被拒绝（订阅失败，以及更换银行卡后恢复失败） |
| `4000 0000 0000 4012` | 可以登记银行卡，但扣款以过期为由失败 |
| `4000 0000 0000 0002` | 登记银行卡（创建令牌）被拒绝 |

请使用未来的有效期，安全码和姓名可以任意填写。在测试模式下，PAY.JP 的 3-D Secure 测试认证界面可以让你选择结果。

要快速试用续订，请用 API 把订阅的 `trial_end` 设为几分钟之后。续订扣款会在该时间之后的几分钟内进行，而不是正好在该时间。在第一次到期之前再次更新 `trial_end`，会重新开始等待。
要试用付款失败和宽限期，请先订阅，在银行卡页面把卡更换为会被拒绝的卡（`4000 0000 0008 0319`），然后等待订阅续订。

<!-- source: 6-try-it-locally 219997c1a489 -->
## 6. 在本地试用

PAY.JP 的 webhook 会发送到公开的 URL，因此要在你自己机器上的服务器接收它们，需要通过隧道（cloudflared、ngrok 等）公开端口，并把该 URL 注册为测试模式的接收地址。
payjp-cli 的 `payjp listen --forward-to http://localhost:8080/payments/webhooks/payjp` 预计也能转发事件，但它能否转发 v1 事件尚未验证。

请在这台服务器的源上打开付款页面和银行卡页面（它们在 `http://localhost:8080` 上也能使用，但 payjp.js 和 3-D Secure 的行为会因浏览器的设置而异）。

<!-- source: 7-operation fbdd81461f80 -->
## 7. 日常运营

- 类型为 `permission` 的对账失败表示私钥有误。
- “支付服务商”部分的警告中出现 `duplicate_charge`，表示服务器未能完成对已关闭订阅的某笔付款的退款。日志行 `a second subscription could not be canceled and refunded` 会写明该订阅，每次对账都会重试，订阅删除后也是如此。如果这种情况持续，请在控制台中退还该笔扣款：只要 PAY.JP 仍会列出已删除订阅的扣款（参见[待验证事项](#items-to-verify)），下一次重试就会发现它已退款并清除警告。超过 180 天的扣款无法退款：服务器会记录日志并继续。
- 其他退款在 PAY.JP 控制台中处理。全额退款和拒付会立即移除角色并删除订阅，部分退款不会移除角色。
- **拒付的撤销**不会自动记录，因为 PAY.JP 既不发出相应的事件，也没有相应的 API。发生撤销时，请在成本页面（`finance`）的收入界面中记录撤销的金额。角色不会自动恢复。
- **PAY.JP 付费套餐的费用和转账手续费**不会从收入中扣除。请在 `finance` 中把它们记为成本。
- 如果对账经常达到请求上限，请提高 `payments_reconcile_max_requests` 或延长 `payments_reconcile_interval`。

<!-- source: plan-changes 29dec3be3d95 -->
## 更改方案

升级时，先收取按比例计算的差额，扣款成功后才切换方案。扣款被拒绝时，方案保持不变，用户会看到一条错误信息，说明如何更新银行卡，此时没有宽限期，也不会发送付款失败通知，用有效的卡重试只会扣款一次。如果扣款后方案切换失败，重试不会再次扣款。差额低于 50 日元的最低金额时，以及改为同一方案的年付价格（不存在正的差额）时，现在不扣款并立即切换；新价格在周期结束时收取，届时失败则算作续订失败。

<!-- source: items-to-verify 3c2cbf6ed2b0 -->
## 待验证事项

以下行为无法通过 PAY.JP 的文档或测试模式确定。

- 拒付退还的手续费如何计算，以及结算期的结算单是否与服务器计算的手续费一致（测试模式中显示的手续费是舍去尾数的，退款的手续费按其金额计算）。
- 真实的拒付会产生什么效果：PAY.JP 没有说明在测试模式下引发拒付的方法，也没有给出 `dispute.created` 的正文。从控制台测试发送该事件会得到 `200` 响应，且不会改变任何东西。
- 事件列表能回溯多久。
- 删除 Plan 是否会影响现有订阅（服务器从不删除 Plan），以及 Plan ID 的最大长度。
- 加载付款页面时出现的 CSP 报告是否有影响：`pay.js` 会应用一段违反 `style-src 'self'` 的内联样式，而银行卡输入框和 3-D Secure 在 Chromium 中可以正常工作。其他浏览器未经检查。
- payjp-cli 能否转发 v1 事件。
- 对客户已保存的银行卡扣款时，是否确实只在请求要求时才要求 3-D Secure。被拒绝的扣款会以 `paid: false` 出现在 `GET /v1/charges` 的列表中，服务器会忽略未付款的扣款。
- 带 `subscription` 的 `GET /v1/charges` 是否仍会列出已删除订阅的扣款。服务器在删除订阅前先退款，因此只有在退款永久失败之后的重试才依赖这一点。API 参考说明了这个筛选条件，但没有提及已删除的订阅。如果列表为空，服务器会报告失败而不是退款，即使手动退还了扣款，`duplicate_charge` 也会保留。
