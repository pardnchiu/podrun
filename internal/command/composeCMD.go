package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/pardnchiu/go-podrun/internal/model"
	"github.com/pardnchiu/go-podrun/internal/utils"
)

const (
	Reset = "\033[0m"
	Hint  = "\033[90m"
	Ok    = "\033[32m"
	Error = "\033[31m"
	Warn  = "\033[33m"
)

func (p *PodmanArg) ComposeCMD() (*model.Pod, error) {
	d := &model.Pod{
		UID:       p.UID,
		PodID:     filepath.Base(p.RemoteDir),
		PodName:   filepath.Base(p.RemoteDir),
		LocalDir:  p.LocalDir,
		RemoteDir: p.RemoteDir,
		Target:    p.Target,
		File:      p.File,
		Status:    "starting",
		Hostname:  p.Hostname,
		IP:        p.IP,
		Replicas:  1,
	}

	switch p.Command {
	case "up":
		return p.up(d)
	case "clear":
		return p.clear(d)
	case "down", "ps", "logs", "restart", "exec", "build":
		return p.runCMD(d)
	}
	return nil, fmt.Errorf("unsupported command: %s", p.Command)
}

func (p *PodmanArg) up(d *model.Pod) (*model.Pod, error) {
	fmt.Println("[+] create folder if not exist")
	if err := utils.SSHRun("mkdir", "-p", p.RemoteDir); err != nil {
		return nil, err
	}

	// * 同步檔案夾資料
	fmt.Println("[*] syncing files")
	if err := p.RsyncToRemote(d); err != nil {
		return nil, err
	}
	fmt.Println("──────────────────────────────────────────────────" + Reset)

	// * 調整 docker-compose.yml 內容
	fmt.Println("[*] modifying compose file (remove ports)")
	if err := p.ModifyComposeFile(); err != nil {
		return nil, fmt.Errorf("[x] failed to modify compose file: %w", err)
	}

	// * 關閉舊的容器 (if exists)
	fmt.Println("[*] cleaning up old containers")
	_, _ = utils.SSEOutput(fmt.Sprintf(
		"cd '%s' && podman compose -f docker-compose.podrun.yml down -v >/dev/null 2>&1",
		p.RemoteDir,
	))
	removePod(d.UID)

	// * 執行動作
	fmt.Printf("[*] executing: podman compose -f docker-compose.podrun.yml %s\n", strings.Join(p.RemoteArgs, " "))
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	remoteCmd := fmt.Sprintf("cd '%s' && podman compose -f docker-compose.podrun.yml %s 2>&1", p.RemoteDir, shellJoin(p.RemoteArgs))
	if !p.Detach {
		remoteCmd = fmt.Sprintf(`
				cleanup() {
					echo "[*] stopping containers"
					cd '%s' && podman compose -f docker-compose.podrun.yml down
				}
				trap cleanup INT TERM
				%s
			`, p.RemoteDir, remoteCmd)
	}
	if err := utils.SSHRun(remoteCmd); err != nil {
		return nil, err
	}
	fmt.Println(Hint + "──────────────────────────────────────────────────" + Reset)

	// * 取得 Pod 資訊
	projectName := filepath.Base(p.RemoteDir)
	podInfo, err := utils.SSEOutput(fmt.Sprintf(
		"podman pod ps --filter 'name=pod_%s' --format '{{.ID}}\t{{.Name}}'",
		projectName,
	))
	if err == nil && podInfo != "" {
		parts := strings.Split(strings.TrimSpace(podInfo), "\t")
		if len(parts) >= 2 {
			d.PodID = parts[0]
			d.PodName = parts[1]
		}
	}

	// * 輸出結果
	if p.Detach {
		fmt.Println("[*] service ports:")
		fmt.Println(Ok + "──────────────────────────────────────────────────")
		output, _ := utils.SSEOutput(fmt.Sprintf(
			"cd '%s' && podman ps --filter 'label=io.podman.compose.project=%s' --format 'table {{.Names}}\t{{.Ports}}'",
			p.RemoteDir,
			projectName),
		)
		fmt.Println(output)
		fmt.Printf("Pod ID: %s\n", d.PodID)
		fmt.Printf("Pod Name: %s\n", d.PodName)
		fmt.Printf("Hostname: %s\n", d.Hostname)
		fmt.Printf("IP: %s\n", d.IP)
		fmt.Println("──────────────────────────────────────────────────" + Reset)
	}

	// *  發送 Pod 資訊到 API
	if err := upsertPod(d); err != nil {
		return nil, fmt.Errorf("[x] failed to upsert pod: %w", err)
	}
	recordPod(d, "up")

	return d, nil
}

func (p *PodmanArg) clear(d *model.Pod) (*model.Pod, error) {
	// * 停止並移除容器和 volumes
	fmt.Println("[*] remove containers and volumes")
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	downCmd := fmt.Sprintf(
		"cd '%s' && podman compose -f docker-compose.podrun.yml down -v 2>&1 | grep -v 'no container\\|no pod' || true",
		p.RemoteDir,
	)
	removePod(d.UID)
	if err := utils.SSHRun(downCmd); err != nil {
		return nil, fmt.Errorf("failed to remove containers: %w", err)
	}
	fmt.Println("──────────────────────────────────────────────────" + Reset)

	// * 移除映像
	fmt.Println("[*] clean images")
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	imageCmd := fmt.Sprintf(
		"cd '%s' && podman compose -f docker-compose.podrun.yml down --rmi all 2>&1 | grep -v 'no container\\|no pod\\|no image' || true",
		p.RemoteDir,
	)
	if err := utils.SSHRun(imageCmd); err != nil {
		return nil, fmt.Errorf("failed to remove images: %w", err)
	}
	fmt.Println("──────────────────────────────────────────────────" + Reset)

	// * 移除資料夾
	fmt.Println("[*] remove project folder")
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	removeCmd := fmt.Sprintf(
		"podman run --rm --privileged -v '%s:/parent' alpine:latest sh -c 'rm -rf /parent/%s'",
		filepath.Dir(p.RemoteDir),
		filepath.Base(p.RemoteDir),
	)
	if err := utils.SSHRun(removeCmd); err != nil {
		return nil, fmt.Errorf("failed to remove folder: %w", err)
	}
	fmt.Println(Hint + "──────────────────────────────────────────────────" + Reset)

	recordPod(d, "clear")
	return d, nil
}

func (p *PodmanArg) runCMD(d *model.Pod) (*model.Pod, error) {
	fmt.Printf("[*] executing: podman compose -f docker-compose.podrun.yml %s\n", strings.Join(p.RemoteArgs, " "))
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	if err := utils.SSHRun(fmt.Sprintf(
		"cd '%s' && podman compose %s",
		p.RemoteDir,
		shellJoin(p.RemoteArgs)),
	); err != nil {
		return nil, err
	}
	fmt.Println(Hint + "──────────────────────────────────────────────────" + Reset)

	if p.Command == "down" {
		removePod(d.UID)
	}
	recordPod(d, p.Command)
	return d, nil
}

func (p *PodmanArg) RsyncToRemote(d *model.Pod) error {
	env, err := utils.CheckENV()
	if err != nil {
		return err
	}

	checkDirCmd := fmt.Sprintf("[ -d %s ] && [ -n \"$(ls -A %s)\" ] && echo 'not-empty' || echo 'empty'",
		p.RemoteDir, p.RemoteDir)
	checkDirArgs := []string{
		"-p", env.Password,
		"ssh", "-o", "StrictHostKeyChecking=no",
		env.Remote,
		checkDirCmd,
	}
	output, err := utils.CMDOutput("sshpass", checkDirArgs...)
	if err != nil {
		return fmt.Errorf("check remote directory failed: %w", err)
	}
	isRemoteEmpty := strings.TrimSpace(output) == "empty"

	excludes := []string{
		"--exclude=node_modules/", "--exclude=vendor/", "--exclude=__pycache__/",
		"--exclude=*.pyc", "--exclude=.venv/", "--exclude=venv/", "--exclude=env/",
		"--exclude=.env.local", "--exclude=.git/", "--exclude=.gitignore",
		"--exclude=*.log", "--exclude=.DS_Store", "--exclude=Thumbs.db",
		"--exclude=.next/",
		"--exclude=docker-compose.podrun.yml",
		"--exclude=app/package-lock.json", "--exclude=app/package-lock.json",
	}

	baseArgs := []string{
		"-e", "ssh -o StrictHostKeyChecking=no",
		p.LocalDir + "/",
		fmt.Sprintf("%s:%s/", env.Remote, p.RemoteDir),
	}

	if !isRemoteEmpty {
		fmt.Println("[*] checking changes")
		fmt.Println(Hint + "──────────────────────────────────────────────────")
		checkArgs := []string{
			"-p", env.Password,
			"rsync",
			"-avni",
			"--delete",
		}
		checkArgs = append(checkArgs, excludes...)
		checkArgs = append(checkArgs, baseArgs...)
		output, err = utils.CMDOutput("sshpass", checkArgs...)
		if err != nil {
			return fmt.Errorf("preview failed: %w", err)
		}
		fmt.Print(output)
		fmt.Println(Hint + "──────────────────────────────────────────────────" + Reset)

		if changeExist(output) {
			fmt.Print("[!] confirm sync? (y/N): ")
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				return fmt.Errorf("cancelled")
			}
			recordPod(d, "overwrite")
		}
	} else {
		recordPod(d, "sync")
	}

	excludes = []string{
		"--exclude=node_modules/", "--exclude=vendor/", "--exclude=__pycache__/",
		"--exclude=*.pyc", "--exclude=.venv/", "--exclude=venv/", "--exclude=env/",
		"--exclude=.env.local", "--exclude=.git/", "--exclude=.gitignore",
		"--exclude=*.log", "--exclude=.DS_Store", "--exclude=Thumbs.db",
		"--exclude=.next/", "--exclude=app/package-lock.json",
	}

	fmt.Println("[*] syncing")
	fmt.Println(Hint + "──────────────────────────────────────────────────")
	syncArgs := []string{
		"-p", env.Password,
		"rsync",
		"-avz",
		"--delete",
	}
	syncArgs = append(syncArgs, excludes...)
	syncArgs = append(syncArgs, baseArgs...)
	return utils.CMDRun("sshpass", syncArgs...)
}

func (p *PodmanArg) ModifyComposeFile() error {
	var composeFile string

	output, _ := utils.SSEOutput(fmt.Sprintf(
		"test -f '%s/docker-compose.yml' && echo 'yml' || echo 'notfound'",
		p.RemoteDir,
	))
	if strings.TrimSpace(output) == "yml" {
		composeFile = "docker-compose.yml"
	} else {
		output, _ = utils.SSEOutput(fmt.Sprintf(
			"test -f '%s/docker-compose.yaml' && echo 'yaml' || echo 'notfound'",
			p.RemoteDir,
		))
		if strings.TrimSpace(output) == "yaml" {
			composeFile = "docker-compose.yaml"
		} else {
			return fmt.Errorf("docker-compose.yml or docker-compose.yaml not found")
		}
	}

	podrunFile := "docker-compose.podrun.yml"
	copyCmd := fmt.Sprintf(
		"cp '%s/%s' '%s/%s'",
		p.RemoteDir, composeFile,
		p.RemoteDir, podrunFile,
	)
	if err := utils.SSHRun(copyCmd); err != nil {
		return err
	}

	// 移除 ports
	sedCmds := []string{
		`sed -i -E 's/(["\x27]?)[0-9]+:([0-9]+)(["\x27]?)/\1\2\3/g' '%s/%s'`,
		`sed -i -E 's/(["\x27]?)\$\{[^}]+\}:([0-9]+)(["\x27]?)/\1\2\3/g' '%s/%s'`,
		`sed -i -E 's/\$\{[^}]+:[?][^}]+\}://g' '%s/%s'`,
	}

	for _, cmdTemplate := range sedCmds {
		cmd := fmt.Sprintf(cmdTemplate, p.RemoteDir, podrunFile)
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
	`, p.RemoteDir, podrunFile, p.RemoteDir, podrunFile, p.RemoteDir, podrunFile, p.RemoteDir, podrunFile)

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

func upsertPod(d *model.Pod) error {
	fmt.Println("[*] syncing pod info to database")
	jsonData, err := json.Marshal(d)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		"http://localhost:8080/pod/upsert",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func removePod(uid string) {
	jsonData, err := json.Marshal(&model.Pod{
		Dismiss: 1,
	})
	// * slience, if wrong, just wrong
	if err != nil {
		return
	}

	resp, err := http.Post(
		"http://localhost:8080/pod/update/"+uid,
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	// * slience, if wrong, just wrong
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// * slience, if wrong, just wrong
	if resp.StatusCode != http.StatusOK {
		return
	}
}

func recordPod(d *model.Pod, content string) error {
	fmt.Println("[*] add record to database")
	jsonData, err := json.Marshal(&model.Record{
		UID:      d.UID,
		Content:  content,
		Hostname: d.Hostname,
		IP:       d.IP,
	})
	slog.Info("", "data", jsonData)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		"http://localhost:8080/pod/record/insert",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// * exmaple
// ──────────────────────────────────────────────────
// sending incremental file list
// .d..t....... ./

// sent 1,009 bytes  received 25 bytes  2,068.00 bytes/sec
// total size is 32,245  speedup is 31.18 (DRY RUN)
// ──────────────────────────────────────────────────
func changeExist(output string) bool {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "sending") ||
			strings.HasPrefix(line, "sent") ||
			strings.HasPrefix(line, "total size") {
			continue
		}
		if strings.HasPrefix(line, ".d..t.......") {
			continue
		}
		return true
	}
	return false
}
