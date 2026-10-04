package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// NodeState defines the discrete operational states of the node
type NodeState int

const (
	StateIDLE    NodeState = iota // Idle state, awaiting commands or inbound invites
	StateWAITING                  // Waiting for peer response after sending SYN_INVITE
	StateBUSY                     // Active 1-on-1 TCP chat session
)

// NodeManager handles state transitions, connection management, and invitation channels
type NodeManager struct {
	mu                sync.Mutex
	State             NodeState
	MyHostname        string
	ActiveConn        net.Conn
	PendingInviteChan chan bool
	PendingHostname   string
}

// NewNodeManager creates a initialized NodeManager instance
func NewNodeManager(hostname string) *NodeManager {
	return &NodeManager{
		State:      StateIDLE,
		MyHostname: hostname,
	}
}

// StartTCPServer listens for incoming TCP handshake connections
func (m *NodeManager) StartTCPServer(tcpPort int) {
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", tcpPort))
	if err != nil {
		fmt.Printf("Error starting TCP Listener on port %d: %v\n", tcpPort, err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if err == net.ErrClosed {
				return
			}
			continue
		}
		go m.handleIncomingTCP(conn)
	}
}

// handleIncomingTCP processes inbound SYN_INVITE messages
func (m *NodeManager) handleIncomingTCP(conn net.Conn) {
	reader := bufio.NewReader(conn)
	message, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return
	}

	message = strings.TrimSpace(message)
	parts := strings.Split(message, "|")
	cmd := parts[0]

	switch cmd {
	case "SYN_INVITE":
		senderHostname := parts[1]

		m.mu.Lock()
		// Automatically reject if the current node is busy or already processing an invite
		if m.State != StateIDLE || m.PendingInviteChan != nil {
			m.mu.Unlock()
			fmt.Fprintf(conn, "SYN_REJECT|BUSY\n")
			conn.Close()
			return
		}

		replyChan := make(chan bool, 1)
		m.PendingInviteChan = replyChan
		m.PendingHostname = senderHostname
		m.mu.Unlock()

		fmt.Printf("\n[CHAT REQUEST] Ghost '%s' wants to start a private chat with you!\nType 'y' (or /accept) or 'n' (or /decline)\n> ", senderHostname)

		// Wait asynchronously for user decision or timeout after 30 seconds
		select {
		case accepted := <-replyChan:
			if accepted {
				m.mu.Lock()
				m.State = StateBUSY
				m.ActiveConn = conn
				m.PendingInviteChan = nil
				m.mu.Unlock()

				fmt.Fprintf(conn, "SYN_ACCEPT|%s\n", m.MyHostname)
				fmt.Printf("\n[SECURE ROOM] Connected with '%s'! (Type '/quit' to leave)\n> ", senderHostname)
				m.handleChatSession(conn, senderHostname)
			} else {
				fmt.Fprintf(conn, "SYN_REJECT|USER_DECLINED\n")
				conn.Close()
				m.clearPending()
			}
		case <-time.After(30 * time.Second):
			fmt.Fprintf(conn, "SYN_REJECT|TIMEOUT\n")
			conn.Close()
			fmt.Println("\n[TIMEOUT] Invitation expired.")
			m.clearPending()
		}

	default:
		conn.Close()
	}
}

// AcceptPendingInvite pushes user decision into the pending invite channel
func (m *NodeManager) AcceptPendingInvite(accept bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.PendingInviteChan != nil {
		m.PendingInviteChan <- accept
	} else {
		fmt.Println("No pending chat request.")
	}
}

// clearPending resets pending invitation variables
func (m *NodeManager) clearPending() {
	m.mu.Lock()
	m.PendingInviteChan = nil
	m.PendingHostname = ""
	m.mu.Unlock()
}

// ConnectToPeer initiates an outbound TCP connection and performs the handshake
func (m *NodeManager) ConnectToPeer(peerIP string, tcpPort int) {
	m.mu.Lock()
	if m.State != StateIDLE {
		fmt.Println("You are currently busy or waiting for a response.")
		m.mu.Unlock()
		return
	}
	m.State = StateWAITING
	m.mu.Unlock()

	fmt.Printf("Sending SYN invite to %s:%d...\n", peerIP, tcpPort)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", peerIP, tcpPort), 5*time.Second)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		m.resetState()
		return
	}

	fmt.Fprintf(conn, "SYN_INVITE|%s\n", m.MyHostname)

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("No response from peer.")
		conn.Close()
		m.resetState()
		return
	}

	response = strings.TrimSpace(response)
	parts := strings.Split(response, "|")

	if parts[0] == "SYN_ACCEPT" {
		peerHostname := parts[1]
		m.mu.Lock()
		m.State = StateBUSY
		m.ActiveConn = conn
		m.mu.Unlock()

		fmt.Printf("\n[SECURE ROOM] Accepted by '%s'! (Type '/quit' to leave)\n> ", peerHostname)
		m.handleChatSession(conn, peerHostname)
	} else {
		reason := "DECLINED"
		if len(parts) > 1 {
			reason = parts[1]
		}
		fmt.Printf("\n[REJECTED] Peer declined request (%s).\n", reason)
		conn.Close()
		m.resetState()
	}
}

// resetState restores node state to StateIDLE
func (m *NodeManager) resetState() {
	m.mu.Lock()
	m.State = StateIDLE
	m.ActiveConn = nil
	m.PendingInviteChan = nil
	m.mu.Unlock()
}

// handleChatSession blocks until the peer closes the TCP socket
func (m *NodeManager) handleChatSession(conn net.Conn, peerHostname string) {
	done := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			msg := scanner.Text()
			fmt.Printf("\r[%s]: %s\n> ", peerHostname, msg)
		}
		fmt.Printf("\n[DISCONNECTED] '%s' left the room.\n", peerHostname)
		close(done)
	}()

	<-done
	conn.Close()
	m.resetState()
}
