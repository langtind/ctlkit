package httpx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/langtind/ctlkit"
)

func TestReadFilesChecksBeforeAnythingIsSent(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	pdf := write("letter, signed.pdf", "%PDF-1.7 made up")
	text := write("note.txt", "hello")
	big := write("big.pdf", "%PDF-1.7 "+strings.Repeat("x", 100))
	limits := FileLimits{Max: 2, MaxBytes: 50, Types: []string{"application/pdf", "image/png"}}

	files, err := ReadFiles("attach", []string{pdf}, limits)
	if err != nil || len(files) != 1 || files[0].Name != "letter, signed.pdf" || files[0].ContentType != "application/pdf" {
		t.Fatalf("files = %+v, err = %v", files, err)
	}

	for want, paths := range map[string][]string{
		"--attach takes at most 2 files, not 3":  {pdf, pdf, pdf},
		"note.txt holds text/plain":              {text},
		"big.pdf is 109 bytes":                   {big},
		"missing.pdf: no such file or directory": {filepath.Join(dir, "missing.pdf")},
		"is a directory":                         {dir},
	} {
		_, err := ReadFiles("attach", paths, limits)
		if ctlkit.KindOf(err) != ctlkit.KindUsage || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
	if files, err := ReadFiles("attach", nil, limits); err != nil || len(files) != 0 {
		t.Errorf("no paths: %v, %v", files, err)
	}
}
