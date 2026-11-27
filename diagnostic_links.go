package rodlog

import (
 "fmt"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "unicode"
)

func diagnosticCount(n int, noun string) string {
 if n!=1 { noun+="s" }
 return fmt.Sprintf("%d %s",n,noun)
}

func fileHyperlink(file string, line, column int, opts Options) string {
 label:=file
 if line>0 { label+=fmt.Sprintf(":%d",line); if column>0 { label+=fmt.Sprintf(":%d",column) } }
 if !opts.Links || file=="" || strings.IndexFunc(file,unicode.IsControl)>=0 { return label }
 abs,err:=filepath.Abs(file)
 if err!=nil { return label }
 if _,err=os.Stat(abs); err!=nil { return label }
 u:=url.URL{Scheme:"file",Path:filepath.ToSlash(abs)}
 switch opts.LinkEditor {
 case "vscode","cursor":
  u.Scheme=opts.LinkEditor
  u.Host="file"
  if line>0 { u.Path+=fmt.Sprintf(":%d:%d",line,max(1,column)) }
 default:
  u.Host,_=os.Hostname()
 }
 return "\x1b]8;;"+u.String()+"\x1b\\"+label+"\x1b]8;;\x1b\\"
}

func renderLinkedDiagnostic(d *Diagnostic, opts Options) []byte {
 raw:=renderDiagnosticWithTheme(d,opts.SemanticColors,opts.DiagnosticIcons,opts.SyntaxTheme)
 if !opts.Links || d==nil { return raw }
 code:=d.Code
 if d.Tool==TypeScript && len(code)>2 && strings.HasPrefix(code,"TS") && strings.Trim(code[2:],"0123456789")=="" {
  label:="["+code+"]"
  u:="https://www.typescriptlang.org/search?q="+url.QueryEscape(code)
  switch code { case "TS2322","TS2345","TS2339","TS2741": u="https://www.typescriptlang.org/docs/handbook/2/objects.html"; case "TS18048","TS18047","TS2532": u="https://www.typescriptlang.org/docs/handbook/2/narrowing.html" }
  // Unknown codes use a GitHub search in the official compiler repository.
  if strings.Contains(u,"/search?") { u="https://github.com/microsoft/TypeScript/search?q="+url.QueryEscape(code)+"&type=issues" }
  raw=[]byte(strings.Replace(string(raw),label,"\x1b]8;;"+u+"\x1b\\"+label+"\x1b]8;;\x1b\\",1))
 }
 if d.File=="" { return raw }
 label:=formatDiagnosticLocation(d)
 link:=fileHyperlink(d.File,d.Line,d.Column,opts)
 if label==link { return raw }
 return []byte(strings.Replace(string(raw),label,link,1))
}
