package sim

import (
	"fmt"
	"strings"
)

// TCPConnect checks whether a TCP connection from an endpoint succeeds.
// It returns ok, an error kind ("timeout", "refused", "resolve") and a message.
func (s *State) TCPConnect(from Endpoint, host string, port int) (bool, string, string) {
	ip, ok := s.ResolveHost(host, from)
	if !ok {
		return false, "resolve", fmt.Sprintf("getaddrinfo for host \"%s\" port %d: Name or service not known", host, port)
	}
	for _, pid := range SortedKeys(s.Projects) {
		for _, fr := range s.Projects[pid].ForwardingRules {
			if fr.IP == ip {
				if portInList(fr.Ports, port) {
					return true, "", fmt.Sprintf("Connection to %s %d port [tcp/*] succeeded!", host, port)
				}
				return false, "refused", fmt.Sprintf("connect to %s port %d (tcp) failed: Connection refused", host, port)
			}
		}
	}
	ep, ok := s.FindIP(ip)
	if !ok {
		if from.Kind == "vm" || from.Kind == "pod" {
			if ok, why := s.internetPath(from); !ok {
				return false, "timeout", fmt.Sprintf("connect to %s port %d (tcp) timed out: Operation now in progress (%s)", host, port, why)
			}
		}
		return true, "", fmt.Sprintf("Connection to %s %d port [tcp/*] succeeded!", host, port)
	}
	switch ep.Kind {
	case "sql":
		in := s.Projects[ep.Project].SQLInstances[ep.Name]
		if ip == in.PrivateIP {
			if from.Network == "" || !s.networksConnected(from.Project, from.Network, ep.Project, in.Network) {
				return false, "timeout", fmt.Sprintf("connect to %s port %d (tcp) timed out: Operation now in progress", host, port)
			}
			if sp := s.Projects[from.Project]; sp != nil {
				if ok, rule := sp.evalFirewall("EGRESS", from, ep, true, "tcp", port); !ok {
					return false, "timeout", fmt.Sprintf("connect to %s port %d (tcp) timed out (egress denied by %s)", host, port, rule)
				}
			}
		} else {
			src := ""
			switch from.Kind {
			case "internet":
				src = from.IP
			case "vm":
				if vm := s.Projects[from.Project].Instances[from.Name]; vm != nil && vm.ExternalIP != "" {
					src = vm.ExternalIP
				} else if s.natCovers(from.Project, from.Network, from.Region, from.Subnet) {
					src = "34.140.10.1"
				}
			}
			allowed := false
			for _, an := range in.AuthorizedNets {
				if src != "" && IPInCIDR(src, an) {
					allowed = true
				}
			}
			if !allowed {
				return false, "timeout", fmt.Sprintf("connect to %s port %d (tcp) timed out: Operation now in progress (source not in authorized networks)", host, port)
			}
		}
		if port != 5432 && port != 3306 {
			return false, "refused", fmt.Sprintf("connect to %s port %d (tcp) failed: Connection refused", host, port)
		}
		return true, "", fmt.Sprintf("Connection to %s %d port [tcp/postgresql] succeeded!", host, port)
	case "vm":
		dst := ep
		dst.IP = ip
		vm := s.Projects[ep.Project].Instances[ep.Name]
		if from.Kind == "internet" && ip == vm.InternalIP {
			return false, "timeout", fmt.Sprintf("connect to %s port %d (tcp) timed out: private address not routable from the internet", host, port)
		}
		fr := s.CheckFlow(from, dst, "tcp", port)
		if !fr.Allowed {
			return false, "timeout", fmt.Sprintf("connect to %s port %d (tcp) timed out: Operation now in progress", host, port)
		}
		ls, _ := s.VMListeners(ep.Project, vm)
		for _, l := range ls {
			if l.Port == port && l.Running {
				if (l.Bind == "127.0.0.1" || l.Bind == "localhost") && !(from.Kind == "vm" && from.Name == vm.Name) {
					break
				}
				return true, "", fmt.Sprintf("Connection to %s %d port [tcp/*] succeeded!", host, port)
			}
		}
		if port == 22 {
			return true, "", fmt.Sprintf("Connection to %s 22 port [tcp/ssh] succeeded!", host)
		}
		return false, "refused", fmt.Sprintf("connect to %s port %d (tcp) failed: Connection refused", host, port)
	}
	return false, "refused", "connection refused"
}

// Ping checks ICMP reachability.
func (s *State) Ping(from Endpoint, host string) (bool, string) {
	ip, ok := s.ResolveHost(host, from)
	if !ok {
		return false, fmt.Sprintf("ping: %s: Name or service not known", host)
	}
	ep, ok := s.FindIP(ip)
	if !ok || ep.Kind != "vm" {
		if from.Kind == "vm" {
			if ok, _ := s.internetPath(from); !ok {
				return false, fmt.Sprintf("PING %s (%s) 56(84) bytes of data.\n\n--- %s ping statistics ---\n3 packets transmitted, 0 received, 100%% packet loss", host, ip, host)
			}
		}
		return true, fmt.Sprintf("PING %s (%s) 56(84) bytes of data.\n64 bytes from %s: icmp_seq=1 ttl=115 time=1.21 ms\n\n--- %s ping statistics ---\n3 packets transmitted, 3 received, 0%% packet loss", host, ip, ip, host)
	}
	dst := ep
	dst.IP = ip
	fr := s.CheckFlow(from, dst, "icmp", 0)
	if !fr.Allowed {
		return false, fmt.Sprintf("PING %s (%s) 56(84) bytes of data.\n\n--- %s ping statistics ---\n3 packets transmitted, 0 received, 100%% packet loss, time 2031ms", host, ip, host)
	}
	return true, fmt.Sprintf("PING %s (%s) 56(84) bytes of data.\n64 bytes from %s: icmp_seq=1 ttl=64 time=0.412 ms\n64 bytes from %s: icmp_seq=2 ttl=64 time=0.301 ms\n\n--- %s ping statistics ---\n2 packets transmitted, 2 received, 0%% packet loss", host, ip, ip, ip, host)
}

// PortInList exposes port list matching.
func PortInList(ports []string, port int) bool { return portInList(ports, port) }

// InternetPath exposes the egress check.
func (s *State) InternetPath(src Endpoint) (bool, string) { return s.internetPath(src) }

// NatCovers exposes the NAT coverage check.
func (s *State) NatCovers(project, network, region, subnet string) bool {
	return s.natCovers(project, network, region, subnet)
}

// ApplySQL executes simple statements against a Cloud SQL instance model.
func (s *State) ApplySQL(in *SQLInstance, sql string) string {
	q := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))
	l := strings.ToLower(q)
	switch {
	case strings.HasPrefix(l, "create index") || strings.HasPrefix(l, "create unique index"):
		// CREATE INDEX name ON table (col)
		on := strings.Index(l, " on ")
		if on < 0 {
			return "ERROR:  syntax error at end of input"
		}
		rest := strings.TrimSpace(l[on+4:])
		tbl := strings.TrimSpace(strings.SplitN(rest, "(", 2)[0])
		col := ""
		if i := strings.Index(rest, "("); i >= 0 {
			col = strings.Trim(strings.SplitN(rest[i+1:], ")", 2)[0], " ")
		}
		in.Indexes = append(in.Indexes, tbl+"("+col+")")
		var keep []string
		for _, sq := range in.SlowQueries {
			sl := strings.ToLower(sq)
			if strings.Contains(sl, tbl) && strings.Contains(sl, strings.Split(col, ",")[0]) {
				continue
			}
			keep = append(keep, sq)
		}
		in.SlowQueries = keep
		return "CREATE INDEX"
	case strings.HasPrefix(l, "show max_connections"):
		return fmt.Sprintf(" max_connections \n-----------------\n %d\n(1 row)", DBMaxConnections(in))
	case strings.Contains(l, "pg_stat_activity"):
		return fmt.Sprintf(" count \n-------\n %d\n(1 row)", in.Connections)
	case strings.HasPrefix(l, "alter user") || strings.HasPrefix(l, "alter role"):
		f := strings.Fields(q)
		if len(f) >= 5 {
			user := strings.Trim(f[2], `"`)
			pw := strings.Trim(f[len(f)-1], `'`)
			if _, ok := in.Users[user]; ok {
				in.Users[user] = pw
				return "ALTER ROLE"
			}
			return fmt.Sprintf("ERROR:  role \"%s\" does not exist", user)
		}
	case strings.HasPrefix(l, "create database"):
		f := strings.Fields(q)
		in.Databases = append(in.Databases, strings.Trim(f[len(f)-1], `"`))
		return "CREATE DATABASE"
	case strings.HasPrefix(l, "explain"):
		for _, sq := range in.SlowQueries {
			if strings.Contains(strings.ToLower(sq), strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(q, "EXPLAIN"), "explain"))[1:min(30, len(q)-8)]) {
				return "Seq Scan on orders  (cost=0.00..458932.10 rows=12 width=96)\n  Filter: (customer_id = 42)"
			}
		}
		return "Index Scan using idx on orders  (cost=0.43..8.45 rows=12 width=96)"
	case strings.HasPrefix(l, "select"):
		if len(in.SlowQueries) > 0 && strings.Contains(l, "pg_stat_statements") {
			var b strings.Builder
			b.WriteString(" mean_exec_time | query\n----------------+------\n")
			for _, sq := range in.SlowQueries {
				b.WriteString("       8421.337 | " + sq + "\n")
			}
			return b.String() + fmt.Sprintf("(%d rows)", len(in.SlowQueries))
		}
		return " ?column? \n----------\n        1\n(1 row)"
	}
	return "OK"
}
