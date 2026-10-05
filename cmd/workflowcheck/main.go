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

type planView struct {
	ID         string `json:"id"`
	SpecimenID string `json:"specimen_id"`
	Status     string `json:"status"`
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

	case "standard-method-library":
		return checkStandardMethodLibrary(client, server.URL)
	default:
		return fmt.Errorf("unknown workflow %q", workflow)
	}
}

type methodStepView struct {
	Sequence int    `json:"sequence"`
	Name     string `json:"name"`
}

type methodVersionView struct {
	Number    int              `json:"number"`
	Status    string           `json:"status"`
	Steps     []methodStepView `json:"steps"`
	Materials []string         `json:"materials"`
	Boundary  struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"exposure_boundary"`
	PublishedBy  string     `json:"published_by"`
	PublishedAt  *time.Time `json:"published_at"`
	RetiredBy    string     `json:"retired_by"`
	SupersededBy int        `json:"superseded_by"`
}

type methodView struct {
	ID             string              `json:"id"`
	Code           string              `json:"code"`
	Name           string              `json:"name"`
	CurrentVersion int                 `json:"current_version"`
	Versions       []methodVersionView `json:"versions"`
}

type planDetailView struct {
	planView
	Method         string `json:"method"`
	Adhesive       string `json:"adhesive"`
	TargetExposure int    `json:"target_exposure"`
	MethodRef      *struct {
		MethodID  string           `json:"method_id"`
		Code      string           `json:"code"`
		Name      string           `json:"name"`
		Version   int              `json:"version"`
		Steps     []methodStepView `json:"steps"`
		Materials []string         `json:"materials"`
		Boundary  struct {
			Min int `json:"min"`
			Max int `json:"max"`
		} `json:"exposure_boundary"`
	} `json:"method_ref"`
}

func validMethodBody() map[string]any {
	return map[string]any{
		"steps": []map[string]any{
			{"name": "初检记录", "instruction": "拍照并标记裂缝"},
			{"name": "显微剥离", "instruction": "逐层去除基质"},
			{"name": "渗透加固", "instruction": "低浓度可逆树脂渗透"},
		},
		"materials":         []string{"可逆树脂", " Paraloid B-72"},
		"exposure_boundary": map[string]any{"min": 40, "max": 90},
	}
}

func checkStandardMethodLibrary(client *http.Client, baseURL string) error {
	// 非法边界：下界高于上界，必须有确定的 400 结果。
	if _, err := requestExpectStatus(client, http.MethodPost, baseURL+"/methods",
		map[string]any{
			"code": "PPS-INVALID-01",
			"name": "非法边界方法",
			"steps": []map[string]any{
				{"name": "初检", "instruction": "检查"},
			},
			"materials":         []string{"可逆树脂"},
			"exposure_boundary": map[string]any{"min": 90, "max": 40},
		}, http.StatusBadRequest); err != nil {
		return err
	}
	// 重复材料同样拒绝。
	if _, err := requestExpectStatus(client, http.MethodPost, baseURL+"/methods",
		map[string]any{
			"code": "PPS-INVALID-02",
			"name": "重复材料方法",
			"steps": []map[string]any{
				{"name": "初检", "instruction": "检查"},
			},
			"materials":         []string{"可逆树脂", "可逆树脂"},
			"exposure_boundary": map[string]any{"min": 40, "max": 90},
		}, http.StatusBadRequest); err != nil {
		return err
	}

	// 创建方法与首个草案，并发布为第 1 个有效版本。
	var method methodView
	if err := requestJSON(client, http.MethodPost, baseURL+"/methods",
		mergeMaps(validMethodBody(), map[string]any{
			"code":        "MCR-PREP-01",
			"name":        "标准显微剥离法",
			"description": "适用于粉砂岩石板",
		}), http.StatusCreated, &method); err != nil {
		return err
	}
	if method.Code != "MCR-PREP-01" || method.CurrentVersion != 0 || len(method.Versions) != 1 {
		return errors.New("new method should contain exactly one draft and no effective version")
	}
	// 重复方法编码 -> 409。
	if _, err := requestExpectStatus(client, http.MethodPost, baseURL+"/methods",
		mergeMaps(validMethodBody(), map[string]any{"code": "mcr-prep-01", "name": "编码冲突方法"}),
		http.StatusConflict); err != nil {
		return err
	}
	if err := requestJSON(client, http.MethodPost,
		baseURL+"/methods/"+method.ID+"/versions/1/publish",
		contracts.PublishMethodRequest{PublishedBy: "lead-he"},
		http.StatusOK, &map[string]any{}); err != nil {
		return err
	}
	// 重复发布同一草案（已是有效版本）-> 409。
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/methods/"+method.ID+"/versions/1/publish",
		contracts.PublishMethodRequest{PublishedBy: "lead-he"},
		http.StatusConflict); err != nil {
		return err
	}
	// 已发布版本内容不可修改。
	if _, err := requestExpectStatus(client, http.MethodPut,
		baseURL+"/methods/"+method.ID+"/versions/1/draft",
		validMethodBody(), http.StatusConflict); err != nil {
		return err
	}

	// 引用当前有效版本创建方案：步骤、材料范围、边界被冻结，方法名/材料按引用填充。
	firstSpecimen, err := registerSpecimenC(client, baseURL, "PPS-MCR-001")
	if err != nil {
		return err
	}
	var firstPlan planDetailView
	if err := requestJSON(client, http.MethodPost,
		baseURL+"/specimens/"+firstSpecimen.ID+"/plans",
		map[string]any{
			"objective":       "暴露主要关节面并保留围岩参照",
			"target_exposure": 60,
			"method_ref":      map[string]any{"method_id": method.ID},
		}, http.StatusCreated, &firstPlan); err != nil {
		return err
	}
	if firstPlan.MethodRef == nil || firstPlan.MethodRef.Version != 1 {
		return errors.New("plan should freeze the referenced effective version")
	}
	if len(firstPlan.MethodRef.Steps) != 3 || firstPlan.Method != "标准显微剥离法" {
		return errors.New("plan should snapshot steps and fill the method name")
	}
	if firstPlan.MethodRef.Boundary.Min != 40 || firstPlan.MethodRef.Boundary.Max != 90 {
		return errors.New("plan should snapshot the exposure boundary")
	}

	// 引用边界外的目标比例 -> 400；材料范围外的黏结材料 -> 400。
	badBoundarySpecimen, err := registerSpecimenC(client, baseURL, "PPS-MCR-002")
	if err != nil {
		return err
	}
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/specimens/"+badBoundarySpecimen.ID+"/plans",
		map[string]any{
			"objective":       "尝试越界目标",
			"target_exposure": 95,
			"method_ref":      map[string]any{"method_id": method.ID},
		}, http.StatusBadRequest); err != nil {
		return err
	}
	badMaterialSpecimen, err := registerSpecimenC(client, baseURL, "PPS-MCR-003")
	if err != nil {
		return err
	}
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/specimens/"+badMaterialSpecimen.ID+"/plans",
		map[string]any{
			"objective":       "尝试越界材料",
			"target_exposure": 50,
			"adhesive":        "氰基丙烯酸酯",
			"method_ref":      map[string]any{"method_id": method.ID},
		}, http.StatusBadRequest); err != nil {
		return err
	}

	// 停用旧版本前先准备第 2 版草案，并验证并发重复发布只有一次成功。
	var version2 methodVersionView
	if err := requestJSON(client, http.MethodPost, baseURL+"/methods/"+method.ID+"/versions",
		map[string]any{
			"steps": []map[string]any{
				{"name": "CT 预扫", "instruction": "先完成断层扫描"},
				{"name": "气动剥离", "instruction": "使用气动刻磨机"},
			},
			"materials":         []string{"B-72 加固剂"},
			"exposure_boundary": map[string]any{"min": 30, "max": 95},
		}, http.StatusCreated, &version2); err != nil {
		return err
	}
	if version2.Number != 2 {
		return fmt.Errorf("expected new draft version 2, got %d", version2.Number)
	}
	// 已有草案时重复创建草案 -> 409。
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/methods/"+method.ID+"/versions",
		validMethodBody(), http.StatusConflict); err != nil {
		return err
	}
	results := concurrentPublish(client, baseURL+"/methods/"+method.ID+"/versions/2/publish")
	if results[0] != 1 || results[1] != 1 {
		return fmt.Errorf("concurrent publish must yield one success and one conflict, got %d ok / %d conflict", results[0], results[1])
	}
	var updatedMethod methodView
	if err := requestJSON(client, http.MethodGet, baseURL+"/methods/"+method.ID, nil, http.StatusOK, &updatedMethod); err != nil {
		return err
	}
	if updatedMethod.CurrentVersion != 2 {
		return errors.New("publishing v2 must make v2 the effective version")
	}
	var v1, v2 methodVersionView
	for _, version := range updatedMethod.Versions {
		switch version.Number {
		case 1:
			v1 = version
		case 2:
			v2 = version
		}
	}
	if v1.Status != "retired" || v1.SupersededBy != 2 || v1.RetiredBy == "" {
		return errors.New("publishing v2 must atomically retire v1 with supersede trace")
	}
	if v2.Status != "active" || v2.PublishedBy != "lead-he" {
		return errors.New("v2 must be active with publisher recorded")
	}

	// 改版不能反向改写已创建方案：第一个方案仍固定在第 1 版内容上。
	var frozenPlan planSnapshotWithMethod
	if err := requestJSON(client, http.MethodGet, baseURL+"/plans/"+firstPlan.ID, nil, http.StatusOK, &frozenPlan); err != nil {
		return err
	}
	if frozenPlan.Plan.MethodRef == nil || frozenPlan.Plan.MethodRef.Version != 1 ||
		len(frozenPlan.Plan.MethodRef.Steps) != 3 || frozenPlan.Plan.MethodRef.Boundary.Max != 90 {
		return errors.New("existing plan must remain frozen on version 1 after method revision")
	}

	// 旧版本已停用，显式引用旧版本创建方案 -> 409。
	retiredRefSpecimen, err := registerSpecimenC(client, baseURL, "PPS-MCR-004")
	if err != nil {
		return err
	}
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/specimens/"+retiredRefSpecimen.ID+"/plans",
		map[string]any{
			"objective":       "尝试引用旧版本",
			"target_exposure": 50,
			"method_ref":      map[string]any{"method_id": method.ID, "version": 1},
		}, http.StatusConflict); err != nil {
		return err
	}
	// 引用未知方法 -> 404；未知版本 -> 404。
	missingSpecimen, err := registerSpecimenC(client, baseURL, "PPS-MCR-005")
	if err != nil {
		return err
	}
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/specimens/"+missingSpecimen.ID+"/plans",
		map[string]any{
			"objective":       "引用不存在的方法",
			"target_exposure": 50,
			"method_ref":      map[string]any{"method_id": "method-nope"},
		}, http.StatusNotFound); err != nil {
		return err
	}
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/specimens/"+missingSpecimen.ID+"/plans",
		map[string]any{
			"objective":       "引用不存在的版本",
			"target_exposure": 50,
			"method_ref":      map[string]any{"method_id": method.ID, "version": 99},
		}, http.StatusNotFound); err != nil {
		return err
	}

	// 并发引用当前有效版本：两个不同标本的事务必须串行化，两者都成功。
	specA, err := registerSpecimenC(client, baseURL, "PPS-MCR-006")
	if err != nil {
		return err
	}
	specB, err := registerSpecimenC(client, baseURL, "PPS-MCR-007")
	if err != nil {
		return err
	}
	created, conflicted, err := concurrentPlanReferences(client, baseURL, method.ID, specA.ID, specB.ID)
	if err != nil {
		return err
	}
	if created != 2 || conflicted != 0 {
		return fmt.Errorf("concurrent effective-version references must both succeed, got %d created / %d conflicted", created, conflicted)
	}

	// 显式停用第 2 版后没有有效版本，再引用当前版本 -> 409。
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/methods/"+method.ID+"/versions/2/retire",
		contracts.RetireMethodRequest{RetiredBy: "lead-he"}, http.StatusOK); err != nil {
		return err
	}
	noEffectiveSpecimen, err := registerSpecimenC(client, baseURL, "PPS-MCR-008")
	if err != nil {
		return err
	}
	if _, err := requestExpectStatus(client, http.MethodPost,
		baseURL+"/specimens/"+noEffectiveSpecimen.ID+"/plans",
		map[string]any{
			"objective":       "无有效版本时引用",
			"target_exposure": 50,
			"method_ref":      map[string]any{"method_id": method.ID},
		}, http.StatusConflict); err != nil {
		return err
	}

	// 完成第一个方案后再次检查：方法库的任何操作都不能回写已完成方案。
	if err := requestJSON(client, http.MethodPost,
		baseURL+"/plans/"+firstPlan.ID+"/approval",
		contracts.ApprovalRequest{ApprovedBy: "curator-lin"},
		http.StatusOK, &map[string]any{}); err != nil {
		return err
	}
	if err := recordProgress(client, baseURL, firstPlan.ID, 100, "表面加固"); err != nil {
		return err
	}
	var completed planSnapshotWithMethod
	if err := requestJSON(client, http.MethodGet, baseURL+"/plans/"+firstPlan.ID, nil, http.StatusOK, &completed); err != nil {
		return err
	}
	if completed.Plan.Status != "completed" || completed.Plan.MethodRef == nil ||
		completed.Plan.MethodRef.Version != 1 || len(completed.Plan.MethodRef.Steps) != 3 {
		return errors.New("completed plan must keep its frozen method snapshot forever")
	}
	return nil
}

type planSnapshotWithMethod struct {
	Plan planDetailView `json:"plan"`
}

func mergeMaps(base map[string]any, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func registerSpecimenC(client *http.Client, baseURL, catalog string) (specimenView, error) {
	var specimen specimenView
	err := requestJSON(client, http.MethodPost, baseURL+"/specimens",
		contracts.RegisterSpecimenRequest{
			CatalogNumber: catalog,
			FieldLabel:    "北坡-07",
			MatrixKind:    "粉砂岩",
		}, http.StatusCreated, &specimen)
	return specimen, err
}

func requestExpectStatus(
	client *http.Client,
	method string,
	url string,
	body any,
	wantStatus int,
) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != wantStatus {
		return nil, fmt.Errorf("request %s %s returned %d (want %d): %s", method, url, response.StatusCode, wantStatus, string(raw))
	}
	return raw, nil
}

func concurrentPublish(client *http.Client, url string) [2]int {
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicted := 0, 0
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			encoded, _ := json.Marshal(contracts.PublishMethodRequest{PublishedBy: "lead-he"})
			request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
			if err != nil {
				return
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := client.Do(request)
			if err != nil {
				return
			}
			defer response.Body.Close()
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			mu.Lock()
			defer mu.Unlock()
			switch response.StatusCode {
			case http.StatusOK:
				ok++
			case http.StatusConflict:
				conflicted++
			}
		}()
	}
	wg.Wait()
	return [2]int{ok, conflicted}
}

func concurrentPlanReferences(client *http.Client, baseURL, methodID, specA, specB string) (int, int, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, conflicted := 0, 0
	var firstErr error
	for _, specimenID := range []string{specA, specB} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := requestExpectStatus(client, http.MethodPost,
				baseURL+"/specimens/"+id+"/plans",
				map[string]any{
					"objective":       "并发引用有效版本",
					"target_exposure": 45,
					"method_ref":      map[string]any{"method_id": methodID},
				}, http.StatusCreated)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				conflicted++
				return
			}
			created++
		}(specimenID)
	}
	wg.Wait()
	return created, conflicted, firstErr
}

func registerSpecimen(client *http.Client, baseURL string) (specimenView, error) {
	var specimen specimenView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/specimens",
		contracts.RegisterSpecimenRequest{
			CatalogNumber: "PPS-2026-001",
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
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != wantStatus {
		return fmt.Errorf("request %s %s returned %d: %s", method, url, response.StatusCode, string(raw))
	}
	if target == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response for %s %s: %w", method, url, err)
	}
	return nil
}
