# go-podrun 技術文件

← [返回 README](README.zh.md)

## 前置需求

| 需求 | 位置 | 說明 |
|---|---|---|
| Go 1.25.1+ | 本地（編譯） | 編譯二進位檔所需 |
| `sshpass` | 本地（CLI） | 密碼式 SSH 自動化 |
| `rsync` | 本地（CLI） | 檔案同步 |
| `ssh` | 本地（CLI） | 遠端指令執行 |
| `curl`、`unzip` | 本地（CLI） | 若缺少則由 `CheckRelyPackages` 自動安裝 |
| Podman Compose | 遠端伺服器 | 容器 runtime（Rootless） |
| k3s | 遠端伺服器 | 選用；僅在 `--type=k3s` 時需要 |
| `.env` 檔案 | 本地工作目錄 | 由 `godotenv` 自動載入 |

本地缺少的套件會在啟動時自動偵測，並透過 `brew`（macOS）或 `apt`/`dnf`/`yum`/`pacman`（Linux）自動安裝。

## 安裝

**API server + CLI**（同時編譯兩個二進位檔）：

```bash
go build -o podrun-api ./cmd/api
go build -o podrun    ./cmd/cli
```

**僅安裝 CLI**（安裝至 `$GOPATH/bin`）：

```bash
go install github.com/pardnchiu/go-podrun/cmd/cli@latest
```

## 設定

環境變數透過 `godotenv` 從工作目錄中的 `.env` 檔案載入。CLI 的三個變數為必填；`DB_PATH` 為選填。

| 變數 | 必填 | 預設值 | 說明 |
|---|---|---|---|
| `PODRUN_SERVER` | 僅 CLI | — | 遠端伺服器 Hostname 或 IP |
| `PODRUN_USERNAME` | 僅 CLI | — | 遠端伺服器的 SSH 使用者名稱 |
| `PODRUN_PASSWORD` | 僅 CLI | — | SSH 密碼（由 `sshpass` 使用） |
| `DB_PATH` | 僅 API | `~/.podrun/database.db`（主機）/ `/data/database.db`（Docker） | SQLite 資料庫檔案路徑 |

**`.env` 範例：**

```dotenv
PODRUN_SERVER=192.168.1.100
PODRUN_USERNAME=podrun
PODRUN_PASSWORD=yourpassword
```

## 使用方式

### 基本 — 啟動專案

在專案目錄下執行（目錄中必須包含 `docker-compose.yml` 或 `docker-compose.yaml`）：

```bash
podrun up -d
```

執行步驟如下：
1. 在遠端建立專案目錄 `/home/podrun/<project>_<hash>/`
2. 透過 rsync 同步本地檔案至遠端（排除 `node_modules`、`.git`、`*.log` 等）
3. 複製 `docker-compose.yml` 為 `docker-compose.podrun.yml` 並移除 Host Port 綁定
4. 在遠端執行 `podman compose -f docker-compose.podrun.yml up -d`
5. 透過 API server 將部署資訊登錄至本地 SQLite 資料庫

### 進階 — 指定目錄或檔案

```bash
# 明確指定專案資料夾
podrun up -d --folder=/path/to/project

# 指定 compose 檔案
podrun up -d -f ./my-app/docker-compose.yml

# 切換至 k3s runtime
podrun up -d --type=k3s

# 停止容器
podrun down

# 完整清除（容器 + 映像 + 遠端資料夾）
podrun clear
```

## CLI 參考

### 指令

| 指令 | 說明 |
|---|---|
| `up` | 同步檔案、啟動 compose stack、登錄部署資訊 |
| `down` | 停止並移除容器 |
| `clear` | 停止容器、移除映像、刪除遠端專案資料夾 |
| `ps` | 列出專案中正在執行的容器 |
| `logs` | 顯示容器日誌（支援 `-f` 持續追蹤） |
| `restart` | 重新啟動容器 |
| `exec` | 在容器內執行指令 |
| `build` | 建構映像而不啟動容器 |
| `domain` | *(stub)* 設定 Traefik Domain 路由 |
| `deploy` | *(stub)* 部署至 Kubernetes |
| `export` | *(stub)* 匯出專案為 Pod Manifest |

### 旗標

| 旗標 | 縮寫 | 說明 |
|---|---|---|
| `--detach` | `-d` | 在背景執行容器 |
| `--folder=<path>` | | 覆寫本地專案目錄 |
| `--type=<target>` | | Runtime 目標：`podman`（預設）或 `k3s` |
| `--output=<path>` | `-o` | 覆寫遠端目標目錄 |
| `-f <file>` | | 指定 compose 檔案路徑 |
| `-u <uid>` | | 明確指定部署 UID |

### API 端點

API server 監聽 `:8080`，負責管理部署登錄簿。

| 方法 | 路徑 | 說明 |
|---|---|---|
| `GET` | `/api/pod/list` | 列出所有已登錄的部署 |
| `POST` | `/api/pod/upsert` | 新增或更新 Pod 記錄 |
| `POST` | `/api/pod/update/:uid` | 依 UID 更新 Pod（例如標記為已移除） |
| `POST` | `/api/pod/record/insert` | 插入一筆生命週期事件記錄 |
| `GET` | `/api/health` | 健康檢查 — 回傳 `ok` |

### Pod 模型欄位

| 欄位 | 型別 | 說明 |
|---|---|---|
| `id` | `int64` | 自動遞增主鍵 |
| `uid` | `string` | 唯一部署識別碼（MAC + 路徑的 MD5 雜湊） |
| `pod_id` | `string` | Podman Pod ID 或遠端目錄基底名稱 |
| `pod_name` | `string` | Podman Pod 名稱 |
| `local_dir` | `string` | 本地專案目錄的絕對路徑 |
| `remote_dir` | `string` | 遠端目錄路徑（`/home/podrun/<name>_<hash>`） |
| `file` | `string` | Compose 檔案路徑（若使用 `-f` 指定） |
| `target` | `string` | Runtime 目標（`podman` 或 `k3s`） |
| `status` | `string` | 生命週期狀態（`starting`、`running`、`failed`、`removed`） |
| `hostname` | `string` | 本地機器的 Hostname |
| `ip` | `string` | 本地機器的 IP 位址 |
| `replicas` | `int` | 副本數量（預設 `1`） |
| `created_at` | `time.Time` | 建立時間戳記 |
| `updated_at` | `time.Time` | 最後更新時間戳記 |
| `dismiss` | `int` | 軟刪除旗標（`0` = 啟用，`1` = 已移除） |

---

©️ 2025 [pardnchiu](https://github.com/pardnchiu)
