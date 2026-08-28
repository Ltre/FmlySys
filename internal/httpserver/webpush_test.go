package httpserver

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"testing"
)

func TestMedicationVAPIDKeyPersists(t *testing.T) {
	dir := t.TempDir()
	first, err := loadOrCreateMedicationVAPIDKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateMedicationVAPIDKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(medicationVAPIDPublicKeyRaw(first), medicationVAPIDPublicKeyRaw(second)) {
		t.Fatal("VAPID key changed after reload")
	}
}

func TestWebPushEncryptionBuildsAES128GCMRecord(t *testing.T) {
	client, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	body, err := encryptWebPushAES128GCM(client.PublicKey().Bytes(), auth, []byte(`{"hello":"world"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= 16+4+1+65 {
		t.Fatalf("encrypted record too short: %d", len(body))
	}
}
