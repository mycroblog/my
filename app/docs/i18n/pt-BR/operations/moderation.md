<!-- source: account-and-domain-actions-and-handling-reports 59d39b2e3f37 -->
# Ações sobre contas e domínios, e tratamento de denúncias

Como operadores e moderadores agem sobre contas e domínios, decidem recursos, tratam denúncias e leem o registro de auditoria.
Tudo aqui, exceto o gerenciamento de contas (apenas pela API de administração), pode ser feito pela API de administração (`/api/v1/admin/...`, escopos `admin:read` / `admin:write`) e pela CLI (`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`).
A API de administração exige permissões de função: o gerenciamento de contas, as ações e os recursos precisam de `manage_accounts`, as ações sobre domínios precisam de `manage_domains` e as denúncias precisam de `manage_reports`.
A CLI se conecta diretamente ao banco de dados com a mesma configuração do servidor (`-config` ou variáveis de ambiente) e não registra qual usuário a operou (`issued_by_user_id` e campos semelhantes ficam NULL, e o autor no [registro de auditoria](#audit-log) é `cli`).

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## De uma denúncia a uma ação e a um recurso

![O caminho de uma denúncia: ela chega, um moderador a analisa, uma ação pode se seguir com um aviso ao usuário, e o usuário pode recorrer; a coluna da direita mostra o que cada etapa envia a outros servidores. A lista abaixo descreve cada etapa.](images/moderation-lifecycle.svg)

Do topo do diagrama para baixo:

1. **Uma denúncia chega.** Um usuário local registra uma, outro servidor envia um `Flag` (descartado quando o domínio tem
   `reject_reports`, veja [ações sobre domínios](#domain-actions)), ou um [hook de decisão](decision-hooks.md) responde `flag`
   ([denúncias registradas por uma análise externa](#reports-that-an-external-review-filed)). Um usuário local que denuncia uma
   conta de outro servidor pode optar por encaminhar a denúncia, o que envia um `Flag` a esse servidor imediatamente.
2. **Análise.** A denúncia fica `open`, e os moderadores recebem uma notificação `admin_report`. Ao resolvê-la, ela passa a
   `resolved` com o resultado `actioned` ou `dismissed`, e um denunciante local recebe uma notificação `report_resolved`.
   Reabri-la a devolve a `open`. Encaminhar uma denúncia sobre uma conta de outro servidor envia um `Flag` a esse
   servidor ([denúncias](#reports)).
3. **Ação.** Um moderador emite uma [ação sobre a conta](#account-actions) a partir da denúncia, o que resolve a denúncia como
   `actioned`; uma ação também pode ser emitida sem denúncia. A ação tem um tipo, um motivo e um horário de término, e
   termina quando é revogada ou expira. Com `notify_target`, o usuário recebe uma notificação `moderation_notice`
   ([o que o usuário vê](#what-the-user-sees)). Outros servidores recebem um `Delete` das publicações para `delete_statuses`, um
   `Update` do ator quando um `suspend` ou `ban` de uma conta local começa ou termina, e um `Reject` / `Undo` dos
   follows que o `suspend` de uma conta remota corta.
4. **Recurso.** O usuário local recorre da ação. O recurso fica `pending`, e os moderadores recebem uma notificação
   `admin_appeal`. A decisão é `upheld` ou `overturned`, e o usuário recebe uma notificação `appeal_decided`.
   Reverter revoga a ação e cancela a exclusão de um `ban`, de modo que um `suspend` ou `ban` revertido de uma conta
   local envia um `Update` do ator ([recursos](#appeals)).

Atribuições, notas, avisos e recursos ficam neste servidor; somente as atividades da coluna da direita saem dele.

<!-- source: moderation-and-server-administration-in-the-web-client 3beb1c9760a0 -->
## Moderação e administração do servidor no cliente web

O cliente web divide as telas da equipe em duas.
A navegação mostra “Moderação” e “Administração” apenas para pessoas cuja função tem pelo menos uma permissão desse grupo. Cada tela mostra apenas um motivo a quem não tem a permissão, e o servidor também rejeita as requisições dessas pessoas.

| Grupo | Permissões | Telas |
| --- | --- | --- |
| Moderação (`/moderation`) | `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit` | Denúncias (`/moderation/reports`), Recursos (`/moderation/appeals`), Contas (`/moderation/accounts`), Cadastros (`/moderation/registrations`), Bloqueios de cadastro (`/moderation/signup-blocks`), Convites (`/moderation/invites`), Ações de domínio (`/moderation/domain-actions`), Servidores remotos (`/moderation/instances`), Registro de auditoria (`/moderation/audit-log`) |
| Administração do servidor (`/admin`) | `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers` | Configurações do servidor (`/admin/settings`), Regras (`/admin/rules`), Termos e políticas (`/admin/documents`), a fila de jobs e os jobs com falha (`/admin/jobs`), Emojis personalizados (`/admin/custom-emojis`), Comunicados (`/admin/announcements`), Webhooks (`/admin/webhooks`), custos e importação de cobrança (`/admin/finance`, `manage_finance`). `manage_login_recovery` (recuperação de login: remover a autenticação em dois fatores e emitir um link de redefinição de senha) e `manage_roles` (conceder e revogar as funções de uma conta) aparecem como seções da página Gerenciamento de uma conta (`/moderation/accounts/{id}/manage`, que também exige `manage_accounts`) |

- O contador em “Moderação” é o total de denúncias abertas, cadastros aguardando aprovação e recursos aguardando análise ([notificações para moderadores e as contagens pendentes](#notifications-for-moderators-and-the-pending-counts)). “Administração” não mostra contagem.
- Cada grupo abre em uma visão geral; as telas em si ficam no menu ao lado. A visão geral da Moderação tem um bloco para cada fila que sua função conta (denúncias abertas, cadastros aguardando aprovação, recursos aguardando uma decisão), cada um com um link para sua fila, um bloco com há quanto tempo a denúncia aberta mais antiga espera (um link para a lista de denúncias, da mais antiga para a mais nova), e lista as denúncias abertas a tratar primeiro, tiradas de toda a fila: primeiro as de conteúdo ilegal, depois as que esperam há mais tempo, com há quanto tempo cada uma espera e quem a trata.
  A lista de denúncias filtra por estado, categoria e responsável (você ou ninguém) e lista as mais novas ou as mais antigas primeiro; os filtros ficam no endereço da página.
  A visão geral da Administração mostra a saúde do servidor: os jobs com falha, há quanto tempo a fila de jobs mantém seu job mais antigo esperando e os erros abertos, cada bloco com um link para sua tela e marcado quando precisa de atenção.
- O `moderator` embutido tem apenas `manage_reports`, `manage_accounts` e `view_audit`, e não vê “Administração”. Ele também não vê as telas Ações de domínio e Servidores remotos (`manage_domains`),
  portanto, para delegá-las, crie uma função com essa permissão e conceda-a ([conceder e revogar funções](admin-cli.md#granting-and-revoking-roles)).
- Os caminhos da API de administração continuam `/api/v1/admin/...` seja qual for o grupo, e os escopos são o mesmo par `admin:read` / `admin:write`. Os grupos são separados por tags na referência da API (`admin-moderation`, `admin-server`).
- Há um terceiro grupo de permissões, os “recursos de usuário” (`view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`), que fica fora das telas da equipe. Quem tem apenas essas permissões não vê nem “Moderação” nem “Administração”,
  e para quem tem `view_finance` a navegação mostra “Finanças” (`/finance`) depois de Configurações.
  Sua API fica no namespace do usuário (`GET /api/v1/finance/cost_totals` e `GET /api/v1/finance/income_totals`, escopo `read:finance`) e não exige nenhum escopo `admin:*`. O que ela mostra é definido por `finance_viewer_*` nas [configurações da instância](instance-settings.md).

<!-- source: account-actions a61dac7c5921 -->
## Ações sobre contas

Uma ação é aplicada à projeção de restrições (`account_restrictions`) na transação que a registra, e passa a valer a partir da próxima requisição.

| Tipo | Efeito sobre uma conta local |
| --- | --- |
| `warn` | Nenhuma restrição (apenas informa o usuário) |
| `force_sensitive` | Mostra todos os anexos como sensíveis |
| `limit` | Trata as publicações (inclusive as antigas) como apenas para seguidores e as mantém fora das linhas do tempo públicas |
| `restrict_posting` | Proíbe publicar, responder, impulsionar e editar |
| `restrict_interaction` | Proíbe seguir, favoritar, impulsionar e responder |
| `disable_login` | Proíbe o uso do app e da API, e revoga as sessões e os tokens ativos (a página de entrada ainda permite que o usuário veja a ação) |
| `suspend` | Aplica todas as restrições acima e oculta o perfil e as publicações de todos, exceto do usuário |
| `ban` | Aplica as mesmas restrições do `suspend` e exclui a conta após 30 dias. Não pode ser revogado (ele só é cancelado quando um recurso feito durante esses 30 dias é revertido) |
| `delete_statuses` | Exclui as publicações indicadas. Elas ficam invisíveis até para o usuário, e o servidor envia aos outros servidores o mesmo `Delete` de quando o usuário as exclui |
| `mark_statuses_sensitive` | Mostra os anexos das publicações indicadas como sensíveis |

Para uma conta remota, você pode emitir apenas `limit`, `suspend` e `force_sensitive` (qualquer outro tipo dá `422` na API de administração e falha na CLI).
O efeito é local a este servidor. As únicas coisas enviadas ao outro servidor são o `Reject` / `Undo` dos follows cortados pelo `suspend`.

| Tipo | Efeito sobre uma conta remota |
| --- | --- |
| `force_sensitive` | Mostra todos os anexos como sensíveis |
| `limit` | Trata as publicações (inclusive as antigas) como apenas para seguidores, as mantém fora das linhas do tempo públicas e não cria notificações para usuários que não seguem a conta |
| `suspend` | Descarta tudo o que é recebido da conta e oculta seu perfil e suas publicações. Corta os follows entre a conta e os usuários locais nos dois sentidos (os pedidos de follow também) e envia ao outro servidor um `Reject` dos follows da conta e um `Undo` dos follows dos usuários. Ao revogá-lo, a exibição volta e o servidor volta a receber da conta (o que foi descartado e os follows cortados durante a suspensão não voltam) |

- `notify_target` (`-no-notify`) não tem significado para uma conta remota (nenhuma notificação chega a ela).
- Um usuário local que perdeu follows por causa de um `suspend` recebe uma notificação (`relationships_severed`) que informa quantos seguidores e follows foram perdidos ([follows cortados](#severed-follows)).
- A emissão e a revogação são registradas no [registro de auditoria](#audit-log), como nas ações sobre contas locais.

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

Na API de administração: `POST /api/v1/admin/accounts/{id}/actions` (o corpo tem `kind`, `reason_category`, `public_reason`, `private_note`, `report_id`, `notify_target`, `expires_in` em segundos e `status_ids`),
`GET /api/v1/admin/accounts/{id}/actions` (filtre com `active`), `GET /api/v1/admin/account_actions/{id}` e `POST .../{id}/lift` (o corpo `{"reason": "..."}` é opcional).

- Indique uma conta com `@username` (local), `@username@domain` (remota) ou o ID da conta na CLI, e com o ID da conta na API de administração.
- Várias ações se acumulam (a união de suas restrições). Revogar remove apenas as restrições daquela ação, e uma ação com horário de término termina sozinha nesse horário (aparece como `expired` nas listas). O horário de término fica a no máximo dez anos (`expires_in` até 315360000); um prazo maior é recusado com `422`.
- As sessões e os tokens que `disable_login`, `suspend` e `ban` revogaram não voltam quando você revoga a ação. O usuário entra de novo.
- Um `suspend` ou `ban` de uma conta local envia um `Update` do Actor com o perfil esvaziado aos servidores dos seguidores e aos servidores conhecidos, para mostrar a suspensão (o Mastodon a mostra como "suspended").
  A revogação e a expiração enviam um `Update` com o perfil original. A expiração é enviada por um job que roda a cada 5 minutos, então pode atrasar até 5 minutos.
- `delete_statuses` e `mark_statuses_sensitive` recebem de 1 a 100 publicações não excluídas da conta alvo por meio de `-status` (`status_ids`). Não podem ter horário de término.
  Revogar `delete_statuses` não traz de volta as publicações excluídas. Revogar `mark_statuses_sensitive` restaura a exibição original.
- Um `ban` não pode ter horário de término e não pode ser revogado. Ele só é cancelado quando um recurso que o usuário fez nos 30 dias antes da exclusão é revertido ([recursos](#appeals)). Na dúvida, use um `suspend` com horário de término.
  Trinta dias depois da ação, um job que roda a cada hora exclui a conta (a conta e as publicações somem da vista, e o servidor envia um `Delete` aos outros servidores).
- `report_id` (`-report`) só pode indicar uma denúncia sobre a conta alvo. Uma denúncia aberta é resolvida como `actioned` junto com a ação (a resolução também fica no registro de auditoria). Uma denúncia já resolvida não muda.
- `public_reason` é o motivo mostrado ao usuário e `private_note` é uma nota apenas para moderadores. `notify_target` (verdadeiro por padrão; falso com `-no-notify` na CLI) decide se o usuário é informado.

<!-- source: what-the-user-sees 0ca06c59f8e1 -->
### O que o usuário vê

- Uma ação com `notify_target` verdadeiro chega ao usuário como uma notificação (`moderation_notice`). Ela traz o tipo, a categoria do motivo, `public_reason`, o horário de término e o estado. O tipo e a categoria também voltam como nomes de exibição no idioma da interface do usuário.
- O usuário pode consultar as ações contra ele com o `GET /api/v1/moderation/actions` de primeira parte (filtre com `active`), das mais novas para as mais antigas. Ações com `notify_target` falso e ações revogadas ou expiradas também aparecem na lista.
- `private_note`, a denúncia, quem tomou a ação e o motivo da revogação nunca são mostrados ao usuário.
- Um usuário pode recorrer de cada ação uma vez ([recursos](#appeals)). A lista de ações e a ação em uma notificação incluem se ela pode ser contestada (`appealable`), o prazo (`appeal_deadline`) e o recurso e seu resultado (`appeal`).
- Um usuário sob `disable_login`, `suspend` ou `ban` não pode usar o app nem a API, mas ainda pode entrar na página de entrada (`/auth/sign_in`). Ali ele vê o tipo, o motivo e o horário de término da ação em vigor
  (para `ban`, o número de dias até a conta ser excluída), e pode recorrer a partir dessa página ([recursos](#appeals)). O endpoint de autorização (`/oauth/authorize`) responde com `403` e não emite token.
  Um usuário sob `ban` pode entrar até a conta ser excluída, e a exclusão revoga essa sessão também. Um usuário cuja própria exclusão está agendada, e uma conta que foi desativada, não podem entrar nem na página de entrada.

<!-- source: accounts-that-their-owners-deactivated ed010f49b40a -->
### Contas que os próprios donos desativaram

Um usuário pode desativar a própria conta nas configurações da conta, depois de digitar a senha ou um código de dois
fatores novamente, ou de confirmar com uma passkey (um usuário sem nenhum desses entra de novo em um provedor de
login vinculado). A conta não é uma ação de moderação, e os moderadores não podem desativar ninguém:

- O perfil e as publicações ficam ocultos para todos, exceto para os moderadores, e outros servidores recebem um `Update` do
  ator marcado como `suspended`, então podem mostrar a conta como suspensa. Todas as sessões e tokens do usuário terminam.
  Onde o servidor envia e-mails, um e-mail informa a data da exclusão; nenhum outro e-mail é enviado antes dela.
- Nada chega à conta: menções e mensagens diretas são descartadas, nenhuma notificação, e-mail ou Web Push é
  gerado, e novos seguidores são recusados. Favoritos e impulsos são contados. Os follows existentes permanecem.
- O usuário entra em uma página do servidor, que mostra a data da exclusão e permite reativar ou excluir a
  conta. Reativar traz tudo de volta. A desativação seguinte precisa esperar 24 horas.
- Trinta dias depois da desativação, o job horário exclui a conta. Uma reativação após essa data é recusada
  mesmo que o job ainda não tenha rodado. O nome de usuário não é liberado.

Os moderadores veem o estado `deactivated` na lista de contas (`state=deactivated`) e ainda podem ler as denúncias sobre
a conta e agir sobre ela. Uma ação de moderação não altera a data da exclusão, exceto um `ban`, que
a substitui pela sua própria: a partir daí o usuário não pode reativar. Uma suspensão não envia nada a outros servidores enquanto a
conta está desativada, e continua em vigor depois que o usuário reativa. Um usuário não pode desativar uma conta enquanto
a moderação a impede de entrar, enquanto sua exclusão está agendada, depois de ela ter sido movida, enquanto seu cadastro aguarda
análise, enquanto uma assinatura paga ainda está em andamento (inclusive uma configurada para terminar ao fim do período), ou quando
ela é a única administradora que consegue entrar. Um administrador desativado não conta como alguém que consegue. Se a data de um
administrador desativado passa sem que outro administrador consiga entrar, o job horário não exclui a
conta e registra um aviso no log. Esse administrador também não consegue mais entrar, então crie outro administrador com
`my admin create`; a execução seguinte então exclui a conta.

<!-- source: appeals 8a6f1b4c288d -->
## Recursos

Um usuário local pode recorrer de uma ação contra ele.
Um moderador analisa o recurso e o mantém (`uphold`) ou o reverte (`overturn`).

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

O usuário recorre com `POST /api/v1/moderation/actions/{id}/appeal` (escopo `write:accounts`, corpo `{"text": "..."}`).
Um usuário sob `disable_login`, `suspend` ou `ban` não consegue obter um token de acesso e recorre pelo formulário de cada ação na página de entrada (`POST /auth/appeal`).
Enquanto a [entrada está limitada a provedores de login](login-providers.md#limit-sign-in-to-login-providers), esse usuário só chega ao formulário por um provedor vinculado.
A página mostra o formulário e o prazo para uma ação que pode ser contestada, o conteúdo e o resultado para uma ação já contestada, e o motivo para uma ação cujo prazo passou ou que não pode ser contestada. Para um recurso rejeitado, mostra o motivo.
Na API de administração: `GET /api/v1/admin/appeals` (filtre com `state`), `GET .../{id}`, e `POST .../{id}/uphold` e `POST .../{id}/overturn` (o corpo `{"public_reason": "..."}` é opcional). Eles exigem a permissão `manage_accounts`.

- Uma ação pode ser contestada uma vez, e o texto tem de 1 a 2.000 caracteres. O prazo é o número de dias da configuração da instância `appeal_window_days` após a ação (padrão de 180 dias, de 20 a 365; veja [configurações da instância](instance-settings.md)).
  Uma ação fora do prazo e um segundo recurso dão `422` (`/api/v1/problems/appeal-window-closed` e `/api/v1/problems/already-appealed`).
- Um `ban` pode ser contestado até a conta ser excluída (30 dias após a ação, ou até o prazo de `appeal_window_days`, se for mais curto). Um `ban` de uma conta excluída dá `422` (`/api/v1/problems/not-appealable`). Uma ação revogada ou expirada ainda pode ser contestada dentro do prazo.
- Quando um usuário recorre, as pessoas com `manage_accounts` recebem uma notificação (`admin_appeal`). Quando o recurso é analisado, o usuário recebe uma notificação (`appeal_decided`) e pode ver o resultado e `public_reason`. O analista não é mostrado.
- Reverter revoga a ação na mesma transação (o mesmo efeito de `lift`, com um motivo de revogação vazio). Reverter `delete_statuses` não traz de volta as publicações excluídas. Uma ação já revogada antes da análise permanece como está.
- Reverter um `ban` também cancela a exclusão agendada da conta na mesma transação (o registro de auditoria grava `account_deletion_canceled`), encerra a suspensão e envia o `Update` com o perfil original aos outros servidores. As sessões e os tokens revogados não voltam, então o usuário entra de novo.
  Se a conta foi excluída antes da análise, ele não pode ser revertido (`422`, `/api/v1/problems/not-appealable`; manter ainda é possível).
- Uma análise acontece uma vez. Analisar de novo com o mesmo resultado não altera nada, e o resultado oposto dá `409` (`/api/v1/problems/appeal-decided`).

<!-- source: account-management 99149899a198 -->
## Gerenciamento de contas

Pesquisar contas, ver seus detalhes e revelar endereços IP e endereços de e-mail são tarefas da API de administração e exigem a permissão `manage_accounts`.

No cliente web, uma conta tem três páginas, com links entre elas abaixo do título da página:

- **Análise** (`/moderation/accounts/{id}`): o estado e as restrições da conta, o formulário para uma nova ação, as ações mais recentes sobre ela com seus motivos e suas 20 denúncias mais recentes. As denúncias mais antigas estão apenas na lista de denúncias (`/moderation/reports`), que não tem filtro por conta.
- **Ações** (`/moderation/accounts/{id}/actions`): todas as ações sobre a conta, uma página por vez, com “Revogar” para as que estão em vigor. Cada ação abre sua própria página (`/moderation/accounts/{id}/actions/{action id}`) com todos os seus detalhes.
- **Gerenciamento** (`/moderation/accounts/{id}/manage`, apenas contas locais): as funções da conta (`manage_roles`), a supressão de e-mail, a remoção da autenticação em dois fatores e a emissão de um link de redefinição de senha (`manage_login_recovery`), e seus endereços de e-mail e IP. Cada seção aparece apenas para quem tem a permissão correspondente.

A equipe só age sobre níveis inferiores: emitir ou revogar uma ação, decidir um recurso, revogar uma supressão de e-mail, revelar dados privados, emitir um link de redefinição de senha e remover a autenticação em dois fatores exigem que a maior `position` entre suas funções esteja acima da de todas as funções do alvo.
Um nível maior ou igual, inclusive a sua própria conta, retorna `403` com o tipo de problema `/api/v1/problems/target-outranks-actor`, de modo que um moderador não pode banir um administrador e um administrador não pode banir outro. Os comandos `my admin` não têm esse limite.
Uma ação que impede o único administrador que consegue entrar de entrar (`disable_login`, `suspend`, `ban`) é recusada, quem quer que a emita, e esse administrador não pode excluir a própria conta (`409` com `/api/v1/problems/last-administrator`). Veja [admin-cli.md](admin-cli.md) para as funções.

- `GET /api/v1/admin/accounts` retorna contas locais e remotas, das mais novas para as mais antigas (o Actor da instância é excluído).
  Você pode filtrar por prefixo do nome de usuário (`username`), domínio (`domain`), local ou remota (`origin`), estado (`state`) e, para contas locais, endereço de e-mail (`email`), domínio do e-mail (`email_domain`) e o endereço IP usado no cadastro ou na entrada (`ip`).
  Você pode informar apenas um entre `email`, `email_domain` e `ip`. A lista não inclui endereços IP nem endereços de e-mail.
- O estado é o primeiro que se aplica nesta lista: `deleted`, `suspended` (`suspend`, `ban`), `disabled` (`disable_login`), `rejected` e `pending` (análise do cadastro), `limited` (`limit`), `active`.
  O estado e as restrições (`restrictions`) vêm apenas de ações contra a própria conta e não incluem ações sobre domínios.
- `GET /api/v1/admin/accounts/{id}` retorna o estado e as restrições e, para uma conta local, o pedido de cadastro e a análise, a confirmação de e-mail, as funções e até 20 ações e denúncias, das mais novas para as mais antigas.
- `POST /api/v1/admin/accounts/{id}/reveal` retorna o endereço de e-mail de uma conta local, os endereços IP e User-Agents do cadastro e das entradas (20 registros, os mais novos primeiro) e outras contas que usaram o mesmo endereço IP (até 20 por endereço IP).
  Cada chamada grava `revealed_private_data` no registro de auditoria, e os dados só são retornados se o registro pôde ser gravado. Os endereços IP são removidos após o período de retenção (180 dias por padrão), então origens de entrada mais antigas que isso não podem ser mostradas.
- `two_factor_enabled` em `GET /api/v1/admin/accounts/{id}` mostra se o usuário de uma conta local usa autenticação em dois fatores (TOTP).
  Um usuário que perdeu o app autenticador e os códigos de recuperação pode entrar apenas com a senha depois que você remove a autenticação em dois fatores com `POST /api/v1/admin/accounts/{id}/two_factor/disable`.
  Somente usuários com a permissão `manage_login_recovery` podem fazer isso, e por padrão apenas a função de administrador embutida a tem (ela pertence à administração do servidor; o cliente web mostra a seção só para eles).
  A remoção grava `two_factor_disabled` no registro de auditoria. Verifique por outro meio, como o e-mail, que a pessoa é a dona da conta antes de removê-la.
  Quando um administrador sem nenhum outro administrador a perdeu, o operador a remove com o comando da CLI `my admin account disable-two-factor @alice` (o autor no registro de auditoria é a CLI).
- Em um servidor que envia e-mails, um usuário que esqueceu a senha recebe por e-mail um link para definir uma nova a partir de “Esqueceu a senha?” na página de entrada.
  Para um servidor que não envia e-mails (`mail = none`), ou um usuário que o e-mail não alcança, um administrador emite o link e o entrega.
  Nas telas de administração, emita-o com “Redefinição de senha” na página Gerenciamento da conta (`/moderation/accounts/{id}/manage`, `POST /api/v1/admin/accounts/{id}/password_reset_link`).
  Somente usuários com a mesma permissão da remoção da autenticação em dois fatores, `manage_login_recovery`, podem emiti-lo, e por padrão apenas a função de administrador embutida a tem (quem tem o link consegue entrar na conta, então o `manage_accounts` de um moderador não basta).
  O link emitido é mostrado uma vez na hora e pode ser copiado. O servidor guarda apenas um hash do link, então você não consegue vê-lo depois; emita de novo se precisar.
  Quando nenhum administrador está disponível ou as telas de administração não estão, o operador o emite com o comando da CLI `my admin account password-reset-link @alice` (o autor no registro de auditoria é a CLI).
  Enquanto a [entrada está limitada a provedores de login](login-providers.md#limit-sign-in-to-login-providers), os dois ainda emitem o link e avisam que ele não funciona até o limite acabar.
  O link funciona por 1 hora e uma única vez, e deixa de funcionar se a senha mudar depois por outro caminho. As duas formas de emissão gravam `password_reset_link_issued` no registro de auditoria.
  Quem tem o link pode definir a senha, então verifique a pessoa por outro meio, como o e-mail, e entregue o link somente a ela.
  Definir a nova senha revoga todas as sessões e tokens de app do usuário. As passkeys e a autenticação em dois fatores permanecem.
- `email_suppression` em `GET /api/v1/admin/accounts/{id}` mostra que o envio ao endereço principal de uma conta local está parado.
  Um `hard_bounce` (o destino não existe) interrompe todos os e-mails, exceto o de confirmação, e uma `complaint` (marcado como spam) interrompe apenas os e-mails de notificação e de comunicado. Os e-mails de redefinição de senha também param, então emita o link acima e o entregue, se necessário.
  Quando o usuário pede um e-mail de confirmação e confirma o endereço de novo, a supressão por hard bounce é revogada.
  Quando você verificou que o envio está normal, por exemplo porque o usuário corrigiu a caixa de e-mail, revogue-a com `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permissão `manage_accounts`). Isso grava `email_suppression_lifted` no registro de auditoria.
- Uma busca lê até 400 contas para preencher uma página. Quando poucas contas correspondem, até uma página curta ou vazia pode trazer `next_cursor`, então continue lendo.

<!-- source: domain-actions 13dd3d4d3a0f -->
## Ações sobre domínios

| Tipo | Efeito |
| --- | --- |
| `suspend` | Interrompe toda entrega, busca e recebimento (o recebimento recebe `403` antes de a assinatura ser verificada). As contas e publicações existentes ficam ocultas das linhas do tempo e dos perfis. Corta os follows entre usuários locais e as contas do domínio nos dois sentidos (nada é enviado ao outro lado) |
| `limit` | Mantém as publicações fora das linhas do tempo públicas e da busca, e não cria notificações para usuários que não seguem a conta |
| `reject_media` | Não busca mídia |
| `reject_reports` | Descarta denúncias (`Flag`) do domínio |
| `force_sensitive` | Mostra a mídia como sensível |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

Na API de administração: `POST /api/v1/admin/domain_actions` (o corpo tem `domain`, `kind`, `public_comment`, `private_note`, `obfuscate` e `expires_in` em segundos),
`GET /api/v1/admin/domain_actions` (filtre com `domain` e `active`), `GET .../{id}` e `POST .../{id}/lift`.

No cliente web, abra Moderação → Ações de domínio (`/moderation/domain-actions`). A lista mostra o domínio, o tipo, o estado e o término de cada ação, em vigor ou todas;
“Nova ação” abre o formulário (`/moderation/domain-actions/new`), e escolher um domínio abre a página da ação com seus comentários e datas, onde “Revogar” a encerra.
O link “Ações de domínio” na página de um servidor remoto lista apenas as ações desse domínio, e “Nova ação” ali inicia o formulário com o domínio preenchido.

- Os domínios são registrados normalizados para o A-label IDNA (minúsculas, sem ponto final). `例え.jp` e `xn--r8jz45g.jp` são o mesmo domínio.
- Uma ação também se aplica a subdomínios (uma ação sobre `spam.example` se aplica também a `a.spam.example`).
- Uma ação é aplicada à projeção de restrições (`domain_restrictions`) e às restrições das linhas do tempo na transação que a registra, e passa a valer a partir da próxima entrega, busca ou recebimento.
- Uma ação com horário de término (`-duration` / `expires_in`, no máximo dez anos) termina sozinha nesse horário. A expiração não é uma revogação, então as listas a mostram como `expired`.
- Revogar remove apenas as restrições daquela ação. Outras ações sobre o mesmo domínio permanecem. Revogar uma ação já revogada não altera nada.
- As listas vão das mais novas para as mais antigas, e `-active` (`active=true`) mostra apenas as ações em vigor.

<!-- source: importing-and-exporting-mastodon-csv ba9ed39bd32e -->
### Importar e exportar CSV do Mastodon

Você pode passar a lista de ações sobre domínios de e para o Mastodon no formato CSV dos bloqueios de domínio do Mastodon (`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`).
Você pode importar um CSV que o Mastodon exportou, e o Mastodon pode importar um CSV que este servidor exportou.

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

Na API de administração: `GET /api/v1/admin/domain_actions/export` (`text/csv`), `POST .../import/preview` (o corpo tem `csv`) e `POST .../import` (o corpo tem `blocks`, as linhas escolhidas, e `private_note`).
Uma linha para o domínio deste próprio servidor é ignorada (`own_domain`), e uma ação sobre ele é recusada.
No cliente web, abra Moderação → Ações de domínio e escolha “CSV do Mastodon” (`/moderation/domain-actions/import`): “Exportar CSV” salva o arquivo, e a importação mostra uma prévia de um arquivo colado ou escolhido antes de importar as linhas que você mantiver.

| Linha do CSV | Ação neste servidor |
| --- | --- |
| `#severity` é `suspend` (ou vazio) | `suspend` |
| `#severity` é `silence` | `limit` |
| `#severity` é `noop` | Nenhuma |
| `#reject_media` / `#reject_reports` é verdadeiro | Adiciona `reject_media` / `reject_reports`, respectivamente |

- Uma importação tem duas etapas. Primeiro confira na prévia quais ações cada linha emitiria, escolha as linhas a manter e então importe (a CLI não altera nada até você acrescentar `-apply`, e `-only` escolhe linhas por domínio).
- Uma linha cujo domínio, ou um domínio pai, já tem uma ação do mesmo tipo em vigor não emite esse tipo (se todos os tipos já existem, ela é “já aplicada”). Uma segunda linha para o mesmo domínio, uma linha inválida e uma linha que não emitiria nada (apenas `noop`) também são ignoradas. Importar o mesmo CSV de novo não adiciona ações.
- Um CSV sem o cabeçalho `#domain` é lido como uma lista com um domínio por linha, e cada domínio recebe `suspend` (o mesmo que o Mastodon faz). Uma importação aceita até 5.000 linhas.
- As ações importadas duram “até serem revogadas” e carregam o comentário público e a ofuscação da linha e a nota privada que você informou. Elas aparecem no registro de auditoria como as ações emitidas uma a uma (o autor é a pessoa, no caso da API, e `cli`, no caso da CLI).
- Uma exportação grava uma linha por domínio para as ações em vigor. Um domínio com um `suspend` recebe `suspend`, um com um `limit` recebe `silence`, e um com nenhum dos dois recebe `noop`. Um domínio com apenas `force_sensitive`, que os bloqueios de domínio do Mastodon não têm, não é exportado. As notas privadas e os horários de término não são exportados.

<!-- source: severed-follows 51be435d8635 -->
### Follows cortados

Um `suspend` de domínio e um `suspend` de conta remota cortam os follows entre usuários locais e as contas suspensas nos dois sentidos.

- Para um `suspend` de domínio, um job (`moderation.sever_domain`) que roda depois que a ação é registrada os corta um usuário local por vez. Até o job terminar, os follows ainda aparecem nas listas. As contas em subdomínios também são cortadas.
- Para um `suspend` de conta remota, os follows são cortados na transação que registra a ação. Um segundo `suspend` de uma conta já suspensa não corta nada.
- Cada usuário que perdeu follows aprovados recebe uma notificação (`relationships_severed`) que informa quantos seguidores e follows foram perdidos. Um usuário que perdeu apenas pedidos de follow não recebe nenhuma.
- Revogar a ação não traz de volta os follows cortados. Os usuários seguem de novo.
- Processar a mesma ação de novo (por exemplo, quando um job roda outra vez) não adiciona mais registros de relações cortadas nem notificações.

<!-- source: publishing-the-domains-you-act-on b647d2c8c84c -->
### Publicar os domínios sobre os quais você age

Você pode publicar as ações `limit` e `suspend` em vigor por meio de `GET /api/v1/instance/domain_actions`.
A configuração da instância `domain_actions_visibility` (`off` / `signed_in` / `everyone`, padrão `off`) decide quem vê a lista, e
`domain_actions_rationale_visibility` (os mesmos valores, padrão `off`) decide quem vê os comentários públicos ([configurações da instância](instance-settings.md)).

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- Com `off`, a API retorna `404`, e com `signed_in` retorna `401` a uma requisição sem token de acesso.
- Há uma entrada por ação, das mais novas para as mais antigas. Uma entrada tem o domínio, o SHA-256 do domínio (hex) e o tipo, e o comentário público para quem tem permissão de vê-lo. As notas privadas nunca são retornadas.
- Para uma ação com `-obfuscate` (`obfuscate`), a metade central dos caracteres do domínio (exceto os pontos) é substituída por `*` (`bad.example` vira `bad.*****le`).
  Qualquer pessoa pode comparar o SHA-256 de um domínio que conhece para verificar se é um domínio ofuscado.
- As ações `reject_media`, `reject_reports` e `force_sensitive`, e as ações revogadas ou expiradas, não são publicadas.
- A página de informações da instância do cliente web (`https://<host>/about`) mostra essa lista sob as mesmas configurações. Com `off`, a seção não é mostrada, e com `users`, quem não está conectado vê um aviso de que apenas usuários conectados podem vê-la.

<!-- source: refetching-remote-posts 883740b40c9a -->
## Buscar publicações remotas novamente

Você pode buscar de novo, no outro servidor, as publicações remotas armazenadas.
Uma nova busca acontece apenas quando você a pede pela API de administração, pela CLI ou pelo cliente web, nunca periodicamente.

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

Na API de administração: `POST /api/v1/admin/statuses/{id}/refetch`, `POST /api/v1/admin/accounts/{id}/refetch_statuses` e `POST /api/v1/admin/instances/{domain}/refetch_statuses`. Eles exigem a permissão `manage_domains`.
No cliente web, use “Buscar publicações novamente” na página Análise da conta (`/moderation/accounts/{id}`, contas remotas) e na tela do servidor remoto (`/moderation/instances/{domain}`).

- Uma publicação é tratada como um `Update` recebido. Se houver uma versão mais nova, o servidor adiciona uma versão; ele atualiza as contagens de votos de enquetes; e não faz nada se nada mudou. Uma publicação para a qual o outro servidor retorna 410 Gone ou um `Tombstone` é excluída. Uma publicação que não pode ser buscada (inclusive 404) é deixada como está.
- Para uma conta, o servidor busca de novo até as 1.000 publicações armazenadas e não excluídas mais recentes, 100 por vez, com um minuto de intervalo.
- Para um domínio, o servidor busca de novo a partir das contas mais recentes do domínio, até as 20 publicações mais recentes por conta e até 1.000 publicações no total, 5 contas por vez, com um minuto de intervalo.
- Ele não busca em um domínio com `suspend`, nem em um domínio `unavailable` porque as entregas falharam. Uma busca com falha do mesmo URI é lembrada por 1 hora, e uma nova busca nesse intervalo não o busca.
- Sua requisição é gravada no registro de auditoria como `status_refetch_queued`, `account_statuses_refetch_queued` ou `domain_statuses_refetch_queued`.

<!-- source: reports e8a3f112b323 -->
## Denúncias

As denúncias (`Flag`) recebidas de outros servidores entram na lista de denúncias como abertas (`open`).

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
my admin report forward -comment "Spam account, see the linked posts" <report id>
```

Na API de administração: `GET /api/v1/admin/reports` (filtre com `state`, `category` e `assignee`: `me`, `none` ou o ID de uma conta local; `order=oldest` lista primeiro a denúncia que espera há mais tempo, em vez da mais nova), `GET .../{id}`, `POST .../{id}/assign_to_self`, `POST .../{id}/unassign`,
`POST .../{id}/resolve` (corpo `{"outcome": "actioned" | "dismissed"}`), `POST .../{id}/notes` (corpo `{"body": "..."}`), `POST .../{id}/reopen` e `POST .../{id}/forward` (corpo `{"comment": "..."}`, opcional).

- Uma atribuição (`assign`) só pode ir para um usuário local cuja função tem `manage_reports`. Na API de administração, você atribui a si mesmo.
- O resultado de uma resolução (`resolve`) é `actioned` (uma ação foi tomada contra o alvo) ou `dismissed` (nenhuma ação foi necessária). Resolver de novo uma denúncia já resolvida não altera o resultado.
- Quando você emite uma ação a partir de uma denúncia (`report_id` / `-report` da ação), a denúncia é resolvida como `actioned` na mesma operação.
- Uma nota (`note`) é uma nota interna que só os moderadores veem, de até 2.000 caracteres. Ela não é mostrada nem ao alvo nem ao denunciante. Não pode ser editada nem excluída.
- Reabrir (`reopen`) devolve uma denúncia resolvida a `open` (a atribuição permanece). Uma denúncia aberta não pode ser reaberta (`409` na API de administração).
- Encaminhar (`forward`, o botão **Encaminhar ao servidor deles** nos detalhes da denúncia) envia uma denúncia sobre uma conta de outro servidor a esse servidor, como um `Flag` do ator da sua instância com a conta e as publicações denunciadas que ainda existem. O único texto que ele leva é o que você escreve para os moderadores desse servidor ao encaminhar (`-comment`, `comment` na API de administração, ou **Texto para os moderadores deles** na página da denúncia; até 1.000 caracteres, nenhum por padrão). O comentário do denunciante e a nota de uma [análise externa](#reports-that-an-external-review-filed) nunca saem do seu servidor desse modo, e nada do denunciante é enviado. Uma denúncia que o denunciante escolheu encaminhar ao registrá-la saiu imediatamente, com o comentário do denunciante. Encaminhar funciona para uma denúncia registrada neste servidor, por uma pessoa ou por uma [análise externa](#reports-that-an-external-review-filed), aberta ou resolvida, que ainda não foi encaminhada; uma denúncia recebida de outro servidor ou sobre uma sala de chat de voz não pode ser encaminhada (`409` `report-not-forwardable` na API de administração, e os detalhes da denúncia não mostram botão). Uma denúncia é enviada uma vez: encaminhá-la de novo, com qualquer texto, ou encaminhar uma que o denunciante tinha encaminhado ao registrá-la não altera nada. `forwarded` na API de administração diz se ela foi enviada, e `forwardable` diz se você pode enviá-la agora.
- Os detalhes da denúncia (`show`, `GET .../{id}`) retornam as notas, das mais antigas para as mais novas, com o número de outras denúncias sobre o alvo e o número de ações contra o alvo (inclusive as revogadas e expiradas). A lista não retorna notas.
- Atribuições, resoluções, reaberturas, notas e encaminhamentos são mantidos como eventos (`report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`, `report_forwards`), e `report_states` guarda o estado atual.
- Os moderadores podem traduzir as publicações denunciadas e o comentário do denunciante na página da denúncia, ou com `POST .../{id}/statuses/{status_id}/translation` e `POST .../{id}/comment/translation` (corpo `{"target_language": "en"}`).
  O texto vai a um provedor de tradução somente enquanto a [configuração da instância](instance-settings.md) `translation_reports_to_server` está ativada, e então apenas a um provedor autorizado para a visibilidade da publicação: o acesso de um moderador não amplia nada, então uma publicação direta vai apenas a um provedor autorizado para `direct`, e o comentário conta como `private`.
  A tradução do comentário não é guardada, nem a de uma publicação excluída, e os caracteres contam apenas para o limite mensal do servidor. A tradução embutida do navegador, quando `translation_browser` está ativada, funciona nas denúncias seja qual for o valor de `translation_reports_to_server`.
- Um denunciante vê do alvo apenas o que veria da conta em outras circunstâncias: enquanto ela está suspensa ou o bloqueia, a denúncia dele continua listada sem a conta. Os moderadores sempre veem o alvo inteiro.
- Um denunciante local recebe uma notificação (`report_resolved`) na primeira resolução. Reabrir e resolver de novo não acrescenta notificação.
  Um denunciante pode listar as próprias denúncias com `GET /api/v1/reports` e vê apenas se cada uma está aberta ou resolvida. O resultado, a ação tomada, o motivo dado ao alvo e o responsável não são mostrados.

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### Denúncias registradas por uma análise externa

Um [hook de decisão](decision-hooks.md) que responde `flag` registra uma denúncia com o ator da sua instância como denunciante. A denúncia
indica isso: `hook_origin` na API de administração e no webhook `report.created`, uma linha nos detalhes da denúncia e `automated` em
`my admin report show`. Seu comentário é a nota do plugin, e sua categoria vem do motivo. Ela não é encaminhada quando
é registrada, e resolvê-la não notifica ninguém. Quando o alvo está em outro servidor, você mesmo pode encaminhá-la
(`forward`): a nota do plugin fica no seu servidor, e só o texto que você escreve sai. Cancelar a decisão
deixa a denúncia como está.

<!-- source: voice-chat-rooms 7fd142752b07 -->
### Salas de chat de voz

- Um usuário local pode denunciar uma sala de chat de voz aberta que ele consegue ver (`POST /api/v1/reports` com `voice_room_id`, e `account_id` definido como o anfitrião da sala).
  A denúncia tem como alvo o anfitrião naquele momento e aponta para as configurações da sala (nome e descrição) como estavam então. Ela não aponta para nenhuma publicação e nunca é encaminhada a outro servidor.
- Os detalhes da denúncia (`GET /api/v1/admin/reports/{id}`) mostram a sala (`voice_room`): cada alteração de suas configurações, quem participou e quando saiu, quem foi removido dela e como ela terminou.
  Os registros de quem participou são mantidos por 90 dias após o fim da sala (`voice_participants_retention_days`) e enquanto uma denúncia apontar para a sala ([retenção de registros](#record-retention-and-holds)).
  Não há gravações nem chat de texto, então esses registros são a única evidência.
- Quem tem `manage_reports` pode encerrar uma sala aberta para todos os participantes com `POST /api/v1/admin/voice_rooms/{id}/end` (corpo `{"reason": "..."}`, opcional, até 500 caracteres).
  O motivo é apenas para os moderadores; os participantes veem somente que a sala terminou. Funciona também com o chat de voz desativado, e o registro de auditoria grava `voice_room_ended`.

<!-- source: notifications-for-moderators-and-the-pending-counts 7b83fa4097b2 -->
## Notificações para moderadores e as contagens pendentes

Novas denúncias, cadastros aguardando aprovação e recursos chegam às pessoas com a permissão como notificações no app.

| Notificação | Quando chega | Quem recebe |
| --- | --- | --- |
| `admin_report` | Quando o alvo da denúncia não tem outra denúncia aberta (uma segunda denúncia ou posterior sobre a mesma conta não notifica) | Pessoas cuja função tem `manage_reports`, exceto o denunciante e o alvo da denúncia |
| `admin_sign_up` | Quando um cadastro começa aguardando aprovação (modo de aprovação, ou `open` em uma instalação que não pode enviar e-mails) | Pessoas cuja função tem `manage_accounts` |
| `admin_appeal` | Quando um recurso é feito contra uma ação | Pessoas cuja função tem `manage_accounts`, exceto quem recorreu |
| `admin_custom_emoji_request` | Quando se pede a criação de um emoji personalizado ou a troca de sua imagem | Pessoas cuja função tem `manage_custom_emojis`, exceto quem pediu |
| `admin_error_group` | Quando o servidor ou o cliente web relata um erro de um tipo novo, ou um grupo de erros que foi resolvido acontece de novo em outra versão do servidor | Pessoas cuja função tem `manage_errors` |

- `admin_error_group` chega no máximo 10 vezes por hora para o servidor inteiro. O restante da hora é reunido em uma notificação
  que diz quantos grupos de erros a mais houve. Um grupo silenciado nunca notifica. Um erro que o cliente web relata para um
  visitante que não está conectado só notifica quando o navegador de um usuário conectado também o relata, e sua recorrência não notifica ninguém.
  Os grupos de erros são lidos e resolvidos, silenciados ou reativados por meio de `GET /api/v1/admin/error_groups` e
  `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`), que exigem `manage_errors`.
- Quem recebe uma notificação é decidido pela permissão no momento em que a notificação é criada. Quem depois perde a permissão deixa de ver esse tipo na lista e na contagem de não lidas. Se a permissão voltar, a pessoa volta a vê-lo.
- Cada tipo de notificação acima também chega por e-mail, ativado por padrão. Cada pessoa ativa ou desativa cada tipo no app e por
  e-mail na página **Notificações** das configurações (`/settings/notifications` no cliente web), que lista apenas os
  tipos que sua função recebe. Um tipo desativado no app não é criado para ela, então não é enviado por push nem por e-mail; as contagens de
  `GET /api/v1/admin/counts` não mudam, porque contam as próprias denúncias, cadastros e recursos.
  Junto com as denúncias abertas, as contagens também retornam quando a denúncia aberta mais antiga foi registrada (`oldest_open_report_filed_at`).
  O servidor enfileira o job de e-mail na transação que registra a notificação, e o job envia o e-mail logo após o
  commit, sem esperar a notificação ser lida. Se a pessoa perdeu a permissão ou desativou o e-mail desse tipo
  até o momento do envio, ou se seu endereço principal não está confirmado ou seus e-mails estão parados, nada é enviado.
  O e-mail contém apenas o tipo de notificação e um link para a tela de notificações, e nenhum conteúdo da denúncia ou da conta. Seu link de cancelamento de inscrição desativa apenas o e-mail daquele tipo.
  Com `mail = none`, nenhum e-mail é enviado, e `sends_email` em `GET /api/v1/instance` é `false`.

`GET /api/v1/admin/counts` retorna o número de denúncias abertas (`open_reports`, exige `manage_reports`), o número de cadastros aguardando aprovação (`pending_registrations`) e o número de recursos aguardando análise (`pending_appeals`) (os dois exigem `manage_accounts`).
Os itens para os quais você não tem permissão não são retornados.

<!-- source: audit-log 86955f4182c0 -->
## Registro de auditoria

As operações de moderadores e administradores são gravadas, uma linha cada, no registro de auditoria (`audit_log_entries`) na mesma transação da operação.
Operações com falha e operações que não alteraram nada não são gravadas: revogar uma ação já revogada, resolver uma denúncia resolvida, atribuir ao mesmo responsável, retirar uma regra já retirada, mover para a mesma posição, conceder uma função que o usuário já tem e revogar uma função que o usuário não tem.

| Tipo de alvo | Operações |
| --- | --- |
| `account` | `account_action_issued` e `account_action_lifted` (emissão e revogação de uma ação contra uma conta), `account_deletion_canceled` (a exclusão cancelada porque um recurso contra um `ban` foi revertido), `revealed_private_data` (endereços IP e endereços de e-mail revelados), `two_factor_disabled` (autenticação em dois fatores removida), `password_reset_link_issued` (um link de redefinição de senha emitido), `email_suppression_lifted` (supressão de e-mail revogada), `account_moved` (uma conta local restrita movida para uma conta local; as restrições não são levadas ao destino, então você decide se as aplica lá), `account_statuses_refetch_queued` (nova busca das publicações de uma conta remota) |
| `status` | `status_refetch_queued` (nova busca de uma publicação remota; o nome do alvo é o URI da publicação) |
| `domain` | `domain_action_issued` e `domain_action_lifted` (emissão e revogação de uma ação sobre domínio), `domain_statuses_refetch_queued` (nova busca das publicações das contas do domínio), `instance_marked_available` (limpeza das falhas de um servidor) |
| `report` | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added`, `report_forwarded` (atribuir, desatribuir, resolver e reabrir uma denúncia, adicionar uma nota e encaminhá-la) |
| `user` | `registration_approved`, `registration_rejected` (análise de cadastro) |
| `setting` | `setting_changed` (configurações da instância), `text_changed` (textos da instância) |
| `rule` | `rule_added`, `rule_revised`, `rule_moved`, `rule_retired` (regras da instância) |
| `role` | `role_granted`, `role_revoked` (conceder e revogar uma função de um usuário), `role_created`, `role_updated`, `role_deleted` (criar, alterar e excluir uma função) |
| `appeal` | `appeal_upheld`, `appeal_overturned` (análise de recurso; reverter também grava `account_action_lifted` para a ação, e `account_deletion_canceled` para um `ban`) |
| `signup_block` | `signup_block_added`, `signup_block_lifted` (adicionar e revogar um bloqueio de cadastro) |
| `hook_decision` | `hook_decision_applied` (o servidor agiu sobre o veredito de uma análise externa e deixou o estado; o autor é o sistema), `hook_decision_overridden` (um moderador cancelou a decisão, também ao reverter seu recurso), `hook_appeal_upheld` (um moderador manteve o recurso de uma publicação recusada, e a decisão permanece) |
| `custom_emoji` | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` (registrar um emoji personalizado, trocar sua imagem, copiar um emoji de outro servidor, alterá-lo, ativá-lo, desativá-lo e excluí-lo) |
| `webhook` | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` (criar, alterar, excluir, ativar e desativar um webhook, e gerar seu segredo de novo) |
| `announcement` | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn` (criar, alterar, publicar e retirar um comunicado) |
| `invite` | `invite_created`, `invite_revoked` (criar e revogar um convite; o nome do alvo são os 4 primeiros caracteres do código) |
| `billing_source` | `billing_import_queued` (uma importação de cobrança foi iniciada manualmente; o nome do alvo é o nome do provedor) |
| `cost_item` | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_deleted` (registrar um custo, alterar seu nome e similares, revisar seu valor, alterar sua data de término e excluí-lo; o nome do alvo é o nome do item) |
| `supporter_program` | `supporter_program_enabled`, `supporter_program_disabled` (ativar e desativar a cobrança; o nome do alvo é o nome do provedor de pagamento) |
| `supporter_plan` | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` (criar, alterar e retirar um plano de apoiador; o nome do alvo é o nome do plano) |
| `supporter_subscription` | `supporter_subscription_ended` (um administrador encerrou uma assinatura imediatamente; o nome do alvo é o nome do plano e o identificador do usuário) |
| `custom_emoji_request` | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` (enviar, retirar, aprovar e rejeitar um pedido de emoji personalizado; o nome do alvo é o `:shortcode:`) |
| `payment_provider` | `payment_reconcile_queued` (a conciliação com o provedor de pagamento foi iniciada manualmente; o nome do alvo é o nome do provedor) |
| `oauth_client` | `oauth_client_disabled`, `oauth_client_enabled` (desativar e revogar um cliente OAuth; desativar revoga todos os seus tokens; o nome do alvo é o nome do cliente e o `client_id`) |
| `oauth_client_host` | `oauth_client_host_denied`, `oauth_client_host_allowed` (negar e revogar a negação de um host de Client ID Metadata Document; negar revoga os tokens dos clientes desse host; o nome do alvo é o host) |
| `instance` | `service_closure_announced`, `service_closure_withdrawn`, `service_closure_closed`, `service_closure_delivery_completed` (anunciar o encerramento do serviço ou alterar a data, retirar o anúncio, encerrar e concluir a entrega do encerramento; o nome do alvo é `service_closure`) |
| `voice_room` | `voice_room_ended` (um moderador encerrou uma sala de chat de voz aberta; o nome do alvo é o ID da sala, já que o nome some quando a conta que o escreveu é apagada; os detalhes da denúncia mostram o nome) |
| `link_card_domain` | `link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed` (definir uma regra do rastreador de cartões de link em um domínio e removê-la; veja [domínios de cartões de link](instance-settings.md#link-card-domains)) |
| `login_provider` | `login_provider_created`, `login_provider_updated`, `login_provider_deleted`, `login_provider_enabled`, `login_provider_disabled`, `login_provider_secret_rotated` (adicionar, alterar, excluir, ativar e desativar um provedor de login externo, e trocar seu segredo de cliente; o nome do alvo é o nome de exibição do provedor) |

A lista mostra se o alvo ainda existe. Contas, denúncias, recursos e bloqueios de cadastro que sumiram após o período de retenção, e funções, emojis personalizados e webhooks excluídos, mostram apenas o nome no momento do registro e nenhum link para uma tela.

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

Na API de administração: `GET /api/v1/admin/audit_log` (filtre com `actor_account_id`, `action`, `target_kind` e `target_id`). Ela exige a permissão `view_audit`.

- Uma linha é o autor (o usuário para `user`, ou `cli` para a CLI), a operação, o tipo e o ID do alvo, o nome de exibição do alvo, o ID do registro original e a hora.
  O ID do alvo é o ID da conta, denúncia, recurso, usuário, regra, função ou bloqueio de cadastro; para uma ação sobre domínio é o domínio; para configurações e textos é a chave.
- O nome de exibição do alvo (`@alice`, `@bob@example.com`, um domínio, uma chave de configuração, o texto de uma regra; para um texto sua chave, para uma função sua chave e o identificador do usuário, para um bloqueio de cadastro seu tipo e valor) é uma cópia tirada no momento do registro, então continua legível depois que o alvo some.
  Cada linha da API também retorna se o alvo ainda existe (`target_exists`), que é `false` para contas, denúncias, recursos e bloqueios de cadastro que sumiram. O registro de auditoria web mostra apenas o nome de exibição, sem link, para um alvo que sumiu.
- O texto de motivos e notas não é copiado. Consulte-o pelo ID do registro original (a ação, atribuição, resolução, reabertura, nota, análise ou a versão de uma configuração ou regra).
- A lista vai das mais novas para as mais antigas, com paginação por keyset (`-before <entry id>` na CLI).
- Não há API nem CLI para atualizar ou excluir o registro de auditoria. As linhas antigas são removidas após o [período de retenção](#record-retention-and-holds).

<!-- source: record-retention-and-holds bd806b3bbda7 -->
## Retenção de registros e retenções legais

Depois do período de retenção de cada tipo de registro, jobs horários (`moderation.purge_records`, `identity.purge_origins` e `voice.purge_records`) removem os registros de moderação.
Você pode alterar os períodos nas configurações da instância, mas não abaixo do limite inferior ([configurações da instância](instance-settings.md)). Um período alterado passa a valer a partir da próxima execução. Uma retenção legal liberada, com seu motivo, é removida quando o período de retenção das ações de moderação passa a contar desde a sua liberação.

| Registro | Configuração | Padrão | Limite inferior | Conta a partir de |
| --- | --- | --- | --- | --- |
| Endereço IP e User-Agent da entrada | `session_origins_retention_days` | 180 dias | 90 dias | A entrada |
| Endereço IP do cadastro | `registration_origins_retention_days` | 180 dias | 90 dias | O cadastro |
| Denúncias (comentário, publicações alvo, notas, atribuições, resoluções, reaberturas) | `reports_retention_days` | 365 dias | 180 dias | A última resolução |
| Ações contra contas (motivo, nota, publicações alvo, revogação) e recursos | `moderation_actions_retention_days` | 365 dias | 180 dias | O fim da ação |
| Registro de auditoria | `audit_log_retention_days` | 730 dias | 365 dias | O registro |
| Bloqueios de cadastro | Nenhuma (não pode ser alterada) | 30 dias | n/d | O que vier primeiro entre a revogação e a expiração |
| Quem participou de uma sala de chat de voz (participações, oradores e ouvintes, mãos levantadas, convites, remoções) | `voice_participants_retention_days` | 90 dias | 30 dias | O fim da sala |
| As decisões dos [hooks de decisão](decision-hooks.md#the-decisions) | `hook_decisions_retention_days` | 90 dias | 30 dias | A decisão |

- As denúncias abertas nunca são removidas. Uma denúncia a que uma ação se refere (`report_id` da ação) permanece até essa ação ser removida.
- O fim de uma ação é o que vier primeiro entre a revogação e a expiração. Para uma ação que foi contestada, se a análise vier depois disso, o fim é o momento da análise, e a ação é mantida enquanto o recurso aguarda análise.
  `warn` e `delete_statuses` não têm efeito duradouro, então o fim é o momento em que foram emitidos. Uma ação sem fim (um `ban`, um `suspend` sem horário de término e assim por diante) não é removida até ser revogada.
- As publicações para as quais uma denúncia ou uma ação aponta não são excluídas fisicamente pelo job de limpeza, junto com sua mídia anexada, até essa denúncia ou ação ser removida, mesmo que o autor as tenha excluído (elas somem da vista). As publicações remotas também permanecem além do período de retenção (inclusive a configuração da instância `remote_status_max_retention_days`).
- Um bloqueio de cadastro que não tem nem revogação nem horário de término permanece enquanto estiver em vigor.
- Os registros de quem participou de uma sala de chat de voz para a qual uma denúncia aponta permanecem até essa denúncia ser removida. A sala em si (seu nome, descrição, anfitriões e como terminou) nunca é removida.
- Uma execução do job remove apenas um número limitado de linhas por tipo (100 por vez, 20 vezes, para ações, denúncias e bloqueios de cadastro; 1.000 linhas por vez, 20 vezes, para o registro de auditoria). A execução seguinte remove o resto.

<!-- source: holds f3bf57c2c258 -->
### Retenções legais

Enquanto uma ordem judicial proíbe o apagamento (Lei de Plataformas de Distribuição de Informação do Japão, Artigo 16) ou um pedido de divulgação está sendo tratado, você pode impedir o apagamento por conta.

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- Para uma conta sob retenção legal, suas ações e recursos, as denúncias sobre ela e as que ela fez, os registros de auditoria desses itens, os endereços IP de entrada e de cadastro, suas publicações e os registros das salas de chat de voz de que ela participou ou em que atuou não são removidos após o período de retenção.
  Um bloqueio de cadastro criado a partir do banimento dessa conta também é mantido além de 30 dias após a revogação ou expiração.
- Você pode colocar várias retenções legais em uma conta. Quando você as libera todas, os registros além do prazo são removidos na execução seguinte. Liberar uma retenção já liberada não altera nada.
- As retenções legais e sua liberação são feitas apenas com a CLI, e o autor no registro é a CLI.
- Os dados de uma conta excluída (uma cujo `ban` chegou à data, uma que o usuário excluiu, ou uma conta remota que seu servidor excluiu) são apagados 7 dias após a exclusão pelo job horário `account.erase_deleted`. As publicações de uma conta remota também são apagadas, inclusive as que usuários locais salvaram ou responderam.
  Uma conta remota que foi marcada como excluída apenas porque uma busca respondeu `410 Gone`, sem um `Delete` assinado próprio, é buscada mais uma vez antes. Um segundo `410` a apaga. Se o servidor dela voltar a servi-la, ela volta com as publicações, e nada é apagado. Qualquer outra resposta (`404` ou nenhuma resposta) espera uma execução posterior, e 30 dias após a exclusão a conta é apagada de qualquer modo.
  Uma conta sob retenção legal não é apagada. As publicações para as quais uma denúncia ou ação aponta permanecem até esse registro ser removido. O URI e o nome de usuário da conta e os registros de ações permanecem.
  Os nomes e descrições que a conta deu às suas salas de chat de voz são esvaziados; as salas permanecem.

<!-- source: exporting-for-disclosure-requests 89f1074dd950 -->
### Exportar para pedidos de divulgação

Para um pedido de divulgação (RGPD Artigo 15, Lei de Proteção de Informações Pessoais do Japão Artigo 33), responda exportando todos os registros de moderação de uma conta como JSON.

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- A exportação contém as ações contra a conta (inclusive notas privadas, revogação, recursos e análises), as denúncias sobre a conta (comentário, publicações alvo, regras citadas, notas dos moderadores, atribuições, resoluções, reaberturas), as denúncias que a conta fez (apenas se cada uma está aberta ou resolvida) e os registros de auditoria desses itens.
- A identidade dos denunciantes (a conta denunciante, e o URI e o servidor de origem de uma denúncia recebida) não é exportada. Antes de entregar a exportação, confira que o comentário de uma denúncia não diz nada que identifique o denunciante.
- Um moderador é exportado pelo ID de usuário. Coloque uma retenção legal antes, para que os registros não sejam removidos enquanto você trata o pedido.
