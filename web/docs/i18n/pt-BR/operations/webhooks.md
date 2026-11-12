<!-- source: server-webhooks e73d0e0966b7 -->
# Webhooks do servidor

Os webhooks enviam denúncias, pedidos de cadastro, criações de contas e mudanças de perfil, ações de moderação, recursos,
ações de domínio, publicações públicas locais e ações de gerenciamento de emojis personalizados para um relay do Slack ou
do Discord, um sistema de chamados ou uma ferramenta de automação como o n8n. Um servidor sem webhooks configurados não
guarda linhas extras nem roda jobs extras. O que um receptor recebe (cabeçalhos, assinatura, eventos e corpo) está descrito
na [referência de webhooks](../reference/webhooks.md).

<!-- source: set-up-a-webhook c8a80bc40038 -->
## Configurando um webhook

Qualquer pessoa com a permissão `manage_webhooks` pode configurar webhooks na tela de administração “Webhooks”
(`/admin/webhooks`). A função `admin` embutida a tem. “Adicionar um webhook” abre o formulário; escolher um webhook na lista
abre a página dele, onde você altera, testa, ativa, desativa e exclui o webhook, gera um novo segredo e vê as entregas dele.

1. Crie um webhook com uma URL de destino, uma descrição e os eventos a assinar. Cada evento também exige a própria permissão, além de `manage_webhooks`: `manage_reports` para denúncias, `manage_accounts` para cadastros, contas, ações de moderação e recursos, `manage_domains` para ações de domínio, `manage_custom_emojis` para emojis personalizados e `manage_errors` para grupos de erro.
   Publicações públicas não exigem permissão extra. Você não pode selecionar um evento cuja permissão você não tem.
2. O segredo (`whsec_...`) é mostrado uma única vez, quando o webhook é criado. Configure-o no receptor. Ele não pode ser mostrado de novo.
3. Na página do webhook, escolha “Enviar um teste” para enviar `webhook.test`, e confira nas entregas dele que chegou.

Criar, alterar, excluir, ativar, desativar e rotacionar o segredo de um webhook ficam registrados no registro de auditoria.
O alvo aparece como a descrição e o nome do host da URL; a URL em si não é registrada.

<!-- source: rotate-the-secret 928c62f87538 -->
## Rotacionando o segredo

“Novo segredo” cria um novo segredo e o mostra uma única vez. Pelas próximas 24 horas, as mensagens também são assinadas com o
segredo antigo (`webhook-signature` traz duas assinaturas), para que você possa trocar o receptor para o novo segredo
nessa janela. Os segredos são cifrados com `kernel.Sealer` antes de serem guardados no banco de dados; a chave é a
[KEK](key-management.md).

<!-- source: failures-and-redelivery 65aba0d2089e -->
## Falhas e reenvio

- Se o destino responder com qualquer coisa diferente de `2xx`, não responder em até 15 segundos, ou não puder ser alcançado, a mensagem é tentada de novo após 5 segundos, 5 minutos, 30 minutos, 2 horas, 5 horas, 10 horas e mais 10 horas (8 tentativas no total, cerca de 27 horas). Uma entrega que usou todas se torna um job morto, e “Enviar de novo” nas entregas a envia outra vez com o mesmo `webhook-id`.
- Se o destino responder `410 Gone`, o webhook é desativado imediatamente.
- O servidor desativa um destino que não teve sucesso por 72 horas desde a primeira falha e que falhou 20 vezes ou mais seguidas. O registro de auditoria ganha uma entrada `webhook_disabled` por `system`, e todas as pessoas com `manage_webhooks` recebem uma notificação no aplicativo (e um e-mail, se escolheram receber notificações por e-mail).
- A lista na tela de administração mostra o número de falhas seguidas e o motivo pelo qual o servidor desativou um webhook (`gone` ou `failing`). Ative o webhook de novo depois de corrigir o destino. As mensagens de quando ele estava desativado não são enviadas de novo.
- As mensagens e as entregas são excluídas após 14 dias (fixo). Os corpos contêm dados pessoais, como o comentário de uma denúncia.

<!-- source: destinations-on-an-internal-network 3f8b39001831 -->
## Destinos em uma rede interna

A entrega passa pela proteção SSRF do HTTP de saída. Por padrão, ela só alcança endereços públicos por `https` (portas 443
e 80). Para enviar a um destino interno, como um n8n no mesmo host ou um chat dentro da sua VPC, liste nomes de host,
endereços IP e CIDRs, separados por vírgulas, na configuração `webhook_allowed_destinations` (variável de ambiente
`MY_WEBHOOK_ALLOWED_DESTINATIONS`; veja a [referência de configuração](../reference/configuration.md#webhook_allowed_destinations)).

```sh
MY_WEBHOOK_ALLOWED_DESTINATIONS=n8n.internal,10.0.0.0/8
```

- Somente os destinos listados, e somente para a entrega de webhooks, ficam isentos da proteção. Federação, mídia e pré-visualizações de links não mudam. Um destino isento também pode usar `http` e qualquer porta.
- Um nome de host é permitido qualquer que seja o endereço em que ele resolve. Um CIDR é permitido quando a URL informa um endereço IP nessa faixa. Se você usar um nome de host que resolve para dentro da faixa, liste também o nome de host.
- Somente um operador com acesso ao servidor pode alterar a configuração. As permissões da tela de administração, sozinhas, não alcançam uma rede interna.
- Uma tentativa a um destino que a proteção recusou mostra `forbidden_destination` nas entregas.
- A configuração de desenvolvimento `http_allow_private_networks` remove a proteção para todas as finalidades, inclusive os webhooks.
