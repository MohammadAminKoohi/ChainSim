package simulation

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp int64       `json:"timestamp"`
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
}

type BlockMinedData struct {
	MinerID    string  `json:"miner_id"`
	Hash       string  `json:"hash"`
	Difficulty float64 `json:"difficulty"`
	Height     uint64  `json:"height"`
}

type TipUpdatedData struct {
	NodeID string `json:"node_id"`
	OldTip string `json:"old_tip"`
	NewTip string `json:"new_tip"`
	Height uint64 `json:"height"`
}

type MinerStatusData struct {
	MinerID string `json:"miner_id"`
	Status  string `json:"status"` 
}

type MetricsCollector struct {
	mu      sync.Mutex
	file    *os.File
	writer  *bufio.Writer
	encoder *json.Encoder
}

func NewMetricsCollector(filename string) (*MetricsCollector, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriterSize(file, 64*1024)
	
	encoder := json.NewEncoder(writer)

	return &MetricsCollector{
		file:    file,
		writer:  writer,
		encoder: encoder,
	}, nil
}

func (mc *MetricsCollector) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if err := mc.writer.Flush(); err != nil {
		return err
	}
	return mc.file.Close()
}

func (mc *MetricsCollector) emit(eventType string, data interface{}) {
	entry := LogEntry{
		Timestamp: time.Now().UnixMilli(),
		Type:      eventType,
		Data:      data,
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	_ = mc.encoder.Encode(entry)
}

func (mc *MetricsCollector) LogBlockMined(minerID string, hash string, difficulty float64, height uint64) {
	mc.emit("block_mined", BlockMinedData{
		MinerID:    minerID,
		Hash:       hash,
		Difficulty: difficulty,
		Height:     height,
	})
}

func (mc *MetricsCollector) LogTipUpdated(nodeID string, oldTip string, newTip string, height uint64) {
	mc.emit("tip_updated", TipUpdatedData{
		NodeID:  nodeID,
		OldTip:  oldTip,
		NewTip:  newTip,
		Height:  height,
	})
}

func (mc *MetricsCollector) LogMinerStatus(minerID string, status string) {
	mc.emit("miner_status", MinerStatusData{
		MinerID: minerID,
		Status:  status,
	})
}