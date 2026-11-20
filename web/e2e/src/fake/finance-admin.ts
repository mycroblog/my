// SPDX-License-Identifier: AGPL-3.0-only

// The fake's finance as a holder of `manage_finance` keeps it (ADR-0067, ADR-0068): the cost items of the fake's
// finance (finance.ts, whose totals they are), recorded, relabelled, revised from a date, ended and deleted as the
// server checks them; the billing imports of AWS, an estimate for this month and final ones before it, and running
// one now. The totals and the income are finance.ts's; the settings of what the server publishes, admin-settings.ts's.

import { day, idAt, page } from "./common";
import type { Answer, Handlers, Refusal, Schema } from "./spec";

type CostItem = Schema<"CostItem">;
type Money = Schema<"Money">;

/** What the administrators of a fake server keep of its finance. */
export type FakeFinanceAdminOptions = {
  /** The cost items: the fake's five by default, or none. */
  costs?: "none";
  /**
   * The billing imports: AWS, whose last import succeeded, by default; its last import failed for lack of permission
   * (`failing`); or no source (`none`).
   */
  imports?: "failing" | "none";
  /** An import of AWS ran in the last hour, so that another is refused (429). */
  importTooSoon?: boolean;
};

/** What the handlers of the administrators' finance take from the fake. */
export type FinanceAdminContext = {
  readonly options: FakeFinanceAdminOptions | undefined;
  /** When the seed ran, in ms since the epoch: its UTC date is today. */
  readonly started: number;
  /** The reader's permissions. */
  readonly permissions: () => readonly Schema<"Permission">[];
  readonly problem: (status: number, title: string, more?: Partial<Schema<"Problem">>) => Refusal;
  readonly notFound: () => Refusal;
};

/** What a story or a test does with the administrators' finance of the fake. */
export type FakeFinanceAdmin = {
  /** The ID of the cost item of `key` (`vps`, `bucket`, `mail`, `backups`, `domain`; "none" for none). */
  readonly costId: (key: string) => string;
};

/** The codes whose minor unit is not two digits, by its digits, as ISO 4217 and the server's table have them. */
const otherDigits: Readonly<Record<string, number>> = {
  ...Object.fromEntries(
    [
      "BIF",
      "CLP",
      "DJF",
      "GNF",
      "ISK",
      "JPY",
      "KMF",
      "KRW",
      "PYG",
      "RWF",
      "UGX",
      "UYI",
      "VND",
      "VUV",
      "XAF",
      "XOF",
      "XPF",
    ].map((code) => [code, 0]),
  ),
  ...Object.fromEntries(["BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND"].map((code) => [code, 3])),
  CLF: 4,
  UYW: 4,
};

/** The digits of a currency's minor unit, as the server's table has them. */
const digits = (currency: string) => otherDigits[currency] ?? 2;
const moneyValid = (m: Money) =>
  /^[A-Z]{3}$/.test(m.currency) &&
  (digits(m.currency) === 0 ? /^\d+$/ : new RegExp(`^\\d+(\\.\\d{1,${String(digits(m.currency))}})?$`)).test(m.amount);
const dateValid = (date: string | undefined) =>
  date !== undefined &&
  /^\d{4}-\d{2}-\d{2}$/.test(date) &&
  !Number.isNaN(Date.parse(`${date}T00:00:00Z`)) &&
  new Date(`${date}T00:00:00Z`).toISOString().startsWith(date);
const textValid = (text: string, max: number) =>
  text.trim() !== "" && Array.from(text).length <= max && !/\p{Cc}/u.test(text);

/** The seed's cost items: the fake's finance's (finance.ts), by key. */
const seed: readonly (Omit<CostItem, "id" | "prices" | "created_at"> & { key: string; price: Money; at: string })[] = [
  {
    key: "vps",
    name: "VPS",
    provider: "Sakura Cloud",
    category: "compute",
    memo: "2 vCPU, 4 GB, Tokyo zone",
    schedule: "months",
    interval: 1,
    starts_on: "2025-01-15",
    price: { amount: "3300", currency: "JPY" },
    at: "2025-01-14",
  },
  {
    key: "bucket",
    name: "Media bucket",
    provider: "Cloudflare",
    category: "storage",
    memo: "R2 bucket media-prod",
    schedule: "months",
    interval: 1,
    starts_on: "2025-03-01",
    price: { amount: "5.00", currency: "USD" },
    at: "2025-02-27",
  },
  {
    key: "mail",
    name: "Mail delivery",
    provider: "Postmark",
    category: "email",
    memo: "",
    schedule: "months",
    interval: 1,
    starts_on: "2025-02-01",
    price: { amount: "15.00", currency: "USD" },
    at: "2025-01-30",
  },
  {
    key: "backups",
    name: "Backups",
    provider: "Backblaze",
    category: "storage",
    memo: "Paid each quarter",
    schedule: "months",
    interval: 3,
    starts_on: "2025-01-05",
    price: { amount: "900", currency: "JPY" },
    at: "2025-01-04",
  },
  {
    key: "domain",
    name: "Domain demo.test",
    provider: "Gandi",
    category: "domain",
    memo: "Renews each April",
    schedule: "years",
    interval: 1,
    starts_on: "2025-04-10",
    ends_on: "2027-04-10",
    price: { amount: "1650", currency: "JPY" },
    at: "2025-04-01",
  },
];

/** The handlers of the administrators' finance, and what a story does with them. */
export function financeAdminHandlers(c: FinanceAdminContext): { handlers: Handlers; admin: FakeFinanceAdmin } {
  const o = c.options ?? {};
  const iso = (ms: number) => new Date(ms).toISOString();
  let next = 0;
  const newId = () => idAt(Date.now(), 79_000 + (next += 1));
  const keys = new Map<string, string>();
  const items: CostItem[] =
    o.costs === "none"
      ? []
      : seed.map(({ key, price, at, ...item }, i) => {
          const id = idAt(Date.parse(`${at}T09:00:00Z`), 78_500 + i);
          keys.set(key, id);
          const created = `${at}T09:00:00.000Z`;
          return {
            ...item,
            id,
            prices: [
              { effective_on: item.starts_on, price, revised_at: created },
              // The VPS's price went up this year.
              ...(key === "vps"
                ? [
                    {
                      effective_on: "2026-04-15",
                      price: { amount: "3630", currency: "JPY" },
                      revised_at: "2026-03-20T10:00:00.000Z",
                    },
                  ]
                : []),
            ],
            created_at: created,
          };
        });
  const newestFirst = () =>
    items.toSorted((a, b) => b.created_at.localeCompare(a.created_at) || b.id.localeCompare(a.id));
  const may = (name: Schema<"Permission">) => c.permissions().includes(name);
  const missing = (name: Schema<"Permission">) =>
    c.problem(403, "You do not have permission", {
      type: "/api/v1/problems/missing-permission",
      detail: `This needs the ${name} permission.`,
    });
  const invalid = (field: string) =>
    c.problem(422, "This could not be done.", { detail: "This value is not valid. Check what you entered.", field });
  const found = (id: string) => items.find((item) => item.id === id);

  /** The label of a request checked as the server checks it: the member it refuses, or nothing. */
  function labelFault(label: Schema<"CostItemLabelRequest">): string | undefined {
    if (label.name !== undefined && !textValid(label.name, 100)) return "name";
    if (label.provider !== undefined && !textValid(label.provider, 100)) return "provider";
    if (label.memo !== undefined && Array.from(label.memo).length > 10_000) return "memo";
    return undefined;
  }

  // ---- The billing imports ----------------------------------------------------------------------------------------

  const startedDay = Math.floor(c.started / day);
  const monthOf = (offset: number) => {
    const d = new Date(c.started);
    return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + offset, 1)).toISOString().slice(0, 7);
  };
  const aws = (): Schema<"BillingImport"> => {
    const failing = o.imports === "failing";
    const ranAt = iso(startedDay * day + 2 * 3_600_000);
    return {
      source: "aws",
      provider: "AWS",
      method: "cost_explorer",
      time_zone: "UTC",
      last_run: failing
        ? { outcome: "failed", error_kind: "permission", requests: 1, bytes_billed: 0, occurred_at: ranAt }
        : { outcome: "succeeded", error_kind: "none", requests: 2, bytes_billed: 0, occurred_at: ranAt },
      last_success_at: failing ? iso(c.started - 3 * day) : ranAt,
      requests_today: failing ? 1 : 2,
      requests_this_month: 37,
      max_requests_per_day: 10,
      max_requests_per_month: 200,
      bytes_billed_this_month: 0,
      months: [0, -1, -2, -3].map((offset) => ({
        month: monthOf(offset),
        kind: offset === 0 ? ("estimate" as const) : offset === -2 ? ("correction" as const) : ("final" as const),
        totals: [{ amount: (12.4 + (Number(monthOf(offset).slice(5)) % 4) * 1.35).toFixed(2), currency: "USD" }],
      })),
    };
  };

  const handlers: Handlers = {
    "GET /admin/cost_items": ({ query }): Answer<"GET /admin/cost_items"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      return { status: 200, body: page(newestFirst(), query).body };
    },
    "POST /admin/cost_items": ({ body }): Answer<"POST /admin/cost_items"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      const once = body.schedule === "once";
      if (once ? body.interval !== 1 : body.interval < 1 || body.interval > 99) return invalid("interval");
      if (!dateValid(body.starts_on)) return invalid("starts_on");
      if (body.ends_on !== undefined && (!dateValid(body.ends_on) || body.ends_on < body.starts_on))
        return invalid("ends_on");
      const fault = labelFault(body);
      if (fault !== undefined) return invalid(fault);
      if (!moneyValid(body.price)) return invalid("price");
      const now = iso(Date.now());
      const item: CostItem = {
        id: newId(),
        schedule: body.schedule,
        interval: body.interval,
        starts_on: body.starts_on,
        ...(body.ends_on === undefined || once ? {} : { ends_on: body.ends_on }),
        name: body.name,
        provider: body.provider,
        category: body.category,
        memo: body.memo ?? "",
        prices: [{ effective_on: body.starts_on, price: body.price, revised_at: now }],
        created_at: now,
      };
      items.push(item);
      return { status: 201, body: item };
    },
    "GET /admin/cost_items/{id}": ({ params }): Answer<"GET /admin/cost_items/{id}"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      const item = found(params.id);
      return item === undefined ? c.notFound() : { status: 200, body: item };
    },
    "PATCH /admin/cost_items/{id}": ({ params, body }): Answer<"PATCH /admin/cost_items/{id}"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      const item = found(params.id);
      if (item === undefined) return c.notFound();
      const fault = labelFault(body);
      if (fault !== undefined) return invalid(fault);
      if (body.name !== undefined) item.name = body.name;
      if (body.provider !== undefined) item.provider = body.provider;
      if (body.category !== undefined) item.category = body.category;
      if (body.memo !== undefined) item.memo = body.memo;
      return { status: 200, body: item };
    },
    "POST /admin/cost_items/{id}/price_revisions": ({
      params,
      body,
    }): Answer<"POST /admin/cost_items/{id}/price_revisions"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      const item = found(params.id);
      if (item === undefined) return c.notFound();
      if (!dateValid(body.effective_on) || body.effective_on < item.starts_on) return invalid("effective_on");
      if (!moneyValid(body.price)) return invalid("price");
      item.prices.push({ effective_on: body.effective_on, price: body.price, revised_at: iso(Date.now()) });
      return { status: 200, body: item };
    },
    "PUT /admin/cost_items/{id}/end": ({ params, body }): Answer<"PUT /admin/cost_items/{id}/end"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      const item = found(params.id);
      if (item === undefined) return c.notFound();
      if (body.ends_on === undefined) delete item.ends_on;
      else if (!dateValid(body.ends_on) || body.ends_on < item.starts_on) return invalid("ends_on");
      else item.ends_on = body.ends_on;
      return { status: 200, body: item };
    },
    "DELETE /admin/cost_items/{id}": ({ params }): Answer<"DELETE /admin/cost_items/{id}"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      const at = items.findIndex((item) => item.id === params.id);
      if (at < 0) return c.notFound();
      items.splice(at, 1);
      return { status: 204 };
    },

    "GET /admin/billing_imports": (): Answer<"GET /admin/billing_imports"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      return { status: 200, body: { items: o.imports === "none" ? [] : [aws()] } };
    },
    "POST /admin/billing_imports/{source}/runs": ({ params }): Answer<"POST /admin/billing_imports/{source}/runs"> => {
      if (!may("manage_finance")) return missing("manage_finance");
      if (o.imports === "none" || params.source !== "aws") return c.notFound();
      if (o.importTooSoon === true)
        return c.problem(429, "Too Many Requests", { detail: "AWS was imported in the last hour." });
      return { status: 202 };
    },
  };

  return { handlers, admin: { costId: (key) => keys.get(key) ?? "none" } };
}
