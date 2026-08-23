// Package review 实现复核流程：区段标注、人工分段确认与解释包版本管理。
// 复核阶段接收工程师的标注意见与分段修正，并驱动解释包的草稿/发布/替代。
package review

import (
	"errors"
	"fmt"
	"time"
)

// AnnotationInput 标注输入。
type AnnotationInput struct {
	BatchID    string
	TargetType string // track | window
	TargetID   string
	Note       string
	Author     string
}

// Validate 校验标注输入。
func (in AnnotationInput) Validate() error {
	if in.BatchID == "" {
		return errors.New("batchId required")
	}
	if in.TargetType != "track" && in.TargetType != "window" {
		return fmt.Errorf("targetType must be track|window, got %q", in.TargetType)
	}
	if in.TargetID == "" {
		return errors.New("targetId required")
	}
	if in.Note == "" {
		return errors.New("note required")
	}
	if in.Author == "" {
		return errors.New("author required")
	}
	return nil
}

// SegmentDecision 人工分段确认。
type SegmentDecision struct {
	TrackID   string
	StartIdx  int
	EndIdx    int
	Label     string
	Author    string
	Confirmed bool // true=确认该轨迹段；false=仅记录标注
}

// Validate 校验分段决策。
func (d SegmentDecision) Validate() error {
	if d.TrackID == "" {
		return errors.New("trackId required")
	}
	if d.StartIdx < 0 || d.EndIdx < 0 || d.EndIdx < d.StartIdx {
		return fmt.Errorf("invalid segment range [%d,%d]", d.StartIdx, d.EndIdx)
	}
	if d.Author == "" {
		return errors.New("author required")
	}
	return nil
}

// PublishRequest 解释包发布请求。
type PublishRequest struct {
	BatchID string
	Title   string
	Author  string
}

// Validate 校验发布请求。
func (r PublishRequest) Validate() error {
	if r.BatchID == "" {
		return errors.New("batchId required")
	}
	if r.Title == "" {
		return errors.New("title required")
	}
	if r.Author == "" {
		return errors.New("author required")
	}
	return nil
}

// SupersedeRequest 解释包替代请求：发布新版本替换旧包。
type SupersedeRequest struct {
	OldVersion int
	NewTitle   string
	Author     string
}

// Validate 校验替代请求。
func (r SupersedeRequest) Validate() error {
	if r.OldVersion < 1 {
		return errors.New("oldVersion must be >= 1")
	}
	if r.NewTitle == "" {
		return errors.New("newTitle required")
	}
	if r.Author == "" {
		return errors.New("author required")
	}
	return nil
}

// ReviewSummary 复核摘要：一次复核会话的产出描述。
type ReviewSummary struct {
	BatchID      string    `json:"batchId"`
	Annotations  int       `json:"annotations"`
	Segments     int       `json:"segments"`
	TrackCount   int       `json:"trackCount"`
	RecomputedAt time.Time `json:"recomputedAt"`
}

// NewReviewSummary 构造复核摘要。
func NewReviewSummary(batchID string, annotations, segments, trackCount int) *ReviewSummary {
	return &ReviewSummary{
		BatchID:      batchID,
		Annotations:  annotations,
		Segments:     segments,
		TrackCount:   trackCount,
		RecomputedAt: time.Now().UTC(),
	}
}
