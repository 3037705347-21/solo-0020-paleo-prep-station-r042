package contracts

type RegisterSpecimenRequest struct {
	CatalogNumber string `json:"catalog_number"`
	FieldLabel    string `json:"field_label"`
	TaxonHint     string `json:"taxon_hint"`
	MatrixKind    string `json:"matrix_kind"`
	JacketNotes   string `json:"jacket_notes"`
}

type CreatePlanRequest struct {
	Objective      string `json:"objective"`
	Method         string `json:"method"`
	Adhesive       string `json:"adhesive"`
	TargetExposure int    `json:"target_exposure"`
	MethodCode     string `json:"method_code"`
}

type CreateMethodRequest struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Steps            []string `json:"steps"`
	AllowedAdhesives []string `json:"allowed_adhesives"`
	TargetMin        int      `json:"target_min"`
	TargetMax        int      `json:"target_max"`
}

type MethodDraftRequest struct {
	Steps            []string `json:"steps"`
	AllowedAdhesives []string `json:"allowed_adhesives"`
	TargetMin        int      `json:"target_min"`
	TargetMax        int      `json:"target_max"`
}

type PublishMethodRequest struct {
	PublishedBy string `json:"published_by"`
}

type RetireMethodRequest struct {
	RetiredBy string `json:"retired_by"`
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
