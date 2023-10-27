package ip

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"go.uber.org/zap"
)

type PacketReader struct {
	handle *pcap.Handle
	ch     chan []byte
	iface  string
	Recv   func(packet gopacket.Packet)
}

func NewReader(iface string) (*PacketReader, error) {
	handle, err := pcap.OpenLive(iface, 4096, false, pcap.BlockForever)
	if err != nil {
		return nil, err
	}
	err = handle.SetBPFFilter("udp and (ip multicast or ip6 multicast)")
	if err != nil {
		return nil, err
	}

	return &PacketReader{
		handle: handle,
		ch:     make(chan []byte, 100),
		iface:  iface,
	}, nil
}

// Shutdown stops the interface capture handle, releases it, and causes Run to
// return io.EOF.
func (p *PacketReader) Shutdown() {
	p.handle.Close()
}

func (p *PacketReader) Run() error {
	go p.read()

	var err error
	var data []byte

	for err == nil {
		data, _, err = p.handle.ReadPacketData()
		if err != nil {
			break
		}

		p.ch <- data
	}
	close(p.ch)
	return err
}

func (p *PacketReader) read() {
	for rawData := range p.ch {
		packet := gopacket.NewPacket(rawData, layers.LayerTypeEthernet, gopacket.Default)
		p.Recv(packet)
	}
}
