package rodlog

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

const (
	maxTableColumns = 6
	maxTableRows    = 30
	maxCellWidth    = 60
)

var (
	tableBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(0, 1)
	tableCellStyle   = lipgloss.NewStyle().Padding(0, 1)
)

func JSONTable(input string, color bool) (string, bool) {
	var payload any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return "", false
	}

	var records []map[string]any
	switch value := payload.(type) {
	case map[string]any:
		records = []map[string]any{value}
	case []any:
		records = make([]map[string]any, 0, len(value))
		for _, item := range value {
			record, ok := item.(map[string]any)
			if !ok {
				return "", false
			}
			records = append(records, record)
		}
	default:
		return "", false
	}

	if len(records) == 0 {
		return "", false
	}

	keys := selectTableColumns(records)
	if len(keys) == 0 {
		return "", false
	}

	limit := min(len(records), maxTableRows)
	rows := make([][]string, 0, limit)
	for i := 0; i < limit; i++ {
		row := make([]string, len(keys))
		for colIdx, key := range keys {
			row[colIdx] = formatTableCell(records[i][key])
		}
		rows = append(rows, row)
	}

	tbl := table.New().
		Border(lipgloss.RoundedBorder()).
		Headers(keys...).
		Rows(rows...)

	if color {
		tbl = tbl.
			BorderStyle(tableBorderStyle).
			StyleFunc(func(row, col int) lipgloss.Style {
				if row == table.HeaderRow {
					return tableHeaderStyle
				}
				return tableCellStyle
			})
	} else {
		tbl = tbl.StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle().Padding(0, 1)
		})
	}

	var output strings.Builder
	output.WriteString(tbl.Render())
	output.WriteByte('\n')

	if len(records) > maxTableRows {
		fmt.Fprintf(&output, "… %d additional records\n", len(records)-maxTableRows)
	}

	return output.String(), true
}

func selectTableColumns(records []map[string]any) []string {
	priority := []string{"name", "status", "state", "id", "region", "url", "version", "created", "updated", "message"}
	seen := make(map[string]struct{})
	keys := make([]string, 0, maxTableColumns)

	for _, key := range priority {
		for _, record := range records {
			if _, ok := record[key]; ok {
				if _, already := seen[key]; !already {
					keys = append(keys, key)
					seen[key] = struct{}{}
				}
			}
		}
	}

	var extra []string
	for _, record := range records {
		for key := range record {
			if _, already := seen[key]; !already {
				extra = append(extra, key)
				seen[key] = struct{}{}
			}
		}
	}
	slices.Sort(extra)
	keys = append(keys, extra...)

	if len(keys) > maxTableColumns {
		keys = keys[:maxTableColumns]
	}
	return keys
}

func formatTableCell(raw any) string {
	if raw == nil {
		return "—"
	}
	value := fmt.Sprint(raw)
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "\r", " ")
	if len(value) > maxCellWidth {
		return value[:maxCellWidth-3] + "..."
	}
	return value
}
