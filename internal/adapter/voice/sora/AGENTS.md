# Sora HTTP API (Sora and Sora Cloud)

- Calling the HTTP API or adding one of its operations: [Sora documentation](https://sora-doc.shiguredo.jp/llms.txt), search for: `x-sora-target`, `Sora_20230628.TerminateSession`, `Sora_20151104.DisconnectConnection`, `Sora_20201013.ListChannelConnections`
- Changing who may send audio (forwarding filters): [Sora documentation](https://sora-doc.shiguredo.jp/llms.txt), search for: `Sora_20230628.CreateChannelForwardingFilter`, `rules`, `action`, `is_in`
- Changing the answer of the auth webhook: [Sora documentation](https://sora-doc.shiguredo.jp/llms.txt), search for: `allowed`, `client_id`, `metadata`, `sendrecv`, `recvonly`
- Changing the calls to Sora Cloud (its API endpoint, its token, its channel names): [Sora Cloud documentation](https://doc.sora-cloud.shiguredo.app/llms.txt), search for: `https://api.sora-cloud.shiguredo.app/sora-api`, `access_token`, `channel_id`
