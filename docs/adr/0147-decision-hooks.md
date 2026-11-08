# ADR-0147: 判定フックは判定だけを返し、処分は人が決める

- Status: Accepted
- Date: 2026-11-05
- Deciders: @rosylilly
- Issue: #2215

## Decision（決定）

**外部プラグインに「これを通してよいか」を受け付けの前に尋ねるフックを置く。プラグインが返すのは決まった語彙の判定だけで、本文は書き換えず、アカウントの処分もしない。失敗したら既定で通し、`observe` から始め、判定は本文なしで期限付きで記録して取り消せるようにする。**

プロトコルと接続の設定は ADR-0146 が持つ。

### 1. 原則

1. **判定だけを返す。** 投稿の本文を書き換えない。操られた分類器が失わせるのはスパム判定の精度までで、読者に見える文章は書けない。
2. **語彙は固定する。** 判定は `allow`・`drop`・`flag`・`mark_sensitive`・`require_approval`・`reject`。理由は列挙したコードで返す。自由な文章は、モデレーター向けの短い平文のメモ（200 文字まで）だけにする。
3. **処分は人が決める。** フックが止めるのは 1 つの対象までで、できるのは通報の起票まで。`moderation_actions` の行は作らない（`issuer` の CHECK は `moderator` だけのまま）。
4. **既定は通す。** フックの失敗で受け付けを止めない（§4）。
5. **モードはフックごとに `off`・`observe`・`enforce`。** `off` の次は必ず `observe` を経る（§5）。
6. **判定は記録し、取り消せる。** 投稿の本文は記録せず、保持期間を持つ（§6、§9）。
7. **読み取りの経路には置かない。** タイムラインと検索の順序にフックは効かない。

### 2. フックと判定

| フック | 呼ぶ場所 | 判定 | 失敗時の既定（選択肢） | 待ち時間の既定 |
| --- | --- | --- | --- | --- |
| `inbound.status` | `federation.inbox` の `Create` と `Update` | `allow`, `drop`, `flag`, `mark_sensitive` | 通す（`retry`） | 3 秒 |
| `inbound.follow` | `federation.inbox` の `Follow` | `allow`, `drop`, `flag` | 通す（`retry`） | 3 秒 |
| `signup.check` | `SignUp` と `SignUpExternal`。CAPTCHA と拒否リストの後。招待の登録は呼ばない | `allow`, `require_approval`, `reject`（運営者が `hook_signup_check_reject` を有効にしたときだけ） | 通す（`require_approval`） | 2 秒 |
| `media.check` | `media.process` の、結果が `ready` になる前。原本は送らない | `allow`, `mark_sensitive`, `flag`, `reject` | 通す（`retry`） | 10 秒 |
| `local.status` | `content.Service.Create` と `content.Service.Edit` のトランザクションの前。冪等な再送の確認の後。既定は作成から 7 日以内のアカウントだけ（`hook_local_status_account_age_days`） | `allow`, `flag`, `reject` | 通す（選択肢なし） | 1 秒 |
| `inbound.actor` | リモートの Actor のプロフィールの取り込み（初めての取得・`Update`・再取得）のトランザクションの前。フックに送るプロフィールが変わったときだけ | `allow`, `drop`, `flag` | 通す（`retry`） | 3 秒 |
| `local.profile` | ローカルのアカウントのプロフィールの更新のトランザクションの前。既定は作成から 7 日以内のアカウントだけ（`hook_local_profile_account_age_days`） | `allow`, `flag`, `reject` | 通す（選択肢なし） | 1 秒 |

- `media.check` の判定は `allow`・`mark_sensitive`・`flag`・`reject`。処理結果が出ると原本の blob が消えるので（`uploaded_media`）、`reject` は原本を残さない。保全や通報の義務は運営者とその国の話で、ソフトウェアでは決めない。使うかどうかはモードとプラグインの判定で運営者が選び、運用文書にこの性質を書く。
- `local.status` はローカルの投稿の編集も審査する。対象は作成と同じで、`hook_local_status_account_age_days` の範囲のアカウントだけ。対象は版ごとに別で、キーは `<投稿の ID>#<新しい版の ID>` とする。`reject` はその編集だけを拒み、投稿は編集前のまま残る。編集の `reject` には異議申し立てを付けない（投稿は残っていて、モデレーターは通常の操作で扱える）。`flag` は投稿と同じく通報を起票し（§7）、`allow` は通す。
- `inbound.actor` と `local.profile` が送るプロフィールは、表示名・自己紹介・プロフィールの項目と、アイコンとヘッダー（`inbound.actor` はその URL、`local.profile` は変わったかどうか）である。画像の中身は送らない。どちらも `flag` はアカウントへの通報を起票する（§7）。
- `inbound.actor` の `drop` は既知の Actor の更新を捨て、保存済みのプロフィールを前のまま残す。初めての取得の Actor は、フォローとメンションの解決に行が要るので捨てられず、`drop` を `flag` として扱う。プロフィールを書き換えて取り込むことはしない（§1 の 1）。
- `local.profile` の `reject` はその更新だけを拒み、プロフィールは更新の前のまま残る。異議申し立ては付けない（利用者は直して送り直せる）。拒んだ理由は見せず、決まった文を i18n のメッセージで出す（§12）。
- `Delete`・`Undo`・`Reject`、`Like`・リアクション・ブースト、配送の前にはフックを置かない。1 つのフックに複数のプラグインは連ねない。
- 事後の確認は webhook（ADR-0060）と API の役目で、ここには持ち込まない。
- フックを設定していないサーバーはプラグインを呼ばない。
- 投稿の本文を送る `inbound.status` と `local.status`、メールアドレスと IP アドレスを送る `signup.check` は平文の TCP を拒み、`inbound.follow`・`media.check`・公開のプロフィールを送る `inbound.actor` と `local.profile` は使える（ADR-0146 §6）。
- 設定はインスタンス設定に置く。フックごとのモードは `hook_<hook>_mode`（`<hook>` はフック名の `.` を `_` にしたもの。既定は `observe`。§5）。失敗時の選択肢は `hook_<hook>_on_failure` で、既定は `pass`（通す）、もう 1 つは表の括弧の中のもの。選択肢のない `local.status` はこの設定を持たない。`signup.check` の `reject` を効かせるのは `hook_signup_check_reject`（真偽値、既定は無効）で、無効の間は `reject` を記録するだけで登録を通す。`local.status` の対象は `hook_local_status_account_age_days`（既定は 7、`0` はすべてのアカウント）で絞る。`local.profile` も同じく `hook_local_profile_account_age_days` で絞り、`local.status` と同じく失敗時の選択肢を持たない。`inbound.status` の `direct` は §10 の `hook_inbound_status_unfollowed_direct`。

### 3. `inbound.status` の位置

`internal/app/federation/statuses.go` の次の場所で呼ぶ。プラグインの呼び出しはトランザクションの外で、結果をトランザクションに持ち込む（ADR-0003）。

- **`Create` と、未知の投稿の `Update`（`ingest`）**: `inScope` が真で、`input` が公開範囲を決め、宛先が無効なアカウントだけの `direct` を落とした後、`UnitOfWork.Tx` の前。`relayed` の経路（`inScope` が偽でリレーが運んだもの）では呼ばない。
- **既知の投稿の `Update`**: この経路は今 `inScope` を通らないので、フックの前に `inScope` を呼び、範囲外なら呼ばずに版を保存する。さもないと、リレーが運んだ投稿の更新がプラグインに届く。
- 投票（`vote`）と、保存済みの投稿への再度の `Create` は、この手前で返るので呼ばれない。
- `drop` は `ErrDropActivity` を包んで返す。ジョブは完了し、順序キーを止めない。
- 送るのは範囲内の投稿で、`any_local_follower`（`followed` の結果）を添える（§10）。

### 4. 失敗時と順序キー

`federation.inbox` の順序キーは `inbox:<actor の URI>` で、同じ Actor の後続のジョブは種類によらず前のジョブを待つ（ADR-0004 §18）。ハンドラのエラーは `queue.Retry` になり、`federation.inbox` は既定の方針で動く（`queue.RetryPolicy`: 10 回、30 秒から倍々）。したがってリトライを既定にすると、プラグインの障害で **その Actor の後続が最長で約 4 時間 15 分止まり、その中には `Delete` と `Undo` も入る**。プラグインの障害はすべての Actor に同時に及ぶので、既定は通す。

- `retry` は運営者が選ぶときだけ。文書にこの遅れを書く。回数を使い切ったジョブは dead job になり（順序キーは解放される）、再実行できる。
- タイムアウト・エラー・開いたブレーカーは、判定とは別の結果として記録する。
- 判定は 1 つのフックと対象につき 1 行で、ジョブの再実行は記録を読み、プラグインを呼ばない。例外は `retry` の失敗で、これは判定として記録せず（試行は `queue.AttemptLog` と指標に残る）、次の実行で呼び直す。

### 5. モード

- `observe` は判定を記録するだけで、効果を出さない。`enforce` で効果を出す。`off` は呼ばない。
- **`drop` は最初の版から持つ。** 最初は `observe` で動かす。誤った `drop` はフォロワーからも投稿を黙って隠すので、運営者は `observe` の記録を見てから `enforce` にする。
- モードはインスタンス設定に置き、管理画面で変える。

覆る条件: `observe` の記録から `drop` の誤りを評価できないと分かったとき。

### 6. 判定の記録

ADR-0020 の分類で次を足す。`schema/16-hook/<dialect>.sql` に置き、`internal/app/hook` が所有する。

- `hook_decisions`（イベント）: フック、対象のキー、モード、結果（判定または失敗の種類）、理由コード、メモ、待ち時間、日時。`(hook, subject_key)` で一意。本文・メールアドレス・IP は持たない。`inbound.*` の対象のキーは受信したアクティビティの URI で、`Update` の版ごとに別の判定になる。`local.status` のキーは新しい投稿ではその投稿の ID、編集では `<投稿の ID>#<新しい版の ID>` で、編集ごとに別の判定になる（§2）。`inbound.actor` のキーは `<Actor の URI>#<送るプロフィールのハッシュ>` で、プロフィールの中身ごとに 1 つの判定になり、同じプロフィールの再取得はプラグインを呼ばない。`local.profile` のキーは `<アカウントの ID>#<プロフィールの版の ID>` である。
- `hook_decision_statuses`（イベントの子）: 効果が及んだ投稿。`moderation_action_statuses` に対応し、§8 の投影の元になる。`Create` では投稿の ID が決まる取り込みのトランザクションで書く。
- `hook_decision_overrides`（イベント）: モデレーターの取り消し（§9）。
- `hook_decision_appeals`（リソース）と `hook_decision_appeal_decisions`（イベント）: `local.status` の `reject` への異議申し立てとその審査（§9）。処分の記録と分けた、この判定のためだけの表で、`moderation_appeals` と `moderation_appeal_decisions` と同じ形をとる。`hook_decision_appeals` は作者が送る本文を持つ。判定が本文を持たず、審査に要るからで、判定の表に本文がない原則はそのままである。1 つの判定に 1 件まで。
- `hook_account_exemptions`（現在値）: 申し立てが覆って対象から外れたアカウントとフックの組。行の存在が現在値で（ADR-0020 規約 5）、保持期間で消える判定や申し立てから導かない。

判定は呼び出しの直後に短いトランザクションで書く。その後の取り込みが失敗しても、再実行は同じ判定を読む。

**保持期間: 既定 90 日、下限 30 日**（`hook_decisions_retention_days`）。定期ジョブが消す。次は消さない。

- §8 の投影が指している判定。投影を再構築できなくなるため（ADR-0020 規約 1）。投稿がなくなれば消せる。
- `retention_holds` が保全するアカウントの判定（ADR-0058 §8）。
- `local.status` の `reject` のうち、投稿の作成を拒んだもので、異議申し立ての期間内か審査待ちの判定。期間は `appeal_window_days` に合わせる。編集の `reject` には異議申し立てがないので（§9）、この扱いはなく、ほかの判定と同じ保持期間で消す。

申し立ての行は判定と一緒に消す。アカウントの削除では、そのアカウントの判定・申し立て・`hook_account_exemptions` の行を消す（保全の間を除く）。

### 7. 自動の通報

`flag` は通報を起票する。

- `reporter_account_id` は**インスタンス Actor** にする。`accounts` に行があり（`Service.EnsureInstanceActor`、`InstanceActorTx`）、`NOT NULL` を変えずに済む。
- 出所は `hook_decision_id` で記録する。`report_voice_rooms` と同じく通報の子のリソース `report_hook_origins`（`report_id` PK、`hook_decision_id` 一意、`hook`、`reason_code`）に置く。一意制約が再実行での二重の起票を防ぐ。フックと理由は不変の事実なので写す（ADR-0020 規約 4）。判定が先に消えても、通報は自動のものと分かり、理由が読める。
- **1 つの判定に 1 件の通報** で、まとめない。リモートのサーバーへ自動で転送しない。`uri` は NULL、`forwarded` は偽にする。転送するかはモデレーターが通常の後からの転送（ADR-0058 §6）で決め、その `Flag` はメモを運ばない。`content` はモデレーターが転送するときに書く文（既定は空）だけにする。メモはこのサーバーのモデレーター向けのプラグインの出力だから。カテゴリは理由コードから決め、`comment` はメモ（なければ空）にする。投稿と同じトランザクションで書くので、`moderation` にトランザクションの中で起票する入口を足す。`FileRemoteReport` は URI を要し、`LocalReports.File` は閲覧者の確認をするので、どちらも使えない。
- `EventReportResolved` の通知の宛先は `reporter_account_id` で、インスタンス Actor はローカルなので、今のままでは通知の行を作りに行く。自動の通報は通知しない。
- 管理 API の通報と webhook `report.created`（管理 API の表現を写す）に、自動の起票であることを足す。

### 8. `mark_sensitive`

判定の効果は処分と同じ投影 `status_sensitivity_overrides` に載せる。ただし 1 行の出所は **モデレーションの処分か判定のどちらか一方だけ** で、表の形で強制する。自動の判定と人の処分を記録の上で混ぜないため。

```sql
-- Projection (content): a status shown as sensitive because of a mark_statuses_sensitive action
-- or a mark_sensitive decision of a hook, until that source ends. source_kind names what
-- source_id points to.
CREATE TABLE status_sensitivity_overrides (
  status_id uuid NOT NULL,
  source_kind text NOT NULL CHECK (source_kind IN ('action', 'hook_decision')),
  source_id uuid NOT NULL,
  PRIMARY KEY (status_id, source_kind, source_id)
);
CREATE INDEX status_sensitivity_overrides_source_kind_source_id ON status_sensitivity_overrides (source_kind, source_id);
```

主キーは `(status_id, source_kind, source_id)` とする。`source_kind` が `action` なら `source_id` は `moderation_actions`、`hook_decision` なら `hook_decisions` の行を指す。1 行が持つ出所は形の上で 1 つだけなので、排他のための NULL 可の列も CHECK も要らない。処分と判定の両方が同じ投稿に付けば 2 行になる。キーは再構築しても変わらないので、`my projection verify` が全列で行を比べても毎回のずれにはならない。代理キー `id` は足さない。再構築のたびに変わる ID はずれと報告されるため。主キーがあるので、論理レプリケーションの publication がある PostgreSQL でもこの表の `DELETE` は止まらない。

- **書き込み**: `content` に `MarkSensitiveByHook(ctx, tx, statusID, decisionID)` と `ClearSensitivityOfHookDecision(ctx, tx, decisionID)` を足し、`InsertStatusSensitivityOverride` を出所の種類ごとの文に分ける（`ON CONFLICT` はどちらも主キー）。`inbound.status` は取り込みか改訂のトランザクションで書く。`media.check` は投稿より先に動くので、添付を持つ投稿を作るトランザクションで、投影の行と `hook_decision_statuses` を書く。`avatar`・`header`・`emoji` には投稿がないので `mark_sensitive` は効かず、記録だけが残る。
- **読み取り**: 変えない。`SensitivityOverridden` は行の有無だけを見るので、`Reader.forcedSensitive` と `preview.go` はそのまま動く。
- **再構築**: `SensitivityMarks` の `mark` は処分か判定のどちらかを受け取る。`internal/platform/projections.go` の `status_sensitivity_overrides` は `moderation.ReplaySensitivityMarks` に加えて `hook.ReplaySensitivityMarks`（`enforce` で `mark_sensitive`、取り消しのない判定の `hook_decision_statuses`）を流す。`content` は他のモジュールの表を読まない。`content.RebuildSensitivityOverrides` は `statuses` に行のない投稿を落とす。投稿の物理削除は投影の行を消すが、判定の子の行はそれより長く残り得るため。保持期間が判定を消して行が戻らなくなることは §6 が防ぐ。
- **何を消すか**: 処分の解除は `source_kind = 'action'` でその処分の行だけを、判定の取り消しは `source_kind = 'hook_decision'` でその判定の行だけを消す。両方の行があれば、片方が消えても投稿はセンシティブのまま。どちらも作者が付けた `Sensitive`（版の列）と `force_sensitive` の制限には触らない。投稿の物理削除（`PurgeStatusSensitivityOverrides`）は両方を消す。
- **効く範囲**: 今の投影と同じ。添付を閲覧者を問わずセンシティブとして表示し、プレビューに画像を出さない。本文は隠さず、添付のない投稿には見える効果がない。`internal/app/federation/document.go` は版の `Sensitive` だけを使うので、連合先には伝わらない。
- **版をまたぐ**: 行は投稿に付くので、後の版が `allow` でも残る。外すのはモデレーターの取り消しだけである。再構築が単純で、失敗の通過と紛れない。

v0.1.0 の前なので移行は要らない（ADR-0023）。

### 9. 取り消し

モデレーターは管理画面の一覧から判定を取り消す。取り消しは `hook_decision_overrides` の行になり、監査ログに残る。起きることは判定によって違う。

| 判定 | 取り消しで起きること |
| --- | --- |
| `mark_sensitive` | §8 の判定の行を消す |
| `flag` | 通報はそのまま。解決はモデレーターの通常の操作で行う |
| `drop` | 原本が残っていれば `federation.inbox` を積み直す。再実行は取り消しのある判定を `allow` と読むので、プラグインを呼ばない。原本は `received_activity_documents` に 7 日（`ReceivedDocumentRetention`）しかなく、それを過ぎたら戻せず、画面がそう示す |
| `reject`（`signup.check`） | 申請は残らないので、記録だけ。応募者は登録し直す |
| `reject`（`local.status`） | 投稿は残らないので、取り消しは下の異議申し立ての審査で行う |
| `require_approval` | 登録審査の通常の操作で承認する |

**`local.status` の `reject` への異議申し立て。** 作者は `appeal_window_days` の間に、投稿しようとした本文を添えて申し立てる。審査は人が行い、プラグインを呼ばない。退ければ判定はそのままである。覆すと、同じトランザクションで判定を取り消し（`hook_decision_overrides`）、そのアカウントを `local.status` の対象から外す（`hook_account_exemptions`）。投稿は戻さず、作者が投稿し直す。申し立てられるのは新しい投稿の `reject` だけで、編集の `reject` は申し立てられない（§2）。

**申し立ての知らせと読み取り。** 既存の通知の仕組みを使う。

- 申し立てが届くと、`manage_reports` を持つモデレーター（申し立てた本人を除く）に `admin_hook_appeal` を通知する。通知の設定ではモデレーター向けの 1 行（`admin_hook_appeal`）で、メールも選べる。
- 審査されると、維持でも覆しでも、作者に `hook_appeal_decided` を通知する。インスタンスからの知らせ（`moderation` の群）に入る。通知は申し立ての状態（`pending`・`upheld`・`overturned`）と審査の日時だけを持ち、判定の理由やメモは持たない（§12）。
- 作者は `GET /hook_decisions/{id}/appeal`（第一者のクライアントだけ）で、自分の申し立ての状態を読める。投稿画面は申し立てを送った後にこれを読み、審査の結果を拒否の知らせに出す。拒まれた投稿の一覧は作らない。

`drop` は tombstone を残さない。受信の重複排除（`received_activities`、30 日）が同じアクティビティを止めるが、スレッドの取得（`threads.go`）や `Announce` の取得は別の経路で同じ投稿を運び得る。それらは実際にすり抜けが出るまで広げない。

### 10. `inbound.status` の対象

`inbound.status` の対象を、ローカルの誰もフォローしていない作者に絞らない。範囲内（`inScope`）の投稿すべてが対象で、プラグインは `any_local_follower` を見て重みを変えられる。

フォローされていない作者の `direct` を送るかは運営者が選ぶ設定 `hook_inbound_status_unfollowed_direct`（真偽値）で、既定は送らない。ローカルの `private` と `direct` の投稿はどの設定でも送らない。

### 11. 監査ログ

- モードの変更は、既存の `setting_changed`（`actor_kind = user` か `cli`）で残す。
- 判定の取り消しは、新しい `hook_decision_overridden` と `target_kind = hook_decision` で残す。申し立てを覆したときも同じ。
- 申し立てを退けたときは、`hook_appeal_upheld`（`target_kind = hook_decision`、対象は判定）で残す。判定は変わらないが、人の決定なので記録する。
- 自動の効果のうち、残っているものに状態を残すもの（`mark_sensitive`、`flag` の起票、`require_approval`）は `actor_kind = system` の `hook_decision_applied` で残す。何も保存しない `drop` と `reject` は `hook_decisions` にだけ残す。監査ログの保持は 730 日で表示名の写しを持つので（ADR-0058 §8）、スパムの波の 1 件ごとを書くと、§6 の短い保持が意味をなくす。

覆る条件: `drop` にも監査ログが要ると法令の確認で分かったとき。

### 12. 利用者と運営者への説明

- インスタンスの情報ページに、外部の審査を使っていること、対象、送るデータを、設定から生成して出す。
- 申請者に判定の理由は見せない。`reject` は決まった文を i18n のメッセージで出し、プラグインの文章を出さない。
- 運営者向けの文書に、送るデータ（`signup.check` のメールアドレスと IP、`inbound.actor` と `local.profile` のプロフィール）、受信量に比例する費用、誤った `drop` が黙って投稿を隠すことを書く。

### 13. 作る順序

`v0.1.0` までに #2222 から #2227 と、プロフィールのフック #2431（`inbound.actor`）・#2432（`local.profile`）を作る。順序は #2222、#2223、#2224、#2227、#2226、#2225 で、どのフックも `observe` を既定に出す。

## 経緯

FR-719 の拒否リストと CAPTCHA は固定の規則にしか効かず、FR-701 の通報と ADR-0060 の webhook は起きた後にしか効かない。連合のメンションスパムのように、受け付ける前に止めることに価値がある場所に限ってフックを置く。

最初に原則を決めた。判定だけを返すのは、プラグインが操られても読者に届く文章を作れないようにするためで、固定の語彙がそれを保つ。既定で通すのは、順序キーのために失敗の影響が 1 件で済まないから。

`drop` は誤りの代償が大きい一方で最も価値のある判定なので、外すのではなく `observe` から始める形にした。通報者は、`NOT NULL` を保てる行のあるインスタンス Actor にした。`mark_sensitive` は、投影の出所の列が処分しか指せないことが問題で、処分を作ると原則の 3 に反するので、出所を処分か判定のどちらか一方にした。その形は、はじめ NULL 可の 2 列と排他の CHECK で、主キーのない表だった。主キーのない表は、論理レプリケーションの publication がある PostgreSQL（Cloud SQL の移行や blue-green の更新が作る）で `DELETE` が失敗し、処分の解除・判定の取り消し・保持の削除が止まる。v0.1.0 の前で投影は作り直せるため、`(status_id, source_kind, source_id)` を主キーとする形に改めた。これに伴い、保持期間のある判定から投影を作ることになり、§6 が投影の指す判定を残す形になった。`media.check` の `reject` は原本を残さないので、保全や通報の義務をソフトウェアでは決めず、運営者がモードとプラグインで選び、運用文書にこの性質を書く形で判定に入れた（§2）。`local.status` の `reject` には、今の `moderation_appeals.action_id` が処分しか指せないので、専用の異議申し立てを付けた。保持期間の設定の名前は、ほかの保持期間の設定（`<対象>_retention_days`）と `hook_` で始まるフックの設定に揃えて `hook_decisions_retention_days` とした。

編集を審査しないと、日の浅いアカウントが無害な投稿を後からスパムに書き換えられる。作成と同じ対象に絞って編集も審査し、拒むのはその編集だけにした。申し立てには知らせがなく、作者は結果を知る手段がなく、モデレーターは画面を見に行くしかなかったので、既存の通知の仕組みで両方に知らせ、作者が状態を読む API を足した。維持にも監査ログを残すのは、覆しが `hook_decision_overridden` で残るのと揃えるためである。

プロフィールのフックは、投稿・フォロー・登録・メディアのフックの後に、決定者が `v0.1.0` の前に足すと決めた。連合のスパムは表示名や自己紹介の URL でも来るのに、リモートの Actor の取り込みとローカルのプロフィールの更新はどのフックも通らなかったからである。フックの名前はスキーマの `CHECK` にあり、`my.plugin.hook.v1` はタグで凍るので、後から足すとスキーマの更新が要る。

## 採らなかった案

- プラグインに本文の書き換えを許す: 汚れた部分だけを除けるが、操られたプラグインが読者に見える文章を作れる。
- `mark_sensitive` や `drop` でシステムの `moderation_actions` を作る（`issuer` に `hook` を足す）: 解除・監査・投影の仕組みをそのまま使えるが、自動の判定と人の処分が同じ記録に混ざり、モデレーターが見る相手の処分の履歴（FR-703）と異議申し立ての対象が濁る。
- 判定用のセンシティブの投影を別の表にする: 既存の PK を保てるが、`SensitivityOverridden` と再構築が 2 つの表を見ることになる。読み手が知りたいのは出所ではなく有無である。
- 判定を保持期間どおりに消し、投影の行は消す、または再構築が行を残す（`webhook_endpoint_failures` の方式）: 判定を例外なく消せるが、前者はセンシティブの表示が黙って外れ、後者は行の出所をたどれず取り消せない。
- 自動の通報の `reporter_account_id` を NULL にする、専用の `reporter_kind` を持たせる: 人でないことを直接表せるが、通報者を読む経路（`ReportsFiledBy`、管理 API、書き出し）すべてを直すことになる。
- 既定を `retry` にする: 審査されていない投稿を通さないが、§4 のとおり Actor の `Delete` まで止まる。
- `inbound.status` を、フォローされていない作者に絞る: 費用と送るデータを大きく減らせるが、フォローされた作者や乗っ取られたアカウントからのスパムを見られない。
- `status_sensitivity_overrides` の出所を NULL 可の `source_action_id` と `source_hook_decision_id` に分け、排他の CHECK で 1 つに絞る（主キーなし）: 列名でどちらを指すかが読めるが、主キーが持てず、論理レプリケーションの publication がある PostgreSQL では `DELETE` が失敗する。
- `status_sensitivity_overrides` を主キーなしのまま、PostgreSQL で `REPLICA IDENTITY FULL` を設定する: 形を変えずに済むが、Atlas CE の扱う範囲の外で、運営者の手順に頼ることになる。
- 後の版が `allow` のとき判定から来たセンシティブの行を外す: 版に判定が付くことと揃うが、再構築が投稿ごとの最新の判定を見ることになる。
- `media.check` から `reject` を外す: 原本が消える判定がなくなるが、運営者が公開前に止められない。法令への対応はソフトウェアではなく運営者が決めることなので、手段を減らさない。
- `local.status` の `reject` に異議申し立てを付けない: 表も審査も増えないが、作者が誤りを正す道がなくなる。
- 異議申し立てを既存の `moderation_appeals` に載せる: 審査の仕組みを共有できるが、`action_id` が処分を指すので、処分のない判定を載せると原則の 3 の記録の分離が崩れる。
- 編集を審査しない: 費用は最小だが、日の浅いアカウントが投稿を後からスパムに書き換えれば審査をすり抜ける。
- 編集の `reject` にも異議申し立てを付ける: 作者が誤りを正せるが、投稿は残っていてモデレーターが通常の操作で扱えるので、表と審査を増やすほどの価値がない。
- 作者が拒まれた投稿の一覧と申し立ての期限を読めるようにする: タブを閉じても申し立てられるが、拒まれた投稿の記録を作者に見せる面が増える。使われ方を見てから考える。
- 申し立ての通知を持たず、モデレーターが画面を見に行き、作者は投稿し直して確かめる: 何も足さずに済むが、§9 の「作者が投稿し直す」が結果を知らないまま行われる。
- 拒否したものも監査ログにすべて書く: 1 つの場所で追えるが、§11 のとおり保持の短さを損なう。

## Consequences（影響）

- 良い: 運営者は自分の信号で受け付けを止められ、`observe` で先に試せる。プラグインが落ちても受け付けは続く。自動の判定と人の処分が記録の上で分かれ、処分の履歴は人の決定だけのままである。
- 悪い: 送るデータと費用が受信量に比例して増える。既定で通すので、プラグインの障害の間はスパムが入る（指標とモデレーターへの通知で気づく）。
- 悪い: `flag` は投稿ごとに通報を作るので、スパムの波でモデレーターの列があふれ得る。`FirstOpen` は通知を抑えるが、列は減らない。
- 悪い: `media.check` の `reject` は原本を残さないので、運営者に保全の義務があるなら、それはプラグインの側で行う。
- 悪い: `local.status` の異議申し立ては、投稿を拒まれた作者の本文を一時的に保存し、モデレーターの審査の列を増やす。対象から外れたアカウントは以後審査されないので、乗っ取られても `local.status` は働かない。
- 悪い: `drop` を戻せるのは 7 日だけで、`drop` した投稿が別の経路で入ることがある（§9）。
- 悪い: `status_sensitivity_overrides` の `source_id` は `source_kind` を見ないと何を指すか分からない。判定を残すために §6 の保持に例外ができる。
- 後続: data-model §4.4・§5.2・§8・§9、`schema/` の両方言、`docs/operations/`、`tools/agentkit/context-map.yaml` は #2222 以降のタスクで直す。

## Open Questions / 要検証

法令への適合（違法なコンテンツの保全と通報、自動の判断の説明や異議申し立ての義務）は、インスタンスの運営者とその所属国の問題で、ソフトウェアでは制御しない。`my` は手段（モード、記録、異議申し立て）を用意し、どう使うかを運営者に委ねる。

- 既定値（保持 90 日、§2 の待ち時間、メモの 200 文字）は、#2223 の `observe` の運用で確かめる。
