package ldapsrv

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
	"github.com/teapotovh/teapot/lib/observability"
)

type Handler[T any] interface {
	Bind(ctx context.Context, state T, r ldap.BindRequest) (T, error)
	Search(ctx context.Context, state T, r ldap.SearchRequest) ([]ldap.SearchResultEntry, T, error)
	Add(ctx context.Context, state T, r ldap.AddRequest) (T, error)
	Del(ctx context.Context, state T, r ldap.DelRequest) (T, error)
	Modify(ctx context.Context, state T, r ldap.ModifyRequest) (T, error)
	ModifyDN(ctx context.Context, state T, r ldap.ModifyDNRequest) (T, error)
	Compare(ctx context.Context, state T, r ldap.CompareRequest) (bool, T, error)
	Extended(ctx context.Context, state T, r ldap.ExtendedRequest) (T, error)
}

type UnimplementedHandler[T any] struct {
}

func (u *UnimplementedHandler[T]) Bind(ctx context.Context, state T, _ ldap.BindRequest) (T, error) {
	return state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) Search(
	ctx context.Context,
	state T,
	r ldap.SearchRequest,
) ([]ldap.SearchResultEntry, T, error) {
	return nil, state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) Add(ctx context.Context, state T, r ldap.AddRequest) (T, error) {
	return state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) Del(ctx context.Context, state T, r ldap.DelRequest) (T, error) {
	return state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) Modify(ctx context.Context, state T, r ldap.ModifyRequest) (T, error) {
	return state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) ModifyDN(ctx context.Context, state T, r ldap.ModifyDNRequest) (T, error) {
	return state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) Compare(ctx context.Context, state T, r ldap.CompareRequest) (bool, T, error) {
	return false, state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) Extended(ctx context.Context, state T, r ldap.ExtendedRequest) (T, error) {
	return state, ErrUnimplemented
}

// Ensure UnimplementedHandler implements Handler.
var _ Handler[any] = &UnimplementedHandler[any]{}

// Constant to LDAP Request protocol Type names.
const (
	operationSearch   = "SearchRequest"
	operationBind     = "BindRequest"
	operationAdd      = "AddRequest"
	operationDel      = "DelRequest"
	operationModify   = "ModifyRequest"
	operationModifyDN = "ModifyDNRequest"
	operationCompare  = "CompareRequest"
	operationExtended = "ExtendedRequest"
)

func (s *LDAPSrv[T]) handle(ctx context.Context, state T, w ResponseWriter, r *Message[T]) T {
	// Catch a AbandonRequest not handled by user
	switch v := r.ProtocolOp().(type) {
	case ldap.AbandonRequest:
		// retrieve the request to abandon, and send a abort signal to it
		if requestToAbandon, ok := r.Client.GetMessageByID(int(v)); ok {
			requestToAbandon.Abandon()
		}
	}

	code := ldap.ResultCodeSuccess

	var err error

	start := time.Now()

	defer func() {
		s.metrics.duration.WithLabelValues(r.ProtocolOpName(), ldap.EnumeratedLDAPResultCode[code]).
			Observe(time.Since(start).Seconds())
	}()

	state, code, err = s.runHandler(ctx, state, w, r)

	res := NewResponse(code)
	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}

	// Write with the appropriate format by casting to the correct type
	switch r.ProtocolOpName() {
	case operationBind:
		w.Write(ldap.BindResponse{LDAPResult: res})
	case operationSearch:
		w.Write(ldap.SearchResultDone(res))
	case operationAdd:
		w.Write(ldap.AddResponse(res))
	case operationDel:
		w.Write(ldap.DelResponse(res))
	case operationModify:
		w.Write(ldap.ModifyResponse(res))
	case operationModifyDN:
		w.Write(ldap.ModifyDNResponse(res))
	case operationCompare:
		w.Write(ldap.CompareResponse(res))
	case operationExtended:
		r := ldap.ExtendedResponse{LDAPResult: res}
		r.SetResponseName(NoticeOfPasswordModify)
		w.Write(r)

	default:
		w.Write(res)
	}

	return state
}

func (s *LDAPSrv[T]) runHandler(
	ctx context.Context,
	is T,
	w ResponseWriter,
	r *Message[T],
) (state T, code ldap.ENUMERATED, err error) {
	code = ldap.ResultCodeSuccess

	ctx, span := observability.TracerFromContext(ctx).Start(ctx, r.ProtocolOpName())
	defer func() {
		span.SetAttributes(attribute.Int("code", code.Int()))
		observability.SpanEnd(span, err)
	}()

	span.SetAttributes(
		attribute.String("operation", r.ProtocolOpName()),
		attribute.String("x-request-id", RequestID(ctx).String()),
	)

	if err = checkControls(r); err != nil {
		return state, s.errorCode(ctx, r, err), err
	}

	switch r.ProtocolOpName() {
	case operationBind:
		state, err = s.handler.Bind(ctx, is, r.GetBindRequest())
	case operationSearch:
		var results []ldap.SearchResultEntry

		results, state, err = s.handler.Search(ctx, is, r.GetSearchRequest())

		for _, result := range results {
			w.Write(result)
		}
	case operationAdd:
		state, err = s.handler.Add(ctx, is, r.GetAddRequest())
	case operationDel:
		state, err = s.handler.Del(ctx, is, r.GetDeleteRequest())
	case operationModify:
		state, err = s.handler.Modify(ctx, is, r.GetModifyRequest())
	case operationModifyDN:
		state, err = s.handler.ModifyDN(ctx, is, r.GetModifyDNRequest())
	case operationCompare:
		var matched bool

		matched, state, err = s.handler.Compare(ctx, is, r.GetCompareRequest())

		if matched {
			code = ldap.ResultCodeCompareTrue
		} else {
			code = ldap.ResultCodeCompareFalse
		}
	case operationExtended:
		state, err = s.handler.Extended(ctx, is, r.GetExtendedRequest())
	default:
		err = ErrUnsupported
	}

	if err != nil {
		code = s.errorCode(ctx, r, err)
	}

	return state, code, err
}

// errorCode logs the handler error and returns the LDAP result code.
func (s *LDAPSrv[T]) errorCode(ctx context.Context, r *Message[T], err error) ldap.ENUMERATED {
	s.logger.ErrorContext(ctx, "error while handling operation", "operation", r.ProtocolOpName(), "err", err)

	type withErrorCode interface {
		error
		LDAPCode() ldap.ENUMERATED
	}

	if ec, ok := errors.AsType[withErrorCode](err); ok {
		return ec.LDAPCode()
	}

	return ldap.ResultCodeOther
}

func checkControls[T any](r *Message[T]) error {
	controls := r.Controls()
	if controls == nil {
		return nil
	}

	for _, control := range *controls {
		if control.Criticality() {
			return fmt.Errorf("%w: %q", ErrUnavailableCriticalExtension, control.ControlType())
		}
	}

	return nil
}
