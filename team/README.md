# 分工與介面契約

四個人平行作業的共用約定。每個人另有一份自己的工作文件，但**介面只在這份定義一次**，
各角色文件不重複抄寫，避免到 7/28 出現四份互相矛盾的版本。

專題目標、已驗證事實、日程表與 demo 檢查清單一律看 `../PROJECT_SCOPE.md`，這份不重抄。

## 誰做什麼

| 工作流         | 文件                | 內容                                                       |
| -------------- | ------------------- | ---------------------------------------------------------- |
| 協定層         | `PROTOCOL.md`       | Go binary 的 CLI 與 Modbus wire format，五個 function code |
| 序列與 C2      | `SEQUENCE_C2.md`    | 兩條原子攻擊序列、Mythic 投遞與 tasking                    |
| 逆向核對與量測 | `RE_MEASUREMENT.md` | Ghidra 報告複核、暫存器行為實驗、Suricata 側錄             |
| 簡報與文件     | `SLIDES.md`         | 三欄比較表、ATT&CK 對應、簡報與彩排                        |

四條線可同時開工。唯一的硬相依是協定層要先交出 `internal/modbus` 的可用實作，序列層才
能真的跑起來；但序列層可以先照下面的簽章寫殼子，不用等。

## 程式碼落點與檔案所有權

```
frostygoop-rewrite/
  go.mod                        module github.com/abb00717/frostygoop-rewrite
  cmd/frostygoop/main.go        [協定層] flag 解析、mode 分派
  internal/modbus/conn.go       [協定層] 連線、逾時、重試、MBAP 組裝
  internal/modbus/client.go     [協定層] 五個 function code
  internal/attack/addr.go       [序列層] PLC 位址常數
  internal/attack/coil.go       [序列層] coil 接管序列
  internal/attack/setpoint.go   [序列層] pressure setpoint 序列
  internal/attack/report.go     [序列層] 結果結構與 JSON 輸出
  testdata/fake_slave.py        [協定層] 本機測試用假 PLC，支援全部五個 FC
```

**一個檔案只有一個負責人。** 需要動不屬於自己的檔案時，先講一聲，不要直接改，不然 merge
會很痛。Go 版本用 host 上的 1.26.0，建置目標固定 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`。

## 介面契約

協定層負責產出下面這組簽章，序列層照這組寫。**這是兩邊唯一的接觸面。**

```go
package modbus

type Options struct {
    Timeout time.Duration // -timeout, 預設 10s
    Retries int           // -try, 預設 3
    UnitID  byte          // 預設 1
    Debug   bool          // -debug, 印出收送的 hex frame
}

func Dial(addr string, opt Options) (*Conn, error)
func (c *Conn) Close() error

func (c *Conn) ReadCoils(addr, count uint16) ([]bool, error)      // FC01
func (c *Conn) ReadHolding(addr, count uint16) ([]uint16, error)  // FC03
func (c *Conn) WriteCoil(addr uint16, on bool) error              // FC05
func (c *Conn) WriteSingle(addr, value uint16) error              // FC06
func (c *Conn) WriteMultiple(addr uint16, values []uint16) error  // FC16
```

重試與逾時包在 `Conn` 裡，序列層不要自己再實作一層。Modbus exception response 回成
Go 的 `error`，序列層據此判斷成敗、決定要不要回滾。

序列層負責產出：

```go
package attack

type Step struct {
    Name  string   // 例如 "read run_bit"
    FC    byte
    Addr  uint16
    Value []uint16 // 讀取類步驟填讀回值，寫入類填寫入值
    Err   string   // 空字串表示成功
}

type Result struct {
    Steps      []Step
    Success    bool
    RolledBack bool
}

func CoilTakeover(c *modbus.Conn, f1, f2, purge, product uint16) (*Result, error)
func PressureSetpoint(c *modbus.Conn, value uint16) (*Result, error)
```

`cmd/frostygoop/main.go` 由協定層擁有，負責把 `-mode` 字串對到上面兩個序列函式。

## 環境是共用的，只有一台 PLC

GRFICS 只有一台 `plc`，四個人共用。有人在跑寫入測試、另一個人同時在錄暫存器持久性數
據，兩邊的結果都會是垃圾，而且不會有人察覺。

- **重啟 `plc` 前先在群組講一聲。** 重置順序固定 `plc` 然後 `simulation`，只重啟其中一
  個都會壞（理由見 `../PROJECT_SCOPE.md` 的 demo 檢查清單）。重連約 30 秒，壓力爬回
  2700 kPa 基線還要幾分鐘，實際上一次重置要抓五分鐘以上。
- **協定層平常不要碰 PLC。** 用 `testdata/fake_slave.py` 在本機測，那才是它存在的理由。
  只有序列層跑完整鏈路、以及量測工作，才需要真的 PLC。
- 量測工作要獨占環境時，直接說要占多久。

## 進度同步

每天收工前各自在群組回報一句：做完什麼、卡在哪、有沒有東西擋到別人。重點是第三項。

介面契約要改（例如發現某個簽章不夠用），改之前先講，因為它同時綁著兩個人。改完更新這
份文件，不要只在群組講完就算數。

7/28 20:00 code freeze，之後只修 bug 不加功能。
