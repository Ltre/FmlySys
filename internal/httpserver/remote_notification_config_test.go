package httpserver

import "testing"

func TestRemoteNotificationConfigEncryptedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := RemoteNotificationConfig{Host: "127.0.0.1", Port: 23001, Username: "u0_a123", Password: "secret-value"}
	if err := saveRemoteNotificationConfig(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadRemoteNotificationConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
	if _, err := loadOrCreateRemoteNotificationKey(dir); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteNotificationConfigRejectsNonLoopback(t *testing.T) {
	_, err := validateRemoteNotificationConfig(RemoteNotificationConfig{Host: "example.com", Port: 22, Username: "u", Password: "p"})
	if err == nil {
		t.Fatal("non-loopback SSH target must be rejected")
	}
}
