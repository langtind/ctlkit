package ctltest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/langtind/ctlkit"
	"github.com/langtind/ctlkit/httpx"
	"github.com/langtind/ctlkit/keystore"
	"github.com/langtind/ctlkit/session"
)

// toy is a small tool against a fake: it renews a login and posts a letter.
func toy(fake *Fake, store keystore.Store) func() *ctlkit.Program {
	return func() *ctlkit.Program {
		login := &session.Manager{Store: store}
		login.Renew = func(ctx context.Context, refresh string) (session.Session, error) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fake.URL()+"/renew", strings.NewReader(refresh))
			body, err := httpx.Do(fake.Client(), req, "Toy", "toy/1")
			return session.Session{AccessToken: string(body)}, err
		}
		p := ctlkit.New("toyctl", "post letters", ctlkit.Build{Version: "1"})
		p.Interactive = false
		p.Commands = []ctlkit.Command{
			{Name: "refresh", Summary: "Renew the login", Run: func(ctx context.Context, c *ctlkit.Call) error {
				s, _ := login.Load()
				_, err := login.Refresh(ctx, s.AccessToken)
				return err
			}},
			{Name: "post", Summary: "Post a letter", Mutates: true,
				Flags: []ctlkit.Flag{ctlkit.DryRunFlag, ctlkit.YesFlag, ctlkit.JSONFlag},
				Run: func(ctx context.Context, c *ctlkit.Call) error {
					if c.DryRun() {
						return c.Print(map[string]bool{"dryRun": true}, func(w io.Writer) { fmt.Fprintln(w, "Would post.") })
					}
					if err := c.Confirm("Post the letter?"); err != nil {
						return err
					}
					var form httpx.Form
					body, contentType, _ := form.JSON("request", map[string]string{"to": "Kari"}).
						File("attachments", httpx.FormFile{Name: "a.pdf", Data: []byte("%PDF-1.7")}).Body()
					req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fake.URL()+"/letters", strings.NewReader(string(body)))
					req.Header.Set("Content-Type", contentType)
					req.AddCookie(&http.Cookie{Name: "sid", Value: "s1"})
					_, err := httpx.Do(fake.Client(), req, "Toy", "toy/1")
					return err
				}},
		}
		return p
	}
}

func TestFakeKeepsWhatWasSentAndTheProbesPass(t *testing.T) {
	fake := NewFake(t)
	renewals := 0
	fake.Handle("POST /renew", func(c Call) Answer {
		renewals++
		return Answer{Body: fmt.Sprintf("access-%d", renewals)}
	})
	fake.On("POST /letters", http.StatusCreated, `{}`)
	fake.Check = func(t *testing.T, c Call) {
		if c.Header.Get("User-Agent") != "toy/1" {
			t.Errorf("User-Agent = %q", c.Header.Get("User-Agent"))
		}
	}
	store := &keystore.Memory{}
	login := &session.Manager{Store: store}
	if err := login.Save(session.Session{AccessToken: "access-0", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	build := toy(fake, store)

	RefreshRenewsEveryTime(t, build, func() session.Session { s, _ := login.Load(); return s })
	ChangeNeedsAYes(t, build, fake, []string{"post"}, "POST /renew")

	if code, _, stderr := Run(build(), "", "post", "--yes"); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	c := fake.Calls[0]
	if fake.Sent() != "POST /letters" || c.Form["request"] != `{"to":"Kari"}` || c.Cookies["sid"] != "s1" ||
		len(c.Files) != 1 || c.Files[0].Field != "attachments" || c.Files[0].Name != "a.pdf" || c.Files[0].ContentType != "application/pdf" {
		t.Errorf("sent = %s, call = %+v", fake.Sent(), c)
	}
	if got := fake.Changes(); len(got) != 1 {
		t.Errorf("changes = %v", got)
	}

	fake.Reset()
	req, _ := http.NewRequest(http.MethodGet, fake.URL()+"/nowhere", nil)
	if _, err := httpx.Do(fake.Client(), req, "Toy", "toy/1"); ctlkit.KindOf(err) != ctlkit.KindNotFound || !strings.Contains(err.Error(), "the fake has no answer for GET /nowhere") {
		t.Errorf("unknown route: %v", err)
	}
}
