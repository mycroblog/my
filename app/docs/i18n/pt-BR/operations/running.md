<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# Executando o servidor: iniciar, parar e verificações de saúde

Como iniciar e parar o servidor, obter a imagem dele e acompanhá-lo enquanto roda: métricas, traces, a fila de jobs e verificações de saúde.

<!-- source: start-the-server 0d5f8898a2f5 -->
## Iniciar o servidor

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` valida a configuração, monta os adaptadores de banco de dados, fila de jobs e gerenciamento de chaves, e aceita HTTP em `listen`.
Ele fala HTTP/1.1 puro ali, sem TLS: coloque na frente um proxy reverso ou balanceador de carga que encerre o HTTPS (e o HTTP/2 ou HTTP/3 para os clientes) e encaminhe por HTTP/1.1.
As atualizações em tempo real (`GET /api/v1/streaming`, os streams das linhas do tempo local e federada e os streams das salas de chat de voz) são respostas de longa duração que enviam um comentário de heartbeat a cada 30 segundos (15 em uma sala de voz) e permanecem abertas enquanto ociosas.
O proxy não deve armazenar essas respostas em buffer (nginx: o servidor envia `X-Accel-Buffering: no`), e o timeout de ociosidade ou de leitura dele deve ser maior que o intervalo do heartbeat.
Ele não aplica o schema: aplique-o com `my schema apply` antes de iniciar (veja [Banco de dados](database.md); as atualizações o executam como parte de [Atualizando](database.md#upgrading)).
Na configuração do Docker Compose (`deploy/docker/compose.yaml`; veja [Implantando com Docker Compose](deploy-docker-compose.md)), um serviço `schema` de execução única roda `my schema apply` antes de o servidor iniciar.
Se o schema não foi aplicado, o processo ainda inicia e a verificação de prontidão retorna 503, e ele não serve mais nada até o schema existir.

O primeiro `my serve`, ou comando `my` que usa o banco de dados, registra nele `host` e `account_domain`. Quando a configuração difere dos valores registrados, `my serve` registra no log `start failed` com os dois pares de valores e termina com código 1, e os comandos exibem o mesmo erro. As exceções são `my schema apply` e `my db backup`, que não verificam, e o modo de manutenção, que não lê o banco de dados. `my config check -instance` e `my db upgrade -check` verificam, mas não registram nada.
Um processo que não conseguiu ler o banco de dados quando iniciou não fica pronto até conseguir e, até lá, não executa jobs e responde `503` com `Retry-After` a toda requisição, exceto as três verificações de saúde abaixo (`/healthz`, `/readyz`, `/api/v1/instance/health`). É o que acontece também com um servidor iniciado antes de `my schema apply`. Se depois ele encontrar valores diferentes, para com o mesmo erro.

No perfil `production`, `my config check` também verifica se as configurações válidas servem a uma instância de produção: erros, como um `host` que outros servidores não conseguem alcançar ou uma KEK que não é aleatória, fazem-no terminar com código 1, e avisos, como `mail = none`, não.
`my serve` registra ambos como avisos ao iniciar, mas inicia mesmo assim; portanto, execute `my config check` como uma etapa da implantação.
O [checklist de produção](production-checklist.md) lista as verificações e o que conferir manualmente antes de entrar no ar.

| `role` | HTTP | Worker de jobs |
| --- | --- | --- |
| `all` (padrão) | Todas as rotas | Executa |
| `web` | Todas as rotas | Não executa |
| `worker` | Somente verificações de saúde (para as probes da plataforma) | Executa |

Os logs vão para a saída de erro padrão como JSON Lines. Toda linha tem `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) e `severity`, o mesmo nível na forma que o Cloud Logging lê (`DEBUG`, `INFO`, `WARNING`, `ERROR` e `CRITICAL` para um erro que encerra o processo).
Os logs escritos ao tratar uma requisição trazem um `request_id`, e o mesmo ID é devolvido no cabeçalho de resposta `X-Request-Id`.
Se o `X-Request-Id` da requisição tiver no máximo 128 caracteres entre letras, dígitos e `-_.:`, ele é usado como está, como o ID atribuído por um balanceador de carga.
Os logs dentro de um trace (requisições e jobs) também trazem `trace_id` e `span_id` (veja [Traces](#traces)).

Cada requisição HTTP e cada execução de job escreve uma linha ao terminar. A linha tem os mesmos nomes e valores dos rótulos das [métricas](#metrics) abaixo,
de modo que, no Cloud Run com `metrics_exporter = none`, você pode criar métricas baseadas em logs do Cloud Logging (contadores e distribuições) a partir dessas linhas.

| Linha (`msg`) | Campos |
| --- | --- |
| `http request` (`INFO`; as verificações de saúde usam `DEBUG`) | `http.request.method`, `http.route`, `my.surface` e `http.response.status_code` (um número) têm os mesmos valores dos rótulos de `http.server.request.duration`. Também `duration_seconds` (um número de segundos), `url.path` (o caminho sem a query string), `http.response.body.size` e `request_id`. `http.route` e `my.surface` estão ausentes nas requisições que não chegaram a uma rota (verificações de saúde, requisições recusadas antes do roteamento), e a métrica não as conta. Uma requisição cujo cliente saiu antes de o servidor poder responder tem o status `499` em vez de um 5xx, a menos que um handler tenha entrado em pânico; o cliente não recebeu nada, então ela não conta como erro do servidor |
| `job` (`INFO` em caso de sucesso e para `postponed`, `WARN` para `retry`, `ERROR` para `dead`) | `my.job.type` e `my.outcome` têm os mesmos valores dos rótulos de `my.job.duration`. Também `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt` e `error` em caso de falha |
| `error report` (`ERROR`; `CRITICAL` para uma falha que encerra o processo) | Uma linha por erro reportado ([Relatórios de erro](#error-reports)) |

<!-- source: error-reports 3d780744461c -->
### Relatórios de erro

O guia [Relatório de erros](error-reporting.md) explica onde ler essas linhas em cada plataforma, a tela Erros, as notificações e o envio de relatórios a um rastreador de erros.

O servidor reporta os erros que precisam da atenção do operador como uma linha `error report` cada:

- um pânico em um handler HTTP, um job ou uma tarefa em segundo plano;
- um erro por trás de uma resposta 5xx da API de cliente, da API de administração, das páginas de login, do ActivityPub, do OAuth ou de mídia;
- um job que falha de vez (`dead`): ele esgotou as tentativas ou se recusou a executar de novo. As entregas que desistiram porque o outro servidor continuou falhando não são reportadas;
- uma falha ao iniciar: uma configuração inválida, ou um banco de dados ou adaptador que não pode ser configurado.

Respostas 4xx, novas tentativas de jobs, o modo de manutenção, verificações de prontidão com falha, requisições cujo cliente saiu e falhas causadas por outros servidores não são reportadas; as métricas e as demais linhas de log cobrem esses casos.

Quando o cliente de uma requisição vai embora, as outras linhas de erro do servidor para essa requisição são escritas em `INFO` em vez de `ERROR`, já que a maioria delas apenas diz que a requisição foi cancelada.

A linha está na forma que o Google Cloud Error Reporting lê: no Cloud Run, os relatórios aparecem no Error Reporting agrupados como erros, sem nenhuma configuração. Em outras plataformas, é uma linha JSON simples:

| Campo | Valor |
| --- | --- |
| `@type`, `serviceContext` | O tipo de evento do Error Reporting, e `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 dígitos hexadecimais que são os mesmos para toda ocorrência do mesmo erro: montados a partir da origem, do local, do tipo do erro e das funções do servidor na pilha, sem números de linha |
| `my.error.source`, `my.error.kind` | `server` (ou `web`), e `panic` ou `error` |
| `my.error.where` | Onde aconteceu: `<surface> <method> <route>` para uma requisição (como `client_api GET /api/v1/statuses/{id}`), o tipo do job para um job, o nome da tarefa para uma tarefa em segundo plano, `startup` para uma falha ao iniciar |
| `my.role`, `deployment.environment.name` | O `role` do processo e `errors_environment` |
| `request_id`, `job_id`, `attempt`, `trace_id`, `span_id` | Os IDs para encontrar as linhas de requisição ou de job do mesmo erro, quando ele os tem |
| `message` | Por último na linha: `<type>: <message>`, uma quebra de linha e a pilha do Go |

Antes de um relatório sair do processo, a mensagem tem URLs, endereços de e-mail, handles `@user@host`, endereços IP, UUIDs e ULIDs e números de 10 dígitos ou mais substituídos por `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` e `<n>`, e é cortada em 1 KiB.
A pilha mantém as posições de arquivo, mas não os valores dos argumentos das linhas de função (eles aparecem como `(...)`), e é cortada em 16 KiB. Nenhum relatório traz corpo de requisição, cabeçalho, query, cookie, conta, nome de usuário, endereço IP ou User-Agent.
Nenhuma outra linha traz uma pilha; assim, o Error Reporting encontra apenas as pilhas dessas linhas.

O nome de um campo é uma única chave que contém `.`; ela não é aninhada. Coloque-o entre aspas nos filtros e extrações do Cloud Logging (por exemplo `jsonPayload."http.route"`, e `EXTRACT(jsonPayload.duration_seconds)` para o valor de uma distribuição).
Os campos usados como rótulos recebem valores de um conjunto finito apenas; `url.path`, `request_id` e `job_id` nunca são rótulos.

Com `metrics_exporter = emf`, essas duas linhas são as próprias linhas EMF. A linha `http request` traz o valor de `http.server.request.duration`, e a linha `job` traz os valores de `my.job.duration` e `my.job.wait` (chaves com o nome das métricas),
além da diretiva `_aws`, `service.name`, `service.version` e `my.role`. Essas três métricas não são escritas em nenhuma outra linha. Os campos que você escolhe em `metrics_emf_dimensions` são escritos como strings, como o EMF exige (por exemplo, a dimensão `http.response.status_code` é `"200"`).
Os logs vão para a saída de erro padrão e as demais linhas EMF vão para a saída padrão. O driver de log `awslogs` do ECS envia ambas ao CloudWatch Logs, de modo que as métricas são extraídas de ambas.

<!-- source: docker-compose-files df45a902f45b -->
## Arquivos do Docker Compose

`deploy/docker/` contém o que implanta uma instância. `compose.yaml` roda sozinho, e os outros arquivos se sobrepõem a ele.

| Arquivo | Adiciona | Use quando |
| --- | --- | --- |
| `compose.yaml` | O servidor e seus serviços de banco de dados de execução única, com SQLite e arquivos de mídia no volume `data` | Sempre |
| `compose.postgres.yaml` | PostgreSQL em um contêiner, no volume `postgres` | Você quer PostgreSQL nesta máquina. Para um servidor PostgreSQL em outro lugar, deixe-o de fora e defina `MY_DATABASE` e `MY_DATABASE_URL` em `.env` |
| `compose.s3.yaml` | Mídia em um bucket compatível com S3; com `COMPOSE_PROFILES=garage`, também um armazenamento [Garage](https://garagehq.deuxfleurs.fr/) de um nó no volume `garage` | A mídia passa de um disco, ou você a quer em um bucket. Sem o perfil, o armazenamento é o externo das configurações `MY_BLOB_S3_*` |
| `compose.caddy.yaml` | Caddy, com certificados TLS nas portas 80 e 443 | `MY_HOST` é um domínio público e nada mais encerra o TLS nesta máquina |
| `compose.build.yaml` | Compila a imagem a partir de um checkout | Você executa a sua própria compilação |

Escolha o banco de dados e o armazenamento de mídia antes da primeira inicialização: os dados não se movem sozinhos quando os arquivos mudam
([Backup](backup.md) mostra o que precisa ser movido).

**Sobreponha os arquivos** com opções `-f` nessa ordem, ou nomeie-os uma vez em `.env`, para que todo comando `docker compose` posterior
(`exec`, `logs`, `pull` e os dos outros guias) use os mesmos arquivos:

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

Um arquivo chamado `compose.override.yaml` no diretório só é lido quando nem `-f` nem `COMPOSE_FILE` nomeiam arquivos. Com
eles, liste-o também.

**Configurações.** Copie `example.env` para `.env`. O Compose lê `.env` para preencher as variáveis de seus arquivos e entrega o arquivo
inteiro aos contêineres do servidor; assim, toda configuração da [referência de configuração](../reference/configuration.md) funciona
bastando defini-la ali. O modelo lista todas, com uma frase sobre cada uma.

- A chave de criptografia de chaves fica fora de `.env`, no arquivo `secrets/kek`, e os contêineres a leem como o segredo do Docker
  `/run/secrets/my_kek`, de modo que ela nunca vira uma variável de ambiente. Uma linha `MY_KEK` em `.env` faz o servidor parar, pois ele
  recusa `MY_KEK` e `MY_KEK_FILE` juntos ([gerenciando a chave](key-management.md)).
- O servidor roda como o usuário `65532`, e o Compose monta o arquivo com as permissões que ele tem; portanto, o arquivo precisa ser
  legível por outros. Mantenha-o em um diretório em que só você possa entrar, como fazem os comandos em `example.env`.
- Os outros segredos chegam aos contêineres como variáveis de ambiente. Para passar um como arquivo, monte-o com um segredo
  em um `compose.override.yaml` seu e defina o `<NAME>_FILE` dele, como `compose.yaml` faz para a chave.
- `.env` guarda segredos: `chmod 600 .env`, e mantenha uma cópia junto dos seus backups.
- `MY_LISTEN` permanece `:8080`, pois os arquivos se conectam a essa porta.

**TLS com Caddy.** Defina `MY_HOST` como um domínio cujo DNS aponte para a máquina, com as portas 80 e 443 acessíveis pela
internet, e remova `MY_WEB_ORIGIN` de `.env`. O Caddy obtém e renova o certificado sozinho na primeira requisição.
O servidor confia no `X-Forwarded-For` dos pools de endereços padrão do Docker; defina `MY_TRUSTED_PROXIES` se os seus forem diferentes.

**O Garage incluído.** `compose.s3.yaml` com `COMPOSE_PROFILES=garage` precisa dos valores que `example.env` lista no início.
O serviço de execução única `garage-init` prepara o armazenamento a cada `up`. Ele fica dentro da rede do Compose, já que o servidor
serve a mídia por conta própria. Um nó é um disco: faça backup do volume `garage` ou do bucket ([Backup](backup.md#media-in-a-bucket)).

<!-- source: get-the-image 238d00ea9826 -->
## Obter a imagem

A imagem do servidor é `ghcr.io/mycroblog/my`, pública, para `linux/amd64` e `linux/arm64`, com todos os adaptadores.

| Tag | Aponta para | Use para |
| --- | --- | --- |
| `0.1.0` | Essa versão, para sempre | Nomear uma versão |
| `0.1` | O patch mais recente da `v0.1` | Docker Compose, que aplica o schema a cada `up` |
| `0.1.0-rc.1` | Essa pré-versão | Testar uma versão antes de ela sair |

Não há `latest` nem `0`: ambos atravessariam uma versão menor, e cada atualização para uma nova versão menor precisa passar [pela anterior](database.md#skipping-releases). Nomeie uma versão em todo comando; `docker pull ghcr.io/mycroblog/my` sozinho falha com `manifest unknown`.
Uma versão de patch também pode precisar de `my schema apply` e `my db upgrade`; portanto, fora do Docker Compose, siga [Atualizando](database.md#upgrading) para cada versão. Em produção, fixe o digest que as notas da versão informam:

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

Onde executá-la:

- **Docker Compose**: `deploy/docker/compose.yaml` executa a imagem de `MY_VERSION` em `.env` ([Arquivos do Docker Compose](#docker-compose-files)). Para compilá-la a partir de um checkout, adicione a sobreposição `compose.build.yaml` (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`) e defina `MY_VERSION` com um nome seu, pois ele nomeia a sua imagem local e é o que `my version` exibe.
- **Kubernetes**: baixe-a do `ghcr.io` como está.
- **Cloud Run**: use a imagem do `ghcr.io` diretamente. Uma implantação resolve a tag para um digest.
- **AWS (ECS, EKS)**: copie a imagem para o Amazon ECR, ou use um pull through cache do ECR, que precisa de credenciais do ghcr.io.
- **Uma imagem menor**: `docker build --build-arg VERSION=<name> --build-arg GO_TAGS=... -t my:<name> .` em um checkout compila uma com menos adaptadores ([build tags](../reference/build-tags.md)). É uma compilação sua, portanto não é coberta pelas verificações abaixo.

<!-- source: verify-an-image b696c257662d -->
### Verificar uma imagem

Cada imagem publicada tem uma proveniência assinada pelo Sigstore, que liga o digest ao workflow e ao commit que a compilaram, e um SBOM. Com a [GitHub CLI](https://cli.github.com/), e com login no `ghcr.io` se o comando pedir:

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

O primeiro comando falha, a menos que a imagem tenha sido compilada pelo workflow de release deste repositório. Ele exibe o commit, que é o commit da tag da versão; o rótulo `org.opencontainers.image.revision` da imagem nomeia o mesmo.

<!-- source: publish-the-source-code-agpl-30 2f3b73ce9dfc -->
## Publicar o código-fonte (AGPL-3.0)

`my` é licenciado sob a AGPL-3.0-only. A seção 13 da AGPL-3.0 exige que quem executa o servidor e permite que usuários interajam com ele por uma rede
ofereça a esses usuários uma forma de obter o código-fonte correspondente do programa em execução. Isso vale para todos que interagem com o servidor, não apenas para quem tem uma conta.

Indique onde o código-fonte é publicado com a configuração `source_url` (variável de ambiente `MY_SOURCE_URL`; veja a [referência de configuração](../reference/configuration.md#source_url)).

| Valor | Significado |
| --- | --- |
| Não definido | O padrão, `https://github.com/mycroblog/my`. Use-o quando executar o `my` sem modificações: o `GET /instance` de uma versão então nomeia o código-fonte dessa versão, `https://github.com/mycroblog/my/tree/v0.1.0`, e o NodeInfo o repositório |
| Uma URL `https:` | Um lugar onde o código-fonte pode ser lido (o repositório de um fork, a tag ou o arquivo de uma versão específica, e assim por diante) |
| Um URI `mailto:` | Um endereço de e-mail para pedir o código-fonte (por exemplo `mailto:source@example.org`; `?subject=...` é permitido) |

Qualquer outro valor (`http:`, `git@...`, um caminho relativo, uma URL com informações de usuário ou um `#`, um `mailto:` sem endereço, e assim por diante) faz `my config check` falhar, e o servidor não inicia.

O valor aparece nestes lugares:

- `source_url` no `GET /instance` da API de cliente
- A seção “Código-fonte” da página de informações da instância do cliente web (`/about`) e um link na página de apresentação (`/`)
- `metadata.sourceCode` no NodeInfo (`/nodeinfo/2.1`). Uma URL `https:` também é publicada como `software.repository`
  (o schema do NodeInfo 2.1 permite apenas uma URL `http(s)` em `software.repository`, de modo que um valor `mailto:` aparece somente em `metadata.sourceCode`)

<!-- source: when-you-fork-or-modify-my be83256fe6a3 -->
### Quando você faz um fork ou modifica o `my`

Se você executa um `my` modificado (aplica patches, compila um fork ou troca dependências), a URL padrão deixa de apontar para o código-fonte do que está em execução.
Altere `source_url` para onde o código-fonte da versão em execução, com as suas modificações, pode ser encontrado. Apontar para o commit, a tag ou o arquivo que corresponde à versão em execução, em vez do estado mais recente de um repositório, permite que os usuários obtenham o mesmo código-fonte de forma confiável.
Se você executa uma versão oficial sem modificações, mantenha o padrão.

<!-- source: when-you-use-mailto 11155a8b71ed -->
### Quando você usa `mailto:`

- Esteja pronto para entregar o código-fonte correspondente (incluindo os scripts necessários para compilá-lo) prontamente quando alguém pedir. Apenas nomear um endereço, sem estar preparado para entregar, não cumpre a obrigação.
- Use um endereço que continue funcionando depois que você parar de operar o servidor ou que a pessoa responsável mudar. Um endereço compartilhado de operações é melhor que uma caixa de entrada pessoal.
- Garanta que o filtro de spam não descarte os pedidos.
- Use um endereço que você aceite tornar público: qualquer pessoa pode vê-lo na página de informações, na página de apresentação e no NodeInfo.

<!-- source: metrics 2dea3a7f8f83 -->
## Métricas

A configuração `metrics_exporter` (padrão `none`) exporta métricas do serviço. As configurações relacionadas estão na [referência de configuração](../reference/configuration.md).

| `metrics_exporter` | Caminho | Indicado para |
| --- | --- | --- |
| `none` | Nada é exportado. A instrumentação custa quase nada | Cloud Run (use as métricas da plataforma e as métricas baseadas em logs) |
| `prometheus` | `GET /metrics` em `metrics_listen` (padrão `127.0.0.1:9464`). É um listener separado do `listen` público, sem autenticação, e roda para todo `role` | Auto-hospedagem (coletado por um servidor compatível com Prometheus ou pelo KEDA) |
| `otlp` | Enviado a um OpenTelemetry Collector por OTLP/HTTP. O destino vem das variáveis de ambiente padrão do OTel, como `OTEL_EXPORTER_OTLP_ENDPOINT` | Configurações em que você pode executar um collector |
| `emf` | Linhas JSON do CloudWatch Embedded Metric Format. As métricas de requisição e de job vão nas linhas de encerramento acima; as demais são escritas na saída padrão, uma linha por conjunto de rótulos, quando uma requisição ou job termina | AWS ECS |

As métricas estão listadas abaixo. No Prometheus, `.` em um nome vira `_` e um sufixo de unidade (`_seconds`, e `_total` para um contador) é adicionado.

| Métrica | Tipo | Rótulos |
| --- | --- | --- |
| `http.server.request.duration` | Histograma, segundos | `http.request.method`, `http.route` (o caminho do padrão da rota; `other` quando nenhum corresponde), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Contador | `activity.type` (um tipo de Activity conhecido, ou `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (somente com `rejected`: `signature` para uma assinatura ou prova de integridade ausente, inválida ou que não verifica, ou cuja chave não pôde ser obtida; `suspended` para um domínio suspenso; `federation_off`; `rate_limited`; `invalid` para uma activity sem `id`, `type`, `actor` ou `object` válido) |
| `my.relay.activities` | Contador | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving`: se uma activity trazida por um relay foi ingerida e por que foi descartada; veja [Relays de federação](relays.md)) |
| `my.move.activities` | Contador | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`: se um `Move` recebido foi registrado e por que foi descartado) |
| `my.delivery.attempts` | Contador | `my.outcome` (`success` / `retry` / `gone` / `failed`; `gone` cobre um 410 e as entregas ignoradas porque o servidor está indisponível ou suspenso) |
| `my.delivery.duration` | Histograma, segundos | `my.outcome` |
| `my.plugin.failures` | Contador | `my.plugin.kind` (um tipo de [plugin](plugins.md)), `my.plugin.code` (o código Connect da falha) |
| `my.plugin.slot_wait` | Histograma, segundos | `my.plugin.kind` (quanto tempo uma chamada esperou a sua vez entre as chamadas desse tipo executadas ao mesmo tempo) |
| `my.hook.decisions` | Contador | `hook`, `mode`, `outcome` (as decisões que um [gancho de decisão](decision-hooks.md) registrou) |
| `my.hook.failures` | Contador | `hook`, `outcome` (`timeout`, `error`, `invalid` ou `breaker_open`) |
| `my.hook.duration` | Histograma | `hook` (segundos por chamada do plugin) |
| `my.job.duration` | Histograma, segundos | `my.job.type` (um tipo de job registrado, ou `other`), `my.outcome` (`success` / `retry` / `dead` / `postponed`: `postponed` é uma execução que não fez trabalho e não usa nenhuma das tentativas do job, como a busca de um cartão de link que espera pelo seu host) |
| `my.job.wait` | Histograma, segundos | `my.job.type` |
| `my.link_card.host_pauses` | Contador | `http.response.status_code` (`429` / `503`). Uma por pausa das buscas de cartões de link em um host; veja [Prévias de links](link-previews.md#limits-per-host) |
| `my.translation.requests` | Contador | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`). Uma por pedido de tradução de uma publicação que quem vê pode ver, ou da publicação ou do comentário de uma denúncia que um moderador pede: respondido com as próprias palavras do autor, com uma tradução guardada ou por um provedor, ou recusado porque nenhum provedor pode receber a publicação, um limite mensal foi atingido, nenhum provedor traduz o idioma, ou nenhum provedor devolveu uma tradução utilizável. |
| `my.translation.characters` | Contador | `my.provider` (o ID de um provedor de tradução). Os caracteres de publicações e de comentários de denúncias enviados ao provedor, sem marcadores, incluindo chamadas que falham. É o que os [limites mensais](instance-settings.md#admin-screens-and-the-client-api) contam, e pode diferir do que o provedor cobra |
| `my.timeline.delivery.lag` | Histograma, segundos | Nenhum (da criação de uma publicação local até a sua escrita nas linhas do tempo Início e a notificação aos streams) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `timeline` para os streams das linhas do tempo local e federada, `voice` para os streams das salas de chat de voz) |
| `my.accounts.registered` | Contador | Nenhum |
| `my.statuses.created` | Contador | `my.visibility` |
| `my.billing.imports` | Contador | `my.billing.source` (`aws` / `gcp` / `plugin`), `my.outcome` (`success` / `failed`), `error.type` (somente com `failed`: o tipo de falha, como em [Importando cobranças de uso da nuvem](billing-import.md)). Toda tentativa conta, incluindo as novas tentativas e as falhas que esperam a próxima importação diária |
| `my.voice.turn.credentials` | Contador | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). Uma por credencial TURN que um participante do chat de voz pediu ao entrar em uma sala ou renovar a conexão: `reused` significa que credenciais da Cloudflare emitidas antes para a mesma conexão foram devolvidas de novo, e `failed` significa que a Cloudflare não pôde ser alcançada ou recusou, de modo que o participante recebeu um erro e não conseguiu se conectar |
| `my.errors.exported` | Contador | `my.outcome` (`success` / `dropped` / `failed`: uma por relatório de erro para o serviço de `errors_exporter`. `dropped` significa que o relatório não foi enviado porque já havia 1.000 relatórios esperando ou o serviço pediu ao servidor que desacelerasse, e o servidor espera o tempo que o serviço pedir antes de enviar de novo; `failed` significa que o serviço não pôde ser alcançado ou recusou o relatório. Um relatório é enviado uma vez e nunca repetido, e os relatórios que ainda esperam 2 segundos após o início de um desligamento são perdidos) |
| `my.queue.ready` | Gauge | `my.job.type` (o número de jobs executáveis na fila do banco de dados) |
| `my.queue.oldest_ready_age` | Gauge, segundos | `my.job.type` (há quanto tempo o job executável mais antigo de cada tipo na fila do banco de dados espera; um tipo sem job executável não tem valor) |
| `my.queue.dead` | Gauge | Nenhum |
| `my.instance.users`, `my.instance.statuses` | Gauge | Nenhum (as contagens de usuários locais e de publicações, como no NodeInfo) |
| `my.media.stored_bytes` | Gauge, bytes | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Gauge, bytes | Nenhum (o tamanho do banco de dados: `pg_database_size` no PostgreSQL; o arquivo do banco de dados sem o write-ahead log no SQLite) |
| `my.billing.import.failing` | Gauge | `my.billing.source` (1 quando a importação mais recente da fonte falhou, 0 caso contrário) |
| `my.federation.instances` | Gauge | `my.federation.instance.state` (`available`: servidores para os quais as entregas ainda vão, mas que falharam em pelo menos um dia desde a última vez que responderam; `unavailable`: servidores cujas entregas estão pausadas) |
| `my.metrics.snapshot.age` | Gauge, segundos | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Contador | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). Uma por [relatório de erro](#error-reports) |

As verificações de saúde (`/healthz`, `/readyz`) não são medidas. Nenhum rótulo contém um domínio remoto, conta, URL ou IP de cliente.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` e `my.billing.import.failing` são valores da instância inteira: todo processo reporta o mesmo valor a partir de um único snapshot guardado no armazenamento chave-valor.

<!-- source: system-metrics 8b5d40a4eb98 -->
### Métricas do sistema

As métricas sobre o processo, o runtime do Go e as conexões com recursos são exportadas pelo mesmo `metrics_exporter`.
CPU, memória, número de instâncias e requisições de entrada do contêiner ficam por conta das métricas da plataforma; estas cobrem a saturação dentro do processo, que a plataforma não enxerga.
Não as use para disparar o escalonamento; use-as para alertas e para estimar o limite de instâncias.

| Métrica | Tipo | Rótulos |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Gauge, bytes | Somente `go.memory.used` tem `go.memory.type` (`stack` / `other`). `go.memory.limit` aparece apenas quando `GOMEMLIMIT` está definido |
| `go.memory.allocated`, `go.memory.allocations` | Contador | Nenhum |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Gauge | Nenhum |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histograma, segundos | Nenhum (a espera entre ficar executável e executar, e as pausas do GC) |
| `go.memory.gc.cycles` | Contador | Nenhum |
| `process.cpu.time` | Observable Counter, segundos (total desde o início) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | Gauge, bytes | Nenhum (memória residente, RSS) |
| `process.unix.file_descriptor.count`, `my.process.file_descriptor.limit` | Gauge | Nenhum (descritores de arquivo abertos e o limite) |
| `process.uptime` | Gauge, segundos | Nenhum |
| `db.client.connection.count` | Gauge | `db.client.connection.pool.name` (`postgres` / `sqlite`), `db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | Gauge | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`, `my.db.client.connection.waits`, `db.client.connection.timeouts` | Observable Counter (total desde o início; segundos ou contagem) | `db.client.connection.pool.name`. `timeouts` existe apenas no PostgreSQL |
| `my.adapter.operation.duration` | Histograma, segundos | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`), `my.operation`, `my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (conexões `valkey` e assinantes de `stream.<stream>`) |
| `my.adapter.reconnects` | Contador | `my.adapter` |
| `http.client.request.duration` | Histograma, segundos | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots`), `http.request.method`, `my.http.status_class` (`2xx` e assim por diante), `error.type` (`forbidden_destination` é uma recusa da proteção SSRF) |
| `http.client.open_connections` | UpDownCounter | Nenhum |
| `my.readiness.failures` | Contador | Nenhum |
| `my.http.origin_refused` | Contador | Nenhum. Uma por requisição recusada por não ter passado pela CDN ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)). Sondagens do endereço da origem a aumentam devagar; um salto enquanto o tráfego para significa que a CDN deixou de enviar o cabeçalho. O modo de manutenção não mede nada, então só o seu log de acesso as mostra |

- As métricas de runtime do Go (`go.*`) são exportadas apenas por `prometheus` e `otlp`. Com `emf`, um job periódico de snapshot escreve os gauges e os observable counters do processo e do pool de conexões.
- As esperas do pool e o tempo de CPU são Observable Counters: o pool e o kernel guardam totais desde o início, e eles são lidos no momento da coleta. No Prometheus são contadores `_total` (como `process_cpu_time_seconds_total`), no OTLP são somas monotônicas, e você vê o crescimento deles com `rate()` do PromQL.
  O `emf` não tem contadores de total, então escreve o aumento desde a escrita anterior (some-os com a estatística Sum do CloudWatch).
- O `claim` da fila é medido apenas quando pega um job ou falha. O polling de um worker em espera não registra nada.
- `db.client.connection.pending_requests` não é exportada: as estatísticas do pgx e do `database/sql` não informam quantas estão esperando; por isso, o número e a duração das esperas a substituem.
- Crie alertas para a razão entre a soma de `go.memory.used` e `go.memory.limit`, e para o crescimento de `db.client.connection.wait_time` (o limite do pool está sendo atingido).

<!-- source: memory-limit-gomemlimit 8ea44722eefd -->
### Limite de memória (`GOMEMLIMIT`)

`GOMEMLIMIT` é uma variável de ambiente do runtime do Go que o operador define; a aplicação não a escolhe.
Sem ela, o Go roda com o padrão (sem limite), e o heap não é coletado com antecedência ao se aproximar do limite de memória do contêiner.
Em configurações com limite de memória do contêiner (Cloud Run, ECS, Kubernetes), defina-a para cerca de 90% desse limite (por exemplo `GOMEMLIMIT=460MiB` para um contêiner de 512 MiB).
Não defina `GOMAXPROCS`: o padrão do Go acompanha o limite de CPU do contêiner.

<!-- source: traces e274793f5885 -->
## Traces

A configuração `traces_exporter` (padrão `none`) exporta traces distribuídos. As configurações relacionadas estão na [referência de configuração](../reference/configuration.md).

| Span | Nome | Atributos |
| --- | --- | --- |
| Requisição HTTP (servidor) | `<method> <http.route>` (`<method>` para uma requisição que não chegou a uma rota) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. Um 5xx é uma falha |
| Execução de job (consumidor) | O tipo do job (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Cada nova tentativa é um span separado |
| Transação | `my.db.transaction` | `db.system.name`. Cada nova tentativa é um span separado |
| Query (cliente) | O nome da query do sqlc (como `InsertJob`; caso contrário, a primeira palavra do SQL, como `BEGIN`) | `db.system.name`, `db.operation.name` |
| Entrega (`my.delivery`) | `my.delivery` | `activity.type`, `my.outcome` |
| HTTP de saída (cliente) | `<method>` | `http.request.method`, `server.address` (somente o nome do host de destino), `http.response.status_code`. Um 4xx ou 5xx é uma falha |

Um span com falha tem `error.type` (um nome finito como `timeout`, ou um código de status). Os atributos nunca contêm IDs de conta, publicação ou job, URLs, caminhos ou queries, IPs de cliente, valores de User-Agent, texto SQL ou strings de erro.
`server.address` deixa os nomes de host dos servidores remotos com o receptor, de modo que você decide por quanto tempo o receptor os guarda e quem pode vê-los. As verificações de saúde e as requisições recusadas por `origin_secret` não criam span.

- Uma execução de job não é filha do span que a enfileirou (em geral uma requisição HTTP). Ela é a **raiz de um novo trace** com um link para o span que a enfileirou. Em um visualizador, siga o link a partir do trace da requisição.
- Um `traceparent` recebido só se torna o pai quando o par conectado está em `trusted_proxies` e, com [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), a requisição traz o segredo. Caso contrário, é descartado e um novo trace começa. O `tracestate` é sempre descartado. O HTTP de saída (federação, WebFinger, mídia, Web Push e assim por diante) não carrega `traceparent`.
- Amostragem: uma requisição é registrada se o pai dela, vindo de um proxy confiável, foi registrado, ou se o alvo do link de um job foi registrado; caso contrário, é registrada na proporção `traces_sample_ratio` (padrão 0,01). Reduzir a proporção nunca deixa sem trace os jobs que continuam uma requisição registrada.

| Destino | Configurações | Caminho |
| --- | --- | --- |
| Padrão, auto-hospedagem pequena | `traces_exporter = none` | Nenhum span é exportado. Siga `request_id` e `job_id` nos logs. Se um proxy confiável fornecer um `traceparent`, o `trace_id` e o `span_id` dele são adicionados aos logs |
| Auto-hospedagem (Jaeger, Tempo, um OpenTelemetry Collector e assim por diante) | `traces_exporter = otlp`, `traces_flush = batch` | Enviado por OTLP/HTTP ao receptor em `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (ou `OTEL_EXPORTER_OTLP_ENDPOINT`, padrão `http://localhost:4318`). Os cabeçalhos de autenticação vão em `OTEL_EXPORTER_OTLP_HEADERS`. Para manter com segurança os erros e os traces lentos, use a amostragem tail do Collector |
| Google Cloud (Cloud Run) | `none`, ou `otlp` com `unit` para ativar os traces | Veja abaixo |
| AWS (ECS) | `none`, ou `otlp` com `batch` para ativar os traces | Veja abaixo |

<!-- source: cloud-run 7f1a21bbfbef -->
### Cloud Run

Mesmo com `none`, o trace de requisição que o Cloud Run cria automaticamente e o `trace_id` nos logs compartilham o mesmo ID (quando você confia, com `trusted_proxies`, na conexão que vem da frente do Cloud Run).
Ainda não foi verificado se o Cloud Run sempre sobrescreve o `traceparent` das requisições públicas.
Os logs não têm o formato do campo de trace que o Cloud Logging reconhece (`logging.googleapis.com/trace`).

Para enviar spans da aplicação ao Cloud Trace, execute um **OpenTelemetry Collector como sidecar** no mesmo serviço. O servidor não guarda credenciais do Google; o sidecar envia ao endpoint OTLP do Cloud Trace com as Application Default Credentials.

- Servidor (o contêiner de entrada): `MY_TRACES_EXPORTER=otlp`, `MY_TRACES_FLUSH=unit`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
- Sidecar: um OpenTelemetry Collector que recebe com um receptor `otlp` por HTTP em `localhost:4318` e envia com o exporter `googlecloud`, ou o exporter `otlphttp` (`https://telemetry.googleapis.com`, com a extensão `googleclientauth`). Conceda à conta de serviço permissão para escrever no Cloud Trace.
- Ao fim de cada requisição ou job registrado, `unit` descarrega os spans até ali para o sidecar antes de retornar. Para que o sidecar possa enviá-los enquanto nenhuma CPU está alocada, use cobrança baseada em instância em uma configuração com traces ativados.
- O custo são os recursos do sidecar e o volume de spans (preços do Cloud Trace). Escolha a proporção com `traces_sample_ratio`.

<!-- source: aws-ecs 52fa09412f20 -->
### AWS ECS

Para enviar ao X-Ray, execute o **AWS Distro for OpenTelemetry (ADOT) Collector** como um contêiner sidecar na task. O servidor não faz assinatura SigV4; o Collector assina e envia ao endpoint OTLP do X-Ray (`https://xray.<region>.amazonaws.com/v1/traces`).

- Servidor: `MY_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`. Na configuração do Collector, receba com um receptor `otlp` em `localhost:4318` e envie ao X-Ray com um exporter `otlphttp` que usa a extensão `sigv4auth`.
- Conceda à função da task permissão para escrever no X-Ray.
- O custo é a parte do sidecar nos recursos da task e o número de traces que o X-Ray registra (os primeiros 100.000 por mês são gratuitos).

<!-- source: stop-the-server 37d4c0cc3d97 -->
## Parar o servidor

Ao receber SIGTERM ou SIGINT, o servidor desliga de forma ordenada.

1. Ele para de aceitar novas conexões e de pegar jobs.
2. Espera até 25 segundos para as requisições em andamento e os jobs em execução terminarem.
3. Após 25 segundos, cancela o contexto das requisições e jobs restantes e fecha as conexões.
   Em seguida, dá a um job cancelado até mais 3 segundos para registrar o resultado e termina com código 1 (no máximo 28 segundos no total, dentro do período de tolerância padrão de 30 segundos do Kubernetes e do ECS).
   Um job cancelado é repetido. Um job que não para nesses 3 segundos, ou que é encerrado no meio, é abandonado e retomado por um worker quando o lease dele expira; portanto, pode executar duas vezes.

Um pânico em uma das tarefas em segundo plano do servidor inicia o mesmo desligamento, para que o servidor nunca continue rodando em um estado quebrado.
A linha de log `serve failed` nomeia a tarefa, e o processo termina com código 1 para que a plataforma ou o gerenciador de processos o reinicie.

Para trabalhos no banco de dados, no armazenamento chave-valor ou na fila, reinicie em modo de manutenção (`MY_MAINTENANCE=true`) em vez de parar. Veja [Manutenção](maintenance.md).

<!-- source: the-job-queue-and-failed-dead-jobs 55023b12a1aa -->
## A fila de jobs e os jobs com falha (dead)

[Jobs](../reference/jobs.md) lista todos os tipos de job: o que ele faz, o que o inicia, como tenta de novo e o que acontece ao executá-lo outra vez.

A página de jobs da administração (`/admin/jobs`, permissão `manage_jobs`) tem três páginas:

- **Estado da fila** (`/admin/jobs`): a contagem por estado e por tipo e estado, por quanto tempo cada tipo mantém um job esperando e há quanto tempo o job em espera mais antigo aguarda.
- **Lista de jobs** (`/admin/jobs/list`): uma tabela dos jobs em um estado (pendente, em espera, em execução, em nova tentativa, com falha), uma página por vez, com o tipo, o estado, o horário em que está agendado ou falhou e o número de tentativas. Os jobs com falha têm **Executar de novo**.
- **Job** (`/admin/jobs/<id>`, a partir do tipo na tabela): todos os fatos do job, o último erro, as chaves e o payload, o histórico de tentativas malsucedidas (resultado, erro, worker e horário de início e fim de cada tentativa) e **Executar de novo** para um job com falha.

A API de administração é `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}`, `GET /api/v1/admin/jobs/{id}/attempts` e `POST /api/v1/admin/jobs/{id}/retry`.
Um job concluído não está mais na fila, então a página dele não é encontrada.

- Os jobs em espera, em execução e aguardando nova tentativa só podem ser listados para as filas cujos jobs o servidor guarda: `queue = db`, `queue = cloudtasks` (os jobs também estão no banco de dados) e `queue = inline`. Os jobs com falha e o histórico de tentativas estão no banco de dados da aplicação para qualquer fila, então sempre podem ser listados.
- O histórico de tentativas guarda apenas as tentativas malsucedidas (as novas tentativas e a tentativa que por fim falhou). Acompanhe uma execução bem-sucedida pela linha de log estruturada `job` (que traz `job_id`).
  O histórico é removido pela limpeza horária depois que `job_attempt_retention` (padrão 30 dias; veja a [configuração](../reference/configuration.md#job_attempt_retention)) passou desde o fim da tentativa.
  O histórico não guarda payload nem log de handler, e uma mensagem de erro é cortada nos primeiros 2.000 bytes. Os handlers de jobs mantêm endereços de e-mail, endereços IP, tokens e corpos de publicações fora das mensagens de erro.
  Para uma visão limitada aos tipos de job de e-mail, veja [Envio de e-mail](email.md).

Um job cujo handler decidiu que falhou de vez, e um job que esgotou as tentativas, vira um job dead na fila do banco de dados (`queue = db` e `queue = cloudtasks`).
`my job` os lista, repete e exclui. O estado “Com falha” da página de jobs e a API de administração também podem listá-los e repeti-los.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- Se um job com a mesma chave de idempotência já está esperando, esse job faz as vezes do que foi repetido. Os handlers de jobs são idempotentes, então recolocar o mesmo job na fila quantas vezes for preciso é seguro.
- Um ID desconhecido é exibido e o comando termina com código 1; os outros IDs ainda são processados. O código de saída 2 indica erro de uso.
- Um job dead que ninguém repete é removido pela limpeza horária depois que `dead_job_retention` (padrão 30 dias; veja a [configuração](../reference/configuration.md#dead_job_retention)) passou desde a falha.
- Os jobs dead de `queue = inline` ficam na memória do processo do servidor, então `my job` não consegue tratá-los (código de saída 1).

<!-- source: splitting-workers-by-job-type 89ced36a3c80 -->
### Dividindo workers por tipo de job

Por padrão, todo worker executa todo tipo de job, na ordem em que os jobs venceram, um job por vez em cada processo (veja [Executando vários jobs ao mesmo tempo](#running-several-jobs-at-once)). Uma rajada de entregas então atrasa o e-mail e as notificações, e qualquer worker que você adicionar divide o tempo dele entre todos eles. Com `queue = db`, você pode dar a alguns tipos os próprios workers:

- [`worker_job_types`](../reference/configuration.md#worker_job_types): este processo executa apenas estes tipos.
- [`worker_job_types_except`](../reference/configuration.md#worker_job_types_except): este processo executa todos os tipos, menos estes.

Cada uma é uma lista separada por vírgulas de [tipos de job](../reference/jobs.md) (`my job types` os exibe; `my config check` recusa um nome que não corresponde a nenhum), escritos como um nome (`federation.deliver`) ou um prefixo (`federation.*`). Defina-as por processo, por exemplo com `MY_WORKER_JOB_TYPES`. Não é possível definir as duas em um mesmo processo. Um nome ou prefixo que não corresponde a nenhum tipo de job impede o início, o que também detecta um tipo que uma versão renomeou, ou um tipo que só existe com uma configuração que você não definiu. Elas não funcionam com `role = web`, nem com `queue = cloudtasks` ou `inline`, em que o início também falha: ali um sinal nomeia o job a executar, então um processo não pode escolher os seus jobs.

No início, um processo com uma das configurações registra no log os tipos que ignora. Ele não enxerga as configurações dos outros processos, então **garanta que algum processo execute todos os tipos**, ou esses jobs esperarão para sempre. A limpeza periódica e o e-mail também são tipos de job. Todo processo ainda inicia os jobs periódicos; qual processo os executa depende dessas configurações.

Decida o que dividir a partir das métricas, não dos nomes dos tipos. `my.job.wait` (Prometheus: `my_job_wait_seconds`) mostra quanto tempo os jobs esperaram por tipo antes de começar:

```promql
histogram_quantile(0.95, sum by (my_job_type, le) (rate(my_job_wait_seconds_bucket[15m])))
```

`sum by (my_job_type) (rate(my_job_duration_seconds_sum[1h]))` mostra o tempo de worker que cada tipo usa, que também é aproximadamente quantos processos ele precisa. Um tipo que usa a maior parte do tempo e faz os outros esperarem é o que deve ser movido.

Três layouts comuns, com o número de processos à sua escolha:

| Layout | Processos |
| --- | --- |
| Evitar que as entregas atrasem o resto | `worker_job_types = federation.fanout,federation.deliver` para as entregas, e `worker_job_types_except = federation.fanout,federation.deliver` para todo o resto. Adicione processos ao primeiro grupo durante uma rajada. |
| Evitar que um backend de busca atrase o resto | `worker_job_types = search.*` para os jobs de busca, e `worker_job_types_except = search.*` para todo o resto ([Evitando que a busca atrase outros jobs](search.md#keeping-search-from-delaying-other-jobs)). |
| Manter o e-mail e as notificações rápidos | `worker_job_types = identity.verification_email,identity.password_reset_email,identity.password_notice_email,identity.closure_email,identity.deactivation_email,notification.email,notification.moderator_email,notification.supporter_email,notification.push,notification.post_subscribers,instance.notify_announcement,voice.notify_started` para esses, e um worker sem configuração para todos, o que também ajuda com esses tipos quando está livre. Deixe de fora os tipos de configurações que você não definiu, ou o início falha: `notification.push` precisa de `vapid_private_key`, e `notification.email`, `notification.moderator_email` e `notification.supporter_email` precisam de `mail`. Veja [Jobs que dependem de configurações](../reference/jobs.md#jobs-that-depend-on-settings). |

Um prefixo como `identity.*` é o módulo de código que possui os tipos, não um tipo de trabalho. `identity.*` também pega exportações de dados, importações e jobs de expiração, e não pega `instance.notify_announcement` nem `voice.notify_started`; portanto, um layout para um tipo de trabalho nomeia cada tipo.

Mantenha estes juntos, em workers com a mesma configuração. Os jobs de uma mesma chave de ordenação executam em ordem, qualquer que seja o tipo; assim, se nada executa o job anterior, o posterior também espera, e se o grupo do job anterior está congestionado, o job posterior espera por ele:

| Tipos | O que mantêm em ordem |
| --- | --- |
| `federation.fanout`, `federation.deliver` | As entregas de um like e do desfazer dele |
| `voice.apply_media_event`, `voice.apply` | Os eventos de uma sala de voz |
| `supporters.sync`, `supporters.command` | As mudanças de uma assinatura |

Ao atualizar:

- Um processo com `worker_job_types` não executa os tipos que uma nova versão adiciona. Confira as notas da versão e os tipos ignorados no log. Um processo com `worker_job_types_except` os executa, o que o torna o lugar mais seguro para todo o resto.
- Um tipo renomeado impede o início, então leia primeiro as notas da versão.

Para implantar uma divisão, inicie primeiro os novos workers e depois adicione a configuração aos antigos. Para desfazê-la, remova a configuração de um grupo antes de parar o outro. Os jobs permanecem na fila, então nada se perde, e um tipo que ninguém executa por um tempo apenas espera.

**Detectando um tipo que nada executa.** Os jobs desse tipo esperam, então `my.queue.oldest_ready_age` cresce para ele, e os alertas `MyQueueBacklog` e `MyQueueStalled` disparam para o tipo que espera há mais tempo. Para ver qual é, ou para criar alertas por tipo:

```promql
max by (my_job_type) (my_queue_oldest_ready_age_seconds) > 1800
```

A página de jobs da administração mostra a mesma espera por tipo. Um worker com uma das configurações também deixa de lado um job cujo tipo não tem nenhum handler (ele conta como `other` na métrica); assim, sem nenhum worker sem configuração, esse job espera em vez de virar um job dead.

<!-- source: running-several-jobs-at-once 91e5181261b2 -->
### Executando vários jobs ao mesmo tempo

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) define quantos jobs um processo executa ao mesmo tempo, de 1 a 128. Funciona com `queue = db`; com o Cloud Tasks, a plataforma decide. O padrão é um job por vez, e o valor 1 se comporta da mesma forma.

Aumente-o para jobs que passam o tempo esperando outros servidores, como `federation.deliver`. Eles usam pouca memória e poucas conexões de banco de dados por job, então um processo faz o trabalho de muitos: em um teste local em que um job apenas esperava 200 ms por uma resposta, um processo com 32 slots igualou 32 processos com um slot cada, usando uma pequena parte da memória. Esses jobs não faziam mais nada, então meça a sua própria instância. Combine-o com `worker_job_types` e dê a cada tipo de job um processo com o próprio número.

Tenha estes limites em mente:

- **Conexões de banco de dados**: um job precisa de uma conexão para ser reivindicado, para o próprio trabalho e para registrar o resultado. Com `database = postgres`, a configuração deve ficar abaixo do máximo do pool, ou o servidor não inicia. O máximo é `pool_max_conns` em `database_url`, por padrão o maior entre 4 e o número de CPUs, então uma vCPU permite 3 até você aumentá-lo. Confira se o seu banco de dados ou pooler de conexões aceita as conexões de todos os seus processos.
- **SQLite**: ele escreve uma transação por vez. Acima de 32, o servidor registra um aviso e, em um teste local, o SQLite ficou mais lento com 128 do que com 32.
- **Jobs que exigem muita CPU**: os slots dividem a CPU e a memória de um processo. `media.process` espera a sua vez em um limite de metade das CPUs por padrão, e a espera conta contra o lease de 5 minutos do job, então um processo cheio de `media.process` pode passar do lease e recomeçar. Dê a esses jobs um processo com poucos slots.
- **Um servidor por vez**: um processo faz no máximo 8 conexões com qualquer host, então um processo entrega a um único servidor não mais rápido do que 8 conexões permitem, não importa quantos slots tenha. Onde a maior parte das entregas vai para poucos servidores grandes, mais processos ajudam onde mais slots não ajudam.
- **Um servidor travado prende slots**: os slots que esperam um servidor lento não ficam livres para outros jobs, então mantenha as entregas separadas do e-mail e das notificações.
- **Implantações consomem tentativas**: quando o servidor para, os jobs ainda em execução depois de `ShutdownTimeout` (25 segundos por padrão) são cancelados, e cada job cancelado consome uma tentativa. Quanto mais slots executam jobs lentos, mais tentativas cada implantação consome. Um job cancelado cujo resultado não é registrado a tempo espera o lease expirar antes de executar de novo.
- **Uma falha derruba todos os slots**: se o processo fica sem memória, todos os jobs dele esperam os leases expirarem.

Para decidir se deve aumentá-lo, leia `my.job.wait` por `my.job.type`: se os jobs de um tipo esperam muito enquanto `my.job.duration` é em grande parte tempo gasto esperando outros servidores e a CPU e a memória ficam baixas, mais slots devem ajudar. Se as esperas continuam altas depois de aumentá-lo, olhe as conexões de banco de dados e o limite por host acima antes de aumentá-lo de novo.

Enquanto nenhum job está vencido, um slot faz o polling e os outros esperam, de modo que um processo ocioso custa ao banco de dados o mesmo que um processo de um job só.

<!-- source: waiting-for-a-database-connection f481d577ea58 -->
### Esperando uma conexão de banco de dados

Com `database = postgres`, cada instrução e transação primeiro espera uma conexão livre do pool do processo.
[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (padrão `60s`) limita essa espera, e nada além dela: uma consulta ou
transação longa que já tem a sua conexão continua, por mais que demore. `0` espera sem limite. O SQLite não tem limite de conexões a esperar, então defini-la ali falha na inicialização.

Quando a espera acaba:

- Uma requisição é respondida com `503 Service Unavailable` e `Retry-After: 5`.
- Um job executa de novo 30 segundos depois e não consome nenhuma tentativa, de modo que um banco de dados ocupado nunca transforma um job em um job dead.
- A métrica `db.client.connection.timeouts` a conta, e o alerta [`MyDBPoolWaiting`](alerts.md#mydbpoolwaiting) dispara.

O padrão é o timeout de escrita de 60 segundos do servidor: uma requisição que esperou mais não poderia enviar a resposta de qualquer forma, então o padrão não faz falhar nenhuma requisição que ainda poderia ter dado certo.
Escolha um valor conforme o funcionamento da sua instância:

- **Reduza-o** para descartar carga rapidamente: os clientes tentam de novo após `Retry-After` em vez de manter as requisições abertas. Mantenha-o maior que as suas transações normais mais lentas, ou uma rajada curta passa a responder `503`.
- **Dimensione o pool primeiro.** Um processo precisa de aproximadamente tantas conexões quanto o trabalho que executa ao mesmo tempo. `pool_max_conns` em `database_url` tem como padrão o maior entre 4 e o número de CPUs, e todos os processos juntos devem ficar abaixo do limite de conexões do banco de dados.
  - Com [`worker_job_concurrency`](#running-several-jobs-at-once), os slots também esperam conexões, e a configuração deve ficar abaixo do máximo do pool.
  - No Cloud Run, um serviço aceita 80 requisições simultâneas por instância por padrão. Com um pool de 4, uma rajada de requisições lentas forma fila aqui; portanto, aumente `pool_max_conns`, reduza a concorrência do serviço, ou ambos.
- **`503` frequentes ou `db.client.connection.timeouts`** significam que o pool é pequeno demais ou que as consultas ficaram mais lentas; um timeout maior apenas esconde isso. Siga [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated).

<!-- source: cloud-tasks ef3f7d7cc896 -->
### Cloud Tasks

[Implantando no Cloud Run](deploy-cloud-run.md) configura tudo o que está abaixo com módulos do OpenTofu.

Com `queue = cloudtasks`, os jobs permanecem no banco de dados (`database = postgres`), e o Google Cloud Tasks apenas acorda o servidor para cada um deles:

- Para cada job que guarda, repete ou recoloca na fila, o servidor cria uma task em `queue_cloudtasks_queue` depois que a transação é confirmada. A task chama
  `POST {queue_cloudtasks_url}/_internal/jobs` no horário em que o job vence, com o ID do job como corpo e um token de ID OIDC de
  `queue_cloudtasks_service_account`. Um job que vence com mais de 30 dias de antecedência é sinalizado aos 30 dias, e de novo quando essa task chega.
- O processo que recebe a task executa o job e responde `204` assim que o resultado é registrado, também quando o job já executou ou não está vencido.
  Um job que falha é repetido pelo servidor no próximo horário de tentativa dele, não pelo Cloud Tasks. O Cloud Tasks repete uma task apenas quando a
  resposta é `5xx` (o banco de dados não pôde ser alcançado) ou `401` (um token que o servidor recusou), então o `retryConfig` dele não precisa de ajuste.
- Nenhum processo faz polling da tabela e nenhum processo executa os jobs periódicos por conta própria. Crie um job do Cloud Scheduler que chame
  `POST {queue_cloudtasks_url}/_internal/tick` a cada minuto (`* * * * *`) com um token OIDC da mesma conta de serviço (e a audiência de
  `queue_cloudtasks_audience`, quando definida). Cada tick enfileira os jobs periódicos cujo período chegou, uma vez para a instância inteira (os períodos são reivindicados no KV,
  então `kv` deve ser `db` ou `valkey`), e sinaliza de novo os jobs que estão vencidos há mais de 2 minutos sem executar, o que recupera um sinal perdido.
- `/_internal/jobs` e `/_internal/tick` são servidos pelos processos com `role = all` ou `worker`, e aceitam apenas um token assinado pelo Google para
  `queue_cloudtasks_service_account` com um e-mail verificado, cuja audiência é `queue_cloudtasks_audience` ou, por padrão, a URL chamada. Qualquer outra requisição recebe `401`.
- Uma entrega executa um job, por no máximo o lease de 5 minutos. Mantenha `queue_cloudtasks_dispatch_deadline` (padrão 10 minutos) e o timeout de requisição do
  serviço do Cloud Run maiores que isso.
- A própria conta de serviço do servidor precisa de `roles/cloudtasks.enqueuer` na fila e de `roles/iam.serviceAccountUser` em `queue_cloudtasks_service_account`.
  Ela se autentica com as Application Default Credentials.

<!-- source: routing-job-types-to-their-own-queues 292b3f6071a8 -->
### Direcionando tipos de job para filas próprias

Com `queue = cloudtasks`, um worker não pode escolher os jobs que executa: `worker_job_types` funciona apenas com `queue = db`, porque a plataforma decide quem recebe um sinal.
Para dar a alguns tipos de job capacidade própria, envie os sinais deles para uma fila própria e defina a capacidade dessa fila na plataforma.
Nada muda a menos que você defina uma rota.

[`queue_cloudtasks_routes`](../reference/configuration.md#queue_cloudtasks_routes): `federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com,notification.email=projects/p/locations/l/queues/my-mail`.
Uma rota sem `@` e uma URL chama `queue_cloudtasks_url`.

O lado esquerdo é um tipo de job da [referência de jobs](../reference/jobs.md) ou um prefixo. Um nome vence um prefixo, e um prefixo mais longo vence um mais curto.
Os tipos que não correspondem a nenhuma rota, incluindo os que uma versão futura adicionar, usam a fila padrão (`queue_cloudtasks_queue`), então continuam executando.
`my config check` verifica o formato das rotas; um lado esquerdo que não corresponde a nenhum tipo de job impede `my serve` de iniciar.

**Dê as mesmas rotas a todos os processos, `role = web` inclusive.** Um sinal é enviado pelo processo que guarda o job: um processo web para a maior parte do que os usuários fazem, como o
primeiro job de uma publicação, e um worker para os jobs que um job guarda, as novas tentativas, o próximo job de uma chave de ordenação e a varredura de sinais perdidos.
Onde cada função tem as próprias configurações, como os serviços do Cloud Run, passe o mesmo valor a todos:

```sh
routes='federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com'
gcloud run services update my-web --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-federation-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
```

Um processo com outras rotas não gera erro e não perde nenhum job: os sinais dele vão para outra fila, e os jobs executam lá, fora da capacidade que você definiu.
`my.queue.oldest_ready_age` não mostra isso, porque esses jobs não esperam. Para comparar os processos, cada um registra no log ao iniciar uma linha com as suas `routes` e o `routes_hash` delas:
o hash tem de ser o mesmo em todos os lugares. Para ver para onde os sinais vão, leia as métricas da própria plataforma de cada fila.

**Permissões.** As credenciais de cada processo precisam enviar para toda fila das rotas: `roles/cloudtasks.enqueuer`.
Um envio que falha é registrado no log e não é tentado em outra fila, para que um tipo que você limita não transborde para os outros. Esses jobs começam atrasados, quando a varredura envia os sinais deles de novo após 2 minutos.

**Capacidade.** Qualquer receptor executa qualquer job, então você escolhe até onde separá-los:

| Plataforma | Um receptor | Receptores separados |
| --- | --- | --- |
| Google Cloud | Um serviço. Cada fila tem os próprios `maxConcurrentDispatches` e `maxDispatchesPerSecond`; o limite de instâncias do serviço é compartilhado | Um serviço por fila, nomeado pela URL na rota, cada um com o próprio limite de instâncias e concorrência. Cada um define `queue_cloudtasks_url` com a própria URL |

Uma fila precisa da mesma configuração que a padrão: a função na fila.
Apenas a fila padrão precisa do tick.

**O que uma fila acrescenta.** Cada fila é mais uma coisa para criar, acompanhar e pagar, e o quanto isso importa depende do tamanho da sua instância.
Divida o que você viu esperando em `my.job.wait`, não tudo.

**A varredura.** A cada minuto, a varredura procura jobs vencidos há mais de 2 minutos sem executar, até 500 por rota e 500 para a fila padrão, e os sinaliza de novo.
Assim, uma fila que não dá conta recebe até 500 sinais duplicados por minuto. Eles não executam um job duas vezes, mas são mensagens que você paga, e cessam quando o acúmulo se esgota.

**As chaves de ordenação** continuam valendo: o banco de dados decide o que pode executar. Os jobs de uma chave podem estar em filas diferentes, mas uma fila parada também atrasa os jobs posteriores das suas chaves.

**Adicionando, movendo e removendo uma fila.**

1. Crie a fila e o receptor dela, e dê a todos os processos permissão para enviar a ela.
2. Altere as rotas em todos os processos. Enquanto elas diferem, os sinais vão para qualquer uma das filas e os jobs executam nos dois lugares.
3. Para remover uma fila, tire-a das rotas em todos os lugares primeiro, e espere até que esteja vazia antes de excluí-la. Um sinal em uma fila excluída se perde, mas não o job: a varredura sinaliza o job de novo quando ele estiver vencido há 2 minutos.
   Uma task do Cloud Tasks pode guardar o sinal de um job com até 30 dias de antecedência, então um job que vence depois da espera é simplesmente sinalizado pela varredura quando chega a hora dele.

<!-- source: health-checks 06a8834ade0d -->
## Verificações de saúde

| Caminho | Tipo | Quando retorna 200 |
| --- | --- | --- |
| `GET /healthz` | liveness | O processo consegue responder HTTP. Ele não olha dependências como o banco de dados (para que um banco de dados lento não reinicie o processo) |
| `GET /readyz` | readiness | O banco de dados está acessível e o schema foi aplicado |
| `GET /api/v1/instance/health` | Monitoramento de disponibilidade, páginas de status e clientes (servido pela função `web`) | O mesmo que a readiness. O resultado fica em cache no processo por 60 segundos |

- Um 200 de `/healthz` ou `/readyz` tem o corpo `{"status":"ok"}`.
- `/readyz` verifica o banco de dados a cada requisição, e as requisições simultâneas compartilham uma verificação.
- `/api/v1/instance/health` retorna 200 e `{"status":"ok","checked_at":"<RFC 3339>"}` quando saudável, e 503 e `{"status":"unavailable","checked_at":...}` em caso de falha (`Cache-Control: no-store`).
  Cada processo toca o banco de dados no máximo uma vez por minuto, não importa com que frequência seja monitorado ou requisitado, e não olha nem o token de acesso nem o limite de requisições. Aponte os monitores de disponibilidade para este endpoint, não para a probe (`/readyz`).
  Para a configuração por serviço, o intervalo de monitoramento e o custo, e como a manutenção é tratada, veja o [guia de monitoramento de disponibilidade](monitoring.md).
- Os três registram no nível Debug no log de acesso e não criam span de trace.
- Quando a readiness não é atendida, a resposta é 503 com um corpo RFC 9457 `application/problem+json`.
  O `title` dele é traduzido conforme o `Accept-Language` e não contém detalhes internos. A causa (um erro de conexão ou os nomes das tabelas ausentes) vai para o log.
- Uma verificação de readiness é interrompida após 3 segundos.
- A readiness também exige que o banco de dados contenha o `host` e o `account_domain` configurados, ou que o processo consiga registrá-los.
- **Schema aplicado** significa que toda tabela que os arquivos de schema do banco de dados (`schema/<NN>-<module>/<dialect>.sql`) declaram com `CREATE TABLE` existe
  (o SQLite é verificado por `sqlite_schema`, o PostgreSQL por `current_schema()` da conexão). `my schema apply` aplica o schema inteiro, então uma tabela ausente significa que o schema não foi aplicado ou é mais antigo que o binário.
  Diferenças de colunas e índices não são verificadas; `my schema apply -dry-run` exibe o que mudaria.

<!-- source: requests-that-bypass-the-cdn 2cca4127af07 -->
## Requisições que contornam a CDN

Quando o servidor está atrás de uma CDN e o próprio endereço dele continua público, defina `origin_secret` e `origin_secret_header` para que ele recuse o que não passou pela CDN. As verificações de saúde continuam acessíveis. Veja [Recusar requisições que não passaram pela CDN](cdn.md#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: security-headers 8c5e82848149 -->
## Cabeçalhos de segurança

Toda resposta traz `Strict-Transport-Security`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy: same-site` e `X-Frame-Options`,
e, para as respostas que não são documentos, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`.
Um handler os substitui nas respostas que precisam de outros valores, como o JSON do ActivityPub e a mídia pública.
`Permissions-Policy` nega a câmera, a geolocalização, o pagamento e o USB. Nega também o microfone, exceto enquanto o chat de voz está ativado (`voice` não é `off`):
então ela permite o microfone apenas para as páginas da própria origem do servidor (`microphone=(self)`), para que os navegadores possam falar em uma chamada.
O HTML do cliente web recebe uma CSP de documento que não usa nonce (`script-src 'self'` e assim por diante).
Ela exige Trusted Types (`require-trusted-types-for 'script'`) e limita as políticas que podem ser criadas às duas do cliente (`trusted-types my-server-sanitized-html my-service-worker`).

<!-- source: web-client f35b79dd2cc3 -->
## Cliente web

Um processo cujo `role` é `web` ou `all` serve o cliente web embutido no binário.

| Caminho | Conteúdo | `Cache-Control` |
| --- | --- | --- |
| Qualquer `GET` que nenhuma outra rota atenda (`/`, `/@alice`, `/@alice/status/{id}`, `/settings` e assim por diante) | O shell HTML do SPA. As configurações da instância são passadas em `<script type="application/json" id="config">` | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Arquivos de build com nomes com hash | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | O manifesto do app web montado a partir das configurações da instância | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` e outros arquivos diretamente sob o build | Servidos como estão | `no-cache`, ETag |
| `GET /licenses/*` | As licenças do que o build empacota (avisos de pacotes npm, e `LICENSE-Unicode.txt` e `OFL.txt` para dados empacotados que não são do npm). Servidas como `text/plain; charset=utf-8` qualquer que seja a extensão | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Montados a partir das configurações da instância (veja [Mecanismos de busca e crawlers de IA](crawlers.md)) | `public, max-age=3600`, ETag |

- Os caminhos sob `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (a folha de estilos do tema do servidor) e `/licenses`, e `/robots.txt`, nunca retornam o shell; se nenhuma outra rota atende, retornam um problema 404.
- O shell é servido depois da negociação de conteúdo do ActivityPub. Requisitar um caminho do shell com um `Accept` do ActivityPub retorna 404 (as páginas de publicação redirecionam, como descrito abaixo).
  O shell sempre traz `Vary: Accept`. Ao colocar uma CDN na frente, inclua o tipo de `Accept` (ActivityPub ou HTML) na chave de cache, ou exclua do cache o shell, `/ap/*` e `/@*`.
  Sirva da CDN apenas `/assets/*`.
- Um binário compilado sem o cliente web (um `go build` simples, sem compilar o cliente antes) registra um aviso ao iniciar e serve um shell sem scripts.

<!-- source: federation-endpoints 7aaee8c9da42 -->
## Endpoints de federação

Um processo cujo `role` é `web` ou `all` responde a estes endpoints, que outros servidores do fediverso usam para encontrar e exibir as suas contas.

| Caminho | Conteúdo |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger (RFC 7033). `resource` é `acct:alice@{account_domain}` (`acct:alice@{host}` também funciona), o `id` de um Actor ou a URL `/@alice`. `subject` é sempre o handle do lado do `account_domain` |
| `GET /.well-known/host-meta`, `/.well-known/host-meta.json` | O modelo de URI do WebFinger (XRD / JRD) |
| `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.1` | NodeInfo 2.1. `metadata.sourceCode` é a configuração `source_url`, e `software.repository` também é definido para uma URL `https:` (veja [Publicar o código-fonte](#publish-the-source-code-agpl-30)) |
| `GET /ap/actors/{uuid}` | O Actor (`publicKey` é o `#main-key` RSA). Uma requisição que prefere HTML recebe um 303 para `/@{username}` |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | Coleções |
| `POST /ap/actors/{uuid}/inbox`, `POST /ap/inbox` | A inbox de cada actor local e a inbox compartilhada. As requisições devem ter assinatura HTTP |
| `GET /@{username}` | Um 302 para o `id` do Actor com um `Accept` do ActivityPub; caso contrário, o HTML do cliente web (veja abaixo) |
| `GET /@{username}/status/{uuid}` | Com um `Accept` do ActivityPub, um 302 para o `id` (`/ap/objects/{uuid}`) da publicação se um visitante sem login pode vê-la, e 404 se não. Caso contrário, o HTML do cliente web |

- O handle do Actor da instância é `{account_domain}@{account_domain}` (a mesma forma do Mastodon).
- Uma conta que não existe ou não pode ser exibida (suspensa, desativada, excluída ou remota) sempre recebe o mesmo 404 (`Cache-Control: no-store`).
- As respostas anônimas trazem `Cache-Control: public, max-age=60`. As respostas que variam por `Accept` trazem `Vary: Accept`.
  Para uma CDN que não consegue tratar `Vary: Accept`, exclua do cache `/ap/*` e `/@*`.

<!-- source: separate-handle-and-delivery-domains b1c23d994d0e -->
### Domínios separados para handle e entrega

Quando `account_domain` difere de `host`, as requisições a `/.well-known/webfinger`, `/.well-known/host-meta` e `/.well-known/nodeinfo` do lado do `account_domain` são redirecionadas para `host` com um 301 (com `Access-Control-Allow-Origin: *`).
Se você encaminha as requisições de `account_domain` para este servidor, o próprio servidor devolve esse redirecionamento.
Se outro servidor ou proxy reverso responde no lugar, redirecione para `https://{host}` mantendo o caminho e a query string.
