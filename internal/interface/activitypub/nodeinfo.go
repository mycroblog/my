// SPDX-License-Identifier: AGPL-3.0-only

package activitypub

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/domain/federation"
)

// nodeInfoSchema names NodeInfo 2.1, as the link relation and the profile of the document.
const nodeInfoSchema = "http://nodeinfo.diaspora.software/ns/schema/2.1"

// softwareName is the server's NodeInfo software name (^[a-z0-9-]+$).
const softwareName = "my"

// nodeInfoIndex serves GET /.well-known/nodeinfo: the link to the NodeInfo 2.1 document.
func (h *Handler) nodeInfoIndex(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, r, typeJSON, struct {
		Links []jrdLink `json:"links"`
	}{Links: []jrdLink{{Rel: nodeInfoSchema, Href: "https://" + h.o.Host + PathNodeInfo}}})
}

// NodeMetadata is what NodeInfo's free-form metadata says about the instance besides its name.
// Empty fields are left out.
type NodeMetadata struct {
	// Description is the instance's one-paragraph summary (nodeDescription).
	Description string
	// ContactActorURI is the actor id of the local account to contact about the instance, which
	// staffAccounts lists, and ContactName is its display name or username (maintainer.name).
	ContactActorURI, ContactName string
	// ContactEmail is the email address to contact about the instance (maintainer.email).
	ContactEmail string
	// ThumbnailURL and ThumbnailAlt are the image that represents the instance and its alt text
	// (thumbnail.url and thumbnail.alt).
	ThumbnailURL, ThumbnailAlt string
	// Closure is the closure of the service (serviceClosure); the zero value is none.
	Closure NodeClosure
}

// NodeClosure is the closure of the service as NodeInfo's metadata states it (ADR-0088 §2).
type NodeClosure struct {
	// State is the state of instance.Closure (none, announced, closed, delivery_completed); ""
	// is none.
	State string
	// ClosesAt and ExportUntil are its schedule, the zero time for none.
	ClosesAt, ExportUntil time.Time
}

// nodeServiceClosure is metadata.serviceClosure: the state, and the dates while a closure is
// announced or after it.
type nodeServiceClosure struct {
	State       string     `json:"state"`
	ClosesAt    *time.Time `json:"closesAt,omitempty"`
	ExportUntil *time.Time `json:"exportUntil,omitempty"`
}

func newNodeServiceClosure(c NodeClosure) nodeServiceClosure {
	if c.State == "" || c.State == "none" {
		return nodeServiceClosure{State: "none"}
	}
	closes, until := c.ClosesAt.UTC(), c.ExportUntil.UTC()
	return nodeServiceClosure{State: c.State, ClosesAt: &closes, ExportUntil: &until}
}

// nodeMaintainer is the maintainer of Misskey's NodeInfo metadata.
type nodeMaintainer struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// nodeThumbnail is the thumbnail of the metadata, shaped as the thumbnail of Mastodon's
// instance API.
type nodeThumbnail struct {
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

// metadata returns NodeInfo's metadata: nodeName and, from m, the keys that Mastodon, Misskey,
// and Pleroma read (nodeDescription; maintainer as Misskey; staffAccounts as Pleroma, the actor
// ids) with the thumbnail, and the closure of the service.
func metadata(name, sourceURL string, m NodeMetadata, mode federation.Mode) map[string]any {
	// federationMode tells other servers and their users the direction this server federates
	// in (ADR-0076); serviceClosure tells them whether and when it closes (ADR-0088 §2).
	out := map[string]any{"nodeName": name, "federationMode": mode, "serviceClosure": newNodeServiceClosure(m.Closure)}
	if sourceURL != "" {
		// Where users get the source (AGPL-3.0 section 13, ADR-0011), also when it is a mailto: address, which
		// software.repository cannot hold.
		out["sourceCode"] = sourceURL
	}
	if m.Description != "" {
		out["nodeDescription"] = m.Description
	}
	if m.ContactName != "" || m.ContactEmail != "" {
		out["maintainer"] = nodeMaintainer{Name: m.ContactName, Email: m.ContactEmail}
	}
	if m.ContactActorURI != "" {
		out["staffAccounts"] = []string{m.ContactActorURI}
	}
	if m.ThumbnailURL != "" {
		out["thumbnail"] = nodeThumbnail{URL: m.ThumbnailURL, Alt: m.ThumbnailAlt}
	}
	return out
}

type nodeInfo struct {
	Version           string           `json:"version"`
	Software          nodeInfoSoftware `json:"software"`
	Protocols         []string         `json:"protocols"`
	Services          nodeInfoServices `json:"services"`
	OpenRegistrations bool             `json:"openRegistrations"`
	Usage             nodeInfoUsage    `json:"usage"`
	Metadata          map[string]any   `json:"metadata"`
}

// repository is the source URL as NodeInfo 2.1 software.repository, whose schema takes only http(s) URLs: a
// mailto: address is left out of it (and published in metadata.sourceCode only).
func repository(sourceURL string) string {
	if !strings.HasPrefix(sourceURL, "https://") {
		return ""
	}
	return sourceURL
}

type nodeInfoSoftware struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Repository is the source code that AGPL-3.0 §13 offers to users (ADR-0011).
	Repository string `json:"repository,omitempty"`
}

type nodeInfoServices struct {
	Inbound  []string `json:"inbound"`
	Outbound []string `json:"outbound"`
}

type nodeInfoUsage struct {
	Users      nodeInfoUsers `json:"users"`
	LocalPosts int64         `json:"localPosts"`
}

type nodeInfoUsers struct {
	Total int `json:"total"`
}

// nodeInfo serves GET /nodeinfo/2.1 (FR-402). openRegistrations follows the registration mode,
// as Mastodon reports it: anyone may sign up, with or without a moderator's approval. localPosts
// counts the local posts as Mastodon does (content.Service.LocalPostCount); active-user counts
// are omitted because logins are not counted.
func (h *Handler) nodeInfo(w http.ResponseWriter, r *http.Request) {
	mode, ok := h.mode(w, r)
	if !ok {
		return
	}
	users, err := h.o.Accounts.CountLocalUsers(r.Context())
	if err != nil {
		h.o.Logger.ErrorContext(r.Context(), "nodeinfo: count users failed", slog.Any("error", err))
		h.o.Problem(w, r, http.StatusInternalServerError)
		return
	}
	open := false
	if h.o.OpenRegistrations != nil {
		if open, err = h.o.OpenRegistrations(r.Context()); err != nil {
			h.o.Logger.ErrorContext(r.Context(), "nodeinfo: read the registration mode failed", slog.Any("error", err))
			h.o.Problem(w, r, http.StatusInternalServerError)
			return
		}
	}
	var posts int64
	if h.o.LocalPosts != nil {
		if posts, err = h.o.LocalPosts(r.Context()); err != nil {
			h.o.Logger.ErrorContext(r.Context(), "nodeinfo: count local posts failed", slog.Any("error", err))
			h.o.Problem(w, r, http.StatusInternalServerError)
			return
		}
	}
	// nodeName is the name that Mastodon and other servers read from NodeInfo's free-form
	// metadata to show the instance by.
	name := h.o.Host
	if h.o.InstanceName != nil {
		if n := h.o.InstanceName(r.Context()); n != "" {
			name = n
		}
	}
	var meta NodeMetadata
	if h.o.Metadata != nil {
		if meta, err = h.o.Metadata(r.Context()); err != nil {
			h.o.Logger.ErrorContext(r.Context(), "nodeinfo: read the instance metadata failed", slog.Any("error", err))
			h.o.Problem(w, r, http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	h.writeJSON(w, r, typeNodeInfo, nodeInfo{
		Version:           "2.1",
		Software:          nodeInfoSoftware{Name: softwareName, Version: h.o.SoftwareVersion, Repository: repository(h.o.SourceURL)},
		Protocols:         []string{"activitypub"},
		Services:          nodeInfoServices{Inbound: []string{}, Outbound: []string{}},
		OpenRegistrations: open,
		Usage:             nodeInfoUsage{Users: nodeInfoUsers{Total: users}, LocalPosts: posts},
		Metadata:          metadata(name, h.o.SourceURL, meta, mode),
	})
}
