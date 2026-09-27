package secretservice

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestClassifyProbe(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		alias      dbus.ObjectPath
		aliasErr   error
		unlockErr  error
		want       error
	}{
		{name: "ok", alias: "/org/freedesktop/secrets/collection/login"},
		{name: "missing", serviceErr: errors.New("no name owner"), want: ErrServiceMissing},
		{name: "locked", alias: "/org/freedesktop/secrets/collection/login", unlockErr: errors.New("prompt was dismissed"), want: ErrServiceLocked},
		{name: "no default collection", alias: nullObjectPath, want: ErrServiceWrongWallet},
		{name: "alias lookup failed", aliasErr: errors.New("ReadAlias failed"), want: ErrServiceWrongWallet},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyProbe(tc.serviceErr, tc.alias, tc.aliasErr, tc.unlockErr)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("classifyProbe = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("classifyProbe = %v, want %v", err, tc.want)
			}
		})
	}
}
