package bulk

import (
	"net"
	"strings"
)

// NewHostChecker builds the server mode Host guard, a defence against DNS
// rebinding. It accepts localhost, IP literals and single-label names (a
// Kubernetes Service name, for example). Anything else, such as an ingress
// FQDN, must be listed in extra; an entry of the form "*.example.com" matches
// any subdomain. A rejected host only disables the bulk routes: the client
// falls back to /api/call.
func NewHostChecker(extra []string) func(string) bool {
	exact := map[string]bool{}
	var suffixes []string
	for _, e := range extra {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if strings.HasPrefix(e, "*.") {
			suffixes = append(suffixes, e[1:])
		} else {
			exact[e] = true
		}
	}
	return func(hostport string) bool {
		host := hostport
		if h, _, err := net.SplitHostPort(hostport); err == nil {
			host = h
		}
		host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
		if host == "" {
			return false
		}
		if host == "localhost" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
			return true
		}
		if exact[host] {
			return true
		}
		for _, s := range suffixes {
			if strings.HasSuffix(host, s) {
				return true
			}
		}
		return false
	}
}
