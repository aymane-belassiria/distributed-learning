package service

import (
	hrv1 "github.com/aymane-belassiria/hr-platform/gen/go/hr/v1"
	db "github.com/aymane-belassiria/hr-platform/services/employee/db/gen"
)

type Service struct {
	hrv1.UnimplementedEmployeeServiceServer
	q *db.Queries
}

func New(q *db.Queries) *Service {
	return &Service{q: q}
}
