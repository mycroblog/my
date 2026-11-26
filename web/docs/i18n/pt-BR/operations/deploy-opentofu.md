<!-- source: deploying-with-opentofu 178b85dbd9bd -->
# Implantar com o OpenTofu

O que toda implantação com os módulos raiz do OpenTofu tem em comum: [Implantar no Google Cloud](deploy-google-cloud.md) e
[Implantar na AWS](deploy-aws.md) acrescentam o que a plataforma de cada uma precisa. Os dois módulos exigem o
[OpenTofu](https://opentofu.org) 1.11 ou posterior. O Terraform não é compatível.

<!-- source: the-root-modules 4e0bf528bd0e -->
## Os módulos raiz

Um bloco de um módulo raiz implanta o servidor inteiro: o serviço, os jobs dele e um backend para cada função.

| Módulo raiz | Plataforma | Exemplo para copiar |
| --- | --- | --- |
| `mycroblog-gcp` | Cloud Run no Google Cloud | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | ECS no Fargate, na AWS. Nunca foi aplicado | `examples/aws-ecs-neon` |

Um exemplo é o provedor da plataforma e um bloco, `module "mycroblog"`, cuja origem é uma fonte Git fixada em uma
release:

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

A fixação é obrigatória: um módulo sem fixação poderia mudar os recursos da KEK debaixo dos seus pés. A imagem do servidor usa por padrão a
release que `ref` nomeia, então só `ref` já diz o que executa, e aumentá-lo é uma atualização. O módulo existe apenas a partir da primeira
release que o contém.

<!-- source: sections 728259c0b297 -->
### Seções

Cada função é uma seção do bloco, um objeto cujo `kind` escolhe o backend, como `database = { kind = "external" }`.
Todo campo tem um padrão, a configuração recomendada, então você escreve apenas o que muda. Uma combinação que não pode funcionar falha
antes do plano, com uma mensagem que nomeia as entradas. [A referência dos módulos](../../deploy/opentofu/README.md#sections)
lista as seções, os tipos e os campos de cada módulo.

<!-- source: other-settings-and-secrets 112fdabc5bb0 -->
### Outras configurações e segredos

`settings` aceita qualquer outra configuração da [referência de configuração](../reference/configuration.md) pela chave dela, e o
módulo a passa ao serviço, aos workers e aos jobs. `secrets` aceita as chaves de configurações secretas: para cada uma, o
módulo cria um segredo vazio `<name>-<key>`, com hifens no lugar de sublinhados, passa-o ao servidor e o lista para você preencher.

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

Uma chave que uma seção define ou que o módulo fixa faz o plano falhar, e o mesmo vale para uma chave que o módulo não aceita, como `listen`:
o erro dá o motivo. Uma seção nula, como `translation`, e um sinal de `observability` definido como `settings`
não definem nada, então `settings` pode nomear as chaves deles. Relatórios de erros para o Sentry, por exemplo:

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

As configurações alteradas manualmente na plataforma não duram: o próximo apply restaura o que o bloco diz.

<!-- source: secrets d58c587dc144 -->
### Segredos

Nenhum valor secreto que você fornece chega ao plano ou ao estado do OpenTofu. O módulo gera o que pode por meio de atributos
somente de escrita: a KEK, a chave VAPID e a senha de um banco de dados que ele cria. A única exceção é o segredo de origem de
`ingress.kind = cloudflare`, que a regra do Cloudflare guarda, então ele vai parar no estado, que deve permanecer privado.

Os segredos que só você tem, como a URL de um banco de dados externo, uma senha SMTP, uma chave de acesso ou as chaves de
`secrets`, o módulo cria vazios. `tofu output secrets_to_fill` lista cada um, com o que vai nele e o comando que o
adiciona. Um servidor cujo segredo não tem valor não inicia. O servidor lê os segredos quando uma instância ou uma task inicia,
então um novo valor passa a valer na próxima.

**O OpenTofu gera a KEK uma vez e nunca mais.** O segredo e a versão dele têm `prevent_destroy`, e a versão que
gravaria um novo valor é fixa no módulo: uma nova KEK aleatória torna ilegível tudo o que a antiga criptografou. Para
rotacioná-la, siga [Rotacionar a KEK](key-management.md#rotate-the-kek), adicionando o novo valor com a CLI da plataforma, que
o OpenTofu não rastreia.

<!-- source: first-deployment 9ae06fd14fc1 -->
## Primeira implantação

Você precisa do `tofu` 1.11 ou posterior, do `jq` e da CLI da plataforma (`gcloud` ou `aws`) onde você aplica, já que um apply executa a migração a partir
dali; de um domínio para [`host`](../reference/configuration.md#host), que não pode mudar depois
([Configurações que você não pode mudar depois](production-checklist.md#settings-you-cannot-change-later)); e de uma versão do `my` a partir das
[releases](running.md#get-the-image). Não existe tag `latest`. Faça primeiro o que a página da plataforma pede, como criar
o banco de dados.

<!-- source: 1-copy-the-example-and-set-the-variables 8f1ba80ba8d2 -->
### 1. Copie o exemplo e defina as variáveis

Copie o exemplo, com o arquivo de lock dos provedores, da release que você implanta para um diretório vazio:

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Defina `ref` como essa release** em `main.tf`, o que a cópia de uma release estável já tem certo. Para uma pré-release, defina
`ref` e a imagem, a versão sem o `v`, já que o módulo de uma pré-release não usa por padrão a própria imagem:
`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf` e `image = { version = "0.1.0-rc.4" }` no bloco.

Preencha `terraform.tfvars` e adicione ao bloco em `main.tf` qualquer seção que você mude. Decida agora sobre
[`account_domain`](../reference/configuration.md#account_domain), como `settings = { account_domain = "example.org" }`, se
os handles usam outro domínio.

Mantenha este diretório: ele guarda as suas configurações e, por padrão, o estado. O estado não guarda valores secretos, mas é o único
registro do que foi criado: mantenha-o em um backend remoto, como um bucket `gcs` ou `s3`, se mais de uma pessoa aplica.

<!-- source: 2-create-the-secrets-and-add-their-values 9f13e8109bc9 -->
### 2. Crie os segredos e adicione os valores deles

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` busca o módulo e os scripts dele naquela release, sob o nome do seu bloco: `migrate.sh`, que um apply
executa, e `console.sh`, que executa [o console](#the-console).

Aplique primeiro só os segredos, o que os cria e gera a KEK e a chave VAPID, com o módulo do tipo da KEK:

| Módulo raiz | Apply |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

Com `kek = { kind = "kms" }`, use como alvo `module.mycroblog.module.kek_kms` no lugar do segredo da KEK.

**Copie a KEK agora** e guarde a cópia separada do banco de dados, por exemplo em um gerenciador de senhas: sem ela um
[backup](backup.md) não pode ser restaurado, e as chaves das contas são perdidas. `tofu output kek` tem o comando. Copie a chave VAPID,
`<name>-vapid-key`, do mesmo modo: perdê-la encerra todas as assinaturas de Web Push.

Depois execute cada comando de `tofu output secrets_to_fill` com o valor dele. Adicione todos antes do próximo passo.

<!-- source: 3-apply b9f715c7b55d -->
### 3. Aplique

```sh
tofu apply
```

Isto cria o job da migração com o que ele precisa, executa `my upgrade` nele por meio de `migrate.sh` e espera por ele:
`my config check`, `my db upgrade -check`, `my schema apply` e `my db upgrade`, que para no primeiro que falha. A saída
dele aparece na do apply. O primeiro comando que usa o banco de dados grava nele `host` e `account_domain`. Só então
o apply cria os serviços, cuja verificação de saúde falha até o esquema existir.

Se a migração falha, o apply para antes dos serviços. Corrija a causa, em geral um segredo sem valor, e execute
`tofu apply` de novo: ele executa a migração outra vez.

<!-- source: 4-point-the-domain-at-the-server b7b1ead901bc -->
### 4. Aponte o domínio para o servidor

`tofu output dns_records` lista os registros a adicionar no seu provedor de DNS, e fica vazio onde o módulo os cria.

<!-- source: 5-create-the-first-administrator bb599aec43c2 -->
### 5. Crie o primeiro administrador

O console não tem terminal nem entrada padrão, então `my admin create` imprime um link que define a senha em vez de
lê-la:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Abra o link imediatamente e defina a senha. Ele funciona uma vez, durante uma hora, e **fica nos logs da plataforma até lá**,
onde qualquer pessoa que possa lê-los pode usá-lo. Se ele expirar, execute `my admin account password-reset-link @alice` pelo
console. Depois ative a autenticação em dois fatores.

<!-- source: 6-go-through-the-checklist 12999c41613d -->
### 6. Percorra a lista de verificação

Percorra a [lista de verificação de produção](production-checklist.md), começando pelo [e-mail](production-checklist.md#email): SPF,
DKIM e DMARC no domínio de `mail.from`.

<!-- source: the-console 1b3d6dfa9c3c -->
## O console

O console executa um comando `my` com a imagem, as configurações e os segredos do serviço. Ele só existe enquanto
`console_enabled` está ativado, então desative-o quando terminar:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` espera o comando, imprime a saída dele, que lê de volta dos logs da plataforma, e termina com o código de saída
dele. Ele usa a imagem que o serviço executa: depois de uma atualização que parou na migração, essa é a release anterior, à qual
o banco de dados ainda se ajusta.

O comando não tem entrada padrão nem terminal, então um comando que lê uma senha ou um arquivo deles não pode ser executado
assim. O que ele imprime fica nos logs, então não execute um comando cuja saída você não escreveria ali.

<!-- source: upgrading 4be8a01734ce -->
## Atualizar

Leia primeiro [Atualizar](database.md#upgrading) e as notas da release, e avance uma versão menor de cada vez. Uma atualização é um
apply da nova release:

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

Leia o plano: o módulo de uma nova release também pode alterar recursos. O apply faz isto e para no primeiro passo que falha:

1. Dá aos jobs de migração e de console (definições de task na AWS) a nova imagem, junto com o resto do plano de que os serviços não dependem.
2. Executa `my upgrade` no job de migração por meio de `migrate.sh`, a partir de onde você aplica, e espera por ele, como na
   [primeira implantação](#3-apply). Antes, ele espera qualquer execução que um apply interrompido deixou em andamento.
3. Move cada worker para a nova imagem e depois o serviço. Uma nova revisão só recebe tráfego depois que a verificação de saúde dela passa.

**Onde você aplica** precisa do `jq`, da CLI da plataforma e de credenciais que possam executar o job e ler os logs dele, que a
página da plataforma lista. O mesmo vale para um job de CI que aplica.

**Quando um passo falha**, os serviços permanecem nas suas revisões, e as notas da release ou a saída da migração dizem por quê. Corrija
a causa e execute `tofu apply` de novo com a mesma versão: ele executa a migração outra vez, e os passos que terminaram não encontram
mais nada a fazer. Um apply da mesma imagem não executa nada. Para voltar depois que o esquema mudou, restaure o backup: veja
[Reverter](database.md#rolling-back). Um apply de uma versão anterior para na migração, já que essa release recusa um esquema
mais novo, e não altera nenhum serviço.

Para experimentar uma release candidate, ou para manter a imagem de uma release anterior sob um módulo mais novo, defina você mesmo a versão, a
tag da imagem sem o `v` da release:

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 84178f101ce0 -->
### Verificar o SQL primeiro

O plano não mostra o SQL da migração. Para lê-lo antes do apply, aplique apenas o job de migração com a nova
imagem, execute nele `my upgrade -dry-run`, que não altera nada, e leia os logs. A página da plataforma tem os comandos.

<!-- source: destructive-changes 0bea88d60224 -->
### Alterações destrutivas

Quando as notas da release pedem `-allow-destructive`, nomeie essa versão no bloco:

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` recebe `-allow-destructive` apenas no apply dessa versão, então um valor deixado ali não permite que nenhuma release
posterior descarte dados.

<!-- source: pinning-the-image-by-digest 9c3f49e5ee10 -->
### Fixar a imagem por digest

A [lista de verificação de produção](production-checklist.md#operations) recomenda um digest, das notas da release:

```hcl
  image = { digest = "sha256:..." }
```

Um digest nomeia uma única imagem, qualquer que seja a tag ao lado dele, então mude-o na mesma edição que `ref`: um digest deixado da
release anterior mantém a imagem dessa release sob a nova versão.

<!-- source: the-maintenance-mode 57bd5487b045 -->
### O modo de manutenção

O [modo de manutenção](maintenance.md) exige três applies, já que os serviços mudam antes da atualização e depois dela. Adicione-o
às configurações do bloco e aplique, faça o backup, aumente `ref` e aplique, depois remova-o e aplique:

```hcl
  settings = { maintenance = "true" }
```

Sem ele, a revisão antiga continua atendendo durante a migração, o que
[não tem garantia de funcionar](database.md#the-server-during-an-upgrade).

<!-- source: splitting-jobs-into-workers 6336c0873c82 -->
## Dividir jobs em workers

Quando um tipo de job atrasa o resto, dê a ele o próprio serviço. Decida o que dividir a partir das métricas, como
[Dividir os workers por tipo de job](running.md#splitting-workers-by-job-type) diz. `workers` aceita um mapa por uma chave que nomeia cada
worker, com os `job_types` dele, e o módulo cria a partir dele cada fila, rota e configuração da divisão, então você não escreve nenhuma
delas. Para impedir que as entregas atrasem o resto:

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

Para manter rápidos o e-mail e as notificações:

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
    }
  }
```

O plano recusa uma divisão dos tipos que devem ficar juntos. Deixe de fora os tipos de configurações que você não definiu, como essa página
diz: um tipo que o servidor não registrou impede a inicialização, e o plano não consegue ver quais são. O serviço padrão
executa todos os tipos que nenhum worker nomeia, e nenhum dos que algum nomeia.

Um worker também aceita o tamanho do próprio serviço, cada um o do serviço padrão onde não é nomeado:
[a referência dos módulos](../../deploy/opentofu/README.md#workers) lista os campos de cada plataforma. Um worker não recebe
tráfego de fora, e uma atualização o move depois da migração e antes do serviço padrão. Cada worker pode trazer o que
o serviço custa, e as conexões das instâncias dele contam para o limite do banco de dados.

<!-- source: changing-a-backend df886f733f8e -->
## Trocar um backend

Mude o `kind` da seção dele. Cada tipo define no servidor o que precisa, e o plano diz o que muda. Trocar o
banco de dados também significa mover os dados dele, já que um novo está vazio: faça um [backup](backup.md) do antigo e restaure-o no
novo enquanto nada grava.

<!-- source: the-kek 62356f591998 -->
### A KEK

Mudar `kek.kind` é uma linha, mas em uma instância com dados não é de graça: o banco de dados guarda segredos encapsulados com a
chave atual, entre eles as chaves de todas as contas, então uma instância que a perde não consegue mais lê-los. Siga
[Mover entre a KEK local e um KMS](key-management.md#move-between-the-local-kek-and-a-kms). Do segredo para um KMS, mantenha
a KEK atual como `kek_previous` enquanto os segredos são movidos: adicione `secrets = ["kek_previous"]` e preencha o segredo
`<name>-kek-previous` com a KEK atual. Execute `my kek rewrap` pelo [console](#the-console) e depois remova-a quando
`my kek status` não contar mais chaves `local:`. Mantenha a KEK por todo o tempo em que você mantiver backups do banco de dados anteriores ao rewrap.

Em uma instância nova sem dados, nada precisa ser movido.
