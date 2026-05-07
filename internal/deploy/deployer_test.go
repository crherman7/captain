package deploy

import "testing"

func TestStampHash_CreatesNestedMap(t *testing.T) {
	values := map[string]interface{}{}
	stampHash(values, "abc")

	cap, ok := values["captain"].(map[string]interface{})
	if !ok {
		t.Fatalf("captain key missing or wrong type: %#v", values)
	}
	if cap["deployHash"] != "abc" {
		t.Errorf("deployHash = %v, want %q", cap["deployHash"], "abc")
	}
}

func TestStampHash_PreservesExistingCaptainKeys(t *testing.T) {
	values := map[string]interface{}{
		"captain": map[string]interface{}{"other": "x"},
	}
	stampHash(values, "abc")

	cap := values["captain"].(map[string]interface{})
	if cap["other"] != "x" {
		t.Errorf("existing key lost: %#v", cap)
	}
	if cap["deployHash"] != "abc" {
		t.Errorf("deployHash = %v, want %q", cap["deployHash"], "abc")
	}
}

func TestStampHash_EmptyHashIsNoop(t *testing.T) {
	values := map[string]interface{}{}
	stampHash(values, "")
	if _, ok := values["captain"]; ok {
		t.Errorf("expected no captain key when hash is empty: %#v", values)
	}
}

func TestStoredHash(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want string
	}{
		{
			name: "happy path",
			in:   map[string]interface{}{"captain": map[string]interface{}{"deployHash": "xyz"}},
			want: "xyz",
		},
		{
			name: "missing captain key",
			in:   map[string]interface{}{"foo": "bar"},
			want: "",
		},
		{
			name: "captain not a map",
			in:   map[string]interface{}{"captain": "not-a-map"},
			want: "",
		},
		{
			name: "deployHash missing",
			in:   map[string]interface{}{"captain": map[string]interface{}{"other": "x"}},
			want: "",
		},
		{
			name: "nil values",
			in:   nil,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := storedHash(tt.in); got != tt.want {
				t.Errorf("storedHash() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStampThenRead_RoundTrip(t *testing.T) {
	values := map[string]interface{}{"image": map[string]interface{}{"tag": "v1"}}
	stampHash(values, "deadbeef")
	if storedHash(values) != "deadbeef" {
		t.Errorf("round trip failed: %#v", values)
	}
}
