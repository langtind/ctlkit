package ctlkit

import (
	"context"
	"fmt"
)

// Write is a change that the service does not make safe to repeat: sent
// twice, it is made twice. Call Check before the question is asked, and Do
// to send. Do sends once and never again, whatever the answer.
type Write[T any] struct {
	// What names the thing in messages: "an application for 15 October".
	What string
	// Find looks the thing up, and returns nil when it is not there. It
	// reads only.
	Find func(ctx context.Context) (*T, error)
	// Send makes the change. It is called once.
	Send func(ctx context.Context) error
	// List is the command that shows such things, for the hints.
	List string
}

// Check refuses with a conflict when the thing is there already. A dry run
// calls it too, since it only reads.
func (w Write[T]) Check(ctx context.Context) error {
	found, err := w.Find(ctx)
	if err != nil {
		return err
	}
	if found != nil {
		return Errorf(KindConflict, "%s exists already", w.What).WithHint("%s", w.hint("it was not sent again."))
	}
	return nil
}

// Do sends the change and looks the thing up afterwards. When the service
// gave no clear answer, it looks instead of sending again. When it still
// cannot tell, the error says so, and the caller must not run the command
// again before somebody has looked.
func (w Write[T]) Do(ctx context.Context) (*T, error) {
	sendErr := w.Send(ctx)
	if sendErr != nil && KindOf(sendErr) != KindUnavailable {
		// A clear no: nothing was made.
		return nil, sendErr
	}
	found, findErr := w.Find(ctx)
	switch {
	case findErr == nil && found != nil:
		return found, nil
	case sendErr != nil:
		return nil, Errorf(KindUnavailable, "it is not known whether %s was sent: %w", w.What, sendErr).
			WithHint("%s", w.hint("do not run the command again before you have looked."))
	case findErr != nil:
		return nil, fmt.Errorf("%s was sent, but could not be looked up afterwards: %w", w.What, findErr)
	}
	return nil, Errorf(KindFailure, "%s was sent, but is not there afterwards", w.What).
		WithHint("%s", w.hint("do not run the command again before you have looked."))
}

func (w Write[T]) hint(text string) string {
	if w.List == "" {
		return text
	}
	return fmt.Sprintf(`%s Run "%s" to see what is there.`, text, w.List)
}
