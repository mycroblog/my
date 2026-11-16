// SPDX-License-Identifier: AGPL-3.0-only

// Managing the reader's lists (#3124, ADR-0013 §11–§15), in the reader's language (the toolbar's locale; ja, else en):
// the index with New list, its rows plain; a list being made in place at its top, New list giving way while it is, and
// refused at the 50 lists' limit; the index's states (none, loading, failed); a list's own screen, whose menu renames
// it, opens its members and deletes it; renaming in place; the confirmation before deleting; a list's members under a
// back link named by the list, where adding someone the reader follows puts them in the list and its count at once,
// refused at the 1,000 members' limit, and none; and Add to list in a profile's menu, for an account the reader
// follows and one they do not. The words are placeholders of the feature's.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { Button, TextButton } from "./button";
import { ConfirmDialog } from "./dialog";
import { FeedNotice } from "./feed";
import { TextField } from "./form-page";
import { AccountRow, BackButton, ButtonPair, OpenRow, ScreenStack, ScreenTitle, StateText } from "./list";
import { MenuButton, MenuGroup, MenuItem, MenuPanel } from "./menu";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const words = {
  en: {
    lists: "Lists",
    newList: "New list",
    name: "List name",
    create: "Create",
    cancel: "Cancel",
    save: "Save",
    more: "More",
    rename: "Rename",
    members: "Members",
    delete: "Delete list…",
    count: (n: number) => `${n.toLocaleString("en")} ${n === 1 ? "member" : "members"}`,
    empty: "No lists yet. Your lists show here, each with the posts of its members.",
    loading: "Loading your lists…",
    failed: "Your lists could not be loaded.",
    retry: "Try again",
    limitLists: "You can have at most 50 lists. Delete one first.",
    deleteTitle: (name: string) => `Delete the list ${name}?`,
    deleteText: "The list and its timeline go; you keep following its members. This cannot be undone.",
    deleteConfirm: "Delete",
    backLists: "Lists",
    add: "Add an account you follow",
    addButton: "Add",
    added: "Added",
    remove: "Remove",
    membersEmpty: "No one in this list yet. Add accounts you follow to read their posts here.",
    limitMembers: "A list can have at most 1,000 members. Remove one first.",
    addToList: "Add to list",
    addToListAbout: "You can add only accounts you follow",
    profileMore: "More",
    mute: "Mute",
    block: "Block…",
    listNames: ["Photographers", "Friends", "Bread people", "Local news"],
  },
  ja: {
    lists: "リスト",
    newList: "新しいリスト",
    name: "リスト名",
    create: "作成",
    cancel: "キャンセル",
    save: "保存",
    more: "その他",
    rename: "名前を変更",
    members: "メンバー",
    delete: "リストを削除…",
    count: (n: number) => `${String(n)} 人`,
    empty: "まだリストはありません。リストはここに表示され、それぞれのメンバーの投稿を読めます。",
    loading: "リストを読み込んでいます…",
    failed: "リストを読み込めませんでした。",
    retry: "もう一度試す",
    limitLists: "作れるリストは 50 個までです。先にどれかのリストを削除してください。",
    deleteTitle: (name: string) => `リスト「${name}」を削除しますか？`,
    deleteText: "リストとそのタイムラインは消えますが、メンバーのフォローはそのままです。取り消せません。",
    deleteConfirm: "削除",
    backLists: "リスト",
    add: "フォロー中のアカウントを追加",
    addButton: "追加",
    added: "追加済み",
    remove: "外す",
    membersEmpty: "このリストにはまだ誰もいません。フォロー中のアカウントを追加すると、その投稿をここで読めます。",
    limitMembers: "リストのメンバーは 1000 人までです。先にどれかのメンバーを外してください。",
    addToList: "リストに追加",
    addToListAbout: "フォロー中のアカウントだけ追加できます",
    profileMore: "その他",
    mute: "ミュート",
    block: "ブロック…",
    listNames: ["写真家", "友だち", "パン好き", "地元のニュース"],
  },
} as const;

type W = (typeof words)[Lang];

const counts = [12, 4, 27, 3];

type Person = { name: string; handle: string; avatar?: string | undefined };
const people: Person[] = [
  { name: "Mika", handle: "@mika@my.example", avatar: images.dusk },
  { name: "Ren", handle: "@ren@my.example", avatar: images.snow },
  { name: "Sora", handle: "@sora@photos.example", avatar: images.logo },
  { name: "Aoi Tanaka", handle: "@aoi@my.example" },
];
const found: Person[] = [
  { name: "Haruto", handle: "@haruto@photos.example", avatar: images.dusk },
  { name: "Hana", handle: "@hana@my.example" },
];

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), paddingBlock: rem(16) }}>{children}</div>
);

/** A menu shown open, as wide as its items, at the end of the column. */
const OpenMenu = ({ label, children }: { label: string; children: ReactNode }) => (
  <div style={{ display: "flex", justifyContent: "flex-end" }}>
    <MenuPanel label={label}>{children}</MenuPanel>
  </div>
);

/** What may be done with a list from its own screen: rename it, its members, delete it. */
const ListItems = ({ w }: { w: W }) => (
  <>
    <MenuGroup>
      <MenuItem id="rename" icon="edit">
        {w.rename}
      </MenuItem>
      <MenuItem id="members" icon="members">
        {w.members}
      </MenuItem>
    </MenuGroup>
    <MenuGroup>
      <MenuItem id="delete" icon="delete">
        {w.delete}
      </MenuItem>
    </MenuGroup>
  </>
);

/** The index: its title with New list, which gives way while a list is made (`children`, over the rows). */
function Index({ children }: { children?: ReactNode }) {
  const w = words[useLang()];
  const creating = children !== undefined;
  return (
    <ScreenStack>
      <ScreenTitle
        glyph="list"
        actions={
          creating ? undefined : (
            <Button size="timeline" kind="primary">
              {w.newList}
            </Button>
          )
        }
      >
        {w.lists}
      </ScreenTitle>
      {children}
      <div>
        {w.listNames.map((name, i) => (
          <OpenRow key={name} href="#" icon="list" title={name} hint={w.count(counts[i] ?? 0)} />
        ))}
      </div>
    </ScreenStack>
  );
}

/** A list's own screen: back to the index, its title and count, its menu in the title's row. */
function ListScreen({ open = false }: { open?: boolean }) {
  const w = words[useLang()];
  return (
    <ScreenStack>
      <ScreenTitle
        back={<BackButton onPress={() => undefined}>{w.backLists}</BackButton>}
        glyph="list"
        count={w.count(12)}
        actions={
          <MenuButton icon="more" label={w.more}>
            <ListItems w={w} />
          </MenuButton>
        }
      >
        {w.listNames[0]}
      </ScreenTitle>
      {open && (
        <OpenMenu label={w.more}>
          <ListItems w={w} />
        </OpenMenu>
      )}
    </ScreenStack>
  );
}

/**
 * A list being made, in place at the index's top: its name, Cancel and Create. `limit`: refused at 50 lists, said
 * under the field as a field's error, Create dimmed until a list is deleted.
 */
function Create({ limit = false }: { limit?: boolean }) {
  const w = words[useLang()];
  const id = useId();
  return (
    <Column>
      <Index>
        <div style={{ display: "grid", gap: rem(12) }}>
          <TextField
            id={id}
            name="name"
            label={w.name}
            defaultValue={limit ? w.listNames[1] : undefined}
            error={limit ? w.limitLists : undefined}
          />
          <ButtonPair>
            <Button kind="quiet" size="timeline">
              {w.cancel}
            </Button>
            <Button kind="primary" size="timeline" isDisabled={limit}>
              {w.create}
            </Button>
          </ButtonPair>
        </div>
      </Index>
    </Column>
  );
}

/** The index in a state: none, loading, or failed with Try again. */
function IndexState({ state }: { state: "empty" | "loading" | "failed" }) {
  const w = words[useLang()];
  return (
    <Column>
      <ScreenStack>
        <ScreenTitle
          glyph="list"
          actions={
            state === "empty" ? (
              <Button size="timeline" kind="primary">
                {w.newList}
              </Button>
            ) : undefined
          }
        >
          {w.lists}
        </ScreenTitle>
        {state === "empty" ? (
          <FeedNotice start>{w.empty}</FeedNotice>
        ) : state === "loading" ? (
          <FeedNotice start>{w.loading}</FeedNotice>
        ) : (
          <FeedNotice start alert action={<TextButton>{w.retry}</TextButton>}>
            {w.failed}
          </FeedNotice>
        )}
      </ScreenStack>
    </Column>
  );
}

/** Renaming a list in place of its title, under the same back link: the name, Cancel and Save. */
function Rename() {
  const w = words[useLang()];
  const id = useId();
  return (
    <Column>
      <ScreenStack>
        <BackButton onPress={() => undefined}>{w.backLists}</BackButton>
        <div style={{ display: "grid", gap: rem(12) }}>
          <TextField id={id} name="name" label={w.name} defaultValue={w.listNames[0]} />
          <ButtonPair>
            <Button kind="quiet" size="timeline">
              {w.cancel}
            </Button>
            <Button kind="primary" size="timeline">
              {w.save}
            </Button>
          </ButtonPair>
        </div>
      </ScreenStack>
    </Column>
  );
}

/** The confirmation before deleting a list, over the dimmed and blurred page. */
function Delete() {
  const w = words[useLang()];
  return (
    <div>
      <Column>
        <ListScreen />
      </Column>
      <div
        style={{
          position: "fixed",
          inset: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          padding: rem(16),
          background: "var(--dialog-scrim)",
          backdropFilter: "blur(var(--dialog-blur))",
        }}
      >
        <ConfirmDialog
          layer={false}
          title={w.deleteTitle(w.listNames[0])}
          cancel={w.cancel}
          confirm={w.deleteConfirm}
          onCancel={() => undefined}
          onConfirm={() => undefined}
        >
          {w.deleteText}
        </ConfirmDialog>
      </div>
    </div>
  );
}

const avatar = (p: Person) => <Avatar name={p.name} src={p.avatar} size="post" decorative />;

/**
 * A list's members, under a back link named by the list: the title and count, the field that finds someone the reader
 * follows, what it found on a raised surface in the members' rhythm (Add, or Added once in the list), then the members,
 * each with Remove. Adding puts the account in the members and the count at once. `limit`: refused at 1,000 members,
 * said under the field, Add dimmed.
 */
function Members({ state = "normal" }: { state?: "normal" | "searching" | "empty" | "limit" }) {
  const w = words[useLang()];
  const id = useId();
  const searching = state === "searching" || state === "limit";
  // Searching, Hana was just added: at the top of the members, in the count, and Added where she was found.
  const [members, setMembers] = useState<Person[]>(
    state === "empty" ? [] : state === "searching" ? [...found.slice(1), ...people] : people,
  );
  const [extra] = useState(state === "limit" ? 1000 - people.length : 0);
  const inList = (p: Person) => members.some((m) => m.handle === p.handle);
  return (
    <Column>
      <ScreenStack>
        <ScreenTitle
          back={<BackButton onPress={() => undefined}>{w.listNames[0]}</BackButton>}
          glyph="members"
          count={w.count(members.length + extra)}
        >
          {w.members}
        </ScreenTitle>
        <TextField
          id={id}
          name="find"
          label={w.add}
          defaultValue={searching ? "ha" : undefined}
          error={state === "limit" ? w.limitMembers : undefined}
        />
        {searching && (
          <div style={{ padding: rem(12), borderRadius: rem(12), background: "var(--color-surface-raised)" }}>
            <ScreenStack>
              {found.map((p) => (
                <AccountRow
                  key={p.handle}
                  avatar={avatar(p)}
                  name={p.name}
                  handle={p.handle}
                  end={
                    inList(p) ? (
                      <StateText>{w.added}</StateText>
                    ) : (
                      <Button
                        size="timeline"
                        isDisabled={state === "limit"}
                        onPress={() => {
                          setMembers((m) => [p, ...m]);
                        }}
                      >
                        {w.addButton}
                      </Button>
                    )
                  }
                />
              ))}
            </ScreenStack>
          </div>
        )}
        {members.length === 0 ? (
          <FeedNotice start>{w.membersEmpty}</FeedNotice>
        ) : (
          members.map((p) => (
            <AccountRow
              key={p.handle}
              avatar={avatar(p)}
              name={p.name}
              handle={p.handle}
              href="#"
              end={
                <Button size="timeline" kind="quiet">
                  {w.remove}
                </Button>
              }
            />
          ))
        )}
      </ScreenStack>
    </Column>
  );
}

/** Adding someone found: they join the members at the top, the count with them, and their row says Added. */
function MembersAdded() {
  return <Members state="searching" />;
}

const held = [true, false, true, false];

/** The profile's menu with Add to list, which cannot be done for an account the reader does not follow. */
function ProfileMenu({ w, followed }: { w: W; followed: boolean }) {
  return (
    <MenuPanel label={w.profileMore}>
      <MenuGroup title="@sam">
        <MenuItem id="add-to-list" icon="list" disabled={!followed} about={followed ? undefined : w.addToListAbout}>
          {w.addToList}
        </MenuItem>
        <MenuItem id="mute" icon="mute">
          {w.mute}
        </MenuItem>
        <MenuItem id="block" icon="block">
          {w.block}
        </MenuItem>
      </MenuGroup>
    </MenuPanel>
  );
}

/**
 * Add to list, a second menu in place of the profile's, which it was opened from: each list an item, its box filled
 * and checked while it holds the account, then New list (shown here under the first).
 */
function AddToList() {
  const w = words[useLang()];
  return (
    <Column>
      <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: rem(8) }}>
        <ProfileMenu w={w} followed />
        <MenuPanel label={w.addToList}>
          <MenuGroup title={w.addToList}>
            {w.listNames.map((name, i) => (
              <MenuItem key={name} id={`list-${String(i)}`} icon="included" on={held[i] === true}>
                {name}
              </MenuItem>
            ))}
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="new" icon="add">
              {w.newList}
            </MenuItem>
          </MenuGroup>
        </MenuPanel>
      </div>
    </Column>
  );
}

/** The profile's menu of an account the reader does not follow: Add to list muted, saying why. */
function NotFollowed() {
  const w = words[useLang()];
  return (
    <Column>
      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <ProfileMenu w={w} followed={false} />
      </div>
    </Column>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "List management", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Lists: Story = {
  render: () => (
    <Column>
      <Index />
    </Column>
  ),
};
export const ListOpen: Story = {
  render: () => (
    <Column>
      <ListScreen open />
    </Column>
  ),
};
export const ListCreating: Story = { render: () => <Create /> };
export const ListLimit: Story = { render: () => <Create limit /> };
export const ListsEmpty: Story = { render: () => <IndexState state="empty" /> };
export const ListsLoading: Story = { render: () => <IndexState state="loading" /> };
export const ListsFailed: Story = { render: () => <IndexState state="failed" /> };
export const ListRenaming: Story = { render: () => <Rename /> };
export const ListDeleteConfirm: Story = { render: () => <Delete /> };
export const ListMembers: Story = { render: () => <Members /> };
export const ListMembersAdding: Story = { render: () => <MembersAdded /> };
export const ListMembersLimit: Story = { render: () => <Members state="limit" /> };
export const ListMembersEmpty: Story = { render: () => <Members state="empty" /> };
export const AddToListFollowed: Story = { render: () => <AddToList /> };
export const AddToListNotFollowed: Story = { render: () => <NotFollowed /> };
