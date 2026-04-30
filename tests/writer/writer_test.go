package writer_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kparser/internal/kuper"
	"kparser/internal/writer"
)

func sampleProducts() []kuper.Product {
	return []kuper.Product{
		{
			CategoryID:   "1001",
			CategoryName: "Молочная продукция",
			ID:           "milk-001",
			Name:         "Молоко 2.5%, 900 мл",
			Price:        109.99,
			URL:          "https://kuper.ru/product/milk-001",
		},
		{
			CategoryID:   "1002",
			CategoryName: "Хлебобулочные",
			ID:           "bread-001",
			Name:         "Батон, нарезной",
			Price:        52.4,
			URL:          "https://kuper.ru/product/bread-001",
		},
	}
}

func TestWriteCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "products.csv")

	if err := writer.WriteCSV(path, sampleProducts()); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("чтение csv: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("ожидалось 3 строки (header+2), получено %d", len(rows))
	}
	want := []string{"category_id", "category_name", "name", "price", "url"}
	for i, h := range want {
		if rows[0][i] != h {
			t.Errorf("заголовок [%d]: got %q, want %q", i, rows[0][i], h)
		}
	}
	if rows[1][2] != "Молоко 2.5%, 900 мл" {
		t.Errorf("запятая в названии должна обрабатываться: %q", rows[1][2])
	}
	if rows[1][3] != "109.99" {
		t.Errorf("цена должна быть с 2 знаками: %q", rows[1][3])
	}
}

func TestWriteJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "products.json")

	if err := writer.WriteJSON(path, sampleProducts()); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []kuper.Product
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("невалидный JSON: %v\n%s", err, string(raw))
	}
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 товара, получено %d", len(got))
	}
	if got[0].Price != 109.99 {
		t.Errorf("цена не сериализуется: %v", got[0].Price)
	}
	if !strings.Contains(string(raw), "Молочная продукция") {
		t.Errorf("UTF-8 должен сохраняться без \\u-экранирования")
	}
}
