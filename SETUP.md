# 環境建置指南

從一台乾淨的 Linux 主機建出 GRFICSv3 + Mythic 這兩個獨立的 Docker Compose stack，細節架構、port 對照表、攻擊路徑現況看 `README.md`，這份文件只講怎麼把環境生出來。

## 前置需求

- Docker Engine + Docker Compose plugin，Mythic repo 內附 `install_docker_ubuntu.sh`，用 root 跑就會裝好。
- 把自己帳號加進 `docker` group，之後跑 `mythic-cli`、`docker compose` 都不用 sudo：
  ```bash
  sudo usermod -aG docker $USER
  ```
  加完要登出重登才會生效。
- `git` 和 `git-lfs`（GRFICSv3 的模擬畫面用 Unity WebGL assets，靠 git-lfs 抓）。
- Go 編譯環境（`mythic-cli` 是從原始碼編出來的二進位檔，`make` 裡面會呼叫 Go）。

## 抓 repo

GRFICSv3 和 Mythic 是各自獨立的 fork，各自有自己的 git repo，平行放在同一層目錄下：

```bash
git clone https://github.com/abb00717/GRFICSv3.git
git clone https://github.com/abb00717/Mythic.git
```

## 建置 GRFICSv3（模擬環境那一側）

```bash
cd GRFICSv3
git lfs install
./build.sh
```

只需要跑 `simulation`、`plc`、`hmi`、`router` 四個服務，`workstation`/`attacker`/`caldera`/`wazuh` 這個 fork 雖然還留著但故意不用，攻擊工具改用 Mythic 那邊：

```bash
docker compose up -d simulation plc hmi router
```

## 建置 Mythic（C2 那一側）

```bash
cd Mythic
make
sudo ./mythic-cli start
```

首次啟動會自動生成 `.env`（含隨機密碼），`.env` 沒有進 git，每台機器各自獨立，啟動後把 port 改成跟 `README.md` 的 port 對照表一致，至少要改：

- `NGINX_PORT` -> `8080`（C2 操作介面）
- `HASURA_PORT` -> `8082`
- `DOCUMENTATION_PORT` -> `8091`
- `POSTGRES_PORT` -> 如果這台機器本身已經有 native postgresql 佔用 5432，改成 `5433` 之類沒衝突的值

改完 `.env` 後要重啟讓設定生效：

```bash
sudo ./mythic-cli stop
sudo ./mythic-cli start
```

裝 Poseidon agent 和 HTTP C2 profile：

```bash
sudo ./mythic-cli install github https://github.com/MythicAgents/poseidon
sudo ./mythic-cli install github https://github.com/MythicC2Profiles/http
```

C2 profile 和 agent 這類第三方服務，mythic-cli 裝的時候一律用 `network_mode: host`，不用另外設定，這也是 HTTP C2 能透過 gateway IP 打到 GRFICS `b-ics-net` 而不用手動接兩個 Compose 專案網路的原因。

## 驗證

- GRFICS 模擬儀表板：http://localhost:8090
- GRFICS HMI（ScadaLTS）：http://localhost:6081
- Mythic 操作介面：https://localhost:8080

三個都能開得起來就算建置完成。
