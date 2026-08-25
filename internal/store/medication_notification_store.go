package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) SaveMedicationPushSubscription(ctx context.Context, memberID int64, endpoint, p256dh, auth, userAgent string) error {
	endpoint = strings.TrimSpace(endpoint); p256dh = strings.TrimSpace(p256dh); auth = strings.TrimSpace(auth)
	if memberID <= 0 || endpoint == "" || p256dh == "" || auth == "" { return errors.New("PWA 推送订阅参数不完整") }
	_, err := s.DB.ExecContext(ctx, `INSERT INTO medication_push_subscriptions(member_id,endpoint,p256dh,auth,user_agent,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(endpoint) DO UPDATE SET member_id=excluded.member_id,p256dh=excluded.p256dh,auth=excluded.auth,user_agent=excluded.user_agent,updated_at=excluded.updated_at`, memberID, endpoint, p256dh, auth, strings.TrimSpace(userAgent), now(), now()); return err
}
func (s *Store) MedicationPushSubscriptions(ctx context.Context, memberID int64)([]MedicationPushSubscription,error){rows,err:=s.DB.QueryContext(ctx,`SELECT id,member_id,endpoint,p256dh,auth,user_agent,created_at,updated_at FROM medication_push_subscriptions WHERE member_id=? ORDER BY id`,memberID);if err!=nil{return nil,err};defer rows.Close();var out []MedicationPushSubscription;for rows.Next(){var v MedicationPushSubscription;if err:=rows.Scan(&v.ID,&v.MemberID,&v.Endpoint,&v.P256DH,&v.Auth,&v.UserAgent,&v.CreatedAt,&v.UpdatedAt);err!=nil{return nil,err};out=append(out,v)};return out,rows.Err()}
func (s *Store) DeleteMedicationPushSubscription(ctx context.Context,id int64)error{_,err:=s.DB.ExecContext(ctx,`DELETE FROM medication_push_subscriptions WHERE id=?`,id);return err}
func (s *Store) RecordMedicationNotificationDelivery(ctx context.Context,planID int64,date,stage,channel,status,detail string)error{if len(detail)>1000{detail=detail[:1000]};_,err:=s.DB.ExecContext(ctx,`INSERT OR IGNORE INTO medication_notification_deliveries(plan_id,scheduled_date,stage,channel,status,detail,created_at) VALUES(?,?,?,?,?,?,?)`,planID,date,stage,channel,status,detail,now());return err}
func (s *Store) MedicationAutomaticStageAttempted(ctx context.Context,planID int64,date,stage string)(bool,error){if stage=="manual"{return false,nil};var exists int;err:=s.DB.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM medication_notification_deliveries WHERE plan_id=? AND scheduled_date=? AND stage=?)`,planID,date,stage).Scan(&exists);return exists!=0,err}
func (s *Store) MedicationActivePlansForDate(ctx context.Context,date string)([]MedicationPlanFull,error){rows,err:=s.DB.QueryContext(ctx,`SELECT p.id,p.patient_member_id,patient.name,p.medicine_name,p.dosage,p.scheduled_time,p.instructions,p.start_date,COALESCE(p.end_date,''),p.created_by,COALESCE(creator.name,''),p.created_at,p.updated_at FROM medication_plans p JOIN members patient ON patient.id=p.patient_member_id LEFT JOIN members creator ON creator.id=p.created_by WHERE p.is_deleted=0 AND p.start_date<=? AND (p.end_date IS NULL OR p.end_date>=?) ORDER BY p.scheduled_time,p.id`,date,date);if err!=nil{return nil,err};return scanMedicationPlans(rows)}
func (s *Store) MedicationNotificationDeliveries(ctx context.Context,planID int64,date string)([]MedicationNotificationDelivery,error){rows,err:=s.DB.QueryContext(ctx,`SELECT id,plan_id,scheduled_date,stage,channel,status,detail,created_at FROM medication_notification_deliveries WHERE plan_id=? AND scheduled_date=? ORDER BY id DESC`,planID,date);if err!=nil{return nil,err};defer rows.Close();var out []MedicationNotificationDelivery;for rows.Next(){var v MedicationNotificationDelivery;if err:=rows.Scan(&v.ID,&v.PlanID,&v.ScheduledDate,&v.Stage,&v.Channel,&v.Status,&v.Detail,&v.CreatedAt);err!=nil{return nil,err};out=append(out,v)};return out,rows.Err()}
func MedicationStageForTime(now time.Time,scheduled time.Time)string{if now.Before(scheduled){return ""};if !now.Before(scheduled.Add(2*time.Hour)){return "plus2h"};if !now.Before(scheduled.Add(time.Hour)){return "plus1h"};return "scheduled"}
func (s *Store) ValidateMedicationPlanActiveToday(ctx context.Context,planID int64,date string)(MedicationPlanFull,error){plan,err:=s.MedicationPlanFullByID(ctx,planID,date);if err!=nil{return MedicationPlanFull{},err};if !plan.ActiveOn(date){return MedicationPlanFull{},fmt.Errorf("该计划在 %s 不处于进行中状态",date)};return plan,nil}
