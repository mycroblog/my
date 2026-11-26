<!-- source: deploying-on-aws d7e8643cd93d -->
# Implantar na AWS

Como executar o `my` na AWS com o módulo do OpenTofu [`mycroblog-aws`](../../deploy/opentofu/README.md#aws): ECS no Fargate atrás de
um Application Load Balancer, os jobs no banco de dados, S3, SNS e e-mail pelo Amazon SES. Nada escala a zero: espere
cerca de US$ 42 por mês em repouso, antes do banco de dados, e leia [Custos](#costs).

**Nada disso jamais foi aplicado na AWS.** As verificações o planejam com provedores simulados e sem credenciais, o que mostra que
a configuração é consistente, não que a AWS a aceite nem que o servidor inicie. Trate uma primeira implantação como um teste.

O que toda implantação com o OpenTofu tem em comum, as entradas do módulo, os segredos, o console e as atualizações, está em
[Implantar com o OpenTofu](deploy-opentofu.md). Esta página acrescenta o que a AWS precisa.

<!-- source: what-it-creates 1ca642b05ab0 -->
## O que ele cria

Um bloco de `mycroblog-aws` cria tudo isto. Cada linha é uma seção do módulo, e o padrão dela é a configuração
recomendada; [a referência do módulo](../../deploy/opentofu/README.md#sections-of-mycroblog-aws) lista todos os campos.

| Parte | Seção | Padrão | Outros tipos |
| --- | --- | --- | --- |
| Servidor | `service`, `jobs` | Um cluster e um serviço do ECS no Fargate com `role = all`, definições de task para a migração e, enquanto você precisar, para o console, e um grupo de logs `/ecs/<name>` | |
| Rede | `network` | Uma VPC em duas ou mais zonas, com uma sub-rede pública e uma privada em cada | Uma VPC sua: [Redes](#networks) |
| Tráfego | `ingress` | Um Application Load Balancer, sempre presente, com o seu certificado | CloudFront ou Cloudflare na frente dele: [Como o tráfego chega ao serviço](#how-traffic-reaches-the-service) |
| Jobs | | Nenhum: as tasks permanecem no ar e pegam os jobs do banco de dados | |
| Banco de dados | `database` | Um segredo para a URL de um PostgreSQL que você executa, como o Neon | Amazon RDS: [Escolha o banco de dados](#2-choose-the-database) |
| Mídia | `media` | Um bucket S3 privado | Cloudflare R2, ou qualquer armazenamento compatível com S3 |
| Caches e limites de taxa | `kv` | Nenhum: o banco de dados os guarda | ElastiCache Serverless para Valkey, ou um Valkey que você executa |
| Atualizações em tempo real | `stream` | Um tópico SNS. O servidor cria e remove as próprias filas SQS | O banco de dados, ou o Valkey de `kv` |
| E-mail | `mail` | A identidade do SES do domínio de `mail.from`: [E-mail pelo Amazon SES](#email-through-amazon-ses) | Qualquer serviço SMTP |
| Segredos | | Secrets Manager: uma chave VAPID gerada, e os segredos que só você pode preencher, vazios | |
| KEK | `kek` | Um segredo gerado no Secrets Manager | Uma chave no AWS KMS, que nunca sai dele |
| Métricas, traces, erros | `observability` | Métricas e relatórios de erros no CloudWatch, traces para o X-Ray por um coletor em cada task: [Traces para o X-Ray](#traces-to-x-ray) | Seus próprios exportadores, por `settings` |
| Alertas | `monitoring` | Um canary do CloudWatch Synthetics e alarmes: [Monitoramento e alertas](#monitoring-and-alerts) | Nenhum, com `null` |

`translation = {}` adiciona o Amazon Translate, `search = { domain_name = "..." }` um domínio do Amazon OpenSearch Service que você
criou, e `billing = {}` a importação do custo de uso do Cost Explorer: [Traduzir publicações](translation.md#amazon-translate),
[Mecanismos de busca](search-engines.md) e [Importar custos de uso da nuvem](billing-import.md).

<!-- source: before-you-start 9d5614a650e1 -->
## Antes de começar

- O que [a primeira implantação](deploy-opentofu.md#first-deployment) precisa, com `aws` como a CLI da plataforma.
- Uma conta da AWS, usada apenas para esta instância, e credenciais dela que possam criar tudo o que foi listado acima, como
  `AdministratorAccess`.
- Uma região. O SES envia a partir dela, e o módulo a toma do provedor `aws`.
- Recomendado: uma zona hospedada pública de `host` no Route 53, onde o módulo então cria o registro de `host`, inclusive no apex
  de uma zona.

<!-- source: first-deployment 26b562a3db03 -->
## Primeira implantação

Faça aqui os três primeiros passos e depois siga [a primeira implantação](deploy-opentofu.md#first-deployment) com as notas do
quarto.

<!-- source: 1-request-the-certificates 3b26d03ba06c -->
### 1. Solicite os certificados

O balanceador de carga precisa de um certificado validado para `host` no AWS Certificate Manager, na região. O módulo o recebe como um
ARN, já que validá-lo dentro do apply seguraria o apply até você adicionar registros que só o apply imprime.

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

Com `ingress.kind = cloudfront`, solicite um segundo em `us-east-1`, a única região de que o CloudFront aceita um; em `us-east-1`
um certificado serve aos dois. Com `ingress.media_host`, o certificado também deve cobrir esse host.

<!-- source: 2-choose-the-database d9cc7b8b2210 -->
### 2. Escolha o banco de dados

`database.kind` o escolhe. De um jeito ou de outro, as tasks consultam o banco de dados em busca de jobs cerca de uma vez por segundo, então ele nunca descansa.

**`external`, o padrão e o recomendado**, lê a URL de um PostgreSQL que você executa, como o [Neon](https://neon.com),
de um segredo que você preenche. O módulo nunca cria o banco de dados, então a senha dele fica fora do estado do OpenTofu. No Neon,
crie um projeto em uma região próxima da sua e copie as strings de conexão:

- A **direta** (o host sem `-pooler`) é obrigatória, no segredo `<name>-database-url`. A migração e o
  console sempre a usam: `my schema apply` mantém um lock em nível de sessão, que um pooler em modo de transação não preserva.
- A **com pool** (o host com `-pooler`) é opcional, para o serviço: defina `database = { kind = "external", pooled = true }`,
  e o módulo cria o segredo `<name>-database-url-pooled`. Veja
  [Por um pooler de conexões](database.md#through-a-connection-pooler).

Espere o plano Launch do Neon: como o compute nunca para, as horas de compute do plano Free acabam dentro do mês.

**`rds`** cria o Amazon RDS para PostgreSQL nas sub-redes privadas, com uma senha que o módulo gera e grava em
`<name>-database-url` por conta própria, então não há nada a preencher:

```hcl
  database = { kind = "rds" }
```

Por padrão é `db.t4g.micro` com 20 GB (`instance_class`, `allocated_storage_gb`), mantém 7 dias de backups automáticos e
tem proteção contra exclusão. Ele não tem endereço público, então só as tasks o alcançam: um `pg_dump` precisa ser executado de dentro da VPC e,
antes de uma atualização, um snapshot toma o lugar dele,
`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade`. Ele não tem URL com pool.

<!-- source: 3-write-the-block db167b7d9566 -->
### 3. Escreva o bloco

`examples/aws-ecs-neon` é um jeito de começar: o provedor `aws` e um bloco com os padrões, cujas variáveis
`terraform.tfvars.example` lista. Seja qual for o ponto de partida, o bloco precisa disto:

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

- `network.egress` não tem padrão, já que as duas saídas são cobradas por hora: leia [Redes](#networks).
- `ingress.route53_zone_id` nomeia a zona hospedada de `host`, se você tiver uma.
- `media.bucket_name` deve ser único em toda a AWS.
- `monitoring` nomeia quem os alarmes notificam, ou é `null`.

Adicione a seção do banco de dados que você escolheu e qualquer outra que você mude.

<!-- source: 4-deploy 891c5c7cef83 -->
### 4. Implante

Siga [a primeira implantação](deploy-opentofu.md#first-deployment). Na AWS, onde os comandos desta página usam o `name`
padrão, `my`:

- **Segredos**: eles estão no Secrets Manager, com o nome `<name>-...`. `tofu output kek` tem o comando que copia a KEK, e
  `aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` copia a chave VAPID. A
  KEK chega ao servidor como `MY_KEK`, já que o Fargate não monta nenhum arquivo. Uma task lê os segredos dela quando inicia: depois de você mudar
  um, execute `aws ecs update-service --cluster my --service my --force-new-deployment`, e o mesmo para cada worker,
  `my-<key>`.
- **Apply**: a definição de task da migração vem primeiro, com o que ela precisa, e os serviços só depois que a migração
  teve sucesso. A verificação do balanceador de carga consulta `/readyz`, que responde `503` até o esquema existir.
- **DNS**: `tofu output dns_records` lista o registro de `host`, a menos que o Route 53 ou o Cloudflare o crie, e os três registros
  DKIM do SES. No apex de uma zona sem o Route 53, o registro de `host` precisa do que o seu provedor de DNS chama de `ALIAS` ou
  `ANAME`, como `tofu output dns_note` diz.
- **E-mail**: o SES só envia para endereços que você verificou até a AWS conceder acesso à produção, então ninguém mais pode se cadastrar ainda:
  leia [E-mail pelo Amazon SES](#email-through-amazon-ses).
- **Alertas**: cada endereço de `monitoring.notification_emails` recebe do SNS um e-mail de assinatura, que ele deve confirmar.
- **O primeiro administrador**: o link fica no CloudWatch Logs, em `/ecs/<name>`, onde qualquer pessoa que possa ler os logs da conta
  pode usá-lo.

<!-- source: the-console-and-upgrades-on-aws 0e4bc0bdeff2 -->
## O console e as atualizações na AWS

[O console](deploy-opentofu.md#the-console) e as [atualizações](deploy-opentofu.md#upgrading) executam tasks avulsas do Fargate a partir de onde
você aplica, por meio de `aws`, e leem a saída delas no CloudWatch Logs. Um apply interrompido deixa a task em execução, e o próximo
apply espera que ela termine. Em uma atualização, toda definição de task recebe uma nova revisão com a imagem, que nada executa até um
serviço adotá-la; cada serviço então espera as novas tasks dele iniciarem, e no serviço padrão passarem pela verificação do
balanceador de carga, antes de as antigas saírem, e o circuit breaker de implantação devolve uma revisão cujas tasks não iniciam à
anterior.

As credenciais de onde você aplica, ou do job de CI que aplica, precisam, além do que o próprio apply precisa:

- para a migração: `ecs:RunTask`, `ecs:DescribeTasks`, `ecs:ListTasks`, `logs:FilterLogEvents` e `iam:PassRole` nos
  dois papéis das tasks;
- para o console, também: `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `ecs:ListTagsForResource` e
  `ecs:RegisterTaskDefinition`, já que o console primeiro toma a imagem do serviço em execução.

Para [verificar o SQL primeiro](deploy-opentofu.md#checking-the-sql-first), ative o console enquanto o bloco ainda nomeia a
release atual, depois aumente `ref`, aplique apenas a definição de task da migração e execute `my upgrade -dry-run` com ela:

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

<!-- source: monitoring-and-alerts 9b712c82cae2 -->
## Monitoramento e alertas

`monitoring` adiciona um canary do CloudWatch Synthetics de `/readyz`, métricas no CloudWatch e alarmes com os nomes do
[runbook](alerts.md#aws-ecs), que notificam os endereços de `notification_emails` ou os tópicos SNS de
`notification_topic_arns`. A entrada não tem padrão: nomeie quem é notificado, ou defina `monitoring = null` para deixar tudo isso de fora.
Cada execução do canary é cobrada, e `uptime_period_minutes`, 15 por padrão, é a frequência com que ele executa: leia [Custos](#costs).

<!-- source: choosing-the-backends b83753b1f860 -->
## Escolher os backends

Toda seção usa por padrão o tipo recomendado; [mude o `kind` de uma seção](deploy-opentofu.md#changing-a-backend) para
outro. O banco de dados está [acima](#2-choose-the-database).

| Seção | Recomendado | Outros tipos, e quando |
| --- | --- | --- |
| `media` | `s3`, um bucket na região, com `versioning` desativado | `r2` em `cloudflare.account_id`, ou `s3-compatible` em `endpoint`, cada um com uma chave de acesso em segredos que você preenche. Para compartilhar um bucket, `existing_bucket = true` usa o seu e `prefix` (`tenant-a/`) inicia o nome de todo objeto; com `s3`, o papel das tasks então alcança apenas os objetos sob ele |
| `kv` | `db`, que não custa nada a mais | `elasticache`, que é cobrado mesmo em repouso e fica nas sub-redes privadas, ou `external`, uma URL Valkey em um segredo que você preenche |
| `stream` | `sns`, que alcança todas as tasks | `postgres`, pela conexão direta do banco de dados, ou `valkey`, o Valkey de `kv` |
| `mail` | `ses`, sem senha | `smtp`, com `smtp.host`, `smtp.port` (587) e `smtp.username`, cuja senha vai em um segredo que você preenche |
| `kek` | `secrets-manager`, que você copia uma vez | `kms`, uma chave que nunca sai do AWS KMS. Em uma instância com dados, [mudá-lo](deploy-opentofu.md#the-kek) é uma mudança de lugar |
| `observability` | CloudWatch e X-Ray | `traces = "none"`, ou `"settings"` para qualquer sinal que você envie para outro lugar, como o Sentry |

`architecture = "arm64"` executa as tasks no Graviton, que custa menos; a imagem lançada tem as duas.

<!-- source: email-through-amazon-ses 0d8bc9f93296 -->
## E-mail pelo Amazon SES

`mail.kind = "ses"`, o padrão, envia com a API do SES na região da implantação, a partir do domínio de `mail.from`. O
módulo cria a identidade do domínio com Easy DKIM e permite às tasks `ses:SendEmail` nela, então não há senha, e
`mail.configuration_set` cria um conjunto de configurações e o aplica a toda mensagem, ou, com
`existing_configuration_set = true`, nomeia um que as instâncias compartilham; cada instância então age apenas sobre as notificações do
e-mail que enviou. Antes de o SES enviar aos seus usuários, adicione os
registros que `tofu output dns_records` lista e saia do sandbox:
[Enviar e-mail pelo Amazon SES](email-ses.md) tem os passos, e como o servidor fica sabendo de bounces e reclamações.

<!-- source: traces-to-x-ray e5ad1c35fdd5 -->
## Traces para o X-Ray

`observability.traces = "x-ray"` é o padrão, porque o coletor se encaixa na configuração recomendada sem custo extra. É o
coletor AWS Distro for OpenTelemetry, que executa ao lado do servidor em cada task, escuta apenas no loopback da task e
grava no X-Ray com o papel das tasks (`xray:PutTraceSegments`). Ele toma 32 unidades de CPU e 128 MiB de cada task, que a AWS
cobra apenas pelo tamanho da task, então a task do serviço padrão, de 256 unidades de CPU e 1.024 MiB, custa o mesmo e dá ao
servidor 224 unidades de CPU e 896 MiB. O `traces_sample_ratio` padrão, 1%, registra cerca de 30.000 traces por mês para 100.000
requisições por dia, abaixo dos 100.000 traces por mês que o X-Ray registra de graça.

O que custa é a imagem do coletor, baixada a cada início de uma task, e a memória do servidor. Defina `traces = "none"` quando
você não lê os traces, ou para uma task pequena demais para dispensar 128 MiB: o plano recusa um tamanho que deixe ao servidor menos de
128 unidades de CPU ou 512 MiB, em `service`, `jobs` ou um worker. O coletor não é essencial: se ele para, o servidor continua
sem traces. Que o X-Ray aceita os spans dele não foi verificado.

<!-- source: networks 36dc33a6cbef -->
## Redes

O ECS sempre precisa de uma VPC. `network.kind = "vpc"`, o recomendado, cria uma em `cidr_block` (`10.0.0.0/16`) em
`availability_zones`, pelo menos duas, de que o balanceador de carga e o RDS precisam: uma sub-rede pública e uma privada em cada zona, e um
internet gateway. Ela é somente IPv4, então o servidor não alcança nenhum par somente IPv6. `network.kind = "existing"` recebe uma VPC sua
no lugar: `vpc_id`, `vpc_cidr`, `public_subnet_ids`, `public_subnet_cidrs` e `private_subnet_ids`.

Uma task precisa de uma saída para a internet, para a imagem, o banco de dados, a federação e as APIs da AWS, e `network.egress` a escolhe.
O tráfego de entrada chega a uma task apenas pelo balanceador de carga, de qualquer jeito:

| | `public-ip` (recomendado) | `nat` |
| --- | --- | --- |
| Sub-redes das tasks | Públicas, com um endereço IPv4 público em cada task | Privadas |
| Cada hora | US$ 0,005 pelo endereço de cada task | Um NAT gateway, cerca de US$ 0,045, e o endereço dele |
| Cada GB | Nada pelo endereço | Cerca de US$ 0,045 pelo gateway, inclusive mídia e federação |
| Com `existing` | Nada a mais | Suas sub-redes privadas já devem alcançar a internet |

Use `public-ip`, a menos que uma política sua proíba sub-redes públicas: custa muito menos e não é mais aberto. `vpc` cria um NAT
gateway, na primeira zona: quando essa zona cai, as sub-redes privadas perdem a internet até ela voltar. O RDS e o
ElastiCache ficam nas sub-redes privadas com qualquer um dos dois, e não precisam de saída.

<!-- source: how-traffic-reaches-the-service d009efcf3ee2 -->
## Como o tráfego chega ao serviço

O balanceador de carga está sempre presente, com o certificado de `ingress.certificate_arn`, e as tasks admitem apenas ele.
`ingress.kind` decide o que fica na frente dele:

| | `load-balancer` (recomendado) | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` aponta para | O balanceador de carga | Uma distribuição do CloudFront | O Cloudflare, que se conecta ao balanceador de carga |
| Em cache | Nada | `/media/`, pelo `Cache-Control` do servidor | Nada em `host`; `media_host` adiciona um host em cache |
| Registros | O Route 53 os cria com `route53_zone_id`, ou `dns_records` os lista | O Route 53 com `route53_zone_id`, ou `dns_records` | O Cloudflare o cria |
| Precisa de | | `cloudfront_certificate_arn`, em `us-east-1`. `cloudfront_price_class` escolhe as edge locations | `cloudflare = { zone_id = "..." }` e o token em `CLOUDFLARE_API_TOKEN` |
| Fecha a origem por | O security group das tasks | O security group do balanceador de carga admite apenas o CloudFront, e o listener dele recusa uma requisição sem um cabeçalho que o CloudFront envia | Um cabeçalho que a regra do Cloudflare adiciona, que o servidor verifica |

Atrás do CloudFront ou do Cloudflare, o valor do cabeçalho vai parar no estado do OpenTofu, que deve permanecer privado. Um registro do
balanceador de carga de antes do CloudFront deve sair primeiro: um registro existente com o mesmo nome faz o apply falhar. Os
endereços do CloudFront vêm da AWS quando você planeja, então aplique de vez em quando, ou uma edge em um novo endereço compartilha os limites de taxa dos seus visualizadores.

`ingress.media_host` adiciona um host de mídia em cache no Cloudflare, ao lado de `load-balancer` ou `cloudflare`, com
`cloudflare.zone_id`. Um arquivo excluído permanece em cache até você limpá-lo: leia [Servir mídia por uma CDN](cdn.md). Com o Cloudflare
na frente de qualquer coisa, desative o Bot Fight Mode no painel dele, já que um desafio em `/ap/`, `/.well-known/` ou `/api/` quebra a
federação, e o módulo não o gerencia. Os tipos do Cloudflare só foram executados com provedores simulados.

<!-- source: limits 42ed50837851 -->
## Limites

- **Saúde**: a única verificação do balanceador de carga consulta `/readyz`, que também falha enquanto o banco de dados está fora do ar: depois de 2,5 minutos de
  falhas o ECS substitui a task, e continua fazendo isso até o banco de dados voltar. Não há verificação de contêiner, então um worker,
  que não tem balanceador de carga, falha por uma imagem ruim apenas nos logs dele.
- **Envios**: o balanceador de carga não define limite para o corpo de uma requisição, então as configurações `media_max_*` do servidor decidem.
- **Streams**: um stream envia um heartbeat a cada 30 segundos, abaixo do timeout de inatividade de 60 do balanceador de carga. Ele permanece aberto até
  a task dele sair, e o cliente então se conecta de novo.
- **Endereços de clientes**: os limites de taxa dependem do que o servidor considera confiável, o que o módulo define a partir do ingress. Verifique os endereços
  de clientes nos primeiros logs de requisições: um endereço das sub-redes do balanceador de carga, ou do CloudFront, significa que está errado.
- **O disco** vai com a task, então o plano recusa SQLite e mídia no sistema de arquivos.
- **Segredos** sob uma chave KMS gerenciada pelo cliente também precisam de `kms:Decrypt`, que o módulo não concede.

<!-- source: costs 19e0e43bc201 -->
## Custos

Nada escala a zero no ECS: a task, o balanceador de carga e os endereços públicos dele são cobrados o mês inteiro. Estes valores vêm da
lista de preços da AWS para Tóquio, de 2026-10 e 2026-11, a menos que uma linha diga o contrário, e não foram conferidos com uma fatura.

| Parte | Por mês | Escolha |
| --- | --- | --- |
| Uma task de 0,25 vCPU e 1 GB | US$ 13,26 em x86, US$ 10,61 em arm64 | `service`, `architecture`. Cada worker acrescenta as próprias tasks |
| Balanceador de carga | US$ 17,74, mais US$ 0 a US$ 5,84 por cada unidade de capacidade usada o mês inteiro | Sempre presente |
| Os endereços públicos do balanceador de carga | US$ 7,30, para duas zonas | Um por zona |
| A saída | US$ 3,65 por task com `public-ip`; cerca de US$ 36,50 e US$ 0,045 por GB com `nat`, na tarifa de Ohio | `network.egress` |
| Banco de dados | Cerca de US$ 19 no plano Launch do Neon com 0,25 CU, já que o compute nunca para; cerca de US$ 21 com `rds`, `db.t4g.micro` e 20 GB | `database` |
| Caches e limites de taxa | US$ 0 com `db`; com `elasticache`, cerca de US$ 6,13, ou cerca de US$ 61 segundo outro documento da AWS, na N. Virginia | `kv` |
| O canary | Cerca de US$ 3,34 a cada 15 minutos; cerca de US$ 51,72 a cada minuto | `monitoring.uptime_period_minutes` |
| Traces | US$ 0 até 100.000 traces por mês | `observability.traces` |

**Em repouso, cerca de US$ 42 com `public-ip` e cerca de US$ 75 com `nat`**, mais as unidades de capacidade, antes do banco de dados, do armazenamento, do tráfego
e do canary. Não lidos, e portanto em nenhuma linha: CloudFront, Secrets Manager, CloudWatch Logs, S3, SNS e SQS, dados saindo da AWS e qualquer
nível gratuito. As tasks avulsas custam o tamanho delas pelos minutos em que executam, e uma atualização executa uma segunda task do serviço por alguns minutos.
