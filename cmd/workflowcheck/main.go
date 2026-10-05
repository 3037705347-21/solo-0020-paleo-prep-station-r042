package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"example.com/paleo-prep-station/internal/contracts"
	"example.com/paleo-prep-station/internal/httpapi"
	"example.com/paleo-prep-station/internal/repository"
	"example.com/paleo-prep-station/internal/station"
)

type specimenView struct {
	ID            string `json:"id"`
	CatalogNumber string `json:"catalog_number"`
	Status        string `json:"status"`
}

type standardMethodView struct {
	Code    string   `json:"code"`
	Version int      `json:"version"`
	Steps   []string `json:"steps"`
}

type planView struct {
	ID             string              `json:"id"`
	SpecimenID     string              `json:"specimen_id"`
	Status         string              `json:"status"`
	StandardMethod *standardMethodView `json:"standard_method"`
}

type planSnapshot struct {
	Plan     planView `json:"plan"`
	Progress []struct {
		Percent int `json:"percent"`
	} `json:"progress"`
}

type releaseView struct {
	ID         string `json:"id"`
	SpecimenID string `json:"specimen_id"`
}

type methodVersionView struct {
	Version int      `json:"version"`
	Status  string   `json:"status"`
	Steps   []string `json:"steps"`
}

type methodView struct {
	ID       string              `json:"id"`
	Code     string              `json:"code"`
	Versions []methodVersionView `json:"versions"`
}

func main() {
	workflow := flag.String("workflow", "", "workflow id to check")
	flag.Parse()

	if err := run(*workflow); err != nil {
		fmt.Fprintln(os.Stderr, "workflow check failed:", err)
		os.Exit(1)
	}
	fmt.Printf("workflow check passed: %s\n", *workflow)
}

func run(workflow string) error {
	handler := httpapi.NewRouter(station.New(repository.NewMemory()))
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	switch workflow {
	case "register-specimen":
		specimen, err := registerSpecimen(client, server.URL)
		if err != nil {
			return err
		}
		if specimen.Status != "registered" {
			return fmt.Errorf("expected registered status, got %s", specimen.Status)
		}
		var fetched specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetched); err != nil {
			return err
		}
		if fetched.ID != specimen.ID {
			return errors.New("registered specimen cannot be read back")
		}
		return nil

	case "approve-prep-plan":
		specimen, err := registerSpecimen(client, server.URL)
		if err != nil {
			return err
		}
		plan, err := createPlan(client, server.URL, specimen.ID)
		if err != nil {
			return err
		}
		var approved planView
		if err := requestJSON(
			client,
			http.MethodPost,
			server.URL+"/plans/"+plan.ID+"/approval",
			contracts.ApprovalRequest{ApprovedBy: "curator-lin"},
			http.StatusOK,
			&approved,
		); err != nil {
			return err
		}
		if approved.Status != "approved" {
			return fmt.Errorf("expected approved plan, got %s", approved.Status)
		}
		var fetchedSpecimen specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetchedSpecimen); err != nil {
			return err
		}
		if fetchedSpecimen.Status != "preparing" {
			return fmt.Errorf("expected preparing specimen, got %s", fetchedSpecimen.Status)
		}
		return nil

	case "log-preparation":
		specimen, plan, err := completedPlanSetup(client, server.URL)
		if err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 35, "矩阵剥离"); err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 100, "表面加固"); err != nil {
			return err
		}
		var snapshot planSnapshot
		if err := requestJSON(client, http.MethodGet, server.URL+"/plans/"+plan.ID, nil, http.StatusOK, &snapshot); err != nil {
			return err
		}
		if snapshot.Plan.Status != "completed" || len(snapshot.Progress) != 2 {
			return errors.New("plan did not complete with exactly two progress entries")
		}
		var fetchedSpecimen specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetchedSpecimen); err != nil {
			return err
		}
		if fetchedSpecimen.Status != "stabilized" {
			return fmt.Errorf("expected stabilized specimen, got %s", fetchedSpecimen.Status)
		}
		return nil

	case "release-for-study":
		specimen, plan, err := completedPlanSetup(client, server.URL)
		if err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 100, "表面加固"); err != nil {
			return err
		}
		var release releaseView
		if err := requestJSON(
			client,
			http.MethodPost,
			server.URL+"/specimens/"+specimen.ID+"/release",
			contracts.ReleaseRequest{
				Purpose:    "比较解剖研究",
				Recipient:  "地层研究组",
				ApprovedBy: "keeper-zhou",
			},
			http.StatusCreated,
			&release,
		); err != nil {
			return err
		}
		if release.SpecimenID != specimen.ID {
			return errors.New("release record references the wrong specimen")
		}
		var fetched specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetched); err != nil {
			return err
		}
		if fetched.Status != "available" {
			return fmt.Errorf("expected available specimen, got %s", fetched.Status)
		}
		return nil
	case "reference-standard-method":
		return checkReferenceStandardMethod(client, server.URL)

	case "govern-method-versions":
		return checkGovernMethodVersions(client, server.URL)

	default:
		return fmt.Errorf("unknown workflow %q", workflow)
	}
}

func registerSpecimen(client *http.Client, baseURL string) (specimenView, error) {
	return registerSpecimenWithCatalog(client, baseURL, "PPS-2026-001")
}

func registerSpecimenWithCatalog(client *http.Client, baseURL, catalog string) (specimenView, error) {
	var specimen specimenView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/specimens",
		contracts.RegisterSpecimenRequest{
			CatalogNumber: catalog,
			FieldLabel:    "北坡-07",
			TaxonHint:     "大型脊椎动物",
			MatrixKind:    "粉砂岩",
			JacketNotes:   "夹克边角完好",
		},
		http.StatusCreated,
		&specimen,
	)
	return specimen, err
}

func createPlan(client *http.Client, baseURL, specimenID string) (planView, error) {
	var plan planView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/specimens/"+specimenID+"/plans",
		contracts.CreatePlanRequest{
			Objective:      "暴露主要关节面",
			Method:         "手工显微剥离",
			Adhesive:       "可逆树脂",
			TargetExposure: 80,
		},
		http.StatusCreated,
		&plan,
	)
	return plan, err
}

func completedPlanSetup(client *http.Client, baseURL string) (specimenView, planView, error) {
	specimen, err := registerSpecimen(client, baseURL)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	plan, err := createPlan(client, baseURL, specimen.ID)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	var approved planView
	if err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+plan.ID+"/approval",
		contracts.ApprovalRequest{ApprovedBy: "curator-lin"},
		http.StatusOK,
		&approved,
	); err != nil {
		return specimenView{}, planView{}, err
	}
	return specimen, approved, nil
}

func recordProgress(client *http.Client, baseURL, planID string, percent int, stage string) error {
	var ignored struct{}
	return requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+planID+"/progress",
		contracts.ProgressRequest{
			Stage:      stage,
			Percent:    percent,
			Note:       "本地链路检查",
			RecordedBy: "preparator-chen",
		},
		http.StatusCreated,
		&ignored,
	)
}

func requestJSON(
	client *http.Client,
	method string,
	url string,
	body any,
	wantStatus int,
	target any,
) error {
	raw, status, err := requestRaw(client, method, url, body)
	if err != nil {
		return err
	}
	if status != wantStatus {
		return fmt.Errorf("request %s %s returned %d: %s", method, url, status, string(raw))
	}
	if target == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response for %s %s: %w", method, url, err)
	}
	return nil
}

func requestRaw(client *http.Client, method string, url string, body any) ([]byte, int, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, 0, err
	}
	return raw, response.StatusCode, nil
}

// checkReferenceStandardMethod verifies that a plan can pin the effective
// edition of a standard method and that later revisions never rewrite it.
func checkReferenceStandardMethod(client *http.Client, baseURL string) error {
	method, err := createStandardMethod(client, baseURL, "MECH-PREP")
	if err != nil {
		return err
	}
	if len(method.Versions) != 1 || method.Versions[0].Status != "draft" {
		return errors.New("new standard method must start with one draft version")
	}

	specimen, err := registerSpecimenWithCatalog(client, baseURL, "PPS-2026-101")
	if err != nil {
		return err
	}
	if _, err := createPlanExpecting(
		client,
		baseURL,
		specimen.ID,
		contracts.CreatePlanRequest{
			Objective:      "暴露主要关节面",
			TargetExposure: 60,
			MethodCode:     "MECH-PREP",
		},
		http.StatusConflict,
	); err != nil {
		return fmt.Errorf("referencing an unpublished method must conflict: %w", err)
	}

	published, err := publishMethod(client, baseURL, "MECH-PREP", 1, http.StatusOK)
	if err != nil {
		return err
	}
	if published.Versions[0].Status != "published" {
		return fmt.Errorf("expected published version, got %s", published.Versions[0].Status)
	}

	plan, err := createPlanExpecting(
		client,
		baseURL,
		specimen.ID,
		contracts.CreatePlanRequest{
			Objective:      "暴露主要关节面",
			Adhesive:       "可逆树脂",
			TargetExposure: 60,
			MethodCode:     "MECH-PREP",
		},
		http.StatusCreated,
	)
	if err != nil {
		return err
	}
	if plan.StandardMethod == nil || plan.StandardMethod.Version != 1 {
		return errors.New("plan must pin version 1 of the referenced standard method")
	}
	if len(plan.StandardMethod.Steps) != 2 || plan.StandardMethod.Steps[0] != "标记暴露基准线" {
		return errors.New("plan must freeze the steps of the referenced edition")
	}

	outOfRange, err := registerSpecimenWithCatalog(client, baseURL, "PPS-2026-102")
	if err != nil {
		return err
	}
	if _, err := createPlanExpecting(
		client,
		baseURL,
		outOfRange.ID,
		contracts.CreatePlanRequest{
			Objective:      "暴露主要关节面",
			TargetExposure: 95,
			MethodCode:     "MECH-PREP",
		},
		http.StatusBadRequest,
	); err != nil {
		return fmt.Errorf("target exposure outside the pinned bounds must be rejected: %w", err)
	}

	wrongAdhesive, err := registerSpecimenWithCatalog(client, baseURL, "PPS-2026-103")
	if err != nil {
		return err
	}
	if _, err := createPlanExpecting(
		client,
		baseURL,
		wrongAdhesive.ID,
		contracts.CreatePlanRequest{
			Objective:      "暴露主要关节面",
			Adhesive:       "快干环氧",
			TargetExposure: 60,
			MethodCode:     "MECH-PREP",
		},
		http.StatusBadRequest,
	); err != nil {
		return fmt.Errorf("adhesive outside the pinned material range must be rejected: %w", err)
	}

	if _, err := draftMethodVersion(client, baseURL, "MECH-PREP", []string{"整体加固后翻模"}, 30, 70); err != nil {
		return err
	}
	if _, err := publishMethod(client, baseURL, "MECH-PREP", 2, http.StatusOK); err != nil {
		return err
	}

	var snapshot planSnapshot
	if err := requestJSON(client, http.MethodGet, baseURL+"/plans/"+plan.ID, nil, http.StatusOK, &snapshot); err != nil {
		return err
	}
	if snapshot.Plan.StandardMethod == nil || snapshot.Plan.StandardMethod.Version != 1 {
		return errors.New("method revision must not rewrite an existing plan")
	}
	if len(snapshot.Plan.StandardMethod.Steps) != 2 {
		return errors.New("method revision must not replace the frozen steps of an existing plan")
	}
	return nil
}

// checkGovernMethodVersions verifies lifecycle conflicts and deterministic
// outcomes for duplicate publishes and concurrent references.
func checkGovernMethodVersions(client *http.Client, baseURL string) error {
	if _, err := createStandardMethod(client, baseURL, "GOV-METHOD"); err != nil {
		return err
	}
	var duplicated methodView
	if err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/methods",
		contracts.CreateMethodRequest{
			Code:      "GOV-METHOD",
			Name:      "重复编码方法",
			Steps:     []string{"清修"},
			TargetMin: 10,
			TargetMax: 90,
		},
		http.StatusConflict,
		&duplicated,
	); err != nil {
		return fmt.Errorf("duplicate method code must conflict: %w", err)
	}

	if _, err := publishMethod(client, baseURL, "GOV-METHOD", 1, http.StatusOK); err != nil {
		return err
	}
	if _, err := publishMethod(client, baseURL, "GOV-METHOD", 1, http.StatusConflict); err != nil {
		return fmt.Errorf("duplicate publish must conflict deterministically: %w", err)
	}

	if _, err := draftMethodVersion(client, baseURL, "GOV-METHOD", []string{"二次清修"}, 20, 80); err != nil {
		return err
	}
	var secondDraft methodView
	if err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/methods/GOV-METHOD/versions",
		contracts.MethodDraftRequest{Steps: []string{"三次清修"}, TargetMin: 20, TargetMax: 80},
		http.StatusConflict,
		&secondDraft,
	); err != nil {
		return fmt.Errorf("a second open draft must conflict: %w", err)
	}

	statuses := runConcurrent(2, func(_ int) (int, error) {
		_, status, err := requestRaw(
			client,
			http.MethodPost,
			baseURL+"/methods/GOV-METHOD/versions/2/publish",
			contracts.PublishMethodRequest{PublishedBy: "curator-lin"},
		)
		return status, err
	})
	okCount, conflictCount := countStatuses(statuses, http.StatusOK), countStatuses(statuses, http.StatusConflict)
	if okCount != 1 || conflictCount != 1 {
		return fmt.Errorf("concurrent duplicate publish must yield exactly one success, got %v", statuses)
	}

	method, err := getMethod(client, baseURL, "GOV-METHOD")
	if err != nil {
		return err
	}
	if method.Versions[0].Status != "retired" || method.Versions[1].Status != "published" {
		return fmt.Errorf("publishing a new edition must retire the previous one, got %+v", method.Versions)
	}

	if _, err := retireMethod(client, baseURL, "GOV-METHOD", 2, http.StatusOK); err != nil {
		return err
	}
	if _, err := retireMethod(client, baseURL, "GOV-METHOD", 2, http.StatusConflict); err != nil {
		return fmt.Errorf("duplicate retire must conflict deterministically: %w", err)
	}

	retiredSpecimen, err := registerSpecimenWithCatalog(client, baseURL, "PPS-2026-201")
	if err != nil {
		return err
	}
	if _, err := createPlanExpecting(
		client,
		baseURL,
		retiredSpecimen.ID,
		contracts.CreatePlanRequest{
			Objective:      "暴露主要关节面",
			TargetExposure: 50,
			MethodCode:     "GOV-METHOD",
		},
		http.StatusConflict,
	); err != nil {
		return fmt.Errorf("referencing a method without an effective version must conflict: %w", err)
	}

	if _, err := createStandardMethod(client, baseURL, "CONC-METHOD"); err != nil {
		return err
	}
	if _, err := publishMethod(client, baseURL, "CONC-METHOD", 1, http.StatusOK); err != nil {
		return err
	}
	const concurrentPlans = 6
	specimens := make([]specimenView, 0, concurrentPlans)
	for i := 0; i < concurrentPlans; i++ {
		specimen, err := registerSpecimenWithCatalog(client, baseURL, fmt.Sprintf("PPS-2026-3%02d", i))
		if err != nil {
			return err
		}
		specimens = append(specimens, specimen)
	}
	planStatuses := runConcurrent(concurrentPlans, func(i int) (int, error) {
		_, status, err := requestRaw(
			client,
			http.MethodPost,
			baseURL+"/specimens/"+specimens[i].ID+"/plans",
			contracts.CreatePlanRequest{
				Objective:      "暴露主要关节面",
				TargetExposure: 60,
				MethodCode:     "CONC-METHOD",
			},
		)
		return status, err
	})
	if countStatuses(planStatuses, http.StatusCreated) != concurrentPlans {
		return fmt.Errorf("concurrent references must all succeed, got %v", planStatuses)
	}
	var listed struct {
		Items []planView `json:"items"`
	}
	if err := requestJSON(client, http.MethodGet, baseURL+"/specimens/"+specimens[0].ID+"/plans", nil, http.StatusOK, &listed); err != nil {
		return err
	}
	if len(listed.Items) != 1 || listed.Items[0].StandardMethod == nil || listed.Items[0].StandardMethod.Version != 1 {
		return errors.New("concurrent references must pin the same effective version")
	}
	return nil
}

func createStandardMethod(client *http.Client, baseURL, code string) (methodView, error) {
	var method methodView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/methods",
		contracts.CreateMethodRequest{
			Code:             code,
			Name:             "机械清修标准",
			Description:      "常规基质机械清修流程",
			Steps:            []string{"标记暴露基准线", "气动笔逐层剥离"},
			AllowedAdhesives: []string{"可逆树脂", "骨胶"},
			TargetMin:        40,
			TargetMax:        80,
		},
		http.StatusCreated,
		&method,
	)
	return method, err
}

func getMethod(client *http.Client, baseURL, code string) (methodView, error) {
	var method methodView
	err := requestJSON(client, http.MethodGet, baseURL+"/methods/"+code, nil, http.StatusOK, &method)
	return method, err
}

func draftMethodVersion(
	client *http.Client,
	baseURL string,
	code string,
	steps []string,
	targetMin int,
	targetMax int,
) (methodView, error) {
	var method methodView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/methods/"+code+"/versions",
		contracts.MethodDraftRequest{
			Steps:            steps,
			AllowedAdhesives: []string{"可逆树脂"},
			TargetMin:        targetMin,
			TargetMax:        targetMax,
		},
		http.StatusCreated,
		&method,
	)
	return method, err
}

func publishMethod(
	client *http.Client,
	baseURL string,
	code string,
	version int,
	wantStatus int,
) (methodView, error) {
	var method methodView
	err := requestJSON(
		client,
		http.MethodPost,
		fmt.Sprintf("%s/methods/%s/versions/%d/publish", baseURL, code, version),
		contracts.PublishMethodRequest{PublishedBy: "curator-lin"},
		wantStatus,
		&method,
	)
	return method, err
}

func retireMethod(
	client *http.Client,
	baseURL string,
	code string,
	version int,
	wantStatus int,
) (methodView, error) {
	var method methodView
	err := requestJSON(
		client,
		http.MethodPost,
		fmt.Sprintf("%s/methods/%s/versions/%d/retire", baseURL, code, version),
		contracts.RetireMethodRequest{RetiredBy: "curator-lin"},
		wantStatus,
		&method,
	)
	return method, err
}

func createPlanExpecting(
	client *http.Client,
	baseURL string,
	specimenID string,
	payload contracts.CreatePlanRequest,
	wantStatus int,
) (planView, error) {
	var plan planView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/specimens/"+specimenID+"/plans",
		payload,
		wantStatus,
		&plan,
	)
	return plan, err
}

func runConcurrent(count int, fn func(i int) (int, error)) []int {
	statuses := make([]int, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			status, err := fn(index)
			statuses[index] = status
			errs[index] = err
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			statuses = append(statuses, -1)
		}
	}
	return statuses
}

func countStatuses(statuses []int, want int) int {
	count := 0
	for _, status := range statuses {
		if status == want {
			count++
		}
	}
	return count
}
