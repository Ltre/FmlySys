package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Ltre/FmlySys/internal/store"
)

type notificationCenterView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Notifications   []store.MemberNotification
	Timezone        string
}

type notificationDetailView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	Notification    store.MemberNotification
	Timezone        string
}

func (s *Server) notificationCenter(w http.ResponseWriter, r *http.Request) {
	member := currentMember(r)
	items, err := s.Store.MemberNotifications(r.Context(), member.ID, 200)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := notificationCenterView{
		Title:           "通知中心",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   member,
		Permissions:     currentPermissions(r),
		Notifications:   items,
		Timezone:        requestTimezone(r),
	}
	if err := s.Templates.ExecuteTemplate(w, "notifications.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) notificationDetail(w http.ResponseWriter, r *http.Request) {
	member := currentMember(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	n, err := s.Store.MemberNotificationByID(r.Context(), member.ID, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	v := notificationDetailView{
		Title:           n.Title,
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   member,
		Permissions:     currentPermissions(r),
		Notification:    n,
		Timezone:        requestTimezone(r),
	}
	if err := s.Templates.ExecuteTemplate(w, "notification-detail.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) notificationMarkRead(w http.ResponseWriter, r *http.Request) {
	member := currentMember(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	if _, err := s.Store.MemberNotificationByID(r.Context(), member.ID, id); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.Store.MarkMemberNotificationRead(r.Context(), member.ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dispatchNotificationCenterPush(ctx context.Context, memberID, notificationID int64, title, body string) error {
	key, err := loadOrCreateMedicationVAPIDKey(s.Config.DataDir)
	if err != nil {
		return err
	}
	subs, err := s.Store.MedicationPushSubscriptions(ctx, memberID)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return errors.New("接收成员没有启用 PWA 通知")
	}
	payload, _ := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
		"voice": body,
		"url":   "/notifications/" + strconv.FormatInt(notificationID, 10),
	})
	var sent bool
	var lastErr error
	for _, sub := range subs {
		statusCode, sendErr := sendWebPush(ctx, key, sub.Endpoint, sub.P256DH, sub.Auth, payload)
		if sendErr == nil {
			sent = true
			continue
		}
		lastErr = sendErr
		if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
			_ = s.Store.DeleteMedicationPushSubscription(ctx, sub.ID)
		}
	}
	if sent {
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("PWA 通知没有成功投递")
	}
	return lastErr
}

func (s *Server) WithNotificationCenter(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notifications", s.member("", s.notificationCenter))
	mux.HandleFunc("GET /notifications/{id}", s.member("", s.notificationDetail))
	mux.HandleFunc("POST /notifications/{id}/read", s.member("", s.notificationMarkRead))
	mux.Handle("/", next)
	return mux
}
