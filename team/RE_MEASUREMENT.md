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
  的符號，但 `main` 從未呼叫。這一條是整份簡報的主論證，務必親眼確認
- build metadata（go1.20.4、module path 拼錯成 `CleintTCP`）與 DWARF 裡的開發者路徑

驗完把 `../PROJECT_SCOPE.md` 那段警語改成「已複核」，或修正錯誤的部分。**不要驗完只在群組
講一聲**，另外三個人是照文件做事的。

## 量測項目

**暫存器三層行為實驗**

對 HR10-13、HR1026、HR1024 分別寫入後高頻讀回，記錄留存與否。預期結果是 HR10-13 持久但
需要先切 `manual_mode`、HR1026 持久且不需要、HR1024 會被 ST 第 227 行無條件覆蓋。這個實
驗要獨占 PLC，開始前在群組說要占多久。

掃描週期的說法只講到「HR1024 的寫入值不留存，一個掃描週期內即消失」，20ms 這個數字出自原
始碼 `T#20ms`。**不要宣稱量出精確毫秒數**，我們沒有那個量測精度。

**Suricata 離線重放**

host bridge 側錄之後餵給 router 裡的 Suricata，產出 `fast.log` 對照。側錄檔跟序列層要，
他們跑完整攻擊時會錄。

論述要小心措辭：規則是有效的，只是感測器看不到。Suricata 綁在 eth2 只看 DMZ 側，而且
implant 與 plc 同網段、走二層直送不經過 router，所以改綁 eth1 也一樣看不到。**這是偵測盲
區，不是我們繞過了 IDS**，講成後者是誇大而且會被問倒。

**封包並排**

用協定層的 `-debug` 輸出，對重寫版打課程 PDF 那組參數（`-mode write -address 87
-value 88`），跟 PDF 裡的 Wireshark 截圖逐欄比對 function code、reference number、
register value。

## 浮動支援

這條線的工作比較零碎，中間有等待的空檔，適合順手支援卡住的人。四條線裡只有這條沒有下游
在等（除了 7/26 那份 mode 對應表），所以可以當緩衝。
