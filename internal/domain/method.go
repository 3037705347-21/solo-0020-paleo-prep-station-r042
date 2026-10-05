package domain

import (
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

type MethodVersionStatus string

const (
	MethodVersionDraft     MethodVersionStatus = "draft"
	MethodVersionPublished MethodVersionStatus = "published"
	MethodVersionRetired   MethodVersionStatus = "retired"
)

// MethodDraftInput carries the editable content of one method version.
type MethodDraftInput struct {
	Steps            []string
	AllowedAdhesives []string
	TargetMin        int
	TargetMax        int
}

// MethodVersion is one immutable-edition revision of a standard method.
type MethodVersion struct {
	Version          int                 `json:"version"`
	Status           MethodVersionStatus `json:"status"`
	Steps            []string            `json:"steps"`
	AllowedAdhesives []string            `json:"allowed_adhesives"`
	TargetMin        int                 `json:"target_min"`
	TargetMax        int                 `json:"target_max"`
	PublishedBy      string              `json:"published_by,omitempty"`
	PublishedAt      *time.Time          `json:"published_at,omitempty"`
	RetiredBy        string              `json:"retired_by,omitempty"`
	RetiredAt        *time.Time          `json:"retired_at,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
}

// MethodSnapshot freezes the effective version content onto a plan.
type MethodSnapshot struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	Version          int      `json:"version"`
	Steps            []string `json:"steps"`
	AllowedAdhesives []string `json:"allowed_adhesives"`
	TargetMin        int      `json:"target_min"`
	TargetMax        int      `json:"target_max"`
}

// Snapshot copies the version content so later method revisions cannot
// rewrite plans that already referenced this edition.
func (v MethodVersion) Snapshot(code string, name string) MethodSnapshot {
	return MethodSnapshot{
		Code:             code,
		Name:             name,
		Version:          v.Version,
		Steps:            append([]string{}, v.Steps...),
		AllowedAdhesives: append([]string{}, v.AllowedAdhesives...),
		TargetMin:        v.TargetMin,
		TargetMax:        v.TargetMax,
	}
}

// MethodStandard is the aggregate for a reusable preparation method and
// all of its editions. At most one version is published at any moment.
type MethodStandard struct {
	ID          string          `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Versions    []MethodVersion `json:"versions"`
	Revision    int             `json:"revision"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func NewMethodStandard(
	id string,
	code string,
	name string,
	description string,
	draft MethodDraftInput,
	now time.Time,
) (MethodStandard, error) {
	if id == "" {
		return MethodStandard{}, contracts.Internal("missing_generated_id", "标准方法标识未生成", nil)
	}
	normalized, err := NormalizeMethodCode(code)
	if err != nil {
		return MethodStandard{}, err
	}
	nameValue, err := requiredText("方法名称", name, 120)
	if err != nil {
		return MethodStandard{}, err
	}
	descriptionValue, err := optionalText("方法说明", description, 500)
	if err != nil {
		return MethodStandard{}, err
	}
	version, err := newMethodVersion(1, draft, now)
	if err != nil {
		return MethodStandard{}, err
	}
	return MethodStandard{
		ID:          id,
		Code:        normalized,
		Name:        nameValue,
		Description: descriptionValue,
		Versions:    []MethodVersion{version},
		Revision:    1,
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
	}, nil
}

// NormalizeMethodCode converts a user supplied code into its stored form.
func NormalizeMethodCode(code string) (string, error) {
	return validateMethodCode(code)
}

// EffectiveVersion returns the currently published edition, if any.
func (m MethodStandard) EffectiveVersion() (MethodVersion, bool) {
	for _, version := range m.Versions {
		if version.Status == MethodVersionPublished {
			return version, true
		}
	}
	return MethodVersion{}, false
}

// HasDraft reports whether an unpublished draft edition is still open.
func (m MethodStandard) HasDraft() bool {
	for _, version := range m.Versions {
		if version.Status == MethodVersionDraft {
			return true
		}
	}
	return false
}

// AddDraft appends a new draft edition. Only one open draft is allowed so
// that the next publish target is unambiguous.
func (m *MethodStandard) AddDraft(input MethodDraftInput, now time.Time) (MethodVersion, error) {
	if m.HasDraft() {
		return MethodVersion{}, contracts.Conflict("method_draft_exists", "已存在未发布的方法草案，请先发布或停用")
	}
	next := 1
	for _, version := range m.Versions {
		if version.Version >= next {
			next = version.Version + 1
		}
	}
	draft, err := newMethodVersion(next, input, now)
	if err != nil {
		return MethodVersion{}, err
	}
	m.Versions = append(m.Versions, draft)
	m.Revision++
	m.UpdatedAt = now.UTC()
	return draft, nil
}

// Publish promotes a draft edition and retires the previously published
// edition inside the same state change, keeping a single effective version.
func (m *MethodStandard) Publish(version int, publishedBy string, now time.Time) error {
	actor, err := requiredText("发布人", publishedBy, 80)
	if err != nil {
		return err
	}
	index := m.indexOf(version)
	if index < 0 {
		return contracts.NotFound("method_version_not_found", "未找到对应的方法版本")
	}
	if m.Versions[index].Status != MethodVersionDraft {
		return contracts.Conflict("method_version_not_publishable", "只有草案版本可以发布，不能重复发布")
	}
	stamp := now.UTC()
	for i := range m.Versions {
		if m.Versions[i].Status == MethodVersionPublished {
			m.Versions[i].Status = MethodVersionRetired
			m.Versions[i].RetiredBy = actor
			m.Versions[i].RetiredAt = &stamp
		}
	}
	m.Versions[index].Status = MethodVersionPublished
	m.Versions[index].PublishedBy = actor
	m.Versions[index].PublishedAt = &stamp
	m.Revision++
	m.UpdatedAt = stamp
	return nil
}

// Retire withdraws the currently published edition without a replacement.
func (m *MethodStandard) Retire(version int, retiredBy string, now time.Time) error {
	actor, err := requiredText("停用人", retiredBy, 80)
	if err != nil {
		return err
	}
	index := m.indexOf(version)
	if index < 0 {
		return contracts.NotFound("method_version_not_found", "未找到对应的方法版本")
	}
	if m.Versions[index].Status != MethodVersionPublished {
		return contracts.Conflict("method_version_not_retirable", "只有已发布版本可以停用")
	}
	stamp := now.UTC()
	m.Versions[index].Status = MethodVersionRetired
	m.Versions[index].RetiredBy = actor
	m.Versions[index].RetiredAt = &stamp
	m.Revision++
	m.UpdatedAt = stamp
	return nil
}

func (m MethodStandard) indexOf(version int) int {
	for i := range m.Versions {
		if m.Versions[i].Version == version {
			return i
		}
	}
	return -1
}

func newMethodVersion(number int, input MethodDraftInput, now time.Time) (MethodVersion, error) {
	steps, adhesives, targetMin, targetMax, err := normalizeMethodDraft(input)
	if err != nil {
		return MethodVersion{}, err
	}
	return MethodVersion{
		Version:          number,
		Status:           MethodVersionDraft,
		Steps:            steps,
		AllowedAdhesives: adhesives,
		TargetMin:        targetMin,
		TargetMax:        targetMax,
		CreatedAt:        now.UTC(),
	}, nil
}

func normalizeMethodDraft(input MethodDraftInput) ([]string, []string, int, int, error) {
	if len(input.Steps) == 0 {
		return nil, nil, 0, 0, contracts.Invalid("invalid_steps", "标准方法至少需要一个操作步骤")
	}
	steps := make([]string, 0, len(input.Steps))
	for _, step := range input.Steps {
		value, err := requiredText("操作步骤", step, 160)
		if err != nil {
			return nil, nil, 0, 0, err
		}
		steps = append(steps, value)
	}
	adhesives := make([]string, 0, len(input.AllowedAdhesives))
	seen := make(map[string]struct{}, len(input.AllowedAdhesives))
	for _, adhesive := range input.AllowedAdhesives {
		value, err := requiredText("黏结材料", adhesive, 120)
		if err != nil {
			return nil, nil, 0, 0, err
		}
		if _, duplicated := seen[value]; duplicated {
			continue
		}
		seen[value] = struct{}{}
		adhesives = append(adhesives, value)
	}
	if err := validatePercent(input.TargetMin); err != nil {
		return nil, nil, 0, 0, contracts.Invalid("invalid_target_range", "目标暴露范围下限必须位于 0 到 100 之间")
	}
	if err := validatePercent(input.TargetMax); err != nil {
		return nil, nil, 0, 0, contracts.Invalid("invalid_target_range", "目标暴露范围上限必须位于 0 到 100 之间")
	}
	if input.TargetMin > input.TargetMax {
		return nil, nil, 0, 0, contracts.Invalid("invalid_target_range", "目标暴露范围下限不能超过上限")
	}
	return steps, adhesives, input.TargetMin, input.TargetMax, nil
}
