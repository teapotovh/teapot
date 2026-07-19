package ldapsrv

import (
	"fmt"

	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
)

type Message[T any] struct {
	*ldap.LDAPMessage

	Client *client[T]
	Done   chan bool
}

func (m *Message[T]) String() string {
	return fmt.Sprintf("MessageId=%d, %s", m.MessageID(), m.ProtocolOpName())
}

// Abandon close the Done channel, to notify handler's user function to stop any
// running process.
func (m *Message[T]) Abandon() {
	m.Done <- true
}

func (m *Message[T]) GetAbandonRequest() ldap.AbandonRequest {
	return m.ProtocolOp().(ldap.AbandonRequest)
}

func (m *Message[T]) GetSearchRequest() ldap.SearchRequest {
	return m.ProtocolOp().(ldap.SearchRequest)
}

func (m *Message[T]) GetBindRequest() ldap.BindRequest {
	return m.ProtocolOp().(ldap.BindRequest)
}

func (m *Message[T]) GetAddRequest() ldap.AddRequest {
	return m.ProtocolOp().(ldap.AddRequest)
}

func (m *Message[T]) GetDeleteRequest() ldap.DelRequest {
	return m.ProtocolOp().(ldap.DelRequest)
}

func (m *Message[T]) GetModifyRequest() ldap.ModifyRequest {
	return m.ProtocolOp().(ldap.ModifyRequest)
}

func (m *Message[T]) GetCompareRequest() ldap.CompareRequest {
	return m.ProtocolOp().(ldap.CompareRequest)
}

func (m *Message[T]) GetExtendedRequest() ldap.ExtendedRequest {
	return m.ProtocolOp().(ldap.ExtendedRequest)
}
