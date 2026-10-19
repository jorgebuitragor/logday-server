// Package httperr centraliza la respuesta 500 de los handlers, para que
// el error real quede en el log del servidor y no solo como un status
// code del lado cliente (ver specs/convenciones-codigo, "Logging y
// manejo de errores").
package httperr

import (
	"log"
	"net/http"
	"strconv"
)

// Internal loguea err con el método y la ruta del pedido y responde
// 500 "internal error". El cliente nunca ve err: puede traer detalle de
// queries SQL o de la implementación. La ruta va sin query string, que
// podría llevar datos del pedido.
//
// Método y ruta van citados con strconv.Quote porque los manda el
// cliente: la ruta ya decodificada puede traer un salto de línea
// (%0A) y, sin escaparlo, falsificaría una línea entera del log
// (gosec G706, inyección en el log).
func Internal(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("%s %s: %v", strconv.Quote(r.Method), strconv.Quote(r.URL.Path), err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
