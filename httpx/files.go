package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/langtind/ctlkit"
)

// FileLimits is what a service takes as files. A zero field means no limit.
// Read the limits out of the service's own client: the service may not
// check them itself.
type FileLimits struct {
	// Max is the highest number of files.
	Max int
	// MaxBytes is the largest one file may be.
	MaxBytes int64
	// Types lists the content types that are taken, such as
	// "application/pdf". The type is read from what the file holds, and
	// not from its name.
	Types []string
}

// ReadFiles reads the files a flag named, and checks them against limits
// before anything is sent. flag names the flag in the errors. Each file
// gets its last name element only, so that a path on this machine is never
// told to the service.
func ReadFiles(flag string, paths []string, limits FileLimits) ([]FormFile, error) {
	if limits.Max > 0 && len(paths) > limits.Max {
		return nil, ctlkit.Usagef("--%s takes at most %d files, not %d", flag, limits.Max, len(paths))
	}
	files := make([]FormFile, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		switch {
		case err != nil:
			return nil, ctlkit.Usagef("--%s: cannot read %s: %v", flag, path, pathError(err))
		case info.IsDir():
			return nil, ctlkit.Usagef("--%s: %s is a directory", flag, path)
		case limits.MaxBytes > 0 && info.Size() > limits.MaxBytes:
			return nil, ctlkit.Usagef("--%s: %s is %s, and the most that is taken is %s", flag, path, size(info.Size()), size(limits.MaxBytes))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, ctlkit.Usagef("--%s: cannot read %s: %v", flag, path, pathError(err))
		}
		contentType, _, _ := strings.Cut(http.DetectContentType(data), ";")
		if len(limits.Types) > 0 && !slices.Contains(limits.Types, contentType) {
			return nil, ctlkit.Usagef("--%s: %s holds %s, and what is taken is %s", flag, path, contentType, strings.Join(limits.Types, ", "))
		}
		files = append(files, FormFile{Name: filepath.Base(path), ContentType: contentType, Data: data})
	}
	return files, nil
}

// pathError drops the path from an os error, which the message names already.
func pathError(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func size(bytes int64) string {
	if bytes >= 1<<20 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	}
	return fmt.Sprintf("%d bytes", bytes)
}
