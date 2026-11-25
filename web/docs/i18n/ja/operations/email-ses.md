<!-- source: sending-email-through-amazon-ses 6deee2054246 -->
# Amazon SES でメールを送る

サーバーのメールを送るために Amazon SES を準備する方法です。送信者のドメインを DNS レコードで検証し、新しいアカウントが始まるサンドボックスから出て、アカウントを良好な状態に保ちます。OpenTofu モジュールの [`mycroblog-aws`](deploy-aws.md#email-through-amazon-ses) は、既定で SES を使って送信し、ID と権限を作ります。レコードと本番環境へのアクセスのリクエストは、自分で行う手順です。
設定そのものは[メールの送信](email.md#amazon-ses-api)にあります。

**AWS へのデプロイは、まだ一度も適用されていません。** そのため、これらの手順は AWS のドキュメントに基づくもので、このプロジェクトのサーバーで最後まで実行して確かめてはいません。

<!-- source: 1-add-the-records-of-the-domain b96b2365fdd4 -->
## 1. ドメインのレコードを追加する

SES は、検証済みの ID から送信します。`mycroblog-aws` は、`mail.from` のドメイン（`noreply@social.example` なら `social.example`）の ID を、Easy DKIM を有効にして、デプロイのリージョンに作ります。apply のあと、出力 `dns_records` に、DNS プロバイダーに追加するレコードが `host` のレコードとともに一覧されます。

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

3 つの CNAME レコードは、SES が署名に使う鍵を公開します。SES はそれらを自分で確認し、名前が引けるようになると ID を検証済みにします。通常は 1 時間以内、長くても 72 時間以内で、それまでは送信を拒否します。SES コンソールでは、**Identities** で ID の状態を確認できます。

SPF と DMARC のレコードも追加してください。受信側が DKIM と並べて確認します。それぞれに何を書くかは、[SPF、DKIM、DMARC](email.md#spf-dkim-and-dmarc)にあります。SES が付ける DKIM 署名は `mail.from` のドメインと一致するので、DMARC はこれらのレコードだけで通ります。SPF も一致させるカスタム MAIL FROM ドメインは任意です。

<!-- source: 2-leave-the-sandbox 7f27054422ae -->
## 2. サンドボックスから出る

新しいアカウントはすべて、リージョンごとに別々に、SES のサンドボックスから始まります。サンドボックスでは、SES は自分で検証したアドレスとドメインにしか送れず、24 時間に最大 200 通、1 秒に 1 通までです。サンドボックスのサーバーは、運営者自身にテストメッセージを送れますが、ほかの人の登録は失敗します。

全員に送るには、デプロイのリージョンで本番環境へのアクセスをリクエストします。

1. そのリージョンの SES コンソールを開き、**Account dashboard** で **Request production access**（または **Get set up**）を選びます。
2. メールの種類に **Transactional** を選び、サーバーの URL を伝えて、送る内容を説明します。登録と新しいアドレスの確認、パスワードのリセット、各ユーザーがオンにして、ワンクリックの配信停止でオフにできる通知です。
3. バウンスと苦情をどう扱うかを説明します。下の通知が届くと、サーバーはバウンスまたは苦情の出たアドレスへの送信を止めます。

AWS は通常 1 日以内にメールで返信し、アクセスを認める前に追加の情報を求めることがあります。認められると、コンソールに送信クォータが表示され、利用に応じて増えていきます。

<!-- source: 3-watch-bounces-and-complaints b1182f32de88 -->
## 3. バウンスと苦情を見守る

SES は、バウンスしたり苦情を受けたりしたメッセージの割合を監視し、その割合が高いままのアカウントを、審査にかけ、そのあと停止します。サーバーは、Amazon SNS 経由の SES の通知からバウンスや苦情を知り、そのアドレスへの送信を止めます。`mycroblog-aws` はトピックを作りません。[SES の通知を受け取る](email.md#receive-ses-notifications-sns)に従って設定し、その ARN をモジュールの `settings` でサーバーに渡してください。

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

SES コンソールの **Reputation metrics** で、アカウントのバウンス率と苦情率を確認できます。

<!-- source: sending-from-another-region 61edc3b77ea9 -->
## 別のリージョンから送る

モジュールは ID をデプロイのリージョンに作り、サーバーもそこから送信します。別のリージョンの SES から送るとき、または代わりに SMTP サービスを使うときは、そのサービスの SMTP インターフェースで `mail = { kind = "smtp", ... }` を設定します。設定は [Amazon SES (SMTP インターフェース)](email.md#amazon-ses-smtp-interface)にあります。
