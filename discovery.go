package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
)

// encodeMessage builds the payload string: "UDP_PORT|TCP_PORT|HOSTNAME"
func encodeMessage(udpPort int, tcpPort int, hostname string) []byte {
	text := fmt.Sprintf("%d|%d|%s", udpPort, tcpPort, hostname)
	return []byte(text)
}

// decodeMessage extracts ports and hostname from a payload string
func decodeMessage(data []byte) (int, int, string, error) {
	parts := strings.Split(string(data), "|")
	if len(parts) < 3 {
		return 0, 0, "", fmt.Errorf("invalid message format")
	}
	udpPort, err1 := strconv.Atoi(parts[0])
	tcpPort, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, "", fmt.Errorf("invalid port format")
	}
	return udpPort, tcpPort, parts[2], nil
}

// getBroadcastAddr determines the subnet broadcast IPv4 address
func getBroadcastAddr(port int) *net.UDPAddr {
	interfaces, err := net.Interfaces()
	if err != nil {
		return &net.UDPAddr{IP: net.IPv4bcast, Port: port}
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			ip := ipNet.IP.To4()
			mask := ipNet.Mask
			broadcast := make(net.IP, len(ip))
			for i := 0; i < len(ip); i++ {
				broadcast[i] = ip[i] | ^mask[i]
			}
			return &net.UDPAddr{IP: broadcast, Port: port}
		}
	}
	return &net.UDPAddr{IP: net.IPv4bcast, Port: port}
}

// Broadcast sends a single UDP broadcast packet announcing presence on the LAN
func Broadcast(udpPort int, tcpPort int, hostname string) {
	addr := getBroadcastAddr(udpPort)
	udpConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer udpConn.Close()

	buffer := encodeMessage(udpPort, tcpPort, hostname)
	_, _ = udpConn.Write(buffer)
}

// ListenBroadcast runs a background UDP listener to discover active peers
func ListenBroadcast(udpPort int, myHostname string, registry *PeerRegistry) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
			return err
		},
	}

	packetConn, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf("0.0.0.0:%d", udpPort))
	if err != nil {
		fmt.Printf("Error starting UDP listener: %v\n", err)
		return
	}
	defer packetConn.Close()

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := packetConn.ReadFrom(buf)
		if err != nil {
			continue
		}

		_, peerTCPPort, peerHostname, err := decodeMessage(buf[:n])
		if err != nil || peerHostname == myHostname {
			continue
		}

		udpAddr, ok := remoteAddr.(*net.UDPAddr)
		if !ok {
			continue
		}
		ipStr := udpAddr.IP.String()

		isNew := registry.AddOrUpdate(peerHostname, ipStr, peerTCPPort)
		if isNew {
			fmt.Printf("\n[DISCOVERED] New peer '%s' discovered at %s (TCP Port: %d)\n(Ghost) > ",
				peerHostname, ipStr, peerTCPPort)
		}
	}
}
