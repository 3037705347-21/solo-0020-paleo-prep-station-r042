package contracts

type RegisterSpecimenRequest struct {
	CatalogNumber string `json:"catalog_number"`
	FieldLabel    string `json:"field_label"`
	TaxonHint     string `json:"taxon_hint"`
	MatrixKind    string `json:"matrix_kind"`
	JacketNotes   string `json:"jacket_notes"`
}

// MethodStepInput is one ordered step submitted for a method draft.
type MethodStepInput struct {
	Name        string `json:"name"`
	Instruction string `json:"instruction"`
}

// MethodDraftBody is the editable content shared by method creation, new
// draft creation and draft revision requests.
type MethodDraftBody struct {
	Steps         []MethodStepInput `json:"steps"`
	Materials     []string          `json:"materials"`
	ExposureBound BoundaryInput     `json:"exposure_boundary"`
}

type BoundaryInput struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type CreateMethodRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MethodDraftBody
}

type NewMethodVersionRequest struct {
	MethodDraftBody
}

type PublishMethodRequest struct {
	PublishedBy string `json:"published_by"`
}

type RetireMethodRequest struct {
	RetiredBy string `json:"retired_by"`
}

// MethodReference selects the standard method version a plan is based on.
// When Version is zero the current effective version is used; otherwise the
// given version must be the effective one (otherwise conflict).
type MethodReference struct {
	MethodID string `json:"method_id"`
	Version  int    `json:"version"`
}

type CreatePlanRequest struct {
	Objective      string           `json:"objective"`
	Method         string           `json:"method"`
	Adhesive       string           `json:"adhesive"`
	TargetExposure int              `json:"target_exposure"`
	MethodRef      *MethodReference `json:"method_ref"`
}

type ApprovalRequest struct {
	ApprovedBy string `json:"approved_by"`
}

type ProgressRequest struct {
	Stage      string `json:"stage"`
	Percent    int    `json:"percent"`
	Note       string `json:"note"`
	RecordedBy string `json:"recorded_by"`
}

type ReleaseRequest struct {
	Purpose    string `json:"purpose"`
	Recipient  string `json:"recipient"`
	ApprovedBy string `json:"approved_by"`
}
