package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/johnlotito/wt/internal/terminal"
	"github.com/spf13/cobra"
)

func newTerminalsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "terminals",
		Short: "List available terminal adapters",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tDETECTED")
			for _, name := range terminal.Names() {
				a, _ := terminal.Get(name)
				det := ""
				if a.Detect() {
					det = "yes (current)"
				}
				fmt.Fprintf(w, "%s\t%s\n", name, det)
			}
			return w.Flush()
		},
	}
}
