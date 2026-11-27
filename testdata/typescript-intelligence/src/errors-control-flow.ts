import type { Session, User } from "./types.js";

export function unsafeSession(session: Session): string {
  if (session.kind === "anonymous") {
    return session.user.name;
  }
  if (session.kind === "user") {
    return session.elevatedAt.toISOString();
  }
  return session.missing;
}

export function nullable(user: User | null | undefined): string {
  return user.name.toUpperCase();
}

export function indexed(users: User[]): string {
  return users[99].email;
}

export function optional(user: User): string {
  return user.address.city;
}

export function impossible(value: string | number): number {
  if (typeof value === "string") {
    return value.toFixed(2);
  }
  return value.toUpperCase();
}

export function exhaustive(session: Session): never {
  return session;
}

export function catchUnknown(): string {
  try {
    throw new Error("boom");
  } catch (error) {
    return error.message;
  }
}
