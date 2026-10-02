package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
)

// encodeMessage formats port and hostname into byte slice
// Format: "PORT|HOSTNAME" (e.g., "8888|Ghost-PC")
func encodeMessage(port int, hostname string) []byte {
	text := fmt.Sprintf("%d|%s", port, hostname)
	return []byte(text)
}

// decodeMessage parses raw byte payload into a port integer and hostname string
func decodeMessage(data []byte) (int, string, error) {
	parts := strings.Split(string(data), "|")
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("invalid message format")
	}
	port, err := strconv.Atoi(parts[0])
	return port, parts[1], err
}

// getBroadcastAddr calculates the active LAN subnet's broadcast IPv4 address
func getBroadcastAddr(port int) *net.UDPAddr {
	interfaces, err := net.Interfaces()
	if err != nil { // fallback to limited broadcast (255.255.255.255) if interface lookup fails
		return &net.UDPAddr{IP: net.IPv4bcast, Port: port}
	}

	for _, iface := range interfaces {
		// skip inactive or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil { //ipv4? if no skip
				continue
			}

			ip := ipNet.IP.To4()
			mask := ipNet.Mask
			broadcast := make(net.IP, len(ip))
			for i := 0; i < len(ip); i++ { //bitwise operation Broadcast IP = IP | (^SubnetMask)
				broadcast[i] = ip[i] | ^mask[i]
			}

			return &net.UDPAddr{IP: broadcast, Port: port}
		}
	}

	return &net.UDPAddr{IP: net.IPv4bcast, Port: port}
}

// broadcast sends a single UDP broadcast packet announcing presence to the LAN
func Broadcast(port int, hostname string) {
	addr := getBroadcastAddr(port)
	fmt.Printf("sending broadcast message to: %s\n", addr)

	udpConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Printf("Error dialing UDP: %v\n", err)
		return
	}
	defer udpConn.Close()

	buffer := encodeMessage(port, hostname)
	_, err = udpConn.Write(buffer)

	if err != nil {
		fmt.Printf("Error sending broadcast: %v\n", err)
	}
}

// ListenBroadcast runs a UDP listener in the background to detect active peer broadcasts
func ListenBroadcast(port int, myHostname string) {
	//enable SO_REUSEADDR socket to allow multiple local instances for testing
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
			return err
		},
	}
	// bind listener to all interfaces (0.0.0.0) on the designated UDP port
	packetConn, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		fmt.Printf("Error starting UDP listener: %v\n", err)
		return
	}
	defer packetConn.Close()

	fmt.Printf("Listener is running on UDP port %d...\n", port)

	buf := make([]byte, 1024)
	for {
		// read incoming UDP packet into RAM buffer
		n, remoteAddr, err := packetConn.ReadFrom(buf)
		if err != nil {
			continue
		}
		// decode payload into peer details
		peerPort, peerHostname, err := decodeMessage(buf[:n])
		if err != nil {
			continue
		}

		//self echo check
		if peerHostname == myHostname {
			continue
		}

		fmt.Printf("\n[DISCOVERED] Found peer '%s' at %s:%d\n",
			peerHostname, remoteAddr.String(), peerPort)
	}
}
