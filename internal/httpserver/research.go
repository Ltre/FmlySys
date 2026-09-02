package httpserver

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ltre/FmlySys/internal/store"
)

type researchQuickSite struct {
	Name    string
	Logo    string
	IconURL string
	URL     string
}

type researchRecommendation struct {
	Source      string
	Title       string
	Description string
	URL         string
}

type researchPageView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	CurrentMember   store.Member
	Permissions     map[string]bool
	QuickSites      []researchQuickSite
	Recommendations []researchRecommendation
}

func (s *Server) WithResearch(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /research", s.member("", s.researchPage))
	mux.HandleFunc("GET /research/proxy", s.member("", s.researchProxy))
	mux.HandleFunc("GET /research/sw.js", s.member("", s.researchServiceWorker))
	mux.Handle("/", next)
	return mux
}

func (s *Server) researchPage(w http.ResponseWriter, r *http.Request) {
	v := researchPageView{
		Title:           "查资料",
		ActivePartition: s.PM.ActiveID,
		CurrentMember:   currentMember(r),
		Permissions:     currentPermissions(r),
		QuickSites: []researchQuickSite{
			{Name: "Google", Logo: "G", IconURL: "https://www.google.com/favicon.ico", URL: "https://www.google.com/"},
			{Name: "Wikipedia", Logo: "W", IconURL: "https://zh.wikipedia.org/static/favicon/wikipedia.ico", URL: "https://zh.wikipedia.org/"},
			{Name: "YouTube", Logo: "▶", IconURL: "https://www.youtube.com/favicon.ico", URL: "https://www.youtube.com/"},
			{Name: "Google 新闻", Logo: "N", IconURL: "https://news.google.com/favicon.ico", URL: "https://news.google.com/"},
			{Name: "翻译", Logo: "文", IconURL: "https://translate.google.com/favicon.ico", URL: "https://translate.google.com/"},
			{Name: "PubMed", Logo: "P", IconURL: "https://pubmed.ncbi.nlm.nih.gov/favicon.ico", URL: "https://pubmed.ncbi.nlm.nih.gov/"},
		},
	}
	v.Recommendations = s.researchRecommendations(r, v.Permissions)
	if err := s.Templates.ExecuteTemplate(w, "research.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func researchSearchURL(query string) string {
	return "https://www.google.com/search?q=" + url.QueryEscape(strings.TrimSpace(query))
}

func (s *Server) researchRecommendations(r *http.Request, perms map[string]bool) []researchRecommendation {
	items := []researchRecommendation{
		{Source: "家庭通用", Title: "家庭办事与权益资料", Description: "查询社保、医保、银行、证件、继承与公共事务的官方说明。", URL: researchSearchURL("家庭 社保 医保 银行 证件 办事 官方 指南")},
		{Source: "家庭通用", Title: "家庭资产与财务管理", Description: "查找家庭共同财产、费用记录、预算与风险管理资料。", URL: researchSearchURL("家庭 共同财产 财务管理 预算 风险管理")},
	}

	if perms["matters.view"] {
		rows, err := s.Store.DB.QueryContext(r.Context(), `
SELECT title, COALESCE(description,'')
FROM matters
WHERE status IN ('planned','active')
ORDER BY CASE WHEN due_date IS NULL OR due_date='' THEN 1 ELSE 0 END, due_date, updated_at DESC
LIMIT 4`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var title, description string
				if rows.Scan(&title, &description) == nil {
					query := title
					if strings.TrimSpace(description) != "" {
						query += " " + description
					}
					items = append(items, researchRecommendation{Source: "进行中的家族事务", Title: title, Description: "根据当前家族事务生成的查资料建议。", URL: researchSearchURL(query)})
				}
			}
		}
	}

	if perms["medication.view"] {
		rows, err := s.Store.DB.QueryContext(r.Context(), `
SELECT DISTINCT medicine_name
FROM medication_plans
WHERE COALESCE(is_deleted,0)=0
ORDER BY updated_at DESC
LIMIT 3`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var medicine string
				if rows.Scan(&medicine) == nil && strings.TrimSpace(medicine) != "" {
					items = append(items, researchRecommendation{Source: "服药管理", Title: "了解 " + medicine, Description: "查询药品说明、常见注意事项与权威健康资料；具体用药仍以医生和药师意见为准。", URL: researchSearchURL(medicine + " 药品说明 注意事项")})
				}
			}
		}
	}

	if perms["share.view"] {
		rows, err := s.Store.DB.QueryContext(r.Context(), `
SELECT title, category
FROM archives
WHERE visibility='family'
ORDER BY updated_at DESC, id DESC
LIMIT 3`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var title, category string
				if rows.Scan(&title, &category) == nil {
					items = append(items, researchRecommendation{Source: "家庭共享资料 · " + category, Title: title, Description: "围绕家庭已有共享资料继续补充外部信息。", URL: researchSearchURL(title + " " + category)})
				}
			}
		}
	}

	if len(items) > 10 {
		items = items[:10]
	}
	return items
}

func (s *Server) researchServiceWorker(w http.ResponseWriter, r *http.Request) {
	member := currentMember(r)
	if member.ID <= 0 {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/research/")
	_, _ = fmt.Fprint(w, researchServiceWorkerJS)
}

const researchServiceWorkerJS = `
const CACHE_NAME = 'fmly-research-pages-v1';
const MAX_ENTRIES = 60;
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
self.addEventListener('fetch', event => {
  const request = event.request;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin || url.pathname !== '/research/proxy') return;
  event.respondWith((async () => {
    const cache = await caches.open(CACHE_NAME);
    try {
      const response = await fetch(request);
      if (response.ok) {
        await cache.put(request, response.clone());
        const keys = await cache.keys();
        while (keys.length > MAX_ENTRIES) {
          await cache.delete(keys.shift());
        }
      }
      return response;
    } catch (error) {
      const cached = await cache.match(request);
      if (cached) return cached;
      throw error;
    }
  })());
});
`
