// SPDX-License-Identifier: AGPL-3.0-only

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Catalog } from "./Catalog";

const root = document.getElementById("root");
if (!root) throw new Error("index.html has no #root");
createRoot(root).render(
  <StrictMode>
    <Catalog />
  </StrictMode>,
);
