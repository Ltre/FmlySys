package httpserver

import (
	"encoding/json"
	"net/http"
	"time"
)

const wechatDeveloperEventLimit = 50

type wechatCallbackProbeEvent struct {
	ReceivedAt   string `json:"received_at"`
	MsgType      string `json:"msg_type"`
	Event        string `json:"event"`
	FromUserName string `json:"from_user_name"`
	ToUserName   string `json:"to_user_name"`
	EventKey     string `json:"event_key"`
}

func (s *Server) recordWeChatCallbackEvent(message wechatCodeMessage) {
	event := wechatCallbackProbeEvent{
		ReceivedAt:   time.Now().UTC().Format(time.RFC3339),
		MsgType:      message.MsgType,
		Event:        message.Event,
		FromUserName: message.FromUserName,
		ToUserName:   message.ToUserName,
		EventKey:     message.EventKey,
	}

	s.wechatEventMu.Lock()
	defer s.wechatEventMu.Unlock()
	s.wechatEvents = append(s.wechatEvents, event)
	if len(s.wechatEvents) > wechatDeveloperEventLimit {
		s.wechatEvents = append([]wechatCallbackProbeEvent(nil), s.wechatEvents[len(s.wechatEvents)-wechatDeveloperEventLimit:]...)
	}
}

func (s *Server) latestWeChatCallbackEvents() []wechatCallbackProbeEvent {
	s.wechatEventMu.RLock()
	defer s.wechatEventMu.RUnlock()

	events := make([]wechatCallbackProbeEvent, 0, len(s.wechatEvents))
	for i := len(s.wechatEvents) - 1; i >= 0; i-- {
		events = append(events, s.wechatEvents[i])
	}
	return events
}

func (s *Server) adminDeveloperCenter(w http.ResponseWriter, r *http.Request) {
	v := s.base("开发中心")
	v.AdminUsername = currentAdmin(r).Username
	v.WeChatCallbackPath = "/auth/wechat/code/callback"
	v.WeChatMenuTestPath = "/healthz?source=wechat-menu-view-test"
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, "admin-developer.html", v)
}

func (s *Server) adminWeChatCallbackEventsJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"configured": s.Config.WeChatCodeLoginConfigured(),
		"events":     s.latestWeChatCallbackEvents(),
	})
}
