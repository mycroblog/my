<!-- source: signing-up-and-signing-in e881fcb0feaa -->
# Cadastro e login

Como criar uma conta em um servidor da mycroblog, um servidor que roda `my`, e entrar nela. Cada servidor é administrado
pelos seus próprios operadores, então o que um servidor oferece (quem pode se cadastrar, quais linhas do tempo os
visitantes podem ler, se o chat de voz está ativo) depende deles. A página “Sobre” do servidor e os documentos que ele
linka no cadastro informam as regras dele; este guia descreve apenas como o `my` se comporta.

<!-- source: creating-an-account 965073ac4cf5 -->
## Criando uma conta

Escolha “Cadastrar-se” na navegação (ou “Criar conta” na página de login). O formulário pede:

- **Nome de usuário**: letras (a-z), dígitos e sublinhados. Seu identificador é `@username@this-server`, e você não pode alterar o nome de usuário depois.
- **Endereço de e-mail**: você entra com ele.
- **Senha**: o formulário mostra o tamanho permitido abaixo do campo.
- **Código de convite**, quando o servidor pede um ou aceita um.
- **Por que você quer entrar? (opcional)**, quando o servidor analisa novas contas.
- Uma caixa para confirmar que você leu os termos e as políticas do servidor, quando ele publica algum.

Quais desses campos você vê depende de como o servidor aceita novas contas:

| O servidor | O que você faz |
| --- | --- |
| Está aberto | Cadastre-se. O código de convite é opcional. |
| Analisa novas contas | Cadastre-se e diga por que quer entrar. Um moderador aprova a conta antes que você possa usá-la. Um código de convite permite usar a conta na hora. |
| Aceita somente convites | Informe o código de convite que você recebeu, ou abra o link do convite, que já o preenche. |
| Está fechado | A página informa que o servidor não está aceitando novas contas. |
| Faz cadastro somente por provedores de login | Você se cadastra com um provedor que o servidor oferece (por exemplo, “Entrar com GitHub”) em vez de usar senha. |

Se o servidor envia e-mails, ele envia um link para confirmar seu endereço de e-mail. Abra o link e escolha “Confirmar e-mail”.
Enquanto a conta não for aprovada e o endereço não for confirmado, você pode entrar, mas só vê o que está esperando
(“Sua conta ainda não está pronta”).

Depois do cadastro, uma página de boas-vindas permite escolher um avatar e um nome de exibição (“Continuar” salva os dois;
“Depois” pula essa etapa). Sua linha do tempo inicial então mostra uma lista “Primeiros passos” com “Configure seu perfil”,
“Siga alguém” e “Escreva sua primeira publicação”. “Ocultar estas etapas” fecha a lista naquele navegador. “Contas
sugeridas” lista pessoas que você pode seguir, exceto em um servidor de uma única pessoa, onde “Encontrar pessoas” abre
a busca.

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

<!-- source: signing-out fb81956ae999 -->
## Saindo

Escolha “Sair” na seção “Conta” do aplicativo. A página “Navegadores conectados” das configurações lista os navegadores
conectados à sua conta e os desconecta.

<!-- source: settings 35cf43b05962 -->
## Configurações

As configurações estão agrupadas em “Perfil e comunidade”, “Exibição e notificações”, “Relacionamentos”, “Segurança”,
“Apps” e “Conta e dados”. Em “Segurança” você gerencia sua senha, a autenticação em dois fatores, as passkeys, as contas
vinculadas e os navegadores conectados.
