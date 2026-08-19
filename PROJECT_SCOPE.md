# 專題範圍與工作分解

AIS3 2026 專題 OT/ICS 惡意程式及 C2 實作

## 專題定位

### FrostyGoop 逆向、重實作並接上 C2

FrostyGoop 本身是獨立 CLI 工具，攻擊者取得立足點後手動執行，沒有回連也沒有持續控制，我們根據自己逆向出的程式邏輯重新實作功能對等版本，接上 Mythic C2，實際打進 GRFICS 讓反應器壓力失控

雖然原本在 C2 我們就可以透過 root shell 做到遠端執行攻擊，但它不知道 Modbus 是什麼、不知道哪個暫存器寫下去有用，惡意程式封裝的是攻擊者對目標領域的知識，真實 FrostyGoop 攻擊者當時同樣早有立足點

因為手上有原始樣本的逆向結果與封包側錄，可以做逐項對照包括真實行為、重寫版行為、差異與原因，以及兩者封包並排比較

## 已驗證的環境事實

### FrostyGoop 樣本事實

`file` 判定為 PE32+ Windows x86-64 console，`go version -m` 直接吐出完整 build metadata：go1.20.4、`CGO_ENABLED=0`、`GOARCH=amd64`、`GOOS=windows`、module path `github.com/rolfl/modbus/CleintTCP`，相依 `github.com/rolfl/modbus`、`github.com/hsblhsn/queues`、`gopkg.in/logex.v1`，`CleintTCP` 是作者把 Client 拼錯，可當樣本身分佐證

作者沒有自己實作 Modbus，是直接包一個開源函式庫，DWARF 還留著開發者的建置路徑 `C:/Users/HiroKirashi/Documents/Projects/Golang/go_modbus/CleintTCP/main.go`

`main.Cmd` 的欄位即 CLI 介面：`ip`、`inputTask`、`inputList`、`inputTarget`、`cycle`、`output`、`mode`、`address`、`count`、`value`、`threads`、`timeout`、`try`、`debug`、`history`，單目標走 `-ip`，多目標與排程走 `-inputTask`/`-inputList`/`-inputTarget`/`-cycle` 指向的 JSON 檔，結果經 `-output` 寫出，JSON schema 的欄位有 `Ip`、`Code`、`Address`、`Count`、`Value`、`State`、`Tasks`、`Iplist`、`Targetlist`、`StartTime`、`WorkTime`、`PeriodTime`、`IntervalTime`

`main.main` 對 `mode` 字串的解析只有三條分支：`write` 給 `Code = 6`、`write-m` 給 `Code = 0x10`，其餘一律 `Code = 3`，`main.Task.taskWorker` 依 `Code` 分派到 `MbConfig.read`(FC03)、`MbConfig.write`(FC06)、`MbConfig.writeMultiple`(FC16)，落到 default 則什麼都不做

**FrostyGoop 只能操作 holding register，完全沒有 coil 能力** `rolfl/modbus` 函式庫裡雖然編進了 `ReadCoils`/`WriteSingleCoil` 等符號，但 `main` 從未呼叫

### PLC 邏輯（`GRFICSv3/plc/st_files/326339.st`）

掃描週期 `TASK task0(INTERVAL := T#20ms)`，是 20ms 不是常見假設的 50ms

兩個真實聯鎖，決定攻擊步驟必須有順序：

- `IF manual_mode THEN` 包住 `f1_manual_sp` 等四個手動設定值的套用，未先將 coil 0 置位，寫入 HR10-13 完全無效
- `IF NOT run_bit THEN` 強制排放閥全開（`purge_valve_sp := 65535`） `run_bit` 為 FALSE 時壓力會被洩掉，攻擊直接中和，攻擊前必須確認其為 TRUE

### 暫存器行為分三層

| 位址             | 變數                                           | 行為                                                                                                                |
| ---------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| coil 0 + HR10-13 | `manual_mode`, `f1/f2/purge/product_manual_sp` | 寫入持久，直接接管閥門開度，但 manual_mode 旗標在 HMI 上可見                                                        |
| HR1026 (%MW2)    | `pressure_sp`                                  | 全程只被 `LIMIT()` 夾值、從未被重新賦值，已實測寫入持久，且不需切 manual_mode，由自動控制迴路自己把壓力推過破壞門檻 |
| HR1024 (%MW0)    | `product_flow_setpoint`                        | 第 227 行 `product_flow_setpoint := 30000;` 無條件執行，寫入不留存                                                  |

同一台 PLC 上有些位址寫了就穩、有些寫了就被吃掉，這是惡意程式需要先讀取偵察的直接理由，也對應 FrostyGoop 先 FC03 讀再寫的行為模式

### 協定覆蓋範圍決定攻擊性質

樣本只有 FC03/06/16，碰不到 coil，這限制了它能做的攻擊種類：切換 `manual_mode` 需要 FC05，做不到，所以真實 FrostyGoop 在這台 PLC 上唯一能走的是 HR1026（`pressure_sp`）: holding register、FC06 可寫、不需 manual_mode，由自動控制迴路自己把壓力推上去，製程表面上全程正常

我們的重寫版刻意補上 FC01/FC05，於是多開了第二類攻擊：置位 coil 0 繞過整個控制迴路，閥門開度直接由攻擊者指定

**同一台 PLC、同一支惡意程式家族，兩種性質完全不同的攻擊，差別純粹來自協定覆蓋範圍** 間接經控制迴路生效與直接接管執行器，是兩個不同的威脅等級

### HR1026 路徑實測記錄

壓力的工程單位換算為 `kPa = raw / 65535 * 3200`（出自 `simulation/remote_io/modbus/tank.py:10`），物理模型本身在 `TE_process.cc:232` 把壓力夾在 3200 kPa，正常運轉的 `pressure_sp` 是 55295 raw，即 2700 kPa

壓力穩定在 55295 附近，purge 閥持續調節維持，`manual_mode = 0`

單一個 FC06 寫入 HR1026 = 65535（3200 kPa）之後，purge 閥維持全關，壓力以每秒約 35 raw 單位單調爬升，約三到四分鐘後達到 63583 raw = **3104.7 kPa，超過 GRFICS 的 3000 kPa 破壞門檻**

回滾同樣實測成立：把 55295 寫回 HR1026，purge 閥立刻全開，壓力回落

值得注意的副作用：ST 第 197 行的 `pressure_override()` 本來是壓力升高時降低產品流量的安全響應，但第 227 行無條件把 `product_flow_setpoint` 重設為 30000，等於把它抵銷掉，這是 PLC 程式自身的邏輯缺陷，不是攻擊造成的

爬升速率也是 demo 節奏的資訊：**隱蔽的路徑同時也是慢的路徑** 直接接管執行器快而明顯，改 setpoint 慢而安靜

> 簡單來說，setpoint把安全控制器關掉，然後讓他一直上升到爆炸，takeover是把安全控制器的安全閥值設定到我們要的目標值

### 偵測盲區

`router` 容器內 Suricata 持續運行，已載入 Digital Bond Quickdraw SCADA 規則集，其中包含 `SCADA_IDS: Modbus TCP - Unauthorized Write Request to a PLC`，正對應我們的攻擊行為

該規則永遠不會觸發，兩個獨立的結構性原因：

- Suricata 以 `-i eth2` 啟動，router 上 eth2 是 `192.168.90.x`（c-dmz-net），只監聽 DMZ 側
- 即使改監聽 eth1 也一樣看不到，implant（192.168.95.3）與 plc（192.168.95.2）位於同一網段，流量經 Linux bridge 二層直送，封包不經過 router

這是 OT 環境最典型的偵測盲區：IDS 部署於 IT/OT 邊界，攻擊者一旦進入 OT 網段，流量完全隱形

### Binary 投遞前提

implant 容器 `ot-workstation-implant`：uid 0、x86_64、Debian 13 glibc、`/tmp` 可寫且 rootfs 無 `noexec`，host 有 Go 1.26，`CGO_ENABLED=0 GOOS=linux GOARCH=amd64` 靜態編譯即可執行，已用一支 hello-world 靜態 ELF 走完整條投遞鏈實測通過，FrostyGoop 重寫版走同一條路

Poseidon build 沒有原生 `chmod` command（GraphQL command table 是 payload type 層級的全集，實際 build 未必全編入，agent 對未編入的 command 回 `Unknown command`），`upload`/`shell` 有、`chmod` 沒有，所以設執行位元要走 `shell chmod +x`，scripting 走 nginx `127.0.0.1:8080`（TLS 入口），不是 `MYTHIC_SERVER_PORT` 17443


### 實作

- **Go binary（FrostyGoop 重寫版）** CLI 介面對齊逆向出的 `main.Cmd`：`-ip -mode -address -value -count -threads -timeout -try -output -debug`，實作樣本原有的 FC03/FC06/FC16，**外加樣本沒有的 FC01/FC05**。`-mode` 保留樣本的 `write`/`write-m` 語義，另增 `read-coil`/`write-coil` 兩個值走新增的 coil 路徑
- **序列層編排** coil 路徑：FC01 讀 `run_bit` 確認為 TRUE -> FC05 置位 `manual_mode` -> FC16 一次寫入 HR10-13 -> FC01/FC03 讀回驗證 -> 失敗則將 `manual_mode` 寫回 0 回滾，holding register 路徑：FC03 讀 HR1026 存下原值 -> FC06 寫入新 setpoint -> FC03 讀回驗證 -> 失敗則將原值寫回，兩者都沿用樣本 `Task.taskWorker` 已有的 read/dispatch/回報結構，不是外加的框架

