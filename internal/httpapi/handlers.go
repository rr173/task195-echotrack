package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"task195-echotrack/internal/fitting"
	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/model"
	"task195-echotrack/internal/review"
	"task195-echotrack/internal/service"
)

// Handler 聚合各领域服务，作为 HTTP 处理器实现。
type Handler struct {
	arrays  *service.ArrayService
	batches *service.BatchService
	process *service.ProcessService
	review  *service.ReviewService
}

// NewHandler 构造 Handler。
func NewHandler(arr *service.ArrayService, batch *service.BatchService,
	proc *service.ProcessService, rev *service.ReviewService) *Handler {
	return &Handler{arrays: arr, batches: batch, process: proc, review: rev}
}

// ctx 提取请求上下文。
func ctx(r *http.Request) context.Context { return r.Context() }

// httpStatus 错误到 HTTP 状态映射。
func httpStatus(err error) int {
	switch err {
	case model.ErrNotFound:
		return http.StatusNotFound
	case model.ErrConflict, model.ErrDuplicateWindow, model.ErrImmutableSnapshot:
		return http.StatusConflict
	case model.ErrInvalidState:
		return http.StatusUnprocessableEntity
	case model.ErrFrozenBatch, model.ErrSeqRegression:
		return http.StatusLocked
	case model.ErrBadSampling, model.ErrUnknownElement, model.ErrBrokenPhase:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// handleErr 统一错误处理。
func handleErr(w http.ResponseWriter, err error) {
	writeError(w, httpStatus(err), err.Error())
}

// ---- 阵列 ----

// CreateArrayRequest 注册阵列请求。
type CreateArrayRequest struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	SampleRateHz float64 `json:"sampleRateHz"`
	ElementCount int     `json:"elementCount"`
}

// CreateArray POST /api/arrays
func (h *Handler) CreateArray(w http.ResponseWriter, r *http.Request) {
	var req CreateArrayRequest
	if !parseBody(w, r, &req) {
		return
	}
	arr, err := h.arrays.Register(ctx(r), req.ID, req.Name, req.SampleRateHz, req.ElementCount)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, arr)
}

// ListArrays GET /api/arrays
func (h *Handler) ListArrays(w http.ResponseWriter, r *http.Request) {
	list, err := h.arrays.List(ctx(r))
	if err != nil {
		handleErr(w, err)
		return
	}
	if list == nil {
		list = []*model.Array{}
	}
	writeJSON(w, http.StatusOK, list)
}

// GetArray GET /api/arrays/{id}
func (h *Handler) GetArray(w http.ResponseWriter, r *http.Request) {
	arr, err := h.arrays.Get(ctx(r), pathValue(r, "id"))
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, arr)
}

// ArrayStats GET /api/arrays/{id}/stats
func (h *Handler) ArrayStats(w http.ResponseWriter, r *http.Request) {
	st, err := h.arrays.Stats(ctx(r), pathValue(r, "id"))
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// UpdateElementDelayRequest 更新阵元延迟。
type UpdateElementDelayRequest struct {
	DelayUs float64 `json:"delayUs"`
}

// UpdateElementDelay PUT /api/arrays/{id}/elements/{elementNo}/delay
func (h *Handler) UpdateElementDelay(w http.ResponseWriter, r *http.Request) {
	var req UpdateElementDelayRequest
	if !parseBody(w, r, &req) {
		return
	}
	no, err := strconv.Atoi(pathValue(r, "elementNo"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid elementNo")
		return
	}
	el, err := h.arrays.UpdateElementDelay(ctx(r), pathValue(r, "id"), no, req.DelayUs)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, el)
}

// ---- 批次 ----

// CreateBatchRequest 创建批次。
type CreateBatchRequest struct {
	ID               string `json:"id"`
	ArrayID          string `json:"arrayId"`
	ReferenceElement int    `json:"referenceElement"`
}

// CreateBatch POST /api/batches
func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var req CreateBatchRequest
	if !parseBody(w, r, &req) {
		return
	}
	batch, err := h.batches.Create(ctx(r), req.ID, req.ArrayID, req.ReferenceElement)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, batch)
}

// ListBatches GET /api/batches?arrayId=
func (h *Handler) ListBatches(w http.ResponseWriter, r *http.Request) {
	list, err := h.batches.List(ctx(r), r.URL.Query().Get("arrayId"))
	if err != nil {
		handleErr(w, err)
		return
	}
	if list == nil {
		list = []*model.Batch{}
	}
	writeJSON(w, http.StatusOK, list)
}

// GetBatch GET /api/batches/{id}
func (h *Handler) GetBatch(w http.ResponseWriter, r *http.Request) {
	batch, err := h.batches.Get(ctx(r), pathValue(r, "id"))
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

// UploadWindow POST /api/batches/{id}/windows
func (h *Handler) UploadWindow(w http.ResponseWriter, r *http.Request) {
	var in ingest.WindowInput
	if !parseBody(w, r, &in) {
		return
	}
	batchID := pathValue(r, "id")
	batch, receipt, err := h.batches.IngestWindowForBatch(ctx(r), batchID, in)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"batch":   batch,
		"receipt": receipt,
	})
}

// ListWindows GET /api/batches/{id}/windows
func (h *Handler) ListWindows(w http.ResponseWriter, r *http.Request) {
	list, err := h.batches.ListWindows(ctx(r), pathValue(r, "id"))
	if err != nil {
		handleErr(w, err)
		return
	}
	if list == nil {
		list = []*model.Window{}
	}
	writeJSON(w, http.StatusOK, list)
}

// GetWindow GET /api/batches/{id}/windows/{windowId}
func (h *Handler) GetWindow(w http.ResponseWriter, r *http.Request) {
	win, err := h.batches.GetWindow(ctx(r), pathValue(r, "windowId"))
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, win)
}

// IgnoreWindow POST /api/batches/{id}/windows/{elementNo}/{seqNo}/ignore
func (h *Handler) IgnoreWindow(w http.ResponseWriter, r *http.Request) {
	no, err := strconv.Atoi(pathValue(r, "elementNo"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid elementNo")
		return
	}
	seq, err := strconv.ParseInt(pathValue(r, "seqNo"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid seqNo")
		return
	}
	if err := h.batches.IgnoreWindow(ctx(r), pathValue(r, "id"), no, seq); err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
}

// ProcessBatch POST /api/batches/{id}/process
func (h *Handler) ProcessBatch(w http.ResponseWriter, r *http.Request) {
	res, err := h.process.Process(ctx(r), pathValue(r, "id"), fitting.DefaultOptions())
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ChangeReference POST /api/batches/{id}/reference/{elementNo}
func (h *Handler) ChangeReference(w http.ResponseWriter, r *http.Request) {
	no, err := strconv.Atoi(pathValue(r, "elementNo"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid elementNo")
		return
	}
	res, err := h.process.DifferentialRecompute(ctx(r), pathValue(r, "id"), no)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---- 轨迹 ----

// ListTracks GET /api/batches/{id}/tracks
func (h *Handler) ListTracks(w http.ResponseWriter, r *http.Request) {
	tracks, err := h.process.Tracks(ctx(r), pathValue(r, "id"))
	if err != nil {
		handleErr(w, err)
		return
	}
	if tracks == nil {
		tracks = []*model.Track{}
	}
	writeJSON(w, http.StatusOK, tracks)
}

// GetTrack GET /api/tracks/{trackId}
func (h *Handler) GetTrack(w http.ResponseWriter, r *http.Request) {
	track, err := h.process.Track(ctx(r), pathValue(r, "trackId"))
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, track)
}

// AddSegmentRequest 添加轨迹段。
type AddSegmentRequest struct {
	StartIdx  int    `json:"startIdx"`
	EndIdx    int    `json:"endIdx"`
	Label     string `json:"label"`
	Author    string `json:"author"`
	Confirmed bool   `json:"confirmed"`
}

// AddSegment POST /api/tracks/{trackId}/segments
func (h *Handler) AddSegment(w http.ResponseWriter, r *http.Request) {
	var req AddSegmentRequest
	if !parseBody(w, r, &req) {
		return
	}
	seg, err := h.review.AddSegment(ctx(r), review.SegmentDecision{
		TrackID: pathValue(r, "trackId"), StartIdx: req.StartIdx, EndIdx: req.EndIdx,
		Label: req.Label, Author: req.Author, Confirmed: req.Confirmed,
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, seg)
}

// ListSegments GET /api/tracks/{trackId}/segments
func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	list, err := h.review.ListSegments(ctx(r), pathValue(r, "trackId"))
	if err != nil {
		handleErr(w, err)
		return
	}
	if list == nil {
		list = []*model.Segment{}
	}
	writeJSON(w, http.StatusOK, list)
}

// ConfirmTrack POST /api/tracks/{trackId}/confirm
func (h *Handler) ConfirmTrack(w http.ResponseWriter, r *http.Request) {
	if err := h.review.ConfirmTrack(ctx(r), pathValue(r, "trackId")); err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

// ---- 标注 ----

// AddAnnotationRequest 添加标注。
type AddAnnotationRequest struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	Note       string `json:"note"`
	Author     string `json:"author"`
}

// AddAnnotation POST /api/batches/{id}/annotations
func (h *Handler) AddAnnotation(w http.ResponseWriter, r *http.Request) {
	var req AddAnnotationRequest
	if !parseBody(w, r, &req) {
		return
	}
	a, err := h.review.AddAnnotation(ctx(r), review.AnnotationInput{
		BatchID: pathValue(r, "id"), TargetType: req.TargetType,
		TargetID: req.TargetID, Note: req.Note, Author: req.Author,
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// ListAnnotations GET /api/batches/{id}/annotations
func (h *Handler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	list, err := h.review.ListAnnotations(ctx(r), pathValue(r, "id"))
	if err != nil {
		handleErr(w, err)
		return
	}
	if list == nil {
		list = []*model.Annotation{}
	}
	writeJSON(w, http.StatusOK, list)
}

// ---- 解释包 ----

// CreateInterpretationRequest 创建解释包。
type CreateInterpretationRequest struct {
	Title string `json:"title"`
}

// CreateInterpretation POST /api/batches/{id}/interpretations
func (h *Handler) CreateInterpretation(w http.ResponseWriter, r *http.Request) {
	var req CreateInterpretationRequest
	if !parseBody(w, r, &req) {
		return
	}
	in, err := h.review.CreateInterpretation(ctx(r), pathValue(r, "id"), req.Title)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, in)
}

// ListInterpretations GET /api/batches/{id}/interpretations
func (h *Handler) ListInterpretations(w http.ResponseWriter, r *http.Request) {
	list, err := h.review.ListInterpretations(ctx(r), pathValue(r, "id"))
	if err != nil {
		handleErr(w, err)
		return
	}
	if list == nil {
		list = []*model.Interpretation{}
	}
	writeJSON(w, http.StatusOK, list)
}

// PublishInterpretation POST /api/interpretations/{interpretationId}/publish
func (h *Handler) PublishInterpretation(w http.ResponseWriter, r *http.Request) {
	in, err := h.review.PublishInterpretation(ctx(r), pathValue(r, "interpretationId"))
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// SupersedeRequest 替代请求。
type SupersedeRequest struct {
	OldVersion int    `json:"oldVersion"`
	NewTitle   string `json:"newTitle"`
	Author     string `json:"author"`
}

// SupersedeInterpretation POST /api/interpretations/supersede
func (h *Handler) SupersedeInterpretation(w http.ResponseWriter, r *http.Request) {
	var req SupersedeRequest
	if !parseBody(w, r, &req) {
		return
	}
	in, err := h.review.SupersedeInterpretation(ctx(r), review.SupersedeRequest{
		OldVersion: req.OldVersion, NewTitle: req.NewTitle, Author: req.Author,
	})
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, in)
}

// ---- 系统 ----

// SystemStats GET /api/stats
func (h *Handler) SystemStats(w http.ResponseWriter, r *http.Request) {
	st, err := h.process.SystemStats(ctx(r))
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Health GET /api/health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
