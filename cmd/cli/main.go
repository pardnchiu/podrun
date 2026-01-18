package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/pardnchiu/go-podrun/internal/command"
	"github.com/pardnchiu/go-podrun/internal/utils"
)

func init() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("Error loading .env file",
			slog.String("error", err.Error()))
	}
}

func main() {
	if err := utils.CheckRelyPackages(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_, err := utils.GetENV()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cmd, err := command.New()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := utils.SSHTest(); err != nil {
		slog.Error("failed to connect to remote server",
			"err", err)
		os.Exit(1)
	}

	switch cmd.RemoteArgs[0] {
	case "domain":
	case "deploy":
	default:
		_, err := cmd.ComposeCMD()
		if err != nil {
			slog.Error("failed to connect to remote server",
				"err", err)
		}
		// case "rm":
		// case "ports":
		// case "export":
	}
}
