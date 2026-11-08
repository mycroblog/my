<!-- source: production-checklist e5f6c0c5b10a -->
# Lista de verificação de produção

Siga esta lista antes de abrir o servidor para os usuários e para outros servidores, e de novo depois de qualquer mudança na implantação. Algumas configurações não podem ser alteradas depois que o servidor já federou, e alguns erros, como perder a chave de criptografia de chaves, não têm como ser desfeitos.

Cada item diz por que importa, como verificar e onde estão os detalhes.
Comece pela parte automática:

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

<!-- source: what-my-config-check-checks f5feea1abf22 -->
## O que o `my config check` verifica

No perfil `production` (o padrão de [`profile`](../reference/configuration.md#profile)), o `my config check` primeiro valida cada configuração e depois verifica se as configurações válidas são adequadas a uma instância de produção.
Os problemas são listados em dois grupos, sob um resumo no idioma da localidade do processo, cada um com o detalhe técnico em inglês:

- **Erros** fazem o `my config check` terminar com código 1. São configurações que quebram uma instância de produção ou enfraquecem a sua segurança:
  - `host` ou `account_domain` que outros servidores não conseguem alcançar: com uma porta, `localhost`, um nome sem ponto ou um nome sob um domínio de topo reservado para uso local ou testes (`.localhost`, `.local`, `.test`, `.example`, `.invalid`, `.internal`, `.home.arpa`).
  - `web_origin` com `http`.
  - Um `database_url` do SQLite que mantém o banco de dados na memória.
  - Um `kek` que não é aleatório, como todos zeros ou um byte repetido.
  - Uma chave de site ou chave secreta do `captcha` que é uma das chaves de teste publicadas pelos serviços de CAPTCHA.
  - `worker_job_types`, `worker_job_types_except` ou uma rota de fila que não nomeia nenhum tipo de job (em todos os perfis; `my job types` os lista).
  - Com `-instance`, um `worker_job_concurrency` que o pool de conexões do banco de dados não consegue atender.
- **Avisos** mantêm o código de saída em 0. São configurações comuns em desenvolvimento e que podem ser deliberadas em produção:
  - `mail = none`.
  - Um banco de dados SQLite ou o armazenamento de blobs `fs` em um diretório temporário (`/tmp`, `/var/tmp`, `/dev/shm` ou `$TMPDIR`).
  - `queue = inline`.
  - Nenhum `vapid_private_key`.
  - `stream = hub` ou `kv = memory` com um `role` diferente de `all`: elas funcionam apenas dentro de um processo.
  - Configurações de voz que provavelmente não foram pretendidas, como `voice = p2p` sem STUN ou TURN.
  - Um [plugin](plugins.md) em um endereço `http://`, ou em um endereço `https://` sem token nem certificado de cliente.
  - Uma chave de modo de teste do provedor de pagamento.
  - `origin_secret` definido com `trusted_proxies` vazio, e `origin_secret_previous` deixado definido depois de uma rotação.
  - Com `-instance`: os textos da instância `name`, `short_description`, `terms` e `privacy_policy`; as configurações da instância `contact_account`, `contact_email` e `security_contact` quando não estão definidas; e um servidor sem regras.
    Quando não é possível alcançar o banco de dados, isso é mais um aviso, e o restante da verificação continua.

`my serve` registra os mesmos achados como avisos quando inicia, mas inicia mesmo assim.
O perfil `development` pula essas verificações; use-o para desenvolvimento local e testes, nunca para uma instância que outras pessoas usam.

Execute `my config check` no mesmo ambiente do servidor (as mesmas variáveis de ambiente, arquivos de segredo e arquivo de configuração),
por exemplo como um passo da implantação, antes que a nova versão receba tráfego.

<!-- source: settings-you-cannot-change-later b2c961cb26f5 -->
## Configurações que você não pode mudar depois

<!-- source: the-servers-domain-and-the-handle-domain b01ce235bcc0 -->
### O domínio do servidor e o domínio dos identificadores

- **Por quê**: `host` faz parte de todo id do ActivityPub do servidor, e outros servidores armazenam esses ids.
  `account_domain` é o domínio dos identificadores (`@alice@example.org`). A primeira execução de `my serve` ou de um comando de `my` que usa o banco de dados grava os dois nele, e a partir daí `my serve` e esses comandos se recusam a rodar quando a configuração difere:
  uma mudança depois de federar quebra todo seguimento e toda referência às publicações e contas do servidor.
  Defina os dois antes da primeira inicialização. Um erro encontrado antes de federar é corrigido
  [recriando o banco de dados](database.md#recreating-erasing-every-row), o que apaga todas as linhas.
- **Decida**: se os identificadores usam o domínio do servidor ou um domínio separado (por exemplo, identificadores em `example.org` e o servidor em `social.example.org`). Um domínio de identificadores separado precisa redirecionar o seu WebFinger, host-meta e NodeInfo para o servidor.
- **Verifique**: `my config check` mostra `host` e `account_domain` e informa um domínio que outros servidores não conseguem alcançar.
  `my config check -instance` também falha quando eles diferem dos valores no banco de dados, e não grava nada.
  Com um domínio de identificadores separado, `curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"`
  responde `301` para `https://<host>/.well-known/webfinger?...`.
- **Detalhes**: [executando o servidor](running.md) (os endpoints de federação e a entrega dos identificadores a partir de outro domínio),
  [`host`](../reference/configuration.md#host) e [`account_domain`](../reference/configuration.md#account_domain).

<!-- source: reachable-over-https 1cf53c613f7b -->
### Acessível por `https`

- **Por quê**: outros servidores buscam os ids em `https://<host>/` na porta 443, e os navegadores enviam os cookies de login apenas por `https`.
- **Verifique**: `curl -sI https://<host>/healthz` responde `200` de fora da sua rede, com um certificado válido para `host`,
  e `my config check` não informa nenhum erro sobre `web_origin`. `http://<host>/` redireciona para `https`, o que o proxy reverso, o balanceador de carga ou a CDN na frente do servidor faz (com o Docker Compose, a camada `compose.caddy.yaml` é esse proxy).
- **Detalhes**: [executando o servidor](running.md), [`web_origin`](../reference/configuration.md#web_origin).

<!-- source: not-reachable-past-the-cdn 58310b9f29d7 -->
### Não acessível por trás da CDN

- **Por quê**: com uma CDN na frente, e com o endereço próprio do servidor ainda público (`*.run.app` no Cloud Run), qualquer pessoa pode chamar o servidor passando pelo WAF e pelo cache,
  e quem chama a partir da própria plataforma da CDN pode escolher o IP do cliente que os limites de requisições enxergam. Isso só é necessário nesse caso: não quando um firewall ou a plataforma já limita quem pode se conectar,
  e sem CDN não há o que contornar.
- **Verifique**: `origin_secret` e `origin_secret_header` estão definidos e a CDN define o cabeçalho em todos os nomes de host. `curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/`
  responde `403`, o `/healthz` dele responde `200`, e `https://<host>/` responde `200`.
- **Detalhes**: [Recuse requisições que não passaram pela CDN](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), incluindo como se recuperar quando isso bloqueia todo mundo.

<!-- source: secrets ec71f1f20574 -->
## Segredos

<!-- source: the-key-encryption-key-kek 89df548c5bf9 -->
### A chave de criptografia de chaves (KEK)

- **Por quê**: a KEK criptografa as chaves privadas de todo ator. Sem ela o servidor não consegue assinar nada, e as chaves não podem ser recuperadas do banco de dados: cada conta teria de ser recriada e os outros servidores deixariam de confiar nele.
- **Verifique**:
  - Ela foi gerada de forma aleatória (`openssl rand -base64 32`); `my config check` informa uma chave que não é aleatória.
  - Ela é guardada em um lugar diferente do banco de dados e dos seus backups, por exemplo em um gerenciador de senhas ou no cofre de segredos da nuvem, e ao menos mais uma pessoa que executa o servidor consegue acessá-la.
  - Ela chega ao servidor somente por `MY_KEK` ou `MY_KEK_FILE` (o arquivo de configuração recusa segredos). Com o Docker Compose, ela é o arquivo `secrets/kek` e não fica no `.env` ([arquivos do Docker Compose](running.md#docker-compose-files)).
- **Detalhes**: [gerenciamento de chaves](key-management.md).

<!-- source: the-vapid-key-of-web-push 76bc2a1dd030 -->
### A chave VAPID do Web Push

- **Por quê**: sem ela o servidor não envia notificações Web Push. Mudá-la depois encerra todas as assinaturas, então gere-a uma vez e guarde-a como a KEK.
- **Verifique**: `my config check` avisa quando `vapid_private_key` não está definida. Gere uma com `my config vapid-key`.
- **Detalhes**: [gerenciamento de chaves](key-management.md) (a chave VAPID do Web Push).

<!-- source: other-secrets-stay-out-of-the-repository-and-the-image 94f290afcdbb -->
### Os demais segredos ficam fora do repositório e da imagem

- **Por quê**: um segredo em um repositório, em uma imagem de contêiner ou em um arquivo de configuração é copiado para todo lugar aonde eles vão.
- **Verifique**: `database_url`, `mail_smtp_password`, `captcha_secret_key`, as chaves do provedor de pagamento, `valkey_url` e toda outra configuração marcada como segredo na referência de configuração vêm de variáveis de ambiente ou de arquivos `<NAME>_FILE` que o cofre de segredos da plataforma preenche (segredos do Docker ou do Kubernetes, Secret Manager, Parameter Store). O arquivo de configuração não pode conter esses valores:
  `my config check` recusa um segredo ali. Com o Docker Compose, eles vêm do `.env`, que só você pode ler (`chmod 600`). Pesquise no repositório e na imagem para ter certeza de que não sobrou nenhuma cópia dos valores.
  `my config check` mostra `(redacted)` para cada segredo que estiver definido.
- **Detalhes**: [referência de configuração](../reference/configuration.md) (a coluna Secret), [CAPTCHA](captcha.md),
  [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md).

<!-- source: instance-settings baae9cc3e09e -->
## Configurações da instância

As configurações da instância ficam no banco de dados e podem ser alteradas enquanto o servidor roda, nas telas de administração do servidor
ou com `my instance set`. `my config check -instance` avisa sobre os textos, contatos e regras que não estão definidos.

- **Nome, descrição curta e descrição**: exibidos na página inicial, na página de informações (`/about`), nas prévias de link e no NodeInfo. Verifique em `/about`.
- **Contatos**: `contact_account` (a conta a que os usuários recorrem sobre o servidor) e `contact_email` (o endereço para o qual usuários e outros servidores escrevem; os e-mails enviados pelo servidor também levam as respostas para ele). Verifique em `/about`.
- **Contato de vulnerabilidades**: `security_contact`, um URI `https:` ou `mailto:`. Definido, o servidor responde em `/.well-known/security.txt`. Verifique com `curl -s https://<host>/.well-known/security.txt`.
- **Termos de serviço e política de privacidade**: `terms` e `privacy_policy`. A política de privacidade deve citar os serviços que recebem dados pessoais, como o serviço de CAPTCHA e o provedor de e-mail. Verifique os links em `/about` e na tela de cadastro.
- **Regras do servidor**: o que os usuários aceitam ao se cadastrar e o que os moderadores fazem valer. Adicione-as com `my instance rule create` ou nas telas de administração, e verifique-as em `/about`.
- **Modo de cadastro**: `registration_mode` é `approval` até que você o altere; escolha `open`, `approval`, `invite` ou `closed` para definir como as pessoas entram. Com `mail = none`, os cadastros `open` também aguardam a aprovação de um moderador.
- **Buscadores e IA**: `search_indexing`, `ai_usage` e `indexable_default` recusam ambos até que você os permita. Verifique `/robots.txt` e `/.well-known/tdmrep.json`.
- **Página de status**: defina [`status_page_url`](../reference/configuration.md#status_page_url) (uma configuração, não uma configuração da instância, para que o link apareça durante a manutenção) com uma página de status em um serviço separado do servidor.

**Detalhes**: [configurações da instância](instance-settings.md), [cadastro](registration.md),
[buscadores e rastreadores de IA](crawlers.md), [CAPTCHA](captcha.md) (o que escrever na política de privacidade),
[modelo de política de privacidade](privacy-policy-template.md), [modelo de registro de atividades de tratamento](processing-record-template.md),
[monitoramento](monitoring.md) (a página de status).

<!-- source: email 40c18999c46f -->
## E-mail

- **Por quê**: o e-mail verifica endereços e envia redefinições de senha e notificações. Com `mail = none`, nenhum e-mail chega aos usuários.
- **Verifique**:
  - `mail` é `smtp`, `ses` ou `plugin`, e `mail_from` é um endereço de um domínio que você controla; `my config check` avisa sobre `mail = none`.
  - O domínio de `mail_from` tem registros SPF, DKIM e DMARC, e o provedor assina com DKIM. Sem eles, as mensagens caem no spam.
  - Cadastre-se com uma conta de teste e receba o e-mail de verificação; solicite uma redefinição de senha e receba também esse. Verifique os cabeçalhos da mensagem recebida para `spf=pass`, `dkim=pass` e `dmarc=pass`.
  - Com o SES, a conta saiu do sandbox e as notificações de rejeição e reclamação chegam ao servidor.
- **Detalhes**: [e-mail](email.md), [cadastro](registration.md) (verificação por e-mail).

<!-- source: administrators 2c05e6644d88 -->
## Administradores

- **Por quê**: a conta de um administrador pode alterar qualquer configuração, ler dados privados e suspender qualquer pessoa.
- **Verifique**:
  - Todo administrador ativou a autenticação em dois fatores (Configurações, Autenticação em dois fatores no cliente web)
    e guarda os códigos de recuperação.
  - Apenas as pessoas que precisam dele têm o papel `admin`; os moderadores têm `moderator` ou um papel com apenas as permissões que usam.
    Liste-os com `my admin role` e revise-os nas telas de administração.
- **Detalhes**: [administradores e papéis](admin-cli.md), [moderação](moderation.md) (permissões e recuperação de login).

<!-- source: operations 56bd643c2241 -->
## Operação

- **Backups e recuperação**: o banco de dados, o armazenamento de blobs (`blob_fs_dir` ou o bucket) e a KEK têm backup agendado, em um lugar separado do servidor, e uma restauração para um servidor novo foi ensaiada: o servidor restaurado inicia e as contas conseguem entrar e publicar. Um backup que nunca foi restaurado não se sabe se funciona. O banco de dados e a mídia não podem estar em um diretório temporário; `my config check` avisa quando estão.
  Veja [gerenciamento de chaves](key-management.md) (backup da KEK) e [banco de dados](database.md).
- **Monitoramento e alertas**: um monitor externo verifica `/healthz` e avisa alguém que possa agir, e os jobs mortos da fila de jobs são observados. Veja [monitoramento](monitoring.md) e [executando o servidor](running.md) (verificações de saúde, métricas e a fila de jobs).
- **A imagem**: a instância executa uma imagem lançada e fixada por digest (`ghcr.io/mycroblog/my:0.1.0@sha256:...`, das notas da versão), e não uma que muda a cada reinício, e [a sua proveniência foi verificada](running.md#verify-an-image). Não existe tag `latest`.
  Uma atualização muda essa versão de propósito e segue [Atualizando](database.md#upgrading), uma versão menor de cada vez.
- **Código-fonte (AGPL-3.0)**: o servidor precisa oferecer o seu código-fonte a todos que o usam. Deixe [`source_url`](../reference/configuration.md#source_url) no padrão para uma versão lançada sem modificações; aponte-o para o seu próprio código-fonte quando executar um fork ou uma versão modificada. Verifique o link em `/about`.
  Veja [executando o servidor](running.md) (publicação do código-fonte).
- **Manutenção**: saiba colocar o servidor em modo de manutenção antes de precisar dele. Veja [manutenção](maintenance.md).

<!-- source: federation a5ab186ec04b -->
## Federação

- **Por quê**: outros servidores encontram contas por meio do WebFinger e do NodeInfo e depois buscam o ator. Se qualquer um deles falhar a partir de fora,
  o servidor fica invisível para o fediverso.
- **Verifique** a partir de uma máquina fora da sua rede:
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` retorna a conta,
    seguindo o redirecionamento para `host` quando os domínios diferem (`curl -L`).
  - `curl -s https://<host>/.well-known/nodeinfo` aponta para `/nodeinfo/2.1`, que informa o software e o seu código-fonte.
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` redireciona para o id do ator,
    e o ator nesse id é JSON com um `publicKey`.
  - A partir de uma conta no Mastodon ou em outro servidor, pesquise por `@<user>@<account_domain>`, siga-a e veja uma nova publicação chegar.
  - Uma CDN na frente do servidor não faz cache de `/ap/*` e `/@*`, a menos que respeite `Vary: Accept`.
- **Detalhes**: [executando o servidor](running.md) (os endpoints de federação), [CDN](cdn.md), [relays](relays.md).
