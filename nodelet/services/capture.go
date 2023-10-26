package services

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/udpfw/nodelet/ip"
	"go.uber.org/zap"
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
		log:        zap.L().With(zap.String("facility", "packet_handler")),
		reader:     reader,
		packetChan: packetChan,
		writeLock:  &sync.Mutex{},
		sock4Fd:    fd4,
		sock6Fd:    fd6,
		iface:      iface,
	}, nil
}

type PacketHandler struct {
	packetChan  chan []byte
	writeLock   *sync.Mutex
	reader      *ip.PacketReader
	errorLocked bool
	sock4Fd     int
	sock6Fd     int
	log         *zap.Logger
	iface       string
}

func (c *PacketHandler) Start() error {
	c.log.Info("Packet handler now capturing and injecting packets", zap.String("iface", c.iface))
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
		c.log.Error("Failed pushing packet to raw socket",
			zap.ByteString("data", data),
			zap.Error(err))
		return err
	}

	return nil
}

func (c *PacketHandler) RoutePacket(rawPkt []byte) (int, syscall.Sockaddr, []byte) {
	pkt := gopacket.NewPacket(rawPkt, layers.LayerTypeEthernet, gopacket.Default)
	c.log.Debug("Routing packet", zap.String("dump", pkt.Dump()))
	udpLayer := pkt.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		c.log.Info("Dropped packet with no UDP layer", zap.ByteString("packet", rawPkt))
		return -1, nil, nil
	}

	udp := udpLayer.(*layers.UDP)
	err := udp.SetNetworkLayerForChecksum(pkt.NetworkLayer())
	if err != nil {
		c.log.Error("Failed setting network layer for checksum", zap.Error(err))
		return -1, nil, nil
	}

	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
	}

	buf := gopacket.NewSerializeBuffer()
	if err = gopacket.SerializePacket(buf, options, pkt); err != nil {
		c.log.Error("Failed serializing packet", zap.Error(err))
		return -1, nil, nil
	}

	addr, err := extractAddress(pkt, udp)
	if err != nil {
		c.log.Error("Failed extracting address", zap.Error(err))
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
