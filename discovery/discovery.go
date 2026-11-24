package discovery

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type DiscoveryProtocol string

const (
	DiscoveryIPv4         DiscoveryProtocol = "ipv4"
	DiscoveryIPv6         DiscoveryProtocol = "ipv6"
	_ssdpServiceTarget                      = "urn:dmtf-org:service:redfish-rest:1"
	_ssdpPort                               = 1900
	_ssdpMaxResponseBytes                   = 8192
	_ssdpMulticastHops                      = 2
)

type SSDPOptions struct {
	Timeout    time.Duration
	Protocol   DiscoveryProtocol
	Interface  string
	MaxResults int
}

type DiscoveredService struct{ UUID, Location, SourceAddress string }

func (service DiscoveredService) Host() string {
	u, err := url.Parse(service.Location)
	if err != nil {
		return ""
	}
	return u.Host
}

func (service DiscoveredService) Origin() string {
	u, err := url.Parse(service.Location)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func (service DiscoveredService) ServiceRoot() string {
	u, err := url.Parse(service.Location)
	if err != nil {
		return ""
	}
	return strings.TrimRight(u.Path, "/") + "/"
}

var _discoveredUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func redfishServiceTarget(value string) bool {
	value = strings.ToLower(value)
	if value == _ssdpServiceTarget {
		return true
	}
	suffix, ok := strings.CutPrefix(value, _ssdpServiceTarget+":")
	if !ok || suffix == "" {
		return false
	}
	for _, char := range suffix {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func ParseSSDPResponse(data []byte, source string) (*DiscoveredService, error) {
	if len(data) >= _ssdpMaxResponseBytes {
		return nil, errors.New("SSDP packet exceeds size limit")
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || (response.Proto != "HTTP/1.0" && response.Proto != "HTTP/1.1") {
		return nil, errors.New("unexpected SSDP status")
	}
	for _, key := range []string{"ST", "USN", "AL"} {
		if len(response.Header.Values(key)) != 1 {
			return nil, errors.New("SSDP requires unique ST, USN and AL headers")
		}
	}
	if !redfishServiceTarget(strings.TrimSpace(response.Header.Get("ST"))) {
		return nil, errors.New("SSDP target is not Redfish")
	}
	identity, target, ok := strings.Cut(strings.TrimSpace(response.Header.Get("USN")), "::")
	if !ok || !redfishServiceTarget(target) || !strings.HasPrefix(strings.ToLower(identity), "uuid:") {
		return nil, errors.New("invalid Redfish USN")
	}
	uuid := strings.TrimPrefix(strings.ToLower(identity), "uuid:")
	if !_discoveredUUID.MatchString(uuid) {
		return nil, errors.New("invalid SSDP UUID")
	}
	location := strings.Trim(strings.TrimSpace(response.Header.Get("AL")), "<>")
	u, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(location, "\r\n\t") {
		return nil, errors.New("invalid Redfish service location")
	}
	return &DiscoveredService{UUID: uuid, Location: location, SourceAddress: source}, nil
}

// DiscoverSSDP returns untrusted endpoint metadata; it never fetches or authenticates endpoints.
func DiscoverSSDP(ctx context.Context, options SSDPOptions) ([]DiscoveredService, error) {
	if options.Timeout < 0 || options.MaxResults < 0 {
		return nil, errors.New("invalid discovery limits")
	}
	if options.Timeout == 0 {
		options.Timeout = 3 * time.Second
	}
	if options.MaxResults == 0 {
		options.MaxResults = 256
	}
	if options.Protocol == "" {
		options.Protocol = DiscoveryIPv4
	}
	network, group := "udp4", "239.255.255.250"
	var local *net.UDPAddr
	zone := ""
	index := 0
	switch options.Protocol {
	case DiscoveryIPv4:
		if options.Interface != "" {
			ip := net.ParseIP(options.Interface)
			if ip == nil || ip.To4() == nil {
				return nil, errors.New("IPv4 interface must be a local IPv4 address")
			}
			local = &net.UDPAddr{IP: ip}
		}
	case DiscoveryIPv6:
		network = "udp6"
		group = "ff02::c"
		zone = options.Interface
		if zone == "" {
			return nil, errors.New("IPv6 multicast requires an interface")
		}
		var iface *net.Interface
		var err error
		if n, parseErr := strconv.Atoi(zone); parseErr == nil {
			iface, err = net.InterfaceByIndex(n)
		} else {
			iface, err = net.InterfaceByName(zone)
		}
		if err != nil {
			return nil, err
		}
		index = iface.Index
		zone = iface.Name
	default:
		return nil, errors.New("unsupported discovery IP protocol")
	}
	connection, err := net.ListenUDP(network, local)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	if err = configureMulticast(connection, options.Protocol, index, net.ParseIP(options.Interface)); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err = connection.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	authority := net.JoinHostPort(group, strconv.Itoa(_ssdpPort))
	mx := min(5, max(1, int((options.Timeout+time.Second-1)/time.Second)))
	request := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\nHOST: %s\r\nMAN: \"ssdp:discover\"\r\nMX: %d\r\nST: %s\r\n\r\n", authority, mx, _ssdpServiceTarget)
	if _, err = connection.WriteToUDP([]byte(request), &net.UDPAddr{IP: net.ParseIP(group), Port: _ssdpPort, Zone: zone}); err != nil {
		return nil, err
	}
	buffer := make([]byte, _ssdpMaxResponseBytes)
	results := []DiscoveredService{}
	seen := map[string]bool{}
	for len(results) < options.MaxResults {
		count, source, err := connection.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return results, nil
				}
				return results, ctx.Err()
			}
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				return results, nil
			}
			return results, err
		}
		service, err := ParseSSDPResponse(buffer[:count], source.String())
		if err != nil {
			continue
		}
		key := service.UUID + "|" + service.Location
		if !seen[key] {
			seen[key] = true
			results = append(results, *service)
		}
	}
	return results, nil
}
