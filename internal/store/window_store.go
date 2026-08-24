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
//
// 本函数遵守传入的 ctx：若客户端在上传期间取消请求，正在进行的查询与写入会随
// ctx 一同中断，不会把已取消的窗口落库。本函数仅写窗口本身；窗口写入与批次游标
// 推进的原子化由 UpsertWindowAndAdvanceCursor 完成。
func (s *Store) UpsertWindow(ctx context.Context, w *model.Window) (ingest.IngestReceipt, bool, error) {
	if err := ctx.Err(); err != nil {
		return ingest.IngestReceipt{}, false, model.ErrRequestCancelled
	}
	// 先查既有记录。
	existing, err := s.getWindowByKey(ctx, w.BatchID, w.ElementNo, w.SeqNo)
	if err == nil {
		if existing.Checksum != w.Checksum {
			return ingest.IngestReceipt{}, false, model.ErrDuplicateWindow
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

// UpsertWindowAndAdvanceCursor 在单个事务内原子地完成“窗口幂等写入 + 批次游标推进”。
//
// 取消语义：事务以传入的 ctx 绑定；若客户端在提交前取消请求，事务回滚，
// 既不写入窗口也不推进游标，调用方据此返回可识别的取消结果，后续查询看不到该窗口。
//
// 游标推进规则：仅在本次真正插入新窗口（非幂等命中）且新序号超过当前游标时推进；
// 游标推进不改变批次状态（uploading/processing 均可接收），因此绕过状态机校验。
// 已发布/封存批次不可再推进，返回 ErrFrozenBatch。
func (s *Store) UpsertWindowAndAdvanceCursor(ctx context.Context, w *model.Window, cursorFloor int64) (ingest.IngestReceipt, bool, *model.Batch, error) {
	if err := ctx.Err(); err != nil {
		return ingest.IngestReceipt{}, false, nil, model.ErrRequestCancelled
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	// 1. 读取批次，校验可接收与冻结态。
	var bStatus string
	var bCursor int64
	var bID, bArrayID string
	var bRef int
	var bRate float64
	var bCreated, bUpdated string
	if err := tx.QueryRowContext(ctx,
		`SELECT id, array_id, status, reference_element, window_cursor, sample_rate_hz, created_at, updated_at
		 FROM batches WHERE id=?`, w.BatchID).
		Scan(&bID, &bArrayID, &bStatus, &bRef, &bCursor, &bRate, &bCreated, &bUpdated); err != nil {
		if err == sql.ErrNoRows {
			return ingest.IngestReceipt{}, false, nil, model.ErrNotFound
		}
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("query batch: %w", err)
	}
	if bStatus == model.BatchStatusPublished || bStatus == model.BatchStatusArchived {
		return ingest.IngestReceipt{}, false, nil, model.ErrFrozenBatch
	}

	// 2. 幂等查既有窗口。
	var existingID, existingChecksum, existingStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT id, status, checksum FROM windows WHERE batch_id=? AND element_no=? AND seq_no=?`,
		w.BatchID, w.ElementNo, w.SeqNo).Scan(&existingID, &existingStatus, &existingChecksum)
	switch {
	case err == nil:
		// 幂等命中：内容一致返回既有记录，不推进游标。
		if existingChecksum != w.Checksum {
			return ingest.IngestReceipt{}, false, nil, model.ErrDuplicateWindow
		}
		if err := tx.Commit(); err != nil {
			return ingest.IngestReceipt{}, false, nil, fmt.Errorf("commit (dedup): %w", err)
		}
		batch := &model.Batch{ID: bID, ArrayID: bArrayID, Status: bStatus, ReferenceElement: bRef,
			WindowCursor: bCursor, SampleRateHz: bRate}
		batch.CreatedAt, _ = time.Parse(time.RFC3339Nano, bCreated)
		batch.UpdatedAt, _ = time.Parse(time.RFC3339Nano, bUpdated)
		return ingest.IngestReceipt{WindowID: existingID, Inserted: false, Duplicated: true, Checksum: existingChecksum},
			false, batch, nil
	case err != sql.ErrNoRows:
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("query existing window: %w", err)
	}

	// 3. 插入新窗口。
	iJSON, err := MarshalFloats(w.I)
	if err != nil {
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("marshal i: %w", err)
	}
	qJSON, err := MarshalFloats(w.Q)
	if err != nil {
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("marshal q: %w", err)
	}
	result, err := tx.ExecContext(ctx,
		`INSERT INTO windows (id, batch_id, element_no, seq_no, i_json, q_json, sample_rate, status, checksum, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(batch_id, element_no, seq_no) DO NOTHING`,
		w.ID, w.BatchID, w.ElementNo, w.SeqNo, iJSON, qJSON, w.SampleRate, w.Status, w.Checksum,
		w.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("insert window: %w", err)
	}
	insertedRows, err := result.RowsAffected()
	if err != nil {
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("inspect window insert: %w", err)
	}
	if insertedRows == 0 {
		// 并发竞态：另一事务先插入了同一键，按幂等命中处理。
		var id, checksum string
		if err := tx.QueryRowContext(ctx,
			`SELECT id, checksum FROM windows WHERE batch_id=? AND element_no=? AND seq_no=?`,
			w.BatchID, w.ElementNo, w.SeqNo).Scan(&id, &checksum); err != nil {
			return ingest.IngestReceipt{}, false, nil, fmt.Errorf("race lookup: %w", err)
		}
		if checksum != w.Checksum {
			return ingest.IngestReceipt{}, false, nil, model.ErrDuplicateWindow
		}
		if err := tx.Commit(); err != nil {
			return ingest.IngestReceipt{}, false, nil, fmt.Errorf("commit (race): %w", err)
		}
		batch := &model.Batch{ID: bID, ArrayID: bArrayID, Status: bStatus, ReferenceElement: bRef,
			WindowCursor: bCursor, SampleRateHz: bRate}
		batch.CreatedAt, _ = time.Parse(time.RFC3339Nano, bCreated)
		batch.UpdatedAt, _ = time.Parse(time.RFC3339Nano, bUpdated)
		return ingest.IngestReceipt{WindowID: id, Inserted: false, Duplicated: true, Checksum: checksum},
			false, batch, nil
	}

	// 4. 仅当新序号超过当前游标时推进；状态不变，故不触发状态机校验。
	newCursor := bCursor
	if w.SeqNo+1 > newCursor {
		newCursor = w.SeqNo + 1
	}
	if newCursor != bCursor {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx,
			`UPDATE batches SET window_cursor=?, updated_at=? WHERE id=?`, newCursor, now, w.BatchID); err != nil {
			return ingest.IngestReceipt{}, false, nil, fmt.Errorf("advance cursor: %w", err)
		}
		bCursor = newCursor
		bUpdated = now
	}

	// 5. 提交事务——此为唯一对外可见的生效点；提交前取消则整体回滚。
	if err := tx.Commit(); err != nil {
		return ingest.IngestReceipt{}, false, nil, fmt.Errorf("commit window: %w", err)
	}
	batch := &model.Batch{ID: bID, ArrayID: bArrayID, Status: bStatus, ReferenceElement: bRef,
		WindowCursor: bCursor, SampleRateHz: bRate}
	batch.CreatedAt, _ = time.Parse(time.RFC3339Nano, bCreated)
	batch.UpdatedAt, _ = time.Parse(time.RFC3339Nano, bUpdated)
	return ingest.IngestReceipt{WindowID: w.ID, Inserted: true, Duplicated: false, Checksum: w.Checksum},
		true, batch, nil
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
