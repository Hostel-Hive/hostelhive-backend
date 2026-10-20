package pdfreport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	pdfread "github.com/ledongthuc/pdf"
)

func compactPDF(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func decodePDF(t *testing.T, data []byte) (*pdfread.Reader, string) {
	t.Helper()
	if !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(data), []byte("%%EOF")) {
		t.Fatal("invalid PDF envelope")
	}
	reader, err := pdfread.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		value, err := page.GetPlainText(nil)
		if err != nil {
			t.Fatal(err)
		}
		text.WriteString(value)
		if !strings.Contains(compactPDF(value), fmt.Sprintf("Page%dof%d", i, reader.NumPage())) {
			t.Fatal("missing page number", i, value)
		}
		for _, piece := range page.Content().Text {
			if piece.S == "" {
				continue
			}
			if piece.X < margin-0.5 || piece.X+piece.W > pageWidth-margin+0.5 || piece.Y < 12 || piece.Y > pageHeight-15 {
				t.Fatalf("text outside printable area on page %d: %+v", i, piece)
			}
		}
		if !page.V.Key("Annots").IsNull() {
			t.Fatal("unexpected annotations / hyperlinks")
		}
	}
	root := reader.Trailer().Key("Root")
	for _, key := range []string{"OpenAction", "AcroForm", "Names"} {
		if !root.Key(key).IsNull() {
			t.Fatal("active or external content", key)
		}
	}
	return reader, text.String()
}

func sampleDocument(kind string) domain.ExportDocument {
	headers := map[string][]string{
		"attendance": {"Event ID", "Device ID", "Student ID", "Direction", "Occurred At", "Access Result", "Late Access ID", "Incident ID"},
		"daily":      {"Date", "Entries", "Exits", "Distinct Students"},
		"leaves":     {"Leave ID", "Request ID", "Student ID", "Submitted By", "Starts At", "Ends At", "Reason", "Status", "Created At", "Decided At", "Decided By", "Decision Note", "Cancelled At", "Cancelled By"},
		"complaints": {"Complaint ID", "Request ID", "Student ID", "Submitted By", "Category", "Description", "Status", "Version", "Created At", "Updated At", "Resolution Recorded", "Resolution Update ID", "Resolved At", "Resolved By", "Resolution Note"},
	}[kind]
	rows := [][]domain.ExportCell{}
	for i := 0; i < 5; i++ {
		row := make([]domain.ExportCell, len(headers))
		for col := range row {
			row[col].Text = fmt.Sprintf("record-%d-field-%d", i, col)
		}
		if kind == "complaints" {
			row[5].Text = "=HYPERLINK(\"https://example.invalid\",\"text only\")\n" + strings.Repeat("Long description with punctuation & < > ( ) ", 65) + "TAIL-MARKER"
		}
		rows = append(rows, row)
	}
	return domain.ExportDocument{Title: "Student " + kind + " report", Headers: headers, Rows: rows, Metadata: [][2]string{{"From", "2026-10-01"}, {"To", "2026-10-15"}, {"Timezone", "Asia/Colombo"}}, Summary: []domain.ExportMetric{{Name: "Total Matching Rows", Value: 5}, {Name: "Exported Rows", Value: 5}}}
}

func TestPDFReportsTextPaginationAndGeometry(t *testing.T) {
	for _, kind := range []string{"attendance", "daily", "leaves", "complaints"} {
		t.Run(kind, func(t *testing.T) {
			doc := sampleDocument(kind)
			data, err := NewReports().Build(context.Background(), doc)
			if err != nil {
				t.Fatal(err)
			}
			if directory := os.Getenv("PDF_ARTIFACT_DIR"); directory != "" {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "sample-"+kind+".pdf"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			reader, text := decodePDF(t, data)
			compact := compactPDF(text)
			for _, value := range []string{"Summary", "Data", "Asia/Colombo", "Total Matching Rows", "Exported Rows", "2026-10-01", "2026-10-15"} {
				if !strings.Contains(compact, compactPDF(value)) {
					t.Fatal("missing", value, text)
				}
			}
			for _, row := range doc.Rows {
				for _, cell := range row {
					if len(cell.Text) < 500 && !strings.Contains(compact, compactPDF(cell.Text)) {
						t.Fatal("missing or changed cell", cell.Text)
					}
				}
			}
			if kind == "complaints" {
				var description strings.Builder
				for p := 2; p <= reader.NumPage(); p++ {
					for _, piece := range reader.Page(p).Content().Text {
						if piece.X >= margin+40+240+200-0.1 && piece.X < margin+40+240+200+170 && piece.Y < pageHeight-(tableTop+23) && piece.Y > pageHeight-tableBottom {
							// The low-level reader synthesizes U+FFFD at word-spacing adjustments.
							// Poppler and pypdf confirm these are not glyphs in the PDF.
							description.WriteString(strings.ReplaceAll(piece.S, "\uFFFD", ""))
						}
					}
				}
				if !strings.Contains(compactPDF(description.String()), compactPDF(doc.Rows[0][5].Text)) {
					t.Fatal("continued description lost text", description.String())
				}
				if reader.NumPage() < 4 {
					t.Fatal("long rows did not paginate", reader.NumPage())
				}
				for page := 2; page <= reader.NumPage(); page++ {
					text, err := reader.Page(page).GetPlainText(nil)
					if err != nil || !strings.Contains(text, "Identifiers") || !strings.Contains(text, "Resolution") {
						t.Fatal("missing repeated headings", page, text, err)
					}
				}
			}

		})
	}
}

func TestPDFEmptyUnicodeAndFailureBounds(t *testing.T) {
	doc := sampleDocument("complaints")
	doc.Rows = nil
	data, err := NewReports().Build(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	_, text := decodePDF(t, data)
	if !strings.Contains(text, "No matching records") {
		t.Fatal(text)
	}
	doc.Rows = [][]domain.ExportCell{make([]domain.ExportCell, len(doc.Headers))}
	doc.Rows[0][5].Text = "Sinhala: සිංහල Tamil: தமிழ் emoji: 😀\t\\ quote: ( ) < > café Ω"
	doc.Rows[0][7].Number = new(int64(9007199254740993))
	data, err = NewReports().Build(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	_, text = decodePDF(t, data)
	compact := compactPDF(text)
	for _, want := range []string{`\u{0DC3}`, `\u{0BA4}`, `\u{1F600}`, `\t`, `\\`, "café", "Ω", "9007199254740993"} {
		if !strings.Contains(compact, compactPDF(want)) {
			t.Fatal(want, text)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if data, err := NewReports().Build(ctx, doc); !errors.Is(err, context.Canceled) || len(data) != 0 {
		t.Fatal("cancelled generation", err)
	}
	doc.Rows = make([][]domain.ExportCell, domain.PDFExportRowLimit+1)
	if _, err := NewReports().Build(context.Background(), doc); !errors.Is(err, domain.ErrExportTooLarge) {
		t.Fatal(err)
	}
	doc = sampleDocument("daily")
	doc.Rows[0] = nil
	if _, err := NewReports().Build(context.Background(), doc); err == nil {
		t.Fatal("bad row accepted")
	}
	doc.Headers = []string{"unknown"}
	if _, err := NewReports().Build(context.Background(), doc); err == nil {
		t.Fatal("unknown schema accepted")
	}
	// A pathological text row must fail at the page bound, not silently lose content.
	doc = sampleDocument("complaints")
	doc.Rows = doc.Rows[:1]
	doc.Rows[0][5].Text = strings.Repeat("large text ", 50000)
	if data, err := NewReports().Build(context.Background(), doc); !errors.Is(err, domain.ErrExportTooLarge) || len(data) != 0 {
		t.Fatal("page limit", err)
	}
}

func TestPDFThousandShortRecordsFitPageBudget(t *testing.T) {
	doc := sampleDocument("complaints")
	doc.Rows = nil
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)
		values := []string{id, id, id, id, "maintenance", "PDF boundary", "pending", "1", "2049-01-01T05:30:00+05:30", "2049-01-01T05:30:00+05:30", "No", "", "", "", ""}
		row := make([]domain.ExportCell, len(values))
		for j, value := range values {
			row[j].Text = value
		}
		doc.Rows = append(doc.Rows, row)
	}
	data, err := NewReports().Build(context.Background(), doc)
	if err != nil || len(data) == 0 {
		t.Fatal("short records failed within the row budget", err)
	}
}

func TestCancelledLeavePDFIncludesAudit(t *testing.T) {
	doc := sampleDocument("leaves")
	doc.Rows = doc.Rows[:1]
	doc.Rows[0][7].Text = "cancelled"
	for _, i := range []int{9, 10, 11} {
		doc.Rows[0][i].Text = ""
	}
	doc.Rows[0][12].Text = "2026-10-20T12:00:00+05:30"
	doc.Rows[0][13].Text = "cancelled-student-account"
	doc.Summary = append(doc.Summary, domain.ExportMetric{Name: "Cancelled", Value: 1})
	data, err := NewReports().Build(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	_, text := decodePDF(t, data)
	for _, v := range []string{"Status / Audit", "Cancelled At", "Cancelled By", "cancelled-student-account", "2026-10-20T12:00:00+05:30", "cancelled"} {
		if !strings.Contains(compactPDF(text), compactPDF(v)) {
			t.Fatal("missing cancellation audit", v, text)
		}
	}
}
