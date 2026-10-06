package rendezvous

import (
	"github.com/calebhabesh/file-sync/internal/network"
	"net"
)

// STUNServer is deployed separately from the HTTPS/WSS service.
type STUNServer = network.STUNServer

func NewSTUNServer(socket net.PacketConn) (*STUNServer, error) { return network.NewSTUNServer(socket) }
