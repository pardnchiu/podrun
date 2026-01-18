package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/pardnchiu/go-podrun/internal/command"
	"github.com/pardnchiu/go-podrun/internal/utils"
)

func main() {
	if err := utils.CheckRelyPackages(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	parseCmd, err := command.New()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := command.CheckSSHConnection(); err != nil {
		slog.Error("failed to connect to remote server",
			"err", err)
		os.Exit(1)
	}

	switch parseCmd.RemoteArgs[0] {
	case "domain":
	case "deploy":
	default:
		uid, err := parseCmd.ComposeCMD()
		if err != nil {
			slog.Error("failed to connect to remote server",
				"err", err)
		}
		slog.Info(uid)
		// case "rm":
		// case "ports":
		// case "export":
	}
}
