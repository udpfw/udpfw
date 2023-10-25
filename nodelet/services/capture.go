package services

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/udpfw/nodelet/ip"
	"io"
	"sync"
	"syscall"
)

func NewPacketHandler(iface string) (*PacketHandler, error) {
	packetChan := make(chan []byte, 4096)
	reader, err := ip.NewReader(iface)
	if err != nil {
		return nil, err
	}
	reader.Recv = func(packet gopacket.Packet) {
		packetChan <- packet.Data()
	}

	fd4, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return nil, err
	}

	fd6, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return nil, err
	}

	return &PacketHandler{
		reader:     reader,
		packetChan: packetChan,
		writeLock:  &sync.Mutex{},
		sock4Fd:    fd4,
		sock6Fd:    fd6,
	}, nil
}

type PacketHandler struct {
	packetChan  chan []byte
	writeLock   *sync.Mutex
	reader      *ip.PacketReader
	errorLocked bool
	sock4Fd     int
	sock6Fd     int
}

func (c *PacketHandler) Start() error {
	err := c.reader.Run()
	if err == io.EOF {
		err = nil
	}
	return err
}

func (c *PacketHandler) NextPacket() []byte { return <-c.packetChan }

func (c *PacketHandler) Shutdown() { c.reader.Shutdown() }

func (c *PacketHandler) Inject(pkt []byte) error {
	target, addr, data := c.RoutePacket(pkt)
	if err := syscall.Sendto(target, data, 0, addr); err != nil {
		// TODO: Log
		return err
	}

	return nil
}

func (c *PacketHandler) RoutePacket(rawPkt []byte) (int, syscall.Sockaddr, []byte) {
	pkt := gopacket.NewPacket(rawPkt, layers.LayerTypeEthernet, gopacket.Default)
	udpLayer := pkt.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		fmt.Printf("Dropped packet with no UDP layer(?)\n")
		return -1, nil, nil
	}

	udp := udpLayer.(*layers.UDP)
	err := udp.SetNetworkLayerForChecksum(pkt.NetworkLayer())
	if err != nil {
		fmt.Printf("Failed to set network layer for checksum: %s\n", err)
		return -1, nil, nil
	}

	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
	}

	buf := gopacket.NewSerializeBuffer()
	if err = gopacket.SerializePacket(buf, options, pkt); err != nil {
		fmt.Printf("Failed serializing packet: %#v\n", err)
		return -1, nil, nil
	}

	addr, err := extractAddress(pkt, udp)
	if err != nil {
		fmt.Printf("Failed extracting address: %s\n", err)
		return -1, nil, nil
	}

	var target int
	switch addr.(type) {
	case *syscall.SockaddrInet4:
		target = c.sock4Fd
	default:
		target = c.sock6Fd
	}

	return target, addr, buf.Bytes()[len(pkt.LinkLayer().LayerContents()):]
}

func extractAddress(pkt gopacket.Packet, udp *layers.UDP) (syscall.Sockaddr, error) {
	if ipLayer := pkt.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ipLayer, _ := ipLayer.(*layers.IPv4)
		ipLayer.TTL = 255
		return &syscall.SockaddrInet4{
			Port: int(udp.DstPort),
			Addr: [4]byte(ipLayer.DstIP.To4()),
		}, nil

	}

	if ipLayer := pkt.Layer(layers.LayerTypeIPv6); ipLayer != nil {
		ipLayer, _ := ipLayer.(*layers.IPv6)
		ipLayer.HopLimit = 128
		return &syscall.SockaddrInet6{
			Port:   int(udp.DstPort),
			Addr:   [16]byte(ipLayer.DstIP.To16()),
			ZoneId: 0,
		}, nil
	}

	return nil, fmt.Errorf("no compatible IP layer")
}
