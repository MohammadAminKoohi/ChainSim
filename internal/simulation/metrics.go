package simulation

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogEntry is a single JSONL line written by the MetricsCollector.
type LogEntry struct {
	Timestamp int64       `json:"timestamp"`
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
}

// BlockMinedData records when a miner finds a valid block.
type BlockMinedData struct {
	MinerID    string  `json:"miner_id"`
	Hash       string  `json:"hash"`
	Difficulty float64 `json:"difficulty"`
	Height     uint64  `json:"height"`
}

// TipUpdatedData records when a miner's view of the heaviest tip changes.
type TipUpdatedData struct {
	NodeID string `json:"node_id"`
	OldTip string `json:"old_tip"`
	NewTip string `json:"new_tip"`
	Height uint64 `json:"height"`
}

// MinerStatusData records when a miner joins or leaves the network.
type MinerStatusData struct {
	MinerID string `json:"miner_id"`
	Status  string `json:"status"` // "joined" or "left"
}

// MetricsCollector is a thread-safe JSONL writer for simulation telemetry.
type MetricsCollector struct {
	mu      sync.Mutex
	file    *os.File
	writer  *bufio.Writer
	encoder *json.Encoder
}

// NewMetricsCollector creates a new collector writing to the given file under resultsDir.
// The file is truncated on open to prevent stale data accumulation from previous runs.
func NewMetricsCollector(resultsDir, filename string) (*MetricsCollector, error) {
	// Ensure the results directory exists.
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return nil, err
	}

	fullPath := filepath.Join(resultsDir, filename)

	// O_TRUNC: start fresh each run (fixes stale data accumulation bug).
	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
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

// Close flushes buffered data and closes the file.
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

// LogBlockMined records a block_mined event.
func (mc *MetricsCollector) LogBlockMined(minerID, hash string, difficulty float64, height uint64) {
	mc.emit("block_mined", BlockMinedData{
		MinerID:    minerID,
		Hash:       hash,
		Difficulty: difficulty,
		Height:     height,
	})
}

// LogTipUpdated records a tip_updated event.
func (mc *MetricsCollector) LogTipUpdated(nodeID, oldTip, newTip string, height uint64) {
	mc.emit("tip_updated", TipUpdatedData{
		NodeID: nodeID,
		OldTip: oldTip,
		NewTip: newTip,
		Height: height,
	})
}

// LogMinerStatus records a miner_status event (joined/left).
func (mc *MetricsCollector) LogMinerStatus(minerID, status string) {
	mc.emit("miner_status", MinerStatusData{
		MinerID: minerID,
		Status:  status,
	})
}