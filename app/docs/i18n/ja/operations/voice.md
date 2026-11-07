<!-- source: run-voice-chat 54808e78eb0d -->
# 通話を運用する

通話では、ログインしているユーザーがルームを開き、ほかの人を招待して話せます。音声の伝え方を選ぶまで、通話は無効です。
このガイドでは、その選び方と、それぞれの方法に必要な STUN サーバーと TURN サーバー、またはメディアサーバー、そしてデプロイ、プライバシー、モデレーションについて知っておくことを説明します。
ここで名前を挙げる設定は、すべて[設定のリファレンス](../reference/configuration.md#voice)で説明しています。

<!-- source: choose-how-the-audio-travels 9d60bd0655e9 -->
## 音声の伝え方を選ぶ

`voice` はインスタンス全体で 1 つの方法を選びます。すべてのルームがその方法を使います。

| `voice` | 音声を運ぶもの | 向いている場合 |
| --- | --- | --- |
| `off` | なし。通話の API は `404` を返し、Web クライアントには通話が表示されず、ほかの `voice_*` の設定は起動時に確認されません。 | 既定値です。 |
| `p2p` | 参加者のブラウザーが互いに直接音声を送ります。サーバーは、接続を確立するためのメッセージだけを中継します。 | 小さなルームと、いちばん安い構成。任意の STUN サーバーか TURN サーバーのほかに、追加のサーバーは要りません。 |
| `livekit` | LiveKit サーバー。セルフホストか LiveKit Cloud です。 | より大きなルームと、誰が話せるかの厳密な管理。 |
| `sora` | Sora サーバー。セルフホストか Sora Cloud です。 | 同上。すでに Sora を運用している場合。 |

![3 つの方法を並べた図。p2p では、サーバーがシグナルを中継し、ブラウザーは互いに、または TURN サーバーを通して音声を送ります。livekit と sora では、ブラウザーはシグナルと音声をメディアサーバーに送り、サーバーはそのメディアサーバーと API と Webhook でやり取りします。下の一覧が各方法を説明しています。](images/voice-methods.svg)

どの方法でも、サーバー（`my serve`）はルームと、その参加者とセッションを保持し、各参加者に grant（ブラウザーが接続するのに必要なもの）を
渡します。サーバーが音声を運ぶことはありません。違うのは、シグナリング（接続を確立するためのメッセージ）と音声の行き先です。

- **`p2p`。** ブラウザーは自分のシグナル（オファー、アンサー、ICE 候補）をサーバーに送り、サーバーはそれをルームのイベントストリームで
  相手のブラウザーに渡します。シグナルはキーバリューストアで最大 30 秒待ちます。音声は
  ブラウザーからブラウザーへ直接届きます。grant には、`voice_stun_urls` の STUN サーバーと、`voice_turn`（`coturn` か `cloudflare`）の TURN サーバーが、
  有効期間の短い認証情報とともに入っています。ブラウザーは STUN で自分の公開アドレスを知り、
  2 つのブラウザーが直接の経路を見つけられないときは TURN サーバーが音声を中継します。
- **`livekit`。** grant には `voice_livekit_url` と 60 秒有効のアクセストークンが入っています。ブラウザーは自分のシグナルと
  音声を LiveKit サーバーに送り、LiveKit サーバーは自前の STUN と TURN を持ち、音声をほかの参加者に転送します（selective forwarding unit、
  SFU）。
  サーバーは LiveKit の API で、LiveKit のルームを作成し、誰が話せるかを変更します。LiveKit は参加と
  退出を Webhook でサーバーに知らせます。
- **`sora`。** grant には `voice_sora_signaling_urls` と、署名された参加用の値が入っています。ブラウザーは自分のシグナルと
  音声を Sora サーバーに送り、Sora サーバーは自前の STUN と TURN を持っています。Sora は接続のたびに事前にサーバーの認証 Webhook に問い合わせ、
  サーバーは Sora の API でルームを管理し、Sora は接続を Webhook で知らせます。

選ぶときの注意点です。

- `p2p` では、音声がこのサーバーを通ることはありません（TURN サーバーを使うときは、TURN サーバーを通ります）。サーバーの負荷は、ルームのメッセージ、ハートビート、ストリームだけです。
- 小さなインスタンスには、`p2p` か、LiveKit Cloud の無料の Build プランが最適です（[LiveKit Cloud](#livekit-cloud) を参照してください）。
- `livekit` か `sora` では、下の STUN と TURN の設定と ICE のポリシーは使われません（設定すると、サーバーは起動時に警告します）。メディアサーバーが自前のサーバーを持っているためです。このとき、参加者の IP アドレスはメディアサーバーにだけ見え、ほかの参加者には見えません。
- ビルドタグ `no_voice_livekit` と `no_voice_sora` は、2 つのメディアサーバーの方法をバイナリーから除きます（[ビルドタグ](../reference/build-tags.md)）。
- インスタンスは HTTPS で配信してください。ブラウザーは、安全なページでしかマイクへのアクセスを許しません。

上限は、有料のメディアサーバーの費用に歯止めをかけます。`0` にすると、最後のものを除いて上限がなくなります。

| 設定 | 既定値 | 意味 |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | ルームが開いていられる最長の時間です。これに達したルームは終了します。 |
| `voice_max_open_rooms` | `10` | このサーバーで同時に開いていられるルームの数です。 |
| `voice_sfu_max_participants` | `10` | `livekit` と `sora` のルームの人数です。 |
| `voice_p2p_max_participants` | `10` | `p2p` のルームの人数です。2 から 50 まで指定できます。 |

ルームを開ける人は、インスタンスの設定 `voice_room_starters` で決まります（[インスタンスの設定](instance-settings.md)）。既定では、権限 `start_voice_rooms` を持つ人だけです（組み込みの `admin` ロールが持っています）。`everyone` にすると、ログインしているローカルユーザーのうち、凍結も制限もされていない人なら誰でもルームを開けます。

<!-- source: peer-to-peer-rooms 2199ca9a427c -->
## ピアツーピアのルーム

<!-- source: try-it-with-docker-compose ac3681681768 -->
### Docker Compose で試す

Docker Compose の構成（[`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)）で、自分のマシンで通話を試すには、
`.env` に `MY_VOICE=p2p` を設定してサーバーを作り直します。

```sh
cd deploy/docker
docker compose up -d
```

- インスタンスを `http://localhost:8080`（`example.env` の `MY_WEB_ORIGIN`）で開いてください。ブラウザーがページにマイクを使わせるのは
  `localhost` か HTTPS のときだけなので、`http://192.168.1.10:8080` のようなアドレスでは話せません。
- 同じマシンのブラウザー同士（2 つのブラウザーのプロファイル、または通常のウィンドウとプライベートウィンドウ）は、STUN サーバーも TURN
  サーバーもなしで接続します。ほかのマシンのブラウザーとつなぐには、インスタンスを HTTPS の後ろに置き、`.env` に `MY_VOICE_STUN_URLS` を設定してください。
- 既定では、ルームを開けるのは `admin` ロールだけです。作成した管理者としてログインするか、サーバー設定の「通話」のページで `voice_room_starters` を `everyone`
  にして、すべてのユーザーがルームを開けるようにしてください。

<!-- source: size-and-bandwidth 9872bf829e26 -->
### 人数と帯域

2 つのブラウザーは、少なくとも一方が話しているときに互いに接続します。聞いているだけの 2 人は接続しません。
そのため、話す人はほかの参加者それぞれに、自分の音声の別々のコピーを送ります。Opus の音声はオーバーヘッドを含めておよそ 30 から 50 kbit/s なので、10 人のルームで話す人はおよそ 9 つのコピー、数百 kbit/s をアップロードし、
50 人のルームで話す人は 49 のコピー、2 Mbit/s 前後をアップロードします。これは、モバイルデータ通信のスマートフォンや家庭の回線では維持できないことがあります。N 人のルームの接続の数は最大 N(N-1)/2 で、10 人なら 45、50 人なら 1,225 です。
そのため既定値は 10 です。`voice_p2p_max_participants` は、ユーザーにアップロードの帯域があるときにだけ上げてください。大きなルームには `livekit` か `sora` を使ってください。

<!-- source: stun-and-turn 51e427976a77 -->
### STUN と TURN

NAT の後ろのブラウザーには、自分の公開アドレスを知るための STUN サーバーと、直接の経路がないとき（厳しい NAT、企業のファイアウォール、一部のモバイルネットワーク）に音声を中継する TURN サーバーが必要です。

- `voice_stun_urls`: カンマ区切りの `stun:` か `stuns:` の URL。公開されている STUN サーバーならどれでも使え、下の coturn サーバーも STUN に応答します。
- `voice_turn`: `none`、`coturn`、`cloudflare` のいずれか。

どちらもない場合、サーバーは起動時に警告します。接続できるのは同じネットワークのブラウザーだけです。公開のインスタンスには STUN が、そして直接接続できないユーザーのために TURN が必要です。

サーバーは各参加者に有効期間の短い TURN の認証情報を渡し、共有のパスワードは決して渡しません。その有効期間は `voice_turn_credential_ttl`（既定値は `6h`）で決まります。Web クライアントは期限切れの 10 分前に新しい認証情報を求めて ICE を再開するので、期限が切れても通話は止まりません。

<!-- source: coturn d665e942c91b -->
#### coturn

`voice_turn = coturn`、`voice_turn_coturn_urls`、`voice_turn_coturn_secret` を設定します。サーバーは認証情報を自分で計算する（ユーザー名は有効期限とセッション ID、パスワードはそれをシークレットで計算した HMAC-SHA1）ので、TURN サーバーにリクエストを送りません。
coturn もそれに合わせて、`turnserver.conf` で設定します。

```conf
# Time-limited credentials signed with a shared secret.
use-auth-secret
static-auth-secret=<the same value as voice_turn_coturn_secret>
realm=turn.example.org

listening-port=3478
tls-listening-port=5349
# If the host is behind NAT (most cloud VMs), tell coturn its public address.
external-ip=203.0.113.10

# Do not let the relay reach your internal networks.
no-multicast-peers
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=100.64.0.0-100.127.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
```

- `denied-peer-ip` は重要です。これがないと、認証情報を持つ誰もが、TURN サーバーを使ってその内部ネットワークのホスト（クラウドのメタデータサービス、データベース、ほかの VM）に届いてしまいます。プライベート、ループバック、リンクローカルの範囲をすべて拒否し、内部のアドレス（`allowed-peer-ip`）は、本当にそこへ中継する必要があるときにだけ許可してください。
- UDP と TCP の 3478、`turns:` 用の TCP 5349、coturn の中継用ポートの範囲（`min-port` / `max-port`、既定では 49152-65535）を開けてください。
- `voice_turn_coturn_urls` には、`turns:` の URL か `?transport=tcp` 付きの URL も加えてください。UDP をふさぐネットワークのユーザーも接続できるようになります。
- coturn が認証情報の有効期限を確認するのは、アロケーションを最初に作るときだけです。すでにあるアロケーションは期限を過ぎても動き続け、新しい認証情報は次のアロケーションから有効になります。次のアロケーションは、Web クライアントが ICE を再開して作ります。

<!-- source: cloudflare-turn be5846573be8 -->
#### Cloudflare TURN

Cloudflare のダッシュボードで作成した TURN キーから、`voice_turn = cloudflare`、`voice_turn_cloudflare_key_id`、`voice_turn_cloudflare_api_token` を設定します。サーバーは、参加者がルームを開くか参加するときにその参加者の認証情報を Cloudflare に求め（失敗した場合、リクエストは `503` を返し、何も開かず参加もしないので、やり直せます）、参加者が退出するか退出させられたときに取り消します。

- 認証情報の有効期間は最大 48 時間なので、`voice_turn_credential_ttl` は `48h` を超えられません（超えると、サーバーは起動しません）。
- **Cloudflare は、認証情報の期限が切れると間もなく中継を切断します。** Web クライアントはその前に更新するので、`voice_turn_credential_ttl` は 10 分より十分に長くしておいてください。
- 料金（2026-10-26 時点で確認）: 月の最初の 1,000 GB は無料で、それ以降はクライアントに送った 1 GB あたり $0.05 です。当てにする前に、Cloudflare の現在の料金を確認してください。

<!-- source: hiding-ip-addresses de67ffa9e7b7 -->
### IP アドレスを隠す

`voice_ice_transport_policy = all`（既定値）では、ブラウザーはできるときには直接接続するので、ルームの参加者は互いの IP アドレスを知ることがあります。

`relay` では、ブラウザーは TURN サーバーだけを使うので、参加者に見えるのは TURN サーバーのアドレスで、ほかの参加者のアドレスは見えません。音声はブラウザー間で暗号化されたままです。`relay` が隠さないもの: TURN サーバーを運用する人（あなたか Cloudflare）にはすべての参加者のアドレスが見え、このサーバーには、その API を呼び出すすべての人のアドレスが見えます。
`relay` には、`voice_turn` が `coturn` か `cloudflare` であることが必要です（そうでなければ、サーバーは起動しません）。また、すべての通話が TURN サーバーを通るので、coturn ではあなたの帯域を、Cloudflare では転送料金を使います。

<!-- source: what-p2p-cannot-enforce 31e06affcd4f -->
### P2P で強制できないこと

ルームで話せるのは一部の参加者、つまりホストと、ホストが許可したリスナーだけです。`p2p` では、これを 2 か所で強制します。話せない人から音声を送ることになる接続のオファーを、サーバーが中継しないことと、話す人ではない参加者からの音声を、ほかのブラウザーが再生しないことです。

これで一般のユーザーと公式の Web クライアントは止められますが、完全な防御ではありません。**改変したクライアントを使う 2 人の参加者が協力すれば、それでも音声をやり取りできます**。話せない人の改変したクライアントが音声を送り、受け手の改変したクライアントがそれを再生でき、サーバーは暗号化されたメディアの中身を見られません。
誰が話すかを保証しなければならない場合（たとえば、モデレーションされた公開のイベント）は、`livekit` か `sora` を使ってください。そこでは、メディアサーバー自身が、許可のない人の音声を捨てます。

<!-- source: deployment-notes b0f03da306b7 -->
## デプロイの注意点

- **ユーザーごとに 2 つのストリーム**: 通話中の参加者は、アプリが常に開いているストリームに加えて、ルーム自身のイベントストリーム（Server-Sent Events）を保持します。接続数の上限は、通話中のユーザー 1 人あたり 2 つの長時間のリクエストを見込んで決めてください。
- **複数のプロセス**: アプリのほかの部分と同じく、共有のストリームアダプター（`stream` を `postgres`、`valkey`、`sns`、`pubsub` のいずれか）を使ってください。`stream = hub` で `role` が `all` 以外の場合、サーバーは起動時に、通話がプロセスをまたいで動かないことを警告します。
- **Cloud Run**: ストリームは 1 つのリクエストで、開いている間は CPU が割り当てられます。リクエストのタイムアウトで切れると、クライアントは再接続します。タイムアウトはできるだけ長くしてください。
- **メディアサーバーには VM が必要**: LiveKit と Sora は、長時間動き続けるプロセスで UDP の音声を受けます。Cloud Run は HTTP のリクエストしか受け付けないので、それらを動かせません。それら（と coturn）は VM で動かすか、LiveKit Cloud、Sora Cloud、Cloudflare TURN を使ってください。

<!-- source: livekit b16c1d192419 -->
## LiveKit

`voice = livekit` と、次の設定をします。

| 設定 | 値 |
| --- | --- |
| `voice_livekit_url` | ブラウザーが接続する `wss:` の URL。たとえば `wss://livekit.example.org`。 |
| `voice_livekit_api_url` | このサーバーが呼び出す `https:` の URL。上のものと違う場合に指定します。既定では、同じホストを `https:` にしたものです。 |
| `voice_livekit_api_key`、`voice_livekit_api_secret` | LiveKit サーバーのキーの組。シークレットは参加者のアクセストークンに署名し、Webhook を検証します。 |

サーバーは各ルームを LiveKit に作成してルーム ID を名前に付け、各参加者に 60 秒有効のアクセストークンを渡します。リスナーのトークンでは配信できず、話す人のトークンで配信できるのはマイクだけです。

<!-- source: self-hosted-livekit 2b118136f4dc -->
### セルフホストの LiveKit

`livekit.yaml` で次のように設定します。

```yaml
keys:
  <api key>: <api secret>
room:
  # Rooms are created only by this server.
  auto_create: false
  # Audio only.
  enabled_codecs:
    - mime: audio/opus
    - mime: audio/red
webhook:
  api_key: <api key>
  urls:
    - https://<your domain>/voice/webhooks/livekit
```

- LiveKit サーバーは、HTTPS で Webhook の URL に届かなければなりません。サーバーは、すべての Webhook の署名を API シークレットで検証します。
- Webhook は処理を速めるだけのものです。サーバーは 1 分ごとに、開いている各ルームの参加者を LiveKit が報告するものと比べ、いるべきでない人を退出させます。そのため、インスタンスがコールドの間に失われた Webhook も埋め合わされます。
- 自分のマシンで試すには、`livekit-server --dev` が固定の開発用キーの組で起動します。
- **退出させた参加者は、短い間なら再び参加できます。** セルフホストの LiveKit はトークンを取り消せません。接続しているクライアントには新しいトークンを送り続け、それぞれ最大でおよそ 10 分有効です。そのため、退出させられた人は、サーバーが気付いて（参加の Webhook か 1 分ごとの確認で）切断するまで、最大でおよそ 10 分の間、最新のトークンで再び参加でき、その間の数秒から 1 分ほど声が聞こえることがあります。退出をすぐに反映させなければならない場合は、LiveKit Cloud を使ってください。LiveKit Cloud はトークンをすぐに取り消します。

<!-- source: livekit-cloud 5bd9f833be11 -->
### LiveKit Cloud

プロジェクトの WebSocket の URL、API キー、シークレットを使い、プロジェクトの設定で `https://<your domain>/voice/webhooks/livekit` を Webhook のエンドポイントとして加えてください。
無料の Build プランでは、月に 5,000 分と 50 GB、同時接続 100 が使えます（2026-10-26 時点で確認）。現在のプランは LiveKit の料金のページで確認してください。

<!-- source: sora 5e9209dd3baa -->
## Sora

**Sora のアダプターは、Sora のドキュメントに従って模した Sora に対してだけテストしており、実際の Sora サーバーに対してはテストしていません。** 頼る前に、ステージングのインスタンスで実際の Sora と組み合わせて試してください。

`voice = sora`、`voice_sora_signaling_urls`（ブラウザーが接続する、カンマ区切りの `wss:` の URL）、`voice_sora_join_secret`（32 バイト以上。たとえば `openssl rand -base64 32` で作ります。参加者が参加に使う値に署名します）、`voice_sora_mode`（`self_hosted` か `cloud`）を設定します。
Sora は `https://<your domain>` に呼び返すので、インスタンスは HTTPS で配信しなければなりません。

<!-- source: webhooks 11b2c486a476 -->
### Webhook

Sora は、このサーバーの 2 つのエンドポイントを呼び出します。

| エンドポイント | 用途 |
| --- | --- |
| `/voice/webhooks/sora/auth` | 認証 Webhook。Sora は接続のたびに事前にこれを呼び出します。サーバーが許可するのは、ルームにいて、退出させられておらず、話す人として接続する場合は話すことを許可されていて、人数の上限内にいる参加者だけです。また、音声だけを受け付けるよう Sora に伝えます。 |
| `/voice/webhooks/sora/events` | イベントとセッションの Webhook（接続の作成と破棄、セッションの終了）。 |

**認証 Webhook は時間内に応答しなければなりません。** Sora が待つのは、セルフホストでは 5 秒、Sora Cloud では 10 秒です。サーバーはほかに何も呼び出さずに自分のデータから応答しますが、インスタンスのコールドスタート（Cloud Run のゼロからのスケール）ではそれを超えることがあり、そのとき接続は拒否されます。通話が使われている間は、インスタンスを 1 つ温めておいてください。

セルフホストでは、`sora.conf` に `default_duplicate_client_id = evict` を設定してください。話す人になったリスナーは、同じ ID で再接続します（Sora は接続の役割を変えられません）。`evict` にすると、新しい接続が届いたときに Sora が古い接続を切ります。
既定の `allow` では、サーバーが新しい接続を知った時点で古い接続を自分で切るので、2 つが一瞬重なります。

<!-- source: self-hosted-sora dd3244368b73 -->
### セルフホストの Sora

| 設定 | 値 |
| --- | --- |
| `voice_sora_mode` | `self_hosted` |
| `voice_sora_api_url` | Sora の HTTP API の前に置いたリバースプロキシの `https:` の URL。 |
| `voice_sora_api_header_name`、`voice_sora_api_header_value` | プロキシがこのサーバーを認証するためのヘッダー。たとえば `Authorization` と `Basic ...`。 |
| `voice_sora_webhook_username`、`voice_sora_webhook_password` | Webhook の Basic 認証の認証情報。必須です。 |

**Sora 自身の HTTP API には認証がなく**、ループバックアドレスでだけ待ち受けることを前提にしています。決して公開しないでください。認証情報を求めるリバースプロキシをその前に置き、その認証情報を 2 つのヘッダーの設定でこのサーバーに渡してください。
Sora の Webhook の URL には、同じ Basic 認証の認証情報を設定してください。サーバーは `https:` の API の URL だけを受け付け（`http:` は localhost に限ります）、Webhook の認証情報がないと起動しません。Cloud Run と ECS の前のロードバランサーはサーバーの手前で TLS を終端するので、mTLS には対応していません。

<!-- source: sora-cloud ac4935c7f7a7 -->
### Sora Cloud

`voice_sora_mode = cloud`、`voice_sora_cloud_project_id`、`voice_sora_cloud_api_key` を設定します。サーバーはこのキーで、アクセストークンに署名し、API を呼び出し、Webhook の署名を検証します。Basic 認証の認証情報は要りません。

<!-- source: behavior-to-expect 03fd5e1655fa -->
### 想定しておく動作

- 話すことを許可されたリスナーは、自動で話す人として再接続するので、新しい接続が始まるまで少し途切れます。
- 権利を取り上げられた話す人は、転送のフィルターですぐに音声が止められ、そのあとリスナーとして再接続します。

<!-- source: licensing 534e304817e3 -->
### ライセンス

Sora は時雨堂の商用製品です。2026-10-26 時点で確認した内容: Sora Cloud には月額 ¥5,000 の基本料金があり、無料期間はありません。セルフホストにはパッケージライセンスが必要で、同時接続数と期間を決めて前払いします。製品のページには、日本国外の企業、団体、個人には提供していないとあるので、計画を立てる前に購入できるかを確認してください。LiveKit は、セルフホストならオープンソースです。

<!-- source: ending-rooms-and-records b4d3d196ecd7 -->
## ルームの終了と記録

- `manage_reports` を持つモデレーターは、通話が無効の間も、開いているルームを全員に対して終了できます（[モデレーション](moderation.md#voice-chat-rooms)）。
- ルームに誰が参加したか（参加、話す人とリスナー、挙手、招待と退出）は、ルームの終了後 `voice_participants_retention_days` 日（既定値は 90、最低 30）の間、またはルームを指す通報がある間、保持します。ルームの名前、説明、ホスト、どのように終了したかは保持します。音声、マイクの状態、リアクションは決して記録しません。
- `local` のルームに参加できない訪問者が、そのページとリンクプレビューでルームの名前と開いているかどうかを見られるかは、`voice_public_summary` で決まります（[インスタンスの設定](instance-settings.md)）。

<!-- source: notes-for-users f44150f15915 -->
## ユーザー向けの注意点

- iPhone や iPad では、アプリがバックグラウンドにある間、音声の送信が止まることがあります。iOS は前面にない Web ページを停止させることがあるためです。通話中はアプリを画面に開いたままにしてください。
- ブラウザーは、初めてのときにマイクを使う許可を求めます。拒否した場合は、ブラウザーのサイトの設定でもう一度許可してください。
- ピアツーピアのルームでは、インスタンスが通話を中継経由にしていない限り、ほかの参加者があなたの IP アドレスを知ることができる場合があります。気になる場合は運営者に尋ねてください。
