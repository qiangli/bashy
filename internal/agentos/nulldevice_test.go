package agentos

import (
	"reflect"
	"testing"
)

func TestMapNullDevice(t *testing.T) {
	in := []string{"-sS", "-o", "/dev/null", "--output=/dev/null", "-w", "%{http_code}", "/dev/nullx", "http://x"}
	want := []string{"-sS", "-o", "NUL", "--output=NUL", "-w", "%{http_code}", "/dev/nullx", "http://x"}
	if got := mapNullDevice("windows", in); !reflect.DeepEqual(got, want) {
		t.Fatalf("windows: got %v want %v", got, want)
	}
	if got := mapNullDevice("linux", in); !reflect.DeepEqual(got, in) {
		t.Fatalf("linux must be unchanged: got %v", got)
	}
}
