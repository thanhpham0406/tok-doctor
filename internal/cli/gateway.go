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
	return cmd
}

func newGatewayStartCommand(ctx context.Context, stdout io.Writer, tok *app.App) *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start one or all enabled gateway profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, set, err := tok.GatewayStart(cmd.Context(), profileName)
			if err != nil {
				return err
			}
			defer rt.Shutdown(context.Background())

			printGatewayStartup(stdout, set.Profiles)

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			if err := rt.Wait(ctx); err != nil && err != context.Canceled {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "start only the named profile")
	return cmd
}

func newGatewayStatusCommand(stdout io.Writer, tok *app.App) *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the status of gateway profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := tok.GatewayStatus(profileName)
			if err != nil {
				return err
			}
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].Profile < rows[j].Profile })
			fmt.Fprintln(stdout, "Profile          Requests   Last request   State")
			fmt.Fprintln(stdout, "---------------  ---------  -------------  -------")
			for _, row := range rows {
				last := "—"
				if !row.LastSeen.IsZero() {
					last = humaniseRelative(time.Since(row.LastSeen))
				}
				fmt.Fprintf(stdout, "%-15s  %9d  %-13s  %s\n",
					truncate(row.Profile, 15),
					row.Requests,
					last,
					row.State,
				)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "show status for only the named profile")
	return cmd
}

func printGatewayStartup(w io.Writer, profiles []gateway.Profile) {
	fmt.Fprintln(w, "TokDoctor Gateway")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Profile          Listen           Protocol             Upstream")
	for _, p := range profiles {
		fmt.Fprintf(w, "%-15s  %-15s  %-20s %s\n",
			truncate(p.Name, 15),
			truncate(p.Listen, 15),
			truncate(p.Protocol, 20),
			truncate(p.Upstream, 60),
		)
	}
	fmt.Fprintln(w)
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
