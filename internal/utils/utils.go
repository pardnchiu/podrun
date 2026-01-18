package utils

import (
	"fmt"
	"net"
	"os"
	"strings"
)

func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func FileExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func GetMAC() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, e := range ifaces {
		if e.Flags&net.FlagLoopback == 0 && len(e.HardwareAddr) > 0 {
			return e.HardwareAddr.String(), nil
		}
	}
	return "", fmt.Errorf("no valid MAC address found")
}

type Podrun struct {
	Server   string
	Username string
	Password string
	Remote   string
}

func GetENV() (*Podrun, error) {
	server := os.Getenv("PODRUN_SERVER")
	username := os.Getenv("PODRUN_USERNAME")
	password := os.Getenv("PODRUN_PASSWORD")
	if server == "" || username == "" || password == "" {
		var missing []string
		if server == "" {
			missing = append(missing, "PODRUN_SERVER")
		}
		if username == "" {
			missing = append(missing, "PODRUN_USERNAME")
		}
		if password == "" {
			missing = append(missing, "PODRUN_PASSWORD")
		}
		return nil, fmt.Errorf("missing required environment: %s", strings.Join(missing, ", "))
	}
	return &Podrun{
		Server:   server,
		Username: username,
		Password: password,
		Remote:   fmt.Sprintf("%s@%s", username, server),
	}, nil
}

func GetHostName() string {
	if host, err := os.Hostname(); err == nil {
		return host
	}
	return "unknown"
}
