# ICS/OT + Mythic C2 Demo

模擬工控環境（GRFICSv3）透過真實 C2 框架（Mythic）發動攻擊，兩者都是本機獨立跑的 Docker
Compose stack。

## 目錄結構

- `GRFICSv3/` -- Fortiphyd OT/ICS lab 的 fork，有自己的 git repo/remote，只提供模擬與
  視覺化的攻擊目標，架構見它自己的 `CLAUDE.md`。
- `Mythic/` -- Mythic C2 框架的 fork，有自己的 git repo/remote，提供攻擊者那一側。
- `frostygoop-rewrite/` -- 本專題唯一自己寫的程式碼，FrostyGoop 樣本的 Go 重寫版，投遞
  到 implant 上跑的靜態 binary，跟 GRFICS/Mythic 都沒有 build 耦合。
- `team/` -- 四人分工與介面契約，`team/README.md` 是協定層與序列層之間唯一定義一次的
  Go interface。

`GRFICSv3/` 和 `Mythic/` 都是各自獨立 clone 的 repo，不是 submodule，這個頂層 repo 只
追蹤專案文件跟 `frostygoop-rewrite/`。

第一次接觸這個專案？`SETUP.md` 從乾淨的 Linux 主機開始，把兩個 stack 都建起來。

## Port 對照表

| Port                | 服務                              | 備註                                                                    |
| ------------------- | --------------------------------- | ------------------------------------------------------------------------ |
| 8090                | GRFICS 模擬儀表板                 | 只開 localhost                                                          |
| 6081                | GRFICS HMI（ScadaLTS）            | 只開 localhost                                                          |
| 51820/udp           | GRFICS router WireGuard           | 只開 localhost                                                          |
| 8080                | Mythic operator UI（nginx）       | 只開 localhost                                                          |
| 5433                | Mythic postgres                   | 原本的 5432 被這台機器上另一個無關專案的 native postgresql 佔用，才改的 |
| 8082/8091/3000/8888 | Mythic hasura/docs/react/jupyter  | 為了避開跟上面 8080/8090 這組慣例衝突，全部移出預設值                   |

GRFICS 的 `plc` 容器沒有對外開 port，只能從 `b-ics-net` 這個 Docker network 內部連到，
這就是重點 -- 它才是真正的攻擊目標。

## 攻擊路徑現況

Mythic 的 HTTP C2 profile（`http` 容器）用 `network_mode: host` 跑，直接監聽 host 的
`0.0.0.0:80`，不走 `mythic_default` 這個 bridge network。放在 GRFICS `b-ics-net`
（192.168.95.0/24，`plc` 所在網段）上的 implant，靠回連 bridge 的 gateway 位址
`192.168.95.1:80` 就能連到它 -- 不需要在兩個 Compose 專案之間 `docker network connect`。

目前進度：Poseidon agent、HTTP C2 profile、完整投遞鏈都已驗證跑通（hello-world 靜態
ELF 走完 register -> upload -> chmod -> shell 執行整條路）。HR1026（`pressure_sp`）攻擊
路徑已手動驗證成立，壓力可推到 3104.7 kPa（超過 3000 kPa 破壞門檻），回滾也驗證過。下一
步是把 `frostygoop-rewrite/` 的 Go binary 與兩條攻擊序列（coil 路徑、holding register
路徑）做完，換掉手動下 Modbus 指令，接上 Mythic tasking 跑完整鏈路。詳細分工與時程見
`PROJECT_SCOPE.md`、`team/README.md`。

## Ops 補充

**手動改 `.env` 之後 Postgres/RabbitMQ 密碼對不上**：兩個容器都是第一次 init 時把密碼
寫進 bind-mount 的資料目錄（`postgres-docker/database`、`rabbitmq-docker/storage`）。
之後如果 `.env` 裡的 `POSTGRES_PASSWORD` 或 `RABBITMQ_PASSWORD` 換了（例如密碼管理器重
新產生一組新密碼），已經 init 過的服務還是吃舊密碼，`mythic_server` 兩邊都認證失敗，回
報 unhealthy。修法是把對應的資料目錄清掉再重啟，讓它用目前 `.env` 的值重新 init。只有全
新安裝、還沒有任何 operation/checkin 資料時這樣做才安全，一旦有真的 Mythic operation 資
料就不能清。

**UFW 預設擋掉 bridge 到 host 的流量**：這台機器的 UFW 預設丟棄從 Docker bridge network
打進來的 `INPUT`/`FORWARD` 流量，會讓上面 `b-ics-net` -> `192.168.95.1:80` 的 callback
路徑安靜失敗（逾時，不是連線被拒）。需要針對相關 subnet/port 開一條明確的 allow rule；
細節看這個檔案的 git history，或是先問過，不要假設新機器上這條已經開好。

## 期限

簡報 2026-07-30 14:00 繳交，現場 demo 20:30-20:45。
