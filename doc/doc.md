# go-podrun Documentation

← [Back to README](../README.md)

## Prerequisites

| Requirement | Where | Notes |
|---|---|---|
| Go 1.25.1+ | Local (build) | Required to compile binaries |
| `sshpass` | Local (CLI) | Password-based SSH automation |
| `rsync` | Local (CLI) | File synchronization |
| `ssh` | Local (CLI) | Remote command execution |
| `curl`, `unzip` | Local (CLI) | Auto-installed by `CheckRelyPackages` if missing |
| Podman Compose | Remote server | Container runtime (rootless) |
| k3s | Remote server | Optional; required only for `--type=k3s` |
| `.env` file | Local working dir | Loaded automatically via `godotenv` |

Missing local packages are detected at startup and installed automatically via `brew` (macOS) or `apt`/`dnf`/`yum`/`pacman` (Linux).

## Installation

**API server + CLI** (build both binaries):

```bash
go build -o podrun-api ./cmd/api
go build -o podrun    ./cmd/cli
```

**CLI only** (install to `$GOPATH/bin`):

```bash
go install github.com/pardnchiu/go-podrun/cmd/cli@latest
```

## Configuration

Environment variables are loaded from `.env` in the working directory via `godotenv`. All three CLI variables are required; `DB_PATH` is optional.

| Variable | Required | Default | Description |
|---|---|---|---|
| `PODRUN_SERVER` | CLI only | — | Remote server hostname or IP address |
| `PODRUN_USERNAME` | CLI only | — | SSH username on the remote server |
| `PODRUN_PASSWORD` | CLI only | — | SSH password (used by `sshpass`) |
| `DB_PATH` | API only | `~/.podrun/database.db` (host) / `/data/database.db` (Docker) | SQLite database file path |

**Example `.env`:**

```dotenv
PODRUN_SERVER=192.168.1.100
PODRUN_USERNAME=podrun
PODRUN_PASSWORD=yourpassword
```

## Usage

### Basic — bring up a project

Run from within the project directory (must contain `docker-compose.yml` or `docker-compose.yaml`):

```bash
podrun up -d
```

This performs the following steps:
1. Creates the remote project directory under `/home/podrun/<project>_<hash>/`
2. Syncs local files to the remote server via rsync (excludes `node_modules`, `.git`, `*.log`, etc.)
3. Copies `docker-compose.yml` to `docker-compose.podrun.yml` and strips host-port bindings
4. Runs `podman compose -f docker-compose.podrun.yml up -d` on the remote server
5. Registers the deployment in the local SQLite database via the API server

### Advanced — targeting a specific directory or file

```bash
# Explicit project folder
podrun up -d --folder=/path/to/project

# Specific compose file
podrun up -d -f ./my-app/docker-compose.yml

# Target k3s instead of Podman
podrun up -d --type=k3s

# Tear down
podrun down

# Full cleanup (containers + images + remote folder)
podrun clear
```

## CLI Reference

### Commands

| Command | Description |
|---|---|
| `up` | Sync files, (re-)build compose stack, register deployment |
| `down` | Stop and remove containers |
| `clear` | Stop containers, remove images, delete remote project folder |
| `ps` | List running containers in the project |
| `logs` | Show container logs (`-f` supported for follow mode) |
| `restart` | Restart containers |
| `exec` | Execute a command inside a container |
| `build` | Build images without starting containers |
| `domain` | *(stub)* Configure Traefik domain routing |
| `deploy` | *(stub)* Deploy to Kubernetes |
| `export` | *(stub)* Export project to pod manifest |

### Flags

| Flag | Short | Description |
|---|---|---|
| `--detach` | `-d` | Run containers in background |
| `--folder=<path>` | | Override local project directory |
| `--type=<target>` | | Runtime target: `podman` (default) or `k3s` |
| `--output=<path>` | `-o` | Override remote destination directory |
| `-f <file>` | | Specify compose file path |
| `-u <uid>` | | Specify deployment UID explicitly |

### API Endpoints

The API server listens on `:8080` and manages the deployment registry.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/pod/list` | List all registered deployments |
| `POST` | `/api/pod/upsert` | Create or update a pod record |
| `POST` | `/api/pod/update/:uid` | Update a pod by UID (e.g., mark dismissed) |
| `POST` | `/api/pod/record/insert` | Insert a lifecycle event record |
| `GET` | `/api/health` | Health check — returns `ok` |

### Pod Model Fields

| Field | Type | Description |
|---|---|---|
| `id` | `int64` | Auto-increment primary key |
| `uid` | `string` | Unique deployment identifier (MD5 of MAC + path) |
| `pod_id` | `string` | Podman pod ID or remote directory base name |
| `pod_name` | `string` | Podman pod name |
| `local_dir` | `string` | Absolute path to local project directory |
| `remote_dir` | `string` | Remote directory path (`/home/podrun/<name>_<hash>`) |
| `file` | `string` | Compose file path (if `-f` was specified) |
| `target` | `string` | Runtime target (`podman` or `k3s`) |
| `status` | `string` | Lifecycle status (`starting`, `running`, `failed`, `removed`) |
| `hostname` | `string` | Local machine hostname |
| `ip` | `string` | Local machine IP address |
| `replicas` | `int` | Number of replicas (default `1`) |
| `created_at` | `time.Time` | Creation timestamp |
| `updated_at` | `time.Time` | Last update timestamp |
| `dismiss` | `int` | Soft-delete flag (`0` = active, `1` = dismissed) |

---

©️ 2025 [pardnchiu](https://github.com/pardnchiu)
