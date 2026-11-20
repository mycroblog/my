<!-- source: moving-your-account 95d8d22a22bb -->
# Mudando sua conta

Você pode mudar sua conta para outra conta, no mesmo servidor ou em outro. Seus seguidores passam a seguir a conta nova,
e a conta antiga informa que foi movida. O mesmo vale no sentido contrário, quando você muda para este servidor vindo de
outro.

<!-- source: what-moves-and-what-stays a45d57421940 -->
## O que muda e o que fica

| Muda | Fica na conta antiga |
| --- | --- |
| Seus seguidores | Suas publicações, as contas que você segue, seus bloqueios, silenciamentos e itens salvos |

![A conta antiga à esquerda e a conta nova à direita: um alias, a mudança, seus seguidores indo para a conta nova e o que fica na conta antiga. A lista abaixo diz o mesmo.](images/account-move.svg)

De cima para baixo no diagrama:

1. **Adicione um alias.** Na conta nova, adicione a conta antiga em “Aliases”.
2. **Mude a conta.** Na conta antiga, escolha “Mover conta” e informe a conta nova.
3. **O que muda.** Seus seguidores passam a seguir a conta nova. A conta antiga mostra “Esta conta migrou para …”: você ainda pode entrar, ler e exportar seus dados por ela.
4. **O que fica aqui.** Suas publicações, as contas e hashtags que você segue e seus bloqueios, silenciamentos e itens salvos continuam na conta antiga. Suas publicações não se mudam. Leve o restante como arquivos CSV, com “Exportar” na conta antiga e “Importar” na conta nova ([levando seus seguidos com você](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 8470ed8a1f13 -->
## Mudando desta conta

1. Na conta nova, adicione esta conta como alias: “Aliases”, em “Conta e dados” nas configurações, em “Adicionar um alias”. “Mover conta” mostra o identificador desta conta, que você informa lá.
2. Nesta conta, abra “Mover conta” em “Conta e dados” nas configurações, que informa o que muda e o que fica. Em “Conta para a qual você está se mudando”, informe o identificador da conta nova (como `@name@example.com`) ou o endereço do perfil dela e escolha “Mover conta”.
3. Confirme que é você com sua senha, uma passkey ou um provedor vinculado. Se o servidor recusar a mudança, ele informa o motivo.

Depois da mudança, a conta antiga:

- Informa para onde foi movida, nessa página e no perfil. Você ainda pode entrar, ler, exportar seus dados e excluir a conta, mas não pode publicar, impulsionar, reagir, seguir nem votar.
- Não pode ser movida de novo por 30 dias.

“Cancelar a mudança”, na mesma página, traz a conta antiga de volta: ela pode publicar e seguir de novo, e os outros servidores são
avisados. Os seguidores que já se mudaram continuam com a conta nova.

<!-- source: deactivating-or-deleting-this-account 599007a2d409 -->
## Desativando ou excluindo esta conta

Em “Conta e dados” nas configurações, “Desativar conta” oculta a conta e encerra suas sessões em todos os lugares: entre
de novo até a data que a página informa para trazê-la de volta, ou ela será excluída nessa data. “Excluir conta” a exclui
na hora, de forma definitiva. Use “Exportar” para guardar seus dados antes.

<!-- source: moving-to-this-account 4b769e76357c -->
## Mudando para esta conta

Adicione a conta de onde você está saindo como alias em “Aliases”, nas configurações desta conta (“Adicionar um alias”).
Um alias sozinho não muda nada; ele apenas permite que a outra conta mude os seguidores dela para você. Você pode ter 5
aliases, e “Remover” exclui um.

<!-- source: taking-your-follows-with-you 09ca5e285436 -->
## Levando seus seguidos com você

“Exportar” e “Importar”, em “Conta e dados” nas configurações, exportam e importam arquivos CSV no formato que o Mastodon usa.
Você pode exportar “Seguindo”, “Hashtags seguidas”, “Contas bloqueadas”, “Contas silenciadas”, “Servidores silenciados” e
“Itens salvos”, e importá-los na conta nova. O Mastodon não tem um arquivo de hashtags seguidas, então apenas um servidor
que executa o mesmo software que este importa esse arquivo. “Mesclar” mantém o que você tem e acrescenta o que o arquivo lista; “Substituir” deixa a lista
exatamente igual ao que o arquivo lista. “Pré-visualizar” mostra o que um arquivo faria antes de você escolher “Importar”; a importação então roda em segundo plano.

“Solicitar um arquivo”, em “Exportar”, prepara um arquivo ZIP com suas publicações, seu perfil, seus filtros de palavras-chave, suas notas privadas e seus seguidos, hashtags seguidas, bloqueios, silenciamentos, servidores silenciados e itens salvos, para você baixar ali quando estiver pronto.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Quando uma conta que você segue muda

Você recebe uma notificação (“… migrou para outra conta”). Quando você pode seguir a conta nova, passa a segui-la na hora,
ou o seu pedido aguarda aprovação. Você não passa a segui-la quando a bloqueia, quando ela bloqueia você, ou quando você
silenciou o servidor dela; a notificação informa qual é o caso.
