package containerservice

import (
	"fmt"
	"strconv"
	"strings"
)

// minorVersion is one supported Kubernetes minor with its patch releases
// (oldest first).
type minorVersion struct {
	Minor   string
	Patches []string
	Default bool
	LTS     bool
}

// Versions is the emulated AKS version catalogue (az aks get-versions).
var Versions = []minorVersion{
	{Minor: "1.32", Patches: []string{"1.32.7", "1.32.9"}, LTS: true},
	{Minor: "1.33", Patches: []string{"1.33.3", "1.33.5"}},
	{Minor: "1.34", Patches: []string{"1.34.1", "1.34.2"}, Default: true},
	{Minor: "1.35", Patches: []string{"1.35.0"}},
}

// DefaultVersion returns the default minor's latest patch.
func DefaultVersion() string {
	for _, v := range Versions {
		if v.Default {
			return v.Patches[len(v.Patches)-1]
		}
	}
	v := Versions[len(Versions)-1]
	return v.Patches[len(v.Patches)-1]
}

// resolveVersion maps "1.34" or "1.34.1" to a supported patch version.
func resolveVersion(in string) (string, error) {
	in = strings.TrimPrefix(strings.TrimSpace(in), "v")
	if in == "" {
		return DefaultVersion(), nil
	}
	for _, v := range Versions {
		if in == v.Minor {
			return v.Patches[len(v.Patches)-1], nil
		}
		for _, p := range v.Patches {
			if in == p {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("Version %s is not supported in this region. Please use [az aks get-versions] command to get the supported version list in this region. For more information, please check https://aka.ms/supported-version-list", in)
}

// minorOf returns the minor number of "1.34.1" (34).
func minorOf(v string) int {
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
}

// cmpVersion compares two x.y.z versions.
func cmpVersion(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// upgradesFrom lists versions a cluster on v can upgrade to (same minor
// newer patches, and the next minor).
func upgradesFrom(v string) []string {
	var out []string
	m := minorOf(v)
	for _, mv := range Versions {
		mm := minorOf(mv.Minor + ".0")
		if mm != m && mm != m+1 {
			continue
		}
		for _, p := range mv.Patches {
			if cmpVersion(p, v) > 0 {
				out = append(out, p)
			}
		}
	}
	return out
}

// checkUpgrade validates a control-plane version change.
func checkUpgrade(from, to string) error {
	switch c := cmpVersion(to, from); {
	case c == 0:
		return nil
	case c < 0:
		return fmt.Errorf("Upgrading from %s to %s is not allowed: downgrading the Kubernetes version is not supported", from, to)
	case minorOf(to) > minorOf(from)+1:
		return fmt.Errorf("Upgrading from %s to %s is not allowed: you can only upgrade one minor version at a time. Upgrade to 1.%d first", from, to, minorOf(from)+1)
	}
	return nil
}

// imageVersion is the node image AKS reports for an OS SKU.
func imageVersion(osSKU, osType string) string {
	if strings.EqualFold(osType, "Windows") {
		return "AKSWindows-2022-containerd-20348.4294.251015"
	}
	if strings.EqualFold(osSKU, "AzureLinux") {
		return "AKSAzureLinux-V3gen2-202510.20.0"
	}
	return "AKSUbuntu-2204gen2containerd-202510.20.0"
}
