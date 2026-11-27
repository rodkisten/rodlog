package main

import (
 "embed"
 "fmt"
 "io"
 "strings"
)

//go:embed demos/*.log
var demos embed.FS

func runDemo(args []string, stdout, stderr io.Writer) int {
 names := []string{"rust", "typescript", "go", "npm", "pnpm", "docker"}
 name := "all"
 if len(args)>0 { name=args[0]; args=args[1:] }
 if name=="list" { fmt.Fprintln(stdout, strings.Join(names,"\n")); return 0 }
 found := false
 for _, tool := range names {
  if name!="all" && name!=tool { continue }
  found=true
  data, err := demos.ReadFile("demos/"+tool+".log")
  if err!=nil { fmt.Fprintln(stderr,err); return 1 }
  fmt.Fprintf(stdout,"\n=== %s ===\n",tool)
  mode:="transform"
  if tool=="docker" || tool=="npm" { mode="enhance" }
  flags:=append([]string{"--tool="+tool,"--mode="+mode,"--color=always"},args...)
  if code:=run(flags,strings.NewReader(string(data)),stdout,stderr); code!=0 { return code }
 }
 if !found { fmt.Fprintln(stderr,"usage: rodlog demo [all|list|rust|typescript|go|npm|pnpm|docker] [flags]"); return 2 }
 return 0
}
