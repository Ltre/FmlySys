package httpserver

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultSystemTimezone = "Asia/Shanghai"

func normalizeTimezoneName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultSystemTimezone
	}
	if decoded, err := url.QueryUnescape(raw); err == nil {
		raw = decoded
	}
	if _, err := time.LoadLocation(raw); err != nil {
		return defaultSystemTimezone
	}
	return raw
}

func requestTimezone(r *http.Request) string {
	if c, err := r.Cookie("fmly_timezone"); err == nil {
		return normalizeTimezoneName(c.Value)
	}
	return defaultSystemTimezone
}

func timezoneLocation(name string) *time.Location {
	name = normalizeTimezoneName(name)
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return time.FixedZone("UTC+8", 8*60*60)
}

func requestLocation(r *http.Request) *time.Location {
	return timezoneLocation(requestTimezone(r))
}

func medicationDateInTimezone(now time.Time, timezone string) string {
	return now.In(timezoneLocation(timezone)).Format("2006-01-02")
}

func medicationDateForRequest(r *http.Request) string {
	return time.Now().In(requestLocation(r)).Format("2006-01-02")
}
