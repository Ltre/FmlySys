package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Ltre/FmlySys/internal/adminauth"
	"github.com/Ltre/FmlySys/internal/config"
	"github.com/Ltre/FmlySys/internal/partition"
	"github.com/Ltre/FmlySys/internal/store"
)

func TestAssetEventImageEvidenceFrontAndAdmin(t *testing.T) {
	ctx := context.Background()
	pm, err := partition.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pm.Close() })
	st := store.New(pm.ActiveDB)
	result, err := pm.ActiveDB.ExecContext(ctx, `INSERT INTO members(name,relation,status,created_at) VALUES('图片测试成员','','active','2026-10-20T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	memberID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMemberPermissions(ctx, memberID, []string{"assets.view", "assets.self_change"}); err != nil {
		t.Fatal(err)
	}
	memberToken, err := st.CreateMemberSession(ctx, memberID)
	if err != nil {
		t.Fatal(err)
	}
	admin := adminauth.New(pm.SystemDB, bytes.Repeat([]byte{1}, 32), pm.DataDir)
	app, err := New(pm, st, admin, config.Config{DataDir: pm.DataDir}, memberID)
	if err != nil {
		t.Fatal(err)
	}
	handler := WithEnhancedFormResponses(WithAsyncMultipartFormCompatibility(app.WithMoneyRecordDetails(app.WithMoneyWorkflowV3(app.WithAssetWorkflowFixes(app.Handler())))))
	serve := func(method, path, cookieName, token, contentType string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	makeForm := func(fields map[string]string, filename string) ([]byte, string) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		file, err := writer.CreateFormFile("evidence", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(pngBytes); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return body.Bytes(), writer.FormDataContentType()
	}
	memberBody, memberType := makeForm(map[string]string{
		"event_type": "ASSET_IN", "amount": "12.50", "description": "图片凭证测试",
	}, "member-proof.png")
	memberPost := serve(http.MethodPost, "/assets/self-events", "fmly_session", memberToken, memberType, memberBody)
	if memberPost.Code != http.StatusSeeOther {
		t.Fatalf("member asset event status=%d: %s", memberPost.Code, memberPost.Body.String())
	}
	memberKey := memberPost.Header().Get(moneyRecordHeader)
	if !strings.HasPrefix(memberKey, "asset_event:") {
		t.Fatalf("member record key=%q", memberKey)
	}
	memberEventID, err := strconv.ParseInt(strings.TrimPrefix(memberKey, "asset_event:"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	memberEvidence, err := st.EvidenceFor(ctx, "asset_event", memberEventID)
	if err != nil || len(memberEvidence) != 1 {
		t.Fatalf("member asset evidence=%+v, err=%v", memberEvidence, err)
	}
	memberImagePath := "/evidence/" + strconv.FormatInt(memberEvidence[0].ID, 10)
	assetsPage := serve(http.MethodGet, "/assets", "fmly_session", memberToken, "", nil)
	if assetsPage.Code != http.StatusOK || !strings.Contains(assetsPage.Body.String(), `href="`+memberImagePath+`"`) || !strings.Contains(assetsPage.Body.String(), `/static/evidence-image-preview.js`) {
		t.Fatalf("front asset image preview missing, status=%d", assetsPage.Code)
	}
	previewScript := serve(http.MethodGet, "/static/evidence-image-preview.js", "", "", "", nil)
	if previewScript.Code != http.StatusOK || !strings.Contains(previewScript.Body.String(), "showModal()") {
		t.Fatalf("image preview script unavailable, status=%d", previewScript.Code)
	}
	imageResponse := serve(http.MethodGet, memberImagePath, "fmly_session", memberToken, "", nil)
	if imageResponse.Code != http.StatusOK || imageResponse.Header().Get("Content-Disposition") != "inline" || !strings.HasPrefix(imageResponse.Header().Get("Content-Type"), "image/png") || !bytes.Equal(imageResponse.Body.Bytes(), pngBytes) {
		t.Fatalf("image response status=%d, type=%q, disposition=%q", imageResponse.Code, imageResponse.Header().Get("Content-Type"), imageResponse.Header().Get("Content-Disposition"))
	}
	if guestImage := serve(http.MethodGet, memberImagePath, "", "", "", nil); guestImage.Code != http.StatusSeeOther || guestImage.Header().Get("Location") != "/login" {
		t.Fatalf("guest image status=%d, location=%q", guestImage.Code, guestImage.Header().Get("Location"))
	}
	detailPath := "/money-record/asset_event/" + strconv.FormatInt(memberEventID, 10)
	detail := serve(http.MethodGet, detailPath, "fmly_session", memberToken, "", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `href="`+memberImagePath+`"`) {
		t.Fatalf("asset detail image missing, status=%d", detail.Code)
	}
	plainEvent := serve(http.MethodPost, "/assets/self-events", "fmly_session", memberToken, "application/x-www-form-urlencoded", []byte("event_type=ASSET_IN&amount=1.00&description=no-file"))
	if plainEvent.Code != http.StatusSeeOther {
		t.Fatalf("asset event without image status=%d: %s", plainEvent.Code, plainEvent.Body.String())
	}

	adminResult, err := pm.SystemDB.ExecContext(ctx, `INSERT INTO admin_users(username,password_hash,created_at,updated_at) VALUES('test-admin','','2026-10-20T00:00:00Z','2026-10-20T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := adminResult.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err := admin.BeginSession(ctx, adminID, "authenticated")
	if err != nil {
		t.Fatal(err)
	}
	adminBody, adminType := makeForm(map[string]string{
		"event_type": "ASSET_IN", "holder_id": strconv.FormatInt(memberID, 10),
		"amount": "5.00", "description": "后台图片凭证测试",
	}, "admin-proof.png")
	adminPost := serve(http.MethodPost, "/admin/assets/events", "fmly_admin_session", adminToken, adminType, adminBody)
	if adminPost.Code != http.StatusSeeOther {
		t.Fatalf("admin asset event status=%d: %s", adminPost.Code, adminPost.Body.String())
	}
	adminEventID, err := strconv.ParseInt(strings.TrimPrefix(adminPost.Header().Get(moneyRecordHeader), "asset_event:"), 10, 64)
	if err != nil || adminEventID <= 0 {
		t.Fatalf("admin record key=%q, err=%v", adminPost.Header().Get(moneyRecordHeader), err)
	}
	adminEvidence, err := st.EvidenceFor(ctx, "asset_event", adminEventID)
	if err != nil || len(adminEvidence) != 1 {
		t.Fatalf("admin asset evidence=%+v, err=%v", adminEvidence, err)
	}
	adminPage := serve(http.MethodGet, "/admin", "fmly_admin_session", adminToken, "", nil)
	adminImagePath := "/evidence/" + strconv.FormatInt(adminEvidence[0].ID, 10)
	if adminPage.Code != http.StatusOK || !strings.Contains(adminPage.Body.String(), `href="`+adminImagePath+`"`) || !strings.Contains(adminPage.Body.String(), `/static/evidence-image-preview.js`) {
		t.Fatalf("admin asset image preview missing, status=%d", adminPage.Code)
	}

	otherResult, err := pm.ActiveDB.ExecContext(ctx, `INSERT INTO members(name,relation,status,created_at) VALUES('付款成员','','active','2026-10-20T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := otherResult.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddAssetEventDetailed(ctx, memberID, otherID, "ASSET_IN", 10000, "报销资金", "2026-10-20T14:00:00Z", 0); err != nil {
		t.Fatal(err)
	}
	expenseID, err := st.CreateExpenseAuto(ctx, memberID, store.ExpenseInputV2{
		Title: "图片消费", AmountCent: 5000, HandlerMemberID: memberID,
		OccurredAt: "2026-10-20T15:00:00Z", PaymentChannel: "微信",
	})
	if err != nil {
		t.Fatal(err)
	}
	transferID, err := st.CreateTransferV2(ctx, memberID, otherID, memberID, 100, "图片转账", "微信", "2026-10-20T16:00:00Z", 0)
	if err != nil {
		t.Fatal(err)
	}
	reimbursementID, err := st.CreateReimbursementV2(ctx, memberID, expenseID, otherID, 500, "微信", "2026-10-20T17:00:00Z", "图片报销")
	if err != nil {
		t.Fatal(err)
	}
	type recordImage struct {
		kind, section string
		id            int64
		attachmentID  int64
	}
	records := []recordImage{
		{kind: "expense", section: "expense-records", id: expenseID},
		{kind: "transfer", section: "transfer-records", id: transferID},
		{kind: "reimbursement", section: "reimbursement-records", id: reimbursementID},
	}
	for i := range records {
		fixtureBody, fixtureType := makeForm(nil, records[i].kind+"-proof.png")
		req := httptest.NewRequest(http.MethodPost, "/test-fixture", bytes.NewReader(fixtureBody))
		req.Header.Set("Content-Type", fixtureType)
		if err := req.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if err := st.SaveEvidenceFiles(ctx, memberID, records[i].kind, records[i].id, evidenceDir(app), req.MultipartForm.File["evidence"]); err != nil {
			t.Fatal(err)
		}
		_ = req.MultipartForm.RemoveAll()
		attachments, err := st.EvidenceFor(ctx, records[i].kind, records[i].id)
		if err != nil || len(attachments) != 1 {
			t.Fatalf("%s attachments=%+v, err=%v", records[i].kind, attachments, err)
		}
		records[i].attachmentID = attachments[0].ID
	}
	assetsPage = serve(http.MethodGet, "/assets", "fmly_session", memberToken, "", nil)
	adminPage = serve(http.MethodGet, "/admin", "fmly_admin_session", adminToken, "", nil)
	for _, record := range records {
		imagePath := "/evidence/" + strconv.FormatInt(record.attachmentID, 10)
		for label, page := range map[string]string{"front": assetsPage.Body.String(), "admin": adminPage.Body.String()} {
			parts := strings.SplitN(page, `id="`+record.section+`"`, 2)
			if len(parts) != 2 || !strings.Contains(strings.SplitN(parts[1], "</section>", 2)[0], `href="`+imagePath+`"`) {
				t.Fatalf("%s %s image link missing", label, record.kind)
			}
		}
		detail := serve(http.MethodGet, "/money-record/"+record.kind+"/"+strconv.FormatInt(record.id, 10), "fmly_session", memberToken, "", nil)
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `href="`+imagePath+`"`) {
			t.Fatalf("%s detail image missing, status=%d", record.kind, detail.Code)
		}
	}
}
