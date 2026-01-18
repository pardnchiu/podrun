package utils

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func CheckRelyPackages() error {
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

		if _, err := exec.LookPath("brew"); err != nil {
			return err
		}
		err := CMDRun("brew", args...)
		if err != nil {
			return err
		}
		return nil

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
				if err := CMDRun(pm.name, pm.args...); err != nil {
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
