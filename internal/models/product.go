package models

type Product struct {
	SKU string
	Name string
	Description string
	Price float64
	Tags []string
	Category string
}

type ImportResult struct {
	Imported int `json:"imported"`
	Skipped int `json:"skipped"`
	Errors []string `json:"errors"`
}