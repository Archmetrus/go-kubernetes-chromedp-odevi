package tasks

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxAssignmentFile = 2 * 1024 * 1024
const maxAssignmentRows = 200

type AssignmentRow struct {
	TeamID        string `json:"team_id"`
	AssigneeEmail string `json:"assignee_email"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	Priority      string `json:"priority"`
	Status        string `json:"status"`
	DueDate       string `json:"due_date"`
}

func parseAssignments(data []byte, media string) ([]AssignmentRow, error) {
	if len(data) > maxAssignmentFile {
		return nil, fmt.Errorf("%w: dosya en fazla 2 MiB olabilir", ErrValidation)
	}
	var rows []AssignmentRow
	if media == "application/json" {
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		var in struct {
			Tasks []AssignmentRow `json:"tasks"`
		}
		if err := d.Decode(&in); err != nil {
			return nil, fmt.Errorf("%w: tasks dizisi içeren JSON gerekli", ErrValidation)
		}
		if d.Decode(new(any)) != io.EOF {
			return nil, ErrValidation
		}
		rows = in.Tasks
	} else if media == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		var err error
		rows, err = parseAssignmentXLSX(data)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrValidation, err)
		}
	} else {
		return nil, fmt.Errorf("%w: JSON veya .xlsx dosyası gerekli", ErrValidation)
	}
	if len(rows) == 0 || len(rows) > maxAssignmentRows {
		return nil, fmt.Errorf("%w: dosya 1–200 görev içermeli", ErrValidation)
	}
	return rows, nil
}

type xlsxText struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

func (x xlsxText) String() string {
	var b strings.Builder
	b.WriteString(x.Text)
	for _, r := range x.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// A bounded, deliberately narrow XLSX import: one sheet, literal text cells,
// shared/inline strings. No formula evaluation, macros, external links or extraction.
func parseAssignmentXLSX(data []byte) ([]AssignmentRow, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("geçersiz XLSX")
	}
	if len(z.File) > 128 {
		return nil, fmt.Errorf("çok fazla ZIP parçası")
	}
	parts := map[string][]byte{}
	var total uint64
	worksheets := 0
	for _, f := range z.File {
		name := f.Name
		lower := strings.ToLower(name)
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(lower, "vba") || strings.Contains(lower, "externallink") {
			return nil, fmt.Errorf("makro/dış bağlantı veya geçersiz dosya yolu")
		}
		if _, ok := parts[name]; ok {
			return nil, fmt.Errorf("tekrarlanan ZIP parçası")
		}
		total += f.UncompressedSize64
		if total > 16*1024*1024 || f.UncompressedSize64 > 8*1024*1024 {
			return nil, fmt.Errorf("açılmış dosya boyutu sınırı aşıldı")
		}
		r, e := f.Open()
		if e != nil {
			return nil, e
		}
		b, e := io.ReadAll(io.LimitReader(r, 8*1024*1024+1))
		r.Close()
		if e != nil || len(b) > 8*1024*1024 {
			return nil, fmt.Errorf("ZIP parçası okunamadı")
		}
		parts[name] = b
		if strings.HasSuffix(name, ".rels") {
			var relationships struct {
				Items []struct {
					Mode string `xml:"TargetMode,attr"`
				} `xml:"Relationship"`
			}
			if err = xml.Unmarshal(b, &relationships); err != nil {
				return nil, fmt.Errorf("geçersiz ilişki dosyası")
			}
			for _, relationship := range relationships.Items {
				if strings.EqualFold(relationship.Mode, "External") {
					return nil, fmt.Errorf("dış bağlantı desteklenmez")
				}
			}
		}
		if strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml") {
			worksheets++
		}
	}
	if worksheets != 1 || parts["xl/worksheets/sheet1.xml"] == nil {
		return nil, fmt.Errorf("tek çalışma sayfası kullan; ilk sayfanın başlık satırını koru")
	}
	var shared struct {
		Items []xlsxText `xml:"si"`
	}
	if b := parts["xl/sharedStrings.xml"]; b != nil {
		if err = xml.Unmarshal(b, &shared); err != nil {
			return nil, fmt.Errorf("shared strings geçersiz")
		}
	}
	var sheet struct {
		Merges []struct{} `xml:"mergeCells>mergeCell"`
		Rows   []struct {
			Cells []struct {
				Ref     string   `xml:"r,attr"`
				Type    string   `xml:"t,attr"`
				Formula *string  `xml:"f"`
				Value   string   `xml:"v"`
				Inline  xlsxText `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err = xml.Unmarshal(parts["xl/worksheets/sheet1.xml"], &sheet); err != nil {
		return nil, fmt.Errorf("sayfa XML geçersiz")
	}
	if len(sheet.Merges) > 0 || len(sheet.Rows) > 201 {
		return nil, fmt.Errorf("birleştirilmiş hücre kullanma; en fazla 200 görev")
	}
	allowed := map[string]bool{"team_id": true, "assignee_email": true, "title": true, "description": true, "category": true, "priority": true, "status": true, "due_date": true}
	headers := map[int]string{}
	out := []AssignmentRow{}
	for ri, row := range sheet.Rows {
		values := map[int]string{}
		for _, c := range row.Cells {
			if c.Formula != nil {
				return nil, fmt.Errorf("satır %d: formül yerine sabit değer kullan", ri+1)
			}
			col := 0
			for _, r := range c.Ref {
				if r < 'A' || r > 'Z' {
					break
				}
				col = col*26 + int(r-'A'+1)
				if col > 8 {
					return nil, fmt.Errorf("yalnızca A–H sütunları desteklenir")
				}
			}
			if col == 0 {
				return nil, fmt.Errorf("hücre adresi eksik")
			}
			if _, ok := values[col]; ok {
				return nil, fmt.Errorf("tekrarlanan hücre")
			}
			value := c.Value
			switch c.Type {
			case "s":
				i, e := strconv.Atoi(value)
				if e != nil || i < 0 || i >= len(shared.Items) {
					return nil, fmt.Errorf("hücre metin indeksi geçersiz")
				}
				value = shared.Items[i].String()
			case "inlineStr":
				value = c.Inline.String()
			case "", "str", "d":
			default:
				return nil, fmt.Errorf("satır %d: hücreyi metin olarak gir", ri+1)
			}
			values[col] = strings.TrimSpace(value)
		}
		if ri == 0 {
			seen := map[string]bool{}
			for col, v := range values {
				if !allowed[v] || seen[v] {
					return nil, fmt.Errorf("başlıklar tanınmıyor veya tekrarlanmış")
				}
				headers[col] = v
				seen[v] = true
			}
			for _, v := range []string{"team_id", "assignee_email", "title"} {
				if !seen[v] {
					return nil, fmt.Errorf("%s başlığı gerekli", v)
				}
			}
			continue
		}
		mapped := map[string]string{}
		nonempty := false
		for col, v := range values {
			if v != "" {
				nonempty = true
			}
			key, ok := headers[col]
			if !ok && v != "" {
				return nil, fmt.Errorf("başlıksız sütun")
			}
			if ok {
				mapped[key] = v
			}
		}
		if !nonempty {
			continue
		}
		b, _ := json.Marshal(mapped)
		var a AssignmentRow
		if err = json.Unmarshal(b, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
