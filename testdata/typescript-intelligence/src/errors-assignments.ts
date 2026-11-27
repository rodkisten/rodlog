import type {
  Address,
  Admin,
  ApiEnvelope,
  DeepReadonly,
  Page,
  Result,
  RuntimeConfig,
  Session,
  TeamId,
  User,
  UserId,
} from "./types.js";

const id: UserId = "plain-string";
const teamId: TeamId = id;

const address: Address = {
  street: "Main",
  city: "Springfield",
};

const user: User = {
  id,
  name: "Ada",
  email: 42,
  age: "thirty",
  roles: ["admin", "owner"],
};

const admin: Admin = {
  ...user,
  roles: ["viewer"],
  permissions: ["users.read"],
};

const anonymous: Session = {
  kind: "anonymous",
  token: "should-be-null",
};

const authenticated: Session = {
  kind: "user",
  token: "token",
};

const page: Page<User> = {
  items: [user],
  cursor: 123,
  total: "one",
};

const envelope: ApiEnvelope<Page<User>> = {
  data: page,
  requestId: 999,
  generatedAt: "today",
};

const success: Result<User> = {
  ok: true,
  value: null,
};

const failure: Result<User> = {
  ok: false,
  error: "boom",
};

const config: RuntimeConfig = {
  apiBaseUrl: 42,
  retries: "three",
  features: {
    analytics: "yes",
    experiments: [1, 2, 3],
  },
};

const readonlyUser: DeepReadonly<User> = user;
readonlyUser.name = "Grace";
readonlyUser.roles.push("viewer");

const tuple: readonly [string, number, boolean] = ["ok", "wrong", true];
const union: "draft" | "published" = "archived";
const literal: 200 | 201 | 204 = 500;

void teamId;
void address;
void admin;
void anonymous;
void authenticated;
void envelope;
void success;
void failure;
void config;
void tuple;
void union;
void literal;
