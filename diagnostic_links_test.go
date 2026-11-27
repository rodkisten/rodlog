package rodlog

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestFileHyperlink(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"a b.ts")
 if err:=os.WriteFile(path,[]byte("const a=1"),0600); err!=nil { t.Fatal(err) }
 got:=fileHyperlink(path,2,3,Options{Links:true,LinkEditor:"vscode"})
 if !strings.Contains(got,"\x1b]8;;vscode://file/") || !strings.Contains(got,"a%20b.ts:2:3") { t.Fatal(got) }
 if got:=fileHyperlink(path,2,3,Options{}); got!=path+":2:3" { t.Fatal(got) }
}
