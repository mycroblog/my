# LiveKit server API and access tokens

- Calling the RoomService (Twirp): [LiveKit documentation](https://docs.livekit.io/llms.txt), search for: `CreateRoom`, `UpdateParticipant`, `MutePublishedTrack`, `RemoveParticipant`, `empty_timeout`
- Changing the grants of an access token: [LiveKit documentation](https://docs.livekit.io/llms.txt), search for: `canPublish`, `canPublishSources`, `canSubscribe`, `roomJoin`, `identity`
- Changing what a participant may publish after it joined: [LiveKit documentation](https://docs.livekit.io/llms.txt), search for: `ParticipantPermission`, `can_publish_sources`, `UpdateParticipant`
- Signing the token of an API call: [LiveKit documentation](https://docs.livekit.io/llms.txt), search for: `roomAdmin`, `roomCreate`, `roomList`
