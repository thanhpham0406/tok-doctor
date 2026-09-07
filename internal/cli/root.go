package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/thanhpham0406/tok-doctor/internal/app"
	reportjson "github.com/thanhpham0406/tok-doctor/internal/report/json"
	"github.com/thanhpham0406/tok-doctor/internal/report/terminal"
	"github.com/thanhpham0406/tok-doctor/internal/webui"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func Execute(ctx context.Context, stdout, stderr io.Writer, logger *slog.Logger) error {
	return newRootCommand(ctx, stdout, stderr, logger).Execute()
}

func newRootCommand(ctx context.Context, stdout, stderr io.Writer, logger *slog.Logger) *cobra.Command {
	tok := app.New()

	cmd := &cobra.Command{
		Use:           "tok",
		Short:         "Local-first token profiler and context optimizer",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newVersionCommand(stdout))
	cmd.AddCommand(newDoctorCommand(ctx, stdout, tok))
	cmd.AddCommand(newUICommand(ctx, stdout, logger, tok))

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	return cmd
}

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(stdout, "tok %s (%s, %s)\n", version, commit, date)
			return err
		},
	}
}

func newDoctorCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	var serveUI bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Analyze local AI coding-agent token usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := tok.Doctor(ctx)
			if err != nil {
				return err
			}

			if serveUI {
				return webui.Serve(ctx, stdout, result)
			}

			switch format {
			case "terminal":
				return terminal.Render(stdout, result)
			case "json":
				return reportjson.Render(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}

	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().BoolVar(&serveUI, "ui", false, "serve the local Web UI on 127.0.0.1")

	return cmd
}

func newUICommand(ctx context.Context, stdout io.Writer, logger *slog.Logger, tok *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Serve the local TokDoctor Web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := tok.Doctor(ctx)
			if err != nil {
				return err
			}

			logger.Info("starting local web ui", "addr", webui.DefaultAddr)
			return webui.Serve(ctx, stdout, result)
		},
	}
}
