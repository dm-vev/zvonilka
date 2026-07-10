package gateway

import (
	"context"

	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
	domainreaction "github.com/dm-vev/zvonilka/internal/domain/reaction"
)

// GetReactionCatalog returns the authenticated user's global reaction catalog.
func (a *api) GetReactionCatalog(
	ctx context.Context,
	req *conversationv1.GetReactionCatalogRequest,
) (*conversationv1.GetReactionCatalogResponse, error) {
	if _, err := a.requireAuth(ctx); err != nil {
		return nil, err
	}

	catalog, notModified, err := a.conversation.GetReactionCatalog(ctx, req.GetKnownVersion())
	if err != nil {
		return nil, grpcError(err)
	}
	response := &conversationv1.GetReactionCatalogResponse{
		Version:      catalog.Version,
		DefaultEmoji: catalog.DefaultEmoji,
		NotModified:  notModified,
	}
	if notModified {
		return response, nil
	}

	response.Reactions = make([]*conversationv1.EmojiReactionDefinition, 0, len(catalog.Reactions))
	for _, definition := range catalog.Reactions {
		assets, assetsErr := a.conversation.ReactionAssets(ctx, definition)
		if assetsErr != nil {
			return nil, grpcError(assetsErr)
		}
		response.Reactions = append(response.Reactions, reactionDefinitionProto(definition, assets))
	}

	return response, nil
}

func reactionDefinitionProto(
	definition domainreaction.Definition,
	assets map[string]domainmedia.MediaAsset,
) *conversationv1.EmojiReactionDefinition {
	result := &conversationv1.EmojiReactionDefinition{
		Emoji:     definition.Emoji,
		Title:     definition.Title,
		Active:    definition.Active,
		SortOrder: definition.SortOrder,
	}
	result.StaticIcon = reactionAssetProto(assets[definition.StaticIcon])
	result.AppearAnimation = reactionAssetProto(assets[definition.AppearAnimation])
	result.SelectAnimation = reactionAssetProto(assets[definition.SelectAnimation])
	result.ActivateAnimation = reactionAssetProto(assets[definition.ActivateAnimation])
	result.EffectAnimation = reactionAssetProto(assets[definition.EffectAnimation])
	if definition.AroundAnimation != "" {
		result.AroundAnimation = reactionAssetProto(assets[definition.AroundAnimation])
		result.CenterAnimation = reactionAssetProto(assets[definition.CenterAnimation])
	}
	return result
}

func reactionAssetProto(asset domainmedia.MediaAsset) *conversationv1.ReactionAsset {
	return &conversationv1.ReactionAsset{
		MediaId:   asset.ID,
		MimeType:  asset.ContentType,
		Width:     asset.Width,
		Height:    asset.Height,
		SizeBytes: asset.SizeBytes,
		Sha256Hex: asset.SHA256Hex,
	}
}
