# 工作流：協定層

負責 Go binary 的 Modbus 實作與 CLI 外殼

## 先做 wire format，flag 表最後做

wire format 現在就寫，flag 名稱等複核結果，複核由逆向與量測那條線負責

`main.go` 的骨架已經有一組暫定 flag 可以跑，而 `-mode`的完整對應表已經在
`README.md` 定好了，那幾列是我們自己的設計，可以照著實作

## 要實作的五個 function code

樣本原本只有 FC03/06/16，FC01/FC04/FC05 是我們刻意加的能力，也是整個專題重寫版比原版多做
了什麼的實質內容，不是順手加的，六個共用同一套 MBAP header 與 PDU 組裝

| FC   | 名稱                     | 來源     |
| ---- | ------------------------ | -------- |
| 0x01 | Read Coils               | 我們新增 |
| 0x03 | Read Holding Registers   | 樣本原有 |
| 0x04 | Read Input Registers       | 我們新增 |
| 0x05 | Write Single Coil        | 我們新增 |
| 0x06 | Write Single Register    | 樣本原有 |
| 0x16 | Write Multiple Registers | 樣本原有 |

MBAP header 是 `transaction_id, protocol_id, length, unit_id` 四個欄位接在 PDU 前面，
big-endian，`protocol_id` 固定 0，FC05 的值只有兩個合法值：`0xFF00` 開、`0x0000` 關，不
是 1/0

回應要處理 exception：function code 最高位被設起來（例如請求 0x06、回應 0x86）時，後面
一個 byte 是 exception code，這種情況回 Go 的 `error`，不要當成功，序列層靠這個判斷要不
要回滾

## 測試用假 PLC

`testdata/fake_slave.py` 已經在 repo 裡，**它只實作 FC03/06/16，你要自己補上 FC01/FC04/FC05**，大概三十行的事，它要多維護一張
coil 表，跟現有的 holding register 表平行

補完之後你整條開發流程都在本機打假 PLC，不需要碰 GRFICS

## 完成的判準

- 六個 FC 對 `testdata/fake_slave.py` 都能正確收送，包含 exception 路徑
- `-timeout` / `-try` 真的生效（拔掉假 PLC 應該看到重試然後逾時，不是直接崩）
- `-debug` 印得出收送的 hex frame，這個之後做封包並排比對會用到
- `go build` 出來的靜態 binary 能在 implant 容器裡跑（`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`）

最後一項別留到最後才驗，投遞鏈已經用 hello-world 靜態 ELF 走通過，所以只要編譯參數對就
不會有意外，但還是早點確認比較好
