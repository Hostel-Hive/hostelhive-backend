package spreadsheet

import (
	"bytes"
	"context"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/xuri/excelize/v2"
	"io"
	"strconv"
	"strings"
)

type ReportWorkbooks struct{}

func NewReportWorkbooks() *ReportWorkbooks { return &ReportWorkbooks{} }

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}
func cell(f *excelize.File, sheet, address string, v domain.ExportCell) error {
	if v.Number == nil {
		return f.SetCellStr(sheet, address, v.Text)
	}
	if v.Text != "" {
		return errors.New("ambiguous spreadsheet cell")
	}
	// Excel preserves only 15 significant numeric digits. Larger exact counts stay text.
	if *v.Number > 999999999999999 || *v.Number < -999999999999999 {
		return f.SetCellStr(sheet, address, strconv.FormatInt(*v.Number, 10))
	}
	return f.SetCellInt(sheet, address, *v.Number)
}
func (s *ReportWorkbooks) Build(ctx context.Context, doc domain.ExportWorkbook) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(doc.Headers) == 0 || len(doc.Headers) > 40 || len(doc.Rows) > domain.ExportRowLimit {
		return nil, errors.New("invalid report workbook")
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Summary"); err != nil {
		return nil, err
	}
	if _, err := f.NewSheet("Data"); err != nil {
		return nil, err
	}
	if err := f.SetDocProps(&excelize.DocProperties{Title: doc.Title, Creator: "HostelHive"}); err != nil {
		return nil, err
	}
	header, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"17365D"}, Pattern: 1}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	if err != nil {
		return nil, err
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	if err != nil {
		return nil, err
	}
	writeRow := func(sheet string, row int, values []domain.ExportCell) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		for i, v := range values {
			address, err := excelize.CoordinatesToCellName(i+1, row)
			if err != nil {
				return err
			}
			if err = cell(f, sheet, address, v); err != nil {
				return err
			}
		}
		return nil
	}
	if err = writeRow("Summary", 1, []domain.ExportCell{{Text: "Field"}, {Text: "Value"}}); err != nil {
		return nil, err
	}
	for i, v := range doc.Metadata {
		if err = writeRow("Summary", i+2, []domain.ExportCell{{Text: v[0]}, {Text: v[1]}}); err != nil {
			return nil, err
		}
	}
	metricRow := len(doc.Metadata) + 3
	if err = writeRow("Summary", metricRow, []domain.ExportCell{{Text: "Metric"}, {Text: "Value"}}); err != nil {
		return nil, err
	}
	for i, v := range doc.Summary {
		n := v.Value
		if err = writeRow("Summary", metricRow+i+1, []domain.ExportCell{{Text: v.Name}, {Number: &n}}); err != nil {
			return nil, err
		}
	}
	endSummary := "B" + strconv.Itoa(metricRow+len(doc.Summary))
	if err = f.SetCellStyle("Summary", "A1", endSummary, wrap); err != nil {
		return nil, err
	}
	for _, row := range []int{1, metricRow} {
		a, b := "A"+strconv.Itoa(row), "B"+strconv.Itoa(row)
		if err = f.SetCellStyle("Summary", a, b, header); err != nil {
			return nil, err
		}
		if err = f.SetRowHeight("Summary", row, 28); err != nil {
			return nil, err
		}
	}
	if err = f.SetColWidth("Summary", "A", "A", 28); err != nil {
		return nil, err
	}
	if err = f.SetColWidth("Summary", "B", "B", 90); err != nil {
		return nil, err
	}
	headers := make([]domain.ExportCell, len(doc.Headers))
	for i, name := range doc.Headers {
		headers[i].Text = name
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, err
		}
		width := 20.0
		if strings.Contains(name, "ID") || strings.HasSuffix(name, " By") {
			width = 39
		}
		if strings.HasSuffix(name, " At") {
			width = 35
		}
		if name == "Description" || name == "Reason" || strings.Contains(name, "Note") {
			width = 60
		}
		if err = f.SetColWidth("Data", col, col, width); err != nil {
			return nil, err
		}
	}
	if err = writeRow("Data", 1, headers); err != nil {
		return nil, err
	}
	for i, row := range doc.Rows {
		if len(row) != len(headers) {
			return nil, errors.New("invalid report row width")
		}
		if err = writeRow("Data", i+2, row); err != nil {
			return nil, err
		}
	}
	lastCol, err := excelize.ColumnNumberToName(len(headers))
	if err != nil {
		return nil, err
	}
	endData := lastCol + strconv.Itoa(len(doc.Rows)+1)
	if err = f.SetCellStyle("Data", "A1", endData, wrap); err != nil {
		return nil, err
	}
	if err = f.SetCellStyle("Data", "A1", lastCol+"1", header); err != nil {
		return nil, err
	}
	if err = f.SetRowHeight("Data", 1, 30); err != nil {
		return nil, err
	}
	if err = f.SetPanes("Data", &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	if err = f.AutoFilter("Data", "A1:"+endData, nil); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	if err = f.Write(contextWriter{ctx, &buffer}); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
