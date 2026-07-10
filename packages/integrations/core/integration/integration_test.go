package integration

import "testing"

func TestCatalogPreservesOrderAndRecognizesKeys(t *testing.T) {
	c := Catalog{{Key: "first"}, {Key: "second"}}
	if !c.Known("second") || c.Known("missing") {
		t.Fatal("Known mismatch")
	}
	keys := c.Keys()
	if len(keys) != 2 || keys[0] != "first" || keys[1] != "second" {
		t.Fatalf("keys=%v", keys)
	}
}
