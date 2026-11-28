import type { Query, Repository, Serializable, User, UserId } from "./types.js";

type WithId<T extends { id: string }> = T & { createdAt: Date };

function attach<T extends { id: string }>(value: T): WithId<T> {
  return { ...value, createdAt: new Date() };
}

const noId = attach({ name: "missing" });
const wrongId = attach({ id: 123, name: "wrong" });
type InvalidWithId = WithId<{ name: string }>;

function first<T>(values: readonly T[]): T {
  return values[0];
}

const firstNumber: number = first(["one", "two"]);

function mapRecord<K extends string, V>(
  keys: readonly K[],
  factory: (key: K) => V,
): Record<K, V> {
  return Object.fromEntries(keys.map(key => [key, factory(key)])) as Record<K, V>;
}

const scores = mapRecord(["a", "b"] as const, key => key.length);
const impossibleScore: string = scores.a;

declare const repository: Repository<User, UserId>;
void repository.find(123);
void repository.remove("plain");
void repository.save({ name: "incomplete" });

const query: Query<User> = {
  select: ["name", "doesNotExist"],
  where: { age: "old" },
  limit: "many",
};

const json: Serializable = new Date();
const fn: Serializable = () => "nope";
const symbolValue: Serializable = Symbol("nope");

void noId;
void wrongId;
type _KeepInvalidWithId = InvalidWithId;
void firstNumber;
void impossibleScore;
void query;
void json;
void fn;
void symbolValue;
