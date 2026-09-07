package source

import (
	"fmt"
	"io"
	"strings"

	coresource "github.com/thanhpham0406/tok-doctor/internal/source"
)

func RenderList(w io.Writer, result coresource.ListResult) error {
	if _, err := fmt.Fprintln(w, "Sources"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "────────────────────────────────────"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	for i, detection := range result.Sources {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if err := renderSummary(w, detection); err != nil {
			return err
		}
	}
	return nil
}

func RenderShow(w io.Writer, detection coresource.Detection) error {
	if err := renderSummary(w, detection); err != nil {
		return err
	}
	if detection.Reason != "" {
		if _, err := fmt.Fprintf(w, "  Reason      %s\n", detection.Reason); err != nil {
			return err
		}
	}
	if len(detection.Evidence) > 0 {
		if _, err := fmt.Fprintln(w, "  Evidence"); err != nil {
			return err
		}
		for _, evidence := range detection.Evidence {
			if _, err := fmt.Fprintf(w, "    %s %s\n", evidence.Kind, evidence.Value); err != nil {
				return err
			}
		}
	}
	if caps := capabilities(detection.Capabilities); caps != "" {
		if _, err := fmt.Fprintf(w, "  Capabilities %s\n", caps); err != nil {
			return err
		}
	}
	return nil
}

func RenderTest(w io.Writer, detection coresource.Detection) error {
	if len(detection.Evidence) > 0 {
		for _, evidence := range detection.Evidence {
			mark := "✗"
			if evidence.OK {
				mark = "✓"
			}
			if _, err := fmt.Fprintf(w, "%s %s %s\n", mark, evidence.Kind, evidence.Value); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	if detection.Status == coresource.StatusReady {
		_, err := fmt.Fprintf(w, "%s is ready.\n", detection.DisplayName)
		return err
	}
	if detection.Reason != "" {
		_, err := fmt.Fprintf(w, "%s is %s: %s\n", detection.DisplayName, detection.Status, detection.Reason)
		return err
	}
	_, err := fmt.Fprintf(w, "%s is %s.\n", detection.DisplayName, detection.Status)
	return err
}

func renderSummary(w io.Writer, detection coresource.Detection) error {
	if _, err := fmt.Fprintf(w, "%s %s\n", marker(detection.Status), detection.DisplayName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Status      %s\n", detection.Status); err != nil {
		return err
	}
	if detection.Location != "" {
		if _, err := fmt.Fprintf(w, "  Location    %s\n", detection.Location); err != nil {
			return err
		}
	}
	if detection.Endpoint != "" {
		if _, err := fmt.Fprintf(w, "  Endpoint    %s\n", detection.Endpoint); err != nil {
			return err
		}
	}
	if detection.Origin != "" {
		if _, err := fmt.Fprintf(w, "  Detection   %s\n", detection.Origin); err != nil {
			return err
		}
	}
	if detection.Confidence != "" {
		if _, err := fmt.Fprintf(w, "  Confidence  %s\n", detection.Confidence); err != nil {
			return err
		}
	}
	return nil
}

func marker(status coresource.Status) string {
	switch status {
	case coresource.StatusReady:
		return "✓"
	case coresource.StatusInstalled:
		return "○"
	default:
		return "✗"
	}
}

func capabilities(caps *coresource.Capabilities) string {
	if caps == nil {
		return ""
	}
	var names []string
	if caps.SessionDiscovery {
		names = append(names, "session-discovery")
	}
	if caps.TokenUsage {
		names = append(names, "token-usage")
	}
	if caps.ToolCalls {
		names = append(names, "tool-calls")
	}
	if caps.ToolOutput {
		names = append(names, "tool-output")
	}
	return strings.Join(names, ", ")
}
