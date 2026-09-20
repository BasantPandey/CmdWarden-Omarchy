// Package secretservice is a minimal client for the freedesktop.org Secret
// Service D-Bus API (org.freedesktop.secrets), used by the agent both to
// hold CmdWarden-Omarchy's own vault collection and to look up gh's own
// stored token (the same store gh's zalando/go-keyring dependency writes
// to). It talks to whatever already owns org.freedesktop.secrets on the
// session bus (gnome-keyring-daemon, KWallet's ksecretservice, ...) — it
// never starts or manages that service itself.
//
// It uses the Secret Service spec's "plain" algorithm (no DH/AES
// negotiation): the session bus is already a private, per-user,
// kernel-authenticated transport, so the extra encryption layer the spec
// offers for less trusted transports buys nothing here — this mirrors what
// gh's own dependency (zalando/go-keyring) does.
package secretservice

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	serviceName = "org.freedesktop.secrets"
	servicePath = dbus.ObjectPath("/org/freedesktop/secrets")

	ifaceService    = "org.freedesktop.Secret.Service"
	ifaceCollection = "org.freedesktop.Secret.Collection"
	ifaceItem       = "org.freedesktop.Secret.Item"
	ifacePrompt     = "org.freedesktop.Secret.Prompt"

	nullObjectPath = dbus.ObjectPath("/")

	promptTimeout = 60 * time.Second
)

// secretStruct mirrors the Secret Service spec's Secret struct, wire
// signature "(oayays)": session path, algorithm parameters, the secret
// value itself, and its content type.
type secretStruct struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// Client is a connected Secret Service session. Callers should keep it
// around for the agent's lifetime rather than reopening a session per call.
type Client struct {
	conn    *dbus.Conn
	session dbus.ObjectPath
}

// Open negotiates a "plain" algorithm session against the running Secret
// Service provider.
func Open(conn *dbus.Conn) (*Client, error) {
	svc := conn.Object(serviceName, servicePath)

	var (
		output  dbus.Variant
		session dbus.ObjectPath
	)
	call := svc.Call(ifaceService+".OpenSession", 0, "plain", dbus.MakeVariant(""))
	if call.Err != nil {
		return nil, fmt.Errorf("secretservice: OpenSession: %w", call.Err)
	}
	if err := call.Store(&output, &session); err != nil {
		return nil, fmt.Errorf("secretservice: decoding OpenSession reply: %w", err)
	}

	return &Client{conn: conn, session: session}, nil
}

// EnsureCollection returns the object path of a collection labeled label,
// creating it (unlocked, under the default login keyring backend) if none
// exists yet.
func (c *Client) EnsureCollection(label string) (dbus.ObjectPath, error) {
	svc := c.conn.Object(serviceName, servicePath)

	var collections []dbus.ObjectPath
	variant, err := svc.GetProperty(ifaceService + ".Collections")
	if err != nil {
		return "", fmt.Errorf("secretservice: reading Collections property: %w", err)
	}
	if err := variant.Store(&collections); err != nil {
		return "", fmt.Errorf("secretservice: decoding Collections property: %w", err)
	}

	for _, path := range collections {
		collLabel, err := c.collectionLabel(path)
		if err != nil {
			continue // a collection we can't introspect just isn't a match
		}
		if collLabel == label {
			return path, nil
		}
	}

	if collection, err := c.tryCreateCollection(label); err == nil {
		return collection, nil
	}

	// No dedicated-collection UI available (e.g. no Secret Service
	// prompter registered in this session — common headless/minimal
	// desktop setups). Fall back to whatever collection the backend
	// itself treats as default; our items still keep their own
	// identifying attributes, so nothing else is affected by sharing it.
	var defaultCollection dbus.ObjectPath
	call := svc.Call(ifaceService+".ReadAlias", 0, "default")
	if call.Err != nil {
		return "", fmt.Errorf("secretservice: could not create a %q collection and ReadAlias(default) failed: %w", label, call.Err)
	}
	if err := call.Store(&defaultCollection); err != nil {
		return "", fmt.Errorf("secretservice: decoding ReadAlias reply: %w", err)
	}
	if defaultCollection == nullObjectPath {
		return "", fmt.Errorf("secretservice: could not create a %q collection and no default collection alias is set", label)
	}
	return defaultCollection, nil
}

// createCollectionPromptTimeout is deliberately short: CreateCollection
// only needs a prompt when the backend wants to set a password on a brand
// new collection, which requires an interactive prompter UI. Where none is
// registered (see EnsureCollection's fallback), failing fast matters more
// than waiting out the full promptTimeout used for prompts we expect to
// actually complete (e.g. unlocking an existing collection).
const createCollectionPromptTimeout = 3 * time.Second

func (c *Client) tryCreateCollection(label string) (dbus.ObjectPath, error) {
	svc := c.conn.Object(serviceName, servicePath)

	props := map[string]dbus.Variant{
		ifaceCollection + ".Label": dbus.MakeVariant(label),
	}
	var (
		collection dbus.ObjectPath
		prompt     dbus.ObjectPath
	)
	call := svc.Call(ifaceService+".CreateCollection", 0, props, "")
	if call.Err != nil {
		return "", fmt.Errorf("CreateCollection: %w", call.Err)
	}
	if err := call.Store(&collection, &prompt); err != nil {
		return "", fmt.Errorf("decoding CreateCollection reply: %w", err)
	}
	if prompt == nullObjectPath {
		return collection, nil
	}

	result, err := c.runPromptWithTimeout(prompt, createCollectionPromptTimeout)
	if err != nil {
		return "", fmt.Errorf("CreateCollection prompt: %w", err)
	}
	if err := result.Store(&collection); err != nil {
		return "", fmt.Errorf("decoding CreateCollection prompt result: %w", err)
	}
	return collection, nil
}

func (c *Client) collectionLabel(path dbus.ObjectPath) (string, error) {
	obj := c.conn.Object(serviceName, path)
	variant, err := obj.GetProperty(ifaceCollection + ".Label")
	if err != nil {
		return "", err
	}
	var label string
	if err := variant.Store(&label); err != nil {
		return "", err
	}
	return label, nil
}

// CreateOrReplaceItem creates (or, if an item with these exact attributes
// already exists, replaces) a secret item in collection.
func (c *Client) CreateOrReplaceItem(collection dbus.ObjectPath, label string, attributes map[string]string, value []byte) (dbus.ObjectPath, error) {
	obj := c.conn.Object(serviceName, collection)

	props := map[string]dbus.Variant{
		ifaceItem + ".Label":      dbus.MakeVariant(label),
		ifaceItem + ".Attributes": dbus.MakeVariant(attributes),
	}
	secret := secretStruct{Session: c.session, Parameters: []byte{}, Value: value, ContentType: "text/plain"}

	var (
		item   dbus.ObjectPath
		prompt dbus.ObjectPath
	)
	call := obj.Call(ifaceCollection+".CreateItem", 0, props, secret, true)
	if call.Err != nil {
		return "", fmt.Errorf("secretservice: CreateItem: %w", call.Err)
	}
	if err := call.Store(&item, &prompt); err != nil {
		return "", fmt.Errorf("secretservice: decoding CreateItem reply: %w", err)
	}
	if prompt != nullObjectPath {
		result, err := c.runPrompt(prompt)
		if err != nil {
			return "", fmt.Errorf("secretservice: CreateItem prompt: %w", err)
		}
		if err := result.Store(&item); err != nil {
			return "", fmt.Errorf("secretservice: decoding CreateItem prompt result: %w", err)
		}
	}
	return item, nil
}

// Search finds every item service-wide whose attributes are a superset of
// attributes, unlocking any that come back locked. Order is not guaranteed.
func (c *Client) Search(attributes map[string]string) ([]dbus.ObjectPath, error) {
	svc := c.conn.Object(serviceName, servicePath)

	var unlocked, locked []dbus.ObjectPath
	call := svc.Call(ifaceService+".SearchItems", 0, attributes)
	if call.Err != nil {
		return nil, fmt.Errorf("secretservice: SearchItems: %w", call.Err)
	}
	if err := call.Store(&unlocked, &locked); err != nil {
		return nil, fmt.Errorf("secretservice: decoding SearchItems reply: %w", err)
	}

	if len(locked) > 0 {
		newlyUnlocked, err := c.Unlock(locked)
		if err != nil {
			return nil, fmt.Errorf("secretservice: unlocking search results: %w", err)
		}
		unlocked = append(unlocked, newlyUnlocked...)
	}
	return unlocked, nil
}

// Unlock unlocks the given objects (items or collections), running an
// interactive prompt if the backend requires one.
func (c *Client) Unlock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, error) {
	svc := c.conn.Object(serviceName, servicePath)

	var (
		unlocked []dbus.ObjectPath
		prompt   dbus.ObjectPath
	)
	call := svc.Call(ifaceService+".Unlock", 0, objects)
	if call.Err != nil {
		return nil, fmt.Errorf("secretservice: Unlock: %w", call.Err)
	}
	if err := call.Store(&unlocked, &prompt); err != nil {
		return nil, fmt.Errorf("secretservice: decoding Unlock reply: %w", err)
	}
	if prompt == nullObjectPath {
		return unlocked, nil
	}

	result, err := c.runPrompt(prompt)
	if err != nil {
		return nil, fmt.Errorf("secretservice: Unlock prompt: %w", err)
	}
	var promptUnlocked []dbus.ObjectPath
	if err := result.Store(&promptUnlocked); err != nil {
		return nil, fmt.Errorf("secretservice: decoding Unlock prompt result: %w", err)
	}
	return append(unlocked, promptUnlocked...), nil
}

// GetSecretValue returns the raw secret bytes for a single item.
func (c *Client) GetSecretValue(item dbus.ObjectPath) ([]byte, error) {
	obj := c.conn.Object(serviceName, item)

	var secret secretStruct
	call := obj.Call(ifaceItem+".GetSecret", 0, c.session)
	if call.Err != nil {
		return nil, fmt.Errorf("secretservice: GetSecret: %w", call.Err)
	}
	if err := call.Store(&secret); err != nil {
		return nil, fmt.Errorf("secretservice: decoding GetSecret reply: %w", err)
	}
	return secret.Value, nil
}

// ItemAttributes returns an item's attribute map (e.g. to disambiguate
// multiple search matches by "username").
func (c *Client) ItemAttributes(item dbus.ObjectPath) (map[string]string, error) {
	obj := c.conn.Object(serviceName, item)
	variant, err := obj.GetProperty(ifaceItem + ".Attributes")
	if err != nil {
		return nil, fmt.Errorf("secretservice: reading Attributes property: %w", err)
	}
	var attrs map[string]string
	if err := variant.Store(&attrs); err != nil {
		return nil, fmt.Errorf("secretservice: decoding Attributes property: %w", err)
	}
	return attrs, nil
}

// DeleteItem permanently removes a secret item.
func (c *Client) DeleteItem(item dbus.ObjectPath) error {
	obj := c.conn.Object(serviceName, item)

	var prompt dbus.ObjectPath
	call := obj.Call(ifaceItem+".Delete", 0)
	if call.Err != nil {
		return fmt.Errorf("secretservice: Delete: %w", call.Err)
	}
	if err := call.Store(&prompt); err != nil {
		return fmt.Errorf("secretservice: decoding Delete reply: %w", err)
	}
	if prompt != nullObjectPath {
		if _, err := c.runPrompt(prompt); err != nil {
			return fmt.Errorf("secretservice: Delete prompt: %w", err)
		}
	}
	return nil
}

// runPrompt drives a Secret Service Prompt object to completion: calls
// Prompt() and waits for its Completed signal, up to promptTimeout.
func (c *Client) runPrompt(prompt dbus.ObjectPath) (*dbus.Variant, error) {
	return c.runPromptWithTimeout(prompt, promptTimeout)
}

func (c *Client) runPromptWithTimeout(prompt dbus.ObjectPath, timeout time.Duration) (*dbus.Variant, error) {
	signalCh := make(chan *dbus.Signal, 1)
	c.conn.Signal(signalCh)
	defer c.conn.RemoveSignal(signalCh)

	matchOpts := []dbus.MatchOption{
		dbus.WithMatchInterface(ifacePrompt),
		dbus.WithMatchMember("Completed"),
		dbus.WithMatchObjectPath(prompt),
	}
	if err := c.conn.AddMatchSignal(matchOpts...); err != nil {
		return nil, fmt.Errorf("watching for Prompt Completed signal: %w", err)
	}
	defer c.conn.RemoveMatchSignal(matchOpts...)

	obj := c.conn.Object(serviceName, prompt)
	if call := obj.Call(ifacePrompt+".Prompt", 0, ""); call.Err != nil {
		return nil, fmt.Errorf("Prompt: %w", call.Err)
	}

	select {
	case sig := <-signalCh:
		var dismissed bool
		var result dbus.Variant
		if err := dbus.Store(sig.Body, &dismissed, &result); err != nil {
			return nil, fmt.Errorf("decoding Completed signal: %w", err)
		}
		if dismissed {
			return nil, fmt.Errorf("prompt was dismissed")
		}
		return &result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out after %s waiting for prompt", timeout)
	}
}
