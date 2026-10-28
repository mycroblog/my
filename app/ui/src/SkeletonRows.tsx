// SPDX-License-Identifier: AGPL-3.0-only
// Skeletons in the shape of the rows a list loads (web-client/16-look-states-and-wording.md G15): a post, a row of an account list, a
// notification, an open voice room's card, a card of facts (a heading over terms and values), and a room's stage. Each one is hidden from assistive technology as a whole. The region they fill sets
// aria-busy="true" and carries the one loading label (a VisuallyHidden text or a role="status" Spinner), however
// many rows it repeats.
import type { ComponentPropsWithRef } from "react";
import { tv } from "tailwind-variants";
import { Card } from "./Card";
import { Skeleton } from "./Skeleton";

const rows = tv({
  slots: {
    // PostCard's padding, border and grid: the avatar beside the header, the body and the actions below.
    post: "@container border-b border-border px-4 py-3",
    postGrid: "grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 gap-y-2 text-base",
    postHeader: "flex flex-col gap-1 self-center",
    postBody: "col-span-2 flex flex-col gap-2 @md:col-span-1 @md:col-start-2",
    postActions: "col-span-2 flex gap-2 @md:col-span-1 @md:col-start-2",
    // A row of an account list: the avatar beside the name and the handle. Account lists separate rows with
    // divide-y, so the row has no border of its own.
    account: "grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 px-4 py-3 text-base",
    accountLines: "flex flex-col gap-1",
    // A notification is a PostCard: the avatar beside the sentence and the time, then the quoted post.
    notification: "@container border-b border-border px-4 py-3",
    notificationGrid: "grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 gap-y-2 text-base",
    notificationLines: "flex flex-col gap-1 self-center",
    notificationBody: "col-span-2 flex flex-col gap-2 @md:col-span-1 @md:col-start-2",
    // VoiceRoomCard's card form: the pill and the stats, the name, the speakers beside the host, the description.
    room: "flex flex-col gap-3",
    roomState: "flex items-center gap-2",
    roomPeople: "flex items-center gap-2",
    // A card of facts: a heading over a row of terms and their values (DescriptionList).
    facts: "@container flex flex-col gap-3",
    factsRow: "grid grid-cols-1 gap-3 @md:grid-cols-3",
    factsItem: "flex flex-col gap-1",
    // A room's stage (the room page): its head, then the speakers' large avatars in a row.
    stage: "flex flex-col gap-6",
    stageHead: "flex flex-col gap-3",
    stageAvatars: "flex flex-wrap gap-4",
  },
});

export type SkeletonRowProps = Omit<ComponentPropsWithRef<"div">, "children">;

/** A placeholder in the shape of a PostCard. It is a size container, as PostCard is. */
export function PostSkeleton({ className, ...props }: SkeletonRowProps) {
  const slots = rows();
  return (
    <div aria-hidden="true" {...props} className={slots.post({ className })}>
      <div className={slots.postGrid()}>
        <Skeleton shape="circle" />
        <div className={slots.postHeader()}>
          <Skeleton className="w-1/3" />
          <Skeleton className="h-[0.875em] w-1/4" />
        </div>
        <div className={slots.postBody()}>
          <Skeleton />
          <Skeleton />
          <Skeleton className="w-2/3" />
        </div>
        <div className={slots.postActions()}>
          {(["reply", "boost", "favourite", "more"] as const).map((action) => (
            <Skeleton key={action} shape="rect" className="h-8 w-12" />
          ))}
        </div>
      </div>
    </div>
  );
}

/** A placeholder in the shape of a row of an account list (followers, search results). */
export function AccountRowSkeleton({ className, ...props }: SkeletonRowProps) {
  const slots = rows();
  return (
    <div aria-hidden="true" {...props} className={slots.account({ className })}>
      <Skeleton shape="circle" />
      <div className={slots.accountLines()}>
        <Skeleton className="w-2/5" />
        <Skeleton className="h-[0.875em] w-1/3" />
      </div>
    </div>
  );
}

/** A placeholder in the shape of a notification: who did what and when, then the post it is about. */
export function NotificationRowSkeleton({ className, ...props }: SkeletonRowProps) {
  const slots = rows();
  return (
    <div aria-hidden="true" {...props} className={slots.notification({ className })}>
      <div className={slots.notificationGrid()}>
        <Skeleton shape="circle" />
        <div className={slots.notificationLines()}>
          <Skeleton className="w-3/5" />
          <Skeleton className="h-[0.875em] w-1/5" />
        </div>
        <div className={slots.notificationBody()}>
          <Skeleton />
          <Skeleton className="w-1/2" />
        </div>
      </div>
    </div>
  );
}

/** A placeholder in the shape of an open voice room's card on a page's list (VoiceRoomCard, `card` surface). */
export function VoiceRoomCardSkeleton(props: SkeletonRowProps) {
  const slots = rows();
  return (
    <div aria-hidden="true" {...props}>
      <Card padding="4">
        <div className={slots.room()}>
          <div className={slots.roomState()}>
            <Skeleton shape="rect" className="h-6 w-14 rounded-full" />
            <Skeleton className="h-[0.875em] w-1/4" />
          </div>
          <Skeleton className="h-[1.25em] w-1/2" />
          <div className={slots.roomPeople()}>
            <Skeleton shape="circle" className="size-8" />
            <Skeleton className="h-[0.875em] w-1/3" />
          </div>
          <Skeleton className="h-[0.875em] w-3/4" />
        </div>
      </Card>
    </div>
  );
}

/** A placeholder in the shape of a card of facts: a heading over three terms with their values. */
export function FactsCardSkeleton(props: SkeletonRowProps) {
  const slots = rows();
  return (
    <div aria-hidden="true" {...props}>
      <Card padding="4">
        <div className={slots.facts()}>
          <Skeleton className="h-[1.25em] w-2/5" />
          <div className={slots.factsRow()}>
            {(["first", "second", "third"] as const).map((key) => (
              <div key={key} className={slots.factsItem()}>
                <Skeleton className="h-[0.875em] w-1/3" />
                <Skeleton className="w-2/3" />
              </div>
            ))}
          </div>
        </div>
      </Card>
    </div>
  );
}

/** A placeholder in the shape of a voice room's stage: the head (the pill, the name, the description) and the speakers. */
export function StageSkeleton(props: SkeletonRowProps) {
  const slots = rows();
  return (
    <div aria-hidden="true" {...props}>
      <Card padding="4">
        <div className={slots.stage()}>
          <div className={slots.stageHead()}>
            <div className={slots.roomState()}>
              <Skeleton shape="rect" className="h-6 w-14 rounded-full" />
              <Skeleton className="h-[0.875em] w-1/4" />
            </div>
            <Skeleton className="h-[1.5em] w-1/2" />
            <Skeleton className="h-[0.875em] w-3/4" />
          </div>
          <div className={slots.stageAvatars()}>
            {(["first", "second", "third"] as const).map((key) => (
              <Skeleton key={key} shape="circle" className="size-16" />
            ))}
          </div>
        </div>
      </Card>
    </div>
  );
}
