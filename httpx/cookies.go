package httpx

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/langtind/ctlkit"
)

// BrowserCookies reads the cookies for one host out of the storage state a
// browser saved after the user logged in, as "playwright-cli state-save"
// writes it. A login that needs a person, such as BankID or a single
// sign-on, is taken over this way: the user logs in once in a browser, and
// the tool keeps the cookies in the keyring.
func BrowserCookies(state []byte, host string) (map[string]string, error) {
	var saved struct {
		Cookies []struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Domain string `json:"domain"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal(state, &saved); err != nil {
		return nil, ctlkit.Usagef("stdin does not hold a browser storage state: %v", err)
	}
	found := map[string]string{}
	for _, cookie := range saved.Cookies {
		if strings.TrimPrefix(cookie.Domain, ".") == host {
			found[cookie.Name] = cookie.Value
		}
	}
	return found, nil
}

// CookieCatcher is a cookie jar that keeps what the answers set and sends
// nothing. Use it on a copy of the client to see which cookies a request
// hands out, for a service that renews a login in Set-Cookie.
type CookieCatcher map[string]string

func (j CookieCatcher) SetCookies(_ *url.URL, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		j[cookie.Name] = cookie.Value
	}
}

func (CookieCatcher) Cookies(*url.URL) []*http.Cookie { return nil }
