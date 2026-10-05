package domain

import (
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

type MethodVersionStatus string

const (
	MethodVersionDraft   MethodVersionStatus = "draft"
	MethodVersionActive  MethodVersionStatus = "active"
	MethodVersionRetired MethodVersionStatus = "retired"
)

// ExposureBoundary is the inclusive target exposure range enforced by a
// published standard method version.
type ExposureBoundary struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// MethodStep is one ordered operation step of a standard method version.
type MethodStep struct {
	Sequence    int    `json:"sequence"`
	Name        string `json:"name"`
	Instruction string `json:"instruction"`
}

// MethodVersion is an immutable-after-publication revision of a standard
// method. Draft content may be replaced; active and retired versions never
// change.
type MethodVersion struct {
	Number       int                 `json:"number"`
	Status       MethodVersionStatus `json:"status"`
	Steps        []MethodStep        `json:"steps"`
	Materials    []string            `json:"materials"`
	Boundary     ExposureBoundary    `json:"exposure_boundary"`
	PublishedBy  string              `json:"published_by,omitempty"`
	PublishedAt  *time.Time          `json:"published_at,omitempty"`
	RetiredBy    string              `json:"retired_by,omitempty"`
	RetiredAt    *time.Time          `json:"retired_at,omitempty"`
	SupersededBy int                 `json:"superseded_by,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// StandardMethod is the method library aggregate. At most one version is
// active at any time and CurrentVersion points to it (zero means none).
type StandardMethod struct {
	ID             string          `json:"id"`
	Code           string          `json:"code"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	CurrentVersion int             `json:"current_version"`
	Versions       []MethodVersion `json:"versions"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// FrozenMethod is the value copied into a prep plan when it references an
// effective method version. The plan owns this copy, so later method
// revisions can never rewrite an existing or completed plan.
type FrozenMethod struct {
	MethodID    string           `json:"method_id"`
	Code        string           `json:"code"`
	Name        string           `json:"name"`
	Version     int              `json:"version"`
	Steps       []MethodStep     `json:"steps"`
	Materials   []string         `json:"materials"`
	Boundary    ExposureBoundary `json:"exposure_boundary"`
	PublishedAt time.Time        `json:"published_at"`
}

const (
	maxMethodSteps     = 20
	maxMethodMaterials = 20
)

// NewStandardMethod creates the method aggregate together with its first
// draft version. Code uniqueness is enforced by the service layer.
func NewStandardMethod(
	id string,
	code string,
	name string,
	description string,
	steps []MethodStep,
	materials []string,
	boundary ExposureBoundary,
	now time.Time,
) (StandardMethod, error) {
	if id == "" {
		return StandardMethod{}, contracts.Internal("missing_generated_id", "标准方法标识未生成", nil)
	}
	methodCode, err := validateMethodCode(code)
	if err != nil {
		return StandardMethod{}, err
	}
	methodName, err := requiredText("方法名称", name, 120)
	if err != nil {
		return StandardMethod{}, err
	}
	desc, err := optionalText("方法说明", description, 500)
	if err != nil {
		return StandardMethod{}, err
	}
	content, err := validateMethodContent(steps, materials, boundary)
	if err != nil {
		return StandardMethod{}, err
	}
	stamp := now.UTC()
	return StandardMethod{
		ID:             id,
		Code:           methodCode,
		Name:           methodName,
		Description:    desc,
		CurrentVersion: 0,
		Versions: []MethodVersion{
			newDraftVersion(1, content, stamp),
		},
		CreatedAt: stamp,
		UpdatedAt: stamp,
	}, nil
}

func newDraftVersion(number int, content methodContent, now time.Time) MethodVersion {
	return MethodVersion{
		Number:    number,
		Status:    MethodVersionDraft,
		Steps:     content.Steps,
		Materials: content.Materials,
		Boundary:  content.Boundary,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

type methodContent struct {
	Steps     []MethodStep
	Materials []string
	Boundary  ExposureBoundary
}

func validateMethodContent(
	rawSteps []MethodStep,
	rawMaterials []string,
	boundary ExposureBoundary,
) (methodContent, error) {
	if len(rawSteps) == 0 {
		return methodContent{}, contracts.Invalid("method_requires_steps", "标准方法至少包含一个步骤")
	}
	if len(rawSteps) > maxMethodSteps {
		return methodContent{}, contracts.Invalid("too_many_steps", "标准方法步骤数量超出限制")
	}
	steps := make([]MethodStep, 0, len(rawSteps))
	for index, raw := range rawSteps {
		name, err := requiredText("步骤名称", raw.Name, 80)
		if err != nil {
			return methodContent{}, err
		}
		instruction, err := optionalText("步骤说明", raw.Instruction, 500)
		if err != nil {
			return methodContent{}, err
		}
		steps = append(steps, MethodStep{
			Sequence:    index + 1,
			Name:        name,
			Instruction: instruction,
		})
	}

	if len(rawMaterials) > maxMethodMaterials {
		return methodContent{}, contracts.Invalid("too_many_materials", "黏结材料范围条目数量超出限制")
	}
	materials := make([]string, 0, len(rawMaterials))
	seen := make(map[string]struct{}, len(rawMaterials))
	for _, raw := range rawMaterials {
		material, err := requiredText("黏结材料", raw, 60)
		if err != nil {
			return methodContent{}, err
		}
		if _, dup := seen[material]; dup {
			return methodContent{}, contracts.Invalid("duplicate_material", "黏结材料范围中存在重复条目")
		}
		seen[material] = struct{}{}
		materials = append(materials, material)
	}

	if boundary.Min < 1 || boundary.Min > 100 || boundary.Max < 1 || boundary.Max > 100 {
		return methodContent{}, contracts.Invalid("invalid_exposure_boundary", "目标暴露比例边界必须位于 1 到 100 之间")
	}
	if boundary.Min > boundary.Max {
		return methodContent{}, contracts.Invalid("invalid_exposure_boundary", "目标暴露比例下界不能高于上界")
	}

	return methodContent{
		Steps:     steps,
		Materials: materials,
		Boundary:  boundary,
	}, nil
}

// AddDraft appends the next draft version. Only one draft may exist at a
// time so concurrent draft creation resolves to a deterministic conflict.
func (m *StandardMethod) AddDraft(
	steps []MethodStep,
	materials []string,
	boundary ExposureBoundary,
	now time.Time,
) (MethodVersion, error) {
	for _, version := range m.Versions {
		if version.Status == MethodVersionDraft {
			return MethodVersion{}, contracts.Conflict("draft_exists", "该方法已存在草案版本，不能重复创建")
		}
	}
	content, err := validateMethodContent(steps, materials, boundary)
	if err != nil {
		return MethodVersion{}, err
	}
	stamp := now.UTC()
	version := newDraftVersion(len(m.Versions)+1, content, stamp)
	m.Versions = append(m.Versions, version)
	m.UpdatedAt = stamp
	return version, nil
}

// UpdateDraft replaces the editable content of a draft version.
func (m *StandardMethod) UpdateDraft(
	number int,
	steps []MethodStep,
	materials []string,
	boundary ExposureBoundary,
	now time.Time,
) error {
	index := m.indexOf(number)
	if index < 0 {
		return contracts.NotFound("method_version_not_found", "未找到方法版本")
	}
	if m.Versions[index].Status != MethodVersionDraft {
		return contracts.Conflict("version_not_draft", "只有草案版本可以修改")
	}
	content, err := validateMethodContent(steps, materials, boundary)
	if err != nil {
		return err
	}
	stamp := now.UTC()
	m.Versions[index].Steps = content.Steps
	m.Versions[index].Materials = content.Materials
	m.Versions[index].Boundary = content.Boundary
	m.Versions[index].UpdatedAt = stamp
	m.UpdatedAt = stamp
	return nil
}

// Publish moves a draft to active. Any previously active version is retired
// atomically within the same aggregate, so the method always has at most one
// effective version. Publishing a non-draft is a deterministic conflict,
// including retried (duplicate) publish requests.
func (m *StandardMethod) Publish(number int, publishedBy string, now time.Time) (MethodVersion, error) {
	index := m.indexOf(number)
	if index < 0 {
		return MethodVersion{}, contracts.NotFound("method_version_not_found", "未找到方法版本")
	}
	if m.Versions[index].Status != MethodVersionDraft {
		return MethodVersion{}, contracts.Conflict("version_not_draft", "只有草案版本可以发布")
	}
	publisher, err := requiredText("发布人", publishedBy, 80)
	if err != nil {
		return MethodVersion{}, err
	}
	stamp := now.UTC()
	if m.CurrentVersion > 0 {
		previous := m.indexOf(m.CurrentVersion)
		if previous >= 0 && m.Versions[previous].Status == MethodVersionActive {
			m.Versions[previous].Status = MethodVersionRetired
			m.Versions[previous].RetiredBy = publisher
			m.Versions[previous].RetiredAt = &stamp
			m.Versions[previous].SupersededBy = number
			m.Versions[previous].UpdatedAt = stamp
		}
	}
	m.Versions[index].Status = MethodVersionActive
	m.Versions[index].PublishedBy = publisher
	m.Versions[index].PublishedAt = &stamp
	m.Versions[index].UpdatedAt = stamp
	m.CurrentVersion = number
	m.UpdatedAt = stamp
	return m.Versions[index], nil
}

// Retire deactivates the active version without a replacement.
func (m *StandardMethod) Retire(number int, retiredBy string, now time.Time) error {
	index := m.indexOf(number)
	if index < 0 {
		return contracts.NotFound("method_version_not_found", "未找到方法版本")
	}
	if m.Versions[index].Status != MethodVersionActive {
		return contracts.Conflict("version_not_active", "只有有效版本可以停用")
	}
	operator, err := requiredText("停用人", retiredBy, 80)
	if err != nil {
		return err
	}
	stamp := now.UTC()
	m.Versions[index].Status = MethodVersionRetired
	m.Versions[index].RetiredBy = operator
	m.Versions[index].RetiredAt = &stamp
	m.Versions[index].UpdatedAt = stamp
	m.CurrentVersion = 0
	m.UpdatedAt = stamp
	return nil
}

func (m *StandardMethod) indexOf(number int) int {
	for index := range m.Versions {
		if m.Versions[index].Number == number {
			return index
		}
	}
	return -1
}

func (m StandardMethod) Version(number int) (MethodVersion, bool) {
	for _, version := range m.Versions {
		if version.Number == number {
			return version, true
		}
	}
	return MethodVersion{}, false
}

func (m StandardMethod) Current() (MethodVersion, bool) {
	if m.CurrentVersion == 0 {
		return MethodVersion{}, false
	}
	return m.Version(m.CurrentVersion)
}

// AsReference produces the frozen value a plan copies when referencing this
// version. Only the active (effective) version can be referenced.
func (v MethodVersion) AsReference(method StandardMethod) (FrozenMethod, error) {
	if v.Status != MethodVersionActive {
		return FrozenMethod{}, contracts.Conflict("method_version_not_effective", "只能引用当前有效方法版本")
	}
	publishedAt := time.Time{}
	if v.PublishedAt != nil {
		publishedAt = *v.PublishedAt
	}
	steps := append([]MethodStep(nil), v.Steps...)
	materials := append([]string(nil), v.Materials...)
	return FrozenMethod{
		MethodID:    method.ID,
		Code:        method.Code,
		Name:        method.Name,
		Version:     v.Number,
		Steps:       steps,
		Materials:   materials,
		Boundary:    v.Boundary,
		PublishedAt: publishedAt,
	}, nil
}
