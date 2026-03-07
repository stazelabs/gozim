package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stazelabs/gozim/zim"
)

func main() {
	cmd := &cobra.Command{
		Use:   "zimverify <file.zim> [file.zim ...]",
		Short: "Verify the MD5 checksum of one or more ZIM files",
		Args:  cobra.MinimumNArgs(1),
		RunE:  run,
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	anyFailed := false

	for _, path := range args {
		ok, msg := verify(path)
		if ok {
			fmt.Printf("OK      %s\n", path)
		} else {
			fmt.Printf("FAILED  %s: %s\n", path, msg)
			anyFailed = true
		}
	}

	if anyFailed {
		return errors.New("one or more files failed verification")
	}
	return nil
}

func verify(path string) (ok bool, msg string) {
	a, err := zim.Open(path)
	if err != nil {
		return false, err.Error()
	}
	defer a.Close()

	if err := a.Verify(); err != nil {
		return false, err.Error()
	}
	return true, ""
}
