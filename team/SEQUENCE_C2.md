# 工作流：攻擊序列與 C2 整合

負責兩條原子攻擊序列，以及把 binary 經 Mythic 投遞、tasking 跑通到壓力衝破門檻。

介面簽章、檔案所有權、環境規則見 `README.md`，這份不重抄。

## 可以立刻開工

序列邏輯只依賴 `README.md` 那組 `modbus` 簽章，不依賴協定層真的寫完。照著簽章寫殼子，等
協定層交出實作再接上去。**不要自己另外實作一版 Modbus 或另外包一層重試**，重試和逾時已經
在 `Conn` 裡。

## PLC 位址

以下位址已由 GRFICS 的 ST 原始碼（`GRFICSv3/plc/st_files/326339.st`）確認，可以直接寫成
`internal/attack/addr.go` 的常數。

| 位址      | ST 變數                 | 說明                                  |
| --------- | ----------------------- | ------------------------------------- |
| coil 0    | `manual_mode` (%QX0.0)  | 置位後 PLC 才吃手動閥門設定值         |
| coil 40   | `run_bit` (%QX5.0)      | 為 FALSE 時排放閥強制全開，攻擊會被中和 |
| HR10-13   | `f1/f2/purge/product_manual_sp` (%QW10-13) | 手動閥門開度       |
| HR1026    | `pressure_sp` (%MW2)    | 正常 55295，寫 65535 觸發超壓         |

## 兩條序列

**coil 路徑（直接接管執行器）** — 這條是重寫版才做得到的，原版樣本沒有 coil 能力。

先 FC01 讀 coil 40 確認 `run_bit` 為 TRUE，是 FALSE 就直接中止，因為排放閥會被強制全開，
打了也沒用。接著 FC05 置位 coil 0，然後 FC16 一次寫入 HR10-13（進料全開、排放與產品關）。
寫完 FC01 加 FC03 讀回驗證。任何一步失敗就把 coil 0 寫回 0 回滾。

`run_bit` 那個前置檢查不是防禦性程式碼湊數，是 PLC 裡真實存在的聯鎖，漏掉的話攻擊會安靜
地無效。

**holding register 路徑（間接經控制迴路）** — 這條是原版樣本唯一能走的路徑。

FC03 讀 HR1026 存下原值，FC06 寫入新 setpoint（65535），FC03 讀回驗證，失敗就把原值寫回。
全程不碰 coil，`manual_mode` 維持 0。

這條已經實測過：單一次寫入之後壓力以每秒約 35 raw 單位爬升，三到四分鐘後達 3104.7 kPa 超
過 3000 kPa 破壞門檻。**它慢**，這是 demo 節奏的重要資訊，台上跑這條要留足時間。回滾也實測
成立，寫回 55295 之後排放閥立刻全開。

兩條都要把每一步記進 `Result.Steps`，因為 operator 在 Mythic UI 上看到的就是這個，它同時
是 demo 的畫面內容。JSON 輸出對應樣本的 `-output`。

## Mythic 投遞

投遞鏈已經用 hello-world 靜態 ELF 完整驗證過，走 `register_file` 上傳、Poseidon `upload`
落地到 `/tmp`、`shell chmod +x`、`shell` 執行。你走同一條，不要用 `docker exec` 抄捷徑，那
會失掉「攻擊透過 C2」這個敘事，而這正是整個專題的重點。

兩個已知的坑，不用再踩一次：

- **這個 Poseidon build 沒有原生 `chmod` command。** GraphQL 的 command 列表是 payload
  type 層級的全集，實際 build 未必全部編進去，agent 對沒編入的 command 只會回
  `Unknown command`。設執行位元要走 `shell chmod +x`。
- **scripting 走 nginx 的 `127.0.0.1:8080`**，不是 `MYTHIC_SERVER_PORT` 17443。

## 完成的判準

- 兩條序列對真 PLC 都跑得起來，含前置檢查、讀回驗證、回滾
- 完整攻擊經 Mythic tasking 跑通，壓力衝到 3000 kPa 以上，3D dashboard 看得到
- 過程中錄一份流量側錄給 Suricata 那條線用

跑真 PLC 之前先在群組講，重置一次要五分鐘以上，別人可能正在錄數據。
