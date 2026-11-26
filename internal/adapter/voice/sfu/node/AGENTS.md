# Audio-only SFU on Pion

- Changing the ICE-lite, mux, or address settings of the node: [Pion WebRTC](https://pkg.go.dev/github.com/pion/webrtc/v4), search for: `SettingEngine`, `SetLite`, `SetICEUDPMux`, `SetICETCPMux`, `SetICEAddressRewriteRules`
- Changing what the node reads from the audio level extension: [RFC 6464](https://www.rfc-editor.org/rfc/rfc6464), search for: `level`, `voice activity`, `-dBov`
- Changing how the RTT and loss of a client are computed: [RFC 3550](https://www.rfc-editor.org/rfc/rfc3550), search for: `fraction lost`, `last SR`, `delay since last SR`
- Changing the WebSocket's accept, ping, or close: [coder/websocket](https://pkg.go.dev/github.com/coder/websocket), search for: `AcceptOptions`, `OriginPatterns`, `Ping`, `Close`
