package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func init() {
	for _, ext := range []string{"mp4", "webm", "mov", "m4v", "mp3", "m4a", "aac", "wav", "ogg", "flac"} {
		evidenceExts[ext] = true
	}
}

type preparedWorkflowEvidence struct {
	storageName string
	original    string
	mimeType    string
	size        int64
	sha256      string
	finalPath   string
}

func cleanupWorkflowEvidence(files []preparedWorkflowEvidence) {
	for _, file := range files {
		_ = os.Remove(file.finalPath)
	}
}

func prepareWorkflowEvidence(uploadDir string, headers []*multipart.FileHeader) ([]preparedWorkflowEvidence, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	if len(headers) > EvidenceMaxFiles {
		return nil, fmt.Errorf("一次最多上传 %d 个支付/转账凭证", EvidenceMaxFiles)
	}
	for _, header := range headers {
		if err := validateEvidenceHeader(header); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return nil, err
	}

	prepared := make([]preparedWorkflowEvidence, 0, len(headers))
	for _, header := range headers {
		src, err := header.Open()
		if err != nil {
			cleanupWorkflowEvidence(prepared)
			return nil, err
		}
		tmp, err := os.CreateTemp(uploadDir, "evidence-*")
		if err != nil {
			_ = src.Close()
			cleanupWorkflowEvidence(prepared)
			return nil, err
		}
		tmpName := tmp.Name()
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(src, EvidenceMaxFileSize+1))
		_ = src.Close()
		closeErr := tmp.Close()
		if copyErr != nil || closeErr != nil || n > EvidenceMaxFileSize {
			_ = os.Remove(tmpName)
			cleanupWorkflowEvidence(prepared)
			if copyErr != nil {
				return nil, copyErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			return nil, fmt.Errorf("文件 %s 超过 10MB", filepath.Base(header.Filename))
		}

		sum := hex.EncodeToString(hash.Sum(nil))
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(header.Filename), "."))
		storage := sum[:20] + "-" + fmt.Sprintf("%d", time.Now().UnixNano()) + "." + ext
		finalPath := filepath.Join(uploadDir, storage)
		if err := os.Rename(tmpName, finalPath); err != nil {
			_ = os.Remove(tmpName)
			cleanupWorkflowEvidence(prepared)
			return nil, err
		}
		prepared = append(prepared, preparedWorkflowEvidence{
			storageName: storage,
			original:    filepath.Base(header.Filename),
			mimeType:    header.Header.Get("Content-Type"),
			size:        n,
			sha256:      sum,
			finalPath:   finalPath,
		})
	}
	return prepared, nil
}

func insertWorkflowEvidenceTx(ctx context.Context, tx *sql.Tx, actor int64, entityType string, entityID int64, files []preparedWorkflowEvidence) error {
	for _, file := range files {
		res, err := tx.ExecContext(ctx, `INSERT INTO record_attachments(entity_type,entity_id,storage_name,original_name,mime_type,size,sha256,uploaded_by,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, entityType, entityID, file.storageName, file.original, file.mimeType, file.size, file.sha256, actor, now())
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if err := auditTx(ctx, tx, actor, "create", "record_attachment", id, nil, map[string]any{"entity_type": entityType, "entity_id": entityID, "original_name": file.original, "sha256": file.sha256}); err != nil {
			return err
		}
	}
	return nil
}

// CreateExpenseAutoWithEvidence keeps the expense row, attachment metadata and
// audit records in one database transaction. Evidence bytes are staged before
// the write transaction starts so slow file I/O never holds the SQLite writer.
func (s *Store) CreateExpenseAutoWithEvidence(ctx context.Context, actor int64, e ExpenseInputV2, uploadDir string, headers []*multipart.FileHeader) (int64, error) {
	if strings.TrimSpace(e.Title) == "" || e.AmountCent <= 0 || e.HandlerMemberID == 0 {
		return 0, errors.New("消费事项、金额和经手人不能为空")
	}
	if err := validatePaymentChannel(e.PaymentChannel); err != nil {
		return 0, err
	}
	prepared, err := prepareWorkflowEvidence(uploadDir, headers)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupWorkflowEvidence(prepared)
		}
	}()

	// Calculate the available holder balance after file staging, immediately
	// before the database write, so upload/hash time does not unnecessarily
	// widen the balance snapshot window.
	bal, err := s.HolderBalanceV2(ctx, e.HandlerMemberID)
	if err != nil {
		return 0, err
	}
	publicPaid := e.AmountCent
	if bal < publicPaid {
		publicPaid = bal
	}
	if publicPaid < 0 {
		publicPaid = 0
	}
	reimbursable := e.AmountCent - publicPaid
	funding := "PUBLIC_HELD_ASSET"
	if reimbursable > 0 {
		funding = "PERSONAL_ADVANCE"
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO public_expenses(title,category,amount_cent,occurred_at,handler_member_id,payer_member_id,funding_type,holder_member_id,payment_channel,merchant,description,matter_id,reimbursable_amount_cent,status,version,created_by,created_at,updated_at,public_paid_amount_cent) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'active',1,?,?,?,?)`, e.Title, e.Category, e.AmountCent, e.OccurredAt, e.HandlerMemberID, e.HandlerMemberID, funding, e.HandlerMemberID, e.PaymentChannel, e.Merchant, e.Description, nullID(e.MatterID), reimbursable, actor, now(), now(), publicPaid)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "expense", id, nil, map[string]any{"title": e.Title, "amount_cent": e.AmountCent, "handler_member_id": e.HandlerMemberID, "payment_channel": e.PaymentChannel, "public_paid_amount_cent": publicPaid, "reimbursable_amount_cent": reimbursable}); err != nil {
		return 0, err
	}
	if err := insertWorkflowEvidenceTx(ctx, tx, actor, "expense", id, prepared); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	committed = true
	return id, nil
}

// AssetMovementsDetailed returns the complete holder-balance movement stream.
// Manual asset events are normalized to signed deltas and human labels, while
// automatic and later expense reimbursements are synthesized from source facts.
func (s *Store) AssetMovementsDetailed(ctx context.Context) ([]AssetEvent, error) {
	base, err := s.AssetEventsDetailed(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AssetEvent, 0, len(base)+32)
	for _, event := range base {
		label := event.TypeLabel()
		delta := event.BalanceDeltaCent()
		event.Type = label
		event.AmountCent = delta
		out = append(out, event)
	}

	expenseRows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.public_paid_amount_cent,m.name,e.title,e.occurred_at FROM public_expenses e JOIN members m ON m.id=COALESCE(e.holder_member_id,e.handler_member_id) WHERE e.status='active' AND e.public_paid_amount_cent>0`)
	if err != nil {
		return nil, err
	}
	for expenseRows.Next() {
		var id, amount int64
		var holder, title, occurred string
		if err := expenseRows.Scan(&id, &amount, &holder, &title, &occurred); err != nil {
			_ = expenseRows.Close()
			return nil, err
		}
		out = append(out, AssetEvent{ID: -id, Type: "消费报销", AmountCent: -amount, HolderName: holder, Description: fmt.Sprintf("消费 #%d %s · 消费发生时自动报销", id, title), OccurredAt: occurred})
	}
	if err := expenseRows.Close(); err != nil {
		return nil, err
	}

	reimbursementRows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.amount_cent,h.name,e.title,recv.name,r.occurred_at FROM reimbursements r JOIN public_expenses e ON e.id=r.expense_id JOIN members h ON h.id=r.payer_holder_meb_id JOIN members recv ON recv.id=r.receiver_member_id WHERE r.status='active'`)
	if err != nil {
		return nil, err
	}
	for reimbursementRows.Next() {
		var id, amount int64
		var holder, title, receiver, occurred string
		if err := reimbursementRows.Scan(&id, &amount, &holder, &title, &receiver, &occurred); err != nil {
			_ = reimbursementRows.Close()
			return nil, err
		}
		out = append(out, AssetEvent{ID: -(1_000_000_000 + id), Type: "消费报销", AmountCent: -amount, HolderName: holder, Description: fmt.Sprintf("消费 %s · 后续报销给 %s", title, receiver), OccurredAt: occurred})
	}
	if err := reimbursementRows.Close(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OccurredAt == out[j].OccurredAt {
			return out[i].ID > out[j].ID
		}
		return out[i].OccurredAt > out[j].OccurredAt
	})
	if len(out) > 400 {
		out = out[:400]
	}
	return out, nil
}
