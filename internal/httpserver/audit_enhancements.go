package httpserver

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/Ltre/FmlySys/internal/store"
)

var superAuditWriteMu sync.Mutex

type auditStatusWriter struct { http.ResponseWriter; status int }
func (w *auditStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *auditStatusWriter) WriteHeader(code int){if w.status==0{w.status=code};w.ResponseWriter.WriteHeader(code)}
func (w *auditStatusWriter) Write(p []byte)(int,error){if w.status==0{w.status=http.StatusOK};return w.ResponseWriter.Write(p)}

func requestIPAddress(r *http.Request) string {for _,key:=range []string{"CF-Connecting-IP","X-Real-IP"}{if v:=strings.TrimSpace(r.Header.Get(key));v!=""{return v}};if v:=strings.TrimSpace(r.Header.Get("X-Forwarded-For"));v!=""{if first:=strings.TrimSpace(strings.Split(v,",")[0]);first!=""{return first}};host,_,err:=net.SplitHostPort(strings.TrimSpace(r.RemoteAddr));if err==nil&&host!=""{return host};return strings.TrimSpace(r.RemoteAddr)}
func shouldRecordMemberAccess(r *http.Request)bool{p:=r.URL.Path;return !(strings.HasPrefix(p,"/static/")||p=="/healthz"||p=="/medication-sw.js")}
func isMutationMethod(method string)bool{switch method{case http.MethodPost,http.MethodPut,http.MethodPatch,http.MethodDelete:return true};return false}
func requestSurface(r *http.Request)string{if strings.HasPrefix(r.URL.Path,"/admin"){return "admin"};return "front"}
func (s *Server) auditRequestMeta(r *http.Request) store.SuperAuditRequestMeta {meta:=store.SuperAuditRequestMeta{Surface:requestSurface(r),IPAddress:requestIPAddress(r),RequestMethod:r.Method,RequestPath:r.URL.Path};if meta.Surface=="admin"{if raw:=cookieValue(r,"fmly_admin_session");raw!=""{if sess,err:=s.Admin.Session(r.Context(),raw);err==nil&&sess.Stage=="authenticated"{meta.AdminUsername=sess.Username}};return meta};if raw:=cookieValue(r,"fmly_session");raw!=""{if m,_,err:=s.Store.MemberFromSession(r.Context(),raw);err==nil{meta.MemberID=m.ID;meta.MemberName=m.Name}};return meta}
func sensitiveFormKey(key string)bool{k:=strings.ToLower(key);for _,token:=range []string{"password","passwd","secret","token","credential","auth","p256dh","private","master_key","totp","otp","recovery_code"}{if strings.Contains(k,token){return true}};return false}
func sanitizedMutationSummary(r *http.Request)map[string]any{out:=map[string]any{"method":r.Method,"path":r.URL.Path};if len(r.URL.Query())>0{q:=map[string][]string{};keys:=make([]string,0,len(r.URL.Query()));for key:=range r.URL.Query(){keys=append(keys,key)};sort.Strings(keys);for _,key:=range keys{if sensitiveFormKey(key){q[key]=[]string{"[REDACTED]"}}else{q[key]=append([]string(nil),r.URL.Query()[key]...)}};out["query"]=q};if r.Form!=nil{form:=map[string][]string{};keys:=make([]string,0,len(r.Form));for key:=range r.Form{keys=append(keys,key)};sort.Strings(keys);for _,key:=range keys{if sensitiveFormKey(key){form[key]=[]string{"[REDACTED]"}}else{form[key]=append([]string(nil),r.Form[key]...)}};if len(form)>0{out["form"]=form}};return out}
func auditCategoryForPath(path string)string{switch{case strings.HasPrefix(path,"/medication"):return "medication";case strings.HasPrefix(path,"/matters"):return "matter";case strings.HasPrefix(path,"/share"):return "archive";case strings.HasPrefix(path,"/assets"):return "asset";case strings.Contains(path,"/members"):return "member";case strings.Contains(path,"/passkeys"):return "passkey";case strings.Contains(path,"/remote-notifications"):return "remote_notification_config";case strings.Contains(path,"/join"):return "join_request";case strings.Contains(path,"/login")||strings.Contains(path,"/logout"):return "session"};trimmed:=strings.Trim(path,"/");if trimmed==""{return "request"};if i:=strings.IndexByte(trimmed,'/');i>=0{trimmed=trimmed[:i]};return trimmed}
func fallbackAuditAction(r *http.Request)string{p:=strings.ToLower(r.URL.Path);if r.Method==http.MethodDelete||strings.Contains(p,"delete")||strings.Contains(p,"remove")||strings.Contains(p,"revoke"){return "delete"};if strings.Contains(p,"create")||strings.Contains(p,"add")||strings.Contains(p,"register")||strings.HasSuffix(p,"/plans")||strings.HasSuffix(p,"/members"){return "create"};return "update"}

func (s *Server) WithSuperAudit(next http.Handler) http.Handler {return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){meta:=s.auditRequestMeta(r);if meta.MemberID>0&&shouldRecordMemberAccess(r){member:=store.Member{ID:meta.MemberID,Name:meta.MemberName};if err:=s.Store.RecordMemberAccess(r.Context(),meta.IPAddress,member,r.Method,r.URL.RequestURI());err!=nil{log.Printf("record member access: %v",err)}};if !isMutationMethod(r.Method){next.ServeHTTP(w,r);return};superAuditWriteMu.Lock();defer superAuditWriteMu.Unlock();beforeID,err:=s.Store.MaxAuditLogID(r.Context());if err!=nil{log.Printf("super audit max id: %v",err);beforeID=0};sw:=&auditStatusWriter{ResponseWriter:w};next.ServeHTTP(sw,r);if sw.status==0{sw.status=http.StatusOK};captured,err:=s.Store.CaptureSuperAuditSince(context.WithoutCancel(r.Context()),beforeID,meta);if err!=nil{log.Printf("capture super audit: %v",err);return};if captured==0&&sw.status<400{summary:=sanitizedMutationSummary(r);if payload,err:=json.Marshal(summary);err==nil&&len(payload)>8192{summary=map[string]any{"method":r.Method,"path":r.URL.Path,"note":"request summary omitted because it exceeded 8 KiB"}};if err:=s.Store.RecordFallbackSuperAudit(context.WithoutCancel(r.Context()),meta,auditCategoryForPath(r.URL.Path),fallbackAuditAction(r),summary);err!=nil{log.Printf("fallback super audit: %v",err)}}})}
