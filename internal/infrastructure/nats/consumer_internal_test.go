package nats

import (
	"errors"
	"testing"
)

func TestIsPermanentError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "validation marker", err: errors.New("validation: bad date"), want: true},
		{name: "malformed marker", err: errors.New("malformed payload"), want: true},
		{name: "invalid_format marker", err: errors.New("invalid_format x"), want: true},
		{name: "transient timeout", err: errors.New("context timeout"), want: false},
		{name: "random error", err: errors.New("boom"), want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isPermanentError(tc.err); got != tc.want {
				t.Fatalf("isPermanentError(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
