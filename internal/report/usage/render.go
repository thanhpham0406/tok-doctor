package usage

import (
	"fmt"
	"io"
	"strconv"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

func Render(w io.Writer, result model.UsageResult) error {
	if len(result.Sources) == 0 {
		_, err := fmt.Fprintln(w, "No usage data available")
		return err
	}

	for i, entry := range result.Sources {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if err := renderEntry(w, entry); err != nil {
			return err
		}
	}
	return nil
}

func renderEntry(w io.Writer, entry model.UsageEntry) error {
	if entry.Usage.Confidence == "" {
		_, err := fmt.Fprintf(w, "%s: no usage data available\n", entry.Source)
		return err
	}
	_, err := fmt.Fprintf(w, "%s\n  input:      %s\n  cached:     %s\n  output:     %s\n  reasoning:  %s\n  total:      %s\n  confidence: %s\n",
		entry.Source,
		FormatTokenCount(entry.Usage.Input),
		FormatTokenCount(entry.Usage.Cached),
		FormatTokenCount(entry.Usage.Output),
		FormatTokenCount(entry.Usage.Reasoning),
		FormatTokenCount(entry.Usage.Total),
		entry.Usage.Confidence)
	return err
}

func FormatTokenCount(n int64) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	var magnitude uint64
	if negative {
		magnitude = uint64(-(n + 1)) + 1
	} else {
		magnitude = uint64(n)
	}

	raw := strconv.FormatUint(magnitude, 10)
	out := make([]byte, 0, len(raw)+(len(raw)-1)/3)
	for i, ch := range raw {
		if i > 0 && (len(raw)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(ch))
	}
	if negative {
		return "-" + string(out)
	}
	return string(out)
}

func Single(source string, usage model.Usage) model.UsageResult {
	return model.UsageResult{Sources: []model.UsageEntry{{Source: source, Usage: usage}}}
}
