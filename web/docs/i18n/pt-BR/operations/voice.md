<!-- source: run-voice-chat f44427fbbe76 -->
# Executar o chat de voz

O chat de voz permite que usuários conectados abram uma sala, convidem outras pessoas e conversem. Ele fica desativado até você escolher como o áudio trafega.
Este guia explica essa escolha, os servidores STUN e TURN ou os nós de mídia que cada opção exige, e o que saber sobre implantação, privacidade e moderação.
Cada configuração citada aqui é descrita na [referência de configuração](../reference/configuration.md#voice).

<!-- source: choose-how-the-audio-travels 2ef72c636c49 -->
## Escolha como o áudio trafega

`voice` escolhe um único método para a instância inteira; todas as salas o usam.

| `voice` | O que transporta o áudio | Indicado para |
| --- | --- | --- |
| `off` | Nada. A API do chat de voz responde `404`, o cliente web não mostra o chat de voz e as outras configurações `voice_*` não são verificadas na inicialização. | O padrão. |
| `p2p` | Os navegadores dos participantes enviam o áudio diretamente uns aos outros. O servidor retransmite apenas as mensagens que estabelecem as conexões. | Salas pequenas e a configuração mais barata: nenhum servidor extra, exceto um servidor STUN ou TURN opcional. |
| `sfu` | Os nós de mídia do próprio servidor: cada navegador envia seu áudio ao nó que abriga sua sala, e esse nó encaminha o áudio de quem fala aos participantes. | Salas maiores e controle rigoroso sobre quem pode falar. |

![Os dois métodos lado a lado. Com p2p, o servidor retransmite os sinais e os navegadores enviam o áudio uns aos outros, ou por meio de um servidor TURN. Com sfu, os navegadores recebem suas concessões do servidor e enviam sinais e áudio ao nó de mídia da sua sala. A lista abaixo descreve cada método.](images/voice-methods.svg)

Em todos os métodos, o servidor (`my serve`) guarda as salas, quem participa delas e suas sessões, e entrega a cada
participante uma concessão: o que o navegador precisa para se conectar. O que muda é por onde passam a sinalização
(as mensagens que estabelecem uma conexão) e o áudio:

- **`p2p`.** Um navegador envia seus sinais (offers, answers e candidatos ICE) ao servidor, que os repassa ao
  outro navegador pelo fluxo de eventos da sala; um sinal espera no armazenamento chave-valor por no máximo 30 segundos. O áudio
  vai diretamente de navegador para navegador. A concessão lista os servidores STUN de `voice_stun_urls` e o servidor TURN de
  `voice_turn` (`coturn` ou `cloudflare`) com credenciais de curta duração. Um navegador descobre seu endereço público pelo STUN,
  e o servidor TURN retransmite o áudio quando os dois navegadores não encontram um caminho direto.
- **`sfu`.** O servidor coloca cada sala em um nó de mídia (uma unidade de encaminhamento seletivo, ou SFU): um processo com
  `role = all` ou `sfu`. A concessão traz a URL `wss:` desse nó (`voice_sfu_public_url`) e um tíquete de uso único.
  O navegador envia seus sinais por um WebSocket ao nó e seu áudio à porta UDP do nó `voice_sfu_udp_port`
  (a porta TCP `voice_sfu_tcp_port` quando o UDP está bloqueado), e o nó encaminha o áudio de quem fala aos demais
  participantes. O servidor TURN de `voice_turn` retransmite para os navegadores que não conseguem alcançar o nó diretamente.

Observações sobre a escolha:

- **`p2p`** não precisa de nenhuma porta UDP no servidor: o áudio nunca passa por ele (passa pelo servidor TURN quando um é usado), e a carga do servidor se resume às mensagens da sala, aos heartbeats e aos fluxos. Serve para uma instância pequena, para salas pequenas e para uma implantação só no Cloud Run.
- **`sfu`** garante no servidor quem pode falar: o nó encaminha apenas o áudio de quem fala. Os navegadores alcançam apenas o nó de mídia, então os endereços IP dos participantes ficam visíveis ao nó, não aos outros participantes. Exige uma máquina com IP público e portas UDP e TCP abertas para cada nó ([Nós de mídia](#media-nodes)). `voice_stun_urls` não é necessário, pois um nó tem um endereço público, e `voice_ice_transport_policy = relay` é recusado.
- Sirva a instância por HTTPS: os navegadores só permitem acesso ao microfone em páginas seguras.

Os limites mantêm o custo das salas sob controle. `0` remove qualquer um deles, exceto o último:

| Configuração | Padrão | Significado |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | O máximo de tempo que uma sala fica aberta. Uma sala que o atinge é encerrada. |
| `voice_max_open_rooms` | `10` | Quantas salas podem estar abertas ao mesmo tempo neste servidor. Uma sala aberta para um horário agendado conta desde a abertura, enquanto espera pelo primeiro participante. |
| `voice_max_participants` | `10` | A maior sala, de 2 a 500 (50 com `p2p`). Quem hospeda uma sala pode definir um máximo menor para ela. |

Quem pode abrir salas é definido pela configuração de instância `voice_room_starters` ([configurações da instância](instance-settings.md)): por padrão, apenas quem tem a permissão `start_voice_rooms` (a função integrada `admin` a tem); `everyone` permite que todo usuário local conectado que não esteja suspenso nem limitado abra salas.

<!-- source: peer-to-peer-rooms 2199ca9a427c -->
## Salas ponto a ponto

<!-- source: try-it-with-docker-compose ac3681681768 -->
### Experimente com o Docker Compose

Para experimentar o chat de voz na sua máquina com a configuração do Docker Compose ([`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)),
defina `MY_VOICE=p2p` em `.env` e recrie o servidor.

```sh
cd deploy/docker
docker compose up -d
```

- Abra a instância em `http://localhost:8080` (o `MY_WEB_ORIGIN` de `example.env`). Os navegadores só deixam uma página usar o microfone em
  `localhost` ou por HTTPS, então um endereço como `http://192.168.1.10:8080` não permite falar.
- Navegadores na mesma máquina (dois perfis de navegador, ou uma janela normal e uma privada) se conectam entre si sem um servidor STUN ou TURN.
  Para alcançar navegadores em outras máquinas, coloque a instância atrás de HTTPS e defina `MY_VOICE_STUN_URLS` em `.env`.
- Por padrão, apenas a função `admin` pode abrir salas. Entre como o administrador que você criou, ou defina `voice_room_starters` como `everyone`
  na página “Chat de voz” das configurações do servidor para que todos os usuários possam abrir salas.

<!-- source: size-and-bandwidth e54157c29322 -->
### Tamanho e largura de banda

Dois navegadores se conectam quando pelo menos um deles está falando; duas pessoas que apenas ouvem não se conectam.
Um falante, portanto, envia uma cópia separada do seu áudio a cada um dos outros participantes. A fala em Opus ocupa aproximadamente de 30 a 50 kbit/s com o overhead, então um falante em uma sala de 10 envia cerca de 9 cópias, algumas centenas de kbit/s,
e um falante em uma sala de 50 envia 49 cópias, cerca de 2 Mbit/s, o que um celular com dados móveis ou uma conexão residencial pode não sustentar. O número de conexões em uma sala de N pessoas é no máximo N(N-1)/2: 45 para 10 e 1.225 para 50.
Por isso o padrão é 10. Aumente `voice_max_participants` apenas quando seus usuários tiverem largura de banda de envio; use `sfu` para salas grandes.

<!-- source: stun-and-turn 51e427976a77 -->
### STUN e TURN

Um navegador atrás de NAT precisa de um servidor STUN para descobrir seu endereço público e de um servidor TURN para retransmitir o áudio quando não existe um caminho direto (NATs restritivos, firewalls corporativos, algumas redes móveis).

- `voice_stun_urls`: URLs `stun:` ou `stuns:` separadas por vírgula. Qualquer servidor STUN público funciona, e o servidor coturn abaixo também responde STUN.
- `voice_turn`: `none`, `coturn` ou `cloudflare`.

Sem nenhum dos dois, o servidor avisa na inicialização: apenas navegadores na mesma rede se conectam. Uma instância pública precisa de STUN, e de TURN para os usuários que não conseguem se conectar diretamente.

O servidor entrega a cada participante credenciais TURN de curta duração, nunca uma senha compartilhada. `voice_turn_credential_ttl` (padrão `6h`) define quanto tempo elas duram. O cliente web pede novas credenciais dez minutos antes de expirarem e reinicia o ICE, então uma chamada não é interrompida quando elas acabam.

<!-- source: coturn d665e942c91b -->
#### coturn

Defina `voice_turn = coturn`, `voice_turn_coturn_urls` e `voice_turn_coturn_secret`. O servidor calcula as credenciais por conta própria (o nome de usuário é o horário de expiração e o ID da sessão, a senha é um HMAC-SHA1 dele com o segredo), então não faz nenhuma requisição ao servidor TURN.
Configure o coturn de forma correspondente, em `turnserver.conf`:

```conf
# Time-limited credentials signed with a shared secret.
use-auth-secret
static-auth-secret=<the same value as voice_turn_coturn_secret>
realm=turn.example.org

listening-port=3478
tls-listening-port=5349
# If the host is behind NAT (most cloud VMs), tell coturn its public address.
external-ip=203.0.113.10

# Do not let the relay reach your internal networks.
no-multicast-peers
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=100.64.0.0-100.127.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
```

- `denied-peer-ip` é importante. Sem ele, qualquer pessoa com uma credencial pode usar o servidor TURN para alcançar hosts da rede interna dele (serviços de metadados da nuvem, bancos de dados, outras VMs). Negue toda faixa privada, de loopback e de link-local; permita um endereço interno (`allowed-peer-ip`) apenas se você realmente precisar retransmitir para ele.
- Abra UDP e TCP 3478, TCP 5349 para `turns:` e a faixa de portas de retransmissão do coturn (`min-port` / `max-port`, por padrão 49152-65535).
- Liste também uma URL `turns:` ou uma com `?transport=tcp` em `voice_turn_coturn_urls`, para que usuários em redes que bloqueiam UDP ainda consigam se conectar.
- O coturn verifica a expiração de uma credencial apenas quando uma alocação é criada pela primeira vez. Uma alocação que já existe continua funcionando depois dela, e as novas credenciais valem para a próxima alocação, que o cliente web cria reiniciando o ICE.

<!-- source: cloudflare-turn be5846573be8 -->
#### Cloudflare TURN

Defina `voice_turn = cloudflare`, `voice_turn_cloudflare_key_id` e `voice_turn_cloudflare_api_token`, a partir de uma chave TURN criada no painel da Cloudflare. O servidor pede à Cloudflare as credenciais de cada participante quando ele abre ou entra em uma sala (se isso falhar, a requisição responde `503` e não abre nem entra em nada, então pode ser tentada de novo) e as revoga quando o participante sai ou é removido.

- As credenciais duram no máximo 48 horas, então `voice_turn_credential_ttl` não pode exceder `48h` (o servidor se recusa a iniciar caso contrário).
- **A Cloudflare desconecta uma retransmissão pouco depois de suas credenciais expirarem.** O cliente web as renova antes disso, então mantenha `voice_turn_credential_ttl` bem acima de dez minutos.
- Preços, conforme verificado em 2026-10-26: os primeiros 1.000 GB por mês são gratuitos, depois US$ 0,05 por GB enviado aos clientes. Verifique os preços atuais da Cloudflare antes de contar com isso.

<!-- source: hiding-ip-addresses de67ffa9e7b7 -->
### Ocultando endereços IP

Com `voice_ice_transport_policy = all` (o padrão), os navegadores se conectam diretamente quando podem, então os participantes de uma sala podem descobrir os endereços IP uns dos outros.

Com `relay`, os navegadores usam apenas o servidor TURN, então um participante vê o endereço do servidor TURN e não o dos outros participantes. O áudio permanece criptografado entre os navegadores. O que `relay` não oculta: quem opera o servidor TURN (você ou a Cloudflare) vê o endereço de cada participante, e este servidor vê o endereço de todos que chamam a API dele.
`relay` exige que `voice_turn` seja `coturn` ou `cloudflare` (o servidor se recusa a iniciar caso contrário) e envia todas as chamadas pelo servidor TURN, o que consome sua largura de banda com o coturn e sua fatura de transferência com a Cloudflare.

<!-- source: what-p2p-cannot-enforce 4f97611bece5 -->
### O que o P2P não consegue garantir

Apenas alguns participantes podem falar em uma sala: os anfitriões e os ouvintes que um anfitrião autoriza. Em `p2p`, isso é imposto em dois lugares: o servidor se recusa a retransmitir uma oferta de conexão que enviaria áudio de alguém que não pode falar, e os outros navegadores não reproduzem o áudio de um participante que não é falante.

Isso detém usuários comuns e o cliente web oficial, mas não é uma defesa completa. **Dois participantes com clientes modificados que trabalhem juntos ainda podem trocar áudio**: o cliente modificado de alguém que não pode falar pode enviá-lo, e o cliente modificado de um receptor pode reproduzi-lo, e o servidor não consegue ver o interior da mídia criptografada.
Se você precisa garantir quem fala, por exemplo em eventos públicos moderados, use `sfu`, em que o próprio nó de mídia encaminha apenas o áudio de quem fala.

<!-- source: media-nodes 17f4fc8eb80b -->
## Nós de mídia

Com `voice = sfu`, cada sala é mantida por um nó de mídia, cujo processo encaminha o áudio da sala. Uma sala nunca se divide
entre nós.

<!-- source: run-media-nodes fac01c5bdbf9 -->
### Executar nós de mídia

- **`role = all`** executa um nó no mesmo processo que todo o resto, em `wss://<host>/voice/sfu`. Uma máquina com as
  portas UDP e TCP abertas basta.
- **`role = sfu`** executa um nó próprio, ao lado dos processos `web` e `worker`. Ele compartilha com eles o banco de dados, o
  `stream` (não `hub`) e o `kv` (não `memory`): os nós registram a si mesmos e suas salas no armazenamento chave-valor, e
  ficam sabendo das mudanças em suas salas pelo stream. Não executa jobs e atende apenas às verificações de saúde e ao
  WebSocket do nó. Execute quantos precisar: um processo `web` coloca cada sala nova no nó com menos vagas reservadas que
  tenha espaço para ela. Um processo `web` não executa nenhum nó.

O que cada nó precisa:

| O quê | Configuração | Observações |
| --- | --- | --- |
| Um endereço IP público | `voice_sfu_public_ip` atrás de um NAT | O nó informa aos navegadores esse endereço, ou o seu próprio quando não está definido. Uma VM ou um contêiner na nuvem costuma ter um endereço privado próprio: defina-o. |
| A porta UDP, aberta para a internet | `voice_sfu_udp_port` (7882) | Todas as sessões do nó a compartilham. |
| A porta TCP, aberta para a internet | `voice_sfu_tcp_port` (7881) | Para navegadores em redes que bloqueiam UDP. |
| Uma URL `wss:` com TLS | `voice_sfu_public_url` (`wss://<host>/voice/sfu`) | Cada nó tem a sua, como `wss://sfu1.example.org/voice/sfu`. Um proxy reverso na frente encerra o TLS e encaminha o WebSocket, ou o próprio nó serve HTTPS: as configurações `tls` funcionam com `role = sfu`, para o host de `voice_sfu_public_url` ([Servir HTTPS pelo servidor](running.md#serving-https-from-the-server)). |
| Quantos participantes comporta | `voice_sfu_node_capacity` (500) | Veja [Capacidade](#capacity). |

Encaminhe as portas UDP e TCP para o nó sem alterá-las: o nó informa aos navegadores os próprios números de porta. Um nó em
outro host que não `host` também precisa de `voice_sfu_origins` nos processos `web`, como `wss://*.sfu.example.org`, para
que a Content Security Policy do cliente web permita a conexão.

<!-- source: docker-compose 35fa31462add -->
### Docker Compose

A camada [`compose.sfu.yaml`](../../deploy/docker/compose.sfu.yaml) executa o servidor como `role = web`, um `worker` e
um nó de mídia `sfu` em uma só máquina, sobre PostgreSQL atrás do Caddy ([Arquivos do Docker Compose](running.md#docker-compose-files)):

```sh
# In .env:
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml:compose.sfu.yaml
# MY_VOICE_SFU_PUBLIC_IP=203.0.113.10   the public address of this machine
docker compose up -d
```

- O Caddy envia `/voice/sfu` ao nó e todas as outras requisições ao servidor, então a URL do nó é a padrão,
  `wss://<MY_HOST>/voice/sfu`, e `voice_sfu_origins` não é necessário.
- Abra UDP 7882 e TCP 7881 no firewall da máquina, além de 80 e 443.
- A camada define `voice = sfu` e `stream = postgres` para os três processos, seja o que for que o `.env` diga.

<!-- source: capacity 2137d8a7bcce -->
### Capacidade

Uma sala reserva o seu máximo (o máximo do anfitrião para a sala, ou `voice_max_participants`) no seu nó quando é
colocada, não importa quantos tenham entrado. Um nó aceita uma sala apenas enquanto a soma de suas reservas permanecer
dentro de `voice_sfu_node_capacity`, de modo que um nó nunca abriga mais participantes do que a sua capacidade. Quando nenhum nó
tem espaço para o máximo de uma sala, abrir a sala ou a primeira entrada responde `503`, e o usuário pode tentar de novo mais tarde.

Defina `voice_sfu_node_capacity` conforme o que a máquina consegue encaminhar ([Custo medido](#measured-cost)), e mantenha
`voice_max_participants` não maior do que suas salas precisam: uma sala de 500 reserva 500 vagas mesmo com 3 pessoas nela.

<!-- source: measured-cost 2d6571f6e923 -->
### Custo medido

Medido com clientes simulados enviando áudio Opus a 50 pacotes por segundo, em um AMD Ryzen 9 7900 (12 núcleos) com
Linux sob WSL2 e Go 1.27, tudo em loopback. A CPU é dada em núcleos dessa máquina; a memória é o heap do nó e as
pilhas das goroutines, somados ao resto do processo.

| O quê | CPU | Memória |
| --- | --- | --- |
| Um nó ocioso | nenhuma mensurável | cerca de 0,1 MiB |
| Cada sala aberta | nenhuma mensurável | menos de cerca de 25 KiB |
| Cada participante | cerca de 0,0002 núcleo em silêncio | cerca de 180 KiB |
| Cada stream encaminhado (alguém que fala, ouvido por um participante) | de 18 a 27 µs por pacote, cerca de 0,001 núcleo | menos de cerca de 25 KiB |
| Uma sala de 100 com 3 pessoas falando | cerca de 0,3 núcleo | cerca de 23 MiB |

Uma máquina mais rápida ou mais lenta escala esses números; as métricas do próprio nó ([Métricas a acompanhar](#metrics-to-watch)) mostram
o que ele sustenta.

O custo de encaminhar acompanha os pacotes que o nó envia: o áudio de cada pessoa que fala vai a todos os outros
participantes, então uma sala de N participantes com K pessoas falando envia cerca de K × (N − 1) streams de 30 a 50 kbit/s. Uma sala com
muitos ouvintes e poucas pessoas falando é barata; uma sala em que todos falam ao mesmo tempo é a que mais custa. Planeje a
largura de banda de saída do nó da mesma forma.

<!-- source: drain-a-node-for-a-deploy 249929addcae -->
### Esvaziar um nó para uma implantação

Ao receber SIGTERM, um nó se esvazia antes de parar:

1. Deixa de aceitar salas novas.
2. Coloca cada uma de suas salas em outro nó e avisa os clientes da sala para se mudarem. Eles se reconectam ao novo nó
   com uma nova concessão, e o áudio para por cerca de 1 a 2 segundos.
3. Espera até que seus clientes tenham saído, por no máximo 20 dos 25 segundos do desligamento, depois espera as
   requisições restantes e encerra ([Parar o servidor](running.md#stop-the-server)).

Dê ao processo um período de carência de pelo menos 30 segundos antes de ele ser encerrado à força (Kubernetes e ECS usam 30 por padrão, o Docker
10, valor que o `compose.sfu.yaml` aumenta), e reinicie os nós um de cada vez, para que os outros possam receber suas salas.
Uma sala para a qual nenhum outro nó tem espaço permanece no nó que está se esvaziando até que termine ou os 20 segundos passem.
**Com um único nó**, todas as salas permanecem: seus participantes perdem o áudio quando o nó encerra e se reconectam quando
o novo nó inicia.

<!-- source: when-a-node-is-lost c577f2240d7e -->
### Quando um nó é perdido

Quando um nó para sem se esvaziar (uma queda, uma máquina perdida), os WebSockets de seus clientes se fecham e seus
leases no armazenamento chave-valor expiram em até 15 segundos. Os clientes pedem uma nova concessão, e o processo `web` coloca a sala de novo
em outro nó. Uma sala não perde nada com o seu nó: seus participantes, quem fala e os silenciados estão no banco de dados.

<!-- source: turn-for-clients-without-udp 1c1230dc875c -->
### TURN para clientes sem UDP

Um navegador alcança um nó por UDP, ou por TCP quando o UDP está bloqueado. Em redes que deixam passar apenas a porta 443 (algumas
redes corporativas e públicas), ele precisa de um servidor TURN sobre TLS na 443: defina `voice_turn` como `coturn`, com uma
URL `turns:` na porta 443 em `voice_turn_coturn_urls`, ou como `cloudflare`, como para `p2p` ([coturn](#coturn),
[Cloudflare TURN](#cloudflare-turn)). `voice_ice_transport_policy = relay` é recusado com `sfu`: os participantes
já veem apenas o endereço do nó.

<!-- source: cloud-run 5cf3ffcbb7a6 -->
### Cloud Run

O Cloud Run aceita apenas requisições HTTP, então `role = sfu` não pode ser executado nele. Execute os processos `web` e `worker` no Cloud
Run e os nós em VMs com IP público, compartilhando com eles o banco de dados, o `stream` (`postgres` ou `pubsub`) e o `kv`
(`db` ou `valkey`). Dê a cada nó o seu próprio nome de host e `voice_sfu_public_url`, sirva TLS no nó
(`tls = acme`) ou na frente dele, e defina `voice_sfu_origins` no serviço do Cloud Run com as origens dos nós. Os
módulos do OpenTofu de [Implantar no Google Cloud](deploy-google-cloud.md) não criam os nós.

<!-- source: metrics-to-watch a8b32402b50d -->
### Métricas a acompanhar

Cada nó exporta suas próprias métricas `my.voice.sfu.*` ([Métricas](running.md#metrics)).

| Métrica | O que ela diz |
| --- | --- |
| `my.voice.sfu.capacity.reserved` em relação a `voice_sfu_node_capacity` | Quão cheio um nó está para salas novas. Quando todos os nós estão perto do limite, as salas novas recebem `503`: adicione um nó, ou aumente a capacidade se o nó tiver CPU de sobra. |
| `my.voice.sfu.capacity.connected`, `my.voice.sfu.sessions`, `my.voice.sfu.rooms` | Quem está conectado. Muito abaixo das reservas significa que os máximos das salas são maiores do que o necessário. |
| `my.voice.sfu.packets` por `my.outcome` | `forwarded` é o trabalho do nó. `silent` e `not_allowed` (um participante que não pode falar ou está silenciado) são descartados por projeto. `queue_full` significa que a conexão de um participante não conseguiu acompanhar: um cliente lento quando são poucos, um nó com falta de CPU ou de largura de banda quando sobe para todos os clientes ao mesmo tempo. |
| `my.voice.sfu.bytes` | O áudio encaminhado: a largura de banda de saída do nó. |
| `my.voice.sfu.client.rtt`, `my.voice.sfu.client.packet_loss` | A qualidade no caminho até os participantes. Alta para a maioria dos clientes aponta para a rede ou a localização do nó; para poucos, para as redes deles. |
| `my.voice.sfu.offer.wait` | Quanto tempo um participante que entra espera pela primeira offer do nó: milissegundos em um nó com CPU de sobra; segundos significam que as entradas esperam em um nó ocupado. |
| `my.voice.sfu.migrations`, `my.voice.sfu.ice_restarts`, `my.voice.sfu.renegotiations` | Salas movidas por um esvaziamento, clientes que mudaram de rede e mudanças de quem fala. |
| `my.voice.sfu.ticket_refusals` por `my.voice.sfu.reason` | Um `other_node` que não para de subir significa que os clientes alcançam um nó diferente do da sua concessão: verifique se o `voice_sfu_public_url` de cada nó leva a esse nó. |

<!-- source: deployment-notes 63042f72a7c5 -->
## Notas de implantação

- **Dois streams por usuário**: durante uma chamada, um participante mantém o stream de eventos da própria sala (Server-Sent Events), além do stream que o app sempre abre. Dimensione os limites de conexão para duas requisições longas por usuário em uma chamada.
- **Vários processos**: use um adaptador de stream compartilhado (`stream` definido como `postgres`, `valkey`, `sns` ou `pubsub`), como no resto do app. Com `stream = hub` e um `role` diferente de `all`, o servidor avisa ao iniciar que o chat de voz não funcionará entre processos.
- **Cloud Run**: um stream é uma requisição, e a CPU é alocada enquanto ela está aberta. Quando o timeout da requisição a corta, o cliente se reconecta. Defina o timeout o mais longo que puder: `service.request_timeout_seconds` de [Implantar no Google Cloud](deploy-google-cloud.md#limits).
- **Nós de mídia precisam de uma VM**: um nó de mídia de `sfu` recebe áudio por UDP em um processo de longa duração, que o Cloud Run não pode hospedar ([Cloud Run](#cloud-run)).

<!-- source: ending-rooms-and-records b4d3d196ecd7 -->
## Encerrando salas e registros

- Um moderador que tenha `manage_reports` pode encerrar uma sala aberta para todos, mesmo com o chat de voz desativado ([moderação](moderation.md#voice-chat-rooms)).
- Quem participou de uma sala (participações, falantes e ouvintes, mãos levantadas, convites e remoções) é mantido por `voice_participants_retention_days` dias depois que a sala termina (padrão 90, no mínimo 30), ou enquanto uma denúncia apontar para a sala. O nome da sala, a descrição, os anfitriões e a forma como ela terminou são mantidos. O áudio, o estado do microfone e as reações nunca são gravados.
- `voice_public_summary` controla se os visitantes que não podem entrar em uma sala `local` veem o nome dela e se ela está aberta, na página da sala e na prévia do link dela ([configurações da instância](instance-settings.md)).

<!-- source: notes-for-users f44150f15915 -->
## Notas para os usuários

- Em um iPhone ou iPad, o envio de áudio pode parar enquanto o app está em segundo plano, porque o iOS pode suspender uma página web que não está em primeiro plano. Mantenha o app aberto na tela durante uma chamada.
- O navegador pede permissão para usar o microfone na primeira vez. Se você a negou, permita-a novamente nas configurações do site no navegador.
- Em uma sala ponto a ponto, outros participantes podem conseguir descobrir seu endereço IP, a menos que a instância encaminhe as chamadas por um relay. Pergunte ao operador se isso importa para você.
