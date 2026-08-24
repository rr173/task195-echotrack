package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/model"
)

// UpsertWindow 幂等写入窗口：以 (batch_id, element_no, seq_no) 为唯一键。
// 已存在时校验 checksum：内容一致返回既有记录（duplicated=true, inserted=false）；
// 内容不一致返回 ErrDuplicateWindow。
func (s *Store) UpsertWindow(ctx context.Context, w *model.Window) (ingest.IngestReceipt, bool, error) {
	// 先查既有记录。
	existing, err := s.getWindowByKey(ctx, w.BatchID, w.ElementNo, w.SeqNo)
	if err == nil {
		if existing.Checksum != w.Checksum {
			// 内容不一致：以 %w 包装 ErrDuplicateWindow，便于上层用 errors.Is 识别为冲突。
			return ingest.IngestReceipt{}, false, fmt.Errorf("%w: batch %s element %d seq %d",
				model.ErrDuplicateWindow, w.BatchID, w.ElementNo, w.SeqNo)
		}
		return ingest.IngestReceipt{WindowID: existing.ID, Inserted: false, Duplicated: true, Checksum: existing.Checksum}, false, nil
	}
	if err != model.ErrNotFound {
		return ingest.IngestReceipt{}, false, err
	}
	iJSON, err := MarshalFloats(w.I)
	if err != nil {
		return ingest.IngestReceipt{}, false, fmt.Errorf("marshal i: %w", err)
	}
	qJSON, err := MarshalFloats(w.Q)
	if err != nil {
		return ingest.IngestReceipt{}, false, fmt.Errorf("marshal q: %w", err)
	}
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO windows (id, batch_id, element_no, seq_no, i_json, q_json, sample_rate, status, checksum, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(batch_id, element_no, seq_no) DO NOTHING`,
		w.ID, w.BatchID, w.ElementNo, w.SeqNo, iJSON, qJSON, w.SampleRate, w.Status, w.Checksum,
		w.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return ingest.IngestReceipt{}, false, fmt.Errorf("insert window: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return ingest.IngestReceipt{}, false, fmt.Errorf("inspect window insert: %w", err)
	}
	if rows == 0 {
		existing, err := s.getWindowByKey(ctx, w.BatchID, w.ElementNo, w.SeqNo)
		if err != nil {
			return ingest.IngestReceipt{}, false, err
		}
		if existing.Checksum != w.Checksum {
			return ingest.IngestReceipt{}, false, model.ErrDuplicateWindow
		}
		return ingest.IngestReceipt{WindowID: existing.ID, Inserted: false, Duplicated: true, Checksum: existing.Checksum}, false, nil
	}
	return ingest.IngestReceipt{WindowID: w.ID, Inserted: true, Duplicated: false, Checksum: w.Checksum}, true, nil
}

// getWindowByKey 按唯一键查窗口。
func (s *Store) getWindowByKey(ctx context.Context, batchID string, elementNo int, seqNo int64) (*model.Window, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, batch_id, element_no, seq_no, i_json, q_json, sample_rate, status, checksum, created_at
		 FROM windows WHERE batch_id=? AND element_no=? AND seq_no=?`,
		batchID, elementNo, seqNo)
	var w model.Window
	var iJSON, qJSON, created string
	if err := row.Scan(&w.ID, &w.BatchID, &w.ElementNo, &w.SeqNo, &iJSON, &qJSON,
		&w.SampleRate, &w.Status, &w.Checksum, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan window: %w", err)
	}
	iVals, err := UnmarshalFloats(iJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal i: %w", err)
	}
	qVals, err := UnmarshalFloats(qJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal q: %w", err)
	}
	w.I = iVals
	w.Q = qVals
	w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return &w, nil
}

// GetWindow 按 ID 查窗口。
func (s *Store) GetWindow(ctx context.Context, id string) (*model.Window, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, batch_id, element_no, seq_no, i_json, q_json, sample_rate, status, checksum, created_at
		 FROM windows WHERE id=?`, id)
	var w model.Window
	var iJSON, qJSON, created string
	if err := row.Scan(&w.ID, &w.BatchID, &w.ElementNo, &w.SeqNo, &iJSON, &qJSON,
		&w.SampleRate, &w.Status, &w.Checksum, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan window: %w", err)
	}
	iVals, err := UnmarshalFloats(iJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal i: %w", err)
	}
	qVals, err := UnmarshalFloats(qJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal q: %w", err)
	}
	w.I = iVals
	w.Q = qVals
	w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return &w, nil
}

// ListWindows 列出批次窗口。
func (s *Store) ListWindows(ctx context.Context, batchID string) ([]*model.Window, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, batch_id, element_no, seq_no, i_json, q_json, sample_rate, status, checksum, created_at
		 FROM windows WHERE batch_id=? ORDER BY element_no, seq_no`, batchID)
	if err != nil {
		return nil, fmt.Errorf("query windows: %w", err)
	}
	defer rows.Close()
	var out []*model.Window
	for rows.Next() {
		var w model.Window
		var iJSON, qJSON, created string
		if err := rows.Scan(&w.ID, &w.BatchID, &w.ElementNo, &w.SeqNo, &iJSON, &qJSON,
			&w.SampleRate, &w.Status, &w.Checksum, &created); err != nil {
			return nil, fmt.Errorf("scan window: %w", err)
		}
		w.I, err = UnmarshalFloats(iJSON)
		if err != nil {
			return nil, err
		}
		w.Q, err = UnmarshalFloats(qJSON)
		if err != nil {
			return nil, err
		}
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, &w)
	}
	return out, rows.Err()
}

// UpdateWindowStatus 更新窗口状态（new -> corrected -> phase_broken/ignored）。
func (s *Store) UpdateWindowStatus(ctx context.Context, id, newStatus string) error {
	w, err := s.GetWindow(ctx, id)
	if err != nil {
		return err
	}
	if err := model.ValidateWindowTransition(w.Status, newStatus); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE windows SET status=? WHERE id=?`, newStatus, id); err != nil {
		return fmt.Errorf("update window status: %w", err)
	}
	return nil
}

// IgnoreWindow 忽略指定阵元的某序号窗口（幂等：已忽略返回 nil）。
func (s *Store) IgnoreWindow(ctx context.Context, batchID string, elementNo int, seqNo int64) error {
	w, err := s.getWindowByKey(ctx, batchID, elementNo, seqNo)
	if err != nil {
		return err
	}
	if w.Status == model.WindowStatusIgnored {
		return nil
	}
	return s.UpdateWindowStatus(ctx, w.ID, model.WindowStatusIgnored)
}
