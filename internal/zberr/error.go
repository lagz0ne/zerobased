package zberr

import (
	"errors"
	"fmt"
)

type Layer string
type Code string
type Kind string

const (
	KindError  Kind = "error"
	KindStatus Kind = "status"
)

type Error struct {
	Layer  Layer
	Code   Code
	Phase  string
	Owner  string
	Cause  error
	Origin []Layer
}

type Option func(*Error)

func New(layer Layer, code Code, options ...Option) *Error {
	err := &Error{
		Layer: layer,
		Code:  code,
	}
	for _, option := range options {
		option(err)
	}
	return err
}

func Critical(layer Layer, cause error, origin ...Layer) *Error {
	chain := append([]Layer(nil), origin...)
	if len(chain) == 0 {
		chain = []Layer{layer}
		var zerr *Error
		if errors.As(cause, &zerr) {
			chain = append(chain, zerr.Origin...)
		}
	}
	return New(layer, CodeCritical, WithCause(cause), WithOrigin(chain...))
}

func WithPhase(phase string) Option {
	return func(err *Error) {
		err.Phase = phase
	}
}

func WithOwner(owner string) Option {
	return func(err *Error) {
		err.Owner = owner
	}
}

func WithCause(cause error) Option {
	return func(err *Error) {
		err.Cause = cause
	}
}

func WithOrigin(origin ...Layer) Option {
	return func(err *Error) {
		err.Origin = append([]Layer(nil), origin...)
	}
}

func (err *Error) Error() string {
	if err == nil {
		return "<nil>"
	}

	msg := fmt.Sprintf("%s:%s", err.Layer, err.Code)
	if err.Phase != "" {
		msg += " phase=" + err.Phase
	}
	if err.Owner != "" {
		msg += " owner=" + err.Owner
	}
	if err.Cause != nil {
		msg += ": " + err.Cause.Error()
	}
	return msg
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func Validate(err *Error) error {
	if err == nil {
		return nil
	}
	if err.Layer == LayerCLI && (err.Code == CodeStartFailed || err.Code == CodeUpFailed) && err.Phase == "" {
		return fmt.Errorf("%s/%s requires phase", err.Layer, err.Code)
	}
	if err.Code == CodeCritical && len(err.Origin) == 0 {
		return fmt.Errorf("%s/%s requires origin chain", err.Layer, err.Code)
	}
	return nil
}

func Is(err error, layer Layer, code Code) bool {
	for err != nil {
		var zerr *Error
		if errors.As(err, &zerr) && zerr.Layer == layer && zerr.Code == code {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}
