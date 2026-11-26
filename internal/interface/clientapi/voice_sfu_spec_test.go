// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"encoding/json"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
)

const sfuParticipant = "01890a5d-ac96-774b-bcce-b302099a8057"

// TestVoiceSFUMessageSchemas checks that the client API document describes the messages of the
// media node's WebSocket (ADR-0101 §11): every message has the version 1 and a type, the client
// sends answer, candidate and restart, and the node sends offer, candidate, speaking, migrate and
// error.
func TestVoiceSFUMessageSchemas(t *testing.T) {
	spec := apitest.Load(t)
	schemas := spec.Doc().Components.Schemas
	visit := func(t *testing.T, name, msg string) error {
		t.Helper()
		ref, ok := schemas[name]
		if !ok || ref.Value == nil {
			t.Fatalf("the document has no schema %s", name)
		}
		var v any
		if err := json.Unmarshal([]byte(msg), &v); err != nil {
			t.Fatalf("%s: %v", msg, err)
		}
		return ref.Value.VisitJSON(v)
	}
	for _, tc := range []struct {
		schema string
		valid  []string
		// invalid are messages the schema refuses.
		invalid []string
	}{
		{
			schema: "VoiceSFUClientMessage",
			valid: []string{
				`{"v":1,"type":"answer","sdp":"v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\n"}`,
				`{"v":1,"type":"candidate","candidate":"{\"candidate\":\"candidate:1 1 udp 2130706431 192.0.2.1 50000 typ host\",\"sdpMid\":\"0\",\"sdpMLineIndex\":0}"}`,
				`{"v":1,"type":"candidate","candidate":"{\"candidate\":\"\"}"}`,
				`{"v":1,"type":"restart"}`,
			},
			invalid: []string{
				`{"type":"restart"}`,
				`{"v":2,"type":"restart"}`,
				`{"v":1}`,
				`{"v":1,"type":"answer"}`,
				`{"v":1,"type":"candidate"}`,
				`{"v":1,"type":"offer","sdp":"v=0","tracks":[]}`,
				`{"v":1,"type":"speaking","participants":[]}`,
				`{"v":1,"type":"hello"}`,
			},
		},
		{
			schema: "VoiceSFUNodeMessage",
			valid: []string{
				`{"v":1,"type":"offer","sdp":"v=0\r\n","tracks":[{"mid":"1","participant":"` + sfuParticipant + `"}]}`,
				`{"v":1,"type":"offer","sdp":"v=0\r\n","tracks":[]}`,
				`{"v":1,"type":"candidate","candidate":"{\"candidate\":\"candidate:1 1 udp 2130706431 203.0.113.7 7882 typ host\"}"}`,
				`{"v":1,"type":"speaking","participants":["` + sfuParticipant + `"]}`,
				`{"v":1,"type":"speaking","participants":[]}`,
				`{"v":1,"type":"migrate"}`,
				`{"v":1,"type":"error","code":"invalid_ticket","message":"the ticket expired"}`,
			},
			invalid: []string{
				`{"type":"migrate"}`,
				`{"v":2,"type":"migrate"}`,
				`{"v":1,"type":"offer","sdp":"v=0"}`,
				`{"v":1,"type":"offer","tracks":[]}`,
				`{"v":1,"type":"offer","sdp":"v=0","tracks":[{"mid":"1"}]}`,
				`{"v":1,"type":"offer","sdp":"v=0","tracks":[{"participant":"` + sfuParticipant + `"}]}`,
				`{"v":1,"type":"speaking"}`,
				`{"v":1,"type":"error","code":"invalid_ticket"}`,
				`{"v":1,"type":"error","message":"no code"}`,
				`{"v":1,"type":"answer","sdp":"v=0"}`,
				`{"v":1,"type":"restart"}`,
			},
		},
	} {
		t.Run(tc.schema, func(t *testing.T) {
			for _, msg := range tc.valid {
				if err := visit(t, tc.schema, msg); err != nil {
					t.Errorf("%s refuses %s: %v", tc.schema, msg, err)
				}
				spec.ValidateSchema(t, tc.schema, []byte(msg))
			}
			for _, msg := range tc.invalid {
				if err := visit(t, tc.schema, msg); err == nil {
					t.Errorf("%s accepts %s", tc.schema, msg)
				}
			}
		})
	}
}

// TestVoiceParticipationSchemas checks that the participation response (VoiceRoomJoin) and a new
// grant (VoiceGrant) describe both methods (ADR-0101 §6, §11): p2p with its ICE servers alone,
// and sfu with the node's wss: URL and its ticket.
func TestVoiceParticipationSchemas(t *testing.T) {
	spec := apitest.Load(t)
	grants := map[string]string{
		"p2p": `{"kind":"p2p","ice_servers":[{"urls":["stun:stun.example.org"]}],"options":{},"relay_only":false,"expires_at":"2026-10-26T15:00:00Z"}`,
		"sfu": `{"kind":"sfu","url":"wss://social.example/voice/sfu","token":"ticket-1","ice_servers":[{"urls":["turns:turn.example.org:5349"],"username":"u","credential":"c"}],"options":{},"relay_only":false,"expires_at":"2026-10-26T09:00:30Z"}`,
	}
	for kind, grant := range grants {
		t.Run(kind, func(t *testing.T) {
			spec.ValidateSchema(t, "VoiceGrant", []byte(grant))
		})
	}
	ref, ok := spec.Doc().Components.Schemas["VoiceGrant"]
	if !ok || ref.Value == nil {
		t.Fatal("the document has no schema VoiceGrant")
	}
	var unknown any
	if err := json.Unmarshal([]byte(`{"kind":"mesh","ice_servers":[],"options":{},"relay_only":false,"expires_at":"2026-10-26T15:00:00Z"}`), &unknown); err != nil {
		t.Fatal(err)
	}
	if err := ref.Value.VisitJSON(unknown); err == nil {
		t.Error("VoiceGrant accepts the kind mesh")
	}
	join, ok := spec.Doc().Components.Schemas["VoiceRoomJoin"]
	if !ok || join.Value == nil || join.Value.Properties["grant"] == nil {
		t.Fatal("VoiceRoomJoin has no grant")
	}
	if g := join.Value.Properties["grant"].Value; g == nil || g.Properties["kind"] == nil || g.Properties["url"] == nil || g.Properties["token"] == nil {
		t.Errorf("the grant of VoiceRoomJoin = %+v, want kind, url and token", g)
	}
}
