package smart

import "testing"

func TestSetsID(t *testing.T) {
	for _, c := range []struct {
		fields []string
		want   bool
	}{
		{[]string{"n", "amount"}, false},
		{[]string{"n", "id"}, true},
		{[]string{"+id"}, true},
		{[]string{"timestamp id"}, true},
		{[]string{"idx", "value->id"}, false},
		{nil, false},
	} {
		if got := setsID(c.fields); got != c.want {
			t.Errorf("setsID(%q) = %v, want %v", c.fields, got, c.want)
		}
	}
}
