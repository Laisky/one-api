package identity

import (
	"errors"
	"fmt"

	"github.com/Laisky/zap"
)

// tagged attaches entity references to an error WITHOUT altering its message.
//
// Error() delegates verbatim to the wrapped error. This is a hard requirement,
// not a style choice: error texts in this repo are (a) returned to API clients
// through relay/adaptor/openai.ErrorWrapper, (b) pattern-matched by the retry and
// channel-health classifiers in controller/relay_error.go and middleware/utils.go,
// and (c) asserted literally by tests. Identity therefore travels beside the
// message, never inside it.
//
// tagged is NOT a substitute for github.com/Laisky/errors/v2 wrapping: it adds no
// stack. Always tag an error that has already been produced by errors.Errorf /
// errors.Wrap / errors.WithStack.
type tagged struct {
	err  error
	refs []Ref
}

// Error returns the wrapped error's message unchanged.
func (t *tagged) Error() string { return t.err.Error() }

// Unwrap supports errors.Is and errors.As.
func (t *tagged) Unwrap() error { return t.err }

// Cause supports the github.com/Laisky/errors/v2 (pkg/errors-style) chain.
func (t *tagged) Cause() error { return t.err }

// Format delegates to the wrapped error so that %+v keeps printing the original
// stack trace captured by errors.Wrap / errors.WithStack.
func (t *tagged) Format(s fmt.State, verb rune) {
	if f, ok := t.err.(fmt.Formatter); ok {
		f.Format(s, verb)
		return
	}
	switch verb {
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", t.err.Error())
	default:
		_, _ = fmt.Fprint(s, t.err.Error())
	}
}

// Refs returns the references attached at this level.
func (t *tagged) Refs() []Ref { return t.refs }

// Tag attaches entity references to err. It is a no-op for a nil error or an
// empty/zero reference list, and it never changes err.Error().
//
// Use it where the entity struct is in hand (typically inside package model) so
// that a log site several layers up, which sees only an error value, can still
// emit the uuid and name via Fields(err).
//
// Parameters:
//   - err: the error to decorate; nil returns nil.
//   - refs: entity references; zero refs are dropped.
//
// Return values:
//   - error: err itself when nothing was attachable, otherwise a transparent
//     wrapper whose message, stack formatting and Is/As behaviour are unchanged.
func Tag(err error, refs ...Ref) error {
	if err == nil {
		return nil
	}
	kept := make([]Ref, 0, len(refs))
	for _, r := range refs {
		if r != nil && !r.IsZero() {
			kept = append(kept, r)
		}
	}
	if len(kept) == 0 {
		return err
	}
	return &tagged{err: err, refs: kept}
}

// Fields walks the error chain and returns the identity fields attached to it,
// outermost first, de-duplicated by field key. It performs no I/O and cannot
// fail, so it is safe on any logging path.
//
// Parameters:
//   - err: the error to inspect; nil returns nil.
//
// Return values:
//   - []zap.Field: identity fields recovered from the chain.
func Fields(err error) []zap.Field {
	if err == nil {
		return nil
	}
	var out []zap.Field
	seen := make(map[string]struct{}, 9)
	for e := err; e != nil; e = errors.Unwrap(e) {
		t, ok := e.(*tagged)
		if !ok {
			continue
		}
		for _, r := range t.refs {
			for _, f := range r.AppendZap(nil) {
				if _, dup := seen[f.Key]; dup {
					continue
				}
				seen[f.Key] = struct{}{}
				out = append(out, f)
			}
		}
	}
	return out
}

// SetFrom extracts a Set from an error chain, for callers that want the values
// rather than zap fields. The outermost tag wins.
//
// Parameters:
//   - err: the error to inspect; nil returns the zero Set.
//
// Return values:
//   - Set: identity recovered from the chain.
func SetFrom(err error) Set {
	var s Set
	for e := err; e != nil; e = errors.Unwrap(e) {
		t, ok := e.(*tagged)
		if !ok {
			continue
		}
		for _, r := range t.refs {
			switch v := r.(type) {
			case UserRef:
				if s.User.IsZero() {
					s.User = v
				}
			case TokenRef:
				if s.Token.IsZero() {
					s.Token = v
				}
			case ChannelRef:
				if s.Channel.IsZero() {
					s.Channel = v
				}
			}
		}
	}
	return s
}
