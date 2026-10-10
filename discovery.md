## LAN Peer Discovery - `discovery.go`

`discovery.go` handles network auto-discovery over UDP broadcast, allowing nodes within the same local network subnet to dynamically locate each other without hardcoded IP configurations.

---
### Workflow

**1. Outbound Beaconing**

Every 5 seconds, `Broadcast()` calculates the network broadcast address via `getBroadcastAddr`, encodes state metadata using `encodeMessage()`, and sends a UDP packet.

**2. Inbound Monitoring**

`ListenBroadcast()` continuously listens on the designated UDP socket. Upon packet arrival, it calls `decodeMessage()` to extract the remote `Hostname` and `TCP Port`, then updates `PeerRegistry`.

### Function Reference

`encodeMessage(udpPort, tcpPort int, hostname string) []byte`: Formats parameter values into a structured payload string `UDP_Port|TCP_Port|Hostname` and casts it to `[]byte`.

`decodeMessage(data []byte) (int, int, string, error)`: Splits string data using the | delimiter, converts string representations of port numbers back to `int`, and returns structured peer metadata

`getBroadcastAddr(port int) (*net.UDPAddr, error)`: Inspects active network interfaces, evaluates IP addresses and subnet masks, and derives the directed broadcast address (eg `192.168.1.255:8888`), ensuring packets reach all local subnetwork hosts.

`Broadcast(udpPort, tcpPort int, hostname string)`: Opens a UDP socket configured with `SO_BROADCAST`, constructs the payload via `encodeMessage()`, and sends it to the broadcast destination address.

`ListenBroadcast(udpPort int, registry *PeerRegistry)`: Binds to the designated UDP port. For every received packet, it invokes `decodeMessage()`, extracts the sender's IP address, and updates the local repository via `registry.AddOrUpdate()`