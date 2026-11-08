<!-- source: run-voice-chat 54808e78eb0d -->
# Executar o chat de voz

O chat de voz permite que usuários conectados abram uma sala, convidem outras pessoas e conversem. Ele fica desativado até você escolher como o áudio trafega.
Este guia explica essa escolha, os servidores STUN e TURN ou o servidor de mídia que cada opção exige, e o que saber sobre implantação, privacidade e moderação.
Cada configuração citada aqui é descrita na [referência de configuração](../reference/configuration.md#voice).

<!-- source: choose-how-the-audio-travels 9d60bd0655e9 -->
## Escolha como o áudio trafega

`voice` escolhe um único método para a instância inteira; todas as salas o usam.

| `voice` | O que transporta o áudio | Indicado para |
| --- | --- | --- |
| `off` | Nada. A API do chat de voz responde `404`, o cliente web não mostra o chat de voz e as outras configurações `voice_*` não são verificadas na inicialização. | O padrão. |
| `p2p` | Os navegadores dos participantes enviam o áudio diretamente uns aos outros. O servidor retransmite apenas as mensagens que estabelecem as conexões. | Salas pequenas e a configuração mais barata: nenhum servidor extra, exceto um servidor STUN ou TURN opcional. |
| `livekit` | Um servidor LiveKit, auto-hospedado ou LiveKit Cloud. | Salas maiores e controle rigoroso sobre quem pode falar. |
| `sora` | Um servidor Sora, auto-hospedado ou Sora Cloud. | O mesmo, se você já executa o Sora. |

![Os três métodos lado a lado. Com p2p, o servidor retransmite os sinais e os navegadores enviam o áudio uns aos outros, ou por meio de um servidor TURN. Com livekit e sora, os navegadores enviam sinais e áudio ao servidor de mídia, e o servidor fala com esse servidor de mídia pela API e por webhooks. A lista abaixo descreve cada método.](images/voice-methods.svg)

Em todos os métodos, o servidor (`my serve`) guarda as salas, quem participa delas e suas sessões, e entrega a cada
participante uma concessão: o que o navegador precisa para se conectar. Ele nunca transporta o áudio. O que muda é por onde passam a sinalização
(as mensagens que estabelecem uma conexão) e o áudio:

- **`p2p`.** Um navegador envia seus sinais (offers, answers e candidatos ICE) ao servidor, que os repassa ao
  outro navegador pelo fluxo de eventos da sala; um sinal espera no armazenamento chave-valor por no máximo 30 segundos. O áudio
  vai diretamente de navegador para navegador. A concessão lista os servidores STUN de `voice_stun_urls` e o servidor TURN de
  `voice_turn` (`coturn` ou `cloudflare`) com credenciais de curta duração. Um navegador descobre seu endereço público pelo STUN,
  e o servidor TURN retransmite o áudio quando os dois navegadores não encontram um caminho direto.
- **`livekit`.** A concessão contém `voice_livekit_url` e um token de acesso de 60 segundos. O navegador envia seus sinais e seu
  áudio ao servidor LiveKit, que traz seus próprios STUN e TURN e encaminha o áudio aos outros participantes (uma unidade de encaminhamento seletivo,
  ou SFU).
  O servidor cria as salas do LiveKit e altera quem pode falar pela API do LiveKit, e o LiveKit informa entradas e
  saídas ao servidor por webhooks.
- **`sora`.** A concessão contém `voice_sora_signaling_urls` e um valor de entrada assinado. O navegador envia seus sinais e seu
  áudio ao servidor Sora, que traz seus próprios STUN e TURN. O Sora consulta o webhook de autenticação do servidor antes de cada
  conexão, o servidor controla as salas pela API do Sora, e o Sora informa as conexões por webhooks.

Observações sobre a escolha:

- Com `p2p`, o áudio nunca passa por este servidor (ele passa pelo servidor TURN quando um é usado). A carga do servidor se resume às mensagens da sala, aos heartbeats e aos fluxos.
- Uma instância pequena é mais bem atendida por `p2p`, ou pelo plano gratuito Build do LiveKit Cloud (veja [LiveKit Cloud](#livekit-cloud)).
- Com `livekit` ou `sora`, as configurações de STUN e TURN e a política de ICE abaixo não são usadas (o servidor avisa na inicialização se você as definir), porque o servidor de mídia traz seus próprios servidores. Os endereços IP dos participantes ficam então visíveis apenas ao servidor de mídia, não aos outros participantes.
- As build tags `no_voice_livekit` e `no_voice_sora` deixam os dois métodos de servidor de mídia fora de um binário ([build tags](../reference/build-tags.md)).
- Sirva a instância por HTTPS: os navegadores só permitem acesso ao microfone em páginas seguras.

Os limites mantêm o custo de um servidor de mídia pago sob controle. `0` remove qualquer um deles, exceto o último:

| Configuração | Padrão | Significado |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | O máximo de tempo que uma sala fica aberta. Uma sala que o atinge é encerrada. |
| `voice_max_open_rooms` | `10` | Quantas salas podem estar abertas ao mesmo tempo neste servidor. |
| `voice_sfu_max_participants` | `10` | O tamanho da sala para `livekit` e `sora`. |
| `voice_p2p_max_participants` | `10` | O tamanho da sala para `p2p`, de 2 a 50. |

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

<!-- source: size-and-bandwidth 9872bf829e26 -->
### Tamanho e largura de banda

Dois navegadores se conectam quando pelo menos um deles está falando; duas pessoas que apenas ouvem não se conectam.
Um falante, portanto, envia uma cópia separada do seu áudio a cada um dos outros participantes. A fala em Opus ocupa aproximadamente de 30 a 50 kbit/s com o overhead, então um falante em uma sala de 10 envia cerca de 9 cópias, algumas centenas de kbit/s,
e um falante em uma sala de 50 envia 49 cópias, cerca de 2 Mbit/s, o que um celular com dados móveis ou uma conexão residencial pode não sustentar. O número de conexões em uma sala de N pessoas é no máximo N(N-1)/2: 45 para 10 e 1.225 para 50.
Por isso o padrão é 10. Aumente `voice_p2p_max_participants` apenas quando seus usuários tiverem largura de banda de envio; use `livekit` ou `sora` para salas grandes.

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

<!-- source: what-p2p-cannot-enforce 31e06affcd4f -->
### O que o P2P não consegue garantir

Apenas alguns participantes podem falar em uma sala: os anfitriões e os ouvintes que um anfitrião autoriza. Em `p2p`, isso é imposto em dois lugares: o servidor se recusa a retransmitir uma oferta de conexão que enviaria áudio de alguém que não pode falar, e os outros navegadores não reproduzem o áudio de um participante que não é falante.

Isso detém usuários comuns e o cliente web oficial, mas não é uma defesa completa. **Dois participantes com clientes modificados que trabalhem juntos ainda podem trocar áudio**: o cliente modificado de alguém que não pode falar pode enviá-lo, e o cliente modificado de um receptor pode reproduzi-lo, e o servidor não consegue ver o interior da mídia criptografada.
Se você precisa garantir quem fala, por exemplo em eventos públicos moderados, use `livekit` ou `sora`, em que o próprio servidor de mídia descarta o áudio de quem não tem permissão.

<!-- source: deployment-notes b0f03da306b7 -->
## Notas de implantação

- **Dois fluxos por usuário**: durante uma chamada, um participante mantém o fluxo de eventos próprio da sala (Server-Sent Events) além do fluxo que o app sempre abre. Dimensione os limites de conexão para duas requisições de longa duração por usuário em uma chamada.
- **Vários processos**: use um adaptador de fluxo compartilhado (`stream` definido como `postgres`, `valkey`, `sns` ou `pubsub`), como no resto do app. Com `stream = hub` e um `role` diferente de `all`, o servidor avisa na inicialização que o chat de voz não funcionará entre processos.
- **Cloud Run**: um fluxo é uma requisição, e a CPU é alocada enquanto ele está aberto. Quando o tempo limite da requisição o interrompe, o cliente se reconecta. Defina o tempo limite o mais longo que puder.
- **Servidores de mídia precisam de uma VM**: LiveKit e Sora recebem áudio por UDP em um processo de longa duração. O Cloud Run aceita apenas requisições HTTP, então não pode hospedá-los. Execute-os (e o coturn) em uma VM, ou use LiveKit Cloud, Sora Cloud ou Cloudflare TURN.

<!-- source: livekit b16c1d192419 -->
## LiveKit

Defina `voice = livekit` e:

| Configuração | Valor |
| --- | --- |
| `voice_livekit_url` | A URL `wss:` à qual os navegadores se conectam, por exemplo `wss://livekit.example.org`. |
| `voice_livekit_api_url` | A URL `https:` que este servidor chama, quando difere da acima. Por padrão, é o mesmo host com `https:`. |
| `voice_livekit_api_key`, `voice_livekit_api_secret` | Um par de chaves do servidor LiveKit. O segredo assina os tokens de acesso dos participantes e verifica os webhooks. |

O servidor cria cada sala no LiveKit, a nomeia com o ID da sala e entrega a cada participante um token de acesso de 60 segundos. O token de um ouvinte não pode publicar; o de um falante pode publicar apenas um microfone.

<!-- source: self-hosted-livekit 2b118136f4dc -->
### LiveKit auto-hospedado

Em `livekit.yaml`:

```yaml
keys:
  <api key>: <api secret>
room:
  # Rooms are created only by this server.
  auto_create: false
  # Audio only.
  enabled_codecs:
    - mime: audio/opus
    - mime: audio/red
webhook:
  api_key: <api key>
  urls:
    - https://<your domain>/voice/webhooks/livekit
```

- O servidor LiveKit precisa alcançar a URL do webhook por HTTPS. O servidor verifica a assinatura de cada webhook com o segredo da API.
- Os webhooks apenas aceleram as coisas. A cada minuto, o servidor compara os participantes de cada sala aberta com o que o LiveKit informa e remove quem não deveria estar lá, então um webhook perdido enquanto a instância estava fria é compensado.
- Para experimentar na sua máquina, `livekit-server --dev` inicia com um par de chaves de desenvolvimento fixo.
- **Um participante removido pode voltar a entrar por um curto período.** O LiveKit auto-hospedado não consegue revogar um token: ele continua enviando tokens novos aos clientes conectados, cada um válido por até cerca de 10 minutos. Alguém que é removido pode, portanto, voltar a entrar com o token mais recente por até cerca de 10 minutos, até o servidor perceber (o webhook de entrada ou a verificação de um minuto) e cortá-lo, e essa pessoa pode ser ouvida de alguns segundos a um minuto nesse intervalo. Use o LiveKit Cloud se a remoção precisar ser imediata: ele revoga o token na hora.

<!-- source: livekit-cloud 5bd9f833be11 -->
### LiveKit Cloud

Use a URL WebSocket, a chave de API e o segredo do projeto, e adicione `https://<your domain>/voice/webhooks/livekit` como endpoint de webhook nas configurações do projeto.
O plano gratuito Build oferece 5.000 minutos e 50 GB por mês e 100 conexões simultâneas, conforme verificado em 2026-10-26; consulte a página de preços do LiveKit para os planos atuais.

<!-- source: sora 5e9209dd3baa -->
## Sora

**O adaptador do Sora é testado apenas contra um Sora simulado que segue a documentação do Sora, não contra um servidor Sora real.** Experimente em uma instância de staging com um Sora real antes de depender dele.

Defina `voice = sora`, `voice_sora_signaling_urls` (URLs `wss:` separadas por vírgula às quais os navegadores se conectam), `voice_sora_join_secret` (pelo menos 32 bytes, por exemplo gerados com `openssl rand -base64 32`; ele assina os valores com que os participantes entram) e `voice_sora_mode` (`self_hosted` ou `cloud`).
A instância deve ser servida por HTTPS, porque o Sora faz chamadas de volta para `https://<your domain>`.

<!-- source: webhooks 11b2c486a476 -->
### Webhooks

O Sora chama dois endpoints deste servidor:

| Endpoint | Finalidade |
| --- | --- |
| `/voice/webhooks/sora/auth` | O webhook de autenticação. O Sora o chama antes de cada conexão. O servidor só a permite para um participante que está na sala, não foi removido, tem permissão para falar se estiver se conectando como falante e está dentro do limite de tamanho, e diz ao Sora para aceitar apenas áudio. |
| `/voice/webhooks/sora/events` | Os webhooks de evento e de sessão (conexões criadas e destruídas, sessões encerradas). |

**O webhook de autenticação precisa responder a tempo.** O Sora espera 5 segundos quando auto-hospedado e 10 segundos no Sora Cloud. O servidor responde com seus próprios dados, sem chamar mais nada, mas uma inicialização a frio da instância (o Cloud Run escalando a partir de zero) pode exceder esse tempo, e a conexão é então recusada. Mantenha uma instância ativa enquanto o chat de voz estiver em uso.

Ao auto-hospedar, defina `default_duplicate_client_id = evict` em `sora.conf`. Um ouvinte que se torna falante se reconecta com a mesma identidade (o Sora não consegue mudar a função de uma conexão), e `evict` faz o Sora descartar a conexão antiga quando a nova chega.
Com o padrão `allow`, o servidor corta a conexão antiga por conta própria assim que toma conhecimento da nova, então as duas se sobrepõem por um instante.

<!-- source: self-hosted-sora dd3244368b73 -->
### Sora auto-hospedado

| Configuração | Valor |
| --- | --- |
| `voice_sora_mode` | `self_hosted` |
| `voice_sora_api_url` | A URL `https:` de um proxy reverso na frente da API HTTP do Sora. |
| `voice_sora_api_header_name`, `voice_sora_api_header_value` | O cabeçalho com o qual o proxy autentica este servidor, por exemplo `Authorization` e `Basic ...`. |
| `voice_sora_webhook_username`, `voice_sora_webhook_password` | As credenciais Basic dos webhooks. Obrigatórias. |

**A API HTTP do próprio Sora não tem autenticação** e deve escutar apenas no endereço de loopback. Nunca a exponha. Coloque na frente dela um proxy reverso que exija uma credencial e entregue essa credencial a este servidor pelas duas configurações de cabeçalho.
Configure as URLs dos webhooks no Sora com as mesmas credenciais Basic. O servidor aceita apenas uma URL de API `https:` (`http:` somente para localhost) e não inicia sem as credenciais dos webhooks. mTLS não é suportado, porque o Cloud Run e o balanceador de carga na frente do ECS encerram o TLS antes do servidor.

<!-- source: sora-cloud ac4935c7f7a7 -->
### Sora Cloud

Defina `voice_sora_mode = cloud`, `voice_sora_cloud_project_id` e `voice_sora_cloud_api_key`. O servidor assina os tokens de acesso com a chave, chama a API com ela e verifica as assinaturas dos webhooks com ela; nenhuma credencial Basic é necessária.

<!-- source: behavior-to-expect 03fd5e1655fa -->
### Comportamento esperado

- Um ouvinte autorizado a falar se reconecta automaticamente como falante, então há uma breve pausa enquanto a nova conexão é iniciada.
- Um falante que perde esse direito é silenciado imediatamente por um filtro de encaminhamento e depois reconectado como ouvinte.

<!-- source: licensing 534e304817e3 -->
### Licenciamento

O Sora é um produto comercial da Shiguredo. Conforme verificado em 2026-10-26: o Sora Cloud tem uma taxa base de ¥5.000 por mês e nenhum período gratuito; a auto-hospedagem exige uma licença de pacote, paga antecipadamente por um número de conexões simultâneas e um período. A página do produto diz que ele não é oferecido a empresas, organizações ou indivíduos fora do Japão, então verifique se você pode comprá-lo antes de fazer planos com base nele. O LiveKit é de código aberto quando auto-hospedado.

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
