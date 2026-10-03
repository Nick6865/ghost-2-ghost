package main

import (
	"fmt"
	"sync"
	"time"
)

// Peer represents an active peer node in memory
type Peer struct {
	Hostname string
	IP       string
	TCPPort  int
	LastSeen time.Time
}

// PeerRegistry provides a thread-safe in-memory table of active LAN peers
type PeerRegistry struct {
	mu    sync.RWMutex
	peers map[string]Peer
}

// NewPeerRegistry instantiates a new PeerRegistry
func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{
		peers: make(map[string]Peer),
	}
}

// AddOrUpdate inserts or updates peer metadata and returns true if it is a new discovery
func (r *PeerRegistry) AddOrUpdate(hostname string, ip string, tcpPort int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s:%d", ip, tcpPort)
	_, exists := r.peers[key]

	r.peers[key] = Peer{
		Hostname: hostname,
		IP:       ip,
		TCPPort:  tcpPort,
		LastSeen: time.Now(),
	}

	return !exists
}

// GetPeers retrieves a copy slice of all currently active peers
func (r *PeerRegistry) GetPeers() []Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Peer, 0, len(r.peers))
	for _, p := range r.peers {
		list = append(list, p)
	}
	return list
}

// FindPeer searches for a peer entry matching either a hostname or an IP address
func (r *PeerRegistry) FindPeer(target string) (Peer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.peers {
		if p.Hostname == target || p.IP == target {
			return p, true
		}
	}
	return Peer{}, false
}

// StartCleanupRoutine periodically removes stale peers that stopped sending broadcasts
func (r *PeerRegistry) StartCleanupRoutine(timeout time.Duration, checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	go func() {
		for range ticker.C {
			r.mu.Lock()
			now := time.Now()
			for key, p := range r.peers {
				if now.Sub(p.LastSeen) > timeout {
					fmt.Printf("\n[OFFLINE] Peer '%s' (%s) left the network.\n(Ghost) > ", p.Hostname, p.IP)
					delete(r.peers, key)
				}
			}
			r.mu.Unlock()
		}
	}()
}
