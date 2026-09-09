package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/thanhpham0406/tok-doctor/internal/app"
	"github.com/thanhpham0406/tok-doctor/internal/gateway"
)

func newGatewayCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Run the local TokDoctor observation gateway",
	}
	cmd.AddCommand(newGatewayStartCommand(ctx, stdout, tok))
	cmd.AddCommand(newGatewayStatusCommand(stdout, tok))
	cmd.AddCommand(newGatewayStopCommand(ctx, stdout, tok))
	cmd.AddCommand(newGatewaySetupCommand(ctx, stdout, tok))
	cmd.AddCommand(newGatewayRemoveCommand(ctx, stdout, tok))
	cmd.AddCommand(newGatewayRequestsCommand(stdout, tok))
	cmd.AddCommand(newGatewayInspectCommand(stdout, tok))
	cmd.AddCommand(newGatewayChainsCommand(stdout, tok))
	cmd.AddCommand(newGatewayChainCommand(stdout, tok))
	cmd.AddCommand(newGatewayServeCommand(ctx, tok))
	return cmd
}

func newGatewayStartCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start one or all enabled gateway profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := tok.GatewayStart(cmd.Context(), profileName)
			if err != nil {
				return err
			}
			return printGatewayStarted(stdout, results)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "start only the named profile")
	return cmd
}

func newGatewayStatusCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var profileName string
	var format string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the status of gateway profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := tok.GatewayStatus(profileName)
			if err != nil {
				return err
			}
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].Profile < rows[j].Profile })
			if format == "json" {
				return writeJSON(stdout, rows)
			}
			if format != "terminal" {
				return fmt.Errorf("unsupported format %q", format)
			}
			if _, err := fmt.Fprintln(stdout, "Profile          State    Proxy                    Requests   Last request"); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(stdout, "---------------  -------  -----------------------  ---------  -------------"); err != nil {
				return err
			}
			for _, row := range rows {
				last := "—"
				if !row.LastSeen.IsZero() {
					last = humaniseRelative(time.Since(row.LastSeen))
				}
				if _, err := fmt.Fprintf(stdout, "%-15s  %-7s  %-23s  %9d  %-13s\n",
					truncate(row.Profile, 15),
					row.State,
					truncate(row.Proxy, 23),
					row.Requests,
					last,
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "show status for only the named profile")
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newGatewayStopCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop one or all running gateway profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := tok.GatewayStop(cmd.Context(), profileName)
			if err != nil {
				return err
			}
			for _, row := range rows {
				if _, err := fmt.Fprintf(stdout, "Gateway stopped: %s\n", row.Profile); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "stop only the named profile")
	return cmd
}

func newGatewaySetupCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var opts app.GatewaySetupOptions
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create a local gateway profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, created, err := tok.GatewaySetup(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if created {
				_, err = fmt.Fprintf(stdout, "Gateway profile created\n\nProfile    %s\nProxy      %s\nUpstream   %s\n", profile.Name, gateway.ProxyURL(profile.Listen), profile.Upstream)
			} else {
				_, err = fmt.Fprintf(stdout, "Gateway profile already exists\n\nProfile    %s\nProxy      %s\nUpstream   %s\n", profile.Name, gateway.ProxyURL(profile.Listen), profile.Upstream)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Source, "source", "", "source name for the gateway profile")
	cmd.Flags().StringVar(&opts.Listen, "listen", "", "loopback listen address")
	cmd.Flags().StringVar(&opts.Protocol, "protocol", "", "gateway protocol")
	cmd.Flags().StringVar(&opts.Upstream, "upstream", "", "upstream API base URL")
	cmd.Flags().StringVar(&opts.Provider, "provider", "", "provider tag for reporting")
	return cmd
}

func newGatewayRemoveCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "remove <profile>",
		Short: "Remove a gateway profile",
		Long:  "Remove a gateway profile. Capture history is preserved unless --purge is given.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := tok.GatewayRemove(cmd.Context(), args[0], app.GatewayRemoveOptions{Purge: purge}); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(stdout, "Gateway profile removed: %s\n", args[0]); err != nil {
				return err
			}
			if purge {
				_, err := fmt.Fprintln(stdout, "Capture history deleted.")
				return err
			}
			_, err := fmt.Fprintln(stdout, "Capture history preserved.")
			return err
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete capture history for the profile")
	return cmd
}

func newGatewayRequestsCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var (
		profileName string
		format      string
	)
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "List captured gateway requests with attributed context totals",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			summaries, err := tok.GatewayRequests(profileName)
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return gateway.RenderRequests(stdout, summaries)
			case "json":
				return writeJSON(stdout, summaries)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "profile to list requests for (required)")
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newGatewayInspectCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var (
		profileName string
		format      string
	)
	cmd := &cobra.Command{
		Use:   "inspect <exchange-id>",
		Short: "Show the attributed context breakdown for a captured gateway request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := tok.GatewayInspect(profileName, args[0])
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return gateway.RenderInspect(stdout, summary)
			case "json":
				return writeJSON(stdout, summary)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "profile the exchange was captured under (required)")
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newGatewayChainsCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var (
		profileName string
		format      string
	)
	cmd := &cobra.Command{
		Use:   "chains",
		Short: "List correlated gateway model-call chains",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			chains, err := tok.GatewayChains(profileName)
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return gateway.RenderChains(stdout, chains)
			case "json":
				return writeJSON(stdout, chains)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "profile to list chains for (required)")
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newGatewayChainCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var (
		profileName string
		format      string
	)
	cmd := &cobra.Command{
		Use:   "chain <chain-id>",
		Short: "Show a correlated gateway model-call chain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chain, err := tok.GatewayChain(profileName, args[0])
			if err != nil {
				return err
			}
			switch format {
			case "terminal":
				return gateway.RenderChain(stdout, chain)
			case "json":
				return writeJSON(stdout, chain)
			default:
				return fmt.Errorf("unsupported format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "profile the chain was captured under (required)")
	cmd.Flags().StringVar(&format, "format", "terminal", "output format: terminal or json")
	return cmd
}

func newGatewayServeCommand(ctx context.Context, tok *app.App) *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:    "_serve",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runCtx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return tok.GatewayServe(runCtx, profileName)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "serve the named profile")
	return cmd
}

func printGatewayStarted(w io.Writer, results []gateway.StartResult) error {
	for i, result := range results {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if result.AlreadyRunning {
			if _, err := fmt.Fprintln(w, "Gateway already running"); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintln(w, "Gateway started"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Profile    %s\n", result.Profile); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Listen     %s\n", result.Listen); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Proxy      %s\n", result.Proxy); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "PID        %d\n", result.PID); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Check status:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  tok gateway status --profile %s\n", result.Profile); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Stop:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  tok gateway stop --profile %s\n", result.Profile); err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func humaniseRelative(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
