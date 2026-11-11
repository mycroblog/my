# Docker Compose deployment

- Changing a service, its health check, or its dependencies: [Compose file reference](https://docs.docker.com/reference/compose-file/), search for: `healthcheck`, `depends_on`, `service_healthy`, `profiles`, `secrets`
- Changing the reverse proxy: [Caddy documentation, Caddyfile](https://caddyserver.com/docs/caddyfile), search for: `reverse_proxy`, Automatic HTTPS, `encode`, Matchers
