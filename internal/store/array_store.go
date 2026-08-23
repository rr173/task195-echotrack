package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"task195-echotrack/internal/model"
)

// CreateArray 写入阵列及其阵元配置（事务内）。
func (s *Store) CreateArray(ctx context.Context, a *model.Array) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO arrays (id, name, sample_rate_hz, created_at) VALUES (?,?,?,?)`,
		a.ID, a.Name, a.SampleRateHz, a.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert array: %w", err)
	}
	for _, e := range a.Elements {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO elements (array_id, element_no, name, x, y, delay_us) VALUES (?,?,?,?,?,?)`,
			a.ID, e.ElementNo, e.Name, e.X, e.Y, e.DelayUs); err != nil {
			return fmt.Errorf("insert element: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetArray 读取阵列（含阵元）。
func (s *Store) GetArray(ctx context.Context, id string) (*model.Array, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, sample_rate_hz, created_at FROM arrays WHERE id=?`, id)
	var a model.Array
	var created string
	if err := row.Scan(&a.ID, &a.Name, &a.SampleRateHz, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("scan array: %w", err)
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	rows, err := s.db.QueryContext(ctx,
		`SELECT array_id, element_no, name, x, y, delay_us FROM elements WHERE array_id=? ORDER BY element_no`, id)
	if err != nil {
		return nil, fmt.Errorf("query elements: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e model.Element
		if err := rows.Scan(&e.ArrayID, &e.ElementNo, &e.Name, &e.X, &e.Y, &e.DelayUs); err != nil {
			return nil, fmt.Errorf("scan element: %w", err)
		}
		a.Elements = append(a.Elements, e)
	}
	return &a, rows.Err()
}

// ListArrays 列出全部阵列摘要。
func (s *Store) ListArrays(ctx context.Context) ([]*model.Array, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, sample_rate_hz, created_at FROM arrays ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("query arrays: %w", err)
	}
	defer rows.Close()
	var out []*model.Array
	for rows.Next() {
		var a model.Array
		var created string
		if err := rows.Scan(&a.ID, &a.Name, &a.SampleRateHz, &created); err != nil {
			return nil, fmt.Errorf("scan array: %w", err)
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, &a)
	}
	return out, rows.Err()
}

// UpdateElementDelay 更新阵元延迟校正量。
func (s *Store) UpdateElementDelay(ctx context.Context, arrayID string, elementNo int, delayUs float64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE elements SET delay_us=? WHERE array_id=? AND element_no=?`, 0, arrayID, elementNo)
	if err != nil {
		return fmt.Errorf("update element: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// CountBatchesByArray 统计阵列的批次数量。
func (s *Store) CountBatchesByArray(ctx context.Context, arrayID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM batches WHERE array_id=?`, arrayID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count batches: %w", err)
	}
	return n, nil
}

// MarshalFloats / UnmarshalFloats 浮点数组的 JSON 编解码。
func MarshalFloats(vals []float64) (string, error) {
	b, err := json.Marshal(vals)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func UnmarshalFloats(raw string) ([]float64, error) {
	var out []float64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// MarshalInts / UnmarshalInts 整数数组的 JSON 编解码。
func MarshalInts(vals []int) (string, error) {
	b, err := json.Marshal(vals)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func UnmarshalInts(raw string) ([]int, error) {
	var out []int
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
