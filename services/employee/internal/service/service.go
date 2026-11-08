package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	hrv1 "github.com/aymane-belassiria/hr-platform/gen/go/hr/v1"
	db "github.com/aymane-belassiria/hr-platform/services/employee/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	hrv1.UnimplementedEmployeeServiceServer
	q *db.Queries
}

func New(q *db.Queries) *Service {
	return &Service{q: q}
}

func (s *Service) CreateEmployee(ctx context.Context, req *hrv1.CreateEmployeeRequest) (*hrv1.CreateEmployeeResponse, error) {
	if strings.TrimSpace(req.GetFirstName()) == "" ||
		strings.TrimSpace(req.GetLastName()) == "" ||
		strings.TrimSpace(req.GetEmail()) == "" {
		return nil, status.Error(codes.InvalidArgument, "first name , last name and email are required")
	}
	deptID, err := uuidOrNUll(req.GetDepartmentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid department id")
	}

	emp, err := s.q.CreateEmployee(ctx, db.CreateEmployeeParams{
		EmployeeCode:   newEmployeeCode(),
		FirstName:      req.GetFirstName(),
		LastName:       req.GetLastName(),
		Email:          req.GetEmail(),
		DepartmentID:   deptID,
		Position:       req.GetPosition(),
		EmploymentType: defaultEmploymentType(req.GetEmploymentType()),
		Status:         "active",
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create employee %v", err)
	}
	return &hrv1.CreateEmployeeResponse{Employee: toProto(emp)}, nil
}

func (s *Service) GetEmployee(ctx context.Context, req *hrv1.GetEmployeeRequest) (*hrv1.GetEmployeeResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	emp, err := s.q.GetEmployee(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "employee not found")
		}
		return nil, status.Error(codes.Internal, "employee not found")
	}
	return &hrv1.GetEmployeeResponse{Employee: toProto(emp)}, nil
}

func (s *Service) ListEmployees(ctx context.Context, req *hrv1.ListEmployeesRequest) (*hrv1.ListEmployeesResponse, error) {
	limit := req.GetPageSize()

	if limit == 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	var offset int32
	if t := req.GetPageToken(); t != "" {
		n, err := strconv.Atoi(t)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid page_token")
		}
		offset = int32(n)
	}
	rows, err := s.q.ListEmployees(ctx, db.ListEmployeesParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list employees:%v", err)
	}
	out := make([]*hrv1.Employee, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProto(r))
	}
	return &hrv1.ListEmployeesResponse{Employees: out}, nil
}

//HELPERS

func uuidOrNUll(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if strings.TrimSpace(s) == "" {
		return u, nil
	}
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q", s)
	}
	return u, nil
}

func newEmployeeCode() string {
	var b [5]byte
	_, _ = rand.Read(b[:])
	return "EMP-" + strings.ToUpper(hex.EncodeToString(b[:]))
}

func defaultEmploymentType(t string) string {
	if strings.TrimSpace(t) == "" {
		return "full_time"
	}
	return t
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		u.Bytes[0:4], u.Bytes[4:6], u.Bytes[6:8], u.Bytes[8:10], u.Bytes[10:16])
}

func toProto(e db.Employee) *hrv1.Employee {
	out := &hrv1.Employee{
		Id:             uuidString(e.ID),
		EmployeeCode:   e.EmployeeCode,
		FirstName:      e.FirstName,
		LastName:       e.LastName,
		Email:          e.Email,
		DepartmentId:   uuidString(e.DepartmentID),
		Position:       e.Position,
		EmploymentType: e.EmploymentType,
		Status:         "active",
	}
	if e.CreatedAt.Valid {
		out.CreatedAt = timestamppb.New(e.CreatedAt.Time)
	}
	return out
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %v", s)
	}
	return u, nil
}
