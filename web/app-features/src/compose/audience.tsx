// SPDX-License-Identifier: AGPL-3.0-only

// The audience tool of a row that sends what is written, which the post composer and the article editor share
// (ADR-0155): a glyph of the visibility that opens the audience menu (the one menu of the design system). The menu holds a
// row for who can see it, which opens the visibilities the writing may have (an article is never direct, ADR-0160), and
// one for who may reply, boost and quote, whose submenu has a row per kind with who may do it now, each opening its
// audiences and, under them, whether others may ask for the author's approval.

import type { Audience, Interaction, InteractionChoice, Visibility } from "@my/app-data";
import { MenuButton, MenuGroup, MenuItem, type IconName } from "@my/design-system";
import type { stringsIn } from "../locale";
import type { MessageId } from "../messages.gen";
import { interactions } from "./model";

type T = ReturnType<typeof stringsIn>["t"];

/** Who may reply, boost and quote: an audience at once, and whether others may ask for approval. */
type Policy = Readonly<Record<Interaction, InteractionChoice>>;

/** The glyph of each visibility, the audience tool's and its item's. */
export const visibilityIcon: Record<Visibility, IconName> = {
  public: "public",
  local: "local",
  unlisted: "unlisted",
  private: "followers",
  direct: "direct",
};
const audienceIcon: Record<Audience, IconName> = {
  everyone: "public",
  followers: "followers",
  mentioned: "direct",
  nobody: "profile",
};
const audiences: readonly Audience[] = ["everyone", "followers", "mentioned", "nobody"];
const interactionIcon: Record<Interaction, IconName> = { reply: "reply", announce: "boost", quote: "quote" };

/**
 * A panel of the audience menu: its first, the visibilities' submenu, the permissions' submenu, or the submenu of one
 * kind of interaction.
 */
export type AudiencePanel = "audience" | "visibility" | "permissions" | Interaction;

/** Who may do it, as the value of its row: the audience, and that others may ask, which anyone needs not. */
const choiceText = (choice: InteractionChoice, t: T) =>
  t("compose.permission.now", {
    audience: t(`compose.audience.${choice.audience}`),
    approval: choice.approval && choice.audience !== "everyone" ? "yes" : "no",
  });

/** Who may reply, boost and quote, as the value of their row: one value for all three, or none where they differ. */
function permissionsText(policy: Policy, t: T): string {
  const [first, ...rest] = interactions.map((i) => choiceText(policy[i], t));
  return first !== undefined && rest.every((text) => text === first) ? first : "";
}

export type AudienceProps<V extends Visibility> = {
  /** Who can see it now. */
  visibility: V;
  policy: Policy;
  /** The visibilities it may have, from the widest. */
  visibilities: readonly V[];
  /** A visibility, or who may reply, boost and quote, chosen. */
  onChange: (change: { visibility: V } | { policy: Policy }) => void;
  /** This server's domain, which a local visibility names. */
  domain: string;
  /** The visibilities that cannot be chosen now, and why, said over them (a quote of a remote post). */
  unavailable?: { why: string; of: (v: V) => boolean } | undefined;
  t: T;
  /** The panel open, none while the menu is closed. */
  open: AudiencePanel | undefined;
  setOpen: (o: AudiencePanel | undefined) => void;
};

/**
 * The items of the audience menu's panel: its first, a row for who can see it and one for who can reply, boost and
 * quote it, each with what it holds now; the visibilities, the chosen one checked, a choice closing the menu; the
 * permissions, a row per kind of interaction with who may do it now; and a kind's own, its audiences and, under them
 * once the audience is not anyone, whether others may ask for approval, which stay open as they go together.
 */
function AudienceItems<V extends Visibility>({
  visibility,
  policy,
  visibilities,
  onChange,
  domain,
  unavailable,
  t,
  panel,
  setOpen,
}: Omit<AudienceProps<V>, "open" | "setOpen"> & { panel: AudiencePanel; setOpen: (o: AudiencePanel) => void }) {
  if (panel === "visibility")
    return (
      <MenuGroup
        title={unavailable?.why}
        chosen={visibility}
        onChoose={(id) => {
          const v = visibilities.find((x) => x === id);
          if (v !== undefined) onChange({ visibility: v });
        }}
      >
        {visibilities.map((v) => (
          <MenuItem
            key={v}
            id={v}
            icon={visibilityIcon[v]}
            about={t(`compose.visibility.${v}.about` as MessageId, { server: domain })}
            disabled={unavailable?.of(v) === true}
          >
            {t(`compose.visibility.${v}`)}
          </MenuItem>
        ))}
      </MenuGroup>
    );
  if (panel === "permissions")
    return (
      <MenuGroup>
        {interactions.map((i) => (
          <MenuItem
            key={i}
            id={i}
            icon={interactionIcon[i]}
            opens={choiceText(policy[i], t)}
            onAction={() => {
              setOpen(i);
            }}
          >
            {t(`compose.interaction.${i}`)}
          </MenuItem>
        ))}
      </MenuGroup>
    );
  if (panel !== "audience") {
    const choice = policy[panel];
    const change = (next: InteractionChoice) => {
      onChange({ policy: { ...policy, [panel]: next } });
    };
    return (
      <>
        <MenuGroup
          chosen={choice.audience}
          stay
          onChoose={(id) => {
            const audience = audiences.find((a) => a === id);
            if (audience !== undefined) change({ ...choice, audience });
          }}
        >
          {audiences.map((a) => (
            <MenuItem key={a} id={a} icon={audienceIcon[a]}>
              {t(`compose.audience.${a}`)}
            </MenuItem>
          ))}
        </MenuGroup>
        {choice.audience !== "everyone" && (
          <MenuGroup
            checked={choice.approval ? ["approval"] : []}
            stay
            onCheck={(_, approval) => {
              change({ ...choice, approval });
            }}
          >
            <MenuItem id="approval" icon="included">
              {t("compose.approval")}
            </MenuItem>
          </MenuGroup>
        )}
      </>
    );
  }
  return (
    <MenuGroup>
      <MenuItem
        id="visibility"
        icon={visibilityIcon[visibility]}
        opens={t(`compose.visibility.${visibility}`)}
        onAction={() => {
          setOpen("visibility");
        }}
      >
        {t("compose.audience")}
      </MenuItem>
      <MenuItem
        id="permissions"
        icon="quote"
        opens={permissionsText(policy, t)}
        onAction={() => {
          setOpen("permissions");
        }}
      >
        {t("compose.permissions")}
      </MenuItem>
    </MenuGroup>
  );
}

/** The submenu a panel of the audience menu is, by its name and the panel back opens; none for its first. */
function audienceSub(panel: AudiencePanel, t: T, setOpen: (o: AudiencePanel) => void) {
  if (panel === "audience") return undefined;
  const [title, parent]: [string, AudiencePanel] =
    panel === "visibility"
      ? [t("compose.audience"), "audience"]
      : panel === "permissions"
        ? [t("compose.permissions"), "audience"]
        : [t(`compose.interaction.${panel}`), "permissions"];
  return {
    title,
    onBack: () => {
      setOpen(parent);
    },
  };
}

/**
 * The audience tool and its menu: who can see it, and who can reply, boost and quote it, each opening its submenu. The
 * tool's glyph is the visibility's, and its name says too whether replies, boosts or quotes are limited.
 */
export function AudienceTool<V extends Visibility>({ open, setOpen, ...props }: AudienceProps<V>) {
  const { visibility, policy, t } = props;
  const limited = interactions.some((i) => policy[i].audience !== "everyone");
  const panel = open ?? "audience";
  return (
    <MenuButton
      icon={visibilityIcon[visibility]}
      label={t("compose.audience.tool", { visibility, limited: limited ? "yes" : "no" })}
      menuLabel={t("compose.audience_menu")}
      align="start"
      open={open !== undefined}
      onOpenChange={(o) => {
        setOpen(o ? "audience" : undefined);
      }}
      sub={audienceSub(panel, t, setOpen)}
    >
      <AudienceItems {...props} panel={panel} setOpen={setOpen} />
    </MenuButton>
  );
}
