<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# Visão geral de um servidor da mycroblog

A primeira página a ler antes de rodar um servidor. A mycroblog é um microblog que se federa com o restante do fediverso
por meio do ActivityPub; o `my` é o software do servidor, um único binário que reúne o servidor, o worker de jobs, o
cliente web e os comandos que cuidam deles. Esta página mostra do que um servidor é feito, o que você escolhe por
configuração, como um servidor pequeno cresce até ficar grande, e onde ler em seguida.

<!-- source: what-a-server-is-made-of 51fb6c4e4a24 -->
## Do que um servidor é feito

![A arquitetura de um servidor mycroblog: os clientes alcançam os processos my por HTTPS; os processos usam as partes escolhidas por configuração e os serviços fora do servidor que você ativa. As seções abaixo descrevem cada parte.](images/architecture.svg)

De cima para baixo no diagrama:

- **Quem se conecta.** As pessoas usam o cliente web no navegador (ele pode ser instalado como app), apps e bots usam a
  API do cliente, e outros servidores do fediverso entregam e buscam publicações por ActivityPub. Todos alcançam o servidor por
  HTTPS.
- **Na frente do servidor.** Por padrão o `my` fala HTTP simples, então um proxy reverso ou balanceador de carga termina o HTTPS na frente dele.
  Em uma única máquina, o `my` pode servir HTTPS por conta própria ([servindo HTTPS pelo servidor](running.md#serving-https-from-the-server)).
  Uma CDN na frente dos arquivos de mídia é opcional.
- **Os processos `my`.** Cada processo executa `my serve`, e o [`role`](../reference/configuration.md#role) dele diz o que ele
  executa: `web` o servidor HTTP, `worker` o worker de jobs, que entrega publicações, busca em outros servidores, envia e-mail,
  processa mídia e executa os jobs periódicos, e `all`, o padrão, os dois em um só processo.
- **As partes que você escolhe.** Cada parte no meio do diagrama tem uma configuração que escolhe o adaptador dela, listadas na
  [próxima seção](#parts-you-choose-by-configuration).
- **Serviços fora do servidor.** CAPTCHA, chat de voz, pagamentos, plugins e a importação de custos de nuvem ficam desativados
  até você configurá-los, e o servidor então chama o serviço que você indicar.

<!-- source: parts-you-choose-by-configuration 34d13fdad524 -->
## Partes que você escolhe por configuração

Cada parte é uma configuração da [referência de configuração](../reference/configuration.md), definida em um arquivo TOML ou como
variável de ambiente (`MY_` e a chave em maiúsculas, como `MY_DATABASE`). A primeira opção de cada lista é o padrão.
Onde uma parte guarda dados, escolha-a antes da primeira inicialização: os dados não se movem sozinhos quando a configuração muda.

| Parte | Configuração | Opções | Leia a seguir |
| --- | --- | --- | --- |
| Banco de dados | [`database`](../reference/configuration.md#database) | `sqlite` (um arquivo), `postgres` (PostgreSQL e serviços compatíveis) | [Banco de dados](database.md), [backup](backup.md) |
| Fila de jobs | [`queue`](../reference/configuration.md#queue) | `db` (uma tabela no banco de dados), `inline` (executa cada job na hora no mesmo processo, para testes e desenvolvimento), `cloudtasks` (o Google Cloud Tasks acorda o servidor para cada job; exige PostgreSQL) | [A fila de jobs](running.md#the-job-queue-and-failed-dead-jobs) |
| Armazenamento chave-valor, para caches, limites de taxa e as respostas guardadas por um dia para escritas reenviadas | [`kv`](../reference/configuration.md#kv) | `db` (uma tabela no banco de dados), `memory` (cada processo por si), `valkey` (Valkey ou outro servidor com protocolo Redis, compartilhado por todos os processos) | [`kv`](../reference/configuration.md#kv) |
| Armazenamento de mídia | [`blob`](../reference/configuration.md#blob) | `fs` (um diretório), `s3` (Amazon S3, Cloudflare R2, MinIO, Garage e outros armazenamentos compatíveis com S3), `gcs` (Google Cloud Storage) | [Arquivos do Docker Compose](running.md#docker-compose-files), [backup da mídia](backup.md#media-in-a-bucket) |
| Processamento de mídia | [`media_image`](../reference/configuration.md#media_image), [`media_video`](../reference/configuration.md#media_video), [`media_audio`](../reference/configuration.md#media_audio), um para cada tipo de arquivo | `native` (no servidor), `plugin` (um plugin seu converte os arquivos desse tipo) | [Convertendo mídia](plugins.md#converting-media) |
| Fluxo em tempo real entre processos | [`stream`](../reference/configuration.md#stream) | `hub` (dentro de um processo), `postgres` (`LISTEN` / `NOTIFY`), `valkey`, `sns` (Amazon SNS e SQS), `pubsub` (Google Cloud Pub/Sub) | [`stream`](../reference/configuration.md#stream) |
| Gerenciador de chaves, que criptografa as chaves privadas das contas | [`key_manager`](../reference/configuration.md#key_manager) | `local` (uma chave de criptografia de chaves que você guarda), `gcpkms` (Google Cloud KMS), `awskms` (AWS KMS), `plugin` | [Gerenciando a chave](key-management.md) |
| E-mail | [`mail`](../reference/configuration.md#mail) | `none`, `smtp`, `ses` (Amazon SES), `plugin` | [Enviando e-mail](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`, `turnstile` (Cloudflare Turnstile), `hcaptcha`, `recaptcha` (Google reCAPTCHA v2) | [CAPTCHA](captcha.md) |
| Chat de voz | [`voice`](../reference/configuration.md#voice) | `off`, `p2p` (de navegador para navegador, com TURN de `voice_turn`: `none`, `coturn` ou `cloudflare`), `livekit`, `sora` | [Chat de voz](voice.md) |
| Pagamentos de apoiadores | [`payments`](../reference/configuration.md#payments) | `none`, `stripe`, `payjp` (PAY.JP) | [Apoiadores](supporters.md), [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md) |
| Plugins | `plugin_<kind>_address` | nenhum por padrão; um endereço para cada tipo: `translator`, `search`, `billing`, `mailer`, `media_image`, `media_video`, `media_audio`, `key_manager` e os ganchos de decisão `inbound.status`, `local.status`, `inbound.follow`, `signup.check`, `media.check`, `inbound.actor`, `local.profile` | [Plugins](plugins.md), [ganchos de decisão](decision-hooks.md) |
| Tradução de publicações | `translator_<provider>_*`, [`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | nenhum (apenas a tradução do próprio navegador); `libretranslate`, `deepl`, `google` (Google Cloud Translation), `aws` (Amazon Translate) ou um plugin seu na frente do serviço que você escolher | [Traduzindo publicações](translation.md), [um plugin tradutor](plugins.md#translating-posts) |
| Busca de texto completo das publicações | [`search`](../reference/configuration.md#search) | `none`; ou `elasticsearch`, `opensearch` ou `meilisearch` ([mecanismos de busca](search-engines.md)); ou `plugin`, um plugin seu na frente de outro mecanismo | [Busca de texto completo](search.md), [um plugin de busca](plugins.md#searching-posts) |
| CDN para mídia | [`media_origin`](../reference/configuration.md#media_origin) | não definido (o servidor serve a mídia a partir da própria origem) ou a origem da sua CDN | [Servindo mídia por uma CDN](cdn.md) |
| Custos de nuvem | [`billing_sources`](../reference/configuration.md#billing_sources) | nenhum, ou qualquer um de `aws` (AWS Cost Explorer), `gcp` (exportação de cobrança ou orçamento do Google Cloud), `cloudflare` (cobranças de uso da Cloudflare), `plugin` | [Importando custos de nuvem](billing-import.md) |
| Métricas | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`, `prometheus`, `otlp` (OpenTelemetry), `emf` (CloudWatch) | [Métricas](running.md#metrics), [alertas](alerts.md) |
| Traces | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`, `otlp` | [Traces](running.md#traces) |
| Relatórios de erros | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none` (apenas a linha de log e os grupos de erros), `sentry` (Sentry, GlitchTip, Datadog), `otlp` | [Relatório de erros](error-reporting.md) |

O binário e a imagem de contêiner publicados trazem todos os adaptadores da tabela.

`my config check` relata todos os problemas da configuração de uma vez, antes de você iniciar o servidor
([lista de verificação de produção](production-checklist.md)).

<!-- source: from-a-small-server-to-a-large-one 2f46b66f047c -->
## De um servidor pequeno a um grande

![Três etapas de um servidor pequeno a um grande: um processo com SQLite e o sistema de arquivos, depois PostgreSQL e armazenamento de objetos atrás de uma CDN, depois processos web e worker separados com um broker de streams. A lista abaixo descreve cada etapa.](images/growth.svg)

1. **Comece pequeno.** Um processo com `role = all`, em SQLite, com a mídia no sistema de arquivos e os padrões para o
   restante: a fila e o armazenamento chave-valor no banco de dados, e o fluxo em tempo real dentro do processo. Os arquivos do
   Docker Compose em `deploy/docker` iniciam essa configuração em uma máquina ([implantando com Docker Compose](deploy-docker-compose.md)).
   Ela serve a um servidor pessoal ou pequeno. `stream = hub` alcança apenas o processo em que roda, então essa configuração fica em um
   processo.
2. **Tire os dados de lá.** PostgreSQL para o banco de dados, um bucket compatível com S3 ou o Google Cloud Storage para a mídia
   e uma CDN na frente da mídia. O servidor ainda é um processo, mas não depende mais de um disco, e os
   processos da etapa seguinte podem compartilhar os dados. Decida isso antes da primeira inicialização, se puder: mover os dados
   do SQLite e do sistema de arquivos depois é um trabalho à parte ([o que incluir no backup](backup.md#what-to-back-up) lista
   o que precisa ser movido).
3. **Escale.** Execute processos com `role = web` para as requisições e `role = worker` para os jobs, quantos cada um
   precisar, e divida os workers por tipo de job ([dividindo workers](running.md#splitting-workers-by-job-type)).
   Com mais de um processo, o fluxo em tempo real precisa de um broker que alcance todos os processos (`postgres`, `valkey`,
   `sns` ou `pubsub`), e o armazenamento chave-valor deve ser compartilhado (`db` ou `valkey`), já que `memory` mantém os limites de taxa
   e as respostas a escritas reenviadas de cada processo separados. A fila de jobs continua no
   PostgreSQL, ou passa para o Google Cloud Tasks ([Cloud Tasks](running.md#cloud-tasks)).

A [implantação no Cloud Run](deploy-google-cloud.md) é um exemplo de configuração entre a segunda e a terceira etapa:
`role = all` no Cloud Run, PostgreSQL no Neon, mídia no Cloud Storage, Cloud Tasks para os jobs e Pub/Sub para o
fluxo, sem nenhuma instância em execução enquanto ninguém usa o servidor.

<!-- source: where-to-go-next 2f1d25c01f8a -->
## Onde ir em seguida

Leia os guias do operador nesta ordem:

1. **Começar:** [rode o servidor](running.md) ou [implante no Cloud Run](deploy-google-cloud.md), e depois passe pelo [checklist de produção](production-checklist.md) antes de abri-lo para os usuários.
2. **Configurar a instância:** suas [configurações](instance-settings.md), o [cadastro](registration.md), os [provedores de login externos](login-providers.md), o [e-mail](email.md), o [CAPTCHA](captcha.md), [o CDN](cdn.md), as [pré-visualizações de links](link-previews.md) e [mecanismos de busca e rastreadores de IA](crawlers.md).
3. **Ativar recursos:** a [moderação](moderation.md), os [relays](relays.md), os [webhooks](webhooks.md), os [plugins](plugins.md) e os [ganchos de decisão](decision-hooks.md), o [chat de voz](voice.md), a [busca de texto completo](search.md), os [apoiadores](supporters.md) e seus pagamentos, e a [importação de custos de nuvem](billing-import.md).
4. **Operar no dia a dia:** o [monitoramento](monitoring.md), os [alertas](alerts.md), os [relatórios de erro](error-reporting.md), a [manutenção](maintenance.md), o [backup](backup.md), [o banco de dados](database.md), [a chave de cifragem de chaves](key-management.md) e [administradores e funções](admin-cli.md).
5. **Jurídico e encerramento:** os modelos de [política de privacidade](privacy-policy-template.md) e de [registro de tratamento](processing-record-template.md), e [encerrando o serviço](service-closure.md).

A [referência de configuração](../reference/configuration.md) lista cada configuração, e a [referência de jobs](../reference/jobs.md)
lista cada job que os workers executam.
