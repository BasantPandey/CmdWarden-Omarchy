package ghauth

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/secretservice"
)

// liveKeyring is the process-local Secret Service session Import stores into.
type liveKeyring struct {
	c    *secretservice.Client
	conn *dbus.Conn // non-nil when this keyring owns the connection
}

// OpenKeyring connects to the session bus and opens a Secret Service
// session. A bus or service that is not there is ErrKeyringMissing.
func OpenKeyring() (Keyring, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyringMissing, err)
	}
	c, err := secretservice.Open(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrKeyringMissing, err)
	}
	return &liveKeyring{c: c, conn: conn}, nil
}

// NewLiveKeyring wraps a Secret Service client the caller owns. Close does
// not close the caller's connection.
func NewLiveKeyring(c *secretservice.Client) Keyring {
	return &liveKeyring{c: c}
}

func (k *liveKeyring) Close() error {
	if k != nil && k.conn != nil {
		return k.conn.Close()
	}
	return nil
}

func (k *liveKeyring) Hold() error {
	if k == nil || k.c == nil {
		return fmt.Errorf("%w", ErrKeyringMissing)
	}
	return mapHold(k.c.ProbeHold())
}

func mapHold(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, secretservice.ErrServiceMissing):
		return fmt.Errorf("%w: %v", ErrKeyringMissing, err)
	case errors.Is(err, secretservice.ErrServiceLocked):
		return fmt.Errorf("%w: %v", ErrKeyringLocked, err)
	case errors.Is(err, secretservice.ErrServiceWrongWallet):
		return fmt.Errorf("%w: %v", ErrKeyringWrongWallet, err)
	default:
		return fmt.Errorf("%w: %v", ErrKeyringMissing, err)
	}
}

func (k *liveKeyring) Save(name, value string) error {
	return k.c.SaveNamed(name, value)
}

func (k *liveKeyring) GHToken(host string) (string, string, error) {
	matches, err := k.c.Search(map[string]string{"service": KeyringServiceName(host)})
	if err != nil {
		return "", "", err
	}
	if len(matches) == 0 {
		return "", "", nil
	}

	activeUser, _ := ActiveUser(host)

	var fallback dbus.ObjectPath
	var fallbackUser string
	for _, m := range matches {
		attrs, attrErr := k.c.ItemAttributes(m)
		if attrErr != nil {
			continue
		}
		user := attrs["username"]
		if activeUser != "" && user == activeUser {
			value, gerr := k.c.GetSecretValue(m)
			if gerr != nil {
				return "", "", gerr
			}
			return string(value), user, nil
		}
		if fallback == "" || (fallbackUser == "" && user != "") {
			fallback = m
			fallbackUser = user
		}
	}
	if fallback == "" {
		return "", "", nil
	}
	value, err := k.c.GetSecretValue(fallback)
	if err != nil {
		return "", "", err
	}
	return string(value), fallbackUser, nil
}
