<!-- source: signing-up-and-signing-in e881fcb0feaa -->
# Cadastro e login

Como criar uma conta em um servidor da mycroblog, um servidor que roda `my`, e entrar nela. Cada servidor é administrado
pelos seus próprios operadores, então o que um servidor oferece (quem pode se cadastrar, quais linhas do tempo os
visitantes podem ler, se o chat de voz está ativo) depende deles. A página “Sobre” do servidor e os documentos que ele
linka no cadastro informam as regras dele; este guia descreve apenas como o `my` se comporta.

<!-- source: creating-an-account 6f66cdf29cd2 -->
## Criando uma conta

Escolha “Cadastrar-se” na navegação (ou “Criar uma conta” na página de entrada). O formulário pede:

- **Nome de usuário**: letras (a-z), dígitos e sublinhados. Seu identificador é `@username@this-server`, e você não pode mudar o nome de usuário depois.
- **Endereço de e-mail**: você entra com ele.
- **Senha**: o formulário mostra o tamanho permitido abaixo do campo.
- **Código de convite**, quando o servidor pede um ou aceita um.
- **Por que você quer entrar? (opcional)**, quando o servidor analisa as contas novas.
- Uma caixa para confirmar que você leu os termos e as políticas do servidor, quando o servidor publica algum.

Quais desses campos você vê depende de como o servidor aceita contas novas:

| O servidor | O que você faz |
| --- | --- |
| É aberto | Cadastre-se. Um código de convite é opcional. |
| Analisa as contas novas | Cadastre-se e diga por que quer entrar. Um moderador aprova a conta antes de você poder usá-la. Um código de convite permite usar a conta na hora. |
| Aceita apenas convites | Informe o código de convite que você recebeu, ou abra o link de convite, que o preenche. |
| É fechado | A página diz que o servidor não está aceitando contas novas. |
| Registra apenas por provedores de login | Você se cadastra com um provedor que o servidor oferece (por exemplo “Entrar com GitHub”) em vez de uma senha. |

Se o servidor envia e-mail, ele envia um link para confirmar seu endereço de e-mail. Abra-o e escolha “Confirmar endereço de e-mail”.
Até a conta ser aprovada e o endereço ser confirmado, você pode entrar, mas só vê o que está esperando
(“Sua conta ainda não está pronta”).

Depois do cadastro, uma etapa de boas-vindas pede um nome de exibição e um avatar, ambos opcionais (“Pular por enquanto”); enquanto você não segue ninguém, sua
linha do tempo inicial mostra as pessoas que o servidor sugere, se ele sugerir alguma.

<!-- source: signing-in fcfbfa76f1d0 -->
## Entrando

Escolha “Entrar” na navegação. O cliente web envia você à página de login do servidor e traz você de volta já conectado.
Nessa página você pode:

- Informar seu endereço de e-mail e sua senha.
- Escolher “Entrar com uma chave de acesso”, depois de ter adicionado uma passkey em “Passkeys” nas configurações.
- Escolher um botão em “Ou entre com”, quando o servidor oferece provedores de login e você vinculou uma conta desse provedor em “Contas vinculadas” nas configurações.
- Escolher “Esqueceu a senha?” para receber um link que define uma nova senha. Definir uma nova senha desconecta você em todos os lugares. Um servidor que não envia e-mails não consegue enviar o link: peça aos administradores dele.

Se você ativou a autenticação em dois fatores, a página então pede o código de seis dígitos do seu aplicativo autenticador,
ou um dos seus códigos de recuperação. Depois de muitas tentativas erradas, a página faz você esperar alguns minutos e
informa quanto tempo.

<!-- source: an-account-that-cannot-be-used-yet 8e6bc1791f14 -->
### Uma conta que ainda não pode ser usada

Depois de entrar, você pode ver uma página no lugar do aplicativo:

- Seu cadastro está aguardando a aprovação de um moderador, ou não foi aprovado.
- Você precisa confirmar seu endereço de e-mail. “Enviar o link de novo” envia um novo link.
- Sua conta está restrita por um moderador. A página lista as ações em vigor e permite contestar uma vez (“Contestar esta ação”). As mesmas ações aparecem em “Situação da conta” nas configurações.
- Você desativou a conta. “Reativar sua conta” a traz de volta até a data em que ela seria excluída; “Excluir conta” a exclui na hora.

<!-- source: signing-out 053a33c01d8e -->
## Saindo

Escolha “Sair…”, a última linha das configurações, e confirme com “Sair”. Isso encerra a sessão deste navegador; seus outros
navegadores e apps continuam conectados. “Navegadores conectados”, em “Segurança” nas configurações, lista os navegadores conectados
à sua conta e encerra a sessão deles.

<!-- source: settings 4cd29f4ed978 -->
## Configurações

“Configurações” na navegação abre as configurações. Elas reúnem “Perfil”, que muda a aparência da sua conta e onde
ela é encontrada; “Exibição e publicação”; “Notificações”; “Relacionamentos”, que lista as contas que você bloqueia e silencia, os
servidores que você silencia e as hashtags que você segue, cada uma com o botão que desfaz a ação; “Filtros de palavras-chave”, que lista seus
filtros e os adiciona, altera e exclui; “Segurança”, onde você gerencia sua senha, passkeys, autenticação
em dois fatores, contas vinculadas e navegadores conectados; “Apps”, que lista os apps que você autorizou a usar sua conta,
que você pode revogar, e os apps que você registrou; “Convites”, quando você pode criar convites, que cria links de convite
e os revoga; e “Conta e dados”, que reúne “Status da conta”, seus dados e publicações, e a mudança, a desativação ou a
exclusão da conta ([Mudando sua conta](moving-your-account.md)).
