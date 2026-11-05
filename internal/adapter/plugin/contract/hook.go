// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"context"
	"errors"
	"fmt"

	hookv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/hook/v1/hookv1connect"
)

// hookCases check that a plugin of a hook answers from the hook's vocabulary. The calls carry probe,
// so a real plugin knows that they are about no real subject.
func hookCases(name string, verdicts ...hookv1.Verdict) []Case {
	return []Case{
		{Name: "verdict", Run: func(ctx context.Context, env Env) error {
			res, err := hookv1connect.NewHookServiceClient(env.Conn.RPC).Check(ctx, &hookv1.CheckRequest{
				Hook: name, SubjectKey: "my-plugin-test", Probe: true,
			})
			if err != nil {
				return describeCallError(env, err)
			}
			for _, v := range verdicts {
				if res.GetVerdict() == v {
					return checkNote(res)
				}
			}
			return fmt.Errorf("the verdict %s is not one that %s takes", res.GetVerdict(), name)
		}},
	}
}

func checkNote(res *hookv1.CheckResponse) error {
	if len([]rune(res.GetNote())) > 200 {
		return errors.New("the note is longer than 200 characters, and the server cuts it")
	}
	return nil
}
