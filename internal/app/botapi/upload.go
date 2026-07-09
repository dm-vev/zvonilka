package botapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

const defaultMultipartMemory = 32 << 20

func (a *api) resolveMediaID(
	ctx context.Context,
	request *http.Request,
	botToken string,
	field string,
	kind domainmedia.MediaKind,
	value string,
) (string, error) {
	value = strings.TrimSpace(value)
	if request == nil {
		return value, domainbot.ErrInvalidInput
	}
	contentType := request.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return value, nil
	}
	if a.media == nil {
		return "", domainbot.ErrInvalidInput
	}

	memoryLimit := defaultMultipartMemory
	if a.uploadLimit > 0 && a.uploadLimit < int64(memoryLimit) {
		memoryLimit = int(a.uploadLimit)
	}
	if err := request.ParseMultipartForm(int64(memoryLimit)); err != nil {
		return "", domainbot.ErrInvalidInput
	}

	partName := field
	if strings.HasPrefix(value, "attach://") {
		partName = strings.TrimSpace(strings.TrimPrefix(value, "attach://"))
		value = ""
	}
	if partName == "" {
		partName = field
	}

	file, header, err := request.FormFile(partName)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return value, nil
		}
		return "", domainbot.ErrInvalidInput
	}
	defer file.Close()

	account, err := a.bot.GetMe(ctx, botToken)
	if err != nil {
		return "", err
	}

	mediaID, err := a.uploadMultipartFile(ctx, account.ID, kind, header, file)
	if err != nil {
		return "", err
	}

	return mediaID, nil
}

func (a *api) uploadMultipartFile(
	ctx context.Context,
	ownerAccountID string,
	kind domainmedia.MediaKind,
	header *multipart.FileHeader,
	file multipart.File,
) (string, error) {
	if a.media == nil || header == nil || file == nil {
		return "", domainbot.ErrInvalidInput
	}

	size := header.Size
	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if size < 0 {
		size = 0
	}

	var body io.Reader = file
	if size == 0 {
		raw, err := io.ReadAll(file)
		if err != nil {
			return "", domainbot.ErrInvalidInput
		}
		size = int64(len(raw))
		body = bytes.NewReader(raw)
	}

	asset, err := a.media.Upload(ctx, domainmedia.UploadParams{
		OwnerAccountID: ownerAccountID,
		Kind:           kind,
		FileName:       header.Filename,
		ContentType:    contentType,
		SizeBytes:      uint64(size),
		Metadata: map[string]string{
			domainmedia.MetadataPurposeKey: "bot_attachment",
		},
		Body: body,
	})
	if err != nil {
		return "", mapMediaError(err)
	}

	return asset.ID, nil
}

func mapMediaError(err error) error {
	switch {
	case errors.Is(err, domainmedia.ErrInvalidInput):
		return domainbot.ErrInvalidInput
	case errors.Is(err, domainmedia.ErrConflict):
		return domainbot.ErrConflict
	case errors.Is(err, domainmedia.ErrForbidden):
		return domainbot.ErrForbidden
	case errors.Is(err, domainmedia.ErrNotFound):
		return domainbot.ErrNotFound
	default:
		return fmt.Errorf("bot media upload: %w", err)
	}
}
