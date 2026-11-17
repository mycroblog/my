<!-- source: external-login-providers 48a582ab87ec -->
# 外部のログインプロバイダー

外部のログインプロバイダーを使うと、ユーザーはほかの場所に持っているアカウントでログインできます。OpenID Connect のプロバイダー
（Google Workspace、Microsoft Entra ID、Keycloak など）、GitHub、Discord、または 1 つの Mastodon サーバーです。
プロバイダーは、サーバーの設定がある環境で `my login-provider` コマンドを使うか、権限 `manage_login_providers` を持って
管理 API（`/api/v1/admin/login_providers`）を使って設定します。この権限を持つのは組み込みの `admin` ロールだけです。
このコマンドは管理者が誰もログインできないうちから使えるので、最初の設定に使えます。
Web クライアントでは、この権限を持つ管理者が管理メニューの「ログインプロバイダー」を使います。一覧には、
プロバイダーごとの種類、状態、登録を許可しているかどうかが表示され、「プロバイダーを追加」でプロバイダーを作るフォームが開きます。
プロバイダーを選ぶとそのページが開き、コールバック URL とそれをコピーするボタン、連携しているユーザーの数が表示されます。
このページで、設定の変更、新しいクライアントシークレットの登録、有効化、無効化、削除を行います。

<!-- source: what-external-sign-in-does 25fa37e22a50 -->
## 外部ログインでできること

- プロバイダーが証明するのは、そのプロバイダーで誰であるかだけです。アカウントがメールアドレスによってユーザーと連携されることはありません。
  ユーザーは設定の「連携したアカウント」でアカウントを連携するか、そのアカウントで新しいユーザーとして登録します。
  既存のユーザーと一致するメールアドレスでも、そのユーザーと連携されることはありません。
- 連携したユーザーも、プロバイダーでログインしたあとに二要素認証のコード（TOTP）を入力します。また、アカウントが
  有効でないユーザーはログインできません。
- 新しいユーザーがプロバイダーで登録できるのは、そのプロバイダーで登録を許可し（`-allow-signup`）、かつ
  [登録モード](registration.md)が登録を認めているときだけです。招待コード、CAPTCHA、登録のブロック、ネットワークの制限は、
  パスワードのフォームと同じように適用されます。この方法で登録した人はパスワードを持たず、プロバイダーでだけログインします。
  [登録の流れの図](registration.md#from-sign-up-to-the-app)で、2 つの登録方法を並べて示しています。

<!-- source: kinds c9123566b5c5 -->
## 種類

| 種類 | `-issuer` | 条件 |
| --- | --- | --- |
| `oidc` | issuer の URL（`https`）。`https://accounts.google.com` のように、プロバイダーの ID トークンに書かれているとおりに指定します | `-claim <claim>=<value>`: ID トークンのそのクレームが、指定した値のいずれかであるユーザーだけ |
| `github` | なし。サーバーは github.com のエンドポイントを知っています | `-org <organization>`: いずれかの Organization のメンバーだけ（`-scope read:org` を付けます） |
| `discord` | なし。サーバーは discord.com のエンドポイントを知っています | `-guild <server ID>`: いずれかの Discord サーバー（ギルド）のメンバーだけ。指定しなければ、すべての Discord ユーザーが入れます |
| `mastodon` | `mastodon.example` のような、そのサーバーの Web のドメイン（Mastodon 4.3 以降） | なし |

Google Workspace では、`-claim hd=<your domain>` でユーザーを自分の組織に限定してください。メールアドレスのドメインに
頼らないでください。

<!-- source: examples 0d19ab463197 -->
## 例

<!-- source: google b99294b8539d -->
### Google

Google Cloud コンソールで、種類が「ウェブ アプリケーション」の OAuth クライアントを作ります。自分の組織に限るには、
OAuth 同意画面のユーザーの種類を「内部」にし（プロジェクトが Google Workspace の組織に属しているときに使えます）、
さらに `hd` の条件も加えます。ユーザーの種類は Google が誰を通すかを制限し、条件はこのサーバーにすべての ID トークンの
`hd` クレームを確認させます。

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

<!-- source: keycloak-and-other-openid-connect-providers 0840dc38a4e0 -->
### Keycloak とそのほかの OpenID Connect プロバイダー

issuer は `https://sso.example.com/realms/main` のようなレルムの URL で、プロバイダーの ID トークンに書かれているとおりに指定します。
標準フローを使うコンフィデンシャルクライアントを作り、スコープ `openid`、`email`、`profile` を付けます。プロバイダーが
プライベートネットワークにあるときは、`login_provider_allowed_destinations` に加えます（[後述](#providers-on-an-internal-network)）。
グループのクレームなど、クレームが指定した値のいずれかであることを求めるには `-claim` を使います。

<!-- source: discord 00e9659d1eae -->
### Discord

[Discord Developer Portal](https://discord.com/developers/applications) でアプリケーションを作り、「OAuth2」を開いて、
クライアント ID とクライアントシークレットをサーバーに渡します。サーバーはスコープ `identify` と `email` を求め、
`-guild` でサーバーを指定したときは `guilds` も求めます。ユーザーは数字の Discord ID で識別されます。ユーザー名と表示名は、
登録フォームのユーザー名の候補になるだけです。メールアドレスは、Discord が確認済みとしたときだけ確認済みとして扱われます。
そうでなければ、ユーザーはほかのアドレスと同じようにメールで確認します。

`-guild` を付けると、サーバーはログイン、登録、連携のたびに、ユーザーが指定したサーバーのいずれかに属していることを確認し、
属していなければ拒否します。付けなければ、すべての Discord ユーザーがログインできます。ギルド ID は最大 20 桁の数字です。
Discord で「開発者モード」をオンにし（ユーザー設定の「詳細設定」）、サーバーのアイコンを右クリックして「サーバー ID を
コピー」を選びます。

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider create -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

`add` がコールバック URL を出力したら、アプリケーションの「OAuth2」の「Redirects」に追加します。

<!-- source: github a7181ea6edd6 -->
### GitHub

OAuth App を作り（Settings、Developer settings、OAuth Apps）、クライアント ID とシークレットを渡します。サーバーは
スコープ `user:email` を求め、`-org` で Organization を指定したときは `read:org` も求めます。`-org` を付けると、サーバーは
ログインのたびにユーザーがいずれかの Organization に属していることを確認するので、ほかの Organization のメンバーは拒否されます。

<!-- source: set-up-a-provider 8e0bb73251d2 -->
## プロバイダーを設定する

プロバイダーが必要とするリダイレクト URI は、そのプロバイダーのコールバック URL `https://<host>/auth/external/<id>/callback` で、
サーバーがプロバイダーに付ける ID を含みます。そのため、次の順に進めます。

1. プロバイダーで OAuth クライアント（GitHub では「OAuth App」）を、ひとまず任意のリダイレクト URI で登録し、発行された
   クライアント ID とクライアントシークレットを控えます。
2. それらを使って `my login-provider create` を実行します。コールバック URL が出力されます。
3. プロバイダーでクライアントのリダイレクト URI を、そのコールバック URL に変えます。

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider create -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

<!-- source: mastodon 6f167bff1f67 -->
### Mastodon

`mastodon` のプロバイダーは、指定した 1 つの Mastodon サーバーです。ほかのサーバーのユーザーはこれでログインできません。そのサーバーは
Mastodon 4.3 以降でなければなりません。`add` は認可サーバーのメタデータ
（`/.well-known/oauth-authorization-server`）を読み、それがないサーバーを拒否します。`-client-id` を付けなければ、`add` は
コールバック URL とスコープ `profile` で、そのサーバーに自分でクライアントを登録し、得たクライアント ID とシークレットを保存します。
そのため、サーバー側ですることはありません。

```sh
my login-provider create -kind mastodon -name mastodon.example -issuer mastodon.example
```

そのサーバーがほかからのクライアントの登録を認めていないと、`add` は失敗します。その場合は、サーバーの開発の設定で、スコープ `profile` と
コールバック URL を指定したアプリケーションを作り、ほかの種類と同じようにクライアント ID とシークレットを渡します
（コールバック URL はプロバイダーの ID を含むので、上の 3 つの手順に従ってください）。

ユーザーはスコープ `profile` だけでログインし、これは自分のアカウントだけを読みます。Mastodon はメールアドレスを
渡さないので、これで登録した人はメールでアドレスを確認します。登録フォームは、そのアカウントのユーザー名を候補に出します。
そのサーバーのドメインまたは親ドメインにドメインへの措置（[ドメインへの措置](moderation.md#domain-actions)）があるあいだは、そのボタンは表示されず、
誰もそれでログイン、登録、アカウントの連携をできません。プロバイダーのサーバーは変えられません。別のサーバーには新しいプロバイダーを
設定してください。

クライアントシークレットは鍵暗号化鍵（[鍵の管理](key-management.md)）で暗号化して保存され、コマンドでも管理 API でも
二度と表示されません。

新しいプロバイダーは有効な状態で作られます。既定では、ユーザーは既存のアカウントをこのプロバイダーに連携できます（`-allow-link`）が、
これで登録することは誰もできません。新しいユーザーがこれで登録できるようにするには `-allow-signup` を付けます。その場合も、登録モードによる
制限は受けます（[登録](registration.md)）。

<!-- source: change-rotate-disable-and-delete 55a0d23ac0d0 -->
## 変更、シークレットの交換、無効化、削除

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` は指定した設定だけを変えます。`-scope`、`-claim`、`-org` は一覧全体を置き換えます。プロバイダーの種類は
  変えられません。代わりに新しいプロバイダーを設定してください。
- `rotate-secret` は、クライアントシークレットを入力したものに置き換えます。先にプロバイダーで交換してから、ここで交換してください。
- `disable` はプロバイダーとそれに連携したアカウントを残しますが、`enable` を実行するまで、誰もそれでログイン、登録、連携をできず、
  ログインページにも表示されません。`disable` と `delete` は、そのプロバイダーでログインできなくなる連携ユーザーの数と、
  設定がオンのときに[ログインプロバイダーへの限定](#limit-sign-in-to-login-providers)がその時点でどう働くかを出力します。
- `delete` はプロバイダーを完全に削除します。

<!-- source: users-who-sign-in-only-through-a-provider 11dc6f466cb7 -->
### プロバイダーでだけログインするユーザー

プロバイダーを無効にしたり削除したりしても、ユーザーには手を付けません。パスワードやパスキー、またはほかの連携したアカウントを
持つユーザーは、その方法でログインします。ただし、[ログインがログインプロバイダーに限定されていて](#limit-sign-in-to-login-providers)
ほかのプロバイダーが有効なままのときは別です。どれも持たないユーザーは、まったくログインできなくなります。`disable` と `delete` は
プロバイダーを失う連携ユーザーの数を出力し、Web クライアントは確認の画面にその数を表示します。`enable` で、無効にしたプロバイダーの
ユーザーは元に戻ります。削除したプロバイダーは戻せません。

そうしたユーザーを助けるには、管理画面のそのアカウントの管理ページか、`my admin account password-reset-link @alice` で
パスワードリセットリンクを発行し、ユーザーに渡します（[登録](registration.md#password-reset-and-change)）。
ユーザーは新しいパスワードでログインします。

すべての変更は、プロバイダーの表示名で監査ログに記録されます。実行者は CLI か、管理 API で変更した管理者です。

<!-- source: limit-sign-in-to-login-providers bb5ebf61b647 -->
## ログインをログインプロバイダーに限定する

インスタンスの設定 `login_providers_only`（既定ではオフ）は、少なくとも 1 つのプロバイダーが有効なあいだ、プロバイダーを
ログインと登録の唯一の方法にします。Web クライアントの「サーバーの設定」の「登録」ページか、CLI でオンにします。

```sh
my instance set login_providers_only true
```

Web クライアントの確認とコマンドの出力は、有効なプロバイダーに連携していない有効なローカルユーザーが何人いるかを示します。
Web クライアントは、そこに自分が含まれるときもそう伝えます。警告はしますが拒否はしません。判断するのはあなたです。

限定が適用されているあいだは、次のようになります。

- パスワードとパスキーでは誰もログインできず、ログインページにはプロバイダーだけが表示されます。パスワードやパスキーで始めて
  二要素認証のコードを待っているログインも拒否されます。
- 誰もパスワードで登録できません。登録ページには、登録を許可している（`-allow-signup`）プロバイダーが、いつもどおり
  [登録モード](registration.md)に従って表示されます。そうしたプロバイダーがなければ、登録はクライアントや NodeInfo に対しても
  `closed` として示されます。
- 誰もパスワードリセットリンクを求めることも使うこともできません。リンクは期限が切れるまで有効なままで、期限内に限定が終われば使えます。
- ユーザーはパスワードの設定や変更、パスキーの追加ができません。どちらでもログインできないためです。削除はできます。
- `my admin account password-reset-link` と管理画面はリンクを発行できますが、限定が終わるまで使えないことを伝えます。
  `my admin create` は管理者を作れますが、そのパスワードではログインできないことを警告します。
- ユーザーは、有効なプロバイダーにある最後のアカウントの連携を解除できません。

引き続き使えるもの:

- セッション、アプリのアクセストークン、リフレッシュトークンは有効なままです。限定によってどれも終わりません。
- パスワードやパスキーは、セッションの中での操作の確認にはまだ使えます。既存のユーザーはこの方法でプロバイダーに移ります。
  ログインしたまま設定の「連携したアカウント」を開き、パスワードかパスキーで確認して、プロバイダーでログインします。
- プロバイダーでのログイン、登録、連携と、プロバイダーでのログイン後の二要素認証のコード。

拒否はどれも、送られたメールアドレス、パスワード、トークンにかかわらず同じ内容なので、どのアカウントについても何も明かしません。

**連携していないユーザーは、セッションが終わると、あなたが対応するまでログインできません。** アカウントがメールアドレスで
連携されることはないので、プロバイダーでのログインでも見つかりません。ログインしているうちにアカウントを連携するよう
頼んでください。セッションを失ったユーザーについては、限定をしばらく解除するか、締め出したままにするかを決める必要があります。
これには `disable_login`、`suspend`、`ban` の状態にあるユーザーも含まれ、連携がなければ[異議を申し立てる](moderation.md)ためのログインページがありません。
連携には、ブラウザーがログインページにログインしている必要もあります。アプリはログインしたままでもブラウザーのセッションが
終わったユーザーは、限定が適用されているあいだは連携できず、あなたの対応を待つことになります。**限定をオンにする前に、
ユーザーに変更を告知してください。** セッションが残っているうちにアカウントを連携してもらうためです。

<!-- source: end-the-limit 77f5c71d013b -->
### 限定を終える

限定は、再起動なしに、リクエストごとに設定とプロバイダーの状態に従います。

- 締め出された管理者は、サーバーの設定がある環境で次を実行します。

  ```sh
  my instance set login_providers_only false
  ```

- 有効なプロバイダーがないときは、限定は適用されません。最後のプロバイダーを無効にするか削除すると、設定にかかわらず
  パスワードとパスキーでのログインが再び使えるようになります。パスワードもパスキーも持たないユーザーには、やはり
  [リセットリンク](registration.md#password-reset-and-change)が必要です。
- 設定がオンで有効なプロバイダーがないときに、プロバイダーを設定するか有効にすると、すぐに限定が始まります。新しいプロバイダーは
  設定した時点で有効になります。Web クライアントは先に警告し、そのプロバイダーに連携していない有効なユーザーの数と、
  そこに自分が含まれるかを示します。警告するのは限定を始めることになる操作のときだけです。`my login-provider create`、`enable`、
  `disable`、`delete` は、設定がオンのとき、限定がその時点でどう働くかを出力します。
- プロバイダーの障害で全員が締め出されたときも、同じコマンドを使います。

<!-- source: re-authentication 8ec1742f8811 -->
## 再認証

パスワードやパスキーを求める操作（パスワードの設定、パスキーの追加、アカウントの削除や移行、
別のアカウントの連携）では、どちらも持たないユーザーに、連携したプロバイダーでもう一度ログインするよう求めます（[登録](registration.md)）。
OpenID Connect のプロバイダーには、ユーザーをもう一度認証するよう求め（`max_age=0` と `prompt=login`）、サーバーは
それより前の認証を拒否します。Mastodon サーバーには `force_login=true` で求めます。GitHub と Discord には新たなログインを
求める方法がないため、そちらでログインしたままのユーザーは何も入力せずに確認されます。サーバーはアカウントが連携したもので
あることは確認しますが、本人がその場にいることの証明としては弱くなります。これが重要なときは、OpenID Connect の
プロバイダーを使うか、ユーザーにパスワードかパスキーを持ち続けてもらってください。

<!-- source: providers-on-an-internal-network 7f19e187a375 -->
## 内部ネットワークのプロバイダー

サーバーは送信 HTTP のガードを通してプロバイダーに接続し、このガードはループバックとプライベートのアドレスを拒否します。
サーバーの隣にある Keycloak など、自分のネットワークにあるプロバイダーを使うには、そのホストまたはアドレスの範囲を
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations) に加えてください。
