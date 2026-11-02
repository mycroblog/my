// SPDX-License-Identifier: AGPL-3.0-only
// The relationship feature: the viewer's relationship with an account, the changes of it and the question before the
// ones that ask first (FR-301 to FR-303), which a profile, a follow button and a post's menu share. It sits below the
// features that show posts or profiles, and imports none of them. Routes and other features use it through this file
// only (web-client/README.md A3).
export { type Relationship, refreshRelationship } from "./api/relationship";
export {
  type ConfirmableRelationshipChange,
  RelationshipConfirmDialog,
  type RelationshipConfirmDialogProps,
} from "./components/RelationshipConfirmDialog";
export { type RelationshipState, useRelationship } from "./hooks/useRelationship";
