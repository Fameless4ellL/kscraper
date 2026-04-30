package writer

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strconv"

	"kparser/internal/kuper"
)

func WriteCSV(path string, items []kuper.Product) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"category_id", "category_name", "name", "price", "url"}); err != nil {
		return err
	}
	for _, item := range items {
		row := []string{
			item.CategoryID,
			item.CategoryName,
			item.Name,
			strconv.FormatFloat(item.Price, 'f', 2, 64),
			item.URL,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return w.Error()
}

func WriteJSON(path string, items []kuper.Product) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(items)
}
