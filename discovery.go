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

// getBroadcastAddresses gets the list of Broadcast addresses of all network cards
func getBroadcastAddr(port int) []*net.UDPAddr {
	var addrs []*net.UDPAddr
	interfaces, err := net.Interfaces()
	if err != nil {
		return []*net.UDPAddr{{IP: net.IPv4bcast, Port: port}}
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrsList, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrsList {
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
			addrs = append(addrs, &net.UDPAddr{IP: broadcast, Port: port})
		}
	}
	addrs = append(addrs, &net.UDPAddr{IP: net.IPv4bcast, Port: port})
	return addrs
}

// Broadcast sends a single UDP broadcast packet announcing presence on the LAN
func Broadcast(udpPort int, tcpPort int, hostname string) {
	buffer := encodeMessage(udpPort, tcpPort, hostname)
	addrs := getBroadcastAddr(udpPort)

	for _, addr := range addrs {
		udpConn, err := net.DialUDP("udp4", nil, addr)
		if err != nil {
			continue
		}
		_, _ = udpConn.Write(buffer)
		udpConn.Close()
	}
}

// ListenBroadcast runs a background UDP listener to discover active peers
func ListenBroadcast(udpPort int, myHostname string, registry *PeerRegistry) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				//go get golang.org/x/sys/unix
				err = setReuseControl(fd)
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
