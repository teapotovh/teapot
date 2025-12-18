package httphandler

type InternalError struct {
	internal error
	external error
}

func NewInternalError(internal, external error) InternalError {
	return InternalError{internal: internal, external: external}
}

// Ensure InternalError implements error.
var _ error = InternalError{}

func (ie InternalError) Error() string {
	return ie.internal.Error()
}

func (ie InternalError) External() error {
	if ie.external == nil {
		return ErrInternal
	}

	return ie.external
}
