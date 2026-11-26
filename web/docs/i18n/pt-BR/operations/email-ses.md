<!-- source: sending-email-through-amazon-ses 6deee2054246 -->
# Enviar e-mail pelo Amazon SES

Como preparar o Amazon SES para enviar o e-mail do servidor: verificar o domínio do remetente com registros DNS, sair do sandbox
em que uma conta nova começa e manter a conta em boa situação. O módulo do OpenTofu [`mycroblog-aws`](deploy-aws.md#email-through-amazon-ses) envia pelo SES por
padrão e cria a identidade e a permissão; os registros e o pedido de acesso à produção são passos seus.
As configurações em si estão em [Enviar e-mail](email.md#amazon-ses-api).

**A implantação na AWS nunca foi aplicada**, então estes passos vêm da documentação da AWS e não foram seguidos de ponta a
ponta em um servidor deste projeto.

<!-- source: 1-add-the-records-of-the-domain b96b2365fdd4 -->
## 1. Adicione os registros do domínio

O SES envia a partir de uma identidade verificada. O `mycroblog-aws` cria a identidade do domínio de `mail.from` (para
`noreply@social.example`, o domínio `social.example`) com Easy DKIM, na região da implantação. Depois do apply, a
saída `dns_records` lista os registros a adicionar no seu provedor de DNS, com o registro de `host`:

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

Os três registros CNAME publicam as chaves com que o SES assina. O SES os verifica por conta própria e marca a identidade como verificada assim que eles
resolvem, em geral em até uma hora e no máximo em 72 horas; até lá ele se recusa a enviar. O console do SES mostra o status
da identidade em **Identities**.

Adicione também registros SPF e DMARC, que os receptores verificam ao lado do DKIM: [SPF, DKIM e DMARC](email.md#spf-dkim-and-dmarc) diz
o que colocar em cada um. O DKIM assinado pelo SES se alinha ao domínio de `mail.from`, então o DMARC passa apenas com esses registros; um
domínio MAIL FROM personalizado, que alinha o SPF também, é opcional.

<!-- source: 2-leave-the-sandbox 7f27054422ae -->
## 2. Saia do sandbox

Toda conta nova começa no sandbox do SES, separadamente em cada região. No sandbox, o SES envia apenas para endereços e
domínios que você mesmo verificou, e no máximo 200 mensagens em 24 horas e uma por segundo. Um servidor no sandbox pode enviar ao
próprio operador uma mensagem de teste, mas o cadastro de qualquer outra pessoa falha.

Para enviar a todos, solicite acesso de produção para a região da implantação:

1. Abra o console do SES nessa região e, em **Account dashboard**, escolha **Request production access** (ou **Get set up**).
2. Escolha **Transactional** como o tipo de e-mail, informe a URL do servidor e descreva o que ele envia: confirmações de
   cadastro e de um novo endereço, redefinições de senha e as notificações que cada usuário ativa e pode desativar com um
   cancelamento de inscrição em um clique.
3. Diga como você trata bounces e reclamações: o servidor deixa de enviar a um endereço que gera bounce ou reclamação assim que as
   notificações abaixo chegam a ele.

A AWS responde por e-mail, em geral em até um dia, e pode pedir mais informações antes de conceder o acesso. Depois que ela concede, o console mostra
sua cota de envio, que cresce com o seu uso.

<!-- source: 3-watch-bounces-and-complaints b1182f32de88 -->
## 3. Acompanhe bounces e reclamações

O SES observa a proporção de mensagens que geram bounce ou reclamações e coloca em análise, depois pausa, uma conta cujas taxas
continuam altas. O servidor fica sabendo de um bounce ou de uma reclamação pelas notificações do SES por meio do Amazon SNS e deixa de enviar a esse
endereço. O `mycroblog-aws` não cria o tópico: configure-o como [Receber notificações do SES](email.md#receive-ses-notifications-sns)
diz e dê ao servidor o ARN dele por meio de `settings` do módulo:

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

O console do SES mostra as taxas de bounce e de reclamação da conta em **Reputation metrics**.

<!-- source: sending-from-another-region 61edc3b77ea9 -->
## Enviando de outra região

O módulo cria a identidade na região da implantação, e o servidor envia dali. Para enviar pelo SES em outra
região, ou por um serviço SMTP no lugar, defina `mail = { kind = "smtp", ... }` com a interface SMTP desse serviço:
[Amazon SES (interface SMTP)](email.md#amazon-ses-smtp-interface) traz as configurações.
