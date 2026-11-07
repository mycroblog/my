<!-- source: instance-settings 4eb467f0dbca -->
# インスタンスの設定

インスタンスの運営のしかたを決める設定です。登録の方式、投稿の長さ、既定のロケール、タイムラインを見られる人、インスタンスの名前、説明、規約、ルール、テーマ、カスタム絵文字があります。

<!-- source: configuration-and-instance-settings a0b0908cef1e -->
## 構成とインスタンスの設定

| | 構成 | インスタンスの設定 |
| --- | --- | --- |
| 対象 | デプロイの組み立て方: ロール、アダプター、エンドポイント、ホスト名、シークレット | インスタンスの運営のしかた: 登録の方式、投稿の長さの上限、既定のロケール、タイムラインを見られる人、インスタンスの名前、説明、規約 |
| 置き場所 | 環境変数 `MY_*` と構成ファイル（[設定リファレンス](../reference/configuration.md)） | データベース。変更はすべてリビジョンとして残ります |
| 変更 | プロセスを再起動すると反映されます | `my instance set`、Web クライアントの管理画面、クライアント API ですぐに反映されます（[管理画面とクライアント API](#admin-screens-and-the-client-api)） |
| 未設定のとき | スキーマの既定値（必須のキーなら、サーバーは起動しません） | コードで定めた既定値 |

- **両方にあるキーはありません。** どちらかがもう一方を上書きすることはありません。インスタンスの設定を環境変数や構成ファイルで与えることはできず、構成の値をデータベースに保存することもできません。
- デプロイごとに固定で、プロセスの構造を変える値は構成です。インスタンスの運営中に変わる値はインスタンスの設定です。

<!-- source: prerequisites 12391490eea0 -->
## 前提条件

- [設定リファレンス](../reference/configuration.md)のとおりに構成が与えられていること（`my config check`）。
- データベースにスキーマが適用されていること（`my schema apply`。[データベース](database.md)を参照）。

<!-- source: keys ae25d36239c2 -->
## キー

<!-- source: settings-one-key-one-value 4f827bf97480 -->
### 設定（1 つのキーに 1 つの値）

| キー | 型 | 既定値 | 説明 |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | 登録の方式（[登録](registration.md)）。サービスの終了を告知している間と終了したあと、および[一人のサーバー](#one-person-server)では、実際の値は `closed` になります。保存された値は変わらず、告知を取り消すか `single_user` をオフにすると元に戻ります。`GET /instance` と NodeInfo は実際の値を示します |
| `max_status_length` | 整数 1-100000 | `500` | 投稿の長さの上限（文字数） |
| `default_locale` | BCP 47 の言語タグ | `en` | 既定のロケール。新しい利用者の最初の UI のロケールであり、利用者の希望に合うカタログがないときの代わりにもなります |
| `local_timeline_visibility` | `off` / `signed_in` / `everyone` | `everyone` | ローカルタイムライン（`GET /api/v1/timelines/local`）を見られる人: 誰も見られない、ログインしている利用者、誰でも。`off` は 404 を返し、`signed_in` はログインしていない訪問者に 401 を返します。どちらの場合もナビゲーションには表示されません。値が `everyone` でなければ、ランディングページに投稿の見本は表示されません。[一人のサーバー](#one-person-server)では `off` として扱われます |
| `federated_timeline_visibility` | `off` / `signed_in` / `everyone` | `signed_in` | 連合タイムライン（`GET /api/v1/timelines/federated`）を見られる人。値の意味はローカルタイムラインと同じで、一人のサーバーではこちらも `off` として扱われます |
| `preview_image` | メディア ID（空 = 未設定） | 空 | 画像のない投稿やプロフィールのリンクプレビューの画像（`og:image`）。未設定のときは同梱の既定の画像を使います（[リンクプレビュー](link-previews.md)） |
| `contact_account` | ローカルアカウントのユーザー名（空 = 未設定） | 空 | 運営者への連絡先のアカウント。[連絡先とサムネイル](#contact-and-thumbnail)を参照してください |
| `contact_email` | メールアドレス（空 = 未設定） | 空 | 運営者への連絡先のメールアドレス。サーバーが送るメールの `Reply-To` と署名にも使われます |
| `thumbnail` | メディア ID（空 = 未設定） | 空 | インスタンスを表す画像（サムネイル） |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | リモートのメディアの複製を保持するかどうか。`none`: メディアプロキシがすべてのリクエストを中継します。`avatars`: アバター、ヘッダー、カスタム絵文字を `remote_media_cache_ttl_seconds` の間 blob ストレージにキャッシュします。`all`: リモートのメディアをすべてキャッシュします。キャッシュの合計サイズは構成 `remote_media_cache_max_bytes` で制限されます |
| `remote_media_cache_ttl_seconds` | 整数 60-31536000（秒） | `86400` | 取得したリモートのメディアを保持してよい期間。メディアプロキシの応答の `Cache-Control: max-age` にもなるので、CDN やブラウザーもこの期間キャッシュします |
| `remote_media_cache_max_bytes` | 1 以上の整数（バイト） | `1073741824` | メディアプロキシが保持するリモートのメディアの複製のサイズの上限。1 時間ごとの掃除で、期限切れの複製を削除し、そのあと上限を超えた分を最後にアクセスされたのが古いものから削除します |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch（セキュアモード）。`true` のとき、検証できる HTTP 署名のない ActivityPub の GET には、オブジェクトとコレクションでは `401` を返し、利用者のアクターでは鍵を含む最小限の文書を返します。インスタンスのアクターは常に文書全体を返すので、どのサーバーも署名の検証のために鍵を取得できます |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | 連合の方向。`off`: 連合しません。`inbound`: リモートのアカウントをフォローして読めますが、ローカルの投稿、プロフィール、リアクションは送り出さず、外部からのフォローは拒否します（`Reject`）。`outbound`: 外部からのフォローを受け入れてローカルの投稿を配送しますが、リモートの投稿とリアクションは取り込みません。`both`: 送信も受信もします。変更はすぐに反映され、権限 `manage_settings` が必要です |
| `dynamic_client_registration` | `true` / `false` | `true` | 動的クライアント登録（RFC 7591）。`true` の間は、`POST /oauth/register` が認証なしでパブリッククライアントの登録を受け付けます。`false` のときは、このエンドポイントがすべてのリクエストを `403`（`access_denied`）で拒否します。登録済みのクライアントは引き続き使えます。定期ジョブが、構成 `dynamic_client_unredeemed_retention` と `dynamic_client_idle_retention` に従って、使われていない動的登録のクライアントを削除します。変更はすぐに反映され、`manage_settings` が必要です |
| `domain_actions_visibility` | `off` / `signed_in` / `everyone` | `off` | 措置を取ったドメインの一覧（`GET /api/v1/instance/domain_actions`）を見られる人: 誰も見られない、ログインしている利用者、誰でも（[モデレーション](moderation.md#publishing-the-domains-you-act-on)） |
| `domain_actions_rationale_visibility` | `off` / `signed_in` / `everyone` | `off` | その一覧の公開コメントを見られる人。一覧を見られない人がコメントを見ることはありません |
| `appeal_window_days` | 整数 20-365（日） | `180` | 措置のあと、当事者が異議を申し立てられる日数（[モデレーション](moderation.md#appeals)）。変えると、すでに取った措置の期限も変わります |
| `session_origins_retention_days` | 整数 90-3650（日） | `180` | ログインの IP アドレスと User-Agent を、ログインのあと保持する日数（[モデレーション](moderation.md#record-retention-and-holds)） |
| `registration_origins_retention_days` | 整数 90-3650（日） | `180` | 登録の IP アドレスを、登録のあと保持する日数 |
| `reports_retention_days` | 整数 180-3650（日） | `365` | 通報を、最後の解決のあと保持する日数。措置が参照している通報は、その措置と同じ期間保持します |
| `moderation_actions_retention_days` | 整数 180-3650（日） | `365` | アカウントへの措置と異議申し立てを、措置の終わり（解除、期限切れ、異議申し立ての審査のうち遅いほう）のあと保持する日数 |
| `audit_log_retention_days` | 整数 365-3650（日） | `730` | 監査ログの行を、記録のあと保持する日数 |
| `remote_status_retention_days` | 整数 1-3650（日） | `90` | このインスタンスで何も参照していない（ローカルの返信、ブースト、お気に入り、ブックマーク、通知、ローカルの投稿を含むスレッドがない）リモートの投稿を、公開のあと保持する日数 |
| `remote_status_max_retention_days` | 整数 0-3650（日） | `0` | このインスタンスが参照していても、リモートの投稿を公開のあと保持する最長の日数。ローカルの返信、ブースト、お気に入り、ブックマークは残り、その投稿を削除済みとして表示します。`0` は上限なしです |
| `relay_status_retention_days` | 整数 1-3650（日） | `7` | リレーだけが運んできたリモートの投稿を、このインスタンスで何も参照しておらず、ローカルのアカウントが誰もその作者をフォローしていないときに保持する日数 |
| `oauth_client_rate_limits` | カンマ区切りの `client_id=requests` の組 | 空 | サーバーに負荷をかけるクライアントを抑えるための、OAuth クライアントのクライアント API リクエストの上限です。そのクライアントの利用者全員を合わせて 5 分間に送れるリクエスト数を指定します。変更の反映には最大 1 分かかります |
| `search_indexing` | `allow` / `deny` | `deny` | 検索エンジンがインスタンスをインデックスしてよいかどうか。`allow` でも、インデックスされるのは `indexable` のアカウントのプロフィールと `public` の投稿だけです（[検索エンジンと AI のクローラー](crawlers.md)） |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | AI サービスへの意思表示。`deny_training` は学習を拒否し、`deny` は学習と AI による利用（AI 検索、アシスタント、エージェント）を拒否します |
| `indexable_default` | `true` / `false` | `false` | 新しいアカウントの `indexable` の既定値。変えても既存のアカウントは変わりません |
| `auto_delete_default_enabled` | `true` / `false` | `false` | 新しいアカウントが、古い投稿の自動削除をオンにした状態で始まるかどうか。ほかの `auto_delete_default_*` のキーと合わせて、新しいアカウントが受け取るポリシーになります（[古い投稿の自動削除](#automatic-deletion-of-old-posts)） |
| `auto_delete_default_min_age_days` | 整数 1-3650（日） | `30` | 新しいアカウントのポリシーが、公開から何日後に投稿を削除するか |
| `auto_delete_default_keep_pinned`, `auto_delete_default_keep_bookmarked`, `auto_delete_default_keep_favourited` | `true` / `false` | `true` | 新しいアカウントのポリシーが、作者がピン留め、ブックマーク、お気に入りした投稿を残すかどうか |
| `auto_delete_default_keep_media`, `auto_delete_default_keep_polls` | `true` / `false` | `false` | 新しいアカウントのポリシーが、メディアや投票のある投稿を残すかどうか |
| `auto_delete_default_keep_public`, `auto_delete_default_keep_local`, `auto_delete_default_keep_unlisted`, `auto_delete_default_keep_private` | `true` / `false` | `false` | 新しいアカウントのポリシーが、その公開範囲の投稿を残すかどうか |
| `auto_delete_default_keep_direct` | `true` / `false` | `true` | 新しいアカウントのポリシーが、ダイレクトの投稿を残すかどうか |
| `auto_delete_default_keep_min_boosts`, `auto_delete_default_keep_min_favourites` | 整数 0-1000000 | `0` | 新しいアカウントのポリシーは、この回数以上ブーストまたはお気に入りされた投稿を残します。`0` はしきい値なしです |
| `auto_delete_max_age_days` | 整数 0-3650（日） | `0` | ローカルの投稿を公開のあと保持する最長の日数（[上限](#a-maximum-for-keeping-posts)）。`0` は上限なしです |
| `auto_delete_max_allow_pinned` | `true` / `false` | `true` | 上限のもとでも、ピン留めした投稿を残してよいかどうか |
| `auto_delete_max_kept_pins` | 整数 1-5 | `5` | 上限のもとで投稿を残すピン留めの数。最近ピン留めしたものから数えます。アカウントのピン留めの最大数である `5` なら、ピン留めした投稿をすべて残します |
| `auto_delete_max_allow_bookmarked`, `auto_delete_max_allow_favourited`, `auto_delete_max_allow_media`, `auto_delete_max_allow_polls` | `true` / `false` | `false` | 上限のもとでも、作者がブックマークまたはお気に入りした投稿や、メディアや投票のある投稿を残してよいかどうか |
| `auto_delete_max_allow_visibilities` | `true` / `false` | `false` | 上限のもとでも、公開範囲によって投稿を残してよいかどうか |
| `auto_delete_max_allow_thresholds` | `true` / `false` | `false` | 上限のもとでも、ブーストとお気に入りの数によって投稿を残してよいかどうか |
| `auto_delete_grace_days` | 整数 0-365（日） | `30` | 新たに上限がかかるアカウントが、知らせを受けてから自分の設定を保てる日数。変えても、すでに知らせを受けたアカウントの日付は動きません |
| `search_visibilities` | `public` / `local` / `public,local` | `public` | 検索バックエンドのあるサーバーで、全文検索の対象にする投稿。`unlisted`、`private`、`direct` の投稿は対象にならず、`indexable` のアカウントだけが対象です。公開範囲を外すと、その投稿がインデックスから削除されます。公開範囲を加えても、[インデックスを作り直す](search.md#changing-search_visibilities)までは、その公開範囲の新しい投稿しか見つかりません。`search = none` のときは何もしません |
| `security_contact` | `https:` または `mailto:` の URI（空 = 未設定） | 空 | このサーバーのセキュリティ上の問題（構成や運用のしかた、アカウントやデータ）の報告先。設定すると、サーバーは `/.well-known/security.txt`（RFC 9116）を配信します。これはソフトウェアではなくあなたの連絡先です。ソフトウェア自体の脆弱性の報告は、[セキュリティポリシー](../../SECURITY.md)のとおりにプロジェクトへ転送してください |
| `require_media_description` | `true` / `false` | `false` | ローカルの投稿に添付するメディアのファイル（画像、動画、音声）すべてに説明を必須にするかどうか。`true` のとき、説明がないか空白だけのファイルを含む投稿や編集は 422 で拒否されます。`GET /api/v1/instance` の `limits.require_media_description` として公開され、Web クライアントは説明の欄を必須にします。送信前に確認するだけの利用者ごとの設定 `require_media_description` とは別のものです |
| `link_cards` | `true` / `false` | `true` | 投稿のリンクにカードを表示するかどうか。`true` のとき、サーバーはリンク先のページを取得してカードを作ります。`false` のときは取得をやめ、すでに作ったカードも隠します。一部のドメインでだけ止めるには、[リンクのカードのドメイン](#link-card-domains)を参照してください |
| `link_cards_ignore_robots_txt` | `true` / `false` | `false` | リンクのカードの取得で `robots.txt` を無視するかどうか。`false` のとき、サーバーはリンク先のサイトの `robots.txt` に従うので、`my-linkcard` か `*` を拒否しているサイトにはカードが付きません（[リンクプレビュー](link-previews.md#cards-of-links-in-posts)）。`/about/crawler` がリンク先のサイトにそのクローラーが何かを伝えます。`true` のとき、サーバーは `robots.txt` を取得も使用もせず、リンク先のサイトはそれでこの取得を拒めなくなります。それでもサーバーは `my-linkcard` と名乗ります。カードが欠けすぎるときにだけオンにしてください。オンの間に作ったカードは、オフにしたあとの次の取得し直しのときに確認されます。ページの指示とリンクのカードのドメインは、どちらの場合も適用されます |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | Client ID Metadata Document のクライアントを受け入れるかどうか。`true` のとき、`client_id` が `https` の URL である認可リクエストについて、サーバーはその URL のメタデータを取得し、それをクライアントとして扱います（事前の登録は不要です）。取得はリダイレクトをたどらず、読み込むのは最大 5 KB で、応答はキャッシュの指示に従って最長 24 時間再利用します。`false` のときは取得をやめ、これらのクライアントの認可リクエスト、トークンの交換、更新を拒否します |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | 運営費（`/admin/finance/costs` で記録し、`/admin/finance/publication` で設定します）をどこまで公開するか。`private`: 何も公開しません（`GET /api/v1/instance/cost_totals` は 404 です）。`totals`: 期間と通貨ごとの合計だけ。`breakdown`: 合計と、プロバイダーとカテゴリーごとの内訳。情報ページ（`/about` と `/about/costs`）と API で誰にでも表示されます。`breakdown` では、どのプロバイダー（たとえば、どのクラウド）で運用しているかがわかります。どの段階でも、項目の名前、メモ、個々の支払いは公開されません。今日（UTC）を含む期間までが公開されます |
| `finance_viewer_item_names` | `true` / `false` | `false` | 権限 `view_finance` を持つ利用者が、会計の画面（`/finance`、`GET /api/v1/finance/cost_totals`）で費用の項目の名前と、取り込んだ費用のサービス名を見られるかどうか。合計と、プロバイダーとカテゴリーごとの内訳は、この設定にかかわらず常に表示されます |
| `finance_viewer_item_memos` | `true` / `false` | `false` | `view_finance` を持つ利用者が、会計の画面で費用の項目のメモを見られるかどうか。名前かメモの設定のどちらかが `true` なら、各期間に項目の行（プロバイダー、カテゴリー、その期間の金額と、有効なら名前とメモ）が表示されます。`manage_finance` を持つ人は、これにかかわらずすべてを見られます |
| `finance_viewer_income` | `totals` / `plans` | `totals` | `view_finance` を持つ利用者が、収入の合計だけ（`totals`）を見るか、プランごとの加入者数も（`plans`）見るか。プランごとの金額と、誰が支払ったかは表示されません。`income_visibility` が `plans` なら、公開している数はこの値にかかわらず表示されます。収入は Web クライアントの会計の画面（`/finance`）と `GET /api/v1/finance/income_totals` で見られます |
| `income_visibility` | `private` / `totals` / `plans` | `private` | サポーターからの収入（決済プロバイダーの手数料を引いたあとの金額）をどこまで公開するか。`finance_visibility` とは別に選びます。`private`: 何も公開しません（`GET /api/v1/instance/income_totals` は 404 です）。`totals`: 期間と通貨ごとの収入の合計だけ。`plans`: プランごとの現在の加入者数も、誰にでも表示します。加入者の少ないプランでは、その数から誰が加入しているかを推測されることがあります。収支（収入から費用を引いたもの）は、この値も `finance_visibility` も `private` でないときにだけ表示されます。そうでなければ、収支から隠した側がわかってしまうためです。どの段階でも、個々の支払い、加入者、プランごとの金額、手数料を引く前の金額と手数料は公開されません。今日（UTC）を含む期間までが公開されます |
| `supporter_list_visibility` | `off` / `signed_in` / `everyone` | `off` | サポーターの一覧（`GET /api/v1/instance/supporters` と `/about/supporters`）を見られる人。`off`: 誰も見られません（API は 404 を返し、`/about` にリンクはありません）。`signed_in`: このサーバーにログインしている人だけ（それ以外には API が 401 を返します）。`everyone`: 誰でも。一覧には、掲載を選んだ有効な加入者のアカウントとプランだけが表示されます。金額、月払いか年払いか、日付、人数は表示されません。一覧のページと API の応答は、`search_indexing` にかかわらず常に `noindex, noarchive` です |
| `user_app_registration` | `true` / `false` | `true` | ログインしている利用者が、クライアント API（`POST /api/v1/oauth_apps`）で自分の OAuth アプリ（パブリックまたはコンフィデンシャル）を登録できるかどうか。`false` のとき、登録には 403 を返します。登録済みのアプリの一覧、変更、削除、シークレットの再発行、自分へのトークンの発行、ほかの利用者による認可は引き続き使えます |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | 構成 `voice` が `off` でないときに、通話のルームを開ける人（[構成](../reference/configuration.md#voice)）。`everyone`: 凍結も制限もされていない、ログインしているローカルの利用者全員。`permitted`: 権限 `start_voice_rooms` を持つ人だけ。この権限は既定では組み込みの `admin` ロールだけが持ち、サポーターのプランのロールに持たせることもできます |
| `voice_public_summary` | `true` / `false` | `false` | 公開範囲が `local` の通話のルームのページで、このサーバーにログインしていない訪問者とリモートの利用者に、ルームの名前、説明、ホスト、まだ開いているかどうかを表示し、リンクプレビューにも出すかどうか。参加者は表示されません。`false` のとき、そうした訪問者には、このサーバーのローカルの利用者向けのルームであることだけが表示されます |
| `public_counts` | `true` / `false` | `true` | ランディングページ（`/`、ログアウト時）と `GET /api/v1/instance/overview` に、サーバーのメンバー数、直近 7 日の各日にサーバーの利用者がした公開投稿の数、サーバーの開始日を表示するかどうか。数えるのはこのサーバーの利用者の公開投稿だけで、数は数分ごとに更新されます。`false` のとき、API はこれらを含めません |
| `featured_accounts` | スペースかカンマで区切った、最大 20 個のローカルアカウントのユーザー名（空 = なし） | 空 | フォローする人のおすすめで、最近投稿したこのサーバーの人より先に、この順で表示するアカウント。持ち主が「おすすめに出し、名前の一部で見つかるようにする」をオフにしたアカウントはおすすめされません。[おすすめ](#suggestions-of-accounts)を参照してください |
| `login_providers_only` | `true` / `false` | `false` | 有効なログインのプロバイダーが 1 つ以上ある間、登録とログインをログインのプロバイダーに限るかどうか。パスワードとパスキーではログインできなくなり、プロバイダーを連携していない利用者は、セッションが切れると再びログインできなくなります。有効なプロバイダーがなければ適用されません。`my instance set` は、有効なプロバイダーと連携していない有効な利用者の数を表示します。[ログインをログインのプロバイダーに限る](login-providers.md#limit-sign-in-to-login-providers)を参照してください。 |
| `invite_creators` | `everyone` / `permitted` | `permitted` | `manage_invites` を持つスタッフのほかに、自分の招待を作れる人（[メンバーの招待](registration.md#member-invites)）。`everyone`: 凍結も制限もされていない、ログインしているローカルの利用者全員。`permitted`: 権限 `create_invites` を持つ人だけ。この権限は既定では組み込みの `admin` ロールだけが持ち、サポーターのプランのロールに持たせることもできます。`approval` の方式では、使える招待があると新しい利用者がすぐに承認されるので、`everyone` にすると全員が審査を飛ばせるようになります |
| `member_invite_max_active` | 整数 0-100 | `5` | 1 人のメンバーが同時に持てる、使える招待（取り消されておらず、期限切れでも使い切ってもいないもの）の数。`0` ではメンバーは招待を作れません。スタッフの招待は数えません |
| `member_invite_max_uses` | 整数 1-1000 | `10` | メンバーが作った 1 つの招待で登録できる人数 |
| `member_invite_max_lifetime_seconds` | 整数 3600-31536000（秒） | `2592000`（30 日） | メンバーが作った 1 つの招待が使える最長の期間。下げても既存の招待は短くなりません |
| `translation_providers` | カンマ区切りのプロバイダー ID | 空 | 有効にするサーバー側の翻訳のプロバイダー。先頭のものから試します。空のときは、どのプロバイダーにも何も送りません。このサーバーにプロバイダーのない ID は何もしません。組み込みの[翻訳のプロバイダー](translation.md)の ID は `libretranslate`、`deepl`、`google`、`aws` で、[翻訳のプラグイン](plugins.md#translating-posts)の ID は `plugin` です |
| `translation_provider_visibilities` | カンマ区切りの `provider=visibility\|visibility` の組 | 空 | 各プロバイダーが受け取ってよいもの。名前の挙がっていないプロバイダーは `public`、`local`、`unlisted` の投稿を受け取ります。後ろに何もない `provider=` は何も受け取りません。`private` と `direct` は、それとともに名前の挙がったプロバイダーにだけ送られるので、あるプロバイダーに許しても次のプロバイダーには許したことになりません |
| `translation_browser` | 真偽値 | `false` | 読む人が、ブラウザーに組み込まれた翻訳を使ってよいかどうか。この翻訳は、このサーバーにもプロバイダーにも何も送りません。サーバー側のプロバイダーも有効なときはプロバイダーが優先され、サーバーが断ったもの（どのプロバイダーも受け付けない投稿や言語、月の上限、失敗したプロバイダー）をブラウザーが翻訳します |
| `translation_user_monthly_characters` | 整数 0-1000000000000 | `50000` | 1 人の利用者のリクエストが、UTC の 1 か月に翻訳のプロバイダーへ送れる文字数。`0` では何も送りません。無制限の値はないので、大きな数を設定してください |
| `translation_server_monthly_characters` | 整数 0-1000000000000 | `500000` | サーバー全体が、UTC の 1 か月に翻訳のプロバイダーへ送れる文字数。`0` では何も送りません。無制限の値はないので、大きな数を設定してください |
| `translation_retention_days` | 整数 1-3650（日） | `30` | 翻訳を、作ってから保持する日数。読んでも延びず、投稿とともに削除されます |
| `translation_reports_to_server` | 真偽値 | `false` | モデレーターが、通報の文章（通報された投稿と通報者のコメント）をサーバー側のプロバイダーに送ってよいかどうか。誰が頼んでも、各プロバイダーが受け取るのは `translation_provider_visibilities` が許した公開範囲だけです。通報されたダイレクトの投稿は `direct` を許されたプロバイダーにだけ送られ、コメントは `private` として扱われます。コメントの翻訳も、削除された投稿の翻訳も保持されず、文字数は `translation_server_monthly_characters` にだけ数えられます。`false` のときは、通報の内容がプロバイダーに届くことはありません |
| `voice_participants_retention_days` | 整数 30-3650（日） | `90` | 通話のルームが終わってから、誰が参加したかの記録（参加、話し手と聞き手、挙手、招待、退出させた記録）を保持する日数。ルーム自体（名前、説明、ホスト、どう終わったか）は保持されます。通報されたルームの記録は通報と同じ期間保持され、保全はほかの記録と同じように適用されます |
| `hook_inbound_status_mode`, `hook_inbound_follow_mode`, `hook_signup_check_mode`, `hook_media_check_mode`, `hook_local_status_mode`, `hook_inbound_actor_mode`, `hook_local_profile_mode` | `off`, `observe`, `enforce` | `observe` | プラグインがある間に、各[判定フック](decision-hooks.md#modes)がどこまで働くか。`off` から `enforce` へは `observe` を経てしか移れません。 |
| `hook_inbound_status_on_failure`, `hook_inbound_follow_on_failure`, `hook_media_check_on_failure`, `hook_inbound_actor_on_failure` | `pass`, `retry` | `pass` | 強制しているときにプラグインが失敗したら、フックがどうするか（[詳細](decision-hooks.md#when-the-plugin-fails)）。 |
| `hook_inbound_status_unfollowed_direct` | 真偽値 | `false` | `inbound.status` が、ローカルのアカウントが誰もフォローしていない作者のダイレクトの投稿も送るかどうか（[詳細](decision-hooks.md#what-inboundstatus-reviews)）。フックを使っている間は、情報ページ（`/about`）にそのことが表示されます。 |
| `hook_signup_check_on_failure` | `pass`, `require_approval` | `pass` | 強制しているときにプラグインが失敗したら、`signup.check` がどうするか。 |
| `hook_signup_check_reject` | 真偽値 | `false` | 強制している間、`signup.check` の `reject` で[登録を拒否する](decision-hooks.md#sign-ups)かどうか。オフのときは、`observe` と同じように答えを記録するだけです。 |
| `hook_local_status_account_age_days` | 整数 0-3650（日） | `7` | `local.status` は、作ってからこの日数に満たないアカウントの[投稿とその編集を審査します](decision-hooks.md#local-posts)。`0` ではすべてのアカウントを審査します。フックを使っている間は、情報ページ（`/about`）にそのことが表示されます。 |
| `hook_local_profile_account_age_days` | 整数 0-3650（日） | `7` | `local.profile` は、作ってからこの日数に満たないアカウントの[プロフィールの変更を審査します](decision-hooks.md#local-profiles)。`0` ではすべてのアカウントを審査します。フックを使っている間は、情報ページ（`/about`）にそのことが表示されます。 |
| `hook_decisions_retention_days` | 整数 30-3650（日） | `90` | [フックの判定](decision-hooks.md#the-decisions)を保持する日数。 |
| `single_user` | 真偽値 | `false` | サーバーを[一人のサーバー](#one-person-server)にするかどうか。サーバーのローカルアカウントがちょうど 1 つのときにだけオンにでき、オフにはいつでもできます |

- 値は保存の前に正規化されます（`TRUE` は `true` に、`0750` は `750` に、`ja_jp` は `ja-JP` になり、メディア ID は小文字になります）。
  無効な値は拒否され、何も書き込まれません。
- `preview_image` と `thumbnail` は、存在する画像のメディア（処理中のものを含む）だけを受け付けます。
  処理が終わるまでと、処理に失敗した画像の代わりには、`preview_image` は既定の画像を使います。
  一度どこかのリビジョンで設定されたメディアは、添付されていないアップロードの掃除で削除されません。
- `contact_account` は、削除されていないローカルの利用者のユーザー名だけを受け付けます（小文字で保存されます）。`contact_email` は `alice@example.com` の形のアドレスだけを受け付けます。
- `featured_accounts` は、削除されていないローカルの利用者のユーザー名だけを受け付けます。先頭の `@` は取り除かれ、名前は小文字になり、重複した名前は 1 つにまとめられ、1 つのスペースで区切って保存されます。

<!-- source: suggestions-of-accounts b7d362466e33 -->
### アカウントのおすすめ

ログインしている利用者には、フォローする人のおすすめが表示されます（登録後の空のホームタイムラインと、`GET /api/v1/accounts/suggestions`）。おすすめは、まず `featured_accounts` のアカウントをその順に並べ、続いて直近 30 日に公開またはローカルの投稿をしたこのサーバーのアカウントを、最後に投稿したものから並べます。利用者自身のアカウント、利用者がフォロー、ブロック、ミュートしているアカウント、利用者をブロックしているアカウント、凍結または無効化されたアカウントは除かれます。

新しいアカウントはおすすめに出て、アカウントの検索で名前の一部から見つかります。プロフィールの設定で「おすすめに出し、名前の一部で見つかるようにする」をオフにした利用者は、その両方から外れます。そのあと検索でそのアカウントが見つかるのは、ユーザー名全体で探したときか、そのアカウントをフォローしている人が探したときだけです。

<!-- source: automatic-deletion-of-old-posts f0cf962c9de0 -->
### 古い投稿の自動削除

利用者は、自分の投稿を、公開から何日かたったときに自動で削除させることができます（設定の「古い投稿の自動削除」）。ポリシーは利用者ごとのもので、オンにするかどうか、何日後か、どの投稿を残すか（ピン留め、ブックマーク、お気に入りした投稿、メディアや投票のある投稿、選んだ公開範囲の投稿、一定数以上のブーストやお気に入りがある投稿）を利用者が決めます。この方法での投稿の削除は、作者が自分で削除するのと同じです。投稿が届いたサーバーには `Delete` が送られます。この方法で削除されるのはローカルのアカウントの投稿だけで、ブーストは削除されません。

`auto_delete_default_*` のキーは、新しいローカルアカウントが、どの方法（登録のフォーム、外部のログインのプロバイダー、招待、`my admin`）で作られたかにかかわらず、最初に持つポリシーです。アカウントは作られたときにこれを受け取り、その後は自分のものとして持ちます。あとでキーを変えても、既存のアカウントも、前の値を受け取ったアカウントも変わりません。設定の画面には、その値が利用者自身の値として表示されます。キーが未設定なら、新しいアカウントは自動削除がオフ、30 日後、ピン留め、ブックマーク、お気に入りした投稿とダイレクトの投稿を残す状態で始まります。既定値はインスタンスの外の人には表示されません。上限が設定されているときは、新しいアカウントは、下で説明するとおり上限に合わせて切り詰めた既定値を受け取るので、上限の内側から始まります。

<!-- source: a-maximum-for-keeping-posts b1d83e01feda -->
#### 投稿を保持する上限

`auto_delete_max_age_days` が `0` より大きいと、どのローカルの投稿も、公開からその日数を超えて保持されません。
権限 `keep_posts_longer` を持たないローカルのアカウントすべてについて、次のようになります。

- 自動削除は常にオンです。利用者はオフにできず、上限より多い日数も選べません（少ないのはかまいません）。設定の画面と `PUT /api/v1/preferences/auto_delete` はどちらも拒否します。
- 投稿を残すのは、`auto_delete_max_allow_*` のキーが許した例外だけです。既定では、ピン留めした投稿だけがすべて残ります。`auto_delete_max_kept_pins` が 5 より小さいときは、最近のピン留めのうちその数だけが投稿を残します。
- 自動削除をオフにした利用者や、一度も設定していない利用者にも、上限がかかります。

上限をオンにしても、下げても、例外を変えても、誰かの設定が書き換えられることはありません。その代わり、1 時間ごとにジョブ `content.auto_delete_statuses` が、上限では残らない投稿を自分の設定で残しているアカウントを見つけます（上限がオンになったか下がったか、アカウントが `keep_posts_longer` を失ったためです）。それぞれのアカウントに一度だけ、既定でオンになっている通知とメールで、そのことと、上限がいつからかかるか（知らせの `auto_delete_grace_days` 後）を伝えます。それまではアカウント自身の設定が有効で、それ以降はその投稿は上限に従います。再び上限の内側に入ったアカウント（利用者が設定を変えた、`keep_posts_longer` を得た、上限がなくなったか上がった）は対象から外れ、のちの変更があると、新しい猶予期間とともに改めて知らせを受けます。

上限をオンにすると、対象になるすべてのアカウントに 1 時間以内に知らせが届きます。削除のペースは一定です。1 つのアカウントにつき 1 時間に最大 100 件の投稿で、それぞれ投稿が届いたサーバーに `Delete` として送られるので、古い投稿の多いアカウントが空になるまでには数時間から数日かかります。その間に利用者が設定を変えると、残りの削除は止まります。

`keep_posts_longer` はメンバーの権限です。ロールに与えると、その持ち主は投稿を好きなだけ保持でき、自動削除をオフにできます。組み込みの `admin` ロールはこの権限を持っています。[サポーターのプラン](supporters.md)にも持たせられるので、投稿を長く保持できることをプランの特典にできます。加入が終わると、猶予期間のあとで再び上限がかかります。

上限は公開されています。`GET /api/v1/instance` は `auto_delete_limit` でそれを返し、概要のページにも表示されるので、登録する前に知ることができます。これらのキーの変更には `manage_settings` が必要で、ほかの設定と同じように監査ログに記録されます。Web クライアントでは「サーバーの設定」の「投稿の自動削除」のページにあり、オンにした上限や厳しくした上限を保存する前に確認を求めます。

<!-- source: texts-one-key-one-value c639fc7dfe23 -->
### テキスト（1 つのキーに 1 つの値）

| キー | 上限 | 説明 |
| --- | --- | --- |
| `name` | 100 文字、1 行（空白だけは不可） | インスタンスの名前（表示名）。[名前とドメイン](#name-and-domain)を参照してください |
| `short_description` | 500 文字 | 短い説明 |
| `description` | 20000 文字 | 説明 |
| `terms` | 100000 文字 | 利用規約（Markdown） |
| `privacy_policy` | 100000 文字 | プライバシーポリシー（Markdown） |
| `commerce_disclosure` | 100000 文字 | 有料のプランについての販売者の表示。たとえば日本の法律が求める表記です（Markdown。[サポーター](supporters.md)） |
| `preview_image_alt` | 1500 文字 | `preview_image` の画像の代替テキスト（`og:image:alt`）。未設定のときは「{instance name} の画像」を使います |
| `thumbnail_alt` | 1500 文字 | `thumbnail` の画像の代替テキスト |

- 各テキストの値は 1 つです。管理者は好きな言語で一度書き、どのロケールの読者にも同じ値が表示されます。ほかの言語の読者は機械翻訳などに頼ります。管理画面の欄のラベルと説明は、ほかの画面の文言と同じように見る人の言語に翻訳されます。翻訳されないのは管理者が書いた値だけです。
- サーバーはテキストの言語を知らないので、Web クライアントとサーバーが描画する画面は、画面の言語を引き継がずに、`dir="auto"` と `lang=""`（言語が不明であることを表す HTML の値）でテキストを出力します。
- テキストを空にすると削除されます。
- テキストは CLI のほかに、Web クライアントの管理画面（名前、短い説明、説明は「サーバーの設定」の「全般」のページ `/admin/settings`、利用規約は「規約とポリシー」の最初のページ `/admin/documents`、プライバシーポリシーは `/admin/documents/privacy-policy`、販売者の表示などは `/admin/documents/commerce-disclosure`）と、クライアント API（`GET /api/v1/admin/instance_texts`、`PUT /api/v1/admin/instance_texts/{key}`）でも変えられます。
  どちらにもロールの権限 `manage_settings` が必要で、変更は監査ログに記録されます（`text_changed`）。

<!-- source: terms-and-privacy-policy 82e7bc325cf1 -->
### 利用規約とプライバシーポリシー

- `terms`、`privacy_policy`、`commerce_disclosure` は Markdown（CommonMark に、GitHub Flavored Markdown の表、取り消し線、自動リンクを加えたもの）で書きます。段落の中の改行は改行のまま残り、`#` の見出しはページのタイトルの 1 つ下の階層（`h2`）から始まります。
- サーバーがそれらを HTML に変換してサニタイズし（匿名の `GET /api/v1/instance/documents/{kind}`）、Web クライアントのページ `/about/terms`、`/about/privacy`、`/about/commerce` が表示します。生の HTML、画像、`http`、`https`、`mailto` 以外のリンクは取り除かれます。ページを見るのにログインはいりません（Web クライアントのページなので、JavaScript は必要です）。
  決済プロバイダーに伝える URL は `/about/commerce` です。
- 書いた文書は、登録の画面、サーバーが描画するすべての画面のフッター、Web クライアント（3 列目、メニュー、設定、ログインの画面）からリンクされます。
  書いていない文書はリンクされず、その API は 404 を返します。
- 利用規約かプライバシーポリシーが書かれている間は、登録の画面の作成ボタンの前にそれらが並び、必須のチェックボックス「上の文書を読み、同意します」が表示されます。サーバーは、これのない登録を、ログインのプロバイダーを通じた登録も含めて拒否します。どちらの文書もなければ、画面は同意を求めません。

<!-- source: name-and-domain 1e14aa3e6c58 -->
### 名前とドメイン

インスタンスには、ドメインと名前が別々にあります。

- **ドメイン**は構成の `host`（とハンドルのドメインである `account_domain`）です。インスタンスの運用中は変えられません。名前があっても、ハンドル（`@alice@example.com`）、URL、ActivityPub の ID、`GET /api/v1/instance` の `domain` はドメインのままです。
- **名前**はテキストの `name` です。インスタンスを人に向けて名乗るところすべてに表示されます。Web クライアントのサイドバーと「このサーバーについて」、ブラウザーのタブのタイトル、サーバーが描画するログイン、登録、メールアドレスの確認、アカウントの状態、認可の同意の画面、メール（メールアドレスの確認とモデレーターへの通知）、リンクプレビューの `og:site_name`、Web App Manifest の `name` / `short_name`、NodeInfo の `metadata.nodeName`、`GET /api/v1/instance` の `name` です。
- 名前はどのロケールでも同じです。名前がないときは、代わりにドメイン（`host`）が表示されます。
- 変更は再起動なしで次のリクエストから反映されます。開いている Web クライアントのタブのタイトルとサイドバーは、管理画面で保存したときに更新されます。

<!-- source: contact-and-thumbnail 71513ab82d51 -->
### 連絡先とサムネイル

運営者への連絡先（`contact_account`、`contact_email`）と、インスタンスを表す画像（`thumbnail` とテキストの `thumbnail_alt`）は、次の場所に表示されます。未設定のものは表示されません。

- Web クライアントの「このサーバーについて」（`/about`）。サムネイルと連絡先のアカウントは、ログインしていない人向けの最初の画面（`/`）にも表示されます。
- `GET /api/v1/instance` の `contact`（`account` と `email`）と `thumbnail`（`url`、`width`、`height`、`alt`）。
- NodeInfo の `metadata`: Misskey の慣習による `maintainer`（`name` は連絡先のアカウントの表示名で、ないときはユーザー名です。`email` は `contact_email` です）、Pleroma と Akkoma の慣習による `staffAccounts`（連絡先のアカウントのアクター ID）、`thumbnail`（`url` と `alt`）、短い説明 `short_description` から作る `nodeDescription`。

連絡先のアカウントが凍結または削除されたときは、アカウントは表示されず、メールアドレスだけが表示されます。
これらは、Web クライアントの管理画面の「全般」のページ（「サーバーの設定」、`/admin/settings`）か、CLI かクライアント API で変えます。

<!-- source: one-person-server 6ba524c58a4f -->
### 一人のサーバー

運営者ひとりだけのサーバー、つまりフェディバースにおける運営者自身の住まいが、一人のサーバーです。`single_user` をオンにします。Web クライアントでは、管理者が **サーバーの設定** の **登録** のページにある **一人のサーバーにする** でオンとオフを切り替えます。ローカルアカウントが 2 つ以上あるときは、そこにサーバーが断った理由が表示されます。

```sh
my instance set single_user true
my instance set single_user false   # back to a community server
```

- オンにできるのは、数に入るローカルアカウントがちょうど 1 つのときだけです。削除されていないローカルアカウントはすべて数に入ります。承認待ち、無効化、削除予定、凍結、制限中のアカウントも、それぞれ 2 人目として戻ってくることがあるので含まれます。登録を却下されたアカウントは数に入りません。それ以外の数のときは、`my instance set` がその数とともに失敗し、管理 API は型 `/api/v1/problems/single-user-requires-one-account` の `422` を返します。
  サーバーのインスタンスのアクターはアカウントではないので、数に入ることはありません。
- オフにはいつでもできます。オンにしてもオフにしても、何も削除されず、ほかの設定の保存された値も変わりません。オフにすると、抑えていたものが元どおりに再び適用されます。ほかの設定と同じく、変更は監査ログに記録されます。

オンの間は、次のようになります。

- **登録は閉じます。** `registration_mode` は保存された値にかかわらず `closed` として扱われるので、登録のページ、ログインのプロバイダーを通じた登録、`my admin create` ではアカウントが作られません。ログインのプロバイダーでのログインは引き続き使えます。
- **ホームタイムラインだけになります。** `local_timeline_visibility` と `federated_timeline_visibility` は `off` として扱われます。ハッシュタグとリストのタイムラインは残ります。
- **コミュニティに必要なものは隠れます。** ローカルと連合のタイムラインとそのストリーム、アカウントのおすすめ、利用者と管理者の招待、登録の審査、登録のブロックは、型 `/api/v1/problems/single-user-mode` の `404` を返します。
- **新しいローカルの投稿はできません。** 一人のサーバーでは、「このサーバー」の投稿はフォロワー以外の誰にも届かないので、新しい投稿をローカルにはできません。クライアント API は `422` を返し、アカウントが投稿できる公開範囲からもそれを外します。既定の公開範囲が「このサーバー」なら「フォロワー」として扱われ、保存はできません。このサーバーの人向けの通話のルームは、フォロワー向けの投稿で告知されます。すでにローカルの投稿はローカルのままです。
- **サーバーは自分が何かを示します。** `GET /api/v1/instance` の `single_user` は `true` になり、実際の登録の方式とタイムラインと、`single_user_account` として運営者のアカウントが含まれます。NodeInfo は `openRegistrations: false` と `metadata.singleUser: true` を返します。
- **トップページは運営者のプロフィールになります。** ログインしていない人がサーバーのアドレスを開くと、その唯一のアカウントのプロフィールが、ログインの方法と、サーバーが公開している概要のページ、利用規約、プライバシーポリシーへのリンクとともに表示されます。Web クライアントは、サーバーが提供しないものを、ナビゲーション、メニュー、管理画面から隠します。ローカルと連合のタイムライン、登録、おすすめのアカウント、招待、登録の審査、登録のブロック、公開範囲「このサーバー」、そして登録、招待、公開のタイムライン、ランディングページ、おすすめのアカウントの設定です。

連合、モデレーション、全文検索、サポーターの仕組み、文書、管理画面は、ほかのサーバーと同じように使えます。

<!-- source: usage 68ad8384438f -->
## 使い方

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- フラグはキーと値の前に書きます。
- `-owner <username>` を付けると、`preview_image` や `thumbnail` の値は画像ファイルのパスとして読まれ、そのローカルアカウントのメディアとしてアップロードされます。画像は、使われる前にワーカーが処理します。1200x630 の横長の画像をおすすめします。
- 変更は、1 つのトランザクションでリビジョン（`instance_*_revisions`）を記録し、現在の値（`instance_settings` / `instance_texts`）を更新します。CLI からの変更は、操作者なしで記録されます（履歴の `BY` は `cli` です）。
- 同じ値をもう一度設定しても、リビジョンは追加されます。
- 終了コード: 成功は 0、値の拒否またはデータベースのエラーは 1（何も変わりません）、引数の誤りは 2 です。

<!-- source: changing-the-direction-of-federation 9b9b63689c84 -->
### 連合の方向を変える

- 方向を狭めても、フォローの関係、取り込んだ投稿、配送の記録は残り、`Undo` も `Reject` も送りません。流れが止まるだけで、`both`（またはほかのモード）に戻すと元のとおりに再開します。
- 狭めている間にした投稿やリアクションは、あとで広げ直しても、さかのぼって配送されません。
- リモート側では、モードが `inbound` の間はローカルの利用者のフォロワーに何も届かず、`outbound` または `off` の間はローカルの利用者のフォローが更新されません。
- モードが `off` の間にした削除（投稿やアカウントの削除）は送り出されません。`inbound` と `outbound` では、すでに送り出したものの取り消し（`Delete` など）は、元の受信者とその時点のフォロワーに届きます。
- 変更は `instance_setting_revisions` と監査ログ（`target_kind = setting`）に記録されます。

関係も断つには、CLI の `my admin federation sever` を使います。

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- ローカルとリモートのアカウントの間の、承認待ちと有効なフォローを、モデレーションによる終了（`follow_terminations`、`reason = moderation`）として終わらせます。モードを変えても、これが自動で呼ばれることはありません。
- リモート側に `Reject`（`in`）や `Undo`（`out`）が送られるのは、現在のモードがその方向に送るときだけです。送らないモード（`off` など）では、フォローはローカル側でだけ終わり、リモート側には知らされません。
- 繰り返し実行しても安全です。終わらせたフォローの数を標準出力に表示します。

<!-- source: admin-screens-and-the-client-api 6a747f50af97 -->
## 管理画面とクライアント API

CLI で変えられる設定は、すべて Web クライアントの管理画面とクライアント API でも変えられます。どちらにもロールの権限 `manage_settings` が必要です（組み込みの `admin` ロールだけが持っています）。CLI と同じく、変更はリビジョンとして残り、キーごとに監査ログに記録されます（`setting_changed`。対象はその設定で、操作者はその利用者です）。監査ログは、モデレーションの画面（「監査ログ」、`/moderation/audit-log`）で確認してください。

- **Web クライアント**（「サーバーの設定」、`/admin/settings`）: 画面は話題ごとに 1 つのページに分かれ、見出しの下にリンクがあります。各ページにはフォームと保存ボタンが 1 つずつあり、保存すると、そのページで変えたものだけが送られます。ほかのページの設定はそのままです。

  | ページ | セクションと設定 |
  | --- | --- |
  | 全般（`/admin/settings`） | 名前と説明（`name`、`short_description`、`description`）、連絡先とサムネイル（`contact_account`、`contact_email`、`thumbnail`、テキストの `thumbnail_alt`） |
  | アイコンとロゴ（`/admin/settings/branding`） | アイコン、ロゴ、暗い背景のロゴ。それぞれ専用のボタンで保存します（[ロゴとアイコン](#logo-and-icon)） |
  | 登録（`/admin/settings/registration`） | 登録（`registration_mode`、`login_providers_only`）、メンバーの招待（`invite_creators`、`member_invite_max_active`、`member_invite_max_uses`、`member_invite_max_lifetime_seconds`）、アプリ（`user_app_registration`、`oauth_metadata_document_clients`、`dynamic_client_registration`） |
  | コンテンツ（`/admin/settings/content`） | 言語（`default_locale`。アプリにある言語から選びます）、タイムライン（`local_timeline_visibility`、`federated_timeline_visibility`）、投稿（`require_media_description`）、ランディングページ（`public_counts`）、おすすめのアカウント（`featured_accounts`） |
  | 投稿の自動削除（`/admin/settings/auto-delete`） | 新しいアカウントの既定値（`auto_delete_default_*` の設定）、上限（`auto_delete_max_age_days`、`auto_delete_max_allow_*` の設定、`auto_delete_max_kept_pins`、`auto_delete_grace_days`）。オンにした上限や厳しくした上限（日数、例外、ピン留めを減らす）を保存するときは、新たに対象になるアカウントに通知が届くので、先に確認を求めます（[投稿を保持する上限](#a-maximum-for-keeping-posts)） |
  | 連合（`/admin/settings/federation`） | 連合（`federation_mode`、`authorized_fetch`、`remote_media_policy`、`remote_media_cache_ttl_seconds`（1 時間、1 日、7 日、30 日、365 日から選びます）、`link_cards`、`link_cards_ignore_robots_txt`）、制限・凍結したサーバー（`domain_actions_visibility`、`domain_actions_rationale_visibility`）、外部の審査（`hook_*` の設定。[判定フック](decision-hooks.md)を参照） |
  | 検索とクローラー（`/admin/settings/discovery`） | 検索エンジンと AI（`search_indexing`、`indexable_default`、`ai_usage`、`security_contact`）、投稿の検索（`search_visibilities`。サーバーに全文検索があるときだけ） |
  | 通話（`/admin/settings/voice`） | `voice_room_starters`、`voice_public_summary`、`voice_participants_retention_days`。このページは、サーバーが通話を提供しているときだけ一覧に出ます |

  「全般」のページでは、画像は選んだときにアップロードされ、保存したときにサムネイルになります。運営費、収入、サポーターの一覧を見られる人は、「会計」と「サポーター」の画面で設定します。

  `max_status_length`、`preview_image`、`appeal_window_days`、保持期間（`*_retention_days`）、`remote_media_cache_max_bytes`、`oauth_client_rate_limits` は画面に表示されません。
  CLI かクライアント API で変えてください。翻訳の設定には、下のとおり専用の画面があります。
- **Web クライアント**（「翻訳」、`/admin/translation`）: このサーバーにあるプロバイダー、その順番と各プロバイダーが受け取ってよいもの、ブラウザーの翻訳、月の上限、翻訳の保持期間を、今月送った文字数とキャッシュのヒット率とともに扱います。各サーバーのプロセスは、キャッシュのヒット数を最大 1 分に 1 回ヒット率に加えるので、最新のリクエストが表示されるまでに 1 分かかることがあり、止まったプロセスの最後の 1 分は数えられないことがあります。サーバー側のプロバイダーを有効にする前に、プライバシーポリシーを更新してください（[ひな形](privacy-policy-template.md#the-services-you-choose)）。プロバイダーに送った文章は取り戻せません。プロバイダーの API キーと[翻訳のプラグイン](plugins.md#translating-posts)のアドレスは[デプロイの構成](../reference/configuration.md)にあります。
  どの設定にも、クライアント API のどの応答にも含まれません。
- **翻訳の上限。** 数えるのは送った文字数（コードポイント単位）です。メンションやリンクなどの代わりに置く目印は数えず、サーバーがすでに保持している翻訳、すでに読む人の言語で書かれた投稿、ブラウザーの翻訳も数えません。プロバイダーへの呼び出しは、失敗したものも含めて 1 回ずつ数えます。プロバイダーが文章を受け取っている可能性があるためです。そのため、この数はプロバイダーの請求と異なることがあります。上限を超えると、リクエストは `429` で拒否され、プロバイダーは呼ばれません。そのとき Web クライアントは、許可されていて、ブラウザーがその投稿を翻訳できるなら、読む人のブラウザーの翻訳を使います。月は UTC で数えます。
- **クライアント API**: `GET /api/v1/admin/settings` は、すべての設定を、現在の値、既定値、設定されているかどうか、型と受け付ける値（選択肢か範囲）とともに返します。`PATCH /api/v1/admin/settings` は、`{"values": {"<key>": "<value>", ...}}` で設定をまとめて変えます。値はすべて文字列で、CLI と同じように正規化して保存されます。キーが 1 つでも不明か、値が無効なら、そのキーを示す 422 を返し、何も変わりません。変えたキーは、同じトランザクションでリビジョンと監査ログに記録されます。
- `open`、`approval`、`invite` の方式で使える招待は、モデレーションの画面（「招待の一覧」、`/moderation/invites`、権限 `manage_invites`。[招待コード](registration.md#invite-codes)を参照）で作るほか、`invite_creators` が許す範囲でメンバーも作れます（[メンバーの招待](registration.md#member-invites)）。

<!-- source: rules b1076bb81e0c -->
## ルール

インスタンスのルールは `GET /api/v1/instance` の `rules` に含まれ、通報はその ID でルールを挙げます。

- ルールには 1 行の文（最大 300 文字）と、任意の補足（最大 1,000 文字）があります。テキストと同じく、管理者は好きな言語で書き、どのロケールにも同じ値が表示されます（`dir="auto"` と `lang=""`）。
  有効なルールは最大 100 個です。
- 変更（文の修正、並べ替え）はすべてリビジョン（`instance_rule_revisions`）として残り、ルールの ID は変わりません。
- 廃止したルールは、有効なルールの一覧から消えます。元に戻すことはできませんが、通報からは引き続き参照できます。

```sh
my instance rule create -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- 管理 API は `/api/v1/admin/rules`（一覧、追加、取得、修正、移動、廃止）で、ロールの権限 `manage_rules` が必要です。
- Web クライアントの管理画面（「ルール」、`/admin/rules`、`manage_rules`）では、一覧に有効なルールが順に表示され、「上へ」と「下へ」で並べ替えます。一覧の上のフィルターで、代わりに廃止したルールを表示できます。「ルールを追加する」で、文と補足を入力するページが開きます。ルールの文を選ぶとそのページが開き、そこで編集と廃止（確認のダイアログのあと）ができます。変更はすべて監査ログに記録されます（`rule_added`、`rule_revised`、`rule_moved`、`rule_retired`）。
- 終了コードは設定と同じです（成功は 0、値の拒否またはデータベースのエラーは 1、引数の誤りは 2）。

<!-- source: link-card-domains c8f674387fa8 -->
## リンクのカードのドメイン

投稿のリンクのカード（`link_cards`）はクローラーが作ります。サーバーがリンク先のページとその画像を取得します。ルールは、1 つのドメインとそのサブドメインでこのクローラーを止め、そのドメインについてのほかのことには手を付けません。ルールには 2 つのモードのどちらかがあります。ルールは運営者の選択で、リンク先のサイト自身が拒否していることに加えて適用されます（[リンクプレビュー](link-previews.md#cards-of-links-in-posts)）。

| モード | 取得するもの | すでにあるカード |
| --- | --- | --- |
| 取得を止める（`no_fetch`） | そのドメインからは、リダイレクトでたどり着く場合も含めて、ページも画像も何も取得しません | 隠します |
| 画像だけ止める（`no_images`） | ページは取得しますが、そのドメインの画像は取得しません | 画像なしで表示します |

- ルールが関係するのはリンクのカードだけです。そのドメインとの連合は続き、その投稿のメディアもいつもどおり取得されます。サーバーを切り離すには[ドメインへの措置](moderation.md)を使ってください。そちらはリンクのカードも止めます。ルールはドメインへの措置と一緒に一覧に出ることも、一緒にエクスポートされることもありません。
- ルールはドメインとそのサブドメインを対象にし、画像はページのホストと画像のホストの両方で確認されます。あるホストを対象にするルールはすべて適用されるので、サブドメインのルールで親のドメインのルールが緩むことはありません。ドメインは、ドメインへの措置と同じように書いて正規化します。スキームもポートもないホスト名で、国際化ドメインはどの表記でもかまいません。
- 変更はすぐに、サーバーがすでに持っているカードにも反映されます。画像を止めている間は、それらのカードのプロキシした画像も配信されなくなります。
- ルールを外すと、サーバーがすでに持っているカードは元どおりに戻ります。飛ばしたリンクは、次に投稿で必要になったときにカードが付きます。まとめて取得し直すことはありません。
  画像を止めている間に取得し直したカードは、次に取得し直すまで画像なしのままです。
- Web クライアントの管理画面（「リンクのカードのドメイン」、`/admin/link-card-domains`、`manage_settings`）では、「ルールを追加」でメモとともにルールを設定します。一覧は 1 ページに 40 個のルールを表示し、ドメインで検索できます。ドメインを選ぶとそのルールのページが開き、そこでモードとメモを変えたり、ルールを外したり（確認のダイアログのあと）できます。
- 管理 API は `/api/v1/admin/link_card_domains`（一覧、設定。ドメインを指定した `DELETE` で外します）で、ロールの権限 `manage_settings` が必要です。サーバーが持てるルールは最大 1,000 個で、それぞれに最大 5,000 文字のメモを付けられます。
- 変更はすべて監査ログに記録されます（`link_card_domain_fetch_stopped`、`link_card_domain_images_stopped`、`link_card_domain_rule_removed`）。

<!-- source: theme 380636a0e583 -->
## テーマ

サーバー全体のテーマは、Web クライアントのデザイントークンの上書きと、任意の 1 つの CSS（カスタム CSS）の組み合わせです。すべての閲覧者の Web クライアントと、サーバーが描画する画面（ログイン、登録、規約など）に、ライトとダークの両方で適用されます。

- `app/ui/src/theme.css` のトークンは、基本の色（`--palette-*`）を除いてすべて上書きできます。一覧、既定値、範囲は [app/ui/README.md](../../app/ui/README.md#server-themes) にあります。色はライトとダークのそれぞれについて `#rrggbb` で指定します（`color-overlay` は `#rrggbbaa` でもかまいません）。ほかのトークンは、その種類（長さ、数値、時間、イージング、フォント名のリスト、影）の形の値を、その範囲内で受け付けます。`font-sans` や `font-mono` を上書きするときは、`"my Noto Color Emoji"` を先頭に残してください。そうしないと、Web クライアントの絵文字が OS のフォントで描かれます。
- 保存すると、サーバーは値の形と範囲、色のトークンのすべての組のコントラスト（上書きしていないトークンは既定値を使います。ライトとダークの両方で WCAG 2.2 AA）、カスタム CSS の長さ（64 KiB）を確認します。どれか 1 つでも通らなければ、何も保存されません。
- サーバーはカスタム CSS を読みません。トークンの規則のあとに、どの `@layer` の外にも置いて、同じスタイルシートに入れます。Web クライアントと同じ CSP のもとで動くので、ほかのオリジンの画像、フォント、`@import` は読み込めません。
  **サーバーはカスタム CSS の効果（コントラスト、横スクロール、タップの対象の大きさ、フォーカスの表示、右から左に書く言語）を確認しません。それは管理者の責任です。** クラス名とページの構造はバージョンによって変わるので、安定して頼れるのはトークンだけです。
- テーマはリビジョン（`instance_theme_revisions`）として残り、現在のリビジョンのスタイルシートは `/theme/{revision ID}.css`（`immutable`、長期間キャッシュされます）で配信されます。HTML のシェルには共有キャッシュ（60 秒と、再検証の 300 秒）があるので、保存してから全員に行き渡るまで数分かかります。すでに開いているページは、再読み込みするまで古い見た目のままです。
- シェルの `<meta name="theme-color">`（ライトとダーク）と、Web App Manifest の `theme_color` / `background_color`（ライト）には、テーマの `color-surface` の値が入ります。
- トークンもカスタム CSS もないリビジョンを保存すると、テーマは既定に戻り、スタイルシートは読み込まれなくなります。
- テーマは、Web クライアントの管理画面（「テーマ」、`/admin/theme`）、クライアント API（`GET` / `PUT /api/v1/admin/theme`）、CLI で変えられます。管理画面とクライアント API にはロールの権限 `manage_settings` が必要で、変更は監査ログに記録されます（`setting_changed`、対象は `theme`）。

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

<!-- source: logo-and-icon a2d23def322b -->
## ロゴとアイコン

サムネイルとプレビューの画像のほかに、インスタンスは独自のアイコン、ロゴ、暗い背景のロゴを持てます。それぞれ 1 つの SVG としてアップロードします。
設定するまでは組み込みの mycroblog の画像が使われ、消すと組み込みの画像に戻ります。

| 画像 | 形 | 表示される場所 |
| --- | --- | --- |
| `icon` | 正方形（誤差 1% 以内） | ファビコン、iOS のホーム画面のアイコン（`apple-touch-icon`）、インストールしたアプリのアイコン（Web App Manifest）、ランディングページのサーバーのカードと Web クライアントのサイドパネルのサーバー名の横、そしてクライアントがサーバーを正方形の絵で表すあらゆる場所 |
| `logo` | 幅が高さの 1 倍から 8 倍 | 組み込みのワードマークとサーバー名の代わりに、Web クライアントのヘッダーと、ログイン、登録、同意のページの上部（ロゴには名前を含めてください。名前がロゴの代替テキストになります）、すべてのメールの HTML 版、`preview_image` が設定されていないときのリンクプレビューの画像 |
| `logo_dark` | `logo` と同じ | 暗い背景。ないときは、そこでも `logo` を使います |

- **サーバーは SVG をサニタイズします。** XML として読み込み、許可された要素（図形、グループ、`defs`、`symbol`、`use`、グラデーション、`clipPath`、`mask`、`pattern`、`style`、`title`、`desc`）と属性（形状、変形、表示属性、`id`、`class`、`style`）だけから新しい文書を書き出します。スクリプト、`foreignObject`、埋め込みの画像、リンク、アニメーション、フィルター（影とぼかし）、イベントハンドラー、ファイルの外への参照、`data:` の URL は取り除かれ、応答に取り除いたものの一覧が含まれます。保存されるのはサニタイズした SVG だけです。
- SVG は、UTF-8 の整形式の XML でない、`DOCTYPE` か実体の宣言がある、`viewBox` がない、256 KiB を超える（サニタイズ後は 128 KiB）、要素が 5,000 を超える、入れ子が 32 段を超える、`use` 要素が 200 を超える、種類に対して形が合わない、`text` を含む（先にエディターで文字をアウトライン化してください）、何かを読み込みうる CSS（`@import`、`@font-face`、`url(#id)` 以外の `url()`、エスケープ）がある、のいずれかのとき拒否されます（何も保存されません）。`<style>` の中の `@media (prefers-color-scheme: dark)` は使えるので、アイコンをダークモードで変えることができます。
- SVG を使えない場所もあります（iOS のホーム画面、古い Safari のファビコン、リンクプレビュー、メール）。それらのために、SVG をアップロードするクライアントが、サニタイズした SVG から PNG 版も描き、一緒に送ります。`icon` では 32、48、180、192、512 px と 512 px のマスカブルアイコン、`logo` では高さ 96 px のメール用のロゴと 1200 × 630 のリンクプレビューの画像、`logo_dark` では高さ 96 px のメール用のロゴです。サーバーは、それぞれが正しい大きさの PNG であること（最大 1 MiB。180 px のアイコン、マスカブルアイコン、リンクプレビューは不透明でなければなりません）を確認し、メタデータなしで書き直します。どれか 1 つでもファイルが正しくなければ、何も保存されません。アイコンの不透明な版は背景色 `#rrggbb` の上に描かれ、既定はテーマのライトの `color-surface` です。サーバーは、PNG 版が SVG と同じ絵になっているかは確認しません。
- ファイルは blob ストアの `instance/brand/<revision>/` に保存され、`https://<host>/brand/<revision>/<version>.svg|.png` で、`Cache-Control: public, max-age=31536000, immutable` を付けて配信されます。リビジョンの内容は変わらないためです。SVG は `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox` と `X-Content-Type-Options: nosniff` を付けて配信されるので、単独で開いても何も実行せず、何も読み込みません。
  以前のリビジョンのファイルも残るので、送ったメールのロゴはそのまま表示されます。
- アイコンを設定すると、HTML のシェルはその PNG のファビコン、SVG、180 px の `apple-touch-icon` にリンクし、Web App Manifest はその 512 px、192 px、マスカブル、SVG のアイコンをこの順に並べ（ほかのフェディバースのサーバーは最初のものをこのサーバーのアイコンとして使います）、`/apple-touch-icon.png` は 180 px の版を返します。シェルの ETag は画像とともに変わり、シェルの共有キャッシュはテーマと同じく数分続きます。インストールしたアプリは、Android では Chrome が次にマニフェストを確認したときに新しいアイコンになります。デスクトップでは、インストールし直すまで古いアイコンのままです。
- `GET /api/v1/instance` は、`icon`、`logo`、`logo_dark` を、SVG の URL、各 PNG 版の URL と大きさ、リビジョンとともに返します。組み込みの画像を使っている間は、それぞれ含まれません。
- 設定は Web クライアントの **サーバーの設定** > **アイコンとロゴ**（`/admin/settings/branding`、ロールの権限 `manage_settings`）で行います。アイコン、ロゴ、暗い背景のロゴの SVG ファイルを選ぶと、サーバーがそれを確認し、ブラウザーが PNG 版を描き、フォームがそのプレビュー（ブラウザーのタブ、ホーム画面、アプリのアイコン、明るい背景と暗い背景のメール、リンクプレビュー）を、サーバーが取り除いたものの一覧とともに表示します。サーバーがファイルを拒否したときは、ボタンの下に理由と直し方が表示されます。アイコンについては、不透明な版の背景色もそこで設定します。**保存する** で SVG とすべての PNG 版を送ります。**組み込みのアイコンに戻す**、**組み込みのロゴに戻す**、**暗い背景のロゴを外す** でそれぞれを消します。Web クライアントは、表示した画像をオフラインで使えるように保持します。
- クライアント API でも、ロールの権限 `manage_settings` で同じことができます。
  `POST /api/v1/admin/instance/brand/sanitize` は SVG を確認し、サニタイズした SVG と、描くべき PNG 版を返します（何も保存しません）。`PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` は SVG をすべての PNG 版とともに保存します（`multipart/form-data`、版ごとに 1 つのパート）。同じパスへの `DELETE` で 1 つを消し、`GET /api/v1/admin/instance/brand` は現在の画像をアイコンの背景色とともに返します。変更はすべてリビジョン（`instance_brand_revisions`）として残り、監査ログに記録されます（`setting_changed`、対象は `brand_icon`、`brand_logo`、`brand_logo_dark` のいずれか）。
- PNG 版はクライアントが描くので、CLI では設定できません。ただし、表示したり、壊れたものを消したりはできます。

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

<!-- source: custom-emoji 5e0ddb3251cf -->
## カスタム絵文字

ローカルのカスタム絵文字は、投稿の本文や注意書き、または表示名、自己紹介、プロフィールの項目に `:shortcode:` と書くと、画像として表示されます。送り出す Note とアクターの `Emoji` タグを通じて、Mastodon、Misskey、GoToSocial、Akkoma にも届きます。

- ショートコードは 2-30 文字の英字、数字、`_` です。大文字と小文字を区別し、絵文字を登録したあとは変えられません。
  別名（最大 10 個）とカテゴリー（最大 64 文字）は、絵文字ピッカーでの検索とグループ分けに使われます。
- 画像は PNG、GIF、WebP で、`custom_emoji_max_bytes`（既定 256 KiB）以下、各辺 1,024 px 以下です。サーバーは 64、128、256 px の枠に収まる派生画像を作り、元の画像は保持しません。APNG は最初のフレームの静止画になります。GoToSocial は既定で 100 KiB を超える絵文字を取得しないので、小さな画像をおすすめします。
- 無効にした絵文字はどこにも表示されず（投稿、プロフィール、連合、ピッカー）、`:shortcode:` は再び文字として表示されます。ピッカーから隠す設定にした絵文字は、ピッカーに出ないだけで、書けば使えます。ほかのサーバーから来た絵文字は、無効にするか、このサーバーの絵文字として複製することしかできません。
- 複製は、メディアプロキシと同じ制限のもとでほかのサーバーの絵文字の画像を取得し（`reject_media` のドメインの画像は取得しません）、上と同じ確認をして、新しいローカルの絵文字として登録します。ショートコードは元のものか、指定したものになります。画像は 256 px の枠に縮小され、取得できなかったり確認を通らなかったりした画像は何も残しません。
- 削除は元に戻せません。過去の投稿の絵文字も文字に戻り、同じショートコードをもう一度登録すると別の絵文字になります。連合先のサーバーが複製した画像は削除できません。
- 管理画面（「カスタム絵文字」、`/admin/custom-emojis`）、クライアント API（`/api/v1/admin/custom_emojis`）、CLI は同じことができます。画面では、「カスタム絵文字を登録する」で登録のフォーム（`/admin/custom-emojis/new`）が開き、一覧で絵文字を選ぶとそのページ（`/admin/custom-emojis/{id}`）が開きます。そこで設定の変更、画像の差し替え、有効化と無効化、削除、ほかのサーバーの絵文字の複製ができます。一覧そのものでも、1 回押すだけで絵文字を有効にしたり無効にしたりできます。管理画面とクライアント API にはロールの権限 `manage_custom_emojis` が必要です（組み込みの `admin` だけが持っています）。登録、複製、画像の差し替え、設定と別名の変更、有効化と無効化、削除は、どの方法で行っても監査ログに記録されます（対象は `custom_emoji`。CLI からの操作は CLI の操作として記録されます）。

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- ロールの権限 `create_custom_emojis`（利用者の機能）を持つ利用者は、設定の「カスタム絵文字」（`/settings/custom-emojis`）から絵文字の作成を申請でき、自分が作った絵文字の別名、カテゴリー、ピッカーでの表示を変えたり、削除したりできます。画像の差し替えも申請になります。申請は、`manage_custom_emojis` を持つ人が承認するまでどこにも表示されません（ピッカー、投稿、リアクション、連合）。申請は、管理画面の「カスタム絵文字」の「申請」のページ（`/admin/custom-emojis/requests`）、クライアント API（`/api/v1/admin/custom_emoji_requests`）、上の CLI で承認または却下します。`manage_custom_emojis` を持つ人には申請が通知され（`admin_custom_emoji_request`）、申請者には承認か却下が通知されます。申請、取り下げ、承認、却下は監査ログに記録されます（対象は `custom_emoji_request`）。
- 保留中の作成の申請は、そのショートコードを予約します。保留中の申請が却下されるか取り下げられるまで、同じショートコードで申請や登録はできません。承認された絵文字の画像はインスタンスのアクターのアップロードとして複製されるので、申請者のアカウントが削除されても残ります。
- 作成者が自分の絵文字を変えられるのは、`create_custom_emojis` を持っている間だけです。ロールを外されても絵文字は残ります。
  管理者は、作成者にかかわらずすべての絵文字を変更、削除できます。
- ロールがサポーターのプランから来ている場合、利用者が毎月作れる絵文字の数は、プランの月の絵文字の上限で制限されます。数えるのは、利用者が今の暦月（UTC）に出した作成の申請のうち、承認済みか保留中のものです。却下されたものと取り下げたものは数えず、絵文字を削除しても枠は空きません。申請は、翌月に承認されても、出した月に数えます。数は毎月 1 日の 00:00 UTC にリセットされ、設定のページには利用者自身のタイムゾーンでそれがいつかが表示されます。絵文字の画像の差し替えと、管理者が登録した絵文字は数えません。管理者が手で割り当てたロールと、サポーターを無効にしたサーバーには上限がありません。
