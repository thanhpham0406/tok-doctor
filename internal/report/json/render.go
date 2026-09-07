package json

import (
	"encoding/json"
	"io"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
)

func Render(w io.Writer, result analyze.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
