package ctlkit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// installSkill copies the embedded agent skill into a skills directory.
// Reinstalling replaces the files, which is how the skill follows an upgrade.
func (p *Program) installSkill(ctx context.Context, c *Call) error {
	if p.Skill == nil {
		return errors.New("this build has no skill embedded")
	}

	parent := c.String("path")
	if parent == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("find home directory: %w", err)
		}
		parent = filepath.Join(home, ".claude", "skills")
	}
	dest := filepath.Join(parent, p.Name)

	count := 0
	err := fs.WalkDir(p.Skill, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(p.Skill, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		count++
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return fmt.Errorf("install skill: %w", err)
	}
	fmt.Fprintf(p.Stdout, "Installed %d file(s) to %s\n", count, dest)
	return nil
}
