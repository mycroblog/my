<!-- source: moving-your-account 95d8d22a22bb -->
# Mudando sua conta

Você pode mudar sua conta para outra conta, no mesmo servidor ou em outro. Seus seguidores passam a seguir a conta nova,
e a conta antiga informa que foi movida. O mesmo vale no sentido contrário, quando você muda para este servidor vindo de
outro.

<!-- source: what-moves-and-what-stays 4ea7b64d67bd -->
## O que muda e o que fica

| Muda | Fica na conta antiga |
| --- | --- |
| Seus seguidores | Suas publicações, as contas que você segue, seus bloqueios, silenciamentos e itens salvos |

![A conta antiga à esquerda e a conta nova à direita: um alias, a mudança, seus seguidores indo para a conta nova e o que fica na conta antiga. A lista abaixo diz o mesmo.](images/account-move.svg)

De cima para baixo no diagrama:

1. **Adicione um alias.** Na conta nova, adicione a conta antiga em “Aliases”.
2. **Mude a conta.** Na conta antiga, escolha “Mudar de conta” e informe a conta nova.
3. **O que muda.** Seus seguidores passam a seguir a conta nova. A conta antiga mostra “Sua conta mudou”: você ainda pode entrar, ler e exportar seus dados por ela.
4. **O que fica aqui.** Suas publicações, as contas que você segue e seus bloqueios, silenciamentos e itens salvos continuam na conta antiga. Suas publicações não se mudam. O restante você leva por conta própria: exporte seus seguidos, bloqueios, silenciamentos e itens salvos como arquivos CSV com “Importar e exportar” na conta antiga, e importe-os na conta nova ([levando seus seguidos com você](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 7836944c7996 -->
## Mudando desta conta

1. Na conta nova, adicione esta conta como alias: “Aliases”, em “Conta e dados” nas configurações, com “Adicionar um alias”. A página de mudança mostra o endereço desta conta, que você informa lá.
2. Nesta conta, abra “Mudar de conta” em “Conta e dados” nas configurações. Informe o identificador da conta nova (um identificador como `@name@example.com`, ou o endereço da conta) e escolha “Continuar”.
3. O cliente mostra o que muda e o que fica. Informe sua senha para confirmar (ou, se você não tem senha, confirme com uma passkey ou entrando de novo em um provedor vinculado) e escolha “Mudar conta”.

Depois da mudança, a conta antiga:

- Mostra “Sua conta mudou”. Você ainda pode entrar, ler, exportar seus dados e excluir a conta, mas não pode publicar, impulsionar, reagir, seguir nem votar.
- É exibida no perfil dela como movida para a conta nova.
- Não pode ser movida de novo por 30 dias.

“Cancelar a mudança” traz a conta antiga de volta: ela pode publicar e seguir de novo, e os outros servidores são
avisados. Os seguidores que já se mudaram continuam com a conta nova.

<!-- source: moving-to-this-account adb43ba4eef3 -->
## Mudando para esta conta

Adicione a conta de onde você está saindo como alias em “Aliases”, nas configurações desta conta (“Adicionar um alias”).
Um alias sozinho não muda nada; ele apenas permite que a outra conta mude os seguidores dela para você. Você pode ter
poucos aliases, e “Remover” exclui um.

<!-- source: taking-your-follows-with-you ab2475388452 -->
## Levando seus seguidos com você

“Importar e exportar”, em “Conta e dados” nas configurações, exporta e importa arquivos CSV no formato que o Mastodon usa.
Você pode exportar “Seguindo”, “Contas bloqueadas”, “Contas silenciadas”, “Domínios silenciados” e “Itens salvos”, e
importá-los na conta nova. “Mesclar” mantém o que você tem e acrescenta o que o arquivo lista; “Substituir” deixa a lista
exatamente igual ao que o arquivo lista. Contas e publicações de outros servidores são buscadas em segundo plano, o que
pode levar um tempo.

“Pacote de dados e exclusão” prepara um arquivo ZIP com suas publicações, seu perfil, e seus seguidos, bloqueios,
silenciamentos e itens salvos.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Quando uma conta que você segue muda

Você recebe uma notificação (“… migrou para outra conta”). Quando você pode seguir a conta nova, passa a segui-la na hora,
ou o seu pedido aguarda aprovação. Você não passa a segui-la quando a bloqueia, quando ela bloqueia você, ou quando você
silenciou o servidor dela; a notificação informa qual é o caso.
