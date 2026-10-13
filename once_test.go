package ctlkit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/langtind/ctlkit"
)

type letter struct{ ID string }

// post is a write against a mailbox that holds what was sent.
func post(box *[]letter, sendErr error, lands bool) (ctlkit.Write[letter], *int) {
	sent := 0
	return ctlkit.Write[letter]{
		What: "a letter for Monday",
		List: "postctl letters",
		Find: func(context.Context) (*letter, error) {
			if len(*box) == 0 {
				return nil, nil
			}
			return &(*box)[0], nil
		},
		Send: func(context.Context) error {
			sent++
			if lands {
				*box = append(*box, letter{ID: "l1"})
			}
			return sendErr
		},
	}, &sent
}

func TestAWriteIsSentOnceAndLookedUp(t *testing.T) {
	ctx := context.Background()
	timeout := ctlkit.Errorf(ctlkit.KindUnavailable, "no answer")

	var box []letter
	w, sent := post(&box, nil, true)
	if err := w.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := w.Do(ctx); err != nil || got.ID != "l1" || *sent != 1 {
		t.Errorf("got = %+v, err = %v, sent = %d", got, err, *sent)
	}
	// It is there now, so a second run is refused before anything is sent.
	if err := w.Check(ctx); ctlkit.KindOf(err) != ctlkit.KindConflict || !strings.Contains(err.Error(), "a letter for Monday exists already") {
		t.Errorf("second check: %v", err)
	}

	// No answer, but the letter landed: it is found, and that is success.
	box = nil
	w, sent = post(&box, timeout, true)
	if got, err := w.Do(ctx); err != nil || got == nil || *sent != 1 {
		t.Errorf("landed after a timeout: got = %+v, err = %v, sent = %d", got, err, *sent)
	}

	// No answer and nothing found: nobody knows, and it is not sent again.
	box = nil
	w, sent = post(&box, timeout, false)
	_, err := w.Do(ctx)
	var kindErr *ctlkit.Error
	if !errors.As(err, &kindErr) || kindErr.Kind != ctlkit.KindUnavailable || *sent != 1 ||
		!strings.Contains(err.Error(), "it is not known whether a letter for Monday was sent") ||
		!strings.Contains(kindErr.Hint, `do not run the command again before you have looked. Run "postctl letters"`) {
		t.Errorf("unknown: err = %v, sent = %d", err, *sent)
	}

	// A clear no: nothing was made, and the error is the service's own.
	box = nil
	w, _ = post(&box, ctlkit.Usagef("the date has passed"), false)
	if _, err := w.Do(ctx); ctlkit.KindOf(err) != ctlkit.KindUsage {
		t.Errorf("refused: %v", err)
	}
}

func TestParseDateAndLocalTime(t *testing.T) {
	oslo := time.FixedZone("CEST", 2*60*60)
	day, err := ctlkit.ParseDate("from", "2026-10-15", oslo)
	if err != nil || !day.Equal(time.Date(2026, 10, 15, 0, 0, 0, 0, oslo)) {
		t.Errorf("day = %v, err = %v", day, err)
	}
	if _, err := ctlkit.ParseDate("from", "15.10.2026", oslo); ctlkit.KindOf(err) != ctlkit.KindUsage || !strings.Contains(err.Error(), "--from takes a date as yyyy-mm-dd") {
		t.Errorf("err = %v", err)
	}

	want := time.Date(2026, 10, 15, 8, 30, 0, 0, oslo)
	for _, text := range []string{"2026-10-15T08:30:00", "2026-10-15T08:30:00.000", "2026-10-15 08:30:00", "2026-10-15T06:30:00Z"} {
		if got := ctlkit.LocalTime(text, oslo); !got.Equal(want) {
			t.Errorf("LocalTime(%q) = %v", text, got)
		}
	}
	for _, text := range []string{"", "null", "soon"} {
		if got := ctlkit.LocalTime(text, oslo); !got.IsZero() {
			t.Errorf("LocalTime(%q) = %v", text, got)
		}
	}
}
