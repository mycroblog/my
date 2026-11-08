<!-- source: external-login-providers 48a582ab87ec -->
# Provedores de login externos

Um provedor de login externo permite que os usuários entrem com uma conta que já têm em outro lugar: um provedor OpenID
Connect (Google Workspace, Microsoft Entra ID, Keycloak e semelhantes), o GitHub, o Discord ou um único servidor Mastodon.
Você configura os provedores com o comando `my login-provider` em um host que tenha a configuração do servidor, ou pela
API de administração (`/api/v1/admin/login_providers`) com a permissão `manage_login_providers`, que somente a função
integrada `admin` tem. O comando funciona antes de qualquer administrador conseguir entrar, então você pode usá-lo na
primeira configuração.
No cliente web, os administradores com essa permissão usam “Provedores de login” no menu de administração. A lista mostra
o tipo de cada provedor, o estado e se o cadastro é permitido; “Adicionar um provedor” abre o formulário que cria um.
Ao escolher um provedor, abre-se a página dele, que mostra a URL de callback com um botão para copiá-la e quantos usuários
estão vinculados, e onde você altera as configurações, gera um novo segredo do cliente, ativa, desativa e exclui o provedor.

<!-- source: what-external-sign-in-does 25fa37e22a50 -->
## O que o login externo faz

- Um provedor apenas comprova quem alguém é naquele provedor. Uma conta nunca é vinculada a um usuário pelo endereço de
  e-mail: o usuário vincula uma conta em “Contas vinculadas”, nas configurações, ou se cadastra com ela como novo usuário.
  Um endereço de e-mail igual ao de um usuário existente nunca vincula esse usuário.
- Um usuário vinculado ainda informa o código de dois fatores (TOTP) depois de entrar com um provedor, e um usuário cuja
  conta não está ativa não consegue entrar.
- Novos usuários só podem se cadastrar com um provedor quando você permite isso para ele (`-allow-signup`) e o
  [modo de cadastro](registration.md) permite que se registrem; o código de convite, o CAPTCHA, os bloqueios de cadastro e
  o limite de rede valem como no formulário de senha. Quem se cadastra assim não tem senha e entra somente pelo provedor.
  O [diagrama dos caminhos de cadastro](registration.md#from-sign-up-to-the-app) mostra as duas formas de cadastro lado a lado.

<!-- source: kinds c9123566b5c5 -->
## Tipos

| Tipo | `-issuer` | Condições |
| --- | --- | --- |
| `oidc` | A URL do emissor (`https`), exatamente como os tokens de ID do provedor o nomeiam, como `https://accounts.google.com` | `-claim <claim>=<value>`: somente usuários cujo token de ID tenha a claim com um dos valores informados para ela |
| `github` | Nenhum: o servidor conhece os endpoints do github.com | `-org <organization>`: somente membros de uma das organizações (adicione `-scope read:org`) |
| `discord` | Nenhum: o servidor conhece os endpoints do discord.com | `-guild <server ID>`: somente membros de um dos servidores do Discord (guilds); sem isso, qualquer usuário do Discord entra |
| `mastodon` | O domínio web do servidor, como `mastodon.example` (Mastodon 4.3 ou posterior) | Nenhuma |

Para o Google Workspace, limite os usuários à sua organização com `-claim hd=<your domain>`. Não confie no domínio do
endereço de e-mail.

<!-- source: examples 0d19ab463197 -->
## Exemplos

<!-- source: google b99294b8539d -->
### Google

No console do Google Cloud, crie um cliente OAuth do tipo “Aplicativo da Web”. Para restringi-lo à sua organização,
defina o tipo de usuário da tela de consentimento OAuth como “Interno” (disponível quando o projeto pertence a uma
organização do Google Workspace) e adicione também a condição `hd`: o tipo de usuário restringe quem o Google deixa
entrar, e a condição faz este servidor verificar a claim `hd` de cada token de ID.

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

<!-- source: keycloak-and-other-openid-connect-providers 0840dc38a4e0 -->
### Keycloak e outros provedores OpenID Connect

O emissor é a URL do realm, como `https://sso.example.com/realms/main`, exatamente como os tokens de ID do provedor o
nomeiam. Crie um cliente confidencial com o fluxo padrão e os escopos `openid`, `email` e `profile`. Se o provedor estiver
em uma rede privada, inclua-o em `login_provider_allowed_destinations` ([abaixo](#providers-on-an-internal-network)).
Use `-claim` para exigir uma claim, como uma claim de grupo, com um dos valores informados.

<!-- source: discord 00e9659d1eae -->
### Discord

Crie um aplicativo no [Discord Developer Portal](https://discord.com/developers/applications), abra “OAuth2” e informe ao
servidor o ID do cliente e o segredo do cliente. O servidor solicita os escopos `identify` e `email`, e também `guilds`
quando você lista servidores com `-guild`. Um usuário é identificado pelo ID numérico do Discord; o nome de usuário e o
nome de exibição apenas sugerem o nome de usuário no formulário de cadastro. O endereço de e-mail só conta como
verificado quando o Discord diz que ele está verificado; caso contrário, o usuário o confirma por e-mail, como qualquer
outro endereço.

Com `-guild`, o servidor verifica a cada login, cadastro e vinculação que o usuário pertence a um dos servidores listados
e, caso contrário, recusa o usuário. Sem ele, qualquer usuário do Discord pode entrar. O ID de uma guild é um número de
até 20 dígitos: no Discord, ative o “Modo desenvolvedor” (Configurações de usuário, Avançado), clique com o botão direito
no ícone do servidor e escolha “Copiar ID do servidor”.

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider create -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

Adicione a URL de callback em “OAuth2”, “Redirects” no aplicativo, depois que `add` a imprimir.

<!-- source: github a7181ea6edd6 -->
### GitHub

Crie um OAuth App (Settings, Developer settings, OAuth Apps) e informe a ele o ID e o segredo do cliente. O servidor
solicita o escopo `user:email`, e também `read:org` quando você lista organizações com `-org`. Com `-org`, o servidor
verifica a cada login que o usuário pertence a uma das organizações, então membros de outras organizações são recusados.

<!-- source: set-up-a-provider 8e0bb73251d2 -->
## Configurar um provedor

O URI de redirecionamento de que o provedor precisa é a URL de callback do provedor,
`https://<host>/auth/external/<id>/callback`, que contém o ID que o servidor dá ao provedor. Portanto:

1. Registre um cliente OAuth no provedor (um “OAuth App” no GitHub) com qualquer URI de redirecionamento por enquanto e
   anote o ID e o segredo do cliente que ele fornece.
2. Execute `my login-provider create` com eles. O comando imprime a URL de callback.
3. Altere o URI de redirecionamento do cliente no provedor para essa URL de callback.

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider create -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

<!-- source: mastodon 6f167bff1f67 -->
### Mastodon

Um provedor `mastodon` é um único servidor Mastodon que você indica; usuários de outros servidores não conseguem entrar
com ele. O servidor precisa ser Mastodon 4.3 ou posterior: `add` lê os metadados do servidor de autorização
(`/.well-known/oauth-authorization-server`) e recusa um servidor que não os tenha. Sem `-client-id`, `add` registra um
cliente no próprio servidor, com a URL de callback e o escopo `profile`, e guarda o ID e o segredo do cliente que recebe,
então não há nada a fazer no servidor:

```sh
my login-provider create -kind mastodon -name mastodon.example -issuer mastodon.example
```

Se o servidor não permite que outros registrem clientes, `add` falha. Nesse caso, crie um aplicativo com o escopo
`profile` e a URL de callback nas configurações de desenvolvimento do servidor e informe o ID e o segredo do cliente como
nos outros tipos (a URL de callback contém o ID do provedor, então siga os três passos acima).

Os usuários entram somente com o escopo `profile`, que lê a conta deles e nada mais. O Mastodon não fornece endereço de
e-mail, então quem se cadastra com ele confirma o endereço por e-mail. O formulário de cadastro sugere o nome de usuário
da conta. Enquanto o domínio do servidor, ou um domínio pai, tiver uma ação de domínio ([ações de domínio](moderation.md#domain-actions)),
o botão dele some e ninguém entra, se cadastra ou vincula uma conta com ele. O servidor de um provedor não muda; configure
um novo provedor para outro servidor.

O segredo do cliente é armazenado criptografado com a chave de criptografia de chaves ([gerenciamento de chaves](key-management.md))
e nunca é mostrado novamente, nem pelo comando nem pela API de administração.

Um novo provedor vem ativado. Por padrão, os usuários podem vincular a conta existente a ele (`-allow-link`), mas ninguém
pode se cadastrar com ele: adicione `-allow-signup` para permitir que novos usuários se registrem com ele, o que o modo
de cadastro ainda limita ([cadastro](registration.md)).

<!-- source: change-rotate-disable-and-delete 55a0d23ac0d0 -->
## Alterar, trocar o segredo, desativar e excluir

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` altera somente as configurações que ele nomeia. `-scope`, `-claim` e `-org` substituem a lista inteira. O tipo de
  um provedor não muda; configure um novo provedor.
- `rotate-secret` substitui o segredo do cliente pelo que você informar. Troque-o primeiro no provedor e depois aqui.
- `disable` mantém o provedor e as contas vinculadas a ele, mas ninguém entra, se cadastra ou vincula com ele, e a página
  de login não o mostra, até você executar `enable`. `disable` e `delete` imprimem quantos usuários vinculados não podem
  mais entrar com o provedor e o que o [limite aos provedores de login](#limit-sign-in-to-login-providers) faz agora,
  enquanto a configuração dele está ativada.
- `delete` remove o provedor definitivamente.

<!-- source: users-who-sign-in-only-through-a-provider 11dc6f466cb7 -->
### Usuários que entram somente por um provedor

Desativar ou excluir um provedor não afeta os usuários. Um usuário que tem senha ou passkey, ou outra conta vinculada,
entra por esse meio, a menos que o [login esteja limitado aos provedores de login](#limit-sign-in-to-login-providers) e
outro provedor continue ativado. Um usuário que não tem nada disso não consegue mais entrar: `disable` e `delete`
imprimem quantos usuários vinculados perdem o provedor, e o cliente web mostra o número na confirmação. `enable` restaura
os usuários de um provedor desativado; um provedor excluído não pode voltar.

Para recuperar um usuário assim, emita um link de redefinição de senha na página Gerenciamento da conta nas telas de
administração, ou com `my admin account password-reset-link @alice`, e entregue-o ao usuário ([cadastro](registration.md#password-reset-and-change)).
O usuário então entra com a nova senha.

Toda alteração é registrada no registro de auditoria sob o nome de exibição do provedor, como a CLI ou como o
administrador que a fez pela API de administração.

<!-- source: limit-sign-in-to-login-providers bb5ebf61b647 -->
## Limitar o login aos provedores de login

A configuração da instância `login_providers_only` (desativada por padrão) torna os provedores a única forma de entrar e
de se registrar, enquanto pelo menos um provedor estiver ativado. Ative-a na página “Cadastro” de “Configurações do servidor”
no cliente web, ou com a CLI:

```sh
my instance set login_providers_only true
```

A confirmação no cliente web e a saída do comando informam quantos usuários locais ativos não têm vínculo com um provedor
ativado. O cliente web também avisa quando isso inclui você. Ele avisa e não recusa: a decisão é sua.

Enquanto o limite vale:

- Senhas e passkeys não fazem ninguém entrar, e a página de login mostra somente os provedores. Um login que tinha
  começado com senha ou passkey e aguarda o código de dois fatores também é recusado.
- Ninguém se registra com senha. A página de cadastro mostra os provedores que permitem cadastro (`-allow-signup`), sob o
  [modo de cadastro](registration.md) como de costume. Sem nenhum provedor assim, o cadastro aparece como `closed`, também
  para os clientes e no NodeInfo.
- Ninguém solicita nem usa um link de redefinição de senha. Os links continuam válidos até expirar e funcionam se o limite
  acabar a tempo.
- Os usuários não podem definir nem alterar a senha, nem adicionar uma passkey, porque nenhuma das duas faria alguém
  entrar. Ainda podem removê-las.
- `my admin account password-reset-link` e as telas de administração ainda emitem um link e avisam que ele não funciona
  até o limite acabar. `my admin create` ainda cria um administrador e avisa que a senha não consegue fazer login.
- Um usuário não pode desvincular a última conta em um provedor ativado.

O que continua funcionando:

- Sessões, tokens de acesso de aplicativos e tokens de atualização continuam válidos. O limite não encerra nenhum deles.
- Uma senha ou uma passkey ainda confirma operações em uma sessão. É assim que um usuário existente passa para um
  provedor: com a sessão iniciada, ele abre “Contas vinculadas” nas configurações, confirma com a senha ou a passkey e
  entra no provedor.
- Login, cadastro e vinculação em um provedor, e o código de dois fatores depois do login por um provedor.

Toda recusa tem o mesmo texto, qualquer que seja o endereço de e-mail, a senha ou o token enviado, então ela não revela
nada sobre nenhuma conta.

**Usuários sem vínculo não conseguem entrar depois que as sessões deles terminam, até você agir.** Uma conta nunca é
vinculada pelo endereço de e-mail, então o login de um provedor também não os encontra. Peça que vinculem uma conta
enquanto estão com a sessão iniciada. Um usuário que perdeu a sessão precisa que você encerre o limite por um tempo, ou
que o mantenha de fora. Isso inclui um usuário sob `disable_login`, `suspend` ou `ban`, que sem um vínculo não tem página
de login de onde fazer um [recurso](moderation.md).
Vincular também exige que o navegador esteja com a sessão iniciada nas páginas de login: um usuário cujo aplicativo ainda
está conectado, mas cuja sessão do navegador terminou, não consegue vincular enquanto o limite vale e precisa esperar por
você. **Avise seus usuários da mudança antes de ativar o limite,** para que vinculem uma conta enquanto as sessões duram.

<!-- source: end-the-limit 77f5c71d013b -->
### Encerrar o limite

O limite acompanha a configuração e os provedores de uma requisição para a outra, sem reiniciar.

- Um administrador que ficou de fora executa isto em um host que tenha a configuração do servidor:

  ```sh
  my instance set login_providers_only false
  ```

- Quando nenhum provedor está ativado, o limite não vale: desativar ou excluir o último provedor reabre o login por senha
  e por passkey, seja qual for a configuração. Usuários sem senha nem passkey ainda precisam de um
  [link de redefinição](registration.md#password-reset-and-change).
- Com a configuração ativada e nenhum provedor ativado, configurar ou ativar um provedor inicia o limite imediatamente; um
  novo provedor fica ativado assim que é configurado. O cliente web avisa antes, com quantos usuários ativos não têm
  vínculo com esse provedor e se isso inclui você. Ele avisa apenas para a ação que iniciaria o limite. `my login-provider create`,
  `enable`, `disable` e `delete` imprimem o que o limite faz agora, enquanto a configuração está ativada.
- Quando uma indisponibilidade do provedor deixa todos de fora, use o mesmo comando.

<!-- source: re-authentication 8ec1742f8811 -->
## Reautenticação

As operações que pedem a senha ou uma passkey (definir uma senha, adicionar uma passkey, excluir ou mover a conta,
vincular outra conta) pedem a um usuário que não tem nenhuma das duas que entre de novo em um provedor vinculado
([cadastro](registration.md)). Um provedor OpenID Connect é solicitado a autenticar o usuário novamente (`max_age=0` e
`prompt=login`) e o servidor recusa uma autenticação anterior; um servidor Mastodon é solicitado com `force_login=true`.
O GitHub e o Discord não têm como pedir um novo login, então um usuário que ainda está conectado lá é confirmado sem
digitar nada. O servidor ainda verifica que a conta é a vinculada, mas essa é uma prova mais fraca de que a pessoa está
no teclado. Prefira um provedor OpenID Connect, ou peça que os usuários mantenham uma senha ou uma passkey, quando isso
importar.

<!-- source: providers-on-an-internal-network 7f19e187a375 -->
## Provedores em uma rede interna

O servidor acessa os provedores por meio da proteção de HTTP de saída, que recusa endereços de loopback e privados. Para
usar um provedor na sua própria rede, como um Keycloak ao lado do servidor, inclua o host ou a faixa de endereços dele em
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations).
