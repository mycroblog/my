// SPDX-License-Identifier: AGPL-3.0-only
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { NotificationKindIcon, NotificationKindMark } from "./NotificationKindMark";

describe("a kind this bundle does not know", () => {
  it("does not break the mark or the icon", () => {
    expect(() => render(<NotificationKindMark kind={"not_yet_known" as "notice"} />)).not.toThrow();
    expect(() => render(<NotificationKindIcon kind={"not_yet_known" as "notice"} />)).not.toThrow();
  });
});
