// SPDX-License-Identifier: AGPL-3.0-only

// The server data of the web client (ADR-0025 §2, ADR-0028 §6, §7, §18): the client API through
// @mycroblog/mykit, and TanStack Query as the one cache of what it answers. Features read and write the server only
// through the hooks and queries here, and never copy what they return into state or context.

export { createAppData, createQueryClient, type AppData } from "./app-data";
export { receiveHomeGap, receiveHomeStatuses, useHomeArrivals, type Arrival } from "./arrivals";
export { AppDataProvider } from "./provider";
export { openSession } from "./session";
export { updateCachedStatuses, type Status, type StatusPage } from "./statuses";
export { homeTimelineQuery, useHomeTimeline, type HomeTimeline } from "./timeline";
