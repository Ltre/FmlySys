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

func TestSharedAndMatterImagePreviewPermissions(t *testing.T) {
	ctx := context.Background()
	pm, err := partition.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pm.Close() })
	st := store.New(pm.ActiveDB)
	createMember := func(name string, perms []string) string {
		t.Helper()
		result, err := pm.ActiveDB.ExecContext(ctx, `INSERT INTO members(name,relation,status,created_at) VALUES(?,'','active','2026-10-21T00:00:00Z')`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		if err := st.SetMemberPermissions(ctx, id, perms); err != nil {
			t.Fatal(err)
		}
		token, err := st.CreateMemberSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	ownerToken := createMember("事务创建人", []string{"matters.manage_self", "share.manage_self"})
	viewerToken := createMember("事务查看人", []string{"matters.view", "share.view"})
	noAccessToken := createMember("无查看权限", nil)
	assetOnlyToken := createMember("仅查看公共资产", []string{"assets.view"})
	admin := adminauth.New(pm.SystemDB, bytes.Repeat([]byte{1}, 32), pm.DataDir)
	app, err := New(pm, st, admin, config.Config{DataDir: pm.DataDir}, 1)
	if err != nil {
		t.Fatal(err)
	}
	handler := app.Handler()
	serve := func(method, path, token, contentType string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "fmly_session", Value: token})
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	fileForm := func(name string, content []byte) ([]byte, string) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return body.Bytes(), writer.FormDataContentType()
	}

	matterPost := serve(http.MethodPost, "/matters", ownerToken, "application/x-www-form-urlencoded", []byte("title=%E6%B5%8B%E8%AF%95%E4%BA%8B%E5%8A%A1"))
	if matterPost.Code != http.StatusSeeOther {
		t.Fatalf("create matter status=%d: %s", matterPost.Code, matterPost.Body.String())
	}
	matters, err := st.Matters(ctx)
	if err != nil || len(matters) != 1 {
		t.Fatalf("matters=%+v, err=%v", matters, err)
	}
	matterID := strconv.FormatInt(matters[0].ID, 10)
	imageForm, imageType := fileForm("事务照片.png", png)
	if denied := serve(http.MethodPost, "/matters/"+matterID+"/attachments", viewerToken, imageType, imageForm); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer upload status=%d", denied.Code)
	}
	if uploaded := serve(http.MethodPost, "/matters/"+matterID+"/attachments", ownerToken, imageType, imageForm); uploaded.Code != http.StatusSeeOther {
		t.Fatalf("matter upload status=%d: %s", uploaded.Code, uploaded.Body.String())
	}
	matterAttachments, err := st.EvidenceFor(ctx, "matter", matters[0].ID)
	if err != nil || len(matterAttachments) != 1 {
		t.Fatalf("matter attachments=%+v, err=%v", matterAttachments, err)
	}
	matterAttachmentID := strconv.FormatInt(matterAttachments[0].ID, 10)
	matterPath := "/matter-files/" + matterAttachmentID
	matterPage := serve(http.MethodGet, "/matters", viewerToken, "", nil)
	if matterPage.Code != http.StatusOK || !strings.Contains(matterPage.Body.String(), `href="`+matterPath+`"`) || !strings.Contains(matterPage.Body.String(), `/static/evidence-image-preview.js`) {
		t.Fatalf("matter attachment preview missing, status=%d", matterPage.Code)
	}
	if strings.Contains(matterPage.Body.String(), "添加事务附件") {
		t.Fatal("viewer must not see matter upload control")
	}

	ownerID := matters[0].CreatedBy
	archiveID, err := st.CreateArchive(ctx, ownerID, "测试共享", "资料", "图片资料", "family")
	if err != nil {
		t.Fatal(err)
	}
	shareForm, shareType := fileForm("共享照片.png", png)
	archivePath := "/share/" + strconv.FormatInt(archiveID, 10) + "/attachments"
	if uploaded := serve(http.MethodPost, archivePath, ownerToken, shareType, shareForm); uploaded.Code != http.StatusSeeOther {
		t.Fatalf("share upload status=%d: %s", uploaded.Code, uploaded.Body.String())
	}
	attachments, err := st.Attachments(ctx, archiveID)
	if err != nil || len(attachments) != 1 {
		t.Fatalf("share attachments=%+v, err=%v", attachments, err)
	}
	sharePath := "/files/" + strconv.FormatInt(attachments[0].ID, 10)
	sharePage := serve(http.MethodGet, "/share-info?id="+strconv.FormatInt(archiveID, 10), viewerToken, "", nil)
	if sharePage.Code != http.StatusOK || !strings.Contains(sharePage.Body.String(), `href="`+sharePath+`"`) || !strings.Contains(sharePage.Body.String(), `/static/evidence-image-preview.js`) {
		t.Fatalf("share attachment preview missing, status=%d", sharePage.Code)
	}
	previewScript := serve(http.MethodGet, "/static/evidence-image-preview.js", "", "", nil)
	if previewScript.Code != http.StatusOK || !strings.Contains(previewScript.Body.String(), `href^="/files/"`) || !strings.Contains(previewScript.Body.String(), `href^="/matter-files/"`) {
		t.Fatalf("shared/matter image links are not enhanced by preview script, status=%d", previewScript.Code)
	}
	for _, path := range []string{matterPath, sharePath} {
		image := serve(http.MethodGet, path, viewerToken, "", nil)
		if image.Code != http.StatusOK || image.Header().Get("Content-Disposition") != "inline" || !strings.HasPrefix(image.Header().Get("Content-Type"), "image/png") || !bytes.Equal(image.Body.Bytes(), png) {
			t.Fatalf("image %s status=%d, type=%q, disposition=%q", path, image.Code, image.Header().Get("Content-Type"), image.Header().Get("Content-Disposition"))
		}
		if denied := serve(http.MethodGet, path, noAccessToken, "", nil); denied.Code != http.StatusForbidden {
			t.Fatalf("no-access image %s status=%d", path, denied.Code)
		}
	}
	if exposed := serve(http.MethodGet, "/evidence/"+matterAttachmentID, assetOnlyToken, "", nil); exposed.Code == http.StatusOK {
		t.Fatal("matter image must not be accessible through financial evidence route")
	}
	textForm, textType := fileForm("说明.txt", []byte("ordinary attachment"))
	if uploaded := serve(http.MethodPost, archivePath, ownerToken, textType, textForm); uploaded.Code != http.StatusSeeOther {
		t.Fatalf("text upload status=%d: %s", uploaded.Code, uploaded.Body.String())
	}
	attachments, err = st.Attachments(ctx, archiveID)
	if err != nil || len(attachments) != 2 {
		t.Fatalf("share text attachment=%+v, err=%v", attachments, err)
	}
	textResponse := serve(http.MethodGet, "/files/"+strconv.FormatInt(attachments[1].ID, 10), viewerToken, "", nil)
	if textResponse.Code != http.StatusOK || !strings.HasPrefix(textResponse.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("non-image response status=%d, disposition=%q", textResponse.Code, textResponse.Header().Get("Content-Disposition"))
	}
	if err := st.DeleteMatterAttachment(ctx, ownerID, matters[0].ID, matterAttachments[0].ID+10000, evidenceDir(app)); err == nil {
		t.Fatal("an unrelated attachment cannot be removed from the matter")
	}
	deletePath := "/matters/" + matterID + "/attachments/" + matterAttachmentID + "/delete"
	if denied := serve(http.MethodPost, deletePath, viewerToken, "", nil); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer delete status=%d", denied.Code)
	}
	if deleted := serve(http.MethodPost, deletePath, ownerToken, "", nil); deleted.Code != http.StatusSeeOther {
		t.Fatalf("owner delete status=%d: %s", deleted.Code, deleted.Body.String())
	}
	if missing := serve(http.MethodGet, matterPath, viewerToken, "", nil); missing.Code != http.StatusNotFound {
		t.Fatalf("deleted matter file status=%d", missing.Code)
	}
}
