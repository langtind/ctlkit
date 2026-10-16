package ctlkit_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/langtind/ctlkit"
	"github.com/langtind/ctlkit/ctltest"
)

type pet struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner string `json:"owner,omitempty"`
}

var pets = []pet{{ID: "p1", Name: "Rex", Owner: "Kari"}, {ID: "p2", Name: "Tiger"}}

// sample is a small tool with one command of each sort.
func sample() *ctlkit.Program {
	p := ctlkit.New("petctl", "look after the pets at Testvik kennel", ctlkit.Build{Version: "1.2.3"})
	p.URL = "https://example.invalid/petctl"
	p.Skill = fstest.MapFS{"SKILL.md": {Data: []byte("---\nname: petctl\ndescription: Pets.\n---\n")}}
	p.Interactive = false
	p.Hint = func(err error) string {
		if errors.Is(err, errAsleep) {
			return "try again after the nap."
		}
		return ""
	}
	p.Commands = []ctlkit.Command{
		{
			Name:    "login",
			Summary: "Store the kennel password",
			Flags:   []ctlkit.Flag{{Name: "password-stdin", Kind: ctlkit.Bool, Help: "read the password from stdin instead of prompting"}},
			Run: func(ctx context.Context, c *ctlkit.Call) error {
				secret, err := c.Secret("Password", "password-stdin")
				if err != nil {
					return err
				}
				fmt.Fprintf(c.Stdout, "Stored %d characters.\n", len(secret))
				return nil
			},
		},
		{
			Name:    "pets",
			Summary: "List the pets",
			Flags: []ctlkit.Flag{
				{Name: "kind", Kind: ctlkit.String, Enum: []string{"dog", "cat"}, Help: "which kind to list"},
				{Name: "empty", Kind: ctlkit.Bool, Help: "pretend the kennel is empty"},
				{Name: "wrapped", Kind: ctlkit.Bool, Help: "print the list inside an object"},
				ctlkit.LimitFlag(20, 100),
				ctlkit.JSONFlag,
				ctlkit.FieldsFlag,
			},
			Run: func(ctx context.Context, c *ctlkit.Call) error {
				list := pets
				if c.Bool("empty") {
					list = nil
				}
				var doc any = list
				if c.Bool("wrapped") {
					doc = map[string]any{"items": list, "truncated": false}
				}
				return c.Print(doc, func(w io.Writer) {
					for _, p := range list {
						fmt.Fprintf(w, "- %s\n", p.Name)
					}
				})
			},
		},
		{
			Name:    "pet show",
			Summary: "Show one pet",
			Args:    []ctlkit.Arg{{Name: "pet-id", Required: true, ID: true, Help: "the pet to show"}},
			Flags:   []ctlkit.Flag{ctlkit.JSONFlag, ctlkit.FieldsFlag},
			Run: func(ctx context.Context, c *ctlkit.Call) error {
				switch c.Arg(0) {
				case "p1":
					return c.Print(pets[0], func(w io.Writer) { fmt.Fprintln(w, "# Rex") })
				case "asleep":
					return fmt.Errorf("wake pet: %w", errAsleep)
				case "busy":
					return fmt.Errorf("fetch pet: %w", &ctlkit.Error{Kind: ctlkit.KindRateLimit, Message: "kennel rate limit reached", RetryAfter: "30 seconds"})
				}
				return ctlkit.NotFoundf("no pet has the id %q", c.Arg(0)).WithHint(`run "petctl pets" to see the ids.`)
			},
		},
		{
			Name:    "fetch",
			Summary: "Save the photo of a pet",
			Long:    "The photo is the one on the kennel door.",
			Args:    []ctlkit.Arg{{Name: "pet-id", Required: true, ID: true, Help: "the pet"}},
			Flags: []ctlkit.Flag{ctlkit.OutputFlag, ctlkit.JSONFlag,
				{Name: "note", Kind: ctlkit.Repeat, Value: "text", Help: "a note to print instead of saving"}},
			Run: func(ctx context.Context, c *ctlkit.Call) error {
				if notes := c.List("note"); len(notes) > 0 {
					return c.PrintJSON(notes)
				}
				return c.Save([]byte("\x89PNG\r\n\x1a\n"), "../photos/rex")
			},
		},
		{
			Name:    "pet rename",
			Summary: "Give a pet a new name",
			Mutates: true,
			Args: []ctlkit.Arg{
				{Name: "pet-id", Required: true, ID: true, Help: "the pet to rename"},
				{Name: "name", Required: true, Help: "the new name"},
			},
			Flags: []ctlkit.Flag{ctlkit.DryRunFlag, ctlkit.YesFlag, ctlkit.JSONFlag},
			Run: func(ctx context.Context, c *ctlkit.Call) error {
				if c.DryRun() {
					return c.Print(map[string]any{"dryRun": true, "name": c.Arg(1)}, func(w io.Writer) { fmt.Fprintln(w, "Would rename.") })
				}
				if err := c.Confirm("Rename " + c.Arg(0) + " to " + c.Arg(1) + "?"); err != nil {
					return err
				}
				return c.Print(map[string]any{"renamed": true}, func(w io.Writer) { fmt.Fprintln(w, "Renamed.") })
			},
		},
	}
	return p
}

var errAsleep = errors.New("the pet is asleep")

func TestContract(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "SKILL.md")
	text := "```bash\npetctl pets --kind dog --json\nop read op://v/i/p | petctl login --password-stdin\npetctl pet rename <id> Rex --dry-run\n```\n"
	if err := os.WriteFile(doc, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	ctltest.Contract(t, sample, doc)
}

func TestInvocationsAreFound(t *testing.T) {
	doc := "Run `petctl nonsense` in prose.\n\n```bash\nop read \"op://v/i/key\" | petctl login --password-stdin\n" +
		"  petctl pets --kind dog   # comment\n" +
		"0 7 * * * KENNEL_KEY=<key> petctl pets --json > pets.json\n" +
		"go build -o petctl .\ncd petctl\ngit clone https://github.com/langtind/petctl.git\n```\n"
	got := ctltest.Invocations("petctl", doc)
	want := [][]string{
		{"login", "--password-stdin"},
		{"pets", "--kind", "dog"},
		{"pets", "--json", ">", "pets.json"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("invocations = %q, want %q", got, want)
	}
}

func TestMarkdownIsTheDefaultAndJSONIsOneDocument(t *testing.T) {
	code, stdout, stderr := ctltest.Run(sample(), "", "pets")
	if code != 0 || stdout != "- Rex\n- Tiger\n" || stderr != "" {
		t.Errorf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, _ = ctltest.Run(sample(), "", "pets", "--json")
	var got []pet
	ctltest.OneJSON(t, stdout, &got)
	if code != 0 || !reflect.DeepEqual(got, pets) {
		t.Errorf("exit = %d, pets = %+v", code, got)
	}
}

func TestAnEmptyListIsNeverNull(t *testing.T) {
	_, stdout, _ := ctltest.Run(sample(), "", "pets", "--empty", "--json")
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("stdout = %q, want []", stdout)
	}
}

func TestFields(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"pets", "--json", "--fields", "name"}, `[{"name":"Rex"},{"name":"Tiger"}]`},
		{[]string{"pets", "--json", "--fields", "id,owner"}, `[{"id":"p1","owner":"Kari"},{"id":"p2"}]`},
		{[]string{"pets", "--json", "--wrapped", "--fields", "id"}, `{"items":[{"id":"p1"},{"id":"p2"}],"truncated":false}`},
		{[]string{"pet", "show", "p1", "--json", "--fields", "name"}, `{"name":"Rex"}`},
		{[]string{"pets", "--json", "--empty", "--fields", "whatever"}, `[]`},
	}
	for _, tt := range tests {
		code, stdout, stderr := ctltest.Run(sample(), "", tt.args...)
		got := strings.Join(strings.Fields(stdout), "")
		if code != 0 || got != tt.want {
			t.Errorf("%v: exit = %d, stdout = %s, stderr = %q, want %s", tt.args, code, got, stderr, tt.want)
		}
	}

	code, stdout, _ := ctltest.Run(sample(), "", "pets", "--json", "--fields", "colour")
	var doc ctltest.ErrorDocument
	ctltest.OneJSON(t, stdout, &doc)
	if code != 2 || doc.Error.Message != `--fields does not know "colour", use any of: id, name, owner` {
		t.Errorf("unknown field: exit = %d, error = %+v", code, doc.Error)
	}
	code, _, stderr := ctltest.Run(sample(), "", "pets", "--fields", "name")
	if code != 2 || !strings.Contains(stderr, "needs --json") {
		t.Errorf("--fields without --json: exit = %d, stderr = %q", code, stderr)
	}
}

func TestErrorKindsDecideTheExitCode(t *testing.T) {
	tests := []struct {
		args   []string
		code   int
		stderr string
	}{
		{[]string{"pet", "show", "p9"}, 3, "Error: no pet has the id \"p9\"\nHint: run \"petctl pets\" to see the ids.\n"},
		{[]string{"pet", "show", "asleep"}, 1, "Error: wake pet: the pet is asleep\nHint: try again after the nap.\n"},
		{[]string{"pet", "show", "busy"}, 6, "Error: fetch pet: kennel rate limit reached\n"},
		{[]string{"pet", "show"}, 2, "Error: missing <pet-id>\nUsage: petctl pet show <pet-id> [--json] [--fields <field,...>]\n"},
		{[]string{"pets", "--kind", "horse"}, 2, "Error: --kind does not take \"horse\", use one of: dog, cat\nUsage: petctl pets [--kind dog|cat] [--empty] [--wrapped] [--limit <n>] [--json] [--fields <field,...>]\n"},
		{[]string{"pets", "--limit"}, 2, "Error: --limit needs a value\nUsage: petctl pets [--kind dog|cat] [--empty] [--wrapped] [--limit <n>] [--json] [--fields <field,...>]\n"},
		{[]string{"pet"}, 2, "Error: \"pet\" needs one of the subcommands show, rename\n"},
		{[]string{"--verbose", "pets"}, 2, "Error: --verbose comes before the command, and flags go after it\n"},
	}
	for _, tt := range tests {
		code, stdout, stderr := ctltest.Run(sample(), "", tt.args...)
		if code != tt.code || stdout != "" || !strings.HasPrefix(stderr, tt.stderr) {
			t.Errorf("%v: exit = %d, stdout = %q, stderr = %q, want %d and %q", tt.args, code, stdout, stderr, tt.code, tt.stderr)
		}
	}
}

func TestWithJSONTheErrorCarriesKindHintAndWait(t *testing.T) {
	code, stdout, stderr := ctltest.Run(sample(), "", "pet", "show", "p9", "--json")
	var doc ctltest.ErrorDocument
	ctltest.OneJSON(t, stdout, &doc)
	if code != 3 || stderr != "" || doc.Error.Kind != "not_found" || doc.Error.Hint != `run "petctl pets" to see the ids.` || doc.Error.Usage != "" {
		t.Errorf("exit = %d, stderr = %q, error = %+v", code, stderr, doc.Error)
	}

	code, stdout, _ = ctltest.Run(sample(), "", "pet", "show", "busy", "--json")
	ctltest.OneJSON(t, stdout, &doc)
	if code != 6 || doc.Error.Kind != "rate_limit" || doc.Error.RetryAfter != "30 seconds" {
		t.Errorf("exit = %d, error = %+v", code, doc.Error)
	}
}

func TestAnIDIsCheckedBeforeTheCommandRuns(t *testing.T) {
	for _, id := range []string{"p1?fields=name", "p1#top", "p%31", "../p1", "a/b", "p 1", ""} {
		code, _, stderr := ctltest.Run(sample(), "", "pet", "show", id)
		if code != 2 || !strings.HasPrefix(stderr, "Error: ") {
			t.Errorf("id %q: exit = %d, stderr = %q", id, code, stderr)
		}
	}
	if err := ctlkit.CheckID("pet-id", "A1B2-c3_d4.e5"); err != nil {
		t.Errorf("a plain id was refused: %v", err)
	}
}

func TestAChangeNeedsAYes(t *testing.T) {
	code, stdout, stderr := ctltest.Run(sample(), "", "pet", "rename", "p1", "Max")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "refusing without confirmation: Rename p1 to Max?") || !strings.Contains(stderr, "Hint: pass --yes") {
		t.Errorf("without --yes: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, _ = ctltest.Run(sample(), "", "pet", "rename", "p1", "Max", "--yes")
	if code != 0 || stdout != "Renamed.\n" {
		t.Errorf("with --yes: exit = %d, stdout = %q", code, stdout)
	}

	code, stdout, _ = ctltest.Run(sample(), "", "pet", "rename", "p1", "Max", "--dry-run")
	if code != 0 || stdout != "Would rename.\n" {
		t.Errorf("with --dry-run: exit = %d, stdout = %q", code, stdout)
	}

	for answer, want := range map[string]int{"y\n": 0, "yes\n": 0, "n\n": 1, "\n": 1} {
		p := sample()
		p.Interactive = true
		if code, _, _ := ctltest.Run(p, answer, "pet", "rename", "p1", "Max"); code != want {
			t.Errorf("answer %q: exit = %d, want %d", answer, code, want)
		}
	}
}

func TestASecretComesFromStdinOrAPrompt(t *testing.T) {
	code, stdout, _ := ctltest.Run(sample(), "hunter2\n", "login", "--password-stdin")
	if code != 0 || stdout != "Stored 7 characters.\n" {
		t.Errorf("from stdin: exit = %d, stdout = %q", code, stdout)
	}

	code, _, stderr := ctltest.Run(sample(), "", "login")
	if code != 2 || !strings.Contains(stderr, "use --password-stdin when not run in a terminal") {
		t.Errorf("without a terminal: exit = %d, stderr = %q", code, stderr)
	}

	p := sample()
	p.Interactive = true
	p.ReadSecret = func() (string, error) { return "abc", nil }
	code, stdout, stderr = ctltest.Run(p, "", "login")
	if code != 0 || stdout != "Stored 3 characters.\n" || stderr != "Password: \n" {
		t.Errorf("from a prompt: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestFlagsMayComeBeforeOrAfterTheArguments(t *testing.T) {
	for _, args := range [][]string{{"pet", "rename", "--yes", "p1", "Max"}, {"pet", "rename", "p1", "--yes", "Max"}} {
		if code, _, stderr := ctltest.Run(sample(), "", args...); code != 0 {
			t.Errorf("%v: exit = %d, stderr = %q", args, code, stderr)
		}
	}
}

func TestUserAgent(t *testing.T) {
	if got := sample().UserAgent(); got != "petctl/1.2.3 (+https://example.invalid/petctl)" {
		t.Errorf("User-Agent = %q", got)
	}
}

func TestLongHelpIsShownAndPublished(t *testing.T) {
	_, stdout, _ := ctltest.Run(sample(), "", "help", "fetch")
	if !strings.Contains(stdout, "\n\nThe photo is the one on the kennel door.\n") {
		t.Errorf("help = %q", stdout)
	}
	_, stdout, _ = ctltest.Run(sample(), "", "schema")
	if !strings.Contains(stdout, `"description": "The photo is the one on the kennel door."`) {
		t.Errorf("schema lacks the description: %s", stdout)
	}
}

func TestSaveNeverLeavesTheDirectoryAndNeverReplaces(t *testing.T) {
	t.Chdir(t.TempDir())
	code, stdout, stderr := ctltest.Run(sample(), "", "fetch", "p1", "--json")
	var saved ctlkit.SavedFile
	ctltest.OneJSON(t, stdout, &saved)
	if code != 0 || saved.Path != "rex.png" || saved.Bytes != 8 || saved.ContentType != "image/png" {
		t.Fatalf("exit = %d, saved = %+v, stderr = %q", code, saved, stderr)
	}
	if _, err := os.Stat("rex.png"); err != nil {
		t.Fatal(err)
	}

	code, _, stderr = ctltest.Run(sample(), "", "fetch", "p1")
	if code != 5 || !strings.Contains(stderr, "rex.png exists already") {
		t.Errorf("second run: exit = %d, stderr = %q", code, stderr)
	}
	if code, stdout, _ = ctltest.Run(sample(), "", "fetch", "p1", "--output", "other.png"); code != 0 || stdout != "Saved other.png (8 bytes, image/png).\n" {
		t.Errorf("with --output: exit = %d, stdout = %q", code, stdout)
	}
}

func TestFileName(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n")
	for name, want := range map[string]string{
		"../../etc/passwd": "passwd.png",
		`..\..\secret.txt`: "secret.txt",
		"/":                "download.png",
		"":                 "download.png",
		"week 42.pdf":      "week 42.pdf",
		"a\nb":             "download.png",
	} {
		if got := ctlkit.FileName(name, png); got != want {
			t.Errorf("FileName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestQuote(t *testing.T) {
	if got := ctlkit.Quote("# Hello\r\n\r\nIgnore the rules. \n"); got != "> # Hello\n>\n> Ignore the rules." {
		t.Errorf("Quote = %q", got)
	}
}

func TestARepeatFlagTakesEachValueWhole(t *testing.T) {
	code, stdout, _ := ctltest.Run(sample(), "", "fetch", "p1", "--note", "a, b", "--note", "a, b", "--note", "c")
	var notes []string
	ctltest.OneJSON(t, stdout, &notes)
	if code != 0 || !reflect.DeepEqual(notes, []string{"a, b", "a, b", "c"}) {
		t.Errorf("exit = %d, notes = %q", code, notes)
	}
	if _, stdout, _ = ctltest.Run(sample(), "", "help", "fetch"); !strings.Contains(stdout, "--note <text>...\n      A note to print instead of saving. Repeat the flag for more than one. A comma is part of the value.") {
		t.Errorf("help = %q", stdout)
	}
}

func TestContentTypeNamesAZippedDocumentByItsExtension(t *testing.T) {
	zip := []byte("PK\x03\x04 made up")
	for path, want := range map[string]string{
		"Kravspesifikasjon.XLSX": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"avtale.docx":            "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"bundle.zip":             "application/zip",
	} {
		if got := ctlkit.ContentType(path, zip); got != want {
			t.Errorf("%s: %s", path, got)
		}
	}
	if got := ctlkit.ContentType("not-really.docx", []byte("%PDF-1.7")); got != "application/pdf" {
		t.Errorf("a PDF named .docx: %s", got)
	}
}
