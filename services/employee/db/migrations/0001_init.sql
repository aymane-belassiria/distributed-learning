-- +goose Up
CREATE TABLE departments(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE employees (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_code   text NOT NULL UNIQUE,
  first_name      text NOT NULL,
  last_name       text NOT NULL,
  email           text NOT NULL UNIQUE,
  department_id   uuid REFERENCES departments(id),
  position        text NOT NULL DEFAULT '',
  manager_id      uuid REFERENCES employees(id),
  hire_date       date NOT NULL DEFAULT current_date,
  employment_type text NOT NULL DEFAULT 'full_time',
  status          text NOT NULL DEFAULT 'active',
  created_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP Table employees;
DROP TABLE departments;