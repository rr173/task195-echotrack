package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"task195-echotrack/internal/model"
)

// CreateBatch 创建批次。
func (s *Store) CreateBatch(ctx context.Context, b *model.Batch) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO batches (id, array_id, status, reference_element, window_cursor, sample_rate_hz, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		b.ID, b.ArrayID, b.Status, b.ReferenceElement, b.WindowCursor, b.SampleRateHz,
		b.CreatedAt.Format(time.RFC3339Nano), b.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert batch: %w", err)
	}
	return nil
}

// GetBatch 读取批次。
func (s *Store) GetBatch(ctx context.Context, id string) (*model.Batch, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, array_id, status, reference_element, window_cursor, sample_rate_hz, created_at, updated_at
		 FROM batches WHERE id=?`, id)
	var b model.Batch
	var created, updated string
	if err := row.Scan(&b.ID, &b.ArrayID, &b.Status, &b.ReferenceElement, &b.WindowCursor,
		&b.SampleRateHz, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan batch: %w", err)
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &b, nil
}

// ListBatches 按阵列列出批次。
func (s *Store) ListBatches(ctx context.Context, arrayID string) ([]*model.Batch, error) {
	query := `SELECT id, array_id, status, reference_element, window_cursor, sample_rate_hz, created_at, updated_at
	          FROM batches`
	var args []any
	if arrayID != "" {
		query += ` WHERE array_id=?`
		args = append(args, arrayID)
	}
	query += ` ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query batches: %w", err)
	}
	defer rows.Close()
	var out []*model.Batch
	for rows.Next() {
		var b model.Batch
		var created, updated string
		if err := rows.Scan(&b.ID, &b.ArrayID, &b.Status, &b.ReferenceElement, &b.WindowCursor,
			&b.SampleRateHz, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan batch: %w", err)
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, &b)
	}
	return out, rows.Err()
}

// UpdateBatchState 更新批次状态并推进游标，返回更新后的批次。
// 若批次已发布/封存且状态不允许变更则返回 ErrFrozenBatch。
func (s *Store) UpdateBatchState(ctx context.Context, id, newState string, cursor int64) (*model.Batch, error) {
	b, err := s.GetBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := model.ValidateTransition(b.Status, newState); err != nil {
		return nil, err
	}
	if b.Status == model.BatchStatusPublished || b.Status == model.BatchStatusArchived {
		return nil, model.ErrFrozenBatch
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE batches SET status=?, window_cursor=?, updated_at=? WHERE id=?`,
		newState, cursor, now, id); err != nil {
		return nil, fmt.Errorf("update batch: %w", err)
	}
	b.Status = newState
	b.WindowCursor = cursor
	b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return b, nil
}

// SetBatchReferenceElement 切换参考阵元（仅 uploading/processing/reviewing 允许）。
func (s *Store) SetBatchReferenceElement(ctx context.Context, id string, elementNo int) (*model.Batch, error) {
	b, err := s.GetBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.Status == model.BatchStatusPublished || b.Status == model.BatchStatusArchived {
		return nil, model.ErrFrozenBatch
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE batches SET reference_element=?, updated_at=? WHERE id=?`,
		elementNo, now, id); err != nil {
		return nil, fmt.Errorf("update reference: %w", err)
	}
	b.ReferenceElement = elementNo
	b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return b, nil
}

// ListWindowsJSON 批量读取窗口（返回紧凑 JSON 行，用于拟合）。
func (s *Store) ListWindowsJSON(ctx context.Context, batchID string) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, element_no, seq_no, i_json, q_json, status, checksum
		 FROM windows WHERE batch_id=? ORDER BY element_no, seq_no`, batchID)
	if err != nil {
		return nil, fmt.Errorf("query windows: %w", err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var id string
		var elementNo int
		var seqNo int64
		var iJSON, qJSON, status, checksum string
		if err := rows.Scan(&id, &elementNo, &seqNo, &iJSON, &qJSON, &status, &checksum); err != nil {
			return nil, fmt.Errorf("scan window: %w", err)
		}
		rec, _ := json.Marshal(map[string]any{
			"id": id, "elementNo": elementNo, "seqNo": seqNo,
			"i": json.RawMessage(iJSON), "q": json.RawMessage(qJSON),
			"status": status, "checksum": checksum,
		})
		out = append(out, rec)
	}
	return out, rows.Err()
}

// NextCursor 返回批次的排他游标上界（下一个待处理 seqNo）。
func (s *Store) NextCursor(ctx context.Context, batchID string) (int64, error) {
	b, err := s.GetBatch(ctx, batchID)
	if err != nil {
		return 0, err
	}
	return b.WindowCursor, nil
}
