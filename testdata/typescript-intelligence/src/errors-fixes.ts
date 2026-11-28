// Errors here are intentional: they exercise TypeScript Language Service code fixes.

const upper = "rodlog".toUppercase();
const fixed = (12.34).toFixedd(2);

interface Greeter {
  greet(name: string): string;
}

class BrokenGreeter implements Greeter {}

void upper;
void fixed;
void BrokenGreeter;
