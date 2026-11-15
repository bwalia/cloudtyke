package network

import (
	"crypto/sha256"
	"fmt"
	"net/netip"
	"strings"
)

// parsePrefixes parses CIDRs and rejects host bits, like Azure does.
func parsePrefixes(list []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		p, err := netip.ParsePrefix(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("'%s' is not a valid CIDR", s)
		}
		if p.Masked() != p {
			return nil, fmt.Errorf("'%s' is not a valid CIDR: host bits are set, use %s", s, p.Masked())
		}
		out = append(out, p)
	}
	return out, nil
}

func overlaps(a, b netip.Prefix) bool { return a.Overlaps(b) }

// within reports whether inner is fully contained in outer.
func within(inner, outer netip.Prefix) bool {
	return outer.Bits() <= inner.Bits() && outer.Contains(inner.Addr())
}

func withinAny(inner netip.Prefix, outers []netip.Prefix) bool {
	for _, o := range outers {
		if within(inner, o) {
			return true
		}
	}
	return false
}

func anyOverlap(a, b []netip.Prefix) (netip.Prefix, netip.Prefix, bool) {
	for _, x := range a {
		for _, y := range b {
			if overlaps(x, y) {
				return x, y, true
			}
		}
	}
	return netip.Prefix{}, netip.Prefix{}, false
}

// fakePublicIP returns a stable, Azure-looking public address for a resource id.
func fakePublicIP(id string) string {
	h := sha256.Sum256([]byte(strings.ToLower(id)))
	firsts := []int{20, 40, 51, 52, 98, 104, 137, 172}
	first := firsts[int(h[0])%len(firsts)]
	if first == 172 { // stay out of 172.16.0.0/12
		return fmt.Sprintf("172.%d.%d.%d", 32+int(h[1])%64, h[2], 1+int(h[3])%254)
	}
	return fmt.Sprintf("%d.%d.%d.%d", first, h[1], h[2], 1+int(h[3])%254)
}
