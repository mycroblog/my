<!-- source: posting-and-choosing-who-sees-a-post 5c4adb3025f6 -->
# Publicar e escolher quem vê uma publicação

<!-- source: writing-a-post 5f24a2f4b092 -->
## Escrevendo uma publicação

Escolha “Escrever uma publicação” para abrir o formulário “Nova publicação”. Digite em “Sua publicação” e escolha “Publicar”. O formulário mostra quantos
caracteres restam. Além de texto, você pode:

- Anexar imagens, vídeo ou áudio com “Adicionar imagens, vídeo ou áudio”. Descreva cada anexo para quem não pode vê-lo ou ouvi-lo; alguns servidores exigem uma descrição em todos os anexos, e “Exigir descrições de mídia” nas configurações de “Exibição e publicação” a exige das suas próprias publicações. A localização e outros metadados são removidos dos arquivos.
- “Adicionar uma enquete”, com opções e uma duração, e as escolhas “Permitir escolher várias opções” e “Ocultar os votos até a enquete terminar”.
- “Adicionar um aviso de conteúdo”. Os leitores veem o aviso primeiro e escolhem “Mostrar conteúdo”.
- Escolher “Ocultar as imagens até os leitores mostrá-las” para marcar a mídia como sensível.
- Definir o idioma da publicação e quem pode responder, impulsionar e citar (veja [Quem pode responder, impulsionar e citar](#who-can-reply-boost-and-quote)).
- Responder a uma publicação (“Responder”) ou citar uma (“Citar”).

Uma publicação que não pôde ser enviada fica em “Publicações sendo enviadas”, onde você pode “Enviar de novo”, “Editar” ou “Descartar” ela.
Um servidor pode recusar uma publicação após uma análise automática. Quando a recusa pode ser contestada, a publicação oferece “Contestar”.

<!-- source: drafts-and-scheduled-posts 9cee5f0d17b2 -->
## Rascunhos e publicações agendadas

Escolha “Salvar rascunho” para guardar no servidor o que você está escrevendo e esvaziar o formulário. Um rascunho salvo abre em qualquer dispositivo em que você inicie sessão: o link “Rascunhos e publicações agendadas” sob o formulário leva à página que os lista, onde “Editar” abre um rascunho no formulário novamente. Quando o formulário já contém outra coisa, você é consultado antes de ela ser substituída. Salvar um rascunho que você abriu nessa página o substitui; publicá-lo o publica e o remove dos seus rascunhos. Se o mesmo rascunho foi salvo em outro dispositivo depois que você o abriu, nada é sobrescrito nem publicado: salve de novo para guardar a sua versão como um novo rascunho, ou publique de novo para publicar a sua versão como uma nova publicação.

Para publicar uma publicação mais tarde, escolha “Agendar a publicação” (o relógio entre as ferramentas do formulário), escolha uma data e um horário e escolha “Agendar”. A data e o horário estão no seu fuso horário, o das suas preferências, e o formulário o nomeia. Um horário no passado, cedo demais ou distante demais é recusado antes de qualquer envio; o servidor define com que antecedência mínima e máxima você pode agendar. A publicação é publicada nesse horário, ou logo depois.

A página “Rascunhos e publicações agendadas” lista primeiro as suas publicações agendadas, com o horário em que cada uma é publicada, e depois os seus rascunhos. Uma publicação agendada pode ser movida com “Alterar horário” ou voltar a ser um rascunho com “Cancelar agendamento”, e qualquer rascunho pode ser excluído. Só você vê os seus rascunhos.

Quando uma publicação agendada não pode ser publicada no horário, por exemplo porque a publicação a que ela responde não existe mais, ela fica entre os seus rascunhos com o motivo, e você recebe uma notificação que leva aos seus rascunhos, para poder corrigi-la e agendá-la de novo.

<!-- source: who-can-see-a-post be247913f548 -->
## Quem pode ver uma publicação

A ferramenta “Quem pode ver” do formulário abre as opções para essa publicação. O padrão para novas publicações é “Quem pode ver”
em “Novas publicações”, nas configurações de “Exibição e publicação”.

| Opção | Quem pode ver |
| --- | --- |
| Pública | Qualquer pessoa, aqui e em outros servidores. Aparece nas linhas do tempo públicas e nas hashtags. |
| Vizinhos | Pessoas conectadas a este servidor e seus seguidores em outros servidores. Aparece na linha do tempo local deste servidor. |
| Pública discreta | Qualquer pessoa com o link ou com o seu perfil, mas não aparece nas linhas do tempo públicas nem nas hashtags. |
| Somente seguidores | Apenas seus seguidores e as pessoas que você menciona. |
| Direta | Apenas as pessoas que você menciona. |

![Quem pode ver uma publicação neste servidor e em outros servidores, para cada uma das cinco opções. A lista abaixo diz o mesmo.](images/visibility.svg)

Linha por linha, neste servidor e depois em outros servidores:

- **Pública**: qualquer pessoa, inclusive quem não está conectado, e a publicação aparece nas linhas do tempo local e federada. Em outros servidores qualquer pessoa pode lê-la, e ela é enviada aos seus seguidores e às pessoas que você menciona.
- **Vizinhos**: as pessoas conectadas a este servidor, e a publicação aparece na linha do tempo local. Outros servidores a recebem apenas para os seus seguidores lá, que a veem como uma publicação “Somente seguidores”; ela não é enviada a mais ninguém.
- **Pública discreta**: qualquer pessoa com o link ou com o seu perfil, mas a publicação não aparece nas linhas do tempo públicas nem nas hashtags. Em outros servidores também, qualquer pessoa com o link pode lê-la, e ela é enviada aos seus seguidores e às pessoas que você menciona.
- **Somente seguidores**: seus seguidores e as pessoas que você menciona, neste servidor e em outros servidores.
- **Direta**: apenas as pessoas que você menciona, neste servidor e em outros servidores.

Em um servidor de uma só pessoa, onde ninguém mais está conectado, “Vizinhos” significa o mesmo que “Somente seguidores”: novas publicações
não podem ter essa opção, e um padrão “Vizinhos” funciona como “Somente seguidores”. “Quem pode ver” e o padrão nas configurações
a omitem, e uma resposta a uma publicação para vizinhos é “Somente seguidores”. Publicações que já a têm a mantêm.

Um bloqueio sempre prevalece: quem bloqueia você, ou quem você bloqueia, não vê sua publicação, seja qual for a opção que você escolheu.

<!-- source: neighbors 2581a57320f5 -->
### “Vizinhos”

Uma publicação para vizinhos fica no seu servidor. Quem não está conectado a ele não pode lê-la, nem com o
link, e ela nunca aparece na linha do tempo federada. Outros servidores a recebem apenas para os seus seguidores lá, e a
mostram a esses seguidores como uma publicação somente para seguidores.

- Se você menciona alguém de outro servidor que não segue você, essa pessoa não recebe a publicação, e seus seguidores lá veem a menção como um link simples. O formulário avisa sobre isso.
- Contas do seu servidor podem impulsionar uma publicação para vizinhos, e o impulso aparece apenas neste servidor.
- Uma resposta a uma publicação para vizinhos também começa como “Vizinhos”; você pode mudar.

<!-- source: other-things-to-know b36265de1760 -->
### Outras coisas para saber

- Você não pode mudar quem pode ver uma publicação depois de publicá-la. Uma edição mantém quem pode ver e a publicação à qual ela responde.
- Um moderador pode limitar o alcance das publicações de uma conta. O formulário então diz “Sua conta pode publicar no máximo como ...” e uma opção mais ampla é publicada no nível mais amplo permitido.
- Qualquer pessoa que possa ver uma publicação “Pública”, “Vizinhos” ou “Pública discreta” pode impulsioná-la. Só você pode impulsionar suas próprias publicações “Somente seguidores”, e só seus seguidores veem esse impulso. Publicações “Diretas” não podem ser impulsionadas.
- O autor de uma publicação pode escolher quem pode responder a ela e quem pode impulsioná-la; publicações de alguns outros servidores também trazem essa escolha. Uma resposta ou um impulso fora dela é recusado, com uma mensagem como “O autor desta publicação não permite que você responda a ela.” O autor, as pessoas que a publicação menciona e o autor da publicação à qual ela responde sempre podem responder.
- O autor também pode permitir que algumas pessoas respondam ou impulsionem apenas depois de aprovar. Essa resposta ou esse impulso fica aguardando: você é notificado (“... pediu para responder à sua publicação”), e ele fica fora da sua publicação até você aprovar. Uma resposta aguardando, de outro servidor, não aparece na conversa sob a sua publicação nem na contagem de respostas dela; ela ainda aparece no perfil do autor e na própria página. Um impulso aguardando não é contado. Aprovar adiciona-o à sua publicação; recusar o mantém de fora, e o outro servidor é avisado nos dois casos.
- Uma resposta ou um impulso de outro servidor que está totalmente fora da sua escolha é mantido fora da sua publicação do mesmo modo, e não notifica você de nada.
- Quando você responde ou impulsiona uma publicação cujo autor aprova respostas ou impulsos manualmente, a sua aguarda essa aprovação. Até lá, só você vê a sua resposta, sob a publicação e na própria página, marcada como “Aguardando aprovação”; os outros não veem nada dela, e nada é enviado a outros servidores. Seu impulso só é feito quando aprovado. Você é notificado quando o autor aprova ou recusa (“... aprovou sua resposta, impulso ou citação”). Excluir uma resposta aguardando, ou desfazer um impulso aguardando, retira o pedido.

<!-- source: who-can-reply-boost-and-quote 5d5ece5f0cb2 -->
## Quem pode responder, impulsionar e citar

“Quem pode responder, impulsionar e citar” no menu “Quem pode ver” abre as opções para essa publicação, cada uma com seu valor,
por exemplo “Quem pode impulsionar: Seguidores, outros com a sua aprovação”. Para respostas, impulsos e citações, escolha
quem pode fazê-lo de imediato: “Qualquer pessoa”, “Seguidores”, “Pessoas que você menciona” ou “Somente você”. Abaixo de cada um, “Permitir outras pessoas com a sua
aprovação” permite que todos os demais peçam, em vez de serem recusados.

- As pessoas que uma publicação menciona sempre podem responder a ela. Uma publicação “Somente seguidores” só pode ser impulsionada e citada por você, e uma publicação “Direta” por ninguém, seja qual for a sua escolha.
- Os padrões para novas publicações, com as mesmas opções, ficam em “Quem pode responder, impulsionar e citar” nas configurações de “Exibição e publicação”.
- Os pedidos que aguardam você são listados em “Aguardando aprovação”, aberto pela linha no topo das notificações. “Aprovar” adiciona a resposta, o impulso ou a citação à sua publicação, e a conversa sob ela mostra na hora uma resposta aprovada; “Recusar” a mantém de fora.
- Na publicação de outra pessoa, “Responder” não aparece e “Impulsionar” não pode ser pressionado quando o autor não permite. Quando o autor os aprova manualmente, o formulário avisa antes de você responder, e depois de você impulsionar, a publicação diz “Impulso aguardando aprovação”.

<!-- source: after-posting e11fd06a76e8 -->
## Depois de publicar

Sob uma publicação você encontra “Responder”, “Impulsionar”, “Citar”, “Favoritar”, “Reagir” e “Mais”. “Mais” tem “Copiar link”, “Abrir
página original” (para uma publicação de outro servidor), “Compartilhar” e “Salvar”; seus itens salvos ficam na página “Itens salvos”,
e só você os vê. Na sua própria publicação ele também tem “Editar” (não para uma publicação com enquete), “Excluir e reescrever…” e
“Excluir…”. As duas exclusões pedem confirmação antes. “Excluir…” remove a publicação para todos, junto com seus impulsos e favoritos;
“Excluir e reescrever…” faz o mesmo e abre o formulário com o texto, as imagens e a enquete dela como uma nova publicação.

Uma publicação editada mostra “· editada” depois do horário; escolha-o para abrir “Histórico de edições”, com a versão mais recente primeiro e as
mudanças marcadas.

Uma enquete aceita um voto, ou vários quando o autor permite: escolha e depois “Votar”. Os resultados aparecem depois que você votou ou
a enquete terminou, e para o autor a qualquer momento. “Reagir” abre um seletor de emoji (busca, categorias, tom de pele e os emoji personalizados
deste servidor); cada pessoa dá a uma publicação um favorito ou uma reação. Na página da própria publicação, um emoji lista
quem reagiu com ele. O formulário também tem um botão “Adicionar emoji”.

Uma publicação em outro idioma que não o seu tem “Traduzir” quando o servidor tem um serviço de tradução ou seu navegador pode
traduzir; “Mostrar original” volta. Atrás de um aviso de conteúdo, a linha fica na faixa do aviso. Se o servidor envia
o texto das publicações a um serviço de tradução, “Quem pode ver” informa para quais opções.

Imagens, GIFs, vídeo e áudio aparecem na publicação. Escolha uma imagem para abrir o visualizador: deslize ou use as setas, dê zoom e
leia a descrição dela. A mídia marcada como sensível fica oculta sob uma linha como “Imagem sensível”, até você escolher “Mostrar”. Um link em uma
publicação aparece como um cartão.

<!-- source: writing-an-article 3bc498986329 -->
## Escrevendo um artigo

“Escrever um artigo”, sob o formulário de nova publicação, abre o editor de artigos, uma página própria. Dê ao artigo um título, um
resumo se quiser (deixado vazio, ele é feito a partir do início do corpo), uma imagem de capa com a descrição dela e o
corpo, escrito em Markdown: `#` inicia um título, `**text**` é negrito, e “Negrito” e “Link” acima do corpo fazem o mesmo para
o texto que você seleciona (Ctrl+B e Ctrl+K, ou Cmd em um Mac). Solte ou cole imagens no corpo, ou escolha-as com “Adicionar
imagens”: cada uma é enviada e entra onde está o cursor como `![](media:…)`; escreva a descrição dela entre os colchetes.
Só as imagens que você envia aparecem em um artigo; uma imagem de outro endereço vira um link com a descrição dela.

- As hashtags vão em um campo próprio, separadas por espaços; o artigo é listado nelas, e um `#` no corpo continua sendo texto. As menções no corpo levam às pessoas, mas não as notificam.
- “Pré-visualizar” mostra o artigo como os leitores o verão, renderizado pelo servidor exatamente como será publicado, com o tamanho do corpo e o número de imagens em relação aos limites deste servidor.
- “Marcar como sensível” mostra aos leitores o título e o resumo primeiro e recolhe a capa e o corpo. Um artigo não pode ser “Direta”.
- Enquanto você escreve, o artigo é salvo como rascunho toda vez que você faz uma pausa (“Rascunho salvo”), e assim ele abre em qualquer dispositivo em “Rascunhos e publicações agendadas”, onde “Editar” o traz de volta ao editor. Se o rascunho foi alterado em outro dispositivo nesse meio tempo, nada é sobrescrito: seu texto continua no editor, e “Salvar como novo rascunho” o mantém como outro rascunho.
- “Publicar” publica o artigo agora e abre a página dele; “Agendar” o publica no horário que você escolher, como em uma publicação.
- Para alterar um artigo que você publicou, escolha “Editar” no menu “Mais ações” dele. O editor abre com o que você escreveu; “Salvar alterações” o substitui, e a versão anterior continua no “Histórico de edições”. Quem pode ver um artigo não muda.

Um servidor pode permitir que apenas algumas pessoas escrevam novos artigos; então “Escrever um artigo” não aparece para as demais, que ainda podem
editar os artigos que publicaram.

<!-- source: reading-articles f2811fffd462 -->
## Ler artigos

Um artigo é uma publicação longa com título, resumo e, muitas vezes, uma imagem de capa, escrita neste servidor ou em outro. As linhas do tempo e os perfis mostram um artigo como um cartão com a capa, a palavra “Artigo”, o título e o resumo; selecione o título para ler o artigo inteiro na página própria dele, que o título encabeça. Respostas, impulsos, citações, favoritos e reações funcionam como em qualquer publicação. Os artigos não têm “Traduzir”.

Um artigo que o autor marcou como sensível mostra o título e o resumo e oculta o resto: no cartão, “Mostrar mídia sensível” mostra a capa, e na página dele “Mostrar o artigo” mostra a capa e o texto. A aba “Artigos” de um perfil lista apenas os artigos daquela pessoa, os mais recentes primeiro; para alguém de outro servidor, apenas os artigos que este servidor recebeu.

<!-- source: direct-messages e262b7a83b69 -->
## Mensagens diretas

“Mensagens diretas”, na navegação, reúne as suas publicações “Diretas” e as que você recebe, uma linha por
conversa, com a de publicação mais recente primeiro. Cada linha mostra quem está na conversa, quando a publicação mais recente dela foi
escrita e o início dessa publicação. O número ao lado de “Mensagens diretas” é quantas conversas têm uma publicação que você
ainda não leu; cada uma delas tem um ponto antes dos nomes.

Escolha o início de uma publicação para abrir o tópico da conversa; isso marca a conversa como lida em todos os seus dispositivos. Uma
resposta a uma publicação “Direta” também é sempre “Direta”, e assim não alcança ninguém que a conversa não alcançava. Em
celulares, “Mensagens diretas” fica em “Menu”.

<!-- source: deleting-old-posts-automatically ab68788c9a84 -->
## Excluindo publicações antigas automaticamente

“Excluir publicações antigas automaticamente”, em “Conta e dados” nas configurações, exclui suas publicações quando elas ficam mais antigas que
a idade que você escolhe, como se você mesmo as tivesse excluído: elas saem daqui e dos servidores para os quais foram enviadas, e não podem ser restauradas. As publicações que você mantém
(fixadas, salvas ou favoritadas por você, com mídia ou enquete, artigos, por quem pode vê-las, ou com muitos impulsos ou
favoritos) permanecem. Seus impulsos de publicações de outras pessoas nunca são excluídos. O servidor verifica a cada hora e exclui no
máximo 100 das suas publicações por vez, então muitas publicações antigas levam várias horas para sumir.

Uma conta nova começa com as configurações que o servidor escolheu para contas novas; a partir daí elas são suas, e você pode
alterá-las como qualquer outra configuração.

Um servidor também pode manter as publicações por no máximo alguns dias. A página “Sobre” do servidor informa isso antes de você se cadastrar. Sob esse
máximo, a exclusão automática está sempre ativada para você, a idade que você escolhe não pode ser maior que o máximo, e só os
tipos de publicação que o servidor permite podem ser mantidos (por padrão, apenas as fixadas, então seus artigos também são excluídos,
a menos que o servidor permita mantê-los). Alguns servidores permitem que algumas pessoas mantenham as publicações por mais tempo, por exemplo como benefício de um plano de apoiador.

Quando um máximo passa a valer para você (o servidor o introduz ou o reduz, ou você perde o direito de manter publicações por mais tempo),
você recebe uma notificação, que leva a essa página, e um e-mail, a menos que você tenha desativado, dizendo a partir de quando ele vale.
Até lá suas configurações continuam em vigor. A página mostra a data e, em “Publicações que seriam excluídas”, quantas saem nessa data.
