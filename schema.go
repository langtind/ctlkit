package ctlkit

import (
	"context"
	"strconv"
	"strings"
)

// schemaDocument describes the command line for a program. It is built from
// the command table, so it always matches what the dispatcher accepts. It
// needs no login, no config and no network.
type schemaDocument struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Summary    string            `json:"summary"`
	ExitCodes  map[string]string `json:"exitCodes"`
	ErrorKinds []schemaKind      `json:"errorKinds"`
	Commands   []schemaCommand   `json:"commands"`
}

type schemaKind struct {
	Kind     Kind   `json:"kind"`
	ExitCode int    `json:"exitCode"`
	Meaning  string `json:"meaning"`
}

type schemaCommand struct {
	Name    string `json:"name"`
	Usage   string `json:"usage"`
	Summary string `json:"summary"`
	// Description is the long help, for a command that has one.
	Description string `json:"description,omitempty"`
	// Mutates is true when the command changes something at the service.
	Mutates bool         `json:"mutates"`
	Args    []schemaArg  `json:"args"`
	Flags   []schemaFlag `json:"flags"`
}

type schemaArg struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Variadic bool   `json:"variadic"`
	Help     string `json:"help"`
}

type schemaFlag struct {
	Name string `json:"name"`
	// Type is bool, string, int or list. A list flag can be repeated and
	// takes comma-separated values.
	Type string `json:"type"`
	// Default is null when the flag has none.
	Default any      `json:"default"`
	Enum    []string `json:"enum,omitempty"`
	Help    string   `json:"help"`
}

func (p *Program) schema(ctx context.Context, c *Call) error {
	doc := schemaDocument{
		Name:      p.Name,
		Version:   p.Build.Version,
		Summary:   p.Summary,
		ExitCodes: map[string]string{"0": "success"},
	}
	for _, k := range kinds {
		doc.ExitCodes[strconv.Itoa(k.code)] = k.meaning
		doc.ErrorKinds = append(doc.ErrorKinds, schemaKind{Kind: k.kind, ExitCode: k.code, Meaning: k.meaning})
	}
	for _, cmd := range p.table() {
		entry := schemaCommand{
			Name:        cmd.Name,
			Usage:       p.Name + " " + cmd.usage(),
			Summary:     cmd.Summary,
			Description: strings.TrimSpace(cmd.Long),
			Mutates:     cmd.Mutates,
			Args:        []schemaArg{},
			Flags:       []schemaFlag{},
		}
		for _, arg := range cmd.Args {
			entry.Args = append(entry.Args, schemaArg{Name: arg.Name, Required: arg.Required, Variadic: arg.Variadic, Help: sentence(arg.Help)})
		}
		for _, f := range cmd.Flags {
			entry.Flags = append(entry.Flags, schemaFlag{Name: f.Name, Type: string(f.Kind), Default: f.schemaDefault(), Enum: f.Enum, Help: f.description()})
		}
		doc.Commands = append(doc.Commands, entry)
	}
	return writeJSON(p.Stdout, doc)
}

func (f Flag) schemaDefault() any {
	switch {
	case f.Kind == Bool:
		return false
	case f.Default == "":
		return nil
	case f.Kind == Int:
		if n, err := strconv.ParseInt(f.Default, 10, 64); err == nil {
			return n
		}
	}
	return f.Default
}
