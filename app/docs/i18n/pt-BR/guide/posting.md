<!-- source: posting-and-choosing-who-sees-a-post 5c4adb3025f6 -->
# Publicar e escolher quem vê uma publicação

<!-- source: writing-a-post b3b7ce030983 -->
## Escrever uma publicação

Escolha “Escrever uma publicação” para abrir o formulário “Nova publicação”. Digite em “Sua publicação” e escolha “Publicar”. O formulário mostra quantos caracteres ainda restam. Além do texto, você pode:

- Anexar imagens, vídeo ou áudio com “Adicionar imagens, vídeo ou áudio”. Descreva cada anexo para as pessoas que não podem vê-lo ou ouvi-lo; alguns servidores exigem uma descrição para todos os anexos. A localização e outros metadados são removidos dos arquivos.
- “Adicionar uma enquete”, com opções e duração, e as opções “Permitir escolher várias opções” e “Ocultar os votos até a enquete terminar”.
- “Adicionar um aviso de conteúdo”. Os leitores veem o aviso primeiro e escolhem “Mostrar conteúdo”.
- Escolher “Ocultar as imagens até os leitores as mostrarem” para marcar a mídia como sensível.
- Definir o idioma da publicação e quem pode responder a ela, impulsioná-la e citá-la (veja [Quem pode responder, impulsionar e citar](#who-can-reply-boost-and-quote)).
- Responder a uma publicação (“Responder”) ou citar uma (“Citar”).

Uma publicação que não pôde ser enviada fica em “Publicações sendo enviadas”, onde você pode “Enviar de novo”, “Editar” ou “Descartar” a publicação.
Um servidor pode recusar uma publicação após uma análise automática. Quando a recusa pode ser contestada, a publicação oferece “Contestar”.

<!-- source: drafts-and-scheduled-posts 9cee5f0d17b2 -->
## Rascunhos e publicações agendadas

Escolha “Salvar rascunho” para guardar no servidor o que você está escrevendo e esvaziar o formulário. Um rascunho salvo abre em qualquer dispositivo em que você inicie sessão: o link “Rascunhos e publicações agendadas” sob o formulário leva à página que os lista, onde “Editar” abre um rascunho no formulário novamente. Quando o formulário já contém outra coisa, você é consultado antes de ela ser substituída. Salvar um rascunho que você abriu nessa página o substitui; publicá-lo o publica e o remove dos seus rascunhos. Se o mesmo rascunho foi salvo em outro dispositivo depois que você o abriu, nada é sobrescrito nem publicado: salve de novo para guardar a sua versão como um novo rascunho, ou publique de novo para publicar a sua versão como uma nova publicação.

Para publicar uma publicação mais tarde, escolha “Agendar a publicação” (o relógio entre as ferramentas do formulário), escolha uma data e um horário e escolha “Agendar”. A data e o horário estão no seu fuso horário, o das suas preferências, e o formulário o nomeia. Um horário no passado, cedo demais ou distante demais é recusado antes de qualquer envio; o servidor define com que antecedência mínima e máxima você pode agendar. A publicação é publicada nesse horário, ou logo depois.

A página “Rascunhos e publicações agendadas” lista primeiro as suas publicações agendadas, com o horário em que cada uma é publicada, e depois os seus rascunhos. Uma publicação agendada pode ser movida com “Alterar horário” ou voltar a ser um rascunho com “Cancelar agendamento”, e qualquer rascunho pode ser excluído. Só você vê os seus rascunhos.

Quando uma publicação agendada não pode ser publicada no horário, por exemplo porque a publicação a que ela responde não existe mais, ela fica entre os seus rascunhos com o motivo, e você recebe uma notificação que leva aos seus rascunhos, para poder corrigi-la e agendá-la de novo.

<!-- source: who-can-see-a-post 267cf73d0612 -->
## Quem pode ver uma publicação

O botão “Visibilidade” do formulário abre “Quem pode ver esta publicação?” para essa publicação. O padrão das novas publicações é a configuração “Quem vê suas publicações por padrão”, em “Exibição, publicação e linhas do tempo” nas configurações.

| Escolha | Quem pode ver |
| --- | --- |
| Pública | Qualquer pessoa, aqui e em outros servidores. Ela aparece nas linhas do tempo públicas. |
| Este servidor | Pessoas com sessão iniciada neste servidor, e seus seguidores em outros servidores. Ela aparece na linha do tempo local deste servidor. |
| Não listada | Qualquer pessoa com o link, mas ela não aparece nas linhas do tempo públicas. |
| Seguidores | Somente seus seguidores e as pessoas que você menciona. |
| Pessoas mencionadas | Somente as pessoas que você menciona. |

![Quem pode ver uma publicação neste servidor e em outros servidores, para cada uma das cinco visibilidades. A lista abaixo diz o mesmo.](images/visibility.svg)

Linha por linha, neste servidor e depois em outros servidores:

- **Pública**: qualquer pessoa, inclusive quem não iniciou sessão, e a publicação aparece nas linhas do tempo local e federada. Em outros servidores, qualquer pessoa pode lê-la, e ela é enviada aos seus seguidores e às pessoas que você menciona.
- **Este servidor**: as pessoas com sessão iniciada neste servidor, e a publicação aparece na linha do tempo local. Os outros servidores a recebem somente para os seus seguidores lá, que a veem como uma publicação de “Seguidores”; ela não é enviada a mais ninguém.
- **Não listada**: qualquer pessoa com o link, mas a publicação não aparece nas linhas do tempo públicas. Em outros servidores também, qualquer pessoa com o link pode lê-la, e ela é enviada aos seus seguidores e às pessoas que você menciona.
- **Seguidores**: seus seguidores e as pessoas que você menciona, neste servidor e em outros servidores.
- **Pessoas mencionadas**: somente as pessoas que você menciona, neste servidor e em outros servidores.

Em um servidor de uma só pessoa, onde nenhuma outra pessoa tem sessão iniciada, “Este servidor” não significa mais do que “Seguidores”: as novas publicações não podem usar essa opção, e um padrão de “Este servidor” funciona como “Seguidores”. O menu de visibilidade e o padrão em “Exibição, publicação e linhas do tempo” deixam essa opção de fora, e uma resposta a uma publicação de “Este servidor” fica como “Seguidores”. As publicações que já a têm a mantêm.

Um bloqueio sempre prevalece: quem bloqueia você, ou quem você bloqueia, não vê a sua publicação, qualquer que seja a escolha feita.

<!-- source: this-server d39e8d3e2df6 -->
### “Este servidor”

Uma publicação definida como “Este servidor” fica no seu servidor. As pessoas que não têm sessão iniciada nele não podem lê-la, mesmo com o link, e ela nunca aparece na linha do tempo federada. Os outros servidores a recebem somente para os seus seguidores lá, e a mostram a esses seguidores como uma publicação apenas para seguidores.

- Se você mencionar alguém de outro servidor que não segue você, essa pessoa não recebe a publicação, e os seus seguidores lá veem a menção como um link comum. O formulário avisa você sobre isso.
- Contas do seu servidor podem impulsionar uma publicação de “Este servidor”, e o impulso aparece somente neste servidor.
- Uma resposta a uma publicação de “Este servidor” também começa como “Este servidor”; você pode mudar isso.

<!-- source: other-things-to-know d1f569ac2d1d -->
### Outras coisas a saber

- Você não pode mudar a visibilidade de uma publicação depois de publicá-la. Ao editar uma publicação, a visibilidade e a publicação à qual ela responde permanecem como estão.
- Um moderador pode limitar o alcance das publicações de uma conta. O formulário então diz “Sua conta pode publicar, no máximo, como …”, e uma escolha mais ampla é publicada no nível mais amplo permitido.
- Qualquer pessoa que possa ver uma publicação pública, de “Este servidor” ou não listada pode impulsioná-la. Somente você pode impulsionar as suas próprias publicações de “Seguidores”, e somente os seus seguidores veem esse impulso. Publicações de “Pessoas mencionadas” não podem ser impulsionadas.
- O autor de uma publicação pode escolher quem pode responder a ela e quem pode impulsioná-la; publicações de alguns outros servidores também trazem essa escolha. Uma resposta ou um impulso fora dela é recusado, com uma mensagem como “O autor desta publicação não permite que você responda a ela.” O autor, as pessoas que a publicação menciona e o autor da publicação a que ela responde sempre podem responder.
- O autor também pode permitir que algumas pessoas respondam ou impulsionem apenas depois que ele aprovar. Uma resposta ou um impulso assim fica aguardando: você é notificado (“… pediu para responder à sua publicação”), e ele fica fora da sua publicação até você aprová-lo. Uma resposta em espera vinda de outro servidor não está na conversa sob a sua publicação nem na contagem de respostas dela; ela ainda aparece no perfil do autor e na própria página. Um impulso em espera não é contado. Aprová-lo o adiciona à sua publicação; recusá-lo o mantém fora, e o outro servidor é informado nos dois casos.
- Uma resposta ou um impulso vindo de outro servidor que esteja totalmente fora da sua escolha é mantido fora da sua publicação do mesmo modo, e não notifica você de nada.
- Quando você responde ou impulsiona uma publicação cujo autor aprova respostas ou impulsos manualmente, a sua fica aguardando essa aprovação. Até lá, só você vê a sua resposta, sob a publicação e na própria página, marcada como em espera; os outros não veem nada dela, e nada é enviado a outros servidores. O seu impulso não é feito até ser aprovado. Você é notificado quando o autor aprova ou recusa (“… aprovou a sua resposta, o seu impulso ou a sua citação”). Excluir uma resposta em espera, ou desfazer um impulso em espera, retira o pedido.
- Você pode fixar até cinco das suas publicações no seu perfil com “Fixar no perfil”. Uma publicação de “Pessoas mencionadas” não pode ser fixada.

<!-- source: who-can-reply-boost-and-quote c9ea034f7416 -->
## Quem pode responder, impulsionar e citar

O botão “Quem pode responder, impulsionar e citar”, entre as ferramentas do formulário, abre as escolhas para aquela publicação. Para cada um entre respostas, impulsos e citações, escolha quem pode fazê-lo de imediato: “Qualquer pessoa”, “Seguidores e pessoas que você menciona”, “Pessoas que você menciona” ou “Somente você”. Abaixo de cada um, “Outras pessoas podem responder com a sua aprovação” (e o mesmo para impulsos e citações) permite que todas as outras pessoas peçam em vez de serem recusadas. Quando uma escolha admite menos que qualquer pessoa, o formulário mostra o resultado acima das ferramentas, por exemplo “Respostas: qualquer pessoa. Impulsos: somente você. Citações: qualquer pessoa.”, antes de você publicar.

- As pessoas que uma publicação menciona sempre podem responder a ela. Uma publicação de “Seguidores” só pode ser impulsionada e citada por você, e uma de “Pessoas mencionadas” por ninguém, qualquer que seja a sua escolha.
- Os padrões para novas publicações ficam em “Quem pode responder, impulsionar e citar suas novas publicações” em “Exibição, publicação e linhas do tempo” nas configurações.
- Para alterá-los em uma publicação que você já enviou, escolha “Alterar quem pode responder, impulsionar e citar” no menu “Mais ações” dela. Cada escolha é salva na hora. As respostas, os impulsos e as citações que você aprovou antes permanecem, e os pedidos que a nova escolha admite são aprovados.
- Os pedidos que aguardam você são listados em “Solicitações de aprovação” nas configurações, ao lado de “Pedidos para seguir”; a notificação de cada um leva até lá com “Analisar as solicitações”. “Aprovar” adiciona a resposta, o impulso ou a citação à sua publicação, e a conversa sob ela mostra na hora uma resposta aprovada; “Recusar” a mantém fora.
- Na publicação de outra pessoa, “Responder” não é mostrado e “Impulsionar” não pode ser pressionado quando o autor não permite. Quando o autor os aprova manualmente, o formulário avisa antes de você responder, e depois que você impulsiona, a publicação diz “Seu impulso aguarda a aprovação do autor.”

<!-- source: after-posting dc553fa0ead9 -->
## Depois de publicar

Sob uma publicação você encontra “Responder”, “Impulsionar”, “Citar”, “Favoritar” e “Salvar”. O menu “Mais ações” tem “Copiar link” e, para as suas publicações, “Editar”, “Fixar no perfil” e “Excluir”. Uma publicação que foi editada mostra “Editada”, e as versões anteriores estão na página “Histórico de edições”. “Excluir” remove a publicação para todos, junto com os seus impulsos e favoritos.

Para uma publicação de outra pessoa, o mesmo menu oferece seguir, silenciar, bloquear ou denunciar o autor, além de “Enviar mensagem direta a …”. Os seus itens salvos aparecem na página “Itens salvos”; somente você os vê.

Quando o servidor oferece tradução, ou o seu navegador pode traduzir, as publicações em outros idiomas têm “Traduzir” entre as ações e no menu “Mais ações”, que o contém quando a publicação é estreita demais para mais um botão. Enquanto uma publicação mostra a tradução, uma linha sob o texto diz de que idioma ela foi traduzida e por quem, com “Mostrar original” para voltar. Se o servidor envia o texto das publicações a um serviço de tradução, o seletor de visibilidade informa para quais escolhas isso acontece.

<!-- source: writing-an-article 0dfa0a80bb8e -->
## Escrever um artigo

“Escrever um artigo”, sob o formulário de nova publicação, abre o editor de artigos, uma página própria. Dê ao artigo um título, um resumo se quiser (se ficar vazio, ele é feito a partir do início do corpo), uma imagem de capa com a descrição dela e o corpo, escrito em Markdown: `#` inicia um título, `**text**` é negrito, e “Negrito” e “Link” acima do corpo fazem o mesmo com o texto que você selecionar (Ctrl+B e Ctrl+K, ou Cmd em um Mac). Solte ou cole imagens no corpo, ou escolha-as com “Adicionar imagens”: cada uma é enviada e entra onde está o cursor como `![](media:…)`; escreva a descrição dela entre os colchetes. Só as imagens que você envia aparecem em um artigo; uma imagem de outro endereço se torna um link com a descrição dela.

- As hashtags vão em um campo próprio, separadas por espaços; o artigo é listado nelas, e um `#` no corpo continua sendo texto. As menções no corpo levam às pessoas, mas não as notificam.
- “Prévia” mostra o artigo como os leitores o verão, renderizado pelo servidor exatamente como será publicado, com o tamanho do corpo e o número de imagens em relação aos limites deste servidor.
- “Marcar como sensível” mostra aos leitores primeiro o título e o resumo e recolhe a capa e o corpo. Um artigo não pode ser somente de “Pessoas mencionadas”.
- Enquanto você escreve, o artigo é salvo como rascunho a cada pausa (“Rascunho salvo”), então ele abre em qualquer dispositivo a partir de “Rascunhos e publicações agendadas”, onde “Editar” o traz de volta ao editor. Se o rascunho foi alterado em outro dispositivo nesse meio-tempo, nada é sobrescrito: seu texto continua no editor, e “Salvar como novo rascunho” o guarda como outro rascunho.
- “Publicar” publica o artigo agora e abre a página dele; “Agendar” o publica no horário que você escolher, como em uma publicação.
- Para alterar um artigo que você publicou, escolha “Editar” no menu “Mais ações” dele. O editor abre com o que você escreveu; “Salvar alterações” o substitui, e a versão anterior continua no “Histórico de edições” dele. A visibilidade de um artigo não muda.

Um servidor pode permitir que apenas algumas pessoas escrevam novos artigos; então “Escrever um artigo” não é mostrado às outras, que ainda podem editar os artigos que publicaram.

<!-- source: reading-articles f2811fffd462 -->
## Ler artigos

Um artigo é uma publicação longa com título, resumo e, muitas vezes, uma imagem de capa, escrita neste servidor ou em outro. As linhas do tempo e os perfis mostram um artigo como um cartão com a capa, a palavra “Artigo”, o título e o resumo; selecione o título para ler o artigo inteiro na página própria dele, que o título encabeça. Respostas, impulsos, citações, favoritos e reações funcionam como em qualquer publicação. Os artigos não têm “Traduzir”.

Um artigo que o autor marcou como sensível mostra o título e o resumo e oculta o resto: no cartão, “Mostrar mídia sensível” mostra a capa, e na página dele “Mostrar o artigo” mostra a capa e o texto. A aba “Artigos” de um perfil lista apenas os artigos daquela pessoa, os mais recentes primeiro; para alguém de outro servidor, apenas os artigos que este servidor recebeu.

<!-- source: direct-messages 59062702f4be -->
## Mensagens diretas

“Mensagens diretas”, na navegação, reúne as suas publicações de “Pessoas mencionadas” e as que você recebe, uma linha por conversa, com a de publicação mais recente primeiro. Cada linha mostra quem está na conversa, quando a publicação mais recente dela foi escrita e o início dessa publicação. O número ao lado de “Mensagens diretas” é quantas conversas têm uma publicação que você ainda não leu; cada uma delas tem um ponto antes dos nomes.

Escolha o início de uma publicação para abrir a conversa; isso marca a conversa como lida em todos os seus dispositivos. Uma resposta a uma publicação de “Pessoas mencionadas” também é sempre de “Pessoas mencionadas”, então não alcança ninguém que a conversa não alcançava. Em celulares, “Mensagens diretas” fica em “Menu”, a menos que você o mova para a barra de abas em “Exibição, publicação e linhas do tempo” nas configurações.

<!-- source: deleting-old-posts-automatically e581c8d27380 -->
## Excluir publicações antigas automaticamente

“Excluir publicações antigas” nas configurações exclui as suas publicações assim que elas ficam mais antigas que a idade que você escolher, como se você as tivesse excluído manualmente: elas somem daqui e dos servidores para os quais foram enviadas, e não podem ser restauradas. As publicações que você mantém (fixadas, salvas ou favoritadas por você, com mídia ou enquete, artigos, de algumas visibilidades, ou com muitos impulsos ou favoritos) permanecem. Os seus impulsos de publicações de outras pessoas nunca são excluídos. O servidor verifica a cada hora e exclui no máximo 100 das suas publicações de cada vez, então muitas publicações antigas levam várias horas para sumir.

Uma conta nova começa com as configurações que o servidor escolheu para contas novas; a partir daí elas são suas, e você pode alterá-las como qualquer outra configuração.

Um servidor também pode manter publicações por no máximo alguns dias. A página “Sobre” do servidor informa isso antes que você se cadastre. Sob esse limite, a exclusão automática fica sempre ativa para você, a idade escolhida não pode ser maior que o limite, e somente os tipos de publicação que o servidor permite podem ser mantidos (por padrão, apenas as publicações fixadas, então seus artigos também são excluídos, a menos que o servidor permita mantê-los). Alguns servidores permitem que algumas pessoas mantenham as suas publicações por mais tempo, por exemplo como um benefício de um plano de apoiador.

Quando um limite passa a valer para você (o servidor o introduz ou o reduz, ou você perde o direito de manter publicações por mais tempo), você recebe uma notificação e um e-mail, a menos que tenha desativado essa opção, informando a partir de quando ele vale. Até lá, as suas próprias configurações continuam em vigor. A página de configurações mostra a data e, com a sua prévia, quais das suas publicações serão excluídas nesse momento.
