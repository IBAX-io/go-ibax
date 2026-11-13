package syspar

import "testing"

// TestColTypeVersion checks each replacement of the data types of the columns advances their
// version, after the types are replaced
func TestColTypeVersion(t *testing.T) {
	saved := GetTableColType()
	t.Cleanup(func() { SetTableColType(saved) })
	v := ColTypeVersion()
	types := map[string]map[string]string{"1_t": {"b": "bytea"}}
	SetTableColType(types)
	if got := ColTypeVersion(); got != v+1 {
		t.Fatalf("version %d after a replacement of the types at %d", got, v)
	}
	if !IsByteColumn("1_t", "b") {
		t.Fatal("the types of the version are not the new types")
	}
	SetTableColType(types)
	if got := ColTypeVersion(); got != v+2 {
		t.Fatalf("version %d after the same types again at %d", got, v+1)
	}
}
