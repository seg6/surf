package clipboard

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("host clipboard integration is unavailable")

type systemClipboard interface {
	Name() string
	Read(context.Context) (string, error)
	Write(context.Context, string) error
}

// unavailableSystem is a shared fallback, not an unsupported-OS
// implementation. Linux also uses it when no Wayland or X11 provider exists.
type unavailableSystem struct{}

func (unavailableSystem) Name() string                         { return "unavailable" }
func (unavailableSystem) Read(context.Context) (string, error) { return "", ErrUnavailable }
func (unavailableSystem) Write(context.Context, string) error  { return ErrUnavailable }
