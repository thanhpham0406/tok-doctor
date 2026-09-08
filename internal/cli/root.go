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
	"github.com/thanhpham0406/tok-doctor/internal/pricing"
	reportcost "github.com/thanhpham0406/tok-doctor/internal/report/cost"
	reportinspect "github.com/thanhpham0406/tok-doctor/internal/report/inspect"
	reportjson "github.com/thanhpham0406/tok-doctor/internal/report/json"
	reportpricing "github.com/thanhpham0406/tok-doctor/internal/report/pricing"
	reportsessions "github.com/thanhpham0406/tok-doctor/internal/report/sessions"
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
	cmd.AddCommand(newSessionsCommand(ctx, stdout, tok))
	cmd.AddCommand(newInspectCommand(ctx, stdout, tok))
	cmd.AddCommand(newCostCommand(ctx, stdout, tok))
	cmd.AddCommand(newPricingCommand(stdout, tok))
	cmd.AddCommand(newSourcesCommand(ctx, stdout, tok))
	cmd.AddCommand(newSourceCommand(ctx, stdout, tok))

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	return cmd
}

func newCostCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	var all bool
	var provider string

	cmd := &cobra.Command{
		Use:   "cost [session-id]",
		Short: "Estimate API-equivalent cost",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (all || provider != "") {
				return fmt.Errorf("session-id cannot be combined with --all or --provider")
			}
			if all && provider != "" {
				return fmt.Errorf("--all cannot be combined with --provider")
			}
			if len(args) == 0 && !all && provider == "" {
				return fmt.Errorf("session-id is required unless --all or --provider is set")
			}
			switch format {
			case "terminal":
				if all || provider != "" {
					result, err := costCollection(ctx, tok, all, provider)
					if err != nil {
						return err
					}
					return reportcost.RenderCollection(stdout, result)
				}
				result, err := tok.Cost(ctx, args[0])
				if err != nil {
					return err
				}
				return reportcost.Render(stdout, result)
			case "json":
				if all || provider != "" {
					result, err := costCollection(ctx, tok, all, provider)
					if err != nil {
						return err
					}
					return reportcost.RenderCollectionJSON(stdout, result)
				}
				result, err := tok.Cost(ctx, args[0])
				if err != nil {
					return err
				}
				return reportcost.RenderJSON(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().BoolVar(&all, "all", false, "estimate all resolvable cost grouped by provider and source")
	cmd.Flags().StringVar(&provider, "provider", "", "estimate all resolvable cost for a pricing provider")
	return cmd
}

func costCollection(ctx context.Context, tok *app.App, all bool, provider string) (pricing.CostCollection, error) {
	if all {
		return tok.CostAll(ctx)
	}
	return tok.CostProvider(ctx, provider)
}

func newPricingCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pricing",
		Short: "Manage pricing catalog data",
	}
	cmd.AddCommand(newPricingStatusCommand(stdout, tok))
	cmd.AddCommand(newPricingUpdateCommand(stdout, tok))
	cmd.AddCommand(newPricingListCommand(stdout, tok))
	cmd.AddCommand(newPricingShowCommand(stdout, tok))
	return cmd
}

func newPricingStatusCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show active pricing catalog status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := tok.PricingStatus()
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportpricing.RenderStatus(stdout, status)
			case "json":
				return writeJSON(stdout, status)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newPricingUpdateCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Download the maintained TokDoctor pricing catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := tok.PricingUpdate()
			switch format {
			case "terminal":
				if renderErr := reportpricing.RenderUpdate(stdout, result, err); renderErr != nil {
					return renderErr
				}
			case "json":
				if renderErr := writeJSON(stdout, result); renderErr != nil {
					return renderErr
				}
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newPricingListCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pricing profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			active, err := tok.PricingList()
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportpricing.RenderList(stdout, active)
			case "json":
				return writeJSON(stdout, active)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newPricingShowCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "show <model>",
		Short: "Show a resolved pricing profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			active, profile, ok, err := tok.PricingShow(args[0])
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportpricing.RenderShow(stdout, active, args[0], profile, ok)
			case "json":
				return writeJSON(stdout, struct {
					Catalog pricing.CatalogRef      `json:"catalog"`
					Model   string                  `json:"model"`
					Found   bool                    `json:"found"`
					Profile *pricing.PricingProfile `json:"profile,omitempty"`
				}{
					Catalog: pricing.CatalogRef{Source: active.Source, Version: active.Catalog.Version},
					Model:   args[0],
					Found:   ok,
					Profile: profilePtr(profile, ok),
				})
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func profilePtr(profile pricing.PricingProfile, ok bool) *pricing.PricingProfile {
	if !ok {
		return nil
	}
	return &profile
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

func newSessionsCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var format string
	var sourceName string

	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List source sessions with authoritative usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := sessionsResult(ctx, tok, sourceName)
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportsessions.Render(stdout, result)
			case "json":
				return writeJSON(stdout, result)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().StringVar(&sourceName, "source", "", "source name")
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

func sessionsResult(ctx context.Context, tok *app.App, sourceName string) (model.SessionsResult, error) {
	if sourceName == "" {
		return tok.SessionsAll(ctx)
	}
	return tok.Sessions(ctx, sourceName)
}

func newInspectCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var (
		format   string
		allTurns bool
		turn     int
		evidence bool
	)

	cmd := &cobra.Command{
		Use:   "inspect <session-id>",
		Short: "Show the authoritative usage and per-turn breakdown of a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if allTurns && turn > 0 {
				return fmt.Errorf("--all-turns and --turn cannot be combined")
			}
			session, err := tok.Inspect(ctx, args[0])
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return reportinspect.Render(stdout, session, reportinspect.Options{
					AllTurns:     allTurns,
					Turn:         turn,
					ShowEvidence: evidence,
				})
			case "json":
				return reportinspect.RenderJSON(stdout, session)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	cmd.Flags().BoolVar(&allTurns, "all-turns", false, "show every turn for the session")
	cmd.Flags().IntVar(&turn, "turn", 0, "show a single turn by sequence number")
	cmd.Flags().BoolVar(&evidence, "evidence", false, "include evidence/provenance in the rendered output")
	return cmd
}
