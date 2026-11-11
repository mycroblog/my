# Web client shell, feeds, and well-known files

- Changing the Atom feed: [RFC 4287](https://www.rfc-editor.org/rfc/rfc4287), search for: `atom:entry`, `atom:updated`, `atom:id`, Text Constructs
- Changing `robots.txt`: [RFC 9309](https://www.rfc-editor.org/rfc/rfc9309), search for: `User-agent`, `Disallow`, Longest Match
- Changing `security.txt`: [RFC 9116](https://www.rfc-editor.org/rfc/rfc9116), search for: `Contact`, `Expires`, `/.well-known/security.txt`
- Changing the Web App Manifest: [Web Application Manifest](https://www.w3.org/TR/appmanifest/), search for: `start_url`, `display`, `purpose`, `maskable`, `scope`
- Reading the build manifest of Vite or writing the tags of the shell: [Vite documentation](https://vite.dev/llms.txt), search for: Backend Integration, `manifest.json`, `isEntry`, `modulepreload`
- Changing the Content Security Policy of the shell: [Content Security Policy Level 3](https://www.w3.org/TR/CSP3/), search for: `script-src`, `style-src`, `connect-src`, `frame-ancestors`
- Changing the Open Graph tags of a page: [The Open Graph protocol](https://ogp.me/), search for: `og:type`, `og:image`, `article:published_time`, `profile:username`
- Changing the text and data mining reservation: [TDM Reservation Protocol](https://www.w3.org/community/reports/tdmrep/CG-FINAL-tdmrep-20240202/), search for: `tdm-reservation`, `tdm-policy`
- Changing the AI training preference in `robots.txt` (an Internet-Draft): [draft-kelley-aipref-training-preference-signal](https://datatracker.ietf.org/doc/draft-kelley-aipref-training-preference-signal/), search for: `ai-train`, `Content-Usage`
