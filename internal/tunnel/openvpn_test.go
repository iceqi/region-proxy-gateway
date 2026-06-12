package tunnel

import (
	"reflect"
	"testing"
)

func TestOpenVPNCommandIncludesCoreOptions(t *testing.T) {
	got := OpenVPNCommand("/usr/sbin/openvpn", "/tmp/client.ovpn", "tun-test")

	want := []string{
		"/usr/sbin/openvpn",
		"--config", "/tmp/client.ovpn",
		"--dev", "tun-test",
		"--dev-type", "tun",
		"--route-nopull",
		"--pull-filter", "ignore", "route-ipv6",
		"--pull-filter", "ignore", "ifconfig-ipv6",
		"--connect-retry-max", "1",
		"--connect-timeout", "15",
		"--auth-nocache",
		"--verb", "3",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenVPNCommand() = %#v, want %#v", got, want)
	}
}

func TestOpenVPNCommandDefaultsBinary(t *testing.T) {
	got := OpenVPNCommand("", "/tmp/client.ovpn", "tun-test")

	if got[0] != "openvpn" {
		t.Fatalf("OpenVPNCommand()[0] = %q, want openvpn", got[0])
	}
}
