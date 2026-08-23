// Package model 定义声学阵列回波相位追踪台的领域实体与状态机。
// 实体按“阵列 -> 批次 -> 采样窗口 -> 校正记录 -> 回波轨迹 -> 轨迹段 -> 标注 -> 解释包”组织。
package model

import (
	"errors"
	"fmt"
	"time"
)

// 批次状态机：uploading -> processing -> reviewing -> published -> archived
const (
	BatchStatusUploading  = "uploading"   // 上传中：可继续接收采样窗口
	BatchStatusProcessing = "processing"  // 处理中：延迟校正 + 相位拼接 + 轨迹拟合进行中
	BatchStatusReviewing  = "reviewing"   // 待复核：轨迹已生成，等待工程师标注/确认
	BatchStatusPublished  = "published"   // 已发布：解释包已冻结，输入不可再改
	BatchStatusArchived   = "archived"    // 已封存：仅保留历史只读访问
)

// 采样窗口状态机：new -> corrected -> phase_broken / ignored
const (
	WindowStatusNew          = "new"          // 新建：尚未校正
	WindowStatusCorrected    = "corrected"    // 已校正：延迟已补偿，相位已解卷绕
	WindowStatusPhaseBroken  = "phase_broken" // 相位断裂：存在无法修复的跳变
	WindowStatusIgnored      = "ignored"      // 已忽略：复核者显式排除
)

// 回波轨迹状态机：fitting -> continuous / needs_segmentation -> confirmed
const (
	TrackStatusFitting            = "fitting"            // 拟合中：正在跨窗口拼接
	TrackStatusContinuous         = "continuous"         // 连续：相位全程连续
	TrackStatusNeedsSegmentation  = "needs_segmentation" // 需人工分段：存在断裂区段
	TrackStatusConfirmed          = "confirmed"          // 已确认：人工确认后锁定
)

// 解释包状态机：draft -> published -> superseded
const (
	InterpretationStatusDraft      = "draft"      // 草稿：可编辑
	InterpretationStatusPublished  = "published"  // 已发布：冻结输入快照
	InterpretationStatusSuperseded = "superseded" // 已替代：新版本发布后旧包标记
)

// 常见领域错误。
var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidState      = errors.New("invalid state transition")
	ErrDuplicateWindow   = errors.New("duplicate window")
	ErrBadSampling       = errors.New("bad sampling data")
	ErrUnknownElement    = errors.New("unknown element")
	ErrBrokenPhase       = errors.New("phase continuity broken")
	ErrFrozenBatch       = errors.New("batch is frozen")
	ErrImmutableSnapshot = errors.New("interpretation snapshot is immutable")
	ErrSeqRegression     = errors.New("sequence number regression")
)

// Element 阵元配置：编号、几何位置与硬件延迟校正量。
type Element struct {
	ArrayID   string  `json:"arrayId"`
	ElementNo int     `json:"elementNo"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	DelayUs   float64 `json:"delayUs"` // 阵元固有延迟（微秒），校正时补偿
}

// Array 阵列：一组带编号的声学阵元，拥有统一采样率。
type Array struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SampleRateHz float64   `json:"sampleRateHz"`
	Elements     []Element `json:"elements"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ArrayStats 阵列统计快照。
type ArrayStats struct {
	ID         string `json:"id"`
	BatchCount int    `json:"batchCount"`
}

// Batch 阵列批次：一次采集任务，包含若干采样窗口。
type Batch struct {
	ID                string    `json:"id"`
	ArrayID           string    `json:"arrayId"`
	Status            string    `json:"status"`
	ReferenceElement  int       `json:"referenceElement"` // 参考阵元：差分相位基准
	WindowCursor      int64     `json:"windowCursor"`     // 已处理窗口游标（按 seqNo）
	SampleRateHz      float64   `json:"sampleRateHz"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// Window 采样窗口：单个阵元的一段复数采样（I/Q 序列）。
type Window struct {
	ID        string    `json:"id"`
	BatchID   string    `json:"batchId"`
	ElementNo int       `json:"elementNo"`
	SeqNo     int64     `json:"seqNo"` // 阵元内序号，幂等键 (batchId, elementNo, seqNo)
	I         []float64 `json:"i"`
	Q         []float64 `json:"q"`
	SampleRate float64  `json:"sampleRate"`
	Status    string    `json:"status"`
	Checksum  string    `json:"checksum"` // 内容指纹，重复窗口校验
	CreatedAt time.Time `json:"createdAt"`
}

// Correction 校正记录：某窗口的延迟补偿与相位旋转结果。
type Correction struct {
	ID         string    `json:"id"`
	BatchID    string    `json:"batchId"`
	WindowID   string    `json:"windowId"`
	ElementNo  int       `json:"elementNo"`
	DelayUs    float64   `json:"delayUs"`
	RotationRad float64  `json:"rotationRad"` // 延迟对应的相位旋转量
	Unwrapped  []float64 `json:"unwrapped"`   // 解卷绕后的连续相位
	AppliedAt  time.Time `json:"appliedAt"`
}

// Track 回波轨迹：跨窗口拼接出的相位连续路径。
type Track struct {
	ID          string    `json:"id"`
	BatchID     string    `json:"batchId"`
	ElementNo   int       `json:"elementNo"` // 主导阵元
	Status      string    `json:"status"`
	PhasePoints []float64 `json:"phasePoints"`
	TimePoints  []float64 `json:"timePoints"` // 对应时间（秒）
	BrokenAt    []int     `json:"brokenAt"`   // 断裂点索引
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Segment 轨迹段：人工或自动划定的连续区间。
type Segment struct {
	ID        string    `json:"id"`
	TrackID   string    `json:"trackId"`
	StartIdx  int       `json:"startIdx"`
	EndIdx    int       `json:"endIdx"`
	Label     string    `json:"label"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

// Annotation 区段标注：工程师对可疑区段的注释。
type Annotation struct {
	ID        string    `json:"id"`
	BatchID   string    `json:"batchId"`
	TargetType string   `json:"targetType"` // track | window
	TargetID  string    `json:"targetId"`
	Note      string    `json:"note"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

// Interpretation 解释包：冻结的轨迹解释版本，绑定输入快照。
type Interpretation struct {
	ID          string    `json:"id"`
	BatchID     string    `json:"batchId"`
	Version     int       `json:"version"`
	Status      string    `json:"status"`
	Title       string    `json:"title"`
	SnapshotRef string    `json:"snapshotRef"` // 快照键（batchId@version）
	TrackCount  int       `json:"trackCount"`
	CreatedAt   time.Time `json:"createdAt"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

// NewArray 构造阵列，填充默认阵元。
func NewArray(id, name string, sampleRateHz float64, elementCount int) (*Array, error) {
	if id == "" || name == "" {
		return nil, fmt.Errorf("array id and name required")
	}
	if sampleRateHz <= 0 {
		return nil, fmt.Errorf("sample rate must be positive")
	}
	if elementCount < 1 || elementCount > 64 {
		return nil, fmt.Errorf("element count out of range [1,64]")
	}
	a := &Array{ID: id, Name: name, SampleRateHz: sampleRateHz, CreatedAt: time.Now().UTC()}
	for i := 1; i <= elementCount; i++ {
		a.Elements = append(a.Elements, Element{
			ArrayID: id, ElementNo: i, Name: fmt.Sprintf("EL%d", i),
			X: float64(i), Y: 0, DelayUs: 0,
		})
	}
	return a, nil
}

// ElementByNo 查找阵元，不存在返回 ErrUnknownElement。
func (a *Array) ElementByNo(no int) (Element, error) {
	for _, e := range a.Elements {
		if e.ElementNo == no {
			return e, nil
		}
	}
	return Element{}, fmt.Errorf("%w: element %d", ErrUnknownElement, no)
}

// BatchState 返回批次当前状态。
func (b *Batch) BatchState() string { return b.Status }

// CanReceive 判断批次是否仍可接收窗口。
func (b *Batch) CanReceive() bool {
	return b.Status == BatchStatusUploading || b.Status == BatchStatusProcessing
}

// ValidateTransition 校验批次状态流转合法性。
func ValidateTransition(from, to string) error {
	allowed := map[string][]string{
		BatchStatusUploading:  {BatchStatusProcessing, BatchStatusArchived},
		BatchStatusProcessing: {BatchStatusReviewing, BatchStatusUploading, BatchStatusArchived},
		BatchStatusReviewing:  {BatchStatusPublished, BatchStatusProcessing, BatchStatusArchived},
		BatchStatusPublished:  {BatchStatusArchived},
		BatchStatusArchived:   {},
	}
	nexts, ok := allowed[from]
	if !ok {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidState, from)
	}
	for _, n := range nexts {
		if n == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidState, from, to)
}

// ValidateWindowTransition 校验窗口状态流转。
func ValidateWindowTransition(from, to string) error {
	allowed := map[string][]string{
		WindowStatusNew:         {WindowStatusCorrected, WindowStatusPhaseBroken, WindowStatusIgnored},
		WindowStatusCorrected:   {WindowStatusPhaseBroken, WindowStatusIgnored},
		WindowStatusPhaseBroken: {WindowStatusIgnored},
		WindowStatusIgnored:     {},
	}
	nexts, ok := allowed[from]
	if !ok {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidState, from)
	}
	for _, n := range nexts {
		if n == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidState, from, to)
}

// ValidateTrackTransition 校验轨迹状态流转。
func ValidateTrackTransition(from, to string) error {
	allowed := map[string][]string{
		TrackStatusFitting:           {TrackStatusContinuous, TrackStatusNeedsSegmentation},
		TrackStatusContinuous:        {TrackStatusConfirmed},
		TrackStatusNeedsSegmentation: {TrackStatusContinuous, TrackStatusConfirmed},
		TrackStatusConfirmed:         {},
	}
	nexts, ok := allowed[from]
	if !ok {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidState, from)
	}
	for _, n := range nexts {
		if n == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidState, from, to)
}

// ValidateInterpretationTransition 校验解释包状态流转。
func ValidateInterpretationTransition(from, to string) error {
	allowed := map[string][]string{
		InterpretationStatusDraft:      {InterpretationStatusPublished},
		InterpretationStatusPublished:  {InterpretationStatusSuperseded},
		InterpretationStatusSuperseded: {},
	}
	nexts, ok := allowed[from]
	if !ok {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidState, from)
	}
	for _, n := range nexts {
		if n == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidState, from, to)
}

// WithDelay returns the element configuration after applying a calibration value.
func (e Element) WithDelay(delayUs float64) Element {
	return e
}
