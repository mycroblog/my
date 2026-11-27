# ADR-0101: 通話の接続と SFU: 方式のポート・P2P のシグナリング・STUN / TURN・my の SFU

- Status: Accepted
- Date: 2026-10-26
- Deciders: @rosylilly
- Issue: #1349, #1350, #2863, #3559, #3569

## Decision（決定）

通話の方式はインスタンスで 1 つ（`voice = off / p2p / sfu`）で、全ルームが同じ方式を使う。§1〜§9 が方式に共通の部分と P2P、§10〜§16 が my に組み込んだ音声だけの SFU を決める。

### 1. 設定

デプロイの設定（[設定のリファレンス](../reference/configuration.md)、環境変数 `MY_*`。秘密は `_FILE` でも渡せ、TOML には書けない）に置く。

| キー | 値 | 既定 |
| --- | --- | --- |
| `voice` | `off` / `p2p` / `sfu` | `off` |
| `voice_max_participants` | ルームの人数の上限の最大（2〜500。`p2p` では 50 まで）。ルームの上限（ADR-0100 §3）はこれを超えない | `10` |
| `voice_max_room_duration` | 1 ルームの最長時間（0 は上限なし） | `4h` |
| `voice_max_open_rooms` | 同時に開けるルームの数（0 は上限なし） | `10` |
| `voice_stun_urls` | STUN の URL の一覧（`stun:` / `stuns:`） | なし |
| `voice_turn` | `none` / `coturn` / `cloudflare` | `none` |
| `voice_turn_coturn_urls` | coturn の TURN の URL の一覧（`turn:` / `turns:`） | なし |
| `voice_turn_coturn_secret` | coturn の `static-auth-secret`（秘密） | なし |
| `voice_turn_cloudflare_key_id` / `voice_turn_cloudflare_api_token` | Cloudflare の TURN キーの ID と API トークン（トークンは秘密） | なし |
| `voice_turn_credential_ttl` | TURN の資格情報の有効期間（`cloudflare` では 48 時間まで） | `6h` |
| `voice_ice_transport_policy` | `all` / `relay`（TURN 経由だけ。`p2p` だけ） | `all` |
| `voice_sfu_udp_port` / `voice_sfu_tcp_port` | SFU のノードが媒体を受ける UDP（mux）と ICE-TCP のポート（§15） | `7882` / `7881` |
| `voice_sfu_public_ip` | SFU のノードの公開の IP（NAT の内側のとき） | なし（ノードの IP） |
| `voice_sfu_public_url` | クライアントがつなぐノードの `wss:` の URL（§11） | サーバーの公開の URL から |
| `voice_sfu_node_capacity` | ノード 1 台の容量（参加者の数。§14） | `500` |

- `voice = off` のとき、通話の API は `404` を返し、Web は通話の画面と入口を出さない（ADR-0100 §2）。起動時に他の `voice_*` は検査しない。
- 起動時の検査（失敗したら起動しない）: `voice_ice_transport_policy = relay` で `voice_turn = none` は誤り（TURN がないと繋がらない）。`voice_turn = coturn` で URL か秘密がない、`cloudflare` でキーかトークンがない、または `voice_turn_credential_ttl` が 48 時間を超えるのも誤り。
  `voice = p2p` で STUN も TURN もないときは警告をログに出す（同じ LAN の中でしか繋がらない）。
- `voice = sfu` で `voice_ice_transport_policy = relay` は誤り、`voice = p2p` で `voice_max_participants` が 50 を超えるのも誤り（§15）。`role = sfu` で `voice` が `sfu` でないのも誤り（§10）。
- 上限は §1 の設定で、ADR-0100 §5・§6 が評価する（参加の `409`、`voice.sweep` の `time_limit`、開く操作の `409`）。

### 2. 方式のポート

`internal/app/voice` に置く。型は標準ライブラリだけで、SDK の型を出さない。adapter は `internal/adapter/voice/<name>`（`p2p`、§13 の `sfu`、テスト用の `fake`）。契約は `testkit.VoiceTransportContract`。
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
    // SetSpeaking allows or stops the audio of a session.
    SetSpeaking(ctx context.Context, room RoomRef, session Session, canSpeak bool) error
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
    ServerEnforced bool // false for p2p: speaking is enforced by the relay and the peers (§4)
}

type Grant struct {
    Kind      string      // "p2p", "sfu"
    URL       string      // the wss: URL of the node that holds the room (§11); empty for p2p
    Token     string      // the single-use ticket for that node (§11); empty for p2p
    ICE       []ICEServer // STUN / TURN servers with short-lived credentials
    RelayOnly bool              // iceTransportPolicy = "relay"
    ExpiresAt time.Time         // the client asks for a new grant before this
}
```

- **セッション**: 参加（ADR-0100 の参加の行）ごとに、端末が参加するたびに新しい不透明なセッションの ID を作る。同じアカウントが別の端末で参加すると、セッションが替わり、前のセッションのストリームは `replaced` を受けて閉じ、`Remove` で前の接続を切る（ADR-0100 §5）。
  セッションの ID は `KeyValueStore` のハートビートの値のキーで、DB には書かない。SFU の利用者の識別子もセッションの ID である（§12）。
- **呼び出しの時機**: ポートの呼び出しは外部の I/O になりうるので、トランザクションの中では呼ばない。ルームの変更をコミットしたら、outbox のジョブ `voice.apply`（冪等。キーは（ルーム, 変更のイベント））が `Open`・`SetSpeaking`・`Mute`・`Remove`・`Close` を呼ぶ（[ADR-0004](0004-job-queue-abstraction.md)）。
  `Grant` は参加と更新の要求の中で、`Unmute` はマイクの操作（§6）の中で、トランザクションの外で呼ぶ。
- **照合**: `Sessions` は ADR-0100 §6 の `voice.sweep` が開いたルームごとに呼び、ルームのドメインにいてメディアの経路にいないセッションを退出させ（ハートビートが生きていても）、メディアの経路にいてドメインで退出・禁止のセッションを `Remove` する。
- **P2P の adapter**: `Capabilities` はすべて偽。`Grant` は §5 の ICE の設定だけを返す。`Open`・`SetSpeaking`・`Mute`・`Unmute`・`Remove`・`Close` はメディアを持たないので何もしない。`Sessions` はハートビートの値を返す。
  P2P の強制は §4 のシグナリングの中継と §3 のストリームのイベントが行う。
- **SFU の adapter** は §13。
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
  Cloud Run のリクエストの時間切れは、この再接続で吸収する。
- P2P の通話の間は 1 人につき SSE が 2 本になる。SFU では SSE と、ノードとの WebSocket（§11）が 1 本ずつになる。

### 4. P2P のシグナリング

- **結ぶ組**: 少なくとも一方が話者（ホスト・共同ホストで話している人を含む）である 2 つのセッションを結ぶ。聞き手どうしは結ばない。聞き手が話者になったら、まだ結んでいないすべての参加者と結び、話者が聞き手に戻ったら、聞き手どうしになった組を切る。
  人数の上限（§1 の `voice_max_participants`。既定 10）で、組の数は最大 45 に収まる。P2P ではこの上限を 50 までにする。
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

ADR-0100 §14 の形に、接続のための次の操作を加える（細部は [ADR-0016](0016-client-api-style.md) の規約で決める）。

- `POST /voice/rooms/{id}/participation` は `{session, grant}` を返す。クライアントはそのセッションでストリームを開く。
- `POST /voice/rooms/{id}/heartbeat`（`{session}`、15 秒ごと）: ADR-0100 §6 のハートビート。応答は `204`。ストリームを開いているだけではハートビートにしない（中継の装置がバッファしてストリームの書き込みが失敗しないことがあり、端末が生きていることを確かめられないため）。
- `POST /voice/rooms/{id}/mic`（`{session, on}`）: マイクのオン・オフ。DB に書かず、`voice_mic` の知らせで配る。ミュートさせられた後にオンにするときは、`Transport.Unmute` を呼んでから配る（§2）。
- `POST /voice/rooms/{id}/grant`（`{session}`）: 資格情報の取り直し（§5）。SFU では、つなぎ直し・移動・ノードの喪失（§11・§14）のときの新しいノードの URL とチケットも、これで得る。

### 7. クライアントの実装の置き場所と、ブラウザの前提

- 音声を運ぶものは `@my/rtc`（[ADR-0025](0025-web-client-stack.md) §2）に置く。mycroblog を知らない `RoomSession` が、接続・マイクの公開・ミュート・出力の選択と、参加者・発話・通信の品質のイベントを持つ。
  実装は Web の 2 つ（P2P と SFU）で、どちらもブラウザの WebRTC の API だけを使い、依存を足さない。P2P のシグナリングはポートとして外から渡し、SFU はノードとの WebSocket（§11）を自分で持つ。
- 公式のモバイルアプリ（別のリポジトリのネイティブのアプリ）は、どちらの方式も libwebrtc と WebSocket だけで実装できる。
  画面のロックや電話の着信を越えて続く通話は、iOS では CallKit、Android では foreground service と Jetpack の Telecom が持つ（Android の文書、2026-11-12 に確認）。
- 通話の安定（回線が替わったときの ICE の再起動、つなぎ直し、デバイスの切り替え）は `RoomSession` の実装が持つ。`app-features` の通話は、ホスト・挙手・発話の許可などルームの状態だけを持つ。通話は、シェルが残すスロットに出て、画面を移っても続く。
- 実装は通信の品質（RTT・ジッタ・パケットロス）を ADR-0167 の計測に送る。
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
| Cloud Run | ストリームはリクエストで、開いている間 CPU が割り当てられる。リクエストの時間切れで切れたら §3 の再接続。`role = sfu` は動かせない（§15） |
| SFU のノード（`role = sfu`） | `stream` と `KeyValueStore` を web と共有する（§13・§14）。公開の IP・UDP と TCP のポート・ホスト名と TLS が要る |

- P2P では、音声はこのサーバーを通らない（TURN を使うときは TURN のサーバーを通る）。サーバーの負荷はシグナリング・ハートビート・ストリームだけである。

### 9. 運営者の文書と計測

- `docs/operations/` に通話の文書を置く（SFU の分は §16）: 方式の選び方、P2P の人数と回線の目安、STUN / TURN の設定（coturn・Cloudflare）、`relay` の意味、§4 の強制の限界、構成ごとの注意（§8）。`tools/agentkit/context-map.yaml` の `voice` の項目の `userdocs` にする。
- 計測（[ADR-0055](0055-service-observability.md)）: 開いたルームの数・参加者の数（Gauge）、中継したシグナリングと拒んだシグナリング（理由ごと）・TURN の資格情報の発行と失敗（Counter）。名前は ADR-0055 の規則で実装のときに決め、`agentkit docs inventory` の文書に載せる。

### 10. SFU: 置き場所と起動

- SFU は my の中にあり、外部のメディアサーバーを使わない。扱うのは音声（Opus）だけで、映像・画面の共有・配信は考えない（配信を作るなら別の仕組みにする）。
- 役割に `sfu` を足す（`all` / `web` / `worker` / `sfu`）。`role = sfu` のプロセスは §11 の WebSocket と媒体だけを受け、ジョブを実行しない。`all` は 1 プロセスで SFU も動かす。
- **SFU が動くのは `voice = sfu` のときだけ**。それ以外の値では、どの役割でも Pion・UDP と TCP の受け口・ノードの登録・WebSocket の受け口を作らない。`role = sfu` で `voice` が `sfu` でなければ起動しない。
- 依存は `pion/webrtc` v4（MIT）と、WebSocket の `coder/websocket`（ISC）（確認日 2026-11-26: 最新の版は v4.2.29 と v1.8.15）。ビルドタグ `no_voice_sfu` で外せ、外したビルドで `voice = sfu` にすると起動時に止まる。

### 11. SFU: 接続とシグナリング

- **入口**: `voice = sfu` の参加と取り直し（§6）の応答の `grant` は、ルームを持つノードの `wss://` の URL と、使い捨てのチケット（30 秒、`KeyValueStore`）を返す。ブラウザは WebSocket にヘッダーを付けられないので、チケットは URL のクエリで渡す。
  ノードはチケットを消費して（ルーム, セッション）を得る。期限切れ・使用済み・別のノード宛てのチケットは拒む。
- **シグナリングはノードとの WebSocket で直接行う**。web は媒体に関わらない。ルームの状態は §3 のストリームのまま。
- **offer はサーバーだけが出す**（glare がない）。1 セッションに `RTCPeerConnection` は 1 本。最初の offer は、本人のマイクを受ける 1 本と、話者 1 人ごとの送信の 1 本を持つ。話者の顔ぶれが変わったときだけ再 offer し、マイクのオン・オフや無音では再 offer しない。
- **メッセージ**は版（`v`）のある JSON で、client API の OpenAPI にスキーマとして載せる。TypeScript もネイティブのアプリもそこから型を作る。種類は `offer`・`answer`・`candidate`・`restart`（クライアントが ICE の再起動を求める）・`speaking`（§12）・`migrate`（§14）・`error`。
  ネイティブのアプリは libwebrtc と WebSocket だけで実装できる。
- **再接続**: 回線が替わったら、クライアントは `restart` を送り、サーバーは ICE の再起動の offer を出す。PeerConnection が生きたまま WebSocket が切れたら、取り直しで新しいチケットを得て同じセッションでつなぎ直す。ノードが落ちたら §14。

### 12. SFU: 転送と強制

- Opus をそのまま転送する。変換・ミキシング・帯域の推定・NACK はしない。サーバーの offer で DTX と帯域内の FEC を有効にする。
- **強制は転送の段で行う**。話せない（ADR-0100 §4 で話者でない）セッションと、ミュートさせられたセッションの RTP は、SDP が何であっても捨てる。退出・退出させられたセッションは接続を切る。P2P の結託の限界（§4）はない。
- **無音を捨てる**: クライアントが付ける音量の拡張ヘッダー（RFC 6464）で無音のパケットを捨て、音を出している話者はすべて転送する。捨てた分の欠けは、受け手には DTX と同じに見えるよう連番を詰める。
  転送の量は「音を出している話者 × 参加者」で、上限はルームの人数の上限（ADR-0100 §3）が抑える。
- **発話の表示**: 同じ音量からノードが話している人を求め、WebSocket の `speaking` で 250 ms に 1 回まで送る。
- **遅い受け手がルームを止めない**: 受け手ごとの送信は長さに上限のある列を通し、あふれたら捨てて数える。転送の経路でルーム全体のロックを取らない。

### 13. SFU: ルームの状態への追従

- ルームのドメインが正で、ノードはそれに従う（§2）。ノードは自分が持つルームの `voice_room`・`voice_mute` の知らせ（§3）を受けるたびと、定期的に（`voice.sweep` と同じ 1 分ごと）、ルームの状態を読み直して、話せる人・ミュート・いなくなった人・終わったルームを当てはめる。
  知らせが落ちても、次の読み直しで直る。ノードの間の新しい呼び出しは作らない。
- 方式のポート（§2）の `sfu` の adapter は、`Open` でルームを置き（§14）、`Grant` で §11 のチケットを出し、`Close` でルームを外す。`SetSpeaking`・`Mute`・`Unmute`・`Remove` は、ノードが状態を読み直すので何もしない。`Sessions` は、ノードが報告するつながっているセッションを返す。

### 14. SFU: 配置・容量・drain・ノードの喪失

- **登録**: ノードは `KeyValueStore` に、公開の URL・容量・予約の合計・つながっている数・drain 中かを、リース（5 秒ごとに更新、15 秒で切れる）で置く。ルームとノードの対応も、持つノードが更新するリースである。
- **配置**: ルームは 1 台のノードに置き、ノードをまたがない。ノードの容量（参加者の数）に対して、置いたルームの**人数の上限の合計**を予約として数え、入るノードのうち合計の最も小さいものに置く。
  置いた後にルームの人が増えても、ノードは容量を超えない。どのノードにも入らなければ、開く操作か最初の参加を `503` で拒む。
- **drain**: 止める合図（SIGTERM）で、ノードは新しいルームを受けなくなり、持つルームを他のノードに置き直して、WebSocket で `migrate` を送る。クライアントは取り直しで新しいノードのチケットを得てつなぎ直す（音の途切れは 1〜2 秒）。
  置き直す先がないときは、ルームが終わるか止める期限まで待つ。
- **ノードの喪失**: WebSocket が切れ、リースが切れたら、ルームの対応は消える。クライアントの取り直しが別のノードにルームを置き直す。

### 15. SFU: ネットワーク

- ノードは公開の IP で ICE-lite として動き、媒体を UDP の 1 つのポート（UDP mux）と、受け身の ICE-TCP の 1 つのポートで受ける。NAT の内側のノードは公開の IP を設定で渡す。ノードの WebSocket は公開のホスト名と TLS を持つ。
- **TURN は §5 のものを使う**（`none` / coturn / Cloudflare。資格情報の発行と取り直しは P2P と同じ）。既定の `none` では UDP と ICE-TCP だけで、443 番しか通らない利用者のために TURN/TLS が要る運営者が足す。ノードは公開の IP を持つので、SFU に STUN は要らない。
- **`relay` は `voice = sfu` で起動時に誤り**にする。SFU では利用者の IP はもともと他の参加者に見えず、ノードにだけ見える。`relay` は TURN の転送量を増やすだけである。
- Cloud Run は UDP を受けられないので、`role = sfu` は動かせない。Cloud Run の web の横に、公開の IP を持つ VM の SFU のノードを置く。

### 16. SFU: 計測・運営者の文書・消した方式

- 計測（ADR-0055）: ノードごとの、開いたルーム・セッション・予約とつながっている数（Gauge）、転送したパケットとバイト、捨てたもの（理由: 無音・話せない・列のあふれ）、再 offer・ICE の再起動・移動（Counter）、RTCP から求めたクライアントの RTT と損失（Histogram）。
  話者の多いルームの負荷は転送の量で見える。
- 運営者の文書（§9）: 方式の選び方、`role = sfu` のノードの動かし方（ポート・公開の IP と URL・TLS・容量）、drain、TURN、Cloud Run との組み合わせ、見るべき指標、ルームと参加者 1 人の CPU とメモリの実測。
- LiveKit と Sora は消した。消した値（`voice = livekit / sora` とその設定、方式ごとの人数の上限、ビルドタグ）を設定したデプロイは、何を設定すればよいかを言って起動しない。

### この決定を見直す条件

- P2P の利用が上限の人数でも成り立たないと分かったとき（P2P の人数の上限を下げる）。
- `Bus` の adapter のいずれかで KV の読み直しが遅すぎて接続が間に合わないと分かったとき（本文を `Bus` の知らせに載せる案を通話の知らせに限って検討する）。
- 1 台のノードに収まらないルームが要るとき（ノードをまたぐ転送を設計する）。
- 音を出す話者の多いルームで転送の量が指標の上で問題になったとき（音量の大きい順に上位の人だけ転送する案を検討する）。
- 映像や配信が要るとき（この SFU は広げず、別の仕組みを決める）。

## 経緯

M4.3（Voice Chat、epic #1345）のルームの振る舞いとデータモデルは ADR-0100 が決め、設定 `voice`、ルームのストリームとハートビートの経路、マイクとミュートの配り方、運営者の上限をこの ADR に任せた（FR-920・FR-921）。P2P なら最小の構成（Cloud Run・1 プロセスの SQLite）で、WebSocket の基盤も常駐のメディアサーバーも要らないようにし、ルームのドメインが正で、方式を 1 つのポートに閉じ込めることを優先した。`Bus` の約束（ADR-0050: 1 KiB まで、内容を載せない）は崩さず、運営者が望めば相手に IP を見せない構成にできるようにした。

SFU は初め LiveKit と Sora の adapter だった（spike #1347）が、epic #3559 で my の SFU に替えた。3 つの方式は `@my/rtc` に 3 つの実装を要し、ネイティブのアプリは SFU ごとの SDK と libwebrtc の組み込みを要した。Sora は動く台数ごとにライセンスが要り、実機では確かめていなかった。音声だけの SFU は、Opus をそのまま転送すれば、映像に要る帯域の推定・サイマルキャスト・キーフレームの扱いがなく小さい。
P2P は、UDP を受けられない構成（Cloud Run だけ）と小さなインスタンスのために残した。SFU は役割として web・worker と別に水平に増やせ、1 プロセスでもすべて動くことを条件にした。ノードの容量はルームの上限を予約して数え、ルームが置いた後に育ってもノードが不安定にならないことを、台数の効率より優先した。

## 採らなかった案

- LiveKit と Sora を残す: 運営者が SFU を選べるが、クライアントの実装が方式の数だけ要り、ネイティブのアプリは各 SDK を組み込む。
- SFU だけにして P2P も消す: 実装は 1 つになるが、UDP を受けられる VM がない構成では通話が使えない。
- シグナリングを web を通して運ぶ（今の P2P のように `POST` とストリーム）: 公開の入口が 1 つで済むが、サーバーからの再 offer が `Bus` と KV を通り、遅れと部品が増える。web への WebSocket をノードに中継する案も、web に長い接続とプロキシの層を持たせる。
- クライアントも offer する（perfect negotiation）: P2P と同じ手順にできるが、サーバーとの間で glare の扱いが要る。サーバーだけが offer すれば手順が一方向で、ネイティブの実装が単純になる。
- 音量の大きい上位の話者だけ転送する: 騒がしいルームの負荷を固定できるが、同時に話した人が聞こえなくなる。無音を捨てたうえで計測で見る（見直す条件に置く）。
- ノードの容量をつながっている数で数える: 台数は少なく済むが、置いた後にルームが育つとノードが容量を超える。
- drain でルームの終わりを待つ: 途切れはないが、デプロイが通話の長さ（最長 4 時間）に縛られる。
- SFU に TURN を組み込む: 外部のサービスが要らないが、443 番で受けるには WebSocket とは別の IP かポートと TLS が要り、P2P と共通の §5 があるので足さない。
- ネイティブのアプリでは SFU だけにし、P2P を使えなくする: アプリの実装が 1 つ減るが、P2P のサーバーではアプリで通話できない。どちらの方式も libwebrtc だけで実装できる（#2863）。

## Consequences（影響）

- 良い影響: P2P では新しい基盤なしで通話が動く。いまのストリームの仕組み（`Hub`・`Relay`・`Bus`）をそのまま使う。
- 良い影響: 方式のポートがルームのドメインと音声の経路を分ける。ルームの状態は DB が正で、ノードが後から従う。
- 良い影響: SFU では発話の制限をサーバーが強制し、利用者の IP は他の参加者に見えない。外部のメディアサーバーもライセンスも要らない。
- 良い影響: クライアントの実装は P2P と SFU の 2 つで、どちらも素の WebRTC なので、ネイティブのアプリは libwebrtc だけで書ける。
- 良い影響: SFU のノードは役割として増減でき、drain でデプロイが通話を待たない。
- 悪い影響: P2P の発話の制限は、結託した改造クライアントを防げない（§4）。文書で示し、厳密さが要る運営者には SFU を勧める。
- 悪い影響: 通話の間は SSE が 1 人 2 本（P2P）、または SSE と WebSocket が 1 本ずつ（SFU）になり、接続の数が増える。
- 悪い影響: シグナリングの本文を KV に置くので、P2P で `KeyValueStore = db` では接続のたびに数十行の書き込みが起きる（短い期限で消える）。
- 悪い影響: SFU のノードは公開の IP・UDP と TCP のポート・ホスト名と TLS を要し、Cloud Run だけの構成では動かない。
- 悪い影響: ノードの容量をルームの上限で予約するので、空きの多いルームが席を押さえ、台数が多めに要る。
- 悪い影響: 音を出す話者が多いルームでは、転送の量が話者の数とともに増える（人数の上限だけが抑える）。
- 悪い影響: drain とノードの喪失では、音が 1〜2 秒途切れる。

## Open Questions / 要検証

- iOS のホーム画面の Web アプリを背面にしたとき、マイクの取り込み（WebRTC の送信）が続くか（要検証: 実機。公開の文書では確かめられない）
