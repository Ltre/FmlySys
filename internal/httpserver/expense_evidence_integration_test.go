package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestExpenseEvidenceEditHTTPFlow(t *testing.T) {
	ctx := context.Background()
	pm, err := partition.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pm.Close() })
	st := store.New(pm.ActiveDB)
	createMember := func(name string, permissions []string) (int64, string) {
		t.Helper()
		result, err := pm.ActiveDB.ExecContext(ctx, `INSERT INTO members(name,relation,status,created_at) VALUES(?,'','active',?)`, name, "2026-10-20T00:00:00Z")
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetMemberPermissions(ctx, id, permissions); err != nil {
			t.Fatal(err)
		}
		token, err := st.CreateMemberSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return id, token
	}
	memberID, editorToken := createMember("编辑成员", []string{"assets.view", "expenses.edit"})
	_, viewerToken := createMember("只读成员", []string{"assets.view"})
	expenseID, err := st.CreateExpenseAuto(ctx, memberID, store.ExpenseInputV2{
		Title: "测试消费", AmountCent: 1250, HandlerMemberID: memberID,
		OccurredAt: "2026-10-20T12:00:00Z", PaymentChannel: "微信",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherExpenseID, err := st.CreateExpenseAuto(ctx, memberID, store.ExpenseInputV2{
		Title: "其它消费", AmountCent: 500, HandlerMemberID: memberID,
		OccurredAt: "2026-10-20T13:00:00Z", PaymentChannel: "微信",
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := adminauth.New(pm.SystemDB, bytes.Repeat([]byte{1}, 32), pm.DataDir)
	app, err := New(pm, st, admin, config.Config{DataDir: pm.DataDir}, memberID)
	if err != nil {
		t.Fatal(err)
	}
	handler := WithEnhancedFormResponses(WithAsyncMultipartFormCompatibility(app.WithAssetWorkflowFixes(app.Handler())))
	path := "/assets/expenses/" + strconv.FormatInt(expenseID, 10)
	serve := func(method, url, token, contentType string, body []byte, enhanced bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, url, bytes.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "fmly_session", Value: token})
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if enhanced {
			req.Header.Set(enhancedFormHeader, "1")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	page := serve(http.MethodGet, path+"/edit", editorToken, "", nil, false)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `action="`+path+`/attachments"`) {
		t.Fatalf("editor page status=%d, upload form missing: %s", page.Code, page.Body.String())
	}
	readOnly := serve(http.MethodGet, path+"/edit", viewerToken, "", nil, false)
	if readOnly.Code != http.StatusOK || strings.Contains(readOnly.Body.String(), `action="`+path+`/attachments"`) {
		t.Fatalf("viewer page status=%d, upload form should be hidden", readOnly.Code)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("evidence", "invoice.txt")
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("expense evidence integration fixture")
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	contentType := writer.FormDataContentType()
	forbidden := serve(http.MethodPost, path+"/attachments", viewerToken, contentType, body.Bytes(), false)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer upload status=%d, want 403", forbidden.Code)
	}
	unauthenticated := serve(http.MethodPost, path+"/attachments", "", contentType, body.Bytes(), false)
	if unauthenticated.Code != http.StatusSeeOther || unauthenticated.Header().Get("Location") != "/login" {
		t.Fatalf("unauthenticated upload status=%d, location=%q", unauthenticated.Code, unauthenticated.Header().Get("Location"))
	}
	if empty := serve(http.MethodPost, path+"/attachments", editorToken, "", nil, false); empty.Code != http.StatusBadRequest {
		t.Fatalf("empty upload status=%d, want 400", empty.Code)
	}
	upload := serve(http.MethodPost, path+"/attachments", editorToken, contentType, body.Bytes(), true)
	if upload.Code != http.StatusOK {
		t.Fatalf("editor upload status=%d: %s", upload.Code, upload.Body.String())
	}
	var response enhancedFormResponse
	if err := json.Unmarshal(upload.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Redirect != path+"/edit#evidence" {
		t.Fatalf("upload response=%+v", response)
	}
	attachments, err := st.EvidenceFor(ctx, "expense", expenseID)
	if err != nil || len(attachments) != 1 || attachments[0].OriginalName != "invoice.txt" {
		t.Fatalf("uploaded attachments=%+v, err=%v", attachments, err)
	}
	attachmentID := attachments[0].ID
	page = serve(http.MethodGet, path+"/edit", editorToken, "", nil, false)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "invoice.txt") || !strings.Contains(page.Body.String(), "/attachments/"+strconv.FormatInt(attachmentID, 10)+"/delete") {
		t.Fatalf("uploaded attachment missing from edit page, status=%d", page.Code)
	}
	download := serve(http.MethodGet, "/evidence/"+strconv.FormatInt(attachmentID, 10), editorToken, "", nil, false)
	if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), content) {
		t.Fatalf("evidence download status=%d, body=%q", download.Code, download.Body.String())
	}
	var replacementBody bytes.Buffer
	replacementWriter := multipart.NewWriter(&replacementBody)
	replacementFile, err := replacementWriter.CreateFormFile("evidence", "replacement.pdf")
	if err != nil {
		t.Fatal(err)
	}
	replacementContent := []byte("%PDF-1.4 replacement evidence")
	if _, err := replacementFile.Write(replacementContent); err != nil {
		t.Fatal(err)
	}
	if err := replacementWriter.Close(); err != nil {
		t.Fatal(err)
	}
	replacement := serve(http.MethodPost, path+"/attachments", editorToken, replacementWriter.FormDataContentType(), replacementBody.Bytes(), false)
	if replacement.Code != http.StatusSeeOther {
		t.Fatalf("replacement upload status=%d: %s", replacement.Code, replacement.Body.String())
	}
	attachments, err = st.EvidenceFor(ctx, "expense", expenseID)
	if err != nil || len(attachments) != 2 || attachments[1].OriginalName != "replacement.pdf" {
		t.Fatalf("replacement attachments=%+v, err=%v", attachments, err)
	}
	replacementID := attachments[1].ID
	deleteURL := path + "/attachments/" + strconv.FormatInt(attachmentID, 10) + "/delete"
	if got := serve(http.MethodPost, deleteURL, viewerToken, "", nil, false); got.Code != http.StatusForbidden {
		t.Fatalf("viewer delete status=%d, want 403", got.Code)
	}
	wrongExpenseURL := "/assets/expenses/" + strconv.FormatInt(otherExpenseID, 10) + "/attachments/" + strconv.FormatInt(attachmentID, 10) + "/delete"
	if got := serve(http.MethodPost, wrongExpenseURL, editorToken, "", nil, false); got.Code != http.StatusBadRequest {
		t.Fatalf("cross-expense delete status=%d, want 400", got.Code)
	}
	deleted := serve(http.MethodPost, deleteURL, editorToken, "", nil, false)
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != path+"/edit#evidence" {
		t.Fatalf("delete status=%d, location=%q", deleted.Code, deleted.Header().Get("Location"))
	}
	attachments, err = st.EvidenceFor(ctx, "expense", expenseID)
	if err != nil || len(attachments) != 1 || attachments[0].ID != replacementID {
		t.Fatalf("replacement lost when deleting old evidence: %+v, err=%v", attachments, err)
	}
	page = serve(http.MethodGet, path+"/edit", editorToken, "", nil, false)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "/evidence/"+strconv.FormatInt(attachmentID, 10)+`"`) || !strings.Contains(page.Body.String(), "/evidence/"+strconv.FormatInt(replacementID, 10)+`"`) {
		t.Fatalf("replacement not shown after deleting old evidence, status=%d", page.Code)
	}
	if got := serve(http.MethodGet, "/evidence/"+strconv.FormatInt(attachmentID, 10), editorToken, "", nil, false); got.Code == http.StatusOK {
		t.Fatal("deleted evidence is still downloadable")
	}
	if got := serve(http.MethodGet, "/evidence/"+strconv.FormatInt(replacementID, 10), editorToken, "", nil, false); got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), replacementContent) {
		t.Fatalf("replacement evidence download status=%d", got.Code)
	}
	replacementDeleteURL := path + "/attachments/" + strconv.FormatInt(replacementID, 10) + "/delete"
	if got := serve(http.MethodPost, replacementDeleteURL, editorToken, "", nil, false); got.Code != http.StatusSeeOther {
		t.Fatalf("replacement delete status=%d", got.Code)
	}
	attachments, err = st.EvidenceFor(ctx, "expense", expenseID)
	if err != nil || len(attachments) != 0 {
		t.Fatalf("attachments after final delete=%+v, err=%v", attachments, err)
	}
	page = serve(http.MethodGet, path+"/edit", editorToken, "", nil, false)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "暂无支付凭证") {
		t.Fatalf("empty evidence state missing, status=%d", page.Code)
	}
	for _, action := range []string{"add_evidence", "delete_evidence"} {
		var count int
		if err := pm.ActiveDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action=? AND entity_type='expense' AND entity_id=?`, action, expenseID).Scan(&count); err != nil || count != 2 {
			t.Fatalf("audit %s count=%d, err=%v", action, count, err)
		}
	}

	adminResult, err := pm.SystemDB.ExecContext(ctx, `INSERT INTO admin_users(username,password_hash,created_at,updated_at) VALUES('test-admin','',?,?)`, "2026-10-20T00:00:00Z", "2026-10-20T00:00:00Z")
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
	adminServe := func(method, url, contentType string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, url, bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "fmly_admin_session", Value: adminToken})
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	adminPage := adminServe(http.MethodGet, path+"/edit", "", nil)
	if adminPage.Code != http.StatusOK || !strings.Contains(adminPage.Body.String(), `action="`+path+`/attachments"`) {
		t.Fatalf("admin edit page status=%d, upload form missing", adminPage.Code)
	}
	adminUpload := adminServe(http.MethodPost, path+"/attachments", replacementWriter.FormDataContentType(), replacementBody.Bytes())
	if adminUpload.Code != http.StatusSeeOther {
		t.Fatalf("admin upload status=%d: %s", adminUpload.Code, adminUpload.Body.String())
	}
	attachments, err = st.EvidenceFor(ctx, "expense", expenseID)
	if err != nil || len(attachments) != 1 || attachments[0].OriginalName != "replacement.pdf" {
		t.Fatalf("admin uploaded attachments=%+v, err=%v", attachments, err)
	}
	adminDeleteURL := path + "/attachments/" + strconv.FormatInt(attachments[0].ID, 10) + "/delete"
	if got := adminServe(http.MethodPost, adminDeleteURL, "", nil); got.Code != http.StatusSeeOther {
		t.Fatalf("admin delete status=%d: %s", got.Code, got.Body.String())
	}
}
