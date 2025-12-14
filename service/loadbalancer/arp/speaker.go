package arp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/teapotovh/teapot/lib/run"
)

type unit struct{}

type Speaker struct {
	logger *slog.Logger

	arp *ARP
	ips map[netip.Addr]unit
}

func NewSpeaker(arp *ARP, logger *slog.Logger) *Speaker {
	return &Speaker{
		logger: logger,

		arp: arp,
		ips: map[netip.Addr]unit{},
	}
}

func filterIPv4s(ips []netip.Addr) []netip.Addr {
	var result []netip.Addr

	for _, ip := range ips {
		if ip.Is4() {
			result = append(result, ip)
		}
	}

	return result
}

func (spkr *Speaker) handlePacket(packet *layers.ARP) error {
	srcIP := netip.AddrFrom4([net.IPv4len]byte(packet.SourceProtAddress))

	dstIP := netip.AddrFrom4([net.IPv4len]byte(packet.DstProtAddress))
	if _, registered := spkr.ips[dstIP]; !registered || packet.Operation == layers.ARPRequest {
		return nil
	}

	spkr.logger.Debug(
		"received APR request",
		"dst_ip",
		dstIP,
		"dst_mac",
		spkr.arp.mac,
		"src_ip",
		srcIP,
		"src_mac",
		packet.DstHwAddress,
	)

	reply := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		HwAddressSize:     6,
		ProtAddressSize:   net.IPv4len,
		Operation:         layers.ARPReply,
		SourceHwAddress:   spkr.arp.mac,
		SourceProtAddress: dstIP.AsSlice(),
		DstHwAddress:      packet.SourceHwAddress,
		DstProtAddress:    packet.SourceProtAddress,
	}

	eth := &layers.Ethernet{
		SrcMAC:       spkr.arp.mac,
		DstMAC:       packet.SourceHwAddress,
		EthernetType: layers.EthernetTypeARP,
	}

	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{}, eth, reply); err != nil {
		return fmt.Errorf("error while serializing ARP reply: %w", err)
	}

	if err := spkr.arp.handle.WritePacketData(buf.Bytes()); err != nil {
		return fmt.Errorf("error while writing ARP reply on the wire: %w", err)
	}

	spkr.logger.Debug("repleid to ARP request", "packet", reply)

	return nil
}

// Run implements run.Runnable.
func (spkr *Speaker) Run(ctx context.Context, notify run.Notify) error {
	sub := spkr.arp.lb.Broker().Subscribe()
	defer sub.Unsubscribe()

	notify.Notify()

L:
	for {
		select {
		case <-ctx.Done():
			break L
		case update := <-sub.Chan():
			filtered := filterIPv4s(update)
			spkr.logger.Info("updated handled IPs", "ips", filtered)

			for _, ip := range filtered {
				spkr.ips[ip] = unit{}
			}
		case packet := <-spkr.arp.packets:
			if err := spkr.handlePacket(packet); err != nil {
				return fmt.Errorf("error while handling incoming packet %q: %w", packet, err)
			}
		}
	}

	return nil
}
