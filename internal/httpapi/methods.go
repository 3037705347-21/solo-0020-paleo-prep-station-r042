package httpapi

import (
	"net/http"
	"strconv"

	"example.com/paleo-prep-station/internal/contracts"
)

func (api *API) createMethod(w http.ResponseWriter, r *http.Request) {
	var request contracts.CreateMethodRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.CreateMethod(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/methods/"+value.Code)
	writeJSON(w, http.StatusCreated, value)
}

func (api *API) listMethods(w http.ResponseWriter, r *http.Request) {
	values, err := api.service.ListMethods(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (api *API) getMethod(w http.ResponseWriter, r *http.Request) {
	value, err := api.service.GetMethod(r.Context(), r.PathValue("code"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (api *API) createMethodDraft(w http.ResponseWriter, r *http.Request) {
	var request contracts.MethodDraftRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.CreateMethodDraft(r.Context(), r.PathValue("code"), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (api *API) publishMethodVersion(w http.ResponseWriter, r *http.Request) {
	version, ok := methodVersionParam(w, r)
	if !ok {
		return
	}
	var request contracts.PublishMethodRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.PublishMethodVersion(r.Context(), r.PathValue("code"), version, request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (api *API) retireMethodVersion(w http.ResponseWriter, r *http.Request) {
	version, ok := methodVersionParam(w, r)
	if !ok {
		return
	}
	var request contracts.RetireMethodRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := api.service.RetireMethodVersion(r.Context(), r.PathValue("code"), version, request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func methodVersionParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || version < 1 {
		writeError(w, contracts.Invalid("invalid_method_version", "方法版本号必须是正整数"))
		return 0, false
	}
	return version, true
}
