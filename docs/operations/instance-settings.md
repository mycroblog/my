# インスタンス設定

運営者向けの手順です。要件は FR-001・FR-101・FR-202・FR-408・FR-704・FR-705・FR-718・FR-725（[機能要件](../requirements/01-functional.md)）、
テーブルは [データモデル §4.9](../architecture/data-model/04-tables/09-instance.md#49-instance) です。

## 設定（環境変数）とインスタンス設定（DB）の分担

| | 設定 | インスタンス設定 |
| --- | --- | --- |
| 内容 | デプロイの構成: 役割・アダプタ・接続先・ホスト名・秘密情報 | 運営の方針: 登録モード・文字数上限・既定のロケール・タイムラインを誰に見せるか、インスタンスの名前・説明・規約 |
| 置き場所 | 環境変数 `MY_*` と設定ファイル（[設定リファレンス](../reference/configuration.md)、ADR-0010） | DB。変更はすべて版として残る |
| 変更 | プロセスの再起動で反映 | `my instance set`・Web クライアントの管理画面・クライアント API で即時に反映（[管理画面とクライアント API](#管理画面とクライアント-api)） |
| 未設定のとき | スキーマの既定値（必須なら起動しない） | コードで定義した既定値 |

- **同じキーが両方にあることはありません。** どちらかがもう一方を上書きする優先順位は存在しません。
  環境変数や設定ファイルでインスタンス設定を与えることはできず、DB に設定（環境変数）の値を置くこともできません。
  両方のキーが重ならないことはテスト（`TestConfigAndInstanceSettingsShareNoKey`）で保証します。
- 新しい値をどちらに置くかは、「再起動なしに運営者が変えるものか」で決めます。デプロイごとに決まり、変えるとプロセスの構成が変わるものは設定、
  運営中に変えるものはインスタンス設定です。

## 前提

- 設定が [設定リファレンス](../reference/configuration.md) どおりに与えられている（`my config check`）。
- DB にスキーマが適用済み（Atlas で `schema/<dialect>/` を適用する。[ADR-0023](../adr/0023-declarative-schema-management.md)）。

## キー

### 設定値（1 キー 1 値）

| キー | 型 | 既定値 | 説明 |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | 登録モード（FR-001、[利用者登録](registration.md)）。サービスの終了の予告中とクローズの後は、実効値が `closed` になる（設定の値は変わらず、予告を取り消すと元に戻る。`GET /instance` と NodeInfo は実効値を示す。[ADR-0088](../adr/0088-service-shutdown-modes.md) §2） |
| `max_status_length` | 整数 1〜100000 | `500` | 投稿の文字数上限（FR-101） |
| `default_locale` | BCP 47 言語タグ | `en` | 既定のロケール。新しい利用者の UI ロケールの初期値で、利用者の希望に合うカタログが無いときにも使う（ADR-0009） |
| `local_timeline_access` | `off` / `logged_in` / `everyone` | `everyone` | ローカルタイムライン（`GET /api/v1/timelines/local`）を誰に見せるか: 誰にも / ログインした利用者に / 誰にでも（FR-202）。`off` は 404、`logged_in` はログインしていない訪問者に 401 を返し、ナビゲーションにも出さない。`everyone` でないとき、ランディングページは見本の投稿を出さない |
| `federated_timeline_access` | `off` / `logged_in` / `everyone` | `everyone` | 連合タイムライン（`GET /api/v1/timelines/public`）を誰に見せるか。値の意味はローカルタイムラインと同じ（FR-202） |
| `preview_image` | メディア ID（空 = 未設定） | 空 | 画像のない投稿・プロフィールのリンクプレビュー画像（`og:image`）。未設定なら同梱の既定画像（[リンクプレビュー](link-previews.md)） |
| `contact_account` | ローカルアカウントのユーザー名（空 = 未設定） | 空 | 運営への連絡先のアカウント。[連絡先とサムネイル](#連絡先とサムネイル) |
| `contact_email` | メールアドレス（空 = 未設定） | 空 | 運営への連絡先のメールアドレス |
| `thumbnail` | メディア ID（空 = 未設定） | 空 | インスタンスを表す画像（サムネイル） |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | リモートのメディアの複製を持つか（FR-409、ADR-0005）。`none` はメディアプロキシが毎回中継し、`avatars` はアバター・ヘッダー・カスタム絵文字、`all` はすべてを `remote_media_cache_ttl` の間 Blob にキャッシュする。キャッシュの合計の上限は設定 `remote_media_cache_max_bytes`（[federation.md §9](../architecture/federation.md#9-リモートメディア)） |
| `remote_media_cache_ttl` | 整数 60〜31536000（秒） | `86400` | 取得したリモートのメディアを持ってよい期間。メディアプロキシのレスポンスの `Cache-Control: max-age` で、CDN とブラウザがこの間キャッシュする（[federation.md §9](../architecture/federation.md#9-リモートメディア)） |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch（secure mode、FR-408）。`true` にすると、検証できる HTTP 署名の無い ActivityPub の GET に、オブジェクトとコレクションは `401` で、利用者の Actor は鍵の取得のための最小の文書で答える（[federation.md §10](../architecture/federation.md#10-コレクションとオブジェクトの-get)）。インスタンス Actor は署名なしでも文書全体を返すので、署名の検証のための鍵の取得はどのサーバーからもできる |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | 連合の向き（FR-413、[federation.md §14](../architecture/federation.md#14-連合の向き)）。`off` は連合しない。`inbound` は外のアカウントをフォローして読むが、ローカルの投稿・プロフィール・反応を外へ出さず、外からのフォローは断る（`Reject`）。`outbound` は外からのフォローを受けてローカルの投稿を届けるが、外の投稿・反応を取り込まない。`both` は送受信する。変更はすぐに効き、`manage_settings` の権限で変えられる |
| `dynamic_client_registration` | `true` / `false` | `true` | 動的クライアント登録（RFC 7591、ADR-0090 §1）。`true` の間、`POST /oauth/register` が認証なしで public client の登録を受ける。`false` にすると、このエンドポイントはすべての要求を `403`（`access_denied`）で断る。すでに登録されたクライアントはそのまま使える。使われない動的登録のクライアントは、設定 `dynamic_client_unredeemed_ttl`・`dynamic_client_idle_ttl` に従って定期のジョブが削除する。変更はすぐに効き、`manage_settings` の権限で変えられる |
| `domain_actions_visibility` | `none` / `users` / `all` | `none` | 処分したドメインの一覧（`GET /api/v1/instance/domain_actions`）を誰に見せるか: 誰にも / ログインした利用者に / 誰にでも（FR-718、[ドメイン単位の処分と通報の対応](moderation.md#処分したドメインの公開)） |
| `domain_actions_rationale_visibility` | `none` / `users` / `all` | `none` | その一覧の公開コメントを誰に見せるか。一覧を見られない相手には見せない（FR-718） |
| `appeal_window` | 整数 20〜365（日） | `180` | 処分から何日のあいだ、本人が異議を申し立てられるか（FR-707、ADR-0038、[異議申し立て](moderation.md#異議申し立て)）。変えると、すでに出た処分の期限も変わる |
| `retention_session_origins` | 整数 90〜3650（日） | `180` | ログイン元の IP アドレスと User-Agent を、ログインから何日残すか（ADR-0038、[記録の保持期間と保全](moderation.md#記録の保持期間と保全)） |
| `retention_registration_origins` | 整数 90〜3650（日） | `180` | 登録元の IP アドレスを、登録から何日残すか |
| `retention_reports` | 整数 180〜3650（日） | `365` | 通報を、最後の解決から何日残すか。処分が参照する通報は処分と同じだけ残る |
| `retention_moderation_actions` | 整数 180〜3650（日） | `365` | アカウントへの処分と異議申し立てを、処分の終わり（解除・期限切れ・異議の審査の遅い方）から何日残すか |
| `retention_audit_log` | 整数 365〜3650（日） | `730` | 監査ログの行を、記録から何日残すか |
| `search_indexing` | `allow` / `deny` | `deny` | 検索エンジンに索引を許すか（FR-725、ADR-0057）。`allow` でも、索引されるのは `indexable` のアカウントのプロフィールと `public` の投稿だけ（[検索エンジンと AI クローラー](crawlers.md)） |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | AI への表明（FR-725、ADR-0057）。`deny_training` は学習を拒否し、`deny` は学習と AI での利用（AI 検索・アシスタント・エージェント）を拒否する |
| `indexable_default` | `true` / `false` | `false` | 新規アカウントの `indexable` の既定（FR-412）。変えても既存のアカウントの `indexable` は変わらない |
| `security_contact` | `https:` か `mailto:` の URI（空 = 未設定） | 空 | 脆弱性の連絡先。設定すると `/.well-known/security.txt`（RFC 9116）を返す |
| `require_media_description` | `true` / `false` | `false` | ローカルの投稿に添付するすべての画像に代替テキストを必須にするか（FR-105）。`true` にすると、代替テキストが無いか空白だけの画像のある投稿と編集を 422 で拒む。`GET /api/v1/instance` の `limits.require_media_description` で公開し、Web クライアントは説明の入力欄を必須にする。利用者ごとの設定 `require_media_description`（送る前に確かめるだけ）とは別 |
| `link_cards_enabled` | `true` / `false` | `true` | 投稿中のリンクのカードを出すか（FR-117、ADR-0062）。`true` ではサーバーがリンクの先のページを取得してカードを作る（[federation.md §9.1](../architecture/federation.md#91-リンクのカード)）。`false` にすると取得をやめ、作ったカードも出さない |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | Client ID Metadata Document のクライアントを受けるか（[ADR-0090](../adr/0090-third-party-oauth-apps.md) §1）。`true` では `client_id` が `https` の URL の認可要求で、サーバーがその URL のメタデータを取得してクライアントとして扱う（事前の登録は要らない）。取得はリダイレクトを追わず、5 KB までで、応答のキャッシュ指示に従って最大 24 時間使い回す。`false` にすると取得をやめ、このクライアントの認可要求・トークンの交換と更新を断る |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | 運営の費用（`/admin/finance` で記録したもの）をどこまで公開するか（FR-903、[ADR-0067](../adr/0067-finance-cost-records.md) §8）。`private` は公開しない（`GET /api/v1/instance/cost_totals` は 404）、`totals` は期間ごとの通貨ごとの合計だけ、`breakdown` は合計と事業者・区分ごとの内訳を、情報ページ（`/about` と `/about/costs`）と API で誰にでも見せる。`breakdown` ではどの事業者（クラウドなど）で動いているかが知られる。どの段階でも項目の名前・メモ・個々の支払いは公開しない。公開するのは今日（UTC）を含む期間まで |
| `finance_viewer_item_names` | `true` / `false` | `false` | 権限 `view_finance` を持つ利用者が会計の画面（`/finance`、`GET /api/v1/finance/cost_totals`）で、費用の項目の名前と、取り込んだ費用のサービス名を見るか（[ADR-0070](../adr/0070-permission-split-emoji-finance.md) §5）。合計と事業者・区分ごとの内訳はこの設定によらずいつも見せる |
| `finance_viewer_item_memos` | `true` / `false` | `false` | 権限 `view_finance` を持つ利用者が会計の画面で費用の項目のメモを見るか（ADR-0070 §5）。名前かメモのどちらかが `true` なら、期間ごとに項目の行（事業者・区分・その期間の額に、有効なら名前・メモ）を出す。`manage_finance` を持つ人は設定によらずすべてを見る |
| `finance_viewer_income` | `totals` / `plans` | `totals` | 権限 `view_finance` を持つ利用者に、収入を合計だけ（`totals`）か、加えてプランごとの加入者の数まで（`plans`）見せるか（ADR-0070 §5）。プランごとの金額と誰が払ったかはどの値でも見せない。`income_visibility` が `plans` なら、この値によらず公開されている人数を見せる。収入は Web の会計の画面（`/finance`）と `GET /api/v1/finance/income_totals` で読む |
| `income_visibility` | `private` / `totals` / `plans` | `private` | サポーターの収入（決済の事業者の手数料を引いた額、FR-917、[ADR-0073](../adr/0073-income-records-and-publication.md) §7）をどこまで公開するか。`finance_visibility` とは別に選ぶ。`private` は公開しない（`GET /api/v1/instance/income_totals` は 404）、`totals` は期間ごとの通貨ごとの収入の合計だけ、`plans` は加えていまのプランごとの加入者の数を誰にでも見せる。加入者の少ないプランでは、人数から誰が加入しているかが推測されうる。収入と費用の差（収支）は、この値と `finance_visibility` がどちらも `private` でないときだけ出す（片方だけの公開では、収支から隠した側が分かるため）。どの値でも、個々の支払い・加入者・プランごとの金額・総額と手数料は公開しない。公開するのは今日（UTC）を含む期間まで |
| `supporter_list_visibility` | `off` / `public` / `signed_in` | `off` | サポーターの一覧（`GET /api/v1/instance/supporters` と `/about/supporters`、[ADR-0071](../adr/0071-role-badges-and-supporter-list.md) §4）を誰に見せるか。`off` は誰にも見せない（API は 404、`/about` からのリンクも出ない）、`public` は誰にでも、`signed_in` はこのサーバーにサインインした人だけに見せる（サインインしていなければ API は 401）。一覧に出るのは、加入中の人のうち自分で掲載を選んだ人のアカウントとプランだけで、金額・月払いか年払いか・日付・人数はどの値でも出さない。一覧のページと API の応答は、`search_indexing` に関わらずいつも `noindex, noarchive` |
| `user_app_registration` | `true` / `false` | `true` | ログインした利用者が自分の OAuth のアプリ（public / confidential）を client API（`POST /api/v1/oauth_apps`）で登録できるか（FR-003、[ADR-0090](../adr/0090-third-party-oauth-apps.md) §1）。`false` にすると登録は 403 になる。登録済みのアプリの一覧・変更・削除・シークレットの再発行・自分へのトークン発行と、ほかの利用者による認可はそのまま使える |

- 値は保存前に正規形にします（`TRUE` → `true`、`0750` → `750`、`ja_jp` → `ja-JP`、メディア ID は小文字）。不正な値は何も書き込まずに拒否します。
- `preview_image` と `thumbnail` は、存在する画像のメディア（処理中を含む）だけを受け付けます。`preview_image` は、処理が終わるまでと、処理に失敗した画像の代わりには既定画像が出ます。
  どの版でも一度設定したメディアは、未添付アップロードの消去（FR-803）で消えません。
- `contact_account` は、削除されていないローカルの利用者のユーザー名だけを受け付けます（小文字にして保存）。`contact_email` は `alice@example.com` の形のアドレスだけを受け付けます。

### 文章（1 キー 1 値）

| キー | 上限 | 説明 |
| --- | --- | --- |
| `name` | 100 文字・1 行（空白だけは不可） | インスタンスの名前（表示名）。[名前とドメイン](#名前とドメイン) |
| `short_description` | 500 文字 | 短い説明 |
| `description` | 20000 文字 | 説明 |
| `terms` | 100000 文字 | 利用規約（Markdown） |
| `privacy_policy` | 100000 文字 | プライバシーポリシー（Markdown） |
| `commerce_disclosure` | 100000 文字 | 特定商取引法に基づく表記など、課金のプランの販売者の表記（Markdown。[サポーター](supporters.md)） |
| `preview_image_alt` | 1500 文字 | `preview_image` の画像の代替テキスト（`og:image:alt`）。無ければ「〇〇のイメージ画像」 |
| `thumbnail_alt` | 1500 文字 | `thumbnail` の画像の代替テキスト |

- 文章はキーごとに 1 つの値です。管理者が好きな言語で 1 回書き、どのロケールの読み手にも同じ値を出します。ほかの言語の読み手は機械翻訳などで読みます。
  管理画面の欄の名前や説明は、ほかの画面の文言と同じく閲覧者の言語に翻訳します（対象になるのは管理者が書く値だけです）。
- 書かれた言語はサーバーには分からないので、Web クライアントとサーバーが描画する画面は、文章を `dir="auto"` と `lang=""`（言語が不明であることを示す HTML の値）で出し、画面の言語を引き継がせません。
- 空の文章を設定すると、その文章を消したことになります。
- 文章は CLI のほか、Web クライアントの管理画面（名前・短い説明・説明は「サーバーの設定」、`/admin/settings`。利用規約・プライバシーポリシー・特定商取引法に基づく表記などは「規約とポリシー」、`/admin/documents`）とクライアント API（`GET /api/v1/admin/instance_texts`・`PUT /api/v1/admin/instance_texts/{key}`）で変えられます。どちらもロールの権限 `manage_settings` が要り、変更は監査ログ（`text_changed`）に残ります。

### 利用規約とプライバシーポリシー

- `terms`・`privacy_policy`・`commerce_disclosure` は Markdown で書きます（CommonMark と、GitHub Flavored Markdown の表・取り消し線・URL の自動リンク）。段落の中の改行はそのまま改行になり、`#` の見出しはページの題の下の段（`h2`）から始まります。
- サーバーが HTML にしてサニタイズし（匿名の `GET /api/v1/instance/documents/{kind}`）、Web クライアントのページ `/about/terms`・`/about/privacy`・`/about/commerce` が表示します。生の HTML・画像・`http` / `https` / `mailto` 以外のリンクは出ません。ページはサインインが要りません（Web クライアントなので JavaScript は要ります）。`/about/commerce` は決済の事業者に渡す URL です。
- 書いた文書は、登録の画面（作成のボタンの前の同意の文とリンク）、サーバーが描画するすべての画面のフッター、Web クライアント（3 列目・メニュー・設定・サインインの画面）からリンクされます。書かれていない文書はリンクされず、本文の API は 404 です。

### 名前とドメイン

インスタンスはドメインと名前を別々に持ちます。

- **ドメイン**は設定 `host`（とハンドルのドメイン `account_domain`）で、運営中は変えられません。ハンドル（`@alice@example.com`）・URL・ActivityPub の ID・`GET /api/v1/instance` の `domain` は、名前があってもドメインのままです。
- **名前**は文章 `name` で、人に向けてインスタンスを名指す場所に出ます: Web クライアントのサイドバーと「このサーバーについて」、ブラウザのタブのタイトル、
  サーバーが描画するログイン・登録・メール確認・アカウントの状態・認可同意の画面、メール（メールアドレスの確認・モデレーターへの通知）、
  リンクプレビューの `og:site_name`、Web App Manifest の `name` / `short_name`、NodeInfo の `metadata.nodeName`、`GET /api/v1/instance` の `name`。
- 名前はどのロケールでも同じです。名前が無ければ、名前の代わりにドメイン（`host`）を出します。
- 変更は再起動なしで、次の要求から反映されます。開いている Web クライアントのタブのタイトルとサイドバーは、管理画面で保存したときに更新されます。

### 連絡先とサムネイル

運営への連絡先（`contact_account`・`contact_email`）とインスタンスを表す画像（`thumbnail` と文章 `thumbnail_alt`）は、次の場所に出ます。
どれも未設定なら出ません。

- Web クライアントの「このサーバーについて」（`/about`）。サムネイルはサインインしていない人の最初の画面（`/`）にも出ます。
- `GET /api/v1/instance` の `contact`（`account` と `email`）と `thumbnail`（`url`・`width`・`height`・`alt`）。
- NodeInfo の `metadata`。Misskey の慣行の `maintainer`（`name` は連絡先のアカウントの表示名、無ければユーザー名。`email` は `contact_email`）、
  Pleroma・Akkoma の慣行の `staffAccounts`（連絡先のアカウントの Actor の ID）、`thumbnail`（`url` と `alt`）、短い説明 `short_description` の `nodeDescription`。

連絡先のアカウントが凍結・削除されたときは、アカウントは出さずにメールアドレスだけを出します。
変更は Web クライアントの管理画面（「サーバーの設定」、`/admin/settings`）か、CLI・クライアント API で行います。

## 使い方

```sh
my instance list                                      # すべての設定値（既定値かどうか）と文章
my instance get registration_mode
my instance set registration_mode open
my instance set name "例のソーシャル"                 # どのロケールでも同じ名前
my instance set description - < description.txt       # 値 "-" は標準入力から読む
my instance get name
my instance set name ""                               # 名前を消す
my instance history registration_mode                 # 変更履歴（新しい順、-limit で件数）
my instance history description
my instance set -owner admin preview_image ./preview.png   # 画像を admin のメディアとしてアップロードし、プレビュー画像にする
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # アップロード済みのメディア ID を指定
my instance set preview_image_alt "青い鳥のマスコット"
my instance set preview_image ""                      # 同梱の既定画像に戻す
my instance set contact_account admin                 # 連絡先のアカウント
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # サムネイル
my instance set thumbnail_alt "海辺の町の写真"
```

- フラグはキーと値より前に書きます。
- `-owner <ユーザー名>` を付けると、`preview_image`・`thumbnail` の値を画像ファイルのパスとして読み、そのローカルアカウントのメディアとしてアップロードします。
  画像はワーカーが処理してから使われます（[メディア](../architecture/data-model/04-tables/04-content.md#44-content)）。推奨は 1200×630 の横長の画像です。
- 変更は 1 回のトランザクションで版（`instance_*_revisions`）を記録し、現在値（`instance_settings` / `instance_texts`）を更新します。
  CLI からの変更は変更者なし（履歴の `BY` が `cli`）として残ります。
- 同じ値を設定し直しても版は増えます。
- 終了コード: 0 は成功、1 は値の拒否や DB のエラー（何も変更しない）、2 は引数の誤り。

### 連合の向きを変えるとき

- 狭めても、フォロー関係・取り込んだ投稿・配送の記録は残り、`Undo` / `Reject` は送りません。流れが止まるだけで、`both` などに戻すとそのまま再開します。
- 狭めている間に作った投稿・反応は、戻しても遡って配送しません。
- 外の側では、`inbound` の間はローカルの利用者のフォロワーに何も届かず、`outbound` / `off` の間はローカルの利用者のフォローが更新されません。
- `off` の間に行った削除（投稿・アカウント）は外に伝わりません。`inbound` / `outbound` では、それまでに外へ出したものの撤回（`Delete` など）は元の配送先とその時点のフォロワーに送ります。
- 変更は `instance_setting_revisions` と監査ログ（`target_kind = setting`）に残ります。

関係も切りたいときは、CLI `my admin federation sever` を使います。

```sh
my admin federation sever -direction in     # 外のアカウントによる、ローカルの利用者のフォローを終わらせる
my admin federation sever -direction out    # ローカルの利用者による、外のアカウントのフォローを終わらせる
my admin federation sever -direction both
```

- ローカルのアカウントと外のアカウントのあいだの承認待ちと有効なフォローを、モデレーションによる終了（`follow_terminations`、`reason = moderation`）として終わらせます。モードの変更は自動では呼びません。
- 外へ `Reject`（`in`）・`Undo`（`out`）を送るのは、そのときのモードが送るときだけです。送らないモード（`off` など）では、ローカルの側だけで終わり、外の側には伝わりません。
- 何度実行しても安全です。終わった数を標準出力に示します。

## 管理画面とクライアント API

CLI で変えられる設定値は、Web クライアントの管理画面とクライアント API でも変えられます。どちらもロールの権限 `manage_settings` が要り（組み込みの `admin` だけが持つ）、
変更は CLI と同じく版として残り、キーごとに監査ログ（`setting_changed`、対象は設定、操作者はその利用者）に記録されます。監査ログはモデレーションの画面（「監査ログ」、`/moderation/audit-log`）で引けます。

- **Web クライアント**（「サーバーの設定」、`/admin/settings`、[ADR-0058](../adr/0058-moderation-admin-separation.md) のサーバー管理）: 名前・説明と、次の設定を変えられます。
  設定はまとめて 1 つのフォームで保存し、変えたものだけを送ります。

  | 区画 | 設定 |
  | --- | --- |
  | 登録 | `registration_mode` |
  | 言語 | `default_locale`（アプリのある言語から選ぶ） |
  | タイムライン | `local_timeline_access`・`federated_timeline_access` |
  | 投稿 | `require_media_description` |
  | 連合 | `authorized_fetch`・`remote_media_policy`・`remote_media_cache_ttl`（1 時間・1 日・7 日・30 日・365 日から選ぶ）・`link_cards_enabled` |
  | 制限・凍結したサーバー | `domain_actions_visibility`・`domain_actions_rationale_visibility` |
  | 運営の費用 | `finance_visibility`（`breakdown` の選択肢に、事業者と区分が知られることを書く）・`finance_viewer_item_names`・`finance_viewer_item_memos`・`finance_viewer_income`（会計を閲覧できる人に見せる範囲） |
  | 収入 | `income_visibility`（`plans` の選択肢に、加入者の少ないプランでは誰が加入しているかが推測されうることを書く） |
  | サポーターの一覧 | `supporter_list_visibility` |
  | 検索エンジンと AI | `search_indexing`・`indexable_default`・`ai_usage`・`security_contact` |

  連絡先とサムネイル（`contact_account`・`contact_email`・`thumbnail` と文章 `thumbnail_alt`）は、名前と説明の下の「連絡先とサムネイル」の区画で変えます。画像は選んだときにアップロードされ、保存したときにサムネイルになります。

  `max_status_length`・`preview_image`・`appeal_window`・保持期間（`retention_*`）は、画面には出ず、CLI とクライアント API で変えます。
- **クライアント API**: `GET /api/v1/admin/settings` がすべての設定値を、現在値・既定値・設定済みか・型と受け付ける値（選択肢・範囲）とともに返します。
  `PATCH /api/v1/admin/settings` は `{"values": {"<キー>": "<値>", ...}}` の設定値をまとめて変えます。値はすべて文字列で、CLI と同じく正規形にして保存します。
  1 つでも知らないキーか不正な値があれば、そのキーを挙げた 422 で、何も変えません。変えたキーは同じトランザクションで版と監査ログに残ります。
- 登録モードを `invite` にしたときの招待は、モデレーションの画面（「招待」、`/moderation/invites`、権限 `manage_invites`）で作ります（[利用者登録](registration.md#招待コード)）。

## ルール

インスタンスのルール（FR-705）は、`GET /api/v1/instance` の `rules` が並べ、通報が ID で引用します。

- ルールは 1 行の文（300 文字まで）と任意の補足（1,000 文字まで）を 1 つずつ持ちます。文章と同じく、管理者が好きな言語で書き、どのロケールでも同じ値を出します（`dir="auto"` と `lang=""`）。有効なルールは 100 個までです。
- 変更（文の改訂・並べ替え）はすべて版（`instance_rule_revisions`）として残り、ルールの ID は変わりません。
- 廃止したルールは有効なルールの一覧から消えます。元には戻せませんが、通報からは引き続き参照できます。

```sh
my instance rule add -hint "嫌がらせをしない" "親切に"   # 有効なルールの最後に追加
my instance rule edit <rule> "いつも親切に"             # 文を変える。補足は -hint を付けたときだけ変わる
my instance rule move <rule> 1                        # 1 番目に移し、有効なルールを 1, 2, ... と振り直す
my instance rule retire <rule>                        # 廃止する
my instance rule list                                 # 有効なルール（-all で廃止したものも）
```

- 管理 API は `/api/v1/admin/rules`（一覧・追加・取得・改訂・移動・廃止）で、ロールの権限 `manage_rules` を要します。
- Web クライアントの管理画面（「ルール」、`/admin/rules`、`manage_rules`）で、文と補足でのルールの追加、編集、「上へ」「下へ」での並べ替え、廃止（確認のダイアログを経る）ができます。
  どの変更も監査ログ（`rule_added`・`rule_revised`・`rule_moved`・`rule_retired`）に残ります。
- 終了コードは設定と同じです（0 は成功、1 は値の拒否や DB のエラー、2 は引数の誤り）。

## テーマ

サーバー単位のテーマ（FR-605、[ADR-0059](../adr/0059-design-tokens-and-instance-theme.md)）は、Web クライアントのデザイントークンの上書きと、任意の CSS（カスタム CSS）1 つです。
すべての閲覧者の Web クライアントとサーバーが描画する画面（ログイン・登録・規約など）に、ライトとダークの両方で当たります。

- 上書きできるトークンは `app/ui/src/theme.css` の原色（`--palette-*`）以外で、一覧・既定値・範囲は [app/ui/README.md](../../app/ui/README.md#server-themes) にあります。
  色はライト / ダークそれぞれ `#rrggbb`（`color-overlay` は `#rrggbbaa` も可）で、ほかは種類ごとの形（長さ・数・時間・緩急・フォント名の並び・影）と範囲に収まる値です。
  `font-sans` / `font-mono` を上書きするときは、先頭に `"my Noto Color Emoji"` を残してください。残さないと、Web クライアントの絵文字が OS のフォントで描かれます（[ADR-0096](../adr/0096-emoji-css-import.md)）。
- 保存のときにサーバーが、値の形と範囲、色のトークンのすべての組のコントラスト（上書きしないトークンは既定値で、ライト / ダークの両方で WCAG 2.2 AA）、カスタム CSS の長さ（64 KiB）を検査し、
  1 つでも満たさなければ何も保存しません。
- カスタム CSS は、サーバーは中身を読まずに、トークンの規則の後に `@layer` の外で同じスタイルシートに入れて返します。Web クライアントと同じ CSP の下で動くので、別のオリジンの画像・フォント・`@import` は読めません。
  **カスタム CSS の結果（コントラスト・横スクロール・タップ領域・フォーカスの表示・右から左の言語）はサーバーが確かめず、管理者の責任です。** クラス名とページの構造は版で変わるので、安定して使えるのはトークンだけです。
- テーマは版（`instance_theme_revisions`）として残り、現在の版のスタイルシートを `/theme/{版の ID}.css`（`immutable` で長期キャッシュ）で配ります。
  HTML の外枠は共有キャッシュ（60 秒と再検証の 300 秒）を持つので、保存してから全員に届くまで数分かかります。開いているページは読み込み直すまで前の見た目のままです。
- 外枠の `<meta name="theme-color">`（ライト / ダーク）と Web App Manifest の `theme_color` / `background_color`（ライト）は、テーマの `color-surface` の値になります。
- トークンもカスタム CSS も無い版を保存すると、テーマは既定に戻り、スタイルシートを読み込まなくなります。
- Web クライアントの管理画面（「テーマ」、`/admin/theme`）とクライアント API（`GET` / `PUT /api/v1/admin/theme`）と CLI で変えられます。
  管理画面とクライアント API はロールの権限 `manage_settings` が要り、変更は監査ログ（`setting_changed`、対象 `theme`）に残ります。

```sh
my instance theme get > theme.json                    # 今のテーマを管理 API と同じ JSON で出す
my instance theme set < theme.json                    # テーマ全体を置き換える（検査に通らなければ理由を出して何もしない）
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # 既定に戻す
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

## カスタム絵文字

ローカルのカスタム絵文字（FR-111、[ADR-0051](../adr/0051-custom-emoji.md)）は、投稿の本文・CW とプロフィールの表示名・自己紹介・補足情報で `:shortcode:` と書くと画像で表示され、
送る Note と Actor の `Emoji` タグで Mastodon・Misskey・GoToSocial・Akkoma にも届きます。

- shortcode は 2〜30 文字の英字・数字・`_` で、大文字と小文字を区別し、登録した後は変えられません。別名（10 個まで）とカテゴリ（64 文字まで）は絵文字ピッカーの検索と分類に使います。
- 画像は PNG・GIF・WebP で、設定 `custom_emoji_max_bytes`（既定 256 KiB）以下、縦横 1,024 px 以下です。64 / 128 / 256 px の箱に収めた派生を作り、元の画像は残しません。
  APNG は最初のフレームだけの静止画になります。GoToSocial は既定で 100 KiB を超える絵文字を取得しないので、小さい画像を勧めます。
- 無効にした絵文字は、投稿・プロフィール・連合・ピッカーのどこにも出ず、`:shortcode:` の文字に戻ります。ピッカーに出さない設定にした絵文字は、ピッカーに出ないだけで、書けば使えます。
  ほかのサーバーから届いた絵文字は、無効にすることと、このサーバーの絵文字として複製することだけができます。
- 複製は、ほかのサーバーの絵文字の画像をメディアプロキシと同じ制限で取得し（`reject_media` のドメインの画像は取得しない）、上と同じ検査をして、新しいローカルの絵文字として登録します。
  shortcode は元の絵文字のものか、指定したものです。画像は 256 px の箱に縮み、取得できない・検査に通らない画像は何も残しません。
- 削除は取り消せません。過去の投稿の絵文字も文字に戻り、同じ shortcode で登録し直しても別の絵文字です。連合先が複製した画像は消せません。
- 管理画面（「カスタム絵文字」、`/admin/custom-emojis`）とクライアント API（`/api/v1/admin/custom_emojis`）と CLI で、同じ操作ができます。
  管理画面とクライアント API はロールの権限 `manage_custom_emojis`（組み込みの `admin` だけが持つ）が要ります。
  登録・複製・画像の差し替え・設定と別名の変更・有効化と無効化・削除は、どこから行っても監査ログ（対象 `custom_emoji`）に残ります（CLI の操作は CLI の操作として）。

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # 登録する
my admin emoji list -local                                                        # ローカルの絵文字を新しい順に出す
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # カテゴリと別名を変える（別名は置き換え。-no-aliases で消す）
my admin emoji update -hidden <emoji id>                                          # ピッカーに出さない（-hidden=false で戻す）
my admin emoji replace-image <emoji id> blobcat2.png                              # 画像を差し替える
my admin emoji disable <emoji id>                                                 # 無効にする（enable で戻す。リモートの絵文字も）
my admin emoji list -remote -domain mastodon.example                              # ほかのサーバーの絵文字を出す
my admin emoji copy [-shortcode <name>] <emoji id>                                # リモートの絵文字をローカルへ複製する
my admin emoji delete <emoji id>                                                  # 削除する
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # 作成の申請を出す（既定は審査中）
my admin emoji requests approve <request id>                                      # 申請を承認する
my admin emoji requests reject [-reason <text>] <request id>                      # 申請を却下する（理由は申請者に見える）
```

- ロールの権限 `create_custom_emojis`（利用者の機能。[ADR-0070](../adr/0070-permission-split-emoji-finance.md)）を持つ人は、設定の「カスタム絵文字」（`/settings/custom-emojis`）から
  絵文字の作成を申請し、自分が作った絵文字の別名・カテゴリ・ピッカーに出すかを変え、削除できます。画像の差し替えも申請です。
  申請は `manage_custom_emojis` を持つ人が承認するまで、どこにも出ません（ピッカー・投稿・リアクション・連合）。管理画面の「カスタム絵文字」の「申請」のタブ、クライアント API（`/api/v1/admin/custom_emoji_requests`）、上の CLI で承認・却下します。
  申請は `manage_custom_emojis` を持つ人に通知され（`admin_custom_emoji_request`）、承認と却下は申請者に通知されます。申請・取り下げ・承認・却下は監査ログ（対象 `custom_emoji_request`）に残ります。
- 審査中の作成の申請は shortcode を予約します。同じ shortcode の申請と絵文字の登録は、却下か取り下げまでできません。承認した絵文字の画像はインスタンスの Actor のアップロードとして複製するので、申請者のアカウントを消しても残ります。
- 作成者は、`create_custom_emojis` を持つ間だけ自分の絵文字を変えられます。ロールを外しても絵文字は残ります。管理者は作成者にかかわらずすべての絵文字を変更・削除できます。
- 作成数の上限は、ロールがサポーターのプランから来るときプランの「絵文字の上限」（[ADR-0072](../adr/0072-supporter-plans-and-roles.md)）で、作った絵文字と審査中の作成の申請の合計を数えます。管理者が手で付けたロールと、サポーター制度が無効のときは上限がありません。
