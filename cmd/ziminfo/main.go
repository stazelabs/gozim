package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stazelabs/gozim/zim"
)

func main() {
	cmd := &cobra.Command{
		Use:   "ziminfo <file.zim>",
		Short: "Display metadata and structure of a ZIM file",
		Args:  cobra.ExactArgs(1),
		RunE:  run,
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	path := args[0]
	a, err := zim.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer a.Close()

	fmt.Printf("File:          %s\n", path)
	fmt.Printf("UUID:          %x\n", a.UUID())
	fmt.Printf("Entry count:   %d\n", a.EntryCount())
	fmt.Printf("Cluster count: %d\n", a.ClusterCount())
	fmt.Printf("Has main page: %v\n", a.HasMainEntry())

	if a.HasMainEntry() {
		main, err := a.MainEntry()
		if err == nil {
			resolved, err := main.Resolve()
			if err == nil {
				fmt.Printf("Main page:     %s\n", resolved.FullPath())
			}
		}
	}

	fmt.Printf("\nMIME types:\n")
	for i, m := range a.MIMETypes() {
		fmt.Printf("  [%d] %s\n", i, m)
	}

	nsCounts := map[byte]int{}
	for e := range a.Entries() {
		nsCounts[e.Namespace()]++
	}
	fmt.Printf("\nNamespaces:\n")
	for ns, count := range nsCounts {
		fmt.Printf("  %c: %d entries\n", ns, count)
	}

	fmt.Printf("\nChecksum verification: ")
	if err := a.Verify(); err != nil {
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		fmt.Printf("OK\n")
	}

	return nil
}
