# Server-side message formatting

- Changing how a message is compiled or formatted in Go: [messageformat-go mf1 package documentation](https://pkg.go.dev/github.com/kaptinlin/messageformat-go/mf1), search for: `Compile`, `MessageFormat`, `plural`, `select`
- Changing how the locale of a request is matched: [golang.org/x/text/language package documentation](https://pkg.go.dev/golang.org/x/text/language), search for: `NewMatcher`, `ParseAcceptLanguage`, `MatchStrings`
- Relying on a syntax of ICU MessageFormat in a message that Go formats: [ICU documentation, Formatting Messages](https://unicode-org.github.io/icu/userguide/format_parse/messages/), search for: `plural`, `select`, `offset`, apostrophe
