package agentclient

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/xdgpaths"
)

const socketUnit = `[Unit]
Description=CmdWarden-Omarchy Session Agent activation socket

[Socket]
ListenStream=%t/` + contracts.SocketFileName + `

[Install]
WantedBy=sockets.target
`

const serviceUnitTemplate = `[Unit]
Description=CmdWarden-Omarchy Session Agent
Requires=%s

[Service]
Type=notify
ExecStart=%s
`

// InstallUnits writes cmdwarden-agent.socket/.service into
// ~/.config/systemd/user, reloads the systemd --user manager, and enables +
// starts the socket unit (not the service — the whole point is that the
// service starts lazily on first connection).
func InstallUnits() error {
	agentPath, err := resolveAgentBinary()
	if err != nil {
		return err
	}

	unitDir, err := xdgpaths.UserSystemdUnitDir()
	if err != nil {
		return fmt.Errorf("agentclient: %w", err)
	}

	socketPath := filepath.Join(unitDir, contracts.SocketUnitName)
	servicePath := filepath.Join(unitDir, contracts.ServiceUnitName)

	if err := os.WriteFile(socketPath, []byte(socketUnit), 0o644); err != nil {
		return fmt.Errorf("agentclient: writing %s: %w", socketPath, err)
	}
	serviceUnit := fmt.Sprintf(serviceUnitTemplate, contracts.SocketUnitName, agentPath)
	if err := os.WriteFile(servicePath, []byte(serviceUnit), 0o644); err != nil {
		return fmt.Errorf("agentclient: writing %s: %w", servicePath, err)
	}

	if err := runSystemctl("daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl("enable", "--now", contracts.SocketUnitName); err != nil {
		return err
	}
	return nil
}

// UninstallUnits reverses InstallUnits: stops and disables the socket unit,
// stops the service if it happens to be running, and removes both unit
// files.
func UninstallUnits() error {
	_ = runSystemctl("disable", "--now", contracts.SocketUnitName)
	_ = runSystemctl("stop", contracts.ServiceUnitName)

	unitDir, err := xdgpaths.UserSystemdUnitDir()
	if err != nil {
		return fmt.Errorf("agentclient: %w", err)
	}
	for _, name := range []string{contracts.SocketUnitName, contracts.ServiceUnitName} {
		path := filepath.Join(unitDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("agentclient: removing %s: %w", path, err)
		}
	}
	return runSystemctl("daemon-reload")
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("agentclient: systemctl --user %v: %w", args, err)
	}
	return nil
}

// resolveAgentBinary finds the cmdwarden-agent binary to point ExecStart at:
// first next to the currently running cw executable (the normal case when
// both are built into the same bin/ directory), then on PATH.
func resolveAgentBinary() (string, error) {
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), "cmdwarden-agent")
		if info, statErr := os.Stat(sibling); statErr == nil && !info.IsDir() {
			return sibling, nil
		}
	}
	if path, err := exec.LookPath("cmdwarden-agent"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("agentclient: could not find cmdwarden-agent binary next to cw or on PATH")
}
