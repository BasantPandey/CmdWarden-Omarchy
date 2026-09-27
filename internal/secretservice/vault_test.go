package secretservice

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestSelectGHWalletPrefersLogin(t *testing.T) {
	other := dbus.ObjectPath("/org/freedesktop/secrets/collection/other")
	login := loginCollectionPath

	path, ok := selectGHWallet([]dbus.ObjectPath{other, login})
	if !ok || path != login {
		t.Fatalf("selectGHWallet = (%s, %v), want login", path, ok)
	}

	// Login present means the default alias is not consulted. An unset
	// alias must not turn this into a wrong wallet.
	if _, ok := selectGHWallet([]dbus.ObjectPath{login}); !ok {
		t.Fatal("login collection was ignored")
	}

	if _, ok := selectGHWallet([]dbus.ObjectPath{other}); ok {
		t.Fatal("a non-login collection was treated as gh's login wallet")
	}
	if _, ok := selectGHWallet(nil); ok {
		t.Fatal("empty collection list selected a wallet")
	}
}

func TestClassifyWalletUnlock(t *testing.T) {
	login := loginCollectionPath
	other := dbus.ObjectPath("/org/freedesktop/secrets/collection/other")

	t.Run("login unlocks", func(t *testing.T) {
		if err := classifyWalletUnlock(true, login, []dbus.ObjectPath{login}, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("locked login is not accepted via another collection", func(t *testing.T) {
		err := classifyWalletUnlock(true, login, nil, errors.New("prompt was dismissed"))
		if !errors.Is(err, ErrServiceLocked) {
			t.Fatalf("err = %v, want locked", err)
		}
		err = classifyWalletUnlock(true, login, []dbus.ObjectPath{other}, nil)
		if !errors.Is(err, ErrServiceWrongWallet) {
			t.Fatalf("err = %v, want wrong wallet", err)
		}
	})

	t.Run("default alias unlocks when login is absent", func(t *testing.T) {
		alias := dbus.ObjectPath("/org/freedesktop/secrets/collection/default")
		if err := classifyWalletUnlock(false, alias, []dbus.ObjectPath{alias}, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("missing default alias is the wrong wallet", func(t *testing.T) {
		_, err := walletWithoutLogin(nullObjectPath, nil)
		if !errors.Is(err, ErrServiceWrongWallet) {
			t.Fatalf("err = %v, want wrong wallet", err)
		}
		alias := dbus.ObjectPath("/org/freedesktop/secrets/collection/default")
		got, err := walletWithoutLogin(alias, nil)
		if err != nil || got != alias {
			t.Fatalf("walletWithoutLogin = (%s, %v), want the default collection", got, err)
		}
	})
}
