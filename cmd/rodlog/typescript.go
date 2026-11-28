package main

import (
 _ "embed"
 "fmt"
 "io"
 "os"
 "strings"
)

//go:embed typescript-intelligence.mjs
var typescriptAdapter []byte

func runTypeScript(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
 project := ""
 transport := "terminal"
 flags := []string{}

 for i:=0;i<len(args);i++ {
  arg := args[i]
  switch {
  case arg=="--project" || arg=="-p":
   if i+1>=len(args) { fmt.Fprintln(stderr,"missing project path"); return 2 }
   i++; project=args[i]
  case strings.HasPrefix(arg,"--project="):
   project=strings.TrimPrefix(arg,"--project=")
  case arg=="--transport":
   if i+1>=len(args) { fmt.Fprintln(stderr,"missing transport"); return 2 }
   i++; transport=args[i]
  case strings.HasPrefix(arg,"--transport="):
   transport=strings.TrimPrefix(arg,"--transport=")
  case arg=="--help" || arg=="-h":
   fmt.Fprintln(stdout,"Usage: rodlog tsc [-p tsconfig.json] [--transport terminal|jsonl] [rodlog flags]")
   fmt.Fprintln(stdout,"  terminal  render rich TypeScript diagnostics (default)")
   fmt.Fprintln(stdout,"  jsonl     emit raw enriched Diagnostic JSON, suitable for Docker/BuildKit transport")
   return 0
  default:
   flags=append(flags,arg)
  }
 }

 if transport!="terminal" && transport!="jsonl" {
  fmt.Fprintln(stderr,"invalid TypeScript transport:",transport)
  return 2
 }

 f,err:=os.CreateTemp("","rodlog-ts-*.mjs")
 if err!=nil { fmt.Fprintln(stderr,err); return 2 }
 defer os.Remove(f.Name())
 _,err=f.Write(typescriptAdapter)
 closeErr:=f.Close()
 if err!=nil || closeErr!=nil { fmt.Fprintln(stderr,"could not write TypeScript adapter"); return 2 }

 mode := "transform"
 color := ""
 if transport=="jsonl" {
  // The embedded adapter already emits one enriched Diagnostic JSON object per
  // line. Raw mode is therefore the lossless transport: no Event envelope,
  // no terminal styling, and no information discarded.
  mode = "raw"
  color = "--color=never"
 }

 command:=[]string{"exec"}
 command=append(command,flags...)
 // Transport invariants are appended last so duplicate user flags cannot
 // accidentally wrap or colorize the machine-readable Diagnostic stream.
 command=append(command,"--tool=typescript","--mode="+mode)
 if color!="" {
  command=append(command,color,"--output=terminal")
 }
 command=append(command,"--","node",f.Name())
 if project!="" { command=append(command,project) }
 return run(command,stdin,stdout,stderr)
}
