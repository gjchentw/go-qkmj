# Spec Delta: architecture

## MODIFIED Requirements

### Requirement: 安全基線與外部輸入邊界驗證
系統 SHALL 於所有系統邊界驗證外部輸入（帳號名稱、密碼、聊天訊息、斜線指令、終端控制尺寸），並強制遵守以下安全基線：密碼 MUST 使用 `Argon2id`（OWASP 推薦 memory-hard 參數、隨機鹽與 PHC 格式，搭配常數時間比對）雜湊保存；資料庫存取 MUST 僅使用參數化查詢且禁止拼接 SQL 字串；系統 MUST NOT 將任何使用者輸入傳遞至 `os/exec`；Web 終端機 WebSocket 入口 MUST 預設啟用同源（Same-Origin）CSWSH 防護。

#### Scenario: 跨站 WebSocket 升級與非法輸入被邊界攔截
- **WHEN** 跨來源網頁嘗試升級 `/ws` 連線，或使用者於登入／斜線指令送入超長或含惡意字元之輸入
- **THEN** Web 閘道層 SHALL 預設拒絕跨來源升級（`403 Forbidden`），且邊界驗證器 SHALL 拒絕不合法輸入而不影響內部 Actor 或資料庫完整性

#### Scenario: 密碼以 Argon2id PHC 格式安全保存且不暴露雜湊
- **WHEN** 玩家註冊新帳號或修改密碼並隨後查詢帳號資料
- **THEN** 系統 SHALL 以隨機鹽與 Argon2id 產生 PHC 雜湊字串儲存，驗證時採用常數時間比對，且帳號查詢回傳結構不得包含密碼雜湊
