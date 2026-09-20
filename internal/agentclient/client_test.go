package agentclient

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestIsNoOwnerError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"service unknown", dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}, true},
		{"name has no owner", dbus.Error{Name: "org.freedesktop.DBus.Error.NameHasNoOwner"}, true},
		{"unrelated dbus error", dbus.Error{Name: "org.freedesktop.DBus.Error.Failed"}, false},
		{"non-dbus error", errors.New("boom"), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNoOwnerError(tc.err); got != tc.want {
				t.Errorf("isNoOwnerError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
