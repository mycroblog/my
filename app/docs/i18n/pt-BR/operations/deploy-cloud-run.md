<!-- source: deploying-on-cloud-run 9d2acbfd443c -->
# Implantação no Cloud Run

Como executar o `my` no Google Cloud com os módulos OpenTofu de [`deploy/opentofu`](../../deploy/opentofu/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage e Pub/Sub, com um banco de dados PostgreSQL no Neon, atrás de um Application Load Balancer externo global. O serviço não executa nenhuma instância enquanto ninguém o usa, exceto por um tick que acorda o servidor a cada minuto. Um servidor custa dinheiro: espere cerca de US$ 37 por mês, antes de armazenamento, tráfego e streams abertos, e leia [Custos](#costs) para ver o que o cuidado pode reduzir.

Os módulos exigem OpenTofu 1.11 ou posterior. O Terraform não é compatível.

<!-- source: what-it-creates c95687df197f -->
## O que ele cria

| Parte | Módulo | Recursos |
| --- | --- | --- |
| Servidor | `app-gcp-cloud-run` | Um serviço do Cloud Run com `role = all`, um job de migração, um job de console enquanto você precisar, uma conta de serviço e acesso público |
| Tráfego | `ingress-gcp-load-balancer`, ou `ingress-gcp-domain-mapping` ou `ingress-cloudflare-app`, e opcionalmente `ingress-cloudflare-media` | Um Application Load Balancer externo global com certificado, ou um mapeamento de domínio, ou o Cloudflare, e um host de mídia |
| Jobs | `queue-gcp-cloud-tasks` | Uma fila do Cloud Tasks, um job do Cloud Scheduler e uma conta de serviço que chama o servidor |
| Mídia | `blob-gcp-gcs` | Um bucket privado |
| Atualizações em tempo real | `stream-gcp-pubsub` | Um tópico e uma função personalizada. O servidor cria e remove suas próprias assinaturas |
| Banco de dados | `db-external` | Nada: você cria o banco de dados manualmente |
| E-mail | `mail-smtp` | Nada: qualquer serviço SMTP |
| Segredos | `secrets-gcp-secret-manager` | Segredos da URL do banco de dados e da senha SMTP, que você preenche, e uma chave VAPID gerada |
| KEK | `kek-gcp-secret-manager` ou `kek-gcp-kms` | Um segredo gerado montado como arquivo, ou uma chave do Cloud KMS |

Os valores dos segredos nunca chegam ao plano nem ao estado do OpenTofu: você mesmo adiciona os que vêm de outro lugar, e a implantação gera a KEK e a chave VAPID por atributos somente de escrita.

<!-- source: before-you-start 4c624a6b7316 -->
## Antes de começar

- `tofu` 1.11 ou posterior, `gcloud` e `jq`.
- Um projeto do Google Cloud com faturamento, usado apenas para esta instância, e uma conta nele com a função Owner.
- Um domínio para [`host`](../reference/configuration.md#host). Ele não pode ser alterado depois: leia [Configurações que você não pode alterar depois](production-checklist.md#settings-you-cannot-change-later).
- Uma versão do `my` das [versões publicadas](running.md#get-the-image). Não existe tag `latest`.

<!-- source: first-deployment 996fcc6eb35e -->
## Primeira implantação

<!-- source: 1-enable-the-apis 2d9c831fb77d -->
### 1. Ative as APIs

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com
```

`compute.googleapis.com` é para o load balancer.

<!-- source: 2-create-the-database-on-neon 139308cfc452 -->
### 2. Crie o banco de dados no Neon

Conte com o plano Launch: com o tick padrão, o compute nunca para. O plano Free também funciona, com as ressalvas em [Custos](#costs).

Há duas formas, e o exemplo usa a primeira:

- **Manualmente, com `db-external`.** No console do Neon, crie um projeto em uma região próxima da sua região do Cloud Run e copie as strings de conexão. A senha fica fora do estado do OpenTofu.
- **Com [`db-neon`](../../deploy/opentofu/modules/db-neon/README.md)**, que cria o projeto e grava as URLs nos segredos para você. **A senha do banco de dados então fica no estado**, e o provider não tem manutenção: o README dele explica o que isso significa, e um comentário no bloco `db` em `main.tf` mostra a troca.

Um projeto tem duas strings de conexão, e a implantação usa cada uma onde ela cabe:

- A **direta** (o host sem `-pooler`) é obrigatória. O job de migração e o console sempre a usam: `my schema apply` mantém um lock de nível de sessão, que um pooler em modo de transação não preserva.
- A **com pool** (o host com `-pooler`) é opcional, para o serviço. O pooler do Neon cuida do que o servidor precisa: veja [Por meio de um pooler de conexões](database.md#through-a-connection-pooler). Informe-a em `pooled_url_secret_id` do módulo `db`, em um segredo próprio, adicionado a `referenced` do módulo `secrets`.

Sem uma string com pool, o serviço também usa a direta, no limite de conexões do banco de dados, que `max_instances` do módulo app limita. Para aumentar o pool do padrão de 4 por instância, para as 80 requisições que uma instância do Cloud Run recebe ao mesmo tempo, adicione `pool_max_conns` à string que o serviço usa, por exemplo `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Veja [Esperando por uma conexão com o banco de dados](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 3abe6b730543 -->
### 3. Copie o exemplo e defina as variáveis

Comece com uma cópia em um diretório vazio, com o arquivo de lock dos providers, da versão que você implanta:

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Defina `ref` para essa versão.** Cada bloco de módulo em `main.tf` nomeia uma versão com `?ref=`, que a cópia de uma versão estável já tem correta. Para uma pré-versão, defina você mesmo: `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`. Os módulos existem apenas a partir da primeira versão que os contém, então uma tag anterior não tem exemplo para copiar. Mantenha a mesma versão em `ref` e em `image`, e mantenha este diretório: ele guarda o seu estado e as suas configurações.

Preencha `terraform.tfvars`. `mail_from`, `mail_smtp_host`, `mail_smtp_port` (587 por padrão; o Google Cloud bloqueia a porta de saída 25) e `mail_smtp_username` configuram o [e-mail](email.md). `env` e `secret_env` recebem qualquer outra configuração da [referência de configuração](../reference/configuration.md), por valor e por ID de segredo. Uma configuração nomeada em dois lugares faz o plano falhar.

Para não enviar e-mail, remova o módulo `mail` e o seu segredo de `main.tf`, e `mail` continua `none`. Leia antes [o que isso significa para os cadastros](production-checklist.md#email).

Decida agora sobre [`account_domain`](../reference/configuration.md#account_domain), como `MY_ACCOUNT_DOMAIN` em `env`, se os handles usarem outro domínio.

**O estado** fica neste diretório por padrão. Ele não contém valores secretos, mas é o único registro do que foi criado: guarde-o em um bucket com um bloco `backend "gcs"` se mais de uma pessoa executar isto.

<!-- source: 4-create-the-secrets-and-add-their-values f94fd484e28f -->
### 4. Crie os segredos e adicione seus valores

Isto cria os segredos e gera a KEK e a chave VAPID:

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

`tofu init` também baixa `upgrade.sh` e `console.sh` dessa versão, que os próximos passos usam:

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
```

**Copie a KEK agora** e guarde a cópia separada do banco de dados, por exemplo em um gerenciador de senhas. Sem ela, um [backup](backup.md) não pode ser restaurado e as chaves das contas são perdidas. Faça o mesmo com a chave VAPID, cuja perda encerra todas as assinaturas de Web Push:

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Depois, adicione os valores que vêm de outro lugar. Uma instância cujo segredo não pode ser lido não inicia, então adicione-os antes do app:

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

A KEK chega ao servidor como um arquivo montado, nomeado por `MY_KEK_FILE`. Os outros são variáveis de ambiente resolvidas quando uma instância inicia, então uma nova versão passa a valer na próxima instância: implante uma nova revisão para garantir.

**O OpenTofu gera a KEK uma vez e nunca mais.** O segredo e a sua versão têm `prevent_destroy`, e o número de versão que gravaria um novo valor é fixo no módulo: não o altere, porque uma nova KEK aleatória torna ilegível tudo o que a antiga criptografou. Para rotacionar, siga [Rotacionar a KEK](key-management.md#rotate-the-kek), adicionando o novo valor com `gcloud secrets versions add`. O OpenTofu não acompanha o que você adiciona ali.

Para manter a KEK no Cloud KMS desde o início, substitua o bloco `kek` em `main.tf` como o comentário dele mostra e ative `cloudkms.googleapis.com`. Para trazer uma KEK ou uma chave VAPID próprias, defina `create = false` no módulo `kek`, ou remova a chave de `generate_p256_keys`, antes deste passo. Um segredo criado em outro lugar só precisa do seu ID: passe-o para `secret_env` do módulo app, ou para o módulo que o usa.

<!-- source: 5-create-what-the-migration-needs eab1bb33867c -->
### 5. Crie o que a migração precisa

O startup probe do serviço espera por `/readyz`, que responde `503` até o schema existir, então o schema precisa vir primeiro. Crie o job de migração e o que ele exige, o que inclui o endereço do load balancer, já que as configurações do servidor o citam:

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

<!-- source: 6-apply-the-schema 2946ca18d263 -->
### 6. Aplique o schema

```sh
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

Ele executa `my config check`, `my db upgrade -check`, `my schema apply` e `my db upgrade`, cada um como uma execução do job, e para no primeiro que falhar. O primeiro comando que usa o banco de dados grava nele `host` e `account_domain`.

<!-- source: 7-create-the-rest 368abac4aa08 -->
### 7. Crie o restante

```sh
tofu apply
```

<!-- source: 8-point-the-domain-at-the-load-balancer 7e54edc163b2 -->
### 8. Aponte o domínio para o load balancer

`tofu output dns_records` lista um registro `A`, de `host` para o endereço do load balancer. Adicione-o no seu provedor de DNS. Se esse provedor tiver um proxy próprio, deixe-o desligado para este registro.

O certificado é gerenciado pelo Google e só fica ativo depois que o DNS aponta para o endereço, o que pode demorar. Até lá, `https://{host}` falha com um erro de TLS, então a federação e os navegadores ainda não conseguem alcançar o servidor. Nada mais espera por isso: o Cloud Tasks e o Cloud Scheduler chamam a URL `run.app` padrão do serviço, `https://<service>-<project number>.<region>.run.app`. **Não desative a URL padrão**, ou os jobs param. Essa URL recusa requisições vindas da internet, e o Cloud Run considera esses dois como internos apenas quando vêm do mesmo projeto e nessa URL, não em um domínio personalizado.

Veja [Como o tráfego chega ao serviço](#how-traffic-reaches-the-service) para o que mais você pode escolher.

<!-- source: 9-create-the-first-administrator c5b03212fb36 -->
### 9. Crie o primeiro administrador

Os jobs do Cloud Run não têm terminal nem entrada padrão, então `my admin create` imprime um link que define a senha em vez de ler uma. Ative o [console](#the-console) e execute:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Abra o link imediatamente e defina a senha. Ele funciona uma vez, por uma hora. **Ele permanece no Cloud Logging até lá**, onde qualquer pessoa que possa ler os logs do projeto pode usá-lo. Se ele expirar, execute `my admin account password-reset-link @alice` pelo console. Depois, ative a autenticação de dois fatores.

<!-- source: 10-go-through-the-checklist 90add86e4288 -->
### 10. Percorra a checklist

Percorra a [checklist de produção](production-checklist.md), começando pelo [e-mail](production-checklist.md#email): SPF, DKIM e DMARC no domínio de `mail_from`.

<!-- source: the-console 5aebf704c6d7 -->
## O console

O console executa um comando `my` com a imagem, as configurações e os segredos do serviço. Ele só existe enquanto `console_enabled` estiver ativado, então desative-o quando terminar:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` espera o comando, imprime a saída e o código de saída dele e encerra com esse código. Ele precisa de `roles/logging.viewer`, porque a saída de um job vai para o Cloud Logging. Ele usa a imagem que o serviço executa, seja qual for o valor de `image`.

O comando não tem entrada padrão nem terminal, então um comando que leia uma senha ou um arquivo deles não pode ser executado assim. O que ele imprime fica nos logs, então não execute um comando cuja saída você não escreveria ali.

<!-- source: upgrading 1a1e204c5173 -->
## Atualização

Leia antes [Atualização](database.md#upgrading) e as notas de versão, e avance uma versão menor por vez.

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade. These scripts are of the release your modules name, which is enough: they only run gcloud and my.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

O script faz o seguinte e para no primeiro passo que falhar:

1. Coloca o serviço no [modo de manutenção](maintenance.md) com `--maintenance`.
2. Dá ao job de migração a nova imagem.
3. Executa `my config check`, `my db upgrade -check`, `my schema apply` e `my db upgrade`, cada um esperando o anterior.
4. Implanta a nova revisão, sem o modo de manutenção. Ela só recebe tráfego quando o startup probe passa em `/readyz`.

Sem `--maintenance`, a revisão antiga continua atendendo durante os passos 2 e 3, o que [não tem garantia de funcionar](database.md#the-server-during-an-upgrade). Adicione `--allow-destructive` quando as notas de versão pedirem. Fixe um digest com `--image ghcr.io/mycroblog/my:0.2.0@sha256:...`, o que a [checklist de produção](production-checklist.md#operations) recomenda.

Depois, defina `image` em `terraform.tfvars` com o mesmo valor, e `ref` em `main.tf` com a mesma versão, seguido de `tofu init` e `tofu plan`. Leia esse plano antes de aplicá-lo: os módulos de uma nova versão podem alterar recursos. `tofu apply` nunca altera a imagem de um serviço ou job que já existe, então um `image` esquecido não causa dano, mas o valor é aquele a partir do qual um novo job ou um serviço recriado começaria.

**Quando um passo falha**, o serviço permanece na sua revisão, no modo de manutenção se você o pediu. Corrija a causa e execute o script de novo: os passos podem ser repetidos com segurança. Para sair do modo de manutenção sem atualizar, quando `my schema apply` não foi executado:

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

Para voltar depois que o schema mudou, restaure o backup: veja [Reversão](database.md#rolling-back).

<!-- source: monitoring-and-alerts 67bc34d23fdf -->
## Monitoramento e alertas

O módulo [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) adiciona uma verificação de disponibilidade de `/readyz`, políticas de alerta com os nomes do [runbook](alerts.md) e notificações por e-mail. Ele nunca foi aplicado, então teste-o uma vez fazendo uma verificação falhar. Ative `monitoring.googleapis.com` antes e leia no README dele quanto a verificação custa: cada execução acorda uma instância e, por meio de `/readyz`, o banco de dados, o que se soma ao [compromisso do tick](#costs).

<!-- source: changing-a-backend ef4660f60191 -->
## Trocar um backend

Substitua o bloco do módulo em `main.tf`. O módulo app recebe o que o bloco retornar, então nada mais muda. Trocar o banco de dados também significa mover os dados, já que um novo está vazio: [`db-gcp-cloud-sql`](../../deploy/opentofu/modules/db-gcp-cloud-sql/README.md#coming-from-db-external) traz os passos. Por exemplo, um banco de dados seu:

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = "another-database-url"
}
```

Para um armazenamento fora do Google Cloud, substitua o bloco `blob` por [`blob-s3`](../../deploy/opentofu/modules/blob-s3/README.md), para qualquer serviço compatível com S3, ou [`blob-cloudflare-r2`](../../deploy/opentofu/modules/blob-cloudflare-r2/README.md).

<!-- source: when-you-need-a-network fb392704a1e7 -->
## Quando você precisa de uma rede

Este guia não precisa de nenhuma: Neon, Cloud Tasks, Cloud Storage e Pub/Sub são todos acessados pela internet com a identidade do próprio serviço, então o Cloud Run não se conecta a nada e não há o que pagar nem o que cuidar.

Você precisa de uma para colocar o Memorystore na implantação, já que ele só tem endereços privados, ou para dar ao Cloud SQL um endereço privado no lugar do público. Adicione [`network-gcp-vpc`](../../deploy/opentofu/modules/network-gcp-vpc/README.md), ou descreva uma VPC que você já tenha como um objeto do [mesmo formato](../../deploy/opentofu/README.md#the-network-on-google-cloud), e passe a mesma ao app e a cada módulo que a recebe:

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}
```

Dê também ao app `network = module.network` e adicione `module.kv` aos seus `backends`. Apenas intervalos privados passam pela rede, então o serviço alcança a internet como antes, sem Cloud NAT. Um backend em endereços privados com um app em outra rede, ou em nenhuma, faz o plano falhar. Ative antes as APIs Service Networking e Network Connectivity, e a API Compute Engine. Uma rede aumenta a partida a frio de uma instância: leia [o que a rede privada acrescenta](../../deploy/opentofu/modules/kv-gcp-memorystore/README.md#what-the-private-network-adds).

<!-- source: splitting-jobs-into-workers ed661aede0a3 -->
## Dividir os jobs em workers

Quando um tipo de job atrasa os demais, dê a ele o seu próprio serviço. Decida o que dividir a partir das métricas, como diz [Dividir workers por tipo de job](running.md#splitting-workers-by-job-type). Um único valor vai para a fila e para o app, e toda fila, URL, rota e configuração vêm das suas chaves, então você não escreve nenhuma delas:

```hcl
locals {
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
}
```

Defina `workers = local.workers` no bloco `queue` e no bloco `app`. O plano recusa uma divisão dos tipos que precisam ficar juntos e uma fila e um app com valores diferentes. Ele também exige um broker que alcance todos os processos, o que `stream-gcp-pubsub` é. O mesmo valor vai para `monitoring-gcp`, se você o usar.

Os dois layouts comuns daquela página ficam assim. Para evitar que as entregas atrasem o restante, use o valor acima. Para manter o e-mail e as notificações rápidos:

```hcl
locals {
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
      max_instances = 10
    }
  }
}
```

Deixe de fora os tipos de configurações que você não definiu, como diz aquela página: um tipo que o servidor não registrou impede a inicialização, e o plano não consegue ver quais são. Com `workers`, o serviço padrão não ajuda com esses tipos quando está livre, já que as tarefas deles vão para a própria fila.

Cada campo, e o que um worker tem do app, está no [contrato](../../deploy/opentofu/README.md#workers). O serviço de um worker não recebe tráfego de fora. `upgrade.sh` move cada um depois da migração e antes do serviço padrão, e sem nada que o avise: apenas um passo que falha o interrompe. Cada worker pode trazer o que o serviço custa, e as conexões das suas instâncias contam contra o limite do banco de dados.

<!-- source: how-traffic-reaches-the-service d1e6d0ed195b -->
## Como o tráfego chega ao serviço

O bloco `ingress` de `main.tf` decide isso. O exemplo usa o load balancer, que é pensado para produção. O mapeamento de domínio está em um comentário ao lado, para testar uma implantação a baixo custo: para trocá-lo, altere esse bloco e `service_ingress` do bloco `app`, e o plano avisa se os dois divergirem.

| | `ingress-gcp-load-balancer` (o exemplo) | `ingress-gcp-domain-mapping` |
| --- | --- | --- |
| Custo mensal | Cerca de US$ 18 pelas forwarding rules, mais uma cobrança por GB. Confira a [página de preços](https://cloud.google.com/vpc/network-pricing): este valor não pôde ser confirmado lá | Nenhum |
| Situação | A forma recomendada pelo Google Cloud | Um recurso em Preview que o Google Cloud não recomenda para produção, por causa da latência |
| Regiões | Qualquer uma | `asia-northeast1`, `us-central1` e algumas outras |
| DNS | Um registro `A`, de `tofu output dns_records` | Os registros que a mesma saída lista |
| Certificado | Gerenciado pelo Google, ativo quando o DNS aponta para o endereço | Gerenciado pelo Google, até 24 horas |
| Propriedade do domínio | Nada além do DNS | Você, e toda outra conta que se aplique, deve ser um [proprietário verificado](https://cloud.google.com/run/docs/mapping-custom-domains) antes de aplicar |
| URL `run.app` | Recusada vinda da internet, então ninguém contorna o load balancer | Acessível por qualquer pessoa: um mapeamento de domínio não pode adicionar um cabeçalho, então a origem fica aberta |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` e o endereço do load balancer | `169.254.0.0/16` |
| `service_ingress` do bloco `app` | `internal-and-load-balancer`. Uma divergência com o bloco `ingress` faz o plano falhar | `all` |

O Cloud Tasks e o Cloud Scheduler não chegam por nenhum dos dois: eles chamam diretamente a URL `run.app` padrão.

Para usar a URL do próprio serviço ou outra coisa na frente, como uma CDN, remova o bloco `ingress` e a sua linha em `backends`, e mantenha `service_ingress` no que você precisar: ele é sempre uma escolha explícita sua, e `all` deixa a URL `run.app` aberta a qualquer pessoa. Defina `MY_TRUSTED_PROXIES` em `env` com o que se conecta ao servidor. O que você puser na frente ainda deve responder em `https://{host}`.

<!-- source: cloudflare 95b7a9baaff1 -->
### Cloudflare

`ingress-cloudflare-app` coloca o Cloudflare na frente de `host`, com o cache ignorado para tudo, e `ingress-cloudflare-media` adiciona um host de mídia com cache ao lado de qualquer ingress. Leia antes o README de cada módulo ([app](../../deploy/opentofu/modules/ingress-cloudflare-app/README.md), [media](../../deploy/opentofu/modules/ingress-cloudflare-media/README.md)). O token vai em `CLOUDFLARE_API_TOKEN`. Esses módulos só foram executados com providers simulados.

- **O seu plano decide a origem.** O Cloud Run roteia por `Host`, e o Cloudflare oferece a substituição do cabeçalho Host apenas no plano Enterprise. Em qualquer outro plano, uma origem `*.run.app` faz o plano falhar, então a origem precisa aceitar o seu host por conta própria: mantenha um `ingress-gcp-domain-mapping` desse host, fora de `backends`, e defina `origin_host = "ghs.googlehosted.com"`. Cada host precisa de um mapeamento próprio. Não foi verificado se o certificado dele é emitido e renovado atrás do proxy, então comece com `proxied = false`.
- **Um cabeçalho fecha a origem.** `ingress-cloudflare-app` adiciona uma regra de transformação de cabeçalho de requisição, e o servidor recusa com `403` uma requisição sem o cabeçalho, exceto as verificações de integridade e os `POST`s dos próprios jobs, que verificam seus tokens. Assim, ninguém contorna o Cloudflare pela URL `run.app`, e um Cloudflare Worker de quem faz a chamada não pode escolher o endereço de cliente que os limites de taxa enxergam.
- **O valor fica no estado.** O provider do Cloudflare não tem uma forma somente de escrita para o valor de cabeçalho de uma regra. Gere um com `openssl rand -hex 32`, adicione-o a um segredo com `gcloud secrets versions add`, passe o mesmo valor com `TF_VAR_origin_secret` e mantenha o estado privado. [O guia da CDN](cdn.md#rotate-the-value) explica como rotacioná-lo.
- **Nada adiciona o cabeçalho enquanto um registro não passa pelo proxy**, então o servidor recusa tudo, exceto as verificações de integridade: com `proxied = false` para o certificado de um mapeamento de domínio, configure o segredo depois disso.
- **Desative o Bot Fight Mode** no painel. Ele cobre a zona inteira e nenhuma regra pode ignorá-lo, e um desafio em `/ap/`, `/.well-known/` ou `/api/` quebra a federação, porque os outros servidores não são navegadores. Os jobs não passam pelo Cloudflare. Os módulos não adicionam nenhum desafio e desativam a Browser Integrity Check, mas não gerenciam o Bot Fight Mode. Pelo mesmo motivo, não ative “I'm Under Attack”.
- **Ative Always Use HTTPS** no painel se quiser que `http://` seja redirecionado. Ele cobre a zona inteira, então os módulos deixam isso com você.
- **Timeouts**: o Cloudflare responde `524` quando a origem não envia nada por 125 segundos. Os jobs não são afetados, já que chamam diretamente a URL `run.app`. Os streams enviam um heartbeat a cada 30 segundos, abaixo do limite de ociosidade de 900 segundos. Não foi verificado se ele armazena streams em buffer em um host comum com proxy.
- **Uploads**: 100 MB nos planos Free e Pro, acima dos 32 MiB do Cloud Run, que continua sendo o limite.
- **Vídeo**: os termos do Cloudflare restringem a entrega de vídeo e de uma quantidade desproporcional de arquivos grandes nos planos Free, Pro e Business, e ele pode redirecionar o conteúdo ou tomar outras medidas. Decida isso antes de colocar vídeo em cache no host de mídia.

**Um arquivo excluído continua em cache.** O host de mídia segue o `Cache-Control` do servidor, então um arquivo local excluído pode continuar visível por até um ano, e um remoto por `remote_media_cache_ttl_seconds`. O servidor não o purga. Quando algo precisar desaparecer, purgue manualmente, em **Caching > Configuration > Custom Purge** no painel, ou:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Uma URL de proxy guarda o seu `sig` na query string, que faz parte do que é colocado em cache, então informe a URL inteira.

<!-- source: changing-the-kek-backend c5e918ea5c81 -->
## Trocar o backend da KEK

Substituir o bloco `kek` é um único bloco em `main.tf`, mas numa instância com dados isso não é gratuito. O banco de dados guarda segredos encapsulados com a chave atual, então uma instância que a perde não consegue mais lê-los, e as chaves de todas as contas estão entre eles.

Siga [Migrar entre a KEK local e um KMS](key-management.md#move-between-the-local-kek-and-a-kms). Do segredo para o Cloud KMS, mantenha a KEK atual como `kek_previous` enquanto os segredos são movidos, passando o seu segredo pelo módulo app, com `secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }`; execute `my kek rewrap` pelo [console](#the-console) e remova-a quando `my kek status` não contar mais chaves `local:`. Guarde a KEK pelo tempo em que você mantiver backups do banco de dados anteriores ao rewrap.

Numa instância nova, sem dados, nada precisa ser movido.

<!-- source: limits 5bd276dd3a89 -->
## Limites

- **Uploads**: o Cloud Run recusa uma requisição HTTP/1 acima de 32 MiB, e o servidor fala HTTP/1.1. Por isso o exemplo define `MY_MEDIA_MAX_AV_BYTES` como 30 MiB, contra o padrão de 100 MiB, para que um upload maior receba a resposta do próprio servidor.
- **Streams**: o timeout de requisição, de 15 minutos por padrão, encerra um stream aberto, e o cliente se conecta de novo.
- **Endereços de cliente**: os limites de taxa dependem de `MY_TRUSTED_PROXIES`, que o módulo app define a partir do que o módulo `ingress` retorna. Nenhum documento que lemos diz o que o Cloud Run adiciona a `X-Forwarded-For` numa requisição direta nem qual endereço de par um contêiner enxerga, e o contrato do contêiner não diz nada sobre nenhum dos dois, então `169.254.0.0/16` é uma suposição. A forma documentada do load balancer externo é `[<supplied>,]<client-ip>,<lb-ip>`. Confira os endereços nos logs das primeiras requisições: um IP de cliente que seja um endereço `169.254` ou um endereço do Google significa que a configuração está errada para o seu projeto.
- **Acesso público**: o serviço concede `roles/run.invoker` a `allUsers`, o que uma política de organização de compartilhamento restrito a domínios recusa. Um projeto assim precisa de uma exceção.

<!-- source: costs 1ab8d07f6847 -->
## Custos

Um servidor custa dinheiro. Estes valores são um guia do que esperar e de onde o cuidado reduz o custo, não limites a que a implantação esteja sujeita. São preços de tabela aproximados para uma região, de 2026-10 e 2026-11. Confira-os nas páginas de preços do [Cloud Run](https://cloud.google.com/run/pricing) e do [Neon](https://neon.com/pricing). O compute é gratuito por conta de faturamento até 180.000 vCPU-segundos e 360.000 GiB-segundos por mês.

| Parte | Ocioso | O que aumenta |
| --- | --- | --- |
| Load balancer | Cerca de US$ 18 por mês, não confirmado na página de preços | Uma cobrança por GB; veja [Como o tráfego chega ao serviço](#how-traffic-reaches-the-service) |
| Neon, no plano Launch | Cerca de US$ 19 por mês, com o tick padrão | Armazenamento a US$ 0,35 por GB-mês. `db-gcp-cloud-sql`, em vez disso, tem um custo fixo de cerca de US$ 9,40 por mês, para uma máquina menor: o README dele traz o detalhamento |
| Cloud Run | US$ 0: o mínimo de instâncias é 0 e a CPU só é cobrada durante as requisições | Cada stream aberto mantém uma instância cobrada; veja abaixo |
| Cloud Tasks | US$ 0 | 1 milhão de operações gratuitas, depois US$ 0,40 por milhão. Um job usa cerca de 2 |
| Cloud Scheduler | US$ 0: 3 jobs gratuitos por conta de faturamento | US$ 0,10 por job acima disso |
| Cloud Storage | Cerca de US$ 0,02 por GiB por mês | 5 GiB são gratuitos apenas em `us-east1`, `us-west1` e `us-central1`. A mídia sai pelo Cloud Run |
| Pub/Sub | US$ 0: 10 GiB gratuitos | Apenas enquanto há streams abertos |
| Secret Manager | Centavos | Algumas versões gratuitas, depois alguns centavos por versão |

**O tick mantém o Neon acordado.** Cada tick lê e grava no banco de dados, e o Neon só para um compute após 5 minutos sem atividade, então com o padrão `* * * * *` o compute nunca para. São 730 horas por mês, ou 182,5 CU-horas a 0,25 CU, que é o valor de US$ 19 acima. O tick permanece a cada minuto para que os jobs periódicos rodem no horário e um sinal de despertar perdido seja recuperado logo.

**Um stream aberto mantém uma instância cobrada.** Na cobrança por requisição, uma instância é cobrada enquanto atende qualquer requisição, e as atualizações em tempo real são uma única requisição longa. Um cliente que mantém o stream aberto o mês inteiro custa uma instância 24 horas por dia: cerca de US$ 60 por mês com 1 vCPU e 512 MiB, depois da cota gratuita. Mais streams dividem a instância, até `concurrency`. Se os streams ficam abertos na maior parte do tempo, compare com a cobrança por instância, ou com uma VM pequena e [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 5711f378aadb -->
### Onde o cuidado reduz o custo

- **O mapeamento de domínio** no lugar do load balancer elimina os US$ 18, com [as ressalvas acima](#how-traffic-reaches-the-service). Ele serve para testar uma implantação.
- **O plano Free do Neon** elimina os US$ 19, e uma instância pequena pode rodar nele, com estas ressalvas:
  - Ele permite 100 CU-horas por mês, que o tick padrão consome após cerca de 17 dias. Fique dentro do limite com uma programação mais longa, como `tick_schedule = "*/10 * * * *"`: no máximo metade do tempo acordado, cerca de 91 CU-horas, antes de qualquer outra coisa acordá-lo. Os jobs periódicos então rodam com até 10 minutos de atraso, e um sinal de despertar perdido é recuperado mais tarde.
  - Um servidor federado recebe entregas o dia inteiro, o que também acorda o banco de dados, então uma instância movimentada ultrapassa o limite, seja qual for a programação.
  - Ele guarda 1 GB por projeto e um histórico de 6 horas para restauração, e o seu compute sempre para após 5 minutos ocioso, então a primeira requisição depois disso espera por ele.
