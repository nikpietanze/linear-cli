package output

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

// Printer controls output format.
// When JSON is true, PrintJSON will be used; otherwise tabular output.
// Errors should be printed via Error to ensure non-zero exit semantics upstream.

type Printer struct {
	JSON bool
}

func (p Printer) JSONEnabled() bool { return p.JSON }

func (p Printer) PrintJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p Printer) Table(header []string, rows [][]string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	// header
	for i, h := range header {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprint(w, "\n")
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprint(w, "\n")
	}
	return w.Flush()
}

func (p Printer) PrintOrTable(header []string, rows [][]string, jsonValue interface{}) error {
	if p.JSON {
		return p.PrintJSON(jsonValue)
	}
	return p.Table(header, rows)
}

func (p Printer) PrintError(err error) {
	if p.JSON {
		_ = p.PrintJSON(map[string]interface{}{"error": err.Error()})
		return
	}
	fmt.Fprintln(os.Stderr, "Error:", err.Error())
}
