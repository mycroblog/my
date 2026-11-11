# Plugin protocol definitions (protobuf, Buf)

- Adding or changing a field or a message without breaking a plugin: [Protocol Buffers documentation](https://protobuf.dev/), search for: Updating A Message Type, `reserved`, Field Presence, Proto Best Practices
- Changing the lint or the breaking-change rules: [Buf documentation](https://buf.build/docs/), search for: `buf lint`, `buf breaking`, `WIRE_JSON`, `STANDARD`
- Changing the code generation: [Buf documentation](https://buf.build/docs/), search for: `buf.gen.yaml`, Managed mode, `go_package_prefix`
