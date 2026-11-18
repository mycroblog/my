// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package main

import (
	"context"
	"io"
	"net/http"

	pushfake "github.com/mycroblog/my/internal/adapter/pushservice/fake"
)

// This file is the only one outside the tests that imports the fake push service, and only the
// test builds with the build tag e2e_fakes have it. scripts/go-fakes.sh fails when cmd/my
// without the tag depends on the fake, the Dockerfile refuses the tag, and
// scripts/check-release-build.sh refuses a binary built with it.

// runDevPushService implements `my dev push-service`: it serves the fake push service until ctx
// ends. It refuses every profile but development.
func runDevPushService(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return devFake{
		name: "my dev push-service", what: "a fake Web Push service", risk: "accepts any push message",
		originFlag: "origin", originUsage: "origin of the endpoints and of the VAPID audience",
		listen: "127.0.0.1:8930", cert: "push-service",
		handler: func(origin string) (http.Handler, error) {
			svc, err := pushfake.New(pushfake.Options{Origin: origin})
			if err != nil {
				return nil, err
			}
			return svc.Handler(), nil
		},
	}.run(ctx, args, stdout, stderr)
}
