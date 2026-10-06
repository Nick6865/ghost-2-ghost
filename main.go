package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// getFreeTCPPort scans for an available local TCP port to support multi-instance testing on one machine
func getFreeTCPPort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return startPort
}

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Ghost-Unknown"
	}

	if len(os.Args) > 1 {
		hostname = os.Args[1]
	} else {
		var choice string
		fmt.Printf("Do you want to change your host name? You are '%s' [y/n]: ", hostname)
		fmt.Scanln(&choice)

		if choice == "y" || choice == "Y" {
			fmt.Print("Enter new host name: ")
			fmt.Scanln(&hostname)
		}
	}

	const discoveryPort = 8888
	tcpPort := getFreeTCPPort(8889) // Dynamically assign an open TCP port

	fmt.Printf("\n=== GHOST-2-GHOST CHAT ===\n")
	fmt.Printf("My Hostname : %s\n", hostname)
	fmt.Printf("My TCP Port  : %d\n", tcpPort)
	fmt.Println("Commands    : '/peers', '/connect ', 'y', 'n', '/quit'")
	fmt.Println("-------------------------------------------------------------")

	registry := NewPeerRegistry()
	registry.StartCleanupRoutine(12*time.Second, 3*time.Second)

	nodeMgr := NewNodeManager(hostname)
	go nodeMgr.StartTCPServer(tcpPort)

	go ListenBroadcast(discoveryPort, hostname, registry)

	// Periodically broadcast presence on the LAN
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		Broadcast(discoveryPort, tcpPort, hostname)
		for range ticker.C {
			Broadcast(discoveryPort, tcpPort, hostname)
		}
	}()

	// Centralized keyboard input processing loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if nodeMgr.State == StateIDLE {
			fmt.Print("(Ghost) > ")
		}

		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// When in an active chat session, forward messages to the remote peer
		if nodeMgr.State == StateBUSY {
			if line == "/quit" || line == "/leave" {
				nodeMgr.LeaveRoom()
				fmt.Println("Left the chat room.")
			} else if strings.HasPrefix(line, "/connect ") {
				//roomate can invite another peer to the room
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					target := parts[1]
					peer, found := registry.FindPeer(target)
					if found {
						go nodeMgr.ConnectToPeer(peer.IP, peer.TCPPort)
					} else {
						fmt.Printf("Peer '%s' not found in active list.\n", target)
					}
				}
			} else {
				nodeMgr.CurrentRoom.Broadcast(nodeMgr.MyHostname, line)
				fmt.Print("> ")
			}
			continue
		}

		parts := strings.Fields(line)
		cmd := parts[0]

		switch cmd {
		case "/peers":
			peers := registry.GetPeers()
			fmt.Printf("\n--- Active Ghost Peers (%d) ---\n", len(peers))
			for _, p := range peers {
				fmt.Printf("• Hostname: %-15s | IP: %-15s | TCP Port: %d\n", p.Hostname, p.IP, p.TCPPort)
			}
			fmt.Println("-------------------------------")

		case "/connect":
			if len(parts) < 2 {
				fmt.Println("Usage: /connect ")
				continue
			}
			target := parts[1]
			peer, found := registry.FindPeer(target)
			if !found {
				fmt.Printf("Peer '%s' not found in active list. Use /peers to check.\n", target)
				continue
			}
			// CRITICAL FIX: Run ConnectToPeer as a Goroutine so it does NOT block the main CLI input loop!
			go nodeMgr.ConnectToPeer(peer.IP, peer.TCPPort)

		case "/accept", "y", "Y":
			nodeMgr.AcceptPendingInvite(true)

		case "/decline", "n", "N":
			nodeMgr.AcceptPendingInvite(false)

		case "/quit", "/exit":
			fmt.Println("Exiting Ghost Chat. Goodbye!")
			os.Exit(0)

		default:
			fmt.Println("Unknown command. Commands: '/peers', '/connect ', 'y', 'n', '/quit'")
		}
	}
}
