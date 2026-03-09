package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/stazelabs/gozim/zim"
)

var jsonOutput bool

func main() {
	cmd := &cobra.Command{
		Use:   "ziminfo <file.zim>",
		Short: "Display metadata and structure of a ZIM file",
		Args:  cobra.ExactArgs(1),
		RunE:  run,
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type zimInfo struct {
	File          string         `json:"file"`
	UUID          string         `json:"uuid"`
	EntryCount    uint32         `json:"entryCount"`
	ClusterCount  uint32         `json:"clusterCount"`
	HasMainPage   bool           `json:"hasMainPage"`
	MainPage      string         `json:"mainPage,omitempty"`
	MIMETypes     []string       `json:"mimeTypes"`
	Namespaces    map[string]int `json:"namespaces"`
	ChecksumValid bool           `json:"checksumValid"`
	ChecksumError string         `json:"checksumError,omitempty"`
}

func gather(path string) (*zimInfo, error) {
	a, err := zim.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer a.Close()

	info := &zimInfo{
		File:         path,
		UUID:         fmt.Sprintf("%x", a.UUID()),
		EntryCount:   a.EntryCount(),
		ClusterCount: a.ClusterCount(),
		HasMainPage:  a.HasMainEntry(),
		MIMETypes:    a.MIMETypes(),
		Namespaces:   map[string]int{},
	}

	if a.HasMainEntry() {
		if main, err := a.MainEntry(); err == nil {
			if resolved, err := main.Resolve(); err == nil {
				info.MainPage = resolved.FullPath()
			}
		}
	}

	for e := range a.Entries() {
		info.Namespaces[string(e.Namespace())]++
	}

	if err := a.Verify(); err != nil {
		info.ChecksumError = err.Error()
	} else {
		info.ChecksumValid = true
	}

	return info, nil
}

func run(cmd *cobra.Command, args []string) error {
	info, err := gather(args[0])
	if err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	fmt.Printf("File:          %s\n", info.File)
	fmt.Printf("UUID:          %s\n", info.UUID)
	fmt.Printf("Entry count:   %d\n", info.EntryCount)
	fmt.Printf("Cluster count: %d\n", info.ClusterCount)
	fmt.Printf("Has main page: %v\n", info.HasMainPage)
	if info.MainPage != "" {
		fmt.Printf("Main page:     %s\n", info.MainPage)
	}

	fmt.Printf("\nMIME types:\n")
	for i, m := range info.MIMETypes {
		fmt.Printf("  [%d] %s\n", i, m)
	}

	// Sort namespace keys for stable output.
	nsKeys := make([]string, 0, len(info.Namespaces))
	for ns := range info.Namespaces {
		nsKeys = append(nsKeys, ns)
	}
	sort.Strings(nsKeys)
	fmt.Printf("\nNamespaces:\n")
	for _, ns := range nsKeys {
		fmt.Printf("  %s: %d entries\n", ns, info.Namespaces[ns])
	}

	fmt.Printf("\nChecksum verification: ")
	if info.ChecksumValid {
		fmt.Printf("OK\n")
	} else {
		fmt.Printf("FAILED (%s)\n", info.ChecksumError)
	}

	return nil
}
