package zim_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/stazelabs/gozim/zim"
)

func zimPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata", "small.zim")
}

func skipIfMissing() {
	if _, err := os.Stat(zimPath()); os.IsNotExist(err) {
		fmt.Println("(skipped: test ZIM not found)")
		os.Exit(0)
	}
}

func ExampleOpen() {
	skipIfMissing()

	a, err := zim.Open(zimPath())
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	fmt.Println("entries:", a.EntryCount())
	fmt.Println("has main:", a.HasMainEntry())
	// Output:
	// entries: 16
	// has main: true
}

func ExampleArchive_EntryByPath() {
	skipIfMissing()

	a, err := zim.Open(zimPath())
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	entry, err := a.EntryByPath("C/main.html")
	if err != nil {
		log.Fatal(err)
	}

	data, err := entry.ReadContent()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("title:", entry.Title())
	fmt.Println("html:", strings.Contains(string(data), "<html>"))
	// Output:
	// title: Test ZIM file
	// html: true
}

func ExampleArchive_Entries() {
	skipIfMissing()

	a, err := zim.Open(zimPath())
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	var count int
	for entry := range a.Entries() {
		if entry.MIMEType() == "text/html" {
			count++
		}
	}
	fmt.Println("html entries:", count)
	// Output:
	// html entries: 1
}

func ExampleArchive_MainEntry() {
	skipIfMissing()

	a, err := zim.Open(zimPath())
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	main, err := a.MainEntry()
	if err != nil {
		log.Fatal(err)
	}

	// MainEntry may be a redirect; resolve to the content entry.
	resolved, err := main.Resolve()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("redirect:", main.IsRedirect())
	fmt.Println("path:", resolved.FullPath())
	// Output:
	// redirect: true
	// path: C/main.html
}
