package ldapsrv

import (
	"context"
	"errors"
	"time"

	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
)

type Handler interface {
	Bind(ctx context.Context, r ldap.BindRequest) (context.Context, error)
	Search(ctx context.Context, r ldap.SearchRequest) ([]ldap.SearchResultEntry, error)
	Add(ctx context.Context, r ldap.AddRequest) error
	Del(ctx context.Context, r ldap.DelRequest) error
	Modify(ctx context.Context, r ldap.ModifyRequest) error
	Compare(ctx context.Context, r ldap.CompareRequest) (bool, error)
	Extended(ctx context.Context, r ldap.ExtendedRequest) error
}

type UnimplementedHandler struct {
}

func (u *UnimplementedHandler) Bind(ctx context.Context, _ ldap.BindRequest) (context.Context, error) {
	return ctx, ErrUnimplemented
}

func (u *UnimplementedHandler) Search(ctx context.Context, r ldap.SearchRequest) ([]ldap.SearchResultEntry, error) {
	return nil, ErrUnimplemented
}

func (u *UnimplementedHandler) Add(ctx context.Context, r ldap.AddRequest) error {
	return ErrUnimplemented
}

func (u *UnimplementedHandler) Del(ctx context.Context, r ldap.DelRequest) error {
	return ErrUnimplemented
}

func (u *UnimplementedHandler) Modify(ctx context.Context, r ldap.ModifyRequest) error {
	return ErrUnimplemented
}

func (u *UnimplementedHandler) Compare(ctx context.Context, r ldap.CompareRequest) (bool, error) {
	return false, ErrUnimplemented
}

func (u *UnimplementedHandler) Extended(ctx context.Context, r ldap.ExtendedRequest) error {
	return ErrUnimplemented
}

// Ensure UnimplementedHandler implements Handler.
var _ Handler = &UnimplementedHandler{}

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
func (s *LDAPSrv) handle(ctx context.Context, w ResponseWriter, r *Message) context.Context {
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
		s.metrics.duration.WithLabelValues(r.ProtocolOpName(), ldap.EnumeratedLDAPResultCode[code]).Observe(time.Since(start).Seconds())
	}()

	switch r.ProtocolOpName() {
	case operationBind:
		ctx, err = s.handler.Bind(ctx, r.GetBindRequest())
	case operationSearch:
		var results []ldap.SearchResultEntry

		results, err = s.handler.Search(ctx, r.GetSearchRequest())

		for _, result := range results {
			w.Write(result)
		}
	case operationAdd:
		err = s.handler.Add(ctx, r.GetAddRequest())
	case operationDel:
		err = s.handler.Del(ctx, r.GetDeleteRequest())
	case operationModify:
		err = s.handler.Modify(ctx, r.GetModifyRequest())
	case operationCompare:
		var matched bool

		matched, err = s.handler.Compare(ctx, r.GetCompareRequest())

		if matched {
			code = ldap.ResultCodeCompareTrue
		} else {
			code = ldap.ResultCodeCompareFalse
		}
	case operationExtended:
		err = s.handler.Extended(ctx, r.GetExtendedRequest())
	default:
		err = ErrUnsupported
	}

	// If the error is not nil, extract the status code from the error type, if available,
	// let's use the unknown error code 'Other'.
	if err != nil {
		s.logger.Error("error while handling operation", "operation", r.ProtocolOpName(), "err", err)

		type withErrorCode interface{ LDAPCode() ldap.ENUMERATED }

		var ec withErrorCode
		if errors.As(err, &ec) {
			code = ec.LDAPCode()
		} else {
			code = ldap.ResultCodeOther
		}
	}

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

	return ctx
}
