# 工作流：逆向核對與量測

負責把還沒驗過的逆向報告驗證，以及所有簡報要引用的量測數字。

介面簽章、檔案所有權、環境規則見 `README.md`。

## 最優先：複核 Ghidra 逆向報告（7/26 收工前）

**協定層在等其中一項**

`main.Cmd` 的欄位名稱，以及 `-mode` 字串對應到哪個 function code。
先驗這兩個，驗完立刻給出去，其餘的可以之後補。

要驗的項目，依重要性：

- `main.Cmd` 的欄位（`ip`/`mode`/`address`/`value`/`count`/`threads`/`timeout`/`try` 等），
  以及各自的預設值
- `main.main` 對 `mode` 的解析是否真的只有三條分支：`write` 給 Code 6、`write-m` 給
  Code 0x10、其餘一律 Code 3
- `Task.taskWorker` 依 Code 分派到 read/write/writeMultiple，default 什麼都不做
- **樣本完全沒有 coil 能力**，也就是 `rolfl/modbus` 雖然編進了 `ReadCoils`/`WriteSingleCoil`
  的符號，但 `main` 從未呼叫。
- build metadata（go1.20.4、module path 拼錯成 `CleintTCP`）與 DWARF 裡的開發者路徑

## 量測項目

**暫存器三層行為實驗**

對 HR10-13、HR1026、HR1024 分別寫入後高頻讀回，記錄留存與否，預期結果是 HR10-13 持久但
需要先切 `manual_mode`、HR1026 持久且不需要、HR1024 會被 ST 第 227 行無條件覆蓋。


**Suricata 離線重放**

host bridge 側錄之後餵給 router 裡的 Suricata，產出 `fast.log` 對照，側錄檔跟序列層要，
他們跑完整攻擊時會錄。

