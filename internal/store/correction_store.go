package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"task195-echotrack/internal/model"
)

// SaveCorrection 保存校正记录。
func (s *Store) SaveCorrection(ctx context.Context, c *model.Correction) error {
	unwrappedJSON, err := MarshalFloats(c.Unwrapped)
	if err != nil {
		return fmt.Errorf("marshal unwrapped: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO corrections (id, batch_id, window_id, element_no, delay_us, rotation_rad, unwrapped_json, applied_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		c.ID, c.BatchID, c.WindowID, c.ElementNo, c.DelayUs, c.RotationRad, unwrappedJSON,
		c.AppliedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert correction: %w", err)
	}
	return nil
}

// GetCorrectionByWindow 查询窗口的校正记录。
func (s *Store) GetCorrectionByWindow(ctx context.Context, windowID string) (*model.Correction, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, batch_id, window_id, element_no, delay_us, rotation_rad, unwrapped_json, applied_at
		 FROM corrections WHERE window_id=?`, windowID)
	var c model.Correction
	var unwrappedJSON, applied string
	if err := row.Scan(&c.ID, &c.BatchID, &c.WindowID, &c.ElementNo, &c.DelayUs,
		&c.RotationRad, &unwrappedJSON, &applied); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan correction: %w", err)
	}
	unwrapped, err := UnmarshalFloats(unwrappedJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal unwrapped: %w", err)
	}
	c.Unwrapped = unwrapped
	c.AppliedAt, _ = time.Parse(time.RFC3339Nano, applied)
	return &c, nil
}

// ListCorrections 列出批次全部校正记录（按阵元、序号排序，经窗口连接）。
func (s *Store) ListCorrections(ctx context.Context, batchID string) ([]*model.Correction, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.batch_id, c.window_id, c.element_no, c.delay_us, c.rotation_rad, c.unwrapped_json, c.applied_at
		 FROM corrections c JOIN windows w ON c.window_id = w.id
		 WHERE c.batch_id=? ORDER BY c.element_no, w.seq_no`, batchID)
	if err != nil {
		return nil, fmt.Errorf("query corrections: %w", err)
	}
	defer rows.Close()
	var out []*model.Correction
	for rows.Next() {
		var c model.Correction
		var unwrappedJSON, applied string
		if err := rows.Scan(&c.ID, &c.BatchID, &c.WindowID, &c.ElementNo, &c.DelayUs,
			&c.RotationRad, &unwrappedJSON, &applied); err != nil {
			return nil, fmt.Errorf("scan correction: %w", err)
		}
		unwrapped, err := UnmarshalFloats(unwrappedJSON)
		if err != nil {
			return nil, err
		}
		c.Unwrapped = unwrapped
		c.AppliedAt, _ = time.Parse(time.RFC3339Nano, applied)
		out = append(out, &c)
	}
	return out, rows.Err()
}

// DeleteCorrectionsByBatch 删除批次全部校正记录（重算前清理）。
func (s *Store) DeleteCorrectionsByBatch(ctx context.Context, batchID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM corrections WHERE batch_id=?`, batchID); err != nil {
		return fmt.Errorf("delete corrections: %w", err)
	}
	return nil
}

// DeleteTracksByBatch 删除批次全部轨迹（重算前清理）。
func (s *Store) DeleteTracksByBatch(ctx context.Context, batchID string) error {
	// 先删轨迹段。
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM segments WHERE track_id IN (SELECT id FROM tracks WHERE batch_id=?)`, batchID); err != nil {
		return fmt.Errorf("delete segments: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM tracks WHERE batch_id=?`, batchID); err != nil {
		return fmt.Errorf("delete tracks: %w", err)
	}
	return nil
}
