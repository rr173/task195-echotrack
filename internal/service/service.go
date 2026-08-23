// Package service 编排业务闭环：采集 -> 校正 -> 拟合 -> 复核 -> 发布。
// Service 依赖 store 层做持久化，调用 correction/fitting/review 完成计算。
package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// IDGen 生成唯一 ID（短随机 hex）。
func IDGen(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

// Now 返回 UTC 时间。
func Now() time.Time { return time.Now().UTC() }
