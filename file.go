package ctlkit

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SavedFile is what a command that fetches a file prints.
type SavedFile struct {
	Path        string `json:"path"`
	Bytes       int    `json:"bytes"`
	ContentType string `json:"contentType"`
}

// extensions names a file that came without an extension, by what it holds.
var extensions = map[string]string{
	"application/pdf": ".pdf", "image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif",
	"image/webp": ".webp", "application/zip": ".zip", "text/plain; charset=utf-8": ".txt",
}

// zipped are the file types that are zip archives inside, which content
// sniffing alone calls application/zip.
var zipped = map[string]string{
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odt":  "application/vnd.oasis.opendocument.text",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".odp":  "application/vnd.oasis.opendocument.presentation",
	".epub": "application/epub+zip",
}

// ContentType says what a saved file holds, from its content, and from its
// extension when the content is a zip archive such as a .docx or an .xlsx.
func ContentType(path string, data []byte) string {
	detected := http.DetectContentType(data)
	if detected == "application/zip" {
		if t, ok := zipped[strings.ToLower(filepath.Ext(path))]; ok {
			return t
		}
	}
	return detected
}

// FileName turns a name a service gave a file into one that is safe to
// write in the current directory: the last element only, whichever way the
// slashes lean. A name without an extension gets one from what data holds.
func FileName(name string, data []byte) string {
	name = filepath.Base(filepath.Clean("/" + strings.ReplaceAll(name, `\`, "/")))
	if name == "/" || name == "." || checkControl([]string{name}) != nil || strings.ContainsAny(name, "\n\t") {
		name = "download"
	}
	if filepath.Ext(name) == "" {
		name += extensions[http.DetectContentType(data)]
	}
	return name
}

// Save writes a fetched file and prints where it went. The command takes
// OutputFlag and JSONFlag. Without --output the file gets name, cut down
// with FileName, in the current directory. A file that exists is never
// replaced.
func (c *Call) Save(data []byte, name string) error {
	path := ""
	if c.Has("output") {
		path = c.String("output")
	}
	if path == "" {
		path = FileName(name, data)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return Errorf(KindConflict, "%s exists already", path).WithHint("give another path with --output.")
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	saved := SavedFile{Path: path, Bytes: len(data), ContentType: ContentType(path, data)}
	return c.Print(saved, func(w io.Writer) {
		fmt.Fprintf(w, "Saved %s (%d bytes, %s).\n", saved.Path, saved.Bytes, saved.ContentType)
	})
}

// Quote returns text from a service as a Markdown block quote, so that a
// heading or an instruction in it cannot pass for the tool's own output.
func Quote(text string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n")), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight("> "+line, " ")
	}
	return strings.Join(lines, "\n")
}
