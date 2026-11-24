//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package discovery

import (
	"errors"
	"net"
)

func configureMulticast(connection *net.UDPConn, protocol DiscoveryProtocol, index int, interfaceIP net.IP) error {
	return errors.New("SSDP multicast socket configuration is unsupported on this platform")
}
