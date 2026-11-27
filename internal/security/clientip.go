package security

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIPConfig dice cómo sacar la IP real del cliente cuando el server
// está detrás de un reverse proxy (specs/limites-auth). Sin proxies de
// confianza la IP es la de la conexión, como antes: un self-hosted sin
// proxy no necesita configurar nada, y uno con proxy que no lo configure
// sigue viendo la IP del proxy, que es el comportamiento anterior.
type ClientIPConfig struct {
	// TrustedProxies son las redes cuyas cabeceras se creen. Solo una
	// conexión que viene de una de ellas puede decir cuál es la IP del
	// cliente; cualquier otra cabecera se ignora.
	TrustedProxies []netip.Prefix
	// Header es la cabecera que trae la IP. X-Forwarded-For (por
	// defecto) se recorre de derecha a izquierda saltando los proxies de
	// confianza. Cualquier otra (CF-Connecting-IP, X-Real-IP) se lee
	// como un solo valor.
	Header string
}

const forwardedForHeader = "X-Forwarded-For"

// ParseClientIPConfig arma la configuración desde TRUSTED_PROXIES (IPs o
// CIDR separados por coma) y CLIENT_IP_HEADER (vacío = X-Forwarded-For).
func ParseClientIPConfig(trustedProxies, header string) (ClientIPConfig, error) {
	cfg := ClientIPConfig{Header: http.CanonicalHeaderKey(strings.TrimSpace(header))}
	if cfg.Header == "" {
		cfg.Header = forwardedForHeader
	}
	for _, raw := range strings.Split(trustedProxies, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			p, err := netip.ParsePrefix(raw)
			if err != nil {
				return ClientIPConfig{}, fmt.Errorf("TRUSTED_PROXIES: %q is not an IP or CIDR", raw)
			}
			cfg.TrustedProxies = append(cfg.TrustedProxies, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(raw)
		if err != nil {
			return ClientIPConfig{}, fmt.Errorf("TRUSTED_PROXIES: %q is not an IP or CIDR", raw)
		}
		a = a.Unmap()
		cfg.TrustedProxies = append(cfg.TrustedProxies, netip.PrefixFrom(a, a.BitLen()))
	}
	return cfg, nil
}

func (c ClientIPConfig) trusted(a netip.Addr) bool {
	for _, p := range c.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ClientIP devuelve la IP del cliente de r según la configuración.
func (c ClientIPConfig) ClientIP(r *http.Request) netip.Addr {
	remote, ok := parseHostAddr(r.RemoteAddr)
	if !ok || len(c.TrustedProxies) == 0 || !c.trusted(remote) {
		return remote
	}

	if c.Header != forwardedForHeader {
		if a, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get(c.Header))); err == nil {
			return a.Unmap()
		}
		return remote
	}

	// Cada proxy agrega a la derecha la IP de quien le habló. Lo que está
	// a la izquierda del primer salto que no es de confianza lo pudo
	// escribir el cliente, así que no se cree.
	var hops []string
	for _, v := range r.Header.Values(forwardedForHeader) {
		hops = append(hops, strings.Split(v, ",")...)
	}
	client := remote
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		client = a.Unmap()
		if !c.trusted(client) {
			break
		}
	}
	return client
}

// ClientIPMiddleware reemplaza r.RemoteAddr por la IP real del cliente,
// para que el log de acceso, el rate limit del login y la IP que se
// guarda de cada dispositivo usen la misma.
func ClientIPMiddleware(c ClientIPConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(c.TrustedProxies) > 0 {
				if ip := c.ClientIP(r); ip.IsValid() {
					_, port, err := net.SplitHostPort(r.RemoteAddr)
					if err != nil {
						port = "0"
					}
					r.RemoteAddr = net.JoinHostPort(ip.String(), port)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseHostAddr(hostport string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}
