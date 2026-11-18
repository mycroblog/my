// SPDX-License-Identifier: AGPL-3.0-only

// The controls of a settings screen that saves at once (ADR-0110 §5): a setting on or off, and a choice from a list.
// Each shows what the reader chose the moment they choose it, whether or not what holds the value shows it ahead of
// the server's answer (the preferences do, the profile does not), says where the change is in the line under its name
// (saving.tsx), and goes back to what the server has when the server does not take it; each gives its name and its
// page's address (`page`), by which a change that fails once its control is gone is still told. One the server decides
// (`fixed`) cannot be changed, and the line under it says why. And the notice that stands in a screen's place until
// the server answered what it shows.

import {
  Button,
  FeedNotice,
  Select,
  SettingRow,
  Switch,
  useLocale,
  type SelectGroup,
  type SelectOption,
} from "@my/design-system";
import { useId, useState } from "react";
import { stringsIn } from "../locale";
import { useSaving } from "./saving";

/**
 * What the reader chose of the setting named `setting`, on the settings page at `page`, while it is on its way to the
 * server, shown in place of `held`, what the server has; and `choose`, which shows `value` and saves it with `write`,
 * again when the reader tries again.
 */
function useChoice<V>(setting: string, page: string, held: V, write: (value: V) => Promise<unknown>) {
  const [chosen, setChosen] = useState<{ value: V } | undefined>(undefined);
  const { state, save } = useSaving(setting, page);
  const choose = (value: V) => {
    save(() => {
      const own = { value };
      setChosen(own);
      // Once the server answered, what it has shows: the choice, or what was there before it.
      return write(value).finally(() => {
        setChosen((now) => (now === own ? undefined : now));
      });
    });
  };
  return { shown: chosen === undefined ? held : chosen.value, state, choose };
}

/** A setting on or off that saves at once; `fixed`, it cannot be changed, and the line under it says why. */
export function Toggle({
  page,
  label,
  about,
  on,
  fixed,
  onChange,
}: {
  /** The address of the settings page it is on (/settings/display). */
  page: string;
  label: string;
  about?: string | undefined;
  /** Its state as the server has it; of a fixed one, what the server enforces. */
  on: boolean;
  /** Why the reader cannot change it (the server decides it, or this device cannot do it). */
  fixed?: string | undefined;
  /** Saves the state chosen. */
  onChange: (on: boolean) => Promise<unknown>;
}) {
  const { shown, state, choose } = useChoice(label, page, on, onChange);
  return (
    <Switch
      label={label}
      description={about}
      isSelected={shown}
      isDisabled={fixed !== undefined}
      reason={fixed}
      onChange={choose}
      save={state}
    />
  );
}

/** A choice from a list that saves at once: its name over the line about it, the picker at the row's end. */
export function Choice({
  page,
  label,
  about,
  options,
  value,
  onChoose,
}: {
  /** The address of the settings page it is on (/settings/display). */
  page: string;
  label: string;
  about?: string | undefined;
  options: readonly (SelectOption | SelectGroup)[];
  /** Its value as the server has it. */
  value: string;
  /** Saves the value chosen. */
  onChoose: (value: string) => Promise<unknown>;
}) {
  const id = useId();
  const { shown, state, choose } = useChoice(label, page, value, onChoose);
  return (
    <SettingRow id={id} label={label} about={about} save={state}>
      <Select options={options} value={shown} onChange={choose} />
    </SettingRow>
  );
}

/**
 * What a settings screen says in its sections' place (`notice` of a `SettingsScreen`) until what it shows is read, as
 * the screen's text reads, at its start: that it loads, or that it could not be read, with Try again after it, a
 * button whose target is a finger's on a touch screen.
 */
export function ReadingNotice({ failed, onRetry }: { failed: boolean; onRetry: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  return failed ? (
    <FeedNotice
      alert
      lead
      action={
        <Button size="timeline" touch onPress={onRetry}>
          {t("settings.retry")}
        </Button>
      }
    >
      {t("settings.failed")}
    </FeedNotice>
  ) : (
    <FeedNotice lead>{t("settings.loading")}</FeedNotice>
  );
}
