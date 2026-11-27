type Handler<T> = (value: T, index: number) => boolean;

function filterMap<T, R>(
  values: readonly T[],
  predicate: Handler<T>,
  mapper: (value: T) => R,
): R[] {
  return values.filter(predicate).map(mapper);
}

const result = filterMap(
  [1, 2, 3],
  value => value.toUpperCase(),
  (value: string) => value.length,
);

const handler: Handler<string> = (value: number) => value > 0;

async function fetchCount(): Promise<number> {
  return "five";
}

async function fetchLabel(): Promise<string> {
  return Promise.resolve(123);
}

function takesCallback(callback: (error: Error | null, value: string) => void): void {
  callback(null, "ok");
}

takesCallback((error: string, value: number) => {
  console.log(error, value);
});

function neverReturns(): never {
  return "actually returns";
}

function requiresThis(this: { ready: boolean }, value: string): string {
  return this.ready ? value : "";
}

requiresThis.call({ ready: "yes" }, 123);

void result;
void handler;
