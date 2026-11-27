import type {
  ApiEnvelope,
  EventMap,
  EventName,
  Page,
  Query,
  Result,
  RuntimeConfig,
  Serializable,
  Session,
  User,
  UserId,
  UserRepository,
} from "./types.js";

/** Fetches one user by its branded identifier. */
export async function getUser(repository: UserRepository, id: UserId): Promise<User> {
  const user = await repository.find(id);
  if (!user) throw new Error("User not found");
  return user;
}

/** Updates a user's display name and returns the saved entity. */
export async function renameUser(
  repository: UserRepository,
  user: User,
  name: string,
): Promise<User> {
  return repository.save({ ...user, name });
}

/** Resolves a human-readable label for a session. */
export function sessionLabel(session: Session): string {
  switch (session.kind) {
    case "anonymous":
      return "anonymous";
    case "user":
    case "admin":
      return session.user.name;
  }
}

/** Formats values using intentionally useful overloads for diagnostic fixtures. */
export function format(value: string): string;
export function format(value: number, precision: number): string;
export function format(value: Date, locale?: string): string;
export function format(value: string | number | Date, option?: string | number): string {
  if (typeof value === "number") return value.toFixed(typeof option === "number" ? option : 2);
  if (value instanceof Date) return value.toLocaleDateString(typeof option === "string" ? option : undefined);
  return value.toUpperCase();
}

/** Emits one typed domain event. */
export function emit<K extends EventName>(name: K, payload: EventMap[K]): void {
  void name;
  void payload;
}

/** Performs a strongly typed projection query. */
export function query<T extends object>(rows: readonly T[], spec: Query<T>): Partial<T>[] {
  return rows.slice(0, spec.limit ?? rows.length).map(row => {
    const projected: Partial<T> = {};
    for (const key of spec.select) projected[key] = row[key];
    return projected;
  });
}

/** Wraps a page in the API transport envelope. */
export function envelope<T>(page: Page<T>, requestId: string): ApiEnvelope<Page<T>> {
  return { data: page, requestId, generatedAt: new Date() };
}

/** Converts a result to a serializable transport value. */
export function serialize<T>(result: Result<T>): Serializable {
  return result.ok
    ? { ok: true, value: String(result.value) }
    : { ok: false, error: result.error.message };
}

/** Loads runtime configuration. */
export function configure(config: RuntimeConfig): RuntimeConfig {
  return config;
}
