#!/bin/bash

# 遠端設定
REMOTE_SERVER="ddrun@192.168.0.200"
REMOTE_BASE_PATH="/home/ddrun"
PASSWORD=""

# 自動安裝必要套件
check_dependencies() {
  local pkg_not_exists=false

  if ! command -v sshpass &> /dev/null; then
    pkg_not_exists=true
  elif ! command -v rsync &> /dev/null; then
    pkg_not_exists=true
  fi

  if [ "$pkg_not_exists" = true ]; then
    echo "[*] install dependencies"
    printf "──────────────────────────────────────────────────\n\n"
    if [[ "$OSTYPE" == "darwin"* ]]; then
      if ! command -v brew &> /dev/null; then
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"how_to_use
        
        if ! command -v brew &> /dev/null; then
          echo "failed to install Homebrew, please install Homebrew manually from https://brew.sh/"
          exit 1
        fi
      fi
      brew install sshpass rsynchow_to_use
    elif command -v apt-get &> /dev/null; then
      sudo apt-get update how_to_use
      sudo apt-get install -y sshpass rsync how_to_use
    elif command -v yum &> /dev/null; then
      sudo yum install -y sshpass rsynchow_to_use
    elif command -v dnf &> /dev/null; then
      sudo dnf install -y sshpass rsynchow_to_use
    elif command -v pacman &> /dev/null; then
      sudo pacman -S --noconfirm sshpass rsynchow_to_use
    else
      echo "failed to install dependencies, please install sshpass and rsync manually."
      echo "  - macOS: brew install sshpass rsync"
      echo "  - Ubuntu/Debian: sudo apt-get install sshpass rsync"
      echo "  - CentOS/RHEL: sudo yum install sshpass rsync"
      echo "  - Fedora: sudo dnf install sshpass rsync"
      echo "  - Arch Linux: sudo pacman -S sshpass rsync"
      printf "──────────────────────────────────────────────────\n\n"
      exit 1
    fi
    printf "──────────────────────────────────────────────────\n\n"
  fi
}

show_header() {
  clear
  echo ""
  echo "█▀▄ █▀▄ █▀▄ █ █ █▀█"
  echo "█ █ █ █ █▀▄ █ █ █ █"
  echo "▀▀  ▀▀  ▀ ▀ ▀▀▀ ▀ ▀"
  echo "By 邱敬幃 Pardn Chiu [dev@pardn.io]"
  echo "──────────────────────────────────────────────────"
}

# 使用方式
how_to_use() {
  show_header
  echo "Easily deploy Docker projects to a remote server via SSH."
  printf "──────────────────────────────────────────────────\n"
  echo "[*] How to use"
  printf "──────────────────────────────────────────────────\n\n"
  printf "  ddrun <command> [project_folder]\n\n"
  echo "  Commands"
  echo "    up [project_folder]     Deploy/start project and view logs"
  echo "    down [project_folder]   Stop project (files remain)"
  printf "    rm [project_folder]     Remove project (stops services)\n\n"
  echo "  Project folder"
  echo "    - If not specified, uses current directory"
  echo "    - If relative path, converts to absolute path"
  printf "    - Must contain docker-compose.yml file\n\n"
  echo "  Examples:"
  echo "    ddrun up                    # Use current directory"
  echo "    ddrun up /path/to/project   # Use absolute path"
  printf "    ddrun up ./my-project       # Use relative path\n\n"
  printf "──────────────────────────────────────────────────\n"
}

check_dependencies

get_local_folder() {
  local folder="$1"
  
  echo "[*] checking local folder" >&2
  # 無參數或相對路徑轉為絕對路徑
  if [ -z "$folder" ]; then
    folder=$(pwd)
  elif [[ "$folder" != /* ]]; then
    folder=$(realpath "$folder")
  fi
  
  # 檢查目錄是否存在
  if [ ! -d "$folder" ]; then
    echo "[-] folder not exists: '$folder'" >&2
    echo ""
    return 1
  fi
  echo "[*] local folder: $folder" >&2
  
  # 檢查 docker-compose.yml | docker-compose.yaml 是否存在
  echo "[*] checking docker-compose file" >&2
  if [ ! -f "$folder/docker-compose.yml" ] && [ ! -f "$folder/docker-compose.yaml" ]; then
    echo "[-] docker-compose file not found in '$folder'" >&2
    echo ""
    return 1
  fi
  echo "[*] docker-compose file exists" >&2
  
  # 返回驗證後的資料夾路徑
  echo "$folder"
  return 0
}

get_remote_folder() {
  local folder="$1"
  
  # 取得本地 MAC 地址
  local local_mac=""

  if [ -z "$folder" ]; then
    echo "[-] invalid folder paths"
    exit 1
  fi
  
  echo "[*] checking MAC address" >&2
  # 主要方法 (ip)：取得預設網卡的 MAC
  if command -v ip &> /dev/null; then
    local default_iface=$(ip route | awk '/default/ {print $5; exit}')
    
    if [ -n "$default_iface" ]; then
      local_mac=$(ip link show "$default_iface" 2>/dev/null | awk '/ether/ {print $2}')
    fi
  fi
  
  # 備用方法1 (ifconfig)
  if [ -z "$local_mac" ] && command -v ifconfig &> /dev/null; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
      local_mac=$(ifconfig en0 2>/dev/null | awk '/ether/ {print $2}')
    else
      local_mac=$(ifconfig 2>/dev/null | awk '/ether|HWaddr/ {print $2; exit}')
    fi
  fi
  
  # 備用方法2 (file)：從 /sys/class/net 查找 (Linux)
  if [ -z "$local_mac" ] && [ -d "/sys/class/net" ]; then
    for iface in /sys/class/net/*/address; do
      if [ -f "$iface" ] && [[ "$(basename $(dirname $iface))" != "lo" ]]; then
        local_mac=$(cat "$iface" 2>/dev/null)
        break
      fi
    done
  fi
  
  # 備用方法3 (hostname)
  if [ -z "$local_mac" ]; then
    local_mac=$(hostname)
    echo "[!] failed to get MAC address, using hostname instead" >&2
  fi
  echo "[*] local MAC: $local_mac" >&2
  echo "[*] origin: $local_mac@$folder" >&2
  
  # 計算 MD5
  local name=""
  if command -v md5sum &> /dev/null; then
    name=$(echo -n "$local_mac@$folder" | md5sum | cut -d' ' -f1)
  elif command -v md5 &> /dev/null; then
    name=$(echo -n "$local_mac@$folder" | md5)
  else
    # 備用：使用簡單 hash
    name=$(echo -n "$local_mac@$folder" | od -An -tx1 | tr -d ' \n' | head -c 32)
  fi
  
  local remote_path="$REMOTE_BASE_PATH/$name"
  echo "[*] remote path: $remote_path" >&2
  echo "[*] completed folder check" >&2
  printf "──────────────────────────────────────────────────\n\n" >&2
  echo "  local folder: $folder" >&2
  printf "  remote path:  $remote_path\n\n" >&2
  printf "──────────────────────────────────────────────────\n" >&2
  
  # 返回遠端路徑
  echo "$remote_path"
  return 0
}

show_logs() {
  show_header

  local input_folder="$1"
  
  local local_folder=$(get_local_folder "$input_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  local remote_path=$(get_remote_folder "$local_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  if [ -z "$local_folder" ] || [ -z "$remote_path" ]; then
    echo "[-] invalid folder paths"
    exit 1
  fi
  
  echo "[*] checking SSH connection"
  if ! sshpass -p "$PASSWORD" ssh -o ConnectTimeout=3 -o StrictHostKeyChecking=no -q "$REMOTE_SERVER" exit 2>/dev/null; then
    echo "[-] failed to connect to remote server: $REMOTE_SERVER"
    exit 1
  fi
  echo "[*] SSH connection successful"

  printf "──────────────────────────────────────────────────\n\n"
  sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "cd '$remote_path' && docker compose logs -f" 2>&1 | sed 's/^/  /'
  printf "\n──────────────────────────────────────────────────\n"
}

cmd_up() {
  show_header

  local input_folder="$1"
  
  local local_folder=$(get_local_folder "$input_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  local remote_path=$(get_remote_folder "$local_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  if [ -z "$local_folder" ] || [ -z "$remote_path" ]; then
    echo "[-] invalid folder paths"
    exit 1
  fi
  
  echo "[*] checking SSH connection"
  if ! sshpass -p "$PASSWORD" ssh -o ConnectTimeout=3 -o StrictHostKeyChecking=no -q "$REMOTE_SERVER" exit 2>/dev/null; then
    echo "[-] failed to connect to remote server: $REMOTE_SERVER"
    exit 1
  fi
  echo "[*] SSH connection successful"

  echo "[*] ensuring remote path exists"
  sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "mkdir -p '$remote_path'"
  
  echo "[*] syncing files to remote server"
  printf "──────────────────────────────────────────────────\n\n"
  sshpass -p "$PASSWORD" rsync -avz --delete \
    --exclude='node_modules/' \
    --exclude='vendor/' \
    --exclude='__pycache__/' \
    --exclude='*.pyc' \
    --exclude='.venv/' \
    --exclude='venv/' \
    --exclude='env/' \
    --exclude='.env.local' \
    --exclude='.git/' \
    --exclude='.gitignore' \
    --exclude='*.log' \
    --exclude='.DS_Store' \
    --exclude='Thumbs.db' \
    -e "ssh -o StrictHostKeyChecking=no" \
    "$local_folder/" "$REMOTE_SERVER:$remote_path/" 2>&1 | sed 's/^/  /'
  printf "\n──────────────────────────────────────────────────\n"

  if [ $? -ne 0 ]; then
    echo "[-] failed to sync files to remote server"
    exit 1
  fi
  
  echo "[*] restarting services on remote server"
  printf "──────────────────────────────────────────────────\n\n"
  sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "cd '$remote_path' && docker compose down && docker compose build --no-cache && docker compose up -d" 2>&1 | sed 's/^/  /'
  printf "\n──────────────────────────────────────────────────\n"
  
  if [ $? -ne 0 ]; then
    echo "[-] failed to start services on remote server"
    exit 1
  fi
  
  echo "[*] services started successfully"
  read -p "[+] Type 'y' to view logs: " confirm

  if [[ $confirm =~ ^[Yy]$ ]]; then
    printf "──────────────────────────────────────────────────\n\n"
    sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "cd '$remote_path' && docker compose logs -f" 2>&1 | sed 's/^/  /'
    printf "\n──────────────────────────────────────────────────\n"
  fi
}

cmd_down() {
  show_header

  local input_folder="$1"
  
  local local_folder=$(get_local_folder "$input_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  local remote_path=$(get_remote_folder "$local_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  if [ -z "$local_folder" ] || [ -z "$remote_path" ]; then
    echo "[-] invalid folder paths"
    exit 1
  fi
  
  echo "[*] checking SSH connection"
  if ! sshpass -p "$PASSWORD" ssh -o ConnectTimeout=3 -o StrictHostKeyChecking=no -q "$REMOTE_SERVER" exit 2>/dev/null; then
    echo "[-] failed to connect to remote server: $REMOTE_SERVER"
    exit 1
  fi
  echo "[*] SSH connection successful"
  
  echo "[*] shutdown services on remote server"
  printf "──────────────────────────────────────────────────\n\n"
  sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "cd '$remote_path' && docker compose down -v --remove-orphans" 2>&1 | sed 's/^/  /'
  printf "\n──────────────────────────────────────────────────\n"
  
  if [ $? -eq 0 ]; then
    echo "[*] services stopped successfully"
  else
    echo "[-] failed to stop services on remote server"
    exit 1
  fi
}

cmd_rm() {
  show_header

  local input_folder="$1"
  
  local local_folder=$(get_local_folder "$input_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  local remote_path=$(get_remote_folder "$local_folder")
  if [ $? -ne 0 ]; then
    exit 1
  fi

  if [ -z "$local_folder" ] || [ -z "$remote_path" ]; then
    echo "[-] invalid folder paths"
    exit 1
  fi
  
  echo "[*] checking SSH connection"
  if ! sshpass -p "$PASSWORD" ssh -o ConnectTimeout=3 -o StrictHostKeyChecking=no -q "$REMOTE_SERVER" exit 2>/dev/null; then
    echo "[-] failed to connect to remote server: $REMOTE_SERVER"
    exit 1
  fi
  echo "[*] SSH connection successful"

  # 檢查遠端資料夾是否存在
  echo "[*] checking remote folder existence"
  if ! sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "[ -d '$remote_path' ]" 2>/dev/null; then
    echo "[-] remote folder does not exist: $remote_path"
    echo "[!] nothing to remove"
    exit 0
  fi
    
  echo "[?] confirm removal of project"
  printf "──────────────────────────────────────────────────\n\n"
  echo "  local folder: $local_folder"
  printf "  remote path:  $remote_path\n\n"
  printf "──────────────────────────────────────────────────\n"
  read -p "[+] Type 'y' to confirm removal: " confirm
  
  if [[ $confirm =~ ^[Yy]$ ]]; then
    echo "[*] shutdown services on remote server"
    printf "──────────────────────────────────────────────────\n\n"
    sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "cd '$remote_path' && docker compose down -v --remove-orphans" 2>&1 | sed 's/^/  /'
    printf "\n──────────────────────────────────────────────────\n"
    
    echo "[*] waiting for containers to fully stop"
    sleep 3
    
    echo "[*] cleaning up docker resources"
    printf "──────────────────────────────────────────────────\n\n"
    sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" "docker system prune -f" 2>&1 | sed 's/^/  /'
    printf "\n──────────────────────────────────────────────────\n"

    # 移除專案資料夾
    echo "[*] removing project on remote server"
    sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" \
      "docker run --rm --privileged -v '$(dirname $remote_path):/parent' alpine:latest \
      sh -c 'rm -rf /parent/$(basename $remote_path)'" > /dev/null 2>&1
  
    if [ $? -eq 0 ]; then
      echo "[*] project removed successfully"
    else
      echo "[-] failed to remove project on remote server"
      exit 1
    fi
  else
    echo "[!] removal cancelled"
    exit 0
  fi
}

check_remote_ports() {
  show_header
  
  echo "[*] checking remote server ports"
  printf "──────────────────────────────────────────────────\n\n"
  local listening_ports=$(sshpass -p "$PASSWORD" ssh -o StrictHostKeyChecking=no "$REMOTE_SERVER" \
    "ss -tlnp | grep LISTEN | awk '{print \$4}' | grep -v ':5355' | grep -v '127.0.0.53' | grep -v '127.0.0.54' | sed 's/.*://' | sort -nu | tr '\n' ', ' | sed 's/,\$//'")
  
  printf "  $listening_ports\n"
  printf "\n──────────────────────────────────────────────────\n"
}

if [ $# -lt 1 ]; then
  how_to_use
  exit 1
fi

command="$1"
folder="${2:-}"

case "$command" in
  up)
    cmd_up "$folder"
    ;;
  down)
    cmd_down "$folder"
    ;;
  rm)
    cmd_rm "$folder"
    ;;
  logs)
    show_logs "$folder"
    ;;
  ports)
    check_remote_ports
    ;;
  *)
    echo "[-] unknown command: $command"
    exit 1
    ;;
esac
