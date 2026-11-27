package main

import (
 _ "embed"
 "fmt"
 "io"
 "os"
)

//go:embed typescript-intelligence.mjs
var typescriptAdapter []byte

func runTypeScript(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
 project := ""
 flags := []string{}
 for i:=0;i<len(args);i++ {
  switch args[i] {
  case "--project","-p":
   if i+1>=len(args) { fmt.Fprintln(stderr,"missing project path"); return 2 }
   i++; project=args[i]
  case "--help","-h":
   fmt.Fprintln(stdout,"Usage: rodlog tsc [-p tsconfig.json] [rodlog flags]"); return 0
  default: flags=append(flags,args[i])
  }
 }
 f,err:=os.CreateTemp("","rodlog-ts-*.mjs")
 if err!=nil { fmt.Fprintln(stderr,err); return 2 }
 defer os.Remove(f.Name())
 _,err=f.Write(typescriptAdapter)
 closeErr:=f.Close()
 if err!=nil || closeErr!=nil { fmt.Fprintln(stderr,"could not write TypeScript adapter"); return 2 }
 command:=append([]string{"exec","--tool=typescript","--mode=transform"},flags...)
 command=append(command,"--","node",f.Name())
 if project!="" { command=append(command,project) }
 return run(command,stdin,stdout,stderr)
}
