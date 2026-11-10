// SPDX-License-Identifier: AGPL-3.0-only
// The followed hashtags page below its heading (web-client/screens/settings-followed-tags.md): the list-and-add part
// (`ListSection`) with the followed hashtags, the latest first, each a link to its page with the button that unfollows
// it, and the button that opens the form that follows one. The form follows the browser's constraint validation (React
// Aria's native validation); a follow is undone as easily, so neither asks first. What following did, and a failed
// unfollow, show above the list; unfollowing is announced and moves focus to the next row
// (web-client/16-look-states-and-wording.md G24).
import { Button, Cluster, FormActionBar, Hint, Link, ListSection, Stack, Text, TextField } from "@my/ui";
import { useRef } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { tagPath } from "../../../lib/paths";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type FollowedHashtag, hashtagLimit, maxFollowedTags } from "../api/tags";
import {
  type FollowTagForm,
  type UnfollowTag,
  useFollowedTagList,
  useFollowTagForm,
  useUnfollowTag,
} from "../hooks/useFollowedTags";

export function FollowedTags() {
  const t = useT();
  const list = useFollowedTagList();
  const form = useFollowTagForm();
  const unfollow = useUnfollowTag();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((tag) => tag.name),
  );
  const count = list.items.length;
  let result: { tone: "success" | "danger"; message: string } | undefined;
  if (unfollow.error !== undefined) result = { tone: "danger", message: unfollow.error };
  else if (form.status !== undefined) result = { tone: "success", message: form.status };
  const addLabel = t("client:tag.followed_list.new.title");
  return (
    <ListSection
      title={t("client:tag.followed_list.list.title")}
      description={t("client:tag.followed_list.description")}
      items={list.items}
      itemKey={(tag) => tag.name}
      renderItem={(tag) => (
        <FollowedTagRow
          tag={tag}
          unfollow={unfollow}
          onUnfollowed={() => {
            removed(tag.name);
          }}
        />
      )}
      listRef={listRef}
      countLabel={t("client:tag.followed_list.count", { count, max: String(maxFollowedTags) })}
      limit={maxFollowedTags}
      empty={{
        heading: t("client:tag.followed_list.empty"),
        description: t("client:tag.followed_list.empty.description"),
      }}
      add={
        list.isFull
          ? { unavailable: <Hint>{t("client:tag.follow.too_many", { max: String(maxFollowedTags) })}</Hint> }
          : { label: addLabel, panel: (close) => <FollowTagFormFields form={form} close={close} /> }
      }
      result={result}
    />
  );
}

/** The form that follows a hashtag, in the panel of the list. */
function FollowTagFormFields({ form, close }: { form: FollowTagForm; close: () => void }) {
  const t = useT();
  return (
    <Stack
      as="form"
      onSubmit={(event) => {
        form.submit(event, close);
      }}
      gap="4"
    >
      <TextField
        label={t("client:tag.name")}
        description={t("client:tag.name.description")}
        value={form.tag}
        onChange={form.setTag}
        isRequired
        // One more for the `#` the user may type.
        maxLength={hashtagLimit + 1}
        validate={form.validate}
        autoComplete="off"
        autoCorrect="off"
        spellCheck="false"
        errorMessage={(validation) =>
          validation.validationDetails.valueMissing
            ? t("client:tag.name.required")
            : (validation.validationErrors[0] ?? t("client:tag.name.invalid"))
        }
      />
      <FormActionBar result={form.error === undefined ? undefined : { tone: "danger", message: form.error }}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:tag.followed_list.follow")}
        </Button>
        <Button onPress={close}>{t("client:settings.list.cancel")}</Button>
      </FormActionBar>
    </Stack>
  );
}

type FollowedTagRowProps = { tag: FollowedHashtag; unfollow: UnfollowTag; onUnfollowed: () => void };

/** A followed hashtag: a link to its page and when it was followed, with the button that unfollows it. */
function FollowedTagRow({ tag, unfollow, onUnfollowed }: FollowedTagRowProps) {
  const t = useT();
  const intl = useIntl();
  const label = t("client:tag.followed_list.unfollow");
  const name = t("client:tag.title", { tag: tag.name });
  return (
    <Stack gap="3" rowAt="md">
      <Stack as="p" shrink grow>
        <Link href={tagPath(tag.name)} variant="standalone" wrap="anywhere">
          {/* The hashtag is the user's text, in any script (web-client/README.md Y10). */}
          <Text dir="auto" weight="semibold">
            {name}
          </Text>
        </Link>
        <Hint as="span">
          {t("client:tag.followed_list.since", {
            date: intl.formatDate(tag.followed_at, { dateStyle: "medium" }),
          })}
        </Hint>
      </Stack>
      <Cluster gap="2">
        <Button
          size="sm"
          isPending={unfollow.pendingName === tag.name}
          aria-label={t("client:settings.list.action", { action: label, name })}
          onPress={() => {
            unfollow.run(tag.name, onUnfollowed);
          }}
        >
          {label}
        </Button>
      </Cluster>
    </Stack>
  );
}
