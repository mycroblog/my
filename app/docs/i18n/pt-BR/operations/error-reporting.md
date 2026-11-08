<!-- source: error-reporting 1ccf0c1c98a4 -->
# Relatórios de erro

Como o servidor informa sobre os próprios bugs e falhas, onde ler os relatórios em cada plataforma e como enviá-los a um rastreador de erros.
As métricas dizem que algo está errado; um relatório de erro diz o que falhou, com a sua pilha de chamadas. A [referência de configuração](../reference/configuration.md) lista todas as configurações.

Três coisas acontecem com todo relatório, qualquer que seja a configuração:

1. Uma linha `error report` vai para a saída de erro padrão, com a pilha ([A linha de log](#the-log-line)).
2. O relatório é contado em um grupo de erros no banco de dados. A tela de administração mostra os grupos, e as pessoas com a permissão `manage_errors` são notificadas dos grupos novos e dos que se repetem ([Grupos de erros e notificações](#error-groups-and-notifications)).
3. Se você definir `errors_exporter`, o relatório também vai para um serviço externo ([Enviando relatórios para um serviço](#sending-reports-to-a-service)).

Nada sai do servidor a menos que você defina `errors_exporter`.

<!-- source: what-is-reported 8e5ac9d14c36 -->
## O que é reportado

| Reportado | Não reportado (as métricas e as outras linhas de log cobrem esses casos) |
| --- | --- |
| Um panic em um handler HTTP, em um job ou em uma tarefa em segundo plano | Respostas 4xx (entrada inválida, autorização, limites de taxa) e o modo de manutenção |
| Um erro por trás de uma resposta 5xx da API cliente, da API administrativa, das páginas de login, do ActivityPub, do OAuth ou da mídia | Uma requisição cujo cliente desistiu |
| Um job que falhou de vez (`dead`) | Uma nova tentativa de job. Ela é reportada quando esgota as tentativas |
| Uma falha ao iniciar: uma configuração inválida, ou um banco de dados ou adaptador que não pode ser preparado | Uma verificação de prontidão que falhou |
| Um erro que o cliente web não tratou, enviado pelo navegador ao servidor | Falhas causadas por outros servidores: novas tentativas e desistências de entrega, assinaturas rejeitadas, buscas remotas que falharam |
| | Os ruídos do próprio navegador: uma rede perdida, erros de extensões do navegador e pedaços de código que um novo deploy removeu |

Uma falha de um serviço externo (e-mail, armazenamento, pagamentos, voz) só é reportada quando faz um job falhar de vez ou uma requisição responder 5xx. Uma falha temporária aparece nas métricas `my.adapter.*` ([Executando o servidor](running.md#metrics)).

Um panic em uma tarefa em segundo plano para o processo depois de reportado, para que o servidor nunca continue rodando em estado quebrado. A sua plataforma o reinicia.

<!-- source: the-log-line 6b570d0bfd09 -->
## A linha de log

A linha tem `msg` = `error report` e `level` `ERROR` (`severity` é `CRITICAL` quando a falha para o processo).
Os seus campos, e o que a sanitização faz com eles, estão em [Executando o servidor](running.md#error-reports). Em resumo:

- `my.error.fingerprint` é o mesmo para toda ocorrência do mesmo erro. Pesquise ou agrupe por ele.
- `my.error.where` indica onde aconteceu, por exemplo `client_api GET /api/v1/statuses/{id}`.
- `request_id`, `job_id`, `trace_id` e `span_id` encontram as outras linhas da mesma requisição ou do mesmo job.
- `message` fica por último na linha: o tipo e a mensagem do erro, uma quebra de linha e a pilha do Go.

Nenhuma outra linha traz uma pilha.

<!-- source: google-cloud-cloud-run 925c99683713 -->
### Google Cloud (Cloud Run)

O Cloud Error Reporting lê a linha sem nenhuma configuração e lista os erros no console, em Error Reporting. O serviço é gratuito; você paga apenas pelo Cloud Logging.

- Ele agrupa os erros pelo tipo e pelos frames do topo da pilha. Não consegue usar a impressão digital do servidor, então um grupo no Error Reporting pode ser diferente de um grupo na tela de administração.
- Ele mantém os grupos por 30 dias, e avisa sobre erros novos e sobre erros resolvidos que acontecem de novo, no máximo 20 notificações por hora para o projeto. Configure o canal de notificação dele no console do Google Cloud.
- Para descobrir o que aconteceu em torno de um relatório, pesquise no Cloud Logging por `jsonPayload."request_id"="<id>"`, ou o mesmo com `trace_id`.

<!-- source: aws 6ca6594afd7e -->
### AWS

O CloudWatch Logs lê a linha como JSON comum.

- Ative a detecção de anomalias de log para o grupo de logs. Ela sinaliza um padrão de linha que nunca viu antes e pode disparar um alarme no SNS. Ela lê apenas os primeiros 1.500 caracteres de uma linha, por isso `message` é o último campo. Ela não mantém um estado de resolvido.
- Para contar os relatórios, crie um filtro de métrica como `{ $.msg = "error report" }`. Não use como dimensão nenhum valor de `my.error.fingerprint`: cada valor de uma dimensão é cobrado como uma métrica própria. `my.error.source` e `my.error.kind` são seguros.
- Encontre o contexto com o Logs Insights: ``filter `request_id` = "<id>"``.

<!-- source: self-hosted 90de7a419474 -->
### Instalação própria

Leia a linha no log do contêiner: `docker compose logs my | jq -c 'select(.msg == "error report")'`.
Com `metrics_exporter = prometheus`, o alerta `MyErrorReportsIncreasing` ([Alertas e runbook](alerts.md#myerrorreportsincreasing)) avisa quando os relatórios sobem de forma acentuada; o contador por trás dele é `my.errors.reported`.
Para notificações que não precisam de um serviço externo, use a tela de administração e os webhooks abaixo. Para um rastreador, use [`errors_exporter`](#sending-reports-to-a-service).

<!-- source: error-groups-and-notifications 8f740a328309 -->
## Grupos de erros e notificações

O servidor conta os relatórios por impressão digital no banco de dados. Ele não precisa de configuração nem de serviço extra, e grava pouco: o primeiro relatório de uma impressão digital é gravado de imediato, e os seguintes são somados em memória e gravados uma vez por minuto. Uma parada forçada do processo pode perder as contagens do último minuto; as linhas de log não se perdem desse jeito.
Contagens com mais de 90 dias são excluídas todos os dias. Se o banco de dados é o que falhou, o relatório fica apenas na linha de log.

<!-- source: the-administration-screen 09092782a7c1 -->
### A tela de administração

As pessoas cuja função tem `manage_errors` (a função integrada `admin` tem) veem “Erros” no menu de administração (`/admin/errors`).

- A lista mostra os grupos, por padrão os abertos. Filtre por estado (aberto, resolvido, silenciado, todos os estados) e por origem (o servidor, o cliente web). Cada linha mostra o tipo do erro, onde aconteceu, as primeiras linhas da mensagem, as contagens das últimas 24 horas e dos últimos 7 dias, e a primeira e a última vez e versão.
- A página de um grupo mostra a ocorrência mais recente (horário, versão, função do processo, ID da requisição, ID do trace, os tipos, a mensagem e a pilha), as ocorrências por hora dos últimos 90 dias e o histórico das mudanças de estado. O ID da requisição e o ID do trace têm um botão “Copiar”: cole-os na sua busca de logs.
- “Resolver” serve para um erro que você corrigiu. Se ele acontecer de novo em outra versão do servidor, o grupo volta a abrir e notifica de novo. O mesmo erro na versão resolvida é apenas contado, então as últimas requisições da versão antiga não o reabrem.
- “Silenciar” continua contando, mas não envia notificação nem evento de webhook. “Deixar de silenciar” reabre o grupo.

As mesmas ações estão disponíveis em `GET /api/v1/admin/error_groups` e `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`).

<!-- source: notifications-and-webhooks 99836c7f02ec -->
### Notificações e webhooks

- As pessoas com `manage_errors` recebem a notificação `admin_error_group` para um grupo de erros novo e para uma recorrência. No máximo 10 chegam por hora para o servidor inteiro; o restante da hora se agrupa em uma notificação que diz que mais erros foram reportados. Elas também podem recebê-la por e-mail ([Moderação](moderation.md#notifications-for-moderators-and-the-pending-counts)).
- Os eventos de webhook `error_group.created` e `error_group.regressed` trazem os mesmos grupos, para que você possa enviá-los ao Slack, ao Discord ou a uma ferramenta de automação ([Webhooks do servidor](webhooks.md), [Referência de webhooks](../reference/webhooks.md)). O webhook exige `manage_webhooks`, e os eventos exigem `manage_errors`.
- Um grupo silenciado nunca notifica.

<!-- source: errors-from-the-web-client e5b2f45b62be -->
### Erros do cliente web

O cliente web envia os erros que as suas telas não trataram para `POST /api/v1/client_errors` no seu próprio servidor, então nenhum terceiro os vê e nenhuma configuração de segurança de conteúdo muda. O servidor responde `204` a tudo e descarta o que não aceita: um corpo maior que 16 KiB, uma pilha sem um arquivo do seu próprio cliente web e mais de 10 relatórios por minuto de um mesmo endereço IP. O mesmo erro é enviado uma vez por visualização de página, e no máximo 20 por visualização de página.
Um relatório enviado sem uma sessão iniciada é contado e registrado, mas não notifica ninguém por si só, então um script anônimo não consegue encher a sua caixa de entrada. O grupo notifica quando o navegador de uma pessoa com sessão iniciada também o reporta.
As pilhas são as da build minificada e não são traduzidas de volta para o código-fonte, então leia primeiro a rota e a mensagem. A versão é a do servidor que serviu a página. Uma página que abriu offline, ou depois de 3 segundos sem resposta, vem da cópia do navegador, que indica a versão de quando o navegador armazenou o cliente web: uma versão mais antiga com o mesmo cliente web.

<!-- source: sending-reports-to-a-service eb5f208ca8fc -->
## Enviando relatórios para um serviço

Defina `errors_exporter` para enviar também cada relatório a um rastreador de erros. A linha de log e os grupos no banco de dados continuam ativos. Um relatório é enviado uma única vez e nunca tentado de novo, e nada espera pelo serviço.

| Valor | Envia para |
| --- | --- |
| `none` (padrão) | Lugar nenhum |
| `sentry` | Qualquer serviço compatível com Sentry em `errors_sentry_dsn` |
| `otlp` | Um coletor ou backend do OpenTelemetry, como registros de log |

`errors_environment` (padrão `production`) nomeia a implantação em cada relatório, como `staging`.

Os dois exportadores enviam os mesmos campos sanitizados da linha de log, com a impressão digital e os IDs de trace e de span, e nada sobre o host: nenhum nome de host, endereço IP, usuário, corpo de requisição, cabeçalho ou cookie.

O servidor limita o que envia para que o serviço não seja inundado:

- A mesma impressão digital sai no máximo uma vez por minuto por processo, e todos os relatórios no máximo 60 por minuto. O restante fica no log e nos grupos.
- Até 1.000 relatórios esperam para ser enviados. Um relatório que encontra a fila cheia é descartado. Quando o servidor para, ele aguarda até 2 segundos para a fila esvaziar.
- Quando o serviço responde `429` ou pede para desacelerar (`Retry-After`, `X-Sentry-Rate-Limits`), o servidor para de enviar por esse tempo.
- O servidor nunca segue um redirecionamento do serviço, para que a chave do DSN e o relatório não vão para nenhum outro lugar: um redirecionamento conta como `failed`. Use o DSN que o serviço mostra, que aponta para a própria entrada dele.

Acompanhe o contador `my.errors.exported`: `my.outcome` é `success`, `dropped` (acima de um limite, a fila estava cheia ou o serviço pediu para desacelerar) ou `failed` (o serviço não pôde ser alcançado ou recusou o relatório) ([Executando o servidor](running.md#metrics)).

<!-- source: sentry-glitchtip-and-datadog 12c935ec3c05 -->
### Sentry, GlitchTip e Datadog

```sh
MY_ERRORS_EXPORTER=sentry
MY_ERRORS_SENTRY_DSN_FILE=/run/secrets/sentry_dsn   # or MY_ERRORS_SENTRY_DSN; the DSN is a secret
```

O servidor fala o protocolo do Sentry por conta própria, então nenhum SDK do Sentry está envolvido e nenhuma variável de ambiente `SENTRY_*` é lida. Ele define a impressão digital, para que o serviço agrupe por ela, e as tags `service` (`my`), `my.role`, `my.error.where` e `error.fingerprint`. A release é a versão do servidor e o ambiente é `errors_environment`.

- **Sentry** (hospedado ou instalado por você): copie o DSN das chaves de cliente do projeto.
- **GlitchTip**: copie o DSN do projeto. Ele é de código aberto e roda em um servidor pequeno. Verifique quais regras de alerta a sua versão oferece para problemas novos e recorrentes.
- **Datadog Error Tracking**: use o DSN do Sentry do seu site do Datadog, `https://<DD_API_KEY>@sentry-intake.<DD_SITE>/1`. O Datadog exige a tag `service`, que o servidor sempre define. O Datadog guarda esses eventos como logs e os agrupa no Error Tracking. Se, em vez disso, você enviar a linha de log ao Datadog por um agente ou um encaminhador (Error Tracking for Logs), a pilha fica em `message`; mapeie-a para `error.stack` com um pipeline de logs, senão o Datadog não a vê como pilha. O exportador `sentry` não precisa desse mapeamento.

<!-- source: otlp-backends 778540dabdc8 -->
### Backends OTLP

```sh
MY_ERRORS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318   # or OTEL_EXPORTER_OTLP_LOGS_ENDPOINT
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20...
```

Cada relatório é um registro de log do OpenTelemetry chamado `exception`, com severidade `ERROR` (`FATAL` quando para o processo), `exception.type`, `exception.message`, `exception.stacktrace`, `my.error.fingerprint`, `my.error.where`, `my.error.source`, `my.error.kind` e os IDs de trace e de span. Ele vai por OTLP/HTTP (protobuf, gzip) com o mesmo recurso das métricas e dos traces.

Se o backend transforma esses registros em erros agrupados depende do backend: alguns montam as visões de erro só a partir dos dados de trace, e o Datadog precisa de `error.stack`. Para Sentry, GlitchTip e Datadog, use `sentry`. Use `otlp` quando quiser os registros junto dos seus outros logs. A build tag `no_otel` deixa `otlp` de fora ([Build tags](../reference/build-tags.md)).

<!-- source: what-scrubbing-does-and-does-not-guarantee ccf1b4716bdd -->
## O que a sanitização faz e o que não garante

A sanitização roda uma vez, quando o relatório é criado, então a linha de log, o banco de dados e todos os exportadores recebem apenas o texto sanitizado.

- Na mensagem, URLs, endereços de e-mail, identificadores `@user@host`, endereços IP, UUIDs e ULIDs, e números de 10 dígitos ou mais são substituídos por `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` e `<n>`, e a mensagem é cortada em 1 KiB.
- Na pilha, os valores dos argumentos das funções são substituídos por `(...)` e a pilha é cortada em 16 KiB. As posições dos arquivos permanecem, relativas à árvore do código-fonte, para que os caminhos da máquina de build não apareçam.
- Nenhum relatório tem corpo de requisição, cabeçalho, query, cookie, ID de conta, nome de usuário, endereço IP ou User-Agent.

A sanitização compara padrões, então não garante que uma mensagem não contenha dados pessoais. Um nome, um texto livre ou um número com menos de 10 dígitos que uma mensagem de erro cite podem permanecer. O campo `error` das linhas comuns `http request` e `job` não é sanitizado dessa forma. Trate os logs e o rastreador de erros como lugares que podem conter fragmentos de conteúdo de usuários, e dê a eles o mesmo controle de acesso e a mesma retenção que ao restante dos seus logs. Quando você define `errors_exporter`, a localização, a retenção e as regras de acesso desse serviço passam a valer para os relatórios.
