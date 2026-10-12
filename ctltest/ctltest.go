// Package ctltest holds a tool to the contract in CONVENTIONS.md. A tool
// calls Contract from one test and gets every check that does not depend on
// what the tool does.
package ctltest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/langtind/ctlkit"
)

// Run runs p with args, with stdin as its input and no terminal, and returns
// the exit code and what it wrote.
func Run(p *ctlkit.Program, stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	p.Stdin, p.Stdout, p.Stderr = strings.NewReader(stdin), &out, &errOut
	code = p.Run(args)
	return code, out.String(), errOut.String()
}

// OneJSON fails unless s is one JSON value and nothing else, which is what
// --json promises on stdout. The value is decoded into into.
func OneJSON(t *testing.T, s string, into any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	if err := dec.Decode(into); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, s)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout holds more than one JSON document:\n%s", s)
	}
}

// ErrorDocument is what a failed --json run puts on stdout.
type ErrorDocument struct {
	Error struct {
		Kind       string `json:"kind"`
		Message    string `json:"message"`
		Hint       string `json:"hint"`
		RetryAfter string `json:"retryAfter"`
		Usage      string `json:"usage"`
	} `json:"error"`
}

type schemaDocument struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	ExitCodes  map[string]string `json:"exitCodes"`
	ErrorKinds []struct {
		Kind     string `json:"kind"`
		ExitCode int    `json:"exitCode"`
	} `json:"errorKinds"`
	Commands []struct {
		Name    string `json:"name"`
		Usage   string `json:"usage"`
		Summary string `json:"summary"`
		Mutates bool   `json:"mutates"`
		Args    []struct {
			Name string `json:"name"`
			Help string `json:"help"`
		} `json:"args"`
		Flags []struct {
			Name string   `json:"name"`
			Type string   `json:"type"`
			Enum []string `json:"enum"`
			Help string   `json:"help"`
		} `json:"flags"`
	} `json:"commands"`
}

var (
	commandName = regexp.MustCompile(`^[a-z][a-z0-9-]*( [a-z][a-z0-9-]*)?$`)
	flagName    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Contract checks everything the tools share. build returns a new program
// for each run. It must work with no login, no config and no network, since
// no check gets as far as calling a service. docs are Markdown files whose
// code blocks teach commands, such as the skill and the README.
func Contract(t *testing.T, build func() *ctlkit.Program, docs ...string) {
	t.Helper()
	name := build().Name
	table := build().Table()

	t.Run("the table is well formed", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, cmd := range table {
			if !commandName.MatchString(cmd.Name) {
				t.Errorf("command name %q: use one or two lower-case words", cmd.Name)
			}
			if seen[cmd.Name] {
				t.Errorf("command %q is in the table twice", cmd.Name)
			}
			seen[cmd.Name] = true
			if cmd.Summary == "" || cmd.Run == nil {
				t.Errorf("%s: needs a summary and a run function", cmd.Name)
			}
			for i, arg := range cmd.Args {
				if arg.Name == "" || arg.Help == "" {
					t.Errorf("%s: argument %d needs a name and help", cmd.Name, i)
				}
				if arg.Variadic && i != len(cmd.Args)-1 {
					t.Errorf("%s: only the last argument can be variadic", cmd.Name)
				}
			}
			for _, f := range cmd.Flags {
				if !flagName.MatchString(f.Name) || f.Help == "" {
					t.Errorf("%s --%s: needs a lower-case name and help", cmd.Name, f.Name)
				}
				if f.Kind != ctlkit.Bool && f.Value == "" && len(f.Enum) == 0 {
					t.Errorf("%s --%s: name its value, or list what it takes", cmd.Name, f.Name)
				}
			}
		}
	})

	t.Run("a command that changes something takes --dry-run and --yes", func(t *testing.T) {
		for _, cmd := range table {
			has := func(name string) bool {
				return slices.ContainsFunc(cmd.Flags, func(f ctlkit.Flag) bool { return f.Name == name && f.Kind == ctlkit.Bool })
			}
			if cmd.Mutates && !(has("dry-run") && has("yes")) {
				t.Errorf("%s changes something at the service, so it needs ctlkit.DryRunFlag and ctlkit.YesFlag", cmd.Name)
			}
			if !cmd.Mutates && has("dry-run") {
				t.Errorf("%s takes --dry-run, so mark it Mutates", cmd.Name)
			}
		}
	})

	t.Run("no arguments and an unknown command are usage errors", func(t *testing.T) {
		code, stdout, stderr := Run(build(), "")
		if code != 2 || stdout != "" || !strings.HasPrefix(stderr, name+" - ") {
			t.Errorf("no arguments: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		code, stdout, stderr = Run(build(), "", "no-such-command")
		if code != 2 || stdout != "" || !strings.Contains(stderr, `unknown command "no-such-command"`) {
			t.Errorf("unknown command: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})

	t.Run("help lists every command, and every command has help", func(t *testing.T) {
		for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
			code, stdout, stderr := Run(build(), "", args...)
			if code != 0 || stderr != "" {
				t.Fatalf("%v: exit = %d, stderr = %q", args, code, stderr)
			}
			for _, cmd := range table {
				if !strings.Contains(stdout, "  "+name+" "+cmd.Name) || !strings.Contains(stdout, "      "+cmd.Summary+"\n") {
					t.Errorf("%v: help does not list %s", args, cmd.Name)
				}
			}
		}
		for _, cmd := range table {
			words := strings.Fields(cmd.Name)
			for _, args := range [][]string{append([]string{"help"}, words...), append(slices.Clone(words), "--help"), append(slices.Clone(words), "-h")} {
				code, stdout, stderr := Run(build(), "", args...)
				if code != 0 || stderr != "" || !strings.HasPrefix(stdout, name+" "+cmd.Name+" - "+cmd.Summary+"\n") {
					t.Errorf("%v: exit = %d, stdout = %q, stderr = %q", args, code, stdout, stderr)
				}
				for _, f := range cmd.Flags {
					if !strings.Contains(stdout, "\n  --"+f.Name) {
						t.Errorf("%v: help does not describe --%s", args, f.Name)
					}
				}
			}
		}
	})

	t.Run("schema describes the table the dispatcher uses", func(t *testing.T) {
		code, stdout, stderr := Run(build(), "", "schema")
		if code != 0 || stderr != "" {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		var schema schemaDocument
		OneJSON(t, stdout, &schema)
		if schema.Name != name || schema.ExitCodes["0"] == "" || len(schema.ErrorKinds) == 0 || len(schema.ExitCodes) != len(schema.ErrorKinds)+1 {
			t.Errorf("schema header = %+v", schema)
		}
		if len(schema.Commands) != len(table) {
			t.Fatalf("schema lists %d commands, the table has %d", len(schema.Commands), len(table))
		}
		for i, entry := range schema.Commands {
			cmd := table[i]
			if entry.Name != cmd.Name || entry.Summary != cmd.Summary || entry.Mutates != cmd.Mutates || !strings.HasPrefix(entry.Usage, name+" "+cmd.Name) {
				t.Errorf("command %d = %+v, want %s", i, entry, cmd.Name)
			}
			if entry.Args == nil || entry.Flags == nil || len(entry.Args) != len(cmd.Args) || len(entry.Flags) != len(cmd.Flags) {
				t.Errorf("%s: args = %v, flags = %v", cmd.Name, entry.Args, entry.Flags)
				continue
			}
			for j, f := range entry.Flags {
				if f.Name != cmd.Flags[j].Name || f.Type != string(cmd.Flags[j].Kind) || !slices.Equal(f.Enum, cmd.Flags[j].Enum) || f.Help == "" {
					t.Errorf("%s --%s = %+v", cmd.Name, cmd.Flags[j].Name, f)
				}
			}
		}
	})

	// Every value the schema offers must be one the dispatcher takes, and
	// the other way round.
	t.Run("every advertised flag is accepted, and no other", func(t *testing.T) {
		for _, cmd := range table {
			args := strings.Fields(cmd.Name)
			for _, arg := range cmd.Args {
				if arg.Required {
					args = append(args, "x")
				}
			}
			for _, f := range cmd.Flags {
				switch {
				case f.Kind == ctlkit.Bool:
					args = append(args, "--"+f.Name)
				case len(f.Enum) > 0:
					args = append(args, "--"+f.Name, f.Enum[len(f.Enum)-1])
				case f.Kind == ctlkit.Int:
					args = append(args, "--"+f.Name, "7")
				default:
					args = append(args, "--"+f.Name, "x")
				}
			}
			if _, _, err := build().Parse(args); err != nil {
				t.Errorf("%s does not parse its own flags %v: %v", cmd.Name, args, err)
			}
			_, _, err := build().Parse(append(args, "--not-in-the-table"))
			if ctlkit.KindOf(err) != ctlkit.KindUsage || !strings.Contains(err.Error(), "unknown flag --not-in-the-table") {
				t.Errorf("%s with a flag that is not in the table: %v", cmd.Name, err)
			}
		}
	})

	t.Run("with --json an error is one JSON document on stdout", func(t *testing.T) {
		for _, cmd := range table {
			if !slices.ContainsFunc(cmd.Flags, func(f ctlkit.Flag) bool { return f.Name == "json" }) {
				continue
			}
			args := append(strings.Fields(cmd.Name), "--json", "--not-in-the-table")
			code, stdout, stderr := Run(build(), "", args...)
			var doc ErrorDocument
			OneJSON(t, stdout, &doc)
			if code != 2 || stderr != "" || doc.Error.Kind != "usage" || doc.Error.Message == "" || !strings.HasPrefix(doc.Error.Usage, name+" "+cmd.Name) {
				t.Errorf("%v: exit = %d, stderr = %q, error = %+v", args, code, stderr, doc.Error)
			}
		}
	})

	t.Run("a control character is refused", func(t *testing.T) {
		code, _, stderr := Run(build(), "", "help", "a\x00b")
		if code != 2 || !strings.Contains(stderr, "control character") {
			t.Errorf("exit = %d, stderr = %q", code, stderr)
		}
	})

	t.Run("version", func(t *testing.T) {
		p := build()
		for _, arg := range []string{"version", "--version", "-v"} {
			code, stdout, _ := Run(p, "", arg)
			if code != 0 || !strings.HasPrefix(stdout, name+" version "+p.Build.Version+"\n") {
				t.Errorf("%s: exit = %d, stdout = %q", arg, code, stdout)
			}
		}
	})

	t.Run("install-skill replaces an older copy", func(t *testing.T) {
		p := build()
		if p.Skill == nil {
			t.Fatal("the program has no skill to install")
		}
		dir := t.TempDir()
		target := filepath.Join(dir, name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("an older copy"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := Run(p, "", "install-skill", "--path", dir)
		if code != 0 || !strings.Contains(stdout, "Installed ") {
			t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) == "an older copy" || !strings.HasPrefix(string(got), "---\nname: "+name+"\n") {
			t.Errorf("SKILL.md was not replaced by a skill named %s: %v\n%.200s", name, err, got)
		}
	})

	// The skill and the README teach commands. This holds them to the
	// table, so a renamed command or flag cannot leave either one teaching
	// something that no longer exists.
	for _, doc := range docs {
		t.Run("the commands in "+filepath.Base(doc)+" exist", func(t *testing.T) {
			data, err := os.ReadFile(doc)
			if err != nil {
				t.Fatal(err)
			}
			calls := Invocations(name, string(data))
			if len(calls) == 0 {
				t.Fatalf("no %s commands found in the code blocks", name)
			}
			for _, args := range calls {
				cmd, rest, err := build().Lookup(args)
				if err != nil {
					t.Errorf("%s %s: %v", name, strings.Join(args, " "), err)
					continue
				}
				for _, word := range rest {
					flag, _, _ := strings.Cut(word, "=")
					if !documentedFlag.MatchString(flag) {
						continue
					}
					if !slices.ContainsFunc(cmd.Flags, func(f ctlkit.Flag) bool { return "--"+f.Name == flag }) {
						t.Errorf("%s %s: %s has no flag %s", name, strings.Join(args, " "), cmd.Name, flag)
					}
				}
			}
		})
	}
}

var (
	codeBlock      = regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")
	documentedFlag = regexp.MustCompile(`^--[a-z][a-z0-9-]*$`)
)

// Invocations returns every call of the program in the fenced code blocks of
// a Markdown text, as argument lists. The program counts as called when it
// starts a command: at the start of a line, after a pipe or another
// separator, or after an environment assignment. "go build -o name ." and
// "cd name" do not count.
func Invocations(name, markdown string) [][]string {
	invocation := regexp.MustCompile(`(?:^|[|;&(]|[A-Z_][A-Z0-9_]*=\S*)\s*` + regexp.QuoteMeta(name) + `((?:\s+[^\s|;&)#]+)*)`)
	var calls [][]string
	for _, block := range codeBlock.FindAllStringSubmatch(markdown, -1) {
		for _, line := range strings.Split(block[1], "\n") {
			for _, match := range invocation.FindAllStringSubmatch(line, -1) {
				if args := strings.Fields(match[1]); len(args) > 0 {
					calls = append(calls, args)
				}
			}
		}
	}
	return calls
}
