package domain

import (
	"strings"
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

type PlanStatus string

const (
	PlanDraft     PlanStatus = "draft"
	PlanApproved  PlanStatus = "approved"
	PlanActive    PlanStatus = "active"
	PlanCompleted PlanStatus = "completed"
	PlanCancelled PlanStatus = "cancelled"
)

type PrepPlan struct {
	ID             string          `json:"id"`
	SpecimenID     string          `json:"specimen_id"`
	Objective      string          `json:"objective"`
	Method         string          `json:"method"`
	Adhesive       string          `json:"adhesive"`
	TargetExposure int             `json:"target_exposure"`
	StandardMethod *MethodSnapshot `json:"standard_method,omitempty"`
	Status         PlanStatus      `json:"status"`
	ApprovedBy     string          `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time      `json:"approved_at,omitempty"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func NewPrepPlan(
	id string,
	specimen Specimen,
	objective string,
	method string,
	adhesive string,
	targetExposure int,
	standard *MethodSnapshot,
	now time.Time,
) (PrepPlan, error) {
	if id == "" {
		return PrepPlan{}, contracts.Internal("missing_generated_id", "方案标识未生成", nil)
	}
	if !specimen.CanCreatePlan() {
		return PrepPlan{}, contracts.Conflict("specimen_not_plannable", "当前标本状态不能创建修复方案")
	}
	goal, err := requiredText("修复目标", objective, 200)
	if err != nil {
		return PrepPlan{}, err
	}
	approach := strings.TrimSpace(method)
	if approach == "" && standard != nil {
		approach = standard.Name
	}
	approach, err = requiredText("修复方法", approach, 160)
	if err != nil {
		return PrepPlan{}, err
	}
	adhesiveValue, err := optionalText("黏结材料", adhesive, 120)
	if err != nil {
		return PrepPlan{}, err
	}
	if err := validatePercent(targetExposure); err != nil {
		return PrepPlan{}, contracts.Invalid("invalid_target_exposure", "目标暴露比例必须位于 0 到 100 之间")
	}
	if targetExposure == 0 {
		return PrepPlan{}, contracts.Invalid("invalid_target_exposure", "目标暴露比例必须大于零")
	}
	if standard != nil {
		if targetExposure < standard.TargetMin || targetExposure > standard.TargetMax {
			return PrepPlan{}, contracts.Invalid(
				"target_exposure_out_of_range",
				"目标暴露比例超出所引用标准方法的目标边界",
			)
		}
		if adhesiveValue != "" && !snapshotAllowsAdhesive(*standard, adhesiveValue) {
			return PrepPlan{}, contracts.Invalid(
				"adhesive_out_of_range",
				"黏结材料超出所引用标准方法的材料范围",
			)
		}
	}
	return PrepPlan{
		ID:             id,
		SpecimenID:     specimen.ID,
		Objective:      goal,
		Method:         approach,
		Adhesive:       adhesiveValue,
		TargetExposure: targetExposure,
		StandardMethod: standard,
		Status:         PlanDraft,
		Version:        1,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}, nil
}

func snapshotAllowsAdhesive(snapshot MethodSnapshot, adhesive string) bool {
	if len(snapshot.AllowedAdhesives) == 0 {
		return true
	}
	for _, allowed := range snapshot.AllowedAdhesives {
		if allowed == adhesive {
			return true
		}
	}
	return false
}

func (p PrepPlan) IsOpen() bool {
	return p.Status == PlanDraft || p.Status == PlanApproved || p.Status == PlanActive
}

func (p PrepPlan) IsCompleted() bool {
	return p.Status == PlanCompleted
}

func (p *PrepPlan) Approve(approvedBy string, now time.Time) error {
	if p.Status != PlanDraft {
		return contracts.Conflict("plan_not_approvable", "只有草稿方案可以审批")
	}
	approver, err := requiredText("审批人", approvedBy, 80)
	if err != nil {
		return err
	}
	stamp := now.UTC()
	p.Status = PlanApproved
	p.ApprovedBy = approver
	p.ApprovedAt = &stamp
	p.Version++
	p.UpdatedAt = stamp
	return nil
}

func (p *PrepPlan) Begin(now time.Time) error {
	if p.Status == PlanActive {
		return nil
	}
	if p.Status != PlanApproved {
		return contracts.Conflict("plan_not_active", "只有已审批方案可以开始制备")
	}
	p.Status = PlanActive
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *PrepPlan) Complete(now time.Time) error {
	if p.Status == PlanCompleted {
		return nil
	}
	if p.Status != PlanActive {
		return contracts.Conflict("plan_not_completable", "只有进行中的方案可以完成")
	}
	p.Status = PlanCompleted
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *PrepPlan) Cancel(now time.Time) error {
	if p.Status != PlanDraft {
		return contracts.Conflict("plan_not_cancellable", "只有草稿方案可以取消")
	}
	p.Status = PlanCancelled
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}
