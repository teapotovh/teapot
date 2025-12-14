package arp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"

	"github.com/teapotovh/teapot/lib/run"
)

type Listener struct {
	logger *slog.Logger

	arp    *ARP
	source *gopacket.PacketSource
}

type ListenerConfig struct {
	Device string
}

func NewListener(arp *ARP, config ListenerConfig, logger *slog.Logger) (*Listener, error) {
	source := gopacket.NewPacketSource(arp.handle, layers.LinkTypeEthernet)

	return &Listener{
		logger: logger,

		arp:    arp,
		source: source,
	}, nil
}

// Run implements run.Runnable
func (lsnr *Listener) Run(ctx context.Context, notify run.Notify) error {
	notify.Notify()
L:
	for {
		select {
		case <-ctx.Done():
			break L
		default:
		}

		packet, err := lsnr.source.NextPacket()
		if errors.Is(err, afpacket.ErrTimeout) {
			continue
		} else if errors.Is(err, io.EOF) {
			break L
		} else if err != nil {
			return fmt.Errorf("error while reading ARP packet: %w", err)
		}

		if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
			packet := arpLayer.(*layers.ARP)
			lsnr.arp.packets <- packet
		}
	}

	close(lsnr.arp.packets)
	return nil
}
