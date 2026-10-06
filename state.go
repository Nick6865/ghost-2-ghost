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

// multi chat
type Room struct {
	mu    sync.RWMutex
	Peers map[string]net.Conn // Key: Hostname or IP:Port -> Value: TCP Connection
}

// NodeManager handles state transitions, connection management, and invitation channels
type NodeManager struct {
	mu                sync.Mutex
	State             NodeState
	MyHostname        string
	CurrentRoom       *Room
	PendingInviteChan chan bool
	PendingHostname   string
}

func NewRoom() *Room {
	return &Room{
		Peers: make(map[string]net.Conn),
	}
}

// add a new connection to room
func (r *Room) AddPeer(hostname string, conn net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Peers[hostname] = conn
}

// remove and close a peer's connection
func (r *Room) RemovePeer(hostname string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if conn, ok := r.Peers[hostname]; ok {
		conn.Close()
		delete(r.Peers, hostname)
	}
}

// send message to all peers in room
func (r *Room) Broadcast(senderHostname, message string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	formattedMsg := fmt.Sprintf("%s\n", message)
	for _, conn := range r.Peers {
		_, _ = conn.Write([]byte(formattedMsg))
	}
}

func (r *Room) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, conn := range r.Peers {
		conn.Close()
	}
	r.Peers = make(map[string]net.Conn)
}

// safe read lock
func (r *Room) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Peers) == 0
}

// NewNodeManager creates a initialized NodeManager instance
func NewNodeManager(hostname string) *NodeManager {
	return &NodeManager{
		State:       StateIDLE,
		MyHostname:  hostname,
		CurrentRoom: NewRoom(),
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
		// Only reject if you are waiting for a response to another invitation
		if m.State == StateWAITING || m.PendingInviteChan != nil {
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
				m.CurrentRoom.AddPeer(senderHostname, conn)
				m.PendingInviteChan = nil
				m.mu.Unlock()

				fmt.Fprintf(conn, "SYN_ACCEPT|%s\n", m.MyHostname)
				fmt.Printf("\n[SECURE ROOM] Connected with '%s'! (Type '/quit' to leave)\n> ", senderHostname)

				go m.listenToPeer(conn, senderHostname)
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
	if m.State == StateWAITING {
		fmt.Println("You are currently busy or waiting for a response.")
		m.mu.Unlock()
		return
	}
	prevState := m.State
	m.State = StateWAITING
	m.mu.Unlock()

	fmt.Printf("Sending SYN invite to %s:%d...\n", peerIP, tcpPort)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", peerIP, tcpPort), 5*time.Second)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		m.resetState(prevState)
		return
	}

	fmt.Fprintf(conn, "SYN_INVITE|%s\n", m.MyHostname)

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("No response from peer.")
		conn.Close()
		m.resetState(prevState)
		return
	}

	response = strings.TrimSpace(response)
	parts := strings.Split(response, "|")

	if parts[0] == "SYN_ACCEPT" {
		peerHostname := parts[1]
		m.mu.Lock()
		m.State = StateBUSY
		m.CurrentRoom.AddPeer(peerHostname, conn)
		m.mu.Unlock()

		fmt.Printf("\n[SECURE ROOM] Accepted by '%s'! (Type '/quit' to leave)\n> ", peerHostname)

		go m.listenToPeer(conn, peerHostname)
	} else {
		reason := "DECLINED"
		if len(parts) > 1 {
			reason = parts[1]
		}
		fmt.Printf("\n[REJECTED] Peer declined request (%s).\n", reason)
		conn.Close()
		m.resetState(prevState)
	}
}

// resetState restores node state to StateIDLE
func (m *NodeManager) resetState(oldState NodeState) {
	m.mu.Lock()
	m.State = StateIDLE
	m.PendingInviteChan = nil
	m.mu.Unlock()
}

// goroutine to read messages from each TCP connection.
func (m *NodeManager) listenToPeer(conn net.Conn, peerHostname string) {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		fmt.Printf("\r[%s]: %s\n> ", peerHostname, msg)
	}

	fmt.Printf("\n[DISCONNECTED] '%s' left the room.\n> ", peerHostname)
	m.CurrentRoom.RemovePeer(peerHostname)

	if m.CurrentRoom.IsEmpty() {
		m.mu.Lock()
		m.State = StateIDLE
		m.mu.Unlock()
		fmt.Println("[ROOM CLOSED] All peers left. Returned to IDLE state.")
	}
}

// close all connection and reset to IDLE
func (m *NodeManager) LeaveRoom() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CurrentRoom.CloseAll()
	m.State = StateIDLE

	m.CurrentRoom.CloseAll()
}
