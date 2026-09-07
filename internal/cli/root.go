package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/thanhpham0406/tok-doctor/internal/app"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	reportjson "github.com/thanhpham0406/tok-doctor/internal/report/json"
	reportsource "github.com/thanhpham0406/tok-doctor/internal/report/source"
	"github.com/thanhpham0406/tok-doctor/internal/report/terminal"
	reportusage "github.com/thanhpham0406/tok-doctor/internal/report/usage"
	"github.com/thanhpham0406/tok-doctor/internal/source"
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
	cmd.AddCommand(newUsageCommand(ctx, stdout, tok))
	cmd.AddCommand(newSourcesCommand(ctx, stdout, tok))
	cmd.AddCommand(newSourceCommand(ctx, stdout, tok))

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	return cmd
}

func newSourcesCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "sources",
		Short: "List supported AI coding-agent sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := tok.Sources(ctx)
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportsource.RenderList(stdout, result)
			case "json":
				return writeJSON(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newSourceCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage source detection configuration",
	}
	cmd.AddCommand(newSourceShowCommand(ctx, stdout, tok))
	cmd.AddCommand(newSourceSetCommand(stdout, tok))
	cmd.AddCommand(newSourceTestCommand(ctx, stdout, tok))
	cmd.AddCommand(newSourceResetCommand(stdout, tok))
	return cmd
}

func newSourceShowCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	var path string
	var endpoint string

	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show source detection details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := tok.ShowSource(ctx, args[0], source.Override{Path: path, Endpoint: endpoint})
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportsource.RenderShow(stdout, result)
			case "json":
				return writeJSON(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().StringVar(&path, "path", "", "use a source path for this command only")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "use a source endpoint for this command only")
	return cmd
}

func newSourceSetCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var path string
	var endpoint string

	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Set a source location override",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := tok.SetSource(args[0], source.Override{Path: path, Endpoint: endpoint}); err != nil {
				return err
			}
			_, err := fmt.Fprintf(stdout, "Updated %s source configuration.\n", args[0])
			return err
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "source data path")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "source endpoint")
	return cmd
}

func newSourceTestCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	var path string
	var endpoint string

	cmd := &cobra.Command{
		Use:   "test <name>",
		Short: "Test whether a source is usable",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := tok.TestSource(ctx, args[0], source.Override{Path: path, Endpoint: endpoint})
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportsource.RenderTest(stdout, result)
			case "json":
				return writeJSON(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().StringVar(&path, "path", "", "use a source path for this command only")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "use a source endpoint for this command only")
	return cmd
}

func newSourceResetCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "reset <name>",
		Short: "Reset a source override to auto-detection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := tok.ResetSource(args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(stdout, "Reset %s source configuration.\n", args[0])
			return err
		},
	}
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
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

func newUsageCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	var sourceName string
	var all bool

	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Report authoritative token usage for a source",
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && sourceName != "" {
				return fmt.Errorf("--all cannot be used with --source")
			}
			if !all && sourceName == "" {
				return fmt.Errorf("--source is required unless --all is set")
			}

			result, err := usageResult(ctx, tok, sourceName, all)
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportusage.Render(stdout, result)
			case "json":
				return writeJSON(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().StringVar(&sourceName, "source", "", "source name")
	cmd.Flags().BoolVar(&all, "all", false, "report usage for all usage-capable sources")
	return cmd
}

func usageResult(ctx context.Context, tok *app.App, sourceName string, all bool) (model.UsageResult, error) {
	if all {
		return tok.UsageAll(ctx)
	}
	entry, err := tok.Usage(ctx, sourceName)
	if err != nil {
		return model.UsageResult{}, err
	}
	return model.UsageResult{Sources: []model.UsageEntry{entry}}, nil
}
