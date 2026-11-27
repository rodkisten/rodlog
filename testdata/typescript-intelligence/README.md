# TypeScript intelligence torture fixture

This directory is an intentionally broken multi-file TypeScript project.

It exists to exercise `rodlog tsc` against real TypeScript Compiler API diagnostics rather than a repeated synthetic log. The fixture deliberately covers branded types, interfaces, aliases, discriminated unions, generics, overloads, callbacks, async return types, contextual typing, excess properties, exact optional properties, nullability, indexed access, readonly values, tuples, `satisfies`, and cross-file declarations.

Do not "fix" the TypeScript errors. The errors are the fixture.
