// SPDX-License-Identifier: AGPL-3.0-only

package platform

import "github.com/mycroblog/my/internal/app/voice"

// setUpVoice builds the voice module (ADR-0100): the projectors of its projections, which the
// commands of the rooms rely on.
func (a *App) setUpVoice(repo voice.Repository) {
	voice.RegisterProjectors(a.Projectors, repo)
}
