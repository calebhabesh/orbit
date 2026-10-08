package rendezvous

import (
	"github.com/calebhabesh/orbit/internal/network"
	"net"
)

// STUNServer is deployed separately from the HTTPS/WSS service.
type STUNServer = network.STUNServer

func NewSTUNServer(socket net.PacketConn) (*STUNServer, error) { return network.NewSTUNServer(socket) }
