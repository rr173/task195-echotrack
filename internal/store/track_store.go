package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"task195-echotrack/internal/model"
)

// SaveTrack 保存轨迹。
func (s *Store) SaveTrack(ctx context.Context, t *model.Track) error {
	phaseJSON, err := MarshalFloats(t.PhasePoints)
	if err != nil {
		return fmt.Errorf("marshal phase: %w", err)
	}
	timeJSON, err := MarshalFloats(t.TimePoints)
	if err != nil {
		return fmt.Errorf("marshal time: %w", err)
	}
	brokenJSON, err := MarshalInts(t.BrokenAt)
	if err != nil {
		return fmt.Errorf("marshal broken: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tracks (id, batch_id, element_no, status, phase_json, time_json, broken_json, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		t.ID, t.BatchID, t.ElementNo, t.Status, phaseJSON, timeJSON, brokenJSON,
		t.CreatedAt.Format(time.RFC3339Nano), t.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert track: %w", err)
	}
	return nil
}

// GetTrack 读取轨迹。
func (s *Store) GetTrack(ctx context.Context, id string) (*model.Track, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, batch_id, element_no, status, phase_json, time_json, broken_json, created_at, updated_at
		 FROM tracks WHERE id=?`, id)
	var t model.Track
	var phaseJSON, timeJSON, brokenJSON, created, updated string
	if err := row.Scan(&t.ID, &t.BatchID, &t.ElementNo, &t.Status, &phaseJSON, &timeJSON,
		&brokenJSON, &created, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan track: %w", err)
	}
	var err error
	t.PhasePoints, err = UnmarshalFloats(phaseJSON)
	if err != nil {
		return nil, err
	}
	t.TimePoints, err = UnmarshalFloats(timeJSON)
	if err != nil {
		return nil, err
	}
	t.BrokenAt, err = UnmarshalInts(brokenJSON)
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &t, nil
}

// ListTracks 列出批次轨迹。
func (s *Store) ListTracks(ctx context.Context, batchID string) ([]*model.Track, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, batch_id, element_no, status, phase_json, time_json, broken_json, created_at, updated_at
		 FROM tracks WHERE batch_id=? ORDER BY element_no`, batchID)
	if err != nil {
		return nil, fmt.Errorf("query tracks: %w", err)
	}
	defer rows.Close()
	var out []*model.Track
	for rows.Next() {
		var t model.Track
		var phaseJSON, timeJSON, brokenJSON, created, updated string
		if err := rows.Scan(&t.ID, &t.BatchID, &t.ElementNo, &t.Status, &phaseJSON, &timeJSON,
			&brokenJSON, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan track: %w", err)
		}
		var err error
		t.PhasePoints, err = UnmarshalFloats(phaseJSON)
		if err != nil {
			return nil, err
		}
		t.TimePoints, err = UnmarshalFloats(timeJSON)
		if err != nil {
			return nil, err
		}
		t.BrokenAt, err = UnmarshalInts(brokenJSON)
		if err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, &t)
	}
	return out, rows.Err()
}

// UpdateTrackStatus 更新轨迹状态。
func (s *Store) UpdateTrackStatus(ctx context.Context, id, newStatus string) error {
	t, err := s.GetTrack(ctx, id)
	if err != nil {
		return err
	}
	if err := model.ValidateTrackTransition(t.Status, newStatus); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tracks SET status=?, updated_at=? WHERE id=?`, newStatus, now, id); err != nil {
		return fmt.Errorf("update track: %w", err)
	}
	return nil
}

// AddSegment 添加轨迹段。
func (s *Store) AddSegment(ctx context.Context, seg *model.Segment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO segments (id, track_id, start_idx, end_idx, label, author, created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		seg.ID, seg.TrackID, seg.StartIdx, seg.EndIdx, seg.Label, seg.Author,
		seg.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert segment: %w", err)
	}
	return nil
}

// ListSegments 列出轨迹段。
func (s *Store) ListSegments(ctx context.Context, trackID string) ([]*model.Segment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, track_id, start_idx, end_idx, label, author, created_at
		 FROM segments WHERE track_id=? ORDER BY start_idx`, trackID)
	if err != nil {
		return nil, fmt.Errorf("query segments: %w", err)
	}
	defer rows.Close()
	var out []*model.Segment
	for rows.Next() {
		var seg model.Segment
		var created string
		if err := rows.Scan(&seg.ID, &seg.TrackID, &seg.StartIdx, &seg.EndIdx, &seg.Label,
			&seg.Author, &created); err != nil {
			return nil, fmt.Errorf("scan segment: %w", err)
		}
		seg.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, &seg)
	}
	return out, rows.Err()
}

// CountTracksByBatch 统计批次轨迹数。
func (s *Store) CountTracksByBatch(ctx context.Context, batchID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tracks WHERE batch_id=?`, batchID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count tracks: %w", err)
	}
	return n, nil
}
