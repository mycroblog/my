// SPDX-License-Identifier: AGPL-3.0-only

// The brand images, at /admin/settings/branding (ADR-0059 §8–§10): the icon (a square mark: the favicon, the app's
// icons, the server's avatar), the logo (a wordmark that includes the server's name) and the logo for dark
// backgrounds, each an SVG. A file picked is checked by the server at once, which says what it removed from it or why
// it cannot be used, and shows as it will; the icon's opaque pictures (home screens) take the color chosen behind it.
// Save sends each image picked, the browser drawing its PNG pictures from the checked SVG, and brings back the built-in
// image where one was removed: a form saved as a whole, one image after another, what is wrong under the image.

import {
  brandErrorOf,
  refusalDetail,
  useAdminBrands,
  useBrandWrites,
  type BrandKind,
  type BrandSanitized,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  focusRefusedField,
  FormAlert,
  lightSurfaceColor,
  ModalPresence,
  PictureField,
  SaveState,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useStaffStrings, type T } from "../parts";
import { LeaveDialog, ScreenNotice, type AdminSubProps } from "./parts";

const kinds = ["icon", "logo", "logo_dark"] as const satisfies readonly BrandKind[];

/** An image picked and checked: the server's SVG, shown from a `blob:` address. */
type Picked = { readonly sanitized: BrandSanitized; readonly url: string };

/** What the form holds of one image: one picked, the built-in one chosen back, or what is wrong with it. */
type Draft = {
  readonly picked?: Picked | undefined;
  readonly removed?: boolean | undefined;
  readonly checking?: boolean | undefined;
  readonly error?: string | undefined;
};

/** A color as the server takes it: # and six hex digits. */
const colorPattern = /^#[0-9a-f]{6}$/;

/** Why an image cannot be used, in the form's words. */
function brandError(t: T, error: unknown): string {
  const brand = brandErrorOf(error);
  if (brand !== undefined) return t("staff_settings.brand.error", { code: brand.code });
  return refusalDetail(error) ?? t("staff_settings.brand.error", { code: "other" });
}

export function BrandingForm({
  back,
  go,
  onUnsaved,
  leaving,
}: AdminSubProps & { back: { readonly label: string; readonly href: string } }): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const read = useAdminBrands();
  const writes = useBrandWrites();
  const [drafts, setDrafts] = useState<Partial<Record<BrandKind, Draft>>>({});
  const [background, setBackground] = useState<string | undefined>(undefined);
  const [sent, setSent] = useState<"idle" | "sending" | "saved" | "refused" | "failed">("idle");
  const [reason, setReason] = useState<string | undefined>(undefined);
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });
  const busy = sent === "sending";
  const unsaved = kinds.some((k) => drafts[k]?.picked !== undefined || drafts[k]?.removed === true);
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  const backButton = (
    <BackButton
      onPress={() => {
        go(back.href);
      }}
    >
      {back.label}
    </BackButton>
  );
  const title = t("staff_settings.topic.branding");
  const brands = read.brands;
  if (brands === undefined)
    return (
      <SettingsScreen
        sub
        title={title}
        back={backButton}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_settings.brand.loading"), failed: t("staff_settings.brand.failed") }}
          />
        }
      />
    );

  const draft = (kind: BrandKind, next: Draft) => {
    setDrafts((now) => ({ ...now, [kind]: next }));
    setSent((was) => (was === "saved" || was === "refused" || was === "failed" ? "idle" : was));
  };
  const controlOf = (kind: BrandKind) => `${id}-${kind}`;
  const colorId = `${id}-background`;
  const color = background ?? brands.icon?.background ?? lightSurfaceColor;
  const colorError = sent === "refused" && !colorPattern.test(color) ? t("staff_settings.brand.color.rule") : undefined;

  /** Checks the file picked for `kind` with the server, which says what it removed or why it cannot be used. */
  const pick = (kind: BrandKind, file: File) => {
    draft(kind, { checking: true });
    writes.sanitize(kind, file).then(
      (sanitized) => {
        draft(kind, {
          picked: {
            sanitized,
            url: URL.createObjectURL(new Blob([sanitized.svg], { type: "image/svg+xml" })),
          },
        });
      },
      (error: unknown) => {
        draft(kind, { error: brandError(t, error) });
        focusNext.current = controlOf(kind);
      },
    );
  };

  const save = async () => {
    if (busy) return;
    if (!unsaved) {
      go(back.href);
      return;
    }
    if (drafts.icon?.picked !== undefined && !colorPattern.test(color)) {
      setSent("refused");
      focusNext.current = colorId;
      return;
    }
    setSent("sending");
    for (const kind of kinds) {
      const d = drafts[kind];
      try {
        if (d?.picked !== undefined)
          await writes.set(kind, d.picked.sanitized, kind === "icon" ? color : lightSurfaceColor);
        else if (d?.removed === true) await writes.clear(kind);
        else continue;
        setDrafts((now) => ({ ...now, [kind]: {} }));
      } catch (error) {
        const brand = brandErrorOf(error);
        if (brand !== undefined) {
          setDrafts((now) => ({ ...now, [kind]: { ...d, error: brandError(t, error) } }));
          setSent("refused");
          focusNext.current = controlOf(kind);
        } else {
          setReason(refusalDetail(error));
          setSent("failed");
        }
        return;
      }
    }
    setSent("saved");
  };

  const field = (kind: BrandKind) => {
    const d = drafts[kind] ?? {};
    const current = brands[kind];
    const src = d.picked?.url ?? (d.removed === true ? undefined : current?.svg_url);
    const dropped = d.picked?.sanitized.dropped ?? [];
    return (
      <SettingsSection
        key={kind}
        heading={t("staff_settings.brand.kind", { kind })}
        about={t("staff_settings.brand.kind.about", { kind })}
      >
        <PictureField
          id={controlOf(kind)}
          label={t("staff_settings.brand.file", { kind })}
          hint={
            d.checking === true
              ? t("staff_settings.brand.checking")
              : src === undefined
                ? t("staff_settings.brand.builtin")
                : undefined
          }
          shape={kind === "icon" ? "square" : "wordmark"}
          dark={kind === "logo_dark"}
          accept="image/svg+xml,.svg"
          src={src}
          error={d.error}
          choose={t("staff_settings.brand.choose")}
          onFile={(file) => {
            pick(kind, file);
          }}
          remove={
            src === undefined
              ? undefined
              : {
                  label: t("staff_settings.brand.remove"),
                  onPress: () => {
                    draft(kind, { removed: current !== undefined });
                  },
                }
          }
          isDisabled={busy || d.checking === true}
        />
        {dropped.length > 0 && (
          <Box as="p" text="caption" tone="muted" breaks="phrase">
            {t("staff_settings.brand.dropped", { names: dropped.map((x) => x.name).join(", ") })}
          </Box>
        )}
        {kind === "icon" && d.picked !== undefined && (
          <TextField
            id={colorId}
            name="background"
            label={t("staff_settings.brand.color")}
            hint={t("staff_settings.brand.color.about")}
            defaultValue={color}
            autoCapitalize="none"
            error={colorError}
            onChange={(value) => {
              setBackground(value.trim().toLowerCase());
            }}
          />
        )}
      </SettingsSection>
    );
  };

  return (
    <>
      <SettingsScreen
        sub
        title={title}
        back={backButton}
        foot={
          <SubmitBar
            result={
              sent === "saved" ? (
                <SaveState state="saved" />
              ) : sent === "refused" ? (
                <FormAlert>{t("staff_settings.save.refused")}</FormAlert>
              ) : sent === "failed" ? (
                <FormAlert>{reason ?? t("staff_settings.save.failed")}</FormAlert>
              ) : undefined
            }
          >
            <Button
              kind="primary"
              busy={busy}
              onPress={() => {
                void save();
              }}
            >
              {t("staff_settings.save")}
            </Button>
          </SubmitBar>
        }
      >
        {kinds.map(field)}
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}
