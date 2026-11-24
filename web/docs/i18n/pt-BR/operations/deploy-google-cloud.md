<!-- source: deploying-on-cloud-run 7728d9a37c56 -->
# Implantando no Cloud Run

Como executar o `my` no Google Cloud com o módulo OpenTofu [`mycroblog-gcp`](../../deploy/opentofu/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage e Pub/Sub, com um banco de dados PostgreSQL no Neon, atrás de um Application Load Balancer externo global. O serviço não executa nenhuma instância enquanto ninguém o usa, exceto por um tick que acorda o servidor a cada minuto. Um servidor custa dinheiro: espere cerca de US$ 37 por mês antes de armazenamento, tráfego e streams abertos, e leia [Custos](#costs) para saber o que o cuidado pode reduzir.

O módulo exige o OpenTofu 1.11 ou posterior. O Terraform não é compatível.

<!-- source: what-it-creates cae1e55a1722 -->
## O que ele cria

Um bloco do `mycroblog-gcp` cria tudo isto. Cada linha é uma seção do módulo, e o padrão dela é o que este guia implanta; [o README do módulo](../../deploy/opentofu/README.md#sections) lista os outros tipos.

| Parte | Seção | Recursos |
| --- | --- | --- |
| Servidor | `service`, `jobs` | Um serviço do Cloud Run com `role = all`, um job de migração, um job de console enquanto você precisar dele, uma conta de serviço e acesso público |
| Tráfego | `ingress` | Um Application Load Balancer externo global com um certificado. Um mapeamento de domínio ou a Cloudflare são os outros tipos, e um host de mídia na Cloudflare pode ficar ao lado de qualquer um |
| Jobs | `queue` | Uma fila do Cloud Tasks, um job do Cloud Scheduler e uma conta de serviço que chama o servidor |
| Mídia | `media` | Um bucket privado |
| Atualizações em tempo real | `stream` | Um tópico e uma função personalizada. O servidor cria e remove as próprias assinaturas |
| Banco de dados | `database` | Um segredo para a URL dele: você cria o banco de dados à mão |
| E-mail | `mail` | Um segredo para a senha do SMTP: qualquer serviço SMTP |
| Segredos | | Uma chave VAPID gerada e os segredos que só você pode preencher, vazios |
| KEK | `kek` | Um segredo gerado montado como arquivo. O Cloud KMS é o outro tipo |
| Métricas, traces, erros | `observability` | As APIs do Cloud Monitoring, do Cloud Trace e do Error Reporting, que leem o que o Cloud Run envia a elas |

Os valores dos segredos nunca chegam ao plano nem ao estado do OpenTofu: você adiciona por conta própria os que vêm de outro lugar, e o módulo gera a KEK e a chave VAPID por atributos somente de escrita.

<!-- source: before-you-start 10846ce6f3b1 -->
## Antes de começar

- `tofu` 1.11 ou posterior, `gcloud` e `jq`, onde você aplica: um apply que muda a versão executa a migração a partir dali, pelo `gcloud`.
- Um projeto do Google Cloud com cobrança, usado apenas para esta instância, e uma conta nele com o papel Owner.
- Um domínio para [`host`](../reference/configuration.md#host). Ele não pode mudar depois: leia [Configurações que você não pode mudar depois](production-checklist.md#settings-you-cannot-change-later).
- Uma versão do `my` dos [releases](running.md#get-the-image). Não há tag `latest`.

<!-- source: first-deployment 996fcc6eb35e -->
## Primeira implantação

<!-- source: 1-enable-the-apis c7f2ef4a9ee9 -->
### 1. Ative as APIs

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` é para o balanceador de carga.

<!-- source: 2-create-the-database-on-neon ca5f51e8979e -->
### 2. Crie o banco de dados no Neon

Espere o plano Launch: com o tick padrão, o compute nunca para. O plano Free também funciona, com as contrapartidas em [Custos](#costs).

No console do Neon, crie um projeto em uma região próxima da sua região do Cloud Run e copie as strings de conexão dele. O módulo nunca o cria, para que a senha dele fique fora do estado do OpenTofu. Um projeto tem duas strings de conexão, e a implantação usa cada uma onde ela cabe:

- A **direta** (o host sem `-pooler`) é obrigatória. O job de migração e o console sempre a usam: `my schema apply` mantém um lock em nível de sessão, que um pooler em modo de transação não mantém.
- A **com pool** (o host com `-pooler`) é opcional, para o serviço. O pooler do Neon cuida do que o servidor precisa: veja [Por um pooler de conexões](database.md#through-a-connection-pooler). Defina `database = { pooled = true }`, e o módulo cria um segundo segredo para ela.

Sem uma string com pool, o serviço usa a direta também, no limite de conexões do banco de dados, que `service.max_instances` limita. Para aumentar o pool do padrão de 4 por instância, para as 80 requisições que uma instância do Cloud Run recebe de uma vez, adicione `pool_max_conns` à string que o serviço usa, por exemplo `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Veja [Esperando uma conexão com o banco de dados](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 1b99f37cb3a7 -->
### 3. Copie o exemplo e defina as variáveis

Comece de uma cópia em um diretório vazio, com o arquivo de lock dos provedores, a partir da versão que você implanta:

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` é o provedor `google` e um bloco, `module "mycroblog"`. **Defina `ref` para essa versão.** O bloco nomeia uma versão com `?ref=`, que a cópia de uma versão estável já traz certo. A imagem do servidor é a mesma versão, então `ref` sozinho nomeia o que executa. Para uma pré-versão, defina os dois por conta própria: `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`, e `image = { version = "0.1.0-rc.4" }` no bloco, a versão sem o `v`, já que o módulo de uma pré-versão não usa a própria imagem como padrão. O módulo existe apenas a partir da primeira versão que o contém, então uma tag anterior não tem exemplo para copiar. Mantenha este diretório: ele guarda o seu estado e as suas configurações.

Preencha `terraform.tfvars`. `mail_from`, `mail_smtp_host` e `mail_smtp_username` configuram o [e-mail](email.md); a porta é 587, já que o Google Cloud bloqueia a porta de saída 25. Para mudar qualquer outra coisa, adicione a seção ao bloco em `main.tf`, como `ingress = { kind = "domain-mapping" }`: todas as seções e o que cada uma recusa estão em [o README do módulo](../../deploy/opentofu/README.md#sections).

`settings` aceita qualquer outra configuração da [referência de configuração](../reference/configuration.md) pela chave dela, e `secrets` as chaves de configurações secretas, cada uma das quais ganha um segredo vazio para preencher. Uma chave que uma seção ou o módulo define faz o plano falhar, e o mesmo vale para uma chave que o módulo não aceita no Cloud Run, como `listen`; o erro diz o motivo. Decida agora sobre [`account_domain`](../reference/configuration.md#account_domain), como `settings = { account_domain = "example.org" }`, se os identificadores usam outro domínio.

**O estado** fica neste diretório por padrão. Ele não guarda valores secretos, mas é o único registro do que foi criado: guarde-o em um bucket com um bloco `backend "gcs"` se mais de uma pessoa executa isto.

<!-- source: 4-create-the-secrets-and-add-their-values 51cddae33ec9 -->
### 4. Crie os segredos e adicione os valores deles

Isto cria os segredos e gera a KEK e a chave VAPID:

```sh
tofu init
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager
```

`tofu init` também busca os scripts dessa versão: `migrate.sh`, que um apply executa, e `console.sh`, que uma etapa posterior usa:

```sh
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

**Copie a KEK agora** e guarde a cópia separada do banco de dados, por exemplo em um gerenciador de senhas. Sem ela um [backup](backup.md) não pode ser restaurado, e as chaves das contas se perdem. `tofu output kek` mostra o comando. Faça o mesmo com a chave VAPID, cuja perda encerra todas as assinaturas de Web Push:

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Depois adicione os valores que vêm de outro lugar. `tofu output secrets_to_fill` lista cada segredo que o módulo criou vazio, com o que vai nele e o comando que o adiciona. Uma instância cujo segredo não pode ser lido não inicia, então adicione-os antes do serviço:

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

A KEK chega ao servidor como um arquivo montado, nomeado por `MY_KEK_FILE`. Os outros são variáveis de ambiente resolvidas quando uma instância inicia, então uma nova versão vale na próxima instância: implante uma nova revisão para ter certeza.

**O OpenTofu gera a KEK uma vez e nunca mais.** O segredo e a versão dele têm `prevent_destroy`, e o número de versão que gravaria um novo valor é fixo no módulo: uma nova KEK aleatória torna ilegível tudo o que a antiga criptografou. Para trocá-la, siga [Trocar a KEK](key-management.md#rotate-the-kek), adicionando o novo valor com `gcloud secrets versions add`. O OpenTofu não acompanha o que você adiciona ali.

Para manter a KEK no Cloud KMS desde o início, defina `kek = { kind = "kms" }` antes desta etapa, use como alvo `module.mycroblog.module.kek_kms` no lugar de `kek_secret_manager` e ative `cloudkms.googleapis.com`.

<!-- source: 5-apply 2076866c0f3b -->
### 5. Aplique

```sh
tofu apply
```

Isto cria todo o restante, em uma ordem de que a sonda de inicialização do serviço precisa: a sonda espera `/health/ready`, que responde `503` até o esquema existir. O job de migração vem primeiro, com o que ele depende, o que inclui o endereço do balanceador de carga, já que as configurações do servidor o nomeiam. Depois o apply executa `my upgrade` como uma execução do job, a partir da sua máquina pelo `gcloud`, e espera por ele: `my config check`, `my db upgrade -check`, `my schema apply` e `my db upgrade`, que para no primeiro que falha. A saída dele aparece na do apply. O primeiro comando que usa o banco de dados grava `host` e `account_domain` nele. Só então o apply cria os serviços.

Se a migração falhar, o apply para antes dos serviços. Corrija a causa, muitas vezes um segredo sem valor, e execute `tofu apply` de novo: ele executa a migração de novo.

<!-- source: 6-point-the-domain-at-the-load-balancer c81cc51be2af -->
### 6. Aponte o domínio para o balanceador de carga

`tofu output dns_records` lista um registro `A`, de `host` para o endereço do balanceador de carga. Adicione-o no seu provedor de DNS. Se esse provedor tem um proxy próprio, deixe-o desligado para este registro.

O certificado é gerenciado pelo Google e só fica ativo depois que o DNS aponta para o endereço, o que pode demorar. Até lá `https://{host}` falha com um erro de TLS, então a federação e os navegadores ainda não alcançam o servidor. Nada mais espera por isso: o Cloud Tasks e o Cloud Scheduler chamam a URL `run.app` padrão do serviço, `https://<service>-<project number>.<region>.run.app`. **Não desative a URL padrão**, ou os jobs param. Essa URL recusa requisições da internet, e o Cloud Run conta esses dois como internos apenas do mesmo projeto e nessa URL, não em um domínio personalizado.

Veja [Como o tráfego chega ao serviço](#how-traffic-reaches-the-service) para o que mais você pode escolher.

<!-- source: 7-create-the-first-administrator 19137d6d01f0 -->
### 7. Crie o primeiro administrador

Os jobs do Cloud Run não têm terminal nem entrada padrão, então `my admin create` imprime um link que define a senha em vez de ler uma. Ative o [console](deploy-opentofu.md#the-console) e execute:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Abra o link logo e defina a senha. Ele funciona uma vez, por uma hora. **Ele fica no Cloud Logging até lá**, onde qualquer pessoa que possa ler os logs do projeto pode usá-lo. Se ele expirar, execute `my admin account password-reset-link @alice` pelo console. Depois ative a autenticação em dois fatores.

<!-- source: 8-go-through-the-checklist e7fe33d10e80 -->
### 8. Percorra a lista de verificação

Percorra a [lista de verificação de produção](production-checklist.md), começando pelo [e-mail](production-checklist.md#email): SPF, DKIM e DMARC no domínio de `mail_from`.

<!-- source: the-console 52df6f8c43ac -->
## O console

O console executa um comando `my` com a imagem, as configurações e os segredos do serviço. Ele existe apenas enquanto `console_enabled` está ativado, então desative-o quando terminar:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` espera o comando, imprime a saída e o código de saída dele e termina com esse código. Ele precisa de `roles/logging.viewer`, porque a saída de um job vai para o Cloud Logging. Ele usa a imagem que o serviço executa: depois de uma atualização que parou na migração, essa é a versão anterior, à qual o banco de dados ainda serve.

O comando não tem entrada padrão nem terminal, então um comando que lê uma senha ou um arquivo deles não pode executar assim. O que ele imprime fica nos logs, então não execute um comando cuja saída você não escreveria ali.

<!-- source: upgrading fe1bed77d3af -->
## Atualizando

Leia primeiro [Atualizando](database.md#upgrading) e as notas da versão, e avance uma versão menor por vez. Uma atualização é um apply da nova versão: o plano mostra cada serviço e job recebendo a imagem dela, e o apply executa a migração antes de qualquer um deles servi-la.

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

A imagem é a versão que `ref` nomeia, então mudar `ref` é a atualização inteira. Leia o plano antes de aplicá-lo: o módulo de uma nova versão pode mudar recursos também. O apply faz isto e para no primeiro passo que falha:

1. Dá aos jobs de migração e de console a nova imagem, com o restante do plano de que os serviços não dependem.
2. Executa `my upgrade` como uma execução do job de migração, de onde você aplica, e espera por ele: `my config check`, `my db upgrade -check`, `my schema apply` e `my db upgrade`. A saída dele aparece na do apply.
3. Implanta a nova revisão de cada worker, depois do serviço. Uma revisão só recebe tráfego quando a sonda de inicialização dela passa em `/health/ready`.

**Onde você aplica** precisa de `gcloud` e `jq`, e de credenciais que possam executar o job e ler os logs dele (`roles/run.developer` e `roles/logging.viewer`, que o Owner inclui). O mesmo vale para um job de CI que aplica.

**Quando um passo falha**, os serviços ficam nas revisões deles, e as notas da versão ou a saída da migração dizem o motivo. Corrija a causa e execute `tofu apply` de novo, com a mesma versão: ele executa a migração de novo, e os passos que terminaram não encontram mais nada a fazer. Um apply que foi interrompido, ou que perdeu a conexão, deixa a execução rodando no Cloud Run; o próximo apply espera ela terminar antes de iniciar a sua. Para voltar depois que o esquema mudou, restaure o backup: veja [Revertendo](database.md#rolling-back). Um apply com uma versão anterior para na migração, já que essa versão recusa um esquema mais novo, e não muda nenhum serviço.

Para experimentar uma release candidate, ou manter a imagem de uma versão anterior sob um módulo mais novo, defina a versão por conta própria; ela é a tag da imagem, sem o `v` da versão:

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 9ff10080c36d -->
### Verificando o SQL antes

O plano não mostra o SQL da migração. Para lê-lo antes do apply, dê ao job de migração apenas a nova imagem e execute nele `my upgrade -dry-run`, que não muda nada:

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: destructive-changes 5b3fdea90a55 -->
### Mudanças destrutivas

Quando as notas da versão pedirem `-allow-destructive`, nomeie essa versão no bloco:

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` recebe `-allow-destructive` apenas no apply dessa versão, então um valor deixado ali não deixa nenhuma versão posterior descartar dados.

<!-- source: pinning-the-image-by-digest b31962ecc908 -->
### Fixando a imagem por digest

A [lista de verificação de produção](production-checklist.md#operations) recomenda um digest, das notas da versão:

```hcl
  image = { digest = "sha256:..." }
```

Um digest nomeia uma imagem, seja o que for que a tag ao lado dele diga, então mude-o na mesma edição que `ref`: um digest deixado da versão anterior mantém a imagem dessa versão sob a nova versão.

<!-- source: the-maintenance-mode 518e9aa176c0 -->
### O modo de manutenção

O [modo de manutenção](maintenance.md) leva três applies, já que os serviços mudam antes da atualização e depois dela. Adicione-o às configurações do bloco e aplique, faça o backup, aumente `ref` e aplique, depois remova-o e aplique:

```hcl
  settings = { maintenance = "true" }
```

Sem ele, a revisão antiga continua servindo durante a migração, o que [não tem garantia de funcionar](database.md#the-server-during-an-upgrade). As configurações mudadas à mão com o `gcloud` não duram: o próximo apply devolve o que o bloco diz.

<!-- source: monitoring-and-alerts 930234cd0dde -->
## Monitoramento e alertas

O módulo adiciona uma verificação de disponibilidade de `/health/live`, métricas baseadas em logs e políticas de alerta com os nomes do [runbook](alerts.md), que notificam os endereços de `monitoring.notification_emails` (`alert_emails` no exemplo) ou os canais de `notification_channels`. A entrada não tem padrão: nomeie quem é notificado, ou defina `monitoring = null` para deixar tudo isso de fora. Ele nunca foi aplicado, então teste-o uma vez fazendo uma verificação falhar. Cada execução da verificação acorda uma instância, o que se soma à [contrapartida do tick](#costs): `uptime_period`, 15 minutos por padrão, é a frequência.

<!-- source: changing-a-backend c20598caf230 -->
## Trocando um backend

Mude o `kind` da seção dele. Cada tipo define o que precisa no servidor, e o plano diz o que muda. Trocar o banco de dados também significa mover os dados dele, já que um novo está vazio: faça um [backup](backup.md) do antigo e restaure-o no novo enquanto nada escreve. Por exemplo, Cloud SQL no lugar do Neon:

```hcl
  database = { kind = "cloud-sql" }
```

Para um armazenamento fora do Google Cloud, defina `media.kind` como `s3`, para qualquer serviço compatível com S3 com o `endpoint` dele, ou como `r2`, com `cloudflare.account_id`. Ambos leem uma chave de acesso dos segredos que `secrets_to_fill` lista.

<!-- source: when-you-need-a-network 0a4359c002ed -->
## Quando você precisa de uma rede

Este guia não precisa de nenhuma: Neon, Cloud Tasks, Cloud Storage e Pub/Sub são todos alcançados pela internet com a identidade do próprio serviço, então o Cloud Run não se conecta a nada e não há nada a pagar nem a cuidar.

Você precisa de uma para colocar o Memorystore na implantação, já que ele só tem endereços privados, ou para dar ao Cloud SQL um endereço privado no lugar do público. `network = { kind = "vpc" }` cria uma, e `network = { kind = "existing", ... }` se conecta a uma VPC que você tem, que você descreve com `network`, `subnetwork`, `private_services_range` e `memorystore_connection_policy`:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

Apenas intervalos privados passam pela rede, então o serviço alcança a internet como antes, sem Cloud NAT. `nat = true` envia tudo pelo Cloud NAT no lugar, para um endereço de origem estático. Ative primeiro as APIs Service Networking e Network Connectivity, e a API Compute Engine. Uma rede aumenta o cold start de uma instância, e o Memorystore é cobrado enquanto o servidor está ocioso.

<!-- source: splitting-jobs-into-workers 831929244c6d -->
## Dividindo jobs em workers

Quando um tipo de job segura os demais, dê a ele o próprio serviço. Decida o que dividir a partir das métricas, como [Dividindo workers por tipo de job](running.md#splitting-workers-by-job-type) diz. Um valor, `workers`, vai para o serviço, a fila e o monitoramento, e toda fila, URL, rota e configuração vem das chaves dele, então você não escreve nenhuma delas:

```hcl
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
```

O plano recusa uma divisão dos tipos que precisam ficar juntos. Ele também precisa de um broker que alcance todos os processos, o que `stream.kind = pubsub` é.

Os dois layouts comuns dessa página ficam assim. Para evitar que as entregas atrasem o restante, use o valor acima. Para manter o e-mail e as notificações rápidos:

```hcl
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
```

Deixe de fora os tipos das configurações que você não definiu, como essa página diz: um tipo que o servidor não registrou impede a inicialização, e o plano não consegue ver quais são. Com `workers`, o serviço padrão não ajuda com esses tipos quando está livre, já que as tarefas deles vão para a fila própria.

Cada worker aceita `job_types`, e `min_instances`, `max_instances`, `cpu`, `memory_mib`, `concurrency`, `max_concurrent_dispatches` e `max_dispatches_per_second`, cada um o do serviço ou o da fila quando não é nomeado. O serviço de um worker não recebe tráfego de fora. Uma atualização move cada um depois da migração e antes do serviço padrão, e sem nada para avisá-lo: apenas um passo que falha o interrompe. Cada worker pode trazer o que o serviço custa, e as conexões das instâncias dele contam contra o limite do banco de dados.

<!-- source: how-traffic-reaches-the-service 156fc6f76743 -->
## Como o tráfego chega ao serviço

`ingress.kind` decide isso. O padrão é o balanceador de carga, que é pensado para produção. O mapeamento de domínio serve para experimentar uma implantação de forma barata: defina `ingress = { kind = "domain-mapping" }`.

| | `load-balancer` (o padrão) | `domain-mapping` |
| --- | --- | --- |
| Taxa mensal | Cerca de US$ 18 pelas regras de encaminhamento, mais uma cobrança por GB. Confira a [página de preços](https://cloud.google.com/vpc/network-pricing): este valor não pôde ser confirmado lá | Nenhuma |
| Situação | A forma recomendada pelo Google Cloud | Um recurso em Preview que o Google Cloud não recomenda para produção, por causa da latência |
| Regiões | Qualquer uma | `asia-northeast1`, `us-central1` e algumas outras |
| DNS | Um registro `A`, de `tofu output dns_records` | Os registros que a mesma saída lista |
| Certificado | Gerenciado pelo Google, ativo depois que o DNS aponta para o endereço | Gerenciado pelo Google, até 24 horas |
| Propriedade do domínio | Nada além do DNS | Você, e toda outra conta que aplica, precisa ser um [proprietário verificado](https://cloud.google.com/run/docs/mapping-custom-domains) antes de aplicar |
| URL `run.app` | Recusada pela internet, então ninguém contorna o balanceador de carga | Acessível por qualquer pessoa: um mapeamento de domínio não pode adicionar um cabeçalho, então a origem fica aberta |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` e o endereço do balanceador de carga | `169.254.0.0/16` |

O Cloud Tasks e o Cloud Scheduler não chegam por nenhum dos dois: eles chamam a URL `run.app` padrão diretamente.

<!-- source: cloudflare e55e367a0064 -->
### Cloudflare

`ingress = { kind = "cloudflare" }` coloca a Cloudflare na frente de `host`, com o cache ignorado para tudo, e `ingress.media_host` adiciona um host de mídia em cache ao lado de qualquer tipo. Ambos precisam de `cloudflare = { zone_id = "..." }`, e o token vai em `CLOUDFLARE_API_TOKEN`. Eles só foram executados em provedores simulados.

- **O seu plano decide a origem.** O Cloud Run roteia por `Host`, e a Cloudflare oferece a substituição do cabeçalho Host apenas no plano Enterprise. Em qualquer outro plano a origem precisa aceitar o seu host por conta própria, então o módulo cria um mapeamento de domínio de cada host, com as contrapartidas da tabela acima, e a Cloudflare se conecta a ele. Se o certificado dele é emitido e renovado atrás do proxy não foi verificado, então comece com `cloudflare.proxied = false`. No plano Enterprise, `cloudflare.host_header_override = true` se conecta à URL `run.app` no lugar, sem mapeamento de domínio.
- **Um cabeçalho fecha a origem.** O módulo adiciona uma regra de transformação do cabeçalho da requisição, e o servidor recusa com `403` uma requisição sem o cabeçalho, exceto as verificações de saúde e os `POST`s dos próprios jobs, que verificam os tokens deles. Assim ninguém contorna a Cloudflare pela URL `run.app`, e um Cloudflare Worker do próprio chamador não pode escolher o endereço de cliente que os limites de taxa veem.
- **O valor vai parar no estado.** O módulo o gera e o grava no segredo `my-origin-secret`, mas o provedor da Cloudflare não tem uma forma somente de escrita do valor do cabeçalho de uma regra, então mantenha o estado privado. [O guia da CDN](cdn.md#rotate-the-value) explica como funciona a troca.
- **Nada adiciona o cabeçalho enquanto um registro não passa pelo proxy**, então o servidor recusa tudo, exceto as verificações de saúde: com `proxied = false` para o certificado de um mapeamento de domínio, ative o proxy quando o certificado estiver ativo.
- **Desative o Bot Fight Mode** no painel. Ele cobre a zona inteira e nenhuma regra pode ignorá-lo, e um desafio em `/ap/`, `/.well-known/` ou `/api/` quebra a federação, porque os outros servidores não são navegadores. Os jobs não passam pela Cloudflare. O módulo não adiciona nenhum desafio e desativa o Browser Integrity Check, mas não gerencia o Bot Fight Mode. Não ative o “I'm Under Attack” pelo mesmo motivo.
- **O Email Address Obfuscation pode continuar ativado.** Ele reescreve endereços de e-mail em HTML como `[email protected]` e adiciona um script para restaurá-los, que a Content-Security-Policy do servidor recusa. As páginas do próprio servidor, como a que diz para onde foi o e-mail de confirmação, marcam o conteúdo delas para que ele deixe os endereços em paz.
- **Ative o Always Use HTTPS** no painel se quiser `http://` redirecionado. Ele cobre a zona inteira, então o módulo o deixa com você.
- **Timeouts**: a Cloudflare responde `524` quando a origem não envia nada por 125 segundos. Os jobs não são afetados, já que chamam a URL `run.app` diretamente. Os streams enviam um heartbeat a cada 30 segundos, abaixo do limite de ociosidade de 900 segundos. Se ela armazena streams em buffer em um host comum com proxy não foi verificado.
- **Envios**: 100 MB nos planos Free e Pro, acima dos 32 MiB do Cloud Run, que continua sendo o limite.
- **Vídeo**: os termos da Cloudflare restringem servir vídeo e uma quantidade desproporcional de arquivos grandes nos planos Free, Pro e Business, e ela pode redirecionar o conteúdo ou tomar outra ação. Decida isso antes de colocar vídeo em cache no host de mídia.

**Um arquivo excluído continua em cache.** O host de mídia segue o `Cache-Control` do servidor, então um arquivo local excluído pode continuar visível por até um ano, e um remoto por `remote_media_cache_ttl_seconds`. O servidor não o limpa. Quando algo precisar desaparecer, limpe-o à mão, em **Caching > Configuration > Custom Purge** no painel, ou:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Uma URL de proxy guarda o `sig` dela na query string, que faz parte do que fica em cache, então informe a URL inteira.

<!-- source: changing-the-kek-backend b1e8cbb9ccfb -->
## Trocando o backend da KEK

Mudar `kek.kind` é uma linha em `main.tf`, mas em uma instância com dados não é de graça. O banco de dados guarda segredos envolvidos com a chave atual, então uma instância que a perde não consegue mais lê-los, e as chaves de todas as contas estão entre eles.

Siga [Mudar entre a KEK local e um KMS](key-management.md#move-between-the-local-kek-and-a-kms). Do segredo para o Cloud KMS, mantenha a KEK atual como `kek_previous` enquanto os segredos se movem: adicione `secrets = ["kek_previous"]` e preencha o segredo `my-kek-previous` com a KEK atual. Execute `my kek rewrap` pelo [console](deploy-opentofu.md#the-console) e depois remova-a quando `my kek status` não contar mais chaves `local:`. Guarde a KEK pelo tempo em que você guardar backups do banco de dados anteriores ao rewrap.

Em uma instância nova sem dados, nada precisa se mover.

<!-- source: limits 3ff113a57279 -->
## Limites

- **Envios**: o Cloud Run recusa uma requisição HTTP/1 acima de 32 MiB, e o servidor fala HTTP/1.1. O módulo portanto define `MY_MEDIA_MAX_AV_BYTES` como 30 MiB, contra o padrão de 100 MiB, para que um envio maior receba a resposta do próprio servidor.
- **Streams**: o timeout de requisição, 15 minutos por padrão, encerra um stream aberto, e o cliente se conecta de novo.
- **Endereços de cliente**: os limites de taxa dependem de `MY_TRUSTED_PROXIES`, que o módulo define a partir do ingress. Nenhum documento que lemos diz o que o Cloud Run adiciona a `X-Forwarded-For` para uma requisição direta nem qual endereço de par um contêiner vê, e o contrato de contêiner não diz nada sobre nenhum dos dois, então `169.254.0.0/16` é uma suposição. A forma documentada do balanceador de carga externo é `[<supplied>,]<client-ip>,<lb-ip>`. Confira os endereços nos logs das primeiras requisições: um IP de cliente que é um endereço `169.254` ou do Google significa que a configuração está errada para o seu projeto.
- **Acesso público**: o serviço concede `roles/run.invoker` a `allUsers`, o que uma política de organização de compartilhamento restrito por domínio recusa. Um projeto assim precisa de uma exceção.

<!-- source: costs b8be3997d723 -->
## Custos

Um servidor custa dinheiro. Estes valores são um guia do que esperar e de onde o cuidado reduz, não limites a que a implantação esteja presa. São preços de tabela aproximados para uma região, de 2026-10 e 2026-11. Confira-os nas páginas de preços do [Cloud Run](https://cloud.google.com/run/pricing) e do [Neon](https://neon.com/pricing). O compute é gratuito por conta de cobrança até 180.000 vCPU-segundos e 360.000 GiB-segundos por mês.

| Parte | Ocioso | O que se soma |
| --- | --- | --- |
| Balanceador de carga | Cerca de US$ 18 por mês, não confirmado na página de preços | Uma cobrança por GB; veja [Como o tráfego chega ao serviço](#how-traffic-reaches-the-service) |
| Neon, no plano Launch | Cerca de US$ 19 por mês, com o tick padrão | Armazenamento a US$ 0,35 por GB-mês. `database.kind = cloud-sql` no lugar tem um custo fixo de cerca de US$ 9,40 por mês, para uma máquina menor |
| Cloud Run | US$ 0: min instances é 0 e a CPU é cobrada apenas durante as requisições | Cada stream aberto mantém uma instância cobrada; veja abaixo |
| Cloud Tasks | US$ 0 | 1 milhão de operações grátis, depois US$ 0,40 por milhão. Um job leva cerca de 2 |
| Cloud Scheduler | US$ 0: 3 jobs grátis por conta de cobrança | US$ 0,10 por job acima disso |
| Cloud Storage | Cerca de US$ 0,02 por GiB por mês | 5 GiB são grátis apenas em `us-east1`, `us-west1` e `us-central1`. A mídia sai pelo Cloud Run |
| Pub/Sub | US$ 0: 10 GiB grátis | Apenas enquanto há streams abertos |
| Secret Manager | Centavos | Algumas versões grátis, depois alguns centavos por versão |

**O tick mantém o Neon acordado.** Cada tick lê e escreve no banco de dados, e o Neon só para um compute depois de 5 minutos sem atividade, então com o padrão `* * * * *` o compute nunca para. São 730 horas por mês, ou 182,5 CU-horas a 0,25 CU, que são os US$ 19 acima. O tick fica a cada minuto para que os jobs periódicos executem no horário, e um sinal de despertar perdido seja recuperado logo.

**Um stream aberto mantém uma instância cobrada.** Na cobrança por requisição, uma instância é cobrada enquanto trata qualquer requisição, e as atualizações em tempo real são uma requisição longa. Um cliente que mantém o stream aberto o mês todo custa uma instância 24 horas por dia: cerca de US$ 60 por mês com 1 vCPU e 512 MiB, depois da cota grátis. Mais streams compartilham a instância, até `concurrency`. Se os streams ficam abertos a maior parte do tempo, compare com a cobrança por instância, ou com uma VM pequena e o [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 28bfc2e5ffcc -->
### Onde o cuidado reduz

- **O mapeamento de domínio** no lugar do balanceador de carga tira os US$ 18, com [as contrapartidas acima](#how-traffic-reaches-the-service). Ele serve para experimentar uma implantação.
- **O plano Free do Neon** tira os US$ 19, e uma instância pequena pode executar nele, com estas contrapartidas:
  - Ele permite 100 CU-horas por mês, que o tick padrão consome depois de cerca de 17 dias. Fique dentro dele com um agendamento mais longo, como `queue = { tick_schedule = "*/10 * * * *" }`: no máximo metade do tempo acordado, cerca de 91 CU-horas, antes de qualquer outra coisa acordá-lo. Os jobs periódicos então executam com até 10 minutos de atraso, e um sinal de despertar perdido é recuperado mais tarde.
  - Um servidor federado recebe entregas 24 horas por dia, o que também acorda o banco de dados, então uma instância movimentada o supera seja qual for o agendamento.
  - Ele guarda 1 GB por projeto e um histórico de 6 horas para restaurar, e o compute sempre para depois de 5 minutos ocioso, então a primeira requisição depois disso espera por ele.
