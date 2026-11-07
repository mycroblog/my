<!-- source: production-checklist e5f6c0c5b10a -->
# 本番環境のチェックリスト

サーバーをユーザーとほかのサーバーに公開する前と、デプロイを変えたあとに、この一覧を確認してください。
いったん連合したサーバーでは変えられない設定があり、鍵暗号化鍵をなくすといった誤りは
まったく元に戻せません。

各項目には、なぜ重要か、どう確認するか、詳しい説明がどこにあるかを書いています。
まず自動の部分から始めます。

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

<!-- source: what-my-config-check-checks f5feea1abf22 -->
## `my config check` が確認すること

`production` プロファイル（[`profile`](../reference/configuration.md#profile) の既定値）では、`my config check` は
まずすべての設定を検証し、次に有効な設定が本番のインスタンスに適しているかを確認します。
問題は 2 つのグループに分けて一覧され、プロセスのロケールの言語による要約の下に、それぞれ英語の技術的な詳細が付きます。

- **エラー**があると、`my config check` は 1 で終了します。本番のインスタンスを壊すか、そのセキュリティを弱める設定です。
  - ほかのサーバーから届かない `host` または `account_domain`。ポート付きのもの、`localhost`、ドットのない名前、
    ローカル用やテスト用に予約されたトップレベルドメインの下の名前（`.localhost`、`.local`、`.test`、`.example`、`.invalid`、`.internal`、`.home.arpa`）です。
  - `http` の `web_origin`。
  - データベースをメモリに置く SQLite の `database_url`。
  - すべてゼロや同じバイトの繰り返しのような、ランダムでない `kek`。
  - CAPTCHA のサービスが公開しているテスト用の鍵である、`captcha` のサイトキーまたはシークレットキー。
  - どのジョブの種類も指していない `worker_job_types`、`worker_job_types_except`、またはキューのルート（すべてのプロファイルで。一覧は `my job types` で確認できます）。
  - `-instance` を付けたとき、データベースのプールでは賄えない `worker_job_concurrency`。
- **警告**では、終了コードは 0 のままです。開発ではよくあり、本番でも意図的なことがある設定です。
  - `mail = none`。
  - 一時ディレクトリ（`/tmp`、`/var/tmp`、`/dev/shm`、`$TMPDIR`）にある SQLite のデータベースまたは `fs` の blob ストア。
  - `queue = inline`。
  - `vapid_private_key` がないこと。
  - `stream = hub` または `kv = memory` で、`role` が `all` 以外であること。これらは 1 つのプロセスの中でしか働きません。
  - STUN も TURN もない `voice = p2p` のような、おそらく意図していない通話の設定。
  - `http://` のアドレスにある[プラグイン](plugins.md)、またはトークンもクライアント証明書もない `https://` のアドレスにあるプラグイン。
  - 決済の事業者のテストモードの鍵。
  - `origin_secret` を設定したのに `trusted_proxies` が空であることと、交換のあとも `origin_secret_previous` が設定されたままであること。
  - `-instance` を付けたとき: 設定されていないインスタンスのテキスト `name`、`short_description`、`terms`、`privacy_policy` とインスタンスの設定
    `contact_account`、`contact_email`、`security_contact`、そしてルールのないサーバー。
    データベースに接続できないときは、それが警告として 1 つ加わり、残りの確認はそのまま行われます。

`my serve` は起動時に同じ結果を警告としてログに出しますが、起動はします。
`development` プロファイルはこれらの確認を省きます。ローカルでの開発とテストに使い、ほかの人が使うインスタンスには決して使わないでください。

`my config check` は、サーバーと同じ環境（同じ環境変数、シークレットのファイル、設定ファイル）で実行してください。
たとえば、新しいバージョンがトラフィックを受ける前のデプロイの手順の 1 つとして実行します。

<!-- source: settings-you-cannot-change-later b2c961cb26f5 -->
## あとから変えられない設定

<!-- source: the-servers-domain-and-the-handle-domain b01ce235bcc0 -->
### サーバーのドメインとハンドルのドメイン

- **理由**: `host` はサーバーのすべての ActivityPub の id の一部で、ほかのサーバーはその id を保存します。
  `account_domain` はハンドル（`@alice@example.org`）のドメインです。データベースを使う最初の `my serve` または `my` のコマンドが
  両方をデータベースに記録し、以降、設定が異なると `my serve` とそれらのコマンドは実行を拒否します。
  連合したあとに変えると、すべてのフォローと、サーバーの投稿やアカウントへのすべての参照が壊れます。
  両方とも最初の起動の前に設定してください。連合する前に見つけた誤りは、
  すべての行を消す[データベースの作り直し](database.md#recreating-erasing-every-row)で直します。
- **決めること**: ハンドルにサーバーのドメインを使うか、別のドメインを使うか（たとえば、ハンドルは `example.org`、
  サーバーは `social.example.org`）。別のハンドルのドメインは、WebFinger、host-meta、NodeInfo をサーバーへ
  リダイレクトしなければなりません。
- **確認**: `my config check` は `host` と `account_domain` を表示し、ほかのサーバーから届かないドメインを報告します。
  `my config check -instance` は、それらがデータベースの値と異なるときにも失敗し、何も記録しません。
  別のハンドルのドメインでは、`curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` が
  `https://<host>/.well-known/webfinger?...` への `301` を返します。
- **詳細**: [サーバーの運用](running.md)（連合のエンドポイントと、別のドメインからのハンドルの配信）、
  [`host`](../reference/configuration.md#host) と [`account_domain`](../reference/configuration.md#account_domain)。

<!-- source: reachable-over-https 1cf53c613f7b -->
### `https` で届くこと

- **理由**: ほかのサーバーは id を `https://<host>/` のポート 443 で取得し、ブラウザーはログインの Cookie を `https` でだけ送ります。
- **確認**: ネットワークの外から `curl -sI https://<host>/healthz` を実行すると、`host` に有効な証明書で `200` が返り、
  `my config check` が `web_origin` についてのエラーを報告しないこと。`http://<host>/` は `https` にリダイレクトされます。これはサーバーの前にある
  リバースプロキシ、ロードバランサー、CDN が行います（Docker Compose では、レイヤー `compose.caddy.yaml` がそうしたプロキシです）。
- **詳細**: [サーバーの運用](running.md)、[`web_origin`](../reference/configuration.md#web_origin)。

<!-- source: not-reachable-past-the-cdn 58310b9f29d7 -->
### CDN を迂回して届かないこと

- **理由**: 前に CDN があり、サーバー自身のアドレスも公開されたまま（Cloud Run の `*.run.app`）だと、誰でも WAF とキャッシュを迂回してサーバーを呼べ、
  CDN と同じプラットフォームにいる呼び出し元は、レート制限が見るクライアントの IP を選べてしまいます。必要なのはこの場合だけです。ファイアウォールやプラットフォームがすでに接続できる相手を制限しているときは不要で、
  CDN がなければ迂回するものもありません。
- **確認**: `origin_secret` と `origin_secret_header` が設定され、CDN がすべてのホスト名でそのヘッダーを付けていること。`curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/`
  が `403` を返し、その `/healthz` が `200` を返し、`https://<host>/` が `200` を返すこと。
- **詳細**: [CDN を通らなかったリクエストを拒否する](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)。全員が締め出されたときの戻し方も説明しています。

<!-- source: secrets ec71f1f20574 -->
## シークレット

<!-- source: the-key-encryption-key-kek 89df548c5bf9 -->
### 鍵暗号化鍵（KEK）

- **理由**: KEK は、すべてのアクターの秘密鍵を暗号化します。KEK がなければサーバーは何にも署名できず、鍵をデータベースから
  取り戻すこともできません。すべてのアカウントを作り直すことになり、ほかのサーバーはそれを信頼しなくなります。
- **確認**:
  - ランダムに生成されていること（`openssl rand -base64 32`）。`my config check` はランダムでない鍵を報告します。
  - データベースとそのバックアップ以外の場所、たとえばパスワードマネージャーやクラウドのシークレットストアに保管され、
    サーバーを運用するほかの少なくとも 1 人がそれにアクセスできること。
  - `MY_KEK` または `MY_KEK_FILE` だけでサーバーに渡されていること（設定ファイルはシークレットを拒否します）。Docker Compose では、
    ファイル `secrets/kek` で、`.env` には入っていないこと（[Docker Compose のファイル](running.md#docker-compose-files)）。
- **詳細**: [鍵の管理](key-management.md)。

<!-- source: the-vapid-key-of-web-push 76bc2a1dd030 -->
### Web Push の VAPID 鍵

- **理由**: これがないと、サーバーは Web Push の通知を送りません。あとで変えるとすべての購読が終わるので、1 回だけ生成し、
  KEK と同じように保管してください。
- **確認**: `vapid_private_key` が設定されていないと、`my config check` が警告します。`my config vapid-key` で生成します。
- **詳細**: [鍵の管理](key-management.md)（Web Push の VAPID 鍵）。

<!-- source: other-secrets-stay-out-of-the-repository-and-the-image 94f290afcdbb -->
### そのほかのシークレットをリポジトリとイメージに入れない

- **理由**: リポジトリ、コンテナイメージ、設定ファイルに入ったシークレットは、それらが行くすべての場所に複製されます。
- **確認**: `database_url`、`mail_smtp_password`、`captcha_secret_key`、決済の事業者の鍵、`valkey_url`、そのほか
  設定リファレンスでシークレットとされているすべての設定が、プラットフォームのシークレットストア（Docker や Kubernetes の
  シークレット、Secret Manager、Parameter Store）が埋める環境変数か `<NAME>_FILE` のファイルから来ていること。設定ファイルにはこれらを書けず、
  `my config check` はそこにあるシークレットを拒否します。Docker Compose では、自分だけが読める `.env`（`chmod 600`）から来ます。リポジトリとイメージを値で検索し、複製が残っていないことを確かめてください。
  `my config check` は、設定されているすべてのシークレットを `(redacted)` と表示します。
- **詳細**: [設定リファレンス](../reference/configuration.md)（Secret の列）、[CAPTCHA](captcha.md)、
  [Stripe](payments-stripe.md)、[PAY.JP](payments-payjp.md)。

<!-- source: instance-settings baae9cc3e09e -->
## インスタンスの設定

インスタンスの設定はデータベースにあり、サーバーの動作中に、サーバーの管理画面か
`my instance set` で変えられます。`my config check -instance` は、設定されていないテキスト、連絡先、ルールについて警告します。

- **名前、短い説明、説明**: ランディングページ、情報ページ（`/about`）、リンクのプレビュー、
  NodeInfo に表示されます。`/about` で確認してください。
- **連絡先**: `contact_account`（ユーザーがサーバーについて連絡するアカウント）と `contact_email`（ユーザーと
  ほかのサーバーが連絡するアドレス。サーバーからのメールも、返信をこのアドレスに送ります）。`/about` で確認してください。
- **脆弱性の連絡先**: `security_contact`。`https:` または `mailto:` の URI です。設定すると、サーバーは
  `/.well-known/security.txt` に応答します。`curl -s https://<host>/.well-known/security.txt` で確認してください。
- **利用規約とプライバシーポリシー**: `terms` と `privacy_policy`。プライバシーポリシーには、CAPTCHA のサービスやメールの事業者など、
  個人データを受け取るサービスを書かなければなりません。`/about` と登録の画面のリンクを確認してください。
- **サーバーのルール**: ユーザーが登録するときに同意し、モデレーターが守らせるものです。`my instance rule create`
  か管理画面で追加し、`/about` で確認してください。
- **登録モード**: `registration_mode` は、変えるまで `approval` です。人がどう参加するかに合わせて、`open`、`approval`、`invite`、`closed` から
  選びます。`mail = none` のときは、`open` での登録もモデレーターの承認を待ちます。
- **検索エンジンと AI**: `search_indexing`、`ai_usage`、`indexable_default` は、許可するまでどちらも拒否します。
  `/robots.txt` と `/.well-known/tdmrep.json` を確認してください。
- **ステータスページ**: [`status_page_url`](../reference/configuration.md#status_page_url)（インスタンスの設定ではなく設定なので、
  メンテナンス中もリンクが表示されます）に、サーバーとは別のサービス上のステータスページを設定します。

**詳細**: [インスタンスの設定](instance-settings.md)、[登録](registration.md)、
[検索エンジンと AI のクローラー](crawlers.md)、[CAPTCHA](captcha.md)（プライバシーポリシーに書くこと）、
[プライバシーポリシーのひな形](privacy-policy-template.md)、[処理活動の記録のひな形](processing-record-template.md)、
[監視](monitoring.md)（ステータスページ）。

<!-- source: email 40c18999c46f -->
## メール

- **理由**: メールでアドレスを確認し、パスワードのリセットと通知を送ります。`mail = none` では、ユーザーにメールが届きません。
- **確認**:
  - `mail` が `smtp`、`ses`、`plugin` のいずれかで、`mail_from` が自分の管理するドメインのアドレスであること。`my config check` は `mail = none` について警告します。
  - `mail_from` のドメインに SPF、DKIM、DMARC のレコードがあり、事業者が DKIM で署名していること。これらがないと、メールは迷惑メールになります。
  - テスト用のアカウントで登録して確認のメールを受け取り、パスワードのリセットを求めてそのメールも受け取ること。
    受け取ったメッセージのヘッダーで、`spf=pass`、`dkim=pass`、`dmarc=pass` を確認してください。
  - SES では、アカウントがサンドボックスを出ていて、バウンスと苦情の通知がサーバーに届くこと。
- **詳細**: [メール](email.md)、[登録](registration.md)（メールアドレスの確認）。

<!-- source: administrators 2c05e6644d88 -->
## 管理者

- **理由**: 管理者のアカウントは、すべての設定を変え、非公開のデータを読み、誰でも凍結できます。
- **確認**:
  - すべての管理者が二要素認証をオンにし（Web クライアントの「設定」の「二要素認証」）、
    リカバリーコードを保管していること。
  - 必要な人だけが `admin` ロールを持ち、モデレーターは `moderator` か、使う権限だけを持つロールを持っていること。
    `my admin role` で一覧し、管理画面で見直してください。
- **詳細**: [管理者とロール](admin-cli.md)、[モデレーション](moderation.md)（権限とログインの回復）。

<!-- source: operations 56bd643c2241 -->
## 運用

- **バックアップと復旧**: データベース、blob ストア（`blob_fs_dir` またはバケット）、KEK が、定期的に、
  サーバーとは別の場所にバックアップされ、新しいサーバーへの復元を予行演習していること。復元したサーバーが起動し、アカウントが
  ログインして投稿できることを確かめます。一度も復元したことのないバックアップは、使えるかどうか分かりません。データベースとメディアは
  一時ディレクトリに置いてはいけません。置いていると `my config check` が警告します。
  [鍵の管理](key-management.md)（KEK のバックアップ）と[データベース](database.md)を参照してください。
- **監視とアラート**: 外部の監視が `/healthz` を確認し、対応できる人にアラートを送ること、そしてジョブキューの dead のジョブを
  見張っていること。[監視](monitoring.md)と[サーバーの運用](running.md)（ヘルスチェック、メトリクス、ジョブキュー）を参照してください。
- **イメージ**: インスタンスが、再起動で変わるイメージではなく、ダイジェストで固定したリリース済みのイメージ（`ghcr.io/mycroblog/my:0.1.0@sha256:...`、リリースノートにあります）を実行し、
  [その来歴を確認していること](running.md#verify-an-image)。`latest` タグはありません。
  アップグレードでは、そのバージョンを意図して変え、[アップグレード](database.md#upgrading)に従って、マイナーバージョンを 1 つずつ上げます。
- **ソースコード（AGPL-3.0）**: サーバーは、使うすべての人にソースコードを提供しなければなりません。変更していないリリースでは
  [`source_url`](../reference/configuration.md#source_url) を既定値のままにし、フォークや変更したバージョンを運用するときは
  自分のソースを指すようにします。`/about` のリンクを確認してください。
  [サーバーの運用](running.md)（ソースコードの公開）を参照してください。
- **メンテナンス**: 必要になる前に、サーバーをメンテナンスモードにする方法を知っておいてください。[メンテナンス](maintenance.md)を参照してください。

<!-- source: federation a5ab186ec04b -->
## 連合

- **理由**: ほかのサーバーは WebFinger と NodeInfo でアカウントを見つけ、それからアクターを取得します。外からこれらのどれかが失敗すると、
  そのサーバーは Fediverse から見えません。
- **確認**は、ネットワークの外のマシンから行います。
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` がアカウントを返すこと。
    ドメインが異なるときは、`host` へのリダイレクトをたどります（`curl -L`）。
  - `curl -s https://<host>/.well-known/nodeinfo` が `/nodeinfo/2.1` にリンクし、そこにソフトウェアとそのソースが書かれていること。
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` がアクターの id にリダイレクトし、
    その id のアクターが `publicKey` を持つ JSON であること。
  - Mastodon やほかのサーバーのアカウントから `@<user>@<account_domain>` を検索してフォローし、新しい投稿が届くのを確かめること。
  - サーバーの前にある CDN が、`Vary: Accept` に従わないかぎり `/ap/*` と `/@*` をキャッシュしないこと。
- **詳細**: [サーバーの運用](running.md)（連合のエンドポイント）、[CDN](cdn.md)、[リレー](relays.md)。
