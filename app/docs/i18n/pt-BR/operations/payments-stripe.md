<!-- source: supporter-payments-with-stripe f12ff4a0af71 -->
# Pagamentos de apoiadores com o Stripe

Com `payments = stripe`, os usuários locais assinam planos mensais ou anuais na página de pagamento hospedada do Stripe (Checkout), e a função do plano é concedida e removida conforme o estado do pagamento muda.
As chaves de implantação estão descritas na [referência de configuração](../reference/configuration.md#payments). Para o programa como um todo (planos, funções, a lista de membros), veja [supporters.md](supporters.md).
Um administrador ativa o programa e cria os planos na administração. Esta configuração apenas permite que o servidor se comunique com o Stripe.

Um servidor só pode usar um provedor. Para trocar de provedor depois, peça aos assinantes existentes que assinem de novo com o novo.

<!-- source: what-the-server-does-at-stripe ca3e0d4f59f3 -->
## O que o servidor faz no Stripe

[Como funciona uma assinatura](supporters.md#how-a-subscription-works) mostra em um diagrama como a página de pagamento, o webhook, a reconciliação, a função do plano e a receita se encaixam.

- **Planos e preços**: para cada plano, o servidor cria um Product pela API (ID `my_plan_<plan ID>`, com `metadata.my_plan_id`) e um Price por intervalo e moeda (`lookup_key` é `my_<plan ID>_<month|year>_<currency>`).
  Um Price do Stripe não pode mudar de valor, então alterar o valor cria um novo Price e arquiva o antigo. As assinaturas no Price antigo são renovadas pelo valor dele. O desconto do pagamento anual é expresso no valor do Price anual; não são usados cupons.
  Não edite nem exclua esses Products e Prices no painel.
  Um plano pode usar as moedas da [lista de cobranças mínimas do Stripe](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts), e os valores mensal e anual dele devem atingir cada um esse mínimo: USD, AED, ARS, AUD, BRL, CAD, CHF, COP, CZK, DKK, EUR, GBP, HKD, HUF, IDR, ILS, INR, JPY, KRW, MXN, MYR, NOK, NZD, PHP, PLN, RON, RUB, SEK, SGD, THB e ZAR. O servidor recusa qualquer outra moeda.
  A lista vale para uma cobrança na moeda de liquidação da conta. Uma cobrança que o Stripe converte precisa atingir o mínimo da moeda de liquidação depois da conversão, e o servidor não verifica isso, então escolha para o plano um valor bem acima do mínimo quando a moeda dele for diferente da da sua conta bancária.
- **Assinar**: o servidor cria uma sessão do Checkout com `mode=subscription` e envia o navegador do usuário à URL dela (`checkout.stripe.com`). A página de pagamento expira após uma hora.
  O ID da assinatura do servidor vai em `client_reference_id`, em `metadata.my_subscription_id` e no `metadata.my_subscription_id` da assinatura do Stripe. O cálculo automático de impostos (Stripe Tax) não é ativado.
- **Recomeçar**: no máximo uma página de uma assinatura pendente pode receber um pagamento. Um usuário que recomeça no mesmo plano e intervalo volta à página aberta, que abre vazia de novo, a menos que lhe restem menos de 15 minutos. Qualquer outro início recebe uma página nova, e o servidor expira a anterior, que então mostra a mensagem de expirada do Stripe. Se uma página anterior já foi paga, o usuário não recebe página: ele vai para a tela de retorno, que mostra a assinatura. Se o servidor não consegue expirar uma página anterior, o início falha e o usuário pode tentar de novo.
  Uma assinatura pendente que é encerrada (o plano retirado, a conta excluída) expira a sua página da mesma forma.
- **Uma segunda assinatura**: dois pagamentos ainda podem se encontrar, quando as duas páginas são enviadas no mesmo instante. A assinatura que chega primeiro ao servidor é a do usuário. O servidor cancela a outra na hora e reembolsa integralmente todas as faturas pagas dela, renovações incluídas, já que não entregou nada por nenhum período, com o motivo `duplicate`, e registra no log `a second subscription was canceled and refunded`. Um pagamento de uma assinatura pendente que foi encerrada recebe o mesmo tratamento, inclusive quando chegou ao servidor logo antes do encerramento. O usuário recebe o reembolso do Stripe e vê uma única assinatura. O Stripe não devolve a sua taxa da cobrança reembolsada, e essa taxa aparece na receita.
  Uma cobrança em disputa não é reembolsada, nem um pagamento que ainda não tinha sido liquidado quando o servidor olhou (um método de débito, por exemplo). Uma assinatura que alguém já cancelou no painel é deixada como está, com as suas cobranças, e nunca se torna a assinatura do usuário.
  **Antes de atualizar um servidor de uma versão sem isto**, procure no Stripe clientes com duas assinaturas cujo `metadata.my_subscription_id` seja o mesmo. A primeira reconciliação cancela a que não está em uso e reembolsa tudo o que ela cobrou. Cancele no painel as que você preferir resolver manualmente, e o servidor as deixa em paz.
- **Portal do cliente**: no primeiro uso, o servidor cria pela API uma configuração de portal (`metadata.my=1`) que ativa apenas a atualização de métodos de pagamento, o histórico de faturas e o cancelamento no fim do período. As mudanças de plano são feitas nas telas deste servidor, não no portal.
  Um cancelamento no portal aparece neste servidor como agendado para cancelar, e o usuário pode desfazê-lo pela tela do servidor. Uma mudança de plano é recusada até que seja desfeito.
- **Uma data de cancelamento definida manualmente**: o portal define o `cancel_at` da assinatura, não `cancel_at_period_end`, e o servidor lê os dois. Um `cancel_at` que você define no painel ou pela API vale do mesmo modo quando não é posterior ao fim do período atual. Quando é anterior a esse fim, o servidor mostra essa data como o fim da assinatura, e o usuário mantém a função até lá. Um `cancel_at` posterior ainda não é um cancelamento: a assinatura continua ativa até o período que contém a data.
- **Mudanças de plano**: um upgrade, e uma mudança de mensal para anual, cobra a diferença proporcional na hora (`proration_behavior=always_invoice`) e só troca quando essa fatura é paga (`payment_behavior=pending_if_incomplete`). Um downgrade, e uma mudança de anual para mensal, trocam no fim do período por meio de um Subscription Schedule.
- **Uma mudança de plano que falha**: quando o pagamento de uma mudança falha ou o cartão pede autenticação, o servidor anula (void) essa fatura, o que descarta a mudança, e o usuário vê um erro. A assinatura permanece no seu plano e ativa, sem período de carência e sem notificação de pagamento com falha. O usuário pode atualizar o cartão no portal e tentar de novo, e é cobrado uma só vez. Se o pagamento ainda estiver em processamento, o servidor o deixa, e o plano troca quando ele for bem-sucedido. As renovações não são afetadas: uma renovação que falha vai para o período de carência, como descrito abaixo.
- **Ler o estado de volta**: o servidor trata um webhook como um sinal e lê de volta, pela API do Stripe, a assinatura, a sessão do Checkout e as movimentações de dinheiro para decidir o estado.
  Além disso, uma reconciliação a cada `payments_reconcile_interval` (padrão 6 horas) lê a lista de eventos, as sessões do Checkout não concluídas, as balance transactions e uma varredura de todas as assinaturas, e repara qualquer webhook que o servidor tenha perdido.
- **Receita**: o servidor registra o valor bruto, a taxa e o líquido de cada balance transaction. Taxas como os 0,7% do Billing aparecem como transações `stripe_fee` separadas e são subtraídas da receita.
  `stripe_fee` cobre as taxas da conta inteira, então **use uma conta do Stripe dedicada a este servidor**. Em uma conta usada para outras vendas, essas taxas também são subtraídas da receita.
- **Datas**: as datas da receita são contadas no fuso horário da conta do Stripe (o fuso em “Settings > Business > Account details” no painel, `settings.dashboard.timezone` na API). Quando ele não pode ser lido, o servidor conta em UTC e o lê de novo a cada hora.

Cada processo do servidor envia no máximo 20 requisições por segundo (10 no modo de teste). Os [limites do Stripe](https://docs.stripe.com/rate-limits) são por conta: 100 requisições por segundo no modo live, 25 em um sandbox, e 25 por segundo para cada endpoint. Os processos de um servidor os compartilham, assim como qualquer outra coisa que use a conta.
Quando o Stripe retorna `429`, o servidor espera o tempo que `Retry-After` indicar, se for 30 segundos ou menos (1 segundo sem o cabeçalho), e tenta de novo até 2 vezes; além disso, deixa a nova tentativa para o job.

O Stripe também aloca requisições de leitura por conta: uma média de 500 por transação em 30 dias, e pelo menos 10.000 por mês, seja qual for o número de transações. No intervalo padrão, a reconciliação fica muito abaixo disso em um servidor pequeno. Uma reconciliação que não encontra nada ainda lê algumas páginas, então o intervalo mínimo de 15 minutos (2.880 execuções em 30 dias) chega perto de 10.000 em uma conta com poucas transações.
O Stripe limita cada assinatura a 10 faturas novas por minuto e 20 por dia, e uma mudança de plano cobrada na hora gera uma fatura nova.

<!-- source: card-data-and-pci-dss f45a3c7d1846 -->
## Dados de cartão e PCI DSS

Os métodos de pagamento são informados apenas no Checkout e no portal do cliente do Stripe, e o cliente web deste servidor não carrega nem o script do Stripe nem um iframe.
O servidor guarda apenas IDs de objetos do Stripe (`cus_`, `sub_`, `price_`, `prod_`, `cs_`, `txn_`), valores, períodos e estados. Ele não guarda números de cartão, últimos quatro dígitos, datas de validade nem endereços de cobrança. Os corpos de webhook e as respostas do Stripe não são mantidos nem nos logs nem no banco de dados.
Por isso, espera-se que o escopo de PCI DSS do operador seja o de um comerciante que usa uma página de pagamento hospedada (SAQ A). Confira o escopo aplicável e o tipo de questionário de autoavaliação na orientação do painel do Stripe (“Settings > Compliance”).

<!-- source: 1-prepare-the-stripe-account 06eefed5779b -->
## 1. Prepare a conta do Stripe

1. Crie uma conta do Stripe e informe os dados da sua empresa no painel. Se você vende no Japão, registre a URL da sua página de divulgação comercial em “Settings > Business > Public information > Notice based on the Act on Specified Commercial Transactions”, e um link aparecerá abaixo do Checkout.
2. Confira o fuso horário em “Settings > Business > Account details”. As datas da receita são contadas nele.
3. Em “Settings > Billing > Subscriptions and emails > Manage failed payments”, escolha o comportamento de novas tentativas (Smart Retries) e o que acontece quando as tentativas se esgotam.
   O período de carência antes da remoção da função (7 dias a partir da primeira falha) é contado por este servidor, então as configurações de novas tentativas do Stripe não o afetam. Quando as tentativas se esgotam, recomenda-se “Cancelar a assinatura”.
4. Para enviar recibos aos assinantes, ative o recibo de pagamento bem-sucedido em “Settings > Customer emails”.

<!-- source: 2-create-an-api-key 95e2c858acca -->
## 2. Crie uma chave de API

Em “Developers > API keys”, recomenda-se criar uma **chave restrita** (`rk_test_...` / `rk_live_...`). Uma chave secreta (`sk_test_...` / `sk_live_...`) também funciona.
O prefixo da chave define o modo, e uma chave de teste não cobra nada. Uma chave publicável (`pk_`) não pode ser usada.

Dê a uma chave restrita estas permissões. Uma chave restrita com as permissões desta tabela executou, em um sandbox, assinatura, mudanças de plano, cancelamento, reconciliação e anulação de faturas. Reembolsos, expiração de página e disputas não foram executados com ela, e ninguém verificou se a tabela é o menor conjunto que funciona.

| Recurso | Permissão | APIs usadas |
| --- | --- | --- |
| Products | Escrita | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | Escrita | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | Escrita | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}`, `POST /v1/checkout/sessions/{id}/expire` |
| Portal do cliente | Escrita | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | Escrita | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules (parte de Subscriptions) | Escrita | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | Escrita | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments`, `POST /v1/invoices/{id}/void` |
| Cobranças, Reembolsos | Escrita | `GET /v1/charges/{id}`, `GET /v1/refunds`, `POST /v1/refunds` |
| Disputas | Leitura | Lidas por meio das balance transactions e das cobranças |
| Origens de balance transactions | Leitura | `GET /v1/balance_transactions` |
| Events | Leitura | `GET /v1/events` |
| Account (informações de uma conta fora do Connect) | Leitura | `GET /v1/account` (fuso horário) |

Sem a permissão de escrita de reembolsos, o servidor ainda cancela uma segunda assinatura, e `duplicate_charge` permanece nos avisos (veja [Registre o endpoint do webhook](#3-register-the-webhook-endpoint)) até que ele possa reembolsar. Uma chave criada antes de esta permissão ser listada precisa que ela seja adicionada.

Passe a chave na variável de ambiente `MY_PAYMENTS_STRIPE_SECRET_KEY` (ou o caminho de um arquivo em `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`). Ela não pode ser escrita no arquivo de configuração.
Se a chave vazar, rotacione-a no painel e reimplante com a nova chave.

<!-- source: 3-register-the-webhook-endpoint 1cbc2cf9ace3 -->
## 3. Registre o endpoint do webhook

Em “Developers > Webhooks > Add endpoint”, registre o seguinte. O servidor não cria o endpoint pela API.

- URL: `https://<server domain>/payments/webhooks/stripe`
- Eventos de: “Sua conta” (não de contas conectadas)
- Estilo do payload: snapshot. O servidor lê apenas `id`, `type`, `livemode`, `created`, `data.object.id` e `data.object.object` do corpo, então a versão da API do endpoint não importa.
- Eventos a enviar (16 tipos):

  - `checkout.session.completed`, `checkout.session.expired`
  - `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `customer.subscription.paused`, `customer.subscription.resumed`
  - `invoice.paid`, `invoice.payment_failed`
  - `charge.refunded`, `refund.created`, `refund.updated`
  - `charge.dispute.created`, `charge.dispute.closed`, `charge.dispute.funds_withdrawn`, `charge.dispute.funds_reinstated`

Passe o segredo de assinatura do endpoint (`whsec_...`) na variável de ambiente `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` (ou `_FILE`).

O endpoint verifica a assinatura (HMAC-SHA256 em `Stripe-Signature`) e o carimbo de data e hora (dentro de 5 minutos para mais ou para menos), e rejeita com `400` um evento de teste recebido com uma chave live (e o inverso).
Ele registra cada evento uma só vez por ID, enfileira um job e retorna `2xx`. Quando não consegue gravar no banco de dados, retorna `503`, e o Stripe envia o evento de novo. O corpo é limitado a 1 MiB.
Enquanto você rotaciona o segredo de assinatura, o Stripe assina com o antigo e o novo, então colocar o novo segredo na configuração é suficiente.

O estado do provedor (o modo, quando chegou o último webhook, e o resultado e o tipo de falha da última reconciliação) é mostrado na seção “Provedor de pagamento” de `/admin/supporters` na administração, e pela API de administração `GET /api/v1/admin/payment_status` (permissão `manage_supporters`). A seção explica como corrigir cada tipo de falha de reconciliação.
`warnings` lista `no_webhook` (nenhum webhook foi registrado até hoje) e `reconciliation_corrected` (a última reconciliação reparou uma assinatura). Suspeite de um webhook mal configurado quando qualquer um dos dois aparecer.
Ele também lista `duplicate_charge`: uma segunda assinatura ainda não foi cancelada e reembolsada. Veja [Operação](#7-operation).
“Reconciliar agora” na seção (`POST /api/v1/admin/payment_reconciliation/runs`) reconcilia na hora, em vez de esperar a execução agendada. Ele é limitado a uma vez por hora UTC (uma segunda chamada na mesma hora retorna `429`), e o registro de auditoria grava `payment_reconcile_queued`.

<!-- source: 4-configure-the-server 7b3f7d01839f -->
## 4. Configure o servidor

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` verifica a configuração. Um binário compilado com a build tag `no_payments_stripe` recusa `payments = stripe` (veja [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode 94db104964c3 -->
## 5. Experimente no modo de teste

Comece com uma chave de teste (`rk_test_` / `sk_test_`, ou uma chave de sandbox). O modo de teste não cobra nada.
Chaves de teste e chaves live têm Products, Prices, clientes, assinaturas e endpoints de webhook separados. Para entrar em produção, registre o endpoint de novo no modo live, troque a chave e o segredo de assinatura pelos live e reimplante. Os Prices dos planos são criados novamente na conta live.

Cartões de teste ([documentação do Stripe](https://docs.stripe.com/testing)):

| Número | Comportamento |
| --- | --- |
| `4242 4242 4242 4242` | Bem-sucedido |
| `4000 0000 0000 0341` | Pode ser vinculado, mas a cobrança falha (para verificar a falha de pagamento e o início do período de carência) |
| `4000 0000 0000 9995` | Falha por saldo insuficiente |
| `4000 0000 0000 0259` | Uma disputa é criada após o pagamento |

Use qualquer data de validade futura e qualquer código de segurança de 3 dígitos.

Para experimentar a renovação e a falha de pagamento, use um [test clock](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage). Um cliente criado pelo Checkout não tem relógio na criação, então assine primeiro e depois crie um relógio para esse cliente pela API, passando `customer` para `POST /v1/test_helpers/test_clocks` (versão da API `2026-05-27.dahlia` ou posterior).
O Stripe só permite isso quando `frozen_time` não está no passado, a conta não tem Automations configuradas e o cliente tem no máximo três assinaturas. O cliente não pode sair do relógio depois, e excluir o relógio exclui o cliente e as suas assinaturas. Avance o tempo em “Billing > Test clocks” no painel para disparar a cobrança da renovação e a falha.

Um relógio não consegue mostrar tudo:

- **O fim do período de carência**: o servidor compara o tempo real com o tempo que o Stripe informa para a falha (`finalized_at`), que é simulado e está à frente do tempo real. A assinatura entra no período de carência, e avançar mais o relógio não o encerra. A função permanece até que o tempo real passe 7 dias após a falha simulada, ou até que o Stripe cancele a assinatura quando as suas tentativas se esgotarem, se a conta estiver configurada para isso.
- **A varredura de todas as assinaturas**: a lista de assinaturas do Stripe deixa de fora as assinaturas em um relógio, a menos que a requisição nomeie o relógio ou o cliente, e a varredura do servidor não nomeia nenhum dos dois. Apenas os webhooks e a lista de eventos levam as mudanças delas ao servidor.

<!-- source: 6-try-it-locally-stripe-cli 8a3e87b02ce0 -->
## 6. Experimente localmente (Stripe CLI)

O [Stripe CLI](https://docs.stripe.com/stripe-cli) (Apache-2.0) encaminha eventos do Stripe para um servidor na sua máquina. Nenhuma URL pública é necessária.

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

Coloque o `whsec_...` que `stripe listen` imprime em `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` e inicie o servidor (combine a porta com a configuração do seu `listen`).

Com Docker Compose, defina `MY_PAYMENTS=stripe`, `MY_PAYMENTS_STRIPE_SECRET_KEY` e `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` em `deploy/docker/.env` e execute `docker compose up -d`.

Em outro terminal, você pode disparar eventos individuais.

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

Os objetos que `stripe trigger` cria não carregam nenhum ID de assinatura deste servidor, então nenhuma assinatura muda de estado (o servidor os lê de volta e ignora o que não é uma assinatura sua). Para verificar o fluxo completo da assinatura, assine pelo cliente web e pague com um cartão de teste.

<!-- source: 7-operation f70cd2967b9f -->
## 7. Operação

- Uma falha de reconciliação do tipo `permission` significa uma chave errada ou permissões ausentes. A linha de log do servidor (`payment reconciliation failed`) traz a mensagem do Stripe, que nomeia a permissão a adicionar.
- Quando as reconciliações atingem o limite de requisições com frequência, aumente `payments_reconcile_max_requests` ou alongue `payments_reconcile_interval`. Cada leitura conta para a alocação de leitura do Stripe (veja [O que o servidor faz no Stripe](#what-the-server-does-at-stripe)), então prefira o intervalo mais longo em uma conta com poucas transações.
- Quando a reconciliação ficou parada por mais de 30 dias, o servidor não consegue se atualizar a partir da lista de eventos do Stripe (que retrocede 30 dias). A varredura de todas as assinaturas repara o estado nesse caso.
- `duplicate_charge` nos avisos significa que um usuário pagou duas vezes e o servidor não conseguiu terminar de cancelar e reembolsar a segunda assinatura. A linha de log `a second subscription could not be canceled and refunded` nomeia as duas assinaturas e o tipo de falha, e aparece de novo a cada reconciliação, que tenta outra vez. Com o tipo `permission`, adicione a permissão de escrita de reembolsos. Caso contrário, reembolse a cobrança no painel: a próxima tentativa não encontra mais nada a reembolsar e limpa o aviso. “Reconciliar agora” tenta na hora.
- Outros reembolsos e chargebacks são tratados no painel do Stripe. Um reembolso integral e um chargeback removem a função e cancelam a assinatura na hora, e um reembolso parcial não remove a função. Ganhar uma disputa não restaura a função automaticamente.

<!-- source: items-to-verify 8960333fa514 -->
## Itens a verificar

Estes não foram testados no Stripe, nem em sandbox nem em live, e se apoiam na documentação do Stripe.

- Uma renovação de uma assinatura em um Price arquivado, que deveria permanecer nesse Price.
- `status_transitions.finalized_at` em uma renovação real. O servidor o toma como o momento em que o pagamento falhou pela primeira vez e conta o período de carência a partir dele.
- Se uma transação `stripe_fee` pode ser atribuída a uma assinatura. O servidor não tenta: ele subtrai essas taxas da receita como um todo.
- Se `settings.dashboard.timezone` de `GET /v1/account` é lido com uma chave restrita. Uma leitura que falha não interrompe o servidor, que conta em UTC, então um servidor funcionando não a mostra. Confira se as datas da receita caem no fuso horário da conta.
- Que `GET /v1/invoice_payments` filtra por `payment[payment_intent]`, que liga um pagamento à sua assinatura.
- Reembolsos integrais e parciais e chargebacks, do painel até o estado no servidor.
- Que a varredura de todas as assinaturas repara uma assinatura cujo estado o servidor perdeu.
- O que um upgrade faz para uma assinatura que paga com um método que as atualizações pendentes não listam (débitos diretos ACH, SEPA e Bacs). Mantenha esses métodos desativados no painel até que isso seja confirmado.
- Que `cancel_at_period_end=true` substitui um `cancel_at` em um período posterior.
- Voltar a uma página aberta. O servidor compara o que `GET /v1/checkout/sessions/{id}` retorna (`line_items`, `success_url`, `cancel_url`, `customer`, `customer_email`) com a nova requisição, e qualquer diferença gera uma página nova. Se o Stripe reescreve `success_url` ou `customer_email` ao armazená-los, o usuário recebe uma página nova a cada vez e perde o que digitou, mas paga uma só vez.
- Expirar uma sessão no momento em que o pagamento dela é concluído. A documentação do Stripe diz que uma sessão que não está aberta retorna um erro, e o servidor então lê a sessão. O código exato do erro e o status enquanto uma autenticação de cartão está em andamento não estão documentados.
- Que uma sessão não pode ser paga duas vezes a partir de duas abas. Uma sessão contém uma assinatura e é concluída uma vez, mas a documentação não afirma isso para dois envios no mesmo instante.
- Se uma autenticação com falha na página de pagamento deixa uma assinatura `incomplete` que nomeia a assinatura do servidor. O servidor a vincularia e manteria o usuário pendente, e um pagamento em outra página seria reembolsado como o segundo.
- Um reembolso de uma segunda assinatura, com uma chave restrita cujas permissões “Cobranças” e “Reembolsos” sejam as da tabela, e as movimentações de dinheiro que o seguem.
