package kuper

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Product struct {
	CategoryID   string  `json:"category_id"`
	CategoryName string  `json:"category_name"`
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Price        float64 `json:"price"`
	URL          string  `json:"url"`
}
