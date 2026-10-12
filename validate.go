package ctlkit

import (
	"fmt"
	"strings"
)

// The caller is often a language model, and a model makes other mistakes
// than a person does: it puts a query string on an id, encodes a value that
// is encoded again later, or writes a character nobody can see. These checks
// stop such a value before it reaches the service.

// CheckID reports a usage error unless value can be the id of one thing at a
// service. what names the argument in the message.
func CheckID(what, value string) error {
	if value == "" {
		return Usagef("the %s must not be empty", what)
	}
	for _, r := range value {
		var problem string
		switch {
		case r <= ' ' || r == 0x7f:
			problem = "white space or a control character"
		case r == '?' || r == '#':
			problem = fmt.Sprintf("%q, which starts a query or a fragment", string(r))
		case r == '%':
			problem = `"%", which means the value is already URL-encoded`
		case r == '/' || r == '\\':
			problem = fmt.Sprintf("%q, which makes it a path", string(r))
		default:
			continue
		}
		return Usagef("%q cannot be the %s: it holds %s", value, what, problem).WithHint("pass the bare id, as a list command prints it.")
	}
	if strings.Contains(value, "..") {
		return Usagef(`%q cannot be the %s: it holds ".."`, value, what)
	}
	return nil
}

// checkControl rejects control characters anywhere on the command line. A
// line break and a tab are allowed, because free text may hold them.
func checkControl(args []string) error {
	for _, arg := range args {
		for _, r := range arg {
			if (r < ' ' && r != '\n' && r != '\t') || r == 0x7f {
				return Usagef("the argument %q holds the control character %U", arg, r)
			}
		}
	}
	return nil
}
