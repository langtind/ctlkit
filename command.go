package ctlkit

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// The command table is the single description of the command line. The
// dispatcher parses flags from it, help prints it, and the schema command
// publishes it, so the three cannot drift apart.

type FlagKind string

const (
	Bool   FlagKind = "bool"
	String FlagKind = "string"
	Int    FlagKind = "int"
	// List is a string flag that can be repeated and also takes a
	// comma-separated list.
	List FlagKind = "list"
	// Repeat is a string flag that can be repeated, and whose value is
	// taken whole. Use it for a path or free text, where a comma is part
	// of the value.
	Repeat FlagKind = "repeat"
)

type Flag struct {
	Name string
	Kind FlagKind
	// Value names the value in usage text, such as "yyyy-mm-dd".
	Value string
	// Default is the default as text. Empty means the flag has none.
	Default string
	// Enum lists the accepted values. Input is matched in lower case.
	Enum []string
	// Help is a fragment: lower case first, no full stop.
	Help string
}

type Arg struct {
	Name     string
	Required bool
	// Variadic lets the last argument take any number of words.
	Variadic bool
	// ID marks an argument that names one thing at the service. It is
	// checked with CheckID before the command runs.
	ID bool
	// Help is a fragment: lower case first, no full stop.
	Help string
}

type Command struct {
	// Name is one word, or two for a subcommand such as "watch add".
	Name    string
	Summary string
	// Long is help that does not fit in the summary: full sentences, shown
	// by "help <command>" and in the schema. Put the steps of a login that
	// needs a browser here, and not in a hint.
	Long  string
	Args  []Arg
	Flags []Flag
	// Mutates is true when the command changes something at the service.
	// Such a command takes DryRunFlag and YesFlag.
	Mutates bool
	Run     func(ctx context.Context, c *Call) error
}

// The flags below mean the same in every tool.
var (
	JSONFlag   = Flag{Name: "json", Kind: Bool, Help: "print one JSON document instead of Markdown"}
	FieldsFlag = Flag{Name: "fields", Kind: List, Value: "field", Help: "with --json, keep only these fields of each item"}
	YesFlag    = Flag{Name: "yes", Kind: Bool, Help: "make the change without asking for confirmation"}
	DryRunFlag = Flag{Name: "dry-run", Kind: Bool, Help: "show what would be sent, and send nothing"}
	// OutputFlag is where Call.Save writes a fetched file.
	OutputFlag = Flag{Name: "output", Kind: String, Value: "path", Help: "where to write the file, instead of under its own name in the current directory"}
)

// LimitFlag is --limit with a default and a highest value.
func LimitFlag(def, highest int) Flag {
	return Flag{Name: "limit", Kind: Int, Value: "n", Default: strconv.Itoa(def),
		Help: fmt.Sprintf("how many to show, 1 to %d", highest)}
}

func (c *Command) usage() string {
	parts := []string{c.Name}
	for _, arg := range c.Args {
		parts = append(parts, arg.usage())
	}
	for _, f := range c.Flags {
		parts = append(parts, "["+f.usage()+"]")
	}
	return strings.Join(parts, " ")
}

// shortUsage folds the flags into one word, for the overview.
func (c *Command) shortUsage() string {
	parts := []string{c.Name}
	for _, arg := range c.Args {
		parts = append(parts, arg.usage())
	}
	if len(c.Flags) > 0 {
		parts = append(parts, "[flags]")
	}
	return strings.Join(parts, " ")
}

func (a Arg) usage() string {
	name := a.Name
	if a.Variadic {
		name += "..."
	}
	if a.Required {
		return "<" + name + ">"
	}
	return "[" + name + "]"
}

func (f Flag) usage() string {
	switch {
	case f.Kind == Bool:
		return "--" + f.Name
	case f.Kind == List:
		return "--" + f.Name + " <" + f.Value + ",...>"
	case f.Kind == Repeat:
		return "--" + f.Name + " <" + f.Value + ">..."
	case f.Value == "":
		return "--" + f.Name + " " + strings.Join(f.Enum, "|")
	}
	return "--" + f.Name + " <" + f.Value + ">"
}

// sentence turns a help fragment from the table into a sentence.
func sentence(fragment string) string {
	if fragment == "" {
		return ""
	}
	return strings.ToUpper(fragment[:1]) + fragment[1:] + "."
}

func (f Flag) description() string {
	text := sentence(f.Help)
	if f.Kind == List {
		text += " Repeat the flag or separate values with commas."
	}
	if f.Kind == Repeat {
		text += " Repeat the flag for more than one. A comma is part of the value."
	}
	if len(f.Enum) > 0 && f.Value != "" {
		text += " One of: " + strings.Join(f.Enum, ", ") + "."
	}
	if f.Default != "" {
		text += " Default: " + f.Default + "."
	}
	return text
}

// Input is what a command was called with.
type Input struct {
	Args  []string
	flags map[string]any
	given map[string]bool
}

func (in *Input) Bool(name string) bool     { return *in.value(name).(*bool) }
func (in *Input) String(name string) string { return *in.value(name).(*string) }
func (in *Input) Int(name string) int       { return int(*in.value(name).(*int64)) }

// List returns the values of a List or a Repeat flag.
func (in *Input) List(name string) []string {
	if whole, ok := in.value(name).(*repeatValue); ok {
		return *whole
	}
	return *in.value(name).(*listValue)
}

// Given reports whether the flag was on the command line, which a default
// value cannot tell.
func (in *Input) Given(name string) bool { return in.given[name] }

// Has reports whether the command has the flag at all.
func (in *Input) Has(name string) bool { return in.flags[name] != nil }

// Arg returns the positional argument at i, or "" when it was left out.
func (in *Input) Arg(i int) string {
	if i < len(in.Args) {
		return in.Args[i]
	}
	return ""
}

func (in *Input) value(name string) any {
	v, ok := in.flags[name]
	if !ok {
		panic("ctlkit: the command reads --" + name + ", which is not in its table entry")
	}
	return v
}

// listValue collects a repeatable flag. Each occurrence may hold several
// values separated by commas.
type listValue []string

func (l *listValue) String() string { return strings.Join(*l, ",") }

func (l *listValue) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return errors.New("empty value")
		}
		if !slices.Contains(*l, item) {
			*l = append(*l, item)
		}
	}
	return nil
}

// repeatValue collects a repeatable flag whose values are taken whole.
type repeatValue []string

func (r *repeatValue) String() string { return strings.Join(*r, "\n") }

func (r *repeatValue) Set(value string) error {
	if value == "" {
		return errors.New("empty value")
	}
	*r = append(*r, value)
	return nil
}

var errHelp = errors.New("help requested")

// parse reads flags placed on either side of the positional arguments and
// checks both against the command's table entry.
func (c *Command) parse(args []string) (*Input, error) {
	if err := checkControl(args); err != nil {
		return nil, err
	}

	fs := flag.NewFlagSet(c.Name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	in := &Input{flags: make(map[string]any), given: make(map[string]bool)}
	for _, f := range c.Flags {
		switch f.Kind {
		case Bool:
			in.flags[f.Name] = fs.Bool(f.Name, false, f.Help)
		case String:
			in.flags[f.Name] = fs.String(f.Name, f.Default, f.Help)
		case Int:
			def, _ := strconv.ParseInt(f.Default, 10, 64)
			in.flags[f.Name] = fs.Int64(f.Name, def, f.Help)
		case List:
			list := new(listValue)
			fs.Var(list, f.Name, f.Help)
			in.flags[f.Name] = list
		case Repeat:
			whole := new(repeatValue)
			fs.Var(whole, f.Name, f.Help)
			in.flags[f.Name] = whole
		}
	}

	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, errHelp
			}
			return nil, Usagef("%s", flagMessage(err))
		}
		args = fs.Args()
		if len(args) > 0 {
			in.Args = append(in.Args, args[0])
			args = args[1:]
		}
	}
	fs.Visit(func(f *flag.Flag) { in.given[f.Name] = true })

	for _, f := range c.Flags {
		if err := f.checkEnum(in); err != nil {
			return nil, err
		}
	}
	return in, c.checkArgs(in.Args)
}

// flagMessage rewords the flag package's errors, which name a flag with one
// dash where help and schema name it with two.
func flagMessage(err error) string {
	msg := err.Error()
	if name, ok := strings.CutPrefix(msg, "flag provided but not defined: -"); ok {
		return "unknown flag --" + strings.TrimLeft(name, "-")
	}
	if rest, ok := strings.CutPrefix(msg, "flag needs an argument: -"); ok {
		return "--" + strings.TrimLeft(rest, "-") + " needs a value"
	}
	return strings.Replace(msg, "for flag -", "for flag --", 1)
}

func (f Flag) checkEnum(in *Input) error {
	if len(f.Enum) == 0 {
		return nil
	}
	check := func(value string) (string, error) {
		lower := strings.ToLower(value)
		if !slices.Contains(f.Enum, lower) {
			return "", Usagef("--%s does not take %q, use one of: %s", f.Name, value, strings.Join(f.Enum, ", "))
		}
		return lower, nil
	}

	var err error
	switch value := in.flags[f.Name].(type) {
	case *string:
		if in.given[f.Name] {
			*value, err = check(*value)
		}
	case *listValue:
		for i := range *value {
			if (*value)[i], err = check((*value)[i]); err != nil {
				break
			}
		}
	}
	return err
}

func (c *Command) checkArgs(args []string) error {
	for i, arg := range c.Args {
		if arg.Required && i >= len(args) {
			return Usagef("missing %s", arg.usage())
		}
	}
	if extra := len(args) - len(c.Args); extra > 0 && (len(c.Args) == 0 || !c.Args[len(c.Args)-1].Variadic) {
		return Usagef("unexpected argument %q", args[len(c.Args)])
	}
	for i, value := range args {
		if spec := c.Args[min(i, len(c.Args)-1)]; spec.ID {
			if err := CheckID(spec.Name, value); err != nil {
				return err
			}
		}
	}
	return nil
}
