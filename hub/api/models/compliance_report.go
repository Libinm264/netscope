package models

import "time"

// ReportSchedule is a persisted compliance report schedule.
type ReportSchedule struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Framework  string    `json:"framework"` // "soc2" | "pci-dss" | "hipaa"
	Format     string    `json:"format"`    // "pdf" | "csv"
	Schedule   string    `json:"schedule"`  // "daily" | "weekly" | "monthly"
	Recipients []string  `json:"recipients"`
	Enabled    bool      `json:"enabled"`
	LastSent   time.Time `json:"last_sent,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ReportRun is one recorded execution of a report schedule.
type ReportRun struct {
	ID         string    `json:"id"`
	ScheduleID string    `json:"schedule_id"`
	Framework  string    `json:"framework"`
	Format     string    `json:"format"`
	Recipients []string  `json:"recipients"`
	Rows       uint64    `json:"rows"`
	SentAt     time.Time `json:"sent_at"`
	Error      string    `json:"error,omitempty"`
}
