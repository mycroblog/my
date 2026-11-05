// SPDX-License-Identifier: AGPL-3.0-only

package contract_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/media/native"
	"github.com/mycroblog/my/internal/adapter/plugin/client"
	"github.com/mycroblog/my/internal/adapter/plugin/contract"
	"github.com/mycroblog/my/internal/adapter/plugin/fakeplugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/media"
)

func runMedia(t *testing.T, o fakeplugin.Options) map[string]contract.Result {
	t.Helper()
	k, _ := kind.ByName(kind.Media)
	h, _ := fakeplugin.Connect(o)
	conn, err := client.New(client.Options{Kind: k, Destination: kind.Destination{Address: fakeplugin.ServeTCP(t, h)}, KeepDetail: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	out := map[string]contract.Result{}
	contract.Run(t.Context(), k, contract.Env{Conn: conn, Protocol: k.Protocol}, func(r contract.Result) { out[r.Case] = r })
	return out
}

var mediaCaseNames = []string{"process_photo", "process_emoji", "process_animation", "remux_audio"}

func TestMediaCases(t *testing.T) {
	results := runMedia(t, fakeplugin.Options{Media: native.New(native.Options{})})
	for _, name := range append([]string{"reachable"}, mediaCaseNames...) {
		if r := results[name]; r.Outcome != contract.Pass {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

// keeping is a plugin's processor that hands back the upload as its large output, with its metadata: the server's checks
// accept it, and the cases do not.
type keeping struct {
	*native.Processor
}

func (k keeping) Process(ctx context.Context, data []byte, limits media.Limits, sizes media.Sizes, req media.Request) (media.Output, error) {
	out, err := k.Processor.Process(ctx, data, limits, sizes, req)
	if err != nil || media.Sniff(data) != media.FormatJPEG {
		return out, err
	}
	probe, err := k.Inspect(ctx, data, limits)
	if err != nil {
		return out, err
	}
	out.Still.Large = media.Variant{Data: data, ContentType: "image/jpeg", Width: probe.Width, Height: probe.Height}
	return out, nil
}

func (k keeping) Remux(ctx context.Context, data []byte, limits media.AVLimits) (media.AVOutput, error) {
	probe, err := k.InspectAV(ctx, data, limits)
	return media.AVOutput{AVProbe: probe, Data: data}, err
}

func TestMediaCasesFindTheLocation(t *testing.T) {
	results := runMedia(t, fakeplugin.Options{Media: keeping{native.New(native.Options{})}})
	for name, want := range map[string]string{"process_photo": "MY-PLUGIN-TEST-LOCATION", "remux_audio": "the tags of the file came back"} {
		if r := results[name]; r.Outcome != contract.Fail || !strings.Contains(r.Detail, want) {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

// A plugin that breaks the rules of the output fails the case with the reason the server would refuse it.
func TestMediaCasesReportAnOutputTheServerRefuses(t *testing.T) {
	results := runMedia(t, fakeplugin.Options{Media: native.New(native.Options{}), MediaUnderstate: true})
	for _, name := range mediaCaseNames {
		if r := results[name]; r.Outcome != contract.Fail || !strings.Contains(r.Detail, "longer than its declared") {
			t.Errorf("%s: %+v", name, r)
		}
	}
}
