package api

import (
	"encoding/json"
	"io"
	"net/http"

	hrv1 "github.com/aymane-belassiria/hr-platform/gen/go/hr/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Handler struct {
	employees hrv1.EmployeeServiceClient
}

func New(employees hrv1.EmployeeServiceClient) *Handler {
	return &Handler{employees: employees}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healtz)
	mux.HandleFunc("GET /api/v1/employees", h.listEmployees)
	mux.HandleFunc("POST /api/v1/employees", h.createEmployee)
	mux.HandleFunc("GET /api/v1/employees/{id}", h.getEmployee)
	return mux
}

func (h *Handler) healtz(w http.ResponseWriter, _ *http.Request) {
	writeJson(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listEmployees(w http.ResponseWriter, r *http.Request) {
	resp, err := h.employees.ListEmployees(r.Context(), &hrv1.ListEmployeesRequest{PageSize: 50})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

func (h *Handler) createEmployee(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJson(w, http.StatusBadRequest, map[string]string{"error": "cannot read body"})
		return
	}
	var req hrv1.CreateEmployeeRequest
	if err := protojson.Unmarshal(body, &req); err != nil {
		writeJson(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON " + err.Error()})
		return
	}
	resp, err := h.employees.CreateEmployee(r.Context(), &req)
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeProto(w, http.StatusCreated, resp)
}

func (h *Handler) getEmployee(w http.ResponseWriter, r *http.Request) {
	resp, err := h.employees.GetEmployee(r.Context(), &hrv1.GetEmployeeRequest{
		Id: r.PathValue("id"),
	})
	if err != nil {
		writeGRPCError(w, err)
		return
	}
	writeJson(w, http.StatusOK, resp)
}

// helpers

func writeGRPCError(w http.ResponseWriter, err error) {
	st, _ := status.FromError(err)
	code := http.StatusInternalServerError
	switch st.Code() {
	case codes.InvalidArgument:
		code = http.StatusBadRequest
	case codes.NotFound:
		code = http.StatusNotFound
	case codes.AlreadyExists:
		code = http.StatusConflict
	case codes.Unavailable, codes.DeadlineExceeded:
		code = http.StatusServiceUnavailable
	}
	writeJson(w, code, map[string]string{
		"error": st.Message(),
		"code":  st.Code().String(),
	})
}

func writeJson(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProto(w http.ResponseWriter, code int, msg proto.Message) {
	b, err := protojson.Marshal(msg)
	if err != nil {
		writeJson(w, http.StatusInternalServerError, map[string]string{"error": "encode response"})
		return
	}
	w.Header().Set("Conent-Type", "application/json")
	w.WriteHeader(code)
	_,_ = w.Write(b)
}
