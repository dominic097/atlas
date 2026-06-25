package cli

import (
	"encoding/json"
	"io"
)

// renderJSON writes v as indented JSON. The full CLI adds table/plain/ndjson
// renderers selected by --format; the scaffold ships JSON so output is stable
// and scriptable.
func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
