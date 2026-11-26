<!-- source: deploying-on-google-cloud 03b87618467f -->
# Implantar no Google Cloud

Como executar o `my` no Google Cloud com o módulo do OpenTofu `mycroblog-gcp`: um serviço do Cloud Run e um backend para cada função,
que você escolhe pelo `kind` da seção dela. Toda seção usa por padrão a recomendação do módulo, que não executa nenhuma instância
enquanto ninguém usa o servidor, exceto por um tick que o acorda a cada minuto. Um servidor custa dinheiro: a configuração recomendada com um
banco de dados no Neon sai por cerca de US$ 37 por mês antes do armazenamento e do tráfego, e [Custos](#costs) dá o valor de cada escolha.

O que toda implantação com o OpenTofu tem em comum, o bloco e as seções dele, os segredos, o console e as atualizações, está em
[Implantar com o OpenTofu](deploy-opentofu.md). Esta página acrescenta o que o Google Cloud precisa.

<!-- source: what-it-creates 7b13c8bbc968 -->
## O que ele cria

Um bloco de `mycroblog-gcp` cria o seguinte. O primeiro tipo de cada linha é o recomendado e o padrão;
[a referência dos módulos](../../deploy/opentofu/README.md#sections) lista todos os campos.

| Função | Seção | Recomendado | Outros tipos |
| --- | --- | --- | --- |
| Servidor | `service`, `jobs` | Um serviço do Cloud Run com `role = all`, um job de migração, um job de console enquanto você precisar, uma conta de serviço e acesso público | |
| Tráfego | `ingress` | `load-balancer`: um Application Load Balancer externo global com um certificado gerenciado pelo Google | `domain-mapping`, `cloudflare` e um host de mídia em cache no Cloudflare ao lado de qualquer um |
| Banco de dados | `database` | `external`: um segredo para a URL de um PostgreSQL que você executa, como o Neon | `cloud-sql`: uma instância, o banco de dados, o usuário e a senha dela |
| Jobs | `queue` | `cloud-tasks`: uma fila do Cloud Tasks, o job do Cloud Scheduler do tick e uma conta de serviço que chama o servidor | `db`: os jobs no banco de dados |
| Mídia | `media` | `gcs`: um bucket privado do Cloud Storage | `r2`, `s3` |
| Caches e limites de taxa | `kv` | `db`: no banco de dados, sem nada a criar | `memorystore`, `external` (Valkey) |
| Atualizações em tempo real | `stream` | `pubsub`: um tópico e um papel personalizado com o qual o servidor gerencia as próprias assinaturas | `postgres`, `valkey` |
| E-mail | `mail` | Qualquer serviço SMTP, com um segredo para a senha dele | |
| KEK | `kek` | `secret-manager`: um segredo gerado, montado como um arquivo | `kms`: uma chave no Cloud KMS |
| Rede | `network` | `none`: nada a anexar | `vpc`, `existing` |
| Métricas, traces, erros | `observability` | Cloud Monitoring, Cloud Trace e Error Reporting, que leem o que o Cloud Run envia a eles. O módulo ativa as APIs deles | `settings`, como o Sentry |
| Alertas | `monitoring` | Uma verificação de disponibilidade, métricas baseadas em logs e políticas de alerta: [Monitoramento e alertas](#monitoring-and-alerts) | `null`, nenhum |

O módulo também gera a chave VAPID e cria vazios os segredos que só você pode preencher.

`examples/gcp-cloud-run-neon` é um jeito de começar: o provedor e um bloco com todos os tipos recomendados, que funciona com
qualquer banco de dados externo, Neon ou não. Adicione ao bloco dele as seções que você mudar.

<!-- source: before-you-start 9869afda9a07 -->
## Antes de começar

- O que [a primeira implantação](deploy-opentofu.md#first-deployment) precisa, com `gcloud` como a CLI da plataforma e
  `gcloud auth application-default login` para o provedor.
- Um projeto do Google Cloud com faturamento, usado apenas para esta instância, e uma conta nele com o papel Owner. O papel
  personalizado de `stream = pubsub` vai no projeto, então alcançaria as assinaturas de qualquer outra coisa que estiver ali.

<!-- source: first-deployment 894b6af2cee9 -->
## Primeira implantação

Faça aqui os dois primeiros passos e depois siga [a primeira implantação](deploy-opentofu.md#first-deployment) com as notas do
terceiro.

<!-- source: 1-enable-the-apis fdd2dc400c20 -->
### 1. Ative as APIs

Toda implantação precisa destas:

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

Adicione a API de cada escolha que precisar de uma:

| Escolha | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` ou `existing` | `compute.googleapis.com`, `servicenetworking.googleapis.com`, `networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`, `serviceconsumermanagement.googleapis.com` e as da rede |

`translation` ativa a própria API, e `observability` também. [Importar custos de uso da nuvem](billing-import.md) lista o que
`billing` precisa.

<!-- source: 2-prepare-the-database b54e3b01002b -->
### 2. Prepare o banco de dados

- **`external`**: crie o banco de dados e guarde a URL de conexão direta dele e, se tiver uma, a URL com pool para
  `secrets_to_fill`: [O banco de dados](#the-database) diz qual vai onde. O módulo nunca o cria, então a senha dele fica
  fora do estado do OpenTofu.
- **`cloud-sql`**: nada a fazer. O módulo cria a instância e grava a URL dela, com uma senha gerada, no segredo.

<!-- source: 3-deploy 4470affd40e4 -->
### 3. Implante

Siga [a primeira implantação](deploy-opentofu.md#first-deployment). No Google Cloud:

- **Variáveis**: `mail_from`, `mail_smtp_host` e `mail_smtp_username` do exemplo configuram o [e-mail](email.md). A porta
  é a 587, já que o Google Cloud bloqueia a porta de saída 25.
- **Segredos**: a KEK é `my-kek` e a chave VAPID `my-vapid-key`, cada um começando com `name`, e
  `gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` os copia. A KEK chega ao servidor como um
  arquivo montado, nomeado por `MY_KEK_FILE`, e os outros segredos como variáveis de ambiente.
- **Apply**: o job de migração vem primeiro, com o que ele precisa, o que inclui o ingress, já que as configurações do servidor
  o nomeiam. A sonda de inicialização do serviço espera `/health/ready`, que responde `503` até o esquema existir.
- **DNS**: adicione os registros de `tofu output dns_records`, como [Como o tráfego chega ao serviço](#how-traffic-reaches-the-service)
  diz para cada tipo. Até o certificado ficar ativo, `https://{host}` falha com um erro de TLS, então a federação e os navegadores
  ainda não alcançam o servidor. Nada mais espera por isso: o Cloud Tasks e o Cloud Scheduler chamam a URL padrão do serviço,
  `https://<name>-<project number>.<region>.run.app`. **Não desative a URL padrão**, ou os jobs param. O Cloud Run conta
  esses dois chamadores como internos apenas do mesmo projeto e nessa URL, o que permite que um balanceador de carga a feche para
  a internet.
- **O primeiro administrador**: o link fica no Cloud Logging, onde qualquer pessoa que possa ler os logs do projeto pode usá-lo.

<!-- source: the-console-and-upgrades a40a1df56865 -->
## O console e as atualizações

[O console](deploy-opentofu.md#the-console) e as [atualizações](deploy-opentofu.md#upgrading) executam execuções dos jobs do Cloud Run
`<name>-console` e `<name>-migrate` a partir de onde você aplica, por meio de `gcloud`. Elas precisam de credenciais que possam executar
os jobs e ler os logs deles, `roles/run.developer` e `roles/logging.viewer`, que o Owner inclui. Um apply interrompido
deixa a execução rodando no Cloud Run, e o próximo apply espera que ela termine.

Para [verificar o SQL primeiro](deploy-opentofu.md#checking-the-sql-first):

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: the-database 0303fdd046ef -->
## O banco de dados

O módulo recomenda `external`, que não precisa de rede e não cobra nada no Google Cloud: um banco de dados que para quando ocioso,
como o Neon, mantém barato um servidor tranquilo. `cloud-sql` mantém tudo em um projeto, a um custo fixo. Mover dados entre
eles é [uma troca de backend](deploy-opentofu.md#changing-a-backend).

<!-- source: an-external-postgresql d6604bfb9e2f -->
### Um PostgreSQL externo

`database = { kind = "external" }`, o padrão, aceita qualquer PostgreSQL que o Cloud Run consiga alcançar: o Neon, outro PostgreSQL
hospedado ou um que você executa, pela internet ou em um endereço privado por [uma rede](#networks). O módulo cria o
segredo `my-database-url`, para a URL **direta**, que o job de migração e o console sempre usam: `my schema apply` mantém
um lock em nível de sessão, que um pooler em modo de transação não preserva.

`database = { kind = "external", pooled = true }` adiciona o segredo `my-database-url-pooled`, para uma URL **com pool**, que o
serviço então usa. Um pooler serve ao servidor, como [Por um pooler de conexões](database.md#through-a-connection-pooler)
diz. Sem um, o serviço usa também a URL direta, então `service.max_instances` vezes o pool de cada instância deve ficar
abaixo do limite de conexões do banco de dados. O pool é de 4 conexões por instância por padrão, para as 80 requisições que uma instância
aceita ao mesmo tempo: aumente-o com `pool_max_conns` na URL que o serviço usa, como `...?sslmode=require&pool_max_conns=8`
([Esperar por uma conexão com o banco de dados](running.md#waiting-for-a-database-connection)).

**No Neon**, crie um projeto em uma região próxima da sua região do Cloud Run. A string de conexão direta dele tem o host simples, e a
com pool o host com `-pooler`, que é o pooler do Neon e serve ao servidor: defina `pooled = true`. Espere o plano Launch,
já que o tick padrão mantém o compute acordado; o plano Free funciona com [os compromissos em Custos](#where-care-reduces-it).

<!-- source: cloud-sql dc9461df8cc2 -->
### Cloud SQL

`database = { kind = "cloud-sql" }` cria uma instância PostgreSQL 18, `db-f1-micro` por padrão, com backups diários e
recuperação para um ponto no tempo mantida por 7 dias, o banco de dados e o usuário dela e uma senha gerada que não chega nem ao plano nem ao
estado. `tier`, `disk_size_gb`, `retained_backups` e `deletion_protection` a alteram. Nunca foi aplicado: trate a
primeira implantação como o teste dele.

- **Sem uma rede**, o Cloud Run o alcança pelo socket do conector do Cloud SQL, e o endereço público da instância não admite
  mais nada.
- **Com uma rede**, ele tem apenas um endereço privado, e um `pg_dump` deve ser executado de dentro da rede.
- **Ele não tem pooler**: `pooled` é recusado, e `service.max_instances` e `pool_max_conns` limitam as conexões. Mantenha o
  pool pequeno no `db-f1-micro`, que tem 0,6 GB de memória.
- **Ele nunca escala a zero**, então cobra qualquer que seja o tráfego, e o SLA do Google não cobre um nível de núcleo compartilhado.

<!-- source: the-other-backends 91ef8e2dea11 -->
## Os outros backends

Cada um tem a recomendação dele como padrão, e [uma troca de `kind`](deploy-opentofu.md#changing-a-backend) passa para outro.

- **Jobs**: `cloud-tasks` envia cada job ao serviço, que pode escalar a zero. `queue.tick_schedule`, a cada minuto por
  padrão, acorda o servidor e o banco de dados para os jobs periódicos. `db` consulta o banco de dados a partir de um processo que permanece no ar, então
  precisa de `service.min_instances` igual a 1 ou mais e de `service.cpu_always_allocated = true`.
- **Mídia**: `gcs` cria um bucket privado, `media.bucket_name`, na região. `r2` cria um bucket R2 em
  `cloudflare.account_id`, e `s3` usa qualquer armazenamento compatível com S3 em `endpoint`; os dois leem uma chave de acesso dos segredos
  que `secrets_to_fill` lista. Para compartilhar um bucket com outras instâncias, `existing_bucket = true` usa o seu e `prefix`
  (`tenant-a/`) inicia o nome de todo objeto; com `gcs`, a conta de serviço então alcança apenas os objetos sob ele, o que exige
  acesso uniforme em nível de bucket.
- **Caches e limites de taxa**: `db` os mantém no banco de dados. `memorystore` cria o Memorystore para Valkey, que precisa de uma
  rede e não tem senha: qualquer coisa na rede pode alcançá-lo. `external` lê de um segredo a URL de um Valkey que você executa.
- **Atualizações em tempo real**: `pubsub` alcança todas as instâncias. `postgres` usa `LISTEN` e `NOTIFY` pela conexão direta do banco de dados,
  e `valkey` o Valkey de `kv`.
- **A KEK**: `secret-manager` a gera uma vez, para você copiar. `kms` a mantém no Cloud KMS, de onde ela nunca sai, então um
  dump do banco de dados vazado e uma configuração vazada juntos ainda não expõem nenhum segredo. Mudá-la em uma instância com dados é
  [uma mudança de lugar](deploy-opentofu.md#the-kek).

<!-- source: networks 9d28311e11a6 -->
## Redes

A configuração recomendada não precisa de nenhuma: um banco de dados externo, o Cloud Tasks, o Cloud Storage e o Pub/Sub são todos alcançados pela
internet com a identidade do próprio serviço, então o Cloud Run não se anexa a nada.

Você precisa de uma para o Memorystore, que só tem endereços privados, para dar um endereço privado ao Cloud SQL, ou para alcançar um banco de dados
seu em uma. `network = { kind = "vpc" }` cria uma VPC, as sub-redes dela e o acesso a serviços privados, e
`network = { kind = "existing", ... }` se anexa a uma VPC que você tem, que você descreve por `network`, `subnetwork`,
`private_services_range` e `memorystore_connection_policy`:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

O serviço e os jobs se anexam pelo Direct VPC egress, e apenas os intervalos privados passam pela rede, então a internet
mantém o próprio caminho, sem Cloud NAT. Com `vpc`, `nat = true` envia tudo pelo Cloud NAT, para um endereço de
origem estático. Uma rede pode atrasar o início de uma instância em um minuto ou mais, o que um servidor que escala a zero encontra
com frequência. A VPC que o módulo cria nunca foi aplicada.

<!-- source: how-traffic-reaches-the-service 094ddaf201c8 -->
## Como o tráfego chega ao serviço

`ingress.kind` decide isso. O módulo recomenda o balanceador de carga, para produção. O mapeamento de domínio serve para experimentar uma
implantação de forma barata.

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| Tarifa mensal | Cerca de US$ 18 pelas regras de encaminhamento, mais uma cobrança por GB. Confira a [página de preços](https://cloud.google.com/vpc/network-pricing): este valor não pôde ser confirmado lá | Nenhuma |
| Status | O jeito recomendado do Google Cloud | Um recurso em Preview que o Google Cloud não recomenda para produção, por causa da latência |
| Regiões | Qualquer uma | `asia-northeast1`, `us-central1` e algumas outras |
| DNS | Um registro `A` de `host` para o endereço dele. Deixe desativado qualquer proxy do seu provedor de DNS para ele | Os registros que `dns_records` lista |
| Certificado | Gerenciado pelo Google, ativo quando o DNS aponta para o endereço, o que pode levar um tempo | Gerenciado pelo Google, até 24 horas |
| Propriedade do domínio | Nada além do DNS | Quem aplica deve primeiro ser um [proprietário verificado](https://cloud.google.com/run/docs/mapping-custom-domains) do domínio |
| URL `run.app` | Recusada pela internet, então ninguém contorna o balanceador de carga | Acessível por qualquer pessoa: um mapeamento de domínio não pode adicionar um cabeçalho, então a origem fica aberta |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` e o endereço do balanceador de carga | `169.254.0.0/16` |

O Cloud Tasks e o Cloud Scheduler não chegam por nenhum dos dois: eles chamam diretamente a URL `run.app` padrão.

<!-- source: cloudflare 35dc1e54d2bb -->
### Cloudflare

`ingress = { kind = "cloudflare" }` coloca o Cloudflare na frente de `host`, com o cache ignorado para tudo, e
`ingress.media_host` adiciona um host de mídia em cache ao lado de qualquer tipo. Os dois precisam de `cloudflare = { zone_id = "..." }`, o provedor
lê o token dele de `CLOUDFLARE_API_TOKEN`, e o módulo cria os registros, então `dns_records` fica vazio. Eles só foram executados
com provedores simulados.

- **O seu plano decide a origem.** O Cloud Run roteia por `Host`, e o Cloudflare só substitui o cabeçalho Host no plano
  Enterprise. Em qualquer outro plano, o módulo cria um mapeamento de domínio de cada host, com os compromissos da tabela acima, e
  o Cloudflare se conecta a ele. Não foi verificado se o certificado dele é emitido e renovado atrás do proxy, então comece com
  `cloudflare.proxied = false`. No plano Enterprise, `cloudflare.host_header_override = true` se conecta à URL `run.app`
  no lugar, sem mapeamento de domínio.
- **Um cabeçalho fecha a origem.** Uma regra de transformação de cabeçalho de requisição adiciona um valor que o módulo gera, e o servidor
  recusa uma requisição sem ele com um `403`, exceto as verificações de saúde e os próprios `POST`s dos jobs, que verificam os tokens deles.
  Assim ninguém contorna o Cloudflare pela URL `run.app`, e ninguém pode escolher o endereço de cliente que os limites de taxa veem.
- **Nada adiciona o cabeçalho enquanto um registro não passa pelo proxy**, então o servidor recusa tudo, exceto as verificações de saúde:
  com `proxied = false` para o certificado de um mapeamento de domínio, ative o proxy quando o certificado estiver ativo.
- **O valor vai parar no estado**, como [Segredos](deploy-opentofu.md#secrets) diz, e no segredo `my-origin-secret`.
  [O guia da CDN](cdn.md#rotate-the-value) explica como rotacioná-lo.
- **Desative o Bot Fight Mode** no painel e não ative “I'm Under Attack”: um desafio em `/ap/`, `/.well-known/`
  ou `/api/` quebra a federação, já que os outros servidores não são navegadores. Os dois cobrem a zona inteira, que nenhuma regra pode isentar.
  O módulo não adiciona nenhum desafio e desativa o Browser Integrity Check.
- **A ofuscação de endereços de e-mail pode permanecer ativada.** As páginas do próprio servidor marcam o conteúdo cujos endereços ele deve deixar em paz.
- **Ative o Always Use HTTPS** no painel se quiser que `http://` seja redirecionado. Ele cobre a zona inteira, então o módulo
  o deixa para você.
- **Timeouts**: o Cloudflare responde `524` quando a origem não envia nada por 125 segundos. Os jobs não são afetados, já que não
  passam pelo Cloudflare. Os streams enviam um heartbeat a cada 30 segundos, abaixo do limite de inatividade de 900 segundos. Não foi verificado se o Cloudflare
  armazena os streams em buffer em um host com proxy.
- **Vídeo**: os termos do Cloudflare restringem servir vídeo e uma quantidade desproporcional de arquivos grandes nos planos Free, Pro e
  Business, e ele pode agir sobre o conteúdo. Decida isso antes de armazenar vídeo em cache no host de mídia.

**Um arquivo excluído permanece em cache.** O host de mídia segue o `Cache-Control` do servidor, então um arquivo local excluído pode continuar visível
por até um ano, e um remoto por `remote_media_cache_ttl_seconds`. O servidor não o limpa. Quando algo precisar
desaparecer, limpe-o manualmente, em **Caching > Configuration > Custom Purge** no painel, ou:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Uma URL de proxy guarda o `sig` dela na query string, que faz parte do que é armazenado em cache, então informe a URL inteira.

<!-- source: monitoring-and-alerts eb28e653de36 -->
## Monitoramento e alertas

`monitoring` adiciona uma verificação de disponibilidade de `/health/live`, métricas baseadas em logs e políticas de alerta com os nomes do
[runbook](alerts.md), que notificam os endereços de `notification_emails` (`alert_emails` no exemplo) ou os canais
de `notification_channels`. A seção não tem padrão: nomeie quem é notificado, ou defina `monitoring = null` para deixar tudo isso
de fora. Ela nunca foi aplicada, então teste-a uma vez fazendo uma verificação falhar. Cada execução da verificação acorda uma instância:
`uptime_period`, 15 minutos por padrão, é a frequência.

<!-- source: traces da2e58dc6a3a -->
### Traces

O Cloud Trace mostra o trace que o Cloud Run registra de cada requisição, e as linhas de log do servidor trazem o `trace_id` dele, já que o
módulo confia no ingress (`MY_TRUSTED_PROXIES`). As linhas não preenchem o campo de trace que o Cloud Logging vincula
(`logging.googleapis.com/trace`), e não foi verificado se o Cloud Run sempre substitui o `traceparent` de uma requisição pública.
Os próprios spans do servidor exigiriam um OpenTelemetry Collector ao lado dele, que o módulo não executa.

<!-- source: limits 3d4721415f18 -->
## Limites

- **Envios**: o Cloud Run recusa uma requisição HTTP/1 acima de 32 MiB, e o servidor fala HTTP/1.1, então o módulo define
  `MY_MEDIA_MAX_AV_BYTES` como 30 MiB, contra o padrão de 100 MiB, para que um envio maior receba a resposta do próprio servidor.
  O limite do próprio Cloudflare, 100 MB no Free e no Pro, está acima dele.
- **Streams**: `service.request_timeout_seconds`, 15 minutos por padrão, encerra um stream aberto, e o cliente se conecta de novo.
- **Conexões com o banco de dados**: o Google limita uma instância do Cloud Run a 100 conexões com o Cloud SQL.
- **Endereços de clientes**: os limites de taxa dependem de `MY_TRUSTED_PROXIES`, que o módulo define a partir do ingress. Nenhum documento que
  lemos diz o que o Cloud Run acrescenta a `X-Forwarded-For` em uma requisição direta nem qual endereço de par um contêiner vê, então
  `169.254.0.0/16` é uma suposição. A forma documentada do balanceador de carga é `[<supplied>,]<client-ip>,<lb-ip>`. Verifique os
  endereços nos primeiros logs de requisições: um endereço de cliente em `169.254` ou do Google significa que a configuração está errada para o seu projeto.
- **Acesso público**: o serviço concede `roles/run.invoker` a `allUsers`, o que uma política da organização de compartilhamento
  restrito a domínios recusa. Um projeto assim precisa de uma exceção.
- **Nomes que continuam ocupados**: o Cloud SQL e o Cloud Tasks recusam o nome de uma instância ou fila excluída por cerca de uma semana, e um
  papel personalizado excluído mantém o ID dele por um tempo, então uma implantação refeita depois de um `destroy` precisa de outro `name`.

<!-- source: costs ea658d24d79d -->
## Custos

Estes valores são um guia do que esperar e de onde o cuidado reduz o custo, não limites a que a implantação esteja sujeita. São
preços de tabela aproximados para uma região, de 2026-10 e 2026-11: confira-os nas páginas de preços do
[Cloud Run](https://cloud.google.com/run/pricing), do [Cloud SQL](https://cloud.google.com/sql/pricing) e do
[Neon](https://neon.com/pricing). O compute do Cloud Run é gratuito por conta de faturamento até 180.000 vCPU-segundos e 360.000
GiB-segundos por mês.

O que toda implantação paga, cerca de US$ 0 em repouso:

| Parte | Em repouso | O que acrescenta |
| --- | --- | --- |
| Cloud Run | US$ 0: nenhuma instância executa, e a CPU é cobrada apenas durante as requisições | Cada stream aberto mantém uma instância cobrada; veja abaixo |
| Cloud Tasks | US$ 0 | 1 milhão de operações grátis, depois US$ 0,40 por milhão. Um job leva cerca de 2 operações (enfileirá-lo e entregá-lo) |
| Cloud Scheduler | US$ 0: o tick é um job, e 3 são grátis por conta de faturamento | US$ 0,10 por job acima disso |
| Pub/Sub | US$ 0: 10 GiB grátis | Apenas enquanto há streams abertos |
| Secret Manager | Centavos | Algumas versões grátis, depois alguns centavos por versão |
| Cloud Storage | Cerca de US$ 0,02 por GiB por mês | 5 GiB são grátis apenas em `us-east1`, `us-west1` e `us-central1`. A mídia sai pelo Cloud Run |
| Monitoring | US$ 0 dentro das cotas gratuitas | |

O que cada escolha acrescenta:

| Seção | Escolha | Por mês |
| --- | --- | --- |
| `ingress` | `load-balancer` | Cerca de US$ 18, não confirmado na página de preços, mais uma cobrança por GB |
| | `domain-mapping` | US$ 0 |
| | `cloudflare` | O seu plano do Cloudflare, e a origem: US$ 0 com um mapeamento de domínio |
| `database` | `external` no plano Launch do Neon | Cerca de US$ 19 com o tick padrão, mais US$ 0,35 por GB-mês |
| | `external` no plano Free do Neon | US$ 0, dentro dos limites dele: [abaixo](#where-care-reduces-it) |
| | `external` em outro lugar | O que esse banco de dados custa |
| | `cloud-sql` | Cerca de US$ 9,40 para `db-f1-micro` e 10 GiB em `us-central1`, mais os backups, qualquer que seja o tráfego |
| `queue` | `db` | Uma instância cobrada o dia todo, com a CPU sempre alocada |
| `kv` | `memorystore` | Cerca de US$ 23 pelo menor nó, qualquer que seja o tráfego |
| `kek` | `kms` | Centavos, pela chave e pelas operações dela |
| `network` | `vpc` com `nat = true` | Cloud NAT, por hora e por GB |

A configuração recomendada no plano Launch do Neon, o balanceador de carga e o Neon, sai por cerca de US$ 37 por mês; com o Cloud SQL, por
cerca de US$ 27.

**O tick mantém o Neon acordado.** Cada tick lê e grava no banco de dados, e o Neon só para um compute depois de 5 minutos sem
atividade, então com o `* * * * *` padrão o compute nunca para. São 730 horas por mês, ou 182,5 CU-horas a 0,25 CU,
que são os US$ 19 acima. O tick permanece a cada minuto para que os jobs periódicos executem no horário, e um sinal de despertar perdido é
recuperado logo. O Cloud SQL cobra o mesmo, qualquer que seja o tick.

**Um stream aberto mantém uma instância cobrada.** Na cobrança por requisição, uma instância é cobrada enquanto atende qualquer requisição,
e as atualizações em tempo real são uma requisição longa. Um cliente que mantém o stream aberto o mês inteiro custa uma instância o
dia todo: cerca de US$ 60 por mês com 1 vCPU e 512 MiB, depois da cota gratuita. Mais streams compartilham a instância, até
`service.concurrency`. Se os streams ficam abertos na maior parte do tempo, compare com a cobrança por instância
(`service.cpu_always_allocated`), ou com uma VM pequena e o [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 427be9d46d32 -->
### Onde o cuidado reduz o custo

- **O mapeamento de domínio** no lugar do balanceador de carga tira os US$ 18, com
  [os compromissos dele](#how-traffic-reaches-the-service). Serve para experimentar uma implantação.
- **O plano Free do Neon** tira os US$ 19, e uma instância pequena pode executar nele, com estes compromissos:
  - Ele permite 100 CU-horas por mês, que o tick padrão esgota depois de cerca de 17 dias. Fique dentro dele com uma
    programação mais longa, como `queue = { tick_schedule = "*/10 * * * *" }`: no máximo metade do tempo acordado, cerca de 91 CU-horas, antes de
    qualquer outra coisa acordá-lo. Os jobs periódicos então executam com até 10 minutos de atraso, e um sinal de despertar perdido é recuperado mais tarde.
  - Um servidor federado recebe entregas o dia todo, o que também acorda o banco de dados, então uma instância movimentada o ultrapassa
    qualquer que seja a programação.
  - Ele guarda 1 GB por projeto e um histórico de 6 horas para restaurar, e o compute dele sempre para depois de 5 minutos ocioso, então
    a primeira requisição depois disso espera por ele.
