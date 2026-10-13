-- name: CreateEmployee :one
INSERT INTO employees (
  employee_code, first_name, last_name, email,
  department_id, position, employment_type, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetEmployee :one
SELECT * FROM employees WHERE id = $1;

-- name: ListEmployees :many
SELECT * FROM employees
ORDER BY last_name, first_name
LIMIT $1 OFFSET $2;