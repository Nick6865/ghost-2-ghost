## Entry Point & CLI Driver — `main.go`

`main.go` is the **application entry point**. It initializes shared managers,
spawns asynchronous background services (goroutines), and manages the
continuous CLI input loop.

---

### Workflow

**1. Initialization**

Instantiates the core system controllers via `NewNodeManager()` and
`NewPeerRegistry()`.

**2. Port Allocation**

Calls `getFreeTCPPort()` to find an available local TCP port, scanning
upward from **8889**.
This avoids conflicts when multiple instances run
on the same machine.

**3. Background Services**

Four goroutines are spawned to keep the node alive without blocking
the CLI:

- `Broadcast()` — periodically announces local node availability across the LAN.
- `go ListenBroadcast(...)` — monitors inbound UDP broadcast discovery packets.
- `go nodeMgr.StartTCPServer(...)` — listens for inbound chat connections.
- `go StartCleanupRoutine(...)` — removes inactive peers periodically.

**4. CLI Event Loop**
Reads standard input line-by-line using `bufio.Scanner` and broadcasts the raw string as a room chat message via `Room.Broadcast`. Input is routed by prefix:

- Commands starting with `/` are treated as directives (`/peers`, `/connect`, `/quit`, `/exit`).
- Bare `y` / `n` confirm or deny a pending action.

---

### Function Reference

```go
func getFreeTCPPort(startPort int) (int, error)   