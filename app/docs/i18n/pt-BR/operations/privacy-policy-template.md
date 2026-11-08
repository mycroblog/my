<!-- source: privacy-policy-template 9966bbc6844c -->
# Modelo de política de privacidade

Uma política de privacidade para você adaptar ao seu servidor: ela descreve o que este servidor guarda, por quê, por quanto tempo e quem recebe os dados.

**Este é um modelo e não constitui aconselhamento jurídico.** Ele descreve o que o software faz com as configurações padrão. Ele não tem como saber como você administra sua instância e não decide quais leis se aplicam a você. O que uma política precisa dizer depende da sua jurisdição e da dos seus usuários (por exemplo, o GDPR na UE, o GDPR do Reino Unido ou a Lei de Proteção de Informações Pessoais do Japão). Peça uma revisão para a sua jurisdição antes de publicá-la e trate as bases legais e a declaração sobre transferências abaixo como questões a resolver, não como respostas.

O registro das atividades de tratamento que acompanha este modelo está em [Modelo de registro das atividades de tratamento](processing-record-template.md).

<!-- source: how-to-use-it 9cb843cc1ba2 -->
## Como usar

1. Copie [o modelo](#the-template) para um arquivo, `privacy-policy.md`.
2. Substitua cada `{{...}}`. Um valor entre as chaves, como `{{180 days}}`, é o padrão do software: confirme-o na sua instância ([Os prazos e suas configurações](#the-periods-and-their-settings)) e remova as chaves.
3. Uma linha ou uma frase que começa com `{{IF ...}}` vale só para aquela configuração ([Os serviços que você escolhe](#the-services-you-choose)). Exclua-a quando não se aplicar. Quando se aplicar, remova o marcador.
4. Verifique se não sobrou nada: `grep -n '{{' privacy-policy.md` não imprime nenhuma linha.
5. Publique-a com `my instance set privacy_policy - < privacy-policy.md`, ou na página “Política de privacidade” de “Termos e políticas” (`/admin/documents/privacy-policy`) na administração. Os leitores a encontram em `/about/privacy` ([configurações da instância](instance-settings.md#terms-and-privacy-policy)).

Uma política de privacidade escrita adiciona uma caixa de concordância obrigatória à tela de cadastro. Essa caixa mostra que alguém leu o documento. Não presuma que ela seja consentimento para o tratamento onde a sua lei exige consentimento.

O texto tem um único valor para todos os idiomas. Se você tem leitores em vários idiomas, escreva os idiomas de que precisa dentro desse único texto.

Revise a política quando mudar uma configuração abaixo ou um serviço, e após cada atualização cujas notas de versão mencionem dados pessoais.

<!-- source: the-periods-and-their-settings 111d5a4b6af8 -->
## Os prazos e suas configurações

`my instance list` mostra todas as configurações da instância e se cada uma está no valor padrão. Uma mudança em um prazo vale a partir da próxima execução do seu job horário. Valores fora do intervalo são recusados.

| Registro | Padrão | Configuração da instância | Intervalo (dias) | Conta a partir de |
| --- | --- | --- | --- | --- |
| Endereço IP e User-Agent do login | 180 dias | `session_origins_retention_days` | 90-3650 | O login |
| Endereço IP do cadastro | 180 dias | `registration_origins_retention_days` | 90-3650 | O cadastro |
| Denúncias | 365 dias | `reports_retention_days` | 180-3650 | A última resolução |
| Ações contra contas e recursos | 365 dias | `moderation_actions_retention_days` | 180-3650 | O fim da ação |
| Registro de auditoria | 730 dias | `audit_log_retention_days` | 365-3650 | O registro |
| Quem participou de uma sala de chat de voz | 90 dias | `voice_participants_retention_days` | 30-3650 | O fim da sala |
| As decisões de uma análise externa, quando você usa uma | 90 dias | `hook_decisions_retention_days` | 30-3650 | A decisão |
| Publicações remotas a que nada aqui se refere | 90 dias | `remote_status_retention_days` | 1-3650 | A publicação do post |
| Qualquer publicação remota | Sem limite (`0`) | `remote_status_max_retention_days` | 0-3650 | A publicação do post |
| Publicações remotas que só um relay trouxe | 7 dias | `relay_status_retention_days` | 1-3650 | A publicação do post |
| Cópias de mídia remota | 1 dia, e nenhuma é guardada enquanto `remote_media_policy` for `none` | `remote_media_cache_ttl_seconds`, `remote_media_policy` | 60-31536000 segundos | A busca |
| Traduções de publicações | 30 dias | `translation_retention_days` | 1-3650 | A tradução. Ler a tradução não prorroga o prazo, e ela some quando a publicação é excluída |

Estas são configurações da implantação ([referência de configuração](../reference/configuration.md)), não configurações da instância:

| Registro | Padrão | Configuração |
| --- | --- | --- |
| Uploads não anexados a nada | 24 horas | `media_unattached_retention` |
| Versões antigas de imagens, depois que uma versão mais nova as substitui | 30 dias | `media_replaced_retention` |
| Jobs que falharam em definitivo | 30 dias | `dead_job_retention` |
| Registros de execuções de jobs que não tiveram sucesso | 30 dias | `job_attempt_retention` |

Estes prazos são fixos no software, e nenhuma configuração os altera:

| Registro | Prazo |
| --- | --- |
| Exclusão definitiva de uma conta excluída | 7 dias após a exclusão; até 30 dias para uma conta remota quando apenas um `410 Gone` do seu servidor indicou isso e o servidor não responde mais |
| Exclusão de uma conta desativada | 30 dias após a desativação |
| Bloqueios de cadastro | 30 dias após a revogação ou a expiração; sem uma nem outra, enquanto estiverem em vigor |
| Sessões, tokens e códigos de autorização | 30 dias após expirarem |
| Arquivos de exportação de dados e importações de dados | 7 dias |
| Registros de atividades recebidas de outros servidores | 30 dias, e o próprio documento 7 dias |
| Registros de atividades entregues a outros servidores | 30 dias; a entrega de uma publicação até 30 dias após a publicação ou o seu autor serem excluídos |
| Verificações de handles remotos | 90 dias, mantendo a mais recente de cada conta |
| Mensagens e entregas de webhook | 14 dias |
| Recibos de webhook de pagamento | 45 dias |
| Contagens horárias de erros e avisos de erro | 90 dias |

Uma [retenção legal](moderation.md#holds) interrompe a exclusão de uma conta, então todos os prazos acima podem durar mais para uma conta sob retenção. Se você usa retenções, diga isso, como o modelo faz.

<!-- source: records-with-no-automatic-end 90a13d1f648e -->
## Registros sem fim automático

O servidor os remove apenas quando uma conta é apagada, ou nunca. Uma lei pode exigir que você justifique mantê-los, ou que os remova por outros meios, então decida o que a sua política diz sobre cada um.

- As linhas de uma conta ativa: endereços de e-mail, versões anteriores do perfil, versões anteriores das publicações, notificações, seguidos, e a solicitação de cadastro de um usuário rejeitado. Uma conta que ninguém exclui permanece.
- O que permanece após a exclusão definitiva: a URI e o nome de usuário da conta (para que nenhum dos dois possa ser reutilizado), seus eventos de ciclo de vida, o registro de que uma publicação foi excluída, os registros de moderação até o fim do seu próprio prazo, convites e atribuições de função, mudanças de conta, salas de chat de voz sem seus nomes e descrições.
- Registros de apoiadores (assinaturas, mudanças de estado, pagamentos, reembolsos, estornos), mantidos para a contabilidade e para a conciliação com o provedor. O servidor não exclui um cliente no provedor de pagamento: faça isso no painel do provedor se as suas obrigações permitirem.
- Uma ação sem fim (um `ban`, ou um `suspend` sem horário de término), até ser revogada. Denúncias abertas.
- Contas remotas, até que o servidor delas as exclua.
- O relatório mais recente de cada grupo de erros, que o próximo relatório desse grupo substitui. Ele é limpo por padrões, então um fragmento de texto de usuário pode permanecer.
- Backups, seus logs e os logs da sua plataforma e do seu proxy. O servidor não gerencia isso: o [backup](backup.md#how-often-and-where) descreve as gerações a manter, e um backup restaurado traz de volta os dados apagados.

<!-- source: the-services-you-choose 81461216c966 -->
## Os serviços que você escolhe

Cada serviço que recebe dados pessoais aparece na política somente se você o ativou. Preencha a tabela da seção 6 a partir desta lista e leia a política e o contrato do próprio serviço: o que o serviço faz com os dados depois que eles chegam não é algo que este documento possa dizer.

| Marcador | Configuração | O que o serviço recebe |
| --- | --- | --- |
| Hospedagem | `database`, `blob`, `queue`, `key_manager` | Tudo o que o banco de dados e o armazenamento de blobs guardam, na forma em que o provedor os armazena. Os payloads de jobs em uma fila na nuvem contêm IDs e categorias, sem endereços, corpos de mensagens ou segredos. |
| `{{IF cdn}}` | `media_origin` | Endereços IP dos leitores e solicitações de mídia ([CDN](cdn.md)). |
| Proxy | Seu proxy reverso ou balanceador de carga, de que o servidor precisa para o TLS | Cada solicitação com seu endereço IP, conforme o registro de logs que você configurar. |
| `{{IF mail}}` | `mail` é `smtp` ou `ses` | Endereços de destinatários, assuntos e corpos. O e-mail de notificação traz o mesmo texto da notificação ([e-mail](email.md)). |
| `{{IF captcha}}` | `captcha` não é `none` | O endereço IP, as informações do navegador e do dispositivo, os cookies e as interações com o widget de todos que abrem a página de login, de cadastro ou de solicitação de redefinição de senha, diretamente do navegador deles. O servidor envia ao serviço apenas o token e não acrescenta nenhum endereço IP ([CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)). |
| `{{IF login providers}}` | [Provedores de login](login-providers.md) | O provedor vê o login. O servidor recebe o identificador da pessoa no provedor, guarda esse identificador e, no cadastro, usa o endereço de e-mail e o nome de usuário que o provedor informa. |
| `{{IF stripe}}` | `payments = stripe` | O endereço de e-mail que preenche previamente o Checkout, o plano e o ID local da assinatura. O Stripe coleta os dados de pagamento em suas próprias páginas ([Stripe](payments-stripe.md#card-data-and-pci-dss)). |
| `{{IF payjp}}` | `payments = payjp` | O endereço de e-mail e o plano. O navegador carrega o payjp.js nas páginas de pagamento do servidor, o cartão vai para o iframe do PAY.JP, e o nome e o endereço de e-mail vão para o PAY.JP para o 3-D Secure ([PAY.JP](payments-payjp.md#card-data-and-pci-dss)). |
| `{{IF error reports}}` | `errors_exporter` | Relatórios limpos, sem conta, endereço IP, corpo ou cabeçalho. A limpeza funciona por padrões, então um fragmento de texto de usuário pode permanecer ([relatório de erros](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)). |
| `{{IF telemetry}}` | `metrics_exporter`, `traces_exporter` | Métricas e spans sem contas nem endereços IP de clientes. Os spans de solicitações de saída trazem os nomes de host de servidores remotos ([execução](running.md)). |
| `{{IF voice media server}}` | `voice` é `livekit` ou `sora` | Os endereços IP dos participantes, o áudio deles enquanto falam, um ID de sessão e o ID da sala. O ID da conta não é enviado ([chat de voz](voice.md)). |
| `{{IF turn}}` | `voice_stun_urls`, `voice_turn` | Os servidores STUN e TURN veem os endereços IP dos participantes, e um servidor TURN retransmite o áudio criptografado deles. |
| `{{IF webhooks}}` | [Webhooks do servidor](webhooks.md) | Seu receptor recebe os corpos dos eventos. Alguns trazem o endereço de e-mail de um candidato ou o comentário de uma denúncia. |
| `{{IF follow review}}` | `plugin_inbound_follow_address` está definido e `hook_inbound_follow_mode` não é `off` | Os endereços das duas contas de cada pedido de seguir vindo de outro servidor, enviados pelo servidor quando o pedido chega ([hooks de decisão](decision-hooks.md)). A decisão registrada guarda o endereço da atividade e qual conta a enviou. |
| `{{IF sign-up review}}` | `plugin_signup_check_address` está definido e `hook_signup_check_mode` não é `off` | O endereço de e-mail e o endereço IP de cada candidato sem convite, enviados pelo servidor quando ele se cadastra ([hooks de decisão](decision-hooks.md#sign-ups)). A decisão registrada não guarda nenhum dos dois. Nomeie o serviço na seção 6, a menos que você mesmo execute o plugin. |
| `{{IF post review}}` | `plugin_local_status_address` está definido e `hook_local_status_mode` não é `off` | O texto de cada publicação `public`, `local` e `unlisted` de uma conta com menos de `hook_local_status_account_age_days` de existência (7 dias por padrão; `0` é toda conta), como digitado e como renderizado, com seu aviso de conteúdo, idioma, se está marcada como sensível, opções de enquete, descrições de imagens, hashtags, quantas contas ela menciona, o endereço que ela receberia, os endereços das publicações a que responde e que cita, o endereço e o nome de usuário do autor e quando a conta foi criada, enviados pelo servidor quando a publicação é feita e novamente a cada edição ([hooks de decisão](decision-hooks.md#local-posts)). Publicações `private` e `direct` nunca são enviadas. A decisão registrada não contém texto. Um autor que recorre de uma publicação recusada envia o texto dela, que o servidor guarda com a decisão. Nomeie o serviço na seção 6, a menos que você mesmo execute o plugin. |
| `{{IF profile review}}` | `plugin_inbound_actor_address` está definido e `hook_inbound_actor_mode` não é `off` | Os perfis públicos de contas em outros servidores, cada vez que um muda: o nome de exibição, a bio, os campos do perfil, os endereços do avatar e da imagem de cabeçalho (sem imagens), o endereço, o servidor e o nome de usuário da conta, e se a conta é nova aqui ([hooks de decisão](decision-hooks.md#profiles-from-other-servers)). Nada das contas do seu servidor é enviado. A decisão registrada guarda o endereço da conta e um resumo (digest) do perfil. |
| `{{IF profile review}}` | `plugin_local_profile_address` está definido e `hook_local_profile_mode` não é `off` | O nome de exibição, a bio como digitada e como renderizada, os campos do perfil, se o avatar ou o cabeçalho mudou, o endereço e o nome de usuário da conta e quando a conta foi criada, enviados pelo servidor cada vez que uma conta com menos de `hook_local_profile_account_age_days` de existência (7 dias por padrão; `0` é toda conta) os altera ([hooks de decisão](decision-hooks.md#local-profiles)). Nenhuma imagem é enviada. A decisão registrada não guarda nada disso. Nomeie o serviço na seção 6, a menos que você mesmo execute o plugin. |
| `{{IF external review of posts}}` | `plugin_inbound_status_address` está definido e `hook_inbound_status_mode` não é `off` | As publicações e revisões que outros servidores enviam e que se relacionam a uma conta daqui, inclusive publicações só para seguidores (`private`): o texto em cada um de seus idiomas, o aviso de conteúdo, se está marcada como sensível, quando foi publicada, seu endereço e URL, o endereço da publicação a que responde, opções de enquete, hashtags, endereços e descrições de anexos (sem mídia), seu autor e o servidor do autor, sua visibilidade, quantas contas ela cita e quantas delas estão aqui, e se alguém daqui segue o autor ([hooks de decisão](decision-hooks.md#what-is-sent-and-what-it-costs)). As publicações diretas só vão quando alguém daqui segue o autor, ou com `hook_inbound_status_unfollowed_direct` de qualquer autor. |
| `{{IF media review}}` | `plugin_media_check_address` está definido e `hook_media_check_mode` não é `off` | A menor imagem processada de cada upload, sem metadados, nunca o original, com a finalidade do arquivo e seu tipo, tamanho e duração ([hooks de decisão](decision-hooks.md#what-mediacheck-reviews)). A decisão registrada guarda o ID do arquivo e quem o enviou. Um upload rejeitado não é guardado de forma alguma: se você precisar preservar ou denunciar um, o seu plugin precisa fazê-lo. Nomeie o serviço na seção 6, a menos que você mesmo execute o plugin. |
| `{{IF translation}}` | `translation_providers` não está vazio | O texto de uma publicação quando um leitor pede para traduzi-la: o corpo, o aviso de conteúdo, as opções de enquete e as descrições de imagens. O endereço da publicação, o autor, o leitor, as menções e os links não são enviados. Por padrão, um provedor recebe publicações `public`, `local` e `unlisted`. Ele atua como o seu prestador de serviços, como a hospedagem, então as publicações `local` chegam a ele embora não sejam federadas: mantenha a frase sobre `local` na seção 5. Um provedor que guarda o texto para uso próprio não é um prestador de serviços nesse sentido, e a política precisa dizer isso. |
| `{{IF translation}}` com o plano gratuito do DeepL | `translation_providers` inclui `deepl` com uma chave terminada em `:fx` | O mesmo. O DeepL pode guardar o texto e as traduções para sempre, e a política precisa dizer isso ([tradução](translation.md#deepl)). |
| `{{IF translation}}` com o Amazon Translate | `translation_providers` inclui `aws` | O mesmo. A menos que a política de exclusão (opt-out) dos serviços de IA do AWS Organizations cubra a sua conta, a Amazon pode armazenar o texto e usá-lo para melhorar seus serviços, possivelmente em outra região, e a política precisa dizer isso ([tradução](translation.md#amazon-translate)). |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` permite a um provedor `private` ou `direct` | O mesmo, para essas publicações. Use este marcador somente quando você tiver permitido isso, por provedor. Ele acrescenta linhas às seções 2 e 5 e amplia a sua promessa sobre essas publicações, então anuncie antes de mudar a configuração. |
| `{{IF translation of reports}}` | `translation_reports_to_server` | O comentário de uma denúncia e as publicações que ela cita, quando um moderador pede para traduzi-los. Cada provedor ainda recebe somente as visibilidades que lhe são permitidas, e o comentário conta como `private`: sem nenhum provedor autorizado a receber `private`, os comentários não chegam a nenhum. A tradução do comentário não é guardada. |
| `{{IF browser translation}}` | `translation_browser` | Nada chega a este servidor nem a um provedor. O navegador traduz no dispositivo do leitor, e o que o fabricante do navegador faz depende desse navegador. |

Os serviços são escolhas suas, então este modelo não diz onde cada um está estabelecido nem qual contrato o cobre. Confirme as duas coisas por conta própria.

Duas coisas chegam a terceiros sem uma configuração sua:

- **Web Push.** Quando um usuário o ativa, o navegador dele escolhe o serviço de push e informa o endereço dele ao servidor. O corpo da mensagem é criptografado para esse navegador, então o serviço de push não deveria conseguir lê-lo. Ele vê o endereço da assinatura, o tamanho e a hora de cada mensagem e o contato (assunto VAPID) do seu servidor, e retém uma mensagem por até 48 horas enquanto o navegador está offline.
- **Federação.** Veja a seção 5 do modelo. `federation_mode` a desativa ou limita a sua direção, e os relays ficam por sua conta ([relays](relays.md)).

<!-- source: the-template 8b6bd2fc983a -->
## O modelo

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, your posts that relate to an account here, followers-only posts included, go with their text to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. {{IF you set a maximum for keeping posts}} At most {{N days}} after you publish them, unless your account may keep them longer. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |
| {{IF translation of reports}} A report's comment and the posts it names | {{TRANSLATION SERVICES YOU ALLOWED `private` POSTS}} receive the comment, and the services in section 6 receive the posts as for a reader, when a moderator asks for a translation. They do not receive who reported, the report, or the posts' addresses |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts and their edits for abuse | Your `public`, `local`, and `unlisted` posts, each time you post or edit one, while your account is younger than {{7 days}}: their text, content warning, language, whether they are marked sensitive, poll options, image descriptions, hashtags, and how many accounts they mention, their address and the addresses of the posts they reply to or quote, your address and username, and when your account was created. Not your media | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening the profiles of accounts on other servers for abuse | Nothing about your account on this server. The public profile of an account on another server when this server takes in a change of it: its display name, bio, profile fields, the addresses of its avatar and header image, its address and username | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening changes of profiles for abuse | Your display name, bio, and profile fields, and whether you changed your avatar or header, each time you change them while your account is younger than {{7 days}}, with your address and username and when your account was created. Not your images | {{COUNTRY}} | {{LINK}} |
| {{IF media review}} {{MEDIA REVIEW SERVICE}} | Screening uploads for abuse before anyone can see them | A small processed copy of each image you upload, or of the poster or cover of a video or audio file, without its metadata, and what the file is for. Not the original, its description, or your account | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it, followers-only posts included: its text and language, content warning, whether it is marked sensitive, when it was published, its address, the address of the post it replies to, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, how many accounts it names and how many of them are here, and whether anyone here follows its author. Direct posts {{only when someone here follows their author, or from any author if you turned on `hook_inbound_status_unfollowed_direct`}}. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, an upload that an external review rejects is refused, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, {{IF profile review}} a change of your profile that an external review refuses, which you can change and save again, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
