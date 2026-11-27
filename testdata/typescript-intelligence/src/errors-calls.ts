import { MemoryUserRepository } from "./repository.js";
import { configure, emit, envelope, format, getUser, query, renameUser, serialize } from "./service.js";
import type { Page, Result, User, UserId } from "./types.js";

const repository = new MemoryUserRepository();
const id = "user-1" as UserId;

void getUser(repository, "not-branded");
void getUser(repository);
void getUser(repository, id, "extra");

const user: User = {
  id,
  name: "Lin",
  email: "lin@example.com",
  age: 32,
  roles: ["editor"],
};

void renameUser(repository, user, 123);
void renameUser(repository, user);
void renameUser(user, repository, "Wrong order");

format(true);
format(1);
format(1, "two");
format(new Date(), 123);
format("text", 2);

emit("user.created", { user, actorId: "plain" });
emit("user.deleted", { userId: id, reason: 123 });
emit("team.changed", { teamId: id, members: [user] });
emit("unknown.event", { anything: true });

query([user], { select: ["missing"] });
query([user], { select: ["name"], limit: "ten" });
query([user], { select: ["name"], where: { age: "old" } });

const page: Page<User> = { items: [user], cursor: null, total: 1 };
envelope(page, 100);

const result: Result<User> = { ok: true, value: user };
serialize({ ok: false, error: "bad" });
serialize(result);

configure({
  apiBaseUrl: "/api",
  retries: 3,
  features: { analytics: true, experiments: ["one"] },
  extra: true,
});
