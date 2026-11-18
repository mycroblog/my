// SPDX-License-Identifier: AGPL-3.0-only

// The controls of a settings screen that saves at once (ADR-0110 §5), over a value the server holds and answers
// (the profile's): a setting on or off, and a choice from a list. Each shows what the reader chose the moment they
// choose it, says where the change is in the line under its name (saving.ts), and goes back to what the server has
// when the server does not take it. One the reader cannot change now is disabled, with the reason in that line. And
// what stands in a screen's place until the server answered what it shows.

import {
  FeedNotice,
  Select,
  SettingRow,
  SettingsScreen,
  Stack,
  Switch,
  TextButton,
  useLocale,
  type IconName,
  type SelectOption,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { useSaving } from "./saving";

/** What a setting takes of a choice the reader cannot make now. */
type Off = {
  /** It cannot be changed now (the server decides it for every account). */
  isDisabled?: boolean | undefined;
  /** Why, in place of the line about it. */
  reason?: string | undefined;
};

/**
 * What the reader chose while it is on its way to the server, shown in place of `held`, what the server has; and
 * `choose`, which shows `value` and saves it with `write`, again when the reader tries again.
 */
function useChoice<V>(held: V, write: (value: V) => Promise<unknown>) {
  const [chosen, setChosen] = useState<{ value: V } | undefined>(undefined);
  const { state, save } = useSaving();
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

/** A setting on or off that saves at once. */
export function Toggle({
  label,
  about,
  on,
  onChange,
  isDisabled,
  reason,
}: {
  label: string;
  about?: string | undefined;
  /** Its state as the server has it. */
  on: boolean;
  /** Saves the state chosen. */
  onChange: (on: boolean) => Promise<unknown>;
} & Off) {
  const { shown, state, choose } = useChoice(on, onChange);
  return (
    <Switch
      label={label}
      description={about}
      isSelected={shown}
      onChange={choose}
      isDisabled={isDisabled}
      reason={reason}
      save={state}
    />
  );
}

/** A choice from a list that saves at once: its name over the line about it, the picker at the row's end. */
export function Choice<V extends string>({
  label,
  about,
  options,
  value,
  onChoose,
  isDisabled,
  reason,
}: {
  label: string;
  about?: string | undefined;
  options: readonly (SelectOption & { value: V })[];
  /** Its value as the server has it. */
  value: V;
  /** Saves the value chosen. */
  onChoose: (value: V) => Promise<unknown>;
} & Off) {
  const id = useId();
  const { shown, state, choose } = useChoice(value, onChoose);
  return (
    <SettingRow id={id} label={label} about={about} isDisabled={isDisabled} reason={reason} save={state}>
      <Select
        labelledBy={id}
        options={options}
        value={shown}
        onChange={(picked) => {
          const option = options.find((o) => o.value === picked);
          if (option !== undefined) choose(option.value);
        }}
      />
    </SettingRow>
  );
}

/**
 * A settings screen whose server data is not read yet: its head, as the screen will have it, over a line that says it
 * loads, or that it failed, with the way to try again.
 */
export function SettingsReading({
  icon,
  title,
  back,
  sub = false,
  failed,
  onRetry,
}: {
  icon?: IconName | undefined;
  title: string;
  back: ReactNode;
  sub?: boolean | undefined;
  failed: boolean;
  onRetry: () => void;
}) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <Stack gap="shell-edge">
      <SettingsScreen icon={icon} title={title} back={back} sub={sub}>
        {null}
      </SettingsScreen>
      {failed ? (
        <FeedNotice alert action={<TextButton onPress={onRetry}>{t("settings.retry")}</TextButton>}>
          {t("settings.failed")}
        </FeedNotice>
      ) : (
        <FeedNotice>{t("settings.loading")}</FeedNotice>
      )}
    </Stack>
  );
}
