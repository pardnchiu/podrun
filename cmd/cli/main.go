package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	if err := checkRelyPackages(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	args, err := parseCommand()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	slog.Info("parsed args", slog.Any("args", args))
}

func checkRelyPackages() error {
	relyPackages := []string{"sshpass", "rsync", "ssh", "curl", "unzip"}
	var missPackages []string

	for _, e := range relyPackages {
		if _, err := exec.LookPath(e); err != nil {
			missPackages = append(missPackages, e)
		}
	}

	if len(missPackages) == 0 {
		return nil
	}

	fmt.Println("[-] missing packages:", strings.Join(missPackages, ", "))

	goos := runtime.GOOS
	if goos != "linux" && goos != "darwin" {
		return fmt.Errorf("[x] only support RHEL / Debian and macOS")
	}

	fmt.Println("──────────────────────────────────────────────────")

	switch goos {
	case "darwin":
		if _, err := exec.LookPath("brew"); err != nil {
			fmt.Println("[-] installing brew")
			if err := exec.Command("bash", "-c", "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)").Run(); err != nil {
				return fmt.Errorf("[x] failed to install brew: %s", err)
			}
		}
		args := append([]string{"install"}, missPackages...)
		if err := installPackage("brew", args...); err != nil {
			return fmt.Errorf("[x] failed to install packages: %s", err)
		}

	case "linux":
		pkgManagers := []struct {
			name string
			args []string
		}{
			{"apt", append([]string{"install", "-y"}, missPackages...)},
			{"dnf", append([]string{"install", "-y"}, missPackages...)},
			{"yum", append([]string{"install", "-y"}, missPackages...)},
			{"pacman", append([]string{"-S", "--noconfirm"}, missPackages...)},
		}

		installed := false
		for _, pm := range pkgManagers {
			if _, err := exec.LookPath(pm.name); err == nil {
				if err := commandExec(pm.name, pm.args...); err != nil {
					return fmt.Errorf("[x] failed to install packages %s", err)
				}
				installed = true
				break
			}
		}
		if !installed {
			return fmt.Errorf("[x] no supported package manager exists")
		}
	}

	fmt.Println("──────────────────────────────────────────────────")
	return nil
}

func commandExec(main string, args ...string) error {
	cmd := exec.Command(main, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func installPackage(pkg string, args ...string) error {
	if _, err := exec.LookPath(pkg); err != nil {
		return err
	}

	err := commandExec(pkg, args...)
	if err != nil {
		return err
	}
	return nil
}

func parseCommand() (*PodmanArg, error) {
	if len(os.Args) < 2 {
		return nil, fmt.Errorf("[x] podrun <command> [args...]")
	}

	command := os.Args[1]
	switch command {
	case "info":
		fmt.Println("show project info")
	case "export":
		fmt.Println("export project to pod manifest")
	case "deploy":
		fmt.Println("deploy project to kubernetes")
	case "clone":
		fmt.Println("clone project to local")
	case "domain":
		fmt.Println("set domain to pod")
	}

	args, err := parseArgs(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("[x] %v", err)
	}

	if len(args.RemoteArgs) == 0 {
		return nil, fmt.Errorf("[x] please ensure docker compose <command> [args...] is valid first before running podrun")
	}

	localDir, err := getLocalDir(args.LocalDir)
	if err != nil {
		return nil, fmt.Errorf("[x] %v", err)
	}
	args.LocalDir = localDir

	remoteDir, err := setRemoteDir(localDir)
	if err != nil {
		return nil, fmt.Errorf("[x] %v", err)
	}
	args.RemoteDir = remoteDir

	return args, nil
}

type PodmanArg struct {
	UID        string
	LocalDir   string
	RemoteDir  string
	RemoteArgs []string
	Target     string
	File       string
}

func parseArgs(args []string) (*PodmanArg, error) {
	newArg := &PodmanArg{Target: "pod"}
	conposeExist := false

	for i := 0; i < len(args); {
		arg := args[i]
		switch {
		case arg == "-u" && i+1 < len(args):
			newArg.UID = args[i+1]
			i += 2
		case strings.HasPrefix(arg, "-u="):
			newArg.UID = strings.TrimPrefix(arg, "-u=")
			i++
		case strings.HasPrefix(arg, "--folder="):
			newArg.LocalDir = strings.TrimPrefix(arg, "--folder=")
			i++
		case arg == "--folder" && i+1 < len(args):
			newArg.LocalDir = args[i+1]
			i += 2
		case strings.HasPrefix(arg, "--type="):
			newArg.Target = strings.TrimPrefix(arg, "--type=")
			i++
		case arg == "--type" && i+1 < len(args):
			newArg.Target = args[i+1]
			i += 2
		case strings.HasPrefix(arg, "--output="):
			newArg.RemoteDir = strings.TrimPrefix(arg, "--output=")
			i++
		case arg == "--output" && i+1 < len(args):
			newArg.RemoteDir = args[i+1]
			i += 2
		case arg == "-o" && i+1 < len(args):
			newArg.RemoteDir = args[i+1]
			i += 2
		case arg == "-f" && i+1 < len(args):
			if conposeExist {
				return nil, fmt.Errorf("not supported multiple files")
			}
			conposeExist = true
			newArg.File = args[i+1]
			if newArg.LocalDir == "" {
				if dir := filepath.Dir(args[i+1]); dir != "." {
					newArg.LocalDir = dir
				}
			}
			newArg.RemoteArgs = append(newArg.RemoteArgs, "-f", filepath.Base(args[i+1]))
			i += 2
		case (strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "/")) && isDir(arg):
			if newArg.LocalDir == "" {
				newArg.LocalDir = arg
			}
			i++
		default:
			newArg.RemoteArgs = append(newArg.RemoteArgs, arg)
			i++
		}
	}
	return newArg, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func getLocalDir(folder string) (string, error) {
	if folder == "" {
		return os.Getwd()
	}
	absPath, _ := filepath.Abs(folder)
	if !isDir(absPath) {
		return "", fmt.Errorf("%s: %s", "folder is not exist", absPath)
	}
	if !fileExists(filepath.Join(absPath, "docker-compose.yml")) &&
		!fileExists(filepath.Join(absPath, "docker-compose.yaml")) {
		return "", fmt.Errorf("%s: %s", "docker-compose.yml not found in folder", absPath)
	}
	return absPath, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func setRemoteDir(localFolder string) (string, error) {
	mac, err := getMAC()
	if err != nil {
		mac, _ = os.Hostname()
	}
	hash := md5.Sum(fmt.Appendf(nil, "%s@%s", mac, localFolder))
	return filepath.Join("/home/podrun", fmt.Sprintf("%s_%s", filepath.Base(localFolder), hex.EncodeToString(hash[:])[:8])), nil
}

func getMAC() (string, error) {
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
