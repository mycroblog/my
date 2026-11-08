<!-- source: instance-settings 4eb467f0dbca -->
# Configurações da instância

As configurações que definem como sua instância é operada: modo de cadastro, tamanho das publicações,
idioma padrão, quem pode ver as timelines, o nome da instância, a descrição, os termos, as regras, o tema e os emojis personalizados.

<!-- source: configuration-and-instance-settings a0b0908cef1e -->
## Configuração e configurações da instância

| | Configuração | Configurações da instância |
| --- | --- | --- |
| O que é | Como a implantação é montada: funções, adaptadores, endpoints, nomes de host, segredos | Como a instância é operada: modo de cadastro, limite de tamanho das publicações, idioma padrão, quem pode ver as timelines, o nome da instância, a descrição e os termos |
| Onde fica | Variáveis de ambiente `MY_*` e o arquivo de configuração ([referência de configuração](../reference/configuration.md)) | O banco de dados. Cada alteração é mantida como uma revisão |
| Alteração | Tem efeito quando o processo reinicia | Tem efeito imediatamente com `my instance set`, as telas de administração do cliente web ou a API de cliente ([telas de administração e a API de cliente](#admin-screens-and-the-client-api)) |
| Quando não definida | O padrão do esquema (o servidor não inicia se a chave for obrigatória) | O padrão definido no código |

- **Nenhuma chave está nas duas.** Nenhuma sobrescreve a outra. Você não pode fornecer uma configuração da instância por uma variável de ambiente ou
  pelo arquivo de configuração, e não pode armazenar um valor de configuração no banco de dados.
- Um valor fixo por implantação e que muda a estrutura do processo é configuração; um valor que muda enquanto
  a instância está em execução é uma configuração da instância.

<!-- source: prerequisites 12391490eea0 -->
## Pré-requisitos

- A configuração é fornecida como descrito na [referência de configuração](../reference/configuration.md) (`my config check`).
- O esquema está aplicado ao banco de dados (`my schema apply`; veja [Banco de dados](database.md)).

<!-- source: keys ae25d36239c2 -->
## Chaves

<!-- source: settings-one-key-one-value 4f827bf97480 -->
### Configurações (uma chave, um valor)

| Chave | Tipo | Padrão | Descrição |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | O modo de cadastro ([cadastro](registration.md)). Enquanto o encerramento do serviço está anunciado e depois que ele ocorre, e em um [servidor de uma pessoa](#one-person-server), o valor efetivo é `closed`. O valor armazenado não muda, e cancelar o anúncio ou desativar `single_user` o restaura. `GET /instance` e o NodeInfo mostram o valor efetivo |
| `max_status_length` | inteiro 1-100000 | `500` | O tamanho máximo de uma publicação, em caracteres |
| `default_locale` | tag de idioma BCP 47 | `en` | O idioma padrão. É o idioma inicial da interface dos novos usuários e o recurso usado quando nenhum catálogo corresponde à preferência de um usuário |
| `local_timeline_visibility` | `off` / `signed_in` / `everyone` | `everyone` | Quem pode ver a timeline local (`GET /api/v1/timelines/local`): ninguém, usuários que entraram ou todos. `off` responde 404 e `signed_in` responde 401 aos visitantes que não entraram; nos dois casos a navegação a oculta. A menos que o valor seja `everyone`, a página inicial não mostra publicações de exemplo. Em um [servidor de uma pessoa](#one-person-server) ela é lida como `off` |
| `federated_timeline_visibility` | `off` / `signed_in` / `everyone` | `signed_in` | Quem pode ver a timeline federada (`GET /api/v1/timelines/federated`). Os valores significam o mesmo que na timeline local, e ela também é lida como `off` em um servidor de uma pessoa |
| `preview_image` | ID de mídia (vazio = não definido) | vazio | A imagem de prévia de link (`og:image`) para publicações e perfis sem imagem. Quando não definida, a imagem padrão incluída é usada ([prévias de link](link-previews.md)) |
| `contact_account` | nome de usuário de uma conta local (vazio = não definido) | vazio | A conta para contatar os operadores. Veja [contato e miniatura](#contact-and-thumbnail) |
| `contact_email` | endereço de e-mail (vazio = não definido) | vazio | O endereço de e-mail para contatar os operadores. Também é o `Reply-To` e a assinatura dos e-mails que o servidor envia |
| `thumbnail` | ID de mídia (vazio = não definido) | vazio | Uma imagem que representa a instância (miniatura) |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | Se devem ser mantidas cópias da mídia remota. `none`: o proxy de mídia repassa cada requisição. `avatars`: avatares, cabeçalhos e emojis personalizados ficam em cache no armazenamento de blobs por `remote_media_cache_ttl_seconds`. `all`: toda a mídia remota fica. O tamanho total do cache é limitado pela configuração `remote_media_cache_max_bytes` |
| `remote_media_cache_ttl_seconds` | inteiro 60-31536000 (segundos) | `86400` | Por quanto tempo a mídia remota buscada pode ser mantida. É também o `Cache-Control: max-age` das respostas do proxy de mídia, então CDNs e navegadores guardam em cache por esse tempo |
| `remote_media_cache_max_bytes` | inteiro a partir de 1 (bytes) | `1073741824` | O limite de tamanho das cópias de mídia remota que o proxy de mídia mantém. A limpeza por hora exclui as cópias expiradas e depois as menos acessadas recentemente acima desse limite |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch (modo seguro). Quando `true`, um GET do ActivityPub sem uma assinatura HTTP verificável é respondido com `401` para objetos e coleções, e com um documento mínimo que traz a chave para o actor de um usuário. O actor da instância sempre retorna seu documento completo, para que qualquer servidor possa buscar a chave e verificar uma assinatura |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | A direção da federação. `off`: nenhuma federação. `inbound`: você segue e lê contas remotas, mas publicações locais, perfis e reações não são enviados, e seguidores de fora são recusados (`Reject`). `outbound`: você aceita seguidores de fora e entrega publicações locais, mas publicações e reações remotas não são recebidas. `both`: enviar e receber. Uma alteração tem efeito na hora e exige a permissão `manage_settings` |
| `dynamic_client_registration` | `true` / `false` | `true` | Registro dinâmico de clientes (RFC 7591). Enquanto `true`, `POST /oauth/register` aceita o registro de clientes públicos sem autenticação. Quando `false`, o endpoint recusa toda requisição com `403` (`access_denied`); os clientes já registrados continuam funcionando. Um job periódico exclui os clientes registrados dinamicamente que não são usados, seguindo a configuração `dynamic_client_unredeemed_retention` e `dynamic_client_idle_retention`. Uma alteração tem efeito na hora e exige `manage_settings` |
| `domain_actions_visibility` | `off` / `signed_in` / `everyone` | `off` | Quem pode ver a lista de domínios sobre os quais houve ações (`GET /api/v1/instance/domain_actions`): ninguém, usuários que entraram ou todos ([moderação](moderation.md#publishing-the-domains-you-act-on)) |
| `domain_actions_rationale_visibility` | `off` / `signed_in` / `everyone` | `off` | Quem pode ver os comentários públicos dessa lista. Quem não pode ver a lista nunca os vê |
| `appeal_window_days` | inteiro 20-365 (dias) | `180` | Quantos dias depois de uma ação a pessoa envolvida pode entrar com recurso ([moderação](moderation.md#appeals)). Alterá-lo também altera o prazo das ações já tomadas |
| `session_origins_retention_days` | inteiro 90-3650 (dias) | `180` | Quantos dias depois de um login manter o endereço IP e o User-Agent do login ([moderação](moderation.md#record-retention-and-holds)) |
| `registration_origins_retention_days` | inteiro 90-3650 (dias) | `180` | Quantos dias depois do cadastro manter o endereço IP do cadastro |
| `reports_retention_days` | inteiro 180-3650 (dias) | `365` | Quantos dias depois da última resolução manter uma denúncia. Uma denúncia a que uma ação se refere é mantida pelo tempo da ação |
| `moderation_actions_retention_days` | inteiro 180-3650 (dias) | `365` | Quantos dias depois do fim de uma ação (a revogação, a expiração ou a análise de um recurso, o que ocorrer por último) manter ações sobre contas e recursos |
| `audit_log_retention_days` | inteiro 365-3650 (dias) | `730` | Quantos dias depois de registrada manter uma linha do registro de auditoria |
| `remote_status_retention_days` | inteiro 1-3650 (dias) | `90` | Quantos dias uma publicação remota é mantida depois de publicada quando nada nesta instância se refere a ela (uma resposta local, impulso, favorito, item salvo, notificação ou uma conversa com uma publicação local) |
| `remote_status_max_retention_days` | inteiro 0-3650 (dias) | `0` | O máximo de dias que qualquer publicação remota é mantida depois de publicada, mesmo quando esta instância se refere a ela. Respostas, impulsos, favoritos e itens salvos locais permanecem e a mostram como excluída. `0` não define limite |
| `relay_status_retention_days` | inteiro 1-3650 (dias) | `7` | Quantos dias uma publicação remota trazida apenas por um relay é mantida, quando nada nesta instância se refere a ela e nenhuma conta local segue seu autor |
| `oauth_client_rate_limits` | pares `client_id=requests` separados por vírgula | vazio | Limites para as requisições da API de cliente dos clientes OAuth, para conter um cliente que sobrecarrega o servidor: as requisições que todos os usuários dele juntos podem fazer em 5 minutos. Uma alteração leva até um minuto para ser aplicada |
| `search_indexing` | `allow` / `deny` | `deny` | Se os mecanismos de busca podem indexar a instância. Mesmo com `allow`, apenas os perfis de contas `indexable` e as publicações `public` são indexados ([mecanismos de busca e crawlers de IA](crawlers.md)) |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | A declaração aos serviços de IA. `deny_training` recusa o treinamento; `deny` recusa o treinamento e o uso por IA (busca por IA, assistentes, agentes) |
| `indexable_default` | `true` / `false` | `false` | O padrão de `indexable` para contas novas. Alterá-lo não muda as contas existentes |
| `auto_delete_default_enabled` | `true` / `false` | `false` | Se as contas novas começam com a exclusão automática de suas publicações antigas ativada. Junto com as outras chaves `auto_delete_default_*`, é a política que uma conta nova recebe ([exclusão automática de publicações antigas](#automatic-deletion-of-old-posts)) |
| `auto_delete_default_min_age_days` | inteiro 1-3650 (dias) | `30` | Quantos dias depois de publicada a política de uma conta nova exclui uma publicação |
| `auto_delete_default_keep_pinned`, `auto_delete_default_keep_bookmarked`, `auto_delete_default_keep_favourited` | `true` / `false` | `true` | Se a política de uma conta nova mantém as publicações que o autor fixou, salvou ou favoritou |
| `auto_delete_default_keep_media`, `auto_delete_default_keep_polls` | `true` / `false` | `false` | Se a política de uma conta nova mantém as publicações com mídia ou enquete |
| `auto_delete_default_keep_public`, `auto_delete_default_keep_local`, `auto_delete_default_keep_unlisted`, `auto_delete_default_keep_private` | `true` / `false` | `false` | Se a política de uma conta nova mantém as publicações dessa visibilidade |
| `auto_delete_default_keep_direct` | `true` / `false` | `true` | Se a política de uma conta nova mantém as publicações diretas |
| `auto_delete_default_keep_min_boosts`, `auto_delete_default_keep_min_favourites` | inteiro 0-1000000 | `0` | A política de uma conta nova mantém uma publicação impulsionada ou favoritada pelo menos essa quantidade de vezes. `0` não define limite mínimo |
| `auto_delete_max_age_days` | inteiro 0-3650 (dias) | `0` | O máximo de dias depois de publicada que qualquer publicação local é mantida ([máximo](#a-maximum-for-keeping-posts)). `0` não define máximo |
| `auto_delete_max_allow_pinned` | `true` / `false` | `true` | Se as publicações fixadas ainda podem ser mantidas sob o máximo |
| `auto_delete_max_kept_pins` | inteiro 1-5 | `5` | Quantas fixações mantêm publicações sob o máximo, começando pela fixada mais recentemente. `5`, o máximo de fixações de uma conta, mantém todas as publicações fixadas |
| `auto_delete_max_allow_bookmarked`, `auto_delete_max_allow_favourited`, `auto_delete_max_allow_media`, `auto_delete_max_allow_polls` | `true` / `false` | `false` | Se as publicações que o autor salvou ou favoritou, ou que têm mídia ou enquete, ainda podem ser mantidas sob o máximo |
| `auto_delete_max_allow_visibilities` | `true` / `false` | `false` | Se as publicações ainda podem ser mantidas por sua visibilidade sob o máximo |
| `auto_delete_max_allow_thresholds` | `true` / `false` | `false` | Se as publicações ainda podem ser mantidas por seus impulsos e favoritos sob o máximo |
| `auto_delete_grace_days` | inteiro 0-365 (dias) | `30` | Quantos dias uma conta a que o máximo passa a se aplicar mantém suas próprias configurações depois de ser avisada. Uma alteração não move as datas das contas já avisadas |
| `search_visibilities` | `public` / `local` / `public,local` | `public` | Quais publicações a busca de texto completo de um servidor com backend de busca abrange. As publicações `unlisted`, `private` e `direct` nunca são abrangidas, e apenas as contas com `indexable` são. Deixar uma visibilidade de fora remove suas publicações do índice. Adicionar uma encontra apenas as publicações novas dela, até você [reconstruir o índice](search.md#changing-search_visibilities). Não faz nada com `search = none` |
| `security_contact` | URI `https:` ou `mailto:` (vazio = não definido) | vazio | Onde relatar problemas de segurança deste servidor: como ele é configurado e operado, e suas contas e dados. Quando definido, o servidor serve `/.well-known/security.txt` (RFC 9116). Este é o seu contato, não o do software: encaminhe um relato de vulnerabilidade no próprio software ao projeto como descreve sua [política de segurança](../../SECURITY.md) |
| `require_media_description` | `true` / `false` | `false` | Se todo arquivo de mídia (imagem, vídeo ou áudio) anexado a uma publicação local precisa de uma descrição. Quando `true`, uma publicação ou edição com um arquivo cuja descrição está ausente ou só tem espaços é rejeitada com 422. Ela é publicada como `limits.require_media_description` em `GET /api/v1/instance`, e o cliente web torna o campo de descrição obrigatório. É separada da configuração por usuário `require_media_description`, que só verifica antes de enviar |
| `link_cards` | `true` / `false` | `true` | Se devem ser exibidos cartões para os links nas publicações. Quando `true`, o servidor busca a página do link para montar um cartão. Quando `false`, ele para de buscar e também oculta os cartões já montados. Para parar apenas em alguns domínios, veja [domínios de cartões de link](#link-card-domains) |
| `link_cards_ignore_robots_txt` | `true` / `false` | `false` | Se as buscas de cartões de link ignoram o `robots.txt`. Quando `false`, o servidor obedece ao `robots.txt` dos sites linkados, então um site que proíbe `my-linkcard` ou `*` não recebe cartão ([prévias de link](link-previews.md#cards-of-links-in-posts)). `/about/crawler` informa aos sites linkados qual é ele. Quando `true`, o servidor não busca nem usa o `robots.txt`, e um site linkado não pode mais recusar essas buscas por ele; o servidor ainda se identifica como `my-linkcard`. Ative apenas quando faltarem cartões demais. Os cartões montados enquanto estava ativado são verificados na próxima nova busca depois que for desativado. As diretivas de uma página e os domínios de cartões de link valem nos dois casos |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | Se devem ser aceitos clientes com Client ID Metadata Document. Quando `true`, para uma requisição de autorização cujo `client_id` é uma URL `https`, o servidor busca os metadados nessa URL e os trata como o cliente (sem registro prévio). A busca não segue redirecionamentos, lê no máximo 5 KB e reutiliza a resposta por no máximo 24 horas, seguindo suas diretivas de cache. Quando `false`, ele para de buscar e recusa requisições de autorização, trocas de token e renovações desses clientes |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | Quanto dos custos de operação (registrados em `/admin/finance/costs` e definidos em `/admin/finance/publication`) publicar. `private`: nada é publicado (`GET /api/v1/instance/cost_totals` é 404). `totals`: apenas o total por período e moeda. `breakdown`: os totais e uma divisão por provedor e categoria, mostrados a todos nas páginas de informação (`/about` e `/about/costs`) e na API. `breakdown` revela em qual provedor (por exemplo, qual nuvem) você opera. Em todos os níveis, os nomes dos itens, as anotações e os pagamentos individuais permanecem privados. Os períodos até o que contém hoje (UTC), inclusive, são publicados |
| `finance_viewer_item_names` | `true` / `false` | `false` | Se os usuários com a permissão `view_finance` veem os nomes dos itens de custo e os nomes dos serviços dos custos importados na tela de finanças (`/finance`, `GET /api/v1/finance/cost_totals`). Os totais e a divisão por provedor e categoria sempre são mostrados, qualquer que seja esta configuração |
| `finance_viewer_item_memos` | `true` / `false` | `false` | Se os usuários com `view_finance` veem as anotações dos itens de custo na tela de finanças. Se a configuração de nome ou a de anotação for `true`, cada período mostra linhas de itens (provedor, categoria e o valor desse período, mais o nome e a anotação quando habilitados). Quem tem `manage_finance` vê tudo, independentemente |
| `finance_viewer_income` | `totals` / `plans` | `totals` | Se os usuários com `view_finance` veem apenas os totais de receita (`totals`) ou também o número de assinantes por plano (`plans`). O valor por plano e quem pagou nunca são mostrados. Se `income_visibility` for `plans`, as contagens publicadas são mostradas qualquer que seja este valor. A receita é lida na tela de finanças do cliente web (`/finance`) e em `GET /api/v1/finance/income_totals` |
| `income_visibility` | `private` / `totals` / `plans` | `private` | Quanto da receita dos apoiadores (o valor depois das taxas do provedor de pagamento) publicar. É escolhida separadamente de `finance_visibility`. `private`: nada é publicado (`GET /api/v1/instance/income_totals` é 404). `totals`: apenas o total de receita por período e moeda. `plans`: também o número atual de assinantes por plano, mostrado a todos. Em um plano com poucos assinantes, a contagem pode permitir adivinhar quem assina. O saldo (receita menos custos) só é mostrado quando nem este valor nem `finance_visibility` é `private`; caso contrário, o saldo revelaria o lado que você ocultou. Em todos os níveis, os pagamentos individuais, os assinantes, o valor por plano e os valores brutos e taxas permanecem privados. Os períodos até o que contém hoje (UTC), inclusive, são publicados |
| `supporter_list_visibility` | `off` / `signed_in` / `everyone` | `off` | Quem pode ver a lista de apoiadores (`GET /api/v1/instance/supporters` e `/about/supporters`). `off`: ninguém (a API responde 404 e `/about` não tem link). `signed_in`: apenas quem entrou neste servidor (caso contrário, a API responde 401). `everyone`: todos. A lista mostra apenas as contas e os planos dos assinantes ativos que escolheram aparecer; valores, cobrança mensal ou anual, datas e contagens nunca são mostrados. A página da lista e a resposta da API são sempre `noindex, noarchive`, qualquer que seja `search_indexing` |
| `user_app_registration` | `true` / `false` | `true` | Se os usuários que entraram podem registrar seus próprios apps OAuth (públicos ou confidenciais) na API de cliente (`POST /api/v1/oauth_apps`). Quando `false`, o registro responde 403. Listar, alterar e excluir apps registrados, reemitir segredos, emitir um token para si mesmo e a autorização por outros usuários continuam funcionando |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | Quem pode abrir salas de chat de voz, enquanto a configuração `voice` não for `off` ([configuração](../reference/configuration.md#voice)). `everyone`: todo usuário local que entrou e não está suspenso nem limitado. `permitted`: apenas quem tem a permissão `start_voice_rooms`, que por padrão só a função integrada `admin` tem e que uma função de um plano de apoiador pode ter |
| `voice_public_summary` | `true` / `false` | `false` | Se a página de uma sala de chat de voz cuja visibilidade é `local` mostra seu nome, descrição, anfitrião e se ainda está aberta aos visitantes que não entraram neste servidor e aos usuários remotos, e em sua prévia de link. Os participantes nunca são mostrados. Quando `false`, esses visitantes veem apenas que a sala é para os usuários locais deste servidor |
| `public_counts` | `true` / `false` | `true` | Se a página inicial (`/`, sem login) e `GET /api/v1/instance/overview` mostram quantos membros o servidor tem, quantas publicações públicas seus usuários fizeram em cada um dos últimos 7 dias e quando o servidor iniciou. Contam apenas as publicações públicas dos usuários deste servidor; os números são atualizados a cada poucos minutos. Quando `false`, a API os omite |
| `featured_accounts` | nomes de usuário de até 20 contas locais, separados por espaços ou vírgulas (vazio = nenhuma) | vazio | As contas que as sugestões de pessoas para seguir mostram primeiro, nesta ordem, antes das pessoas deste servidor que publicaram recentemente. Uma conta cujo dono desativou “Sugerir minha conta e permitir encontrá-la por parte do nome” não é sugerida. Veja [sugestões](#suggestions-of-accounts) |
| `login_providers_only` | `true` / `false` | `false` | Se o cadastro e a entrada ficam limitados aos provedores de login enquanto pelo menos um estiver habilitado: senhas e passkeys deixam de dar acesso, e usuários sem um provedor vinculado não conseguem entrar de novo depois que suas sessões terminarem. Sem nenhum provedor habilitado, não se aplica. `my instance set` mostra quantos usuários ativos não têm vínculo com um provedor habilitado. Veja [limitar a entrada aos provedores de login](login-providers.md#limit-sign-in-to-login-providers). |
| `invite_creators` | `everyone` / `permitted` | `permitted` | Quem pode criar convites próprios além da equipe com `manage_invites` ([convites de membros](registration.md#member-invites)). `everyone`: todo usuário local que entrou e não está suspenso nem limitado. `permitted`: apenas quem tem a permissão `create_invites`, que por padrão só a função integrada `admin` tem e que uma função de um plano de apoiador pode ter. No modo `approval`, um convite utilizável aprova o novo usuário na hora, então `everyone` permite que todo usuário pule a análise |
| `member_invite_max_active` | inteiro 0-100 | `5` | Quantos convites utilizáveis (não revogados, expirados nem esgotados) um membro pode manter ao mesmo tempo; `0` não permite que os membros criem nenhum. Os convites da equipe não contam |
| `member_invite_max_uses` | inteiro 1-1000 | `10` | Quantas pessoas podem se cadastrar com um convite criado por um membro |
| `member_invite_max_lifetime_seconds` | inteiro 3600-31536000 (segundos) | `2592000` (30 dias) | Por quanto tempo, no máximo, um convite criado por um membro pode funcionar. Reduzi-lo não encurta os convites existentes |
| `translation_providers` | IDs de provedores separados por vírgula | vazio | Os provedores de tradução no servidor que estão habilitados, o primeiro sendo o primeiro a ser tentado. Quando vazio, nada é enviado a nenhum provedor. Um ID para o qual este servidor não tem provedor não faz nada. Os [provedores de tradução](translation.md) integrados têm os IDs `libretranslate`, `deepl`, `google` e `aws`, e um [plugin tradutor](plugins.md#translating-posts) o ID `plugin` |
| `translation_provider_visibilities` | pares `provider=visibility\|visibility` separados por vírgula | vazio | O que cada provedor pode receber. Um provedor não citado recebe publicações `public`, `local` e `unlisted`; `provider=` sem nada depois não recebe nenhuma. `private` e `direct` vão apenas a um provedor citado com elas, então permitir um provedor não permite o seguinte |
| `translation_browser` | booleano | `false` | Se os leitores podem usar a tradução integrada ao navegador, que não envia nada a este servidor nem a um provedor. Com um provedor no servidor também habilitado, o provedor vem primeiro, e o navegador traduz o que o servidor recusa: uma publicação ou um idioma que nenhum provedor aceita, um limite mensal ou um provedor que falha |
| `translation_user_monthly_characters` | inteiro 0-1000000000000 | `50000` | Quantos caracteres as requisições de um usuário podem enviar a provedores de tradução em um mês UTC. Com `0`, nada é enviado. Não há valor ilimitado: defina um número grande |
| `translation_server_monthly_characters` | inteiro 0-1000000000000 | `500000` | Quantos caracteres o servidor inteiro pode enviar a provedores de tradução em um mês UTC. Com `0`, nada é enviado. Não há valor ilimitado: defina um número grande |
| `translation_retention_days` | inteiro 1-3650 (dias) | `30` | Quantos dias uma tradução é mantida depois de feita. Lê-la não estende esse prazo, e ela vai embora com a publicação |
| `translation_reports_to_server` | booleano | `false` | Se os moderadores podem enviar o texto de uma denúncia, as publicações denunciadas e o comentário de quem denunciou, aos provedores no servidor. Cada provedor ainda recebe apenas as visibilidades que `translation_provider_visibilities` permite a ele, seja quem for que peça: uma publicação direta denunciada vai apenas a um provedor autorizado para `direct`, e o comentário conta como `private`. A tradução do comentário nunca é mantida, nem a de uma publicação excluída, e os caracteres contam apenas para `translation_server_monthly_characters`. Quando `false`, nada de uma denúncia chega a um provedor |
| `voice_participants_retention_days` | inteiro 30-3650 (dias) | `90` | Quantos dias depois do fim de uma sala de chat de voz manter os registros de quem participou: as participações, os oradores e ouvintes, as mãos levantadas, os convites e as remoções. A sala em si (nome, descrição, anfitriões e como terminou) é mantida. Os registros de uma sala denunciada são mantidos pelo tempo da denúncia, e as retenções se aplicam como aos outros registros |
| `hook_inbound_status_mode`, `hook_inbound_follow_mode`, `hook_signup_check_mode`, `hook_media_check_mode`, `hook_local_status_mode`, `hook_inbound_actor_mode`, `hook_local_profile_mode` | `off`, `observe`, `enforce` | `observe` | Até onde cada [hook de decisão](decision-hooks.md#modes) age enquanto tem um plugin. `off` só chega a `enforce` passando por `observe`. |
| `hook_inbound_status_on_failure`, `hook_inbound_follow_on_failure`, `hook_media_check_on_failure`, `hook_inbound_actor_on_failure` | `pass`, `retry` | `pass` | O que o hook faz quando aplica a decisão e seu plugin falha ([detalhes](decision-hooks.md#when-the-plugin-fails)). |
| `hook_inbound_status_unfollowed_direct` | booleano | `false` | Se `inbound.status` também envia as publicações diretas de autores que nenhuma conta local segue ([detalhes](decision-hooks.md#what-inboundstatus-reviews)). A página de informações (`/about`) informa isso enquanto o hook está em uso. |
| `hook_signup_check_on_failure` | `pass`, `require_approval` | `pass` | O que `signup.check` faz quando aplica a decisão e seu plugin falha. |
| `hook_signup_check_reject` | booleano | `false` | Se um `reject` de `signup.check` [recusa o cadastro](decision-hooks.md#sign-ups) enquanto ele aplica a decisão. Quando desativado, a resposta é apenas registrada, como em `observe`. |
| `hook_local_status_account_age_days` | inteiro 0-3650 (dias) | `7` | `local.status` [analisa as publicações e suas edições](decision-hooks.md#local-posts) de contas mais novas que isso; `0` analisa todas as contas. A página de informações (`/about`) informa isso enquanto o hook está em uso. |
| `hook_local_profile_account_age_days` | inteiro 0-3650 (dias) | `7` | `local.profile` [analisa as alterações de perfil](decision-hooks.md#local-profiles) de contas mais novas que isso; `0` analisa todas as contas. A página de informações (`/about`) informa isso enquanto o hook está em uso. |
| `hook_decisions_retention_days` | inteiro 30-3650 (dias) | `90` | Quantos dias manter as [decisões dos hooks](decision-hooks.md#the-decisions). |
| `single_user` | booleano | `false` | Se o servidor é um [servidor de uma pessoa](#one-person-server). Só pode ser ativado enquanto o servidor tem exatamente uma conta local, e pode ser desativado a qualquer momento |

- Os valores são normalizados antes de serem armazenados (`TRUE` vira `true`, `0750` vira `750`, `ja_jp` vira `ja-JP`, os IDs de mídia são convertidos para minúsculas).
  Um valor inválido é rejeitado e nada é gravado.
- `preview_image` e `thumbnail` aceitam apenas a mídia de uma imagem que existe (inclusive uma ainda em processamento).
  Até o processamento terminar, e no lugar de uma imagem cujo processamento falhou, `preview_image` recorre à imagem padrão.
  Depois que uma mídia foi definida em qualquer revisão, a limpeza de envios não anexados não a exclui.
- `contact_account` aceita apenas o nome de usuário de um usuário local que não foi excluído (armazenado em minúsculas). `contact_email`
  aceita apenas um endereço na forma `alice@example.com`.
- `featured_accounts` aceita apenas os nomes de usuário de usuários locais que não foram excluídos; um `@` inicial é descartado, os nomes
  são convertidos para minúsculas, um nome repetido é mantido uma vez e eles são armazenados separados por espaços simples.

<!-- source: suggestions-of-accounts b7d362466e33 -->
### Sugestões de contas

Os usuários que entraram veem sugestões de pessoas para seguir (na timeline inicial vazia depois do cadastro e com
`GET /api/v1/accounts/suggestions`). As sugestões são as contas de `featured_accounts` na ordem delas, depois as
contas deste servidor que publicaram uma publicação pública ou local nos últimos 30 dias, com a que publicou por último primeiro. A
conta do próprio usuário, as contas que o usuário segue, bloqueia, silencia ou que o bloqueiam, e as contas suspensas ou
desativadas ficam de fora.

As contas novas são sugeridas e podem ser encontradas por parte do nome na busca de contas. Um usuário que desativa “Sugerir minha
conta e permitir encontrá-la por parte do nome” nas configurações do perfil sai das duas: a busca então encontra a conta apenas pelo
nome de usuário completo, ou para as pessoas que a seguem.

<!-- source: automatic-deletion-of-old-posts f0cf962c9de0 -->
### Exclusão automática de publicações antigas

Os usuários podem fazer suas próprias publicações serem excluídas automaticamente alguns dias depois de publicá-las (Configurações, “Excluir publicações antigas”).
A política de cada usuário é só dele: se está ativada, depois de quantos dias e quais publicações ela mantém (publicações fixadas, salvas e
favoritadas, publicações com mídia ou enquete, publicações de visibilidades escolhidas e publicações com pelo menos certa quantidade de impulsos ou
favoritos). Excluir uma publicação dessa forma é o mesmo que o autor excluí-la: os servidores para onde ela foi recebem um `Delete`.
Apenas as publicações de contas locais são excluídas dessa forma, nunca os impulsos.

As chaves `auto_delete_default_*` são a política com que cada nova conta local começa, seja qual for a forma como ela é criada
(o formulário de cadastro, um provedor de login externo, um convite ou `my admin`). A conta a recebe quando é criada e
depois passa a ser dona dela: alterar as chaves mais tarde não muda nem as contas que existem nem as que receberam um valor anterior.
A tela de configurações mostra os valores como do próprio usuário. Com as chaves não definidas, uma conta nova começa com a exclusão
automática desativada, após 30 dias, mantendo as publicações fixadas, salvas e favoritadas e as publicações diretas. O padrão não é mostrado
a quem está fora da instância. Quando um máximo está definido, uma conta nova recebe o padrão reduzido ao máximo, como
descrito abaixo, de modo que começa dentro dele.

<!-- source: a-maximum-for-keeping-posts b1d83e01feda -->
#### Um máximo para manter publicações

Com `auto_delete_max_age_days` acima de `0`, nenhuma publicação local é mantida por mais do que essa quantidade de dias depois de publicada.
Para toda conta local sem a permissão `keep_posts_longer`:

- A exclusão automática está sempre ativada: o usuário não pode desativá-la nem escolher mais dias do que o máximo (menos
  é permitido). A tela de configurações e `PUT /api/v1/preferences/auto_delete` recusam as duas coisas.
- Apenas as exceções que as chaves `auto_delete_max_allow_*` permitem mantêm publicações. Por padrão, apenas as publicações fixadas mantêm, todas
  elas; com `auto_delete_max_kept_pins` abaixo de 5, apenas essa quantidade das fixações mais recentes mantém suas publicações.
- Os usuários que desativaram a exclusão automática, ou nunca a definiram, também ficam sujeitos ao máximo.

Ativar o máximo, reduzi-lo ou alterar as exceções não reescreve as configurações de ninguém. Em vez disso, a cada hora o job
`content.auto_delete_statuses` encontra as contas cujas próprias configurações mantêm publicações que o máximo não mantém (porque
o máximo foi ativado ou reduzido, ou a conta perdeu `keep_posts_longer`). Ele avisa cada uma delas uma vez, com uma
notificação e um e-mail que vêm ativados por padrão, e informa a partir de quando o máximo se aplica: `auto_delete_grace_days` depois
do aviso. Até lá, as configurações da própria conta continuam valendo; a partir de então, suas publicações seguem o máximo. Uma conta
que volta a ficar dentro do máximo (seu usuário altera as configurações, ela recebe `keep_posts_longer`, ou o máximo some ou
aumenta) deixa de ser sujeita a ele, e uma alteração posterior a avisa de novo com um novo prazo de carência.

Ativar o máximo avisa, dentro de uma hora, todas as contas que ele atinge. As exclusões mantêm seu ritmo: no máximo 100 publicações
de uma conta por hora, cada uma enviada como um `Delete` aos servidores para onde a publicação foi, então uma conta com muitas publicações antigas
leva horas ou dias para esvaziar. Um usuário que altera suas configurações nesse meio-tempo interrompe o restante.

`keep_posts_longer` é uma permissão de membro: dê-a a uma função para que quem a tem mantenha suas publicações pelo tempo que escolher
e desative a exclusão automática. A função integrada `admin` a tem. Um [plano de apoiador](supporters.md) pode incluí-la, de modo que
manter as publicações por mais tempo pode ser um benefício do plano; quando a assinatura termina, o máximo volta a se aplicar depois do
período de carência.

O máximo é público: `GET /api/v1/instance` o retorna em `auto_delete_limit`, e a página sobre o exibe, para que as pessoas
o conheçam antes de se cadastrar. Alterações dessas chaves exigem `manage_settings` e são registradas no registro de auditoria como qualquer outra
configuração. No cliente web, elas ficam na página “Exclusão de publicações” de “Configurações do servidor”, que pergunta antes de salvar um
máximo que está sendo ativado ou tornado mais rígido.

<!-- source: texts-one-key-one-value c639fc7dfe23 -->
### Textos (uma chave, um valor)

| Chave | Limite | Descrição |
| --- | --- | --- |
| `name` | 100 caracteres, uma linha (apenas espaços em branco não é permitido) | O nome da instância (nome de exibição). Veja [nome e domínio](#name-and-domain) |
| `short_description` | 500 caracteres | Uma descrição curta |
| `description` | 20000 caracteres | A descrição |
| `terms` | 100000 caracteres | Os termos de serviço (Markdown) |
| `privacy_policy` | 100000 caracteres | A política de privacidade (Markdown) |
| `commerce_disclosure` | 100000 caracteres | A divulgação do vendedor para planos pagos, como o aviso que a lei japonesa exige (Markdown; [apoiadores](supporters.md)) |
| `preview_image_alt` | 1500 caracteres | O texto alternativo da imagem `preview_image` (`og:image:alt`). Quando não definido, usa-se “Imagem de {nome da instância}” |
| `thumbnail_alt` | 1500 caracteres | O texto alternativo da imagem `thumbnail` |

- Cada texto tem um valor. Um administrador o escreve uma vez no idioma que quiser, e os leitores de todos os idiomas veem o
  mesmo valor; os leitores de outros idiomas recorrem à tradução automática ou algo parecido. Os rótulos e as descrições dos campos de
  administração são traduzidos para o idioma de quem os vê como qualquer outro texto de tela; apenas o valor que um administrador escreve não é.
- O servidor não sabe o idioma de um texto, então o cliente web e as telas renderizadas pelo servidor exibem os textos com
  `dir="auto"` e `lang=""` (o valor HTML para um idioma desconhecido) em vez de herdar o idioma da tela.
- Definir um texto vazio o exclui.
- Além da CLI, os textos podem ser alterados nas telas de administração do cliente web (nome, descrição curta e descrição na
  página Geral de “Configurações do servidor”, `/admin/settings`; os termos na primeira página de “Termos e políticas”,
  `/admin/documents`, a política de privacidade em `/admin/documents/privacy-policy`, e a divulgação comercial e similares em
  `/admin/documents/commerce-disclosure`) e com a API de cliente (`GET /api/v1/admin/instance_texts`, `PUT /api/v1/admin/instance_texts/{key}`).
  Ambos exigem a permissão de função `manage_settings`, e as alterações são registradas no registro de auditoria (`text_changed`).

<!-- source: terms-and-privacy-policy 82e7bc325cf1 -->
### Termos e política de privacidade

- Escreva `terms`, `privacy_policy` e `commerce_disclosure` em Markdown (CommonMark mais as tabelas, o tachado
  e os autolinks do GitHub Flavored Markdown). Uma quebra de linha dentro de um parágrafo continua sendo uma quebra de linha, e os títulos com `#` começam um nível abaixo do
  título da página (`h2`).
- O servidor os converte em HTML e o higieniza (`GET /api/v1/instance/documents/{kind}` anônimo), e as páginas do cliente web
  `/about/terms`, `/about/privacy` e `/about/commerce` os exibem. HTML bruto, imagens e links que não sejam `http`,
  `https` e `mailto` são removidos. As páginas não exigem login (são páginas do cliente web, então exigem JavaScript).
  `/about/commerce` é a URL a fornecer ao provedor de pagamento.
- Um documento que você escreve recebe um link na tela de cadastro, no rodapé de toda tela que o servidor renderiza e no cliente web (a terceira coluna, o menu, as configurações e a tela de entrada).
  Um documento que não foi escrito não recebe link, e sua API responde 404.
- Enquanto os termos de serviço ou a política de privacidade estiver escrito, a tela de cadastro lista os documentos antes do botão de criar
  com uma caixa de seleção obrigatória, “Li e concordo com os documentos acima”. O servidor recusa um cadastro sem ela,
  inclusive um feito por um provedor de login. Sem nenhum dos dois documentos, a tela não pede concordância.

<!-- source: name-and-domain 1e14aa3e6c58 -->
### Nome e domínio

Uma instância tem um domínio e um nome, separadamente.

- O **domínio** é a configuração `host` (e `account_domain`, o domínio dos handles). Ele não pode ser alterado enquanto a
  instância está em execução. Handles (`@alice@example.com`), URLs, IDs do ActivityPub e `domain` em `GET /api/v1/instance` continuam
  sendo o domínio mesmo quando há um nome.
- O **nome** é o texto `name`. Ele aparece onde quer que a instância seja citada às pessoas: a barra lateral e “Sobre este servidor” do
  cliente web, o título da aba do navegador, as telas de entrada, cadastro, confirmação de e-mail, status da conta e consentimento de autorização
  renderizadas pelo servidor, os e-mails (confirmação do endereço de e-mail e notificações de moderadores), `og:site_name` das prévias
  de link, `name` / `short_name` do Web App Manifest, `metadata.nodeName` do NodeInfo e `name` em `GET /api/v1/instance`.
- O nome é o mesmo em todos os idiomas. Quando não há nome, o domínio (`host`) é exibido no lugar.
- Uma alteração tem efeito a partir da próxima requisição, sem reiniciar. O título da aba e a barra lateral de um cliente web aberto são atualizados quando
  você salva na tela de administração.

<!-- source: contact-and-thumbnail 71513ab82d51 -->
### Contato e miniatura

O contato para os operadores (`contact_account`, `contact_email`) e a imagem que representa a instância (`thumbnail`
e o texto `thumbnail_alt`) aparecem nos lugares abaixo. O que não estiver definido não aparece.

- “Sobre este servidor” do cliente web (`/about`). A miniatura e a conta de contato também aparecem na primeira tela (`/`)
  para as pessoas que não entraram.
- `contact` (`account` e `email`) e `thumbnail` (`url`, `width`, `height`, `alt`) em `GET /api/v1/instance`.
- `metadata` no NodeInfo: `maintainer` pela convenção do Misskey (`name` é o nome de exibição da conta de contato, ou seu
  nome de usuário quando ela não tem um; `email` é `contact_email`), `staffAccounts` pela convenção do Pleroma e do Akkoma (o ID do actor da
  conta de contato), `thumbnail` (`url` e `alt`) e `nodeDescription` a partir da descrição curta `short_description`.

Quando a conta de contato está suspensa ou excluída, a conta não é exibida e apenas o endereço de e-mail é.
Altere-os na página Geral da tela de administração do cliente web (“Configurações do servidor”, `/admin/settings`), ou com a CLI ou a API de cliente.

<!-- source: one-person-server 6ba524c58a4f -->
### Servidor de uma pessoa

Um servidor só do seu operador, a casa própria do operador no fediverso, é um servidor de uma pessoa: ative `single_user`. No cliente web,
um administrador o ativa e desativa com **Tornar este um servidor de uma pessoa** na página **Cadastro** de **Configurações do servidor**,
que informa por que o servidor recusou quando há mais de uma conta local.

```sh
my instance set single_user true
my instance set single_user false   # back to a community server
```

- Ele só pode ser ativado enquanto exatamente uma conta local conta. Toda conta local que não foi excluída conta,
  inclusive as pendentes de aprovação, desativadas, agendadas para exclusão, suspensas ou limitadas, já que cada uma poderia voltar como
  uma segunda pessoa. Uma conta cujo cadastro foi rejeitado não conta. Com qualquer outro número, `my instance set`
  falha informando a contagem, e a API de administração responde `422` com o tipo `/api/v1/problems/single-user-requires-one-account`.
  O actor da instância do servidor não é uma conta e nunca conta.
- Ele pode ser desativado a qualquer momento. Ativá-lo ou desativá-lo não exclui nada e não altera o valor armazenado de nenhuma outra configuração:
  o que ele retém volta a valer como estava assim que for desativado. Como toda configuração, a alteração vai para o registro de auditoria.

Enquanto está ativado:

- **O cadastro está fechado.** `registration_mode` é lido como `closed` seja qual for o valor armazenado, então a página de cadastro, o cadastro por um
  provedor de login e `my admin create` não criam nenhuma conta. Entrar com um provedor de login continua funcionando.
- **Somente a timeline inicial.** `local_timeline_visibility` e `federated_timeline_visibility` são lidas como `off`. As timelines de hashtags e de listas
  permanecem.
- **O que uma comunidade precisa fica oculto.** As timelines local e federada e seus streams, as sugestões de contas,
  os convites de usuários e de administradores, a análise de cadastros e os bloqueios de cadastro respondem `404` com o tipo
  `/api/v1/problems/single-user-mode`.
- **Sem novas publicações locais.** Em um servidor de uma pessoa, uma publicação para “Este servidor” não chega a ninguém além dos seguidores, então uma nova
  publicação não pode ser local: a API de cliente responde `422` e a deixa fora do que uma conta pode publicar. Uma visibilidade
  padrão de “Este servidor” é lida como “Seguidores” e não pode ser salva, e uma sala de chat de voz para as pessoas deste servidor se anuncia
  em uma publicação para os seguidores. As publicações que já são locais continuam locais.
- **O servidor diz o que ele é.** `single_user` em `GET /api/v1/instance` é `true`, com o modo de cadastro e as timelines efetivos
  e a conta do operador como `single_user_account`, e o NodeInfo responde `openRegistrations: false` com
  `metadata.singleUser: true`.
- **A página inicial é o perfil do operador.** As pessoas que não entraram e abrem o endereço do servidor veem o perfil
  de sua única conta, com o caminho para entrar e links para a página sobre, os termos de serviço e a política de privacidade
  que o servidor publica. O cliente web oculta o que o servidor não oferece, em sua navegação, menus e
  telas de administração: as timelines local e federada, o cadastro, as contas sugeridas, os convites, a análise de
  cadastros, os bloqueios de cadastro, a visibilidade “Este servidor” e as configurações de cadastro, convites, timelines
  públicas, página inicial e contas sugeridas.

A federação, a moderação, a busca de texto completo, o programa de apoiadores, os documentos e as telas de administração funcionam como em qualquer
servidor.

<!-- source: usage 68ad8384438f -->
## Uso

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- Escreva as flags antes da chave e do valor.
- Com `-owner <username>`, o valor de `preview_image` ou `thumbnail` é lido como o caminho de um arquivo de imagem e enviado como
  mídia dessa conta local. O worker processa a imagem antes de ela ser usada. Recomenda-se uma imagem horizontal de 1200x630.
- Uma alteração registra uma revisão (`instance_*_revisions`) e atualiza o valor atual (`instance_settings` / `instance_texts`)
  em uma única transação. Uma alteração feita pela CLI é registrada sem um autor (`BY` é `cli` no histórico).
- Definir o mesmo valor de novo ainda adiciona uma revisão.
- Códigos de saída: 0 para sucesso, 1 para um valor rejeitado ou um erro do banco de dados (nada muda), 2 para argumentos incorretos.

<!-- source: changing-the-direction-of-federation 9b9b63689c84 -->
### Alterar a direção da federação

- Restringi-la mantém os relacionamentos de seguidores, as publicações importadas e os registros de entrega, e não envia `Undo` nem `Reject`. O fluxo
  apenas para, e voltar para `both` (ou outro modo) o retoma como estava.
- As publicações e reações feitas enquanto restrita não são entregues retroativamente depois que você a amplia de novo.
- Do lado remoto, os seguidores dos usuários locais não recebem nada enquanto o modo é `inbound`, e os seguimentos dos usuários locais não são
  atualizados enquanto o modo é `outbound` ou `off`.
- As exclusões (de publicações ou de contas) feitas enquanto o modo é `off` não são enviadas. Em `inbound` e `outbound`, a retratação
  do que já foi enviado (`Delete` e similares) vai aos destinatários originais e aos seguidores da época.
- As alterações são registradas em `instance_setting_revisions` e no registro de auditoria (`target_kind = setting`).

Para cortar também os relacionamentos, use a CLI `my admin federation sever`.

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- Ele encerra os seguimentos pendentes e ativos entre contas locais e remotas como encerramentos por moderação
  (`follow_terminations`, `reason = moderation`). Uma mudança de modo nunca o chama automaticamente.
- `Reject` (`in`) ou `Undo` (`out`) só é enviado ao lado remoto quando o modo atual envia essa direção. Em um modo que
  não envia (como `off`), o seguimento termina apenas no lado local e o lado remoto não é avisado.
- É seguro executá-lo repetidamente. Ele imprime na saída padrão o número de seguimentos que encerrou.

<!-- source: admin-screens-and-the-client-api 6a747f50af97 -->
## Telas de administração e a API de cliente

Toda configuração que a CLI pode alterar também pode ser alterada nas telas de administração do cliente web e com a API de cliente. Ambos exigem
a permissão de função `manage_settings` (apenas a função integrada `admin` a tem). Como na CLI, uma alteração é mantida como uma revisão
e registrada por chave no registro de auditoria (`setting_changed`, o alvo é a configuração e o autor é esse usuário). Consulte o
registro de auditoria na tela de moderação (“Registro de auditoria”, `/moderation/audit-log`).

- **Cliente web** (“Configurações do servidor”, `/admin/settings`): a tela tem uma página por tópico, com link sob o título. Cada
  página tem um formulário e um botão de salvar, e salvar envia apenas o que você alterou nessa página; as configurações das outras páginas
  permanecem como estão.

  | Página | Seções e configurações |
  | --- | --- |
  | Geral (`/admin/settings`) | Nome, Descrições (`name`, `short_description`, `description`); Contato e miniatura (`contact_account`, `contact_email`, `thumbnail` e o texto `thumbnail_alt`) |
  | Ícone e logo (`/admin/settings/branding`) | O ícone, o logotipo e o logotipo para fundos escuros, cada um salvo com seu próprio botão ([Logotipo e ícone](#logo-and-icon)) |
  | Cadastro (`/admin/settings/registration`) | Cadastro (`registration_mode`, `login_providers_only`); Convites de membros (`invite_creators`, `member_invite_max_active`, `member_invite_max_uses`, `member_invite_max_lifetime_seconds`); Apps (`user_app_registration`, `oauth_metadata_document_clients`, `dynamic_client_registration`) |
  | Conteúdo (`/admin/settings/content`) | Idioma (`default_locale`, escolhido entre os idiomas que o app tem); Timelines (`local_timeline_visibility`, `federated_timeline_visibility`); Publicações (`require_media_description`); Página inicial (`public_counts`); Contas sugeridas (`featured_accounts`) |
  | Exclusão de publicações (`/admin/settings/auto-delete`) | Padrão para contas novas (as configurações `auto_delete_default_*`); Máximo (`auto_delete_max_age_days`, as configurações `auto_delete_max_allow_*`, `auto_delete_max_kept_pins`, `auto_delete_grace_days`). Salvar um máximo que está sendo ativado ou tornado mais rígido (menos dias, menos exceções ou fixações) pergunta antes, já que avisa as contas que ele passa a atingir ([Um máximo para manter publicações](#a-maximum-for-keeping-posts)) |
  | Federação (`/admin/settings/federation`) | Federação (`federation_mode`, `authorized_fetch`, `remote_media_policy`, `remote_media_cache_ttl_seconds` (escolha entre 1 hora, 1 dia, 7 dias, 30 dias ou 365 dias), `link_cards`, `link_cards_ignore_robots_txt`); Servidores limitados e suspensos (`domain_actions_visibility`, `domain_actions_rationale_visibility`); Análise externa (as configurações `hook_*`, veja [Hooks de decisão](decision-hooks.md)) |
  | Busca e rastreadores (`/admin/settings/discovery`) | Mecanismos de busca e IA (`search_indexing`, `indexable_default`, `ai_usage`, `security_contact`); Busca de publicações (`search_visibilities`, apenas enquanto o servidor tem busca de texto completo) |
  | Chat de voz (`/admin/settings/voice`) | `voice_room_starters`, `voice_public_summary`, `voice_participants_retention_days`; a página só é listada enquanto o servidor oferece chat de voz |

  Na página Geral, uma imagem é enviada quando você a escolhe e se torna a miniatura quando você salva. Quem pode ver os
  custos de operação, a receita e a lista de apoiadores é definido nas telas “Finanças” e “Apoiadores”.

  `max_status_length`, `preview_image`, `appeal_window_days`, os períodos de retenção (`*_retention_days`), `remote_media_cache_max_bytes` e `oauth_client_rate_limits` não aparecem na tela;
  altere-os com a CLI ou a API de cliente. As configurações de tradução têm uma tela própria, abaixo.
- **Cliente web** (“Tradução”, `/admin/translation`): os provedores que este servidor tem, sua ordem e o que cada um pode
  receber, a tradução do navegador, os limites mensais e por quanto tempo as traduções são mantidas, com os caracteres enviados
  neste mês e a taxa de acertos do cache. Cada processo do servidor soma sua contagem de acertos do cache à taxa no máximo uma vez por minuto,
  então as requisições mais recentes podem levar um minuto para aparecer, e um processo que para pode deixar seu último minuto sem contagem. Atualize sua política de privacidade ([modelo](privacy-policy-template.md#the-services-you-choose))
  antes de habilitar um provedor no servidor: o texto enviado a um provedor não pode ser recolhido. As chaves de API dos provedores e o endereço de um
  [plugin tradutor](plugins.md#translating-posts) ficam na [configuração da implantação](../reference/configuration.md);
  nenhuma configuração e nenhuma resposta da API de cliente as contém.
- **Limites de tradução.** O que conta são os caracteres enviados, em code points: os marcadores que representam menções,
  links e similares não contam, nem uma tradução que o servidor já mantém, uma publicação que já está no idioma do
  leitor ou a tradução do navegador. Cada chamada a um provedor conta, inclusive uma que falha, já que o provedor pode ter
  recebido o texto, então o número pode diferir do que um provedor cobra. Acima de um limite, a requisição é recusada com um `429`
  e nenhum provedor é chamado; o cliente web então usa a tradução do navegador do leitor, quando você a permite e o navegador
  consegue traduzir a publicação. Os meses são em UTC.
- **API de cliente**: `GET /api/v1/admin/settings` retorna todas as configurações com seu valor atual, seu padrão, se está definida
  e seu tipo e valores aceitos (opções ou intervalo). `PATCH /api/v1/admin/settings` altera configurações em conjunto com
  `{"values": {"<key>": "<value>", ...}}`. Todos os valores são strings, normalizadas e armazenadas como na CLI. Se uma única chave
  for desconhecida ou um valor for inválido, a resposta é um 422 que cita a chave e nada muda. As chaves alteradas são registradas
  como revisões e no registro de auditoria na mesma transação.
- Os convites, que funcionam nos modos `open`, `approval` e `invite`, são criados na tela de moderação (“Convites”,
  `/moderation/invites`, permissão `manage_invites`; veja [códigos de convite](registration.md#invite-codes)), e pelos membros
  conforme `invite_creators` permite ([convites de membros](registration.md#member-invites)).

<!-- source: rules b1076bb81e0c -->
## Regras

As regras da instância aparecem em `rules` de `GET /api/v1/instance`, e as denúncias as citam por ID.

- Uma regra tem uma frase de uma linha (até 300 caracteres) e um complemento opcional (até 1.000 caracteres). Como nos textos, o
  administrador as escreve no idioma que quiser e todos os idiomas veem o mesmo valor (`dir="auto"` e `lang=""`).
  Pode haver no máximo 100 regras ativas.
- Toda alteração (uma frase revisada, uma reordenação) é mantida como uma revisão (`instance_rule_revisions`), e o ID de uma regra
  não muda.
- Uma regra retirada desaparece da lista de regras ativas. Ela não pode ser trazida de volta, mas as denúncias ainda podem se referir a ela.

```sh
my instance rule create -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- A API de administração é `/api/v1/admin/rules` (listar, adicionar, obter, revisar, mover, retirar) e exige a permissão de função `manage_rules`.
- Na tela de administração do cliente web (“Regras”, `/admin/rules`, `manage_rules`), a lista mostra as regras em vigor na
  ordem, e “Mover para cima” e “Mover para baixo” as reordenam; o filtro acima da lista mostra as regras retiradas no lugar. “Adicionar uma
  regra” abre uma página com uma frase e um complemento. Escolha a frase de uma regra para abrir sua página, onde você a edita e
  a retira (depois de uma caixa de diálogo de confirmação). Toda alteração é registrada no registro de auditoria (`rule_added`, `rule_revised`,
  `rule_moved`, `rule_retired`).
- Os códigos de saída são os mesmos das configurações (0 para sucesso, 1 para um valor rejeitado ou um erro do banco de dados, 2 para argumentos incorretos).

<!-- source: link-card-domains c8f674387fa8 -->
## Domínios de cartões de link

Os cartões de link das publicações (`link_cards`) vêm de um crawler: o servidor busca a página linkada e sua imagem. Uma regra
para esse crawler em um domínio e seus subdomínios, e não mexe em mais nada sobre o domínio. Ela tem um de dois
modos. Uma regra é escolha do operador e se aplica além do que os próprios sites linkados recusam ([prévias de link](link-previews.md#cards-of-links-in-posts)).

| Modo | O que busca | Cartões que já existem |
| --- | --- | --- |
| Parar de buscar (`no_fetch`) | Nada do domínio, nem páginas nem imagens, inclusive quando um redirecionamento leva até lá | Ocultos |
| Parar apenas as imagens (`no_images`) | Páginas, mas nenhuma imagem do domínio | Mostrados sem a imagem |

- Uma regra afeta apenas os cartões de link. O domínio continua federado, e a mídia de suas publicações é buscada como de costume: para isolar um servidor, use
  uma [ação de domínio](moderation.md), que também interrompe seus cartões de link. Uma regra não é listada junto com as ações de domínio nem exportada com elas.
- Uma regra cobre o domínio e seus subdomínios, e uma imagem é verificada em relação aos hosts da página e da imagem. Toda regra que cobre um host se aplica, então uma regra em um subdomínio não
  afrouxa a regra do domínio pai. Um domínio é escrito e normalizado como em uma ação de domínio: um nome de host sem esquema nem porta, em qualquer grafia de um domínio internacionalizado.
- Uma alteração tem efeito na hora, inclusive sobre os cartões que o servidor já tem, e as imagens com proxy desses cartões deixam de ser servidas enquanto as imagens estiverem paradas.
- Remover uma regra traz de volta os cartões que o servidor já tem como estavam. Um link que foi ignorado recebe seu cartão da próxima vez que uma publicação precisar dele; nada é buscado de novo em massa.
  Um cartão buscado de novo enquanto as imagens estavam paradas continua sem imagem até sua próxima nova busca.
- Na tela de administração do cliente web (“Domínios de cartões de link”, `/admin/link-card-domains`, `manage_settings`), “Adicionar uma regra” define uma regra com uma anotação. A lista mostra 40 regras por página e pode ser pesquisada por domínio; escolher um domínio abre a página da regra, onde você altera o modo e a anotação, e a remove (depois de uma caixa de diálogo de confirmação).
- A API de administração é `/api/v1/admin/link_card_domains` (listar, definir; um `DELETE` com o domínio o remove) e exige a permissão de função `manage_settings`. Um servidor mantém no máximo 1.000 regras, cada uma com uma anotação de até 5.000 caracteres.
- Toda alteração é registrada no registro de auditoria (`link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed`).

<!-- source: theme 380636a0e583 -->
## Tema

O tema de todo o servidor é um conjunto de sobrescritas dos design tokens do cliente web mais um trecho opcional de CSS (CSS personalizado). Ele
se aplica ao cliente web de todo visitante e às telas que o servidor renderiza (entrada, cadastro, termos e assim por diante), tanto no
modo claro quanto no escuro.

- Você pode sobrescrever todos os tokens de `app/ui/src/theme.css`, exceto as cores primitivas (`--palette-*`). A lista, os padrões e os
  intervalos estão em [app/ui/README.md](../../app/ui/README.md#server-themes). Uma cor é `#rrggbb` para o claro e para o escuro
  (`color-overlay` também pode ser `#rrggbbaa`); os outros tokens aceitam um valor do formato de seu tipo (comprimento, número,
  duração, easing, lista de nomes de fontes, sombra) dentro do intervalo. Ao sobrescrever `font-sans` ou `font-mono`, mantenha
  `"my Noto Color Emoji"` em primeiro lugar; caso contrário, os emojis do cliente web são desenhados com a fonte do sistema operacional.
- Ao salvar, o servidor verifica o formato e o intervalo dos valores, o contraste de cada par de tokens de cor (os tokens que você
  não sobrescreve usam seus padrões; WCAG 2.2 AA tanto no claro quanto no escuro) e o tamanho do CSS personalizado (64 KiB). Se qualquer verificação
  falhar, nada é salvo.
- O servidor não lê o CSS personalizado. Ele o coloca na mesma folha de estilo depois das regras dos tokens, fora de qualquer `@layer`. Ele roda
  sob a mesma CSP do cliente web, então imagens, fontes e `@import` de outra origem não podem ser carregados.
  **O servidor não verifica os efeitos do CSS personalizado (contraste, rolagem horizontal, tamanho da área de toque, indicação de foco,
  idiomas da direita para a esquerda); isso é responsabilidade do administrador.** Os nomes de classe e a estrutura das páginas mudam entre
  versões, então apenas os tokens são estáveis para se confiar.
- Um tema é mantido como revisões (`instance_theme_revisions`), e a folha de estilo da revisão atual é servida em
  `/theme/{revision ID}.css` (`immutable`, em cache por muito tempo). O shell HTML tem um cache compartilhado (60 segundos mais 300
  segundos de revalidação), então leva alguns minutos depois que você salva para que todos o recebam. Uma página que já está aberta mantém
  a aparência antiga até ser recarregada.
- `<meta name="theme-color">` (claro e escuro) no shell e `theme_color` / `background_color` (claro) do Web App
  Manifest assumem o valor de `color-surface` do tema.
- Salvar uma revisão sem tokens e sem CSS personalizado redefine o tema para o padrão, e a folha de estilo deixa de ser carregada.
- Você pode alterá-lo na tela de administração do cliente web (“Tema”, `/admin/theme`), com a API de cliente (`GET` / `PUT
  /api/v1/admin/theme`) e com a CLI. A tela de administração e a API de cliente exigem a permissão de função `manage_settings`, e
  as alterações são registradas no registro de auditoria (`setting_changed`, alvo `theme`).

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

<!-- source: logo-and-icon a2d23def322b -->
## Logotipo e ícone

Além da miniatura e da imagem de prévia, a instância pode ter seu próprio ícone, logotipo e logotipo escuro, cada um enviado como um único SVG.
Enquanto nenhum for definido, as imagens integradas do mycroblog são usadas, e limpar um traz de volta a imagem integrada.

| Imagem | Formato | Onde aparece |
| --- | --- | --- |
| `icon` | Quadrado (dentro de 1%) | O favicon, o ícone da tela inicial do iOS (`apple-touch-icon`), os ícones do app instalado (Web App Manifest), o cartão do servidor na página inicial e ao lado de seu nome no painel lateral do cliente web, e onde quer que um cliente mostre o servidor como uma imagem quadrada |
| `logo` | De 1 a 8 vezes mais largo que alto | O cabeçalho do cliente web e o topo das páginas de entrada, cadastro e consentimento no lugar do wordmark integrado e do nome do servidor (o logotipo deve conter o nome, que é seu texto alternativo), a versão HTML de todo e-mail e a imagem de prévia de link quando `preview_image` não está definida |
| `logo_dark` | Como `logo` | Fundos escuros; sem ele, `logo` é usado também ali |

- **O servidor higieniza o SVG.** Ele o lê como XML e escreve um novo documento apenas com os elementos permitidos (formas, grupos,
  `defs`, `symbol`, `use`, gradientes, `clipPath`, `mask`, `pattern`, `style`, `title`, `desc`) e atributos (geometria,
  transformações, atributos de apresentação, `id`, `class`, `style`). Scripts, `foreignObject`, imagens incorporadas, links,
  animações, filtros (sombras e desfoques), manipuladores de eventos, referências a qualquer coisa fora do arquivo e URLs `data:` são removidos,
  e a resposta lista o que foi removido. Apenas o SVG higienizado é armazenado.
- Um SVG é recusado (nada é salvo) quando não é XML bem formado em UTF-8, tem um `DOCTYPE` ou declarações de entidades, não tem
  `viewBox`, tem mais de 256 KiB (128 KiB depois de higienizado), tem mais de 5.000 elementos, tem aninhamento acima de 32 níveis, tem mais de 200 elementos
  `use`, tem o formato errado para o seu tipo, contém `text` (converta o texto em contornos no seu editor antes), ou tem CSS que
  poderia carregar algo (`@import`, `@font-face`, `url()` que não seja `url(#id)`, escapes). `@media (prefers-color-scheme: dark)`
  em um `<style>` é permitido, de modo que um ícone pode mudar no modo escuro.
- Alguns lugares não aceitam SVG (a tela inicial do iOS, favicons do Safari mais antigo, prévias de link, e-mail). Para eles, o cliente que envia o
  SVG também desenha versões PNG a partir do SVG higienizado e as envia junto: 32, 48, 180, 192 e 512 px e um ícone maskable de 512 px
  para `icon`; um logotipo de e-mail com 96 px de altura e uma imagem de prévia de link de 1200 × 630 para `logo`; um logotipo de e-mail com 96 px de altura para
  `logo_dark`. O servidor verifica se cada uma é um PNG do tamanho certo (no máximo 1 MiB; o ícone de 180 px, o ícone maskable e a
  prévia de link precisam ser opacos) e a grava de novo sem metadados. Se qualquer arquivo estiver errado, nada é salvo. As versões opacas
  do ícone são desenhadas sobre uma cor de fundo, `#rrggbb`, que por padrão é o `color-surface` claro do tema. O servidor não
  verifica se as versões PNG mostram a mesma imagem que o SVG.
- Os arquivos são armazenados no armazenamento de blobs em `instance/brand/<revision>/` e servidos em
  `https://<host>/brand/<revision>/<version>.svg|.png` com `Cache-Control: public, max-age=31536000, immutable`, já que o
  conteúdo de uma revisão nunca muda. Um SVG é servido com `Content-Security-Policy: default-src 'none'; style-src
  'unsafe-inline'; sandbox` e `X-Content-Type-Options: nosniff`, de modo que, mesmo aberto sozinho, não executa nada nem carrega nada.
  Os arquivos de revisões anteriores permanecem, então os e-mails já enviados mantêm seu logotipo.
- Com um ícone definido, o shell HTML aponta para seus favicons PNG, seu SVG e seu `apple-touch-icon` de 180 px, o Web App Manifest lista
  seus ícones de 512 px, 192 px, maskable e SVG nessa ordem (outros servidores do fediverso usam o primeiro como o ícone deste servidor),
  e `/apple-touch-icon.png` retorna a versão de 180 px. O ETag do shell muda com as imagens, e o cache compartilhado do shell
  dura alguns minutos, como com o tema. Um app instalado recebe um novo ícone no Android quando o Chrome verifica o manifesto da próxima vez;
  em computadores, o app mantém o ícone antigo até ser instalado de novo.
- `GET /api/v1/instance` retorna `icon`, `logo` e `logo_dark` com a URL do SVG, a URL e o tamanho de cada versão PNG
  e a revisão; cada um é omitido enquanto a imagem integrada está em uso.
- Defina-os no cliente web em **Configurações do servidor** > **Ícone e logo** (`/admin/settings/branding`, permissão de função `manage_settings`). Escolha um arquivo
  SVG para o ícone, o logotipo ou o logotipo para fundos escuros: o servidor o verifica, o navegador desenha as versões PNG e o
  formulário as exibe em prévia (uma aba do navegador, a tela inicial, o ícone do app, um e-mail em fundo claro e escuro e a prévia
  de link) com a lista do que o servidor removeu. Se o servidor recusar o arquivo, o motivo aparece sob o botão com como
  corrigir. Para o ícone, defina ali também a cor de fundo de suas versões opacas. **Salvar** envia o SVG com todas as versões
  PNG; **Usar o ícone padrão**, **Usar o logotipo padrão** e **Remover o logotipo para fundos escuros** os limpam. O
  cliente web guarda as imagens que mostrou para uso offline.
- A API de cliente faz o mesmo, com a permissão de função `manage_settings`:
  `POST /api/v1/admin/instance/brand/sanitize` verifica um SVG e retorna o SVG higienizado com as versões PNG a desenhar (não
  salva nada), `PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` salva o SVG com todas as versões PNG
  (`multipart/form-data`, uma parte por versão), `DELETE` no mesmo caminho limpa uma, e `GET /api/v1/admin/instance/brand` retorna as imagens
  atuais com a cor de fundo do ícone. Toda alteração é mantida como uma revisão (`instance_brand_revisions`) e registrada no
  registro de auditoria (`setting_changed`, alvo `brand_icon`, `brand_logo` ou `brand_logo_dark`).
- A CLI não consegue defini-las, já que as versões PNG são desenhadas pelo cliente, mas pode exibi-las e limpar uma que esteja com defeito:

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

<!-- source: custom-emoji 5e0ddb3251cf -->
## Emojis personalizados

Os emojis personalizados locais são mostrados como imagens quando alguém escreve `:shortcode:` no corpo ou no aviso de conteúdo de uma publicação, ou no
nome de exibição, na bio ou nos campos do perfil. Eles também chegam ao Mastodon, ao Misskey, ao GoToSocial e ao Akkoma por meio da tag `Emoji` das
Notes e dos actors que você envia.

- Um shortcode tem de 2 a 30 letras, dígitos ou `_`. Diferencia maiúsculas de minúsculas e não pode ser alterado depois que o emoji é registrado.
  Os aliases (até 10) e uma categoria (até 64 caracteres) são usados para buscar e agrupar no seletor de emojis.
- A imagem é PNG, GIF ou WebP, de no máximo `custom_emoji_max_bytes` (padrão 256 KiB) e de no máximo 1.024 px em cada lado. O
  servidor cria derivadas que cabem em caixas de 64, 128 e 256 px e não guarda o original. Um APNG vira uma imagem estática
  do seu primeiro quadro. O GoToSocial, por padrão, não busca emojis maiores que 100 KiB, então imagens pequenas são recomendadas.
- Um emoji desativado não aparece em lugar nenhum (publicações, perfis, federação, o seletor), e `:shortcode:` volta a ser exibido como texto. Um emoji
  definido para ficar oculto do seletor só deixa de aparecer no seletor e ainda pode ser usado escrevendo-o. Um emoji que veio de
  outro servidor só pode ser desativado ou copiado como um emoji deste servidor.
- Copiar busca a imagem do emoji de outro servidor sob os mesmos limites do proxy de mídia (as imagens de domínios com
  `reject_media` não são buscadas), executa as mesmas verificações acima e a registra como um novo emoji local. O shortcode é o
  do original ou o que você indicar. A imagem é reduzida à caixa de 256 px, e uma imagem que não pode ser buscada ou não passa nas verificações
  não deixa nada para trás.
- A exclusão não pode ser desfeita. Os emojis em publicações antigas também voltam a ser texto, e registrar o mesmo shortcode de novo cria um
  emoji diferente. As imagens que os servidores federados copiaram não podem ser excluídas.
- A tela de administração (“Emoji personalizado”, `/admin/custom-emojis`), a API de cliente (`/api/v1/admin/custom_emojis`) e a CLI fazem as
  mesmas coisas. Na tela, “Registrar um emoji personalizado” abre o formulário de registro (`/admin/custom-emojis/new`), e escolher
  um emoji na lista abre sua página (`/admin/custom-emojis/{id}`), onde você altera suas configurações, substitui sua imagem, ativa ou
  desativa, exclui ou copia um de outro servidor; a própria lista ativa e desativa um emoji com um toque. A tela de administração e a API de cliente exigem a permissão de função `manage_custom_emojis` (apenas a função integrada `admin`
  a tem). Registrar, copiar, substituir a imagem, alterar configurações e aliases, ativar e desativar, e excluir são
  registrados no registro de auditoria (alvo `custom_emoji`) seja qual for a forma como são feitos (uma operação da CLI é registrada como uma operação
  da CLI).

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- Um usuário com a permissão de função `create_custom_emojis` (um recurso de usuário) pode solicitar a criação de um emoji em “Emoji
  personalizado” nas configurações (`/settings/custom-emojis`), alterar os aliases, a categoria e a visibilidade no seletor dos emojis que
  criou, e excluí-los. Substituir uma imagem também é uma solicitação. Uma solicitação não aparece em lugar nenhum (seletor, publicações, reações,
  federação) até que alguém com `manage_custom_emojis` a aprove. Aprove ou rejeite na página “Solicitações” de “Emoji
  personalizado” na tela de administração (`/admin/custom-emojis/requests`), com a API de cliente (`/api/v1/admin/custom_emoji_requests`) ou com a CLI acima. As pessoas com
  `manage_custom_emojis` são notificadas de uma solicitação (`admin_custom_emoji_request`), e quem solicitou é notificado da
  aprovação ou da rejeição. Solicitar, retirar, aprovar e rejeitar são registrados no registro de auditoria (alvo
  `custom_emoji_request`).
- Uma solicitação de criação pendente reserva seu shortcode. Uma solicitação ou um registro com o mesmo shortcode não é possível até
  que a pendente seja rejeitada ou retirada. A imagem de um emoji aprovado é copiada como um envio do actor da instância, de modo que
  ela permanece mesmo que a conta de quem solicitou seja excluída.
- Um criador pode alterar seus emojis apenas enquanto tem `create_custom_emojis`. Os emojis permanecem se a função for removida.
  Os administradores podem alterar e excluir todos os emojis, independentemente do criador.
- Quando a função vem de um plano de apoiador, o número de emojis que um usuário pode criar por mês é limitado pelo limite
  mensal de emojis do plano. Ele conta as solicitações de criação que o usuário enviou no mês-calendário atual (UTC) que estão
  aprovadas ou pendentes; solicitações rejeitadas e retiradas não contam, e excluir um emoji não libera uma vaga. Uma solicitação
  conta no mês em que foi enviada, mesmo que seja aprovada no seguinte. A contagem é zerada às 00:00 UTC do primeiro
  dia de cada mês, e a página de configurações mostra ao usuário quando, no fuso horário dele. Substituir a imagem de um emoji e os
  emojis que os administradores registram não contam. Uma função que um administrador atribuiu manualmente, e um servidor em que os apoiadores
  estão desativados, não têm limite.
