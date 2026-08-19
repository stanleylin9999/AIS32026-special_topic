# ICS/OT + Mythic C2 Demo

模擬工控環境（GRFICSv3）透過真實 C2 框架（Mythic）發動攻擊，兩者都是本機獨立跑的 Docker
Compose stack。

## 目錄結構

- `frostygoop-rewrite/`: 本專題唯一自己寫的程式碼，FrostyGoop 樣本的 Go 重寫版，投遞到 implant 上跑的靜態 binary。
- `team/`: 小組分工內容 **(我負責協定層的部分)**

第一次接觸這個專案？`SETUP.md` 從Linux 主機開始，把兩個 docker 都建起來。

## Port 對照表

| Port                | 服務                              | 備註                                                                    |
| ------------------- | --------------------------------- | ------------------------------------------------------------------------ |
| 8090                | GRFICS 模擬儀表板                 | 只開 localhost                                                          |
| 6081                | GRFICS HMI（ScadaLTS）            | 只開 localhost                                                          |
| 51820/udp           | GRFICS router WireGuard           | 只開 localhost                                                          |
| 8080                | Mythic operator UI（nginx）       | 只開 localhost                                                          |
| 5433                | Mythic postgres                   | 原本的 5432 被這台機器上另一個無關專案的 native postgresql 佔用，才改的 |
| 8082/8091/3000/8888 | Mythic hasura/docs/react/jupyter  | 為了避開跟上面 8080/8090 這組慣例衝突，全部移出預設值                   |

GRFICS 的 `plc` 容器沒有對外開 port，只能從 `b-ics-net` 這個 Docker network 內部連到，
這就是重點，它才是真正的攻擊目標。

## 攻擊路徑現況

Mythic 的 HTTP C2 profile 用 `network_mode: host` 跑，直接監聽 host 的
`0.0.0.0:80`，不走 `mythic_default` 這個 bridge network，位於 GRFICS `b-ics-net`
上的 implant 只須回連至該 bridge 的 gateway 位址`192.168.95.1:80` 即可通訊，詳情請見 `ATTACK_WALKTHROUGH.md`
