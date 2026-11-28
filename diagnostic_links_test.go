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


func TestRenderLinkedDiagnosticDoesNotDuplicateLocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.ts")
	if err := os.WriteFile(path, []byte("const value = 1\n"), 0600); err != nil { t.Fatal(err) }
	d := &Diagnostic{
		Tool: TypeScript, Level: Error, Code: "TS2322",
		File: path, DisplayFile: "src/a.ts", Line: 2, Column: 3,
		Message: "Type 'number' is not assignable to type 'string'.",
	}
	got := string(renderLinkedDiagnostic(d, Options{Links:true, LinkEditor:"vscode"}))
	if strings.Contains(got, "src/a.ts:2:3:2:3") {
		t.Fatalf("location duplicated in hyperlink label: %q", got)
	}
	if !strings.Contains(got, "src/a.ts:2:3") {
		t.Fatalf("location missing from hyperlink label: %q", got)
	}
}
