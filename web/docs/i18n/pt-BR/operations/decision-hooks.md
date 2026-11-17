<!-- source: decision-hooks 9945608ea473 -->
# Hooks de decisão

Um hook de decisão pergunta a um [plugin](plugins.md) seu se deve aceitar algo antes que o servidor o aceite. O plugin
responde com um veredito de uma lista fixa. Ele nunca reescreve nada e não toma nenhuma ação sobre uma conta: o máximo que pode fazer
é impedir uma publicação, um seguimento, um cadastro, um envio ou uma alteração de perfil, ou abrir uma denúncia para os seus moderadores. As pessoas decidem o que acontece com uma conta.

Nada é analisado por padrão. Um servidor sem endereço de hook não faz nenhuma chamada e não gasta tempo com eles. Esta versão mantém
os registros, os modos e as telas de administração descritos aqui. Entre os caminhos de entrada, as publicações de outros servidores consultam
`inbound.status`, os seguimentos de outros servidores consultam `inbound.follow`, os cadastros consultam `signup.check`, os envios consultam
`media.check`, as publicações de contas locais consultam `local.status`, os perfis de contas em outros servidores consultam `inbound.actor` e
as alterações de perfil de contas locais consultam `local.profile`; os demais hooks ainda não são chamados.

![O caminho de um hook de decisão: uma publicação remota, um seguimento remoto, um cadastro, um envio, uma publicação local ou o perfil alterado de um ator remoto passa pelo modo do hook até o seu plugin, cujo veredito ou falha leva a uma decisão registrada. A lista abaixo descreve cada etapa.](images/decision-hooks.svg)

Do topo do diagrama para baixo:

1. **O que é analisado e quem espera.** Uma publicação de outro servidor (`inbound.status`), um seguimento de outro servidor
   (`inbound.follow`) e um envio (`media.check`) são analisados em um job. Um cadastro (`signup.check`) é analisado enquanto
   o candidato espera, uma publicação de uma conta local (`local.status`) enquanto o autor espera, e uma alteração do
   perfil de uma conta local (`local.profile`) enquanto o dono espera. Um perfil de outro servidor (`inbound.actor`) é
   analisado onde quer que o servidor busque a conta: em um job e também enquanto uma requisição espera, como a de um servidor
   cuja conta é nova aqui.
2. **O modo**, `hook_<hook>_mode`: `off` não faz chamada, `observe` registra o veredito e não age sobre nada, e `enforce`
   age sobre ele ([Modos](#modes)).
3. **Seu plugin** recebe `POST /my.plugin.hook.v1.HookService/Check`. O servidor espera até `plugin_<hook>_timeout`, faz
   no máximo 8 chamadas ao mesmo tempo e, após 5 falhas seguidas, ignora o plugin por 30 segundos (`breaker_open`).
4. **Um veredito**: `allow` aceita o assunto, `flag` aceita e abre uma denúncia, `mark_sensitive` aceita marcando-o como
   sensível, `require_approval` retém um cadastro, e `drop` e `reject` o impedem e não guardam nada (o `reject` de um cadastro só com
   [`hook_signup_check_reject`](#sign-ups)). **Uma falha** é um `timeout`, um `error`, uma resposta `invalid` ou `breaker_open`, e uma resposta que o hook não aceita
   conta como falha. Quando o hook aplica o veredito, `hook_<hook>_on_failure` decide: `pass` aceita o assunto sem análise, `retry`
   não registra decisão e executa o job de novo, e `require_approval` retém um cadastro
   ([Quando o plugin falha](#when-the-plugin-fails)).
5. **Uma decisão por hook e assunto** é registrada, e um job que roda de novo a lê em vez de chamar o plugin
   ([As decisões](#the-decisions)).

<!-- source: the-hooks f1005c829ed5 -->
## Os hooks

| Hook | O que analisa | O que pode responder | Quando o plugin falha |
| --- | --- | --- | --- |
| `inbound.status` | Publicações de outros servidores | `allow`, `drop`, `flag`, `mark_sensitive` | `pass` (padrão) ou `retry` |
| `inbound.follow` | Seguimentos de outros servidores | `allow`, `drop`, `flag` | `pass` (padrão) ou `retry` |
| `signup.check` | Cadastros, depois do CAPTCHA e da lista de bloqueio | `allow`, `require_approval`, `reject` | `pass` (padrão) ou `require_approval` |
| `media.check` | Mídia enviada, antes que alguém possa usá-la | `allow`, `mark_sensitive`, `flag`, `reject` | `pass` (padrão) ou `retry` |
| `local.status` | Publicações de contas locais e suas edições | `allow`, `flag`, `reject` | `pass` |
| `inbound.actor` | Perfis de contas em outros servidores, quando mudam | `allow`, `drop`, `flag` | `pass` (padrão) ou `retry` |
| `local.profile` | Alterações de perfil de contas locais | `allow`, `flag`, `reject` | `pass` |

Defina `plugin_<hook>_address` para dar um plugin a um hook, com o ponto no nome virando `_`
(`plugin_inbound_status_address`). [Plugins](plugins.md) trata do destino, da autenticação dele e de como testar um
plugin com `my plugin test --kind inbound.status <address>`. A chamada é `POST /my.plugin.hook.v1.HookService/Check`.

O que cada resposta faz:

- `drop` impede o assunto e não guarda nada dele. Em `inbound.actor`, mantém o perfil guardado antes
  ([abaixo](#profiles-from-other-servers)). Um `drop` errado também esconde silenciosamente uma publicação dos seguidores dela, então leia
  o que um hook descartaria em `observe` antes de aplicá-lo.
- `flag` deixa passar e abre uma denúncia por decisão. Em `inbound.follow`, a denúncia é sobre a conta que enviou
  o seguimento, não cita nenhuma publicação e é aberta quando o seguimento é registrado; o denunciante, a categoria e o comentário são os de
  [uma denúncia de publicação](#what-inboundstatus-reviews). Em `inbound.actor`, a denúncia é sobre a conta dona do perfil,
  da mesma forma.
- `mark_sensitive` deixa passar e mostra os anexos como sensíveis até que um moderador cancele a decisão.
- `require_approval` retém o cadastro para aprovação.
- `reject` o recusa, com um texto fixo que não mostra nada da resposta do plugin. Em `signup.check`, isso também depende de
  [a configuração abaixo](#sign-ups). Em `media.check`, não guarda nada do envio, nem o original
  ([abaixo](#what-mediacheck-reviews)). Em `local.status`, o autor pode [recorrer](#local-posts). Em `local.profile`, o
  perfil continua como estava e não há recurso ([abaixo](#local-profiles)).

Uma resposta pode trazer um motivo codificado (`spam`, `harassment`, `sexual`, `violence`, `illegal`, `malware`, `other`) e uma nota de
até 200 caracteres para os seus moderadores. Uma resposta que o hook não aceita conta como falha.

<!-- source: what-inboundstatus-reviews 1dcf4ccd16f9 -->
## O que `inbound.status` analisa

O servidor pergunta sobre cada `Create` de uma publicação e cada `Update` de uma que ele aceitaria, antes de guardar qualquer coisa. Cada
revisão é um assunto próprio, com decisão própria.

- Uma publicação só é consultada quando está relacionada a uma conta local: uma conta local segue o autor dela, ela menciona uma conta
  local ou está em uma conversa com uma publicação local. Se uma conta local segue o autor não restringe mais nada:
  o plugin recebe `any_local_follower` e pode ponderar isso. Uma publicação trazida apenas por um relay nunca é consultada, nem um
  `Update` de uma publicação guardada que não se relaciona mais a uma conta local: a revisão dela é guardada sem pergunta.
- Uma publicação direta de um autor que nenhuma conta local segue não é enviada, a menos que você ative
  `hook_inbound_status_unfollowed_direct`. As publicações das suas contas locais nunca são enviadas, em nenhuma configuração.
- Votos de enquetes, exclusões, desfazimentos, curtidas, reações e impulsos nunca são consultados, então parar um plugin não pode impedir
  que uma exclusão chegue. A busca de uma conversa ou a busca de um impulso ainda pode trazer a mesma publicação por outro caminho.

O que cada resposta faz com uma publicação:

- `drop` em um `Create` não guarda nada e não cria notificação nem entrada na linha do tempo, e uma nova entrega é ignorada como
  duplicata. `drop` em um `Update` não cria revisão e mantém a anterior. Uma publicação descartada não é guardada, então um
  `Update` dela publicado nas últimas 24 horas é tratado como uma nova publicação: o plugin é consultado de novo sobre essa revisão, como sobre
  qualquer outra, e a resposta dele ao `Create` anterior não vale para ela.
- `flag` abre uma denúncia por decisão, com o ator da instância como denunciante, a categoria do motivo (`spam` e
  `malware` como Spam, `illegal` como Legal, `harassment`, `sexual` e `violence` como Violation, e qualquer outro como Other) e
  a nota do plugin como comentário. A denúncia informa que uma análise a abriu. Ela não é encaminhada ao servidor do autor
  a menos que um moderador a [encaminhe](moderation.md#reports), e um encaminhamento nunca envia a nota: só o texto que o moderador
  escreve. Resolvê-la não notifica ninguém. Cada decisão abre a sua própria
  denúncia, então uma onda de spam pode encher a fila.
- `mark_sensitive` marca a publicação, não a revisão: a marca permanece nas revisões seguintes, seja qual for a resposta a elas, até que um
  moderador cancele a decisão. Ela mostra os anexos como sensíveis e deixa o texto intacto, então uma publicação sem anexo
  não parece diferente, e outros servidores não a veem.

<!-- source: what-mediacheck-reviews f93110fe5a83 -->
## O que `media.check` analisa

O servidor pergunta sobre cada arquivo que uma conta local envia, para qualquer uso: um anexo, um avatar, um cabeçalho ou um emoji
personalizado. Ele pergunta uma vez, quando o arquivo é processado pela primeira vez e antes de ficar pronto, então ninguém pode usá-lo enquanto o plugin
decide. Usar o mesmo arquivo para outra coisa depois lê a decisão já tomada. A mídia remota nunca é consultada.

Ele envia a menor imagem que o servidor gerou do arquivo, sem os metadados, e nunca o original: para um arquivo de vídeo ou
áudio, a pequena imagem do pôster ou da capa incorporada, ou nenhuma imagem quando não há. Junto vão para que o arquivo
serve, se é imagem, vídeo ou áudio, o tamanho, quanto tempo dura e se tem movimento. O texto alternativo e
qualquer dado da conta ou de uma publicação não são enviados. A mensagem `MediaSubject` do protocolo `HookService` tem os campos.

O que cada resposta faz com um envio:

- `reject` faz o processamento falhar, como para um arquivo que o servidor não consegue ler: quem enviou vê que o arquivo não pôde ser
  processado, e uma publicação não pode anexá-lo. **O servidor não guarda nada de um arquivo rejeitado.** Ele exclui o original quando o
  processamento termina, seja qual for a resposta, e um arquivo rejeitado não recebe versão processada. Se você precisa preservar esses
  arquivos, ou denunciá-los a alguém, o seu plugin tem que fazer isso ao responder: o servidor não pode devolver o arquivo.
  Cancelar um `reject` só altera o registro dele.
- `mark_sensitive` marca a publicação que anexa o arquivo, na transação que cria a publicação ou a edição que adiciona
  o arquivo, e a marca permanece até que um moderador cancele a decisão, como em `inbound.status`. Um avatar, um cabeçalho ou um
  emoji não tem publicação, então a decisão apenas é registrada.
- `flag` abre uma denúncia por decisão, com o ator da instância como denunciante. Para um anexo, ela é aberta junto com a
  publicação que anexa o arquivo e aponta para essa publicação; um arquivo que nunca é publicado não abre nenhuma. Para um avatar, um cabeçalho ou
  um emoji, é aberta quando o arquivo é processado, sobre a conta que o enviou.

O job de processamento espera o plugin por até `plugin_media_check_timeout` (10 segundos por padrão). Adicionar um emoji
personalizado de outro servidor ou aprovar um pedido de emoji processa a imagem enquanto o moderador espera, então essa espera
é dele.

<!-- source: sign-ups be7ddf1149a9 -->
## Cadastros

`signup.check` roda nos cadastros feitos pela página de cadastro e por meio de um provedor de login, depois do CAPTCHA e da
[lista de bloqueio de cadastros](moderation.md), então uma requisição que qualquer um dos dois recuse nunca chega ao plugin. Um cadastro com um
código de convite nunca o chama. Ele envia o endereço de e-mail que o candidato informou e o endereço IP do cliente; a conta
ainda não existe, então a chave do assunto é o ID que a conta receberia, e um candidato que tenta de novo é um novo assunto.

As respostas agem como as da lista de bloqueio quando o hook aplica o veredito:

- `require_approval` deixa o cadastro esperando em **Moderação** > **Cadastros**, como faz um bloqueio com a mesma
  severidade, em todos os modos de cadastro. Aprove ou rejeite-o ali: cancelar a decisão não altera o cadastro.
- `reject` não faz nada até você ativar `hook_signup_check_reject` (**Configurações do servidor** > **Federação** > **Análise externa**, desativado por
  padrão). Enquanto está desativado, um `reject` é registrado em `observe`, já que não impede nada, e o cadastro segue. Enquanto
  está ativado, o candidato vê o mesmo texto fixo de um bloqueio (“Este servidor não pode aceitar este cadastro.”). Nenhuma conta
  é criada, então cancelar essa decisão só altera o registro dela, e o candidato se cadastra de novo.

Os candidatos nunca veem o motivo nem a nota de uma decisão.

A requisição do candidato espera o plugin por até `plugin_signup_check_timeout` (2 segundos por padrão; mantenha-o curto, já que
uma pessoa está esperando). A espera não faz parte de uma transação do banco de dados. Um cadastro que falha depois, por exemplo por um
nome de usuário já em uso, mantém a decisão até que a retenção a remova.

<!-- source: profiles-from-other-servers e324990a50b1 -->
## Perfis de outros servidores

`inbound.actor` analisa o perfil de uma conta em outro servidor no ponto em que o servidor o recebe: na primeira vez que busca
a conta, quando a conta envia um `Update` de si mesma e quando o servidor a busca de novo, como faz uma vez por dia com as contas
que os seus usuários seguem. Ele só pergunta quando o perfil difere do guardado, então buscar uma conta de novo com
o mesmo perfil não faz chamada. O perfil que ele analisa é:

- o nome de exibição, como texto simples,
- a bio e os campos do perfil, como o servidor os guarda (HTML saneado),
- os endereços do avatar e da imagem de cabeçalho no outro servidor. O servidor não envia nenhuma imagem.

O plugin também recebe o endereço da conta, o servidor dela, o nome de usuário e se a conta é nova no seu servidor. A
chave de uma decisão é o endereço da conta, `#` e um resumo do perfil, então cada perfil distinto recebe uma decisão, e
o mesmo perfil enviado de novo a lê.

O que os vereditos fazem:

- `drop` mantém o perfil guardado antes: o nome de exibição, a bio, os campos, o avatar, o cabeçalho e seus emojis
  personalizados continuam como estavam. O servidor ainda guarda o resto do que recebeu: as chaves da conta, a caixa de entrada e outros
  endereços, o nome de usuário e se ela é trancada, um bot, descobrível e indexável. Sem as chaves e os endereços, o
  servidor não poderia verificar as atividades da conta nem entregar a ela, e as configurações de trancada e indexável dizem como
  seguir a conta e se o dono permite a indexação. Enquanto o perfil continua descartado, buscá-lo de novo lê a
  mesma decisão e mantém o guardado.
- Uma conta buscada pela primeira vez é sempre guardada, porque seguimentos e menções precisam dela, então o `drop` dela é tratado como um
  `flag`: o perfil é guardado e uma denúncia é aberta. A decisão continua mostrando `drop`.
- `flag` guarda o perfil e abre uma denúncia sobre a conta por decisão.

Quando o plugin falha, `pass` guarda o perfil sem análise, e o servidor não pergunta sobre ele de novo até que mude.
Com `retry`, o servidor não guarda uma conta cujo perfil mudou enquanto o plugin falha: um `Update` espera no seu
job, o que segura as outras atividades dessa conta, como em [`inbound.status`](#when-the-plugin-fails), e uma busca
feita enquanto uma requisição espera falha, então um servidor cuja conta é nova aqui tem a entrega recusada até que o plugin volte
e ele a envie de novo.

Cancelar um `drop` de `inbound.actor` busca a conta de novo no servidor dela, que guarda o perfil que serve agora
sem chamar o plugin quando ele é o descartado, e pergunta sobre ele quando mudou desde então.

<!-- source: local-posts 37ba27537191 -->
## Publicações locais

`local.status` analisa uma publicação de uma conta do seu servidor antes de o servidor criá-la, e cada edição de uma antes de o
servidor guardá-la. O autor espera o plugin, então
a espera é curta: `plugin_local_status_timeout` (1 segundo por padrão). Uma falha, um timeout ou um breaker aberto sempre deixa a
publicação passar (`pass` é o único comportamento de falha dele), e a espera não faz parte de uma transação do banco de dados.

O que ele envia e quando:

- Somente as publicações de contas mais novas que `hook_local_status_account_age_days` (**Configurações do servidor** > **Federação** >
  **Análise externa**, 7 dias por padrão; `0` envia as publicações de todas as contas). As contas novas são de onde vem a maior parte do spam, e as contas
  estabelecidas não são enviadas.
- Somente publicações `public`, `local` e `unlisted`. As publicações `private` e `direct` nunca são enviadas.
- O texto como o autor o digitou e como o servidor o renderiza, o aviso de conteúdo, o idioma, se está marcada como
  sensível, as descrições das imagens, as opções da enquete, as hashtags, quantas contas ela menciona, o endereço que a publicação
  receberia, a publicação a que responde ou que cita (apenas pelo endereço), o endereço e o nome de usuário do autor e quando a conta foi
  criada. Nenhuma mídia é enviada.
- A chave do assunto é o ID que a publicação receberia. Enviar uma publicação de novo com o mesmo `Idempotency-Key` devolve a publicação
  que foi criada, sem chamar o plugin; uma nova tentativa é um novo assunto.
- Uma edição é enviada da mesma forma, com a publicação como editada, para as mesmas contas: as que estão dentro de
  `hook_local_status_account_age_days`. A chave dela é o ID da publicação, `#` e o ID da nova revisão, então cada edição é um
  assunto próprio.
- Iniciar uma sala de chat de voz publica a primeira publicação dela sem este hook.

Quando o hook aplica o veredito, `reject` recusa a publicação e nada é guardado. O autor vê um texto fixo (“Esta publicação não foi publicada.”)
e um caminho para recorrer; nunca o motivo nem a nota. `flag` deixa a publicação passar e abre uma denúncia dela, contra o
autor.

Em uma edição, `reject` recusa somente essa edição: a publicação continua como estava, e o autor vê um texto fixo (“Esta edição não foi
salva.”). Uma edição não pode ser objeto de recurso, já que a publicação continua lá para os seus moderadores agirem como de costume. `flag` guarda a
edição e abre uma denúncia da publicação, e `allow` a deixa passar.

<!-- source: appeals 79991f552151 -->
### Recursos

O autor de uma publicação recusada pode recorrer dentro de `appeal_window_days` (a mesma janela das ações de moderação), uma vez por
decisão. O recurso leva o texto da publicação, já que o servidor não guardou nada dela, e um comentário opcional para os seus
moderadores. Nenhum plugin é chamado.

Seus moderadores (`manage_reports`) são notificados de cada novo recurso (**Recursos de publicações recusadas** nas configurações de
notificação deles, que também podem recebê-las por e-mail; a notificação abre o recurso) e analisam os recursos em **Moderação** >
**Análises automáticas** > **Recursos de publicações recusadas**. Cada recurso abre em uma página própria, com o texto que o autor enviou:

- **Manter** deixa a decisão como está e vai para o registro de auditoria como `hook_appeal_upheld`.
- **Reverter** cancela a decisão, o que vai para o registro de auditoria como `hook_decision_overridden`, e deixa de enviar as
  publicações da conta a `local.status`, em uma única transação. A publicação recusada não é restaurada: o autor a publica de novo.

De qualquer forma, o autor é notificado do resultado, entre os avisos dos moderadores, e o formulário de publicação o mostra sob
a recusa. A notificação e o formulário dizem apenas se a decisão permanece ou foi cancelada, nunca o motivo nem a
nota. Um cliente próprio lê em que ponto está um recurso com `GET /api/v1/hook_decisions/{id}/appeal`.

Uma conta que um recurso revertido tirou do hook continua fora dele até que a conta seja excluída. Se for assumida depois,
`local.status` não a analisa mais.

<!-- source: local-profiles dbafa1545e2a -->
## Perfis locais

`local.profile` analisa uma alteração do perfil de uma conta do seu servidor antes de o servidor guardá-la. O dono espera
o plugin, então a espera é curta: `plugin_local_profile_timeout` (1 segundo por padrão). Uma falha, um timeout ou um
breaker aberto sempre deixa a alteração passar (`pass` é o único comportamento de falha dele), e a espera não faz parte de uma transação
do banco de dados.

O que ele envia e quando:

- Somente as alterações de contas mais novas que `hook_local_profile_account_age_days` (**Configurações do servidor** > **Federação** >
  **Análise externa**, 7 dias por padrão; `0` envia as de todas as contas).
- Somente uma alteração do que o hook vê: o nome de exibição, a bio, os campos do perfil, o avatar ou o cabeçalho. Uma alteração
  que apenas tranca a conta ou muda as configurações de descoberta dela não é enviada.
- O nome de exibição, a bio como digitada e como o servidor a renderiza, o nome e o valor de cada campo do perfil (como digitados e como
  renderizados), se a alteração define, substitui ou remove o avatar ou o cabeçalho, o endereço e o nome de usuário da conta
  e quando a conta foi criada. Nenhuma imagem é enviada. O perfil é público, então o hook pode usar TCP simples.
- A chave do assunto é o ID da conta, `#` e o ID da revisão do perfil que a alteração guardaria, então cada alteração
  é um assunto próprio.

Quando o hook aplica o veredito, `reject` recusa somente essa alteração: o perfil continua como estava, e o dono vê um texto fixo (“Esta
alteração do seu perfil não foi salva.”), nunca o motivo nem a nota. Não há recurso: o dono pode alterar o perfil
e salvar de novo. `flag` salva a alteração e abre uma denúncia contra a conta, que não cita nenhuma publicação; o denunciante, a
categoria e o comentário são os de [uma denúncia de publicação](#what-inboundstatus-reviews).

<!-- source: modes 6133f824d202 -->
## Modos

Cada hook tem um modo, uma [configuração da instância](instance-settings.md) (`hook_inbound_status_mode` e assim por diante) que você altera em
**Configurações do servidor** > **Federação** > **Análise externa**. As alterações vão para o registro de auditoria como `setting_changed`.

| Modo | O que faz |
| --- | --- |
| `off` | Não chama nada. |
| `observe` | Registra o veredito e não age sobre nada. Este é o padrão de todo hook. |
| `enforce` | Age sobre o veredito. |

Um hook vai de `off` para `enforce` somente passando por `observe`.

<!-- source: when-the-plugin-fails 7c987d71eb78 -->
## Quando o plugin falha

Um timeout, um erro e uma resposta inválida são registrados como resultados próprios (`timeout`, `error`, `invalid`), nunca como
vereditos. Depois de 5 chamadas falhas seguidas, o servidor ignora o plugin por 30 segundos e então tenta uma chamada, de modo que uma indisponibilidade
não custa espera a nenhum assunto; uma chamada ignorada é registrada como `breaker_open`. Cada processo do servidor conta por conta própria.

O que acontece com o assunto então depende de `hook_<hook>_on_failure`:

- `pass` deixa passar sem análise. Spam pode entrar durante uma indisponibilidade, mas nada mais é impedido. Este é o padrão.
- `retry` faz o job falhar, que roda de novo e chama o plugin de novo, e não registra decisão. Os jobs de um mesmo ator rodam
  em ordem, então tudo o que vem depois desse ator espera nesse meio tempo, exclusões inclusive, por até cerca de 4 horas e 15 minutos.
  Depois da última execução, o job vira um [job morto](running.md#the-job-queue-and-failed-dead-jobs), o que libera os outros.
- `require_approval` retém o cadastro para um moderador.

Em `media.check`, `pass` deixa o arquivo pronto sem análise. `retry` mantém o envio como ainda em processamento até que o plugin
responda ou o job esgote as execuções; o arquivo nunca fica pronto nesse meio tempo, e um job morto o deixa sem processar, o que a
limpeza de envios não anexados então exclui. Nenhum outro job espera por ele.

Cada processo faz no máximo 8 chamadas a um plugin ao mesmo tempo, e uma chamada que espera a vez além do prazo do hook conta como
timeout. As esperas aparecem em `my.plugin.slot_wait` ([Execução](running.md)). Mantenha `plugin_inbound_status_timeout` curto: enquanto uma chamada
espera, os jobs seguintes do mesmo ator esperam com ela.

Em `observe`, uma falha nunca segura nada. Os moderadores recebem uma notificação uma vez por dia por hook quando as chamadas dele continuam falhando.
As métricas são `my.hook.decisions`, `my.hook.failures` e `my.hook.duration` ([Execução](running.md)).

<!-- source: the-decisions 4d41f7512468 -->
## As decisões

O servidor registra uma decisão por hook e assunto, então um job que roda de novo a lê e não chama o plugin outra vez.
Uma decisão guarda o hook, a chave do assunto, o modo, o resultado, o motivo, a nota e quanto tempo a chamada levou. Ela
não guarda texto de publicação, endereço de e-mail nem endereço IP.

**Moderação** > **Análises automáticas** (`manage_reports`) mostra as contagens e as esperas dos últimos 7 dias e o que os hooks
impediram, teriam impedido em `observe` ou não conseguiram verificar. Cada decisão abre em uma página própria com o motivo, a nota e a
espera. Cancelar uma decisão, pela lista ou pela página dela, vai para o registro de auditoria como `hook_decision_overridden`. Um `reject` de `local.status` não é cancelado ali: o autor [recorre](#appeals).

O que cancelar faz depende da resposta. Um `mark_sensitive` perde a marca, e uma marca de uma ação de moderação na mesma
publicação permanece. Um `flag` mantém a denúncia, que você resolve como de costume. Um `drop` de `inbound.status` processa a publicação recebida
de novo sem consultar o plugin, enquanto o servidor guardar a atividade que recebeu (7 dias); depois disso, a tela
avisa e cancelar apenas registra o cancelamento.

Cancelar um `drop` de `inbound.follow` executa o seguimento de novo, sem chamar o plugin, enquanto o servidor guardar
a atividade que recebeu (7 dias). Depois disso, o seguimento não pode voltar e o cancelamento apenas fica registrado; o
outro servidor precisa enviar um novo seguimento.

Um seguimento só consulta o hook quando seria registrado como um novo seguimento ou pedido de seguimento. Desfazer um seguimento (`Undo`) nunca
consulta, nem um seguimento que o servidor recusa de qualquer forma: um que o modo de federação recusa, um de uma conta movida ou
desativada, um do ator da instância, ou um entre contas que se bloqueiam, que recebe a rejeição
de costume. Um seguimento enviado de novo para um seguimento ou pedido que já existe também não consulta: o servidor o responde como de costume
e o aceita de novo quando o seguimento já está aceito. Um seguimento descartado não recebe resposta, então o outro servidor continua mostrando-o
como solicitado. Enviar a mesma atividade de novo é ignorado como duplicata.

Um efeito automático vai para o registro de auditoria, com o sistema como ator, somente quando deixa estado: `mark_sensitive`, um
`flag` aberto e `require_approval` (`hook_decision_applied`). `drop` e `reject` não guardam nada, então ficam somente nas
decisões.

O job `hook.purge_decisions` remove as decisões após `hook_decisions_retention_days` dias (padrão 90, no mínimo 30). Ele mantém
aquelas em que uma marca de sensível em uma publicação existente se apoia, as de uma conta sob uma
[retenção legal](moderation.md#record-retention-and-holds) e cada `reject` de uma nova publicação por `local.status` de que o autor ainda
pode recorrer ou cujo recurso espera análise. Um `reject` de uma edição não pode ser objeto de recurso, então some quando o período de retenção
termina. Um recurso vai com a sua decisão. Excluir uma conta remove as decisões dela, os recursos e
a isenção dela em `local.status`.

<!-- source: what-you-tell-your-users 2ab3b8f14096 -->
## O que você diz aos seus usuários

Enquanto um hook tem um plugin e não está em `off`, a sua página de informações (`/about`) declara que uma análise externa está em uso, o que
ela cobre e o que é enviado a ela. O servidor escreve isso a partir das suas configurações, incluindo a idade de conta de
`hook_local_status_account_age_days` e `hook_local_profile_account_age_days` (ou que as de todas as contas são enviadas, em
`0`) e se
`hook_inbound_status_unfollowed_direct` envia as publicações diretas de autores que ninguém aqui segue.

<!-- source: what-is-sent-and-what-it-costs fd966d9abd0d -->
## O que é enviado e quanto custa

As chamadas, o custo delas e os dados enviados crescem com o volume que o servidor recebe, em `inbound.status` acima de tudo: cada publicação no escopo
é uma chamada, e cada revisão, outra. Revise o que você envia antes de ativar um hook:

- `inbound.status` e `local.status` enviam o texto das publicações e recusam TCP simples, mesmo em loopback. `inbound.status` envia
  o que o servidor remoto enviou: o texto em cada um dos idiomas, o aviso de conteúdo, se está marcada como sensível, quando
  foi publicada, o endereço e a URL da publicação, o endereço da publicação a que responde, os endereços e as descrições dos
  anexos (sem mídia), as opções da enquete, as hashtags, o autor e o servidor do autor, a visibilidade e quantas contas
  a publicação menciona e quantas delas são locais. Também envia se uma conta local segue o autor
  (`any_local_follower`), que é um dado sobre os seus usuários. As publicações só para seguidores (`private`) são enviadas como as públicas, e
  as diretas como [acima](#what-inboundstatus-reviews). Não envia nada da publicação a que responde além do endereço dela.
  `local.status` envia os campos listados em [Publicações locais](#local-posts). A mensagem `InboundStatus` do protocolo `HookService` tem os campos.
- `signup.check` envia o endereço de e-mail e o endereço IP de cada candidato e recusa TCP simples.
- `inbound.follow` envia as duas partes de um seguimento.
- `inbound.actor` envia o perfil público de uma conta em outro servidor, como listado em
  [Perfis de outros servidores](#profiles-from-other-servers), e pode usar TCP simples. Ele é chamado sempre que esse perfil
  muda, então o volume dele acompanha o número de contas que o seu servidor conhece.
- `media.check` envia a menor imagem processada de cada envio, nunca o original, com para que o arquivo serve, o
  tipo, o tamanho e a duração. Pode usar TCP simples, já que não envia texto: uma imagem processada ainda pode mostrar uma pessoa.
- `local.profile` envia o perfil público listado em [Perfis locais](#local-profiles), e nenhuma imagem. Pode usar TCP
  simples.
