<!-- source: importing-cloud-usage-costs db52fcd17ff9 -->
# Importação dos custos de uso de nuvem

O servidor importa a cada 24 horas as cobranças de uso dos provedores listados na configuração [`billing_sources`](../reference/configuration.md#billing_sources), ou de [um plugin](#a-plugin-billing_sources--plugin) para um provedor do qual ele não tem adaptador, e as soma aos custos da instância. O padrão é vazio, o que não importa nada.
Ele importa as cobranças do mês de cobrança atual e do anterior (até o dia 15 do mês, ou até o mês anterior ser definitivo), em uma linha por serviço e tipo de cobrança (uso, crédito, reembolso, imposto, suporte, taxa, outro).
Até o provedor fechar um mês, os valores são registrados como estimativas; depois do fechamento, como definitivos; e, quando um valor muda depois de definitivo, como correção. Os registros do passado nunca são reescritos.
O estado das importações (o último sucesso, o tipo da última falha e o número de requisições neste mês) aparece na página de custos da administração e em `GET /api/v1/admin/billing_imports`.
As métricas contam cada tentativa por origem, resultado e tipo de falha (`my.billing.imports`) e informam as origens cuja tentativa mais recente falhou (`my.billing.import.failing`);
[Alertas e runbook](alerts.md#mybillingimportfailing) traz o alerta sobre elas.

Nenhuma credencial é mantida na configuração deste servidor. Dê ao ambiente de execução o que a cadeia de credenciais padrão do SDK de cada provedor lê (na AWS, a função de execução, variáveis de ambiente ou arquivos de credenciais compartilhados; no Google Cloud, as Application Default Credentials).

<!-- source: with-the-opentofu-modules 51ec4f78f058 -->
## Com os módulos do OpenTofu

Os módulos [`billing-aws`](../../deploy/opentofu/modules/billing-aws/README.md) e [`billing-gcp`](../../deploy/opentofu/modules/billing-gcp/README.md) definem `billing_sources` e fazem as partes deste guia que a API de um provedor permite. `billing-gcp` funciona apenas com o módulo de aplicação do Google Cloud, então na AWS os módulos importam somente os custos da AWS. `billing-aws` funciona com os módulos de aplicação da AWS e com o do Cloud Run, em que ambos podem estar em `backends`. Nenhum dos dois foi aplicado em uma conta real ainda.

| Etapa deste guia | Com os módulos |
| --- | --- |
| A política do IAM e a função de "Chamada a partir do Cloud Run com identidade web" | `billing-aws` |
| Ativar o Cost Explorer, o Linked Account Access e as tags de alocação de custos | Manualmente, no console da AWS |
| O dataset da exportação e as funções de "Funções do IAM" | `billing-gcp` |
| Ativar a exportação ("Standard usage cost") | Manualmente, no console do Google Cloud, com uma função na conta de faturamento |
| O tópico e a assinatura pull de um orçamento, e a função de assinante | `billing-gcp` com `method = "budget"` |
| Criar o orçamento e conectá-lo ao tópico | Manualmente, no console do Google Cloud, com uma função na conta de faturamento |
| Ativar a API do BigQuery ou do Pub/Sub | Manualmente, como em [Implantação no Cloud Run](deploy-cloud-run.md#1-enable-the-apis) |
| Um plugin | Não por estes módulos. Sem nenhum módulo de cobrança em `backends`, defina `MY_BILLING_SOURCES = plugin` e as configurações do [plugin](#a-plugin-billing_sources--plugin) no `env` do módulo de aplicação; ao lado de um módulo de cobrança, o módulo de aplicação recusa `MY_BILLING_SOURCES` em `env`, então adicione um backend seu que retorne `billing_sources = ["plugin"]` ([os campos de um backend](../../deploy/opentofu/README.md)) |

<!-- source: aws-billing_sources--aws 9e5fa0132ca3 -->
## AWS (`billing_sources = aws`)

O servidor chama `GetCostAndUsage` do AWS Cost Explorer uma vez por mês de cobrança (um mês civil em UTC), agrupado por serviço (`SERVICE`) e tipo de registro (`RECORD_TYPE`). O endpoint é sempre `us-east-1`.
Os valores são em USD. Enquanto o Cost Explorer marca um mês como estimado (`Estimated`), o servidor registra o valor até o dia anterior como estimativa e, quando ele deixa de marcar, como definitivo.
Um binário compilado com a build tag `no_billing_aws` recusa `aws` (veja [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits a555f91984c3 -->
### Custo e limites

A API do Cost Explorer cobra US$ 0,01 por requisição paginada. Uma conta pequena precisa de uma requisição de uma página por dia, cerca de US$ 0,31 por mês.
Para evitar cobranças inesperadas, o servidor conta as requisições por dia UTC (`billing_aws_max_requests_per_day`, padrão 4) e por mês civil UTC (`billing_aws_max_requests_per_month`, padrão 120 = US$ 1,20) e para sem chamar quando um limite é atingido.
Importações agendadas, novas tentativas de jobs e “Importar agora” na administração contam todas. Uma importação que atinge um limite é registrada como falha do tipo `request_cap`, e a importação do dia seguinte (no caso do limite diário) ou do primeiro dia do mês seguinte (no caso do limite mensal) tenta de novo.
Antes de chamar, uma importação reserva as requisições que os limites deixam, de modo que importações simultâneas não possam exceder os limites juntas.
As importações de um provedor rodam uma de cada vez: uma importação que começa enquanto outra não terminou tenta de novo um minuto depois, e um job que já importou com sucesso não chama nada quando roda de novo.
Uma importação interrompida antes de terminar (o processo saiu) conta as requisições que reservou, e a próxima importação espera por ela por até 15 minutos.

<!-- source: enable-cost-explorer 2f6cd1add48d -->
### Ativar o Cost Explorer

1. Abra o Cost Explorer no console do Billing and Cost Management e ative-o. Depois de ativado, ele não pode ser desativado. Os dados levam até 24 horas para ficar prontos.
2. Uma conta em que ele não está ativado retorna `DataUnavailableException`, e a importação falha com `not_enabled`.

<!-- source: iam-permissions 9f1c23f3cb5c -->
### Permissões do IAM

A única permissão necessária é `ce:GetCostAndUsage` (recurso `*`).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

Como passar as credenciais em cada ambiente de execução:

| Ambiente de execução | Credenciais |
| --- | --- |
| AWS ECS | Anexe esta política à task role. Nenhuma configuração é necessária |
| Google Cloud (Cloud Run) | Crie a função do IAM de "Chamada a partir do Cloud Run com identidade web" abaixo e defina o ARN dela em `billing_aws_role_arn`. Nenhuma chave da AWS é necessária |
| Seu próprio servidor | Passe a chave de acesso de um usuário do IAM com esta política em `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (ou um arquivo de credenciais compartilhado e `AWS_PROFILE`). Passe-a a partir de um repositório de segredos |

Permissões ausentes ou credenciais inválidas falham com `permission`.

<!-- source: calling-from-cloud-run-with-web-identity 7d109914680a -->
### Chamada a partir do Cloud Run com identidade web

Quando `billing_aws_role_arn` está definido, o servidor obtém do servidor de metadados um token de ID do Google da conta de serviço do Cloud Run (a audiência é o ARN da função) e o troca por credenciais temporárias dessa função com `AssumeRoleWithWebIdentity`.

1. No Google Cloud, anote o ID único (o `uniqueId` numérico) da conta de serviço do serviço do Cloud Run: `gcloud iam service-accounts describe <email> --format='value(uniqueId)'`.
2. No IAM da AWS, crie uma função com a política das permissões acima e faça a política de confiança dela confiar em `accounts.google.com`.

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "Federated": "accounts.google.com" },
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Condition": {
           "StringEquals": {
             "accounts.google.com:aud": "<uniqueId of the service account>",
             "accounts.google.com:oaud": "<ARN of this role>"
           }
         }
       }
     ]
   }
   ```

   `accounts.google.com:aud` corresponde ao `azp` do token (o ID da conta de serviço), e `accounts.google.com:oaud` ao `aud` do token (a audiência que o servidor solicita, que é o ARN da função).
3. Defina `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>`.

Esta configuração ainda não foi verificada em uma conta real.

<!-- source: calling-from-a-linked-account-of-organizations d12e10b51af3 -->
### Chamada a partir de uma conta vinculada do Organizations

A conta de gerenciamento do AWS Organizations pode ver os custos de todas as contas-membro da organização. Para chamar com as credenciais de uma conta-membro (vinculada),
ative "Linked Account Access" nas preferências do Cost Explorer da conta de gerenciamento. Quando está desativado, a chamada é recusada seja qual for a permissão do IAM, e a importação falha com `not_enabled` ou `permission`.

<!-- source: choosing-which-costs-to-import-filtering 60cfa862b6a2 -->
### Escolher quais custos importar (filtragem)

Escolha com `billing_aws_filter`.

| `billing_aws_filter` | Custos importados | Configurações adicionais |
| --- | --- | --- |
| `none` (padrão) | Todos os custos que as credenciais podem ver | Nenhuma |
| `linked_account` | Os custos das contas-membro que você nomear (`LINKED_ACCOUNT`) | `billing_aws_linked_accounts` (IDs de conta de 12 dígitos, separados por vírgula) |
| `tag` | Os custos dos recursos que têm a tag de alocação de custos que você nomear | `billing_aws_tag` (`key=value`, por exemplo `app=my`) |

A configuração mais precisa é uma conta da AWS dedicada a este servidor (pode ser uma conta-membro do Organizations), importada com `none` (chamando com as credenciais dessa conta) ou `linked_account` (chamando a partir da conta de gerenciamento).
Impostos, taxas de suporte e alguns créditos não podem ter tag e podem ficar de fora de um filtro por tag (ainda não verificado em uma conta real).

<!-- source: activating-a-cost-allocation-tag-billing_aws_filter--tag 8e642123a388 -->
### Ativar uma tag de alocação de custos (`billing_aws_filter = tag`)

Uma tag em um recurso não aparece nos dados de cobrança até você ativá-la como tag de alocação de custos.

1. Coloque a mesma tag (por exemplo `app=my`) nos recursos deste servidor (serviços do ECS, RDS, buckets do S3, balanceadores de carga e assim por diante).
2. Em "Cost allocation tags" no console do Billing and Cost Management, selecione a chave da tag e ative-a. Uma tag leva até 24 horas para aparecer nesta lista depois de anexada, e até mais 24 horas depois da ativação para aparecer nos dados de cobrança.
3. Para refletir os custos do período anterior à ativação, use "Backfill" na mesma tela (a API `StartCostAllocationTagBackfill`).
4. Defina `MY_BILLING_AWS_FILTER=tag` e `MY_BILLING_AWS_TAG=app=my`.

<!-- source: metric 3e034dd351bd -->
### Métrica

`billing_aws_metric` é `NetUnblendedCost` (o padrão, mais próximo da fatura), que vem depois dos descontos (Savings Plans, instâncias reservadas, preços negociados), ou `UnblendedCost`, que vem antes deles.
Os valores do Cost Explorer e a fatura podem diferir por causa do momento da atualização e de arredondamentos. Quando diferem, a fatura está certa.

<!-- source: kinds-of-failure 2a6f9042f460 -->
### Tipos de falha

| Tipo | Resposta da AWS | Como corrigir |
| --- | --- | --- |
| `permission` | `AccessDeniedException`, credenciais inválidas | Verifique as permissões do IAM e como as credenciais são passadas |
| `not_enabled` | `DataUnavailableException` | Ative o Cost Explorer. Para uma conta vinculada, ative o Linked Account Access |
| `not_found` | `BillExpirationException` | O Cost Explorer não tem mais os dados de cobrança do período a importar |
| `throttled` | `LimitExceededException` | O job tenta de novo após uma pausa (conta para o limite de requisições) |
| `unavailable` | 5xx, falha de rede | O job tenta de novo após uma pausa (conta para o limite de requisições) |
| `invalid_response` | Uma resposta ilegível, `ValidationException` e similares | Verifique as configurações (tag, IDs de conta) |
| `request_cap` | (nenhuma chamada é feita) | Espere passar o dia ou o mês que limita, ou aumente o limite |

Tentar de novo não corrige uma falha que não seja `throttled` ou `unavailable`, então o servidor a registra e para, e a importação agendada do dia seguinte tenta de novo.

<!-- source: google-cloud-billing_sources--gcp c74934cb4d9a -->
## Google Cloud (`billing_sources = gcp`)

O Google Cloud não tem uma API que retorne os custos reais, então o servidor agrega a exportação do Cloud Billing para o BigQuery (standard usage cost) uma vez por dia com um `jobs.query` do BigQuery (`billing_gcp_method = bigquery`, o padrão).
Quando você não consegue configurar a exportação, há também um método simples que importa apenas um total aproximado a partir das notificações do Pub/Sub de um orçamento (veja [Importação a partir de notificações de orçamento](#importing-from-budget-notifications-billing_gcp_method--budget)).
O mês de cobrança é o `invoice.month` da exportação, um mês civil no horário do Pacífico dos EUA (com horário de verão). As datas são registradas no horário do Pacífico, como estão.
Os valores estão na moeda da conta de faturamento, somados como `NUMERIC` por mês de cobrança, serviço (`service.description`) e tipo de cobrança (`cost_type`), e os créditos (`credits[]`) viram uma linha de crédito por serviço. As linhas de correção da exportação que cancelam linhas anteriores fazem parte da soma.
A exportação não tem marca de que o mês é definitivo, então a importação que roda a partir do dia 5 (horário do Pacífico) do mês seguinte trata o mês como definitivo e, até lá, o registra como estimativa do primeiro dia do mês até o dia do último uso. Depois de definitivo, a importação ainda lê o mês de novo até o dia 15 e registra qualquer diferença como correção.
Um binário compilado com a build tag `no_billing_gcp` recusa `gcp` (veja [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits-1 ca56b5d7b34d -->
### Custo e limites

Uma importação é uma consulta, e o BigQuery cobra os bytes que cada consulta varre (no mínimo 10 MB). Uma conta pequena varre cerca de 0,3 GB por mês, o que cabe no primeiro 1 TiB por mês do nível gratuito (compartilhado com os outros usos da conta de faturamento), então custa US$ 0.
Para evitar varreduras inesperadas, toda consulta leva `maximumBytesBilled` (`billing_gcp_max_bytes_billed`, padrão 1 GiB). Uma consulta que o excederia não é cobrada e falha, e é registrada como falha do tipo `bytes_cap`. A administração mostra os bytes cobrados.

<!-- source: enable-the-export-to-bigquery 40c7a176b9f1 -->
### Ativar a exportação para o BigQuery

O operador configura isso uma vez, no console (não se encontrou nenhuma forma de configurá-lo por uma API).

1. Em um projeto vinculado à conta de faturamento, crie um dataset do BigQuery (por exemplo `billing_export`). Escolha uma localização multirregional (`US` ou `EU`). Os dados do mês atual e do anterior são preenchidos retroativamente (com uma única região, os dados começam no dia em que você ativa). A localização não pode ser alterada depois.
   Não defina expiração nas tabelas. Linhas que expiram não voltam.
2. Em "Billing > Billing export > BigQuery export", ative "Standard usage cost" e selecione o dataset da etapa 1. A configuração exige "Billing Account Costs Manager" ou "Billing Account Administrator" na conta de faturamento e a função BigQuery User no projeto.
3. Depois de algumas horas a um dia, a tabela `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>` é criada (os hifens do ID viram `_`).
4. Defina `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`.

Quando o dataset ou a tabela não é encontrado, a importação falha com `not_found`.

<!-- source: iam-roles f5628acd0d94 -->
### Funções do IAM

Dê estas duas funções à conta de serviço do servidor que importa.

| Função | Onde concedê-la | Uso |
| --- | --- | --- |
| `roles/bigquery.jobUser` (BigQuery Job User) | O projeto que executa a consulta (`billing_gcp_query_project`, ou o projeto da tabela quando vazio) | `bigquery.jobs.create`. Os bytes varridos são cobrados deste projeto |
| `roles/bigquery.dataViewer` (BigQuery Data Viewer) | O dataset da exportação | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

Quando a API do BigQuery não está ativada no projeto da consulta, a importação falha com `not_enabled` e, quando falta uma função, com `permission`.

A autenticação usa as Application Default Credentials (ADC).

| Ambiente de execução | Credenciais |
| --- | --- |
| Google Cloud (Cloud Run) | ADC com a conta de serviço do serviço. Sem chave e sem configuração |
| AWS ECS | Coloque um arquivo de configuração de credenciais do Workload Identity Federation (o tipo AWS) e passe o caminho dele em `GOOGLE_APPLICATION_CREDENTIALS` (ainda não verificado em uma conta real) |
| Seu próprio servidor | Workload Identity Federation, ou uma chave JSON de conta de serviço passada em `GOOGLE_APPLICATION_CREDENTIALS`. Passe a chave a partir de um repositório de segredos |

<!-- source: choosing-which-costs-to-import-filtering-1 7067e430a4c7 -->
### Escolher quais custos importar (filtragem)

Por padrão, o servidor importa todos os custos da conta de faturamento. A configuração mais precisa é uma conta de faturamento dedicada a este servidor, importada sem filtros.

| Configuração | Custos importados |
| --- | --- |
| `billing_gcp_projects` (IDs de projeto, separados por vírgula) | Os custos dos projetos que você nomear (`project.id`) |
| `billing_gcp_label` (`key=value`, por exemplo `app=my`) | Os custos dos recursos que têm o rótulo que você nomear (`labels`) |

Quando você define ambos, o servidor importa apenas os custos que correspondem aos dois.
Custos no nível da fatura, como impostos, podem não ter projeto nem rótulo e então ficam de fora do filtro (ainda não verificado em uma conta real).

<!-- source: kinds-of-failure-1 612d14e2ba5c -->
### Tipos de falha

| Tipo | Resposta do BigQuery | Como corrigir |
| --- | --- | --- |
| `permission` | 401, 403 (`accessDenied`) | Verifique as funções da conta de serviço e como as credenciais são passadas |
| `not_enabled` | 403 (`accessNotConfigured` e similares) | Ative a API do BigQuery no projeto da consulta |
| `not_found` | 404 | Verifique `billing_gcp_table` e a configuração da exportação |
| `bytes_cap` | 400 (`bytesBilledLimitExceeded`) | A varredura excedeu `billing_gcp_max_bytes_billed`. Aumente o limite se a tabela for grande |
| `throttled` | 429, 403 (`rateLimitExceeded`, `quotaExceeded`) | O job tenta de novo após uma pausa |
| `unavailable` | 5xx, falha de rede, uma consulta que não termina | O job tenta de novo após uma pausa |
| `invalid_response` | Uma resposta ilegível, linhas que misturam moedas em um mês, qualquer outro 400 | Verifique a tabela de exportação (é o standard usage cost?) |

Tentar de novo não corrige uma falha que não seja `throttled` ou `unavailable`, então o servidor a registra e para, e a importação agendada do dia seguinte tenta de novo.

<!-- source: importing-from-budget-notifications-billing_gcp_method--budget b90e6393e3d3 -->
### Importação a partir de notificações de orçamento (`billing_gcp_method = budget`)

Este é um método simples para quando você não consegue configurar a exportação para o BigQuery. Uma vez por dia, o servidor lê, de uma assinatura pull, as notificações que um orçamento do Cloud Billing envia a um tópico do Pub/Sub.
Ele usa pull em vez de push, então uma notificação nunca acorda o servidor (o Cloud Run, por exemplo).

Este método importa **apenas um total aproximado baseado em estimativas**.

- O `costAmount` de uma notificação é o custo cumulativo estimado do período do orçamento, sem detalhamento por serviço ou tipo de cobrança. O servidor importa apenas uma linha por mês e orçamento (o nome do serviço é o nome de exibição do orçamento, o tipo de cobrança é uso e a categoria é outro). Créditos e impostos estão no total conforme o orçamento está configurado.
- Como no BigQuery, um mês se torna definitivo na importação que roda a partir do dia 5 (horário do Pacífico) do mês seguinte. Uma notificação de orçamento não sabe nada sobre o fechamento da fatura, então mesmo um valor tratado como definitivo pode diferir do valor da fatura.
- Um mês estimado é registrado como o custo do primeiro dia do mês até o dia (horário do Pacífico) em que a última notificação foi publicada.
- A filtragem é decidida pelo escopo do orçamento (projetos, serviços, uma chave e um valor de rótulo), e `billing_gcp_projects` e `billing_gcp_label` não são usados.

<!-- source: prepare-the-budget-the-topic-and-the-subscription ee38acf8cb72 -->
#### Preparar o orçamento, o tópico e a assinatura

1. Crie um tópico e uma assinatura pull que somente este servidor leia. Veja "Como as notificações são lidas" abaixo para os motivos do período de retenção e da expiração.

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. No console, em "Billing > Budgets & alerts", crie um orçamento. Faça o período dele **mensal** (a notificação de um orçamento que não é mensal falha com `invalid_response`).
   Selecione o projeto deste servidor como escopo e, em "Manage notifications > Connect a Pub/Sub topic to this budget", selecione o tópico da etapa 1.
   Criar o orçamento exige "Billing Account Costs Manager" ou "Billing Account Administrator" na conta de faturamento, e `pubsub.topics.setIamPolicy` e `pubsub.topics.list` no projeto do tópico.
   A primeira notificação leva algumas horas para chegar e, depois disso, chegam várias por dia.
3. Dê `roles/pubsub.subscriber` (Pub/Sub Subscriber) na assinatura à conta de serviço do servidor que importa. Ela é usada para puxar mensagens e confirmá-las (`pubsub.subscriptions.consume`).

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. Defina `MY_BILLING_GCP_METHOD=budget` e `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget`.

A autenticação é a mesma das Application Default Credentials do BigQuery (veja a tabela em [Funções do IAM](#iam-roles) acima).
A build tag `no_billing_gcp` remove também este método. A biblioteca cliente do Pub/Sub é compartilhada com `stream = pubsub`, então ela só sai do binário quando você também define `no_pubsub` (veja [build-tags.md](../reference/build-tags.md)).

<!-- source: how-notifications-are-read 646a6dd9b2b6 -->
#### Como as notificações são lidas

- Uma importação repete o Pull até não chegar nenhuma mensagem nova e toma, para cada orçamento e mês, a notificação com o horário de publicação mais recente. Isso absorve entregas duplicadas do Pub/Sub, notificações repetidas com o mesmo valor e reordenação, então receber a mesma notificação muitas vezes conta uma só vez no total.
  O número de requisições contado para uma importação é o número de Pulls, normalmente dois: um que retorna mensagens e um que volta vazio. As confirmações feitas no final (`Acknowledge`, `ModifyAckDeadline`) não são contadas.
- O servidor confirma (ack) apenas uma notificação que foi substituída por uma mais nova do mesmo orçamento e mês, e uma notificação de um mês que não é mais importado (o mês anterior, depois de registrado como definitivo). A notificação mais recente de um mês importado não é confirmada, mas devolvida à assinatura, então a próxima importação lê a mesma notificação de novo, e o valor não muda em um dia sem notificação nova.
- Portanto, defina a retenção de mensagens da assinatura para **pelo menos 14 dias (31 dias recomendados)**. A última notificação do orçamento de um mês chega por volta do dia 2 do mês seguinte, e o mês anterior é lido de novo até o dia 15. Um mês cujas notificações expiraram é registrado com custo 0.
- Por padrão, uma assinatura expira depois de 31 dias sem uso. Defina a expiração como "never" para que ela sobreviva a um período em que as importações ficaram paradas.
- Não envie outras mensagens ao tópico. Uma mensagem que não pode ser lida como notificação de orçamento faz aquela importação falhar com `invalid_response` e é confirmada para que a próxima importação não trave nela.
- Custo: algumas dezenas de mensagens por dia cabem no nível gratuito do Pub/Sub (10 GiB por mês), então custa US$ 0. O servidor não define limite.

<!-- source: kinds-of-failure-budget-notifications c96440fefa1a -->
#### Tipos de falha (notificações de orçamento)

| Tipo | Resposta do Pub/Sub | Como corrigir |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`, `UNAUTHENTICATED` | Verifique `roles/pubsub.subscriber` na assinatura e como as credenciais são passadas |
| `not_enabled` | `PERMISSION_DENIED` (`SERVICE_DISABLED`) | Ative a API do Pub/Sub no projeto da assinatura |
| `not_found` | `NOT_FOUND` | Verifique `billing_gcp_budget_subscription` e se a assinatura expirou |
| `throttled` | `RESOURCE_EXHAUSTED` | O job tenta de novo após uma pausa |
| `unavailable` | `UNAVAILABLE`, `INTERNAL` e similares, falha de rede | O job tenta de novo após uma pausa |
| `invalid_response` | Uma mensagem que não pode ser lida como notificação de orçamento, um orçamento que não é mensal, dois orçamentos com o mesmo nome de exibição | Torne o orçamento mensal, dê nomes de exibição distintos aos orçamentos e não envie outras mensagens ao tópico |

<!-- source: a-plugin-billing_sources--plugin 7589c3333153 -->
## Um plugin (`billing_sources = plugin`)

Para um provedor do qual o servidor não tem adaptador, como Azure, Hetzner ou Sakura, execute um [plugin](plugins.md) do tipo `billing` e deixe-o ler os dados do provedor. O servidor tem uma única origem `plugin`. Um plugin com vários provedores os agrega por conta própria e os distingue pelos nomes de seus serviços.

```toml
billing_sources = "plugin"
plugin_billing_address = "unix:///run/my/billing.sock"
plugin_billing_location = "Europe/Berlin"
plugin_billing_provider = "Hetzner"
```

As três configurações são obrigatórias com esta origem, e o servidor inicia sem chamar o plugin, mesmo que ele esteja fora do ar.

- `plugin_billing_location` é o fuso horário que delimita os meses de cobrança do plugin.
- `plugin_billing_provider` é o nome do provedor nos totais, na página de custos e no registro de auditoria.

O servidor pede ao plugin o mês atual e, como nas outras origens, o anterior até o dia 15 ou até ser definitivo. Os limites das outras origens não se aplicam: `max_requests` é `2147483647`, o que significa sem limite, e o plugin decide o que pode gastar. Ele informa as requisições e os bytes pelos quais foi cobrado, que o servidor registra e mostra.

O tipo permite TCP simples, com um aviso na inicialização, já que ele carrega apenas custos. Um bearer token exige `https://` ou um socket Unix: veja [o destino de um plugin](plugins.md#the-destination).

<!-- source: the-call 6a2bcce82ab2 -->
### A chamada

Com `plugin_billing_protocol = json`, o padrão, um plugin precisa apenas de um servidor HTTP e de uma biblioteca JSON ([Um plugin sem biblioteca](plugins.md#a-plugin-with-no-library)). `Fetch` dura no máximo 2 minutos, ou `plugin_billing_timeout`, abaixo de 5 minutos.

```text
POST /my.plugin.billing.v1.BillingService/Fetch
Content-Type: application/json

{"months":["2026-09","2026-10"],"maxRequests":2147483647}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{
  "outcome": "OUTCOME_OK",
  "requests": 1,
  "lines": [
    {
      "month": "2026-09",
      "coveredStart": "2026-09-01",
      "coveredEnd": "2026-09-30",
      "service": "Hetzner Cloud",
      "chargeType": "CHARGE_TYPE_USAGE",
      "category": "CATEGORY_COMPUTE",
      "amount": "12.3456",
      "currency": "EUR",
      "final": true
    }
  ]
}
```

- Uma linha é um mês, serviço e tipo de cobrança, com suas linhas compensatórias, créditos e reembolsos somados. `service` tem no máximo 200 caracteres e nenhum caractere de controle.
- `coveredStart` e `coveredEnd` ficam dentro do mês: o mês inteiro quando `final` é true e, antes disso, até o último dia com dados.
- `amount` é um decimal sem arredondamento em unidades principais, que pode ser negativo. `currency` é um código ISO 4217.
- `chargeType` é `CHARGE_TYPE_` mais um de `USAGE`, `CREDIT`, `REFUND`, `TAX`, `SUPPORT`, `FEE`, `ADJUSTMENT`, `OTHER`.
- `category` é `CATEGORY_` mais um de `COMPUTE`, `DATABASE`, `STORAGE`, `NETWORK`, `DOMAIN`, `EMAIL`, `SOFTWARE`, `OTHER`.
- `requests` é no máximo `maxRequests`. `bytesBilled` é uma string decimal, para um provedor que cobra por bytes.
- Uma chave que `OUTCOME_OK` não retorna não tem custo no mês dela, então o servidor registra uma revisão de 0 para ela. Um plugin que não conseguiu ler um provedor deve informar uma falha, não omitir as linhas.
- As chamadas podem se repetir: uma nova tentativa de uma importação que falhou é a mesma chamada de novo.

`maxRequests` 0 proíbe qualquer requisição cobrada. O servidor não faz chamada quando enviaria 0, mas `my plugin test --kind billing` faz, com o campo omitido como o mapeamento do proto3 permite, e espera `OUTCOME_REQUEST_CAP` sem linhas e sem requisições.

<!-- source: kinds-of-failure-2 205a6c0ecdd9 -->
### Tipos de falha

Um plugin informa uma falha em `outcome`. O servidor mantém as `requests` de uma falha, ignora as linhas dela e deixa as importações anteriores como estão, como nas outras origens.

| Resultado | Tipo | Nova tentativa |
| --- | --- | --- |
| `OUTCOME_PERMISSION` | `permission` | Não |
| `OUTCOME_NOT_ENABLED` | `not_enabled` | Não |
| `OUTCOME_NOT_FOUND` | `not_found` | Não |
| `OUTCOME_THROTTLED` | `throttled` | Sim |
| `OUTCOME_REQUEST_CAP` | `request_cap` | Não |
| `OUTCOME_BYTES_CAP` | `bytes_cap` | Não |
| `OUTCOME_INVALID_RESPONSE` | `invalid_response` | Não |
| `OUTCOME_UNAVAILABLE` | `unavailable` | Sim |
| Um resultado ausente ou desconhecido, ou uma linha ou contagem que quebra as regras acima | `invalid_response` | Não |
| Um erro HTTP ou Connect, um prazo esgotado, uma resposta acima de 4 MiB, nenhuma resposta | `unavailable` | Sim |

O que não é repetido espera pela próxima importação diária. O motivo de uma falha está no log do plugin, já que o servidor descarta a mensagem de um erro, e ela também conta em [`my.plugin.failures`](running.md).

<!-- source: test-it 10a2c62a53d8 -->
### Testar

```sh
my plugin test --kind billing unix:///run/my/billing.sock
my plugin test --kind billing --billed unix:///run/my/billing.sock
```

O primeiro não faz nenhuma requisição cobrada se o plugin segue o contrato. O segundo também faz uma busca real, que pode custar dinheiro nos provedores do plugin. Veja [Testar um plugin](plugins.md#test-a-plugin).
