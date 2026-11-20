<!-- source: user-registration-registration-modes-invites-approval-email-confirmation 26f54561fd7a -->
# Cadastro de usuários (modos de cadastro, convites, aprovação, confirmação de e-mail)

Como os usuários se cadastram e confirmam o endereço de e-mail, os modos de cadastro, os códigos de convite, a aprovação de solicitações, as redefinições de senha e os bloqueios de cadastro.

<!-- source: from-sign-up-to-the-app fff50db25fa9 -->
## Do cadastro ao app

![Os caminhos de um novo usuário: o formulário de cadastro ou um provedor de login, as verificações que podem recusar o cadastro, o estado que cada modo de cadastro atribui, e a aprovação e a confirmação de e-mail antes do app. A lista abaixo diz a mesma coisa.](images/sign-up-flow.svg)

Do topo do diagrama para baixo:

- **Início.** Um visitante cria uma conta na página de cadastro (`/auth/sign_up`) com um nome de usuário, um endereço de e-mail e uma
  senha, o que a página não oferece enquanto o [cadastro está limitado a provedores de login](#registration-limited-to-login-providers).
  Ou o visitante faz login com um [provedor de login](login-providers.md) cuja conta ainda não está vinculada a um usuário; quando
  o provedor permite cadastros (`-allow-signup`), o visitante recebe o mesmo formulário sem o campo de senha.
- **Verificações, em ordem.** O modo de cadastro (`closed` recusa todo cadastro), o limite por rede (5 a cada 30 minutos),
  o CAPTCHA (quando `captcha` não é `none`), os [bloqueios de cadastro](#sign-up-blocks) (`reject` recusa, `require_approval`
  retém para aprovação), o código de convite (ele precisa ser utilizável, e o modo `invite` exige um), o plugin de
  [`signup.check`](decision-hooks.md#sign-ups) para um cadastro sem convite, e um nome de usuário e um endereço de e-mail
  que não estejam em uso. Um cadastro que falha em uma verificação é recusado com um erro no formulário, e nada é gravado.
- **Estado logo após o cadastro.** O novo usuário já entra com login. No modo `open`, o cadastro é aprovado,
  ou aguarda aprovação com `mail = none`; no modo `approval`, aguarda aprovação, ou é aprovado com um convite utilizável;
  no modo `invite`, é aprovado. Um bloqueio `require_approval` ou uma retenção por `signup.check` o deixa aguardando em qualquer modo.
- **Até a conta ficar pronta.** Um cadastro que aguarda precisa da aprovação de um moderador (`my registration approve`, ou
  as telas de administração). O endereço de e-mail precisa ser confirmado pelo link de um e-mail com `mail = smtp`; um endereço que
  o provedor de login verificou é confirmado de imediato, e com `mail = none` nenhuma confirmação é necessária. Um cadastro
  rejeitado não é aprovado, e a conta não pode usar o app.
- **O app.** Uma conta pronta vai para a etapa de boas-vindas (um nome de exibição e um avatar) e depois para a linha do tempo inicial.

As seções abaixo descrevem cada etapa.

<!-- source: the-sign-up-page 9a7c6e5bdf58 -->
## A página de cadastro

Os usuários se cadastram em `https://<host>/auth/sign_up` com um nome de usuário, um endereço de e-mail e uma senha, a menos que o [cadastro esteja limitado a provedores de login](#registration-limited-to-login-providers).
Depois do cadastro, esse navegador fica conectado na hora.
Como a página de login, ela é protegida por um token CSRF e pela rejeição de requisições de origem cruzada.
Quando você escolhe um CAPTCHA para proteção contra bots, as páginas de cadastro e de login mostram o widget dele e precisam de JavaScript ([CAPTCHA](captcha.md)).

O modo de cadastro é a configuração da instância `registration_mode`. Escolha-o com a CLI ou no cliente web, na página “Cadastro” de “Configurações do servidor” (`/admin/settings/registration`) ([configurações da instância](instance-settings.md)). Uma mudança vale a partir do próximo cadastro.
Enquanto o servidor anuncia o encerramento e depois que ele encerra, o cadastro é tratado como `closed`, diga o que disser a configuração ([CLI de administração](admin-cli.md#announcing-the-end-of-the-service)).
O mesmo vale em um [servidor de uma só pessoa](instance-settings.md#one-person-server), onde convites, a análise de cadastros e os bloqueios de cadastro também não são oferecidos, e nenhum meio cria uma conta, `my admin create` inclusive.

```sh
my instance set registration_mode invite
```

| Modo | Página | Estado logo após o cadastro |
| --- | --- | --- |
| `open` | Qualquer pessoa pode se cadastrar; um código de convite é opcional | Aprovado, exceto com `mail = none`, em que aguarda a aprovação de um moderador (veja abaixo). Um convite não muda nada além de registrar quem convidou o usuário |
| `approval` (padrão) | Também pergunta “Por que você quer entrar?” (opcional, até 1.000 caracteres); um código de convite é opcional | Aguardando aprovação; aprovado na hora com um convite utilizável |
| `invite` | Um código de convite é obrigatório | Aprovado |
| `closed` | Apenas diz que o cadastro não é aceito | Nenhum |

- Em todos os modos, exceto `closed`, a página tem um campo de código de convite, preenchido a partir do link de um convite (`?invite=<code>`). Um código que não funciona (desconhecido, revogado, expirado, esgotado ou de um membro que não pode mais convidar) é recusado em todos os modos, não ignorado, para que um código digitado errado não passe despercebido.
- Um bloqueio de cadastro que exige aprovação mantém um cadastro aguardando aprovação mesmo com um convite ([bloqueios de cadastro](#sign-up-blocks)).
- Com um plugin definido para o gancho de decisão [`signup.check`](decision-hooks.md#sign-ups), um cadastro sem convite também é analisado por esse plugin depois do CAPTCHA e dos bloqueios, e ele pode retê-lo para aprovação ou recusá-lo.

- Os cadastros de uma mesma rede (endereço IP; `/64` para IPv6) são limitados a 5 a cada 30 minutos. Além disso o servidor retorna `429` com `Retry-After`.
- Se o nome de usuário ou o endereço de e-mail já está em uso, a página mostra um erro nesse campo e não grava nada (um convite também não é consumido).
- Quando você configura [provedores de login externos](login-providers.md) (OpenID Connect, GitHub, Discord ou Mastodon) e deixa novos usuários se cadastrarem com um, as páginas de login e de cadastro mostram um botão para cada provedor ativado, rotulado com o nome de exibição do provedor exatamente como você o digitou (um provedor GitHub mostra “Entrar com GitHub”, e um Discord “Entrar com Discord”, no idioma do visitante).
  Quem tem uma conta no provedor que não está vinculada a um usuário deste servidor recebe o mesmo formulário de cadastro sem o campo de senha, e o novo usuário entra apenas com o provedor.
  O modo de cadastro, o código de convite, o motivo, o CAPTCHA, os bloqueios de cadastro, `signup.check` e o limite por rede se aplicam como no formulário de senha; no modo `closed`, ou quando o provedor não permite cadastros, a página de login diz que nenhuma conta está vinculada.
  Um endereço que o provedor marca como verificado (`email_verified`; no GitHub, o endereço principal quando verificado; no Discord, o endereço quando o Discord o marca como verificado) é confirmado na hora se o usuário o mantém; qualquer outro endereço, e todo endereço de um usuário que se cadastra com um servidor Mastodon, que não fornece nenhum, é confirmado por e-mail como abaixo.
  Um endereço de e-mail que corresponde a um usuário existente nunca vincula esse usuário: o formulário diz que o endereço já está em uso.
  Um usuário com autenticação em dois fatores ainda digita o código depois de entrar com um provedor.
  Um provedor GitHub pede ao GitHub o escopo `user:email`, e também `read:org` quando você lista Organizations permitidas; o servidor então verifica a cada login que o usuário pertence a uma delas e recusa o login caso contrário. Registre `https://<host>/auth/external/<provider ID>/callback` como a URL de callback do seu app OAuth do GitHub.
  Um provedor Discord pede ao Discord os escopos `identify` e `email`, e também `guilds` quando você lista servidores permitidos; o servidor então verifica a cada login que o usuário pertence a um deles. Registre a mesma URL de callback no seu aplicativo do Discord.
- Um usuário conectado vincula uma conta de um provedor em “Contas vinculadas” nas configurações, depois de confirmar com a senha, uma passkey ou outra conta vinculada, e apenas para provedores que permitem vínculos.
  O navegador precisa estar conectado nas páginas de login como o mesmo usuário; caso contrário o servidor pede para entrar primeiro.
  Uma conta de um provedor que outro usuário vincula é recusada, e um usuário vincula no máximo uma conta por provedor.
  Um usuário pode desvincular uma conta, a menos que ela seja a última forma de entrar (sem senha, sem passkey e sem outra conta vinculada).
- Um usuário sem senha e sem passkeys confirma as operações que de outro modo as pedem (definir uma senha, adicionar uma passkey, excluir ou mover a conta, vincular outra conta) entrando de novo em um provedor vinculado.
  Um provedor OpenID Connect é solicitado a autenticar o usuário de novo (`max_age=0` e `prompt=login`), e uma autenticação anterior é recusada.
  A confirmação vale uma vez, apenas para essa operação, em até 10 minutos; entrar com uma conta que não é a vinculada conta como uma tentativa errada, contra o mesmo limite de uma senha errada.
- Em todo cadastro e login, o servidor registra o endereço IP do cliente (e, no login, o User-Agent). Os registros são removidos automaticamente após 180 dias
  (as configurações da instância `session_origins_retention_days` e `registration_origins_retention_days`, de no mínimo 90 dias). Os registros de uma conta sob retenção legal são mantidos ([retenção de registros e retenções legais](moderation.md#record-retention-and-holds)).
  Informe esse prazo de retenção na sua política de privacidade ([modelo de política de privacidade](privacy-policy-template.md)).

<!-- source: when-a-user-can-use-the-app 317d6d8b7688 -->
## Quando um usuário pode usar o app

Os usuários que aguardam aprovação, foram rejeitados ou não confirmaram o endereço de e-mail ainda podem fazer login na página de login. O mesmo vale para os usuários que desativaram a própria conta, até a data da exclusão: eles veem essa data ali e podem reativar ou excluir a conta ([moderação](moderation.md#accounts-that-their-owners-deactivated)).
Depois do login, eles veem o que estão aguardando (a aprovação de um moderador ou a confirmação do endereço de e-mail), ou que o cadastro deles não foi aprovado, e não podem avançar.
Somente usuários aprovados podem usar o app (a autorização OAuth e a emissão de tokens da API de cliente) e, nas configurações que exigem confirmação de e-mail, somente os que confirmaram o endereço de e-mail.

Um novo usuário que pode usar o app de imediato vai da página de cadastro direto para o app, para uma etapa de boas-vindas que pede
um nome de exibição e um avatar. A linha do tempo inicial então mostra uma lista de primeiros passos e sugestões de pessoas
para seguir ([sugestões](instance-settings.md#suggestions-of-accounts)). “Continuar” na página que confirma um endereço
de e-mail leva à mesma etapa de boas-vindas.

Um administrador criado com `my admin create` conta como aprovado e confirmado.

<!-- source: email-confirmation 6c1b7759c2cf -->
## Confirmação de e-mail

A configuração `mail` ([referência de configuração](../reference/configuration.md#mail)) seleciona como o e-mail é enviado.
Exemplos de SMTP, a configuração de DNS e como ler falhas de entrega estão em [envio de e-mail](email.md).

- `mail = smtp`: o servidor envia um e-mail de confirmação no cadastro. O link vale por 24 horas e funciona uma vez.
  Abrir o link mostra uma página com um botão de confirmar, e o endereço é confirmado quando o usuário o pressiona (para que scanners que fazem pré-carregamento de links em e-mails não consumam o token).
  Depois do login, o usuário pode pedir o reenvio do e-mail até 3 vezes por hora.
  A conexão com o servidor SMTP é sempre criptografada com STARTTLS, e o servidor verifica o certificado antes de autenticar e enviar. Ele nunca envia a um servidor que não ofereça STARTTLS.
- `mail = none` (padrão): o servidor não envia e-mail e grava `mail not sent (mail = none)` com a categoria da mensagem no log do servidor. Somente com `profile = development` a linha também traz o endereço e o corpo, incluindo o link de confirmação.
  Como o e-mail nunca chega ao usuário, o servidor pede a aprovação de um moderador no lugar da confirmação de e-mail.
  Assim, um cadastro no modo `open` também começa aguardando aprovação, e um endereço de e-mail não confirmado não impede o uso do app.

O token de confirmação não é registrado em nenhum lugar além dessa linha do log de desenvolvimento (os logs de acesso não registram query strings). O banco de dados armazena apenas o SHA-256 do token.

<!-- source: registration-limited-to-login-providers 0d05572b3521 -->
## Cadastro limitado a provedores de login

Enquanto a configuração de instância `login_providers_only` estiver ativada e pelo menos um [provedor de login](login-providers.md#limit-sign-in-to-login-providers) estiver ativado, a página de cadastro não tem formulário de senha e oferece apenas os provedores que permitem cadastros; um código de convite no link passa para o formulário de cadastro do provedor. Um cadastro com senha é recusado, seja qual for o conteúdo do formulário. Sem nenhum provedor que permita cadastros, o cadastro é tratado como `closed`. Sem nenhum provedor ativado, a configuração não se aplica.

<!-- source: password-reset-and-change bec2baef5ab7 -->
## Redefinição e alteração de senha

Enquanto a configuração de instância `login_providers_only` se aplicar, os links de redefinição e a definição ou alteração de senha são recusados ([o que a restrição faz](login-providers.md#limit-sign-in-to-login-providers)).

- `mail = smtp`: quando um usuário digita um endereço de e-mail em “Esqueceu a senha?” na página de login, o servidor envia um link para definir uma nova senha ao usuário cujo endereço principal é esse.
  A resposta da página é a mesma exista ou não uma conta. O link vale por 1 hora e funciona uma vez, e deixa de funcionar se a senha mudar por outro caminho nesse meio-tempo.
  As solicitações são limitadas a 5 a cada 30 minutos por endereço IP de cliente, e os e-mails a 3 por hora por endereço (uma solicitação acima de um limite não envia nada e recebe a mesma resposta). Quando o CAPTCHA está ativado, este formulário também é verificado.
- `mail = none`: a página diz que não consegue enviar um link e orienta o usuário a procurar um administrador. O administrador emite um link na página Gerenciamento da conta, nas telas de administração, ou com `my admin account password-reset-link @alice`, e o entrega ao usuário ([moderação](moderation.md)).
- Um usuário com login feito pode alterar a senha em “Senha” nas configurações, digitando a senha atual (entradas erradas são limitadas a 5 por hora por usuário).
- Uma redefinição ou uma alteração revoga as outras sessões e os tokens de apps do usuário (uma alteração feita nas configurações mantém apenas o app que fez a alteração). Chaves de acesso e autenticação de dois fatores permanecem.
  Um login que aguarda o código de dois fatores e uma autorização de app ainda não concluída também deixam de funcionar: o usuário os inicia de novo.
  Com `mail = smtp`, o servidor também envia um e-mail ao usuário avisando que a senha mudou.

<!-- source: invite-codes 4d49dd1f0927 -->
## Códigos de convite

Os convites funcionam nos modos `open`, `approval` e `invite`. Os convites da equipe são feitos com a CLI, as telas de moderação e a API de administração abaixo; os membros também podem fazer os próprios convites ([convites de membros](#member-invites)).
No modo `closed` nenhum convite pode ser criado e nenhum funciona.

```sh
my invite create                          # single use, expires in 7 days
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # no limit on uses or time
my invite list                            # newest first; states: usable / revoked / expired / used up
my invite revoke <code>
```

- `create` imprime o código (16 caracteres) e o link para a página de cadastro com o código preenchido (`https://<host>/auth/sign_up?invite=<code>`).
- Os códigos são comparados ignorando maiúsculas e minúsculas, espaços e hifens.
- Mesmo quando várias pessoas se cadastram ao mesmo tempo, um código nunca é usado além do seu limite (uma restrição de unicidade em `invite_redemptions`).
- Um código revogado, expirado ou esgotado dá o mesmo erro (um convite inutilizável).
- Além da CLI, você pode criar, listar e revogar convites nas telas de Moderação do cliente web (“Convites”, `/moderation/invites`) e pela API do cliente
  (`GET` / `POST /api/v1/admin/invites` e `POST /api/v1/admin/invites/{id}/revoke`). Ambos exigem a permissão de papel `manage_invites`
  (o `moderator` embutido não a tem, então, para delegar isso, crie um papel com essa permissão e conceda-o).
  Um serviço que distribui convites por conta própria chama a mesma API com um token do escopo `admin:manage_invites` ([permitindo que outro serviço use a API de administração](moderation.md#letting-another-service-use-the-admin-api)).
  “Criar um convite”, na lista, abre o formulário (`/moderation/invites/new`), que mostra o código e o link de cadastro do novo convite.
  A lista mostra o código de cada convite, quem o criou, quantos se cadastraram com ele, o término e o estado; escolha convites e escolha “Revogar”, que pede confirmação antes.
- A lista mostra todos os convites, os da equipe e os dos membros, com quem os criou (`created_by`) e como (`creator_kind`: `staff` ou `member`). Quem tem `manage_invites` pode revogar qualquer um deles.
- A página Análise da conta nas telas de moderação (`GET /api/v1/admin/accounts/{id}`, permissão `manage_accounts`) mostra o convite com que a conta se cadastrou e quem o criou, a pessoa que convidou o usuário.
- Criar e revogar um convite fica registrado no log de auditoria (`invite_created`, `invite_revoked`) pela tela, pela API e pela CLI igualmente. O nome do alvo são os 4 primeiros caracteres do código (`ABCD…`); o código completo não é copiado para o log de auditoria.
  Revogar um convite já revogado não muda nada, então não é registrado.

<!-- source: member-invites 85edfb5c0806 -->
## Convites de membros

Os membros podem criar convites próprios, além da equipe.
Quem pode é a configuração da instância `invite_creators` ([configurações da instância](instance-settings.md)):

- `permitted` (padrão): apenas os usuários cujos papéis têm a permissão `create_invites`. Apenas o papel `admin` embutido a tem por padrão. Você pode adicioná-la a um papel que concede manualmente, ou ao papel de um [plano de apoiador](supporters.md), para que os apoiadores possam convidar pessoas.
- `everyone`: todo usuário local conectado.

De qualquer modo, um usuário cuja conta está suspensa ou limitada, ou cujo cadastro não foi aprovado, não pode criar convites, e ninguém pode enquanto o modo de cadastro é `closed`.
Quem tem `manage_invites` continua fazendo convites da equipe, aos quais nenhum dos limites abaixo se aplica.

Um membro cria, lista e revoga os próprios convites, e copia o link de um convite utilizável, nas configurações do cliente web (“Convites”, `/settings/invites`), ou pela API do cliente (`GET` / `POST /api/v1/invites` e `POST /api/v1/invites/{id}/revoke`).
As configurações listam a página apenas para um usuário que pode criar convites agora; um usuário que não pode ainda a abre pelo endereço dela, onde ela diz o motivo, para revogar os convites que fez.
A lista diz, para cada convite, se ele é utilizável, revogado, expirado, esgotado ou inutilizável enquanto o criador não pode criar convites.

Os limites, também configurações da instância, se aplicam apenas aos convites de membros.
Os administradores os alteram, e `invite_creators`, em “Convites de membros” na página “Cadastro” das configurações do servidor (`/admin/settings/registration`):

| Configuração | Significado | Padrão | Faixa |
| --- | --- | --- | --- |
| `member_invite_max_active` | Quantos convites utilizáveis (não revogados, expirados nem esgotados) um membro pode ter ao mesmo tempo | `5` | 0-100 (0: os membros não podem criar convites) |
| `member_invite_max_uses` | Quantas pessoas podem se cadastrar com um convite | `10` | 1-1.000 |
| `member_invite_max_lifetime_seconds` | Por quanto tempo um convite pode funcionar, em segundos | `2592000` (30 dias) | 3.600 (1 hora) a 31.536.000 (365 dias) |

- Um membro escolhe os usos e a validade dentro dos limites; sem escolha, um convite permite 1 cadastro e funciona por 7 dias (ou menos, quando um limite é menor). Um convite sem limite de usos ou de tempo não pode ser criado.
- Reduzir um limite não altera os convites existentes; ele se aplica ao próximo convite criado.
- Todo cadastro com o convite de um membro verifica se o criador ainda pode criar convites. Quando o criador perdeu `create_invites` (por exemplo, o plano dele terminou), `invite_creators` voltou a `permitted`, ou a conta foi suspensa, limitada ou excluída, o convite não funciona e a página de cadastro diz que ele não é válido.
  O convite em si não é alterado, então ele volta a funcionar, dentro da validade, quando o criador puder convidar de novo.
- Criar e revogar de um membro ficam registrados no log de auditoria como `invite_created` e `invite_revoked`, com o membro como autor.

No modo `approval`, um convite utilizável aprova o novo usuário na hora: quem o convidou substitui a análise dos moderadores.
Com `invite_creators = everyone`, todo usuário pode portanto deixar pessoas entrarem sem análise, o que enfraquece o modo de aprovação.
Se você depende da análise, mantenha `permitted` e dê `create_invites` apenas a pessoas em quem confia, ou mantenha `member_invite_max_active` e `member_invite_max_uses` baixos.

<!-- source: approving-and-rejecting 34e7c6193247 -->
## Aprovando e rejeitando

```sh
my registration list                      # waiting for approval (longest wait first); shows the reason and whether the email is confirmed
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- O argumento é o nome de usuário de uma conta local (o `@` inicial pode ser omitido).
- Apenas os cadastros que aguardam aprovação podem ser decididos. Para um cadastro já decidido, ou um usuário criado sem a página de cadastro (`my admin create`), o comando não muda nada e termina com o código 1.
- O código de saída 2 significa um argumento errado.

As mesmas decisões podem ser tomadas nas telas de administração (a API do cliente). Todas exigem a permissão `manage_accounts`.
No cliente web, Moderação → Cadastros (`/moderation/registrations`) lista os cadastros que aguardam aprovação com o endereço de e-mail, se ele está confirmado e o motivo para entrar; escolha-os e escolha “Aprovar” ou “Rejeitar”, que pede confirmação antes, com uma nota para os moderadores. Um servidor de uma só pessoa não a mostra.

| Operação | O que faz |
| --- | --- |
| `GET /api/v1/admin/registrations` | Cadastros aguardando aprovação (mais antigos primeiro). Retorna o ID do cadastro, o motivo para entrar, o endereço de e-mail e se ele está confirmado, e a conta local |
| `POST /api/v1/admin/registrations/{id}/approve` | Aprova. O corpo `{"note": "..."}` é opcional. A resposta mostra a nota como `decision_note`, e uma resposta sobre um cadastro decidido não tem endereço de e-mail |
| `POST /api/v1/admin/registrations/{id}/reject` | Rejeita. O corpo é o mesmo de `approve` |

- Este é o mesmo caso de uso da CLI, e ele registra a decisão em `registration_decisions` e no log de auditoria (`registration_approved`, `registration_rejected`). O autor de uma decisão da CLI é `cli`; o autor de uma decisão da API é o moderador que decidiu.
- Decidir um cadastro já decidido não muda nada e retorna o estado atual (se duas pessoas decidem ao mesmo tempo, apenas uma decisão é registrada). Um usuário criado sem a página de cadastro dá 404.

<!-- source: sign-up-blocks b70eb18cd52f -->
## Bloqueios de cadastro

Os bloqueios rejeitam cadastros feitos pela página de cadastro, ou os passam para aprovação, por domínio de e-mail, endereço de e-mail ou intervalo de IP.

```sh
my admin signup-block create -kind email_domain spam.example          # also applies to subdomains; a U-label works too
my admin signup-block create -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block create -kind email_hash spammer@example.com     # only a hash of the address is kept
my admin signup-block list -active
my admin signup-block lift <block id>
```

- Um cadastro que corresponde a um bloqueio com `-severity reject` (o padrão) é recusado, e um que corresponde a `require_approval` começa aguardando aprovação, seja qual for o modo de cadastro. Quando vários correspondem, vale o mais forte. A página de cadastro não mostra o motivo de uma recusa.
- `email_hash` é comparado com o endereço em minúsculas, sem a parte da parte local a partir do primeiro `+`. Um intervalo de IP é comparado com o IP do cadastro (o mesmo valor de `registration_origins`) e nunca se aplica a um cadastro cujo IP é desconhecido.
- Bloqueios expirados e revogados não têm efeito. Adicionar e revogar ficam registrados no registro de auditoria (`signup_block_added`, `signup_block_lifted`).
- Quando você bane uma conta local com `-block-email` (`block_email: true` na API), o servidor também cria um bloqueio `email_hash` para o endereço de e-mail desse usuário.

Para uma lista de milhares de entradas, como os domínios de serviços de e-mail descartável, adicione-as em lote com `import` a partir de um arquivo (`-` para a entrada padrão). Todo valor recebe o mesmo tipo, gravidade, comentário e duração.

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- A lista tem um valor por linha (a mesma forma de `<value>` em `add`), ou linhas CSV cuja primeira coluna é o valor. Linhas em branco e linhas que começam com `#` são ignoradas. Uma importação aceita até 10.000 entradas.
- O comando imprime quantas foram adicionadas e, para cada linha que não adicionou, o número da linha, o valor e o motivo. O motivo é `invalid` (não é um valor válido daquele tipo), `duplicate` (a mesma forma normal de uma linha anterior da mesma lista) ou `already_blocked` (há em vigor um bloqueio do mesmo tipo e valor, igual ou mais severo).
- Cada bloqueio adicionado fica registrado no registro de auditoria (`signup_block_added`) como em `add`, e pode ser revogado um a um.
- A importação grava em transações de algumas centenas, então, se falhar no meio, o que foi adicionado antes permanece. Importar a mesma lista de novo adiciona apenas o restante.
- O software não traz nenhuma lista e não busca uma de uma URL periodicamente. Você busca e atualiza listas públicas (como disposable-email-domains no GitHub) por conta própria e importa a lista atualizada de novo.

As mesmas operações estão disponíveis nas telas de administração (a API de cliente). Todas exigem a permissão `manage_accounts`.
No cliente web, abra Moderação → Bloqueios de cadastro (`/moderation/signup-blocks`). A lista mostra o valor, o tipo, o estado e o fim de cada bloqueio, em vigor ou todos;
“Adicionar um bloqueio” abre o formulário (`/moderation/signup-blocks/new`), “Adicionar de uma lista” importa uma lista e mostra as linhas que ignorou (`/moderation/signup-blocks/import`),
e escolher um valor abre a página do bloqueio com o efeito e o comentário dele, onde “Revogar” o encerra.

| Operação | O que faz |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | Lista bloqueios (os mais novos primeiro). Filtre com `kind` e `active` |
| `POST /api/v1/admin/signup_blocks` | Adiciona um bloqueio. O corpo tem `kind`, `value`, `severity`, `comment` e `expires_in` (segundos, no máximo dez anos) |
| `POST /api/v1/admin/signup_blocks/import` | Adiciona bloqueios a partir de uma lista. O corpo tem `kind`, `text` (a lista), `severity`, `comment` e `expires_in`. A resposta tem `added`, o número adicionado, e `skipped`, as linhas não adicionadas (`line`, `value`, `reason`) |
| `GET /api/v1/admin/signup_blocks/{id}` | Retorna um bloqueio |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | Revoga um bloqueio. Um bloqueio já revogado permanece como está |
