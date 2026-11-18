// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package main

import (
	"context"
	"flag"
	"io"
	"net/http"

	oidcfake "github.com/mycroblog/my/internal/adapter/oidc/fake"
)

// This file is the only one outside the tests that imports the fake OpenID Connect provider, and
// only the test builds with the build tag e2e_fakes have it. scripts/go-fakes.sh fails when
// cmd/my without the tag depends on the fake, the Dockerfile refuses the tag, and
// scripts/check-release-build.sh refuses a binary built with it.

// runDevOIDCProvider implements `my dev oidc-provider`: it serves the fake OpenID Connect
// provider until ctx ends. It refuses every profile but development.
func runDevOIDCProvider(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var clientID, clientSecret string
	return devFake{
		name: "my dev oidc-provider", what: "a fake OpenID Connect provider", risk: "signs anyone in without a password",
		originFlag: "issuer", originUsage: "issuer of the provider, the origin it is reached at",
		listen: "127.0.0.1:8931", cert: "oidc-provider",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&clientID, "client-id", "my", "client ID of the one client, the login provider of the server")
			fs.StringVar(&clientSecret, "client-secret", "my-dev-client-secret", "client secret of -client-id")
		},
		handler: func(issuer string) (http.Handler, error) {
			p, err := oidcfake.New(oidcfake.Options{Issuer: issuer, ClientID: clientID, ClientSecret: clientSecret})
			if err != nil {
				return nil, err
			}
			return p.Handler(), nil
		},
	}.run(ctx, args, stdout, stderr)
}
