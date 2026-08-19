## 誰做什麼

| 工作流         | 文件                | 內容                                                       |
| -------------- | ------------------- | ---------------------------------------------------------- |
| 協定層(我負責的)| `PROTOCOL.md`       | Go binary 的 CLI 與 Modbus wire format，六個 function code |
| 序列與 C2      | `SEQUENCE_C2.md`    | 兩個攻擊手法(takeover、setpoint)、Mythic 投遞與 tasking                    |
| 逆向核對與量測 | `RE_MEASUREMENT.md` | Ghidra 報告複核、暫存器行為實驗、Suricata 側錄             |
| 簡報與文件     | `SLIDES.md`         | 三欄比較表、ATT&CK 對應、簡報與彩排                        |

四條線可同時開工，唯一的相依是協定層要先交出 `internal/modbus` 的可用實作，序列層才能真的跑起來

## 程式碼落點與檔案所有權

```
frostygoop-rewrite/
  go.mod                        module github.com/abb00717/frostygoop-rewrite
  cmd/frostygoop/main.go        [協定層] flag 解析、mode 分派
  internal/modbus/conn.go       [協定層] 連線、逾時、重試、MBAP 組裝
  internal/modbus/client.go     [協定層] 六個 function code
  internal/attack/addr.go       [序列層] PLC 位址常數
  internal/attack/coil.go       [序列層] coil 接管序列
  internal/attack/setpoint.go   [序列層] pressure setpoint 序列
  internal/attack/report.go     [序列層] 結果結構與 JSON 輸出
  testdata/fake_slave.py        [協定層] 本機測試用假 PLC，支援全部六個 FC
```

**一個檔案只有一個負責人** 需要動不屬於自己的檔案時，先講一聲，不要直接改，Go 版本用 host 上的 1.26.0，建置目標固定 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`

## 介面契約

協定層負責產出下面這組簽章，序列層照這組寫

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
func (c *Conn) ReadInput(addr, count uint16) ([]uint16, error)  // FC04
func (c *Conn) WriteCoil(addr uint16, on bool) error              // FC05
func (c *Conn) WriteSingle(addr, value uint16) error              // FC06
func (c *Conn) WriteMultiple(addr uint16, values []uint16) error  // FC16
```

重試與逾時包在 `Conn` 裡，序列層不要自己再實作一層，Modbus exception response 回成
Go 的 `error`，序列層據此判斷成敗、決定要不要回滾

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

## `-mode` 對應表


mode 分兩層，**primitive** 只送一個 function code，對應樣本用 mode 字串挑 Code 的做法，
**sequence** 跑一整套含前置檢查、讀回驗證與回滾的序列，是我們才有的

| `-mode`      | 行為                      | 層級      | 來源     |
| ------------ | ------------------------- | --------- | -------- |
| `read`       | FC03                      | primitive | 樣本原有 |
| `write`      | FC06                      | primitive | 樣本原有 |
| `write-m`    | FC16                      | primitive | 樣本原有 |
| `read-coil`  | FC01                      | primitive | 我們新增 |
| `read-input` | FC04                      | primitive | 我們新增 |
| `write-coil` | FC05                      | primitive | 我們新增 |
| `takeover`   | `attack.CoilTakeover`     | sequence  | 我們新增 |
| `setpoint`   | `attack.PressureSetpoint` | sequence  | 我們新增 |

`write-coil` 是單一個 FC05，**不會**幫你做 `run_bit` 前置檢查或回滾，要完整序列請用
`takeover`


## 進度同步

每天收工前各自在群組回報一句：做完什麼、卡在哪、有沒有東西擋到別人，重點是第三項

介面契約要改（例如發現某個簽章不夠用），改之前先講，因為它同時綁著兩個人，改完更新這
份文件，不要只在群組講完就算數

7/28 20:00 code freeze，之後只修 bug 不加功能
