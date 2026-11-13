// SPDX-License-Identifier: AGPL-3.0-only

import "@my/design-system/styles.css";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Catalog } from "./Catalog";
import { Timeline } from "./timeline/Timeline";

// The catalog page at /, and the rough home timeline screen at /timeline.
const Page = location.pathname.replace(/\/+$/, "") === "/timeline" ? Timeline : Catalog;

const root = document.getElementById("root");
if (!root) throw new Error("index.html has no #root");
createRoot(root).render(
  <StrictMode>
    <Page />
  </StrictMode>,
);
