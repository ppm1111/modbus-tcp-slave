package main

import (
    "encoding/binary"
    "encoding/json"
    "fmt"
    "log"
    "net"
    "net/http"
    "os"
    "os/signal"
    "strconv"
    "sync"
    "syscall"
    "time"
)

// Modbus 功能碼常量
const (
    // 讀取功能碼
    FuncReadCoils            = 0x01 // 讀取線圈狀態
    FuncReadDiscreteInputs   = 0x02 // 讀取離散輸入狀態
    FuncReadHoldingRegisters = 0x03 // 讀取保持寄存器
    FuncReadInputRegisters   = 0x04 // 讀取輸入寄存器
    
    // 寫入功能碼
    FuncWriteSingleCoil      = 0x05 // 寫入單個線圈
    FuncWriteSingleRegister  = 0x06 // 寫入單個寄存器
    FuncWriteMultipleCoils   = 0x0F // 寫入多個線圈
    FuncWriteMultipleRegisters = 0x10 // 寫入多個寄存器
)

// Modbus 異常碼
const (
    ExceptionIllegalFunction    = 0x01
    ExceptionIllegalDataAddress = 0x02
    ExceptionIllegalDataValue   = 0x03
    ExceptionSlaveDeviceFailure = 0x04
)

// ModbusSlave 結構體，包含所有四種數據類型
type ModbusSlave struct {
    slaveID         byte
    port            int
    listener        net.Listener
    
    // 數據存儲 (使用 sync.RWMutex 保證線程安全)
    coilsMutex      sync.RWMutex
    coils           []bool          // 線圈狀態 (0x區域)
    
    discreteInputsMutex sync.RWMutex
    discreteInputs  []bool          // 離散輸入狀態 (1x區域)
    
    holdingRegsMutex sync.RWMutex
    holdingRegs     []uint16        // 保持寄存器 (4x區域)
    
    inputRegsMutex  sync.RWMutex
    inputRegs       []uint16        // 輸入寄存器 (3x區域)
    
    isRunning       bool
    stopChan        chan struct{}
    wg              sync.WaitGroup
}

// NewModbusSlave 創建新的 Modbus slave
func NewModbusSlave(slaveID byte, port int) *ModbusSlave {
    return &ModbusSlave{
        slaveID:        slaveID,
        port:           port,
        coils:          make([]bool, 10000),      // 10000 個線圈
        discreteInputs: make([]bool, 10000),     // 10000 個離散輸入
        holdingRegs:    make([]uint16, 10000),   // 10000 個保持寄存器
        inputRegs:      make([]uint16, 10000),   // 10000 個輸入寄存器
        stopChan:       make(chan struct{}),
    }
}

// 初始化測試數據
func (ms *ModbusSlave) InitializeTestData() {
    // 初始化線圈狀態 (基於 slave ID 的模式)
    for i := 0; i < len(ms.coils); i++ {
        ms.coils[i] = ((i + int(ms.slaveID)) % 3 == 0)
    }
    
    // 初始化離散輸入狀態
    for i := 0; i < len(ms.discreteInputs); i++ {
        ms.discreteInputs[i] = ((i + int(ms.slaveID)) % 4 == 0)
    }
    
    // 初始化保持寄存器 (slave ID 作為基準值)
    for i := 0; i < len(ms.holdingRegs); i++ {
        ms.holdingRegs[i] = uint16((i + int(ms.slaveID)*1000) % 65535)
    }
    
    // 初始化輸入寄存器
    for i := 0; i < len(ms.inputRegs); i++ {
        ms.inputRegs[i] = uint16((i*int(ms.slaveID) + int(ms.slaveID)*100) % 65535)
    }
    
    log.Printf("✅ Slave %d (Port %d): 測試數據初始化完成", ms.slaveID, ms.port)
}

// Start 啟動 Modbus slave 服務
func (ms *ModbusSlave) Start() error {
    var err error
    ms.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", ms.port))
    if err != nil {
        return fmt.Errorf("slave %d failed to listen on port %d: %v", ms.slaveID, ms.port, err)
    }
    
    ms.isRunning = true
    log.Printf("🚀 Modbus Slave %d started on port %d", ms.slaveID, ms.port)
    log.Printf("   📊 數據範圍: 線圈(0-9999), 離散輸入(0-9999), 保持寄存器(0-9999), 輸入寄存器(0-9999)")
    
    // 啟動模擬輸入寄存器變化的協程
    ms.wg.Add(1)
    go ms.simulateInputChanges()
    
    // 接受連接
    ms.wg.Add(1)
    go ms.acceptConnections()
    
    return nil
}

// 模擬輸入寄存器和離散輸入的變化
func (ms *ModbusSlave) simulateInputChanges() {
    defer ms.wg.Done()
    
    ticker := time.NewTicker(time.Duration(5+int(ms.slaveID)) * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // 更新部分輸入寄存器 (模擬感測器數據)
            ms.inputRegsMutex.Lock()
            baseValue := uint16(time.Now().Unix() % 65535)
            for i := 0; i < 100; i++ {
                ms.inputRegs[i] = (baseValue + uint16(i) + uint16(ms.slaveID)*100) % 65535
            }
            ms.inputRegsMutex.Unlock()
            
            // 更新部分離散輸入 (模擬開關狀態)
            ms.discreteInputsMutex.Lock()
            for i := 0; i < 50; i++ {
                ms.discreteInputs[i] = ((time.Now().Unix() + int64(ms.slaveID) + int64(i)) % 3 == 0)
            }
            ms.discreteInputsMutex.Unlock()
            
            log.Printf("🔄 Slave %d: 模擬數據已更新", ms.slaveID)
            
        case <-ms.stopChan:
            return
        }
    }
}

// acceptConnections 接受客戶端連接
func (ms *ModbusSlave) acceptConnections() {
    defer ms.wg.Done()
    
    for ms.isRunning {
        conn, err := ms.listener.Accept()
        if err != nil {
            if ms.isRunning {
                log.Printf("❌ Slave %d accept error: %v", ms.slaveID, err)
            }
            continue
        }
        
        ms.wg.Add(1)
        go ms.handleConnection(conn)
    }
}

// handleConnection 處理客戶端連接
func (ms *ModbusSlave) handleConnection(conn net.Conn) {
    defer ms.wg.Done()
    defer conn.Close()
    
    log.Printf("🔗 Slave %d: 新連接來自 %s", ms.slaveID, conn.RemoteAddr())
    
    buffer := make([]byte, 512)
    
    for {
        // 設置讀取超時
        conn.SetReadDeadline(time.Now().Add(30 * time.Second))
        
        n, err := conn.Read(buffer)
        if err != nil {
            if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                log.Printf("⏰ Slave %d: 連接超時", ms.slaveID)
            }
            break
        }
        
        if n < 8 { // Modbus TCP 最小長度
            continue
        }
        
        // 處理 Modbus TCP 請求
        response := ms.processModbusRequest(buffer[:n])
        if response != nil {
            conn.Write(response)
        }
    }
    
    log.Printf("🔌 Slave %d: 連接已關閉 %s", ms.slaveID, conn.RemoteAddr())
}

// processModbusRequest 處理 Modbus 請求
func (ms *ModbusSlave) processModbusRequest(request []byte) []byte {
    if len(request) < 8 {
        return nil
    }
    
    // 解析 Modbus TCP 標頭
    transactionID := binary.BigEndian.Uint16(request[0:2])
    protocolID := binary.BigEndian.Uint16(request[2:4])
    length := binary.BigEndian.Uint16(request[4:6])
    unitID := request[6]
    functionCode := request[7]
    
    // 檢查協議ID和單元ID
    if protocolID != 0 || unitID != ms.slaveID {
        return nil
    }
    
    log.Printf("📨 Slave %d: 收到請求 - 功能碼: 0x%02X, 長度: %d", ms.slaveID, functionCode, length)
    
    var response []byte
    var err error
    
    // 根據功能碼處理請求
    switch functionCode {
    case FuncReadCoils:
        response, err = ms.readCoils(request[8:])
    case FuncReadDiscreteInputs:
        response, err = ms.readDiscreteInputs(request[8:])
    case FuncReadHoldingRegisters:
        response, err = ms.readHoldingRegisters(request[8:])
    case FuncReadInputRegisters:
        response, err = ms.readInputRegisters(request[8:])
    case FuncWriteSingleCoil:
        response, err = ms.writeSingleCoil(request[8:])
    case FuncWriteSingleRegister:
        response, err = ms.writeSingleRegister(request[8:])
    case FuncWriteMultipleCoils:
        response, err = ms.writeMultipleCoils(request[8:])
    case FuncWriteMultipleRegisters:
        response, err = ms.writeMultipleRegisters(request[8:])
    default:
        response = ms.createExceptionResponse(functionCode, ExceptionIllegalFunction)
    }
    
    if err != nil {
        log.Printf("❌ Slave %d: 處理錯誤 - %v", ms.slaveID, err)
        return ms.createExceptionResponse(functionCode, ExceptionSlaveDeviceFailure)
    }
    
    if response == nil {
        return nil
    }
    
    // 添加 Modbus TCP 標頭
    tcpHeader := make([]byte, 6)
    binary.BigEndian.PutUint16(tcpHeader[0:2], transactionID)
    binary.BigEndian.PutUint16(tcpHeader[2:4], 0) // 協議ID
    binary.BigEndian.PutUint16(tcpHeader[4:6], uint16(len(response)+1)) // 長度
    
    fullResponse := append(tcpHeader, ms.slaveID)
    fullResponse = append(fullResponse, response...)
    
    return fullResponse
}

// readCoils 讀取線圈狀態 (功能碼 0x01)
func (ms *ModbusSlave) readCoils(data []byte) ([]byte, error) {
    if len(data) < 4 {
        return ms.createExceptionResponse(FuncReadCoils, ExceptionIllegalDataValue), nil
    }
    
    startAddr := binary.BigEndian.Uint16(data[0:2])
    quantity := binary.BigEndian.Uint16(data[2:4])
    
    if quantity == 0 || quantity > 2000 || int(startAddr)+int(quantity) > len(ms.coils) {
        return ms.createExceptionResponse(FuncReadCoils, ExceptionIllegalDataAddress), nil
    }
    
    ms.coilsMutex.RLock()
    defer ms.coilsMutex.RUnlock()
    
    byteCount := (quantity + 7) / 8
    response := make([]byte, 2+byteCount)
    response[0] = FuncReadCoils
    response[1] = byte(byteCount)
    
    for i := uint16(0); i < quantity; i++ {
        if ms.coils[startAddr+i] {
            byteIndex := i / 8
            bitIndex := i % 8
            response[2+byteIndex] |= (1 << bitIndex)
        }
    }
    
    log.Printf("📖 Slave %d: 讀取線圈 %d-%d, 數量: %d", ms.slaveID, startAddr, startAddr+quantity-1, quantity)
    return response, nil
}

// readDiscreteInputs 讀取離散輸入狀態 (功能碼 0x02)
func (ms *ModbusSlave) readDiscreteInputs(data []byte) ([]byte, error) {
    if len(data) < 4 {
        return ms.createExceptionResponse(FuncReadDiscreteInputs, ExceptionIllegalDataValue), nil
    }
    
    startAddr := binary.BigEndian.Uint16(data[0:2])
    quantity := binary.BigEndian.Uint16(data[2:4])
    
    if quantity == 0 || quantity > 2000 || int(startAddr)+int(quantity) > len(ms.discreteInputs) {
        return ms.createExceptionResponse(FuncReadDiscreteInputs, ExceptionIllegalDataAddress), nil
    }
    
    ms.discreteInputsMutex.RLock()
    defer ms.discreteInputsMutex.RUnlock()
    
    byteCount := (quantity + 7) / 8
    response := make([]byte, 2+byteCount)
    response[0] = FuncReadDiscreteInputs
    response[1] = byte(byteCount)
    
    for i := uint16(0); i < quantity; i++ {
        if ms.discreteInputs[startAddr+i] {
            byteIndex := i / 8
            bitIndex := i % 8
            response[2+byteIndex] |= (1 << bitIndex)
        }
    }
    
    log.Printf("📖 Slave %d: 讀取離散輸入 %d-%d, 數量: %d", ms.slaveID, startAddr, startAddr+quantity-1, quantity)
    return response, nil
}

// readHoldingRegisters 讀取保持寄存器 (功能碼 0x03)
func (ms *ModbusSlave) readHoldingRegisters(data []byte) ([]byte, error) {
    if len(data) < 4 {
        return ms.createExceptionResponse(FuncReadHoldingRegisters, ExceptionIllegalDataValue), nil
    }
    
    startAddr := binary.BigEndian.Uint16(data[0:2])
    quantity := binary.BigEndian.Uint16(data[2:4])
    
    if quantity == 0 || quantity > 125 || int(startAddr)+int(quantity) > len(ms.holdingRegs) {
        return ms.createExceptionResponse(FuncReadHoldingRegisters, ExceptionIllegalDataAddress), nil
    }
    
    ms.holdingRegsMutex.RLock()
    defer ms.holdingRegsMutex.RUnlock()
    
    byteCount := quantity * 2
    response := make([]byte, 2+byteCount)
    response[0] = FuncReadHoldingRegisters
    response[1] = byte(byteCount)
    
    for i := uint16(0); i < quantity; i++ {
        value := ms.holdingRegs[startAddr+i]
        binary.BigEndian.PutUint16(response[2+i*2:4+i*2], value)
    }
    
    log.Printf("📖 Slave %d: 讀取保持寄存器 %d-%d, 數量: %d", ms.slaveID, startAddr, startAddr+quantity-1, quantity)
    return response, nil
}

// readInputRegisters 讀取輸入寄存器 (功能碼 0x04)
func (ms *ModbusSlave) readInputRegisters(data []byte) ([]byte, error) {
    if len(data) < 4 {
        return ms.createExceptionResponse(FuncReadInputRegisters, ExceptionIllegalDataValue), nil
    }
    
    startAddr := binary.BigEndian.Uint16(data[0:2])
    quantity := binary.BigEndian.Uint16(data[2:4])
    
    if quantity == 0 || quantity > 125 || int(startAddr)+int(quantity) > len(ms.inputRegs) {
        return ms.createExceptionResponse(FuncReadInputRegisters, ExceptionIllegalDataAddress), nil
    }
    
    ms.inputRegsMutex.RLock()
    defer ms.inputRegsMutex.RUnlock()
    
    byteCount := quantity * 2
    response := make([]byte, 2+byteCount)
    response[0] = FuncReadInputRegisters
    response[1] = byte(byteCount)
    
    for i := uint16(0); i < quantity; i++ {
        value := ms.inputRegs[startAddr+i]
        binary.BigEndian.PutUint16(response[2+i*2:4+i*2], value)
    }
    
    log.Printf("📖 Slave %d: 讀取輸入寄存器 %d-%d, 數量: %d", ms.slaveID, startAddr, startAddr+quantity-1, quantity)
    return response, nil
}

// writeSingleCoil 寫入單個線圈 (功能碼 0x05)
func (ms *ModbusSlave) writeSingleCoil(data []byte) ([]byte, error) {
    if len(data) < 4 {
        return ms.createExceptionResponse(FuncWriteSingleCoil, ExceptionIllegalDataValue), nil
    }
    
    addr := binary.BigEndian.Uint16(data[0:2])
    value := binary.BigEndian.Uint16(data[2:4])
    
    if int(addr) >= len(ms.coils) || (value != 0x0000 && value != 0xFF00) {
        return ms.createExceptionResponse(FuncWriteSingleCoil, ExceptionIllegalDataAddress), nil
    }
    
    ms.coilsMutex.Lock()
    oldValue := ms.coils[addr]
    ms.coils[addr] = (value == 0xFF00)
    ms.coilsMutex.Unlock()
    
    // 回應原始請求
    response := make([]byte, 5)
    response[0] = FuncWriteSingleCoil
    binary.BigEndian.PutUint16(response[1:3], addr)
    binary.BigEndian.PutUint16(response[3:5], value)
    
    log.Printf("✏️ Slave %d: 寫入線圈 %d: %t -> %t", ms.slaveID, addr, oldValue, ms.coils[addr])
    return response, nil
}

// writeSingleRegister 寫入單個寄存器 (功能碼 0x06)
func (ms *ModbusSlave) writeSingleRegister(data []byte) ([]byte, error) {
    if len(data) < 4 {
        return ms.createExceptionResponse(FuncWriteSingleRegister, ExceptionIllegalDataValue), nil
    }
    
    addr := binary.BigEndian.Uint16(data[0:2])
    value := binary.BigEndian.Uint16(data[2:4])
    
    if int(addr) >= len(ms.holdingRegs) {
        return ms.createExceptionResponse(FuncWriteSingleRegister, ExceptionIllegalDataAddress), nil
    }
    
    ms.holdingRegsMutex.Lock()
    oldValue := ms.holdingRegs[addr]
    ms.holdingRegs[addr] = value
    ms.holdingRegsMutex.Unlock()
    
    // 回應原始請求
    response := make([]byte, 5)
    response[0] = FuncWriteSingleRegister
    binary.BigEndian.PutUint16(response[1:3], addr)
    binary.BigEndian.PutUint16(response[3:5], value)
    
    log.Printf("✏️ Slave %d: 寫入保持寄存器 %d: %d -> %d", ms.slaveID, addr, oldValue, value)
    return response, nil
}

// writeMultipleCoils 寫入多個線圈 (功能碼 0x0F)
func (ms *ModbusSlave) writeMultipleCoils(data []byte) ([]byte, error) {
    if len(data) < 5 {
        return ms.createExceptionResponse(FuncWriteMultipleCoils, ExceptionIllegalDataValue), nil
    }
    
    startAddr := binary.BigEndian.Uint16(data[0:2])
    quantity := binary.BigEndian.Uint16(data[2:4])
    byteCount := data[4]
    
    expectedByteCount := (quantity + 7) / 8
    if quantity == 0 || quantity > 1968 || int(startAddr)+int(quantity) > len(ms.coils) || 
       byteCount != byte(expectedByteCount) || len(data) < 5+int(byteCount) {
        return ms.createExceptionResponse(FuncWriteMultipleCoils, ExceptionIllegalDataAddress), nil
    }
    
    ms.coilsMutex.Lock()
    for i := uint16(0); i < quantity; i++ {
        byteIndex := i / 8
        bitIndex := i % 8
        bitValue := (data[5+byteIndex] & (1 << bitIndex)) != 0
        ms.coils[startAddr+i] = bitValue
    }
    ms.coilsMutex.Unlock()
    
    // 回應
    response := make([]byte, 5)
    response[0] = FuncWriteMultipleCoils
    binary.BigEndian.PutUint16(response[1:3], startAddr)
    binary.BigEndian.PutUint16(response[3:5], quantity)
    
    log.Printf("✏️ Slave %d: 寫入多個線圈 %d-%d, 數量: %d", ms.slaveID, startAddr, startAddr+quantity-1, quantity)
    return response, nil
}

// writeMultipleRegisters 寫入多個寄存器 (功能碼 0x10)
func (ms *ModbusSlave) writeMultipleRegisters(data []byte) ([]byte, error) {
    if len(data) < 5 {
        return ms.createExceptionResponse(FuncWriteMultipleRegisters, ExceptionIllegalDataValue), nil
    }
    
    startAddr := binary.BigEndian.Uint16(data[0:2])
    quantity := binary.BigEndian.Uint16(data[2:4])
    byteCount := data[4]
    
    if quantity == 0 || quantity > 123 || int(startAddr)+int(quantity) > len(ms.holdingRegs) || 
       byteCount != byte(quantity*2) || len(data) < 5+int(byteCount) {
        return ms.createExceptionResponse(FuncWriteMultipleRegisters, ExceptionIllegalDataAddress), nil
    }
    
    ms.holdingRegsMutex.Lock()
    for i := uint16(0); i < quantity; i++ {
        value := binary.BigEndian.Uint16(data[5+i*2 : 7+i*2])
        ms.holdingRegs[startAddr+i] = value
    }
    ms.holdingRegsMutex.Unlock()
    
    // 回應
    response := make([]byte, 5)
    response[0] = FuncWriteMultipleRegisters
    binary.BigEndian.PutUint16(response[1:3], startAddr)
    binary.BigEndian.PutUint16(response[3:5], quantity)
    
    log.Printf("✏️ Slave %d: 寫入多個保持寄存器 %d-%d, 數量: %d", ms.slaveID, startAddr, startAddr+quantity-1, quantity)
    return response, nil
}

// createExceptionResponse 創建異常回應
func (ms *ModbusSlave) createExceptionResponse(functionCode, exceptionCode byte) []byte {
    response := make([]byte, 2)
    response[0] = functionCode | 0x80 // 設置異常位
    response[1] = exceptionCode
    
    log.Printf("⚠️ Slave %d: 異常回應 - 功能碼: 0x%02X, 異常碼: 0x%02X", ms.slaveID, functionCode, exceptionCode)
    return response
}

// Stop 停止 Modbus slave
func (ms *ModbusSlave) Stop() {
    ms.isRunning = false
    close(ms.stopChan)
    
    if ms.listener != nil {
        ms.listener.Close()
    }
    
    ms.wg.Wait()
    log.Printf("🛑 Modbus Slave %d stopped", ms.slaveID)
}

// GetDataRange 獲取指定範圍的數據
func (ms *ModbusSlave) GetDataRange(dataType string, start, count int) interface{} {
    switch dataType {
    case "coils":
        ms.coilsMutex.RLock()
        defer ms.coilsMutex.RUnlock()
        if start+count > len(ms.coils) {
            count = len(ms.coils) - start
        }
        return ms.coils[start : start+count]
        
    case "discrete":
        ms.discreteInputsMutex.RLock()
        defer ms.discreteInputsMutex.RUnlock()
        if start+count > len(ms.discreteInputs) {
            count = len(ms.discreteInputs) - start
        }
        return ms.discreteInputs[start : start+count]
        
    case "holding":
        ms.holdingRegsMutex.RLock()
        defer ms.holdingRegsMutex.RUnlock()
        if start+count > len(ms.holdingRegs) {
            count = len(ms.holdingRegs) - start
        }
        return ms.holdingRegs[start : start+count]
        
    case "input":
        ms.inputRegsMutex.RLock()
        defer ms.inputRegsMutex.RUnlock()
        if start+count > len(ms.inputRegs) {
            count = len(ms.inputRegs) - start
        }
        return ms.inputRegs[start : start+count]
    }
    return nil
}
// GetStatus 獲取當前狀態
func (ms *ModbusSlave) GetStatus() map[string]interface{} {
    ms.coilsMutex.RLock()
    ms.discreteInputsMutex.RLock()
    ms.holdingRegsMutex.RLock()
    ms.inputRegsMutex.RLock()
    
    defer ms.coilsMutex.RUnlock()
    defer ms.discreteInputsMutex.RUnlock()
    defer ms.holdingRegsMutex.RUnlock()
    defer ms.inputRegsMutex.RUnlock()
    
    return map[string]interface{}{
        "slave_id":              ms.slaveID,
        "port":                  ms.port,
        "running":               ms.isRunning,
        "coils_count":           len(ms.coils),
        "discrete_inputs_count": len(ms.discreteInputs),
        "holding_regs_count":    len(ms.holdingRegs),
        "input_regs_count":      len(ms.inputRegs),
        "sample_coil_0":         ms.coils[0],
        "sample_discrete_0":     ms.discreteInputs[0],
        "sample_holding_0":      ms.holdingRegs[0],
        "sample_input_0":        ms.inputRegs[0],
    }
}

// StartWebServer 啟動 Web 監控服務器
func (sm *SlaveManager) StartWebServer(webPort int) {
    http.HandleFunc("/", sm.handleHome)
    http.HandleFunc("/api/slaves", sm.handleAPISlaves)
    http.HandleFunc("/api/data", sm.handleAPIData)
    
    log.Printf("🌐 Web 監控界面啟動於: http://localhost:%d", webPort)
    go func() {
        if err := http.ListenAndServe(fmt.Sprintf(":%d", webPort), nil); err != nil {
            log.Printf("❌ Web 服務器錯誤: %v", err)
        }
    }()
}

func (sm *SlaveManager) handleHome(w http.ResponseWriter, r *http.Request) {
    html := `
<!DOCTYPE html>
<html>
<head>
    <title>Modbus Slaves 監控</title>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .slave-card { border: 1px solid #ddd; margin: 10px; padding: 15px; border-radius: 5px; }
        .data-section { margin: 10px 0; }
        .data-grid { display: grid; grid-template-columns: repeat(10, 1fr); gap: 5px; margin: 10px 0; }
        .data-item { padding: 5px; border: 1px solid #eee; text-align: center; font-size: 12px; }
        .coil { background-color: #e3f2fd; }
        .discrete { background-color: #f3e5f5; }
        .holding { background-color: #e8f5e8; }
        .input { background-color: #fff3e0; }
        button { margin: 5px; padding: 8px 16px; cursor: pointer; }
    </style>
</head>
<body>
    <h1>🔧 Modbus Slaves 監控面板</h1>
    <div id="slaves-container"></div>
    
    <script>
        function loadSlaves() {
            fetch('/api/slaves')
                .then(response => response.json())
                .then(slaves => {
                    const container = document.getElementById('slaves-container');
                    container.innerHTML = '';
                    
                    slaves.forEach(slave => {
                        const card = document.createElement('div');
                        card.className = 'slave-card';
                        card.innerHTML = ` + "`" + `
                            <h3>Slave ID: ${slave.slave_id} (Port: ${slave.port})</h3>
                            <p>運行狀態: ${slave.running ? '✅ 運行中' : '❌ 停止'}</p>
                            <button onclick="loadData(${slave.port}, 'coils', 0, 20)">查看線圈 (0-19)</button>
                            <button onclick="loadData(${slave.port}, 'discrete', 0, 20)">查看離散輸入 (0-19)</button>
                            <button onclick="loadData(${slave.port}, 'holding', 0, 20)">查看保持寄存器 (0-19)</button>
                            <button onclick="loadData(${slave.port}, 'input', 0, 20)">查看輸入寄存器 (0-19)</button>
                            <div id="data-${slave.port}"></div>
                        ` + "`" + `;
                        container.appendChild(card);
                    });
                });
        }
        
        function loadData(port, type, start, count) {
            fetch(` + "`" + `/api/data?port=${port}&type=${type}&start=${start}&count=${count}` + "`" + `)
                .then(response => response.json())
                .then(data => {
                    const container = document.getElementById(` + "`" + `data-${port}` + "`" + `);
                    let html = ` + "`" + `<div class="data-section"><h4>${type} (${start}-${start+count-1}):</h4><div class="data-grid">` + "`" + `;
                    
                    data.forEach((value, index) => {
                        html += ` + "`" + `<div class="data-item ${type}">${start + index}: ${value}</div>` + "`" + `;
                    });
                    
                    html += '</div></div>';
                    container.innerHTML = html;
                });
        }
        
        // 每 5 秒自動更新
        setInterval(loadSlaves, 5000);
        loadSlaves();
    </script>
</body>
</html>`
    
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(html))
}

func (sm *SlaveManager) handleAPISlaves(w http.ResponseWriter, r *http.Request) {
    var slavesInfo []map[string]interface{}
    
    for _, slave := range sm.slaves {
        slavesInfo = append(slavesInfo, slave.GetStatus())
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(slavesInfo)
}

func (sm *SlaveManager) handleAPIData(w http.ResponseWriter, r *http.Request) {
    portStr := r.URL.Query().Get("port")
    dataType := r.URL.Query().Get("type")
    startStr := r.URL.Query().Get("start")
    countStr := r.URL.Query().Get("count")
    
    port, _ := strconv.Atoi(portStr)
    start, _ := strconv.Atoi(startStr)
    count, _ := strconv.Atoi(countStr)
    
    // 找到對應的 slave
    var targetSlave *ModbusSlave
    for _, slave := range sm.slaves {
        if slave.port == port {
            targetSlave = slave
            break
        }
    }
    
    if targetSlave == nil {
        http.Error(w, "Slave not found", http.StatusNotFound)
        return
    }
    
    data := targetSlave.GetDataRange(dataType, start, count)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(data)
}

// SlaveManager 管理多個 slaves
type SlaveManager struct {
    slaves      []*ModbusSlave
    startPort   int
    numSlaves   int
    wg          sync.WaitGroup
}

func NewSlaveManager(startPort, numSlaves int) *SlaveManager {
    return &SlaveManager{
        startPort: startPort,
        numSlaves: numSlaves,
        slaves:    make([]*ModbusSlave, 0, numSlaves),
    }
}

func (sm *SlaveManager) CreateSlaves() error {
    log.Printf("🏗️ 開始創建 %d 個 Modbus slaves...", sm.numSlaves)
    
    for i := 0; i < sm.numSlaves; i++ {
        slaveID := byte(1) // 所有 slaves 都使用 ID 1，這樣可以用相同的 -a 1 參數
        port := sm.startPort + i
        
        slave := NewModbusSlave(slaveID, port)
        slave.InitializeTestData()
        sm.slaves = append(sm.slaves, slave)
        
        if (i+1)%100 == 0 {
            log.Printf("📦 已創建 %d/%d slaves", i+1, sm.numSlaves)
        }
    }
    
    log.Printf("✅ 所有 %d 個 slaves 創建完成", sm.numSlaves)
    return nil
}

func (sm *SlaveManager) StartAll() error {
    log.Printf("🚀 開始啟動 %d 個 slaves...", len(sm.slaves))
    
    // 分批啟動以避免系統負載過大
    batchSize := 50
    batches := (len(sm.slaves) + batchSize - 1) / batchSize
    
    for batch := 0; batch < batches; batch++ {
        start := batch * batchSize
        end := start + batchSize
        if end > len(sm.slaves) {
            end = len(sm.slaves)
        }
        
        log.Printf("📦 啟動批次 %d/%d (Slaves %d-%d)", batch+1, batches, start, end-1)
        
        var batchWg sync.WaitGroup
        errChan := make(chan error, end-start)
        
        for i := start; i < end; i++ {
            batchWg.Add(1)
            go func(slave *ModbusSlave) {
                defer batchWg.Done()
                if err := slave.Start(); err != nil {
                    errChan <- fmt.Errorf("slave %d start failed: %v", slave.slaveID, err)
                }
            }(sm.slaves[i])
            
            // 小延遲避免端口衝突
            time.Sleep(10 * time.Millisecond)
        }
        
        batchWg.Wait()
        close(errChan)
        
        // 檢查是否有錯誤
        for err := range errChan {
            log.Printf("❌ %v", err)
        }
        
        // 批次間暫停
        if batch < batches-1 {
            time.Sleep(100 * time.Millisecond)
        }
    }
    
    log.Printf("🎉 所有 slaves 啟動完成！")
    return nil
}

func (sm *SlaveManager) PrintStats() {
    log.Println("📊 ===== Slaves 統計資訊 =====")
    
    runningCount := 0
    for _, slave := range sm.slaves {
        if slave.isRunning {
            runningCount++
        }
    }
    
    log.Printf("📈 運行中的 slaves: %d/%d", runningCount, len(sm.slaves))
    log.Printf("🔌 端口範圍: %d - %d", sm.startPort, sm.startPort+len(sm.slaves)-1)
    
    // 顯示前幾個 slave 的詳細狀態
    log.Println("🔍 前 5 個 slaves 詳細狀態:")
    for i := 0; i < 5 && i < len(sm.slaves); i++ {
        stats := sm.slaves[i].GetStatus()
        log.Printf("   Slave %d (Port %d): 線圈[0]=%t, 保持寄存器[0]=%d, 輸入寄存器[0]=%d", 
            stats["slave_id"], stats["port"], stats["sample_coil_0"], 
            stats["sample_holding_0"], stats["sample_input_0"])
    }
}

func (sm *SlaveManager) StopAll() {
    log.Println("🛑 停止所有 slaves...")
    
    var wg sync.WaitGroup
    for _, slave := range sm.slaves {
        wg.Add(1)
        go func(s *ModbusSlave) {
            defer wg.Done()
            s.Stop()
        }(slave)
    }
    
    wg.Wait()
    log.Println("✅ 所有 slaves 已停止")
}

func main() {
    if len(os.Args) < 3 {
        fmt.Println("使用方法: go run main.go <起始端口> <slaves數量>")
        fmt.Println("範例:")
        fmt.Println("  go run main.go 10000 10     # 創建 10 個 slaves，端口 10000-10009")
        fmt.Println("  go run main.go 5000 100     # 創建 100 個 slaves，端口 5000-5099")
        fmt.Println("  go run main.go 502 5        # 創建 5 個 slaves，端口 502-506")
        os.Exit(1)
    }
    
    var startPort, numSlaves int
    fmt.Sscanf(os.Args[1], "%d", &startPort)
    fmt.Sscanf(os.Args[2], "%d", &numSlaves)
    
    if numSlaves <= 0 || numSlaves > 10000 {
        log.Fatal("❌ slaves 數量必須在 1-10000 之間")
    }
    
    if startPort < 1024 || startPort+numSlaves > 65535 {
        log.Fatal("❌ 端口範圍無效")
    }
    
    // 創建管理器
    manager := NewSlaveManager(startPort, numSlaves)
    
    // 設置信號處理
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        log.Println("\n🔔 收到停止信號")
        manager.StopAll()
        os.Exit(0)
    }()
    
    // 創建並啟動所有 slaves
    log.Printf("🎯 準備創建 %d 個完整功能的 Modbus slaves", numSlaves)
    log.Printf("📍 端口範圍: %d - %d", startPort, startPort+numSlaves-1)
    
    if err := manager.CreateSlaves(); err != nil {
        log.Fatalf("❌ 創建 slaves 失敗: %v", err)
    }
    
    if err := manager.StartAll(); err != nil {
        log.Fatalf("❌ 啟動 slaves 失敗: %v", err)
    }
    
    // 啟動 Web 監控界面
    webPort := 8080
    manager.StartWebServer(webPort)
    
    // 定期統計
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            manager.PrintStats()
        }
    }()
    
    log.Println("🎉 所有 Modbus slaves 運行中...")
    log.Println("💡 每個 slave 都支援完整的 Modbus 功能:")
    log.Println("   📖 讀取: 線圈、離散輸入、保持寄存器、輸入寄存器")  
    log.Println("   ✏️ 寫入: 線圈、保持寄存器 (單個和多個)")
    log.Println("   🔄 自動模擬: 輸入寄存器和離散輸入數據變化")
    log.Println("   🔒 線程安全: 所有操作都有 mutex 保護")
    log.Printf("\n📋 測試命令範例 (使用 mbpoll):")
    log.Printf("   # 讀取第一個 slave 的保持寄存器")
    log.Printf("   mbpoll -a 1 -r 0 -c 10 -t 4 -1 127.0.0.1 -p %d", startPort)
    log.Printf("   # 讀取第二個 slave 的線圈")
    log.Printf("   mbpoll -a 2 -r 0 -c 10 -t 0 -1 127.0.0.1 -p %d", startPort+1)
    log.Printf("   # 寫入第一個 slave 的寄存器")
    log.Printf("   mbpoll -a 1 -r 10 -t 4 127.0.0.1 -p %d 1234", startPort)
    log.Printf("\n🌐 Web 監控界面: http://localhost:8080")
    log.Println("\n按 Ctrl+C 停止所有服務...")
    
    // 保持運行
    select {}
}