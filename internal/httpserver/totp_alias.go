package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/Ltre/FmlySys/internal/adminauth"
)

const maxTOTPAliasRunes = 80

// WithTOTPSetupAlias wraps the main handler so the setup QR code can use a
// user-selected account alias without changing the TOTP secret itself.
func WithTOTPSetupAlias(s *Server, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/admin/totp/qr" {
			s.adminTOTPQRCodeWithAlias(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func normalizeTOTPAlias(input, fallback string) (string, error) {
	alias := strings.TrimSpace(input)
	if alias == "" {
		alias = strings.TrimSpace(fallback)
	}
	if alias == "" {
		alias = "admin"
	}
	runes := []rune(alias)
	if len(runes) > maxTOTPAliasRunes {
		return "", fmt.Errorf("Google Authenticator 密钥别名最多 %d 个字符", maxTOTPAliasRunes)
	}
	for _, r := range runes {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("Google Authenticator 密钥别名不能包含控制字符")
		}
	}
	return alias, nil
}

func (s *Server) adminTOTPQRCodeWithAlias(w http.ResponseWriter, r *http.Request) {
	sess, _, err := s.adminStage(r, "totp_setup")
	if err != nil {
		http.Error(w, "后台认证状态无效", http.StatusUnauthorized)
		return
	}
	alias, err := normalizeTOTPAlias(r.URL.Query().Get("alias"), sess.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	secret, err := s.Admin.EnsureTOTPSecret(r.Context(), sess.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	png, err := qrcode.Encode(adminauth.OTPAuthURI(alias, secret), qrcode.Medium, 256)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}
