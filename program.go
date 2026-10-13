// Package ctlkit is the command line that every *ctl tool is built on. A tool
// fills in a table of commands and gets the rest: flag parsing, help, the
// schema command, error kinds with their exit codes, JSON output, field
// selection, confirmation, input checks and the skill installer.
//
// CONVENTIONS.md describes the contract the tools share. The ctltest package
// holds a tool to it.
package ctlkit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// Build holds the version information injected at build time.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// Program is one tool: its name, its commands, and the streams it talks on.
type Program struct {
	Name string
	// Summary completes "<name> - " on the first line of help.
	Summary string
	// URL is the project page. Services see it in the User-Agent.
	URL   string
	Build Build

	// Commands is the tool's own commands. schema, install-skill, version
	// and help are added to them.
	Commands []Command

	// Skill holds the agent skill files that install-skill copies out.
	Skill fs.FS

	// Hint says what to do about an error that cannot say it itself.
	Hint func(err error) string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Interactive is true when a person can answer a prompt.
	Interactive bool
	ReadSecret  func() (string, error)

	reader *bufio.Reader
}

// New returns a program that talks on the process's own streams.
func New(name, summary string, build Build) *Program {
	return &Program{
		Name:        name,
		Summary:     summary,
		Build:       build,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: term.IsTerminal(int(os.Stdin.Fd())),
		ReadSecret: func() (string, error) {
			secret, err := term.ReadPassword(int(os.Stdin.Fd()))
			return string(secret), err
		},
	}
}

// UserAgent identifies the program to the services it calls.
func (p *Program) UserAgent() string {
	agent := p.Name + "/" + p.Build.Version
	if p.URL != "" {
		agent += " (+" + p.URL + ")"
	}
	return agent
}

// Main runs the program with the process's arguments and returns the exit code.
func (p *Program) Main() int { return p.Run(os.Args[1:]) }

// Run executes the program with args (without the program name) and returns
// the exit code.
func (p *Program) Run(args []string) int {
	if len(args) == 0 {
		p.printHelp(p.Stderr)
		return ExitCode(KindUsage)
	}
	switch args[0] {
	case "-h", "--help":
		args = []string{"help"}
	case "-v", "--version":
		args = []string{"version"}
	}
	asJSON := wantsJSON(args)

	cmd, rest, err := p.Lookup(args)
	if err != nil {
		code := p.fail(nil, asJSON, err)
		if !asJSON {
			fmt.Fprintln(p.Stderr)
			p.printHelp(p.Stderr)
		}
		return code
	}

	in, err := cmd.parse(rest)
	if errors.Is(err, errHelp) {
		p.printCommandHelp(p.Stdout, cmd)
		return 0
	}
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		err = cmd.Run(ctx, &Call{Input: in, Program: p, Command: cmd, Stdout: p.Stdout, Stderr: p.Stderr})
	}
	if err == nil {
		return 0
	}
	return p.fail(cmd, asJSON, err)
}

// wantsJSON reports whether the caller asked for JSON. It reads the raw
// arguments, because the error to report may be that they do not parse.
func wantsJSON(args []string) bool {
	return slices.ContainsFunc(args, func(arg string) bool {
		name, value, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		return strings.HasPrefix(arg, "-") && name == "json" && value != "false"
	})
}

type errorDocument struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Kind       Kind   `json:"kind"`
	Message    string `json:"message"`
	Hint       string `json:"hint,omitempty"`
	RetryAfter string `json:"retryAfter,omitempty"`
	Usage      string `json:"usage,omitempty"`
}

// fail reports err and returns its exit code. With --json the report is the
// one JSON document on stdout, and stderr stays empty.
func (p *Program) fail(cmd *Command, asJSON bool, err error) int {
	e := p.classify(err)
	usage := ""
	if e.Kind == KindUsage && cmd != nil {
		usage = p.Name + " " + cmd.usage()
	}

	if asJSON {
		body := errorBody{Kind: e.Kind, Message: e.Message, Hint: e.Hint, RetryAfter: e.RetryAfter, Usage: usage}
		if encodeErr := writeJSON(p.Stdout, errorDocument{body}); encodeErr != nil {
			fmt.Fprintf(p.Stderr, "Error: %v\n", encodeErr)
		}
		return ExitCode(e.Kind)
	}

	fmt.Fprintf(p.Stderr, "Error: %s\n", e.Message)
	if usage != "" {
		fmt.Fprintf(p.Stderr, "Usage: %s\n", usage)
	}
	if e.Hint != "" {
		fmt.Fprintf(p.Stderr, "Hint: %s\n", e.Hint)
	}
	return ExitCode(e.Kind)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// table is the tool's commands followed by the ones every tool has.
func (p *Program) table() []Command {
	return append(slices.Clone(p.Commands), p.builtins()...)
}

func (p *Program) builtins() []Command {
	return []Command{
		{
			Name:    "schema",
			Summary: "Print every command, flag and error kind as JSON",
			Run:     p.schema,
		},
		{
			Name:    "install-skill",
			Summary: "Install the agent skill to ~/.claude/skills",
			Flags: []Flag{
				{Name: "path", Kind: String, Value: "dir", Help: "skills directory to install into instead of ~/.claude/skills"},
			},
			Run: p.installSkill,
		},
		{
			Name:    "version",
			Summary: "Show version information",
			Run:     p.version,
		},
		{
			Name:    "help",
			Summary: "Show help for " + p.Name + " or for one command",
			Args:    []Arg{{Name: "command", Variadic: true, Help: "the command to describe"}},
			Run:     p.help,
		},
	}
}

// Lookup finds the command that args name and returns the arguments after it.
func (p *Program) Lookup(args []string) (*Command, []string, error) {
	if len(args) == 0 {
		return nil, nil, Usagef("no command given")
	}
	if strings.HasPrefix(args[0], "-") {
		return nil, nil, Usagef("%s comes before the command, and flags go after it", args[0])
	}
	table := p.table()
	find := func(name string) *Command {
		for i := range table {
			if table[i].Name == name {
				return &table[i]
			}
		}
		return nil
	}
	if len(args) >= 2 {
		if cmd := find(args[0] + " " + args[1]); cmd != nil {
			return cmd, args[2:], nil
		}
	}
	if cmd := find(args[0]); cmd != nil {
		return cmd, args[1:], nil
	}

	var subcommands []string
	for _, cmd := range table {
		if group, sub, ok := strings.Cut(cmd.Name, " "); ok && group == args[0] {
			subcommands = append(subcommands, sub)
		}
	}
	if len(subcommands) > 0 {
		return nil, nil, Usagef("%q needs one of the subcommands %s", args[0], strings.Join(subcommands, ", "))
	}
	return nil, nil, Usagef("unknown command %q", args[0])
}

// Parse finds the command that args name and parses the rest against its
// table entry, without running it.
func (p *Program) Parse(args []string) (*Command, *Input, error) {
	cmd, rest, err := p.Lookup(args)
	if err != nil {
		return nil, nil, err
	}
	in, err := cmd.parse(rest)
	return cmd, in, err
}

// Table returns every command the program takes, its own and the built-in.
func (p *Program) Table() []Command { return p.table() }

func (p *Program) version(ctx context.Context, c *Call) error {
	fmt.Fprintf(p.Stdout, "%s version %s\n", p.Name, p.Build.Version)
	if p.Build.Commit != "" && p.Build.Commit != "unknown" {
		fmt.Fprintf(p.Stdout, "commit: %s\n", p.Build.Commit)
	}
	if p.Build.Date != "" && p.Build.Date != "unknown" {
		fmt.Fprintf(p.Stdout, "built: %s\n", p.Build.Date)
	}
	return nil
}

func (p *Program) readAll() ([]byte, error) {
	if p.reader == nil {
		p.reader = bufio.NewReader(p.Stdin)
	}
	return io.ReadAll(p.reader)
}

func (p *Program) readLine() (string, error) {
	if p.reader == nil {
		p.reader = bufio.NewReader(p.Stdin)
	}
	line, err := p.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
