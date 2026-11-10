// SPDX-License-Identifier: AGPL-3.0-only
// The featured hashtags of a profile (ADR-0006 §17; web-client/screens/profile.md): a titled region under the profile's
// header, in the owner's order, each hashtag a link to its page with how many of the account's posts carry it. A profile
// that features none has no region.
import { Box, Hint, Link, Section, Stack, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { useAccountFeaturedTags } from "../hooks/useFeaturedTags";

export type FeaturedTagsProps = {
  /** The account whose profile it is. */
  accountId: string;
};

export function FeaturedTags({ accountId }: FeaturedTagsProps) {
  const t = useT();
  const tags = useAccountFeaturedTags(accountId);
  if (tags.length === 0) return null;
  return (
    // Inset like the profile's other notes under its header.
    <Box paddingX="4">
      <Section title={t("client:tag.featured.title")}>
        <Stack as="ul" gap="2">
          {tags.map((tag) => (
            <Stack as="li" key={tag.name}>
              <Link href={tag.href} variant="standalone" wrap="anywhere">
                {/* The hashtag is the user's text, in any script (web-client/README.md Y10). */}
                <Text dir="auto" weight="semibold">
                  {t("client:tag.title", { tag: tag.name })}
                </Text>
              </Link>
              <Hint as="span">{tag.stats}</Hint>
            </Stack>
          ))}
        </Stack>
      </Section>
    </Box>
  );
}
