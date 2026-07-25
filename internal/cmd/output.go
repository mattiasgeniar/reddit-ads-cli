package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/mattiasgeniar/reddit-ads-cli/internal/api"
)

// printJSON pretty-prints raw JSON to w, mirroring what the API returned.
func printJSON(w io.Writer, raw json.RawMessage) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		// Not valid JSON to re-indent; emit as-is rather than failing the command.
		_, err := w.Write(raw)
		return err
	}
	_, err := fmt.Fprintln(w, pretty.String())
	return err
}

// printJSONValue marshals v and pretty-prints it. Used for lists assembled from
// paginated pages.
func printJSONValue(w io.Writer, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(raw))
	return err
}

// table renders rows as an aligned text table with a header. Each row must have the
// same number of columns as header.
type table struct {
	header []string
	rows   [][]string
}

func newTable(header ...string) *table {
	return &table{header: header}
}

func (t *table) add(cols ...string) {
	t.rows = append(t.rows, cols)
}

func (t *table) render(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(t.header, "\t"))
	for _, row := range t.rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	tw.Flush()
}

// dollarsOrDash formats a nullable microcurrency pointer as dollars, or "-" if unset.
func dollarsOrDash(micros *int64) string {
	if micros == nil {
		return "-"
	}
	return api.FormatMicros(*micros)
}

// orDash returns s, or "-" when empty, so table cells never collapse a column.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
