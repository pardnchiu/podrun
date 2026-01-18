package command

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pardnchiu/go-podrun/internal/utils"
)

type PodmanArg struct {
	UID        string
	LocalDir   string
	RemoteDir  string
	Command    string
	RemoteArgs []string
	Target     string
	File       string
}

func New() (*PodmanArg, error) {
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

	uid, remoteDir, err := setRemoteDir(localDir)
	if err != nil {
		return nil, fmt.Errorf("[x] %v", err)
	}
	args.RemoteDir = remoteDir

	if args.UID == "" {
		args.UID = uid
	}

	return args, nil
}

func parseArgs(args []string) (*PodmanArg, error) {
	newArg := &PodmanArg{Target: "podman"}
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
		case (strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "/")) && utils.IsDir(arg):
			if newArg.LocalDir == "" {
				newArg.LocalDir = arg
			}
			i++
		default:
			newArg.RemoteArgs = append(newArg.RemoteArgs, arg)
			i++
		}
	}

	if len(newArg.RemoteArgs) > 0 {
		newArg.Command = newArg.RemoteArgs[0]
	}

	return newArg, nil
}

func getLocalDir(folder string) (string, error) {
	var err error
	newFolder := folder
	if newFolder == "" {
		newFolder, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	absPath, err := filepath.Abs(newFolder)
	if err != nil {
		return "", err
	}
	if !utils.IsDir(absPath) {
		return "", fmt.Errorf("folder does not exist: %s", absPath)
	}
	if !utils.FileExist(filepath.Join(absPath, "docker-compose.yml")) &&
		!utils.FileExist(filepath.Join(absPath, "docker-compose.yaml")) {
		return "", fmt.Errorf("docker-compose.yml not found in folder: %s", absPath)
	}
	return absPath, nil
}

func setRemoteDir(localFolder string) (string, string, error) {
	mac, err := utils.GetMAC()
	if err != nil {
		mac, _ = os.Hostname()
	}
	hash := md5.Sum(fmt.Appendf(nil, "%s@%s", mac, localFolder))
	return hex.EncodeToString(hash[:]),
		filepath.Join("/home/podrun",
			fmt.Sprintf("%s_%s", filepath.Base(localFolder), hex.EncodeToString(hash[:])[:8])),
		nil
}
