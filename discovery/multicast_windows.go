//go:build windows

package discovery

import (
	"net"
	"syscall"
)

func configureMulticast(connection *net.UDPConn, protocol DiscoveryProtocol, index int, interfaceIP net.IP) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	var optionErr error
	err = raw.Control(func(fd uintptr) {
		if protocol == DiscoveryIPv6 {
			optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_HOPS, _ssdpMulticastHops)
			if optionErr == nil {
				optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_IF, index)
			}
		} else {
			optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, _ssdpMulticastHops)
			if optionErr == nil && interfaceIP != nil {
				var address [4]byte
				copy(address[:], interfaceIP.To4())
				optionErr = syscall.SetsockoptInet4Addr(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, address)
			}
		}
	})
	if err != nil {
		return err
	}
	return optionErr
}
