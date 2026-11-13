// SPDX-License-Identifier: AGPL-3.0-only
// restrict/design-system-props: what it allows. No line of this file may fail.

import { Feed } from "@my/app-components";
import { Avatar, Button } from "@my/design-system";

// A component of the same file, and one that takes the name of the import in its scope.
const Local = (p: { className: string }) => <div className={p.className} />;
function Shadowed({ Button }: { Button: (p: { className: string }) => null }) {
  return <Button className="x" />;
}

const variant = { kind: "primary" } as const;
const layout = { className: "p-2" };

export const Allowed = () => (
  // Plain DOM elements take a class and a style.
  <div className="p-2" style={{ color: "red" }}>
    <span {...layout} />
    {/* The props and variants of a component of the design system. */}
    <Button {...variant} size="small">
      Go
    </Button>
    <Avatar size="post" />
    {/* A component of another package. */}
    <Feed className="p-2" />
    <Local className="p-2" />
    <Shadowed Button={() => null} />
  </div>
);
