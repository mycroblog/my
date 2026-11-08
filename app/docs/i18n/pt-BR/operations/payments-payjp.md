<!-- source: supporter-payments-with-payjp 1f782a11b125 -->
# Pagamentos de apoiadores com PAY.JP

Com `payments = payjp`, os usuários locais assinam planos mensais ou anuais na página de pagamento deste servidor, e a função do plano é concedida e removida conforme muda o estado do pagamento.
As chaves de implantação são descritas na [referência de configuração](../reference/configuration.md#payments). Para o programa como um todo (planos, funções, a lista de membros), veja [supporters.md](supporters.md).
Um administrador ativa o programa e cria os planos na administração. Esta configuração apenas permite que o servidor se comunique com o PAY.JP.

Um servidor pode usar apenas um provedor. Para mudar do Stripe para o PAY.JP (ou o inverso), peça aos assinantes atuais que assinem de novo com o novo provedor.

<!-- source: what-the-server-does-at-payjp a8abbf012e10 -->
## O que o servidor faz no PAY.JP

O PAY.JP tem as APIs v1 e v2. A v2 ainda não tem API de cobrança recorrente, então o servidor usa a cobrança recorrente da **API v1** (Plan e Subscription). A v1 não tem página de pagamento hospedada nem portal do cliente, então este servidor serve as páginas no lugar delas.

[Como funciona uma assinatura](supporters.md#how-a-subscription-works) mostra em um diagrama como se relacionam a página de pagamento, o webhook, a conciliação, a função do plano e a receita.

- **Planos**: Para cada preço, o servidor cria um Plan cujo ID é `my_<plan ID>_<month|year>_<amount>` (`metadata.my_plan_id`). Um Plan do PAY.JP não pode mudar de valor, então mudar o valor cria um novo Plan, e o Plan antigo não é excluído, mas recebe `metadata.my_archived=1`. As assinaturas do Plan antigo são renovadas pelo valor dele.
  Não edite nem exclua esses Plans na administração do PAY.JP. A única moeda é o iene japonês, e os valores vão de 50 a 9.999.999 ienes.
- **Assinatura**: O servidor envia o navegador do usuário para a sua própria página de pagamento (`/payments/pages/payjp/checkout`). A página mostra os campos de cartão do payjp.js (o iframe do PAY.JP) e executa a autenticação 3-D Secure ao criar o token (o tipo iframe).
  O servidor aceita apenas um token cujo 3-D Secure foi concluído (`three_d_secure_status` é `verified` ou `attempted`), adiciona o cartão ao cliente como cartão padrão (na primeira vez, com o ID `my_<the server's subscription ID>`) e cria uma assinatura com o ID de assinatura do servidor em `metadata.my_subscription_id`. A primeira cobrança acontece na criação da assinatura.
  A URL da página tem curta duração e carrega um estado assinado pelo servidor (válido de 1 a 2 horas a partir da criação).
  Um usuário que sai da página e começa de novo, no mesmo plano ou em outro, recebe agora uma nova URL para o plano que pediu. Uma URL anterior continua funcionando até expirar, e a página que for paga primeiro gera a única assinatura: as outras passam a apontar para ela. Se uma página anterior já tiver sido paga, o usuário vai para a tela de retorno e não recebe uma nova URL.
  Uma URL assinada não pode ser retirada, então uma página continua pagável depois que a assinatura pendente dela é encerrada (o plano aposentado, a conta excluída). Um pagamento nela não compra nada: o servidor exclui a assinatura e reembolsa integralmente as suas cobranças (o PAY.JP aceita reembolso por 180 dias após a venda).
  A API v1 não aceita chave de idempotência, então, enquanto um envio da página cria a assinatura, o servidor marca o checkout como em andamento no banco de dados: um segundo envio (um clique duplo, outra aba ou outro processo do servidor atrás do mesmo load balancer) aguarda até 30 segundos e depois segue para a assinatura que o primeiro criou. A marca expira após 5 minutos se o processo que a detinha tiver parado.
- **Página do cartão e do histórico de pagamentos** (no lugar do portal do cliente do Stripe): Na tela de assinatura, o assinante abre `/payments/pages/payjp/portal` para trocar o cartão e conferir os pagamentos recentes (a URL expira em uma hora). O formulário começa com o endereço de e-mail do cliente no PAY.JP, que é o endereço informado no primeiro pagamento, e não um endereço alterado depois na conta.
  Trocar o cartão retoma uma assinatura interrompida por um pagamento recusado e cobra na hora. Cancelar e trocar de plano são feitos na tela de assinatura deste servidor.
- **Falha de pagamento**: O PAY.JP pausa (`paused`) uma assinatura cuja cobrança de renovação falhou, e **não tenta de novo**. O servidor trata a pausa como um pagamento atrasado e mantém a função por um período de carência de 7 dias a partir do momento da pausa. Se o assinante trocar o cartão nesse período, a assinatura é retomada; se não, a função é removida após 7 dias (do lado do PAY.JP, a assinatura continua pausada e não é cobrada).
- **Mudanças de plano**: Um upgrade cobra a diferença do restante do período atual, proporcional, como cobrança única (sem cobrança abaixo de 50 ienes), troca o plano e cobra o valor integral do novo plano no fim do período (o início do período não muda). Se a cobrança da diferença for recusada, o plano não muda.
  Um downgrade, e a troca de anual para mensal, passam a valer no fim do período com `next_cycle_plan`.
- **Ler o estado de volta**: O servidor trata um webhook como um sinal e lê de volta a assinatura e as movimentações financeiras pela API do PAY.JP para decidir o estado.
  Além disso, uma conciliação a cada `payments_reconcile_interval` lê a lista de eventos, as movimentações financeiras e uma varredura de todas as assinaturas, e corrige qualquer webhook que o servidor tenha perdido.
- **Receita**: Para cada pagamento, o servidor registra o valor bruto, a taxa (calculada a partir do `fee_rate` da cobrança, arredondando para baixo abaixo de um iene) e o valor líquido. Um reembolso e um estorno registram a taxa devolvida como uma taxa negativa.
  Quando um período é encerrado e um extrato é emitido, a diferença entre a taxa do extrato e o cálculo do servidor é registrada como uma movimentação de taxa (o ID dela é o ID do extrato), de modo que o total de taxas do período bata com o extrato.
  Um extrato cobre as taxas de toda a conta, então **use uma conta do PAY.JP dedicada a este servidor**.
- **Datas**: As datas de receita são contadas no horário do Japão.

O servidor envia no máximo 20 requisições por segundo (2 no modo de teste). Isso fica dentro do limite de taxa do PAY.JP (para requisições com chave secreta, 30 por segundo no modo ao vivo e 2 no modo de teste). Diante de um `429`, o servidor espera 1 segundo, dobrando a espera a cada tentativa, e tenta até 3 vezes.
No modo de teste, uma única conciliação é lenta (cerca de 8 minutos no limite de 1.000 requisições). Um job de continuação assume a tarefa, então nenhuma ação é necessária.

<!-- source: card-data-and-pci-dss e7247e5fea0f -->
## Dados de cartão e PCI DSS

Os números de cartão, as datas de validade e os códigos de segurança são digitados somente dentro do iframe do PAY.JP, que o payjp.js exibe. A página e o script deste servidor nunca os tocam. O servidor recebe apenas o ID do token que o payjp.js criou, além do nome do titular e do e-mail que o 3-D Secure exige.
O servidor guarda somente IDs de objetos do PAY.JP (`cus_`, `sub_`, `ch_`, IDs de Plan, IDs de eventos), valores, períodos e estados. Ele não guarda números de cartão, os quatro últimos dígitos nem datas de validade (a página do cartão apenas lê do PAY.JP os quatro últimos dígitos, a data de validade e o e-mail do cliente, e os exibe). Os corpos dos webhooks e as respostas do PAY.JP não ficam guardados nem nos logs nem no banco de dados.

A página de pagamento é servida pela origem deste servidor, então ela foi construída levando em conta a gestão de scripts de páginas de pagamento do PCI DSS v4 (requisito 6.4.3) e a detecção de adulteração (requisito 11.6.1).

- A página carrega apenas `https://js.pay.jp/v2/pay.js` e um pequeno script com um nonce incorporado à página. Ela não carrega o pacote do cliente web nem qualquer script de análise ou publicidade.
- A página sempre traz a sua própria Content-Security-Policy (`script-src` apenas o nonce e `https://js.pay.jp`, `frame-src` apenas `https://js.pay.jp` e `https://api.pay.jp`, `form-action 'self'`, `frame-ancestors 'none'`), `Referrer-Policy: no-referrer`, `Cache-Control: no-store` e `X-Content-Type-Options: nosniff`. Nenhuma configuração altera a CSP.
- O modelo e o script são incorporados ao binário do servidor e não podem ser alterados em tempo de execução. Os envios de formulário só são aceitos a partir da mesma origem.

Por isso, o escopo do operador deve ser o de um comerciante que deixa a digitação do pagamento para o iframe do PAY.JP (SAQ A). Verifique o escopo aplicável e o tipo de questionário de autoavaliação junto às orientações do PAY.JP e às exigências das bandeiras de cartão.
Ao fazer isso, ajuda manter um registro destes pontos: as URLs das páginas de pagamento (`/payments/pages/payjp/checkout` e `/portal`), a lista de scripts que a página carrega (os dois citados acima), a CSP da página e o fato de que o tráfego para o servidor usa TLS (a configuração do proxy reverso).

<!-- source: 1-prepare-the-payjp-account b01b53a61e40 -->
## 1. Preparar a conta do PAY.JP

1. Crie uma conta no PAY.JP e comece no modo de teste. Cobrar no modo ao vivo exige um aplicativo de produção e uma análise.
2. Verifique “API settings > 3-D Secure” no painel do PAY.JP. O servidor pede o 3-D Secure a cada criação de token, então você também pode optar por exigir o 3-D Secure para toda criação de token.
3. A página de divulgação comercial é definida na administração deste servidor (veja [supporters.md](supporters.md)).

<!-- source: 2-keys-and-the-webhook-token 7bee0ef9256a -->
## 2. Chaves e o token do webhook

Em “API” no painel do PAY.JP, confira estas duas chaves. O prefixo de uma chave define o modo, e as chaves de teste não cobram nada. As duas chaves precisam estar no mesmo modo.

| Chave | Exemplo | Uso |
| --- | --- | --- |
| Chave secreta | `sk_test_...` / `sk_live_...` | O servidor chama a API com ela. Ela também é a origem da chave que assina as URLs das páginas |
| Chave pública | `pk_test_...` / `pk_live_...` | payjp.js nas páginas de pagamento e de cartão |

Passe a chave secreta na variável de ambiente `MY_PAYMENTS_PAYJP_SECRET_KEY` (ou o caminho de um arquivo em `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE`). Ela não pode ser escrita no arquivo de configuração.
Se a chave secreta vazar, gere uma nova no painel e reimplante com a nova chave. As URLs das páginas de pagamento e de cartão que estavam abertas antes da reimplantação deixam de funcionar.

<!-- source: 3-register-the-webhook 1afaf50fdaef -->
## 3. Registrar o webhook

Em “Settings > Webhook” no painel do PAY.JP, adicione um destino. Registre o modo de teste e o modo ao vivo separadamente.

- URL: `https://<server domain>/payments/webhooks/payjp`
- Modo: o mesmo das chaves

Um webhook do PAY.JP identifica o remetente pelo **token de origem** da conta (`whook_...`, no cabeçalho `X-Payjp-Webhook-Token`). Passe o token mostrado no painel na variável de ambiente `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` (ou `_FILE`).
O endpoint compara o token em tempo constante e rejeita com `400` um evento de teste recebido com uma chave ao vivo (e o inverso). Não há assinatura nem timestamp, então quem conhece o token pode enviar uma notificação, mas o servidor nunca grava estado a partir do conteúdo de uma notificação; ele apenas lê o estado de volta do PAY.JP.

O servidor usa estes eventos (outros tipos são aceitos, mas não são registrados).

- `subscription.created`, `subscription.updated`, `subscription.deleted`, `subscription.paused`, `subscription.resumed`, `subscription.canceled`, `subscription.renewed`
- `charge.succeeded`, `charge.failed`, `charge.refunded`
- `dispute.created`
- `statement.created`

O PAY.JP reenvia uma notificação apenas a cada 3 minutos, no máximo 3 vezes, e uma notificação além disso não chega até você reenviá-la pelo painel. A conciliação corrige o que foi perdido, então **recomenda-se definir `payments_reconcile_interval` como 1 hora**.
O estado do provedor (o modo, o momento em que chegou o último webhook e o resultado da última conciliação) aparece na seção “Provedor de pagamento” de `/admin/supporters` na administração.

<!-- source: 4-configure-the-server b0d24c7a2da0 -->
## 4. Configurar o servidor

```sh
MY_PAYMENTS=payjp
MY_PAYMENTS_PAYJP_SECRET_KEY=sk_test_...
MY_PAYMENTS_PAYJP_PUBLIC_KEY=pk_test_...
MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN=whook_...
# The recommended reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=1h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` verifica a configuração. Um binário compilado com a build tag `no_payments_payjp` recusa `payments = payjp` (veja [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode f6ebbada0e0e -->
## 5. Experimentar no modo de teste

Comece com as chaves de teste. O modo de teste e o modo ao vivo têm Plans, clientes, assinaturas, webhooks e tokens separados. Para entrar no ar, registre o webhook no modo ao vivo, troque as chaves e o token pelos do modo ao vivo e reimplante. Os Plans são criados de novo na conta ao vivo.

Cartões de teste ([documentação do PAY.JP](https://pay.jp/docs/testcard)):

| Número | Comportamento |
| --- | --- |
| `4242 4242 4242 4242` | Tem sucesso |
| `4000 0000 0008 0319` | O cartão pode ser cadastrado, mas a cobrança é recusada (uma assinatura que falha e uma retomada que falha após a troca de cartão) |
| `4000 0000 0000 4012` | O cartão pode ser cadastrado, mas a cobrança falha por estar vencido |
| `4000 0000 0000 0002` | O cadastro do cartão (a criação do token) é recusado |

Use uma data de validade futura; o código de segurança e o nome são arbitrários. No modo de teste, a tela de autenticação de teste do PAY.JP para o 3-D Secure permite escolher o resultado.

Para testar uma renovação rapidamente, defina o `trial_end` da assinatura alguns minutos à frente, pela API. A cobrança de renovação roda alguns minutos depois desse horário, não nele. Outra atualização de `trial_end` enquanto a primeira ainda está pendente reinicia a espera.
Para testar uma falha de pagamento e o período de carência, assine, troque o cartão na página do cartão pelo que é recusado (`4000 0000 0008 0319`) e deixe a assinatura renovar.

<!-- source: 6-try-it-locally 219997c1a489 -->
## 6. Experimentar localmente

Os webhooks do PAY.JP vão para uma URL pública; então, para recebê-los em um servidor na sua máquina, exponha a porta por um túnel (cloudflared, ngrok e similares) e registre essa URL como um destino do modo de teste.
Espera-se que `payjp listen --forward-to http://localhost:8080/payments/webhooks/payjp` do payjp-cli também encaminhe os eventos, mas não se verificou se ele consegue encaminhar eventos da v1.

Abra as páginas de pagamento e de cartão na origem deste servidor (elas também funcionam em `http://localhost:8080`, mas o comportamento do payjp.js e do 3-D Secure varia conforme as configurações do navegador).

<!-- source: 7-operation fbdd81461f80 -->
## 7. Operação

- Uma falha de conciliação do tipo `permission` indica uma chave secreta errada.
- `duplicate_charge` nos avisos da seção “Provedor de pagamento” significa que o servidor não conseguiu concluir o reembolso de um pagamento de uma assinatura encerrada. A linha de log `a second subscription could not be canceled and refunded` informa a assinatura, e cada conciliação tenta de novo, inclusive depois que a assinatura é excluída. Reembolse a cobrança no painel se isso continuar: a próxima tentativa encontra o reembolso e limpa o aviso, desde que o PAY.JP ainda liste as cobranças da assinatura excluída (veja [Itens a verificar](#items-to-verify)). Uma cobrança com mais de 180 dias não pode ser reembolsada: o servidor registra isso e segue em frente.
- Os outros reembolsos são tratados no painel do PAY.JP. Um reembolso total e um estorno removem a função e excluem a assinatura de imediato, e um reembolso parcial não remove a função.
- **O cancelamento de um estorno** não é registrado automaticamente, porque o PAY.JP não emite evento nem API para ele. Quando isso acontecer, registre o valor cancelado na tela de receita da página de custos (`finance`). A função não é restaurada automaticamente.
- **As taxas de plano pago e as taxas de transferência do PAY.JP** não são subtraídas da receita. Registre-as como custos em `finance`.
- Quando as conciliações atingem o limite de requisições com frequência, aumente `payments_reconcile_max_requests` ou alongue `payments_reconcile_interval`.

<!-- source: plan-changes 29dec3be3d95 -->
## Mudanças de plano

Um upgrade cobra primeiro a diferença proporcional e só muda o plano depois que essa cobrança tem sucesso. Quando a cobrança é recusada, o plano permanece, e o usuário vê um erro com o caminho para atualizar o cartão, sem período de carência e sem notificação de pagamento falho; tentar de novo com um cartão válido cobra uma única vez. Se o plano não mudar depois da cobrança, tentar de novo não cobra outra vez. Uma diferença abaixo do mínimo de 50 ienes, e uma troca para o preço anual do mesmo plano, que não tem diferença positiva, não cobram nada agora e mudam de imediato; o novo valor é cobrado no fim do período, e uma falha nesse momento é uma renovação que falhou.

<!-- source: items-to-verify 3c2cbf6ed2b0 -->
## Itens a verificar

O comportamento a seguir não está definido pela documentação do PAY.JP nem pelo modo de teste.

- Como é calculada a taxa devolvida por um estorno, e se o extrato de um período bate com as taxas do servidor (o modo de teste mostrou uma taxa arredondada para baixo, e a taxa de um reembolso calculada a partir do seu valor).
- O que acontece num estorno real: o PAY.JP não documenta forma de provocar um estorno no modo de teste nem o corpo de `dispute.created`. Um envio de teste do evento pelo painel recebe `200` e não altera nada.
- Até onde a lista de eventos alcança no passado.
- Se excluir um Plan afeta as assinaturas existentes (o servidor nunca exclui um), e o tamanho máximo de um ID de Plan.
- Se o relatório de CSP ao carregar a página de pagamento importa: o `pay.js` aplica um estilo inline contra `style-src 'self'`, enquanto os campos de cartão e o 3-D Secure funcionam no Chromium. Outros navegadores não foram verificados.
- Se o payjp-cli consegue encaminhar eventos da v1.
- Que a cobrança de um cartão salvo do cliente não pede 3-D Secure, a menos que a requisição o peça. Uma cobrança recusada aparece em `GET /v1/charges` com `paid: false`, e o servidor ignora a que não foi paga.
- Que `GET /v1/charges` com `subscription` ainda lista as cobranças de uma assinatura excluída. O servidor reembolsa antes de excluir, então só uma nova tentativa após um reembolso que falhou de vez depende disso. A referência da API documenta o filtro e não diz nada sobre assinaturas excluídas. Se a lista voltar vazia, o servidor registra uma falha em vez de um reembolso, e `duplicate_charge` permanece mesmo depois que a cobrança é reembolsada manualmente.
