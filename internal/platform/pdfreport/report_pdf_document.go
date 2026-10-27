package pdfreport

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/Hostel-Hive/hostelhive-backend/internal/modules/report/domain"
	"github.com/signintech/gopdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

const (
	pageWidth   = 841.89
	pageHeight  = 595.28
	margin      = 28.0
	lineHeight  = 11.0
	bodySize    = 8.5
	tableTop    = 87.0
	tableBottom = pageHeight - 38
	padding     = 5.0
)

type Reports struct{}

func NewReports() *Reports { return &Reports{} }

type layout struct {
	pdf            gopdf.GoPdf
	ctx            context.Context
	font           *sfnt.Font
	escaped        map[rune]string
	widths         map[rune]float64
	title, section string
	headers        []string
	columns        []float64
	y              float64
}

// Display unsupported/complex-script glyphs as explicit code points rather than
// dropping them or drawing incorrect shaping. Excel retains the original Unicode.
func (l *layout) display(value string) string {
	var out strings.Builder
	for _, r := range value {
		if r == '\n' {
			out.WriteRune(r)
			continue
		}
		if v, ok := l.escaped[r]; ok {
			out.WriteString(v)
			continue
		}
		v := string(r)
		glyph, err := l.font.GlyphIndex(nil, r)
		simple := r <= 0x024f || unicode.In(r, unicode.Greek, unicode.Cyrillic, unicode.Punct, unicode.Symbol)
		switch {
		case r == '\\':
			v = `\\`
		case r == '\t':
			v = `\t`
		case unicode.IsControl(r) || !simple || glyph == 0 || err != nil:
			v = fmt.Sprintf(`\u{%04X}`, r)
		}
		l.escaped[r] = v
		out.WriteString(v)
	}
	return out.String()
}

// Measure each rune once. Long unbroken IDs and URLs wrap within their cells.
func (l *layout) wrap(value string, width float64) ([]string, error) {
	var lines []string
	for _, paragraph := range strings.Split(l.display(value), "\n") {
		runes := []rune(paragraph)
		if len(runes) == 0 {
			lines = append(lines, "")
			continue
		}
		start, lastSpace, used := 0, -1, 0.0
		for i := 0; i < len(runes); {
			if err := l.ctx.Err(); err != nil {
				return nil, err
			}
			w, ok := l.widths[runes[i]]
			if !ok {
				var err error
				w, err = l.pdf.MeasureTextWidth(string(runes[i]))
				if err != nil {
					return nil, err
				}
				l.widths[runes[i]] = w
			}
			if used+w > width {
				end := i
				if lastSpace >= start {
					end = lastSpace + 1
				}
				if end == start {
					return nil, errors.New("PDF column too narrow")
				}
				lines = append(lines, string(runes[start:end]))
				start, i, used, lastSpace = end, end, 0, -1
				continue
			}
			used += w
			if runes[i] == ' ' {
				lastSpace = i
			}
			i++
		}
		if start < len(runes) {
			lines = append(lines, string(runes[start:]))
		}
	}
	return lines, nil
}

func (l *layout) text(x, y, width, height float64, value string) error {
	l.pdf.SetXY(x, y)
	return l.pdf.CellWithOption(&gopdf.Rect{W: width, H: height}, value, gopdf.CellOption{Align: gopdf.Left | gopdf.Top})
}

func (l *layout) page() error {
	if err := l.ctx.Err(); err != nil {
		return err
	}
	if l.pdf.GetNumberOfPages() >= domain.PDFExportPageLimit {
		return domain.ErrExportTooLarge
	}
	l.pdf.AddPage()
	l.pdf.SetFillColor(23, 54, 93)
	l.pdf.RectFromUpperLeftWithStyle(0, 0, pageWidth, 7, "F")
	l.pdf.SetTextColor(23, 54, 93)
	if err := l.pdf.SetFont("bold", "", 17); err != nil {
		return err
	}
	if err := l.text(margin, 21, pageWidth-2*margin, 23, "HostelHive | "+l.title); err != nil {
		return err
	}
	if err := l.pdf.SetFont("bold", "", 10); err != nil {
		return err
	}
	if err := l.text(margin, 53, pageWidth-2*margin, 16, l.section); err != nil {
		return err
	}
	l.pdf.SetFillColor(23, 54, 93)
	l.pdf.SetTextColor(255, 255, 255)
	x := margin
	for i, h := range l.headers {
		l.pdf.SetFillColor(23, 54, 93)
		l.pdf.RectFromUpperLeftWithStyle(x, tableTop, l.columns[i], 23, "F")
		if err := l.text(x+padding, tableTop+5, l.columns[i]-2*padding, 15, h); err != nil {
			return err
		}
		x += l.columns[i]
	}
	l.pdf.SetTextColor(30, 41, 59)
	if err := l.pdf.SetFont("regular", "", bodySize); err != nil {
		return err
	}
	l.y = tableTop + 23
	return nil
}

// A record taller than a page is continued, never clipped or silently omitted.
func (l *layout) row(values []string, record int) error {
	lines := make([][]string, len(values))
	maxLines := 1
	for i, v := range values {
		var err error
		lines[i], err = l.wrap(v, l.columns[i]-2*padding)
		if err != nil {
			return err
		}
		if len(lines[i]) > maxLines {
			maxLines = len(lines[i])
		}
	}
	fullHeight := float64(maxLines)*lineHeight + 2*padding
	if fullHeight <= tableBottom-(tableTop+23) && l.y+fullHeight > tableBottom {
		if err := l.page(); err != nil {
			return err
		}
	}
	for offset := 0; offset < maxLines; {
		available := int((tableBottom - l.y - 2*padding) / lineHeight)
		if available < 1 {
			if err := l.page(); err != nil {
				return err
			}
			continue
		}
		n := min(available, maxLines-offset)
		height := float64(n)*lineHeight + 2*padding
		x := margin
		l.pdf.SetStrokeColor(210, 219, 231)
		l.pdf.SetLineWidth(0.3)
		for col := range values {
			if record%2 == 0 {
				l.pdf.SetFillColor(245, 248, 252)
			} else {
				l.pdf.SetFillColor(255, 255, 255)
			}
			l.pdf.RectFromUpperLeftWithStyle(x, l.y, l.columns[col], height, "DF")
			for line := 0; line < n; line++ {
				index := offset + line
				value := ""
				if index < len(lines[col]) {
					value = lines[col][index]
				}
				if col == 0 && record > 0 && offset > 0 && line == 0 {
					value = strconv.Itoa(record) + "+"
				}
				if err := l.text(x+padding, l.y+padding+float64(line)*lineHeight, l.columns[col]-2*padding, lineHeight, value); err != nil {
					return err
				}
			}
			x += l.columns[col]
		}
		l.y += height
		offset += n
		if offset < maxLines {
			if err := l.page(); err != nil {
				return err
			}
		}
	}
	return nil
}

func dataGroups(doc domain.ExportDocument) ([]string, [][]int, error) {
	switch {
	case len(doc.Headers) == 4 && doc.Headers[0] == "Date":
		return []string{"Date", "Entries", "Exits", "Distinct Students"}, [][]int{{0}, {1}, {2}, {3}}, nil
	case len(doc.Headers) == 8 && doc.Headers[0] == "Event ID":
		return []string{"Identifiers", "Movement", "Recorded Time", "Access Details"}, [][]int{{0, 1, 2}, {3}, {4}, {5, 6, 7}}, nil
	case len(doc.Headers) == 14 && doc.Headers[0] == "Leave ID":
		return []string{"Identifiers", "Requested Period / Submission", "Reason", "Status / Audit"}, [][]int{{0, 1, 2, 3}, {4, 5, 8}, {6}, {7, 9, 10, 11, 12, 13}}, nil
	case len(doc.Headers) == 12 && doc.Headers[0] == "Leave ID":
		return []string{"Identifiers", "Requested Period / Submission", "Reason", "Status / Decision"}, [][]int{{0, 1, 2, 3}, {4, 5, 8}, {6}, {7, 9, 10, 11}}, nil
	case len(doc.Headers) == 15 && doc.Headers[0] == "Complaint ID":
		return []string{"Identifiers", "Details / Submission", "Description", "Resolution"}, [][]int{{0, 1, 2, 3}, {4, 6, 7, 8, 9}, {5}, {10, 11, 12, 13, 14}}, nil
	default:
		return nil, nil, errors.New("unsupported PDF report schema")
	}
}

func (s *Reports) Build(ctx context.Context, doc domain.ExportDocument) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(doc.Rows) > domain.PDFExportRowLimit {
		return nil, domain.ErrExportTooLarge
	}
	headers, groups, err := dataGroups(doc)
	if err != nil {
		return nil, err
	}
	font, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	l := &layout{ctx: ctx, font: font, title: doc.Title, escaped: map[rune]string{}, widths: map[rune]float64{}}
	l.pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageWidth, H: pageHeight}})
	l.pdf.SetInfo(gopdf.PdfInfo{Title: doc.Title, Author: "HostelHive", Creator: "HostelHive backend"})
	if err := l.pdf.AddTTFFontData("regular", goregular.TTF); err != nil {
		return nil, err
	}
	if err := l.pdf.AddTTFFontData("bold", gobold.TTF); err != nil {
		return nil, err
	}
	l.section, l.headers, l.columns = "Summary", []string{"Field", "Value"}, []float64{170, pageWidth - 2*margin - 170}
	if err := l.page(); err != nil {
		return nil, err
	}
	for _, pair := range doc.Metadata {
		if err := l.row([]string{pair[0], pair[1]}, 0); err != nil {
			return nil, err
		}
	}
	for _, metric := range doc.Summary {
		if err := l.row([]string{metric.Name, strconv.FormatInt(metric.Value, 10)}, 0); err != nil {
			return nil, err
		}
	}
	notice := `Unsupported or complex-script characters are displayed as Unicode code point escapes. Backslashes are doubled; tabs are escaped. Excel retains original Unicode. Blank fields mean not recorded or not applicable. A + after a row number marks continuation.`
	if err := l.row([]string{"Text display", notice}, 0); err != nil {
		return nil, err
	}
	l.section = "Data | " + strconv.Itoa(len(doc.Rows)) + " matching rows"
	l.headers = append([]string{"#"}, headers...)
	l.columns = []float64{40, 240, 200, 170, pageWidth - 2*margin - 650}
	if len(doc.Headers) == 8 {
		l.columns = []float64{40, 240, 70, 180, pageWidth - 2*margin - 530}
	}
	if len(doc.Headers) == 4 {
		l.columns = []float64{40, 200, 180, 180, pageWidth - 2*margin - 600}
	}
	if err := l.page(); err != nil {
		return nil, err
	}
	if len(doc.Rows) == 0 {
		if err := l.row([]string{"", "No matching records", "", "", ""}, 1); err != nil {
			return nil, err
		}
	}
	for i, row := range doc.Rows {
		if len(row) != len(doc.Headers) {
			return nil, errors.New("invalid PDF row width")
		}
		values := []string{strconv.Itoa(i + 1)}
		for _, indices := range groups {
			var fields []string
			for _, col := range indices {
				value := row[col].Text
				if row[col].Number != nil {
					value = strconv.FormatInt(*row[col].Number, 10)
				}
				if len(doc.Headers) == 8 {
					l.columns = []float64{40, 240, 70, 180, pageWidth - 2*margin - 530}
				}
				if len(doc.Headers) == 4 {
					fields = append(fields, value)
				} else {
					fields = append(fields, doc.Headers[col]+": "+value)
				}
			}
			values = append(values, strings.Join(fields, "\n"))
		}
		if err := l.row(values, i+1); err != nil {
			return nil, err
		}
	}
	pages := l.pdf.GetNumberOfPages()
	for page := 1; page <= pages; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := l.pdf.SetPage(page); err != nil {
			return nil, err
		}
		if err := l.pdf.SetFont("regular", "", 8); err != nil {
			return nil, err
		}
		l.pdf.SetTextColor(90, 105, 125)
		if err := l.text(margin, pageHeight-24, pageWidth-2*margin, 12, fmt.Sprintf("HostelHive | Private staff report                                              Page %d of %d", page, pages)); err != nil {
			return nil, err
		}
	}
	data, err := l.pdf.GetBytesPdfReturnErr()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > domain.PDFExportByteLimit {
		return nil, domain.ErrExportTooLarge
	}
	return data, nil
}
