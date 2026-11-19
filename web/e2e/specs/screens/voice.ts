// SPDX-License-Identifier: AGPL-3.0-only

// A voice chat room's page and the call's bar: Join, the room's controls (the microphone, Leave), and the bar of the
// call that the frame keeps on every screen. And the sound a page receives: an init script keeps each
// RTCPeerConnection the page makes, so that a spec reads how many bytes of audio arrived (getStats), as the browser
// counts them, whatever the app does with them.

import type { Locator, Page } from "@playwright/test";

/** The page keeps each RTCPeerConnection it makes, for `receivedAudio`: give it to `page.addInitScript` first. */
export function keepPeerConnections(): void {
  const Native = window.RTCPeerConnection;
  const kept: RTCPeerConnection[] = [];
  (window as unknown as { __peers: RTCPeerConnection[] }).__peers = kept;
  window.RTCPeerConnection = class extends Native {
    constructor(configuration?: RTCConfiguration) {
      super(configuration);
      kept.push(this);
    }
  };
}

/** How many bytes of audio the page's peer connections that are open have received, together. */
export function receivedAudio(page: Page): Promise<number> {
  return page.evaluate(async () => {
    const peers = (window as unknown as { __peers?: RTCPeerConnection[] }).__peers ?? [];
    let bytes = 0;
    for (const peer of peers) {
      if (peer.connectionState === "closed") continue;
      const stats = await peer.getStats();
      stats.forEach((s: { type: string; kind?: string; bytesReceived?: number }) => {
        if (s.type === "inbound-rtp" && s.kind === "audio") bytes += s.bytesReceived ?? 0;
      });
    }
    return bytes;
  });
}

/** How many of the page's peer connections are connected. */
export function connectedPeers(page: Page): Promise<number> {
  return page.evaluate(
    () =>
      ((window as unknown as { __peers?: RTCPeerConnection[] }).__peers ?? []).filter(
        (p) => p.connectionState === "connected",
      ).length,
  );
}

/** How many of the page's peer connections are not closed. */
export function openPeers(page: Page): Promise<number> {
  return page.evaluate(
    () =>
      ((window as unknown as { __peers?: RTCPeerConnection[] }).__peers ?? []).filter(
        (p) => p.connectionState !== "closed",
      ).length,
  );
}

export class VoiceRoom {
  readonly page: Page;
  readonly id: string;

  constructor(page: Page, id: string) {
    this.page = page;
    this.id = id;
  }

  async open(): Promise<void> {
    await this.page.goto(`/voice/${this.id}`);
  }

  get join(): Locator {
    return this.page.getByRole("main").getByRole("button", { name: "Join", exact: true });
  }

  /** The room's toolbar of controls, while the reader is in it. */
  get controls(): Locator {
    return this.page.getByRole("toolbar", { name: "Room controls", exact: true });
  }

  /** The room's microphone control, named by what a press does now. */
  get unmute(): Locator {
    return this.controls.getByRole("button", { name: "Unmute", exact: true });
  }

  get mute(): Locator {
    return this.controls.getByRole("button", { name: "Mute", exact: true });
  }

  /** Speak, which a listener of a room where everyone may speak can press once the call is live. */
  get speak(): Locator {
    return this.controls.getByRole("button", { name: "Speak", exact: true });
  }

  /** The line that says the call is connecting. */
  get connecting(): Locator {
    return this.page.getByRole("main").getByText("Connecting…", { exact: true });
  }

  /** The bar of the call, a group named by its room. */
  callBar(room: string): Locator {
    return this.page.getByRole("group", { name: room, exact: true });
  }

  /** The bar's Leave. */
  leaveFromBar(room: string): Locator {
    return this.callBar(room).getByRole("button", { name: "Leave", exact: true });
  }
}
