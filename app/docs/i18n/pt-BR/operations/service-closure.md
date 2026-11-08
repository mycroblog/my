<!-- source: closing-the-service 30c438995ba1 -->
# Encerrando o serviço

O que fazer, em ordem, ao parar de executar o servidor: deixar os usuários levarem os seus dados, excluir as contas que restam em outros servidores, e então parar o servidor e trocar para uma página de encerramento estática.
Cada comando é descrito com mais detalhes em [administradores e funções](admin-cli.md#announcing-the-end-of-the-service).

<!-- source: the-sequence 4557c35b6851 -->
## A sequência

![Etapas do encerramento do serviço de cima para baixo: o que o operador faz à esquerda, e o estado do servidor à direita, com a data de encerramento e o prazo de exportação entre elas. A tabela abaixo descreve cada etapa.](images/service-closure.svg)

A tabela segue o diagrama de cima para baixo; a coluna Servidor começa com o estado que `my admin closure status` mostra.
A data de encerramento (`-closes-at`) encerra o período de aviso, e o prazo de exportação (`-export-until`, 30 dias após a data de encerramento se não for definido) encerra a etapa 5.

| Etapa | Servidor | O que o operador faz |
| --- | --- | --- |
| 1. Anunciar | `announced`. Funciona normalmente. Cadastros, convites e novas assinaturas de apoiadores são interrompidos | `my admin closure announce`, publique um aviso na página de status |
| 2. Período de aviso | `announced`. Funciona normalmente. Os usuários podem exportar seus dados e migrar para outro servidor | Reembolse apoiadores anuais, verifique migrações e bloqueios de retenção |
| 3. Encerrar (após a data de encerramento) | `closed`. Os usuários só podem entrar e exportar. O servidor entrega um `Delete` para cada conta | `my admin closure check` e `close` |
| 4. Entrega concluída | Registra `delivery_completed`; os Actors locais também retornam `410` | Confirme com `my admin closure status`. Não pare os workers |
| 5. Até o prazo de exportação | `delivery_completed`. Continua encerrado e segue oferecendo a exportação | Exporte os registros sob bloqueios de retenção |
| 6. Página estática | Parado. O front end serve a página estática, e `410` para a federação | `my admin closure export-html`, troque o front end, pare o `my serve` |
| 7. Descarte | Nenhum. Continue servindo a página estática e `410` por cerca de um ano | Descarte o banco, os blobs e os backups. Mantenha o domínio por cerca de um ano |

Você pode combinar qualquer etapa com o modo de manutenção ([Manutenção](maintenance.md)). Durante a manutenção, as exportações e a entrega das atividades `Delete` param.

<!-- source: 1-announce b34cb9474667 -->
## 1. Anunciar

Defina a data de encerramento (`-closes-at`) e o prazo para retirar os dados (`-export-until`, 30 dias após a data de encerramento se omitido), e então anuncie-os.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for being here."
my admin closure status
```

- Não há limite inferior para a data de encerramento, mas um aviso aparece quando o tempo entre o anúncio e a data de encerramento é menor que 30 dias. Dê aos usuários tempo para exportar e migrar.
- Para mudar a data, execute `announce` de novo; para cancelar, execute `my admin closure withdraw`. Qualquer um dos dois envia a todos os usuários uma notificação `service_closure`.
- Depois do anúncio, o modo de registro efetivo passa a ser `closed`, as solicitações de cadastro pendentes são rejeitadas, e toda assinatura de apoio em vigor é cancelada ao final do seu período (os assinantes recebem uma notificação e um e-mail).
  Retirar o anúncio não desfaz as rejeições nem os cancelamentos.
- As notificações são criadas por jobs, então não pare os workers.
- Na página de status ([monitoramento e página de status](monitoring.md)), publique a data de encerramento, o prazo de exportação e como exportar e migrar. A página de status sobrevive ao servidor, então ela também orienta quem visita depois do encerramento.

<!-- source: 2-notice-period b18535a9c0f7 -->
## 2. Período de aviso

- Os usuários podem exportar seus dados (a exportação de dados e os arquivos CSV) e migrar para outro servidor (`Move`) a partir da tela de configurações. Um banner em cada tela e uma notificação mostram a data de encerramento, o prazo de exportação e o caminho até essa tela.
- **Apoiadores anuais**: o anúncio cancela as assinaturas ao final do período, mas, para uma assinatura cujo período termina depois da data de encerramento, o tempo que não poderá ser usado já foi pago. O servidor não faz reembolso, então reembolse cada assinante na tela do provedor de pagamento (no Stripe, a tela de pagamentos do painel; no PAY.JP, a tela de pagamentos do console de administração) ([pagamentos de apoio (Stripe)](payments-stripe.md), [pagamentos de apoio (PAY.JP)](payments-payjp.md)).
- Depois que a data de encerramento passa, as pessoas com `manage_service_closure` recebem uma notificação `admin_service_closure_due`. O servidor não encerra sozinho.

<!-- source: 3-close c851780cf430 -->
## 3. Encerrar

Depois que a data de encerramento passar, execute as verificações antes de encerrar.

```sh
my admin closure check
```

- **Migrações não concluídas** geram um aviso. Espere que terminem antes de encerrar, porque depois do encerramento ninguém pode iniciar um `Move`.
- **Bloqueios de retenção (`retention_holds`)** em qualquer conta geram um aviso.
  O dever por trás de um bloqueio não termina quando você encerra o servidor, então exporte os registros sob um bloqueio e guarde-os fora do servidor (liste os bloqueios com `my admin retention list`).

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

Quando as verificações estiverem prontas, encerre. Isso não pode ser desfeito. Confirme digitando `y` e o nome do domínio (use `-confirm-domain` em automações).

```sh
my admin closure close
```

Depois do encerramento, os usuários só podem entrar e exportar seus dados. As assinaturas de apoio restantes são canceladas imediatamente, e um `Delete` (Actor) para cada conta local é entregue a outros servidores (mesmo quando `federation_mode` é `off` ou `inbound`).
A inbox e os GETs de publicações e coleções retornam `410 Gone`.

<!-- source: 4-confirming-that-delivery-is-complete b545acd9a55c -->
## 4. Confirmando que a entrega está concluída

**Não pare o `my serve` e os workers (processos cujo `role` é `all` ou `worker`; com uma fila do tipo push, a fila do Cloud Tasks) até que o estado seja `delivery_completed`.**
Os workers entregam as atividades `Delete`, e enquanto isso o servidor continua devolvendo o documento e a chave do Actor local para que os receptores possam verificar a assinatura do `Delete`. Se você parar cedo, as contas permanecem em outros servidores.

```sh
my admin closure status
```

- `accounts with a Delete` é o número de contas para as quais um `Delete` foi registrado, `fan-outs pending` é o trabalho de fan-out que resta, e `deliveries <state>` é o número de entregas por estado.
- As entregas são repetidas pela política usual (até 9 vezes em cerca de 4 dias). Quando todas terminarem, a verificação que roda a cada 10 minutos registra `delivery_completed` (`state: delivery_completed`).
  A partir daí, os Actors locais e o Actor da instância também retornam `410`.
- Uma entrega que virou `dead` não é repetida automaticamente. Para repeti-la, use `my job retry`.

<!-- source: 5-until-the-export-deadline 6464f67f67d1 -->
## 5. Até o prazo de exportação

Mesmo depois que a entrega é concluída, o servidor continua executando encerrado até `export_until` e segue atendendo as exportações dos usuários.
O servidor nunca apaga dados por conta própria, então continua rodando depois do prazo até você pará-lo.
Se você ainda não exportou os registros sob bloqueios de retenção (§3), faça isso agora.

<!-- source: 6-switching-to-the-static-page 9795c9aea7ae -->
## 6. Trocando para a página estática

Após o prazo de exportação, exporte a página de encerramento. **Isso lê o banco de dados, então execute antes de parar o `my serve` e descartar o banco.**

```sh
my admin closure export-html -out ./closure             # all locales; index.html starts from the instance's default locale
my admin closure export-html -out ./closure -lang ja,en # set the locales and the order in index.html
```

- O que ele grava:
  - `<dir>/<locale>.html`, e `<dir>/index.html`, que lista todos os locales na ordem. Eles usam o mesmo template e o mesmo CSS da página de manutenção (`my maintenance export`) e mostram o nome do servidor (`maintenance_server_name`, senão o nome da instância, senão o nome do host), a data de encerramento, a mensagem do operador do anúncio e um link para a página de status (`status_page_url`).
    O CSS é apenas inline; não há scripts nem recursos externos, e a CSP fica em uma tag `<meta>`. Os ajustes de cor do tema são incorporados; o CSS personalizado não é incluído.
  - Exemplos de configuração do front end `nginx.conf`, `Caddyfile` e `cloudfront-function.js`. Cada um retorna `410` sem corpo para os caminhos de federação e descoberta (`/ap/*`, `/inbox`, `/.well-known/webfinger`, `/.well-known/host-meta`, `/.well-known/host-meta.json`, `/.well-known/nodeinfo`, `/nodeinfo/*`) e para perfis (`/@*`), e, para qualquer outro caminho, retorna a página do primeiro idioma em `Accept-Language` (ou `index.html`, se não houver nenhum).
    Um idioma recebe a página do locale em que o app o mostraria: `zh`, `zh-SG` e `zh-Hans` recebem `zh-CN.html`, e `zh-Hant`, `zh-HK` e `zh-MO` recebem `zh-TW.html`.
- O comando se recusa se o servidor não tiver sido encerrado (código de saída 1). Antes de `delivery_completed`, ele avisa que as entregas continuam (e mesmo assim grava os arquivos).

Troque o front end. Coloque apenas as páginas (`*.html`) no local público, e ajuste o nome do host e os caminhos de diretório e de certificados no exemplo ao seu ambiente.

| Front end | Como configurar |
| --- | --- |
| nginx | Coloque as páginas em `/srv/closure` e inclua `nginx.conf` no bloco `http`. As páginas também são servidas com `410` |
| Caddy | Coloque as páginas em `/srv/closure` e use o `Caddyfile`. O Caddy obtém o certificado, e as páginas também são servidas com `410` |
| CloudFront | Coloque as páginas diretamente em um bucket do S3, faça desse bucket a origem do comportamento de cache padrão, e associe `cloudfront-function.js` como uma CloudFront Function de viewer request (`cloudfront-js-2.0`). Coloque `Accept-Language` na chave de cache da política de cache. As páginas são servidas com `200` (uma função de viewer request não consegue mudar o status da resposta da origem) |

Depois de trocar, verifique se `curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` retorna `410` e se `curl -i https://social.example/` retorna a página de encerramento. Depois, pare o `my serve` e os workers.
Para o monitoramento de disponibilidade, ou pare os monitores de verificação de saúde, ou aponte-os para a página de encerramento.

<!-- source: 7-discarding-the-db-blobs-and-backups c7c55aac56e0 -->
## 7. Descartando o banco, os blobs e os backups

Depois de parar, descarte o banco, os blobs (mídia e arquivos de exportação), o KV store e as filas, junto com os respectivos backups e snapshots. Com um [mecanismo de busca](search.md#closing-the-service), descarte também o índice e os snapshots dele: eles guardam o texto das publicações.
Antes de descartar qualquer coisa, confirme mais uma vez que você exportou e guardou os registros sob bloqueios de retenção. Descartar não pode ser desfeito.

**Mantenha o domínio, e continue servindo a página estática e `410` por cerca de um ano.** Outros servidores veem o `410`, param de buscar e entregar, e limpam seus caches.
Os URIs federados usam UUIDs, então há pouco risco de um Actor com o mesmo `id` ser criado de novo, mas se outra pessoa tomar o domínio, o identificador (`acct:alice@social.example`) e as URLs de perfil passam a ser dela, e podem ser usados para se passar pelos seus usuários. Continue renovando o domínio e o certificado, e pense de novo antes de abrir mão do domínio mesmo depois de passado o ano.
