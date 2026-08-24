package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"task195-echotrack/internal/model"
)

// AddAnnotation 添加标注。
func (s *Store) AddAnnotation(ctx context.Context, a *model.Annotation) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO annotations (id, batch_id, target_type, target_id, note, author, created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		a.ID, a.BatchID, a.TargetType, a.TargetID, a.Note, a.Author,
		a.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert annotation: %w", err)
	}
	return nil
}

// ListAnnotations 列出批次标注。
func (s *Store) ListAnnotations(ctx context.Context, batchID string) ([]*model.Annotation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, batch_id, target_type, target_id, note, author, created_at
		 FROM annotations WHERE batch_id=? ORDER BY created_at`, batchID)
	if err != nil {
		return nil, fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()
	var out []*model.Annotation
	for rows.Next() {
		var a model.Annotation
		var created string
		if err := rows.Scan(&a.ID, &a.BatchID, &a.TargetType, &a.TargetID, &a.Note,
			&a.Author, &created); err != nil {
			return nil, fmt.Errorf("scan annotation: %w", err)
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, &a)
	}
	return out, rows.Err()
}

// CountAnnotationsByBatch 统计批次标注数。
func (s *Store) CountAnnotationsByBatch(ctx context.Context, batchID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM annotations WHERE batch_id=?`, batchID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count annotations: %w", err)
	}
	return n, nil
}

// CreateInterpretation 创建解释包。
func (s *Store) CreateInterpretation(ctx context.Context, in *model.Interpretation) error {
	var publishedAt *string
	if in.PublishedAt != nil {
		v := in.PublishedAt.Format(time.RFC3339Nano)
		publishedAt = &v
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO interpretations (id, batch_id, version, status, title, snapshot_ref, track_count, created_at, published_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		in.ID, in.BatchID, in.Version, in.Status, in.Title, in.SnapshotRef, in.TrackCount,
		in.CreatedAt.Format(time.RFC3339Nano), publishedAt)
	if err != nil {
		return fmt.Errorf("insert interpretation: %w", err)
	}
	return nil
}

// GetInterpretation 读取解释包。
func (s *Store) GetInterpretation(ctx context.Context, id string) (*model.Interpretation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, batch_id, version, status, title, snapshot_ref, track_count, created_at, published_at
		 FROM interpretations WHERE id=?`, id)
	var in model.Interpretation
	var created string
	var publishedAt *string
	if err := row.Scan(&in.ID, &in.BatchID, &in.Version, &in.Status, &in.Title, &in.SnapshotRef,
		&in.TrackCount, &created, &publishedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan interpretation: %w", err)
	}
	in.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if publishedAt != nil {
		t, _ := time.Parse(time.RFC3339Nano, *publishedAt)
		in.PublishedAt = &t
	}
	return &in, nil
}

// ListInterpretations 列出批次解释包。
func (s *Store) ListInterpretations(ctx context.Context, batchID string) ([]*model.Interpretation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, batch_id, version, status, title, snapshot_ref, track_count, created_at, published_at
		 FROM interpretations WHERE batch_id=? ORDER BY version`, batchID)
	if err != nil {
		return nil, fmt.Errorf("query interpretations: %w", err)
	}
	defer rows.Close()
	var out []*model.Interpretation
	for rows.Next() {
		var in model.Interpretation
		var created string
		var publishedAt *string
		if err := rows.Scan(&in.ID, &in.BatchID, &in.Version, &in.Status, &in.Title, &in.SnapshotRef,
			&in.TrackCount, &created, &publishedAt); err != nil {
			return nil, fmt.Errorf("scan interpretation: %w", err)
		}
		in.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if publishedAt != nil {
			t, _ := time.Parse(time.RFC3339Nano, *publishedAt)
			in.PublishedAt = &t
		}
		out = append(out, &in)
	}
	return out, rows.Err()
}

// UpdateInterpretationState 更新解释包状态（draft -> published -> superseded）。
func (s *Store) UpdateInterpretationState(ctx context.Context, id, newState string) (*model.Interpretation, error) {
	in, err := s.GetInterpretation(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := model.ValidateInterpretationTransition(in.Status, newState); err != nil {
		return nil, err
	}
	var publishedAt *string
	if newState == model.InterpretationStatusPublished {
		v := time.Now().UTC().Format(time.RFC3339Nano)
		publishedAt = &v
		t, _ := time.Parse(time.RFC3339Nano, v)
		in.PublishedAt = &t
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE interpretations SET status=?, published_at=? WHERE id=?`, newState, publishedAt, id); err != nil {
		return nil, fmt.Errorf("update interpretation: %w", err)
	}
	in.Status = newState
	return in, nil
}

// NextInterpretationVersion 计算批次下一个解释包版本号。
func (s *Store) NextInterpretationVersion(ctx context.Context, batchID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version),0)+1 FROM interpretations WHERE batch_id=?`, batchID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("next version: %w", err)
	}
	return n, nil
}

// FindInterpretationByVersion 在请求指定的批次内按版本号查找解释包。
// 不同批次可能存在相同版本号，因此版本号查询必须限定在批次内，
// 不得跨批次替代到另一个批次的解释包。
func (s *Store) FindInterpretationByVersion(ctx context.Context, batchID string, version int) (*model.Interpretation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, batch_id, version, status, title, snapshot_ref, track_count, created_at, published_at
		 FROM interpretations WHERE batch_id=? AND version=? ORDER BY created_at DESC LIMIT 1`, batchID, version)
	var in model.Interpretation
	var created string
	var publishedAt *string
	if err := row.Scan(&in.ID, &in.BatchID, &in.Version, &in.Status, &in.Title, &in.SnapshotRef,
		&in.TrackCount, &created, &publishedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan interpretation: %w", err)
	}
	in.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if publishedAt != nil {
		t, _ := time.Parse(time.RFC3339Nano, *publishedAt)
		in.PublishedAt = &t
	}
	return &in, nil
}
