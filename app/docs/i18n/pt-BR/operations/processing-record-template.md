<!-- source: record-of-processing-activities-template 75f7037cf953 -->
# Modelo de registro das atividades de tratamento

O registro das atividades de tratamento que o artigo 30(1) do GDPR pede que um controlador mantenha, preenchido até onde o software decide.

**Este é um modelo e não constitui aconselhamento jurídico.** Ele descreve o que o software faz com as configurações padrão e não decide se o GDPR, ou este registro, se aplica a você. Peça uma revisão para a sua jurisdição.

O artigo 30(5) isenta uma organização com menos de 250 funcionários, a menos que o tratamento possa implicar risco para os direitos e as liberdades das pessoas, não seja ocasional ou inclua categorias especiais ou dados criminais. Administrar uma instância mantém contas de forma contínua, então não presuma a isenção sem verificá-la. Manter o registro também ajuda você a responder a solicitações e a redigir a [política de privacidade](privacy-policy-template.md).

<!-- source: how-to-use-it 32e30db3b6f3 -->
## Como usar

1. Copie tudo a partir de [Registro](#record) para os seus próprios registros. Este é um documento interno: não o publique junto com os textos da instância e entregue-o a uma autoridade supervisora quando ela solicitar.
2. Substitua cada `{{...}}`. Um valor entre as chaves, como `{{180 days}}`, é o padrão do software. Confirme-o na sua instância. O [modelo de política de privacidade](privacy-policy-template.md#the-periods-and-their-settings) indica a configuração de cada prazo.
3. Exclua uma atividade, ou uma linha que começa com `{{IF ...}}`, que não se aplique à sua configuração. [Os serviços que você escolhe](privacy-policy-template.md#the-services-you-choose) diz a qual configuração cada serviço corresponde.
4. Verifique se não sobrou nada: `grep -n '{{' <your file>` não imprime nenhuma linha.
5. Revise-o quando mudar uma configuração ou um serviço e date cada revisão.

O artigo 30 não pede a base legal, e o registro tem uma linha para ela porque você precisa dela para a política. Trate as entradas ali como questões a resolver.

Se você administra uma instância em nome de outra pessoa, pode ser operador (processor) para ela. O artigo 30(2) então pede um registro diferente, que este modelo não cobre.

<!-- source: record ed52538cf508 -->
## Registro

<!-- source: controller-article-301a 9720484d93ff -->
### Controlador (artigo 30(1)(a))

| | |
| --- | --- |
| Controlador | {{NAME, POSTAL ADDRESS, EMAIL}} |
| Controlador conjunto | {{NAME AND CONTACT, OR "none"}} |
| Representante na União | {{NAME AND CONTACT, OR "not applicable"}} |
| Encarregado de proteção de dados | {{NAME AND CONTACT, OR "none"}} |
| Instância | {{INSTANCE NAME AND URL}} |
| Software | my, versão {{VERSION}} |
| Última revisão | {{DATE, BY WHOM}} |

<!-- source: recipients-used-in-this-record 8f1748f022f5 -->
### Destinatários usados neste registro

Estes nomes são usados nas atividades abaixo. Liste cada um que você usa.

| Destinatário | Nome, contato e contrato | País |
| --- | --- | --- |
| Hospedagem (servidor, banco de dados, armazenamento de blobs, fila, gerenciamento de chaves) | {{...}} | {{...}} |
| Proxy reverso ou balanceador de carga | {{...}} | {{...}} |
| {{IF cdn}} CDN | {{...}} | {{...}} |
| {{IF mail}} Provedor de e-mail | {{...}} | {{...}} |
| {{IF captcha}} Serviço de CAPTCHA | {{...}} | {{...}} |
| {{IF login providers}} Provedores de login | {{...}} | {{...}} |
| {{IF payments}} Provedor de pagamento (Stripe ou PAY.JP) | {{...}} | {{...}} |
| {{IF error reports}} Rastreador de erros | {{...}} | {{...}} |
| {{IF telemetry}} Serviço de métricas e rastreamento | {{...}} | {{...}} |
| {{IF voice}} Servidor de mídia de voz, STUN e TURN | {{...}} | {{...}} |
| {{IF webhooks}} Receptores de webhook | {{...}} | {{...}} |
| {{IF follow review}} Serviço de análise de pedidos de seguir | {{...}} | {{...}} |
| {{IF sign-up review}} Serviço de análise de cadastros | {{...}} | {{...}} |
| {{IF post review}} Serviço de análise de publicações | {{...}} | {{...}} |
| {{IF profile review}} Serviço de análise de perfis | {{...}} | {{...}} |
| {{IF media review}} Serviço de análise de mídia | {{...}} | {{...}} |
| {{IF translation}} Provedores de tradução | {{...}} | {{...}} |
| {{IF external review of posts}} Análise externa de publicações de outros servidores | {{...}} | {{...}} |
| Armazenamento de backup | {{...}} | {{...}} |
| Servidores federados | Outros servidores da rede, administrados por outros controladores, que mudam com o tempo. `my` mantém a lista de servidores conhecidos | Qualquer país |
| {{IF relays}} Relays | {{EACH RELAY YOU JOINED}} | {{...}} |
| Serviços de push | Escolhidos pelo navegador de cada usuário | Depende do navegador |
| Apps | Autorizados por cada usuário | Depende do app |

<!-- source: transfers-article-301e 82e1298af5f2 -->
### Transferências (artigo 30(1)(e))

- A entrega federada envia dados pessoais a servidores no exterior, em países que não são conhecidos de antemão e não podem ser limitados. {{YOUR ASSESSMENT: whether this is a transfer under GDPR Chapter V, and on what basis. This template leaves it open.}}
- Transferências para os serviços acima: {{FOR EACH, THE THIRD COUNTRY, AND THE ADEQUACY DECISION OR THE SAFEGUARD}}.

<!-- source: 1-accounts-and-signing-in 865bacb0785b -->
### 1. Contas e login

| | |
| --- | --- |
| Finalidades | Criar contas, fazer login, recuperar o acesso, decidir os cadastros |
| Titulares dos dados | Pessoas com uma conta, pessoas que solicitam uma |
| Dados pessoais | Endereços de e-mail, hash da senha, segredo de dois fatores e códigos de recuperação com hash, chaves de acesso, o identificador em um provedor de login, preferências (idioma, fuso horário, padrões de publicação), motivo do cadastro, convite e decisão, funções, tokens de sessão e de apps com hash |
| Destinatários | Hospedagem, provedor de e-mail, provedores de login, equipe com `manage_accounts` |
| Exclusão | Exclusão da conta: 7 dias após a exclusão. Uma conta desativada é excluída após 30 dias. Sessões, tokens e códigos: 30 dias após expirarem. Convites e atribuições de função, a URI da conta e o nome de usuário permanecem. Uma conta que ninguém exclui permanece |
| Base legal | {{e.g. contract}} |

<!-- source: 2-protecting-sign-in-and-registration 66d632cbf187 -->
### 2. Proteção do login e do cadastro

| | |
| --- | --- |
| Finalidades | Prevenir abusos, encontrar contas da mesma pessoa durante a moderação, limitar a taxa de solicitações, impedir bots |
| Titulares dos dados | Pessoas com uma conta, candidatos, visitantes das páginas de login |
| Dados pessoais | Endereço IP do cadastro, endereço IP e User-Agent do login, endereço IP (e, para solicitações de redefinição de senha, o endereço de e-mail) nos contadores de limite de taxa, bloqueios de cadastro (domínio de e-mail, endereço com hash, intervalo de IP). Com CAPTCHA: endereço IP, informações do navegador e do dispositivo, cookies. Com uma análise de cadastro: o endereço de e-mail e o endereço IP de cada candidato sem convite |
| Destinatários | Hospedagem, equipe com `manage_accounts` (cada revelação fica no registro de auditoria), serviço de CAPTCHA, serviço de análise de cadastros |
| Exclusão | Endereços IP de cadastro `{{180 days}}`, origens de login `{{180 days}}`, contadores dentro do dobro da janela do limite, bloqueios de cadastro 30 dias após a revogação ou a expiração. Interrompida por uma retenção legal |
| Base legal | {{e.g. legitimate interests}} |

<!-- source: 3-publishing-and-social-features 5e271f0279f9 -->
### 3. Publicação e recursos sociais

| | |
| --- | --- |
| Finalidades | Publicar perfis e publicações, e as interações que as pessoas pedem |
| Titulares dos dados | Pessoas com uma conta, pessoas que elas mencionam |
| Dados pessoais | Perfil e suas versões, publicações e suas versões, mídia, favoritos, reações, impulsos, itens salvos, fixados, votos em enquetes, seguidos, bloqueios, silenciamentos, listas. As publicações podem conter qualquer dado que uma pessoa escreva, inclusive categorias especiais. `{{IF post review}}` O texto de uma publicação recusada que o autor envia com um recurso |
| Destinatários | Hospedagem, CDN, serviço de análise de mídia (uma pequena cópia processada de cada upload), outros usuários, a web pública para conteúdo público, servidores federados (atividade 4), mecanismos de busca e crawlers de IA conforme `search_indexing` e `ai_usage`, `{{the operator of the full-text search engine, if another company runs it}}`, apps que a pessoa autorizou, `{{IF post review}}` o serviço de análise de publicações para as publicações `public`, `local` e `unlisted` de contas novas e suas edições (o texto como digitado e como renderizado, aviso de conteúdo, idioma, marcação de sensível, opções de enquete, descrições de imagens, hashtags e número de menções, o endereço delas e os endereços das publicações a que respondem ou que citam, o endereço e o nome de usuário do autor e quando a conta foi criada; sem mídia), `{{IF profile review}}` o serviço de análise de perfis para as mudanças de perfil de contas novas (o nome de exibição, a bio como digitada e como renderizada, os campos do perfil, se o avatar ou o cabeçalho mudou, o endereço e o nome de usuário da conta e quando a conta foi criada; sem imagens) |
| Exclusão | Quando a pessoa a exclui, pela sua política de exclusão automática, ou na exclusão da conta. `{{IF auto_delete_max_age_days}}` No máximo `{{auto_delete_max_age_days}}` dias após a publicação, para as contas sem `keep_posts_longer`. Uploads não anexados a nada `{{24 hours}}`. Versões antigas de imagens depois que uma versão mais nova as substitui `{{30 days}}`. O registro da URI de uma publicação excluída permanece. `{{IF post review}}` Um recurso vai embora com a sua decisão (`hook_decisions_retention_days`, mantido enquanto ainda pode ser contestado ou aguarda análise) ou na exclusão da conta |
| Base legal | {{e.g. contract}} |

<!-- source: 4-federation 795a56004c37 -->
### 4. Federação

| | |
| --- | --- |
| Finalidades | Entregar a atividade local a outros servidores e receber a deles |
| Titulares dos dados | Pessoas com uma conta, pessoas em outros servidores |
| Dados pessoais | Enviados: documento do ator, publicações, pedidos de seguir, bloqueios, impulsos, curtidas, reações, votos em enquetes, citações, mudanças de conta, comentários de denúncias encaminhadas (sem o denunciante), mudanças de ciclo de vida. Mantidos: cópias das atividades enviadas e recebidas, tentativas de entrega |
| Destinatários | Servidores federados, relays, serviço de análise de pedidos de seguir, serviço de análise de perfis |
| Exclusão | Aqui: as recebidas 30 dias e seus documentos 7 dias. As enviadas 30 dias, e a entrega de uma publicação até 30 dias após a publicação ou o seu autor serem excluídos. Lá: fora do seu controle. `Delete` é enviado, e espera-se, mas não se força, que os outros servidores o respeitem |
| Base legal | {{e.g. contract, legitimate interests}} |

<!-- source: 5-storing-data-of-people-on-other-servers 3708a84995bb -->
### 5. Armazenamento de dados de pessoas em outros servidores

| | |
| --- | --- |
| Finalidades | Mostrar perfis e publicações remotos aos usuários locais |
| Titulares dos dados | Pessoas em outros servidores, que em geral não têm nenhuma relação com você |
| Dados pessoais | Documentos de atores, versões de perfis, handles, publicações, referências de mídia, cópias de mídia, interações |
| Destinatários | Hospedagem, CDN, usuários locais, páginas públicas, `{{IF external review of posts}}` a análise externa de publicações de outros servidores para as publicações e revisões que se relacionam a uma conta local, inclusive publicações só para seguidores (`private`), e publicações diretas somente quando uma conta local segue o autor, a menos que `hook_inbound_status_unfollowed_direct` esteja ativado (o texto e os idiomas, aviso de conteúdo, marcação de sensível, hora de publicação, endereço e URL, o endereço da publicação a que respondem, opções de enquete, hashtags, endereços e descrições de anexos, o autor e o servidor dele, visibilidade, o número de contas citadas e de contas locais entre elas, e se uma conta local segue o autor) |
| Exclusão | Publicações `{{90 days}}` quando nada aqui se refere a elas, `{{7 days}}` quando só um relay as trouxe, e `{{no limit}}` para qualquer publicação nos demais casos. Cópias de mídia `{{not kept}}`. Verificações de handles 90 dias. Tudo de uma conta 7 dias depois que o servidor dela a exclui, ou até 30 dias quando apenas um `410 Gone` indicou isso e o servidor não responde mais. A URI e o domínio permanecem |
| Base legal | {{e.g. legitimate interests}} |

<!-- source: 6-notifications ad34beea6ab2 -->
### 6. Notificações

| | |
| --- | --- |
| Finalidades | Informar as pessoas sobre a atividade e sobre a conta delas, por e-mail e Web Push |
| Titulares dos dados | Pessoas com uma conta, pessoas citadas em uma notificação |
| Dados pessoais | Notificações e marcadores de leitura, configurações de notificação, endereço de e-mail, o texto de cada e-mail, registros de devolução e de reclamação, endereço e chaves do Web Push, assinaturas de publicações |
| Destinatários | Hospedagem, provedor de e-mail, serviços de push (o corpo da mensagem é criptografado, e eles o retêm por até 48 horas) |
| Exclusão | Na exclusão da conta, ou junto com a publicação a que se referem. Web Push: também quando a pessoa ou o serviço de push o encerra. O provedor de e-mail: {{ITS PERIOD}} |
| Base legal | {{e.g. contract, legitimate interests}} |

<!-- source: 7-moderation-and-audit d72a42b6312e -->
### 7. Moderação e auditoria

| | |
| --- | --- |
| Finalidades | Tratar denúncias, ações e recursos, comprovar o que a equipe fez, retenções para solicitações legais |
| Titulares dos dados | Pessoas denunciadas, denunciantes, equipe, pessoas citadas em um comentário ou em uma nota |
| Dados pessoais | Denúncias (comentário, publicações, regras, notas, atribuições, resoluções), ações (motivo, nota, publicações), recursos, registro de auditoria (autor, ação, o nome do alvo), retenções e seu motivo, publicações mantidas como evidência. Pode incluir dados criminais ou categorias especiais se uma denúncia ou uma publicação os contiver |
| Destinatários | Hospedagem, equipe com a permissão correspondente, a pessoa envolvida por meio dos seus próprios registros de moderação (nunca a identidade do denunciante), o servidor da conta denunciada no caso de uma denúncia encaminhada |
| Exclusão | Denúncias `{{365 days}}` a partir da última resolução, ou junto com uma ação que se refira a elas. Ações e recursos `{{365 days}}` a partir do fim da ação. Registro de auditoria `{{730 days}}`. Denúncias abertas e ações sem fim permanecem. Uma retenção liberada some após o prazo das ações |
| Base legal | {{e.g. legitimate interests, legal obligation}} |

<!-- source: 8-requests-from-people 762c1e8bc31c -->
### 8. Solicitações das pessoas

| | |
| --- | --- |
| Finalidades | Responder a solicitações de acesso e portabilidade, e a importações |
| Titulares dos dados | Pessoas com uma conta |
| Dados pessoais | Arquivos de exportação (ator, publicações, os próprios registros de moderação), arquivos CSV, importações e seus resultados, a correspondência de uma solicitação |
| Destinatários | Hospedagem, a pessoa |
| Exclusão | Arquivos e importações 7 dias. Correspondência: {{PERIOD}} |
| Base legal | {{e.g. legal obligation, contract}} |

A exportação do próprio usuário não contém endereços de e-mail, endereços IP, notificações nem preferências. Para uma resposta completa, use `my admin account export-moderation` para os registros de moderação e a [revelação](moderation.md#account-management) para endereços de e-mail e IP, e leia o restante no banco de dados. A exportação de moderação omite as identidades dos denunciantes: verifique se um comentário não revela uma antes de entregá-lo.

<!-- source: 9-voice-chat e0bf1be42da8 -->
### 9. Chat de voz

Exclua esta atividade quando `voice = off`.

| | |
| --- | --- |
| Finalidades | Administrar salas de chat de voz e moderá-las |
| Titulares dos dados | Anfitriões e participantes |
| Dados pessoais | Nome da sala, descrição, anfitriões e como a sala terminou; quem participou, falou, levantou a mão, foi convidado ou foi removido. Endereços IP e áudio passam pelo servidor de mídia de voz ou pelo servidor TURN, ou chegam diretamente aos outros participantes com `voice = p2p`. O servidor não grava áudio |
| Destinatários | Hospedagem, servidor de mídia de voz, STUN e TURN, participantes |
| Exclusão | Participantes `{{90 days}}` a partir do fim da sala, ou enquanto durar uma denúncia sobre ela. As salas permanecem, com o nome e a descrição apagados na exclusão do autor. Nos serviços: {{THEIR PERIOD}} |
| Base legal | {{e.g. contract, legitimate interests}} |

<!-- source: 10-supporter-payments 99d64d001780 -->
### 10. Pagamentos de apoiadores

Exclua esta atividade quando `payments = none`.

| | |
| --- | --- |
| Finalidades | Receber assinaturas, conceder funções, contabilidade |
| Titulares dos dados | Assinantes |
| Dados pessoais | IDs de objetos do provedor, valores, períodos, estados, reembolsos, estornos, a escolha de ser listado. Endereço de e-mail enviado ao provedor. Sem números de cartão, quatro últimos dígitos nem datas de validade |
| Destinatários | Hospedagem, provedor de pagamento, pessoas que podem ver a lista de apoiadores, equipe com a permissão correspondente |
| Exclusão | Recibos de webhook 45 dias. O vínculo com o cliente e a inclusão na lista na exclusão da conta. Os registros de assinatura e de pagamento não têm fim automático: {{PERIOD AND THE LAW THAT REQUIRES IT}}. No provedor: {{ITS PERIOD, AND WHETHER YOU DELETE THE CUSTOMER}} |
| Base legal | {{e.g. contract, legal obligation}} |

<!-- source: 11-running-the-service 9c89577f9cee -->
### 11. Operação do serviço

| | |
| --- | --- |
| Finalidades | Encontrar falhas, restaurar após uma falha, conectar suas próprias ferramentas |
| Titulares dos dados | Todos os acima |
| Dados pessoais | Logs (caminhos que podem trazer um nome de usuário ou uma publicação, texto de erros), logs do proxy com endereços IP, relatórios de erros limpos por padrões, registros de jobs com IDs, corpos de webhook, backups de tudo |
| Destinatários | Hospedagem, proxy reverso, rastreador de erros, serviço de métricas e rastreamento, receptores de webhook, armazenamento de backup |
| Exclusão | Jobs com falha e registros de execução `{{30 days}}`. Mensagens de webhook 14 dias. Contagens horárias de erros 90 dias. Logs da aplicação e do proxy: {{PERIOD}}. O relatório mais recente de cada grupo de erros: substituído pelo próximo, sem fim automático. Backups: {{PERIOD}}. No rastreador de erros e nos receptores: {{THEIR PERIOD}} |
| Base legal | {{e.g. legitimate interests}} |

<!-- source: 12-translating-posts 17a9b7a81356 -->
### 12. Tradução de publicações

Exclua esta atividade quando `translation_providers` estiver vazio.

| | |
| --- | --- |
| Finalidades | Traduzir uma publicação quando um leitor pede, {{IF translation of reports}} traduzir uma denúncia quando um moderador pede, e manter a tradução dentro dos seus limites |
| Titulares dos dados | Autores de publicações, aqui e em outros servidores, e pessoas citadas no texto. Leitores, pela contagem de caracteres. {{IF translation of reports}} Denunciantes, pelo texto dos seus comentários |
| Dados pessoais | O que quer que o texto de uma publicação contenha: o corpo, o aviso de conteúdo, as opções de enquete e as descrições de imagens. O endereço da publicação, o autor, o leitor, as menções e os links são substituídos ou omitidos antes do envio. Mantidos: as traduções e os caracteres por conta e por mês |
| Destinatários | Provedores de tradução, como seus operadores (processors): publicações `public`, `local` e `unlisted` por padrão. {{IF translation to private and direct}} Também publicações `private` e `direct`, para {{THE PROVIDERS YOU ALLOWED}}. {{IF translation of reports}} Os comentários de denúncias, para os provedores autorizados a receber publicações `private`, e as publicações que as denúncias citam, quando um moderador pede |
| Exclusão | Traduções `{{30 days}}` a partir da tradução, e junto com a publicação quando ela é excluída. Caracteres por conta até o fim do mês seguinte, ou na exclusão da conta. Nos provedores: o que foi enviado não pode ser recolhido. {{THEIR PERIODS}} |
| Base legal | {{e.g. legitimate interests}} |

<!-- source: security-measures-article-301g 756bb9e562e3 -->
### Medidas de segurança (artigo 30(1)(g))

O que o software faz:

- As senhas são armazenadas como hashes Argon2id. Os tokens de sessão, de acesso e de uso único e os códigos de recuperação são armazenados como hashes.
- As chaves privadas de assinatura, os segredos de dois fatores e os segredos de provedores e de webhooks são criptografados com AES-256-GCM sob uma chave de criptografia de chaves (KEK) mantida fora do banco de dados ([gerenciamento de chaves](key-management.md)).
- Os endereços de e-mail e IP são mostrados somente à equipe com `manage_accounts`, por meio de uma ação explícita que o registro de auditoria registra.
- As solicitações de login, de cadastro e de redefinição de senha têm limite de taxa. As permissões da equipe passam por funções.
- As solicitações de saída que servidores remotos podem influenciar passam por uma proteção contra o acesso a endereços internos.

O que você faz:

- TLS no proxy reverso: {{CONFIGURATION}}
- Criptografia do banco de dados, do armazenamento de blobs e dos backups em repouso: {{MEASURES}}
- Quem pode acessar o host, o banco de dados e a KEK, e como o acesso é revisado: {{MEASURES}}
- Contas da equipe, funções e autenticação de dois fatores: {{POLICY}}
- Atualizações do servidor e do seu host: {{POLICY}}
- Tratamento de uma violação de dados pessoais: {{PROCEDURE AND CONTACT}}
