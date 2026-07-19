package ldapsrv

import (
	"context"
	"errors"
	"time"

	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
	"github.com/teapotovh/teapot/lib/observability"
	"go.opentelemetry.io/otel/attribute"
)

type Handler[T any] interface {
	Bind(ctx context.Context, state T, r ldap.BindRequest) (T, error)
	Search(ctx context.Context, state T, r ldap.SearchRequest) ([]ldap.SearchResultEntry, T, error)
	Add(ctx context.Context, state T, r ldap.AddRequest) (T, error)
	Del(ctx context.Context, state T, r ldap.DelRequest) (T, error)
	Modify(ctx context.Context, state T, r ldap.ModifyRequest) (T, error)
	Compare(ctx context.Context, state T, r ldap.CompareRequest) (bool, T, error)
	Extended(ctx context.Context, state T, r ldap.ExtendedRequest) (T, error)
}

type UnimplementedHandler[T any] struct {
}

func (u *UnimplementedHandler[T]) Bind(ctx context.Context, state T, _ ldap.BindRequest) (T, error) {
	return state, ErrUnimplemented
}

func (u *UnimplementedHandler[T]) Search(ctx context.Context, state T, r ldap.SearchRequest) ([]ldap.SearchResultEntry, T, error) {
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
	operationCompare  = "CompareRequest"
	operationExtended = "ExtendedRequest"
)

//nolint:gocyclo
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

func (s *LDAPSrv[T]) runHandler(ctx context.Context, is T, w ResponseWriter, r *Message[T]) (state T, code ldap.ENUMERATED, err error) {
	code = ldap.ResultCodeSuccess

	ctx, span := observability.TracerFromContext(ctx).Start(ctx, r.ProtocolOpName())
	defer observability.SpanEnd(span, err)

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

	// If the error is not nil, extract the status code from the error type, if available,
	// let's use the unknown error code 'Other'.
	if err != nil {
		s.logger.ErrorContext(ctx, "error while handling operation", "operation", r.ProtocolOpName(), "err", err)

		type withErrorCode interface{ LDAPCode() ldap.ENUMERATED }

		var ec withErrorCode
		if errors.As(err, &ec) {
			code = ec.LDAPCode()
		} else {
			code = ldap.ResultCodeOther
		}
	}

	span.SetAttributes(attribute.Int("code", code.Int()))

	return
}
