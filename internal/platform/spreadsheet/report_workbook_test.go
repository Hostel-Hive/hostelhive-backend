package spreadsheet

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/xuri/excelize/v2"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestLiteralTextWorkbookAndStructure(t *testing.T) {
	n, big := int64(42), int64(math.MaxInt64)
	values := []string{"=HYPERLINK(\"https://example.invalid\",\"click\")", "+SUM(1,2)", "-1+2", "@SUM(1,2)", "\t=1+1", "'literal apostrophe", "https://example.invalid", "quoted \" & <xml>\nසිංහල தமிழ்"}
	doc := domain.ExportWorkbook{Title: "Safe report", Metadata: [][2]string{{"Timezone", "Asia/Colombo"}, {"Student ID", "All"}}, Summary: []domain.ExportMetric{{Name: "Count", Value: 42}}, Headers: []string{"Description", "Count", "Exact Version"}}
	for _, text := range values {
		doc.Rows = append(doc.Rows, []domain.ExportCell{{Text: text}, {Number: &n}, {Number: &big}})
	}
	data, err := NewReportWorkbooks().Build(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if sheets := f.GetSheetList(); len(sheets) != 2 || sheets[0] != "Summary" || sheets[1] != "Data" {
		t.Fatal(sheets)
	}
	for i, want := range values {
		address := "A" + strconv.Itoa(i+2)
		got, err := f.GetCellValue("Data", address)
		if err != nil || got != want {
			t.Fatal(address, got, want, err)
		}
		formula, err := f.GetCellFormula("Data", address)
		if err != nil || formula != "" {
			t.Fatal("formula was created", formula, err)
		}
		kind, err := f.GetCellType("Data", address)
		if err != nil || (kind != excelize.CellTypeSharedString && kind != excelize.CellTypeInlineString) {
			t.Fatal(kind, err)
		}
	}
	if got, err := f.GetCellValue("Data", "B2"); err != nil || got != "42" {
		t.Fatal(got, err)
	}
	if got, err := f.GetCellValue("Data", "C2"); err != nil || got != strconv.FormatInt(big, 10) {
		t.Fatal("integer precision lost", got, err)
	}
	panes, err := f.GetPanes("Data")
	if err != nil || !panes.Freeze || panes.YSplit != 1 || panes.TopLeftCell != "A2" {
		t.Fatal(panes, err)
	}
	// Independently parse the ZIP/XML, rather than trusting only the library reader.
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range archive.File {
		if strings.Contains(part.Name, "externalLinks") || strings.Contains(part.Name, "vbaProject") {
			t.Fatal("active/external content", part.Name)
		}
		if !strings.HasSuffix(part.Name, ".xml") && !strings.HasSuffix(part.Name, ".rels") {
			continue
		}
		r, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(r)
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				r.Close()
				t.Fatal(part.Name, err)
			}
			if e, ok := token.(xml.StartElement); ok {
				if strings.HasPrefix(part.Name, "xl/worksheets/") && (e.Name.Local == "f" || e.Name.Local == "hyperlink") {
					t.Fatal("active cell content", part.Name, e.Name.Local)
				}
				if e.Name.Local == "Relationship" {
					for _, a := range e.Attr {
						if a.Name.Local == "TargetMode" && a.Value == "External" {
							t.Fatal("external relationship")
						}
					}
				}
			}
		}
		r.Close()
	}
}
func TestEmptyAndInvalidWorkbooks(t *testing.T) {
	writer := NewReportWorkbooks()
	data, err := writer.Build(context.Background(), domain.ExportWorkbook{Title: "Empty", Headers: []string{"ID"}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.GetRows("Data")
	f.Close()
	if err != nil || len(rows) != 1 || rows[0][0] != "ID" {
		t.Fatal(rows, err)
	}
	n := int64(1)
	for _, doc := range []domain.ExportWorkbook{{}, {Headers: make([]string, 41)}, {Headers: []string{"ID"}, Rows: make([][]domain.ExportCell, domain.ExportRowLimit+1)}, {Headers: []string{"ID"}, Rows: [][]domain.ExportCell{{}}}, {Headers: []string{"ID"}, Rows: [][]domain.ExportCell{{{Text: "both", Number: &n}}}}} {
		if _, err := writer.Build(context.Background(), doc); err == nil {
			t.Fatal("invalid workbook accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writer.Build(ctx, domain.ExportWorkbook{Headers: []string{"ID"}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if _, err := (contextWriter{ctx, &buffer}).Write([]byte("not written")); !errors.Is(err, context.Canceled) || buffer.Len() != 0 {
		t.Fatal(err)
	}
}
