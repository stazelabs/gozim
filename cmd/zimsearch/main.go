package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stazelabs/gozim/zim"
)

func main() {
	var ns string
	var limit int
	var insensitive bool

	cmd := &cobra.Command{
		Use:   "zimsearch <file.zim> <query>",
		Short: "Search entries in a ZIM file by title prefix",
		Long: `zimsearch searches ZIM file entries by title prefix.

By default searches the C (content) namespace case-sensitively.
Use -i for case-insensitive search (performs a full linear scan — slower).

Output: one result per line in the format "fullpath\ttitle".`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(ns) != 1 {
				return fmt.Errorf("namespace must be a single character (e.g. C, M, X)")
			}
			return run(args[0], args[1], ns[0], limit, insensitive)
		},
	}

	cmd.Flags().StringVarP(&ns, "namespace", "n", "C", "namespace to search (single character)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "maximum number of results (0 = unlimited)")
	cmd.Flags().BoolVarP(&insensitive, "insensitive", "i", false, "case-insensitive search (slower, full scan)")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(path, query string, ns byte, limit int, insensitive bool) error {
	a, err := zim.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer a.Close()

	var seq func(func(zim.Entry) bool)
	if insensitive {
		seq = a.EntriesByTitlePrefixFold(ns, query)
	} else {
		seq = a.EntriesByTitlePrefix(ns, query)
	}

	count := 0
	for e := range seq {
		fmt.Printf("%-40s\t%s\n", e.FullPath(), e.Title())
		count++
		if limit > 0 && count >= limit {
			break
		}
	}

	if count == 0 {
		fmt.Fprintf(os.Stderr, "no results for %q in namespace %c\n", query, ns)
	}

	return nil
}
