<!-- source: supporters-paid-plans ea0039323475 -->
# Apoiadores (planos pagos)

Somente quando um administrador ativa o programa, usuários locais podem assinar um plano pago na web e manter a função vinculada ao plano enquanto continuarem pagando. O programa vem desativado por padrão, e nenhum plano ou ponto de entrada de assinatura aparece em lugar nenhum até que ele seja ativado.
A chave de implantação `payments` é descrita na [referência de configuração](../reference/configuration.md#payments).

**O mycroblog não garante conformidade com nenhuma lei, incluindo a Lei de Transações Comerciais Específicas do Japão (Act on Specified Commercial Transactions).** Os operadores devem verificar o conteúdo da divulgação, a tela final de confirmação e o tratamento do imposto sobre consumo e das notas fiscais segundo as leis do próprio país (veja "O que os operadores devem verificar" abaixo).

<!-- source: how-a-subscription-works 13d7fb3772b2 -->
## Como funciona uma assinatura

![Como uma assinatura flui: o usuário paga no provedor de pagamento, os eventos do provedor fazem o servidor ler o estado de volta, e o servidor concede ou remove a função do plano e registra a receita. As etapas estão listadas abaixo.](images/supporter-flow.svg)

De cima para baixo no diagrama:

- **Assinando.** O usuário escolhe um plano e um intervalo (mensal ou anual) em `/settings/supporter`. Após a confirmação final, o servidor (`my serve`) cria a página de pagamento no provedor definido em `payments` (`stripe` ou `payjp`), e o usuário paga nessa página.
- **Eventos.** O provedor envia seus eventos ao endpoint de webhook `POST /payments/webhooks/{provider}`, que registra cada evento uma vez e enfileira um job. Um evento é apenas um sinal: o servidor nunca tira o estado do conteúdo dele.
- **Leitura de volta.** O job `supporters.apply_payment_event`, e a reconciliação `supporters.reconcile_payments` a cada `payments_reconcile_interval`, leem a assinatura e as movimentações de dinheiro de volta da API do provedor. A reconciliação repara qualquer evento que não chegou.
- **O estado e a função.** O servidor deriva o estado da assinatura a partir do que leu e de cada prazo que já passou (o período de carência de 7 dias, por exemplo), e concede ou remove a função do plano com ele.
- **Receita.** As movimentações de dinheiro se tornam os registros de receita (valor bruto, taxa e líquido de cada pagamento, reembolso, chargeback e taxa). Os totais aparecem na tela financeira (`/finance`, para as pessoas com `view_finance`) e, na medida em que `income_visibility` permitir, em `/about/costs` (veja [instance-settings.md](instance-settings.md)).

![Os estados de uma assinatura: active, canceling e past_due mantêm a função do plano; pending, lapsed, expired, revoked e abandoned não. As transições estão listadas abaixo.](images/supporter-states.svg)

Os estados de uma assinatura, como a administração e `my admin supporters subscriptions` os mostram:

- `pending`: iniciada, ainda não paga. Sem função. O primeiro pagamento torna a assinatura `active`, e uma assinatura que termina antes do primeiro pagamento vira `abandoned`.
- `active`: paga. Tem a função do plano, que cada renovação mantém.
- `past_due`: um pagamento falhou. Mantém a função por 7 dias a partir da falha; um pagamento nesse período a torna `active` de novo.
- `lapsed`: passaram 7 dias desde a falha. A função é removida. Um pagamento depois disso a torna `active` de novo, e a função volta.
- `canceling`: cancelada ao final do período. Mantém a função até lá, e então vira `expired`.
- `expired`, `revoked` (um reembolso total ou um chargeback) e `abandoned` são finais e não têm função.

<!-- source: enabling-the-program b07e6c07d73b -->
## Ativando o programa

1. Defina a chave de implantação `payments` com o seu provedor de pagamentos e passe as chaves do provedor e o segredo do webhook. As etapas específicas de cada provedor estão em [payments-stripe.md](payments-stripe.md) e [payments-payjp.md](payments-payjp.md). O programa não pode ser ativado enquanto a chave é `none`.
2. Configure o servidor para enviar e-mail (`mail` diferente de `none`). Apenas usuários com um endereço de e-mail confirmado podem assinar, então um servidor que não envia e-mail não pode ativar o programa.
3. Na página “Informações comerciais” de “Termos e políticas” na administração (`/admin/documents/commerce-disclosure`), escreva as informações comerciais (para o Japão, o aviso exigido pela Lei de Transações Comerciais Específicas), o texto da instância `commerce_disclosure`, em Markdown. Ele é publicado em `/about/commerce`, e você pode registrar essa URL no painel do provedor.
   O programa pode ser ativado com o texto vazio, mas a administração mostra um aviso.
4. Em “Papéis” na administração, crie o papel a vincular ao plano. Ele não pode ter titulares, apenas permissões de membros (grupo `member`), e uma `position` menor que a de todo papel da equipe, `moderator` inclusive.
   As permissões de membros são os benefícios que um plano pode dar: `create_custom_emojis` (solicitar emojis personalizados), `view_finance` (ler as finanças do servidor), `start_voice_rooms` (abrir salas de chat de voz enquanto só usuários autorizados podem), `create_invites` (convidar pessoas enquanto só usuários autorizados podem, dentro dos limites dos [convites de membros](registration.md#member-invites)), `keep_posts_longer` (manter publicações além do [máximo da exclusão automática](instance-settings.md#automatic-deletion-of-old-posts) e desativar a exclusão automática enquanto o máximo está ativo), `create_articles` (escrever artigos enquanto só usuários autorizados podem; veja [artigos](instance-settings.md#articles)) e `create_events` (criar [eventos](instance-settings.md#events) enquanto só usuários autorizados podem).
   Quando a assinatura termina, o papel sai, e os benefícios também: os convites que o apoiador fez deixam de funcionar até ele assinar de novo, e o máximo da exclusão automática volta a se aplicar às publicações dele depois do período de carência.
5. Em “Apoiadores” na administração (permissão `manage_supporters`), abra a página “Planos” (`/admin/supporters/plans`) e escolha “Criar um plano”. Defina o nome, a descrição, o preço mensal, o desconto para pagamento anual, o limite mensal de criação de emojis personalizados (ele é redefinido no primeiro dia de cada mês, UTC; veja [emojis personalizados](instance-settings.md#custom-emoji)), a ordem e a visibilidade, e escolha o papel: o formulário lista os papéis que podem receber um plano ou, sem a permissão `manage_roles`, pede a chave do papel.
   O servidor cria os preços mensal e anual no provedor (não crie preços no painel do provedor). Até os preços existirem, o plano fica “em preparação” e não aceita assinaturas.
6. Na primeira página de “Apoiadores” (`/admin/supporters`), escolha “Ativar o programa”. A página diz o que falta enquanto o programa não pode ser ativado e mostra o provedor de pagamentos da configuração, que só a configuração altera.

Depois que um plano existe, o papel dele não pode mais ser concedido nem revogado manualmente, e você altera as permissões e a posição dele na página do plano (escolha o plano na página “Planos”). Um plano mais alto (uma ordem maior) fica acima dos papéis dos planos mais baixos, tem todas as permissões deles e tem um limite de emojis pelo menos igual ao deles.

<!-- source: setup-at-the-provider 5821be7cae27 -->
## Configuração no provedor

- **Desativação**: Um assinante não pode desativar a conta enquanto a assinatura está ativa, inclusive uma definida para terminar no fim do período. A tela de configurações o leva à assinatura, para que ninguém seja cobrado enquanto está oculto.
- **Novas tentativas (dunning)**: Sete dias depois que um pagamento falha, o servidor remove o papel (independentemente das novas tentativas do provedor). Defina o período de novas tentativas do provedor como no mínimo 7 dias e a ação final como “cancelar”. Quando o pagamento tem sucesso depois do período de carência, o papel volta.
- **Verificando o provedor**: a seção “Provedor de pagamentos” da primeira página de “Apoiadores” mostra o modo do provedor (teste ou produção), a última entrega de webhook e a última reconciliação com o motivo de ela ter parado, e avisa quando o webhook parece não estar configurado. “Reconciliar agora” executa uma reconciliação na hora, no máximo uma vez por hora.
- **Portal do cliente**: O portal é aberto para formas de pagamento e faturas. Desative as mudanças de plano no portal e permita apenas o cancelamento no fim do período. As mudanças de plano e os cancelamentos são feitos nas telas do próprio servidor.
- **Reembolsos**: O servidor reembolsa apenas um pagamento que não comprou nada: uma segunda assinatura de um pagamento e um pagamento de uma assinatura pendente que foi encerrada. O guia do provedor ([Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md)) descreve os dois. Todo outro reembolso é do operador, no painel do provedor. Um reembolso total e um chargeback removem o papel na hora e cancelam a assinatura do provedor na hora. Um reembolso parcial não remove o papel. Ganhar um chargeback não restaura o papel; o usuário assina de novo.

<!-- source: what-users-see 29a1564c7109 -->
## O que os usuários veem

- Na web, o usuário abre "Planos de apoio" (`/settings/supporter`), escolhe um plano e um intervalo de pagamento (mensal ou anual), passa por uma tela final de confirmação e segue para a página de pagamento do provedor. Os dados do cartão nunca passam por este servidor. A página de pagamento retorna a esta tela, que então lê de novo o estado da assinatura.
  Enquanto assinado, o usuário pode mudar de plano pela mesma tela (um upgrade e a troca de mensal para anual passam a valer quando a diferença proporcional for paga, enquanto um downgrade e a troca de anual para mensal passam a valer ao final do período), cancelar uma mudança agendada, cancelar a assinatura (os benefícios duram até o fim do período), desfazer um cancelamento e abrir o portal do cliente para métodos de pagamento e faturas.
  Quando o pagamento de uma mudança falha, ou quando o cartão pede uma autenticação que a mudança não pode pedir, o plano continua o mesmo e o usuário vê um erro com o caminho para atualizar o cartão. Isso não inicia o período de carência nem envia a notificação de pagamento falho; uma renovação falha ainda envia.
- Só usuários deste servidor que tenham endereço de e-mail confirmado e uma conta que não esteja suspensa nem agendada para exclusão podem assinar. Um usuário pode ter um plano por vez.
- `/about` mostra uma seção com os planos públicos. Nos apps móveis oficiais, a tela de assinatura, a seção de planos de `/about` e os links para eles não aparecem. Os benefícios (selo, emoji personalizado, finanças) funcionam no app também.

<!-- source: notifications-to-subscribers 73f507490bca -->
## Notificações para os assinantes

Os assinantes são avisados uma vez sobre cada um dos casos a seguir, por notificação, Web Push e e-mail (para o endereço principal, no idioma do usuário). Um assinante cuja conta foi desativada não recebe nada, nem na hora nem ao voltar.

- Falha de pagamento (`supporter_payment_failed`): quando a assinatura fica em atraso (`past_due`). Ela informa o fim do período de carência (em UTC no e-mail) e um link para a tela onde o método de pagamento pode ser corrigido (`/settings/supporter`).
- Fim da assinatura (`supporter_subscription_ended`): quando uma assinatura deixa um estado com benefícios porque o período de carência acabou, ela expirou, ou terminou com um reembolso ou um chargeback. A mensagem informa o motivo. Um término causado pela exclusão da conta não é anunciado.
- Retirada do plano (`supporter_plan_retired`): para os assinantes de um plano retirado (exceto os pendentes). Diz que a assinatura não será renovada.
- Anúncio do encerramento do serviço (`supporter_service_closure`): para os assinantes cuja assinatura foi cancelada ao final do período quando o anúncio começou (exceto os pendentes). Diz que a assinatura não será renovada. Nenhuma nova assinatura é aceita durante o anúncio.

O texto das notificações Web Push e das notificações no app não leva à tela de pagamento. Só o cliente web põe um link para a tela de assinatura em uma notificação; os apps móveis oficiais não. Os links nos e-mails abrem o cliente web no navegador.

<!-- source: cli 555871b40822 -->
## CLI

`my admin supporters` faz parte das operações da administração (`my admin supporters -help`).

- `status`: se o programa está ativado, o que ele precisa para ser ativado (um provedor, e-mail) e o número de assinaturas em andamento.
- `enable`, `disable`: ativa e desativa o programa. Desativar é recusado enquanto houver qualquer assinatura em andamento.
- `plans`: a lista de planos (inclusive os retirados), com o número de planos em preparação, retirados e de assinaturas.
- `subscriptions`: a lista de assinaturas (filtre com `-plan` e `-state`). `cancel <subscription ID>` cancela uma assinatura de imediato (sem reembolso, exceto um pagamento de assinatura pendente, que não comprou nada).

Ativar, desativar e cancelar são gravados no registro de auditoria como operações de CLI. Criar, alterar e retirar planos é feito na administração ou pela API administrativa.

<!-- source: the-list-of-members df4c915636dc -->
## A lista de membros

- A configuração da instância `supporter_list_visibility` (a seção “Lista de apoiadores” da primeira página de “Apoiadores”, para um administrador com `manage_settings`; veja [instance-settings.md](instance-settings.md)) escolhe quem pode ver a lista de assinantes atuais (`/about/supporters`). Com o padrão `off`, nem a lista nem o link a partir de `/about` aparecem.
- A lista mostra apenas os assinantes que escolheram ser listados na página “Selos e lista de apoiadores” das configurações deles (por padrão ninguém é listado). Ela mostra o nome do plano e o avatar, o nome de exibição e o identificador de cada pessoa, e nunca valores, se o pagamento é mensal ou anual, datas nem contagens.
- A escolha de ser listado permanece quando o assinante muda de plano e desaparece quando a assinatura termina (o papel é removido). Quem assina de novo começa sem estar listado.
- A página da lista e a API dela sempre enviam `noindex, noarchive`, diga o que disser a configuração dos mecanismos de busca (`search_indexing`).

<!-- source: changing-prices-hiding-plans-and-retiring-plans fdeeb52e52bb -->
## Alterando preços, ocultando planos e aposentando planos

- Quando você altera o preço mensal ou o desconto, o novo preço se aplica a novas assinaturas e mudanças de plano. Os assinantes existentes renovam pelo preço original. Não há operação para mover os assinantes existentes para um novo preço. Para movê-los, crie um novo plano e aposente o antigo.
- Ocultar um plano interrompe novas assinaturas e mudanças para esse plano. As assinaturas existentes continuam.
- Aposentar um plano (“Aposentar o plano…” na página dele, depois de digitar o nome) não pode ser desfeito. As assinaturas dele são canceladas no fim do período (as pendentes são encerradas na hora e a página de pagamento delas deixa de aceitar pagamentos, com qualquer pagamento que ainda chegue reembolsado; e as de pagamento atrasado além do período de carência são canceladas na hora). Quando a última assinatura termina, o papel volta a ser um papel comum.

<!-- source: accounts-and-subscriptions 480ba9a583b1 -->
## Contas e assinaturas

- Excluir uma conta e bani-la (o que agenda a exclusão) cancelam as assinaturas dela na hora, sem reembolso (um pagamento de uma assinatura ainda pendente não comprou nada e é reembolsado). Reverter um banimento não restaura as assinaturas.
- Suspender uma conta agenda o cancelamento no fim do período. Levantar a suspensão não o restaura automaticamente; o usuário cancela o cancelamento antes do fim do período ou assina de novo.
- Na página “Assinaturas” de “Apoiadores” (`/admin/supporters/subscriptions`, filtrada por plano e estado), um administrador pode abrir uma assinatura e encerrá-la na hora com “Encerrar agora…”, depois de digitar o identificador do assinante (sem reembolso, exceto um pagamento de uma assinatura pendente, que não comprou nada; isso é gravado no log de auditoria).

<!-- source: disabling-the-program-and-changing-providers ab1548c938f8 -->
## Desativando o programa e trocando de provedor

- O programa não pode ser desativado enquanto houver qualquer assinatura em andamento (pendente, ativa, programada para cancelar, em atraso ou além do período de carência). Retire todos os planos primeiro e desative o programa depois que as assinaturas tiverem terminado.
- Para trocar de provedor, desative o programa, altere `payments` e ative o programa de novo com o novo provedor. Os assinantes atuais assinam de novo. Enquanto o provedor configurado for diferente daquele com o qual o programa foi ativado, o servidor recusa novas assinaturas e mudanças, e a administração mostra um aviso.

<!-- source: tips 106dc656074d -->
## Gorjetas

Uma gorjeta é um pagamento único de um usuário local para a operação do servidor, sem nada em troca: ela não dá papel, selo nem outro benefício, e o usuário vê apenas um agradecimento depois que ela é paga. As gorjetas vêm desativadas por padrão, não precisam de plano e vão apenas para o operador: os usuários não podem dar gorjeta uns aos outros, e não há metas, campanhas nem saldos.

- **O que elas exigem**: o programa de apoiadores ativado no provedor configurado e um provedor que aceita pagamentos únicos. A seção **Gorjetas** da primeira página da tela **Apoiadores** (permissão `manage_supporters`) mostra se os usuários podem dar gorjeta agora e avisa quando o provedor não aceita pagamentos únicos; `GET /api/v1/admin/supporter_tips` informa o mesmo: se o seu provedor os aceita (`one_time_payments`), o menor e o maior valor que ele permite em cada moeda (`limits`) e se os usuários podem dar gorjeta agora (`accepting`). O Stripe os aceita (veja [Gorjetas no Stripe](payments-stripe.md#tips)), o PAY.JP os aceita na página hospedada dele da API v2 (veja [Gorjetas no PAY.JP](payments-payjp.md#tips)), e o provedor de teste das builds de teste também.
- **Valores**: você escolhe de 1 a 6 valores em uma moeda, iene ou dólar americano, cada um de ¥100 a ¥100.000 ou de US$1 a US$1.000 e no mínimo a menor cobrança única do provedor. Os usuários escolhem um deles e nunca digitam um valor; o piso evita que uma gorjeta pequena custe mais do que rende quando é contestada.
- **Ativando, alterando e desativando**: na seção **Gorjetas**, marque **Aceitar gorjetas**, escolha a moeda, escreva os valores um por linha e selecione **Salvar as gorjetas**; desmarque a caixa e salve para desativá-las. A API é `PUT /api/v1/admin/supporter_tips` com `{"enabled": true, "amounts": [{"amount": "500", "currency": "JPY"}, ...]}`, ou `{"enabled": false}`. Cada mudança é gravada no log de auditoria (`supporter_tips_enabled`, `supporter_tips_revised`, `supporter_tips_disabled`). Desativar o programa de apoiadores interrompe as gorjetas também; um checkout já aberto continua pagável até terminar, e um pagamento nele ainda é registrado.
- **Quem pode dar gorjeta**: usuários deste servidor com um endereço de e-mail confirmado cuja conta não está suspensa nem agendada para exclusão. Uma assinatura não é necessária. As iniciações são limitadas a 5 por hora e 20 por dia para cada usuário, e a 100 por hora para o servidor inteiro, o que impede o teste de cartões (tentar cartões roubados com pequenos pagamentos); `server_limit_reached` diz que o limite do servidor foi atingido na última hora.
- **Pagando**: os usuários chegam à página de gorjeta (`/settings/supporter/tip`) pela seção **Gorjetas** da página sobre, sob o medidor de financiamento na coluna lateral, e pela página de planos de apoiador; um visitante que não está conectado é convidado a entrar primeiro. A página confirma o valor, que é pago uma vez, de uma vez, e que nada é dado em troca, com um link para as suas informações comerciais, antes da página do provedor. A página hospedada do provedor recebe o único pagamento, apenas por cartão, e continua pagável por uma hora. Um usuário tem no máximo uma gorjeta aberta: iniciar outra torna a página anterior impagável primeiro, e quando a anterior foi paga nesse meio tempo, o usuário é levado ao agradecimento dela em vez de pagar duas vezes. Nos apps móveis oficiais, as gorjetas não aparecem.
- **Receita**: uma gorjeta paga é contabilizada uma vez no mês em que foi paga, seja o evento do provedor, o retorno da página ou a reconciliação que a informe primeiro. Os reembolsos e chargebacks dela são contabilizados no mês próprio e subtraem da receita. Os totais dos administradores mostram separadamente o líquido das assinaturas, das gorjetas e do outro dinheiro da conta do provedor, e a lista de movimentações de dinheiro deles mostra quem pagou cada gorjeta enquanto a conta existe. Os totais públicos, `/finance` e `/about/costs` nunca mostram uma gorjeta individual, o valor das gorjetas nem quantas houve; com `income_visibility` definido como `plans`, a lista de planos termina com uma linha “Gorjetas” sem número.
- **Reembolsos e disputas**: o servidor não tem operação de reembolso para gorjetas. Reembolse no painel do provedor; um reembolso total, um chargeback e uma disputa ganha são registrados com a gorjeta, e um reembolso parcial apenas na receita. Cada disputa também custa a taxa de disputa do provedor (no Stripe no Japão, ¥1.500 no momento em que isto foi escrito), seja qual for o valor da gorjeta.
- **Contas**: excluir uma conta e bani-la torna impagável a página da gorjeta aberta dela. Apagar a conta exclui as gorjetas dela; a receita permanece, sem o usuário. O provedor guarda o pagamento e o endereço de e-mail que preencheu a página dele, o que o servidor não pode excluir: diga isso na sua política de privacidade.

A métrica `my.supporter.tip_checkouts` conta as iniciações (`started`), as iniciações que os limites recusaram (`rate_limited`) e as gorjetas que foram pagas (`paid`) ou terminaram sem pagamento (`expired`). Dois exemplos de alertas:

- Um pico de iniciações: `sum(rate(my_supporter_tip_checkouts_total{my_outcome=~"started|rate_limited"}[15m]))` bem acima do nível habitual por 15 minutos.
- Muitas iniciações que nunca são pagas: `sum(increase(my_supporter_tip_checkouts_total{my_outcome="expired"}[1h])) / sum(increase(my_supporter_tip_checkouts_total{my_outcome="started"}[1h]))` acima de 0.8 com pelo menos 20 iniciações.

Se um provedor aceita gorjetas, cabe a você conferir com ele, conforme a análise e os termos dele. Redija todo texto das gorjetas, as suas informações comerciais inclusive, como uma gorjeta para manter o serviço, nunca como doação, dedução nem arrecadação para uma causa.

<!-- source: what-operators-must-check-not-yet-verified 4981ac7a2d03 -->
## O que os operadores devem verificar (ainda não verificado)

O mycroblog não julga nenhum destes pontos. Verifique-os contra as leis do seu país e os termos do seu provedor.

- Os itens que a divulgação comercial deve conter (o nome, o endereço e o telefone do vendedor, o preço, quando e como pagar, quando o serviço é prestado, como cancelar, e assim por diante) e como ela deve ser publicada.
- Que a tela final de confirmação (artigo 12-6 da Lei de Transações Comerciais Específicas) mostre a quantidade, o preço, o momento do pagamento, o momento da entrega e a forma de cancelar e o efeito disso, considerando em conjunto a página de pagamento do provedor e a tela de confirmação do `my`.
- O tratamento do imposto sobre consumo, e se você emite notas fiscais qualificadas.
- Para as gorjetas: se o seu provedor aceita gorjetas para manter um serviço (o PAY.JP conta “gorjetas” entre os bens que considera difíceis de aprovar, e o Stripe no Japão pode não aceitá-las para um empresário individual; pergunte ao provedor), o que a divulgação comercial diz sobre elas (o que é uma gorjeta, os valores, que é paga na hora, que nada é fornecido em troca e como os reembolsos são tratados), e o imposto sobre consumo e o imposto de renda delas, que um contador tributarista deve confirmar.
