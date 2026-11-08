<!-- source: sending-email 941ecea58cd9 -->
# Envio de e-mail

Quais e-mails o servidor envia, como configurar o envio e como lidar com falhas e com o cancelamento de inscrição.
As configurações estão listadas na [referência de configuração](../reference/configuration.md#mail).
Os fluxos de confirmação de e-mail e de redefinição de senha, do ponto de vista do usuário, estão em [cadastro e confirmação de e-mail](registration.md).

<!-- source: what-is-sent-and-how-to-configure-it 3f387cd1afcc -->
## O que é enviado e como configurar

O servidor nunca envia e-mail dentro de uma requisição. Ele enfileira um job para cada finalidade, na mesma transação do fato
que dispara o e-mail, e um worker de jobs faz o envio. Uma falha no envio não aparece na resposta de um cadastro ou de uma
redefinição: o usuário é informado de que “enviamos um e-mail de confirmação” e pede o e-mail de novo se ele não chegar.

| Classe | E-mail | Tipo de job | O usuário pode cancelar a inscrição? |
| --- | --- | --- | --- |
| Transacional (`transactional`) | Confirmação do endereço de e-mail | `identity.verification_email` | Não |
| Transacional | Redefinição de senha | `identity.password_reset_email` | Não |
| Transacional | Aviso de alteração ou exclusão de senha | `identity.password_notice_email` | Não |
| Transacional | Aviso de desativação da conta, com a data de sua exclusão | `identity.deactivation_email` | Não |
| Transacional | Encerramento do serviço (o anúncio, uma mudança de data, um cancelamento, 7 dias e 1 dia antes da data de encerramento, o encerramento e 7 dias antes do prazo de exportação) | `identity.closure_email` | Não |
| Notificação (`notification`) | Aviso aos moderadores (ativado por padrão, por tipo de aviso) | `notification.moderator_email` | Sim |
| Notificação (`notification`) | Uma notificação que ficou não lida por 15 minutos: menções, novos seguidores, impulsos e as demais notificações de outros usuários, e avisos dos moderadores (desativado por padrão, por tipo) | `notification.email` | Sim |

Os e-mails de notificação para apoiadores (`notification.supporter_email`) passam pela mesma fila de jobs.

<!-- source: which-notifications-are-emailed 76c2b871762c -->
### Quais notificações são enviadas por e-mail

Cada usuário escolhe, por tipo de notificação, se ela chega no app e por e-mail, na página **Notificações** das
configurações (`/settings/notifications` no cliente web). A coluna E-mail só aparece ali enquanto o servidor envia
e-mail. Um tipo desativado no app nem chega a ser criado, portanto também não é enviado por e-mail nem por push.

| Tipo | E-mail por padrão | Quando é enviado |
| --- | --- | --- |
| Notificações de outros usuários (menções, novos seguidores, solicitações para seguir, solicitações para seguir aceitas, impulsos, favoritos, reações com emoji, citações, enquetes encerradas, contas que você segue mudando de servidor, chats de voz) | Desativado | 15 minutos após a notificação, somente se ela ainda estiver não lida (não marcada como lida, não dispensada) e ainda visível, e se o usuário ainda recebe esse tipo por e-mail. Um e-mail por notificação |
| Avisos dos moderadores (uma ação de moderação na conta, uma denúncia resolvida, um recurso decidido, seguidos perdidos por causa de uma suspensão, uma solicitação de emoji personalizado decidida) | Desativado | O mesmo que acima |
| Avisos aos moderadores (novas denúncias, cadastros aguardando aprovação, recursos, webhooks desativados, solicitações de emoji personalizado, contas limitadas mudando de servidor, a data de encerramento chegando, erros novos e recorrentes), para as pessoas cuja função tem a permissão | Ativado | Imediatamente, sem esperar a leitura |
| Encerramento do serviço e avisos dos planos de apoiador | Sempre | Imediatamente; não podem ser desativados |

Os e-mails de notificação vão apenas para o endereço principal do usuário, depois de confirmado, e nunca para um endereço
cujos e-mails estejam interrompidos (veja abaixo). O assunto do e-mail de uma notificação de outro usuário é o mesmo texto da
mensagem de Web Push, e o e-mail contém um link para a publicação, o perfil ou as notificações.

Os e-mails de encerramento do serviço vão para todos os usuários cujo endereço principal esteja confirmado. Perder um deles pode
significar perder dados, então eles são enviados como e-mail transacional, não trazem link de cancelamento de inscrição e não
são interrompidos por uma reclamação (um hard bounce os interrompe).
O anúncio, uma mudança de data, um cancelamento e o encerramento enfileiram cada um `identity.closure_email_fanout` na mesma
transação do seu registro. O job horário `identity.queue_closure_reminders` decide quando enviar os lembretes de 7 dias e
1 dia antes da data de encerramento e de 7 dias antes do prazo de exportação. Um lembrete cujo momento já havia passado quando o
encerramento foi anunciado não é enviado, porque o e-mail do anúncio informa as datas.
O job de fan-out registra uma linha de `service_closure_emails` por usuário e enfileira `identity.closure_email` apenas para os
usuários que conseguiu registrar; assim, por mais vezes que o job rode, cada usuário recebe cada e-mail uma só vez. Antes de
enviar, o job verifica se o e-mail ainda é válido (se não se refere a um anúncio cancelado ou a uma data que já mudou).

A configuração `mail` escolhe como o e-mail é enviado.

| `mail` | Comportamento |
| --- | --- |
| `none` (padrão) | Não envia nada e escreve `mail not sent (mail = none)` no log do servidor com a categoria da mensagem. Com `profile = development`, a linha também traz os endereços, o assunto e o corpo, links incluídos. Para configurações que não usam e-mail e para desenvolvimento |
| `smtp` | Envia por submissão SMTP. Exige STARTTLS e verifica o certificado antes de autenticar e enviar |
| `ses` | Envia com a API Amazon SES v2 (`SendEmail` com conteúdo bruto). Autentica pela cadeia de credenciais padrão do AWS SDK (uma função do IAM, por exemplo) e não guarda senha SMTP |
| `plugin` | Entrega cada mensagem a um [plugin](#a-plugin-mail--plugin) que você executa, para um serviço de e-mail que oferece apenas uma API HTTP |

As configurações de `smtp` são `mail_from`, `mail_smtp_host`, `mail_smtp_port` (padrão 587), `mail_smtp_username` e `mail_smtp_password`. As
configurações de `ses` são `mail_from`, `mail_ses_region` e `mail_ses_configuration_set`. As configurações de `plugin` são `mail_from`
e `plugin_mailer_address`, com o restante do [seu destino](plugins.md#the-destination).
`mail_from` é um endereço simples, como `noreply@social.example`. `From` vira `"<server name>" <mail_from>`, e o domínio
de `mail_from` também é o domínio de `Message-ID` e aquele ao qual SPF, DKIM e DMARC se aplicam (veja abaixo).
`Reply-To` é a configuração da instância `contact_email`, quando definida ([configurações da instância](instance-settings.md)).
Todo e-mail leva `Auto-Submitted: auto-generated`.
`smtp` e `ses` montam a mesma mensagem. SMTPUTF8 não é usado, então os endereços são enviados em ASCII e domínios
internacionalizados são convertidos para Punycode. Não é possível enviar a um endereço cuja parte local (antes do `@`) não seja ASCII, e o seu job falha.

<!-- source: examples-by-provider 7ab129a02c43 -->
## Exemplos por provedor

Passe os segredos por variável de ambiente ou arquivo (`MY_MAIL_SMTP_PASSWORD` ou `MY_MAIL_SMTP_PASSWORD_FILE`). Não os coloque
no arquivo de configuração.

<!-- source: amazon-ses-api 1cef4371e61c -->
### Amazon SES (API)

Ao executar na AWS, `mail = ses` usa a API SES v2. As credenciais vêm da cadeia padrão do AWS SDK (a task role do
ECS, variáveis de ambiente, o arquivo de configuração compartilhado), então não há segredo a configurar.

1. No SES, verifique o domínio de `mail_from` como uma identidade e ative o Easy DKIM (veja abaixo).
2. Solicite acesso de produção para sair do sandbox. No sandbox você só pode enviar para endereços verificados.
3. Permita `ses:SendEmail` para a função de execução na identidade de `mail_from` (e no configuration set, se você usar um).

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

Quando `mail_ses_configuration_set` está definido, todo e-mail usa esse configuration set. O SES sobrescreve o `Message-ID` e o `Date` do
e-mail com valores próprios.
O SES nunca rejeita um destinatário de forma síncrona no momento do envio. Ele aceita o envio para um endereço da lista de supressão da conta e
depois não o entrega, e o bounce chega mais tarde como uma notificação (SNS).
Para registrar bounces e reclamações, configure as notificações do SES abaixo para chegarem por SNS.

<!-- source: a-plugin-mail--plugin 65b46feaf1cf -->
### Um plugin (`mail = plugin`)

Para um serviço de e-mail que oferece apenas uma API HTTP, execute um [plugin](plugins.md) do tipo `mailer` e deixe que ele chame o serviço.

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

`plugin_mailer_address` é obrigatório com este valor. Uma mensagem pode conter um segredo, como um link de verificação, então o tipo
recusa `http://`, inclusive em loopback: use um socket Unix ou `https://`. O servidor inicia sem chamar o plugin, mesmo que ele
esteja fora do ar, e o e-mail espera em seu job.

Com `plugin_mailer_protocol = json`, o padrão, um plugin precisa apenas de um servidor HTTP e de uma biblioteca JSON
([Um plugin sem biblioteca](plugins.md#a-plugin-with-no-library)). `Send` dura no máximo 30 segundos, ou `plugin_mailer_timeout`,
abaixo de 5 minutos. Uma requisição tem no máximo 2 MiB e uma resposta no máximo 64 KiB.

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

O servidor verifica a mensagem antes de chamar o plugin, então nenhum cabeçalho pode ser injetado por esses campos. A requisição
contém as partes da mensagem, e o plugin monta o que o seu serviço precisa:

- **O plugin adiciona três cabeçalhos:** um `Message-ID` único por mensagem, no domínio de `envelopeFrom`; um `Date`; e
  `Auto-Submitted: auto-generated`, que evita que respostas automáticas entrem em loop. Um serviço que escreve o próprio `Message-ID` e
  `Date` cobre esses dois.
- `From` é `fromName` e `envelopeFrom`. `fromName`, `replyTo`, `html` e `headers` podem estar ausentes. `headers` contém apenas os
  dois cabeçalhos `List-Unsubscribe`.
- Os endereços são UTF-8. Um plugin cujo serviço precisa de ASCII converte o domínio para Punycode e responde `OUTCOME_INVALID` para uma
  parte local que não consiga enviar.
- A mesma mensagem pode chegar duas vezes, já que um job que falha é repetido. Não escreva uma mensagem em um log.

`outcome` diz o que aconteceu com a mensagem:

| `outcome` | Significado | O servidor |
| --- | --- | --- |
| `OUTCOME_SENT` | O serviço aceitou a mensagem | Conclui o job |
| `OUTCOME_INVALID` | Ela não pode ser enviada como está | Falha o job imediatamente |
| `OUTCOME_RECIPIENT_REJECTED` | O endereço não existe | Falha o job imediatamente e [suprime o endereço](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | O serviço a recusou em definitivo, por outro motivo | Falha o job imediatamente |
| `OUTCOME_TEMPORARY` | O serviço a recusou por enquanto, ou não respondeu | Tenta o job de novo |
| Qualquer outro valor, ou nenhum | | Tenta o job de novo |

Um erro HTTP, ou o `{"code":"unavailable"}` do Connect com o status correspondente, também é uma falha temporária.
`enhancedStatus` é opcional: o código de status avançado de uma recusa, como `5.1.1`. O servidor guarda um código de classe 5 de uma
rejeição e de classe 4 de `OUTCOME_TEMPORARY`, e ignora o resto. Ele não guarda nenhum texto de um plugin, então não coloque motivo ali:
o motivo pertence ao log do próprio plugin.

O servidor só toma conhecimento de um bounce por essa resposta. Ele não tem endpoint para os bounces e reclamações posteriores do serviço de um plugin,
então um bounce que o serviço informe depois não suprime o endereço.

Teste o plugin com o binário `my`:

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

O primeiro só verifica se o plugin responde e não envia nada. O segundo envia uma mensagem real para `--mail-to` e envia ao
plugin uma requisição sem destinatário, que ele deve recusar. O servidor não consegue ver a mensagem depois que o plugin a recebe, então olhe
a que chegar: seus cabeçalhos devem conter `Message-ID`, `Date` e `Auto-Submitted: auto-generated`.
Veja [Testar um plugin](plugins.md#test-a-plugin).

<!-- source: receive-ses-notifications-sns e21b650ab5fe -->
### Receber notificações do SES (SNS)

Os bounces e reclamações do SES vão para um tópico SNS, e uma assinatura HTTPS nesse tópico os entrega ao
`POST /mail/events/ses` do servidor.
O endpoint só abre quando `mail_ses_sns_topic_arn` está definido; caso contrário, responde `404`.

1. Crie um tópico SNS padrão na mesma região da identidade do SES.
2. Defina o atributo `SignatureVersion` do tópico como `2` (SHA256withRSA). Uma assinatura com o padrão `1` não é aceita e o
   servidor responde `400`.
3. Na política de acesso do tópico, permita que `ses.amazonaws.com` execute `SNS:Publish` a partir da sua própria conta (o console do SES adiciona isso
   quando cria o tópico).
4. Aponte as notificações do SES para o tópico, de qualquer uma das duas formas (o servidor lê tanto `notificationType` quanto `eventType` no corpo).
   - Notificações de identidade: use este tópico para as notificações de feedback “Bounce” e “Complaint” da identidade `mail_from`.
   - Destino de eventos do configuration set: adicione este tópico como destino do configuration set em `mail_ses_configuration_set`,
     para os tipos de evento `BOUNCE` e `COMPLAINT`.
5. Defina `MY_MAIL_SES_SNS_TOPIC_ARN` com o ARN do tópico, verifique com `my config check` e reinicie.
6. Crie uma assinatura no tópico com o protocolo `https` e o endpoint `https://<server host>/mail/events/ses`.
   **Não ative a entrega de mensagem bruta** (um corpo sem assinatura é rejeitado).

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

Quando o servidor recebe a confirmação da assinatura (`SubscriptionConfirmation`), ele envia um `GET` ao seu `SubscribeURL` para
confirmá-la e registra no log `SNS subscription of the SES notifications confirmed`.
Verifique com `aws sns list-subscriptions-by-topic` que a assinatura não está mais em `PendingConfirmation`.

Antes de usar uma mensagem, o endpoint verifica tudo o que segue. Se algo falhar, ele responde `400` e não registra nada.

- `TopicArn` é igual a `mail_ses_sns_topic_arn`.
- `SignatureVersion` é `2` e a assinatura é válida.
- A URL do certificado da assinatura (`SigningCertURL`) é `https` e seu host é `sns.<region>.amazonaws.com` para a região
  do tópico. O `SubscribeURL` de uma confirmação de assinatura deve ter o mesmo host.

O certificado é obtido com o cliente HTTP de saída que protege contra SSRF e fica em cache no armazenamento KV por 24 horas. O
corpo é lido até 1 MiB, e um maior recebe `413`.
Quando o certificado não pode ser obtido, a confirmação da assinatura falha ou o registro não pode ser gravado, o endpoint
responde `503` e deixa o SNS tentar de novo. Ele também responde `503` durante o modo de manutenção.

Estas são as notificações que ele registra. Ele compara o endereço do destinatário com o endereço principal (sem diferenciar maiúsculas de minúsculas, e um
domínio Punycode também corresponde na forma Unicode); se nenhum corresponder, ele não faz nada.

| Notificação | Registrada como |
| --- | --- |
| Um bounce cujo `bounceType` é `Permanent` (qualquer subtipo, como `General`, `NoEmail`, `Suppressed` ou `OnAccountSuppressionList`) | `hard_bounce` |
| Uma reclamação (exceto aquelas cujo `complaintFeedbackType` é `not-spam`) | `complaint` |
| Um bounce `Transient` ou `Undetermined`, e outras notificações, como entrega ou envio | Não registrada (responde `200`) |

O `feedbackId` do SES é a chave única de um registro, então, por mais vezes que o SNS envie a mesma notificação, há um só registro.

<!-- source: amazon-ses-smtp-interface f46a7a562966 -->
### Amazon SES (interface SMTP)

Quando você não pode fornecer credenciais do IAM, como ao executar fora da AWS, use a interface SMTP do SES com `mail = smtp`. Nessa
configuração, apenas a rejeição do destinatário no momento do envio é registrada como supressão
([supressão de endereços que não podem receber e-mail](#suppression-of-addresses-that-cannot-receive-email)).

1. No SES, verifique o domínio de `mail_from` como uma identidade e ative o Easy DKIM (veja abaixo).
2. Solicite acesso de produção para sair do sandbox. No sandbox você só pode enviar para endereços verificados.
3. Crie credenciais SMTP em “SMTP settings” do SES (elas são diferentes das chaves de acesso do IAM).

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

O SES sobrescreve `Message-ID` e `Date` com valores próprios.

<!-- source: a-general-smtp-submission-server 62921988a793 -->
### Um servidor de submissão SMTP genérico

Use para o seu próprio servidor SMTP, como o Postfix, ou para o servidor de submissão de um provedor de entrega de e-mail.

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

O servidor não envia a um servidor que não ofereça STARTTLS, nem a um cujo certificado não possa ser verificado para o nome em
`mail_smtp_host`. As credenciais só são enviadas depois do STARTTLS.
Depois de alterar as configurações, verifique com `my config check` e reinicie.

<!-- source: spf-dkim-and-dmarc d6b36a4f4d74 -->
## SPF, DKIM e DMARC

O Gmail e o Yahoo exigem SPF, DKIM, DMARC e cancelamento de inscrição com um clique de remetentes que enviam muitos e-mails por dia. Mesmo para um
volume pequeno, e-mail sem eles tem mais chance de ser classificado como spam.
Configure todos no DNS do domínio de `mail_from` (por exemplo, `social.example`), com os valores que o seu provedor fornece.

- **SPF**: crie um registro TXT no domínio que autorize o provedor pelo qual você envia (por exemplo, se você usa um domínio MAIL
  FROM personalizado no SES, o `include:` que o provedor especifica, nesse domínio).
- **DKIM**: coloque no DNS a chave pública emitida pelo provedor e ative a assinatura no lado do envio. No SES, são os três
  registros CNAME do Easy DKIM. O servidor em si não assina, então quem assina é o provedor.
- **DMARC**: um registro TXT em `_dmarc.social.example`. **Comece com `p=none`**, confirme com os relatórios agregados (`rua=`) que SPF
  e DKIM estão alinhados (coincidem com o domínio de From) e, depois, endureça para `quarantine` e `reject`.

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

Para alinhar o SPF no SES, defina um domínio MAIL FROM personalizado (o MAIL FROM padrão é `amazonses.com`, que não coincide com o domínio
de From). Se o DKIM estiver alinhado, o DMARC passa.

<!-- source: look-at-send-failures ab9c4f356d3b -->
## Verificar falhas de envio

Veja as falhas de envio na página de jobs (`/admin/jobs/list`, permissão `manage_jobs`; veja
[a fila de jobs e os jobs com falha](running.md#the-job-queue-and-failed-dead-jobs)).
Filtre pelo tipo de job para ver apenas os jobs de e-mail (na API de administração, `GET /api/v1/admin/jobs?type=identity.verification_email`).
Os tipos estão na tabela acima.

- **Falhas temporárias** (erros de conexão, timeouts, respostas 4xx) são repetidas. O e-mail transacional é tentado 6 vezes (um
  backoff exponencial a partir de 30 segundos, cerca de 15 minutos no total); o e-mail de notificação usa a política padrão (10 vezes, cerca de
  4 horas). Quando as tentativas se esgotam, o job se torna um job com falha (dead job). Um `mail_smtp_host` errado, uma falha de
  autenticação e uma rede bloqueada aparecem aqui. Depois de corrigir a causa, execute-o de novo com `my job retry <id>` ou na
  tela de jobs.
- **Rejeições permanentes** (um 5xx para o destinatário ou o conteúdo) não são repetidas e falham imediatamente.
- Com `mail = ses`, o `MessageRejected` do SES (o conteúdo foi rejeitado) é uma rejeição permanente. `TooManyRequestsException`
  (o limite de taxa de envio), `SendingPausedException`, `MailFromDomainNotVerifiedException`, `AccountSuspendedException` e
  requisições que não chegam ao SES são falhas temporárias, e uma nova tentativa funciona assim que você corrige a conta ou as permissões.
  Apenas o código de erro do SES é mantido no texto do erro.
- Com `mail = plugin`, o [`outcome`](#a-plugin-mail--plugin) da resposta decide. Um plugin que não pode ser alcançado, ou que
  responde com erro, é uma falha temporária e conta em [`my.plugin.failures`](running.md). Apenas o outcome e o código de status
  avançado são mantidos no texto do erro, então o motivo de uma falha está no log do plugin.
- Um e-mail transacional mais antigo que um limite de tempo desde a sua solicitação (15 minutos para uma redefinição, 1 hora para uma confirmação) é
  concluído sem ser enviado. Isso evita que links antigos cheguem atrasados; não é uma falha.
- Quando o job roda e não há motivo para enviar (o endereço está suprimido, foi confirmado nesse meio-tempo, ou o usuário
  perdeu a permissão ou a configuração da notificação), ele é concluído sem enviar.

O payload de um job não contém endereço de e-mail, corpo nem token, e o texto de erro também não. Restam apenas IDs, o tipo do job e o código de
resposta SMTP.
Um envio bem-sucedido pode ser acompanhado na linha de log estruturada `job` de cada execução (`my.job.type`).

<!-- source: suppression-of-addresses-that-cannot-receive-email 18b4750f1531 -->
## Supressão de endereços que não podem receber e-mail

Quando o servidor SMTP diz que o endereço do destinatário não existe (um 5xx em `RCPT TO` com status estendido `5.1.x`), o
servidor registra o endereço como hard bounce e para de enviar para ele.
Com `mail = plugin`, `OUTCOME_RECIPIENT_REJECTED` registra o mesmo hard bounce, e nada mais o faz.
Com `mail = smtp`, apenas essa rejeição no momento do envio é registrada. Bounces assíncronos que voltam depois que o servidor SMTP
aceitou o e-mail, e feedback loops de reclamações, não são recebidos.
Com `mail = ses`, o SES nunca rejeita um destinatário de forma síncrona, então nada é registrado no momento do envio; bounces permanentes e
reclamações são registrados a partir das notificações do SES
([Receber notificações do SES (SNS)](#receive-ses-notifications-sns)).

| Tipo | E-mail interrompido |
| --- | --- |
| `hard_bounce` | Tudo, exceto o e-mail de confirmação. O e-mail de redefinição de senha também é interrompido |
| `complaint` (denunciado como spam) | E-mails de notificação e de anúncio. Os e-mails transacionais continuam sendo enviados |

- Os moderadores veem o tipo e a origem de uma supressão na página de gerenciamento da conta nas telas de administração (`email_suppression` em
  `GET /api/v1/admin/accounts/{id}`).
- Há duas formas de **revogar** a supressão. Quando o usuário pede um e-mail de confirmação e confirma o endereço de novo, a supressão
  por hard bounce é revogada (o e-mail de confirmação é enviado mesmo enquanto o endereço está suprimido). O usuário o solicita pela
  API de cliente (`POST /api/v1/accounts/email/verification`, no máximo 3 por hora) ou na página de status do
  login. Quando você verificou que
  é seguro enviar, por exemplo porque a caixa de correio foi corrigida, um moderador a revoga com
  `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permissão `manage_accounts`). Isso registra
  `email_suppression_lifted` no registro de auditoria ([moderação](moderation.md)).
- Para um usuário cujo e-mail de redefinição de senha está interrompido, um administrador emite um link de redefinição e o entrega
  (`my admin account password-reset-link @alice`).
- Na tela de configurações, os usuários veem que o e-mail não está chegando ao seu endereço principal e desde quando.

<!-- source: unsubscribing 2806dbccc409 -->
## Cancelamento de inscrição

Os e-mails de notificação trazem um link de cancelamento de inscrição e os cabeçalhos `List-Unsubscribe` e `List-Unsubscribe-Post:
List-Unsubscribe=One-Click` (RFC 8058). O botão “cancelar inscrição” dos clientes de e-mail os utiliza.

- As classes de cancelamento correspondem uma a uma às linhas das configurações de notificação cujo e-mail o usuário pode escolher, como
  `mention` ou `admin_report` (uma por tipo de aviso aos moderadores, e `moderation` para os avisos dos moderadores).
- O link é `https://{host}/mail/unsubscribe?token=...` e não exige login. Um token é emitido por e-mail, apenas o seu hash é
  guardado no banco de dados, e ele expira em 90 dias.
  - `GET` apenas mostra uma tela de confirmação e não altera nada (para que scanners que pré-carregam links não interrompam o e-mail).
    O botão envia o mesmo `POST`.
  - `POST` (um clique) desativa o e-mail daquela linha das configurações de notificação, e nada mais. Até o token
    expirar, enviá-lo de novo dá o mesmo resultado.
- **O e-mail transacional não permite cancelar a inscrição.** Confirmação, redefinição e o aviso de alteração de senha
  impediriam que avisos relevantes para a segurança chegassem. Esses e-mails também não trazem esses cabeçalhos.
- Os usuários podem alterar a mesma configuração na página **Notificações** das configurações.

<!-- source: the-html-version 2ca008b1a48a -->
## A versão HTML

Todo e-mail é `multipart/alternative`, com o mesmo conteúdo em `text/plain` e `text/html` (ambos UTF-8). Com `mail = none`
no perfil `development`, apenas a versão em texto é escrita no log.

- Há um único layout externo, com parágrafos, botões e uma assinatura (nome do servidor, URL, contato, o motivo do e-mail
  e o link de cancelamento de inscrição). `<html lang dir>` vem do idioma do destinatário, e idiomas como `ar` são dispostos da direita
  para a esquerda.
- Ele não carrega fontes nem folhas de estilo externas. As cores são os design tokens padrão, e claro e escuro seguem
  `color-scheme` e `prefers-color-scheme`. As personalizações de tema do administrador não afetam o e-mail.
- Ele não carrega imagens, exceto o logotipo da própria instância, quando definido ([logotipo e ícone](instance-settings.md#logo-and-icon)):
  a versão HTML então começa com o logotipo (`https://<host>/brand/...`, o logotipo escuro no lugar dele quando o cliente de e-mail segue
  um esquema de cores escuro), com a mesma URL para todos os destinatários, de modo que não revela quem abriu o e-mail. Clientes que não carregam
  imagens externas mostram o nome do servidor (o texto alternativo da imagem). Sem logotipo, ou quando a URL do servidor não é `https`,
  o e-mail não tem imagem.
- Apenas URLs `https` viram links. Quando uma URL é `http`, como em um ambiente de desenvolvimento em que `web_origin` é `http://{host}`, a URL
  é escrita como texto em vez de um botão ou de um link de cancelamento de inscrição, e a URL do servidor na assinatura é omitida.
- O layout faz parte do binário, então nenhum JavaScript é executado no servidor em tempo de execução.
