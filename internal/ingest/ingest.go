// Package ingest 实现采样窗口的幂等接收与一致性校验。
// 同一批次可并行接收多个阵元的窗口，但 (batchId, elementNo, seqNo) 必须唯一，
// 且采样率必须与阵列一致、序号不得倒退。
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
)

// WindowInput 窗口上传请求。
type WindowInput struct {
	BatchID    string    `json:"batchId"`
	ElementNo  int       `json:"elementNo"`
	SeqNo      int64     `json:"seqNo"`
	I          []float64 `json:"i"`
	Q          []float64 `json:"q"`
	SampleRate float64   `json:"sampleRate"`
}

// WithBatchID binds a request to the batch selected by the route, overriding
// any conflicting batchId carried in the request body. The URL path is the
// source of truth for batch identity, so the response and persisted window
// always belong to the path-specified batch.
func (w WindowInput) WithBatchID(pathBatchID string) WindowInput {
	w.BatchID = pathBatchID
	return w
}

// Validate 基础校验：非空、长度一致、采样率合法。
func (w WindowInput) Validate() error {
	if w.BatchID == "" {
		return fmt.Errorf("batchId required")
	}
	if w.ElementNo < 1 {
		return fmt.Errorf("elementNo must be >= 1")
	}
	if w.SeqNo < 0 {
		return fmt.Errorf("seqNo must be >= 0")
	}
	if len(w.I) != len(w.Q) {
		return fmt.Errorf("i/q length mismatch: %d vs %d", len(w.I), len(w.Q))
	}
	if len(w.I) == 0 {
		return fmt.Errorf("empty window")
	}
	if w.SampleRate <= 0 {
		return fmt.Errorf("sampleRate must be positive")
	}
	return nil
}

// ValidateSampleRate 校验窗口采样率与阵列采样率一致（相对误差 < 1e-6）。
func ValidateSampleRate(windowRate, arrayRate float64) error {
	if arrayRate <= 0 {
		return fmt.Errorf("array sampleRate must be positive")
	}
	if windowRate <= 0 {
		return fmt.Errorf("window sampleRate must be positive")
	}
	rel := math.Abs(windowRate-arrayRate) / arrayRate
	if rel > 1e-6 {
		return fmt.Errorf("sampleRate mismatch: window %.6f vs array %.6f", windowRate, arrayRate)
	}
	return nil
}

// Checksum 计算窗口内容指纹（sha256 of i/q bytes），用于幂等去重。
func Checksum(i, q []float64) string {
	h := sha256.New()
	for _, v := range i {
		var buf [8]byte
		u := math.Float64bits(v)
		for j := 0; j < 8; j++ {
			buf[j] = byte(u >> (8 * j))
		}
		h.Write(buf[:])
	}
	for _, v := range q {
		var buf [8]byte
		u := math.Float64bits(v)
		for j := 0; j < 8; j++ {
			buf[j] = byte(u >> (8 * j))
		}
		h.Write(buf[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IngestReceipt 接收回执：幂等命中时返回既有记录标记。
type IngestReceipt struct {
	WindowID   string `json:"windowId"`
	Inserted   bool   `json:"inserted"` // false 表示幂等命中已有窗口
	Duplicated bool   `json:"duplicated"`
	Checksum   string `json:"checksum"`
}

// NewReceipt 构造接收回执。
func NewReceipt(windowID, checksum string, inserted, duplicated bool) IngestReceipt {
	return IngestReceipt{WindowID: windowID, Inserted: inserted, Duplicated: duplicated, Checksum: checksum}
}
