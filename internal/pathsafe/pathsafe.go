// Package pathsafe valida los campos que Logday Desktop convierte en
// rutas de archivo al aplicar el sync: el id de cada entidad,
// Task.project, Note.folder y las fechas con las que arma la carpeta del
// mes (daily_entries.date, overtime_entries.fecha). Desktop guarda, por
// ejemplo, `projects/<project>/<id>.md` y `dailys/<año>/<mes>/…`, con el
// año y el mes sacados de la fecha con slice fijo. Un valor con "/" o
// "\" que llega por el sync puede escribir fuera de la carpeta de datos
// en todos los Desktop de la cuenta (U-06 de la auditoría de nov 2026).
//
// No valida formato (UUID, YYYY-MM-DD): specs/esquema-datos decide que el
// server no rechace valores legados que siguen en colas offline. Solo
// rechaza lo que puede formar una ruta: separadores, "." y ".." como
// segmento, caracteres de control y valores demasiado largos.
package pathsafe

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Largos máximos en bytes. Un nombre de archivo admite 255 bytes en casi
// todos los sistemas, y Desktop le agrega ".md".
const (
	MaxIDLen      = 128
	MaxSegmentLen = 200
	MaxFolderLen  = 400
	MaxDateLen    = 64
)

// Segment valida un valor que Desktop usa como un solo componente de
// ruta. field es el nombre del campo en la API, para el mensaje de error.
func Segment(field, v string, maxLen int) error {
	if v == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(v) > maxLen {
		return fmt.Errorf("%s must be at most %d bytes", field, maxLen)
	}
	if v == "." || v == ".." {
		return fmt.Errorf("%s cannot be %q", field, v)
	}
	if strings.ContainsAny(v, `/\`) {
		return fmt.Errorf(`%s cannot contain "/" or "\"`, field)
	}
	return checkChars(field, v)
}

// URLParam valida como Segment un parámetro de la URL. chi lo entrega sin
// decodificar (lee r.URL.RawPath), así que "%2F" llega como texto y no
// como "/". Se valida también decodificado, para que un cambio del router
// no abra el agujero.
func URLParam(field, v string, maxLen int) error {
	if err := Segment(field, v, maxLen); err != nil {
		return err
	}
	dec, err := url.PathUnescape(v)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL path segment", field)
	}
	if dec != v {
		return Segment(field, dec, maxLen)
	}
	return nil
}

// Folder valida Note.folder: vacío (la raíz de notas) o segmentos
// separados por "/" (subcarpetas), cada uno válido según Segment.
func Folder(v string) error {
	if v == "" {
		return nil
	}
	if len(v) > MaxFolderLen {
		return fmt.Errorf("folder must be at most %d bytes", MaxFolderLen)
	}
	if strings.Contains(v, `\`) {
		return fmt.Errorf(`folder cannot contain "\"`)
	}
	for _, seg := range strings.Split(v, "/") {
		if seg == "" {
			return fmt.Errorf(`folder cannot start or end with "/" or contain "//"`)
		}
		if err := Segment("folder", seg, MaxSegmentLen); err != nil {
			return err
		}
	}
	return nil
}

func checkChars(field, v string) error {
	if !utf8.ValidString(v) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s cannot contain control characters", field)
		}
	}
	return nil
}
