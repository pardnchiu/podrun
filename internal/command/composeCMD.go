package command

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/pardnchiu/go-podrun/internal/utils"
)

type Deploy struct {
	UID       string
	PodID     string
	PodName   string
	LocalDir  string
	RemoteDir string
	File      string
	Target    string
	Namespace string
	Status    string
	Creator   string
	Replicas  int       // TODO: for k3s usage
	CreatedAt time.Time // TODO: for record to database
	UpdatedAt time.Time // TODO: for record to database
}

func (p *PodmanArg) ComposeCMD() (string, error) {
	d := &Deploy{
		UID:       p.UID,
		PodID:     filepath.Base(p.RemoteDir),
		PodName:   filepath.Base(p.RemoteDir),
		LocalDir:  p.LocalDir,
		RemoteDir: p.RemoteDir,
		File:      "docker-compose.yml",
		Target:    p.Target,
		Status:    "starting",
		Creator:   utils.GetHostName(),
		Replicas:  1,
	}
	slog.Info("up",
		slog.Any("arg", p))

	switch p.Command {
	case "up":
		return p.up(d)
	case "down":
		return p.down(d)
	case "ps":
	case "logs":
	case "restart":
	case "exec":
	case "build":
	}
	return p.UID, nil
}

func (p *PodmanArg) up(d *Deploy) (string, error) {
	slog.Info("Deploying",
		slog.Any("deploy", d),
		slog.Any("arg", p))

	hasDetach := false
	for _, arg := range p.RemoteArgs {
		if arg == "-d" || arg == "--detach" {
			hasDetach = true
			break
		}
	}

	fmt.Println("[*] create folder if not exist")
	if err := utils.SSHRun("mkdir", "-p", p.RemoteDir); err != nil {
		return "", err
	}

	fmt.Println("[*] syncing files")
	fmt.Println("──────────────────────────────────────────────────")
	if err := p.RsyncToRemote(); err != nil {
		return "", err
	}
	fmt.Println("──────────────────────────────────────────────────")

	fmt.Println("[*] modifying compose file (remove ports)")
	if err := p.ModifyComposeFile(); err != nil {
		return "", fmt.Errorf("failed to modify compose file: %w", err)
	}

	fmt.Println("[*] cleaning up old containers")
	// fmt.Println("──────────────────────────────────────────────────")
	if err := utils.SSHRun(fmt.Sprintf("cd '%s' && podman compose down 2>/dev/null || true", p.RemoteDir)); err != nil {
		slog.Warn("failed to clean up old containers", "err", err)
	}
	// fmt.Println("──────────────────────────────────────────────────")

	fmt.Printf("[*] executing: podman compose %s\n", strings.Join(p.RemoteArgs, " "))
	fmt.Println("──────────────────────────────────────────────────")
	remoteCmd := fmt.Sprintf("cd '%s' && podman compose %s", p.RemoteDir, shellJoin(p.RemoteArgs))
	if !hasDetach {
		remoteCmd = fmt.Sprintf(`
				cleanup() {
					echo "[*] stopping containers"
					cd '%s' && podman compose down
				}
				trap cleanup INT TERM
				%s
			`, p.RemoteDir, remoteCmd)
	}

	err := utils.SSHRun(remoteCmd)
	if err != nil {
		return "", err
	}
	fmt.Println("──────────────────────────────────────────────────")

	if hasDetach {
		fmt.Println("[*] service ports:")
		fmt.Println("──────────────────────────────────────────────────")
		output, _ := utils.SSEOutput(fmt.Sprintf("cd '%s' && podman ps --filter 'label=io.podman.compose.project=%s' --format 'table {{.Names}}\t{{.Ports}}'",
			p.RemoteDir,
			filepath.Base(p.RemoteDir)))
		fmt.Println(output)
		fmt.Println("──────────────────────────────────────────────────")
	}
	return p.UID, nil
}

func (p *PodmanArg) down(d *Deploy) (string, error) {
	slog.Info("down",
		slog.Any("arg", p))

	fmt.Printf("[*] executing: podman compose %s\n", strings.Join(p.RemoteArgs, " "))
	fmt.Println("──────────────────────────────────────────────────")
	remoteCmd := fmt.Sprintf("cd '%s' && podman compose %s", p.RemoteDir, shellJoin(p.RemoteArgs))

	err := utils.SSHRun(remoteCmd)
	if err != nil {
		return "", err
	}
	fmt.Println("──────────────────────────────────────────────────")
	return p.UID, nil
}

func (p *PodmanArg) RsyncToRemote() error {
	cmdArgs := []string{
		"-p", Password,
		"rsync",
		"-avz", "--delete",
		"--exclude=node_modules/", "--exclude=vendor/", "--exclude=__pycache__/",
		"--exclude=*.pyc", "--exclude=.venv/", "--exclude=venv/", "--exclude=env/",
		"--exclude=.env.local", "--exclude=.git/", "--exclude=.gitignore",
		"--exclude=*.log", "--exclude=.DS_Store", "--exclude=Thumbs.db",
		"-e", "ssh -o StrictHostKeyChecking=no",
		p.LocalDir + "/",
		fmt.Sprintf("%s:%s/", RemoteServer, p.RemoteDir),
	}
	return utils.CMDRun("sshpass", cmdArgs...)
}

func (p *PodmanArg) ModifyComposeFile() error {
	composeFile := "docker-compose.yml"
	output, _ := utils.SSEOutput("test -f '%s/%s' || echo 'notfound'", p.RemoteDir, composeFile)
	if strings.TrimSpace(output) == "notfound" {
		composeFile = "docker-compose.yaml"
	}

	// 移除 ports
	sedCmds := []string{
		`sed -i -E 's/(["\x27]?)[0-9]+:([0-9]+)(["\x27]?)/\1\2\3/g' '%s/%s'`,
		`sed -i -E 's/(["\x27]?)\$\{[^}]+\}:([0-9]+)(["\x27]?)/\1\2\3/g' '%s/%s'`,
		`sed -i -E 's/\$\{[^}]+:[?][^}]+\}://g' '%s/%s'`,
	}

	for _, cmdTemplate := range sedCmds {
		cmd := fmt.Sprintf(cmdTemplate, p.RemoteDir, composeFile)
		if err := utils.SSHRun(cmd); err != nil {
			return err
		}
	}

	// 強制為所有相對路徑 volume 加入 :z（如果沒有）
	awkCmd := fmt.Sprintf(`
		awk '
		/^\s+- \.\/[^:]+:[^:]+$/ { print $0 ":z"; next }
		/^\s+- \.\/[^:]+:[^:]+:[^z]*$/ { gsub(/:([^z:]+)$/, ":\\1,z"); print; next }
		/^\s+- \.\/[^:]+:[^:]+:.*z/ { print; next }
		{ print }
		' '%s/%s' > '%s/%s.tmp' && mv '%s/%s.tmp' '%s/%s'
	`, p.RemoteDir, composeFile, p.RemoteDir, composeFile, p.RemoteDir, composeFile, p.RemoteDir, composeFile)

	return utils.SSHRun(awkCmd)
}

func shellJoin(args []string) string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t\n'\"\\$`") {
			escaped[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		} else {
			escaped[i] = arg
		}
	}
	return strings.Join(escaped, " ")
}
