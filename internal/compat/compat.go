// Package compat publica la versión del protocolo de sync que habla el
// servidor y rechaza a los clientes demasiado viejos para él (ver
// specs/protocolo-version).
//
// El protocolo es un entero que sube solo cuando cambia algo que un
// cliente viejo no entendería (por ejemplo, el cifrado de extremo a
// extremo). Es independiente de la versión del servidor y de la de cada
// app: Desktop, Web y Mobile llevan números distintos.
package compat

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// ServerVersion es la versión del binario, solo informativa. Se fija al
// compilar: -ldflags "-X github.com/jorgebuitragor/logday-server/internal/compat.ServerVersion=v1.3.0".
var ServerVersion = "dev"

const (
	// Protocol es el protocolo de sync que habla este servidor.
	Protocol = 1
	// MinClientProtocol es el protocolo más viejo que este servidor
	// acepta de un cliente.
	MinClientProtocol = 1
	// Header es la cabecera en la que cada cliente manda su protocolo.
	Header = "X-Logday-Protocol"
	// MinHeader acompaña al 426 con el mínimo que se exige.
	MinHeader = "X-Logday-Min-Protocol"
	// legacyClientProtocol es el protocolo que se asume cuando un
	// cliente no manda Header: las apps anteriores a este contrato
	// hablan el protocolo 1.
	legacyClientProtocol = 1
)

type versionResponse struct {
	ServerVersion     string `json:"server_version"`
	Protocol          int    `json:"protocol"`
	MinClientProtocol int    `json:"min_client_protocol"`
}

// VersionHandler responde GET /version, sin autenticación.
func VersionHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(versionResponse{
		ServerVersion:     ServerVersion,
		Protocol:          Protocol,
		MinClientProtocol: MinClientProtocol,
	}); err != nil {
		log.Printf("writing version response: %v", err)
	}
}

// RequireClientProtocol rechaza con 426 a los clientes cuyo protocolo
// es menor que minProtocol. Un cliente sin Header cuenta como protocolo
// 1; un valor que no sea un entero positivo es un 400. Las rutas de
// exempt pasan siempre: un cliente viejo tiene que poder cerrar sesión
// para que su refresh token no quede vivo en el servidor.
func RequireClientProtocol(minProtocol int, exempt ...string) func(http.Handler) http.Handler {
	skip := make(map[string]bool, len(exempt))
	for _, p := range exempt {
		skip[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			client := legacyClientProtocol
			if raw := r.Header.Get(Header); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 1 {
					http.Error(w, "invalid "+Header+" header", http.StatusBadRequest)
					return
				}
				client = n
			}
			if client < minProtocol {
				w.Header().Set(MinHeader, strconv.Itoa(minProtocol))
				http.Error(w, "this app is too old for this server, update it", http.StatusUpgradeRequired)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
