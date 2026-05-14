package astroprocapi

import "context"

//go:generate mockgen -source=astro_interface.go -destination=mocks/mock_astro_interface.go -package=mocks
type AstroProc interface {
	GetAstroProfile(ctx context.Context, userID string) (AstroProfile, error)
}
