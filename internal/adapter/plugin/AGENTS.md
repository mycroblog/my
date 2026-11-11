# Plugin protocol: Connect over HTTP

- Changing the request or the response on the wire: [Connect protocol reference](https://connectrpc.com/docs/protocol/), search for: `Connect-Protocol-Version`, `Connect-Timeout-Ms`, Unary, `application/proto`, `application/json`
- Mapping an error of a plugin: [Connect protocol reference](https://connectrpc.com/docs/protocol/), search for: Error Codes, `deadline_exceeded`, `unavailable`, `unimplemented`, HTTP to Error Code
- Calling the library: [connect-go v2 package documentation](https://pkg.go.dev/connectrpc.com/connect/v2), search for: `NewClient`, `CodeOf`, `Interceptor`, `Error`
