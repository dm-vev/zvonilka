package gateway

import (
	"context"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	searchv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/search/v1"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

// Search resolves indexed messenger entities visible to the authenticated user.
func (a *api) Search(
	ctx context.Context,
	req *searchv1.SearchRequest,
) (*searchv1.SearchResponse, error) {
	if _, err := a.requireAuth(ctx); err != nil {
		return nil, err
	}

	offset, err := decodeOffset(req.GetPage(), "search")
	if err != nil {
		return nil, grpcError(domainsearch.ErrInvalidInput)
	}

	scopes := make([]domainsearch.SearchScope, 0, len(req.GetScopes()))
	for _, scope := range req.GetScopes() {
		scopes = append(scopes, searchScopeFromProto(scope))
	}

	accountKinds := make([]string, 0, len(req.GetAccountKinds()))
	for _, kind := range req.GetAccountKinds() {
		switch kind {
		case commonv1.AccountKind_ACCOUNT_KIND_USER:
			accountKinds = append(accountKinds, string(domainidentity.AccountKindUser))
		case commonv1.AccountKind_ACCOUNT_KIND_BOT:
			accountKinds = append(accountKinds, string(domainidentity.AccountKindBot))
		}
	}

	result, err := a.search.Search(ctx, domainsearch.SearchParams{
		Query:           req.GetQuery(),
		Scopes:          scopes,
		ConversationIDs: req.GetConversationIds(),
		UserIDs:         req.GetUserIds(),
		AccountKinds:    accountKinds,
		IncludeSnippets: req.GetIncludeSnippets(),
		FuzzyMatch:      req.GetFuzzyMatch(),
		UpdatedAfter:    zeroTime(req.GetUpdatedAfter()),
		UpdatedBefore:   zeroTime(req.GetUpdatedBefore()),
		Limit:           pageSize(req.GetPage()),
		Offset:          offset,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	hits := make([]*searchv1.SearchHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, searchHit(hit))
	}

	return &searchv1.SearchResponse{
		Hits: hits,
		Page: &commonv1.PageResponse{
			NextPageToken: result.NextPageToken,
			TotalSize:     result.TotalSize,
		},
	}, nil
}
