package compliance

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// RenderCSV converts a ReportData into a UTF-8 CSV document.
// One section per check; blank line between sections.
func RenderCSV(data *ReportData) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Header metadata
	_ = w.Write([]string{"NetScope Compliance Report"})
	_ = w.Write([]string{"Framework", frameworkLabel(data.Framework)})
	_ = w.Write([]string{"Generated", data.GeneratedAt.Format(time.RFC3339)})
	_ = w.Write([]string{"Period", data.Period})
	_ = w.Write([]string{})

	for _, check := range data.Checks {
		_ = w.Write([]string{check.CheckName, check.Status, check.Description})
		if len(check.Columns) > 0 {
			_ = w.Write(check.Columns)
		}
		for _, row := range check.Rows {
			_ = w.Write(row)
		}
		_ = w.Write([]string{}) // blank separator
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

// RenderPDF converts a ReportData into a PDF document using gofpdf.
func RenderPDF(data *ReportData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(fmt.Sprintf("NetScope %s Report", frameworkLabel(data.Framework)), false)
	pdf.SetAuthor("NetScope Hub", false)
	pdf.SetCreationDate(data.GeneratedAt)

	// ── Cover page ─────────────────────────────────────────────────────────
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(0, 20, "NetScope", "", 1, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 16)
	pdf.SetTextColor(80, 80, 80)
	pdf.CellFormat(0, 10, "Compliance Report", "", 1, "C", false, 0, "")
	pdf.Ln(6)

	pdf.SetFont("Helvetica", "B", 20)
	pdf.SetTextColor(30, 30, 200)
	pdf.CellFormat(0, 12, frameworkLabel(data.Framework), "", 1, "C", false, 0, "")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(0, 7, "Generated: "+data.GeneratedAt.Format("02 Jan 2006 15:04 UTC"), "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 7, "Period: "+data.Period, "", 1, "C", false, 0, "")

	// ── Summary table ───────────────────────────────────────────────────────
	pdf.Ln(14)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(0, 8, "Summary")
	pdf.Ln(10)

	passCount, warnCount, failCount, infoCount := 0, 0, 0, 0
	for _, c := range data.Checks {
		switch c.Status {
		case "pass":
			passCount++
		case "warn":
			warnCount++
		case "fail":
			failCount++
		default:
			infoCount++
		}
	}

	drawSummaryRow(pdf, "PASS", passCount, 0, 150, 0)
	drawSummaryRow(pdf, "WARN", warnCount, 200, 130, 0)
	drawSummaryRow(pdf, "FAIL", failCount, 200, 0, 0)
	drawSummaryRow(pdf, "INFO", infoCount, 80, 80, 200)

	// ── Check details ───────────────────────────────────────────────────────
	for _, check := range data.Checks {
		pdf.AddPage()
		renderCheck(pdf, check)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawSummaryRow(pdf *gofpdf.Fpdf, label string, count int, r, g, b int) {
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(r, g, b)
	pdf.CellFormat(30, 7, label, "1", 0, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(0, 0, 0)
	pdf.CellFormat(20, 7, fmt.Sprintf("%d", count), "1", 1, "C", false, 0, "")
}

func renderCheck(pdf *gofpdf.Fpdf, check CheckResult) {
	// Check title + status badge
	statusColors := map[string][3]int{
		"pass": {0, 150, 0},
		"warn": {200, 130, 0},
		"fail": {200, 0, 0},
		"info": {80, 80, 200},
	}
	color := statusColors[check.Status]
	if color == [3]int{} {
		color = [3]int{80, 80, 80}
	}

	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(0, 0, 0)
	pdf.MultiCell(0, 7, check.CheckName, "", "L", false)

	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(color[0], color[1], color[2])
	pdf.Cell(20, 6, strings.ToUpper(check.Status))
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf(" — %d finding(s)", check.RowCount))
	pdf.Ln(8)

	// Description
	pdf.SetFont("Helvetica", "I", 9)
	pdf.SetTextColor(80, 80, 80)
	pdf.MultiCell(0, 5, check.Description, "", "L", false)
	pdf.Ln(4)

	if len(check.Rows) == 0 {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(0, 150, 0)
		pdf.Cell(0, 6, "No findings.")
		pdf.Ln(8)
		return
	}

	// Evidence table
	colW := float64(180) / float64(max(len(check.Columns), 1))
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFillColor(40, 40, 80)
	for _, col := range check.Columns {
		pdf.CellFormat(colW, 6, truncate(col, 20), "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(0, 0, 0)
	fill := false
	for _, row := range check.Rows {
		if fill {
			pdf.SetFillColor(240, 240, 248)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		for _, cell := range row {
			pdf.CellFormat(colW, 5, truncate(cell, 25), "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
		fill = !fill
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func frameworkLabel(f string) string {
	switch f {
	case FrameworkSOC2:
		return "SOC 2 Type II"
	case FrameworkPCI:
		return "PCI-DSS v4.0"
	case FrameworkHIPAA:
		return "HIPAA Security Rule"
	default:
		return strings.ToUpper(f)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
