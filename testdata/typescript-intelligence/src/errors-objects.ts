import type { RuntimeConfig, User } from "./types.js";

const exact: RuntimeConfig = {
  apiBaseUrl: "/api",
  retries: 2,
  features: {
    analytics: true,
    experiments: [],
    impossible: true,
  },
};

const user = {
  id: "raw",
  name: "Rod",
  email: "rod@example.com",
  age: 33,
  roles: ["admin", "superadmin"],
  unexpected: 123,
} satisfies User;

const partial: Partial<User> = {
  age: undefined,
};

const record: Record<"a" | "b", number> = {
  a: 1,
  c: 3,
};

const map: Map<string, number> = new Map([
  ["one", "1"],
  [2, 2],
]);

const set: Set<string> = new Set([1, 2, 3]);

const promise: Promise<User> = Promise.resolve("user");
const date: Date = "2026-11-27";

void exact;
void user;
void partial;
void record;
void map;
void set;
void promise;
void date;
