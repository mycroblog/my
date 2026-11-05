// SPDX-License-Identifier: AGPL-3.0-only

package contract_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/contract"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

func runKeyManager(t *testing.T, o fakeplugin.Options) map[string]contract.Result {
	t.Helper()
	k, _ := kind.ByName(kind.KeyManager)
	h, _ := fakeplugin.JSON(o)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeUnix(t, h)}, KeepDetail: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	out := map[string]contract.Result{}
	contract.Run(t.Context(), k, contract.Env{Conn: conn, Protocol: k.Protocol}, func(r contract.Result) { out[r.Case] = r })
	return out
}

func TestKeyManagerCases(t *testing.T) {
	t.Parallel()
	results := runKeyManager(t, fakeplugin.Options{KeyManager: testkit.NewKeyManager("hsm/transit-1")})
	for _, name := range []string{"reachable", "describe", "self_test", "unwrap_changed_value", "unwrap_unknown_key"} {
		if r := results[name]; r.Outcome != contract.Pass {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

// brokenStore is a key store that tests break in one way.
type brokenStore struct {
	kernel.KeyManager

	unwrap func(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error)
}

func (s brokenStore) UnwrapKey(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
	return s.unwrap(ctx, id, wrapped, aad)
}

// The cases tell a plugin's author what is wrong, each on its own.
func TestKeyManagerCasesFail(t *testing.T) {
	t.Parallel()
	good := testkit.NewKeyManager("hsm/transit-1")
	for _, tc := range []struct {
		name   string
		unwrap func(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error)
		fail   string
		detail string
	}{
		{
			name: "a value it cannot open reads as an unknown key",
			unwrap: func(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
				dek, err := good.UnwrapKey(ctx, id, wrapped, aad)
				if err != nil {
					return nil, kernel.ErrUnknownKey
				}
				return dek, nil
			},
			fail:   "unwrap_changed_value",
			detail: "OUTCOME_UNKNOWN_KEY, not OUTCOME_DECRYPT",
		},
		{
			name: "an unknown key reads as a value it cannot open",
			unwrap: func(ctx context.Context, id string, wrapped, aad []byte) ([]byte, error) {
				dek, err := good.UnwrapKey(ctx, id, wrapped, aad)
				if err != nil {
					return nil, kernel.ErrDecrypt
				}
				return dek, nil
			},
			fail:   "unwrap_unknown_key",
			detail: "OUTCOME_DECRYPT, not OUTCOME_UNKNOWN_KEY",
		},
		{
			name: "the AAD is ignored",
			unwrap: func(_ context.Context, _ string, _, _ []byte) ([]byte, error) {
				return []byte("whatever it holds"), nil
			},
			fail:   "self_test",
			detail: "round trip",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results := runKeyManager(t, fakeplugin.Options{KeyManager: brokenStore{KeyManager: good, unwrap: tc.unwrap}})
			r := results[tc.fail]
			if r.Outcome != contract.Fail || !strings.Contains(r.Detail, tc.detail) {
				t.Errorf("%s: %+v", tc.fail, r)
			}
		})
	}
}

// A plugin that serves nothing of the kind is told so.
func TestKeyManagerCasesOfAnotherKindOfPlugin(t *testing.T) {
	t.Parallel()
	results := runKeyManager(t, fakeplugin.Options{})
	if r := results["describe"]; r.Outcome != contract.Fail || !strings.Contains(r.Detail, "does the plugin serve this RPC") {
		t.Errorf("describe: %+v", r)
	}
}
