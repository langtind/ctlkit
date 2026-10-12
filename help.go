package ctlkit

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func (p *Program) help(ctx context.Context, c *Call) error {
	if len(c.Args) == 0 {
		p.printHelp(p.Stdout)
		return nil
	}
	cmd, rest, err := p.Lookup(c.Args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return Usagef("%q is not a command", strings.Join(c.Args, " "))
	}
	p.printCommandHelp(p.Stdout, cmd)
	return nil
}

func (p *Program) printHelp(w io.Writer) {
	fmt.Fprintf(w, "%s - %s\n\nUsage:\n", p.Name, p.Summary)
	for _, cmd := range p.table() {
		fmt.Fprintf(w, "  %s %s\n      %s\n", p.Name, cmd.shortUsage(), cmd.Summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Run \"%s help <command>\" for the flags of one command.\n", p.Name)
	fmt.Fprintln(w, "Data goes to stdout, notes and errors to stderr. With --json, an error is a JSON document on stdout.")
	fmt.Fprintf(w, "Exit codes: %s.\n", exitCodeList())
}

func exitCodeList() string {
	short := map[Kind]string{
		KindFailure:     "failure",
		KindUsage:       "wrong call",
		KindNotFound:    "not found",
		KindAuth:        "not logged in or not allowed",
		KindConflict:    "conflict",
		KindRateLimit:   "rate limit",
		KindUnavailable: "service unavailable",
	}
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = fmt.Sprintf("%d %s", k.code, short[k.kind])
	}
	return strings.Join(parts, ", ")
}

func (p *Program) printCommandHelp(w io.Writer, cmd *Command) {
	fmt.Fprintf(w, "%[1]s %[2]s - %[3]s\n\nUsage:\n  %[1]s %[4]s\n", p.Name, cmd.Name, cmd.Summary, cmd.usage())
	if cmd.Mutates {
		fmt.Fprintln(w, "\nThis command changes something at the service. Run it with --dry-run first.")
	}
	if len(cmd.Args) > 0 {
		fmt.Fprintln(w, "\nArguments:")
		for _, arg := range cmd.Args {
			fmt.Fprintf(w, "  %s\n      %s\n", arg.usage(), sentence(arg.Help))
		}
	}
	if len(cmd.Flags) > 0 {
		fmt.Fprintln(w, "\nFlags:")
		for _, f := range cmd.Flags {
			fmt.Fprintf(w, "  %s\n      %s\n", f.usage(), f.description())
		}
	}
}
