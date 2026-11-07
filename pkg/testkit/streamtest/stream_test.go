// SPDX-License-Identifier: AGPL-3.0-only

package streamtest_test

import (
	"context"
	"testing"

	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/pkg/testkit/streamtest"
)

func TestStreamBusContractMemoryBroker(t *testing.T) {
	streamtest.StreamBusContract(t, func(*testing.T) streamtest.StreamBus {
		broker := stream.NewMemoryBroker()
		return streamtest.StreamBus{
			NewBus: func(*testing.T) stream.Bus { return broker.Bus() },
			Unreachable: func(*testing.T) stream.Bus {
				unreachable := stream.NewMemoryBroker()
				unreachable.SetUnreachable(true)
				return unreachable.Bus()
			},
			Break:     func(*testing.T) { broker.Break() },
			Resources: func(*testing.T) int { return broker.Listeners() },
		}
	})
}

func TestStreamBrokerContractHub(t *testing.T) {
	streamtest.StreamBrokerContract(t, func(t *testing.T, o stream.HubOptions) streamtest.StreamBroker {
		h := stream.NewHub(o)
		t.Cleanup(h.Close)
		return streamtest.StreamBroker{Publisher: h, Subscriber: h}
	})
}

func TestStreamBrokerContractRelay(t *testing.T) {
	streamtest.StreamBrokerContract(t, func(t *testing.T, o stream.HubOptions) streamtest.StreamBroker {
		broker := stream.NewMemoryBroker()
		local := stream.NewRelay(broker.Bus(), stream.RelayOptions{Hub: o})
		remote := stream.NewRelay(broker.Bus(), stream.RelayOptions{})
		for _, r := range []*stream.Relay{local, remote} {
			t.Cleanup(func() {
				if err := r.Close(context.Background()); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
		}
		return streamtest.StreamBroker{
			Publisher: local, Subscriber: local, Remote: remote,
			Break: func(*testing.T) { broker.Break() },
		}
	})
}
