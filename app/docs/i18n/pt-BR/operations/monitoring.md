<!-- source: monitoring-the-server-from-outside 6cd7b2b809fa -->
# Monitorando o servidor de fora

Como apontar um monitor de disponibilidade para o servidor, como evitar que o monitoramento aumente o custo de execução e como distinguir o modo de manutenção ([Manutenção](maintenance.md)) de uma indisponibilidade. Os caminhos das sondas estão em [a seção de verificação de saúde do guia de execução](running.md#health-checks).

Os fatos de terceiros abaixo trazem a data em que foram confirmados e a fonte. Os serviços mudam seus planos e suas telas; confira a página vinculada antes de confiar em um limite.

<!-- source: which-url-to-monitor 50bdaa5c9f31 -->
## Qual URL monitorar

| Caminho | Para | Acessa o banco de dados | No modo de manutenção |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | Monitores de disponibilidade, páginas de status, o cliente web | No máximo uma vez a cada 60 s por processo | `503` com `Retry-After` e `{"status":"maintenance"}` |
| `GET /healthz` | A sonda de liveness da plataforma | Nunca | `200` `{"status":"maintenance"}` |
| `GET /readyz` | A sonda de readiness do load balancer ou do Kubernetes | A cada requisição (requisições concorrentes compartilham uma verificação) | `200` `{"status":"maintenance"}` |

Aponte o monitor externo para **`/api/v1/instance/health`**, não para `/readyz`: `/readyz` consulta o banco de dados a cada requisição, então um monitor com intervalo curto mantém acordado um banco de dados que para quando fica ocioso. As sondas são para a sua plataforma. Os módulos [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) e [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) verificam `/readyz` a partir do monitor próprio da plataforma, em um intervalo longo, e os READMEs deles dizem o que isso custa.

Respostas de `/api/v1/instance/health` (sem autenticação, `Cache-Control: no-store`):

| Estado | HTTP | Corpo |
| --- | --- | --- |
| Saudável | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| Indisponível (o banco de dados está inacessível ou o schema não foi aplicado) | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| Modo de manutenção | `503` com `Retry-After` | `{"status":"maintenance"}` |

`/healthz` e `/readyz` respondem `200` com `{"status":"ok"}` quando saudáveis, e `/readyz` responde `503` com um documento de problema quando o banco de dados não está pronto.
O corpo nunca traz motivo, nome de host ou versão. A causa está no log do servidor.

`checked_at` é o horário da verificação em que a resposta se baseia. O servidor executa a verificação do banco de dados no máximo uma vez por minuto por processo e reutiliza o resultado, então um monitor que consulta com mais frequência que a cada 60 s vê o mesmo `checked_at` e não ganha nada. Com vários processos, cada um executa a sua própria verificação.

Com [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) definido, esses três caminhos continuam respondendo sem o cabeçalho, então não mostram se o CDN perdeu o cabeçalho. Monitore uma página também pelo CDN, como `https://<host>/about`, se um monitor for o seu primeiro sinal de problema.

<!-- source: keep-the-monitoring-cheap 29d1da3d6dc2 -->
## Mantenha o monitoramento barato

- **Bancos de dados que param quando ociosos.** Se o intervalo do monitor for menor que o tempo que o banco de dados espera antes de parar, o banco nunca para.
  O Neon suspende um compute após 5 minutos sem atividade; o plano Free não permite desativar isso (confirmado em 2026-10-25, [Neon: Scale to zero](https://neon.com/docs/introduction/scale-to-zero)).
  Como regra prática, um intervalo de 5 minutos ou mais permite que um banco ocioso pare entre as verificações, e cada verificação o acorda pelo tempo da verificação mais o tempo ocioso.
  Um servidor federado recebe entregas de outros servidores o tempo todo, então o banco fica acordado grande parte do dia de qualquer forma; o custo extra se limita às horas silenciosas.
- **Uma divisão mais barata se o custo ocioso importar mais.** Monitore `/healthz` (ele nunca acessa o banco de dados) em um intervalo curto, e `/api/v1/instance/health`, que acessa o banco, em um intervalo longo (15 minutos ou mais). `/healthz` responde `200` também no modo de manutenção, então verifique `"status":"ok"` no corpo.
- **As requisições em si são pequenas.** Uma verificação a cada 5 minutos são cerca de 8.640 requisições por mês, e uma a cada minuto cerca de 43.200. Isso cabe no nível gratuito do Cloud Run (2 milhões de requisições, 180.000 vCPU-segundos e 360.000 GiB-segundos por mês; tempo arredondado para cima em 100 ms; confirmado em 2026-10-23, [preços do Cloud Run](https://cloud.google.com/run/pricing)).

<!-- source: telling-the-maintenance-mode-from-an-outage 15833e5da3fc -->
## Distinguindo o modo de manutenção de uma indisponibilidade

Durante o modo de manutenção ([guia de manutenção](maintenance.md)), o endpoint de saúde responde `503`, o que a maioria dos monitores trata como uma indisponibilidade.
O código de status sozinho não consegue distinguir os dois casos, então decida pelo corpo:

- Trate `"status":"ok"` no corpo como saudável, e qualquer outra coisa como fora do ar. Um monitor de palavra-chave ou de valor JSON faz isso.
- Para evitar alertas durante uma manutenção planejada, defina a janela de manutenção do monitor, se o serviço tiver uma (veja a tabela abaixo), ou pause o monitor manualmente antes de começar.
- O plano Free do UptimeRobot não tem janela de manutenção: pause o monitor ou aceite o alerta.

<!-- source: setting-up-the-monitors e91307d78a94 -->
## Configurando os monitores

Todos eles verificam a mesma coisa: `GET https://<your-host>/api/v1/instance/health` está saudável quando a resposta é `200` e o corpo contém `"status":"ok"`.

| Serviço | Tipo de monitor | Saudável quando | Janela de manutenção | Confirmado |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | O corpo contém `"status":"ok"` | Não no Free (Solo e acima) | 2026-10-25, [preços](https://uptimerobot.com/pricing.md) |
| Better Stack | Status HTTP, ou Keyword | `2xx`, ou o corpo contém `"status":"ok"` | Não verificada | 2026-10-25 |
| Gatus | Endpoint conditions | `[STATUS] == 200` e `[BODY].status == ok` | `maintenance` na configuração | 2026-10-25, [README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword, ou JSON Query | O corpo contém `"status":"ok"`, ou `status` é `ok` | Integrada | 2026-10-23, [README](https://github.com/louislam/uptime-kuma) |
| Upptime | `sites` em `.upptimerc.yml` | `200` e o corpo contém `"status":"ok"` | Nenhuma | 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/) |

<!-- source: uptimerobot b2350308dfab -->
### UptimeRobot

Crie um monitor do tipo **Keyword**: URL `https://<your-host>/api/v1/instance/health`, palavra-chave `"status":"ok"`, e alerte quando a palavra-chave **não** existir.
O plano Free permite 50 monitores com intervalo de 5 minutos, e inclui monitores de palavra-chave.
"Custom headers & statuses" e janelas de manutenção começam no plano Solo (pago, intervalo de 60 segundos)
(confirmado em 2026-10-25, [preços](https://uptimerobot.com/pricing.md)). Um monitor de palavra-chave basta aqui, porque a verificação é feita no corpo.

<!-- source: better-stack 68d7ade20dca -->
### Better Stack

Duas formas funcionam (confirmado em 2026-10-25, [Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/), [Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)):

- Um monitor de **status HTTP**: ele trata uma resposta `2xx` como ativa, então `200` está ativo e `503` (indisponível ou em manutenção) abre um incidente.
- Um monitor de **Keyword**: "Alert us when" é "URL doesn't contain keyword", com a palavra-chave `"status":"ok"`. A correspondência não diferencia maiúsculas de minúsculas.

A frequência de verificação e as janelas de manutenção do plano Free não foram verificadas: as páginas oficiais não as informam para o plano Free. Os limites do nível gratuito (10 monitores e heartbeats juntos, uma página de status) vêm da [página de preços](https://betterstack.com/pricing), confirmados em 2026-10-23.

<!-- source: gatus 2aa06e9f2812 -->
### Gatus

Em `config.yaml` (intervalo padrão `60s`; confirmado em 2026-10-25, [README](https://github.com/TwiN/gatus)):

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

O Gatus tem uma seção `maintenance` no nível superior (`start` como `hh:mm`, `duration`, `timezone`, que é `UTC` por padrão, e `every` como uma lista de dias da semana, em que vazio significa todos os dias) para trabalhos planejados. O README só diz que ela serve para evitar alertas durante janelas de manutenção; verifique o comportamento na sua versão.

<!-- source: uptime-kuma 7dd44fb38cab -->
### Uptime Kuma

Adicione um monitor do tipo **HTTP(s) - Keyword** com a palavra-chave `"status":"ok"`, ou **HTTP(s) - Json Query** com a consulta `status` e o valor esperado `ok`.
Os códigos de status aceitos são `200-299` por padrão, então a `503` de manutenção e a `503` de indisponibilidade ficam ambas como inativas. O Uptime Kuma tem um recurso de manutenção para janelas de monitores ([wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) o lista; os detalhes não foram verificados).
Os tipos de monitor foram confirmados em 2026-10-23 no código-fonte ([monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720),
[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)).

<!-- source: upptime 90c96c946d06 -->
### Upptime

No `.upptimerc.yml` (confirmado em 2026-10-25, [Configuration](https://upptime.js.org/docs/configuration/)):

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

Por padrão, `20x` e `30x` contam como ativos; defina `expectedStatusCodes: [200]` para aceitar apenas `200`. A verificação roda no GitHub Actions em um agendamento definido para todos os sites (`workflowSchedule.uptime`, padrão a cada 5 minutos, que também é o mínimo que o GitHub Actions permite). O nome da opção diz que ela não é recomendada, mas é o caminho para testar o corpo.

<!-- source: a-status-page 64712ed6d8a5 -->
## Uma página de status

Defina `status_page_url` (`MY_STATUS_PAGE_URL`, uma URL `https:`; veja a [referência de configuração](../reference/configuration.md#status_page_url)) como a página pública do seu monitor. O servidor mostra o link em `GET /api/v1/instance`, nas páginas de login e de erro que ele renderiza, no cliente web (também quando o servidor não responde) e na página de manutenção, para que um usuário que não consegue alcançar o servidor descubra o motivo.
Ele é lido da configuração, não do banco de dados, então funciona quando o banco está fora do ar. Uma mudança passa a valer após um reinício.

Opções para a própria página (este servidor não importa o estado de uma página de status, então publique também os anúncios como um aviso para todo o servidor):

- A página de status hospedada do serviço de monitoramento que você escolheu (o plano Free do UptimeRobot tem uma página de status sem personalização; o nível gratuito do Better Stack tem uma), as páginas que o Gatus e o Uptime Kuma servem por conta própria, ou o site estático do Upptime no GitHub Pages.
- Atlassian Statuspage, que é um lugar para publicar, e não um monitor. Um componente tem um entre cinco estados: Operational, Under maintenance, Degraded performance, Partial outage, Major outage (valores da API `operational`, `under_maintenance`, `degraded_performance`, `partial_outage`, `major_outage`; confirmado em 2026-10-23, [What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/), [Statuspage API](https://developer.statuspage.io/)).
  Não está verificado como mudar o estado de um componente automaticamente a partir do alerta de um monitor (isso exige uma integração por serviço configurada em uma conta real).
