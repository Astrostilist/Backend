package importer 

import (
	"context"
	"database/sql"
    "encoding/csv"
    "encoding/json"
    "fmt"
    "io"
    "strconv"
    "strings"
    "astroapi/internal/models"
)

func RunImport(ctx context.Context, db *sql.DB, r io.Reader) (*models.ImportResult, error) {
	csvReader := csv.NewReader(r)
	csvReader.FieldsPerRecord = 6
	csvReader.ReuseRecord = true

	if _, err := csvReader.Read(); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	batchSize := 500 
	batch := make([]*models.Product, 0, batchSize)
	result := &models.ImportResult{Errors: []string{}}

	rowNum := 1

	for {
        record, err := csvReader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            result.Skipped++
            result.Errors = append(result.Errors, fmt.Sprintf("строка %d: %v", rowNum, err))
            rowNum++
            continue
        }

        prod, err := parseAndValidate(record, rowNum)
        if err != nil {
            result.Skipped++
            result.Errors = append(result.Errors, err.Error())
            rowNum++
            continue
        }

        batch = append(batch, prod)
        rowNum++

        if len(batch) == batchSize {
            if err := flushBatch(ctx, db, batch); err != nil {
                return nil, fmt.Errorf("batch insert failed: %w", err)
            }
            result.Imported += len(batch)
            batch = batch[:0]
        }
    }

    if len(batch) > 0 {
        if err := flushBatch(ctx, db, batch); err != nil {
            return nil, fmt.Errorf("final batch insert failed: %w", err)
        }
        result.Imported += len(batch)
    }

    return result, nil
}

func parseAndValidate(record []string, rowNum int) (*models.Product, error) {
    sku := strings.TrimSpace(record[0])
    if sku == "" {
        return nil, fmt.Errorf("строка %d: sku обязателен", rowNum)
    }
    name := record[1]
    description := record[2]

    price, err := strconv.ParseFloat(record[3], 64)
    if err != nil || price <= 0 {
        return nil, fmt.Errorf("строка %d: price = %q должен быть >0", rowNum, record[3])
    }

    var tags []string
    if raw := strings.TrimSpace(record[4]); raw != "" {
        if err := json.Unmarshal([]byte(raw), &tags); err != nil {
            return nil, fmt.Errorf("строка %d: tags невалидный JSON: %v", rowNum, err)
        }
    }

    category := record[5]
    return &models.Product{
        SKU: sku, Name: name, Description: description,
        Price: price, Tags: tags, Category: category,
    }, nil
}

func flushBatch(ctx context.Context, db *sql.DB, products []*models.Product) error {
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO products (sku, name, description, price, tags, category)
        VALUES ($1, $2, $3, $4, $5::jsonb, $6)
        ON CONFLICT (sku) DO UPDATE SET
            name = EXCLUDED.name,
            description = EXCLUDED.description,
            price = EXCLUDED.price,
            tags = EXCLUDED.tags,
            category = EXCLUDED.category
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, p := range products {
        tagsJSON, _ := json.Marshal(p.Tags)
        _, err := stmt.ExecContext(ctx, p.SKU, p.Name, p.Description, p.Price, string(tagsJSON), p.Category)
        if err != nil {
            return err
        }
    }
    return tx.Commit()

}