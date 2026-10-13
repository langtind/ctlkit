package ctlkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Call is one run of one command: what it was called with, and the streams
// it answers on.
type Call struct {
	*Input
	Program *Program
	Command *Command
	Stdout  io.Writer
	Stderr  io.Writer
}

// JSON reports whether the caller asked for JSON.
func (c *Call) JSON() bool { return c.Has("json") && c.Bool("json") }

// DryRun reports whether the caller asked to see the change without making it.
func (c *Call) DryRun() bool { return c.Has("dry-run") && c.Bool("dry-run") }

// Print writes the result of a data command: v as one JSON document with
// --json, and otherwise whatever render writes, which is Markdown.
func (c *Call) Print(v any, render func(w io.Writer)) error {
	if !c.Has("json") {
		return fmt.Errorf("ctlkit: %q prints data but has no --json flag in its table entry", c.Command.Name)
	}
	fields := c.Has("fields") && c.Given("fields")
	if !c.JSON() {
		if fields {
			return Usagef("--fields selects from the JSON document, so it needs --json")
		}
		render(c.Stdout)
		return nil
	}
	v = emptyList(v)
	if fields {
		kept, err := project(v, c.List("fields"))
		if err != nil {
			return err
		}
		v = kept
	}
	return c.PrintJSON(v)
}

// PrintJSON writes v as the one JSON document a --json run puts on stdout.
// An empty list is [], never null.
func (c *Call) PrintJSON(v any) error {
	return writeJSON(c.Stdout, emptyList(v))
}

func emptyList(v any) any {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.IsNil() {
		return []any{}
	}
	return v
}

// Note writes a line that is not data. It goes to stderr so that stdout
// stays clean for whoever parses it.
func (c *Call) Note(format string, args ...any) {
	fmt.Fprintf(c.Stderr, "Note: "+format+"\n", args...)
}

// Interactive reports whether a person can answer a prompt.
func (c *Call) Interactive() bool { return c.Program.Interactive }

// ReadLine reads one line from stdin.
func (c *Call) ReadLine() (string, error) { return c.Program.readLine() }

// ReadAll reads the rest of stdin, for input that is longer than a line,
// such as a browser's storage state.
func (c *Call) ReadAll() ([]byte, error) { return c.Program.readAll() }

// Confirm asks before a change is made. --yes answers for the caller. Without
// --yes and without a terminal it refuses, because a prompt nobody can answer
// would hang.
func (c *Call) Confirm(question string) error {
	if c.Has("yes") && c.Bool("yes") {
		return nil
	}
	if !c.Program.Interactive {
		return Usagef("refusing without confirmation: %s", question).WithHint("pass --yes to confirm, or --dry-run to see the change first.")
	}
	fmt.Fprintf(c.Stderr, "%s [y/N] ", question)
	answer, err := c.ReadLine()
	if err != nil {
		return fmt.Errorf("read answer: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return errors.New("cancelled")
}

// Ask prompts for a value that was not given as a flag. Without a terminal
// it names the flag to use.
func (c *Call) Ask(label, flag string) (string, error) {
	if !c.Program.Interactive {
		return "", Usagef("--%s is required when not run in a terminal", flag)
	}
	fmt.Fprintf(c.Stderr, "%s: ", label)
	line, err := c.ReadLine()
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.ToLower(label), err)
	}
	return strings.TrimSpace(line), nil
}

// Secret reads a password, a token or a key: one line from stdin when the
// bool flag stdinFlag is set, and otherwise a prompt without echo. A secret
// never comes as an argument, where the process list would show it.
func (c *Call) Secret(label, stdinFlag string) (string, error) {
	var secret string
	var err error
	switch {
	case c.Bool(stdinFlag):
		secret, err = c.ReadLine()
	case c.Program.Interactive:
		fmt.Fprintf(c.Stderr, "%s: ", label)
		secret, err = c.Program.ReadSecret()
		fmt.Fprintln(c.Stderr)
	default:
		return "", Usagef("use --%s when not run in a terminal", stdinFlag)
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.ToLower(label), err)
	}
	if secret = strings.TrimSpace(secret); secret == "" {
		return "", Usagef("the %s must not be empty", strings.ToLower(label))
	}
	return secret, nil
}

// project keeps only the named fields of each item in v. The items are the
// elements of a list, the elements of "items" in an object that has such a
// list, or else the object itself.
func project(v any, fields []string) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var doc any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}

	switch d := doc.(type) {
	case []any:
		return projectItems(d, fields)
	case map[string]any:
		if items, ok := d["items"].([]any); ok {
			kept, err := projectItems(items, fields)
			d["items"] = kept
			return d, err
		}
		kept, err := projectItems([]any{d}, fields)
		if err != nil {
			return nil, err
		}
		return kept[0], nil
	}
	return nil, Usagef("--fields needs a list or an object, and this command prints neither")
}

func projectItems(items []any, fields []string) ([]any, error) {
	known := make(map[string]bool)
	for _, item := range items {
		if object, ok := item.(map[string]any); ok {
			for name := range object {
				known[name] = true
			}
		}
	}
	if len(known) > 0 {
		for _, name := range fields {
			if !known[name] {
				return nil, Usagef("--fields does not know %q, use any of: %s", name, strings.Join(slices.Sorted(maps.Keys(known)), ", "))
			}
		}
	}

	out := make([]any, len(items))
	for i, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			out[i] = item
			continue
		}
		kept := make(map[string]any, len(fields))
		for _, name := range fields {
			if value, ok := object[name]; ok {
				kept[name] = value
			}
		}
		out[i] = kept
	}
	return out, nil
}
