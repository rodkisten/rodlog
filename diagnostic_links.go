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
 return fileHyperlinkLabel(file, file, line, column, opts)
}

func fileHyperlinkLabel(file, label string, line, column int, opts Options) string {
 if label=="" { label=file }
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
 icons:=resolveDiagnosticIcons(opts.DiagnosticIcons,!opts.ASCIISymbols)
 raw:=renderDiagnosticWithDetail(d,opts.SemanticColors,icons,opts.SyntaxTheme,opts.Detail)
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
 rendered:=string(raw)
 label:=formatDiagnosticLocation(d)
 displayFile:=d.DisplayFile
 if displayFile=="" { displayFile=d.File }
 linkLabel:=displayFile
 if d.Line>0 { linkLabel+=fmt.Sprintf(":%d",d.Line); if d.Column>0 { linkLabel+=fmt.Sprintf(":%d",d.Column) } }
 link:=fileHyperlinkLabel(d.File,linkLabel,d.Line,d.Column,opts)
 if label!=link { rendered=strings.Replace(rendered,label,link,1) }
 for _,related:=range d.Related {
  if related==nil || related.File=="" { continue }
  relatedLabel:=formatDiagnosticLocation(related)
  display:=related.DisplayFile
  if display=="" { display=related.File }
  if related.Line>0 { display+=fmt.Sprintf(":%d",related.Line); if related.Column>0 { display+=fmt.Sprintf(":%d",related.Column) } }
  relatedLink:=fileHyperlinkLabel(related.File,display,related.Line,related.Column,opts)
  if relatedLabel!=relatedLink { rendered=strings.Replace(rendered,relatedLabel,relatedLink,1) }
 }
 return []byte(rendered)
}
