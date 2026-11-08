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

<!-- source: enabling-the-program fe86b2e59df5 -->
## Ativando o programa

1. Defina a chave de implantação `payments` com o seu provedor de pagamento e informe as chaves do provedor e o segredo do webhook. As etapas específicas de cada provedor estão em [payments-stripe.md](payments-stripe.md) e [payments-payjp.md](payments-payjp.md). O programa não pode ser ativado enquanto a chave for `none`.
2. Configure o servidor para enviar e-mail (`mail` diferente de `none`). Só usuários com endereço de e-mail confirmado podem assinar, então um servidor que não envia e-mail não pode ativar o programa.
3. Na página "Divulgações comerciais" de "Termos e políticas" na administração (`/admin/documents/commerce-disclosure`), escreva a divulgação comercial (no Japão, o aviso exigido pela Lei de Transações Comerciais Específicas), que é o texto da instância `commerce_disclosure`, em Markdown. Ela é publicada em `/about/commerce`, e você pode cadastrar essa URL no painel do provedor.
   O programa pode ser ativado com o texto vazio, mas a administração mostra um aviso.
4. Em "Funções" na administração, crie a função a ser vinculada ao plano. Ela não pode ter titulares, apenas permissões de membro (grupo `member`), e uma `position` menor que a de cada função de equipe, incluindo `moderator`.
   As permissões de membro são os benefícios que um plano pode conceder: `create_custom_emojis` (solicitar emoji personalizado), `view_finance` (ler as finanças do servidor), `start_voice_rooms` (abrir salas de voz, enquanto só usuários permitidos podem), `create_invites` (convidar pessoas, enquanto só usuários permitidos podem, dentro dos limites de [convites de membros](registration.md#member-invites)) e `keep_posts_longer` (manter publicações além do [máximo da exclusão automática](instance-settings.md#automatic-deletion-of-old-posts), e desligar a exclusão automática enquanto o máximo estiver ativo).
   Quando a assinatura termina, a função sai, e os benefícios também: os convites feitos pelo apoiador deixam de funcionar até ele assinar de novo, e o máximo da exclusão automática volta a valer para as publicações dele depois do seu período de carência.
5. Em "Apoiadores" na administração (permissão `manage_supporters`), abra a página "Planos" (`/admin/supporters/plans`) e escolha "Criar um plano". Defina o nome, a descrição, o preço mensal, o desconto para o pagamento anual, o limite mensal de criação de emoji personalizado (ele é zerado no primeiro dia de cada mês, em UTC; veja [emoji personalizados](instance-settings.md#custom-emoji)), a ordem e a visibilidade, e escolha a função.
   O servidor cria os preços mensal e anual no provedor (não crie preços no painel do provedor). Até que os preços existam, o plano fica em preparação e não aceita assinaturas.
6. Na página "Programa" de "Apoiadores" (`/admin/supporters`), marque "Ativar o programa de apoio" e escolha "Salvar". O mesmo formulário define quem pode ler a lista de apoiadores.

Depois que um plano existe, a função dele não pode mais ser concedida nem revogada manualmente, e você altera as permissões e a posição dela na página do plano (escolha o plano na página "Planos"). Um plano superior (uma ordem maior) fica acima das funções dos planos inferiores, tem todas as permissões deles e um limite de emoji pelo menos tão alto quanto o deles.

<!-- source: setup-at-the-provider 74efcb74cf80 -->
## Configuração no provedor

- **Desativação**: um assinante não pode desativar a conta enquanto a assinatura estiver ativa, inclusive quando ela estiver programada para terminar ao fim do período. A tela de configurações leva o assinante à assinatura, para que ninguém seja cobrado sem perceber.
- **Novas tentativas (dunning)**: sete dias depois que um pagamento falha, o servidor remove a função (independentemente das novas tentativas do provedor). Defina o período de novas tentativas do provedor em pelo menos 7 dias e faça a ação final ser "cancelar". Quando o pagamento for bem-sucedido depois do período de carência, a função volta.
- **Portal do cliente**: o portal é aberto para métodos de pagamento e faturas. Desative as mudanças de plano no portal e permita apenas o cancelamento ao final do período. Mudanças de plano e cancelamentos são feitos nas telas do próprio servidor.
- **Reembolsos**: o servidor reembolsa apenas um pagamento que não comprou nada: uma segunda assinatura de um pagamento, e um pagamento de uma assinatura pendente que foi encerrada. O guia do provedor ([Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md)) descreve os dois casos. Todo outro reembolso é do operador, no painel do provedor. Um reembolso total e um chargeback removem a função imediatamente e cancelam a assinatura no provedor de imediato. Um reembolso parcial não remove a função. Vencer uma disputa de chargeback não restaura a função; o usuário assina de novo.

<!-- source: what-users-see d1216c59368a -->
## O que os usuários veem

- Na web, o usuário abre "Planos de apoio" (`/settings/supporter`), escolhe um plano e um intervalo de pagamento (mensal ou anual), passa por uma tela final de confirmação e segue para a página de pagamento do provedor. Os dados do cartão nunca passam por este servidor. A página de pagamento retorna a esta tela, que então lê de novo o estado da assinatura.
  Enquanto assinado, o usuário pode mudar de plano pela mesma tela (um upgrade e a troca de mensal para anual passam a valer quando a diferença proporcional for paga, enquanto um downgrade e a troca de anual para mensal passam a valer ao final do período), cancelar uma mudança agendada, cancelar a assinatura (os benefícios duram até o fim do período), desfazer um cancelamento e abrir o portal do cliente para métodos de pagamento e faturas.
  Quando o pagamento de uma mudança falha, ou quando o cartão pede uma autenticação que a mudança não pode pedir, o plano continua o mesmo e o usuário vê um erro com o caminho para atualizar o cartão. Isso não inicia o período de carência nem envia a notificação de pagamento falho; uma renovação falha ainda envia.
- Só usuários deste servidor que tenham endereço de e-mail confirmado e uma conta que não esteja suspensa nem agendada para exclusão podem assinar. Um usuário pode ter um plano por vez.
- `/about` mostra uma seção com os planos públicos. No app Tauri, a tela de assinatura, a seção de planos de `/about` e os links para eles não aparecem. Os benefícios (selo, emoji personalizado, finanças) funcionam no app também.

<!-- source: notifications-to-subscribers 91f6ad77e546 -->
## Notificações para os assinantes

Os assinantes são avisados uma vez sobre cada um dos casos a seguir, por notificação, Web Push e e-mail (para o endereço principal, no idioma do usuário). Um assinante cuja conta foi desativada não recebe nada, nem na hora nem ao voltar.

- Falha de pagamento (`supporter_payment_failed`): quando a assinatura fica em atraso (`past_due`). Ela informa o fim do período de carência (em UTC no e-mail) e um link para a tela onde o método de pagamento pode ser corrigido (`/settings/supporter`).
- Fim da assinatura (`supporter_subscription_ended`): quando uma assinatura deixa um estado com benefícios porque o período de carência acabou, ela expirou, ou terminou com um reembolso ou um chargeback. A mensagem informa o motivo. Um término causado pela exclusão da conta não é anunciado.
- Retirada do plano (`supporter_plan_retired`): para os assinantes de um plano retirado (exceto os pendentes). Diz que a assinatura não será renovada.
- Anúncio do encerramento do serviço (`supporter_service_closure`): para os assinantes cuja assinatura foi cancelada ao final do período quando o anúncio começou (exceto os pendentes). Diz que a assinatura não será renovada. Nenhuma nova assinatura é aceita durante o anúncio.

O texto das notificações Web Push e das notificações no app não leva à tela de pagamento. Só o cliente web põe um link para a tela de assinatura em uma notificação; o app Tauri não. Os links nos e-mails abrem o cliente web no navegador.

<!-- source: cli 555871b40822 -->
## CLI

`my admin supporters` faz parte das operações da administração (`my admin supporters -help`).

- `status`: se o programa está ativado, o que ele precisa para ser ativado (um provedor, e-mail) e o número de assinaturas em andamento.
- `enable`, `disable`: ativa e desativa o programa. Desativar é recusado enquanto houver qualquer assinatura em andamento.
- `plans`: a lista de planos (inclusive os retirados), com o número de planos em preparação, retirados e de assinaturas.
- `subscriptions`: a lista de assinaturas (filtre com `-plan` e `-state`). `cancel <subscription ID>` cancela uma assinatura de imediato (sem reembolso, exceto um pagamento de assinatura pendente, que não comprou nada).

Ativar, desativar e cancelar são gravados no registro de auditoria como operações de CLI. Criar, alterar e retirar planos é feito na administração ou pela API administrativa.

<!-- source: the-list-of-members f00ca3b391a3 -->
## A lista de membros

- A configuração da instância `supporter_list_visibility` (na tela "Apoiadores"; veja [instance-settings.md](instance-settings.md)) define quem pode ver a lista de assinantes atuais (`/about/supporters`). Com o padrão `off`, nem a lista nem o link em `/about` aparecem.
- A lista mostra só os assinantes que escolheram aparecer na página "Selos e lista de apoiadores" das suas configurações (ninguém aparece por padrão). Ela mostra o nome do plano e o avatar, o nome de exibição e o identificador de cada pessoa, e nunca valores, se o pagamento é mensal ou anual, datas ou contagens.
- A escolha de aparecer se mantém quando o assinante muda de plano, e desaparece quando a assinatura termina (a função é removida). Quem assina de novo começa sem aparecer na lista.
- A página da lista e a sua API sempre enviam `noindex, noarchive`, qualquer que seja a configuração de busca (`search_indexing`).

<!-- source: changing-prices-hiding-plans-and-retiring-plans a65c92b1c371 -->
## Alterando preços, ocultando planos e retirando planos

- Quando você altera o preço mensal ou o desconto, o novo preço vale para novas assinaturas e mudanças de plano. Os assinantes atuais renovam pelo preço original. Não há operação para mover assinantes atuais para um novo preço. Para isso, crie um plano novo e retire o antigo.
- Ocultar um plano interrompe novas assinaturas e mudanças para esse plano. As assinaturas existentes continuam.
- Retirar um plano não pode ser desfeito. Suas assinaturas são canceladas ao final do período (as pendentes encerram de imediato, e a página de pagamento delas deixa de aceitar pagamentos, com qualquer pagamento que ainda chegue reembolsado; e as cujo pagamento está atrasado além do período de carência são canceladas de imediato). Quando a última assinatura termina, a função volta a ser uma função comum.

<!-- source: accounts-and-subscriptions 8c5e10df3c18 -->
## Contas e assinaturas

- Excluir uma conta e banir (o que agenda a exclusão) cancela as suas assinaturas de imediato, sem reembolso (um pagamento de assinatura ainda pendente não comprou nada, e é reembolsado). Reverter um banimento não restaura as assinaturas.
- Suspender uma conta agenda o cancelamento ao final do período. Levantar a suspensão não o restaura automaticamente; o usuário desfaz o cancelamento antes do fim do período ou assina de novo.
- Na página "Assinaturas" de "Apoiadores" (`/admin/supporters/subscriptions`), um administrador pode abrir uma assinatura e encerrá-la de imediato com "Encerrar agora" (sem reembolso, exceto um pagamento de assinatura pendente, que não comprou nada; a ação fica gravada no registro de auditoria).

<!-- source: disabling-the-program-and-changing-providers ab1548c938f8 -->
## Desativando o programa e trocando de provedor

- O programa não pode ser desativado enquanto houver qualquer assinatura em andamento (pendente, ativa, programada para cancelar, em atraso ou além do período de carência). Retire todos os planos primeiro e desative o programa depois que as assinaturas tiverem terminado.
- Para trocar de provedor, desative o programa, altere `payments` e ative o programa de novo com o novo provedor. Os assinantes atuais assinam de novo. Enquanto o provedor configurado for diferente daquele com o qual o programa foi ativado, o servidor recusa novas assinaturas e mudanças, e a administração mostra um aviso.

<!-- source: what-operators-must-check-not-yet-verified 659a7e51f9d3 -->
## O que os operadores devem verificar (ainda não verificado)

O mycroblog não julga nenhum destes pontos. Verifique-os contra as leis do seu país e os termos do seu provedor.

- Os itens que a divulgação comercial deve conter (o nome, o endereço e o telefone do vendedor, o preço, quando e como pagar, quando o serviço é prestado, como cancelar, e assim por diante) e como ela deve ser publicada.
- Que a tela final de confirmação (artigo 12-6 da Lei de Transações Comerciais Específicas) mostre a quantidade, o preço, o momento do pagamento, o momento da entrega e a forma de cancelar e o efeito disso, considerando em conjunto a página de pagamento do provedor e a tela de confirmação do `my`.
- O tratamento do imposto sobre consumo, e se você emite notas fiscais qualificadas.
