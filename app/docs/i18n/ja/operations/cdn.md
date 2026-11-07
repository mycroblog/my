<!-- source: put-a-cdn-in-front-of-media-delivery 20ee3d2cefe9 -->
# メディアの配信の前に CDN を置く

サーバーの前に置いた CDN（Cloudflare、Google Cloud CDN、CloudFront など）で、メディア（`/media/` と `/media/proxy/`）を配信できます。
CDN のキャッシュから応答したリクエストは、サーバーの CPU も転送量も使いません。
このガイドでは、サーバーを CDN のオリジンにする構成（オリジンプル）を説明します。これはどの blob ストアにも当てはまります。`blob = s3` や `blob = gcs` のときも、サーバーは非公開のバケットからファイルを読み、`blob = fs` のときと同じように `/media/` の下で配信するので、バケットが CDN のオリジンになることはありません。
サーバー全体が CDN の後ろにあるときは、[CDN を経由しなかったリクエストを拒否する](#refuse-requests-that-did-not-come-through-the-cdn)こともできます。

<!-- source: configure-it e915ce01da94 -->
## 設定する

`media_origin`（`MY_MEDIA_ORIGIN`）に CDN のホスト名（たとえば `media.example.org`）を設定します。

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- 値はオリジンだけ（`scheme://host[:port]`）で、パス、クエリー、末尾のスラッシュは付けません。`https` が必須で、`http` は `localhost` のようなループバックのホストにだけ使えます。不正な値だと起動に失敗します。
- 設定しないと、メディアはサーバー自身のオリジン（`https://{host}`）から配信されます。
- 次の URL がこのオリジンを使います。ローカルのメディアには `{media_origin}/media/...`、リモートのメディアのプロキシには `{media_origin}/media/proxy/...` です。
  クライアント API、ActivityPub（`attachment`、`icon`、`image`、カスタム絵文字の `icon`）、リンクプレビュー（OGP）が出力するすべての URL が対象です。
- Web クライアントの Content-Security-Policy は、`img-src` と `media-src` にこのオリジンを加えます。
- ActivityPub のオブジェクトの `id` のような連合の URI は、`media_origin` が何であっても `https://{host}/` の下のままです。変わるのはファイルを配信する場所だけで、識別子は変わりません。
- URL は読み出すたびに組み立てられ、データベースには保存されません。そのため `media_origin` を変えると、既存のメディアの URL も変わります。
  リモートのサーバーが保存した古い URL は、古いオリジンの `/media/` が配信を続ける限り使えます。

<!-- source: point-the-cdn-at-the-server ffb279f38f08 -->
## CDN の向き先をサーバーにする

- オリジン（バックエンド）は、サーバーの公開ホスト名（`https://{host}`）にします。CDN がサーバーから取得する必要があるのは、`/media/` の下のパスだけです。
- サーバーは `/media/` と `/media/proxy/` をホスト名で制限しません。CDN が送ってきた `Host` が何であっても配信します。
- `/media/` には認証が不要で、Authorized Fetch も適用されません。サーバーは CDN からの署名のないリクエストにそのまま応答します。
- リモートのメディア（`/media/proxy/{id}/{variant}?sig=...`）では、`sig` が URL の一部です。**キャッシュキーにクエリー文字列を含めてください。** そうしないと、署名の異なる URL が 1 つのエントリーを共有してしまいます。
- キャッシュする期間は、オリジンの `Cache-Control` に任せてください（CDN で上書きしないでください）。

| パス | `Cache-Control` |
| --- | --- |
| `/media/{...}`（ローカルのメディア） | `public, max-age=31536000, immutable`（1 年） |
| `/media/proxy/{...}`（リモートのメディア） | `public, max-age=` にインスタンスの設定 `remote_media_cache_ttl_seconds` の秒数 |

どちらも `Cross-Origin-Resource-Policy: cross-origin` 付きで返されるので、ほかのオリジンのページからも読み込めます。

Cloud Run では、`ingress-cloudflare-media` が OpenTofu でこれを設定します。[Cloud Run へのデプロイ](deploy-cloud-run.md#cloudflare)を参照してください。

<!-- source: who-can-open-media-urls 27306b0bc9be -->
## メディアの URL を開ける人

メディアの URL にはアクセス制御がありません。ファイルの URL を持っている人なら誰でも、サインインせずに開けます。そのファイルを添付した投稿の公開範囲は関係ありません。
これは CDN の有無にかかわらず当てはまり、フォロワー限定の投稿やダイレクトメッセージの添付、リモートのメディアのプロキシの URL も含みます。

- URL は推測しにくくなっています。ローカルのファイルの URL にはランダムな部分を持つ識別子が含まれ、プロキシの URL には署名が付いています。1 つの URL を知っても、ほかの URL は分かりません。
- 投稿を見た人は誰でも、そのメディアの URL をほかの人に渡せ、その URL はほかの人にも使えます。
- 応答には `public` が付いているので、CDN や、サーバーと読む人の間にある共有キャッシュがコピーを持つことがあります（期間は上の表を参照してください）。
  CDN がサーバーに問い合わせずにメディアを配信できるのは、このためです。

公開でない投稿は、その投稿を見られる人を制限しますが、添付ファイルのリンクを持っている人がそのファイルを開くことは防げない、とユーザーに伝えてください。

<!-- source: deletion-and-the-cache baac405df923 -->
## 削除とキャッシュ

メディアを削除したあとも、CDN のキャッシュは `max-age` の間それを保持します。削除したファイルは、ローカルのメディアでは最大 1 年、プロキシでは最大 `remote_media_cache_ttl_seconds` の間、見えたままになることがあります。
サーバーは CDN のキャッシュをパージしません。削除やモデレーションの操作のあとすぐに消えなければならないものがあるときは、CDN のコンソールか API で手動でパージしてください。

<!-- source: service-worker f6efdae181a5 -->
## Service Worker

Web クライアントの Service Worker は、ほかのオリジンのメディアをキャッシュせず、ブラウザーのネットワーク処理に任せます。そのため、適用されるのは CDN のキャッシュだけです。

<!-- source: refuse-requests-that-did-not-come-through-the-cdn 5df4225c62b3 -->
## CDN を経由しなかったリクエストを拒否する

CDN がサーバーを守れるのは、CDN を通るリクエストからだけです。インターネットからのトラフィックを受ける Cloud Run の `*.run.app` のように、オリジンにそれ自身のアドレスでも到達できるときは、誰でも WAF やキャッシュを通らずにオリジンを直接呼び出せます。
CDN のアドレスが `trusted_proxies` にあると、CDN 自身のサーバーレスプラットフォーム上の呼び出し元（たとえばその人自身の Cloudflare Worker）も信頼されたアドレスから接続することになり、レート制限が見るクライアントの IP を自分で選べてしまいます。

次の 3 つの設定で、これを塞ぎます。CDN は転送するすべてのリクエストに秘密のヘッダーを加え、サーバーはそのヘッダーのないリクエストに、ほかの処理をする前に `403` を返します。

![オリジンの確認をオンにしたときのリクエストの経路。ブラウザー、アプリ、ほかのサーバーは CDN を通り、CDN が秘密のヘッダーを加えます。プラットフォームとそれ以外の誰かは、ヘッダーなしでサーバーに直接到達します。サーバーのオリジンの確認は、ヘッダーを持つリクエストと除外されたリクエストを通し、残りを 403 で拒否します。下の一覧が各経路を説明しています。](images/cdn-request-flow.svg)

- ブラウザーとアプリ（ページ、クライアント API、OAuth、メディアの URL）とほかのサーバー（ActivityPub の配送と取得）は、
  `host` と `media_origin` のホストで CDN に到達します。CDN はキャッシュした `/media/` のファイルには自分で応答し、
  サーバーに転送するすべてのリクエストに、`origin_secret_header` を `origin_secret` の値で設定します。
- プラットフォーム（ヘルスプローブ、死活監視、Cloud Tasks、Cloud Scheduler）と、それ以外の誰か（自分のスクリプトや
  Webhook など）は、ヘッダーなしでサーバー自身のアドレスに直接到達します。
- オリジンの確認は、サーバーの中で最初に行われます。`origin_secret` または `origin_secret_previous` の値でヘッダーを 1 つだけ持つリクエストを通し、
  ヘッダーがない場合は、ヘルスチェックへの `GET` と `HEAD`、`/_internal/` への `POST` だけを通します。
  すべてのリクエストからヘッダーを取り除き、残りを `403` で拒否します。
- 通ったリクエストは、Web クライアント、クライアント API、OAuth、ActivityPub、ヘルスチェック、`/_internal/`、
  または `/media/` と `/media/proxy/` に届きます。後の 2 つはメディアのストレージ（`blob = fs`、`s3`、`gcs`）を読むか、
  ほかのサーバーのファイルを中継します。拒否されたリクエストには `Cache-Control: no-store` 付きの `403` が返り、`my.http.origin_refused` に加算されます。

サーバーが公開のアドレスを持ち、プラットフォームでそのアドレスを CDN 以外のすべてに閉じられないときに使ってください。ファイアウォールやプラットフォームがすでに接続できる相手を制限しているとき、たとえばロードバランサーの後ろのプライベートネットワークでだけ待ち受けるサーバーには必要ありません。
そのロードバランサー自身が CDN のヘッダーを確認し、CDN のアドレスだけを受け入れるときも同じです。CloudFront の後ろの AWS Application Load Balancer は、リスナールールとマネージドプレフィックスリスト `com.amazonaws.global.cloudfront.origin-facing` でこれができます。その場合は、ロードバランサーと CDN のどちらも `X-Forwarded-For` に追記するので、両方のアドレスを `trusted_proxies` に並べてください。

Cloud Run のドメインマッピングのように、プラットフォーム自身の機能がトラフィックを転送し、自分で制御できるヘッダーを加えないときは役に立ちません。役に立つのは、リクエストヘッダーを加えられる CDN かロードバランサーだけです。

| 設定 | 持つもの |
| --- | --- |
| [`origin_secret`](../reference/configuration.md#origin_secret)（`MY_ORIGIN_SECRET`） | 値。表示可能な ASCII で 32 バイト以上。秘密情報で、環境変数か `MY_ORIGIN_SECRET_FILE` から読みます |
| [`origin_secret_header`](../reference/configuration.md#origin_secret_header)（`MY_ORIGIN_SECRET_HEADER`） | ヘッダーの名前。自分で決めます。`origin_secret` と一緒に必須です。CDN が設定を拒む名前や、`cf-` や `x-cf-` で始まる名前のようにサーバーやプロキシが読む名前は不正です |
| [`origin_secret_previous`](../reference/configuration.md#origin_secret_previous)（`MY_ORIGIN_SECRET_PREVIOUS`） | 受け入れる 2 つ目の値。[値を入れ替える間](#rotate-the-value)だけ使います |

どれも設定しなければ何も確認されず、サーバーはこの節がないものとして動きます。

<!-- source: what-passes-without-the-header 38e1ceda6e34 -->
### ヘッダーなしで通るもの

| リクエスト | 理由 |
| --- | --- |
| `/healthz`、`/readyz`、`/api/v1/instance/health` への `GET` または `HEAD` | プラットフォームのプローブや死活監視が直接呼び出すため |
| `/_internal/` のパスへの `POST` | Cloud Tasks と Cloud Scheduler がサーバー自身の URL で呼び出すため。これらは独自の署名付きトークンを持ちます |

それ以外はすべて、メンテナンスモードの間も含めてヘッダーが必要です。Web クライアントとそのアセット、`/media/`、クライアント API、OAuth、ほかのサーバーからの配送を含む ActivityPub が該当します。
そのため、CDN を通さずにサーバー自身のアドレスで直接呼び出すものは、自分の Webhook、音声のメディアサーバー、スクリプトなど、何でも拒否されます。それらの向き先は CDN のホスト名にしてください。

メトリクスのポート（`metrics_listen`）は公開されていない別のリスナーで、確認の対象になりません。

秘密の値を持たないリクエストは、通ったとしても、その後の処理に影響を与えられません。クライアントの IP は `X-Forwarded-For` が何であっても接続の相手になり、`traceparent` と `X-Request-Id` は無視されます。

<!-- source: set-it-up ee0a17ad2d33 -->
### 設定する

CDN がサーバーに転送するすべてのホスト名にヘッダーが必要です。`host`、`media_origin` のホスト、CDN が配信するなら別の `account_domain` もです。同じ CDN の後ろにあるメディアのホストにも設定します。`/media/` もほかと同じように拒否されるためです。

1. 値を生成し、ほかで使われていない名前を選びます。

   ```sh
   openssl rand -hex 32
   ```

2. **先に CDN でヘッダーを設定します。** サーバーは知らないヘッダーを無視するので、この段階ではまだ何も変わりません。
   - **Cloudflare**: リクエストヘッダーの変換ルール（Rules、Transform Rules、Modify Request Header の順）で、すべてのプランで使えます。ゾーンへのすべてのリクエストに一致させ、**Set static** を選んで名前と値を指定します。クライアントが送った値は置き換えられます。一部のホスト名だけのルールにすると、残りのホスト名は締め出されます。
   - **Amazon CloudFront**: 各ディストリビューションのオリジンに、オリジンのカスタムヘッダーを設定します。これもビューアーの値を置き換えます。
   - **Google Cloud のロードバランサー**: バックエンドサービスにカスタムリクエストヘッダーを設定します。
   - ほかの CDN やプロキシ: サーバーへのすべてのリクエストで、ヘッダーを追記ではなく設定する必要があります。
3. `MY_ORIGIN_SECRET` と `MY_ORIGIN_SECRET_HEADER` を設定してデプロイします。`my config check` はこれらを検証し、何が確認されるかを示しますが、CDN の状態は見えません。
4. 外から、オリジン自身のアドレスで確認します。

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                 # 200: through the CDN
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/       # 403: directly
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/healthz # 200: a health check
   ```

CDN のアドレスも [`trusted_proxies`](../reference/configuration.md#trusted_proxies) に並べてください。そうしないと、サーバーからはクライアントが 1 つに見え、すべてのリクエストが CDN のレート制限を共有します。`my config check` はこれを警告します。

拒否されたリクエストには、ヘッダーがどのように間違っていても、ほかの禁止されたリクエストと同じ `403` の problem document が `Cache-Control: no-store` 付きで返ります。応答にヘッダーの名前は一切含まれません。

<!-- source: when-everyone-is-locked-out 47c8f706e5e7 -->
### 全員が締め出されたとき

設定を誤ると、ヘルスチェックは `200` のまま、すべてのリクエストが拒否されます。そのため、プラットフォームからは正常なサーバーに見え、`/healthz` の死活監視も何も検出しません。兆候は次のとおりです。

- CDN を通したすべてのページが `403` になる。
- メトリクス `my.http.origin_refused` が増え、`http.server.request.duration` のリクエストが止まる。
- ログに、ステータス `403` で `http.route` のない `http request` の行が出る。ログとトレースには、ヘッダーもその名前も値も残りません。拒否されたリクエストはトレースのスパンを作りません。

復旧するには、確認なしでデプロイします。`MY_ORIGIN_SECRET` と `MY_ORIGIN_SECRET_HEADER` の設定を外してください。ほかのサーバーは配送を再試行しますが、その間、人はサーバーにアクセスできませんでした。
そのうえで、CDN の名前と値を設定と比べ、ルールがすべてのホスト名を対象にしていることを確認してから、もう一度設定してください。

<!-- source: rotate-the-value b88f695fde31 -->
### 値を入れ替える

CDN とサーバーを同時に変えることはできず、サーバーのレプリカもそれぞれのペースで再起動します。そのため、受け入れる値が 1 つだけだと、変更している間、両者の値が食い違う間ずっとトラフィックが拒否されます。そこでサーバーは、入れ替えの間は 2 つの値を受け入れます。

1. `MY_ORIGIN_SECRET` に新しい値を、`MY_ORIGIN_SECRET_PREVIOUS` に古い値を設定してデプロイします。どちらの値も通ります。
2. CDN の値を変え、その変更がすべての拠点に行き渡るのを待ちます。
3. `MY_ORIGIN_SECRET_PREVIOUS` の設定を外してデプロイします。設定されている間、`my config check` は警告します。

ヘッダーの名前は、この方法では入れ替えられません。名前を変えるには、サーバーの確認を一時的にオフにして[設定](#set-it-up)をやり直すか、デプロイする前に、CDN で古い名前と並べて新しい名前を設定してください。
