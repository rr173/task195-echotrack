// Package correction 实现阵元延迟补偿与相位连续性分析。
// 输入为复数采样（I/Q），输出解卷绕相位、延迟旋转量与断裂检测结果。
package correction

import (
	"math"
)

// complexSample 复数采样点。
type complexSample struct {
	I float64
	Q float64
}

// Phase 计算复数采样点的相位（弧度，范围 [-pi, pi]）。
func Phase(i, q float64) float64 {
	return math.Atan2(q, i)
}

// PhaseOf 逐点计算相位序列。
func PhaseOf(i, q []float64) []float64 {
	n := min(len(i), len(q))
	out := make([]float64, n)
	for k := 0; k < n; k++ {
		out[k] = Phase(i[k], q[k])
	}
	return out
}

// UnwrapPhase 对相位序列解卷绕：相邻差值超过 pi 时累加 2*pi 倍数，
// 得到连续单调相位，消除 ±pi 边界跳变。
func UnwrapPhase(phase []float64) []float64 {
	n := len(phase)
	if n == 0 {
		return nil
	}
	out := make([]float64, n)
	out[0] = phase[0]
	accum := 0.0
	for k := 1; k < n; k++ {
		delta := phase[k] - phase[k-1]
		// 把 delta 归一到 (-pi, pi]
		for delta > math.Pi {
			delta -= 2 * math.Pi
		}
		for delta <= -math.Pi {
			delta += 2 * math.Pi
		}
		accum += delta
		out[k] = phase[0] + accum
	}
	return out
}

// delayRotationRad 计算延迟对应的相位旋转量：rotation = 2*pi * f * delay。
func delayRotationRad(delayUs, sampleRateHz float64) float64 {
	if sampleRateHz <= 0 {
		return 0
	}
	// 以采样率对应频率（每秒采样数），延迟微秒换算为秒。
	delaySec := delayUs / 1e6
	return 2 * math.Pi * sampleRateHz * delaySec
}

// CorrectWindow 对窗口做延迟补偿：将复数采样按延迟旋转，
// 返回校正后的相位、旋转量与应用后的复数采样。
func CorrectWindow(i, q []float64, delayUs, sampleRateHz float64) (rotatedI, rotatedQ []float64, rotationRad float64, err error) {
	if len(i) != len(q) {
		return nil, nil, 0, errLengthMismatch
	}
	if len(i) == 0 {
		return nil, nil, 0, errEmptyWindow
	}
	rotationRad = delayRotationRad(delayUs, sampleRateHz)
	cosA := math.Cos(rotationRad)
	sinA := math.Sin(rotationRad)
	rotatedI = make([]float64, len(i))
	rotatedQ = make([]float64, len(i))
	for k := 0; k < len(i); k++ {
		// 复数旋转：z' = z * e^{jA}
		rotatedI[k] = i[k]*cosA - q[k]*sinA
		rotatedQ[k] = i[k]*sinA + q[k]*cosA
	}
	return rotatedI, rotatedQ, rotationRad, nil
}

// AnalyzeContinuity 分析解卷绕相位的连续性：返回断裂点索引与累计跳变数。
// 阈值参数 thresholdRad 用于判定相邻相位差是否构成断裂。
func AnalyzeContinuity(phase []float64, thresholdRad float64) (brokenAt []int, jumps int) {
	if len(phase) < 2 {
		return nil, 0
	}
	for k := 1; k < len(phase); k++ {
		delta := math.Abs(phase[k] - phase[k-1])
		if delta > thresholdRad {
			brokenAt = append(brokenAt, k)
			jumps++
		}
	}
	return brokenAt, jumps
}

// DifferentialPhase 以参考阵元相位为基准计算差分相位：
// diff[k] = phase[k] - refPhase[refIndex]（跨阵元对齐到相同时间索引）。
func DifferentialPhase(phase, refPhase []float64, refIndex int) []float64 {
	if refIndex < 0 || refIndex >= len(refPhase) {
		return nil
	}
	base := refPhase[refIndex]
	n := len(phase)
	out := make([]float64, n)
	for k := 0; k < n; k++ {
		out[k] = phase[k] - base
	}
	return out
}

// EstimateDelay 基于两阵元相位差估计相对延迟（微秒）。
// 假设相位差线性于频率，delayUs = phaseDiff / (2*pi*f) * 1e6。
func EstimateDelay(phaseDiff, sampleRateHz float64) float64 {
	if sampleRateHz <= 0 {
		return 0
	}
	return phaseDiff / (2 * math.Pi * sampleRateHz) * 1e6
}

var (
	errLengthMismatch = errX("i/q length mismatch")
	errEmptyWindow    = errX("empty window")
)

type errX string

func (e errX) Error() string { return string(e) }
