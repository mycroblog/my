<!-- source: privacy-policy-template 9966bbc6844c -->
# プライバシーポリシーのテンプレート

あなたのサーバーに合わせて書き換えるためのプライバシーポリシーです。このサーバーが何を、なぜ、どれだけの期間保存し、誰に渡すのかを説明します。

**これはテンプレートであり、法的な助言ではありません。** ここに書かれているのは、既定の設定でソフトウェアが行うことです。あなたがインスタンスをどう運用しているかは分からず、どの法律があなたに適用されるかも決めません。ポリシーに何を書かなければならないかは、あなたの法域と利用者の法域によって変わります（たとえば EU の GDPR、英国の UK GDPR、日本の個人情報保護法）。公開する前に、あなたの法域に合わせて確認を受けてください。また、以下の法的根拠と移転に関する記述は、答えではなく、決めるべき問いとして扱ってください。

これと対になる処理活動の記録は、[処理活動の記録のテンプレート](processing-record-template.md)にあります。

<!-- source: how-to-use-it 9cb843cc1ba2 -->
## 使い方

1. [テンプレート](#the-template)をファイル `privacy-policy.md` にコピーします。
2. すべての `{{...}}` を置き換えます。`{{180 days}}` のように波かっこの中にある値は、ソフトウェアの既定値です。あなたのインスタンスの値と照らし合わせて確認し（[期間とその設定](#the-periods-and-their-settings)）、波かっこを取り除いてください。
3. `{{IF ...}}` で始まる行や文は、その構成のときにだけ当てはまります（[あなたが選ぶサービス](#the-services-you-choose)）。当てはまらないときは削除し、当てはまるときは目印を取り除いてください。
4. 何も残っていないことを確かめます。`grep -n '{{' privacy-policy.md` が 1 行も出力しなければ完了です。
5. `my instance set privacy_policy - < privacy-policy.md` で公開するか、管理画面の「規約とポリシー」にある「プライバシーポリシー」のページ（`/admin/documents/privacy-policy`）で公開します。読む人は `/about/privacy` で見られます（[インスタンスの設定](instance-settings.md#terms-and-privacy-policy)）。

プライバシーポリシーを書くと、登録画面に必須の同意チェックボックスが加わります。このチェックボックスが示すのは、その人が文書を読んだということです。あなたの国の法律が処理への同意を求める場合に、これがその同意になるとは考えないでください。

本文はすべてのロケールで共通の 1 つの値です。複数の言語の読者がいる場合は、必要な言語をその 1 つの本文に書いてください。

以下の設定やサービスを変えたとき、またリリースノートに個人データへの言及があるアップグレードのたびに、ポリシーを見直してください。

<!-- source: the-periods-and-their-settings 111d5a4b6af8 -->
## 期間とその設定

`my instance list` は、すべてのインスタンスの設定と、それが既定値かどうかを表示します。期間の変更は、その期間を扱う 1 時間ごとのジョブの次の実行から有効になります。範囲外の値は受け付けられません。

| 記録 | 既定値 | インスタンスの設定 | 範囲（日） | 起点 |
| --- | --- | --- | --- | --- |
| ログイン時の IP アドレスと User-Agent | 180 日 | `session_origins_retention_days` | 90-3650 | ログイン |
| 登録時の IP アドレス | 180 日 | `registration_origins_retention_days` | 90-3650 | 登録 |
| 通報 | 365 日 | `reports_retention_days` | 180-3650 | 最後の解決 |
| アカウントへの処分と異議申し立て | 365 日 | `moderation_actions_retention_days` | 180-3650 | 処分の終了 |
| 監査ログ | 730 日 | `audit_log_retention_days` | 365-3650 | 記録 |
| 通話のルームに参加した人 | 90 日 | `voice_participants_retention_days` | 30-3650 | ルームの終了 |
| 外部の審査を使う場合の、その判定 | 90 日 | `hook_decisions_retention_days` | 30-3650 | 判定 |
| このサーバーの何からも参照されていないリモートの投稿 | 90 日 | `remote_status_retention_days` | 1-3650 | 投稿の公開 |
| すべてのリモートの投稿 | 無制限（`0`） | `remote_status_max_retention_days` | 0-3650 | 投稿の公開 |
| リレーだけが運んできたリモートの投稿 | 7 日 | `relay_status_retention_days` | 1-3650 | 投稿の公開 |
| リモートのメディアのコピー | 1 日。`remote_media_policy` が `none` の間は保存しない | `remote_media_cache_ttl_seconds`、`remote_media_policy` | 60-31536000 秒 | 取得 |
| 投稿の翻訳 | 30 日 | `translation_retention_days` | 1-3650 | 翻訳。読んでも期間は延びず、投稿が削除されると消えます |

次のものはインスタンスの設定ではなく、デプロイの設定です（[設定リファレンス](../reference/configuration.md)）。

| 記録 | 既定値 | 設定 |
| --- | --- | --- |
| 何にも添付されていないアップロード | 24 時間 | `media_unattached_retention` |
| 新しい版に置き換えられたあとの、画像の古い版 | 30 日 | `media_replaced_retention` |
| 最終的に失敗したジョブ | 30 日 | `dead_job_retention` |
| 成功しなかったジョブ実行の記録 | 30 日 | `job_attempt_retention` |

次の期間はソフトウェアで固定されており、どの設定でも変わりません。

| 記録 | 期間 |
| --- | --- |
| 削除されたアカウントの消去 | 削除から 7 日後。リモートのアカウントで、そのサーバーからの `410 Gone` だけが削除を伝え、そのサーバーがその後応答しない場合は最長 30 日 |
| 無効化されたアカウントの削除 | 無効化から 30 日後 |
| 登録のブロック | 解除または期限切れから 30 日後。どちらもない場合は、有効な間ずっと |
| セッション、トークン、認可コード | 期限切れから 30 日後 |
| データのエクスポートのアーカイブと、データのインポート | 7 日 |
| ほかのサーバーから受け取ったアクティビティの記録 | 30 日。文書そのものは 7 日 |
| ほかのサーバーに配送したアクティビティの記録 | 30 日。投稿の配送は、その投稿またはその投稿者が削除されてから 30 日後まで |
| リモートのハンドルの確認 | 90 日。各アカウントの最新のものは残す |
| Webhook のメッセージと配送 | 14 日 |
| 決済の Webhook の受信記録 | 45 日 |
| 1 時間ごとのエラーの件数とエラーの通知 | 90 日 |

[保全](moderation.md#holds)は 1 つのアカウントの消去を止めます。そのため、保全の対象のアカウントでは、上のどの期間も長くなることがあります。保全を使う場合は、テンプレートにあるとおり、そのことを書いてください。

<!-- source: records-with-no-automatic-end 90a13d1f648e -->
## 自動では終わらない記録

サーバーがこれらを削除するのは、アカウントが消去されたときだけか、まったく削除しません。法律によっては、保存し続ける正当な理由を示すことや、ほかの方法で削除することを求められることがあります。それぞれについてポリシーに何を書くかを決めてください。

- 有効なアカウントの行: メールアドレス、プロフィールの過去の版、投稿の過去の版、通知、フォロー、拒否された利用者の登録申請。誰も削除しないアカウントは残り続けます。
- 消去のあとに残るもの: アカウントの URI とユーザー名（どちらも再利用されないようにするため）、そのライフサイクルのイベント、投稿が削除されたという記録、それぞれの期間が終わるまでのモデレーションの記録、招待とロールの付与、アカウントの引っ越し、名前と説明を除いた通話のルーム。
- サポーターの記録（サブスクリプション、状態の変化、支払い、返金、チャージバック）。会計と決済事業者との照合のために保存します。サーバーは決済事業者の顧客を削除しません。あなたの義務が許すなら、事業者のダッシュボードで削除してください。
- 終わりのない処分（`ban`、または終了日時のない `suspend`）は、解除されるまで。未解決の通報。
- リモートのアカウントは、そのサーバーが削除するまで。
- 各エラーグループの最新の報告。同じグループの次の報告で置き換えられます。パターンで消去されるため、利用者の文章の断片が残ることがあります。
- バックアップ、あなたのログ、プラットフォームとプロキシのログ。サーバーはこれらを管理しません。保存する世代は[バックアップ](backup.md#how-often-and-where)で説明しています。バックアップから復元すると、消去したデータも戻ります。

<!-- source: the-services-you-choose 81461216c966 -->
## あなたが選ぶサービス

個人データを受け取るそれぞれのサービスは、あなたがそれを有効にした場合にだけポリシーに載せます。この一覧をもとにセクション 6 の表を埋め、そのサービス自身のポリシーと契約を読んでください。データが届いたあとにそのサービスが何をするかは、この文書では分かりません。

| 目印 | 設定 | サービスが受け取るもの |
| --- | --- | --- |
| ホスティング | `database`、`blob`、`queue`、`key_manager` | データベースとブロブストレージが保持するすべて。形式は事業者の保存方法によります。クラウドのキューにあるジョブのペイロードは ID と種類を持ち、アドレス、メッセージの本文、秘密情報は含みません。 |
| `{{IF cdn}}` | `media_origin` | 読む人の IP アドレスとメディアへのリクエスト（[CDN](cdn.md)）。 |
| プロキシ | サーバーが TLS のために必要とする、あなたのリバースプロキシまたはロードバランサー | すべてのリクエストとその IP アドレス。記録はあなたが設定したログの範囲です。 |
| `{{IF mail}}` | `mail` が `smtp` または `ses` | 宛先のアドレス、件名、本文。通知メールには通知と同じ文章が入ります（[メール](email.md)）。 |
| `{{IF captcha}}` | `captcha` が `none` 以外 | ログイン、登録、パスワード再設定の申し込みのページを開いたすべての人の IP アドレス、ブラウザーと端末の情報、Cookie、ウィジェットの操作。これらはその人のブラウザーから直接送られます。サーバーがサービスに送るのはトークンだけで、IP アドレスは加えません（[CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)）。 |
| `{{IF login providers}}` | [ログインプロバイダー](login-providers.md) | プロバイダーはログインを知ります。サーバーはその人に対するプロバイダーの識別子を受け取って保存し、登録のときにはプロバイダーが渡すメールアドレスとユーザー名を使います。 |
| `{{IF stripe}}` | `payments = stripe` | Checkout にあらかじめ入力するメールアドレス、プラン、ローカルのサブスクリプション ID。支払いの情報は Stripe が自身のページで集めます（[Stripe](payments-stripe.md#card-data-and-pci-dss)）。 |
| `{{IF payjp}}` | `payments = payjp` | メールアドレスとプラン。ブラウザーはサーバーの支払いのページで payjp.js を読み込み、カードは PAY.JP の iframe に入力され、名前とメールアドレスは 3-D セキュアのために PAY.JP に送られます（[PAY.JP](payments-payjp.md#card-data-and-pci-dss)）。 |
| `{{IF error reports}}` | `errors_exporter` | アカウント、IP アドレス、本文、ヘッダーを取り除いた報告。取り除く処理はパターンで照合するため、利用者の文章の断片が残ることがあります（[エラーの報告](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)）。 |
| `{{IF telemetry}}` | `metrics_exporter`、`traces_exporter` | アカウントやクライアントの IP アドレスを含まないメトリクスとスパン。外向きのリクエストのスパンには、リモートのサーバーのホスト名が入ります（[運用](running.md)）。 |
| `{{IF voice media server}}` | `voice` が `livekit` または `sora` | 参加者の IP アドレス、話している間の音声、セッション ID、ルームの ID。アカウントの ID は送られません（[通話](voice.md)）。 |
| `{{IF turn}}` | `voice_stun_urls`、`voice_turn` | STUN と TURN のサーバーは参加者の IP アドレスを知り、TURN サーバーは暗号化された音声を中継します。 |
| `{{IF webhooks}}` | [サーバーの Webhook](webhooks.md) | 受信側はイベントの本文を受け取ります。申請者のメールアドレスや通報のコメントを含むものもあります。 |
| `{{IF follow review}}` | `plugin_inbound_follow_address` が設定され、`hook_inbound_follow_mode` が `off` 以外 | ほかのサーバーからのフォローそれぞれについて、2 つのアカウントのアドレス。フォローが届いたときにサーバーが送ります（[判定フック](decision-hooks.md)）。記録される判定には、アクティビティのアドレスと、どのアカウントが送ったかが残ります。 |
| `{{IF sign-up review}}` | `plugin_signup_check_address` が設定され、`hook_signup_check_mode` が `off` 以外 | 招待なしで申し込んだそれぞれの人のメールアドレスと IP アドレス。登録のときにサーバーが送ります（[判定フック](decision-hooks.md#sign-ups)）。記録される判定にはどちらも残りません。プラグインを自分で動かすのでなければ、セクション 6 にそのサービスを書いてください。 |
| `{{IF post review}}` | `plugin_local_status_address` が設定され、`hook_local_status_mode` が `off` 以外 | 作成から `hook_local_status_account_age_days`（既定は 7 日。`0` はすべてのアカウント）に満たないアカウントの、`public`、`local`、`unlisted` の各投稿の文章（入力されたままのものと表示用に変換したもの）と、その注意書き、言語、センシティブの指定の有無、投票の選択肢、画像の説明、ハッシュタグ、メンションしているアカウントの数、投稿に付くアドレス、返信先と引用元の投稿のアドレス、投稿者のアドレスとユーザー名、アカウントの作成日時。投稿したときと、編集のたびにサーバーが送ります（[判定フック](decision-hooks.md#local-posts)）。`private` と `direct` の投稿は送られません。記録される判定には文章が残りません。拒否された投稿に異議を申し立てた投稿者はその文章を送り、サーバーはそれを判定とともに保存します。プラグインを自分で動かすのでなければ、セクション 6 にそのサービスを書いてください。 |
| `{{IF profile review}}` | `plugin_inbound_actor_address` が設定され、`hook_inbound_actor_mode` が `off` 以外 | ほかのサーバーのアカウントの公開プロフィール。変更のたびに、表示名、自己紹介、プロフィールの項目、アバターとヘッダー画像のアドレス（画像そのものは送らない）、アカウントのアドレス、サーバー、ユーザー名、このサーバーにとって新しいアカウントかどうか（[判定フック](decision-hooks.md#profiles-from-other-servers)）。あなたのサーバーのアカウントについては何も送られません。記録される判定には、アカウントのアドレスとプロフィールのダイジェストが残ります。 |
| `{{IF profile review}}` | `plugin_local_profile_address` が設定され、`hook_local_profile_mode` が `off` 以外 | 表示名、自己紹介（入力されたままのものと表示用に変換したもの）、プロフィールの項目、アバターやヘッダーを変えたかどうか、アカウントのアドレスとユーザー名、アカウントの作成日時。作成から `hook_local_profile_account_age_days`（既定は 7 日。`0` はすべてのアカウント）に満たないアカウントがこれらを変えるたびにサーバーが送ります（[判定フック](decision-hooks.md#local-profiles)）。画像は送られません。記録される判定にはどれも残りません。プラグインを自分で動かすのでなければ、セクション 6 にそのサービスを書いてください。 |
| `{{IF external review of posts}}` | `plugin_inbound_status_address` が設定され、`hook_inbound_status_mode` が `off` 以外 | ほかのサーバーが送ってくる、このサーバーのアカウントに関係する投稿とその改訂。フォロワー限定（`private`）の投稿も含みます。各言語の文章、注意書き、センシティブの指定の有無、公開日時、アドレスと URL、返信先の投稿のアドレス、投票の選択肢、ハッシュタグ、添付のアドレスと説明（メディアそのものは送らない）、投稿者とそのサーバー、公開範囲、名前を挙げているアカウントの数とそのうちこのサーバーにいる数、このサーバーの誰かが投稿者をフォローしているかどうか（[判定フック](decision-hooks.md#what-is-sent-and-what-it-costs)）。ダイレクトの投稿は、このサーバーの誰かが投稿者をフォローしているときだけ、または `hook_inbound_status_unfollowed_direct` が有効ならどの投稿者からのものでも送られます。 |
| `{{IF media review}}` | `plugin_media_check_address` が設定され、`hook_media_check_mode` が `off` 以外 | アップロードそれぞれの、メタデータを取り除いた最も小さい処理済みの画像（元のファイルは送らない）と、そのファイルの用途、種類、サイズ、長さ（[判定フック](decision-hooks.md#what-mediacheck-reviews)）。記録される判定には、ファイルの ID とアップロードした人が残ります。拒否されたアップロードはまったく保存されません。保全や報告が必要な場合は、プラグインの側で行う必要があります。プラグインを自分で動かすのでなければ、セクション 6 にそのサービスを書いてください。 |
| `{{IF translation}}` | `translation_providers` が空でない | 読む人が翻訳を求めたときの投稿の文章: 本文、注意書き、投票の選択肢、画像の説明。投稿のアドレス、投稿者、読む人、メンション、リンクは送られません。既定では、プロバイダーが受け取るのは `public`、`local`、`unlisted` の投稿です。プロバイダーはホスティングと同じくあなたの委託先として働くため、`local` の投稿は連合されないにもかかわらずプロバイダーに届きます。セクション 5 の `local` の文は残してください。文章を自分の目的のために保存するプロバイダーは、この意味での委託先ではなく、ポリシーにそのことを書く必要があります。 |
| DeepL の無料プランでの `{{IF translation}}` | `translation_providers` に、末尾が `:fx` のキーの `deepl` が含まれる | 同上。DeepL は文章と翻訳を無期限に保存することがあり、ポリシーにそのことを書く必要があります（[翻訳](translation.md#deepl)）。 |
| Amazon Translate での `{{IF translation}}` | `translation_providers` に `aws` が含まれる | 同上。あなたのアカウントに AWS Organizations の AI サービスのオプトアウトポリシーが適用されていない限り、Amazon は文章を保存し、自社のサービスの改善に使うことがあります。別のリージョンで行われることもあり、ポリシーにそのことを書く必要があります（[翻訳](translation.md#amazon-translate)）。 |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` がプロバイダーに `private` または `direct` を許可している | 同上。対象はそれらの投稿です。この目印は、プロバイダーごとに、あなたが許可したときだけ使ってください。セクション 2 と 5 に行が加わり、それらの投稿についての約束の範囲が広がるため、設定を変える前に告知してください。 |
| `{{IF translation of reports}}` | `translation_reports_to_server` | モデレーターが翻訳を求めたときの、通報のコメントとそれが挙げる投稿。各プロバイダーが受け取るのは、引き続き許可された公開範囲のものだけで、コメントは `private` として扱われます。`private` を許可されたプロバイダーがなければ、コメントはどこにも届きません。コメントの翻訳は保存されません。 |
| `{{IF browser translation}}` | `translation_browser` | このサーバーにもプロバイダーにも何も届きません。ブラウザーが読む人の端末で翻訳し、ブラウザーの提供元が何をするかはそのブラウザー次第です。 |

これらのサービスはあなたが選ぶものなので、このテンプレートは、それぞれがどこに所在し、どの契約の対象になるかを書いていません。どちらも自分で確認してください。

あなたの設定によらずに第三者に届くものが 2 つあります。

- **Web プッシュ。** 利用者がこれを有効にすると、その人のブラウザーがプッシュサービスを選び、そのアドレスをサーバーに渡します。メッセージの本文はそのブラウザー向けに暗号化されるため、プッシュサービスは読めないはずです。ただし、購読のアドレス、各メッセージのサイズと時刻、あなたのサーバーの連絡先（VAPID の subject）は見え、ブラウザーがオフラインの間はメッセージを最長 48 時間保持します。
- **連合。** テンプレートのセクション 5 を見てください。`federation_mode` で連合を止めたり方向を制限したりでき、リレーを追加するかどうかはあなたが決めます（[リレー](relays.md)）。

<!-- source: the-template 8b6bd2fc983a -->
## テンプレート

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, your posts that relate to an account here, followers-only posts included, go with their text to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. {{IF you set a maximum for keeping posts}} At most {{N days}} after you publish them, unless your account may keep them longer. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |
| {{IF translation of reports}} A report's comment and the posts it names | {{TRANSLATION SERVICES YOU ALLOWED `private` POSTS}} receive the comment, and the services in section 6 receive the posts as for a reader, when a moderator asks for a translation. They do not receive who reported, the report, or the posts' addresses |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts and their edits for abuse | Your `public`, `local`, and `unlisted` posts, each time you post or edit one, while your account is younger than {{7 days}}: their text, content warning, language, whether they are marked sensitive, poll options, image descriptions, hashtags, and how many accounts they mention, their address and the addresses of the posts they reply to or quote, your address and username, and when your account was created. Not your media | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening the profiles of accounts on other servers for abuse | Nothing about your account on this server. The public profile of an account on another server when this server takes in a change of it: its display name, bio, profile fields, the addresses of its avatar and header image, its address and username | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening changes of profiles for abuse | Your display name, bio, and profile fields, and whether you changed your avatar or header, each time you change them while your account is younger than {{7 days}}, with your address and username and when your account was created. Not your images | {{COUNTRY}} | {{LINK}} |
| {{IF media review}} {{MEDIA REVIEW SERVICE}} | Screening uploads for abuse before anyone can see them | A small processed copy of each image you upload, or of the poster or cover of a video or audio file, without its metadata, and what the file is for. Not the original, its description, or your account | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it, followers-only posts included: its text and language, content warning, whether it is marked sensitive, when it was published, its address, the address of the post it replies to, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, how many accounts it names and how many of them are here, and whether anyone here follows its author. Direct posts {{only when someone here follows their author, or from any author if you turned on `hook_inbound_status_unfollowed_direct`}}. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, an upload that an external review rejects is refused, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, {{IF profile review}} a change of your profile that an external review refuses, which you can change and save again, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
