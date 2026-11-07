<!-- source: federation-relays bbd5bb98dde3 -->
# 連合のリレー

リレーは、参加しているサーバーの公開投稿を、ほかの参加サーバーに中継するサーバーです。フォローの少ない小さなインスタンスでも、連合タイムラインやハッシュタグのタイムラインが埋まり、
自分の公開投稿もより遠くまで届きます。リレーに参加しないインスタンスでは、行もジョブも増えません。

<!-- source: join-a-relay 147b07e65534 -->
## リレーに参加する

管理画面の「リレー」(`/admin/relays`) を使います。権限 `manage_settings` が必要で、組み込みのロール `admin` はこの権限を持っています。
「リレーに参加する」で、リレーを登録するフォームが開きます。一覧には各リレーの状態と、脱退またはもう一度参加するためのボタンが表示されます。
リレーを選ぶとそのページが開き、アドレス、直近 24 時間に取り込んだもの、リレーを削除するボタンが表示されます。
CLI は `my admin relay` です (`my admin relay -help`)。

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. リレーの URL を登録します。プロトコルは 2 つあります。
   - **Mastodon**: リレーの **inbox URL** (`https://relay.example/inbox` など)。pub-relay や Activity-Relay を含む多くのリレーがこれを受け付けます。
   - **LitePub**: リレーの **actor URL** (`https://relay.example/relay` など)。Pleroma と Akkoma のリレーのプロトコルです。
   プロトコルを選ばない場合 (「自動で判定」)、サーバーは URL が actor のドキュメントを返すときは LitePub を、そうでないときは Mastodon を使います。
2. 登録すると、インスタンスのアクターがリレーに `Follow` を送り、状態は「リレーの承認待ち」(`pending`) になります。
   リレーが承認すると状態は「参加中」(`accepted`) に、拒否すると「リレーに拒否されました」(`rejected`) になります。リレーによっては、運営者が申請を 1 件ずつ手で承認するため時間がかかります。
3. 登録するときに「リレーから投稿を受け取る」をオフにすると、サーバーは送るだけになります。これを変えるには、リレーから脱退してもう一度登録してください。

ドメインへの措置 (`suspend`) を受けているサーバーのリレーは登録できません。参加中のリレーのサーバーがあとで停止された場合、状態は変わりませんが、
そのリレーとは何も送受信せず、管理画面に警告が表示されます。

<!-- source: what-the-server-sends 1db52c508f9c -->
## サーバーが送るもの

- 送るのは、ローカルの**公開** (`public`) 投稿の作成・編集・削除だけです。`unlisted`、フォロワー限定、ダイレクト、ローカル限定 (`local`) の投稿と、ブーストは送りません。
  投稿をリレーに流したくないときは、`unlisted` か `local` で投稿してください。ユーザーごとのオプトアウトはありません。
- 公開投稿のドキュメントには Linked Data Signature (`RsaSignature2017`) が付くので、リレーがそのまま中継した投稿を Mastodon や Misskey が受け付けられます。
- 配送の数は、参加中のリレーの数と公開投稿の数の積だけ増えます (1 つのリレーへの配送は 1 件にまとめられます)。

<!-- source: what-the-server-receives ff962150d1a7 -->
## サーバーが受け取るもの

- 参加中のリレーが運んできた公開投稿は、取り込む前に元のサーバーからもう一度取得して確認します。そのあと連合タイムラインとハッシュタグのタイムラインに表示されます。ホームタイムラインには表示されず、通知も作りません。
- 取り込むのは、リレーごとに 1 時間あたり最大 `relay_max_activities_per_hour` 件 (既定値 1,000) の投稿です。上限を超えた分は取得せずに捨てます。
  管理画面のリレーのページと `my admin relay list` に、直近 24 時間に取り込んだ数と、上限を超えて捨てた数が表示されます。捨てた数が多いときは、上限を上げるかリレーを見直してください。
- ドメインやアカウントに `suspend` (連合の制限) または `limit` (公開の場所から除く制限) がかかっている投稿は取り込みません。
- リレー経由でだけ届いた投稿は、インスタンス設定 `relay_status_retention_days` (既定値 7) の日数がたつと公開の場所から除かれます。ローカルの人が返信・ブースト・お気に入り・ブックマークした投稿と、
  ローカルの人がフォローしている投稿者の投稿は、ほかのリモートの投稿と同じ保持期間 (`remote_status_retention_days`) になります。
- 取り込みには、投稿 1 件につき元のサーバーからの取得が 1 回かかります (知らない投稿者ならアクターの取得も加わります)。上限の 1 時間 1,000 件なら、1 日に最大 24,000 回の取得です。

設定は[設定リファレンス](../reference/configuration.md#relay_max_activities_per_hour)にあります。取り込みの結果はメトリクス `my.relay.activities` としても出力されます ([サーバーの運用](running.md)を参照)。

<!-- source: leave-join-again-and-delete ea12da94f940 -->
## 脱退・再参加・削除

- 「脱退する」は `Follow` の `Undo` を送り、状態を「脱退済み」(`left`) にします。以後そのリレーとは何も送受信しません。すでに取り込んだ投稿は、保持期間が終わると除かれます。
- 「もう一度参加する」は、サーバーを拒否したリレーやサーバーが脱退したリレーに、新しい `Follow` を送ります。
- 「削除する」は、脱退したリレーや拒否されたリレーの登録を消します。すでにリレーに送った投稿の編集や削除は、リレーを削除したあとも配送されます。

登録・参加・脱退・削除と、リレーによる承認や拒否は、対象の種類「リレー」として監査ログに記録されます (承認と拒否はリモートの操作として記録されます)。
