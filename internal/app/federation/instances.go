// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
)

// InstanceRepository is the persistence port of the remote servers this instance has seen and
// the software their NodeInfo reported (remote_instances and instance_nodeinfo_observations,
// data-model §4.8). The database adapters implement it on their federation repository and pass
// federationtest.RemoteActorContract. Lookups that find nothing return an error wrapping ErrNotFound.
type InstanceRepository interface {
	// InsertRemoteInstance records a server the first time it is seen, and reports whether it
	// did.
	InsertRemoteInstance(ctx context.Context, tx uow.Tx, domain string, firstSeenAt time.Time) (bool, error)
	// LatestNodeInfoObservation returns the newest NodeInfo observation of a server.
	LatestNodeInfoObservation(ctx context.Context, tx uow.Tx, domain string) (federation.NodeInfoObservation, error)
	InsertNodeInfoObservation(ctx context.Context, tx uow.Tx, o federation.NodeInfoObservation) error
}

// InstanceStates reads whether a remote server is a delivery target; Repository implements it.
type InstanceStates interface {
	InstanceState(ctx context.Context, domain string) (federation.InstanceState, error)
}
