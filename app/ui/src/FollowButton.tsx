// SPDX-License-Identifier: AGPL-3.0-only
// The follow button of a profile and of a row of accounts (ADR-0110 P1–P3): where the viewer's follow stands, in its
// look. Not followed, it is the one main action, filled in the accent; once followed or requested it turns quiet, an
// outline with the state's icon, as pressing it again undoes the follow. The change of fill is a color transition of
// `duration-fast` (quiet, web-client/21-experience-principles.md §21.2); the icon and the label say the state without
// it. While the relationship loads, a placeholder of the button's size keeps its place, so nothing moves when it
// arrives.
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { BlockIcon, CheckIcon, PlusIcon, WaitingIcon } from "./Icons";
import { Skeleton } from "./Skeleton";

/** Where the viewer's follow of the account stands; `loading` until the relationship is known. */
export type FollowStatus = "none" | "requested" | "following" | "blocking" | "loading";

const placeholder = tv({
  base: "inline-block shrink-0 rounded-full pointer-coarse:min-h-11",
  variants: {
    size: {
      sm: "h-8 w-24",
      md: "h-10 w-28",
    },
  },
  defaultVariants: { size: "md" },
});

export type FollowButtonProps = {
  status: FollowStatus;
  /** The button's words for the status ("Follow", "Follow back", "Following"); unused while `loading`. */
  label: string;
  onPress?: () => void;
  /** While the change is on its way: the button ignores presses and keeps its focus. */
  isPending?: boolean;
  /** `md` on a profile; `sm` in a row of a list. */
  size?: "sm" | "md";
  /** An element that says more about the state (the server a request waits for). */
  "aria-describedby"?: string | undefined;
};

function StatusIcon({ status }: { status: Exclude<FollowStatus, "loading"> }) {
  switch (status) {
    case "none":
      return <PlusIcon />;
    case "requested":
      return <WaitingIcon />;
    case "following":
      return <CheckIcon />;
    case "blocking":
      return <BlockIcon />;
  }
}

/**
 * One button for every status, so that it keeps the focus when the status changes. Pressing it asks for the change the
 * status allows (follow, or undo); the caller asks first where undoing needs it.
 */
export function FollowButton({ status, label, onPress, isPending, size = "md", ...props }: FollowButtonProps) {
  if (status === "loading") return <Skeleton data-follow-placeholder="" className={placeholder({ size })} />;
  const describedBy = props["aria-describedby"];
  return (
    <Button
      variant={status === "none" ? "primary" : "outline"}
      size={size}
      isPending={isPending ?? false}
      data-follow-status={status}
      className="motion-safe:duration-fast motion-safe:ease-standard"
      {...(describedBy === undefined ? {} : { "aria-describedby": describedBy })}
      {...(onPress === undefined ? {} : { onPress })}
    >
      <StatusIcon status={status} />
      {label}
    </Button>
  );
}
