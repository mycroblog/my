# syntax=docker/dockerfile:1
# SPDX-License-Identifier: MIT
#
# The container image of an example bot of pkg/mybot: static, non-root, distroless. The bot keeps its state, the
# processed notifications and the rotated refresh token, in a FileStore on the volume /data, which must outlive
# the container: a new container with an empty state starts from MYBOT_REFRESH_TOKEN again, which the server has
# seen used, and the token stops working.
#
#   docker build -f pkg/mybot/examples/Dockerfile --build-arg EXAMPLE=echo -t mybot-echo .   # from the repository root
#   docker run -d --env-file bot.env -v mybot-echo:/data mybot-echo
#
# EXAMPLE is echo, schedule, or commands. bot.env holds MYBOT_SERVER, MYBOT_CLIENT_ID, MYBOT_CLIENT_SECRET, and
# MYBOT_REFRESH_TOKEN (and MYBOT_TIME_ZONE for schedule). `docker stop` sends SIGTERM, on which the bot shuts down
# gracefully: it waits up to 10 seconds for its handlers, so stop it with a longer timeout, such as docker stop -t 30.
#
# The build context is the repository root, since pkg/mybot builds with the pkg/mykit of the same commit (the
# replace of its go.mod); Dockerfile.dockerignore keeps the context to those two modules. In a module of your own
# that requires github.com/mycroblog/my/pkg/mybot, copy your go.mod, go.sum, and sources instead.


FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d AS build
ARG TARGETOS TARGETARCH
ARG EXAMPLE=echo
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src/pkg/mybot
COPY pkg/mykit/go.mod pkg/mykit/go.sum /src/pkg/mykit/
COPY pkg/mybot/go.mod pkg/mybot/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY pkg/mykit/ /src/pkg/mykit/
COPY pkg/mybot/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/bot "./examples/${EXAMPLE}" \
    && mkdir /out/data

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
COPY --from=build /out/bot /bot
# The volume's directory belongs to the user nonroot, so that a new named volume takes its owner.
COPY --from=build --chown=65532:65532 /out/data /data
ENV MYBOT_STATE=/data/mybot-state.json
VOLUME /data
USER nonroot:nonroot
ENTRYPOINT ["/bot"]
