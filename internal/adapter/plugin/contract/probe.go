// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect/v2"

	probev1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/probe/v1/probev1connect"
)

// probeCases check that the plugin speaks the protocol and proto3's JSON mapping.
var probeCases = []Case{
	{Name: "echo", Run: func(ctx context.Context, env Env) error {
		// The largest value that a double would round, which a plugin must read as a string.
		return echo(ctx, env, []byte("my plugin test \x00\xff"), 9007199254740993)
	}},
	{Name: "echo_empty", Run: func(ctx context.Context, env Env) error {
		return echo(ctx, env, nil, 0)
	}},
}

func echo(ctx context.Context, env Env, payload []byte, number int64) error {
	res, err := probev1connect.NewProbeServiceClient(env.Conn.RPC).Echo(ctx, &probev1.EchoRequest{Payload: payload, Number: number})
	if err != nil {
		return describeCallError(env, err)
	}
	switch {
	case res.GetOutcome() != probev1.Outcome_OUTCOME_OK:
		return fmt.Errorf("the outcome is %s, not OUTCOME_OK", res.GetOutcome())
	case !bytes.Equal(res.GetPayload(), payload):
		return errors.New("the payload came back changed")
	case res.GetNumber() != number:
		return fmt.Errorf("the number came back as %d, not %d", res.GetNumber(), number)
	}
	return nil
}

// describeCallError adds what to look at to a failed call. A plugin that does not take the protocol answers 415, which
// a Connect client reads as a failed call with no further word.
func describeCallError(env Env, err error) error {
	switch connect.CodeOf(err) {
	case connect.CodeUnimplemented, connect.CodeInternal, connect.CodeUnknown:
		return fmt.Errorf("%w (does the plugin serve this RPC, and take the protocol %s?)", err, env.Protocol)
	}
	return err
}
