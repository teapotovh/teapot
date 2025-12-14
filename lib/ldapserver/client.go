package ldapserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	ldap "github.com/teapotovh/teapot/lib/ldapserver/goldap"
)

var (
	ErrMessageNotFullyWritten = errors.New("message bytes were not fully written")
)

type client struct {
	rwc         net.Conn
	chanOut     chan *ldap.LDAPMessage
	srv         *Server
	br          *bufio.Reader
	bw          *bufio.Writer
	logger      *slog.Logger
	closing     chan bool
	requestList map[int]*Message
	writeDone   chan bool
	rawData     []byte
	wg          sync.WaitGroup
	id          int
	mutex       sync.Mutex
}

func (c *client) GetConn() net.Conn {
	return c.rwc
}

func (c *client) GetRaw() []byte {
	return c.rawData
}

func (c *client) SetConn(conn net.Conn) {
	c.rwc = conn
	c.br = bufio.NewReader(c.rwc)
	c.bw = bufio.NewWriter(c.rwc)
}

func (c *client) GetMessageByID(messageID int) (*Message, bool) {
	if requestToAbandon, ok := c.requestList[messageID]; ok {
		return requestToAbandon, true
	}
	return nil, false
}

func (c *client) Addr() net.Addr {
	return c.rwc.RemoteAddr()
}

func (c *client) ReadPacket() (*messagePacket, error) {
	mP, err := readMessagePacket(c.br)
	c.rawData = make([]byte, len(mP.bytes))
	copy(c.rawData, mP.bytes)
	return mP, err
}

//nolint:all
func (c *client) serve(ctx context.Context) {
	c.srv.wg.Add(1)
	c.closing = make(chan bool)
	defer func() {
		if err := c.close(ctx); err != nil {
			c.logger.DebugContext(ctx, "error while closing client", "err", err)
		}
	}()

	if onc := c.srv.OnNewConnection; onc != nil {
		if err := onc(c.rwc); err != nil {
			c.logger.DebugContext(ctx, "error while running OnNewConnection", "err", err)
			return
		}
	}

	// Create the ldap response queue to be writted to client (buffered to 20)
	// buffered to 20 means that If client is slow to handler responses, Server
	// Handlers will stop to send more respones
	c.chanOut = make(chan *ldap.LDAPMessage)
	c.writeDone = make(chan bool)
	// for each message in c.chanOut send it to client
	go func() {
		for msg := range c.chanOut {
			if err := c.writeMessage(ctx, msg); err != nil {
				c.logger.ErrorContext(ctx, "error while marshaling response", "err", err, "msg", msg)
			}
		}
		close(c.writeDone)
	}()

	// Listen for server signal to shutdown
	go func() {
		for {
			select {
			case <-ctx.Done(): // server signals shutdown process
				c.wg.Add(1)
				r := NewExtendedResponse(ldap.ResultCodeUnwillingToPerform)
				r.SetDiagnosticMessage("server is about to stop")
				r.SetResponseName(NoticeOfDisconnection)

				m := ldap.NewLDAPMessageWithProtocolOp(r)

				c.chanOut <- m
				c.wg.Done()
				if err := c.rwc.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
					c.logger.WarnContext(ctx, "error while setting read deadline when shutting down", "err", err)
				}
				return
			case <-c.closing:
				return
			}
		}
	}()

	c.requestList = make(map[int]*Message)

	for {
		if c.srv.ReadTimeout != 0 {
			if err := c.rwc.SetReadDeadline(time.Now().Add(c.srv.ReadTimeout)); err != nil {
				c.logger.WarnContext(ctx, "error while setting read deadline", "err", err)
			}
		}
		if c.srv.WriteTimeout != 0 {
			if err := c.rwc.SetWriteDeadline(time.Now().Add(c.srv.WriteTimeout)); err != nil {
				c.logger.WarnContext(ctx, "error while setting write deadline", "err", err)
			}
		}

		// Read client input as a ASN1/BER binary message
		messagePacket, err := c.ReadPacket()
		if err != nil {
			_, isTimeout := err.(*net.OpError)
			c.logger.DebugContext(ctx, "error while receiving packet", "client", c.id, "timeout", isTimeout, "err", err)
			return
		}

		// Convert ASN1 binaryMessage to a ldap Message
		message, err := messagePacket.readMessage()
		if err != nil {
			c.logger.DebugContext(ctx, "error while reading packet", "err", err)
			continue
		}

		if br, ok := message.ProtocolOp().(ldap.BindRequest); ok {
			c.logger.DebugContext(ctx, "got bind request", "user", br.Name())
		} else {
			c.logger.DebugContext(ctx, "got request", "message", message)
		}

		// TODO: Use a implementation to limit runnuning request by client
		// solution 1 : when the buffered output channel is full, send a busy
		// solution 2 : when 10 client requests (goroutines) are running, send a busy message
		// And when the limit is reached THEN send a BusyLdapMessage

		// When message is an UnbindRequest, stop serving
		if _, ok := message.ProtocolOp().(ldap.UnbindRequest); ok {
			return
		}

		// If client requests a startTls, do not handle it in a
		// goroutine, connection has to remain free until TLS is OK
		// @see RFC https://tools.ietf.org/html/rfc4511#section-4.14.1
		if req, ok := message.ProtocolOp().(ldap.ExtendedRequest); ok {
			if req.RequestName() == NoticeOfStartTLS {
				c.ProcessRequestMessage(ctx, &message)
				continue
			}
		}

		ctx = c.ProcessRequestMessage(ctx, &message)
	}
}

// close closes client,
// * stop reading from client
// * signals to all currently running request processor to stop
// * wait for all request processor to end
// * close client connection
// * signal to server that client shutdown is ok
func (c *client) close(ctx context.Context) error {
	c.logger.DebugContext(ctx, "closing connection")
	close(c.closing)

	// stop reading from client
	if err := c.rwc.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		return fmt.Errorf("error while setting read deadline when closing connection: %w", err)
	}

	// signals to all currently running request processor to stop
	c.mutex.Lock()
	for messageID, request := range c.requestList {
		c.logger.DebugContext(ctx, "abandoning message", "mid", messageID)
		go request.Abandon()
	}
	c.mutex.Unlock()

	c.wg.Wait()      // wait for all current running request processor to end
	close(c.chanOut) // No more message will be sent to client, close chanOUT

	<-c.writeDone // Wait for the last message sent to be written
	// close client connection
	if err := c.rwc.Close(); err != nil {
		return fmt.Errorf("error while closing network connection: %w", err)
	}
	c.logger.DebugContext(ctx, "connection closed successfully")

	c.srv.wg.Done() // signal to server that client shutdown is ok
	return nil
}

func (c *client) writeMessage(ctx context.Context, msg *ldap.LDAPMessage) error {
	data, err := msg.Write()
	if err != nil {
		return fmt.Errorf("error while encoding message: %w", err)
	}

	c.logger.DebugContext(ctx, "sending message", "msg", msg)
	l, err := c.bw.Write(data.Bytes())
	if err != nil {
		return fmt.Errorf("error while writing message: %w", err)
	}
	if l != len(data.Bytes()) {
		return ErrMessageNotFullyWritten
	}
	if err := c.bw.Flush(); err != nil {
		return fmt.Errorf("error while flushing message: %w", err)
	}
	return nil
}

// ResponseWriter interface is used by an LDAP handler to
// construct an LDAP response.
type ResponseWriter interface {
	// Write writes the LDAPResponse to the connection as part of an LDAP reply.
	Write(po ldap.ProtocolOp)
}

type responseWriterImpl struct {
	chanOut   chan *ldap.LDAPMessage
	messageID int32
}

func (w responseWriterImpl) Write(po ldap.ProtocolOp) {
	m := ldap.NewLDAPMessageWithProtocolOp(po)
	m.SetMessageID(w.messageID)
	w.chanOut <- m
}

type DefaultUser[T any] func() T

func GetUser[T any](ctx context.Context, def DefaultUser[T]) T {
	if user := ctx.Value(ContextKeyUser); user != nil {
		return user.(T)
	}

	return def()
}

func WithUser[T any](ctx context.Context, user T) context.Context {
	return context.WithValue(ctx, ContextKeyUser, user)
}

func (c *client) ProcessRequestMessage(ctx context.Context, message *ldap.LDAPMessage) context.Context {
	c.wg.Add(1)
	defer c.wg.Done()

	m := Message{
		LDAPMessage: message,
		Done:        make(chan bool, 2),
		Client:      c,
	}

	c.registerRequest(&m)
	defer c.unregisterRequest(&m)

	var w responseWriterImpl
	w.chanOut = c.chanOut
	w.messageID = int32(m.MessageID())

	return c.srv.Handler.ServeLDAP(ctx, w, &m)
}

func (c *client) registerRequest(m *Message) {
	c.mutex.Lock()
	c.requestList[m.MessageID().Int()] = m
	c.mutex.Unlock()
}

func (c *client) unregisterRequest(m *Message) {
	c.mutex.Lock()
	delete(c.requestList, m.MessageID().Int())
	c.mutex.Unlock()
}
