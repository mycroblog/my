// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestInstanceEventCreators checks that GET /instance says who may create events, from the
// instance setting event_creators (ADR-0161 §3).
func TestInstanceEventCreators(t *testing.T) {
	setting := func(v instance.EventCreators) *fakeInstance {
		return &fakeInstance{settings: []instance.CurrentSetting{
			{Key: instance.SettingEventCreators, RevisionID: testkit.NamedID("revision-1"), Value: string(v)},
		}}
	}
	for _, tc := range []struct {
		name string
		inst *fakeInstance
		want clientapi.InstanceEventCreators
	}{
		{"the default", &fakeInstance{}, clientapi.InstanceEventCreatorsEveryone},
		{"everyone", setting(instance.EventCreatorsEveryone), clientapi.InstanceEventCreatorsEveryone},
		{"permitted", setting(instance.EventCreatorsPermitted), clientapi.InstanceEventCreatorsPermitted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) { o.Instance = tc.inst })
			res := e.send(t, http.MethodGet, "/instance", "", "")
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET /instance = %d", res.StatusCode)
			}
			if got := decode[clientapi.Instance](t, res); got.EventCreators != tc.want {
				t.Errorf("event_creators %q; want %q", got.EventCreators, tc.want)
			}
		})
	}
}
