package store

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const medicationDateLayout = "2006-01-02"

type MedicationPlanFull struct {
	ID              int64
	PatientMemberID int64
	PatientName     string
	MedicineName    string
	Dosage          string
	ScheduledTime   string
	Instructions    string
	StartDate       string
	EndDate         string
	CreatedBy       int64
	CreatorName     string
	CreatedAt       string
	UpdatedAt       string
	IsDeleted       bool
	DeletedAt       string
	RecordID        int64
	RecordStatus    string
	RecordNote      string
	RecordedBy      int64
	RecordedByName  string
	RecordedAt      string
	CheckinResponse string
	CheckinStatus   string
	CheckinAt       string
	VerifiedByName  string
	VerifiedAt      string
}

func (p MedicationPlanFull) Status(today string) string {
	if p.EndDate != "" && today > p.EndDate {
		return "已结束"
	}
	if today < p.StartDate {
		return "未开始"
	}
	return "进行中"
}

func (p MedicationPlanFull) ActiveOn(date string) bool {
	return !p.IsDeleted && date >= p.StartDate && (p.EndDate == "" || date <= p.EndDate)
}

type MedicationRangeSummary struct {
	FromDate, ToDate                     string
	Days                                 int
	Scheduled, Taken, Missed, Unrecorded int
	TakenPercent                         int
}

type MedicationCheckin struct {
	ID, PlanID, PatientMemberID, VerifiedBy   int64
	ScheduledDate, Response, ResponseAt       string
	VerificationStatus, VerifiedAt, UpdatedAt string
}

type MedicationPushSubscription struct {
	ID, MemberID                      int64
	Endpoint, P256DH, Auth, UserAgent string
	CreatedAt, UpdatedAt              string
}

type MedicationNotificationDelivery struct {
	ID, PlanID                            int64
	ScheduledDate, Stage, Channel, Status string
	Detail, CreatedAt                     string
}

func init() {
	catalog := make([]PermissionDef, 0, len(PermissionCatalog)+1)
	for _, p := range PermissionCatalog {
		switch p.Key {
		case "medication.manage":
			continue
		case "medication.view":
			p.Label = "查看服药管理"
		}
		catalog = append(catalog, p)
	}
	catalog = append(catalog,
		PermissionDef{Key: "medication.manage_self", Label: "管理自己创建的服药计划"},
		PermissionDef{Key: "medication.manage_others", Label: "管理他人创建的服药计划"},
	)
	PermissionCatalog = catalog
}

func CanManageMedicationPlan(perms map[string]bool, memberID, creatorID int64) bool {
	if memberID <= 0 || creatorID <= 0 {
		return false
	}
	if memberID == creatorID {
		return perms["medication.manage_self"]
	}
	return perms["medication.manage_others"]
}

func normalizeMedicationEndDate(startDate, endDate string) (string, error) {
	endDate = strings.TrimSpace(endDate)
	if endDate == "" {
		return "", nil
	}
	if _, err := time.Parse(medicationDateLayout, endDate); err != nil {
		return "", errors.New("结束日期格式无效")
	}
	if endDate < startDate {
		return "", errors.New("结束日期不能早于开始日期")
	}
	return endDate, nil
}

func validateMedicationPlanFields(patientID int64, medicine, dosage, scheduledTime, instructions, startDate, endDate string) (MedicationPlanFull, error) {
	medicine = strings.TrimSpace(medicine)
	dosage = strings.TrimSpace(dosage)
	scheduledTime = strings.TrimSpace(scheduledTime)
	instructions = strings.TrimSpace(instructions)
	startDate = strings.TrimSpace(startDate)
	if patientID <= 0 {
		return MedicationPlanFull{}, errors.New("请选择服药成员")
	}
	if medicine == "" || utf8.RuneCountInString(medicine) > 120 {
		return MedicationPlanFull{}, errors.New("药品名称必填且最多 120 个字符")
	}
	if dosage == "" || utf8.RuneCountInString(dosage) > 120 {
		return MedicationPlanFull{}, errors.New("每次用量必填且最多 120 个字符")
	}
	if _, err := time.Parse("15:04", scheduledTime); err != nil {
		return MedicationPlanFull{}, errors.New("计划服药时间格式无效")
	}
	if utf8.RuneCountInString(instructions) > 500 {
		return MedicationPlanFull{}, errors.New("服药说明最多 500 个字符")
	}
	if startDate == "" {
		startDate = time.Now().Format(medicationDateLayout)
	}
	if _, err := time.Parse(medicationDateLayout, startDate); err != nil {
		return MedicationPlanFull{}, errors.New("开始日期格式无效")
	}
	endDate, err := normalizeMedicationEndDate(startDate, endDate)
	if err != nil {
		return MedicationPlanFull{}, err
	}
	return MedicationPlanFull{
		PatientMemberID: patientID,
		MedicineName:    medicine,
		Dosage:          dosage,
		ScheduledTime:   scheduledTime,
		Instructions:    instructions,
		StartDate:       startDate,
		EndDate:         endDate,
	}, nil
}
