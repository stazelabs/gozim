package zim

import "testing"

func openBenchArchive(b *testing.B) *Archive {
	b.Helper()
	path := testdataPath("small.zim")
	a, err := Open(path)
	if err != nil {
		b.Skipf("test ZIM file not available: %v", err)
	}
	b.Cleanup(func() { a.Close() })
	return a
}

func BenchmarkOpen(b *testing.B) {
	path := testdataPath("small.zim")
	b.ResetTimer()
	for b.Loop() {
		a, err := Open(path)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		a.Close()
	}
}

func BenchmarkEntryByPath(b *testing.B) {
	a := openBenchArchive(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = a.EntryByPath("C/main.html")
	}
}

func BenchmarkEntryByTitle(b *testing.B) {
	a := openBenchArchive(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = a.EntryByTitle('C', "Test ZIM file")
	}
}

func BenchmarkEntryByIndex(b *testing.B) {
	a := openBenchArchive(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = a.EntryByIndex(1)
	}
}

func BenchmarkReadContent(b *testing.B) {
	a := openBenchArchive(b)
	entry, err := a.EntryByPath("C/main.html")
	if err != nil {
		b.Fatalf("EntryByPath: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = entry.ReadContent()
	}
}

func BenchmarkReadContentCold(b *testing.B) {
	path := testdataPath("small.zim")
	b.ResetTimer()
	for b.Loop() {
		a, err := Open(path)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		entry, err := a.EntryByPath("C/main.html")
		if err != nil {
			b.Fatalf("EntryByPath: %v", err)
		}
		_, _ = entry.ReadContent()
		a.Close()
	}
}

func BenchmarkIterateAll(b *testing.B) {
	a := openBenchArchive(b)
	b.ResetTimer()
	for b.Loop() {
		for range a.Entries() {
		}
	}
}

func BenchmarkVerify(b *testing.B) {
	a := openBenchArchive(b)
	b.ResetTimer()
	for b.Loop() {
		_ = a.Verify()
	}
}
