package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Origins are explicit HTTP(S) origins, never URL paths or wildcard patterns.
// An empty setting disables cross-origin sharing without changing API routes.
func (c *Config) loadCORS(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		u, err := url.Parse(origin)
		if err != nil || origin == "" || strings.ContainsAny(origin, "*?#\r\n\t ") ||
			(u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
			(u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must be a comma-separated list of exact HTTP(S) origins without credentials, paths, queries, fragments or wildcards")
		}
		if strings.HasSuffix(u.Host, ":") || (strings.HasPrefix(u.Host, "[") && net.ParseIP(u.Hostname()) == nil) {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS contains an invalid host or port")
		}
		if port := u.Port(); port != "" {
			n, e := strconv.Atoi(port)
			if e != nil || n < 1 || n > 65535 {
				return fmt.Errorf("CORS_ALLOWED_ORIGINS contains an invalid port")
			}
		}
		host := strings.ToLower(u.Hostname())
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		port := u.Port()
		if port != "" && !(u.Scheme == "https" && port == "443") && !(u.Scheme == "http" && port == "80") {
			host = net.JoinHostPort(u.Hostname(), port)
		}
		normalized := u.Scheme + "://" + strings.ToLower(host)
		if !seen[normalized] {
			seen[normalized] = true
			c.CORSAllowedOrigins = append(c.CORSAllowedOrigins, normalized)
		}
	}
	return nil
}
