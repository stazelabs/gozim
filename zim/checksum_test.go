package zim

import (
	"testing"
)

func TestVerify(t *testing.T) {
	path := testdataPath("small.zim")
	skipIfNoTestdata(t, path)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	if err := a.Verify(); err != nil {
		t.Errorf("Verify: %v", err)
	}
}
