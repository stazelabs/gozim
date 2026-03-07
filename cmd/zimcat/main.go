package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stazelabs/gozim/zim"
)

func main() {
	var listFlag bool
	var metaFlag bool

	cmd := &cobra.Command{
		Use:   "zimcat <file.zim> [path]",
		Short: "Extract content from a ZIM file",
		Long:  "Extract content from a ZIM file by path, list all entries, or show metadata.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(args, listFlag, metaFlag)
		},
	}

	cmd.Flags().BoolVarP(&listFlag, "list", "l", false, "list all entries")
	cmd.Flags().BoolVarP(&metaFlag, "meta", "m", false, "show metadata entries (M namespace)")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(args []string, listFlag, metaFlag bool) error {
	a, err := zim.Open(args[0])
	if err != nil {
		return fmt.Errorf("opening %s: %w", args[0], err)
	}
	defer a.Close()

	if listFlag {
		for e := range a.Entries() {
			redir := ""
			if e.IsRedirect() {
				redir = " [redirect]"
			}
			fmt.Printf("%s  %s%s\n", e.MIMEType(), e.FullPath(), redir)
		}
		return nil
	}

	if metaFlag {
		for e := range a.EntriesByNamespace('M') {
			data, err := e.ReadContent()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading %s: %v\n", e.FullPath(), err)
				continue
			}
			fmt.Printf("%-20s %s\n", e.Path()+":", string(data))
		}
		return nil
	}

	if len(args) < 2 {
		return fmt.Errorf("specify a path or use --list/--meta flags")
	}

	entry, err := a.EntryByPath(args[1])
	if err != nil {
		return err
	}

	data, err := entry.ReadContent()
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}

	os.Stdout.Write(data)
	return nil
}
