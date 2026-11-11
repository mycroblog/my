# Sending email through Amazon SES

How to get Amazon SES ready to send the server's email: verify the domain of the sender with DNS records, leave the sandbox
that a new account starts in, and keep the account in good standing. The OpenTofu module [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) sends through SES by
default and creates the identity and the permission; the records and the request for production access are steps of your own.
The settings themselves are in [Sending email](email.md#amazon-ses-api).

**The AWS deployment has never been applied**, so these steps come from AWS's documentation and have not been followed end to
end on a server of this project.

## 1. Add the records of the domain

SES sends from a verified identity. `mycroblog-aws` creates the identity of the domain of `mail.from` (for
`noreply@social.example`, the domain `social.example`) with Easy DKIM, in the region of the deployment. After the apply, the
output `dns_records` lists the records to add at your DNS provider, with the record of `host`:

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

The three CNAME records publish the keys SES signs with. SES checks them by itself and marks the identity verified once they
resolve, usually within an hour and at most within 72 hours; until then it refuses to send. The SES console shows the status
of the identity under **Identities**.

Add SPF and DMARC records as well, which receivers check beside DKIM: [SPF, DKIM, and DMARC](email.md#spf-dkim-and-dmarc) says
what to put in each. DKIM signed by SES aligns with the domain of `mail.from`, so DMARC passes with these records alone; a
custom MAIL FROM domain, which aligns SPF too, is optional.

## 2. Leave the sandbox

Every new account starts in the SES sandbox, separately in each region. In the sandbox, SES sends only to addresses and
domains you have verified yourself, and at most 200 messages in 24 hours and one a second. A server in the sandbox can send its
own operator a test message, but the sign-up of anyone else fails.

To send to everyone, request production access for the region of the deployment:

1. Open the SES console in that region, and on **Account dashboard** choose **Request production access** (or **Get set up**).
2. Choose **Transactional** as the mail type, give the URL of the server, and describe what it sends: confirmations of
   sign-up and of a new address, password resets, and the notifications that each user turns on and can turn off with a
   one-click unsubscribe.
3. Say how you handle bounces and complaints: the server stops sending to an address that bounces or complains once the
   notifications below reach it.

AWS answers by email, usually within a day, and may ask for more before it grants access. Once it does, the console shows
your sending quota, which grows with your use.

## 3. Watch bounces and complaints

SES watches the share of messages that bounce or draw complaints, and puts under review, then pauses, an account whose rates
stay high. The server learns of a bounce or a complaint from SES's notifications through Amazon SNS and stops sending to that
address. `mycroblog-aws` does not create the topic: set it up as [Receive SES notifications](email.md#receive-ses-notifications-sns)
says, and give the server its ARN through the module's `settings`:

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

The SES console shows the bounce and complaint rates of the account under **Reputation metrics**.

## Sending from another region

The module creates the identity in the region of the deployment, and the server sends there. To send through SES in another
region, or through an SMTP service instead, set `mail = { kind = "smtp", ... }` with the SMTP interface of that service:
[Amazon SES (SMTP interface)](email.md#amazon-ses-smtp-interface) has the settings.
