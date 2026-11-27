#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';

const emit = diagnostic => console.log(JSON.stringify(diagnostic));
const fatal = (code, message, help = []) => {
  emit({ tool: 'typescript', level: 'error', code, message, help });
  process.exit(2);
};

function findUp(filename, start) {
  let dir = path.resolve(start);
  while (true) {
    const candidate = path.join(dir, filename);
    if (fs.existsSync(candidate)) return candidate;
    const parent = path.dirname(dir);
    if (parent === dir) return '';
    dir = parent;
  }
}

function resolveConfig(input) {
  if (!input) return findUp('tsconfig.json', process.cwd());
  const candidate = path.resolve(input);
  try {
    if (fs.statSync(candidate).isDirectory()) return path.join(candidate, 'tsconfig.json');
  } catch {}
  return candidate;
}

const baseRequire = createRequire(import.meta.url);

function normalizeTypeScript(mod) {
  // Newer TypeScript package layouts may expose the compiler through a
  // CommonJS/ESM interop default export. Always return the compiler object
  // that owns sys/readConfigFile rather than the wrapper namespace.
  if (mod?.sys && mod?.readConfigFile) return mod;
  if (mod?.default?.sys && mod?.default?.readConfigFile) return mod.default;
  return mod;
}

function packageRequire(base, specifier = 'typescript') {
  const req = createRequire(path.join(base, 'package.json'));
  return normalizeTypeScript(req(req.resolve(specifier)));
}

function typescriptFromExecutable() {
  try {
    const command = process.platform === 'win32' ? 'where' : 'which';
    const executable = execFileSync(command, ['tsc'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] })
      .trim()
      .split(/\r?\n/)[0];
    if (!executable) return null;
    let dir = path.dirname(fs.realpathSync(executable));
    while (true) {
      const manifest = path.join(dir, 'package.json');
      if (fs.existsSync(manifest)) {
        try {
          if (JSON.parse(fs.readFileSync(manifest, 'utf8')).name === 'typescript') {
            return normalizeTypeScript(baseRequire(dir));
          }
        } catch {}
      }
      const parent = path.dirname(dir);
      if (parent === dir) return null;
      dir = parent;
    }
  } catch {
    return null;
  }
}

function hasLegacyCompilerAPI(candidate) {
  return Boolean(candidate?.sys && candidate?.readConfigFile && candidate?.createProgram);
}

function loadTypeScript(projectDir) {
  if (process.env.RODLOG_TYPESCRIPT) {
    try {
      const explicit = normalizeTypeScript(baseRequire(path.resolve(process.env.RODLOG_TYPESCRIPT)));
      if (hasLegacyCompilerAPI(explicit)) return explicit;
    } catch {}
  }

  // TypeScript 7 intentionally removed the legacy JavaScript Compiler API from
  // the root "typescript" export. Microsoft ships @typescript/typescript6 as
  // the compatibility API for tools that still need Program/Checker/sys.
  for (const base of new Set([projectDir, process.cwd()])) {
    for (const specifier of ['typescript', '@typescript/typescript6']) {
      try {
        const candidate = packageRequire(base, specifier);
        if (hasLegacyCompilerAPI(candidate)) return candidate;
      } catch {}
    }
  }

  for (const [command, args] of [['npm', ['root', '-g']], ['pnpm', ['root', '-g']]]) {
    try {
      const root = execFileSync(command, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
      if (!root) continue;
      for (const specifier of ['typescript', '@typescript/typescript6']) {
        try {
          const candidate = normalizeTypeScript(baseRequire(path.join(root, specifier)));
          if (hasLegacyCompilerAPI(candidate)) return candidate;
        } catch {}
      }
    } catch {}
  }

  const executable = typescriptFromExecutable();
  return hasLegacyCompilerAPI(executable) ? executable : null;
}

const config = resolveConfig(process.argv[2]);
if (!config || !fs.existsSync(config)) {
  fatal(
    'RODLOGTS001',
    `No tsconfig.json found from ${process.cwd()}`,
    ['Run rodlog tsc inside a TypeScript project or pass --project path/to/tsconfig.json.'],
  );
}

const projectDir = path.dirname(config);

function normalizedPath(fileName) {
  return path.resolve(fileName).split(path.sep).join('/');
}

function isProjectFile(fileName) {
  const absolute = normalizedPath(fileName);
  const root = normalizedPath(projectDir).replace(/\/$/, '') + '/';
  return absolute.startsWith(root) && !absolute.includes('/node_modules/');
}

function displayPath(fileName) {
  if (isProjectFile(fileName)) return path.relative(projectDir, fileName);
  return path.relative(process.cwd(), fileName);
}

const ts = loadTypeScript(projectDir);
if (!ts?.sys || !ts?.readConfigFile || !ts?.createProgram) {
  fatal(
    'RODLOGTS002',
    `TypeScript compiler could not be resolved for ${projectDir}`,
    [
      'For TypeScript 7, install @typescript/typescript6 alongside typescript so rodlog can use the compatibility Compiler API.',
      'For TypeScript 6 or earlier, the project typescript package already exposes the Compiler API.',
      'rodlog never downloads TypeScript implicitly.',
    ],
  );
}

function diagnostic(d) {
  const result = {
    tool: 'typescript',
    level: d.category === ts.DiagnosticCategory.Error ? 'error' : d.category === ts.DiagnosticCategory.Warning ? 'warning' : 'info',
    code: 'TS' + d.code,
    message: ts.flattenDiagnosticMessageText(d.messageText, '\n'),
  };
  if (d.file && d.start !== undefined) {
    const p = d.file.getLineAndCharacterOfPosition(d.start);
    const end = d.file.getLineAndCharacterOfPosition(d.start + (d.length || 1));
    result.file = path.relative(process.cwd(), d.file.fileName);
    result.display_file = displayPath(d.file.fileName);
    result.line = p.line + 1;
    result.column = p.character + 1;
    result.spans = [{
      file: result.file,
      line: result.line,
      column: result.column,
      end_column: end.line === p.line ? end.character + 1 : p.character + 2,
      primary: true,
    }];
  }
  if (d.relatedInformation) {
    result.related = d.relatedInformation
      .filter(item => !item.file || isProjectFile(item.file.fileName))
      .map(diagnostic);
  }
  return result;
}

const read = ts.readConfigFile(config, ts.sys.readFile);
if (read.error) {
  emit(diagnostic(read.error));
  process.exit(1);
}

const parsed = ts.parseJsonConfigFileContent(read.config, ts.sys, projectDir);
const program = ts.createProgram({
  rootNames: parsed.fileNames,
  options: { ...parsed.options, noEmit: true },
  projectReferences: parsed.projectReferences,
});
const checker = program.getTypeChecker();

function enclosingCall(node, pos) {
  if (pos < node.getFullStart() || pos >= node.getEnd()) return;
  return ts.forEachChild(node, child => enclosingCall(child, pos))
    || (ts.isCallExpression(node) || ts.isNewExpression(node) ? node : undefined);
}

function deepestNode(node, pos) {
  if (pos < node.getFullStart() || pos >= node.getEnd()) return;
  return ts.forEachChild(node, child => deepestNode(child, pos)) || node;
}

function isSignatureDeclaration(node) {
  return Boolean(
    node && (
      ts.isFunctionDeclaration(node)
      || ts.isMethodDeclaration(node)
      || ts.isConstructorDeclaration(node)
      || ts.isGetAccessorDeclaration(node)
      || ts.isSetAccessorDeclaration(node)
      || ts.isFunctionExpression(node)
      || ts.isArrowFunction(node)
      || ts.isCallSignatureDeclaration(node)
      || ts.isConstructSignatureDeclaration(node)
      || ts.isMethodSignature(node)
      || ts.isFunctionTypeNode(node)
      || ts.isConstructorTypeNode(node)
    )
  );
}

function signatureFromDeclaration(decl) {
  if (!isSignatureDeclaration(decl)) return undefined;
  try {
    return checker.getSignatureFromDeclaration(decl);
  } catch {
    return undefined;
  }
}

function signatureParameterFocus(call, sig, pos) {
  if (!call || !sig || !call.arguments?.length) return null;
  const index = call.arguments.findIndex(argument => pos >= argument.getFullStart() && pos < argument.getEnd());
  if (index < 0) return null;

  const parameters = sig.getParameters();
  if (!parameters.length) return null;
  const parameter = parameters[Math.min(index, parameters.length - 1)];
  if (!parameter) return null;

  const declaration = parameter.valueDeclaration || parameter.getDeclarations()?.[0] || sig.getDeclaration();
  const type = checker.getTypeOfSymbolAtLocation(parameter, declaration || call);
  const name = checker.symbolToString(parameter);
  const optional = Boolean(parameter.flags & ts.SymbolFlags.Optional);
  const rest = declaration && ts.isParameter(declaration) && declaration.dotDotDotToken ? '...' : '';
  const focus = rest + name + (optional ? '?' : '') + ': ' + checker.typeToString(type, declaration || call, ts.TypeFormatFlags.NoTruncation);
  return { focus, label: `argument ${index + 1} expects this parameter` };
}

function addSymbol(result, symbol, node, label) {
  if (!symbol) return;
  if (symbol.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
  const flags = ts.TypeFormatFlags.NoTruncation;
  for (const decl of (symbol.getDeclarations() || []).filter(decl => isProjectFile(decl.getSourceFile().fileName)).slice(0, 3)) {
    const file = decl.getSourceFile();
    const p = file.getLineAndCharacterOfPosition(decl.getStart());
    const location = {
      file: path.relative(process.cwd(), file.fileName),
      display_file: displayPath(file.fileName),
      line: p.line + 1,
      column: p.character + 1,
    };
    if (
      (result.file === location.file && result.line === location.line && result.column === location.column)
      || result.related?.some(item =>
        item.file === location.file && item.line === location.line && item.column === location.column
      )
    ) continue;

    const name = checker.symbolToString(symbol);
    const declaredSignature = signatureFromDeclaration(decl);
    const signature = declaredSignature
      ? name + checker.signatureToString(declaredSignature, decl, flags)
      : ts.isTypeAliasDeclaration(decl)
        ? 'type ' + name + ' = ' + decl.type.getText(file)
        : ts.isInterfaceDeclaration(decl)
          ? 'interface ' + name + (decl.typeParameters?.length
            ? '<' + decl.typeParameters.map(param => param.getText(file)).join(', ') + '>'
            : '')
          : name + ': ' + checker.typeToString(checker.getTypeOfSymbolAtLocation(symbol, decl), node, flags);

    (result.related ??= []).push({
      tool: 'typescript',
      level: 'info',
      ...location,
      message: label,
      signature,
      documentation: ts.displayPartsToString(symbol.getDocumentationComment(checker)),
    });
  }
}

const diagnostics = [...parsed.errors, ...ts.getPreEmitDiagnostics(program)];
for (const d of diagnostics) {
  const result = diagnostic(d);
  if (d.file && d.start !== undefined) {
    const call = enclosingCall(d.file, d.start);
    const sig = call && checker.getResolvedSignature(call);
    const focus = signatureParameterFocus(call, sig, d.start);
    const decl = sig?.getDeclaration();
    if (decl && isProjectFile(decl.getSourceFile().fileName)) {
      const file = decl.getSourceFile();
      const p = file.getLineAndCharacterOfPosition(decl.getStart());
      (result.related ??= []).push({
        tool: 'typescript',
        level: 'info',
        file: path.relative(process.cwd(), file.fileName),
        display_file: displayPath(file.fileName),
        line: p.line + 1,
        column: p.character + 1,
        message: 'Defined here (resolved or recovery signature)',
        signature: checker.signatureToString(sig),
        signature_focus: focus?.focus || '',
        signature_focus_label: focus?.label || '',
        documentation: ts.displayPartsToString(sig.getDocumentationComment(checker)),
      });
    }

    const node = deepestNode(d.file, d.start);
    if (node) {
      addSymbol(result, checker.getSymbolAtLocation(node), node, 'Symbol declared here');
      const type = checker.getTypeAtLocation(node);
      addSymbol(result, type.aliasSymbol || type.getSymbol(), node, 'Type declared here');
      if (ts.isExpression(node)) {
        const expected = checker.getContextualType(node);
        if (expected) addSymbol(result, expected.aliasSymbol || expected.getSymbol(), node, 'Expected type declared here');
      }
    }
  }
  emit(result);
}

process.exitCode = diagnostics.some(d => d.category === ts.DiagnosticCategory.Error) ? 1 : 0;
