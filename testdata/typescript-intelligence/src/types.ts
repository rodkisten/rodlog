export type Brand<T, Name extends string> = T & { readonly __brand: Name };

export type UserId = Brand<string, "UserId">;
export type TeamId = Brand<string, "TeamId">;

export interface Address {
  street: string;
  city: string;
  postalCode: string;
}

export interface User {
  id: UserId;
  name: string;
  email: string;
  age: number;
  address?: Address;
  roles: readonly ("admin" | "editor" | "viewer")[];
}

export interface Admin extends User {
  roles: readonly ["admin", ...("editor" | "viewer")[]];
  permissions: Set<string>;
}

export type Session =
  | { kind: "anonymous"; token: null }
  | { kind: "user"; token: string; user: User }
  | { kind: "admin"; token: string; user: Admin; elevatedAt: Date };

export type Result<T, E extends Error = Error> =
  | { ok: true; value: T }
  | { ok: false; error: E };

export interface Page<T> {
  items: T[];
  cursor: string | null;
  total: number;
}

export interface ApiEnvelope<T> {
  data: T;
  requestId: string;
  generatedAt: Date;
}

export type DeepReadonly<T> =
  T extends (...args: any[]) => any
    ? T
    : T extends object
      ? { readonly [K in keyof T]: DeepReadonly<T[K]> }
      : T;

export type EventMap = {
  "user.created": { user: User; actorId: UserId };
  "user.deleted": { userId: UserId; reason?: string };
  "team.changed": { teamId: TeamId; members: User[] };
};

export type EventName = keyof EventMap;

export interface Repository<T, Id> {
  find(id: Id): Promise<T | null>;
  save(value: T): Promise<T>;
  remove(id: Id): Promise<boolean>;
}

export interface UserRepository extends Repository<User, UserId> {
  findByEmail(email: string): Promise<User | null>;
}

export interface RuntimeConfig {
  apiBaseUrl: string;
  retries: number;
  features: {
    analytics: boolean;
    experiments: readonly string[];
  };
}

export type Query<T> = {
  select: readonly (keyof T)[];
  where?: Partial<T>;
  limit?: number;
};

export type Serializable =
  | string
  | number
  | boolean
  | null
  | Serializable[]
  | { [key: string]: Serializable };
