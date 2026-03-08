package views

import "github.com/d3chapma/pocket-tasks/internal/db"

type CompletedDay struct {
	Label string
	Tasks []db.Task
}
