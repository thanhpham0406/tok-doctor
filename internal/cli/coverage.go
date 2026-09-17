package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/thanhpham0406/tok-doctor/internal/app"
	"github.com/thanhpham0406/tok-doctor/internal/coverage"
	reportcoverage "github.com/thanhpham0406/tok-doctor/internal/report/coverage"
)

func newCoverageCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Report how much of the data an analysis rule needs is observable",
	}
	cmd.AddCommand(newCoverageToolOutputCommand(ctx, stdout, tok))
	return cmd
}

func newCoverageToolOutputCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	var sourceName string
	var all bool

	cmd := &cobra.Command{
		Use:   "tool-output [session-id]",
		Short: "Report tool output coverage of sessions, a source, or every source",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) == 1 && (sourceName != "" || all):
				return fmt.Errorf("session-id cannot be combined with --source or --all")
			case all && sourceName != "":
				return fmt.Errorf("--all cannot be combined with --source")
			case len(args) == 0 && sourceName == "" && !all:
				return fmt.Errorf("session-id, --source, or --all is required")
			}

			result, err := coverageResult(cmd.Context(), tok, args, sourceName, all)
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportcoverage.Render(stdout, result)
			case "json":
				return reportcoverage.RenderJSON(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().StringVar(&sourceName, "source", "", "summarize every session of a source")
	cmd.Flags().BoolVar(&all, "all", false, "summarize every source that captures tool output")
	return cmd
}

func coverageResult(ctx context.Context, tok *app.App, args []string, sourceName string, all bool) (coverage.Result, error) {
	switch {
	case all:
		return tok.CoverageAll(ctx)
	case sourceName != "":
		return tok.CoverageSource(ctx, sourceName)
	default:
		return tok.CoverageSession(ctx, args[0])
	}
}
