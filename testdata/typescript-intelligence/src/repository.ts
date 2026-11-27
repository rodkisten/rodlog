import type { User, UserId, UserRepository } from "./types.js";

export class MemoryUserRepository implements UserRepository {
  #users = new Map<UserId, User>();

  async find(id: UserId): Promise<User | null> {
    return this.#users.get(id) ?? null;
  }

  async save(value: User): Promise<User> {
    this.#users.set(value.id, value);
    return value;
  }

  async remove(id: UserId): Promise<boolean> {
    return this.#users.delete(id);
  }

  async findByEmail(email: string): Promise<User | null> {
    return [...this.#users.values()].find(user => user.email === email) ?? null;
  }
}
