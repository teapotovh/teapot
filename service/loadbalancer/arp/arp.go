package arp

import (
	"fmt"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"log/slog"
	"net"

	"github.com/teapotovh/teapot/service/loadbalancer"
)

type ARPConfig struct {
	Device string
}

type ARP struct {
	logger *slog.Logger

	lb     *loadbalancer.LoadBalancer
	mac    net.HardwareAddr
	handle *afpacket.TPacket

	packets  chan *layers.ARP
	listener *Listener
	speaker  *Speaker
}

func NewARP(lb *loadbalancer.LoadBalancer, config ARPConfig, logger *slog.Logger) (*ARP, error) {
	iface, err := net.InterfaceByName(config.Device)
	if err != nil {
		return nil, fmt.Errorf("error while getting MAC address for interface %q: %w", config.Device, err)
	}
	handle, err := afpacket.NewTPacket(afpacket.OptInterface(config.Device))
	if err != nil {
		return nil, fmt.Errorf("error while opening device %q with afpacket: %w", config.Device, err)
	}

	arp := &ARP{
		logger: logger,

		lb:     lb,
		mac:    iface.HardwareAddr,
		handle: handle,

		packets: make(chan *layers.ARP),
	}

	arp.speaker = NewSpeaker(arp, logger.With("component", "speaker"))
	arp.listener, err = NewListener(arp, ListenerConfig{Device: config.Device}, logger.With("component", "listener"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing listener subcomponent: %w", err)
	}

	return arp, nil
}

func (arp *ARP) Listener() *Listener {
	return arp.listener
}

func (arp *ARP) Speaker() *Speaker {
	return arp.speaker
}
