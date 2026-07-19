package ldapsrv

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
	"github.com/teapotovh/teapot/lib/observability"
)

var (
	ErrMessageNotFullyWritten = errors.New("message bytes were not fully written")
)

type client[T any] struct {
	rwc         net.Conn
	chanOut     chan *ldap.LDAPMessage
	srv         *LDAPSrv[T]
	br          *bufio.Reader
	bw          *bufio.Writer
	logger      *slog.Logger
	closing     chan bool
	requestList map[int]*Message[T]
	writeDone   chan bool
	rawData     []byte
	wg          sync.WaitGroup
	id          int
	mutex       sync.Mutex
}

func (c *client[T]) GetConn() net.Conn {
	return c.rwc
}

func (c *client[T]) GetRaw() []byte {
	return c.rawData
}

func (c *client[T]) SetConn(conn net.Conn) {
	c.rwc = conn
	c.br = bufio.NewReader(c.rwc)
	c.bw = bufio.NewWriter(c.rwc)
}

func (c *client[T]) GetMessageByID(messageID int) (*Message[T], bool) {
	if requestToAbandon, ok := c.requestList[messageID]; ok {
		return requestToAbandon, true
	}

	return nil, false
}

func (c *client[T]) Addr() net.Addr {
	return c.rwc.RemoteAddr()
}

func (c *client[T]) ReadPacket() (*messagePacket, error) {
	mP, err := readMessagePacket(c.br)
	c.rawData = make([]byte, len(mP.bytes))
	copy(c.rawData, mP.bytes)

	return mP, err
}

//nolint:all
func (c *client[T]) serve(serveCtx context.Context) {
	ctx, span := c.srv.tracer.Start(
		serveCtx,
		"client.serve",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()
	ctx = observability.ContextWithTracer(ctx, c.srv.tracer)

	c.srv.metrics.active.Inc()
	c.srv.metrics.total.Add(1)
	c.srv.wg.Add(1)
	defer func() {
		c.srv.metrics.active.Dec()
		c.srv.wg.Done()
		if err := c.close(serveCtx); err != nil {
			c.logger.DebugContext(serveCtx, "error while closing client", "err", err)
		}
	}()

	c.closing = make(chan bool)
	// Create the ldap response queue to be writted to client (buffered to 20)
	// buffered to 20 means that If client is slow to handler responses, Server
	// Handlers will stop to send more respones
	c.chanOut = make(chan *ldap.LDAPMessage)
	c.writeDone = make(chan bool)
	// for each message in c.chanOut send it to client
	go func() {
		for msg := range c.chanOut {
			if err := c.writeMessage(serveCtx, msg); err != nil {
				c.logger.ErrorContext(serveCtx, "error while marshaling response", "err", err, "msg", msg)
			}
		}
		close(c.writeDone)
	}()

	// Listen for server signal to shutdown
	go func() {
		for {
			select {
			case <-serveCtx.Done(): // server signals shutdown process
				c.wg.Add(1)
				r := NewExtendedResponse(ldap.ResultCodeUnwillingToPerform)
				r.SetDiagnosticMessage("server is about to stop")
				r.SetResponseName(NoticeOfDisconnection)

				m := ldap.NewLDAPMessageWithProtocolOp(r)

				c.chanOut <- m
				c.wg.Done()
				if err := c.rwc.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
					c.logger.WarnContext(serveCtx, "error while setting read deadline when shutting down", "err", err)
				}
				return
			case <-c.closing:
				return
			}
		}
	}()

	c.requestList = make(map[int]*Message[T])
	state := c.srv.initialState

	for {
		ctx, span := c.srv.tracer.Start(ctx, "client.next")

		if c.srv.readTimeout != 0 {
			if err := c.rwc.SetReadDeadline(time.Now().Add(c.srv.readTimeout)); err != nil {
				c.logger.WarnContext(ctx, "error while setting read deadline", "err", err)
			}
		}
		if c.srv.writeTimeout != 0 {
			if err := c.rwc.SetWriteDeadline(time.Now().Add(c.srv.writeTimeout)); err != nil {
				c.logger.WarnContext(ctx, "error while setting write deadline", "err", err)
			}
		}

		// Read client input as a ASN1/BER binary message
		messagePacket, err := c.ReadPacket()
		if err != nil {
			_, isTimeout := err.(*net.OpError)
			c.logger.DebugContext(
				ctx,
				"error while receiving packet",
				"client",
				c.id,
				"timeout",
				isTimeout,
				"err",
				err,
			)
			span.End()
			return
		}

		// Convert ASN1 binaryMessage to a ldap Message
		message, err := messagePacket.readMessage()
		if err != nil {
			c.logger.DebugContext(ctx, "error while reading packet", "err", err)
			span.End()
			continue
		}

		// Extract tracing information if provided
		var traceparent, tracestate string
		if controls := message.Controls(); controls != nil {
			for _, control := range *controls {
				if control.ControlType() == "1.3.6.1.4.1.1337.1" &&
					control.ControlValue() != nil {
					parts := bytes.SplitN(control.ControlType().Bytes(), []byte{0}, 2)

					traceparent = string(parts[0])
					tracestate = ""
					if len(parts) == 2 {
						tracestate = string(parts[1])
					}

					break
				}
			}
		}

		carrier := propagation.MapCarrier{
			"traceparent": traceparent,
			"tracestate":  tracestate,
		}
		ctx = propagation.TraceContext{}.Extract(ctx, carrier)

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

		state = c.ProcessRequestMessage(ctx, state, &message)
	}
}

// ResponseWriter interface is used by an LDAP handler to
// construct an LDAP response.
type ResponseWriter interface {
	// Write writes the LDAPResponse to the connection as part of an LDAP reply.
	Write(po ldap.ProtocolOp)
}

type responseWriterImpl struct {
	chanOut   chan *ldap.LDAPMessage
	messageID ldap.MessageID
}

func (w responseWriterImpl) Write(po ldap.ProtocolOp) {
	m := ldap.NewLDAPMessageWithProtocolOp(po)
	m.SetMessageID(w.messageID)

	w.chanOut <- m
}

func (c *client[T]) ProcessRequestMessage(ctx context.Context, state T, message *ldap.LDAPMessage) T {
	c.wg.Add(1)
	defer c.wg.Done()

	m := Message[T]{
		LDAPMessage: message,
		Done:        make(chan bool, 2),
		Client:      c,
	}

	c.registerRequest(&m)
	defer c.unregisterRequest(&m)

	var w responseWriterImpl

	w.chanOut = c.chanOut
	w.messageID = m.MessageID()

	return c.srv.handle(ctx, state, w, &m)
}

func (c *client[T]) registerRequest(m *Message[T]) {
	c.mutex.Lock()
	c.requestList[m.MessageID().Int()] = m
	c.mutex.Unlock()
}

func (c *client[T]) unregisterRequest(m *Message[T]) {
	c.mutex.Lock()
	delete(c.requestList, m.MessageID().Int())
	c.mutex.Unlock()
}

func (c *client[T]) writeMessage(ctx context.Context, msg *ldap.LDAPMessage) error {
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

// close closes client,
// * stop reading from client
// * signals to all currently running request processor to stop
// * wait for all request processor to end
// * close client connection
// * signal to server that client shutdown is ok.
func (c *client[T]) close(ctx context.Context) error {
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

	return nil
}
