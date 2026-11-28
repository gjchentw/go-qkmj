# Spec Delta: store/account-economy

## Purpose

定義 `internal/store` 的帳號認證、QKMJ 95p3+ 經濟與排名、內建 AI 帳號（`QKMJ0`–`QKMJ9`）、零和原子結算與來源節點稽核、輕量 RBAC 權限與管理日誌、個人偏好及個人黑名單之持久化契約。

## ADDED Requirements

### Requirement: 帳號名稱驗證與保留字頭
系統 SHALL 僅接受符合 `^[0-9A-Za-z]{1,20}$` 之帳號名稱，保存註冊時之原始大小寫供顯示，並以不分大小寫方式執行唯一性與查詢比對。任何以 `qkmj` 開頭（不分大小寫）之名稱為系統保留字頭，真人註冊與密碼登入 MUST 一律拒絕。

#### Scenario: 拒絕不合法字元、重複名稱與保留字頭
- **WHEN** 使用者嘗試以 `小 明`、`alice_1`、已存在帳號不同大小寫 `ALICE`，或保留字頭 `qkmjHero` 註冊
- **THEN** 系統拒絕建立帳號並回傳對應型別錯誤，不異動資料庫

#### Scenario: 合法英數帳號不分大小寫比對登入
- **WHEN** 使用者以 `Alice99` 註冊，隨後以 `alice99` 與正確密碼登入
- **THEN** 系統認證成功並回傳顯示名稱為 `Alice99` 的帳號資料

### Requirement: 現代密碼安全與變更密碼
系統 SHALL 要求密碼長度介於 8 至 128 位元組（允許任意可列印字元），使用 `crypto/rand` 隨機鹽與 Argon2id（OWASP 推薦參數）以 PHC 字串格式儲存，並以常數時間比對驗證。修改密碼 MUST 先驗證舊密碼；本系統不提供自行刪除帳號（`/suicide`）操作。

#### Scenario: 註冊密碼過短或改密碼舊密碼錯誤皆被拒絕
- **WHEN** 註冊輸入 7 字元密碼，或執行變更密碼時舊密碼不符
- **THEN** 系統回傳驗證錯誤且不建立或修改密碼雜湊

#### Scenario: 驗證正確後更新密碼且不暴露雜湊
- **WHEN** 已註冊帳號以正確舊密碼將密碼更新為合法新密碼
- **THEN** 舊密碼即刻失效、新密碼可正常登入，且任何公開帳號結構皆不含密碼雜湊欄位

### Requirement: 內建 AI 真實帳號池（QKMJ0 至 QKMJ9）
資料庫初始化時，系統 SHALL 冪等建立 `QKMJ0` 至 `QKMJ9` 共 10 個真實帳號，初始賭幣 `20000` 並賦予 `bot` 角色。AI 帳號享有與真人相同之結算、戰績統計與 `/stat` 查詢，但 MUST 永遠拒絕密碼登入驗證，並提供列出全部 AI 帳號之查詢介面。

#### Scenario: 啟動初始化冪等建立 10 個 AI 帳號且不可登入
- **WHEN** 系統對同一資料庫連續執行兩次啟動初始化，隨後嘗試以任意密碼登入 `QKMJ0`
- **THEN** 資料庫恰有 `QKMJ0` 至 `QKMJ9` 共 10 個具 `bot` 角色之帳號且初始金額皆為 `20000`，而登入 `QKMJ0` 遭拒絕

### Requirement: QKMJ 95p3+ 初始金額、登入救濟與全服排名
新註冊帳號初始賭幣 SHALL 為 `20000`。真人登入成功時，系統 SHALL 在單一原子批次內將 `login_count` 加 1、更新 `last_login_at` 與 `last_node_name`；若 `balance < 20000` 且 `games_played >= 16`，自動將 `balance` 補回 `20000` 並標記 `ReliefGranted=true`。系統不設負數開桌門檻；全服排名僅於 `games_played >= 16` 之帳號間依金額計算（同分並列），未滿 16 局無排名。

#### Scenario: 滿 16 局且金額低於 20000 登入觸發救濟
- **WHEN** 帳號餘額為 `-3500`、已玩 `16` 局的玩家於節點 `calm-otter` 登入成功
- **THEN** 系統原子將餘額補回 `20000`、記錄 `last_node_name="calm-otter"`、`login_count` 加 1，並回傳 `ReliefGranted=true`

#### Scenario: 未滿 16 局不觸發救濟亦不計排名
- **WHEN** 帳號餘額為 `5000`、已玩 `15` 局的玩家登入並查詢戰績
- **THEN** 餘額維持 `5000`、`ReliefGranted=false`，且排名結果標記為無排名

#### Scenario: 同分並列全服排名計算
- **WHEN** 合格（>= 16 局）帳號餘額分別為 `50000`、`30000`、`30000`、`10000`
- **THEN** 兩名 `30000` 帳號之排名皆為 `2/4`，`10000` 帳號之排名為 `4/4`

### Requirement: 原子相對增量結算與來源節點冪等稽核
每局結束呼叫 `SettleHand` 時，系統 SHALL 於單一原子批次寫入 `hand_settlements`（含唯一 `hand_id`、`node_name`、`end_reason`、四家明細與時間）並以相對增量（`balance = balance + delta`）更新四家帳號（真人與 AI 一視同仁）：胡牌結算累加 `games_played`、勝負與放槍統計；和局僅累加 `draws` 不增 `games_played`。重複送入相同 `hand_id` MUST 拒絕且不得重複變動餘額；中途強行離桌可透過原子操作累加 `abandoned_hands`。

#### Scenario: 四家（含 AI）胡牌結算原子零和更新並記錄來源節點
- **WHEN** 節點 `brisk-falcon` 以 `hand_id="h-101"` 結算一局胡牌（3 名真人 + `QKMJ0`，四家 delta 總和為 0）
- **THEN** 四家餘額皆依相對增量原子更新、`games_played` 各加 1、生涯自摸／胡牌／放槍統計正確累加，且 `hand_settlements` 留存 `node_name="brisk-falcon"` 與明細

#### Scenario: 重複送入相同 HandID 遭到冪等拒絕
- **WHEN** 同一 `hand_id="h-101"` 因重試再次呼叫 `SettleHand`
- **THEN** 系統回傳重複結算錯誤，四家餘額與戰績皆不產生二次異動

#### Scenario: 和局結算與中途強跳計數
- **WHEN** 一局以四風連和結算，另有一局玩家於打牌中強行離桌呼叫 `RecordAbandonedHand`
- **THEN** 和局使四家 `draws` 加 1 且餘額與 `games_played` 不變；強跳僅使該帳號 `abandoned_hands` 加 1

### Requirement: 輕量 RBAC、跨節點帳號封鎖與管理稽核日誌
系統 SHALL 以 `account_roles` 維護帳號角色（`player`、`moderator`、`admin`、`bot`），新註冊真人預設賦予 `player`，權限透過強型別方法檢查。管理操作（指派／撤銷角色、封鎖／解封帳號）MUST 驗證操作者具備對應權限，並在單一原子批次內寫入 `moderation_logs` 不可變稽核紀錄。遭封鎖且未到期之帳號登入時 MUST 遭拒絕並回傳封鎖原因與到期資訊。

#### Scenario: 管理員封鎖帳號並於單一交易留下稽核紀錄
- **WHEN** 具 `moderator` 角色之帳號對違規帳號執行封鎖並附理由與期限，隨後被封鎖者嘗試登入
- **THEN** 封鎖與 `moderation_logs` 紀錄同步落盤，被封鎖帳號登入失敗並回傳封鎖錯誤；一般 `player` 呼叫封鎖則遭權限拒絕

#### Scenario: 僅 admin 可指派或撤銷他人角色
- **WHEN** `admin` 指派 `moderator` 角色給一般玩家，而 `moderator` 嘗試指派 `admin` 角色
- **THEN** `admin` 操作成功並寫入 `moderation_logs`；`moderator` 操作遭權限拒絕

### Requirement: 玩家偏好設定與個人黑名單
系統 SHALL 為帳號持久化個人偏好（`default_note`、`beep_config`、`tile_colors`、`display_mode`、`function_keys`），未設定時回傳預設值。系統 SHALL 提供個人黑名單（`user_blocks`）之新增、移除與查詢，禁止封鎖自己、須為有效帳號且受數量上限（預設 100 筆）保護，供上層過濾私訊、邀請與入桌。

#### Scenario: 未自訂偏好回傳預設值且儲存後跨裝置一致
- **WHEN** 新帳號讀取偏好，接著更新預設牌桌附註與自訂筒子顏色後再次讀取
- **THEN** 首次讀取回傳系統預設值無錯誤；更新後再次讀取完整保留新設定

#### Scenario: 個人黑名單邊界檢查與清單管理
- **WHEN** 玩家將自己加入黑名單，或加入有效帳號後再移除
- **THEN** 封鎖自己遭拒絕；加入有效帳號後查詢 `IsBlocked` 回傳 `true`，移除後回傳 `false`
