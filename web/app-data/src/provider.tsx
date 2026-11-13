// SPDX-License-Identifier: AGPL-3.0-only
import { QueryClientProvider } from "@tanstack/react-query";
import { createContext, use, type ReactNode } from "react";
import type { AppData } from "./app-data";

const AppDataContext = createContext<AppData | null>(null);

/** Gives the hooks of @my/app-data the session's data; the app places it once, above every feature. */
export function AppDataProvider({ data, children }: { data: AppData; children: ReactNode }) {
  return (
    <AppDataContext value={data}>
      <QueryClientProvider client={data.queryClient}>{children}</QueryClientProvider>
    </AppDataContext>
  );
}

/** The AppData of the nearest AppDataProvider. */
export function useAppData(): AppData {
  const data = use(AppDataContext);
  if (data === null) throw new Error("app-data: a hook of @my/app-data runs outside AppDataProvider");
  return data;
}
