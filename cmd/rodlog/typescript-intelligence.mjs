#!/usr/bin/env node
import { createRequire } from 'node:module';
import path from 'node:path';
const require = createRequire(path.join(process.cwd(), 'package.json'));
const ts = require('typescript');
const config = process.argv[2] || ts.findConfigFile(process.cwd(), ts.sys.fileExists);
const emit = d => console.log(JSON.stringify(d));
function diagnostic(d) {
 const result = {tool:'typescript',level:d.category===ts.DiagnosticCategory.Error?'error':d.category===ts.DiagnosticCategory.Warning?'warning':'info',code:'TS'+d.code,message:ts.flattenDiagnosticMessageText(d.messageText,'\n')};
 if(d.file && d.start!==undefined){
  const p=d.file.getLineAndCharacterOfPosition(d.start);
  const end=d.file.getLineAndCharacterOfPosition(d.start+(d.length||1));
  result.file=path.relative(process.cwd(),d.file.fileName); result.line=p.line+1; result.column=p.character+1;
  result.spans=[{file:result.file,line:result.line,column:result.column,end_column:end.line===p.line?end.character+1:p.character+2,primary:true}];
 }
 if(d.relatedInformation) result.related=d.relatedInformation.map(diagnostic);
 return result;
}
if(!config){console.error('No tsconfig.json found');process.exit(2);}
const read=ts.readConfigFile(config,ts.sys.readFile);
if(read.error){emit(diagnostic(read.error));process.exit(1);}
const parsed=ts.parseJsonConfigFileContent(read.config,ts.sys,path.dirname(path.resolve(config)));
const program=ts.createProgram({rootNames:parsed.fileNames,options:{...parsed.options,noEmit:true},projectReferences:parsed.projectReferences});
const checker=program.getTypeChecker();
function enclosingCall(node,pos){
 if(pos<node.getFullStart()||pos>=node.getEnd())return;
 return ts.forEachChild(node,n=>enclosingCall(n,pos)) || (ts.isCallExpression(node)||ts.isNewExpression(node)?node:undefined);
}
// Resolve actual compiler symbols rather than guessing names from error text.
function deepestNode(node,pos){
 if(pos<node.getFullStart()||pos>=node.getEnd())return;
 return ts.forEachChild(node,n=>deepestNode(n,pos)) || node;
}
function addSymbol(result,symbol,node,label){
 if(!symbol)return;
 if(symbol.flags & ts.SymbolFlags.Alias) symbol=checker.getAliasedSymbol(symbol);
 const flags=ts.TypeFormatFlags.NoTruncation;
 for(const decl of (symbol.getDeclarations()||[]).slice(0,3)){
  const file=decl.getSourceFile(), p=file.getLineAndCharacterOfPosition(decl.getStart());
  const location={file:path.relative(process.cwd(),file.fileName),line:p.line+1,column:p.character+1};
  if(result.related?.some(r=>r.file===location.file&&r.line===location.line&&r.column===location.column))continue;
  const name=checker.symbolToString(symbol);
  const signature=ts.isTypeAliasDeclaration(decl)
   ? 'type '+name+' = '+decl.type.getText(file)
   : ts.isInterfaceDeclaration(decl)
    ? 'interface '+name+(decl.typeParameters?.length?'<'+decl.typeParameters.map(t=>t.getText(file)).join(', ')+'>':'')
    : name+': '+checker.typeToString(checker.getTypeOfSymbolAtLocation(symbol,decl),node,flags);
  (result.related??=[]).push({tool:'typescript',level:'info',...location,message:label,signature,documentation:ts.displayPartsToString(symbol.getDocumentationComment(checker))});
 }
}
const diagnostics=[...parsed.errors,...ts.getPreEmitDiagnostics(program)];
for(const d of diagnostics){
 const result=diagnostic(d);
 if(d.file&&d.start!==undefined){
  const call=enclosingCall(d.file,d.start);
  const sig=call&&checker.getResolvedSignature(call);
  const decl=sig?.getDeclaration();
  if(decl){
   const file=decl.getSourceFile(), p=file.getLineAndCharacterOfPosition(decl.getStart());
   (result.related??=[]).push({tool:'typescript',level:'info',file:path.relative(process.cwd(),file.fileName),line:p.line+1,column:p.character+1,message:'Defined here (resolved or recovery signature)',signature:checker.signatureToString(sig),documentation:ts.displayPartsToString(sig.getDocumentationComment(checker))});
  }
  const node=deepestNode(d.file,d.start);
  if(node){
   addSymbol(result,checker.getSymbolAtLocation(node),node,'Symbol declared here');
   const type=checker.getTypeAtLocation(node);
   addSymbol(result,type.aliasSymbol||type.getSymbol(),node,'Type declared here');
   if(ts.isExpression(node)){
    const expected=checker.getContextualType(node);
    if(expected)addSymbol(result,expected.aliasSymbol||expected.getSymbol(),node,'Expected type declared here');
   }
  }
 }
 emit(result);
}
process.exitCode=diagnostics.some(d=>d.category===ts.DiagnosticCategory.Error)?1:0;
