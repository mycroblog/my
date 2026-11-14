// SPDX-License-Identifier: AGPL-3.0-only

// The column beside the main one, from the frame's `wide` (ADR-0110 §11): what is around the reader. It holds a rough
// placeholder until its features are built: the server's name under a heading that links to its about page.

import { FrameAside, FrameSection } from "@my/design-system";
import type { RuntimeConfig } from "../config";
import { useStrings } from "../locale";

export function Around({ config }: { config: RuntimeConfig }) {
  const t = useStrings();
  return (
    <FrameAside label={t("app.around")}>
      <FrameSection id="around-server" icon="about" href="/about" heading={t("app.about")}>
        <p>{config.instanceName}</p>
      </FrameSection>
    </FrameAside>
  );
}
