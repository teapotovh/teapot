package broker

import (
	"context"
	"iter"
)

type unit struct{}

const bufferSize = 5

type Broker[T any] struct {
	publishCh chan T
	subCh     chan chan T
	unsubCh   chan chan T
}

func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{
		publishCh: make(chan T),
		subCh:     make(chan chan T),
		unsubCh:   make(chan chan T),
	}
}

func (b *Broker[T]) Run(ctx context.Context) {
	subs := map[chan T]unit{}
	doneCn := ctx.Done()

	for {
		select {
		case <-doneCn:
			for msgCh := range subs {
				close(msgCh)
			}

			return

		case msgCh := <-b.subCh:
			subs[msgCh] = unit{}

		case msgCh := <-b.unsubCh:
			delete(subs, msgCh)
			close(msgCh)

		case msg := <-b.publishCh:
			for msgCh := range subs {
				select {
				case msgCh <- msg:
				default:
				}
			}
		}
	}
}

func (b *Broker[T]) Publish(msg T) {
	b.publishCh <- msg
}

func (b *Broker[T]) unsubscribe(msgCh chan T) {
	b.unsubCh <- msgCh
}

type Subscriber[T any] struct {
	msgCh       chan T
	unsubscribe func(chan T)
}

func (b *Broker[T]) Subscribe() Subscriber[T] {
	msgCh := make(chan T, bufferSize)
	b.subCh <- msgCh

	return Subscriber[T]{
		msgCh:       msgCh,
		unsubscribe: b.unsubscribe,
	}
}

func (s *Subscriber[T]) Iter(ctx context.Context) iter.Seq[T] {
	doneCh := ctx.Done()

	return func(yield func(T) bool) {
	L:
		for {
			select {
			case msg, open := <-s.msgCh:
				if !open || !yield(msg) {
					break L
				}
			case <-doneCh:
				break L
			}
		}
	}
}

func (s *Subscriber[T]) Chan() chan T {
	return s.msgCh
}

func (s *Subscriber[T]) Unsubscribe() {
	s.unsubscribe(s.msgCh)
}
