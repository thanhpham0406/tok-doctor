package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thanhpham0406/tok-doctor/internal/app"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	reportusage "github.com/thanhpham0406/tok-doctor/internal/report/usage"
)

var errInspectCancelled = errors.New("inspection cancelled")

func inspectSessionID(cmd *cobra.Command, tok *app.App, args []string, format string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	switch format {
	case "terminal":
		return pickInspectSession(cmd, tok)
	case "json":
		return "", fmt.Errorf("session-id is required with --format json")
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

func pickInspectSession(cmd *cobra.Command, tok *app.App) (string, error) {
	result, err := tok.SessionsAll(cmd.Context())
	if err != nil {
		return "", err
	}
	sessions := result.Sessions
	if len(sessions) == 0 {
		return "", errors.New("no sessions available to inspect")
	}
	out := cmd.OutOrStdout()
	if err := renderSessionPicker(out, sessions); err != nil {
		return "", err
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	for {
		if _, err := fmt.Fprintf(out, "Choose [1-%d]: ", len(sessions)); err != nil {
			return "", err
		}
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", readErr
		}
		choice := strings.TrimSpace(line)
		if choice == "" && errors.Is(readErr, io.EOF) {
			return "", errors.New("no session selected: input closed")
		}
		switch strings.ToLower(choice) {
		case "q", "quit":
			return "", errInspectCancelled
		}
		index, convErr := strconv.Atoi(choice)
		if convErr != nil || index < 1 || index > len(sessions) {
			if _, err := fmt.Fprintf(out, "Enter a number between 1 and %d, or q to quit.\n", len(sessions)); err != nil {
				return "", err
			}
			continue
		}
		return sessions[index-1].ID, nil
	}
}

func renderSessionPicker(w io.Writer, sessions []model.Session) error {
	if _, err := fmt.Fprintln(w, "Select a session (or q to quit):"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for index, session := range sessions {
		if _, err := fmt.Fprintf(tw, "  %d.\t%s\t%s\t%s\t%s\n",
			index+1,
			pickerSource(session),
			pickerUpdated(session),
			pickerModel(session),
			pickerTotal(session),
		); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

func pickerSource(session model.Session) string {
	source := session.Source
	if source == "" {
		source = string(session.Agent)
	}
	if source == "" {
		return "-"
	}
	runes := []rune(source)
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}

func pickerUpdated(session model.Session) string {
	switch {
	case session.UpdatedAt != nil:
		return session.UpdatedAt.Format("2006-01-02")
	case session.StartedAt != nil:
		return session.StartedAt.Format("2006-01-02")
	default:
		return "-"
	}
}

func pickerModel(session model.Session) string {
	if strings.TrimSpace(session.Model) == "" {
		return "-"
	}
	return session.Model
}

func pickerTotal(session model.Session) string {
	if !session.Usage.Total.Available() {
		return "-"
	}
	return reportusage.FormatTokenCount(session.Usage.Total) + " tokens"
}
