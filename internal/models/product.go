package models

type Product struct {
	SKU string
	Name string
	Description string
	Price float64
	Tags []string
	Category string
	Rating float64
}

type ImportResult struct {
	Imported int `json:"imported"`
	Skipped int `json:"skipped"`
	Errors []string `json:"errors"`
}