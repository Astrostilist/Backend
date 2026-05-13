package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"astroapi/config"
	"astroapi/internal/admin"
	"astroapi/internal/database"
)

const commandTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		log.Printf("superadmin init failed: %v", err)
	}
}

func run() error {

	var (
		email    string
		password string
	)

	flag.StringVar(&email, "email", os.Getenv("SUPERADMIN_EMAIL"), "SuperAdmin email, or SUPERADMIN_EMAIL")
	flag.StringVar(&password, "password", os.Getenv("SUPERADMIN_PASSWORD"), "SuperAdmin password, or SUPERADMIN_PASSWORD")
	flag.Parse()

	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	db, err := database.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close database: %v", closeErr)
		}
	}()

	repo := admin.NewPostgresRepository(db.DB)
	created, err := repo.CreateSuperAdmin(ctx, admin.CreateSuperAdminInput{
		Email:    email,
		Password: password,
	})
	if err != nil {
		if errors.Is(err, admin.ErrSuperAdminExists) {
			fmt.Println("SuperAdmin already exists, skipping initialization")
			return nil
		}
		return err
	}

	fmt.Printf("SuperAdmin created: %s\n", created.Email)
	return nil
}
