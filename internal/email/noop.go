package email

import "context"

// noopProvider discards every message silently.
// Used when Config.Provider == "disabled" and in unit tests.
type noopProvider struct{}

// NewNoop returns a Provider that silently discards all messages.
// Safe to use in tests and local dev environments where email is not needed.
func NewNoop() Provider {
	return &noopProvider{}
}

func (n *noopProvider) Send(_ context.Context, _ Message) error {
	return nil
}
