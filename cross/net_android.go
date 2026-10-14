package main

// Android (Termux) では /etc/resolv.conf がなく、Go の名前解決 (DNS) が失敗するので、
// Termux の resolv.conf の DNS サーバーか、公開 DNS (Cloudflare / Google) を使う

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	servers := androidDNSServers()
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			var last error
			for _, s := range servers {
				c, err := d.DialContext(ctx, network, s)
				if err == nil {
					return c, nil
				}
				last = err
			}
			return nil, last
		},
	}
}

func androidDNSServers() []string {
	var out []string
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		prefix = "/data/data/com.termux/files/usr"
	}
	if f, err := os.Open(filepath.Join(prefix, "etc", "resolv.conf")); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if fields := strings.Fields(sc.Text()); len(fields) >= 2 && fields[0] == "nameserver" {
				out = append(out, net.JoinHostPort(fields[1], "53"))
			}
		}
		f.Close()
	}
	return append(out, "1.1.1.1:53", "8.8.8.8:53")
}
