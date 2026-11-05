# SPDX-License-Identifier: AGPL-3.0-only
#
# Akkoma for the live interoperability stack (../compose.yaml): the official OTP release of Akkoma (AGPL-3.0), which
# the project publishes as a zip and not as a container image, on the Debian release it is built for. The base image
# is pinned by tag and digest, and the zip by its SHA-256.

FROM debian:bookworm-20261020-slim@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8

ARG AKKOMA_VERSION=v3.20.0
ARG AKKOMA_SHA256=7f8f3bcdfa823f33d3ae90ce7b5af3e55f3472fc2e614c4d28f6d3c332464152

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl unzip libncurses5 libmagic1 \
	&& rm -rf /var/lib/apt/lists/*

RUN curl -fsSL -o /tmp/akkoma.zip "https://akkoma-updates.s3-website.fr-par.scw.cloud/${AKKOMA_VERSION}/akkoma-amd64.zip" \
	&& echo "${AKKOMA_SHA256}  /tmp/akkoma.zip" | sha256sum -c - \
	&& unzip -q /tmp/akkoma.zip -d /tmp/akkoma \
	&& mv /tmp/akkoma/release /opt/akkoma \
	&& rm -rf /tmp/akkoma /tmp/akkoma.zip \
	&& mkdir -p /var/lib/akkoma/uploads /var/lib/akkoma/static

COPY entrypoint.sh /usr/local/bin/akkoma-entrypoint

ENV AKKOMA_CONFIG_PATH=/etc/akkoma/config.exs LANG=C.UTF-8 ELIXIR_ERL_OPTIONS=+fnu
EXPOSE 4000
ENTRYPOINT ["/usr/local/bin/akkoma-entrypoint"]
