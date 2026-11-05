# ADR-0101: 通話の接続と SFU: 方式のポート・P2P のシグナリング・STUN / TURN・LiveKit と Sora

- Status: Accepted
- Date: 2026-10-26
- Deciders: @rosylilly
- Issue: #1349, #1350

## Decision（決定）

通話の方式はインスタンスで 1 つ（`voice = off / p2p / livekit / sora`）で、全ルームが同じ方式を使う。§1〜§9 が方式に共通の部分と P2P、§10〜§16 が SFU（LiveKit のセルフホストと LiveKit Cloud、ライセンスを買うセルフホストの Sora と Sora Cloud）を決める。

### 1. 設定

デプロイの設定（[設定のリファレンス](../reference/configuration.md)、環境変数 `MY_*`。秘密は `_FILE` でも渡せ、TOML には書けない）に置く。

| キー | 値 | 既定 |
| --- | --- | --- |
| `voice` | `off` / `p2p` / `livekit` / `sora` | `off` |
| `voice_p2p_max_participants` | P2P のルームの人数の上限（2〜50） | `10` |
| `voice_sfu_max_participants` | SFU のルームの人数の上限（0 は上限なし） | `10` |
| `voice_max_room_duration` | 1 ルームの最長時間（0 は上限なし） | `4h` |
| `voice_max_open_rooms` | 同時に開けるルームの数（0 は上限なし） | `10` |
| `voice_stun_urls` | STUN の URL の一覧（`stun:` / `stuns:`） | なし |
| `voice_turn` | `none` / `coturn` / `cloudflare` | `none` |
| `voice_turn_coturn_urls` | coturn の TURN の URL の一覧（`turn:` / `turns:`） | なし |
| `voice_turn_coturn_secret` | coturn の `static-auth-secret`（秘密） | なし |
| `voice_turn_cloudflare_key_id` / `voice_turn_cloudflare_api_token` | Cloudflare の TURN キーの ID と API トークン（トークンは秘密） | なし |
| `voice_turn_credential_ttl` | TURN の資格情報の有効期間（`cloudflare` では 48 時間まで） | `6h` |
| `voice_ice_transport_policy` | `all` / `relay`（TURN 経由だけ） | `all` |

- `voice = off` のとき、通話の API は `404` を返し、Web は通話の画面と入口を出さない（ADR-0100 §2）。起動時に他の `voice_*` は検査しない。
- 起動時の検査（失敗したら起動しない）: `voice_ice_transport_policy = relay` で `voice_turn = none` は誤り（TURN がないと繋がらない）。`voice_turn = coturn` で URL か秘密がない、`cloudflare` でキーかトークンがない、または `voice_turn_credential_ttl` が 48 時間を超えるのも誤り。
  `voice = p2p` で STUN も TURN もないときは警告をログに出す（同じ LAN の中でしか繋がらない）。
- SFU（`livekit` / `sora`）のときの `voice_stun_urls`・`voice_turn*`・`voice_ice_transport_policy` の扱いは §12、SFU の接続先の設定は §14。
- 上限は §1 の設定で、ADR-0100 §5・§6 が評価する（参加の `409`、`voice.sweep` の `time_limit`、開く操作の `409`）。

### 2. 方式のポート

`internal/app/voice` に置く。型は標準ライブラリだけで、SDK の型を出さない。adapter は `internal/adapter/voice/<name>`（`p2p`、§10 の `livekit`・`sora`、テスト用の `fake`）。契約は `testkit.VoiceTransportContract`。
形は spike #1347 のレポート（[ポートの形の案](https://github.com/mycroblog/my/issues/1347#issuecomment-6406319607)）の差を吸収するように決める。

```go
// Transport carries the audio of the rooms (ADR-0101 §2). The room domain is the source
// of truth; a transport enforces what it is told and reports what it saw.
type Transport interface {
    Capabilities() Capabilities
    // Open prepares the media path of a room. Idempotent.
    Open(ctx context.Context, room RoomRef) error
    // Grant returns what the client of a participant session needs to connect.
    // canSpeak is whether the session may publish audio now.
    Grant(ctx context.Context, room RoomRef, session Session, canSpeak bool) (Grant, error)
    // SetSpeaking allows or stops the audio of a session. Reconnect reports that the
    // client must connect again with a new grant for the change to take effect.
    SetSpeaking(ctx context.Context, room RoomRef, session Session, canSpeak bool) (reconnect bool, err error)
    // Mute stops the audio of a session; Unmute lifts it when the participant turns the
    // microphone on again.
    Mute(ctx context.Context, room RoomRef, session Session) error
    Unmute(ctx context.Context, room RoomRef, session Session) error
    // Remove disconnects a session from the media path.
    Remove(ctx context.Context, room RoomRef, session Session) error
    // Close ends the media path of a room. Idempotent.
    Close(ctx context.Context, room RoomRef) error
    // Sessions returns the sessions the media path sees now, for reconciliation.
    Sessions(ctx context.Context, room RoomRef) ([]SessionRef, error)
}

type Capabilities struct {
    ServerEnforced  bool // false for p2p: speaking is enforced by the relay and the peers (§4)
    ConnectCallback bool // the media server asks before every connect (ConnectAuthorizer)
    Events          bool // the media server sends events (EventSource)
}

type Grant struct {
    Kind      string            // "p2p", "livekit", "sora"
    URL       string            // the SFU endpoint; empty for p2p
    Token     string            // the SFU access token; empty for p2p
    Options   map[string]string // method-specific options for the client (§12)
    ICE       []ICEServer       // STUN / TURN servers with short-lived credentials
    RelayOnly bool              // iceTransportPolicy = "relay"
    ExpiresAt time.Time         // the client asks for a new grant before this
}

// ConnectAuthorizer is implemented by a transport whose media server asks before every
// connect; decide applies the room domain (bans, roles, one room per account).
type ConnectAuthorizer interface {
    AuthorizeConnect(ctx context.Context, header http.Header, body []byte, decide func(ConnectAttempt) ConnectDecision) (status int, response []byte, err error)
}

// EventSource is implemented by a transport whose media server sends events.
type EventSource interface {
    // ParseEvents verifies and decodes one delivery of events.
    ParseEvents(ctx context.Context, header http.Header, body []byte) ([]Event, error)
}
```

- **セッション**: 参加（ADR-0100 の参加の行）ごとに、端末が参加するたびに新しい不透明なセッションの ID を作る。同じアカウントが別の端末で参加すると、セッションが替わり、前のセッションのストリームは `replaced` を受けて閉じ、`Remove` で前の接続を切る（ADR-0100 §5）。
  セッションの ID は `KeyValueStore` のハートビートの値のキーで、DB には書かない。SFU の利用者の識別子もセッションの ID である（§12）。
- **呼び出しの時機**: ポートの呼び出しは外部の I/O になりうるので、トランザクションの中では呼ばない。ルームの変更をコミットしたら、outbox のジョブ `voice.apply`（冪等。キーは（ルーム, 変更のイベント））が `Open`・`SetSpeaking`・`Mute`・`Remove`・`Close` を呼ぶ（[ADR-0004](0004-job-queue-abstraction.md)）。
  `Grant` は参加と更新の要求の中で、`Unmute` はマイクの操作（§6）の中で、トランザクションの外で呼ぶ。
- **再接続**: `SetSpeaking` が `reconnect` を返したら（接続の途中で役割を変えられない方式）、`voice.apply` はそのセッションのストリームに新しい `Grant` を `grant` のイベント（`reconnect: true`）で送り、クライアントは入り直す。
- **照合**: `Sessions` は ADR-0100 §6 の `voice.sweep` が開いたルームごとに呼び、ルームのドメインにいてメディアの経路にいないセッションを退出させ（ハートビートが生きていても）、メディアの経路にいてドメインで退出・禁止のセッションを `Remove` する。
  イベント（`EventSource`）は照合を早めるだけで、届くことを前提にしない。受け口とイベントの扱いは §13。
- **P2P の adapter**: `Capabilities` はすべて偽。`Grant` は §5 の ICE の設定だけを返す。`Open`・`SetSpeaking`（`reconnect` は偽）・`Mute`・`Unmute`・`Remove`・`Close` はメディアを持たないので何もしない。`Sessions` はハートビートの値を返す。
  P2P の強制は §4 のシグナリングの中継と §3 のストリームのイベントが行う。
- 上のコードは決定の形で、コメントと任意のインターフェース（重複接続の `SessionDeduplicator`。§12）を含む全体は `internal/app/voice/transport.go` にある。
- adapter の失敗は `voice.apply` が再試行する。再試行を尽くしたら dead job に残し、ルームの状態（DB）は変えない（ルームのドメインが正で、メディアの経路が後から従い、`voice.sweep` の照合が残りを直す）。

### 3. ルームのストリーム

- `GET /voice/rooms/{id}/stream?session=…`: Server-Sent Events。いまの `GET /streaming` と同じく手で書き（[ADR-0016](0016-client-api-style.md)）、`fetch` のストリームで `Authorization: Bearer` を送る。スコープは `read:voice`。
  開けるのは、そのセッションが参加中で生きている（ハートビートの値がある）ときだけ。それ以外は `404`。1 セッションに 1 本で、同じセッションで開き直すと前の 1 本を閉じる。
- イベント:

  | `event` | `data` | 宛先 |
  | --- | --- | --- |
  | `state` | ルームと参加者の一覧（`VoiceRoom`。ADR-0100 §14 の `GET` と同じ形） | 全員 |
  | `mic` | `{participant, on}` | 全員 |
  | `reaction` | `{participant, emoji}` | 全員（送った人とブロックの関係にある人を除く。ADR-0100 §10） |
  | `signal` | `{from, kind, sdp?, candidate?}`（§4） | 宛先のセッション |
  | `muted` | `{by}` | ミュートされたセッション |
  | `grant` | `{grant, reconnect}`（話せるようになって方式が入り直しを要するときなど。§2） | そのセッション |
  | `removed` / `replaced` / `ended` | `{reason}`。送った後に閉じる | そのセッション / 全員 |

- 開くと `: connected` と最初の `state` を送り、その後 15 秒ごとに `: heartbeat` のコメント行を送る。ハートビートのたびにアクセストークンを照合し直す（いまのストリームと同じ）。
- **プロセスをまたぐ**: いまの `stream.Hub` にルームの購読（キーはルームの ID）を足し、`stream.Relay` の `Bus` で運ぶ。知らせは 1 KiB 以下の JSON:
  - `{"v":1,"o":…,"k":"voice_room","r":"<room>"}`: 受けたプロセスはルームの状態をユースケースで読み直し、自分のプロセスの購読者に `state` を送る（閲覧者ごとのブロックの印は ADR-0100 §10 のとおり読み直しで付ける）
  - `{"k":"voice_signal","r":…,"m":"<message id>"}`: §4 の本文を `KeyValueStore` から読み、宛先のセッションを持つプロセスだけが送る
  - `{"k":"voice_mic","r":…,"p":"<participant>","on":true}`、`{"k":"voice_reaction","r":…,"p":…,"e":"<emoji>"}`、`{"k":"voice_mute","r":…,"s":"<session>","b":"<by>"}`
  - 通話の知らせは、内容を載せない ADR-0050 §1 の規則の例外として、マイクの状態・リアクションの絵文字（Unicode か `:shortcode:`、64 バイトまで）・ミュートした人を載せる。どれも DB にない一時の事実で、読み直す先がないからである。
    受けたプロセスは送る前に、宛先がそのルームの参加者であることと、ブロックの関係を確かめる
- `state` は 1 秒に 1 回までにまとめる（同じルームの `voice_room` の知らせが続いても、最後に読み直した状態を 1 回送る）。
- **再接続**: 切れたら、クライアントは同じセッションで開き直し、最初の `state` で追いつく。`Last-Event-ID` での再開はしない。ハートビートの値（45 秒、ADR-0100 §6）が切れる前に戻れば、参加は続く。
  Lambda の 15 分ごとの切断と Cloud Run のリクエストの時間切れは、この再接続で吸収する。
- 通話の間は 1 人につき SSE が 2 本になる（Lambda では実行環境を 2 つ占める。§8）。

### 4. P2P のシグナリング

- **結ぶ組**: 少なくとも一方が話者（ホスト・共同ホストで話している人を含む）である 2 つのセッションを結ぶ。聞き手どうしは結ばない。聞き手が話者になったら、まだ結んでいないすべての参加者と結び、話者が聞き手に戻ったら、聞き手どうしになった組を切る。
  人数の上限 `voice_p2p_max_participants`（既定 10）で、組の数は最大 45 に収まる。
- **offer する側**: 結ぶ組のうち、セッションの ID が大きい側が最初の offer を送る。再ネゴシエーションは WebRTC の perfect negotiation の手順に従い、ID が小さい側を polite とする。
- **送り方**: `POST /voice/rooms/{id}/signals`（`write:voice`）に `{session, to, kind, sdp?, candidate?}`（`kind` は `offer` / `answer` / `candidate` / `restart`）。サーバーは次を検査し、満たさなければ `422` で中継しない:
  - 送り手と宛先が同じルームの生きたセッションで、§4 の結ぶ組である
  - 本文は 16 KiB まで。セッションごとに 1 秒に 20 件まで（`KeyValueStore` のレート制限、`429`）
  - SDP の `m=` 行は `audio` だけ（`video`・`application` を拒む。音声のみ、データチャネルも使わない）
  - 送り手が話せない（ADR-0100 §4 の `CanSpeakFreely` が偽で、話者でない）とき、`m=audio` の向きは `recvonly` か `inactive` だけ。`sendrecv` / `sendonly` を含む offer と answer を拒む
- **運び方**: 検査を通った本文を `KeyValueStore` に 30 秒の期限で置き、`voice_signal` の知らせで ID を流す。宛先のセッションのストリームを持つプロセスが読み、`signal` として送る。届かなかった本文は期限で消える（クライアントは ICE の再起動でやり直す）。
- **受け手の側の強制**: クライアントは、最新の `state` で話者でない参加者から届く音声を再生しない。発話を取り消されたクライアントは送信のトラックを外して再ネゴシエーションする。
  サーバーの SDP の検査と受け手の側の再生の制御を合わせても、**改造したクライアントどうしが結託すれば防げない**（例: 改造した話者でない人と、改造した受け手）。これを運営者の文書に書き、厳密に強制したい運営者には SFU を勧める。
- `Mute`（ミュートさせる）は `muted` のイベントで本人のクライアントにマイクを切らせる。本人は再びオンにできる（ADR-0100 §5）。

### 5. STUN / TURN

- `Grant` の `ICE` は、`voice_stun_urls` と、`voice_turn` が発行する短命の資格情報の TURN を返す。資格情報の有効期間は `voice_turn_credential_ttl`（既定 6 時間。`cloudflare` では 48 時間まで）。
- **取り直し**: クライアントは `ExpiresAt` の 10 分前に `POST /voice/rooms/{id}/grant` で取り直し、各 `RTCPeerConnection` に `setConfiguration` で新しい ICE の設定を入れて `restartIce()` する。
  ICE の再起動で新しい割り当てが新しい資格情報で作られる。期限が切れたときの振る舞いが TURN の事業者で違う（下の 2 つ）ので、どちらでも同じこの手順にする。
- **coturn**（`use-auth-secret`）: ユーザー名は `<期限の Unix 秒>:<セッションの ID>`、パスワードは `base64(HMAC-SHA1(static-auth-secret, ユーザー名))`。サーバーの中で計算し、外部の I/O はない。
  ハッシュ関数は SHA-1 に決まっていて、変える設定はない（確認日 2026-10-26: [README.turnserver](https://github.com/coturn/coturn/blob/master/README.turnserver) の TURN REST API の節 `hmac-sha1`。
  ソース `src/apps/relay/userdb.c`（コミット `596c48b`）も、REST の資格情報では SHA-1 の長さの MESSAGE-INTEGRITY だけを受ける）。
  期限は割り当ての最初の認証で確かめ、通ったらセッションの鍵を覚えて以後の要求（Refresh を含む）に使うので、**期限が切れても確立済みの割り当ては続く**。同じ割り当てでユーザー名は替えられない（441）ので、新しい資格情報は新しい割り当てでだけ使える
  （同日、`src/server/ns_turn_server.c` の `check_stun_auth` の `hmackey_set`）。
- **Cloudflare TURN**: `POST https://rtc.live.cloudflare.com/v1/turn/keys/{key_id}/credentials/generate-ice-servers`（`Authorization: Bearer <API トークン>`、本文 `{"ttl": 秒}`）が `iceServers` を返す。取り消しは `.../credentials/{username}/revoke`
  （確認日 2026-10-26: [Generate credentials](https://developers.cloudflare.com/realtime/turn/generate-credentials/)）。
  **期限が切れると、少しの後に接続を切る**。期限は 48 時間先まで。長い通話では `RTCPeerConnection` の設定で資格情報を更新する（同日、[TURN FAQ](https://developers.cloudflare.com/realtime/turn/faq/)）。
  料金は SFU と使わないとき、クライアントへの送信 1 GB あたり $0.05 で、課金の前に 1,000 GB の無料枠がある（同日、[TURN service](https://developers.cloudflare.com/realtime/turn/)・TURN FAQ）。
  この呼び出しは運営者が設定した事業者の API なので、`internal/adapter/voice/turncloudflare` に置き、SSRF の防御の `kernel.HTTPClient` ではなく専用の HTTP クライアントを使う（AGENTS.md の規則）。`Grant` は参加の要求の中で呼び、失敗したら `503` を返す。
  同じセッションの資格情報は期限まで `KeyValueStore` に置いて使い回す。退出・退出させる操作で取り消す（`voice.apply`）。取り消しが確立済みの接続を切るかは文書にないので、切断は §4 のシグナリングから外す操作に頼り、取り消しは資格情報を再使用させないためだけに行う。
- **IP を隠す（`relay`）**: `voice_ice_transport_policy = relay` のとき、`Grant.RelayOnly` を真にし、クライアントは `iceTransportPolicy: "relay"` で接続する。そのクライアント自身は TURN の候補しか出さないので、相手にはそのクライアントの IP が見えない。
  守られるのは設定に従うクライアントの IP で、改造したクライアントが自分の IP を出すことは防がない（それはそのクライアントの利用者の選択である）。
- TURN の運営の注意（運営者の文書）: coturn では内部のネットワークへの中継を `denied-peer-ip` で拒み、`external-ip` を設定する。TURN の転送量は事業者の料金になる。

### 6. 参加・ハートビート・マイクの API

ADR-0100 §14 の形に、接続のための次の操作を加える（細部は [client-api.md](../architecture/client-api.md) の手順で決める）。

- `POST /voice/rooms/{id}/participation` は `{session, grant}` を返す。クライアントはそのセッションでストリームを開く。
- `POST /voice/rooms/{id}/heartbeat`（`{session}`、15 秒ごと）: ADR-0100 §6 のハートビート。応答は `204`。ストリームを開いているだけではハートビートにしない（中継の装置がバッファしてストリームの書き込みが失敗しないことがあり、端末が生きていることを確かめられないため）。
- `POST /voice/rooms/{id}/mic`（`{session, on}`）: マイクのオン・オフ。DB に書かず、`voice_mic` の知らせで配る。ミュートさせられた後にオンにするときは、`Transport.Unmute` を呼んでから配る（§2）。
- `POST /voice/rooms/{id}/grant`（`{session}`）: 資格情報の取り直し（§5）。

### 7. Web の実装の置き場所と、ブラウザの前提

- `app/web/src/features/voice/` に置く（[ADR-0025](0025-web-client-stack.md)）。WebRTC の接続を管理するコード（`RTCPeerConnection`・perfect negotiation・受け手の側の再生の制御・方式ごとの差）は React を使わない TypeScript として `features/voice/rtc/` に置き、
  `features/voice/hooks` のフックだけがそれを使う。Oxlint の依存の境界の設定（`app/.oxlintrc.json`）に `rtc` の要素を足し、`rtc` は `@my/ui`・`api`・`components` を import しない。
  SFU の方式のブラウザの SDK は §15。P2P はブラウザの WebRTC の API だけを使い、依存を足さない。
- マイクは `getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })`。映像は求めない。HTTPS（secure context）が要る。
- 参加の操作（ボタンを押す）を利用者の操作として、その中で音声の再生を始める（ブラウザの自動再生の制限）。
- 対応するブラウザは Web クライアントの対応範囲のまま。iOS のホーム画面の Web アプリ（PWA）を背面にしたとき音声の再生が止まる不具合は、iOS 15.4 で直っている
  （確認日 2026-10-26: [WebKit bug 198277](https://bugs.webkit.org/show_bug.cgi?id=198277) は [232909](https://bugs.webkit.org/show_bug.cgi?id=232909) の重複として閉じられ、修正は iOS 15.4 に入ったとコメントされている）。
  背面にしたときにマイクの取り込みが続くかは公開の文書で確かめられない（Apple の Developer Forums には、画面のロックか背面で WebRTC が止まるという開発者の報告がある: [thread 774239](https://developer.apple.com/forums/thread/774239)）。
  利用者の文書には「iPhone と iPad では、通話中にアプリを背面にすると話せなくなることがある」と書く。

### 8. 構成ごとの注意

| 構成 | 注意 |
| --- | --- |
| 1 プロセス（SQLite、`stream = hub`） | すべてプロセスの中で動く。`KeyValueStore` は `db` |
| 複数プロセス | `stream` を `postgres` / `valkey` / `sns` / `pubsub` にする（いまのストリームと同じ）。`stream = hub` で `role` が `all` 以外のときは、起動時の警告に通話を含める |
| Lambda | SSE の 1 本が実行環境 1 つを最大 15 分占める。通話の間は 1 人 2 本（§3）で、実行時間は参加者の数 × 時間で増える（[デプロイターゲット](../architecture/deployment-targets.md#ストリームプロセス間の知らせ)の見積もりの 2 倍） |
| Cloud Run | ストリームはリクエストで、開いている間 CPU が割り当てられる。リクエストの時間切れで切れたら §3 の再接続 |

- P2P では、音声はこのサーバーを通らない（TURN を使うときは TURN のサーバーを通る）。サーバーの負荷はシグナリング・ハートビート・ストリームだけである。

### 9. 運営者の文書と計測

- `docs/operations/` に通話の文書を置く: 方式の選び方、P2P の人数と回線の目安、STUN / TURN の設定（coturn・Cloudflare）、`relay` の意味、§4 の強制の限界、構成ごとの注意（§8）。`tools/agentkit/context-map.yaml` の `voice` の項目の `userdocs` にする。
- 計測（[ADR-0055](0055-service-observability.md)）: 開いたルームの数・参加者の数（Gauge）、中継したシグナリングと拒んだシグナリング（理由ごと）・TURN の資格情報の発行と失敗（Counter）。名前は ADR-0055 の規則で実装のときに決め、`agentkit docs inventory` の文書に載せる。

### 10. SFU: 置き場所とビルド

- adapter は `internal/adapter/voice/livekit` と `internal/adapter/voice/sora`。Webhook の受け口は新しい `internal/interface/voice`（client API の OpenAPI に載せない。事業者が呼ぶ経路で、利用者は呼ばない。[ADR-0074](0074-payment-provider-port.md) の `internal/interface/payments` と同じ扱い）。
- ビルドタグ `no_voice_livekit`・`no_voice_sora` で外せる。外したビルドで `voice` にその値を設定すると、起動時に `config.ExcludedError` で止まる（`no_payments_stripe` と同じ形）。
- HTTP のクライアントは adapter が持つ専用のもの（タイムアウト 5 秒）。SFU は運営者が設定した宛先で、セルフホストの SFU はプライベートの IP にありうるので、SSRF の防御の `kernel.HTTPClient` は通さない（AGENTS.md の「運営者が設定した事業者の API」の規則）。`http.DefaultClient` は使わない。

### 11. SFU: サーバー側の呼び出し

- SDK を使わず、`net/http` と自前の HS256 の JWT で、LiveKit の Twirp（HTTP + JSON）と Sora の HTTP API を呼ぶ。JWT は HS256 だけで、`crypto/hmac` と `encoding/json` で書く（数十行）。`server-sdk-go` は `CGO_ENABLED=0` でビルドできるが、RoomService のクライアントだけで 311 のパッケージ（pion/webrtc、moby/buildkit、prometheus など）を引き、バイナリが約 22 MB になる（spike の手元の計測）ので使わない。
- LiveKit の Twirp は `POST <api_url>/twirp/livekit.RoomService/<Method>`、`Authorization: Bearer <JWT>`、JSON。Sora は `POST <api_url>/`（`x-sora-target: Sora_20xxxxxx.<Method>`）、Sora Cloud は `POST https://api.sora-cloud.shiguredo.app/sora-api` に `Authorization: Bearer <API キー>`。
- 契約テスト `testkit.VoiceTransportContract` を、LiveKit は `livekit-server --dev`（CI のコンテナ）と `httptest` の fake の両方で、Sora は `httptest` の fake だけで回す。本物での確認は利用する運営者に任せる。fake は spike のレポートの要求と応答の形に沿う。

### 12. SFU: ポートの各操作

| 操作（§2） | LiveKit | Sora |
| --- | --- | --- |
| `Capabilities` | `ServerEnforced`・`Events` | `ServerEnforced`・`ConnectCallback`・`Events` |
| `Open` | `CreateRoom`（名前はルームの ID、`max_participants` は `voice_sfu_max_participants`、`empty_timeout`・`departure_timeout` は短く）。冪等 | 何もしない（最初の接続でセッションができる） |
| `Grant` | アクセストークン（JWT、寿命 60 秒。`roomJoin`・`room`・`canSubscribe: true`・`canPublishData: false`。話者は `canPublish: true` と `canPublishSources: ["microphone"]`、聞き手は `canPublish: false`）と、クライアント向けの URL | my が署名した寿命 60 秒の参加の値（ルーム・セッション・役割）を `metadata` に入れさせる。Sora Cloud は加えて Cloud の `access_token`（JWT、HS256、`channel_id`・`role`・`exp`）。`Options` に `role`（`sendrecv` / `recvonly`）と `channel_id` |
| `SetSpeaking`（許可） | `UpdateParticipant` の `permission`。再接続なし（`reconnect` は偽） | 役割を変えられないので `reconnect` を真で返す。クライアントは `sendrecv` の新しい `Grant` で入り直す（聞き手は受信のみで入り、聞くだけの人にマイクの許可を求めず上りの帯域も使わない） |
| `SetSpeaking`（取り消し） | `UpdateParticipant` で `canPublish: false`（公開中のトラックは自動で非公開になる） | 転送フィルター（そのコネクションの `audio` を `block`）ですぐ止め、`reconnect` を真で返して `recvonly` で入り直させる |
| `Mute` | `GetParticipant` でトラックを引き、`MutePublishedTrack(muted: true)` | 転送フィルターでそのコネクションの音声を `block` |
| `Unmute` | 何もしない（本人がクライアントでオンにする。サーバーのミュートの後も、本人のミュートの解除の要求はそのまま通る） | 転送フィルターを消す |
| `Remove` | `RemoveParticipant` | `DisconnectConnection` |
| `Close` | `DeleteRoom` | `TerminateSession` |
| `Sessions` | `ListParticipants` | `ListChannelConnections` |

- **Sora の転送フィルター**: 1 人の送り手の音声をチャネルの全員に対して止めるのは、チャネル単位のフィルター（`CreateChannelForwardingFilter`、`action: block`）の 1 つの規則に `{"field": "connection_id", "operator": "is_in", "values": [送り手]}` と `{"field": "kind", "operator": "is_in", "values": ["audio"]}` を AND で並べる。
  `connection_id` は送り手のコネクションを指す（確認日 2026-10-26: [転送フィルター](https://sora-doc.shiguredo.jp/FORWARDING_FILTER.html)）。`allow` は `block` より優先されるので、my は `allow` の規則を作らない。
- **LiveKit のミュートの解除**: `MutePublishedTrack(muted: true)` はサーバーがトラックを止めて本人に知らせるだけで、本人のクライアントからのミュートの解除の要求は制限なく通る（確認日 2026-10-26: `livekit/livekit` のコミット `385aa40` の `pkg/rtc/participant.go` の `SetTrackMuted` と `pkg/rtc/signalling/signalhandler.go`）。
  サーバーからの解除（`muted: false`）は既定で無効（`enable_remote_unmute`）なので、my は使わない。
- **SFU の利用者の識別子**: §2 のセッションの ID を使う（LiveKit の `identity`、Sora の `client_id`）。アカウントの ID は SFU に渡さない。同じアカウントの別の端末は別のセッションで、my が前のセッションを `Remove` する（ADR-0100 §5）。
  禁止や退出の後に古いトークンで入ってきた接続は、識別子が生きたセッションでないので、§13 の照合で切れる。
- **Sora の入り直しと `client_id`**: 役割を変える入り直し（§12）は同じセッション、つまり同じ `client_id` で行う。同じ `client_id` の接続が重なったときの扱いは Sora の設定 `default_duplicate_client_id`（`allow` / `evict`、既定は `allow`）で決まり、
  `evict` なら新しい接続を受けて古い接続を切る（確認日 2026-10-26: [セッションウェブフック](https://sora-doc.shiguredo.jp/SESSION_WEBHOOK.html)、[sora.conf](https://sora-doc.shiguredo.jp/SORA_CONF.html)。文書の版 2026.1.3）。
  運営者の文書はセルフホストで `evict` を勧める。`allow` のとき（と、この設定を変えられるか文書で確かめられない Sora Cloud）は、my が同じ `client_id` の新しい接続の `connection.created` を受けたときと §13 の照合で、古い方の接続を `DisconnectConnection` で切る。
  新しい接続ができてから古い接続を切るので、途切れは新しい接続が音声を流し始めるまでの間になる。
- **音声だけ**: LiveKit はトークンの `canPublishSources: ["microphone"]`、Sora は認証 Webhook の応答の `video: false`・`audio: true`・`audio_codec_type: "OPUS"`。運営者の文書は、セルフホストの LiveKit の `room.enabled_codecs` を音声だけにし、`room.auto_create` を `false` にすることを勧める（my の外でルームを作らせない）。
- **SFU の TURN**: SFU のときの STUN / TURN は SFU が持つもの（LiveKit の組み込みの TURN、Sora の TURN）を使い、`Grant.ICE` は空、`voice_stun_urls`・`voice_turn*`・`voice_ice_transport_policy` は使わない（起動時に設定されていれば警告）。
  SFU の構成では利用者の IP は SFU にだけ見え、他の参加者には見えない。

### 13. SFU: 参加の認可・イベント・照合

- **Sora の認証 Webhook**（`ConnectAuthorizer`）: `POST /voice/webhooks/sora/auth`。Sora が接続のたびに呼び、my は参加の値の署名と期限、セッションが生きていること、ルームで禁止されていないこと、`role` が `sendrecv` なら話者であること（ADR-0100 §4）、人数の上限を確かめ、`allowed` と §12 の音声の設定と `client_id` を返す。
  拒むときは `{"allowed": false, "reason": …}`（100 バイトまで）。応答の既定のタイムアウトは 5 秒（Sora Cloud は 10 秒）なので、判定は投影と KV の読み出しだけにし、外部の I/O をしない。冷えた起動で間に合わないことがある（運営者の文書）。
- **イベント**（`EventSource`）: LiveKit は `POST /voice/webhooks/livekit`（`Authorization` の JWT を API シークレットで検証し、JWT の `sha256` と本文の SHA-256 を定数時間で比べる）。Sora は `POST /voice/webhooks/sora/events`。
  検証した参加・退出・ルームの終了は、イベントの `id` を冪等キーにした outbox のジョブ `voice.apply_media_event` に積む。ジョブは、参加が生きたセッションでなければ（禁止・退出の後・未知）`Remove` し、退出ならそのセッションのハートビートの値を消して ADR-0100 §6 の切断として扱う。受信の行は DB に残さない（イベントは照合を早めるだけ。§2）。
- **照合**: ADR-0100 §6 の `voice.sweep`（1 分ごと）が、開いたルームごとに `Sessions` を呼ぶ（§2）。開いたルームの数は運営者の上限で抑えられ、呼び出しは 1 ルーム 1 回。
- **Webhook が届かないとき**: LiveKit の Webhook は、送る前にキューで 30 秒を超えた知らせを捨て、送るときは失敗したら最大 4 回、1〜30 秒の間隔で再送する（確認日 2026-10-26: `livekit/protocol` のコミット `b20af9b` の `webhook/resource_url_notifier.go` の `MaxAge = 30s`、
  `go-retryablehttp` v0.7.8 の既定 `RetryMax = 4`・`RetryWaitMin = 1s`・`RetryWaitMax = 30s`）。my が冷えた起動で応えられないと知らせは失われうるので、照合が正である。
- **セルフホストの LiveKit の再入室**: `RemoveParticipant` はセルフホストではトークンを失効させない（失効の処理は LiveKit Cloud だけ）。接続中のクライアントはサーバーから 5 分ごとに新しいトークンを受け取り、その寿命は 10 分か元のトークンの残りの長い方である。
  退出させた人は、手元の最新のトークンで最大でおよそ 10 分、入り直せる（確認日 2026-10-26: `livekit/livekit` のコミット `385aa40` の `pkg/service/roommanager.go` の `RemoveParticipant`・`refreshToken`・`tokenRefreshInterval = 5m`・`tokenDefaultTTL = 10m`）。
  入ってきたら参加の Webhook（`participant_joined`）か 1 分ごとの照合で見つけ、すぐ切る。その間、数秒から 1 分ほど音が聞こえうることを運営者の文書に書き、厳密さが要るなら LiveKit Cloud を勧める（LiveKit Cloud の `RemoveParticipant` はトークンをすぐ失効させる）。

### 14. SFU: 設定と守り方

§1 の表に加えて次のキーを置く。秘密は `_FILE` でも渡せ、TOML には書けない。

| キー | 意味 |
| --- | --- |
| `voice_livekit_url` | クライアントが繋ぐ URL（`wss:`） |
| `voice_livekit_api_url` | my が呼ぶ API の URL（既定は `voice_livekit_url` の `https:`） |
| `voice_livekit_api_key` / `voice_livekit_api_secret` | API キーとシークレット（シークレットは秘密）。アクセストークンの署名と Webhook の検証に使う |
| `voice_sora_mode` | `self_hosted` / `cloud` |
| `voice_sora_signaling_urls` | クライアントが繋ぐシグナリングの URL の一覧（`wss:`） |
| `voice_sora_api_url` | `self_hosted`: my が呼ぶ API の URL（運営者の認証つきのリバースプロキシ） |
| `voice_sora_api_header_name` / `voice_sora_api_header_value` | `self_hosted`: API の呼び出しに付けるヘッダー（値は秘密。例 `Authorization: Basic …`） |
| `voice_sora_webhook_username` / `voice_sora_webhook_password` | `self_hosted`: Sora の Webhook の Basic 認証（パスワードは秘密）。必須 |
| `voice_sora_cloud_project_id` / `voice_sora_cloud_api_key` | `cloud`: プロジェクトの ID と API キー（秘密）。`access_token` の署名、API の Bearer、Webhook の署名（`sora-cloud-signature`、HMAC-SHA256）の検証に使う |
| `voice_sora_join_secret` | Sora の参加の値（§12）の署名の秘密。`voice = sora` で必須 |

- セルフホストの Sora の HTTP API には認証がなく、ループバックだけで受ける設定が勧められている。my はそのまま呼ばず、運営者が API の前に認証つきのリバースプロキシを置き、my はそのヘッダーを付ける。Webhook は HTTPS と Basic 認証を必須にし、どちらかが欠けた設定では起動しない。
  Lambda の Function URL と Cloud Run は受け側で mTLS を終端できないので、mTLS は使わない。
- Sora Cloud のチャネルの ID は `<ルームの ID>@<プロジェクトの ID>`。

### 15. SFU: ブラウザの SDK

- `livekit-client`（Apache-2.0）と `sora-js-sdk`（Apache-2.0）を `app/web` の依存に持つ。[ADR-0025](0025-web-client-stack.md) の技術スタックへの追加はこの ADR が決める。
- 大きさ（spike の手元の計測、`bun build --minify`、gzip -9）: `livekit-client` 約 586 KB（gzip 150 KB）、`sora-js-sdk` 約 49 KB（gzip 11 KB）。通話の画面に入り、`Grant.Kind` が分かってから、その方式の SDK だけを動的な `import()` で読み込む。
  他の画面のバンドルは増えない。
- SDK を使うコードは §7 の `features/voice/rtc/` に方式ごとのファイルで置き、P2P と同じ内部の境界（接続・話す・ミュート・入り直す・切る・参加者の音声の再生）に包む。

### 16. SFU: 運営者の文書

§9 の通話の文書に次も書く:

- 方式の選び方: 小さなインスタンスには P2P か LiveKit Cloud の無料のプラン（Build: $0、月 5,000 分・50 GB・同時 100 接続。確認日 2026-10-26、[LiveKit の料金](https://livekit.com/pricing)）。LiveKit と Sora はメディアを UDP で受ける常駐のプロセスなので、Cloud Run・Lambda ではなく別の VM で動かす（Cloud Run のサービスが受けるのは HTTP の要求だけ: 確認日 2026-10-26、[Cloud Run のコンテナの契約](https://docs.cloud.google.com/run/docs/container-contract)。Lambda は呼び出しごとに動き、待ち受けのソケットを持たない）
- LiveKit: セルフホストの設定（`auto_create: false`・`enabled_codecs`・Webhook の URL `/voice/webhooks/livekit`）、`livekit-server --dev` での試し方、セルフホストの再入室の制限（§13）
- Sora: リバースプロキシと Basic 認証の置き方、Webhook の URL（`/voice/webhooks/sora/auth`・`/voice/webhooks/sora/events`）、認証 Webhook のタイムアウトと冷えた起動、聞き手が話者になるときの途切れ（§12）。
  料金とライセンスの形（Sora Cloud は月の基本料 5,000 円から、無料の期間はない。パッケージは同時接続数と期間の前払い。製品のページは海外の法人・団体・個人には提供していないと書く）。
  **Sora の adapter は実際の Sora で確かめていない**（文書に沿った fake でだけ試している）

### この決定を見直す条件

- P2P の利用が上限の人数でも成り立たないと分かったとき（既定の人数を下げる）。
- `Bus` の adapter のいずれかで KV の読み直しが遅すぎて接続が間に合わないと分かったとき（本文を `Bus` の知らせに載せる案を通話の知らせに限って検討する）。
- WebSocket を Lambda で安く使える形が出たとき（ルームのストリームと POST をまとめる）。
- LiveKit の Twirp か Sora の HTTP API が、自前の呼び出しでは追いにくい頻度で変わると分かったとき（`server-sdk-go` か、生成したクライアントにする）。
- Sora が接続の途中で役割を変える API を出したとき（§12 の入り直しをやめる）。
- セルフホストの LiveKit がトークンを失効させる手段を持ったとき（§13 の制限を外す）。

## 経緯

M4.3（Voice Chat、epic #1345）のルームの振る舞いとデータモデルは ADR-0100 が決め、設定 `voice`、ルームのストリームとハートビートの経路、マイクとミュートの配り方、運営者の上限をこの ADR に任せた（FR-920・FR-921）。P2P ならほぼ $0 の構成（Cloud Run・Lambda の Function URL・1 プロセスの SQLite）で、WebSocket の基盤も常駐のメディアサーバーも要らないようにし、ルームのドメインが正で、方式を 1 つのポートに閉じ込めることを優先した。`Bus` の約束（ADR-0050: 1 KiB まで、内容を載せない）は崩さず、運営者が望めば相手に IP を見せない構成にできるようにした。

SFU は spike #1347（[レポート](https://github.com/mycroblog/my/issues/1347#issuecomment-6406319607)、確認日 2026-10-26）の後に決めた。この ADR の SFU の第三者の事実は、断りがなければそのレポートの出典に基づく。Sora の adapter は fake だけで確かめることにし、聞き手は受信のみで入って話者になるときに入り直すこと、セルフホストの LiveKit の再入室の間に音が聞こえうることを文書で示すことを受け入れた。サーバーレスで動くよう my から SFU への呼び出しは 1 回の HTTP で終わらせ、運営者が使わない adapter はビルドから外せるようにした。

## 採らなかった案

- `GET /streaming` に通話のイベントを混ぜる: 接続は 1 本で済むが、各接続が「いまどのルームにいるか」を知り、別のプロセスで受けた参加の要求に合わせて購読を替える必要があり、アカウントのストリームの上限（8）と通話の 1 本の規則も混ざる。
- シグナリングの本文を `Bus` の知らせに載せ、通話だけ上限を広げる: KV が要らないが、音声だけでも SDP は 1 KiB を超えることが多く、PostgreSQL の `NOTIFY` の上限（8,000 バイト未満）にも近づき、上限を広げると ADR-0050 のすべての adapter の前提が変わる。
- 参加者のすべての組を結ぶ（フルメッシュ）: 話者の切り替えが単純だが、10 人・話者 2 人で 45 組（話者を含む組だけなら 17 組）になり、聞き手どうしの接続は音声を運ばず、ICE と TURN の割り当てを無駄に使う（`relay` では TURN の料金にも効く）。
- LiveKit の `server-sdk-go`: 書く量は少なく版の追従も SDK に任せられるが、311 パッケージ・約 22 MB を引き、Sora は結局自前の呼び出しになる。
- SFU のプロトコルをブラウザで自前で書く: SDK に依存しないが、LiveKit と Sora のシグナリングのプロトコルを保つ費用が大きく、版の追従が要る。
- ブラウザの SDK を静的に読み込む: 単純だが、通話を使わない利用者にも 150 KB（gzip）を読ませる。

## Consequences（影響）

- 良い影響: P2P では新しい基盤なしで通話が動く。いまのストリームの仕組み（`Hub`・`Relay`・`Bus`）をそのまま使い、構成ごとの差がない。
- 良い影響: 方式のポートがルームのドメインと音声の経路を分ける。ルームの状態は DB が正で、メディアの経路が後から従う。
- 良い影響: `relay` で、設定に従う利用者の IP を相手に見せない構成にできる。SFU のときは利用者の IP が他の参加者に見えず、発話の制限を SFU が強制する（P2P の結託の限界がない）。
- 良い影響: どちらの SFU も 1 回の HTTP の呼び出しだけで動かせ、サーバーの依存は増えない。通話を使わない画面のバンドルは増えない。
- 悪い影響: P2P の発話の制限は、結託した改造クライアントを防げない（§4）。文書で示し、厳密さが要る運営者には SFU を勧める。
- 悪い影響: 通話の間は SSE が 1 人 2 本になり、Lambda の実行時間と接続の数が増える。
- 悪い影響: シグナリングの本文を KV に置くので、`KeyValueStore = db` では接続のたびに数十行の書き込みが起きる（短い期限で消える）。
- 悪い影響: SFU では `voice.apply` のジョブを通るので、発話の取り消し・退出させる操作がメディアの経路に効くまでキューの遅れが乗る。
- 悪い影響: Sora では聞き手が話者になるとき、話者が聞き手に戻るときに入り直しで、その人の音が新しい接続の確立まで途切れる（長さは実機でしか測れず、Sora は fake でだけ試すので測らない）。
- 悪い影響: セルフホストの LiveKit では、退出させた人が切られるまで数秒から 1 分ほど聞こえうる。
- 悪い影響: Sora の adapter は実機で確かめず、Sora の API の変化に気づくのが運営者の報告になる。セルフホストの Sora は、運営者がリバースプロキシを置く手間を負う。
- 悪い影響: Sora の認証 Webhook は同期で、my の冷えた起動が参加の失敗になりうる。

## Open Questions / 要検証

- iOS のホーム画面の Web アプリを背面にしたとき、マイクの取り込み（WebRTC の送信）が続くか（要検証: 実機。公開の文書では確かめられない）
- Sora Cloud で `default_duplicate_client_id` を `evict` にできるか（要検証: Sora Cloud の文書に記載がない。できなくても §12 の切り直しで動く）
