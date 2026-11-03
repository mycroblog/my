# ADR-0076: 連合の向きをインスタンス設定で選ぶ（オフ / 受信のみ / 送信のみ / 送受信）

- Status: Accepted（`Move` の送信を `retract` の流れに加える部分を ADR-0089（アカウントの移転（Move）と別名（alsoKnownAs））が、閉鎖の間の流れの扱いを ADR-0088（サービスの終了の予告モードとクローズモード）が加える）
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: #976

## Context（背景・課題）

いまの連合（ActivityPub S2S、[ADR-0006](0006-federation-protocol-scope.md)）は常に送受信とも有効で、運営者は向きを選べない。運営者が選べるのは、ドメインごとの処分（FR-702、[federation.md §11](../architecture/federation.md#11-モデレーションとの連携)）と Authorized Fetch（FR-408）だけである。決定者は、次の 4 つの使い方をインスタンス設定で選べるようにすることを求めている（#976）。

- **オフ**: 連合しない、閉じたサーバー
- **受信のみ**: 外のアカウントをフォローして読むが、こちらの投稿は外へ出さない
- **送信のみ**: 外のアカウントを読む気はないが、外からのフォローは受け付け、こちらの投稿を届ける
- **送受信**: いまの振る舞い

向きは HTTP の向きでは決められない。受信のみでも、外のアカウントをフォローするには `Follow` を送り、相手が署名を検証できるようにこちらの Actor の鍵を取得させる必要がある。送信のみでも、外からの `Follow` を受け取って `Accept` を返す必要がある。決めるべきなのは、**どの内容（投稿・プロフィール・反応）がどちらへ流れるか**と、それを成り立たせる関係の手続き（フォローの往復・鍵の取得）をどこまで通すかである。

判定は、受信（federation.md §3）・取得（§6）・配送（§7）・GET（§10）・WebFinger と NodeInfo（FR-401・FR-402）・リレー（ADR-0061）・処分の伝播（ADR-0036）・client API と Web の各入口にまたがる。実装の前に、各モードで何が通るかの表と、判定を 1 か所に集める形を決める。

関連する要件: FR-202（連合タイムライン）、FR-305（URL の検索）、FR-401〜FR-411、FR-702・FR-709（ドメインの処分・通報の転送）。関連する決定: [ADR-0017](0017-visibility-local.md)（`local`）、[ADR-0018](0018-federated-uris.md)（URI）、[ADR-0033](0033-inbox-processing-model.md)（受信）、[ADR-0034](0034-remote-content-scope-and-retention.md)（取り込み範囲）、[ADR-0036](0036-moderation-federation-propagation.md)、[ADR-0061](0061-federation-relays.md)、[ADR-0069](0069-quote-posts.md)（引用）。

## Decision Drivers（判断基準）

1. **運営者の意図どおりに内容が流れない**こと。受信のみでこちらの投稿・プロフィールが外へ出ない、送信のみで外の投稿を取り込まない、オフで何も送受信しない。
2. **相手のサーバーから見て壊れて見えない**こと。フォローが承認待ちのまま残る、Actor が `410` で消えたと扱われる、といった戻せない誤解を生まない。
3. **戻せる**こと。モードを狭めて広げ直したとき、URI（ADR-0018）とフォロー関係が保たれ、そのまま再開できる。
4. **判定が 1 か所**であること。visibility のポリシー関数と同じく、入口ごとに条件を書き散らさない。
5. 運営者に分かりやすい設定の形であること。不正な組み合わせが無い。

## Considered Options（選択肢）

設定の形:

1. **1 つの enum**: `federation_mode` = `off` / `inbound` / `outbound` / `both`（既定 `both`）
2. **2 つの bool**: `federation_inbound` / `federation_outbound`（既定ともに `true`）
3. **Mastodon の限定連合モード（`LIMITED_FEDERATION_MODE`）のような許可リスト**: 許可したドメインとだけ送受信する

向きの区切り方:

- A. **内容の流れで区切る**: 投稿・プロフィール・反応を「外へ出す内容」「取り込む内容」に分け、フォローの往復と鍵の取得はそれぞれの向きに要る分だけ通す
- B. **HTTP の向きで区切る**: 受信のみは inbox だけ開け、配送を止める

B は成り立たない（受信のみでフォローを送れない、送信のみで `Follow` を受けられない）ので、A を前提に設定の形を比べる。

## Decision（決定）

**選択肢 1（1 つの enum）+ A（内容の流れで区切る）を採用する。** タイムラインの公開範囲（`local_timeline_visibility` など、#975）と同じく、状態を 1 つの enum で表す。

### 設定

- インスタンス設定 `federation_mode`（`KindEnum`、`off` / `inbound` / `outbound` / `both`、既定 `both`）。`instance_setting_revisions` に記録し（変更者と時刻が残る）、`manage_settings` で管理 API・CLI・管理画面から変える。リクエストとジョブごとに読む（`authorized_fetch` と同じく、変更はすぐに効く）。
- 管理画面は、モードを狭める変更の前に、下の「切り替え」の影響（届かなくなるもの、承認待ちのまま残るもの）を示して確認を求める。

### 流れの分類

判定は、`internal/domain/federation` の 1 つの関数 `federation.Allows(mode, Flow) bool` に集める。各入口は自分の扱う `Flow` を渡して聞くだけにし、モードの値で分岐しない。

| `Flow` | 内容 | `off` | `inbound` | `outbound` | `both` |
| --- | --- | --- | --- | --- | --- |
| `publish` | ローカルの投稿・プロフィール・反応を外へ出す（配送と GET） | × | × | ○ | ○ |
| `ingest` | 外の投稿・反応を取り込む（受信と取得） | × | ○ | × | ○ |
| `follow_out` | ローカルの利用者が外のアカウントをフォローする往復（`Follow` / `Undo Follow` の送信、`Accept` / `Reject` の受信） | × | ○ | × | ○ |
| `follow_in` | 外のアカウントがローカルの利用者をフォローする往復（`Follow` / `Undo Follow` の受信、`Accept` / `Reject` の送信） | × | 断る | ○ | ○ |
| `retract` | すでに外へ出したものの撤回（投稿の `Delete`、Actor の `Delete`、`Undo Like` / `Undo Announce` / `Undo EmojiReact`、凍結の `Update(Actor)`） | × | ○ | ○ | ○ |
| `protect` | 保護のための手続き（`Block` / `Undo Block` の送受信、`Flag` の送受信、外の Actor の自分自身の `Update` / `Delete` の受信） | × | ○ | ○ | ○ |
| `identify` | 署名の検証のための鍵の取得と提供（外の Actor の取得、ローカルの Actor・インスタンス Actor の GET、WebFinger・host-meta・NodeInfo） | × | ○ | ○ | ○ |

- `inbound` の `follow_in` の「断る」は、受けた `Follow` に `Reject` を返す（承認待ちのまま残さない。判断基準 2）。
- `retract` は、`inbound` に狭めたあとも、それまでに外へ出したものを消せるようにするために通す。宛先は元の配送（`deliveries`）とその時点のフォロワーで、新しい宛先は増やさない。
- `off` は何も通さない。オフの間の削除は外に伝わらない（悪い影響を参照）。

### 入口ごとの振る舞い

| 入口 | `off` | `inbound` | `outbound` | `both` |
| --- | --- | --- | --- | --- |
| inbox（同期、federation.md §3.1） | 署名の検証より前に `403`（ドメインの `suspend` と同じ位置） | 受け付ける | 受け付ける | いまのまま |
| inbox（非同期、§3.2 のハンドラ） | — | `ingest`・`follow_out`・`follow_in`（断る）・`protect` の型だけ処理し、ほかは `dropped` | `follow_in`・`protect` の型だけ処理し、ほかは `dropped`（`QuoteRequest` には `Reject` を返す） | いまのまま |
| 展開と配送（§7.2・§7.3） | すべて `skipped` | `follow_out`・`follow_in` の `Reject`・`retract`・`protect` だけ | `publish`・`follow_in`・`retract`・`protect` だけ | いまのまま |
| 外の取得（§6） | しない | いまのまま | 署名の検証と配送の宛先に要る Actor の取得だけ。投稿・スレッド・outbox の取得はしない。メディアプロキシはフォロワーの Actor の画像だけ | いまのまま |
| ローカルの Actor の GET（§10） | `404`（`410` にしない） | 鍵の取得のための最小の文書（Authorized Fetch の署名なしと同じ項目） | いまのまま | いまのまま |
| 投稿・`/activity`・コレクション（outbox・followers・following・featured）の GET | `404` | `404`（outbox など空にせず存在を見せない） | いまのまま | いまのまま |
| インスタンス Actor の GET | `404` | いまのまま | いまのまま | いまのまま |
| WebFinger・host-meta | `404` | 答える（`follow_out` で相手がハンドルを確かめるため。Mastodon・GoToSocial は Actor を取得したあと WebFinger を引く） | いまのまま | いまのまま |
| NodeInfo | `404` | 答える。`metadata.federationMode` に値を出す | 同左 | 同左 |
| リレー（ADR-0061） | 参加・送信・受信をしない（登録と状態は残す） | 受信だけ（送信のフェーズを飛ばす） | 送信だけ（受信を捨てる） | いまのまま |

- `inbound` で外の Actor の文書に `Create` などの内容が埋め込まれていても、取り込みの範囲は ADR-0034 のまま（ローカルの利用者がフォローしている相手など）。モードは範囲を広げない。
- `inbound` で `Like` / `Announce` / `EmojiReact` がローカルの投稿に届いても、外から見えないはずのものなので捨てる。ローカルの利用者へのメンション（フォロー中の相手から）は `ingest` として取り込む。
- `outbound` で捨てるのは、ローカルの投稿への返信・メンション・ブースト・お気に入り・絵文字リアクション・投票への回答・引用の依頼を含む。外の反応は数にも通知にも出ない。
- ドメインの処分（§11）はモードと独立に効く。モードが通しても、処分が止めるものは止まる。

### client API と Web

- `GET /instance` に `federation.mode` を足し、Web はそれで外向きの操作を出し分ける。
- `ingest` か `follow_out` が通らないモード（`off` / `outbound`）では、外のアカウントのフォロー・URL とハンドルの解決（FR-305）を client API が `403`（専用の problem type `federation-restricted`）で断る。検索はローカルの結果だけを返す。Web は外のアカウントを探す・フォローする操作を出さない。
- `publish` が通らないモード（`off` / `inbound`）では、ローカルの利用者は外の投稿にお気に入り・ブースト・リアクション・返信ができる（ローカルに記録される）が、外には届かない。Web は外の投稿への操作と、外のアカウントへのメンション・返信の投稿画面に「連合先には届きません」と示す。引用の依頼は送れないので、外の投稿の引用は出さない。
- 文言は `locales/en`・`locales/ja` に足す（ADR-0009）。

### 切り替え

- モードを狭めても、**フォロー関係・取り込んだ投稿・配送の記録は消さず、`Undo` / `Reject` も送らない**。流れが止まるだけで、広げ直すとそのまま再開する（判断基準 3）。
  - 狭めている間に作られた投稿・反応は、広げ直しても遡って配送しない（展開は記録の時点のモードで決まり、`skipped` は再送しない）。
  - 外の側では、`inbound` の間はローカルの利用者のフォロワーが何も受け取らず、`outbound` / `off` の間はローカルの利用者のフォローが更新されない状態になる。
- 関係を切りたい運営者のために、CLI `my admin federation sever --direction in|out|both` を置く（ADR-0036 の `follow_terminations`、`reason = moderation` と同じ記録で、送れるモードでは `Reject` / `Undo` を送る）。モードの変更が自動で呼ぶことはしない。
- 展開と配送のジョブは、実行の時点のモードで判定する。切り替えの前に積んだジョブが、狭めた後に送られることはない。
- モードの変更は `instance_setting_revisions` に記録し、監査ログ（ADR-0037）にも `target_kind = setting` で残す（既存の設定の変更と同じ）。

### 文書と要件

- 機能要件に FR-413（連合の向き）を足す（この PR で更新する）。
- 決定のあと、federation.md（§3・§6・§7・§10・§13 と新しい節「連合の向き」）、data-model.md（§4.9 の `instance_setting_revisions.key`）、`docs/operations/instance-settings.md` を実装の PR で更新する。

### 相互運用の確認

federation-live（2026-10-22、[#983](https://github.com/mycroblog/my/issues/983#issuecomment-6337103257)）で、Mastodon 4.7・GoToSocial 0.22・Misskey 2026.10 を相手に次を確かめた。見直す条件には当たらない。

- `inbound` の最小の Actor の文書（名前・アイコン・コレクションなし）からの `Follow` を、3 つとも受け付けて `Accept` を返す。
- Mastodon と GoToSocial は、こちらの Actor を取得したあと `acct:` の WebFinger を引く。Misskey は引かない。`inbound` でも WebFinger に答える。
- `inbound` で外から受けた `Follow` は `Reject` になり、相手の側にもこちらの側にも承認待ちが残らない。
- `off` の Actor・WebFinger・NodeInfo は `404` を返す。Mastodon と Misskey は Actor を取り直して `404` を受けても、アカウントとフォローを消さない。`both` に戻すと投稿がまた届く。
- NodeInfo 2.1 のスキーマは `protocols` を `minItems: 1` とするので、空の配列は返せない。`off` は NodeInfo を `404` にする。

### この決定を見直す条件

- 主要実装（Mastodon・Misskey・GoToSocial・Akkoma）のどれかが、`inbound` の最小の Actor の文書でこちらの `Follow` を受け付けない（プロフィールの項目を必須にする）ことが相互運用テストで分かった場合: `inbound` でも Actor の文書全体を出す（プロフィールは外へ出る）か、`inbound` を外す。
- `outbound` の利用者が、外からの返信・反応を通知で受けたいと求めた場合: `outbound` に「反応だけ取り込む」段を足す（選択肢 2 の形のほうが表しやすくなる）。
- 許可したドメインとだけ連合したい要望が出た場合: 選択肢 3 を、この enum と直交するドメインの許可リストとして別の ADR で決める。

## Consequences（影響）

- 良い影響
  - 閉じたサーバー・読むだけのサーバー・発信だけのサーバーを、設定 1 つで作れる。いつでも `both` に戻せ、URI と関係はそのまま残る。
  - 判定が `federation.Allows` に集まり、入口のテストは「この入口はこの `Flow` を聞く」ことだけを確かめればよい。
  - 既定の `both` では、いまの振る舞いが変わらない。
- 悪い影響・トレードオフ
  - `off` の間の投稿・アカウントの削除は外に伝わらない。`off` に切り替える前に配送済みの投稿は外に残り続ける。管理画面の確認で示す。
  - `inbound` / `outbound` / `off` の間、外の側の関係は「つながっているが何も流れない」状態になる。相手の利用者には理由が見えない。
  - `inbound` では、ローカルの利用者が外の投稿にした返信・反応がローカルにだけ残る。Web の注記に頼る。
  - 入口が多く（受信・配送・取得・GET・WebFinger・NodeInfo・リレー・client API・Web）、実装とテストの量は小さくない。
- 後続で必要になる作業（Accepted の後に issue 化する）
  - server: 設定と `federation.Allows`、受信・配送・取得・GET・WebFinger・NodeInfo・リレーの各入口、`my admin federation sever`
  - client API: `GET /instance` の `federation.mode`、外向きの操作の `403`
  - Web: 管理画面の設定と確認、外向きの操作の出し分けと注記
  - 文書: federation.md・data-model.md・instance-settings.md
  - 相互運用テスト（federation-live）: `inbound` の最小の Actor の文書で Mastodon・Misskey へのフォローが通ること、`outbound` で外からのフォローが通ること

## Pros and Cons of the Options（詳細比較・任意）

### 1. 1 つの enum

- 良い: 4 つの状態がそのまま選択肢になり、管理画面・NodeInfo・`GET /instance` で 1 つの値として扱える。タイムラインの公開範囲（#975）と形が揃う。
- 悪い: 向きを足す（「反応だけ取り込む」など）と値が増え、組み合わせで表せない。

### 2. 2 つの bool

- 良い: 受信と送信を独立に考えられ、将来「反応の受信」などの軸を足しやすい。いちばん強い論拠は、`ingest` と `publish` がもともと独立の軸であること。
- 悪い: 両方 `false` が「オフ」を意味することが名前から読みにくい。管理画面で 2 つの設定の組み合わせの説明が要る。`follow_in` の「断る」のような、軸の組み合わせで決まる振る舞いを説明しにくい。

### 3. 許可リスト（Mastodon の限定連合モード）

- 良い: 「特定の相手とだけつながる」閉じたネットワークを作れる。Mastodon は `LIMITED_FEDERATION_MODE` で、承認したサーバーとだけ連合し、公開ページと一部の REST API を止める（[Mastodon の設定の文書](https://docs.joinmastodon.org/admin/config/)、2026-10-22 確認）。
- 悪い: 決定者が求めているのは相手ではなく向きの制御で、これだけでは「受信のみ」「送信のみ」を表せない。向きと直交するので、要るなら別に決める。

### ほかの実装

- Akkoma（Pleroma 系）は `config :pleroma, :instance, federating:` で連合の有無だけを切り替える（[Akkoma の設定の文書](https://docs.akkoma.dev/stable/configuration/cheatsheet/)、2026-10-22 確認）。向きは選べない。
- Misskey は管理画面で連合を「すべて / 指定したサーバーのみ / なし」から選べる（要検証。公式の文書で確認できていない）。これも向きではなく相手の制御である。

## Open Questions / 要検証

- Akkoma は federation-live の構成に無く、`inbound` の最小の Actor の文書からの `Follow` を受け付けるかを確かめていない（要検証）。
- GoToSocial は保存した Actor で答え、取り直さないため、`off` の `404` への振る舞いを確かめていない（要検証）。
- GoToSocial は、モードを `inbound` から広げたあとも最小の Actor の文書（`followers` なし）を持ち続け、`outbound` で届けたこちらの投稿が相手の利用者のホームに出ない。配送は `202` で受け付けられる。原因は確かめていない（要検証）。
- サーバーの一覧サイトなどに「ActivityPub を話さないが存在する」ことを出したい場合は、`off` でも NodeInfo を返す形（`protocols` は空にできない）を考える。
