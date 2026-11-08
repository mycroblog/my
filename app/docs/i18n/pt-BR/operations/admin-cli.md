<!-- source: administrators-and-roles 1baeba6d67b4 -->
# Administradores e funções

Como criar o primeiro administrador, gerenciar funções pela linha de comando, anunciar e executar o encerramento do serviço, processar de novo as imagens enviadas, reconstruir o índice de busca e exportar a página de manutenção.

<!-- source: prerequisites b7ae62128809 -->
## Pré-requisitos

- A configuração (`host`, `database`, `database_url`, `kek` e assim por diante) é definida como descreve a [referência de configuração](../reference/configuration.md). `my config check` verifica essa configuração.
- O schema é aplicado ao banco de dados (`my schema apply`; veja [database.md](database.md)).
- `host` e `account_domain` são definitivos. O primeiro comando que usa o banco de dados os registra, e todo comando posterior se recusa a executar quando a configuração é diferente (veja [Configurações que não podem ser mudadas depois](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: creating-an-administrator e5fec2e3d287 -->
## Criando um administrador

```sh
my admin create -username alice -email alice@example.com
```

- A partir de um terminal, o comando pede a senha duas vezes, sem exibi-la na tela.
- Quando a entrada padrão não é um terminal, a primeira linha da entrada padrão é a senha (por exemplo, `my admin create ... < password.txt`).
  A senha não pode ser informada como argumento de linha de comando nem como variável de ambiente, porque esses valores acabam na lista de processos e no histórico do shell.
- Quando um comando avulso não tem terminal nem entrada padrão, como um [job do Cloud Run](deploy-cloud-run.md#9-create-the-first-administrator), adicione `-password-link`. Então o comando não lê senha nenhuma: ele define uma senha aleatória que ninguém conhece e imprime, após a linha `created admin`, um link que define a senha uma única vez, durante uma hora, como faz `my admin account password-reset-link` (o registro de auditoria o registra como `password_reset_link_issued`).
  Quem puder ler a saída do comando pode usar o link, e, nessa plataforma, essa saída fica nos registros da plataforma até o link ser usado ou expirar. Abra o link imediatamente. Se ele expirar, emita outro com `my admin account password-reset-link @alice`. Se a emissão do link falhar, o administrador já existe: o comando informa isso e encerra com código 1, e o mesmo comando gera um novo link.
- Uma senha tem de 8 a 128 caracteres. Ela nunca aparece na saída, nos registros nem em mensagens de erro.
- Uma transação cria o usuário, a senha, o endereço de e-mail principal, a conta local (com suas chaves) e a concessão da função `admin` (sem concedente, o que significa a CLI).
  As funções integradas são criadas primeiro, se estiverem faltando.
- Em caso de sucesso, o comando imprime `created admin @alice@<account_domain> (user <id>, account <id>)` e encerra com código 0.
- Em um [servidor de uma pessoa](instance-settings.md#one-person-server), o comando não cria conta e encerra com código 1.
  Enquanto o [login estiver limitado aos provedores de login](login-providers.md#limit-sign-in-to-login-providers), ele também avisa que a senha não pode entrar até o fim do limite.

<!-- source: running-it-again 197692f7f103 -->
## Executando de novo

- Quando o nome de usuário ou o endereço de e-mail (ambos sem distinção de maiúsculas e minúsculas) já está em uso, o comando não altera nada, informa qual deles está em uso e encerra com código 1.
  Ele nunca altera a senha nem as funções do usuário existente.
- Uma execução que falha não grava nada, então você pode corrigir a causa e executar o mesmo comando de novo.
- O código de saída 2 significa erro de uso.

<!-- source: granting-and-revoking-roles b87e1adb131e -->
## Concedendo e revogando funções

Funções guardam permissões. As funções integradas são `admin` (`position` 1000, com todas as permissões) e
`moderator` (`position` 100, com `manage_reports`, `manage_accounts` e `view_audit`).
`my serve` verifica as funções integradas toda vez que inicia (em um processo de qualquer `role`), e `my admin role` as verifica toda vez que é executado.
Cada um cria o que estiver faltando e concede a `admin` todas as permissões da versão em execução.
Quando uma atualização acrescenta uma permissão, nenhum passo manual é necessário. Um `moderator` mantém as permissões e a posição que o administrador definiu.

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- As permissões pertencem a um de três grupos.
  - Moderação: `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit`.
  - Administração do servidor: `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers`.
  - Recursos para membros: `view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`.

  `list` imprime uma coluna por grupo (`MODERATION`, `SERVER`, `MEMBER`). A interface web mostra “Moderação” e “Administração” para os dois primeiros grupos e não conta os recursos para membros em nenhum deles (veja [moderation.md](moderation.md)).
- Uma mudança entra em vigor na próxima requisição do usuário. As notificações para moderadores vão para as pessoas que têm a permissão no momento em que a notificação é criada.
- Concessões e revogações são gravadas no registro de auditoria como `role_granted` e `role_revoked` (tipo de alvo `role`). O autor de uma operação da CLI é `cli` (`my admin audit list -target-kind role`).
- Conceder uma função que o usuário já tem, ou revogar uma que ele não tem, não altera nem registra nada.
- O último titular de `admin` não pode perdê-la (código de saída 1). Conceda `admin` a outro usuário primeiro.
- A API administrativa é `GET /api/v1/admin/roles`, `GET /api/v1/admin/accounts/{id}/roles`, e `PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}`, e exige a permissão `manage_roles`.
  Pela API, você não pode conceder nem revogar uma função acima da `position` mais alta entre as suas próprias funções (403). Revogar o último `admin` retorna 409.
- A equipe age apenas sobre cargos inferiores. Pela API, as ações de moderação e a revogação delas, as decisões sobre recursos, a remoção de supressões de e-mail, a revelação dos dados privados de uma conta, a emissão de um link de redefinição de senha e a remoção da autenticação em dois fatores exigem que a `position` mais alta entre as suas funções seja maior que a de todas as funções do alvo. Uma posição igual ou superior, incluindo a da sua própria conta, retorna 403. Um moderador não pode agir sobre um administrador, e um administrador não pode agir sobre outro.
  Esses comandos (`my admin account ...`) não têm esse limite, então use-os quando os administradores discordarem.
- O único administrador não pode ser impedido de entrar nem removido: `disable_login`, `suspend` e `ban` sobre ele são recusados, pela API (409) e por estes comandos (código de saída 1), e ele não pode excluir a própria conta (409). Outro titular de `admin` só conta enquanto puder entrar: não depois de excluir a conta, nem enquanto estiver suspenso, banido ou sob `disable_login`.

<!-- source: announcing-the-end-of-the-service e1c40ec145f0 -->
## Anunciando o fim do serviço

Para encerrar o servidor, anuncie o encerramento primeiro. [service-closure.md](service-closure.md) descreve o procedimento completo, do anúncio à exclusão dos dados.
O servidor continua funcionando normalmente durante o anúncio. `closure` em `GET /api/v1/instance` e `metadata.serviceClosure` no NodeInfo mostram o estado e as datas.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` não tem limite inferior. `-export-until` (o prazo para retirar dados) deve ser igual ou posterior a `-closes-at`, e o padrão é 30 dias após a data de encerramento.
- Quando a data de encerramento está a menos de 30 dias, o comando imprime um aviso na saída de erro padrão (o anúncio é gravado mesmo assim).
- Alterar as datas é registrado como um novo anúncio, e um anúncio feito após uma retirada também é um novo anúncio. Depois que o servidor é encerrado, você não pode anunciar nem retirar o anúncio (código de saída 1).
- Anunciar, alterar as datas e retirar o anúncio são gravados no registro de auditoria como `service_closure_announced` e `service_closure_withdrawn` (tipo de alvo `instance`; veja `my admin audit list -target-kind instance`).
- A API administrativa é `GET /api/v1/admin/closure`, `PUT` / `DELETE /api/v1/admin/closure/announcement`, `GET /api/v1/admin/closure/check` e `POST /api/v1/admin/closure/close`. Ela exige a permissão `manage_service_closure` (entre as funções integradas, só `admin` a tem).
- Durante o anúncio, o servidor não aceita novos usuários nem novos pagamentos.
  - O modo de registro efetivo é `closed` (o valor de `registration_mode` não muda, e volta a valer quando o anúncio é retirado). Nem cadastros nem cadastros com convite funcionam, e as solicitações pendentes não podem ser aprovadas (`409`).
  - Anunciar rejeita todas as solicitações pendentes (registradas como decididas por `service_closure`) e cancela todas as assinaturas de apoiador em andamento ao fim do período.
    Os assinantes recebem uma notificação (`supporter_service_closure`) e um e-mail. Novas assinaturas retornam `409`. Retirar o anúncio não desfaz as rejeições nem os cancelamentos.
  - Cada anúncio, alteração de datas e retirada envia a notificação `service_closure` a todos os usuários locais. Tudo isso roda como jobs, então não pare o worker.
  - Cada usuário com endereço de e-mail confirmado recebe um e-mail pelo anúncio, por uma alteração de datas, pela retirada, 7 dias e 1 dia antes da data de encerramento, pelo encerramento e 7 dias antes do prazo de exportação. Os usuários não podem optar por não receber (veja [email.md](email.md)).
  - Após a data de encerramento, as pessoas que têm `manage_service_closure` recebem a notificação `admin_service_closure_due`. O servidor não se encerra sozinho.

Após a data de encerramento, encerre o servidor. O encerramento não pode ser desfeito.

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` e `close` mostram o número de contas locais, o número de caixas de entrada de destino (as caixas compartilhadas conhecidas), uma estimativa das atividades `Delete` a enviar (o produto desses dois números), o número de assinaturas de apoiador em andamento, o número de migrações não concluídas (`Move`) e o número de contas sob retenção (`retention_holds`).
  Quando restam migrações ou há uma retenção, o comando emite um aviso na saída de erro padrão. Exporte os registros das contas sob retenção com `my admin account export-moderation` e guarde-os antes de descartar os dados.
- `close` recusa sem perguntar quando não há anúncio e antes da data de encerramento (código de saída 1). Qualquer resposta diferente de `y`, ou uma entrada diferente do domínio do servidor (`host` na configuração, sem distinção de maiúsculas e minúsculas), não altera nada e encerra com código 1.
- O encerramento é gravado no registro de auditoria como `service_closure_closed`. Na mesma transação, ele encerra todas as assinaturas de apoiador em andamento (motivo `service_closed`) e enfileira os jobs que cancelam as assinaturas no provedor de pagamento imediatamente (`CancelNow`).
- Após o encerramento, a API cliente aceita apenas estas requisições: ler a própria conta e as configurações, as exportações (`/exports`, `/csv_exports`), as sessões (`/sessions`, `/authorized_apps`), `GET /instance` e a administração do encerramento e da fila de jobs. Todo o resto é recusado com `403` (`/api/v1/problems/service-closed`). Os streams param, e o Web Push não envia nada. Entrar e redefinir a senha continuam funcionando.
- Após o encerramento, o worker entrega um `Delete` do ator de cada conta local aos pares da federação, 100 por vez (as contas migradas por último). Ele envia mesmo quando `federation_mode` é `off` ou `inbound`, e nunca para um domínio suspenso.
  `my admin closure status` (e `progress` em `GET /api/v1/admin/closure`) mostra quantas contas têm um `Delete` registrado, quantas entregas ainda faltam expandir e o número de entregas em cada estado (`pending`, `succeeded`, `permanent_failure`, `skipped`, `dead`). Mantenha o worker em execução durante a entrega.
  Quando todas as entregas terminam, uma verificação que roda a cada 10 minutos registra `delivery_completed` (`service_closure_delivery_completed` no registro de auditoria), e os atores locais passam a responder `410`.
  O servidor não executa de novo as entregas `dead` por conta própria; execute-as de novo com `my job retry`.

Depois do prazo de exportação, gere a página de encerramento estática que o seu domínio serve depois que o servidor é parado. O comando lê o banco de dados, então execute-o antes de descartar o banco de dados.

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- Saída: `<dir>/<locale>.html` para cada idioma e `<dir>/index.html` com todos os idiomas um após o outro. As páginas usam o mesmo modelo de `my maintenance export` e mostram o nome do servidor, a data de encerramento, a mensagem do operador no anúncio e `status_page_url`.
  A saída também traz exemplos de configuração para o front end: `nginx.conf`, `Caddyfile` e `cloudfront-function.js`. Eles respondem `410` para `/ap/*`, `/inbox`, WebFinger, host-meta, NodeInfo e `/@*`, e servem a página de encerramento para todo o resto.
- O comando recusa (código de saída 1) a menos que o servidor esteja encerrado. Antes de `delivery_completed`, ele imprime um aviso e ainda grava as páginas. [service-closure.md](service-closure.md) descreve como trocar o front end pela página estática.

<!-- source: processing-uploaded-images-again 65a9e3da0bd8 -->
## Processando de novo as imagens enviadas

Cada imagem enviada mantém três tamanhos, e uma animação também mantém as suas versões em movimento. Uma nova versão de `my` pode mudar esses tamanhos ou formatos, e uma imagem existente mantém os seus arquivos até ser processada de novo:

```sh
my admin media reprocess -before 2027-01-31T09:00:00Z
```

- `-before` recebe o momento da mudança, como a hora em que você atualizou, no formato RFC 3339. Só são tratadas as imagens processadas pela última vez antes desse momento, e um horário que ainda não passou é recusado.
- O worker faz o trabalho, 100 imagens por minuto, cada uma em um job próprio (`media.reprocess_walk` e `media.reprocess` na [referência de jobs](../reference/jobs.md)). Mantenha um worker em execução; a página de jobs administrativa (`/admin/jobs`) mostra a fila.
- Executar de novo com o mesmo horário é seguro. As imagens já processadas são deixadas como estão, e as que a primeira execução não alcançou são tratadas.
- Os originais não são mantidos, então cada imagem é refeita a partir da sua versão grande. Detalhes que a versão grande perdeu não voltam, e cada execução com um horário posterior recodifica as imagens mais uma vez e perde um pouco de qualidade. Execute uma vez por mudança.
- Vídeos e áudios não são recodificados e ficam como estão. Uma imagem que os limites atuais não aceitam mais mantém a versão atual, e o worker registra um aviso.

A nova versão de uma imagem tem novas URLs. Os arquivos antigos permanecem por `media_replaced_retention` (30 dias por padrão, [referência de configuração](../reference/configuration.md#media_replaced_retention)), para que as páginas abertas e os caches continuem funcionando, e depois o job horário `media.purge_replaced` os exclui. Outros servidores não são avisados das novas URLs: um servidor que ainda tenha uma URL antiga e não tenha cópia própria deixa de mostrar a imagem depois desse prazo. Reserve espaço extra de armazenamento até que os arquivos antigos sejam removidos.

<!-- source: rebuilding-the-search-index 9061821656ac -->
## Reconstruindo o índice de busca

`my search` cuida do índice da busca de texto completo das publicações, em um servidor com mecanismo de busca. Com `search = none`, cada comando informa isso, não faz nada e encerra com código 0.

```sh
my search rebuild              # empty the index, then have the worker add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second (20 by default, 1000 at most)
my search clear                # empty the index and end any rebuild
my search status               # the backend and the progress and failures of the latest rebuild
```

Execute `my search rebuild` de novo para retomar uma reconstrução que parou. Ela continua a partir da última publicação tratada e não esvazia o índice de novo. Enquanto uma reconstrução estiver incompleta, o outro modo é recusado com código de saída 1. [Busca de texto completo das publicações](search.md) explica quando usar cada um.

<!-- source: exporting-the-maintenance-page 1756fcf1e02e -->
## Exportando a página de manutenção

`my maintenance export` grava a página do modo de manutenção como HTML estático, para que um load balancer, uma CDN ou um proxy reverso a sirva enquanto o próprio `my serve` não pode rodar (uma imagem sendo substituída, uma revisão de implantação que não inicia).
Ela usa o mesmo modelo do modo de manutenção, então ambos mostram as mesmas palavras e a mesma aparência.

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- Saída: `<dir>/<locale>.html` para cada idioma de `-lang`, para front ends que escolhem uma página pelo `Accept-Language`, e `<dir>/index.html` com todos os idiomas um após o outro, para os que não conseguem fazer isso.
  As páginas têm CSS embutido, nenhum script e nenhum recurso externo; a Content-Security-Policy delas fica em um elemento `<meta>`.
- Textos: as configurações `maintenance_server_name`, `maintenance_message`, `maintenance_until` e `status_page_url` ([referência de configuração](../reference/configuration.md)).
- Sem `-offline`, ele também lê o nome da instância (quando `maintenance_server_name` não está definido), o idioma padrão dela (quando `-lang` não é informado) e as substituições de tokens do tema. O CSS personalizado do tema nunca é incluído. Quando não consegue ler o banco de dados, ele imprime um aviso, grava as páginas a partir da configuração e do tema padrão, e encerra com código 0.
- Execute de novo sempre que essas configurações ou o tema mudarem, e antes de cada manutenção, para que as páginas no front end estejam sempre atualizadas.
