package command

import (
	"fmt"
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

const (
	Reset = "\033[0m"
	Hint  = "\033[90m"
	Ok    = "\033[32m"
	Error = "\033[31m"
	Warn  = "\033[33m"
)

func (p *PodmanArg) up(d *Deploy) (string, error) {
	fmt.Println("[+] create folder if not exist")
	if err := utils.SSHRun("mkdir", "-p", p.RemoteDir); err != nil {
		return "", err
	}

	// * 同步檔案夾資料
	fmt.Println("[*] syncing files")
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	if err := p.RsyncToRemote(); err != nil {
		return "", err
	}
	fmt.Println("──────────────────────────────────────────────────" + Reset)

	// * 調整 docker-compose.yml 內容
	fmt.Println("[*] modifying compose file (remove ports)")
	if err := p.ModifyComposeFile(); err != nil {
		return "", fmt.Errorf("[x] failed to modify compose file: %w", err)
	}

	// * 關閉舊的容器 (if exists)
	fmt.Println("[*] cleaning up old containers")
	_, _ = utils.SSEOutput(fmt.Sprintf(
		"cd '%s' && podman compose down -v >/dev/null 2>&1",
		p.RemoteDir,
	))

	// * 執行動作
	fmt.Printf("[*] executing: podman compose %s\n", strings.Join(p.RemoteArgs, " "))
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	remoteCmd := fmt.Sprintf("cd '%s' && podman compose %s 2>&1", p.RemoteDir, shellJoin(p.RemoteArgs))
	if !p.Detach {
		remoteCmd = fmt.Sprintf(`
				cleanup() {
					echo "[*] stopping containers"
					cd '%s' && podman compose down
				}
				trap cleanup INT TERM
				%s
			`, p.RemoteDir, remoteCmd)
	}
	if err := utils.SSHRun(remoteCmd); err != nil {
		return "", err
	}
	fmt.Println(Hint + "──────────────────────────────────────────────────" + Reset)

	// * 輸出結果
	if p.Detach {
		fmt.Println("[*] service ports:")
		fmt.Println(Ok + "──────────────────────────────────────────────────")
		output, _ := utils.SSEOutput(fmt.Sprintf(
			"cd '%s' && podman ps --filter 'label=io.podman.compose.project=%s' --format 'table {{.Names}}\t{{.Ports}}'",
			p.RemoteDir,
			filepath.Base(p.RemoteDir)),
		)
		fmt.Println(output)
		fmt.Println("──────────────────────────────────────────────────" + Reset)
	}
	return p.UID, nil
}

func (p *PodmanArg) down(d *Deploy) (string, error) {
	// * 執行動作
	fmt.Printf("[*] executing: podman compose %s\n", strings.Join(p.RemoteArgs, " "))
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	cmd := fmt.Sprintf(
		"cd '%s' && podman compose %s 2>&1 | grep -v 'no container\\|no pod' || true",
		p.RemoteDir,
		shellJoin(p.RemoteArgs),
	)
	if err := utils.SSHRun(cmd); err != nil {
		return "", err
	}
	fmt.Println(Hint + "──────────────────────────────────────────────────" + Reset)
	return p.UID, nil
}

func (p *PodmanArg) RsyncToRemote() error {
	env, err := utils.GetENV()
	if err != nil {
		return err
	}

	cmdArgs := []string{
		"-p", env.Password,
		"rsync",
		"-avz", "--delete",
		"--exclude=node_modules/", "--exclude=vendor/", "--exclude=__pycache__/",
		"--exclude=*.pyc", "--exclude=.venv/", "--exclude=venv/", "--exclude=env/",
		"--exclude=.env.local", "--exclude=.git/", "--exclude=.gitignore",
		"--exclude=*.log", "--exclude=.DS_Store", "--exclude=Thumbs.db",
		"-e", "ssh -o StrictHostKeyChecking=no",
		p.LocalDir + "/",
		fmt.Sprintf("%s:%s/", env.Remote, p.RemoteDir),
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
