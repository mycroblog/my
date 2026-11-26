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

/** A participant's tile's name: theirs first, then their role and marks after commas. */
const named = (name: string) => new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(?:, .*)?$`);

/** The form that opens a voice chat room, at /voice/new. */
export class VoiceRoomOpen {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  async open(): Promise<void> {
    await this.page.goto("/voice/new");
  }

  get name(): Locator {
    return this.page.getByRole("textbox", { name: "Name", exact: true });
  }

  /** Whether the start post goes out with the room. */
  get startPost(): Locator {
    return this.page.getByRole("checkbox", { name: "Post to this server that the room is open", exact: true });
  }

  get postText(): Locator {
    return this.page.getByRole("textbox", { name: "Text of the post", exact: true });
  }

  /** How many people the room holds at most; empty for the server's maximum. */
  get maxParticipants(): Locator {
    return this.page.getByRole("textbox", { name: "How many can be in the room", exact: true });
  }

  get submit(): Locator {
    return this.page.getByRole("button", { name: "Open and join", exact: true });
  }
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

  /** Raise hand, a listener's control in a room where they must ask to speak. */
  get raiseHand(): Locator {
    return this.controls.getByRole("button", { name: "Raise hand", exact: true });
  }

  /** The room's title, whose name ends with how many of how many are in it ("2 of 2 in the room"). */
  title(name: string, count: number, max: number): Locator {
    const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const pattern = new RegExp(`^${escaped}.*${String(count)} of ${String(max)} in the room$`);
    return this.page.getByRole("main").getByRole("heading", { level: 2, name: pattern });
  }

  /** The paragraphs of the room's page that say `sentence`. */
  saying(sentence: string): Locator {
    return this.page.getByRole("main").getByRole("paragraph").filter({ hasText: sentence });
  }

  /** The room's Leave. */
  get leave(): Locator {
    return this.controls.getByRole("button", { name: "Leave", exact: true });
  }

  /**
   * The tile of the participant named `name`, a button for a host or co-host who may act on them, named by its name
   * first, then their role and marks ("Fresh 1a2b, Co-host, Hand raised").
   */
  tile(name: string): Locator {
    return this.page.getByRole("main").getByRole("button", { name: named(name) });
  }

  /** The participant's tile as anyone sees it: a button for a host, else an image, named alike. */
  participant(name: string): Locator {
    const main = this.page.getByRole("main");
    return main.getByRole("button", { name: named(name) }).or(main.getByRole("img", { name: named(name) }));
  }

  /** The item `item` of the menu a host opens on the participant named `name`. */
  menuItem(name: string, item: string): Locator {
    return this.page
      .getByRole("menu", { name: `Actions for ${name}`, exact: true })
      .getByRole("menuitem", { name: item, exact: true });
  }

  /** The button `name` of the dialog open over the page. */
  dialogButton(name: string): Locator {
    return this.page.getByRole("alertdialog").getByRole("button", { name, exact: true });
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
