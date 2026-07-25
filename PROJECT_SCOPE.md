# 專題範圍與工作分解

AIS3 2026 專題「OT/ICS 惡意程式及 C2 實作」。簡報繳交 2026-07-30 14:00，現場 demo 20:30-20:45。

## 專題定位

### FrostyGoop 逆向、重實作並接上 C2

FrostyGoop 本身是獨立 CLI 工具，攻擊者取得立足點後手動執行、打完結束，沒有回連也沒有持續控制。我們根據自己逆向出的程式邏輯重新實作功能對等版本，接上 Mythic 完整 C2 生命週期（payload build、callback、tasking、operator 介面），實際打進 GRFICS 讓反應器壓力失控。

雖然原本在 C2 我們就可以透過 root shell 做到遠端執行攻擊，但它不知道 Modbus 是什麼、不知道哪個暫存器寫下去有用；惡意程式封裝的是攻擊者對目標領域的知識。真實 FrostyGoop 攻擊者當時同樣早有立足點。

因為手上有原始樣本的逆向結果與封包側錄，可以做逐項對照：真實行為 / 重寫版行為 / 差異與原因，以及兩者封包並排比較。

## 已驗證的環境事實

以下全部實際讀原始碼或量測得出，非理論推估。簡報可直接引用。

### PLC 邏輯（`GRFICSv3/plc/st_files/326339.st`，`active_program` 確認為使用中）

掃描週期 `TASK task0(INTERVAL := T#20ms)`，是 20ms 不是常見假設的 50ms。

兩個真實聯鎖，決定攻擊步驟必須有順序：

- `IF manual_mode THEN` 包住 `f1_manual_sp` 等四個手動設定值的套用。未先將 coil 0（`manual_mode` AT %QX0.0）置位，寫入 HR10-13 完全無效。
- `IF NOT run_bit THEN` 強制排放閥全開（`purge_valve_sp := 65535`）。`run_bit`（%QX5.0）為 FALSE 時壓力會被洩掉，攻擊直接中和。攻擊前必須確認其為 TRUE。

### 暫存器行為分三層（「攻擊者為何選這個不選那個」的素材）

| 位址 | 變數 | 行為 |
| --- | --- | --- |
| coil 0 + HR10-13 | `manual_mode`, `f1/f2/purge/product_manual_sp` | 寫入持久，直接接管閥門開度。但 manual_mode 旗標在 HMI 上可見 |
| HR1026 (%MW2) | `pressure_sp` | 全程只被 `LIMIT()` 夾值、從未被重新賦值，寫入應持久。且不需切 manual_mode，透過自動控制迴路生效，製程表面上仍在自動模式（待實測） |
| HR1024 (%MW0) | `product_flow_setpoint` | 第 227 行 `product_flow_setpoint := 30000;` 無條件執行，寫入不留存 |

同一台 PLC 上有些位址寫了就穩、有些寫了就被吃掉，這是惡意程式需要先讀取偵察的直接理由，也對應 FrostyGoop 先 FC03 讀再寫的行為模式。

### 偵測盲區

`router` 容器內 Suricata 持續運行，已載入 Digital Bond Quickdraw SCADA 規則集（58 條 alert），其中包含 `SCADA_IDS: Modbus TCP - Unauthorized Write Request to a PLC`，正對應我們的攻擊行為。

該規則永遠不會觸發，兩個獨立的結構性原因：

- Suricata 以 `-i eth2` 啟動，router 上 eth2 是 `192.168.90.x`（c-dmz-net），只監聽 DMZ 側。
- 即使改監聽 eth1 也一樣看不到。implant（192.168.95.3）與 plc（192.168.95.2）位於同一網段，流量經 Linux bridge 二層直送，封包不經過 router。

這是真實 OT 環境最典型的偵測盲區：IDS 部署於 IT/OT 邊界只看南北向，攻擊者一旦進入 OT 網段，東西向流量完全隱形。論述重點是「規則有效、只是感測器看不到」，不是「我們繞過了 IDS」。

### Binary 投遞前提（已確認可行）

implant 容器 `ot-workstation-implant`：uid 0、x86_64、Debian 13 glibc、`/tmp` 可寫且 rootfs 無 `noexec`。host 有 Go 1.26。`CGO_ENABLED=0 GOOS=linux GOARCH=amd64` 靜態編譯即可執行。

## 交付項目

### 實作

- **Go binary（FrostyGoop 重寫版）** CLI 介面對齊真實樣本（`-ip -mode read|write -address -value -count -threads -cycle`），實作 FC01/FC03/FC05/FC06/FC16。
- **原子序列編排** 單一 task 展開為：確認 `run_bit` -> 置位 `manual_mode` -> 依序寫 HR10-13 -> FC01/FC03 讀回驗證 -> 任一步失敗則將 `manual_mode` 寫回 0 回滾。
- **多目標介面（降級）** 接受逗號分隔多目標並依序處理。環境只有一台 PLC，不宣稱已驗證規模化，僅呈現設計意圖。

### 量測與素材

- **暫存器三層行為實驗** — 對 HR10-13 / HR1026 / HR1024 分別寫入後高頻讀回，記錄留存與否。
- **掃描週期論述** — 20ms 出自原始碼 `T#20ms`。可展示的實測結論是「寫入 HR1024 的值不留存，一個掃描週期內即消失」，不宣稱量出精確毫秒數。
- **Suricata 離線重放** — host bridge 側錄後餵給 router 內 Suricata，產出 `fast.log` 對照。
- **三欄比較表** — 真實 FrostyGoop 逆向結果 / 重寫版 / 差異與原因。
- **封包並排** — 真實樣本 pcap 與重寫版側錄對照。

### 文件與簡報

- ATT&CK for ICS 對應（僅用 T0836 Modify Parameter、T0855 Unauthorized Command Message、T0831 Manipulation of Control，其餘編號未經驗證不要寫）。
- 更新 `ATTACK_WALKTHROUGH.md` Phase 6，以 Go binary 取代 base64 Python payload。
- 未來展望：轉譯容器架構、與 PIPEDREAM/INCONTROLLER 的能力落差、以及本次發現的網路拓樸限制本身。

## 時程

簡報 7/30 14:00 繳交、demo 20:30。**簡報裡的每一個數字都必須在 7/29 前量測完成**，7/30 只留修字與彩排，不做新實驗。

### 7/25（今天，剩半天）

- [ ] 確認團隊的 FrostyGoop 逆向筆記與真實樣本 pcap 確實在手（三欄比較表與封包並排完全依賴這批素材，若不完整今天就要重新規劃這兩張投影片）
- [ ] 決定 Go 程式碼落點並建目錄，同時更新 `CLAUDE.md` 的 git 規則
- [ ] hello-world binary 走完完整投遞鏈：Poseidon `upload` -> `chmod +x` -> 執行（這條路從沒測過，是關鍵路徑。今天失敗還有 `shell python3` 可退）

### 7/26

- [ ] Go binary 骨架：flag 解析、MBAP header 組裝、連線管理
- [ ] FC01/FC03/FC05/FC06/FC16 實作，逐一對 `plc:502` 驗證
- [ ] 暫存器三層行為實驗，記錄結果

### 7/27

- [ ] 原子序列：前置檢查、順序寫入、讀回驗證、回滾
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

- **`192.168.95.1` 位址衝突**：host bridge `br-ce2d1beff6dd` 與 router 的 eth1 secondary 都是 `192.168.95.1`，而 C2 callback 正好打這個位址。目前 ARP 由 host 勝出所以正常，但重啟 router 或 implant 後可能翻轉，callback 會安靜失敗（連到 router 的 80）。以 `docker exec ot-workstation-implant cat /proc/net/arp` 確認對應 MAC。
- **UFW**：確認 bridge 到 host 的 allow rule 仍在，否則 callback 逾時而非拒絕連線。
- **JWT**：Mythic operator 介面 token 會過期，demo 前重新登入。
- **鍵盤 E**：GRFICS 3D 介面按 E 會觸發 e-stop，demo 時避免焦點停在該視窗。
- **重置**：`docker compose -f GRFICSv3/docker-compose.yml restart simulation`，彩排時計時確認可接受。

## 不做的事

- **協定轉換層** — 環境只有 Modbus 一種協定，沒有第二種語義需要互轉，agent 直接構造封包已可行。
- **獨立轉譯容器** — Mythic 的 Translation Container 在架構上負責 server 與 agent 之間的訊息格式轉換，兩端都不是攻擊目標；對目標發協定封包在設計上屬於 agent 職責。且該容器會生在 `mythic_default` 網段，與 `plc` 所在的 `b-ics-net` 不通，要通就得在 host 手動 `docker network connect`，那是架設者權限而非攻擊者能力。原提案的能力需求本身成立，全部改為實作在 agent 側。
- **掃描週期時序控制** — 攻擊路徑上的 `manual_mode` 與 %QW10-13 在整支程式中只被讀取、從未被寫入，不存在邏輯覆蓋，沒有需要贏的競速。
- **流量規避工程** — 改為量測盲區。不宣稱「零高危告警」。
- **多 PLC 規模化驗證** — 環境只有一台。
