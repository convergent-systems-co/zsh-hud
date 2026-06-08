package module

import "net"

// DialFunc matches net.Dial. Injected so tests can stub the routing decision.
type DialFunc func(network, address string) (net.Conn, error)

// LocalIP returns this host's primary local IP using the UDP routing-decision
// trick: dialing a public address selects the outbound interface without
// sending packets. Returns "" on any error.
func LocalIP(dial DialFunc) string {
	conn, err := dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	ua, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || ua.IP == nil {
		return ""
	}
	return ua.IP.String()
}
