package bot

import (
	"context"
	"fmt"
	"strings"

	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

// GetFileParams describes one getFile request.
type GetFileParams struct {
	BotToken string
	FileID   string
}

// GetFile resolves one bot-visible file projection.
func (s *Service) GetFile(ctx context.Context, params GetFileParams) (File, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return File{}, err
	}

	params.FileID = strings.TrimSpace(params.FileID)
	if params.FileID == "" {
		return File{}, ErrInvalidInput
	}

	asset, err := s.media.MediaAssetByID(ctx, params.FileID)
	if err != nil {
		return File{}, fmt.Errorf("load bot file %s: %w", params.FileID, mapMediaError(err))
	}
	if asset.OwnerAccountID != account.ID {
		return File{}, ErrNotFound
	}
	if asset.Status != domainmedia.MediaStatusReady {
		return File{}, ErrNotFound
	}

	return File{
		FileID:       asset.ID,
		FileUniqueID: asset.ID,
		FileSize:     asset.SizeBytes,
		FilePath:     "/media/" + asset.ID + "/download",
	}, nil
}
