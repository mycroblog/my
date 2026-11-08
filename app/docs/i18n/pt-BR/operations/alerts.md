<!-- source: alerts-and-runbook 2c201450ab32 -->
# Alertas e runbook

Sobre o que as métricas do próprio servidor devem alertar você, o quão urgente é cada alerta e o que verificar
e fazer quando um deles dispara. O monitoramento do servidor por fora (monitores de disponibilidade e uma página de status) está em [Monitorar o servidor por fora](monitoring.md);
configure isso também, porque nenhuma métrica chega de um servidor que está fora do ar.

<!-- source: getting-the-metrics 58d8055fe44d -->
## Obter as métricas

O servidor exporta suas métricas quando `metrics_exporter` está definido ([referência de configuração](../reference/configuration.md#metrics_exporter));
o [guia de execução](running.md) lista cada métrica e seus rótulos. Os alertas abaixo foram escritos para o Prometheus, e os nomes são
os que o Prometheus enxerga: os pontos viram sublinhados, e os sufixos de unidade e de contador são adicionados
(`my.queue.oldest_ready_age` vira `my_queue_oldest_ready_age_seconds`, `my.delivery.attempts` vira `my_delivery_attempts_total`).

| Destino | Configuração | Alertas |
| --- | --- | --- |
| Auto-hospedado (Docker Compose, Kubernetes, uma VM) | `metrics_exporter = prometheus` | O Prometheus coleta `GET /metrics` em `metrics_listen`, e o Alertmanager encaminha os alertas. As regras abaixo funcionam como estão |
| Qualquer destino com um OpenTelemetry Collector | `metrics_exporter = otlp` | O Collector encaminha para o backend de sua escolha. Um backend compatível com o Prometheus que adiciona os sufixos de unidade e de tipo (como o receptor OTLP do próprio Prometheus faz por padrão) enxerga os mesmos nomes |
| Google Cloud (Cloud Run) | `metrics_exporter = none` | Políticas de alerta do Cloud Monitoring sobre uma verificação de disponibilidade, as métricas da plataforma e métricas baseadas em logs (veja [Cloud Run](#google-cloud-cloud-run)) |
| AWS (ECS) | `metrics_exporter = emf` | Alarmes do CloudWatch sobre as métricas que o CloudWatch Logs extrai (veja [AWS ECS](#aws-ecs)) |

<!-- source: prometheus-and-alertmanager 8e8dcaa334e9 -->
### Prometheus e Alertmanager

As regras estão em [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml). Copie o arquivo para junto do seu
`prometheus.yml` e carregue-o:

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` tem como padrão `127.0.0.1:9464`, que só um Prometheus no mesmo host alcança. No Docker Compose ou no Kubernetes, defina
  `MY_METRICS_LISTEN=0.0.0.0:9464` e mantenha a porta na rede privada: o listener não tem autenticação.
- Colete de todos os processos (cada processo com `role = web` e `role = worker` tem seu próprio listener). Os gauges de fila, mídia, tamanho do banco de dados,
  servidores remotos, importação de cobrança e snapshot trazem o mesmo valor de toda a instância em cada processo, por isso as regras usam o máximo deles.
- Toda regra tem um rótulo `severity`: **`page`** precisa de alguém agora (os usuários são afetados ou logo serão), **`ticket`** pode esperar o horário comercial.
  Encaminhe cada um para receptores diferentes no Alertmanager. O `runbook_url` de cada regra aponta para a seção correspondente abaixo.
- Os limites são pontos de partida para um servidor pequeno. Observe os valores por uma semana e ajuste os limites e as durações de `for` ao seu tráfego.
  `MyRemoteMediaCacheOverLimit`, `MyMediaStorageLarge` e `MyDatabaseStorageLarge` trazem tamanhos que você deve ajustar aos seus próprios limites.
- Durante o modo de manutenção, o servidor não serve métricas e responde `503`, então `MyTargetDown` e os alertas de HTTP disparam.
  Silencie os alertas no Alertmanager para o período antes de começar ([Manutenção](maintenance.md#steps)).

Depois de alterar as regras, verifique o arquivo e execute seus testes unitários (`alerts.test.yml`) com o `promtool`, que acompanha o Prometheus:

```sh
cd deploy/monitoring/prometheus
promtool check rules alerts.yml
promtool test rules alerts.test.yml
```

<!-- source: google-cloud-cloud-run 72c348585d24 -->
### Google Cloud (Cloud Run)

Com `metrics_exporter = none`, monte os alertas a partir do que o Cloud Run e o Cloud Logging já têm:

- **Erros e latência de HTTP**: a contagem de requisições do Cloud Run por classe de código de resposta e a métrica de latência de requisições do serviço.
- **Erros**: as linhas `error report` vão para o Error Reporting, que as agrupa e pode avisar você sobre erros novos e recorrentes ([Relatório de erros](error-reporting.md#google-cloud-cloud-run)).
- **Jobs e requisições**: o servidor grava uma linha de log por requisição HTTP (`msg` = `http request`) e por execução de job (`msg` = `job`), com os
  mesmos rótulos das métricas. Crie métricas baseadas em logs a partir delas, por exemplo um contador de `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  para jobs com falha, ou uma distribuição de `jsonPayload.duration_seconds` filtrada por `jsonPayload."my.surface"="client_api"` para a latência.
- **Fila acumulada**: com `queue = cloudtasks`, os jobs ficam no banco de dados, então a tela de jobs e as linhas de log das execuções mostram o acúmulo;
  a métrica de profundidade de fila do Cloud Tasks conta os sinais que aguardam entrega.
- **Banco de dados**: as métricas de utilização de disco e de conexões do Cloud SQL, ou os alertas de uso do seu provedor de banco de dados.
- **Custo**: um orçamento do Cloud Billing com limites de alerta ([Custo e orçamento](#cost-and-budget)).

O módulo [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) cria esses itens como políticas com os nomes da tabela abaixo, e lista cada alerta que cobre de outra forma ou que não consegue expressar. Ele nunca foi aplicado. Uma métrica que só o tráfego produz tem lacunas em um serviço que escala a zero, então um alerta sobre a ausência dela dispara por engano; a verificação de disponibilidade é o que percebe um servidor ocioso que morreu.

<!-- source: aws-ecs 609e5bf632ef -->
### AWS ECS

Com `metrics_exporter = emf`, o CloudWatch Logs transforma as linhas do servidor em métricas no namespace `metrics_emf_namespace`, com os nomes
com pontos (`my.job.duration`, `my.queue.oldest_ready_age`). As métricas de duração de requisições e de jobs vêm das linhas de log de requisições e de jobs, e as
dimensões são os rótulos que você lista em `metrics_emf_dimensions`; inclua `my.outcome` para alarmar sobre jobs com falha e `http.response.status_code`
para alarmar sobre 5xx. Os gauges de toda a instância (fila, mídia, tamanho do banco de dados, servidores remotos, importações de cobrança, snapshot) são gravados pelo job periódico de snapshot. As métricas do runtime do Go (`go.*`)
não são gravadas em EMF; use no lugar delas as métricas de CPU e memória do serviço ECS.

- **Erros de HTTP**: as métricas de 5xx e de contagem de requisições do Application Load Balancer.
- **Fila acumulada**: `my.queue.oldest_ready_age`, que o job de snapshot grava.
- **Banco de dados**: as métricas de armazenamento e de conexões do RDS ou do Aurora, ou os alertas de uso do seu provedor de banco de dados.
- **E-mail** (`mail = ses`): as métricas de reputação do SES (taxas de rejeição e de reclamação) no CloudWatch.
- **Custo**: um orçamento do AWS Budgets com alertas ([Custo e orçamento](#cost-and-budget)).

O módulo [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) cria esses itens como alarmes com os nomes da tabela abaixo, com a mesma lista. Ele nunca foi aplicado.

<!-- source: the-alerts d76842ca02b8 -->
## Os alertas

Severidade: **page** = agir agora, **ticket** = durante o horário comercial. “–” na coluna de regra significa que ainda não há métrica para ela; verifique-a como descrito.

| Área | Alerta | Sinal | Limite | Severidade | Causas prováveis |
| --- | --- | --- | --- | --- | --- |
| Fila | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | acima de 5 min por 10 min | ticket | Poucos workers para um pico (uma publicação popular, um relay), jobs lentos, um worker que parou |
| Fila | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | acima de 30 min por 5 min | page | Nenhum worker em execução (travou, não foi implantado, só `role = web`), nenhum worker executa um tipo de job, workers presos no banco de dados |
| Fila | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | acima de 10 por hora por 15 min | ticket | Um bug em um job, um adaptador mal configurado (e-mail, blob, cobrança), um serviço remoto que recusa |
| Fila | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | parte da fila acima de 5 min, parte dos totais acima de 1 h, por 15 min | ticket | O armazenamento KV está inacessível, as consultas ao banco de dados do snapshot falham ou excedem o tempo |
| Federação | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_delivery_attempts_total` com `my_outcome` `retry` ou `failed` | acima de 50% das tentativas por 30 min | ticket | Muitos seguidores em servidores fora do ar, um servidor grande fora do ar, problema na rede de saída |
| Federação | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | igual | acima de 95% das tentativas por 15 min | page | Rede de saída ou DNS com defeito, egress bloqueado, chaves ou `host` do servidor incorretos |
| Federação | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 5 a mais que um dia antes | ticket | Servidores que foram desativados ou que bloqueiam este, um problema de rede deste lado que durou dias |
| Federação | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | acima de 25% dos POSTs por 30 min | ticket | Falha na verificação de assinatura (diferença de relógio, falha ao buscar chaves), um spammer ou um domínio suspenso tentando de novo, limites de taxa |
| Federação | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | acima de 10% dos POSTs por 30 min | ticket | O relógio do servidor está errado, a busca das chaves dos remetentes falha, um servidor que trocou de chaves |
| Federação | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | acima de 5% dos POSTs por 15 min | ticket | O banco de dados falhando, um bug na caixa de entrada |
| Erros | [`MyErrorReportsIncreasing`](#myerrorreportsincreasing) | `my_errors_reported_total` | acima de 10 em 15 min e 3 × a média do dia anterior, por 15 min | ticket | Um bug novo vindo de uma versão, uma dependência que falha de um jeito novo, uma configuração quebrada |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | 5xx de `http_server_request_duration_seconds_count` | acima de 5% por 10 min | page | O banco de dados ou o KV fora do ar, uma versão ruim, o modo de manutenção |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | igual | acima de 1% por 30 min | ticket | Uma rota com falha, um adaptador que falha de vez em quando |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | p95 da API de cliente (streaming excluído) | acima de 300 ms por 30 min | ticket | Banco de dados saturado ou distante, uma consulta lenta, pouca CPU, cold starts |
| Recursos | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 0 por 5 min | page | O processo está fora do ar, em loop de falhas ou no modo de manutenção; o caminho da coleta está quebrado |
| Recursos | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | qualquer aumento por 5 min | page | O banco de dados está inacessível, ou o esquema não foi aplicado |
| Recursos | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 3 reinícios em uma hora | ticket | Encerrado por falta de memória (OOM), um panic, uma sonda de liveness falhando |
| Recursos | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | acima de 90% por 15 min | ticket | `GOMEMLIMIT` baixo demais para a carga, muitos streams abertos, um vazamento |
| Recursos | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | acima de 90% por 15 min | ticket | Pool pequeno demais para o tráfego, consultas lentas segurando conexões |
| Recursos | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`, `db_client_connection_timeouts_total` | espera acima de 0,5 s por segundo, ou qualquer timeout, por 10 min | ticket | Igual ao anterior; o limite de conexões do próprio banco de dados foi atingido |
| Recursos | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | acima do limite do seu plano menos uma margem (exemplo 8 GiB) por 1 h | ticket | Crescimento de publicações e de conteúdo remoto em cache; hora de aumentar o plano ou reduzir a retenção |
| Recursos | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | acima de 80% por 15 min | ticket | Muitos streams e conexões abertos, um `ulimit -n` baixo |
| Recursos | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | acima de 5% das operações de um adaptador por 15 min | ticket | Valkey, o diretório de blobs ou o backend de streams falhando |
| Recursos | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | acima de 1,2 × `remote_media_cache_max_bytes` por 3 h | ticket | A limpeza por hora não é executada ou falha |
| Recursos | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | acima do tamanho planejado (exemplo 50 GiB) por 1 h | ticket | Crescimento de uploads; hora de aumentar o plano ou o orçamento |
| E-mail | [`MyEmailFailing`](#myemailfailing) | `my_job_duration_seconds_count` dos jobs de e-mail com `my_outcome="dead"` | qualquer um em uma hora | ticket | Configurações de SMTP ou SES incorretas, uma conta suspensa ou sem cota, endereços recusados em definitivo |
| E-mail | [`MyEmailRetrying`](#myemailretrying) | igual, `my_outcome="retry"` | acima de 50% das execuções por 15 min | ticket | O servidor de e-mail está inacessível ou recusa o login, um limite de taxa de envio |
| Custo | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 1 por 6 h | ticket | Permissões ausentes, a exportação de cobrança não configurada, o limite de requisições atingido, o provedor indisponível |
| Custo | Orçamento excedido | – | – | – | Tráfego, armazenamento ou uma configuração incorreta custando mais que o planejado ([abaixo](#cost-and-budget)) |

<!-- source: where-to-look 9ef1d37c4f05 -->
## Onde procurar

Estas ferramentas aparecem em quase todas as entradas do runbook abaixo.

- **A tela de jobs** (Administração → Jobs, `/admin/jobs`, exige `manage_jobs`): as contagens por estado (na fila, em execução, em nova tentativa, com falha) e por tipo, e
  o job mais antigo à espera; a página **Jobs** (`/admin/jobs/list`) lista os jobs de um estado, e a página de cada job mostra seu último erro e as tentativas que não deram certo. Os jobs pendentes só são listados para as filas mantidas no banco de dados
  (`queue = db` e `cloudtasks`); jobs com falha e tentativas são listados para todas as filas.
- **O comando `my job`** em um host que tem a configuração do servidor (`queue = db` e `cloudtasks`):
  `my job list` lista os jobs com falha, do mais novo para o mais antigo; `my job retry <id>...` os devolve à fila; `my job delete <id>...` os descarta.
  É seguro executar os jobs de novo.
- **A tela de servidores remotos** (Moderação → Servidores remotos, `/moderation/instances`, exige `manage_domains`): os servidores que as entregas não conseguiram alcançar,
  e aqueles cujas entregas estão pausadas, com o motivo e o momento. “Retomar entregas” marca um servidor como disponível de novo.
- **Os logs**: linhas JSON na saída de erro padrão. Cada requisição HTTP termina com uma linha cujo `msg` é `http request`, e cada execução de job com uma linha cujo `msg` é `job`
  (`INFO` para sucesso, `WARN` para nova tentativa, `ERROR` para um job que falhou em definitivo, com `error`, `job_id` e `my.job.type`).
  As linhas dentro de uma requisição trazem seu `request_id`, que o cliente vê como `X-Request-Id`.
  - Docker Compose: `docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging: `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights: ``filter msg = "job" and `my.outcome` = "dead"``
- **Relatórios de erro**: as linhas `error report` e a tela de Erros (Administração → Erros, `/admin/errors`, exige `manage_errors`) ([Relatório de erros](error-reporting.md)).
- **Traces**, quando `traces_exporter = otlp`: as linhas de log trazem `trace_id`; abra-o no seu backend de traces para ver onde uma requisição lenta gastou seu tempo.
- **O modo de manutenção** ([Manutenção](maintenance.md)): responde a toda requisição com uma página de manutenção e não toca em nenhum adaptador. Use-o enquanto você repara o banco de dados
  ou o armazenamento KV.

<!-- source: runbook e60e811fcff0 -->
## Runbook

<!-- source: myqueuebacklog e625ad2bb49f -->
### MyQueueBacklog

Jobs (entregas, notificações, e-mails, fan-out de timelines) esperam mais de 5 minutos por um worker.

Verifique:

- A tela de jobs: quais tipos estão na fila e se muitos estão em nova tentativa. Um pico de um só tipo (`federation.deliver` depois de uma publicação com muitos seguidores,
  atividades de relay) se resolve sozinho; confirme que a idade diminui.
- Se os processos worker estão em execução (`role = all` ou `role = worker`) e se os logs deles mostram linhas `job`.
- `MyDBPoolSaturated` ou `MyAPILatencyHigh` ao mesmo tempo: os workers esperam pelo banco de dados.

Corrija:

- Execute mais processos worker (`role = worker`), ou dê mais CPU aos existentes.
- Se um tipo de job está lento, encontre-o nas linhas de log `job` (`duration_seconds`) e procure erros desse tipo.

<!-- source: myqueuestalled c92f00f06f95 -->
### MyQueueStalled

O job pronto mais antigo espera há mais de 30 minutos: nada está sendo processado, ou quase nada.

Verifique:

- Se pelo menos um processo roda com `role = all` ou `role = worker`, e se ele não está em loop de falhas (`MyProcessRestarting`).
- Os logs do worker em busca de erros que se repetem a cada job (banco de dados, KV, um panic).
- `MyNotReady`: um worker que não alcança o banco de dados não pode pegar jobs.
- `max by (my_job_type) (my_queue_oldest_ready_age_seconds)`: se um tipo espera e os outros não, verifique se algum worker o executa ([Dividir workers por tipo de job](running.md#splitting-workers-by-job-type)).

Corrija:

- Inicie ou reinicie os workers. Os jobs que um worker parado segurava voltam à fila quando o lease expira; é seguro executá-los de novo.
- Se o banco de dados for a causa, siga [MyNotReady](#mynotready).

<!-- source: mydeadjobsincreasing b8e928621ac7 -->
### MyDeadJobsIncreasing

Mais de 10 jobs por hora falham em definitivo: esgotaram as tentativas, ou o job se recusou a ser executado de novo.

Verifique:

- A tela de jobs, filtrada por Com falha: os tipos e o último erro. As tentativas de um job mostram cada erro ao longo do caminho.
- Os mesmos erros nas linhas de log `job` com `my.outcome` = `dead`.

Corrija:

- Um erro de configuração (e-mail, blob, importação de cobrança, um destino de webhook): corrija-o e depois execute os jobs de novo pela tela de jobs (“Executar de novo”) ou com `my job retry <id>...`.
- As entregas a servidores que não existem mais falham em definitivo depois de cerca de 4 dias de tentativas; isso é esperado e não exige nada.
- Os jobs com falha que ninguém tenta de novo são removidos após `dead_job_retention` (30 dias por padrão). Exclua com `my job delete` os que você não vai tentar de novo.
- Um erro que parece um bug: relate-o com o tipo do job e o erro (ele não contém dados pessoais).

<!-- source: mymetricssnapshotstale c74fe15e6934 -->
### MyMetricsSnapshotStale

Os gauges de fila e de armazenamento vêm de um único snapshot que os processos compartilham pelo armazenamento KV. Ele está desatualizado, então os alertas de fila e de armazenamento
veem valores antigos e podem ficar em silêncio.

Verifique:

- Os logs em busca de erros sobre o snapshot de métricas ou o armazenamento KV.
- `MyAdapterErrors` para `kv.valkey` ou `kv.db`, e a saúde do banco de dados.

Corrija:

- Repare o armazenamento KV ou a conexão com o banco de dados. O snapshot é calculado de novo na próxima coleta; o job periódico de snapshot também é executado a cada 15 minutos.

<!-- source: mydeliveryfailureshigh 709985dfbf2b -->
### MyDeliveryFailuresHigh

Mais da metade das tentativas de entrega a outros servidores falham. Algumas falhas são normais: servidores desaparecem, e o servidor continua tentando por cerca de 4 dias.

Verifique:

- A tela de servidores remotos: se as falhas se concentram em poucos servidores (eles estão fora do ar; nada a fazer) ou se espalham por muitos (suspeite deste servidor).
- `http_client_request_duration_seconds_count` por `error_type`: timeouts, erros de DNS ou `forbidden_destination` (a proteção contra SSRF recusou o destino).

Corrija:

- Falhas em poucos servidores: nada. Depois de falhas em 7 dias diferentes, as entregas a esse servidor são pausadas, e o servidor é testado diariamente para retomá-las.
- Falhas em todo lugar: siga [MyDeliveryAllFailing](#mydeliveryallfailing).

<!-- source: mydeliveryallfailing c81fcf407cb9 -->
### MyDeliveryAllFailing

Quase todas as entregas falham. Quase sempre o problema está neste servidor.

Verifique:

- A partir do host ou do container do servidor, se o HTTPS de saída funciona (DNS, firewall de egress, NAT, um proxy).
- As linhas de log `job` de `federation.deliver` em busca do erro: erros de TLS, timeouts ou `401` de muitos servidores (eles não conseguem verificar a assinatura deste servidor).
- Depois de uma mudança de domínio ou de chave: `host` deve permanecer o mesmo depois que o servidor passou a federar, e a KEK deve ser a mesma com que as chaves foram seladas
  ([Gerenciamento da KEK](key-management.md)).

Corrija:

- Repare a rede de saída. As tentativas com falha são repetidas com backoff (1 minuto, 5 minutos, 30 minutos, 2 horas e assim por diante), então as entregas se recuperam sozinhas.
- Se as entregas esgotaram as tentativas, execute de novo os jobs `federation.deliver` com falha pela tela de jobs.
- Se servidores remotos foram pausados durante a interrupção, eles são retomados na próxima entrega bem-sucedida, na próxima requisição assinada ou no teste diário; você também pode pressionar
  “Retomar entregas” na tela de servidores remotos.

<!-- source: myremoteserverspaused 17c3ec51fb53 -->
### MyRemoteServersPaused

As entregas a 5 ou mais servidores foram pausadas no último dia. Um servidor é pausado depois que as entregas a ele falham em 7 dias diferentes, então algumas pausas por semana
são normais: servidores são desativados. Muitas de uma vez significam que muitos servidores falharam por dias ao mesmo tempo. `my_federation_instances` com
`my_federation_instance_state` = `available` conta os servidores que ainda recebem entregas mas falharam em algum dia; um salto ali vem antes das pausas.

A métrica não tem rótulo por servidor (os rótulos nunca carregam um domínio remoto). Verifique:

- A tela de servidores remotos: a aba “Pausados” lista os servidores pausados, com quando e por quê. Se eles têm algo em comum (um provedor de hospedagem, um software)
  ou estão espalhados.
- `MyDeliveryFailuresHigh` e `MyDeliveryAllFailing` na última semana: um problema de rede deste lado que durou dias pausa muitos servidores.

Corrija:

- Servidores que não existem mais: nada. Os servidores pausados são testados diariamente e retomados sozinhos quando respondem.
- Depois de um problema deste lado: repare-o e então pressione “Retomar entregas” para os servidores que importam para você, ou aguarde o teste diário.
- Um servidor cujo operador informa que bloqueia este: nada deste lado ajuda. Para parar de federar com um servidor de propósito, use uma ação de domínio
  (Moderação → Ações de domínio).

<!-- source: myinboxrejectionshigh ee5a244ccf12 -->
### MyInboxRejectionsHigh

Mais de um quarto das atividades que outros servidores enviam por POST às caixas de entrada são recusadas.

Verifique:

- `my_inbox_activities_total{my_outcome="rejected"}` por `my_inbox_rejection`: `signature` (siga [MyInboxSignatureFailures](#myinboxsignaturefailures)),
  `suspended` (um domínio suspenso que continua tentando), `federation_off` (a federação está desativada), `rate_limited` (um IP de cliente ou um servidor acima do
  limite de taxa) ou `invalid` (atividades sem `id`, `type`, `actor` ou `object` válidos).
- As linhas de log `http request` das rotas da caixa de entrada (`my.surface` = `activitypub`) trazem a mesma divisão por `http.response.status_code`
  (`401`, `403`, `429`, `400`) e o endereço do cliente, quando você precisa saber de onde vêm as requisições.

Corrija:

- As recusas de um domínio suspenso, ou com a federação desativada, não exigem nada: elas são recusadas como previsto.
- Requisições com limite de taxa vindas de um servidor: um spammer ou um pico; suspenda o domínio se continuar.
- Atividades inválidas de um servidor: o software desse servidor envia algo que este servidor não lê; relate com o software e a versão do servidor.

<!-- source: myinboxsignaturefailures 85aa65f3247c -->
### MyInboxSignatureFailures

Mais de 10% das atividades que outros servidores enviam por POST às caixas de entrada são recusadas porque a assinatura ou a prova de integridade está ausente ou não é verificada,
ou porque não foi possível buscar a chave do remetente.

Verifique:

- O relógio do servidor: as assinaturas são recusadas fora de uma janela de tempo. Verifique o NTP no host ou no runtime de containers.
- A busca das chaves dos remetentes: `MyDeliveryAllFailing` e `http_client_request_duration_seconds_count{my_http_purpose="json"}` por `error_type`
  (a rede de saída, o DNS, a proteção contra SSRF).
- Se as recusas vêm de muitos servidores ou de um só (as linhas `http request` com status `401`): um servidor que trocou de chaves, ou que assina
  de um jeito que este servidor não aceita, afeta apenas esse servidor.

Corrija:

- Corrija o relógio ou a rede de saída. Os outros servidores repetem suas entregas, então nada se perde se for corrigido em cerca de um dia.
- Um servidor que falha: relate ao operador desse servidor, ou a este projeto, com o software e a versão do servidor.

<!-- source: myinboxerrors e3cf874c925e -->
### MyInboxErrors

A caixa de entrada responde `500` a mais de 5% das atividades. Os remetentes tentam de novo, mas as atividades chegam atrasadas.

**Verifique** as linhas de log `http request` e de erro das rotas da caixa de entrada em busca do erro, e a saúde do banco de dados (`MyNotReady`, `MyDBPoolWaiting`).

**Corrija** o banco de dados ou relate o erro como um bug. Nada precisa ser reenviado: os remetentes tentam de novo.

<!-- source: myerrorreportsincreasing deed4980cc8c -->
### MyErrorReportsIncreasing

O servidor relata erros (panics, os erros por trás de 5xx, jobs com falha) a mais de três vezes sua taxa habitual.

Verifique:

- As linhas `error report` da última hora, agrupadas por `my.error.fingerprint`: as mais frequentes, o `my.error.where` delas e a linha de cabeçalho de `message`.
  No Cloud Run, o Error Reporting do Google Cloud mostra os mesmos grupos.
- Se uma versão, uma mudança de configuração ou uma dependência com falha (`MyNotReady`, `MyAdapterErrors`) começou ao mesmo tempo.
- As linhas de requisição ou de job com o mesmo `request_id` ou `job_id`.

Corrija:

- Uma dependência com falha ou um erro de configuração: corrija-o; os jobs com falha podem então ser executados de novo pela tela de jobs.
- Uma versão ruim: volte para a imagem anterior.
- Um bug: relate-o com o cabeçalho e a pilha da linha. Ambos têm os dados pessoais removidos.

<!-- source: myhttperrorshigh dd55e9ba0dfd -->
### MyHTTPErrorsHigh

Mais de 5% de todas as requisições respondem 5xx. Os usuários veem erros.

Verifique:

- Se você está no modo de manutenção (`503` esperado; silencie o alerta da próxima vez).
- `MyNotReady` (o banco de dados), `MyAdapterErrors` (KV, blob, stream) e uma versão recente.
- As linhas `http request` com `http.response.status_code` ≥ 500 por `http.route`, e as linhas de erro com o mesmo `request_id`.

Corrija:

- Repare a dependência com falha. Se o banco de dados precisar de trabalho, passe para o modo de manutenção para que os usuários vejam uma página clara em vez de erros.
- Uma versão ruim: volte para a imagem anterior.
- Informe os usuários ([Avisar os usuários sobre um incidente](#telling-users-about-an-incident)) se durar.

<!-- source: myhttperrorselevated 34d53d12ea4d -->
### MyHTTPErrorsElevated

Mais de 1% das requisições respondem 5xx ao longo de meia hora. Normalmente uma rota ou um adaptador.

**Verifique** as linhas `http request` com um 5xx por `http.route`, e as linhas de erro dessas requisições. **Corrija** a dependência, ou relate o erro como um bug.

<!-- source: myapilatencyhigh 81b66aee9099 -->
### MyAPILatencyHigh

O percentil 95 da API de cliente está acima de 300 ms (a meta para uma página de timeline inicial de 20 publicações, com cache aquecido). O endpoint de streaming fica de fora.

Verifique:

- `http_server_request_duration_seconds` por `http_route`: uma rota lenta, ou todas elas.
- O banco de dados: `MyDBPoolWaiting`, a CPU e o log de consultas lentas do banco de dados, a distância de rede entre o servidor e o banco de dados.
- O processo: limitação de CPU, `MyMemoryNearLimit` (um GC sob pressão de memória), `go_schedule_duration_seconds` (goroutines esperando por uma CPU).
- Traces de requisições lentas, se estiverem habilitados.

Corrija:

- Dê mais recursos ao banco de dados ou ao servidor, ou coloque-os mais perto um do outro.
- Cold starts depois de escalar a zero elevam o percentil em um servidor com pouco uso; isso é esperado.

<!-- source: mytargetdown e0308f141452 -->
### MyTargetDown

O Prometheus não consegue coletar de um processo.

Verifique:

- Se o processo está em execução: `docker compose ps`, `kubectl get pods` ou o console da plataforma. As últimas linhas de log dele dizem por que parou.
- Se ele está no modo de manutenção (nenhuma métrica é servida; esperado).
- Se `metrics_exporter = prometheus` e se `metrics_listen` é alcançável a partir do Prometheus.

Corrija:

- Inicie o processo de novo. Se ele não iniciar, o log diz por quê: um erro de configuração (`my config check` o mostra), o banco de dados inacessível, um esquema não aplicado.

<!-- source: mynotready 1092418fbba2 -->
### MyNotReady

A verificação de prontidão falha: o banco de dados está inacessível, ou suas tabelas não correspondem a esta versão. O balanceador de carga para de enviar tráfego ao processo,
e o endpoint de saúde externo informa `unavailable`.

Verifique:

- As linhas de log da verificação de prontidão: um erro de conexão (rede, credenciais, o banco de dados fora do ar ou sem conexões) ou as tabelas ausentes.
- A página de status e o console do provedor do banco de dados: ele está no ar, sem armazenamento ou em manutenção?
- Depois de uma atualização: o esquema foi aplicado antes de a nova versão iniciar ([Inicializar o banco de dados](database.md))?

Corrija:

- Banco de dados fora do ar ou inacessível: restaure-o ou faça failover. Enquanto você trabalha, o modo de manutenção mostra aos usuários uma página clara.
- Banco de dados sem armazenamento: aumente o plano ou libere espaço ([MyDatabaseStorageLarge](#mydatabasestoragelarge)).
- Esquema não aplicado: execute `my schema apply` da mesma versão, e os processos ficam prontos sozinhos.
- Dados perdidos ou corrompidos: restaure a partir do seu backup e guarde a KEK com que as chaves do backup foram seladas ([Gerenciamento da KEK](key-management.md)).

<!-- source: myprocessrestarting d96b041c2ff0 -->
### MyProcessRestarting

Um processo reiniciou 3 vezes em uma hora.

Verifique:

- O motivo do reinício segundo a plataforma: `OOMKilled` (memória), uma sonda de liveness com falha ou o código de saída.
- As últimas linhas de log antes de cada reinício, em busca de um panic ou de um erro fatal de configuração.

Corrija:

- Falta de memória: defina `GOMEMLIMIT` como cerca de 90% do limite de memória do container, ou dê mais memória ao container ([MyMemoryNearLimit](#mymemorynearlimit)).
- Um panic: relate-o com as linhas de log ao redor.
- O log diz que o banco de dados pertence a outro `host` ou `account_domain`: defina na configuração os valores registrados, ou aponte `database_url` para o banco de dados correto. Esses valores não podem mudar ([Configurações que você não pode alterar depois](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: mymemorynearlimit 438190b311c9 -->
### MyMemoryNearLimit

O runtime do Go usa mais de 90% de `GOMEMLIMIT`. Perto do limite, o coletor de lixo roda com muito mais frequência, o que custa CPU e latência,
e depois dele a plataforma pode encerrar o processo. Este alerta precisa de `GOMEMLIMIT`; sem ele, observe as métricas de memória do container.

**Verifique** `http_server_active_requests` e os streams abertos (`my_stream_connections`), e se a memória continua crescendo enquanto a carga está estável (um vazamento).

**Corrija** aumentando a memória do container e `GOMEMLIMIT` junto com ela, ou execute mais processos. Relate um crescimento constante que não acompanha a carga.

<!-- source: mydbpoolsaturated a737e35a86f5 -->
### MyDBPoolSaturated

Mais de 90% das conexões de banco de dados de um processo estão em uso. Logo as requisições esperarão por uma conexão (`MyDBPoolWaiting`).

**Verifique** se o tráfego cresceu ou se as consultas ficaram mais lentas (o log de consultas lentas do banco de dados, `my_adapter_operation_duration_seconds`).

Corrija:

- Aumente o tamanho do pool em `database_url` (para PostgreSQL, `pool_max_conns`), mantendo o total de todos os processos abaixo do limite de conexões do banco de dados.
- Coloque um pooler de conexões na frente do banco de dados se muitos processos o compartilham.

<!-- source: mydbpoolwaiting eacb1d75cd54 -->
### MyDBPoolWaiting

O trabalho espera por uma conexão livre com o banco de dados, ou desistiu de esperar após [`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (os timeouts existem só no PostgreSQL): as requisições respondem `503`, e os jobs são executados mais tarde. Siga [MyDBPoolSaturated](#mydbpoolsaturated);
verifique também se o próprio banco de dados não está no limite de conexões.

<!-- source: mydatabasestoragelarge edf767115918 -->
### MyDatabaseStorageLarge

O banco de dados ocupa mais do que você planejou. `my_db_size_bytes` é o tamanho do banco de dados PostgreSQL (`pg_database_size`, o que os limites de armazenamento dos provedores contam)
ou do arquivo SQLite (sem o write-ahead log). Isso é crescimento, não uma falha. Defina o limite na regra com uma margem abaixo do limite de armazenamento
do plano do seu banco de dados, ou do volume que contém o arquivo SQLite, que também precisa ter espaço para o write-ahead log e para os backups.

Corrija:

- Aumente o plano ou o volume.
- As publicações remotas são removidas após a configuração da instância `remote_status_retention_days`; reduzi-la desacelera o crescimento.
- As linhas excluídas liberam espaço que ambos os bancos de dados reaproveitam, mas o tamanho raramente diminui: o PostgreSQL mantém o espaço de suas tabelas, e o SQLite mantém as páginas livres
  no arquivo até um `VACUUM` (execute com `sqlite3` enquanto o servidor está parado).

<!-- source: myfiledescriptorsnearlimit 8a3112c11968 -->
### MyFileDescriptorsNearLimit

O processo tem mais de 80% de seu limite de descritores de arquivo abertos. Depois dele, novas conexões falham.

**Corrija** aumentando o limite (`ulimit -n`, o `nofile` do runtime de containers), ou execute mais processos. Verifique `my_stream_connections` e
`http_client_open_connections` para ver o que os segura.

<!-- source: myadaptererrors fcf1d91a0f01 -->
### MyAdapterErrors

Mais de 5% das operações de um adaptador falham: `kv.valkey` (Valkey), `kv.db` (a tabela KV), `blob.fs` (o diretório de mídia),
`queue.db` (a tabela da fila) ou um `stream.<backend>` (as notificações entre processos).

**Verifique** as linhas de erro desse adaptador e o serviço por trás dele: o Valkey no ar e alcançável, o volume de mídia montado, gravável e não cheio.

**Corrija** o serviço. As operações de job que falharam são repetidas; as notificações de stream que falharam apenas atrasam as atualizações ao vivo.

<!-- source: myremotemediacacheoverlimit dea5d39a7027 -->
### MyRemoteMediaCacheOverLimit

As cópias de mídia remota permanecem acima de `remote_media_cache_max_bytes`. A limpeza por hora normalmente remove as cópias expiradas e as menos usadas recentemente.
Defina o limite na regra com o seu valor de `remote_media_cache_max_bytes`.

**Verifique** a tela de jobs e os logs em busca de falhas dos jobs de limpeza, e `MyAdapterErrors` para o armazenamento de blobs.

**Corrija** a causa das falhas, ou reduza o tamanho do cache ou a política de mídia remota da instância.

<!-- source: mymediastoragelarge ff5e5906636c -->
### MyMediaStorageLarge

Os arquivos de mídia ocupam mais do que você planejou. Isso é crescimento, não uma falha. Defina o limite na regra com o que seu plano de armazenamento ou seu orçamento permite.

**Corrija** aumentando o plano ou o orçamento, ou reduza os limites de envio (`media_max_image_bytes`, `media_max_av_bytes`).

<!-- source: myemailfailing f7a0c64c5c60 -->
### MyEmailFailing

Jobs de e-mail falharam em definitivo na última hora: verificação, redefinição de senha, avisos de senha, notificações ou os e-mails de encerramento.
Os usuários que esperam por um e-mail de verificação ou de redefinição não conseguem concluir.

**Verifique** a tela de jobs filtrada pelos tipos de job de e-mail (eles terminam em `_email`) e os últimos erros. [Enviar e-mail](email.md#look-at-send-failures)
lista os erros e o que eles significam.

Corrija:

- Host, porta ou credenciais de SMTP incorretos, ou um problema de conta ou de permissão do SES: corrija-o e depois execute de novo os jobs com falha.
  Os e-mails de verificação e de redefinição mais antigos que a validade do link são concluídos sem envio quando repetidos, então peça a esses usuários que solicitem um novo.
- Endereços recusados em definitivo são suprimidos; nada a fazer.

<!-- source: myemailretrying 40d3ed1a76f5 -->
### MyEmailRetrying

A maioria dos envios de e-mail falha e espera por uma nova tentativa: o servidor de e-mail está inacessível, recusa o login ou limita a taxa. As tentativas duram cerca de 15 minutos para
os e-mails transacionais e cerca de 4 horas para as notificações antes de falharem em definitivo. Siga [MyEmailFailing](#myemailfailing) antes que isso aconteça.

<!-- source: cost-and-budget 5a63bdd2754d -->
### Custo e orçamento

O servidor não alerta sobre custo. Defina um orçamento com alertas no provedor, que vê as cobranças primeiro:

- AWS: um orçamento de custo do AWS Budgets com alertas, por exemplo, em 80% e 100% do valor mensal, filtrado pela tag de alocação de custos deste servidor.
- Google Cloud: um orçamento do Cloud Billing para o projeto, com limites de alerta em 50%, 90% e 100%. As notificações do mesmo orçamento podem alimentar a
  importação de custos ([Importar cobranças de uso de nuvem](billing-import.md)).

As cobranças importadas e a estimativa do mês até hoje aparecem em Administração → Custos → Importações de cobrança. As importações com falha são registradas ali e repetidas pela próxima importação diária;
[`MyBillingImportFailing`](#mybillingimportfailing) avisa você antes que os números do mês fiquem desatualizados.

<!-- source: mybillingimportfailing da997f400ac4 -->
### MyBillingImportFailing

A importação mais recente das cobranças de uso de uma fonte de cobrança falhou, e nenhuma importação posterior teve sucesso por 6 horas. As importações rodam uma vez por dia; limitação de taxa e
indisponibilidade são repetidas em poucas horas, e as outras falhas esperam a importação do dia seguinte, então a tela de custos mostra números mais antigos até lá.

Verifique:

- Administração → Custos → Importações de cobrança: a última importação da fonte e seu erro.
- `my_billing_imports_total{my_outcome="failed"}` por `error_type`, ou a linha de log `WARN` cujo `msg` é `billing import failed`, com `source` e
  `error_kind`. [Importar cobranças de uso de nuvem](billing-import.md) lista os tipos e como corrigir cada um.

Corrija:

- Corrija a causa (permissões, a exportação de cobrança, o limite de requisições) e então pressione “Importar agora” em Administração → Custos → Importações de cobrança em vez de esperar o dia seguinte.

Sem o Prometheus (Cloud Run com `metrics_exporter = none`), crie no lugar disso um alerta baseado em logs sobre a linha `billing import failed`.

<!-- source: telling-users-about-an-incident e8cb0ca13fe8 -->
## Avisar os usuários sobre um incidente

1. **Reconheça** o alerta para que os outros de plantão saibam que alguém cuida dele.
2. **Publique na página de status** (`status_page_url`, veja [Monitorar o servidor por fora](monitoring.md#a-status-page)) assim que os usuários forem afetados:
   o que está quebrado, desde quando e quando você publicará de novo. A página de status continua acessível quando este servidor não está, e o cliente web aponta para ela.
3. **Publique um anúncio** se os usuários ainda alcançam o servidor (degradado, não fora do ar): Administração → Anúncios, escreva o texto e publique.
   Os anúncios aparecem acima da timeline inicial de todos. Use no lugar disso o modo de manutenção com um `maintenance_message` se o servidor precisar parar.
4. **Atualize** os dois no horário que você prometeu, mesmo que nada tenha mudado.
5. **Encerre**: marque o incidente como resolvido na página de status, retire o anúncio (ou publique um acompanhamento curto) e registre o que aconteceu
   e o que você mudou enquanto está fresco na memória.
