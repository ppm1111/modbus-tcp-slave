# Modbus TCP Slave Simulator

一個功能完整的 Modbus TCP Slave 模擬器，使用 Go 語言編寫，支援多個 Slave 實例、完整的 Modbus 功能碼，以及 Web 監控界面。

## 🚀 特性

### 核心功能
- ✅ **完整的 Modbus TCP 協議支援**
- ✅ **支援所有主要功能碼** (0x01-0x06, 0x0F, 0x10)
- ✅ **多 Slave 實例管理** (可同時運行數千個 Slaves)
- ✅ **Web 監控界面** (實時數據查看)
- ✅ **自動數據模擬** (模擬真實感測器數據變化)
- ✅ **線程安全** (支援多客戶端同時連接)
- ✅ **優雅關閉** (Ctrl+C 安全停止)

### 支援的 Modbus 功能碼

| 功能碼 | 功能 | 數據類型 | 操作 |
|--------|------|----------|------|
| 0x01 | Read Coils | 線圈 (Coils) | 讀取 |
| 0x02 | Read Discrete Inputs | 離散輸入 (Discrete Inputs) | 讀取 |
| 0x03 | Read Holding Registers | 保持寄存器 (Holding Registers) | 讀取 |
| 0x04 | Read Input Registers | 輸入寄存器 (Input Registers) | 讀取 |
| 0x05 | Write Single Coil | 線圈 (Coils) | 寫入單個 |
| 0x06 | Write Single Register | 保持寄存器 (Holding Registers) | 寫入單個 |
| 0x0F | Write Multiple Coils | 線圈 (Coils) | 寫入多個 |
| 0x10 | Write Multiple Registers | 保持寄存器 (Holding Registers) | 寫入多個 |

### 數據範圍
每個 Slave 包含：
- **線圈 (Coils)**: 10,000 個 (地址 0-9999)
- **離散輸入 (Discrete Inputs)**: 10,000 個 (地址 0-9999)
- **保持寄存器 (Holding Registers)**: 10,000 個 (地址 0-9999)
- **輸入寄存器 (Input Registers)**: 10,000 個 (地址 0-9999)

### 網頁預覽

<img width="1210" height="564" alt="Screenshot 2025-07-21 at 11 50 19" src="https://github.com/user-attachments/assets/1845af32-fa01-4e32-a3e7-8e29c34b22d6" />
<img width="1011" height="523" alt="Screenshot 2025-07-21 at 11 51 50" src="https://github.com/user-attachments/assets/736d10dc-7967-4750-82e1-aba89f10076b" />


## 📦 安裝與編譯

### 系統需求
- Go 1.18 或更高版本
- Linux/Windows/macOS

### 編譯
```bash
# 克隆或下載代碼
git clone <repository-url>
cd modbus-tcp-slave

# 編譯
go build -o modbus_slave main.go

# 或直接運行
go run main.go <起始端口> <slaves數量>
```

## 🎯 使用方法

### 基本語法
```bash
./modbus_slave <起始端口> <slaves數量>
```

### 使用範例

#### 1. 啟動單個 Slave
```bash
./modbus_slave 502 1
```
- 端口：502
- Slave ID：1
- Web 界面：http://localhost:8080

#### 2. 啟動多個 Slaves
```bash
./modbus_slave 10000 10
```
- 端口範圍：10000-10009
- 每個端口都是 Slave ID 1
- Web 界面：http://localhost:8080

#### 3. 大規模測試
```bash
./modbus_slave 5000 100
```
- 100 個 Slaves，端口 5000-5099
- 適合壓力測試

## 🔧 測試與驗證

### 使用 mbpoll 工具測試

#### 安裝 mbpoll
```bash
# Ubuntu/Debian
sudo apt-get install mbpoll

# CentOS/RHEL
sudo yum install mbpoll

# macOS
brew install mbpoll
```

#### 測試命令範例

```bash
# 假設 Slave 在端口 10000，Slave ID 為 1

# 1. 讀取線圈 (地址 0-19)
mbpoll -a 1 -r 0 -c 20 -t 0 -p 10000 -1 127.0.0.1

# 2. 讀取離散輸入 (地址 0-19)
mbpoll -a 1 -r 0 -c 20 -t 1 -p 10000 -1 127.0.0.1

# 3. 讀取輸入寄存器 (地址 0-19)
mbpoll -a 1 -r 0 -c 20 -t 3 -p 10000 -1 127.0.0.1

# 4. 讀取保持寄存器 (地址 0-19)
mbpoll -a 1 -r 0 -c 20 -t 4 -p 10000 -1 127.0.0.1

# 5. 寫入單個線圈 (地址 10，值 ON)
mbpoll -a 1 -r 10 -t 0 127.0.0.1 -p 10000 1

# 6. 寫入單個寄存器 (地址 10，值 1234)
mbpoll -a 1 -r 10 -t 4 127.0.0.1 -p 10000 1234

# 7. 讀取大範圍數據 (地址 0-99)
mbpoll -a 1 -r 0 -c 100 -t 4 -p 10000 -1 127.0.0.1
```

#### mbpoll 參數說明
- `-a`: Slave ID / Unit ID
- `-r`: 起始地址
- `-c`: 讀取數量
- `-t`: 功能碼類型 (0=線圈, 1=離散輸入, 3=輸入寄存器, 4=保持寄存器)
- `-p`: TCP 端口
- `-1`: 單次讀取
- `127.0.0.1`: IP 地址

## 🌐 Web 監控界面

### 訪問方式
程式啟動後，在瀏覽器中打開：**http://localhost:8080**

### 功能特色
- 📊 **實時數據顯示**：所有 Slaves 的當前狀態
- 🔄 **自動更新**：每 5 秒自動刷新數據
- 🎛️ **互動式操作**：點擊按鈕查看不同類型的數據
- 📈 **視覺化展示**：彩色分類顯示不同數據類型
  - 🔵 線圈 (藍色)
  - 🟣 離散輸入 (紫色)
  - 🟢 保持寄存器 (綠色)
  - 🟠 輸入寄存器 (橙色)

### 界面功能
- 查看每個 Slave 的運行狀態
- 實時監控數據變化
- 查看指定範圍的數據值
- 支援所有數據類型的查看

## 📊 數據模擬

### 自動數據更新
程式會自動模擬真實設備的數據變化：

#### 輸入寄存器
- 每 5-10 秒更新一次 (根據 Slave ID 調整週期)
- 模擬感測器數據 (溫度、壓力等)
- 基於時間戳生成動態值

#### 離散輸入
- 每 5-10 秒更新一次
- 模擬開關狀態、數位訊號
- 基於時間和 Slave ID 生成模式

#### 靜態數據
- **線圈**：可通過 Modbus 寫入命令修改
- **保持寄存器**：可通過 Modbus 寫入命令修改

## 🔒 安全與穩定性

### 線程安全
- 所有數據操作都有 `sync.RWMutex` 保護
- 支援多客戶端同時讀寫
- 無數據競爭問題

### 錯誤處理
- 完整的 Modbus 異常回應
- 網路連接異常處理
- 優雅的錯誤恢復機制

### 資源管理
- 連接超時機制 (30 秒)
- 自動清理無效連接
- 記憶體使用優化

## 🚦 狀態監控

### 日誌輸出
程式提供豐富的日誌信息：
```
🚀 Modbus Slave 1 started on port 10000
📖 Slave 1: 讀取保持寄存器 0-9, 數量: 10
✏️ Slave 1: 寫入線圈 10: false -> true
🔄 Slave 1: 模擬數據已更新
```

### 日誌類型
- 🚀 啟動信息
- 🔗 連接狀態
- 📖 讀取操作
- ✏️ 寫入操作
- 🔄 數據更新
- ⚠️ 異常處理
- 🛑 停止信息

## 🛠️ 故障排除

### 常見問題

#### 1. 端口被占用
```bash
Error: listen tcp :502: bind: address already in use
```
**解決方案**：使用其他端口或停止占用端口的程式

#### 2. 權限不足 (端口 < 1024)
```bash
Error: listen tcp :502: bind: permission denied
```
**解決方案**：使用 sudo 或選擇 > 1024 的端口

#### 3. mbpoll 連接超時
```bash
mbpoll: Operation timed out
```
**檢查事項**：
- Slave 是否正在運行
- 端口號是否正確
- Slave ID 是否匹配
- 防火牆設置

#### 4. 數據不匹配
**檢查事項**：
- 確認 Slave ID：所有端口都使用 Slave ID 1
- 檢查功能碼類型 (`-t` 參數)
- 確認地址範圍 (`-r` 和 `-c` 參數)

### 調試技巧
1. **查看日誌**：程式會輸出詳細的操作日誌
2. **使用 Web 界面**：實時查看數據狀態
3. **分步測試**：先測試簡單的讀取操作
4. **檢查網路**：確保本地網路暢通

## 📋 開發與擴展

### 代碼結構
```
main.go
├── ModbusSlave 結構體      # 單個 Slave 實例
├── SlaveManager 結構體     # 多 Slave 管理器
├── Modbus 功能碼處理       # 協議實現
├── Web 服務器             # 監控界面
└── 主程式邏輯             # 啟動和信號處理
```

### 自定義修改
1. **修改數據範圍**：調整 `make([]bool, 10000)` 中的大小
2. **調整更新頻率**：修改 `simulateInputChanges()` 中的 ticker
3. **更改 Slave ID 分配**：修改 `CreateSlaves()` 中的邏輯
4. **自定義 Web 界面**：修改 `handleHome()` 中的 HTML

### API 擴展
程式提供 REST API：
- `GET /api/slaves`：獲取所有 Slaves 狀態
- `GET /api/data?port=X&type=Y&start=Z&count=W`：獲取指定數據
