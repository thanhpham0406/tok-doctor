package terminal

import (
	"fmt"
	"io"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
)

func Render(w io.Writer, result analyze.Result) error {
	if _, err := fmt.Fprintln(w, "TokDoctor"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source: %s\n", result.Source); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Session: %s\n", result.Session.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Status: %s\n", result.Summary.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s\n", result.Summary.Message); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Findings: %d\n", len(result.Findings)); err != nil {
		return err
	}

	return nil
}
