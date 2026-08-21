package store

import (
	"mime/multipart"
	"testing"
)

func TestValidateEvidenceFiles(t *testing.T) {
	ok := []*multipart.FileHeader{{Filename: "receipt.jpg", Size: 1024}, {Filename: "invoice.pdf", Size: EvidenceMaxFileSize}}
	if err := ValidateEvidenceFiles(ok); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEvidenceFiles([]*multipart.FileHeader{{Filename: "x.exe", Size: 10}}); err == nil {
		t.Fatal("exe should be rejected")
	}
	if err := ValidateEvidenceFiles([]*multipart.FileHeader{{Filename: "big.pdf", Size: EvidenceMaxFileSize + 1}}); err == nil {
		t.Fatal("oversize file should be rejected")
	}
}
