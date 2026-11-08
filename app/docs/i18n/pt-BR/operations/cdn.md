<!-- source: put-a-cdn-in-front-of-media-delivery 20ee3d2cefe9 -->
# Colocar uma CDN na frente da entrega de mídia

Uma CDN na frente do servidor (Cloudflare, Google Cloud CDN, CloudFront e assim por diante) pode entregar a mídia (`/media/` e `/media/proxy/`).
As requisições respondidas pelo cache da CDN não usam nenhuma CPU nem transferência do servidor.
Este guia trata da configuração em que o servidor é a origem da CDN (origin pull). Ela vale para todo armazenamento de blobs: com `blob = s3` ou `blob = gcs`, o servidor lê os arquivos do bucket privado e os serve em `/media/` como com `blob = fs`, então o bucket nunca é a origem da CDN.
Quando o servidor inteiro fica atrás da CDN, ele também pode [recusar as requisições que não passaram por ela](#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: configure-it e915ce01da94 -->
## Configurar

Defina `media_origin` (`MY_MEDIA_ORIGIN`) como o nome do host da CDN (por exemplo, `media.example.org`).

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- O valor é somente uma origem (`scheme://host[:port]`), sem caminho, query nem barra final. `https` é obrigatório, e `http` só é permitido para hosts de loopback, como `localhost`. Um valor inválido faz a inicialização falhar.
- Se não estiver definido, a mídia é servida pela própria origem do servidor (`https://{host}`).
- Estas URLs usam essa origem: `{media_origin}/media/...` para a mídia local e `{media_origin}/media/proxy/...` para o proxy da mídia remota.
  Todas as URLs emitidas pela API cliente, pelo ActivityPub (`attachment`, `icon`, `image` e o `icon` de emojis personalizados) e pelas prévias de links (OGP) são cobertas.
- A Content-Security-Policy do cliente web adiciona essa origem a `img-src` e `media-src`.
- URIs federadas, como o `id` de um objeto ActivityPub, continuam em `https://{host}/` qualquer que seja o valor de `media_origin`. Só muda o lugar onde os arquivos são servidos, não os identificadores.
- As URLs são montadas a cada leitura e não são armazenadas no banco de dados, então mudar `media_origin` também muda as URLs da mídia existente.
  As URLs antigas que outros servidores guardaram continuam funcionando enquanto `/media/` na origem antiga continuar sendo servido.

<!-- source: point-the-cdn-at-the-server ffb279f38f08 -->
## Apontar a CDN para o servidor

- Faça da origem (backend) o nome de host público do servidor (`https://{host}`). A CDN precisa buscar no servidor apenas caminhos em `/media/`.
- O servidor não restringe `/media/` e `/media/proxy/` pelo nome do host. Ele serve qualquer `Host` que a CDN enviar.
- `/media/` não exige autenticação, e o Authorized Fetch não se aplica a ele. O servidor responde às requisições não assinadas vindas da CDN como são.
- Para a mídia remota (`/media/proxy/{id}/{variant}?sig=...`), `sig` faz parte da URL. **Inclua a query string na chave de cache.** Caso contrário, URLs com assinaturas diferentes compartilham uma mesma entrada.
- Deixe o `Cache-Control` da origem decidir por quanto tempo as coisas ficam em cache (não o sobrescreva na CDN).

| Caminho | `Cache-Control` |
| --- | --- |
| `/media/{...}` (mídia local) | `public, max-age=31536000, immutable` (um ano) |
| `/media/proxy/{...}` (mídia remota) | `public, max-age=` o número de segundos da configuração da instância `remote_media_cache_ttl_seconds` |

As duas respostas saem com `Cross-Origin-Resource-Policy: cross-origin`, para que páginas de outras origens possam lê-las.

No Cloud Run, `ingress-cloudflare-media` configura isso com OpenTofu: veja [Implantando no Cloud Run](deploy-cloud-run.md#cloudflare).

<!-- source: who-can-open-media-urls 27306b0bc9be -->
## Quem pode abrir URLs de mídia

As URLs de mídia não têm controle de acesso. Qualquer pessoa que tenha a URL de um arquivo pode abri-lo, sem entrar, qualquer que seja a visibilidade da publicação que o anexa.
Isso vale com ou sem CDN, e inclui os anexos de publicações só para seguidores e de mensagens diretas e as URLs de proxy da mídia remota.

- As URLs são difíceis de adivinhar: a URL de um arquivo local contém identificadores com uma parte aleatória, e uma URL de proxy carrega uma assinatura. Conhecer uma URL não revela as outras.
- Quem viu uma publicação pode repassar as URLs de mídia dela, e as URLs continuam funcionando para outras pessoas.
- As respostas são marcadas como `public`, então a CDN e qualquer cache compartilhado entre o servidor e quem lê podem manter uma cópia (veja a tabela acima para saber por quanto tempo).
  É isso que permite à CDN servir a mídia sem consultar o servidor.

Avise seus usuários de que uma publicação não pública limita quem vê a publicação, mas não impede que alguém que tenha o link de um arquivo anexado abra o arquivo.

<!-- source: deletion-and-the-cache baac405df923 -->
## Exclusão e cache

Depois que você exclui uma mídia, o cache da CDN a mantém pelo `max-age`. Um arquivo excluído pode continuar visível por até um ano, no caso da mídia local, e por até `remote_media_cache_ttl_seconds`, no caso do proxy.
O servidor não limpa o cache da CDN. Se algo precisar desaparecer logo após uma exclusão ou uma ação de moderação, limpe-o manualmente no console ou na API da CDN.

<!-- source: service-worker f6efdae181a5 -->
## Service Worker

O Service Worker do cliente web não armazena em cache a mídia de outra origem e deixa isso para o tratamento de rede do navegador, então só vale o cache da CDN.

<!-- source: refuse-requests-that-did-not-come-through-the-cdn 5df4225c62b3 -->
## Recusar requisições que não passaram pela CDN

Uma CDN protege o servidor apenas das requisições que passam por ela. Quando a origem também é alcançável pelo próprio endereço, como `*.run.app` no Cloud Run com tráfego da internet, qualquer pessoa pode chamá-la diretamente, sem passar pelo WAF nem pelo cache.
Se os endereços da CDN estiverem em `trusted_proxies`, um chamador na própria plataforma serverless da CDN (um Cloudflare Worker seu, por exemplo) também se conecta a partir de um endereço confiável e pode escolher o IP do cliente que os limites de taxa veem.

As três configurações abaixo fecham essa brecha. A CDN adiciona um cabeçalho secreto a cada requisição que encaminha, e o servidor responde `403` a qualquer requisição sem ele, antes de fazer qualquer outra coisa com a requisição.

![Os caminhos das requisições com a verificação de origem ativa: navegadores, apps e outros servidores passam pela CDN, que adiciona o cabeçalho secreto; a plataforma e os demais chegam diretamente ao servidor, sem ele; a verificação de origem do servidor deixa passar o cabeçalho e as requisições isentas, e recusa o resto com 403. A lista abaixo descreve cada caminho.](images/cdn-request-flow.svg)

- Navegadores e apps (páginas, a API cliente, OAuth e URLs de mídia) e outros servidores (entregas e buscas do ActivityPub)
  chegam à CDN em `host` e no host de `media_origin`. A CDN responde sozinha aos arquivos de `/media/` que estão em cache, e define
  `origin_secret_header` com o valor de `origin_secret` em cada requisição que encaminha ao servidor.
- A plataforma (sondas de saúde, monitores de disponibilidade, Cloud Tasks e Cloud Scheduler) e os demais, como um script ou um webhook seu, chegam diretamente ao endereço do próprio servidor, sem o cabeçalho.
- A verificação de origem roda primeiro no servidor. Ela deixa passar uma requisição que traga o cabeçalho com `origin_secret` ou `origin_secret_previous`, e, sem o cabeçalho, apenas `GET` e `HEAD` das verificações de saúde e `POST` para `/_internal/`.
  Ela remove o cabeçalho de toda requisição e recusa o resto com `403`.
- Uma requisição que passa chega ao cliente web, à API cliente, ao OAuth, ao ActivityPub, às verificações de saúde ou a `/_internal/`,
  ou a `/media/` e `/media/proxy/`, que leem o armazenamento de mídia (`blob = fs`, `s3` ou `gcs`) ou repassam o arquivo de
  outro servidor. Uma requisição recusada recebe `403` com `Cache-Control: no-store` e soma em `my.http.origin_refused`.

Use isso quando o servidor tiver um endereço público que a plataforma não consegue fechar para todos, exceto a CDN. Você não precisa disso quando um firewall ou a própria plataforma já limita quem pode se conectar, por exemplo um servidor que escuta apenas em uma rede privada atrás do seu load balancer.
Também não quando esse load balancer verifica ele mesmo o cabeçalho da CDN e admite apenas os endereços da CDN, como um Application Load Balancer da AWS atrás do CloudFront pode fazer com uma regra de listener e a lista de prefixos gerenciada `com.amazonaws.global.cloudfront.origin-facing`. Nesse caso, inclua os endereços do load balancer e os da CDN em `trusted_proxies`, já que cada um acrescenta um valor a `X-Forwarded-For`.

Não ajuda quando o próprio recurso da plataforma encaminha o tráfego e não acrescenta um cabeçalho que você controle, como um domain mapping do Cloud Run. Só uma CDN ou um load balancer que consiga adicionar um cabeçalho de requisição resolve.

| Configuração | O que guarda |
| --- | --- |
| [`origin_secret`](../reference/configuration.md#origin_secret) (`MY_ORIGIN_SECRET`) | O valor, com pelo menos 32 bytes de ASCII visível. Um segredo, lido do ambiente ou de `MY_ORIGIN_SECRET_FILE` |
| [`origin_secret_header`](../reference/configuration.md#origin_secret_header) (`MY_ORIGIN_SECRET_HEADER`) | O nome do cabeçalho, escolhido por você. Obrigatório com `origin_secret`. Nomes que uma CDN se recusa a definir ou que o servidor ou um proxy lê, como os que começam com `cf-` ou `x-cf-`, são inválidos |
| [`origin_secret_previous`](../reference/configuration.md#origin_secret_previous) (`MY_ORIGIN_SECRET_PREVIOUS`) | Um segundo valor aceito, somente [durante uma rotação](#rotate-the-value) |

Sem nenhuma delas definida, nada é verificado e o servidor se comporta como se esta seção não existisse.

<!-- source: what-passes-without-the-header 38e1ceda6e34 -->
### O que passa sem o cabeçalho

| Requisição | Motivo |
| --- | --- |
| `GET` ou `HEAD` de `/healthz`, `/readyz`, `/api/v1/instance/health` | As sondas da plataforma e os monitores de disponibilidade as chamam diretamente |
| `POST` para caminhos `/_internal/` | Cloud Tasks e Cloud Scheduler as chamam na URL do próprio servidor. Elas trazem o próprio token assinado |

Todo o resto exige o cabeçalho, também no modo de manutenção: o cliente web e os seus recursos, `/media/`, a API cliente, o OAuth e o ActivityPub, inclusive as entregas de outros servidores.
Assim, qualquer coisa que chame o servidor diretamente pelo próprio endereço, em vez de passar pela CDN, é recusada: um webhook seu, um servidor de mídia de voz, um script. Aponte-a para o nome de host da CDN.

A porta de métricas (`metrics_listen`) é um listener separado, que não é público e não é verificado.

Uma requisição que não traz o segredo não tem influência no restante do processamento, mesmo quando passa: o IP do cliente dela é o do par da conexão, qualquer que seja o valor de `X-Forwarded-For`, e o `traceparent` e o `X-Request-Id` dela são ignorados.

<!-- source: set-it-up ee0a17ad2d33 -->
### Configurar

Todo nome de host que a CDN encaminha ao servidor precisa do cabeçalho: `host`, o host de `media_origin` e um `account_domain` separado, se a CDN o servir. Um host de mídia atendido pela mesma CDN também o define, já que `/media/` é recusado como todo o resto.

1. Gere o valor e escolha um nome que nenhuma outra coisa use:

   ```sh
   openssl rand -hex 32
   ```

2. **Defina o cabeçalho na CDN primeiro.** O servidor ignora um cabeçalho que não conhece, então este passo ainda não muda nada.
   - **Cloudflare**: uma regra de transformação de cabeçalho de requisição (Rules, depois Transform Rules, depois Modify Request Header), em todos os planos. Associe-a a todas as requisições de entrada da zona e escolha **Set static** com o seu nome e o seu valor. Ela substitui um valor enviado pelo cliente. Uma regra só para alguns nomes de host deixa os outros trancados para fora.
   - **Amazon CloudFront**: um cabeçalho personalizado de origem na origem de cada distribuição. Ele também substitui um valor enviado pelo visualizador.
   - **Load balancers do Google Cloud**: um cabeçalho personalizado de requisição no serviço de backend.
   - Qualquer outra CDN ou proxy: o cabeçalho precisa ser definido, não acrescentado, em toda requisição ao servidor.
3. Defina `MY_ORIGIN_SECRET` e `MY_ORIGIN_SECRET_HEADER` e implante. `my config check` valida essas configurações e repete o que é verificado, mas não consegue enxergar a CDN.
4. Verifique de fora, com o endereço da própria origem:

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                 # 200: through the CDN
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/       # 403: directly
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/healthz # 200: a health check
   ```

Inclua também os endereços da CDN em [`trusted_proxies`](../reference/configuration.md#trusted_proxies); do contrário, o servidor vê um único cliente e todas as requisições compartilham os limites de taxa da CDN. `my config check` avisa sobre isso.

Uma requisição recusada recebe o mesmo problema `403` de qualquer requisição proibida, com `Cache-Control: no-store`, qualquer que seja o modo como o cabeçalho estava errado. Nada na resposta menciona o cabeçalho.

<!-- source: when-everyone-is-locked-out 47c8f706e5e7 -->
### Quando todos ficam trancados para fora

Uma configuração errada recusa todas as requisições enquanto as verificações de saúde continuam com `200`, então a plataforma vê um servidor saudável, e um monitor de disponibilidade de `/healthz` não percebe nada. Os sinais são:

- `403` em todas as páginas acessadas pela CDN.
- A métrica `my.http.origin_refused` subindo enquanto as requisições de `http.server.request.duration` param.
- Linhas `http request` no log com o status `403` e sem `http.route`. O log e os traces nunca contêm o cabeçalho, o nome dele nem o valor. Uma requisição recusada não cria nenhum span de trace.

Para se recuperar, faça um deploy sem a verificação: deixe `MY_ORIGIN_SECRET` e `MY_ORIGIN_SECRET_HEADER` sem definir. Os outros servidores repetem as entregas deles, mas as pessoas não conseguiram alcançar o servidor nesse meio-tempo.
Depois, compare o nome e o valor na CDN com as configurações, verifique se a regra cobre todos os nomes de host e defina-os novamente.

<!-- source: rotate-the-value b88f695fde31 -->
### Rotacionar o valor

A CDN e o servidor não podem mudar ao mesmo tempo, e cada réplica do servidor reinicia no seu próprio ritmo; com um único valor aceito, uma mudança recusaria o tráfego enquanto eles forem diferentes. Por isso, o servidor aceita dois valores durante uma rotação:

1. Defina `MY_ORIGIN_SECRET` com o novo valor e `MY_ORIGIN_SECRET_PREVIOUS` com o antigo, e implante. Os dois valores passam.
2. Troque o valor na CDN e aguarde até que a mudança chegue a todas as suas localidades.
3. Remova `MY_ORIGIN_SECRET_PREVIOUS` e implante. `my config check` avisa enquanto ela estiver definida.

O nome do cabeçalho não rotaciona dessa forma. Para mudá-lo, repita [a configuração](#set-it-up) com a verificação do servidor desligada por um momento, ou defina o novo nome na CDN junto com o antigo antes de implantá-lo.
