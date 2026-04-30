package kuper_test

import (
	"encoding/json"
	"strings"
	"testing"

	"kparser/internal/kuper"
)

func TestParseCategories_FlatArray(t *testing.T) {
	raw := []byte(`[
		{"id":"1001","name":"Молочная продукция"},
		{"id":"1002","name":"Хлебобулочные изделия"}
	]`)

	got, err := kuper.ParseCategories(raw)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 категории, получено %d", len(got))
	}
	if got[0].ID != "1001" || got[0].Name != "Молочная продукция" {
		t.Errorf("неверная первая категория: %+v", got[0])
	}
	if got[1].ID != "1002" || got[1].Name != "Хлебобулочные изделия" {
		t.Errorf("неверная вторая категория: %+v", got[1])
	}
}

func TestParseCategories_NestedDataObject(t *testing.T) {
	raw := []byte(`{
		"data": {
			"categories": [
				{"category_id":"43501","title":"Сыры"}
			]
		}
	}`)

	got, err := kuper.ParseCategories(raw)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 1 || got[0].ID != "43501" || got[0].Name != "Сыры" {
		t.Fatalf("неверный результат: %+v", got)
	}
}

func TestParseCategories_AlternativeFieldNames(t *testing.T) {
	raw := []byte(`{"items":[{"slug":"milk","name":"Молочная"}]}`)
	got, err := kuper.ParseCategories(raw)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 1 || got[0].ID != "milk" || got[0].Name != "Молочная" {
		t.Fatalf("неверный результат: %+v", got)
	}
}

func TestParseCategories_NumericID(t *testing.T) {
	raw := []byte(`{"data":[{"id":1001,"name":"Молочная"}]}`)
	got, err := kuper.ParseCategories(raw)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 1 || got[0].ID != "1001" {
		t.Fatalf("числовой id должен сериализоваться в строку: %+v", got)
	}
}

func TestParseCategories_EmptyArrayReturnsError(t *testing.T) {
	if _, err := kuper.ParseCategories([]byte(`{"data":[]}`)); err == nil {
		t.Fatal("ожидалась ошибка для пустого массива категорий")
	}
}

func TestParseCategories_InvalidJSONReturnsError(t *testing.T) {
	if _, err := kuper.ParseCategories([]byte(`{"broken":`)); err == nil {
		t.Fatal("ожидалась ошибка для невалидного JSON")
	}
}

func TestParseProducts_HappyPath(t *testing.T) {
	raw := []byte(`{
		"data": {
			"products": [
				{"id":"p1","name":"Молоко","price":99.9,"url":"https://example.com/p1"},
				{"id":"p2","name":"Кефир","price":"120,50"}
			]
		}
	}`)
	cat := kuper.Category{ID: "1001", Name: "Молочная продукция"}

	got, err := kuper.ParseProducts(raw, cat, "https://kuper.ru/product/{id}")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ожидалось 2 товара, получено %d", len(got))
	}

	if got[0].URL != "https://example.com/p1" {
		t.Errorf("должен использоваться url из ответа: %q", got[0].URL)
	}
	if got[1].Price != 120.50 {
		t.Errorf("цена со строкой и запятой должна парситься: %v", got[1].Price)
	}
	if got[1].URL != "https://kuper.ru/product/p2" {
		t.Errorf("должен подставляться шаблон url: %q", got[1].URL)
	}
	if got[0].CategoryID != cat.ID || got[0].CategoryName != cat.Name {
		t.Errorf("категория не пробрасывается в товар: %+v", got[0])
	}
}

func TestParseProducts_NestedPrice(t *testing.T) {
	raw := []byte(`{"products":[{"id":"x","name":"Сыр","price":{"amount":250.5}}]}`)
	got, err := kuper.ParseProducts(raw, kuper.Category{ID: "c", Name: "Сыр"}, "")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 1 || got[0].Price != 250.5 {
		t.Fatalf("вложенная цена не распарсилась: %+v", got)
	}
}

func TestParseProducts_SkipsInvalid(t *testing.T) {
	raw := []byte(`{"products":[
		{"id":"a","name":"Без цены"},
		{"id":"","name":"Без id","price":10},
		{"id":"c","name":"","price":10},
		{"id":"d","name":"Ок","price":10}
	]}`)
	got, err := kuper.ParseProducts(raw, kuper.Category{ID: "1", Name: "x"}, "u/{id}")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 1 || got[0].ID != "d" {
		t.Fatalf("должен остаться только валидный товар: %+v", got)
	}
}

func TestExtractStoreAddress(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "data.location.full_address",
			in:   `{"data":{"location":{"full_address":"г. Москва, Тверская ул., 1"}}}`,
			want: "г. Москва, Тверская ул., 1",
		},
		{
			name: "корневой full_address",
			in:   `{"location":{"full_address":"СПб, Невский, 100"}}`,
			want: "СПб, Невский, 100",
		},
		{
			name: "пустой объект",
			in:   `{}`,
			want: "",
		},
		{
			name: "массив вместо объекта",
			in:   `[]`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var node any
			if err := json.Unmarshal([]byte(tc.in), &node); err != nil {
				t.Fatalf("ошибка unmarshal: %v", err)
			}
			got := kuper.ExtractStoreAddress(node)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFirstString(t *testing.T) {
	m := map[string]any{
		"id":   float64(42),
		"name": "  Молоко  ",
	}
	if got := kuper.FirstString(m, "id"); got != "42" {
		t.Errorf("числовой id: got %q", got)
	}
	if got := kuper.FirstString(m, "name"); got != "Молоко" {
		t.Errorf("trim не сработал: got %q", got)
	}
	if got := kuper.FirstString(m, "missing", "name"); got != "Молоко" {
		t.Errorf("должен искать по приоритету: got %q", got)
	}
	if got := kuper.FirstString(m, "absent"); got != "" {
		t.Errorf("должен вернуть пустую строку: got %q", got)
	}
}

func TestFirstNumber(t *testing.T) {
	m := map[string]any{
		"price":     float64(199.5),
		"discount":  "12,30",
		"int_price": 100,
		"broken":    "abc",
	}
	if got := kuper.FirstNumber(m, "price"); got != 199.5 {
		t.Errorf("float: got %v", got)
	}
	if got := kuper.FirstNumber(m, "discount"); got != 12.30 {
		t.Errorf("string with запятой: got %v", got)
	}
	if got := kuper.FirstNumber(m, "int_price"); got != 100 {
		t.Errorf("int: got %v", got)
	}
	if got := kuper.FirstNumber(m, "broken", "price"); got != 199.5 {
		t.Errorf("должен пропустить нечисловое и взять следующее: got %v", got)
	}
	if got := kuper.FirstNumber(m, "absent"); got != 0 {
		t.Errorf("отсутствующий ключ: got %v", got)
	}
}

func TestFillTemplate(t *testing.T) {
	tmpl := "{base_url}/api/v1/content/products?store={merchant_store_id}&cat={category_id}&limit={limit}"
	out := kuper.FillTemplate(tmpl, map[string]string{
		"base_url":          "https://api.kuper.ru",
		"merchant_store_id": "4896",
		"category_id":       "43501",
		"limit":             "20",
	})
	want := "https://api.kuper.ru/api/v1/content/products?store=4896&cat=43501&limit=20"
	if out != want {
		t.Errorf("unexpected url:\n got=%q\nwant=%q", out, want)
	}

	if !strings.Contains(kuper.FillTemplate("a/{x}/b", map[string]string{"x": "значение"}), "значение") {
		t.Error("UTF-8 значения должны подставляться как есть")
	}
}

func TestJoinURL(t *testing.T) {
	cases := map[string]string{
		"https://api.kuper.ru" + "|/api/v1/x":     "https://api.kuper.ru/api/v1/x",
		"https://api.kuper.ru/" + "|/api/v1/x":    "https://api.kuper.ru/api/v1/x",
		"https://api.kuper.ru" + "|https://other": "https://other",
	}
	for in, want := range cases {
		parts := strings.SplitN(in, "|", 2)
		got := kuper.JoinURL(parts[0], parts[1])
		if got != want {
			t.Errorf("joinURL(%q,%q)=%q, want %q", parts[0], parts[1], got, want)
		}
	}
}

func TestExtractArray(t *testing.T) {
	var node any
	if err := json.Unmarshal([]byte(`{"data":{"products":[{"id":"x"}]}}`), &node); err != nil {
		t.Fatal(err)
	}
	arr, ok := kuper.ExtractArray(node, "data", "products")
	if !ok || len(arr) != 1 {
		t.Fatalf("вложенный массив не извлекся: %v", arr)
	}

	if _, ok := kuper.ExtractArray(map[string]any{"foo": "bar"}, "data"); ok {
		t.Error("ожидалось false, когда массива нет")
	}
}
