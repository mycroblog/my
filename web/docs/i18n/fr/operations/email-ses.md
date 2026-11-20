<!-- source: sending-email-through-amazon-ses cb9101de6cc9 -->
# Envoyer des e-mails avec Amazon SES

Comment préparer Amazon SES à envoyer les e-mails du serveur : vérifier le domaine de l’expéditeur avec des enregistrements DNS, sortir du bac à sable
dans lequel démarre un nouveau compte, et garder le compte en règle. Le module OpenTofu [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) envoie via SES par
défaut et crée l’identité et la permission ; les enregistrements et la demande d’accès à la production sont des étapes qui vous incombent.
Les paramètres eux-mêmes se trouvent dans [Envoyer des e-mails](email.md#amazon-ses-api).

**Le déploiement AWS n’a jamais été appliqué** ; ces étapes proviennent donc de la documentation d’AWS et n’ont pas été suivies de bout en
bout sur un serveur de ce projet.

<!-- source: 1-add-the-records-of-the-domain b96b2365fdd4 -->
## 1. Ajouter les enregistrements du domaine

SES envoie depuis une identité vérifiée. `mycroblog-aws` crée l’identité du domaine de `mail.from` (pour
`noreply@social.example`, le domaine `social.example`) avec Easy DKIM, dans la région du déploiement. Après l’application, la
sortie `dns_records` liste les enregistrements à ajouter chez votre fournisseur DNS, avec l’enregistrement de `host` :

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

Les trois enregistrements CNAME publient les clés avec lesquelles SES signe. SES les vérifie lui-même et marque l’identité comme vérifiée dès qu’ils
se résolvent, généralement en moins d’une heure et au plus en 72 heures ; d’ici là, il refuse d’envoyer. La console SES affiche l’état
de l’identité sous **Identities**.

Ajoutez aussi des enregistrements SPF et DMARC, que les destinataires vérifient en plus de DKIM : [SPF, DKIM et DMARC](email.md#spf-dkim-and-dmarc) indique
quoi mettre dans chacun. Le DKIM signé par SES s’aligne sur le domaine de `mail.from`, si bien que DMARC passe avec ces seuls enregistrements ; un
domaine MAIL FROM personnalisé, qui aligne aussi SPF, est facultatif.

<!-- source: 2-leave-the-sandbox 7f27054422ae -->
## 2. Sortir du bac à sable

Chaque nouveau compte démarre dans le bac à sable de SES, séparément dans chaque région. Dans le bac à sable, SES n’envoie qu’aux adresses et
domaines que vous avez vérifiés vous-même, et au plus 200 messages en 24 heures et un par seconde. Un serveur dans le bac à sable peut envoyer à son
propre opérateur un message de test, mais l’inscription de toute autre personne échoue.

Pour envoyer à tout le monde, demandez l’accès à la production pour la région du déploiement :

1. Ouvrez la console SES dans cette région, et sur **Account dashboard** choisissez **Request production access** (ou **Get set up**).
2. Choisissez **Transactional** comme type de courrier, donnez l’URL du serveur et décrivez ce qu’il envoie : les confirmations
   d’inscription et de nouvelle adresse, les réinitialisations de mot de passe, et les notifications que chaque utilisateur active et peut désactiver par un
   désabonnement en un clic.
3. Dites comment vous gérez les rebonds et les plaintes : le serveur cesse d’envoyer à une adresse qui rebondit ou se plaint dès que les
   notifications ci-dessous l’atteignent.

AWS répond par e-mail, généralement en moins d’un jour, et peut demander des compléments avant d’accorder l’accès. Une fois l’accès accordé, la console affiche
votre quota d’envoi, qui augmente avec votre usage.

<!-- source: 3-watch-bounces-and-complaints b1182f32de88 -->
## 3. Surveiller les rebonds et les plaintes

SES surveille la part des messages qui rebondissent ou suscitent des plaintes, et place en examen, puis suspend, un compte dont les taux
restent élevés. Le serveur apprend un rebond ou une plainte par les notifications de SES via Amazon SNS et cesse d’envoyer à cette
adresse. `mycroblog-aws` ne crée pas le sujet : configurez-le comme l’indique [Recevoir les notifications SES](email.md#receive-ses-notifications-sns),
et donnez son ARN au serveur par les `settings` du module :

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

La console SES affiche les taux de rebond et de plainte du compte sous **Reputation metrics**.

<!-- source: sending-from-another-region 61edc3b77ea9 -->
## Envoyer depuis une autre région

Le module crée l’identité dans la région du déploiement, et le serveur envoie à partir de là. Pour envoyer via SES dans une autre
région, ou via un service SMTP à la place, définissez `mail = { kind = "smtp", ... }` avec l’interface SMTP de ce service :
[Amazon SES (interface SMTP)](email.md#amazon-ses-smtp-interface) contient les paramètres.
