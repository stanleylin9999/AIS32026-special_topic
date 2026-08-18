# 專題範圍與工作分解

AIS3 2026 專題「OT/ICS 惡意程式及 C2 實作」

## 專題定位

### FrostyGoop 逆向、重實作並接上 C2

FrostyGoop 本身是獨立 CLI 工具，攻擊者取得立足點後手動執行，沒有回連也沒有持續控制，我們根據自己逆向出的程式邏輯重新實作功能對等版本，接上 Mythic C2，實際打進 GRFICS 讓反應器壓力失控。

雖然原本在 C2 我們就可以透過 root shell 做到遠端執行攻擊，但它不知道 Modbus 是什麼、不知道哪個暫存器寫下去有用，惡意程式封裝的是攻擊者對目標領域的知識，真實 FrostyGoop 攻擊者當時同樣早有立足點。

因為手上有原始樣本的逆向結果與封包側錄，可以做逐項對照包括真實行為、重寫版行為、差異與原因，以及兩者封包並排比較。

## 已驗證的環境事實

以下除了 FrostyGoop 樣本事實外，都是實際量測或直接讀 GRFICS 原始碼得出，非理論推估。

### FrostyGoop 樣本事實（`lab/FrostyGoop/`，Ghidra 專案 `FrostyGoop`）

`file` 判定為 PE32+ Windows x86-64 console，`go version -m` 直接吐出完整 build metadata：go1.20.4、`CGO_ENABLED=0`、`GOARCH=amd64`、`GOOS=windows`、module path `github.com/rolfl/modbus/CleintTCP`，相依 `github.com/rolfl/modbus`、`github.com/hsblhsn/queues`、`gopkg.in/logex.v1`。`CleintTCP` 是作者把 Client 拼錯，可當樣本身分佐證。

作者沒有自己實作 Modbus，是直接包一個開源函式庫，DWARF 還留著開發者的建置路徑 `C:/Users/Hiro Kirashi/Documents/Projects/Golang/go_modbus/CleintTCP/main.go`，含行號（`main` 在 437、`MbConfig.write` 在 116、`Task.taskWorker` 在 251）。

`main.Cmd` 的欄位即 CLI 介面：`ip`、`inputTask`、`inputList`、`inputTarget`、`cycle`、`output`、`mode`、`address`、`count`、`value`、`threads`、`timeout`、`try`、`debug`、`history`，單目標走 `-ip`，多目標與排程走 `-inputTask`/`-inputList`/`-inputTarget`/`-cycle` 指向的 JSON 檔，結果經 `-output` 寫出，JSON schema 的欄位有 `Ip`、`Code`、`Address`、`Count`、`Value`、`State`、`Tasks`、`Iplist`、`Targetlist`、`StartTime`、`WorkTime`、`PeriodTime`、`IntervalTime`。

`main.main` 對 `mode` 字串的解析只有三條分支：`write` 給 `Code = 6`、`write-m` 給 `Code = 0x10`，其餘一律 `Code = 3`，`main.Task.taskWorker` 依 `Code` 分派到 `MbConfig.read`(FC03)、`MbConfig.write`(FC06)、`MbConfig.writeMultiple`(FC16)，落到 default 則什麼都不做。

**FrostyGoop 只能操作 holding register，完全沒有 coil 能力。** `rolfl/modbus` 函式庫裡雖然編進了 `ReadCoils`/`WriteSingleCoil` 等符號，但 `main` 從未呼叫。這直接限制了忠實重寫版能走的攻擊路徑，見下節。

### PLC 邏輯（`GRFICSv3/plc/st_files/326339.st`，`active_program` 確認為使用中）

掃描週期 `TASK task0(INTERVAL := T#20ms)`，是 20ms 不是常見假設的 50ms。

兩個真實聯鎖，決定攻擊步驟必須有順序：

- `IF manual_mode THEN` 包住 `f1_manual_sp` 等四個手動設定值的套用。未先將 coil 0（`manual_mode` AT %QX0.0）置位，寫入 HR10-13 完全無效。
- `IF NOT run_bit THEN` 強制排放閥全開（`purge_valve_sp := 65535`）。`run_bit`（%QX5.0）為 FALSE 時壓力會被洩掉，攻擊直接中和。攻擊前必須確認其為 TRUE。

### 暫存器行為分三層（「攻擊者為何選這個不選那個」的素材）

| 位址             | 變數                                           | 行為                                                                                                                |
| ---------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| coil 0 + HR10-13 | `manual_mode`, `f1/f2/purge/product_manual_sp` | 寫入持久，直接接管閥門開度。但 manual_mode 旗標在 HMI 上可見                                                        |
| HR1026 (%MW2)    | `pressure_sp`                                  | 全程只被 `LIMIT()` 夾值、從未被重新賦值。已實測寫入持久，且不需切 manual_mode，由自動控制迴路自己把壓力推過破壞門檻 |
| HR1024 (%MW0)    | `product_flow_setpoint`                        | 第 227 行 `product_flow_setpoint := 30000;` 無條件執行，寫入不留存                                                  |

同一台 PLC 上有些位址寫了就穩、有些寫了就被吃掉，這是惡意程式需要先讀取偵察的直接理由，也對應 FrostyGoop 先 FC03 讀再寫的行為模式。

### 協定覆蓋範圍決定攻擊性質（本專題最主要的技術結論）

樣本只有 FC03/06/16，碰不到 coil。這限制了它能做的攻擊種類：切換 `manual_mode` 需要 FC05，做不到，所以真實 FrostyGoop 在這台 PLC 上唯一能走的是 HR1026（`pressure_sp`）: holding register、FC06 可寫、不需 manual_mode，由自動控制迴路自己把壓力推上去，製程表面上全程正常。

我們的重寫版刻意補上 FC01/FC05，於是多開了第二類攻擊：置位 coil 0 繞過整個控制迴路，閥門開度直接由攻擊者指定（即現行 demo 的路徑）。

**同一台 PLC、同一支惡意程式家族，兩種性質完全不同的攻擊，差別純粹來自協定覆蓋範圍。** 間接經控制迴路生效、與直接接管執行器，是兩個不同的威脅等級。這是簡報的主論證，兩條路徑台上都打得出來。

**兩條路徑都已從乾淨初始狀態實測成立。** HR1026 路徑的完整量測記錄見下節。

### HR1026 路徑實測記錄（2026-07-25，從重啟 `plc` 的乾淨狀態起算）

壓力的工程單位換算為 `kPa = raw / 65535 * 3200`（出自 `simulation/remote_io/modbus/tank.py:10`），物理模型本身在 `TE_process.cc:232` 把壓力夾在 3200 kPa。正常運轉的 `pressure_sp` 是 55295 raw，即 2700 kPa。

乾淨狀態基線：壓力穩定在 55295 附近，purge 閥持續調節維持，`manual_mode = 0`。

單一個 FC06 寫入 HR1026 = 65535（3200 kPa）之後，purge 閥維持全關，壓力以每秒約 35 raw 單位單調爬升，約三到四分鐘後達到 63583 raw = **3104.7 kPa，超過 GRFICS 的 3000 kPa 破壞門檻**。全程 `manual_mode` 維持 0，未曾送出任何 coil 操作。

回滾同樣實測成立：把 55295 寫回 HR1026，purge 閥立刻全開，壓力回落。這驗證了上面「失敗則將原值寫回」的序列設計。

值得注意的副作用：ST 第 197 行的 `pressure_override()` 本來是壓力升高時降低產品流量的安全響應，但第 227 行無條件把 `product_flow_setpoint` 重設為 30000，等於把它抵銷掉。這是 PLC 程式自身的邏輯缺陷，不是攻擊造成的。

爬升速率也是 demo 節奏的資訊：**隱蔽的路徑同時也是慢的路徑。** 直接接管執行器快而明顯，改 setpoint 慢而安靜，這是真實的權衡而不是修辭。

> 簡單來說，一個把安全控制器關掉，然後讓他一直上升到爆炸。另一個就是把安全控制器的安全閥值設定到我們要的目標值。

### 偵測盲區

`router` 容器內 Suricata 持續運行，已載入 Digital Bond Quickdraw SCADA 規則集（58 條 alert），其中包含 `SCADA_IDS: Modbus TCP - Unauthorized Write Request to a PLC`，正對應我們的攻擊行為。

該規則永遠不會觸發，兩個獨立的結構性原因：

- Suricata 以 `-i eth2` 啟動，router 上 eth2 是 `192.168.90.x`（c-dmz-net），只監聽 DMZ 側。
- 即使改監聽 eth1 也一樣看不到。implant（192.168.95.3）與 plc（192.168.95.2）位於同一網段，流量經 Linux bridge 二層直送，封包不經過 router。

這是真實 OT 環境最典型的偵測盲區：IDS 部署於 IT/OT 邊界只看南北向，攻擊者一旦進入 OT 網段，東西向流量完全隱形。論述重點是「規則有效、只是感測器看不到」，不是「我們繞過了 IDS」。

### Binary 投遞前提（已確認可行）

implant 容器 `ot-workstation-implant`：uid 0、x86_64、Debian 13 glibc、`/tmp` 可寫且 rootfs 無 `noexec`。host 有 Go 1.26。`CGO_ENABLED=0 GOOS=linux GOARCH=amd64` 靜態編譯即可執行。已用一支 hello-world 靜態 ELF 走完整條投遞鏈實測通過（見 7/25 checklist），FrostyGoop 重寫版走同一條路。

投遞坑：這個 Poseidon build 沒有原生 `chmod` command（GraphQL command table 是 payload type 層級的全集，實際 build 未必全編入，agent 對未編入的 command 回 `Unknown command`）。`upload`/`shell` 有、`chmod` 沒有，所以設執行位元要走 `shell chmod +x`。scripting 走 nginx `127.0.0.1:8080`（TLS 入口），不是 `MYTHIC_SERVER_PORT` 17443。

## 交付項目

### 實作

- **Go binary（FrostyGoop 重寫版）** CLI 介面對齊逆向出的 `main.Cmd`：`-ip -mode -address -value -count -threads -timeout -try -output -debug`。實作樣本原有的 FC03/FC06/FC16，**外加樣本沒有的 FC01/FC05**。`-mode` 保留樣本的 `write`/`write-m` 語義，另增 `read-coil`/`write-coil` 兩個值走新增的 coil 路徑，讓「哪些能力是原有的、哪些是我們加的」在 CLI 層面就看得出來。
- **原子序列編排** 兩條序列各一組。coil 路徑：FC01 讀 `run_bit` 確認為 TRUE -> FC05 置位 `manual_mode` -> FC16 一次寫入 HR10-13 -> FC01/FC03 讀回驗證 -> 失敗則將 `manual_mode` 寫回 0 回滾。holding register 路徑：FC03 讀 HR1026 存下原值 -> FC06 寫入新 setpoint -> FC03 讀回驗證 -> 失敗則將原值寫回。兩者都沿用樣本 `Task.taskWorker` 已有的 read/dispatch/回報結構，不是外加的框架。
- **JSON 任務清單與排程（可選，視進度）** 樣本的 `-inputTask`/`-cycle` 機制。做得完就做，因為它是「接上 C2 之後 operator 下發什麼」最自然的載體；來不及就只留單目標 flag 路徑。
- **多目標介面（降級）** 接受逗號分隔多目標並依序處理。環境只有一台 PLC，不宣稱已驗證規模化，僅呈現設計意圖。

### 量測與素材

- **暫存器三層行為實驗**: 對 HR10-13 / HR1026 / HR1024 分別寫入後高頻讀回，記錄留存與否。
- **掃描週期論述**: 20ms 出自原始碼 `T#20ms`。可展示的實測結論是「寫入 HR1024 的值不留存，一個掃描週期內即消失」，不宣稱量出精確毫秒數。
- **Suricata 離線重放**: host bridge 側錄後餵給 router 內 Suricata，產出 `fast.log` 對照。
- **三欄比較表**: 真實 FrostyGoop 逆向結果 / 重寫版 / 差異與原因。已知的刻意差異：新增 FC01/FC05 coil 能力（開啟直接接管執行器的第二類攻擊，是本專題「改進」的實質內容）、平台從 Windows PE 換成 Linux ELF（環境如此，與功能無關）、自行實作 Modbus 而非包 `rolfl/modbus`、接上 C2 tasking 取代人工投放與人工回收 log。
- **封包並排**: 課程 PDF 那組 FC06 request/response 截圖（address 87、value 88）對重寫版打同一組參數的側錄。兩邊 function code、reference number、register value 應逐欄相符。

### 文件與簡報

- ATT&CK for ICS 對應（僅用 T0836 Modify Parameter、T0855 Unauthorized Command Message、T0831 Manipulation of Control，其餘編號未經驗證不要寫）。
- 更新 `ATTACK_WALKTHROUGH.md` Phase 6，以 Go binary 取代 base64 Python payload。
- 未來展望：轉譯容器架構、與 PIPEDREAM/INCONTROLLER 的能力落差、以及本次發現的網路拓樸限制本身。

## 時程

簡報 7/30 14:00 繳交、demo 20:30。**簡報裡的每一個數字都必須在 7/29 前量測完成**，7/30 只留修字與彩排，不做新實驗。

### 7/25

- [x] 確認樣本與逆向素材在手：`lab/FrostyGoop/` 有樣本與自製 `fake_modbus_slave.py`，Ghidra 專案已載入，封包對照素材取自課程 PDF 的 Wireshark 截圖（`-ip 127.0.0.1 -mode write -address 87 -value 88` 那組 FC06 request/response）
- [x] 決定 Go 程式碼落點（`frostygoop-rewrite/`），同時更新 `CLAUDE.md` 的 git 規則。目錄樹與可編譯骨架實際建立於 7/26，見 `team/README.md`
- [x] `lab/` 加進 `.gitignore`（原本既未忽略也未追蹤，一個 `git add .` 就會把活體樣本推上 GitHub）
- [x] 實測 HR1026（`pressure_sp`）路徑成立，壓力達 3104.7 kPa 且 `manual_mode` 全程為 0，回滾亦成立
- [x] 確認復原方式：重啟 `plc` 會把 `manual_mode` 歸 0、HR10-13 回到 ST 宣告初值，但會打斷 simulation 的 Modbus 連線，**必須接著重啟 `simulation`**
- [x] hello-world binary 走完完整投遞鏈：`register_file` -> Poseidon `upload`（1597602 bytes 落地 `/tmp/hello`）-> `shell chmod +x` -> `shell` 執行，implant 回傳 `host=db74400484a4 uid=0 goarch=amd64`。全程走 operator API，未用 docker exec 捷徑。關鍵路徑已綠

### 7/26

- [ ] Go binary 骨架：flag 解析對齊 `main.Cmd`、MBAP header 組裝、連線管理
- [ ] FC03/FC06/FC16 實作，先對 `fake_modbus_slave.py` 驗證（它剛好也只實作這三個），再對 `plc:502` 驗證
- [ ] FC01/FC05 實作（新增能力，共用同一套 MBAP/PDU 組裝，coil 值為 `0xFF00`/`0x0000`）
- [ ] 暫存器三層行為實驗，記錄結果

### 7/27

- [ ] 兩條原子序列：coil 路徑與 holding register 路徑，各含前置檢查、讀回驗證、回滾
- [ ] 完整攻擊經 Mythic tasking 跑通，壓力衝到 3000+ kPa
- [ ] 流量側錄：Python payload 版與 Go binary 版各一份

### 7/28（code freeze 20:00）

- [ ] Suricata 離線重放，產出兩版 `fast.log` 對照
- [ ] 三欄比較表與封包並排素材整理完成
- [ ] 更新 `ATTACK_WALKTHROUGH.md` Phase 6
- [ ] freeze，之後只修 bug 不加功能

### 7/29

- [ ] 所有簡報數字定稿
- [ ] 簡報製作完成
- [ ] 完整彩排並計時，確認塞得進 15 分鐘
- [ ] 跑一次下方 demo 前檢查清單

### 7/30

- [ ] 上午簡報最後修訂
- [ ] 14:00 繳交
- [ ] 20:30-20:45 demo

## Demo 前檢查清單

- **`192.168.95.1` 位址衝突**: host bridge `br-ce2d1beff6dd` 與 router 的 eth1 secondary 都是 `192.168.95.1`，而 C2 callback 正好打這個位址。目前 ARP 由 host 勝出所以正常，但重啟 router 或 implant 後可能翻轉，callback 會安靜失敗（連到 router 的 80）。以 `docker exec ot-workstation-implant cat /proc/net/arp` 確認對應 MAC。
- **UFW**: 確認 bridge 到 host 的 allow rule 仍在，否則 callback 逾時而非拒絕連線。
- **JWT**: Mythic operator 介面 token 會過期，demo 前重新登入。
- **鍵盤 E**: GRFICS 3D 介面按 E 會觸發 e-stop，demo 時避免焦點停在該視窗。
- **重置順序是 `plc` 然後 `simulation`，不能只重啟後者**: `restart simulation` 只重置物理模型，PLC 會繼續握著 `manual_mode` 與被改過的設定值（實測過一次開場就是 `manual_mode = 1`、HR10-13 還是上次攻擊值，代表過去每次 demo 的初始狀態都是未知的）。反過來只重啟 `plc` 會打斷 simulation 的 Modbus 連線，壓力讀值卡在 65535 飽和。兩者都重啟、`plc` 先，重連約需 30 秒，之後壓力還要幾分鐘才爬回 2700 kPa 基線。彩排時把這段時間算進去。

## 不做的事

- **協定轉換層**: 環境只有 Modbus 一種協定，沒有第二種語義需要互轉，agent 直接構造封包已可行。
- **獨立轉譯容器**: Mythic 的 Translation Container 在架構上負責 server 與 agent 之間的訊息格式轉換，兩端都不是攻擊目標；對目標發協定封包在設計上屬於 agent 職責。且該容器會生在 `mythic_default` 網段，與 `plc` 所在的 `b-ics-net` 不通，要通就得在 host 手動 `docker network connect`，那是架設者權限而非攻擊者能力。原提案的能力需求本身成立，全部改為實作在 agent 側。
- **掃描週期時序控制**: 攻擊路徑上的 `manual_mode` 與 %QW10-13 在整支程式中只被讀取、從未被寫入，不存在邏輯覆蓋，沒有需要贏的競速。
- **流量規避工程**: 改為量測盲區。不宣稱「零高危告警」。
- **多 PLC 規模化驗證**: 環境只有一台。
